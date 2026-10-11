package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"vpk-manager/internal/platform/protocol"
)

// 工坊快捷方式（.url）生成（路线图第 7 项）。
//
// 用途与参考实现（sHartet/l4d2-addons 的整理流程）一致：工坊 VPK 被重命名 /
// 搬出 workshop\ 之后，数字 ID 从文件名里消失，双击直达工坊页的入口就没了。
// 在 VPK 旁边生成同名 .url（在资源管理器里双击直接打开工坊页），ID 线索随文件走。
//
// 文件格式与参考实现相同（纯 ASCII、CRLF 三行）：
//
//	[InternetShortcut]
//	URL=https://steamcommunity.com/sharedfiles/filedetails/?id=<id>
//	IconIndex=0
//
// ID 解析链（与前端 modals/detail.js 的 resolveWorkshopID 同序）：
//  1. 同名 .meta 里的 workshop_id；
//  2. 文件位于 workshop\ 且文件名主干是有效工坊 ID（Steam 官方投递的原样命名）；
//  3. 扫描缓存中 addoninfo.txt 解析出的 addonURL0。
//
// 幂等纪律：已有 .url 指向同一 ID 时不动原文件（用户从浏览器拖出来的完整版
// .url 带 IDList 等字段，不该被“统一格式”改写）；指向别的 ID（或解析不出
// ID）时才改写并计入 Updated。

// WorkshopShortcutSkip 记录单个文件没能生成快捷方式的原因。
type WorkshopShortcutSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// WorkshopShortcutResult 是一次「生成工坊快捷方式」的统计结果。
type WorkshopShortcutResult struct {
	Requested int                    `json:"requested"`
	Created   int                    `json:"created"`
	Updated   int                    `json:"updated"`
	Unchanged int                    `json:"unchanged"`
	Skipped   []WorkshopShortcutSkip `json:"skipped,omitempty"`
	Errors    []string               `json:"errors,omitempty"`
}

// 快捷方式内容里 / 既有文件里出现的工坊 ID（`id=123` 写法，大小写不敏感）。
var workshopShortcutIDPattern = regexp.MustCompile(`(?i)\bid=(\d+)`)

// GenerateWorkshopShortcuts 为一批受管 Mod 生成工坊快捷方式（.url）。
//
// 只读纪律：这是纯新增/更新旁车文件的操作，不移动、不删除任何 VPK；无法确定
// 工坊 ID 的文件逐项跳过并在结果里说明原因，绝不猜测。
func (a *App) GenerateWorkshopShortcuts(filePaths []string) (WorkshopShortcutResult, error) {
	result := WorkshopShortcutResult{}
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return result, fmt.Errorf("未选择L4D2目录")
	}
	if len(filePaths) == 0 {
		return result, fmt.Errorf("没有可处理的文件")
	}
	// 沙箱只读闸门：生成 .url 会往真实库目录写文件。
	if err := rejectReadonlyLibraryWrite("生成工坊快捷方式"); err != nil {
		return result, err
	}

	seen := make(map[string]bool, len(filePaths))
	for _, raw := range filePaths {
		filePath := strings.TrimSpace(raw)
		if filePath == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(filePath))
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Requested++

		if !strings.EqualFold(filepath.Ext(filePath), ".vpk") {
			result.Skipped = append(result.Skipped, WorkshopShortcutSkip{Path: filePath, Reason: "不是 .vpk 文件"})
			continue
		}
		if !pathWithinRoot(rootDir, filePath) {
			result.Skipped = append(result.Skipped, WorkshopShortcutSkip{Path: filePath, Reason: "不在受管 addons 目录内"})
			continue
		}
		if info, err := os.Stat(filePath); err != nil || info.IsDir() {
			result.Skipped = append(result.Skipped, WorkshopShortcutSkip{Path: filePath, Reason: "文件不存在或不可读"})
			continue
		}

		workshopID := a.resolveWorkshopShortcutID(rootDir, filePath)
		if workshopID == "" {
			result.Skipped = append(result.Skipped, WorkshopShortcutSkip{
				Path:   filePath,
				Reason: "无法确定工坊 ID（无 .meta、文件名非纯数字、addoninfo.txt 未声明链接）",
			})
			continue
		}

		status, err := writeWorkshopShortcutFile(workshopShortcutPathFor(filePath), workshopID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", filepath.Base(filePath), err))
			continue
		}
		switch status {
		case workshopShortcutCreated:
			result.Created++
		case workshopShortcutUpdated:
			result.Updated++
		default:
			result.Unchanged++
		}
	}

	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].Path < result.Skipped[j].Path })
	sort.Strings(result.Errors)
	return result, nil
}

