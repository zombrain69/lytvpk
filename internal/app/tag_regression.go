package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vpk-manager/internal/grouping"
)

// 标签回归护栏。
//
// 背景（用户硬约束）：Mod 标签识别**可以多标，不能少标**。任何规则改动都必须在
// 真机全量清单上证明"没有 Mod 少掉任何一个标签"。本文件提供两件事：
//
//  1. ExportTagBaseline：把当前扫描结果压成最小基线（entryId → 标签集合），
//     只保留判定召回所需字段，便于长期保存与逐行 diff。
//  2. CheckTagRegression：重新扫描并与基线逐 Mod 比对。**新增标签放行**；
//     消失的标签一律失败，除非它出现在 allowlist 里（用于修正"已确认的错误标签"，
//     例如 scripts/weapon_shotgun_chrome.txt 证明 w_pumpshotgun_A.mdl 是铁喷而不是木喷）。
//
// 设计要点：diff 是纯函数（不碰文件系统），App 上的两个方法只负责取数与写盘，
// 这样 CI 无需安装游戏即可覆盖判定逻辑。

const (
	tagBaselineVersion   = 1
	tagRegressionVersion = 1
	tagAllowlistVersion  = 1
	tagBaselineGenerator = "LytVPK"
)

type tagBaselineFile struct {
	Version     int              `json:"version"`
	GeneratedAt string           `json:"generatedAt"`
	Generator   string           `json:"generator"`
	ModCount    int              `json:"modCount"`
	Notes       string           `json:"notes,omitempty"`
	Mods        []tagBaselineMod `json:"mods"`
}

// tagBaselineMod 只保存"判定少标"必需的字段。
// entryId 是稳定身份（location/相对路径），name 仅用于人工阅读与规则定位。
type tagBaselineMod struct {
	EntryID       string   `json:"entryId"`
	Name          string   `json:"name,omitempty"`
	PrimaryTag    string   `json:"primaryTag,omitempty"`
	SecondaryTags []string `json:"secondaryTags"`
}

// tagRegressionAllowlist 是"已确认错误标签"的显式登记表。
// 没有登记就不许删标签 —— 这是硬约束的机器表达。
type tagRegressionAllowlist struct {
	Version int                       `json:"version"`
	Entries []tagRegressionAllowEntry `json:"entries"`
}

type tagRegressionAllowEntry struct {
	// EntryID 与 Name 二者至少填一个：EntryID 精确匹配，Name 匹配文件名。
	EntryID string `json:"entryId,omitempty"`
	Name    string `json:"name,omitempty"`
	// Scope="global" 表示这条放行对该标签下的**所有 Mod** 生效，
	// 只用于「本体证据已证明是错误标签」的整类修正；reason 与 evidence 必须都填写，
	// 否则本条不生效（失败关闭：宁可报错，也不让写坏的登记静默放行）。
	Scope    string `json:"scope,omitempty"`
	Tag      string `json:"tag"`
	Reason   string `json:"reason"`
	Evidence string `json:"evidence,omitempty"`
}

type tagRegressionChange struct {
	EntryID     string   `json:"entryId"`
	Name        string   `json:"name,omitempty"`
	Added       []string `json:"added,omitempty"`
	Removed     []string `json:"removed,omitempty"`
	Allowlisted []string `json:"allowlisted,omitempty"`
	Unexcused   []string `json:"unexcused,omitempty"`
}

type tagRegressionReport struct {
	Version             int                   `json:"version"`
	GeneratedAt         string                `json:"generatedAt"`
	BaselineGeneratedAt string                `json:"baselineGeneratedAt,omitempty"`
	BaselineMods        int                   `json:"baselineMods"`
	CurrentMods         int                   `json:"currentMods"`
	AddedTags           int                   `json:"addedTags"`
	RemovedTags         int                   `json:"removedTags"`
	AllowlistedRemovals int                   `json:"allowlistedRemovals"`
	MissingMods         []string              `json:"missingMods,omitempty"`
	NewMods             []string              `json:"newMods,omitempty"`
	Changes             []tagRegressionChange `json:"changes,omitempty"`
	OK                  bool                  `json:"ok"`
	Notes               string                `json:"notes,omitempty"`
}

