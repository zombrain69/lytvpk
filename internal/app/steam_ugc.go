package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 工坊「一键退订 / 重新订阅」（路线图第 18 项）：走 Steam 官方本机接口
// （steam_api64.dll 的 ISteamUGC —— 与游戏内订阅管理同一套接口）。
//
// 三条纪律（评估文档 docs/development/workshop-subscription-eval.md §5）：
//  1. DLL 自备 —— Valve 许可不允许随包分发 steam_api64.dll，只能在运行时加载
//     用户本机已有的副本（自动检测 / 手动指定，路径存 config.json）；
//  2. 执行前清单确认 —— 界面层负责展示「ID + 标题 + 总数 + 不可逆说明」；
//     后端在真正写之前先落一份「回滚清单」（所选 ID + 标题 + 时间）到配置目录，
//     没有回滚凭据就不动账号；
//  3. 授权不跨会话 —— 每次批量执行都要用户当次点击确认，后端不缓存授权。
//
// 与 Steam 的交互全部收敛在 steamUGCSession 接口后面：真实实现按平台构建标签
// 分离（steam_ugc_windows.go），单测注入假实现，不碰真实 Steam、不碰真实账号。

const (
	steamUGCBatchActionUnsubscribe = "unsubscribe"
	steamUGCBatchActionSubscribe   = "subscribe"
	// steamUGCConfirmTimeout 是单条操作等待 Steam 确认的上限（写验证实测 ~0.7s）。
	steamUGCConfirmTimeout = 25 * time.Second

	// steamAPIDllFileName 必须是 64 位文件名：L4D2 自带的是 32 位 steam_api.dll，
	// 64 位进程加载不了（评估文档实测结论）。
	steamAPIDllFileName = "steam_api64.dll"

	// 自动检测的扫描边界：每个游戏目录最多下探 3 层、最多访问 4000 个条目，
	// 只找第一个匹配（BFS，浅层优先）；整库候选上限 32 个。
	steamAPIDllMaxDepth      = 3
	steamAPIDllScanBudget    = 4000
	steamAPIDllCandidatesMax = 32
)

// SteamUGCStatus 是「一键退订」可用性状态（发给前端展示，不初始化 Steam、零副作用）。
type SteamUGCStatus struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	// Source：user（自定义路径）| auto（本机自动检测）| none。
	Source string `json:"source"`
	// Hint：path-missing（自定义路径已失效）| no-dll（本机没找到）| 空串。
	Hint string `json:"hint,omitempty"`
}

// SteamAPIDllCandidate 是自动检测到的一份 steam_api64.dll 副本。
type SteamAPIDllCandidate struct {
	Path string `json:"path"`
	// Game 是副本所在的游戏目录名（帮用户判断"这份 DLL 是哪来的"）。
	Game string `json:"game,omitempty"`
}

// SteamUGCBatchItemResult 是批量执行里单条的结果。
type SteamUGCBatchItemResult struct {
	PublishedFileID string `json:"publishedFileId"`
	Title           string `json:"title,omitempty"`
	OK              bool   `json:"ok"`
	Skipped         bool   `json:"skipped,omitempty"`
	// Detail 是已本地化的结果说明（"已退订" / "不在订阅列表中" / 失败原因）。
	Detail     string `json:"detail,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

// SteamUGCBatchResult 是批量退订/重订的汇总结果。
type SteamUGCBatchResult struct {
	Action string `json:"action"`
	Total  int    `json:"total"`
	// Succeeded / Skipped / Failed 三数互斥且相加等于 Total。
	Succeeded int `json:"succeeded"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
	// BackupPath 仅退订方向有值：执行前导出的回滚清单路径。
	BackupPath string                    `json:"backupPath,omitempty"`
	Results    []SteamUGCBatchItemResult `json:"results"`
}

