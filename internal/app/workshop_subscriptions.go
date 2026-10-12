package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// 工坊订阅清单（退订引导 · 只读）：
// 解析 Steam 的 550_subscriptions.vdf（<Steam>\userdata\<账号>\ugc\），
// 与当前会话的 addons 目录对照分类：已下载 / 已转存 addons / 无本地文件 / Steam 本地停用。
//
// 纪律：本功能**零写入** —— 不碰任何 Mod 文件、不动 addonlist.txt、不写 Steam 数据；
// 退订动作全部引导到 Steam 官方页面（应用内不执行退订）。

// WorkshopSubscription 一条工坊订阅（来源：本机 550_subscriptions.vdf 快照）。
type WorkshopSubscription struct {
	PublishedFileID string `json:"publishedFileId"`
	TimeSubscribed  int64  `json:"timeSubscribed"`
	DisabledLocally bool   `json:"disabledLocally"`
	// HasWorkshopFile：addons\workshop\<id>.vpk 存在（Steam 已下载）。
	HasWorkshopFile bool `json:"hasWorkshopFile"`
	// HasRootCopy：addons\<id>.vpk 存在（同名副本，通常是从工坊转存到根目录后的状态）。
	HasRootCopy bool `json:"hasRootCopy"`
	// HasDisabledCopy：addons\disabled\<id>.vpk 存在（工具侧禁用副本）。
	HasDisabledCopy bool `json:"hasDisabledCopy"`
	// Title 来自扫描缓存（对应工坊 VPK 已解析的标题）；解析不到时为空。
	Title string `json:"title,omitempty"`
}

// WorkshopSubscriptionsSummary 订阅清单汇总（找不到数据源时 Found=false + Reason）。
type WorkshopSubscriptionsSummary struct {
	Found           bool                    `json:"found"`
	Reason          string                  `json:"reason,omitempty"` // no-steam | no-vdf
	AccountID       string                  `json:"accountId,omitempty"`
	TimeLastUpdated int64                   `json:"timeLastUpdated"`
	Total           int                     `json:"total"`
	Downloaded      int                     `json:"downloaded"`
	RootCopies      int                     `json:"rootCopies"`
	Missing         int                     `json:"missing"`
	LocallyDisabled int                     `json:"locallyDisabled"`
	Entries         []WorkshopSubscription  `json:"entries"`
}

// subscriptionArchive 一份解析完成的订阅档（某个 Steam 账号的 550_subscriptions.vdf）。
type subscriptionArchive struct {
	AccountID string
	Updated   int64
	Entries   []WorkshopSubscription
}

// parseSubscriptionVDF 解析 550_subscriptions.vdf 的引号词法。
//
// 文件结构（Steam 实际写出，字段顺序固定）：
//
//	"subscribedfiles"
//	{
//		"appid"              "550"
//		"time_last_updated"  "1791328600"
//		"0" { "publishedfileid" "...", "time_subscribed" "...", "disabled_locally" "0" }
//		...
//	}
//
// 逐 token 扫：数值 value 不会被误认为 key（关键字全是字母串），因此不需要严格的
// 块结构解析；对缺字段的条目也尽量宽容（缺什么就用零值）。
func parseSubscriptionVDF(content string) (int64, []WorkshopSubscription) {
	tokens := make([]string, 0, 4096)
	for _, match := range vdfStringPattern.FindAllStringSubmatch(content, -1) {
		tokens = append(tokens, unescapeVDFString(match[1]))
	}

	var updated int64
	entries := make([]WorkshopSubscription, 0, 64)
	for i := 0; i < len(tokens)-1; i++ {
		key := strings.TrimSpace(tokens[i])
		value := strings.TrimSpace(tokens[i+1])
		switch {
		case strings.EqualFold(key, "time_last_updated"):
			if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
				updated = parsed
			}
		case strings.EqualFold(key, "publishedfileid"):
			if value != "" {
				entries = append(entries, WorkshopSubscription{PublishedFileID: value})
			}
		case strings.EqualFold(key, "time_subscribed"):
			if len(entries) > 0 {
				if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
					entries[len(entries)-1].TimeSubscribed = parsed
				}
			}
		case strings.EqualFold(key, "disabled_locally"):
			if len(entries) > 0 {
				entries[len(entries)-1].DisabledLocally = value == "1"
			}
		}
	}
	return updated, entries
}

// findBestSubscriptionArchive 在 <Steam>\userdata\ 的各个账号目录里找 550_subscriptions.vdf，
// 取 time_last_updated 最新的一份（多账号时以最近同步的为准）。
// listDirs / readFile 是注入点，测试不碰真实磁盘。
func findBestSubscriptionArchive(steamInstall string, listDirs func(string) ([]string, error), readFile func(string) ([]byte, error)) (subscriptionArchive, bool) {
	steamInstall = strings.TrimSpace(steamInstall)
	if steamInstall == "" {
		return subscriptionArchive{}, false
	}
	userdataDir := filepath.Join(steamInstall, "userdata")
	accounts, err := listDirs(userdataDir)
	if err != nil {
		return subscriptionArchive{}, false
	}

	var best subscriptionArchive
	found := false
	for _, account := range accounts {
		account = strings.TrimSpace(account)
		if account == "" {
			continue
		}
		content, readErr := readFile(filepath.Join(userdataDir, account, "ugc", "550_subscriptions.vdf"))
		if readErr != nil {
			continue
		}
		updated, entries := parseSubscriptionVDF(string(content))
		if !found || updated > best.Updated {
			best = subscriptionArchive{AccountID: account, Updated: updated, Entries: entries}
			found = true
		}
	}
	return best, found
}