// buildTagBaseline 把 Mod 列表压成基线。entryID 由调用方给出（依赖 App 的路径规则）。
func buildTagBaseline(mods []grouping.Mod, entryID func(grouping.Mod) string) tagBaselineFile {
	entries := make([]tagBaselineMod, 0, len(mods))
	for _, mod := range mods {
		entries = append(entries, tagBaselineMod{
			EntryID:       entryID(mod),
			Name:          mod.Name,
			PrimaryTag:    mod.PrimaryTag,
			SecondaryTags: normalizedTagSlice(mod.SecondaryTags),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].EntryID < entries[j].EntryID })

	return tagBaselineFile{
		Version:     tagBaselineVersion,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Generator:   tagBaselineGenerator + " " + AppVersion,
		ModCount:    len(entries),
		Notes: "标签召回基线：只记录 entryId → 标签集合。回归检查只允许新增标签，" +
			"消失的标签必须登记进 allowlist。",
		Mods: entries,
	}
}

// diffTagBaseline 是纯 diff：新增放行，消失必须被 allowlist 解释。
func diffTagBaseline(baseline tagBaselineFile, current []tagBaselineMod, allowlist tagRegressionAllowlist) tagRegressionReport {
	report := tagRegressionReport{
		Version:             tagRegressionVersion,
		GeneratedAt:         time.Now().Format(time.RFC3339),
		BaselineGeneratedAt: baseline.GeneratedAt,
		BaselineMods:        len(baseline.Mods),
		CurrentMods:         len(current),
	}

	currentByID := make(map[string]tagBaselineMod, len(current))
	for _, mod := range current {
		currentByID[mod.EntryID] = mod
	}
	baselineIDs := make(map[string]struct{}, len(baseline.Mods))

	for _, before := range baseline.Mods {
		baselineIDs[before.EntryID] = struct{}{}
		after, exists := currentByID[before.EntryID]
		if !exists {
			// Mod 被卸载/移动不是"少标"，只提示、不失败。
			report.MissingMods = append(report.MissingMods, before.EntryID)
			continue
		}

		beforeSet := normalizedTagSet(before.SecondaryTags)
		afterSet := normalizedTagSet(after.SecondaryTags)

		change := tagRegressionChange{EntryID: before.EntryID, Name: before.Name}
		for key, display := range afterSet {
			if _, had := beforeSet[key]; !had {
				change.Added = append(change.Added, display)
			}
		}
		for key, display := range beforeSet {
			if _, still := afterSet[key]; still {
				continue
			}
			if allowlistAllows(allowlist, before, display) {
				change.Allowlisted = append(change.Allowlisted, display)
				report.AllowlistedRemovals++
				continue
			}
			change.Removed = append(change.Removed, display)
			change.Unexcused = append(change.Unexcused, display)
		}
		sort.Strings(change.Added)
		sort.Strings(change.Removed)
		sort.Strings(change.Allowlisted)
		sort.Strings(change.Unexcused)

		report.AddedTags += len(change.Added)
		report.RemovedTags += len(change.Removed)
		if len(change.Added) > 0 || len(change.Removed) > 0 || len(change.Allowlisted) > 0 {
			report.Changes = append(report.Changes, change)
		}
	}

	for _, mod := range current {
		if _, known := baselineIDs[mod.EntryID]; !known {
			report.NewMods = append(report.NewMods, mod.EntryID)
		}
	}
	sort.Strings(report.MissingMods)
	sort.Strings(report.NewMods)
	sort.Slice(report.Changes, func(i, j int) bool {
		return report.Changes[i].EntryID < report.Changes[j].EntryID
	})

	report.OK = report.RemovedTags == 0
	if !report.OK {
		report.Notes = "存在未登记的标签消失：这是回归（少标）。" +
			"若确认是修正错误标签，请把 (entryId, tag, reason, evidence) 追加进 allowlist。"
	} else if report.AddedTags > 0 {
		report.Notes = "全部为新增标签，符合「可以多标、不能少标」。"
	} else {
		report.Notes = "标签集合与基线一致。"
	}
	return report
}

func allowlistAllows(allowlist tagRegressionAllowlist, mod tagBaselineMod, tag string) bool {
	tag = canonicalTagKey(tag)
	for _, entry := range allowlist.Entries {
		if canonicalTagKey(entry.Tag) != tag {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(entry.Scope), "global") {
			if strings.TrimSpace(entry.Reason) == "" || strings.TrimSpace(entry.Evidence) == "" {
				continue
			}
			return true
		}
		if entry.EntryID != "" && entry.EntryID == mod.EntryID {
			return true
		}
		if entry.Name != "" && strings.EqualFold(strings.TrimSpace(entry.Name), strings.TrimSpace(mod.Name)) {
			return true
		}
	}
	return false
}