// SteamUGCProgress 是批量执行过程中的进度事件负载（事件名 steam_ugc_progress）。
type SteamUGCProgress struct {
	Action string                  `json:"action"`
	Done   int                     `json:"done"`
	Total  int                     `json:"total"`
	Item   SteamUGCBatchItemResult `json:"item"`
}

// steamUGCSession 是 ISteamUGC 的最小操作面。真实实现见 steam_ugc_windows.go；
// 单测通过替换 openSteamUGCSession 注入假实现。
type steamUGCSession interface {
	// SubscribedItems 返回当前订阅 ID 的快照。
	SubscribedItems() ([]uint64, error)
	// SetSubscribed 执行订阅（want=true）/退订并轮询等待 Steam 确认。
	SetSubscribed(id uint64, want bool) error
	// Close 释放接口（SteamAPI_Shutdown）。
	Close()
}

// openSteamUGCSession 是会话工厂（变量形态便于单测注入；实现按平台分离）。
var openSteamUGCSession = openSteamUGCSessionImpl

// steamUGCDetector 是"本机 steam_api64.dll 自动检测"（测试注入点）。
var steamUGCDetector = func() []SteamAPIDllCandidate {
	installPath := strings.TrimSpace(readSteamInstallPathFromRegistry())
	if installPath == "" {
		return nil
	}
	return detectSteamAPIDllCandidates(installPath, os.ReadFile, os.ReadDir, fileExists)
}

// GetSteamUGCStatus 返回「一键退订」可用性（只读；自定义路径失效时保留路径信息并给出 hint）。
func (a *App) GetSteamUGCStatus() SteamUGCStatus {
	return a.steamUGCStatus()
}

func (a *App) steamUGCStatus() SteamUGCStatus {
	a.mu.RLock()
	userPath := strings.TrimSpace(a.steamAPIDllPath)
	a.mu.RUnlock()

	if userPath != "" {
		if fileExists(userPath) {
			return SteamUGCStatus{Available: true, Path: userPath, Source: "user"}
		}
		return SteamUGCStatus{Available: false, Path: userPath, Source: "user", Hint: "path-missing"}
	}
	if autoPath := a.steamUGCAutoDetectPath(false); autoPath != "" {
		return SteamUGCStatus{Available: true, Path: autoPath, Source: "auto"}
	}
	return SteamUGCStatus{Available: false, Source: "none", Hint: "no-dll"}
}

// steamUGCAutoDetectPath 返回自动检测结果（会话内缓存一次；force=true 强制重扫）。
func (a *App) steamUGCAutoDetectPath(force bool) string {
	a.mu.RLock()
	if !force && a.steamUGCAutoChecked {
		path := a.steamUGCAutoPath
		a.mu.RUnlock()
		return path
	}
	a.mu.RUnlock()

	found := ""
	if candidates := steamUGCDetector(); len(candidates) > 0 {
		found = candidates[0].Path
	}

	a.mu.Lock()
	a.steamUGCAutoPath = found
	a.steamUGCAutoChecked = true
	a.mu.Unlock()
	return found
}

// SetSteamAPIDllPath 设置/清除自定义 DLL 路径（空串 = 恢复自动检测）。成功后写回配置。
func (a *App) SetSteamAPIDllPath(path string) (SteamUGCStatus, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		if !fileExists(path) {
			return SteamUGCStatus{}, fmt.Errorf("指定的文件不存在：%s", path)
		}
		if !strings.EqualFold(filepath.Ext(path), ".dll") {
			return SteamUGCStatus{}, fmt.Errorf("请选择 .dll 文件（steam_api64.dll）")
		}
	}

	a.mu.Lock()
	a.steamAPIDllPath = path
	// 自定义路径变更后旧自动检测缓存不再有意义，下次按需重扫。
	a.steamUGCAutoChecked = false
	a.steamUGCAutoPath = ""
	a.mu.Unlock()

	a.saveConfig()
	return a.GetSteamUGCStatus(), nil
}