// resolveWorkshopShortcutID 按解析链取工坊 ID，取到第一个通过校验的候选就返回。
func (a *App) resolveWorkshopShortcutID(rootDir, filePath string) string {
	// 1. 同名 .meta 的 workshop_id。
	if meta, err := LoadWorkshopMeta(filePath); meta != nil && err == nil {
		if id := normalizeWorkshopShortcutID(meta.WorkshopID); id != "" {
			return id
		}
	}

	// 2. workshop\ 下的纯数字文件名（Steam 官方投递的原样命名）。
	//    隐藏（_ 前缀）的文件名里 ID 仍然有效，先剥掉隐藏前缀再校验。
	if rel, err := filepath.Rel(rootDir, filePath); err == nil {
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) >= 2 && strings.EqualFold(parts[0], "workshop") {
			stem := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			if id := normalizeWorkshopShortcutID(strings.TrimLeft(stem, "_")); id != "" {
				return id
			}
		}
	}

	// 3. addonURL0（仅在扫描缓存可用时；缓存缺失不额外解析 VPK 内容）。
	if cached, ok := a.vpkCache.Load(filePath); ok {
		if cache, ok := cached.(*VPKFileCache); ok {
			if id := workshopIDFromAddonURL(cache.File.AddonURL0); id != "" {
				return id
			}
		}
	}
	return ""
}

// normalizeWorkshopShortcutID 清洗候选 ID：去空白、排除 direct- 前缀，必须是
// 有效工坊 ID（纯数字且 ≥ 100000，与 protocol.IsValidWorkshopID 同口径）。
func normalizeWorkshopShortcutID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" || strings.HasPrefix(strings.ToLower(id), "direct-") {
		return ""
	}
	if !protocol.IsValidWorkshopID(id) {
		return ""
	}
	return id
}

// workshopIDFromAddonURL 从 addoninfo.txt 的 addonURL0 里取工坊 ID
// （认 `...?id=123...` 链接，也认纯数字写法）。
func workshopIDFromAddonURL(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if id := normalizeWorkshopShortcutID(text); id != "" {
		return id
	}
	match := workshopShortcutIDPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return normalizeWorkshopShortcutID(match[1])
}

// workshopShortcutPathFor 返回与 VPK 同目录、同主干的 .url 路径。
func workshopShortcutPathFor(vpkPath string) string {
	return strings.TrimSuffix(vpkPath, filepath.Ext(vpkPath)) + ".url"
}

type workshopShortcutWriteStatus int

const (
	workshopShortcutUnchanged workshopShortcutWriteStatus = iota
	workshopShortcutCreated
	workshopShortcutUpdated
)

// writeWorkshopShortcutFile 幂等写入 .url：内容已指向同一 ID 时原样保留；
// 指向别的 ID 或解析不出 ID 时改写为新内容。
func writeWorkshopShortcutFile(urlPath, workshopID string) (workshopShortcutWriteStatus, error) {
	content := "[InternetShortcut]\r\nURL=" + workshopItemLinkBase + workshopID + "\r\nIconIndex=0\r\n"
	existing, err := os.ReadFile(urlPath)
	if err == nil {
		if workshopShortcutRefersToID(string(existing), workshopID) {
			return workshopShortcutUnchanged, nil
		}
		return workshopShortcutUpdated, os.WriteFile(urlPath, []byte(content), 0o644)
	}
	if !os.IsNotExist(err) {
		return workshopShortcutUnchanged, err
	}
	return workshopShortcutCreated, os.WriteFile(urlPath, []byte(content), 0o644)
}

// workshopShortcutRefersToID 判断既有 .url 内容里是否引用了给定的工坊 ID
// （用 `id=<数字>` 提取后做整串比较，避免 id=12 误配 id=123）。
func workshopShortcutRefersToID(content, workshopID string) bool {
	for _, match := range workshopShortcutIDPattern.FindAllStringSubmatch(content, -1) {
		if match[1] == workshopID {
			return true
		}
	}
	return false
}