func normalizedTagSlice(tags []string) []string {
	set := normalizedTagSet(tags)
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, set[key])
	}
	if result == nil {
		result = []string{}
	}
	return result
}

func normalizedTagSet(tags []string) map[string]string {
	set := make(map[string]string, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := canonicalTagKey(tag)
		// 同一个标签的多种写法（AK47 / ak47）保留首次出现的展示形式，
		// 与 parser.UniqueTagsExcluding 的"首次出现优先"保持一致。
		if _, exists := set[key]; !exists {
			set[key] = tag
		}
	}
	return set
}

// canonicalTagKey 用于跨版本比较：忽略大小写、忽略首尾空白。
// 与 parser.CanonicalTag 的展示层归一化无关 —— 这里只保证"同一个标签"判定稳定。
func canonicalTagKey(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

// ---------------------------------------------------------------------------
// App 包装层：取数（扫描结果）与写盘。
// ---------------------------------------------------------------------------

// ExportTagBaseline 写出当前扫描结果的标签基线（低层，不重新扫描）。
func (a *App) ExportTagBaseline(path string) (string, error) {
	target := strings.TrimSpace(path)
	if target == "" {
		target = filepath.Join(a.configDir, "tag_baseline.json")
	}
	baseline := buildTagBaseline(a.groupingMods(), a.catalogEntryID)
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	return target, nil
}

// CheckTagRegression 与基线比对。allowlistPath 为空时使用 <baseline>.allowlist.json（存在才读）。
func (a *App) CheckTagRegression(baselinePath, allowlistPath, reportPath string) (string, *tagRegressionReport, error) {
	baselinePath = strings.TrimSpace(baselinePath)
	if baselinePath == "" {
		return "", nil, fmt.Errorf("缺少基线文件路径")
	}
	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		return "", nil, fmt.Errorf("读取基线失败: %w", err)
	}
	var baseline tagBaselineFile
	if err := json.Unmarshal(raw, &baseline); err != nil {
		return "", nil, fmt.Errorf("解析基线失败: %w", err)
	}
	if baseline.Version != tagBaselineVersion {
		return "", nil, fmt.Errorf("基线版本不匹配: 文件 %d，当前支持 %d", baseline.Version, tagBaselineVersion)
	}

	allowlist, err := loadTagRegressionAllowlist(baselinePath, allowlistPath)
	if err != nil {
		return "", nil, err
	}

	current := buildTagBaseline(a.groupingMods(), a.catalogEntryID)
	report := diffTagBaseline(baseline, current.Mods, allowlist)

	target := strings.TrimSpace(reportPath)
	if target == "" {
		target = baselinePath + ".report.json"
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", nil, err
	}
	return target, &report, nil
}

// catalogEntryID 与 ExportGroupingCatalog 的 entryId 口径保持一致，
// 这样基线、分组清单与人工排查用的是同一个身份。
func (a *App) catalogEntryID(mod grouping.Mod) string {
	relativePath := a.relativeAddonPath(mod.SourcePath)
	location := addonLocationForRelativePath(relativePath)
	if mod.Key == "" {
		return location + "/" + relativePath
	}
	return location + "/" + mod.Key
}

func loadTagRegressionAllowlist(baselinePath, explicitPath string) (tagRegressionAllowlist, error) {
	target := strings.TrimSpace(explicitPath)
	if target == "" {
		candidate := baselinePath + ".allowlist.json"
		if _, err := os.Stat(candidate); err != nil {
			// 没有 allowlist 是正常情况：此时任何标签消失都算回归。
			return tagRegressionAllowlist{Version: tagAllowlistVersion}, nil
		}
		target = candidate
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return tagRegressionAllowlist{}, fmt.Errorf("读取 allowlist 失败: %w", err)
	}
	var allowlist tagRegressionAllowlist
	if err := json.Unmarshal(raw, &allowlist); err != nil {
		return tagRegressionAllowlist{}, fmt.Errorf("解析 allowlist 失败: %w", err)
	}
	return allowlist, nil
}