// DetectSteamAPIDllCandidates 扫描本机 Steam 游戏目录，列出可用的 steam_api64.dll 副本。
// 只读文件系统；找不到任何副本时返回错误（前端据此提示手动指定）。
func (a *App) DetectSteamAPIDllCandidates() ([]SteamAPIDllCandidate, error) {
	candidates := steamUGCDetector()

	a.mu.Lock()
	a.steamUGCAutoChecked = true
	if len(candidates) > 0 {
		a.steamUGCAutoPath = candidates[0].Path
	} else {
		a.steamUGCAutoPath = ""
	}
	a.mu.Unlock()

	if len(candidates) == 0 {
		return nil, fmt.Errorf("本机 Steam 游戏目录里没有找到 steam_api64.dll，请用「手动指定…」选择一份")
	}
	return candidates, nil
}

// PickSteamAPIDllFile 打开系统文件选择器挑一份 steam_api64.dll。
// 只返回路径，是否采用由前端再调 SetSteamAPIDllPath（用户取消时返回空串，不是错误）。
func (a *App) PickSteamAPIDllFile() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用未就绪")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 steam_api64.dll",
		Filters: []runtime.FileFilter{
			{DisplayName: "Steam API DLL (steam_api64.dll)", Pattern: steamAPIDllFileName},
			{DisplayName: "所有 DLL 文件 (*.dll)", Pattern: "*.dll"},
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(path), nil
}

// UnsubscribeSteamWorkshopItems 通过官方本机接口退订所选订阅。
// 写前先导出回滚清单（配置目录）；逐条执行、轮询确认，逐条结果返回前端。
func (a *App) UnsubscribeSteamWorkshopItems(ids []string) (SteamUGCBatchResult, error) {
	return a.runSteamUGCBatch(steamUGCBatchActionUnsubscribe, ids)
}

// SubscribeSteamWorkshopItems 重新订阅所选条目（回滚入口；对已在订阅列表里的自动跳过）。
func (a *App) SubscribeSteamWorkshopItems(ids []string) (SteamUGCBatchResult, error) {
	return a.runSteamUGCBatch(steamUGCBatchActionSubscribe, ids)
}