// listSubdirectories 列出目录下的一级子目录名（注入点默认实现）。
func listSubdirectories(dir string) ([]string, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.IsDir() {
			names = append(names, item.Name())
		}
	}
	return names, nil
}

// applySubscriptionLocalState 给每条订阅对照本地文件与标题（注入点，测试不碰磁盘）。
func applySubscriptionLocalState(entries []WorkshopSubscription, rootDir string, statFile func(string) bool, titles map[string]string) {
	rootDir = strings.TrimSpace(rootDir)
	for i := range entries {
		id := strings.TrimSpace(entries[i].PublishedFileID)
		if id == "" || rootDir == "" {
			continue
		}
		entries[i].HasWorkshopFile = statFile(filepath.Join(rootDir, "workshop", id+".vpk"))
		entries[i].HasRootCopy = statFile(filepath.Join(rootDir, id+".vpk"))
		entries[i].HasDisabledCopy = statFile(filepath.Join(rootDir, "disabled", id+".vpk"))
		if titles != nil {
			if title, ok := titles[id]; ok && strings.TrimSpace(title) != "" {
				entries[i].Title = strings.TrimSpace(title)
			}
		}
	}
}

// countSubscriptionCategories 汇总四类计数（"无本地文件"= 三个位置都没有）。
func countSubscriptionCategories(entries []WorkshopSubscription) (downloaded, rootCopies, missing, locallyDisabled int) {
	for _, entry := range entries {
		if entry.HasWorkshopFile {
			downloaded++
		}
		if entry.HasRootCopy {
			rootCopies++
		}
		if !entry.HasWorkshopFile && !entry.HasRootCopy && !entry.HasDisabledCopy {
			missing++
		}
		if entry.DisabledLocally {
			locallyDisabled++
		}
	}
	return downloaded, rootCopies, missing, locallyDisabled
}

// collectWorkshopFileTitles 从扫描缓存收集「工坊文件名（数字 ID）→ 标题」。
// 缓存键是完整文件路径；只认 addons\workshop 下、文件名为纯数字的 .vpk。
func (a *App) collectWorkshopFileTitles(rootDir string) map[string]string {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil
	}
	workshopDir := filepath.Join(rootDir, "workshop")
	titles := map[string]string{}
	a.vpkCache.Range(func(cacheKey, cacheValue interface{}) bool {
		path, ok := cacheKey.(string)
		if !ok {
			return true
		}
		if !strings.HasPrefix(strings.ToLower(path), strings.ToLower(workshopDir)) {
			return true
		}
		base := filepath.Base(path)
		if !strings.EqualFold(filepath.Ext(base), ".vpk") {
			return true
		}
		id := strings.TrimSuffix(base, filepath.Ext(base))
		if !digitsOnly(id) {
			return true
		}
		cache, ok := cacheValue.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		if title := strings.TrimSpace(cache.File.Title); title != "" {
			titles[id] = title
		}
		return true
	})
	return titles
}

// digitsOnly 判断字符串是否为非空纯数字（工坊 ID 命名约定）。
func digitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// GetWorkshopSubscriptions 读取本机工坊订阅清单并对照当前 addons 目录分类。
// 只读操作：没有任何写入路径。数据源找不到时返回 Found=false 与原因码。
func (a *App) GetWorkshopSubscriptions() (WorkshopSubscriptionsSummary, error) {
	installPath := strings.TrimSpace(readSteamInstallPathFromRegistry())
	if installPath == "" {
		return WorkshopSubscriptionsSummary{Found: false, Reason: "no-steam"}, nil
	}
	archive, ok := findBestSubscriptionArchive(installPath, listSubdirectories, os.ReadFile)
	if !ok {
		return WorkshopSubscriptionsSummary{Found: false, Reason: "no-vdf"}, nil
	}

	entries := archive.Entries
	rootDir := strings.TrimSpace(a.rootDirectorySnapshot())
	if rootDir != "" {
		statFile := func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		}
		applySubscriptionLocalState(entries, rootDir, statFile, a.collectWorkshopFileTitles(rootDir))
	}

	downloaded, rootCopies, missing, locallyDisabled := countSubscriptionCategories(entries)
	return WorkshopSubscriptionsSummary{
		Found:           true,
		AccountID:       archive.AccountID,
		TimeLastUpdated: archive.Updated,
		Total:           len(entries),
		Downloaded:      downloaded,
		RootCopies:      rootCopies,
		Missing:         missing,
		LocallyDisabled: locallyDisabled,
		Entries:         entries,
	}, nil
}