func (a *App) runSteamUGCBatch(action string, ids []string) (SteamUGCBatchResult, error) {
	targets, err := normalizeSteamUGCIDList(ids)
	if err != nil {
		return SteamUGCBatchResult{}, err
	}

	status := a.steamUGCStatus()
	if !status.Available {
		if status.Hint == "path-missing" {
			return SteamUGCBatchResult{}, fmt.Errorf("自定义的 steam_api64.dll 路径已失效：%s（请重新指定）", status.Path)
		}
		return SteamUGCBatchResult{}, fmt.Errorf("未找到可用的 steam_api64.dll——先点「检测本机 DLL」或「手动指定…」选一份（来自任意 64 位 Steam 游戏目录；Valve 许可不允许应用随包分发）")
	}

	a.mu.Lock()
	if a.steamUGCBusy {
		a.mu.Unlock()
		return SteamUGCBatchResult{}, fmt.Errorf("已有批量操作正在进行，请等它结束")
	}
	a.steamUGCBusy = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.steamUGCBusy = false
		a.mu.Unlock()
	}()

	result := SteamUGCBatchResult{
		Action:  action,
		Total:   len(targets),
		Results: make([]SteamUGCBatchItemResult, 0, len(targets)),
	}

	// 标题随结果返回：退订成功后订阅快照里这些条目就没了，界面仍要能显示"退的是谁"。
	titles := a.workshopIDTitles(targets)

	// 退订前先落回滚清单；写不下去就整体中止（没有回滚凭据就不动账号）。
	if action == steamUGCBatchActionUnsubscribe {
		backupPath, backupErr := a.writeUnsubscribeBackup(targets, titles)
		if backupErr != nil {
			return SteamUGCBatchResult{}, fmt.Errorf("写入回滚清单失败，已中止退订：%w", backupErr)
		}
		result.BackupPath = backupPath
	}

	session, openErr := openSteamUGCSession(status.Path)
	if openErr != nil {
		return SteamUGCBatchResult{}, fmt.Errorf("初始化 Steam 接口失败：%v（请确认 Steam 客户端正在运行且已登录）", openErr)
	}
	defer session.Close()

	subscribed, listErr := session.SubscribedItems()
	if listErr != nil {
		return SteamUGCBatchResult{}, fmt.Errorf("读取当前订阅列表失败：%v", listErr)
	}
	current := make(map[uint64]bool, len(subscribed))
	for _, id := range subscribed {
		current[id] = true
	}

	want := action == steamUGCBatchActionSubscribe
	for index, target := range targets {
		item := SteamUGCBatchItemResult{
			PublishedFileID: strconv.FormatUint(target, 10),
		}
		if title, ok := titles[item.PublishedFileID]; ok {
			item.Title = title
		}

		if current[target] == want {
			// 已经是目标状态：不执行、如实跳过。
			item.OK = true
			item.Skipped = true
			if want {
				item.Detail = "已在订阅列表中"
			} else {
				item.Detail = "不在订阅列表中"
			}
		} else {
			started := time.Now()
			if setErr := session.SetSubscribed(target, want); setErr != nil {
				item.Detail = setErr.Error()
			} else {
				item.OK = true
				item.DurationMs = time.Since(started).Milliseconds()
				if want {
					item.Detail = "已重新订阅"
				} else {
					item.Detail = "已退订"
				}
				current[target] = want
			}
		}

		switch {
		case !item.OK:
			result.Failed++
		case item.Skipped:
			result.Skipped++
		default:
			result.Succeeded++
		}
		result.Results = append(result.Results, item)
		a.emitEvent("steam_ugc_progress", SteamUGCProgress{
			Action: action,
			Done:   index + 1,
			Total:  len(targets),
			Item:   item,
		})
	}
	return result, nil
}

// normalizeSteamUGCIDList 校验并去重工坊 ID 列表（保持原有顺序）。
func normalizeSteamUGCIDList(ids []string) ([]uint64, error) {
	seen := make(map[uint64]bool, len(ids))
	targets := make([]uint64, 0, len(ids))
	for _, raw := range ids {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		id, err := strconv.ParseUint(value, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("无效的工坊 ID：%q", raw)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("没有选择任何条目")
	}
	return targets, nil
}

// workshopIDTitles 从扫描缓存收集所选 ID 的标题（拿不到就留空，只影响展示）。
func (a *App) workshopIDTitles(targets []uint64) map[string]string {
	titles := make(map[string]string, len(targets))
	rootDir := strings.TrimSpace(a.rootDirectorySnapshot())
	if rootDir == "" {
		return titles
	}
	all := a.collectWorkshopFileTitles(rootDir)
	for _, target := range targets {
		key := strconv.FormatUint(target, 10)
		if title, ok := all[key]; ok {
			titles[key] = title
		}
	}
	return titles
}

// unsubscribeBackupPayload 是退订回滚清单的文件结构（写入配置目录，不属于仓库数据）。
type unsubscribeBackupPayload struct {
	Version   int                     `json:"version"`
	Kind      string                  `json:"kind"`
	CreatedAt string                  `json:"createdAt"`
	AccountID string                  `json:"accountId,omitempty"`
	Count     int                     `json:"count"`
	Items     []unsubscribeBackupItem `json:"items"`
}

type unsubscribeBackupItem struct {
	PublishedFileID string `json:"publishedFileId"`
	Title           string `json:"title,omitempty"`
}

// writeUnsubscribeBackup 在执行退订前导出所选项清单（便于日后「重新订阅所选」对照回滚）。
func (a *App) writeUnsubscribeBackup(targets []uint64, titles map[string]string) (string, error) {
	a.ensureConfigPaths()
	configDir := strings.TrimSpace(a.configDir)
	if configDir == "" {
		return "", fmt.Errorf("配置目录不可用")
	}

	accountID := ""
	if installPath := strings.TrimSpace(readSteamInstallPathFromRegistry()); installPath != "" {
		if archive, ok := findBestSubscriptionArchive(installPath, listSubdirectories, os.ReadFile); ok {
			accountID = archive.AccountID
		}
	}

	payload := unsubscribeBackupPayload{
		Version:   1,
		Kind:      "lytvpk-workshop-unsubscribe-backup",
		CreatedAt: time.Now().Format(time.RFC3339),
		AccountID: accountID,
		Count:     len(targets),
		Items:     make([]unsubscribeBackupItem, 0, len(targets)),
	}
	for _, target := range targets {
		key := strconv.FormatUint(target, 10)
		payload.Items = append(payload.Items, unsubscribeBackupItem{
			PublishedFileID: key,
			Title:           titles[key],
		})
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')

	fileName := "workshop_unsubscribe_backup_" + time.Now().Format("20060102_150405") + ".json"
	path := filepath.Join(configDir, fileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// detectSteamAPIDllCandidates 扫描 Steam 库（安装目录 + libraryfolders.vdf 里的各库），
// 在每个游戏的 `steamapps\common\<game>` 目录下按 BFS 找第一份 steam_api64.dll。
// 依赖注入（readFile/listDir/isFile）便于单测不碰真实磁盘。
func detectSteamAPIDllCandidates(
	installPath string,
	readFile func(string) ([]byte, error),
	listDir func(string) ([]os.DirEntry, error),
	isFile func(string) bool,
) []SteamAPIDllCandidate {
	installPath = strings.TrimSpace(installPath)
	if installPath == "" {
		return nil
	}
	libraries := make([]string, 0, 4)
	libraries = append(libraries, installPath)
	libraries = append(libraries, readSteamLibraryFolders(installPath, readFile)...)

	seen := make(map[string]bool)
	candidates := make([]SteamAPIDllCandidate, 0, 8)
	for _, library := range libraries {
		commonDir := filepath.Join(library, "steamapps", "common")
		games, err := listDir(commonDir)
		if err != nil {
			continue
		}
		for _, game := range games {
			if !game.IsDir() {
				continue
			}
			gameDir := filepath.Join(commonDir, game.Name())
			path := findSteamAPIDllInDir(gameDir, listDir, isFile)
			if path == "" {
				continue
			}
			key := strings.ToLower(filepath.Clean(path))
			if seen[key] {
				continue
			}
			seen[key] = true
			candidates = append(candidates, SteamAPIDllCandidate{Path: path, Game: game.Name()})
			if len(candidates) >= steamAPIDllCandidatesMax {
				return candidates
			}
		}
	}
	return candidates
}

// findSteamAPIDllInDir 在游戏目录内 BFS（深度 ≤ steamAPIDllMaxDepth、条目预算内）
// 找第一份 steam_api64.dll；浅层优先，找不到返回空串。
func findSteamAPIDllInDir(rootDir string, listDir func(string) ([]os.DirEntry, error), isFile func(string) bool) string {
	type queueItem struct {
		dir   string
		depth int
	}
	queue := []queueItem{{dir: rootDir}}
	visited := 0
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		direct := filepath.Join(item.dir, steamAPIDllFileName)
		if isFile(direct) {
			return direct
		}
		if item.depth >= steamAPIDllMaxDepth {
			continue
		}
		entries, err := listDir(item.dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			visited++
			if visited > steamAPIDllScanBudget {
				return ""
			}
			if entry.IsDir() {
				queue = append(queue, queueItem{dir: filepath.Join(item.dir, entry.Name()), depth: item.depth + 1})
			}
		}
	}
	return ""
}
