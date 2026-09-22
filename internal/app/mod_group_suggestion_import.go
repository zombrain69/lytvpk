package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"vpk-manager/internal/grouping"
	"vpk-manager/internal/parser"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 外部组建议（模型 / 人工 / 脚本）导入。
//
// 设计目标：把"看懂 Mod 语义"这件事交给更聪明的推导方（大模型、人工整理、外部脚本），
// 本程序只负责：读取标准格式 → 严格校验 → 与内置推导合并展示 → 用户确认后落盘成策略组。
//
// 文件格式（version 1）见 docs/development/group-suggestion-import-format.md：
//
//	{
//	  "version": 1,
//	  "generator": "codex",
//	  "suggestions": [
//	    {"label": "Nick 语音", "reason": "都替换 Nick 语音", "confidence": "high",
//	     "signals": ["外部建议"], "members": ["workshop\\2985551390.vpk", "3004070051.vpk"]}
//	  ]
//	}
//
// 成员可以用：addonlist 键（相对 addons 的路径，斜杠方向不限）、裸文件名、绝对路径。

const (
	groupSuggestionFormatVersion = 1
	groupSuggestionInboxFileName = "group_suggestions.json"
	// groupingCatalogVersion 是"导出给推导方"的清单格式版本。
	// 与建议文件格式版本解耦：清单只增字段，但一旦字段含义变化就 bump 这里。
	groupingCatalogVersion  = 2
	groupingCatalogSchemaRev = "2026-09-22.2"

	// modGroupSignalExternal 标记建议来自外部文件（模型 / 人工），前端会单独展示。
	modGroupSignalExternal = "外部建议"

	// modGroupSuggestionSourceExternal 是"这条建议来自外部文件"的标记。
	modGroupSuggestionSourceExternal = "external"

	// 外部建议是"有人认真看过 Mod"的产物，排在启发式建议之前。
	modGroupSuggestionExternalScore = 90
)

// externalGroupSuggestionFile 是导入文件的顶层结构。
type externalGroupSuggestionFile struct {
	Version     int                        `json:"version"`
	Generator   string                     `json:"generator,omitempty"`
	GeneratedAt string                     `json:"generatedAt,omitempty"`
	Notes       string                     `json:"notes,omitempty"`
	Suggestions []externalGroupSuggestion  `json:"suggestions"`
}

type externalGroupSuggestion struct {
	Label      string            `json:"label"`
	Reason     string            `json:"reason,omitempty"`
	Confidence string            `json:"confidence,omitempty"`
	Strategy   string            `json:"strategy,omitempty"`
	Signals    []string          `json:"signals,omitempty"`
	Members    []string          `json:"members"`
	MemberNote map[string]string `json:"memberNotes,omitempty"`
}

// GroupSuggestionImportResult 汇总一次导入的结果，便于前端提示"导入了什么、跳过了什么"。
type GroupSuggestionImportResult struct {
	File         string   `json:"file"`
	Generator    string   `json:"generator,omitempty"`
	GeneratedAt  string   `json:"generatedAt,omitempty"`
	Total        int      `json:"total"`
	Imported     int      `json:"imported"`
	Skipped      int      `json:"skipped"`
	MemberCount  int      `json:"memberCount"`
	ImportedAt   string   `json:"importedAt"`
	Warnings     []string `json:"warnings,omitempty"`
	ResolvedMods int      `json:"resolvedMods"`
}

// groupSuggestionInboxPath 是"应用会自动读取"的建议文件位置。
func (a *App) groupSuggestionInboxPath() string {
	a.mu.RLock()
	configDir := a.configDir
	a.mu.RUnlock()
	if configDir == "" {
		if a.configPath != "" {
			configDir = filepath.Dir(a.configPath)
		} else {
			return ""
		}
	}
	return filepath.Join(configDir, groupSuggestionInboxFileName)
}

// GetGroupSuggestionInboxPath 让前端/模型知道应该把建议文件写到哪里。
func (a *App) GetGroupSuggestionInboxPath() string {
	return a.groupSuggestionInboxPath()
}

// modKeyIndex 建立"可匹配键 → 缓存里的 Mod"索引，覆盖
// addonlist 键（相对 addons 路径）与裸文件名两种写法。
type modKeyIndex struct {
	byKey    map[string]VPKFile
	byName   map[string][]string // 文件名（小写）→ 所有命中键
	// byEntryID 用 `<location>/<key>` 精确定位（root/123.vpk、disabled/123.vpk）。
	byEntryID map[string]string
	// keysPerName 记录"同一个键在不同位置出现的次数"，用于区分真歧义与同键多副本。
	entryIDByKey map[string][]string
	keyOrder []string
	rootDir  string
}

func (a *App) buildModKeyIndex() modKeyIndex {
	index := modKeyIndex{
		byKey:        make(map[string]VPKFile),
		byName:       make(map[string][]string),
		byEntryID:    make(map[string]string),
		entryIDByKey: make(map[string][]string),
	}
	rootDir := a.rootDirectorySnapshot()
	index.rootDir = rootDir
	keys := make([]string, 0, 2048)

	a.vpkCache.Range(func(_ any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		file := cache.File
		key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, file.Path)
		if err != nil || key == "" {
			return true
		}
		normalized := normalizeAddonListKey(key)
		location := addonLocationForRelativePath(filepath.ToSlash(mustRelative(rootDir, file.Path)))
		entryID := location + "/" + normalized
		index.entryIDByKey[normalized] = append(index.entryIDByKey[normalized], entryID)
		if _, exists := index.byEntryID[entryID]; !exists {
			index.byEntryID[entryID] = normalized
		}
		if _, exists := index.byKey[normalized]; !exists {
			index.byKey[normalized] = file
		}
		name := strings.ToLower(strings.TrimSpace(file.Name))
		if name == "" {
			name = strings.ToLower(filepath.Base(file.Path))
		}
		if name != "" {
			index.byName[name] = append(index.byName[name], normalized)
		}
		keys = append(keys, normalized)
		return true
	})
	sort.Strings(keys)
	index.keyOrder = keys
	return index
}

// mustRelative 计算相对路径，失败时返回空串（调用方按空串处理）。
func mustRelative(rootDir, filePath string) string {
	if rootDir == "" || filePath == "" {
		return ""
	}
	relative, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return ""
	}
	return relative
}

// resolve 把一个成员写法解析成 addonlist 键。
//
// 支持（按优先级）：
//  1. entryId：`root/123.vpk`、`disabled/123.vpk`、`workshop/123.vpk`（唯一寻址）；
//  2. 物理相对路径：`disabled\123.vpk`、`workshop\123.vpk`；
//  3. addonlist 键：`123.vpk`（同一键在多个位置出现时，多个位置都算命中，且不算歧义——
//     它们本来就是同一个 addonlist 键的副本）；
//  4. 裸文件名：命中多个不同键时才算 ambiguous。
func (index modKeyIndex) resolve(raw string) (matches []string, ambiguous bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	// 1) entryId / 带位置前缀的物理路径。
	normalizedValue := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	trimmed := strings.TrimPrefix(normalizedValue, "./")
	if key, ok := index.byEntryID[strings.ToLower(trimmed)]; ok {
		return []string{key}, false
	}
	for _, prefix := range []string{"root/", "disabled/", "workshop/"} {
		if !strings.HasPrefix(strings.ToLower(trimmed), prefix) {
			continue
		}
		key := normalizeAddonListKey(strings.TrimPrefix(trimmed, prefix))
		if _, ok := index.byKey[key]; ok {
			return []string{key}, false
		}
	}
	// 绝对路径：先按 addons 根目录换算成相对键。
	if filepath.IsAbs(value) && index.rootDir != "" {
		if relative, err := filepath.Rel(index.rootDir, value); err == nil {
			normalized := normalizeAddonListKey(relative)
			if _, ok := index.byKey[normalized]; ok {
				return []string{normalized}, false
			}
		}
	}
	normalized := normalizeAddonListKey(value)
	// 2) 精确键匹配优先：`3773210949.vpk` 这类合法根目录键不该被当成"裸文件名命中多处"。
	if entryIDs, ok := index.entryIDByKey[normalized]; ok && len(entryIDs) > 0 {
		// 同一个键在多个位置（root + disabled / workshop）：逻辑上是同一个 Mod 的副本，
		// 只返回一个键，且**不报歧义**——否则会给推导方一堆假警告。
		result := []string{normalized}
		// 但如果这个文件名在别的键下也出现过（例如同名文件同时放在根目录与 workshop，
		// 两者的键不同），把副本一起纳入，方便"整组一起管"。
		for _, key := range index.uniqueByKey(index.byName[strings.ToLower(filepath.Base(filepath.FromSlash(normalized)))]) {
			if key != normalized {
				result = append(result, key)
			}
		}
		return result, len(result) > 1
	}
	// 3) 裸文件名：命中多个不同键时才是真歧义。
	if !strings.ContainsAny(value, `/\`) {
		if keys := index.uniqueByKey(index.byName[strings.ToLower(value)]); len(keys) > 0 {
			if len(keys) == 1 {
				return keys, false
			}
			return append([]string(nil), keys...), true
		}
	}
	base := strings.ToLower(filepath.Base(filepath.FromSlash(value)))
	if keys := index.uniqueByKey(index.byName[base]); len(keys) > 0 {
		if len(keys) == 1 {
			return keys, false
		}
		return append([]string(nil), keys...), true
	}
	return nil, false
}

// uniqueByKey 去重（同一个键的多个位置副本只算一个）。
func (index modKeyIndex) uniqueByKey(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(keys))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}
	return result
}

// parseExternalGroupSuggestionsFile 读取并严格校验建议文件。
func parseExternalGroupSuggestionsFile(path string) (externalGroupSuggestionFile, error) {
	if strings.TrimSpace(path) == "" {
		return externalGroupSuggestionFile{}, fmt.Errorf("建议文件路径为空")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return externalGroupSuggestionFile{}, fmt.Errorf("读取建议文件失败: %w", err)
	}
	var parsed externalGroupSuggestionFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return externalGroupSuggestionFile{}, fmt.Errorf("解析建议文件失败: %w", err)
	}
	if parsed.Version != 0 && parsed.Version != groupSuggestionFormatVersion {
		return externalGroupSuggestionFile{}, fmt.Errorf("不支持的建议文件版本: %d（当前支持 %d）", parsed.Version, groupSuggestionFormatVersion)
	}
	if len(parsed.Suggestions) == 0 {
		return externalGroupSuggestionFile{}, fmt.Errorf("建议文件里没有任何 suggestions")
	}
	return parsed, nil
}

// convertExternalSuggestions 把外部建议解析成推导引擎的候选结构：
// 解析成员键、去重、丢弃无法匹配的成员，并把问题写进 warnings。
// 这样外部建议与内置信号走同一条合并 / 排序 / 配额流水线。
func (a *App) convertExternalSuggestions(parsed externalGroupSuggestionFile) ([]grouping.Candidate, GroupSuggestionImportResult) {
	index := a.buildModKeyIndex()
	result := GroupSuggestionImportResult{
		Generator:   strings.TrimSpace(parsed.Generator),
		GeneratedAt: strings.TrimSpace(parsed.GeneratedAt),
		Total:       len(parsed.Suggestions),
		Warnings:    make([]string, 0, 8),
	}
	seenLabels := make(map[string]int, len(parsed.Suggestions))
	suggestions := make([]grouping.Candidate, 0, len(parsed.Suggestions))

	for position, raw := range parsed.Suggestions {
		label := strings.TrimSpace(raw.Label)
		if label == "" {
			result.Skipped++
			result.Warnings = append(result.Warnings, fmt.Sprintf("第 %d 条建议缺少 label，已跳过", position+1))
			continue
		}
		if len([]rune(label)) > 60 {
			label = string([]rune(label)[:60])
		}

		keys := make([]string, 0, len(raw.Members))
		seen := make(map[string]struct{}, len(raw.Members))
		missing := make([]string, 0, 4)
		ambiguous := make([]string, 0, 4)
		for _, member := range raw.Members {
			matches, isAmbiguous := index.resolve(member)
			if len(matches) == 0 {
				missing = append(missing, strings.TrimSpace(member))
				continue
			}
			if isAmbiguous {
				ambiguous = append(ambiguous, strings.TrimSpace(member))
			}
			for _, key := range matches {
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				keys = append(keys, key)
			}
		}

		if len(keys) < modGroupSuggestionMinMembers {
			result.Skipped++
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("「%s」有效成员不足 %d 个，已跳过（未匹配 %d 个）", label, modGroupSuggestionMinMembers, len(missing)))
			continue
		}
		if len(keys) > modGroupSuggestionMaxCuratedMembers {
			result.Skipped++
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("「%s」成员过多（%d > %d），已跳过", label, len(keys), modGroupSuggestionMaxCuratedMembers))
			continue
		}
		if firstIndex, dup := seenLabels[strings.ToLower(label)]; dup {
			result.Skipped++
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("「%s」与第 %d 条重名，已跳过", label, firstIndex+1))
			continue
		}
		seenLabels[strings.ToLower(label)] = position

		if len(missing) > 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("「%s」有 %d 个成员未在当前列表里，已忽略：%s",
					label, len(missing), strings.Join(truncateStringList(missing, 3), "、")))
		}
		if len(ambiguous) > 0 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("「%s」有 %d 个裸文件名命中多个位置，已全部纳入：%s",
					label, len(ambiguous), strings.Join(truncateStringList(ambiguous, 3), "、")))
		}

		names := make([]string, 0, len(keys))
		for _, key := range keys {
			if file, ok := index.byKey[key]; ok && strings.TrimSpace(file.Title) != "" {
				names = append(names, file.Title)
				continue
			}
			if file, ok := index.byKey[key]; ok && strings.TrimSpace(file.Name) != "" {
				names = append(names, file.Name)
				continue
			}
			names = append(names, filepath.Base(key))
		}

		confidence := normalizeSuggestionConfidence(raw.Confidence)
		strategy := ""
		if value := strings.TrimSpace(raw.Strategy); value != "" {
			if normalized, err := normalizeModStrategyGroupStrategy(value); err == nil {
				strategy = normalized
			} else {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("「%s」的 strategy=%q 无法识别，已按默认互斥单选处理", label, value))
			}
		}
		signals := []string{modGroupSignalExternal}
		for _, signal := range raw.Signals {
			value := strings.TrimSpace(signal)
			if value == "" || value == modGroupSignalExternal {
				continue
			}
			signals = append(signals, value)
		}

		reason := strings.TrimSpace(raw.Reason)
		if reason == "" {
			reason = "来自外部建议文件"
		}

		_ = confidence // 置信度由信号目录统一判定（high），不再逐条保存
		suggestions = append(suggestions, grouping.Candidate{
			Provider: "external",
			Label:    label,
			Reason:   reason,
			// 分数按文件顺序递减：外部建议是"逐条写过"的，展示顺序应保留作者的编排，
			// 同时仍高于内置启发式（90 → 89 → …，外部信号属于 curated，不做规模降权）。
			Score:    externalSuggestionScore(position),
			Signals:  signals,
			Keys:     keys,
			Names:    names,
			Source:   modGroupSuggestionSourceExternal,
			Strategy: strategy,
		})
		result.Imported++
		result.MemberCount += len(keys)
	}

	result.ResolvedMods = len(index.byKey)
	return suggestions, result
}

// externalSuggestionScore 让文件里靠前的建议排得更前（最低 60，仍高于内置启发式的 45）。
func externalSuggestionScore(position int) int {
	score := modGroupSuggestionExternalScore - position
	if score < 60 {
		return 60
	}
	return score
}

// normalizeSuggestionConfidence 只接受 high/medium/low，缺省按 high（外部建议通常是人工确认过的）。
func normalizeSuggestionConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "高":
		return "high"
	case "medium", "中":
		return "medium"
	case "low", "低":
		return "low"
	default:
		return "high"
	}
}

func truncateStringList(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return append(values[:limit:limit], fmt.Sprintf("等 %d 项", len(values)))
}

// externalSuggestionCache 缓存"建议收件箱"的解析结果：
// 文件未变化（mtime + size 相同）时不再重新解析与重建键索引。
type externalSuggestionCache struct {
	mu         sync.Mutex
	modTime    time.Time
	size       int64
	loaded     bool
	candidates []grouping.Candidate
}

func (a *App) externalSuggestionCandidates() ([]grouping.Candidate, error) {
	path := a.groupSuggestionInboxPath()
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			a.externalCache.mu.Lock()
			a.externalCache.loaded = true
			a.externalCache.candidates = nil
			a.externalCache.mu.Unlock()
			return nil, nil
		}
		return nil, err
	}

	cache := &a.externalCache
	cache.mu.Lock()
	if cache.loaded && cache.modTime.Equal(info.ModTime()) && cache.size == info.Size() {
		cached := cache.candidates
		cache.mu.Unlock()
		return cached, nil
	}
	cache.mu.Unlock()

	parsed, err := parseExternalGroupSuggestionsFile(path)
	if err != nil {
		return nil, err
	}
	candidates, _ := a.convertExternalSuggestions(parsed)

	cache.mu.Lock()
	cache.loaded = true
	cache.modTime = info.ModTime()
	cache.size = info.Size()
	cache.candidates = candidates
	cache.mu.Unlock()
	return candidates, nil
}

// GetExternalGroupSuggestions 读取"建议收件箱"并解析成建议列表（只读，不修改任何文件）。
func (a *App) GetExternalGroupSuggestions() ([]ModGroupSuggestion, error) {
	candidates, err := a.externalSuggestionCandidates()
	if err != nil {
		return nil, err
	}
	suggestions := make([]ModGroupSuggestion, 0, len(candidates))
	for _, candidate := range candidates {
		suggestions = append(suggestions, ModGroupSuggestion{
			ID:          "external-" + modGroupSuggestionID(candidate.Keys),
			Label:       candidate.Label,
			Reason:      candidate.Reason,
			Confidence:  grouping.ConfidenceForSignals(candidate.Signals),
			Score:       candidate.Score,
			Signals:     candidate.Signals,
			MemberKeys:  candidate.Keys,
			MemberNames: candidate.Names,
			Source:      candidate.Source,
			Strategy:    candidate.Strategy,
		})
	}
	return suggestions, nil
}

// ImportGroupSuggestionsFromFile 校验一个建议文件并复制到收件箱，
// 之后「分组建议」弹窗会自动带上这些建议。
func (a *App) ImportGroupSuggestionsFromFile(path string) (GroupSuggestionImportResult, error) {
	parsed, err := parseExternalGroupSuggestionsFile(path)
	if err != nil {
		return GroupSuggestionImportResult{}, err
	}
	suggestions, result := a.convertExternalSuggestions(parsed)
	if len(suggestions) == 0 {
		result.File = path
		return result, fmt.Errorf("这个文件里的建议都无法匹配当前 Mod 列表（详见警告）")
	}
	inbox := a.groupSuggestionInboxPath()
	if inbox == "" {
		return result, fmt.Errorf("配置目录不可用，无法保存建议文件")
	}
	data, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(inbox), 0o755); err != nil {
		return result, err
	}
	if err := os.WriteFile(inbox, data, 0o644); err != nil {
		return result, err
	}
	result.File = inbox
	result.ImportedAt = time.Now().Format(time.RFC3339)
	if strings.TrimSpace(path) != inbox {
		result.Warnings = append(result.Warnings, "已复制到建议收件箱："+inbox)
	}
	return result, nil
}

// ImportGroupSuggestionsOpenDialog 弹出文件选择框导入建议（用户取消时返回空结果）。
func (a *App) ImportGroupSuggestionsOpenDialog() (GroupSuggestionImportResult, error) {
	if a.ctx == nil {
		return GroupSuggestionImportResult{}, fmt.Errorf("应用尚未就绪，无法打开文件对话框")
	}
	sourcePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入组建议文件（JSON）",
		Filters: []runtime.FileFilter{
			{DisplayName: "LytVPK 组建议 (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return GroupSuggestionImportResult{}, err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return GroupSuggestionImportResult{}, nil
	}
	return a.ImportGroupSuggestionsFromFile(sourcePath)
}

// GroupSuggestionValidationMember 是 dry-run 校验里单个成员的解析结果。
type GroupSuggestionValidationMember struct {
	Raw       string `json:"raw"`
	Resolved  string `json:"resolved,omitempty"`
	Matched   bool   `json:"matched"`
	Ambiguous bool   `json:"ambiguous,omitempty"`
	Note      string `json:"note,omitempty"`
}

// GroupSuggestionValidationItem 是 dry-run 校验里单条建议的结果。
type GroupSuggestionValidationItem struct {
	Label       string                            `json:"label"`
	Valid       bool                              `json:"valid"`
	Confidence  string                            `json:"confidence,omitempty"`
	Strategy    string                            `json:"strategy,omitempty"`
	MemberCount int                               `json:"memberCount"`
	Members     []GroupSuggestionValidationMember `json:"members"`
	Problems    []string                          `json:"problems,omitempty"`
}

// GroupSuggestionValidation 是 dry-run 的完整结果：逐条建议 + 逐成员 + 全部警告。
type GroupSuggestionValidation struct {
	File        string                          `json:"file"`
	Generator   string                          `json:"generator,omitempty"`
	Total       int                             `json:"total"`
	Valid       int                             `json:"valid"`
	Invalid     int                             `json:"invalid"`
	MemberCount int                             `json:"memberCount"`
	Items       []GroupSuggestionValidationItem `json:"items"`
	Warnings    []string                        `json:"warnings,omitempty"`
}

// ValidateGroupSuggestionsFile 只读校验建议文件（dry-run）：不写收件箱、不改任何文件，
// 返回逐成员的解析结果与**全部**警告，方便外部智能体"写文件 → 校验 → 修正"迭代。
func (a *App) ValidateGroupSuggestionsFile(path string) (GroupSuggestionValidation, error) {
	parsed, err := parseExternalGroupSuggestionsFile(path)
	if err != nil {
		return GroupSuggestionValidation{}, err
	}
	result := GroupSuggestionValidation{
		File:      path,
		Generator: strings.TrimSpace(parsed.Generator),
		Total:     len(parsed.Suggestions),
	}
	index := a.buildModKeyIndex()
	seenLabels := make(map[string]int, len(parsed.Suggestions))

	for position, raw := range parsed.Suggestions {
		item := GroupSuggestionValidationItem{
			Label:      strings.TrimSpace(raw.Label),
			Confidence: normalizeSuggestionConfidence(raw.Confidence),
			Strategy:   strings.TrimSpace(raw.Strategy),
		}
		if item.Label == "" {
			item.Problems = append(item.Problems, "缺少 label")
		} else if len([]rune(item.Label)) > 60 {
			item.Problems = append(item.Problems, "label 超过 60 个字符（导入时会被截断）")
		}
		if first, dup := seenLabels[strings.ToLower(item.Label)]; dup && item.Label != "" {
			item.Problems = append(item.Problems, fmt.Sprintf("与第 %d 条重名", first+1))
		} else if item.Label != "" {
			seenLabels[strings.ToLower(item.Label)] = position
		}
		if item.Strategy != "" {
			if normalized, err := normalizeModStrategyGroupStrategy(item.Strategy); err != nil {
				item.Problems = append(item.Problems, "strategy 无法识别："+item.Strategy)
			} else {
				item.Strategy = normalized
			}
		}

		seenKeys := make(map[string]struct{}, len(raw.Members))
		for _, member := range raw.Members {
			entry := GroupSuggestionValidationMember{Raw: strings.TrimSpace(member)}
			matches, ambiguous := index.resolve(member)
			switch {
			case len(matches) == 0:
				entry.Matched = false
				entry.Note = "在当前 Mod 列表里找不到"
			case len(matches) == 1:
				entry.Matched = true
				entry.Resolved = matches[0]
				if ambiguous {
					entry.Ambiguous = true
					entry.Note = "同名文件命中多个位置，已取第一个"
				}
			default:
				entry.Matched = true
				entry.Ambiguous = ambiguous
				entry.Resolved = strings.Join(matches, " + ")
				entry.Note = fmt.Sprintf("命中 %d 个位置（同名副本会一起纳入）", len(matches))
			}
			for _, key := range matches {
				seenKeys[key] = struct{}{}
			}
			item.Members = append(item.Members, entry)
		}

		item.MemberCount = len(seenKeys)
		if item.MemberCount < modGroupSuggestionMinMembers {
			item.Problems = append(item.Problems, fmt.Sprintf("有效成员不足 %d 个", modGroupSuggestionMinMembers))
		}
		if item.MemberCount > modGroupSuggestionMaxCuratedMembers {
			item.Problems = append(item.Problems, fmt.Sprintf("成员过多（%d > %d）", item.MemberCount, modGroupSuggestionMaxCuratedMembers))
		}
		item.Valid = len(item.Problems) == 0
		if item.Valid {
			result.Valid++
		} else {
			result.Invalid++
		}
		result.MemberCount += item.MemberCount
		result.Items = append(result.Items, item)
	}

	// 完整警告（不再只保留前 3 条）：未匹配成员逐个列出。
	for _, item := range result.Items {
		for _, member := range item.Members {
			if member.Matched {
				continue
			}
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("「%s」找不到成员：%s", item.Label, member.Raw))
		}
	}
	return result, nil
}
// ClearExternalGroupSuggestions 移除收件箱文件（不会删除任何 Mod 或已创建的策略组）。
func (a *App) ClearExternalGroupSuggestions() error {
	path := a.groupSuggestionInboxPath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ExportGroupingCatalog 导出当前已扫描 Mod 的"分组用元数据"，
// 供外部推导方（大模型 / 人工 / 脚本）分析后写出建议文件。
func (a *App) ExportGroupingCatalog(path string) (string, error) {
	target := strings.TrimSpace(path)
	if target == "" {
		target = filepath.Join(a.configDir, "grouping_catalog.json")
	}
	mods := a.groupingMods()
	entries := make([]groupingCatalogEntry, 0, len(mods))
	priorityByKey := a.priorityPlanSnapshot()
	loadOrderByKey := a.loadOrderSnapshot()
	// 一次性准备好"本地管理记录"与"工坊资料"的索引，避免逐个 Mod 重复读文件。
	managementByKey := a.managementSnapshot()
	watchLaterByID := a.watchLaterSnapshot()
	for _, mod := range mods {
		file := a.cachedVPKFileByPath(mod.SourcePath)
		layer := priorityByKey[mod.Key]
		workshop := a.workshopSnapshotFor(file, mod, watchLaterByID)
		// location / relativePath / entryId 一律从物理路径现算：缓存的 Location
		// 在文件被移动后可能还是旧值，用它会让 folder 与 location 互相矛盾。
		relativePath := a.relativeAddonPath(mod.SourcePath)
		location := addonLocationForRelativePath(relativePath)
		entries = append(entries, groupingCatalogEntry{
			Key:               mod.Key,
			EntryID:           location + "/" + mod.Key,
			Name:              mod.Name,
			Title:             mod.Title,
			Author:            mod.Author,
			PrimaryTag:        mod.PrimaryTag,
			SecondaryTags:     mod.SecondaryTags,
			SubjectSummary:    mod.Subject,
			SubjectConfidence: mod.SubjectConfidence,
			ContentSubjects:   file.ContentSubjects,
			VoiceCharacters:   mod.VoiceCharacters,
			XDRSummary:        strings.TrimSpace(file.XDRSummary),
			ModelCount:        file.ModelCount,
			ModelTriangles:    file.ModelTriangles,
			Campaign:          strings.TrimSpace(file.Campaign),
			WorkshopID:        mod.WorkshopID,
			Folder:            mod.Folder,
			Location:          location,
			RelativePath:      relativePath,
			GameEnabled:       file.GameEnabled,
			GameStateKnown:    file.GameStateKnown,
			Size:              file.Size,
			LastModified:      strings.TrimSpace(file.LastModified),
			LoadOrder:         loadOrderByKey[mod.Key],
			EffectiveLayer:    layer.effective,
			PrioritySource:    layer.source,
			Structure:         catalogStructureFor(file),
			AddonInfo:         catalogAddonInfoFor(file),
			Workshop:          workshop,
			Management:        managementByKey[mod.Key],
			XDRSlots:          catalogXDRSlots(file),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })

	entryIDByKey := entryIDIndex(entries)
	clusterHints := a.catalogClusterHints(mods, entryIDByKey)
	duplicateGroups := buildDuplicateGroups(entries)
	payload := groupingCatalogFile{
		Version:     groupingCatalogVersion,
		SchemaRev:   groupingCatalogSchemaRev,
		Capabilities: []string{
			"entryId", "structure", "structure.targets", "workshopMeta", "addonInfo",
			"management", "coverage", "scope", "clusterHints.full", "ungroupedKeys",
			"duplicateGroups", "xdrSlots", "unreadableMods", "themeHints", "preloadHints",
		},
		GeneratedAt: time.Now().Format(time.RFC3339),
		Generator:   "LytVPK " + AppVersion,
		ModCount:    len(entries),
		Mods:        entries,
		Scope:       a.catalogScope(),
		UnreadableMods: a.unreadableModSnapshot(),
		DuplicateGroups: duplicateGroups,
		ClusterHints:    clusterHints,
		UngroupedKeys:   ungroupedKeys(entries, clusterHints),
		ThemeHints:      buildThemeHints(entries, entryIDByKey),
		PreloadHints:    buildPreloadHints(entries),
		Coverage:        coverageForCatalog(entries, clusterHints, duplicateGroups),
		Notes: "清单已包含分组所需全部信息：标签/主体/语音角色/作者、VPK 内部结构（顶层目录、替换目标与代表性路径）、" +
			"体积与位置/游戏开关/优先级，以及预计算好的重复副本分组。智能体无需自行打开 VPK。" +
			"建议先读 clusterHints 与 duplicateGroups（都很小），再用脚本/关键词去 mods 里查这些键的明细，" +
			"不要一次性把整个 mods 数组读进上下文。" +
			"coverage 字段说明各类信息的可用数量：工坊资料（workshop.*）只对本地存在 .meta 的 Mod 有值，" +
			"缺失时可以在 LytVPK 的工坊设置里开启工坊信息存储后重新下载/刷新对应 Mod。",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
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

// ExportGroupingCatalogDialog 弹出保存对话框导出分组用 Mod 清单。
func (a *App) ExportGroupingCatalogDialog() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用尚未就绪，无法打开保存对话框")
	}
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出分组用 Mod 清单",
		DefaultFilename: "grouping_catalog.json",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(target) == "" {
		return "", nil
	}
	return a.ExportGroupingCatalog(target)
}

type groupingCatalogFile struct {
	Version int `json:"version"`
	// SchemaRev 是清单结构版本：字段含义/结构变化时递增，外部推导方据此判断能力。
	SchemaRev string `json:"schemaRev"`
	// Capabilities 声明本版清单具备的能力，避免"同 version 但字段完全不同"的情况。
	Capabilities []string               `json:"capabilities"`
	GeneratedAt string                 `json:"generatedAt"`
	Generator   string                 `json:"generator"`
	ModCount    int                    `json:"modCount"`
	Mods        []groupingCatalogEntry `json:"mods"`
	// Scope 说明本次扫描覆盖哪些位置、哪些位置被有意排除。
	Scope *groupingCatalogScope `json:"scope,omitempty"`
	// UnreadableMods 是"磁盘上有、但解析失败"的文件（例如扩展名是 .vpk 实为 ZIP）。
	UnreadableMods []groupingCatalogUnreadable `json:"unreadableMods,omitempty"`
	// UngroupedKeys 是没有任何 clusterHint 覆盖到的 Mod 键，方便推导方接力分析。
	UngroupedKeys []string `json:"ungroupedKeys,omitempty"`
	// DuplicateGroups 预计算好"疑似同一 Mod 的多个副本"，省去智能体自行比对。
	DuplicateGroups []groupingCatalogDuplicateGroup `json:"duplicateGroups,omitempty"`
	// ThemeHints / PreloadHints 是给外部推导方的"主题套装"与"前置库"线索。
	ThemeHints   []groupingCatalogThemeHint   `json:"themeHints,omitempty"`
	PreloadHints []groupingCatalogPreloadHint `json:"preloadHints,omitempty"`
	// ClusterHints 是内置推导给出的紧凑候选簇（只有组名、信号与成员键）：
	// 智能体可以先读这一段（很小），再按需去 mods 里查这些键的明细。
	ClusterHints []groupingCatalogCluster `json:"clusterHints,omitempty"`
	// Coverage 汇总"这份清单里哪些信息真的存在"，
	// 让智能体知道哪些字段可用、哪些需要用户先补数据。
	Coverage *groupingCatalogCoverage `json:"coverage,omitempty"`
	Notes    string                   `json:"notes,omitempty"`
}

type groupingCatalogScope struct {
	Included         []string `json:"included"`
	Excluded         []string `json:"excluded"`
	ExcludedDirCount int      `json:"excludedDirectoryCount"`
	ExcludedVpkCount int      `json:"excludedVpkCount"`
	Note             string   `json:"note,omitempty"`
}

type groupingCatalogUnreadable struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

type groupingCatalogCoverage struct {
	Mods                 int `json:"mods"`
	WithStructure        int `json:"withStructure"`
	WithStructureTargets int `json:"withStructureTargets"`
	WithAddonInfo        int `json:"withAddonInfo"`
	WithWorkshopMeta     int `json:"withWorkshopMeta"`
	WithWorkshopTags     int `json:"withWorkshopTags"`
	WithWatchLaterStats  int `json:"withWatchLaterStats"`
	WithVoiceCharacters  int `json:"withVoiceCharacters"`
	WithSubject          int `json:"withSubject"`
	WithManagement       int `json:"withManagement"`
	WithProfileOrGroup   int `json:"withProfileOrGroup"`
	ClusterHints         int `json:"clusterHints"`
	DuplicateGroups      int `json:"duplicateGroups"`
}

type groupingCatalogCluster struct {
	Label      string   `json:"label"`
	Confidence string   `json:"confidence"`
	Signals    []string `json:"signals,omitempty"`
	MemberKeys []string `json:"memberKeys"`
	// MemberEntryIDs 与 MemberKeys 一一对应，但用 entryId 寻址：
	// 同一个 key 在 root 与 disabled 各有一份时，只有 entryId 能精确指向其中一条。
	MemberEntryIDs []string `json:"memberEntryIds,omitempty"`
}

// groupingCatalogThemeHint 是"跨槽位的同主题套装"候选：
// 同一主题名出现在多个不同替换目标上（例如同一把近战/武器皮肤 + 通用材质），
// 这类集合更适合用 all（整套一起启用）。
type groupingCatalogThemeHint struct {
	Theme      string   `json:"theme"`
	// MemberEntryIDs 用 entryId 寻址（同键多位置也能区分）。
	MemberEntryIDs []string `json:"memberEntryIds"`
	MemberKeys     []string `json:"memberKeys,omitempty"`
	Subjects   []string `json:"subjects,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// groupingCatalogPreloadHint 是"可能是前置库"的候选：
// 名称/标题里带有 库 / 前置 / Base / lib / xdReanimsBase / KSEP 等特征，
// 这类 Mod 通常被别的 Mod 依赖，适合单独成组或用 all 常开。
type groupingCatalogPreloadHint struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// priorityLayer 记录某个 Mod 的有效分层与来源。
type priorityLayer struct {
	effective *int
	source    string
}

// priorityPlanSnapshot 读取一次优先级计划，供导出清单使用（失败时返回空表）。
func (a *App) priorityPlanSnapshot() map[string]priorityLayer {
	result := make(map[string]priorityLayer)
	plan, err := a.GetModPriorityPlan()
	if err != nil {
		return result
	}
	for _, entry := range plan {
		key := normalizeAddonListKey(entry.Key)
		if key == "" {
			continue
		}
		layer := priorityLayer{source: strings.TrimSpace(entry.Source)}
		if entry.Effective > 0 {
			value := entry.Effective
			layer.effective = &value
		}
		result[key] = layer
	}
	return result
}

// coverageWithClusters 在基础覆盖度上补上簇 / 副本分组数量。
func coverageForCatalog(entries []groupingCatalogEntry, clusters []groupingCatalogCluster, duplicates []groupingCatalogDuplicateGroup) *groupingCatalogCoverage {
	coverage := catalogCoverage(entries)
	coverage.ClusterHints = len(clusters)
	coverage.DuplicateGroups = len(duplicates)
	return coverage
}

// catalogCoverage 统计各类信息的可用数量，便于智能体判断数据是否充分。
func catalogCoverage(entries []groupingCatalogEntry) *groupingCatalogCoverage {
	coverage := &groupingCatalogCoverage{Mods: len(entries)}
	for _, entry := range entries {
		if entry.Structure != nil {
			coverage.WithStructure++
			if len(entry.Structure.Targets) > 0 {
				coverage.WithStructureTargets++
			}
		}
		if entry.AddonInfo != nil {
			coverage.WithAddonInfo++
		}
		if entry.Workshop != nil {
			coverage.WithWorkshopMeta++
			if len(entry.Workshop.Tags) > 0 {
				coverage.WithWorkshopTags++
			}
			if entry.Workshop.InWatchLater {
				coverage.WithWatchLaterStats++
			}
		}
		if len(entry.VoiceCharacters) > 0 {
			coverage.WithVoiceCharacters++
		}
		if strings.TrimSpace(entry.SubjectSummary) != "" {
			coverage.WithSubject++
		}
		if entry.Management != nil {
			coverage.WithManagement++
			if len(entry.Management.Groups) > 0 || len(entry.Management.Profiles) > 0 {
				coverage.WithProfileOrGroup++
			}
		}
	}
	return coverage
}

// catalogClusterHints 复用内置推导，给出紧凑的候选簇（只带组名、信号与成员键）。
func (a *App) catalogClusterHints(mods []grouping.Mod, entryIDByKey map[string]string) []groupingCatalogCluster {
	options := grouping.DefaultOptions()
	// 导出侧追求"广度"：UI 的 40 条列表用默认配额（20/信号）保证多样性，
	// 而给外部推导方的清单需要尽可能多的候选起点，因此放宽到 120/信号 + 600 条上限，
	// 成员仍受 grouping 的上限约束（60 人以内）。
	options.Limit = 600
	options.PerProviderLimit = 120
	options.DisableQuotaForIDs = []string{"collection", "external"}
	options.Injected = a.collectionCandidates(mods)
	ranked, _ := grouping.Suggest(mods, options)
	hints := make([]groupingCatalogCluster, 0, len(ranked))
	for _, item := range ranked {
		entryIDs := make([]string, 0, len(item.MemberKeys))
		for _, key := range item.MemberKeys {
			if entryID, ok := entryIDByKey[key]; ok {
				entryIDs = append(entryIDs, entryID)
				continue
			}
			entryIDs = append(entryIDs, key)
		}
		hints = append(hints, groupingCatalogCluster{
			Label:          item.Label,
			Confidence:     item.Confidence,
			Signals:        item.Signals,
			MemberKeys:     item.MemberKeys,
			MemberEntryIDs: entryIDs,
		})
	}
	return hints
}

// recordUnreadableMod 记录一个解析失败的 VPK（清单里会说明范围里少了哪些文件）。
func (a *App) recordUnreadableMod(filePath, reason string) {
	if strings.TrimSpace(filePath) == "" {
		return
	}
	a.unreadableMods.Store(filePath, strings.TrimSpace(reason))
}

func (a *App) clearUnreadableMod(filePath string) {
	a.unreadableMods.Delete(filePath)
}

// unreadableModSnapshot 导出解析失败清单（文件名 + 原因，最多 50 条）。
func (a *App) unreadableModSnapshot() []groupingCatalogUnreadable {
	result := make([]groupingCatalogUnreadable, 0, 8)
	a.unreadableMods.Range(func(key, value any) bool {
		path, _ := key.(string)
		reason, _ := value.(string)
		if path == "" {
			return true
		}
		result = append(result, groupingCatalogUnreadable{
			Name:   filepath.Base(path),
			Reason: reason,
		})
		return len(result) < 50
	})
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// relativeAddonPath 返回相对 addons 根目录的物理路径（反斜杠、保留原始大小写）。
func (a *App) relativeAddonPath(filePath string) string {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" || strings.TrimSpace(filePath) == "" {
		return ""
	}
	relative, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(relative)
}

// addonLocationForRelativePath 从物理相对路径判定位置（root / workshop / disabled）。
func addonLocationForRelativePath(relativePath string) string {
	value := strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(relativePath), "\\", "/"), "./")
	switch {
	case strings.HasPrefix(value, "workshop/"):
		return "workshop"
	case strings.HasPrefix(value, "disabled/"):
		return "disabled"
	default:
		return "root"
	}
}

// catalogXDRSlots 导出骨骼槽证据的精简形式。
func catalogXDRSlots(file parser.VPKFile) []groupingCatalogXDRSlot {
	if len(file.XDRSlots) == 0 {
		return nil
	}
	slots := make([]groupingCatalogXDRSlot, 0, len(file.XDRSlots))
	for _, slot := range file.XDRSlots {
		slots = append(slots, groupingCatalogXDRSlot{
			Character:  strings.TrimSpace(slot.Character),
			Model:      strings.TrimSpace(slot.Model),
			Scope:      strings.TrimSpace(slot.Scope),
			Slot:       slot.Slot,
			SlotLabel:  strings.TrimSpace(slot.SlotLabel),
			Actions:    slot.Actions,
			Confidence: strings.TrimSpace(slot.Confidence),
		})
	}
	return slots
}

// catalogAddonInfoFor 提取 VPK 内 addoninfo.txt 的字段。
func catalogAddonInfoFor(file parser.VPKFile) *groupingCatalogAddonInfo {
	info := groupingCatalogAddonInfo{
		Version:   strings.TrimSpace(file.Version),
		Desc:      strings.TrimSpace(file.Desc),
		URL:       strings.TrimSpace(file.AddonURL0),
		HasUpdate: file.HasUpdate,
		Chapters:  len(file.Chapters),
		Mode:      strings.TrimSpace(file.Mode),
	}
	if info == (groupingCatalogAddonInfo{}) {
		return nil
	}
	return &info
}

// watchLaterSnapshot 把"稍后再看"里的工坊统计按 ID 建索引。
func (a *App) watchLaterSnapshot() map[string]WorkshopWatchLaterItem {
	result := make(map[string]WorkshopWatchLaterItem)
	store := a.GetWorkshopWatchLaterStorage()
	for _, item := range store.Items {
		id := strings.TrimSpace(item.PublishedFileID)
		if id != "" {
			result[id] = item
		}
	}
	return result
}

// workshopSnapshotFor 汇总工坊信息：.meta 文件 + 合集归属 + 稍后再看统计。
func (a *App) workshopSnapshotFor(file parser.VPKFile, mod grouping.Mod, watchLater map[string]WorkshopWatchLaterItem) *groupingCatalogWorkshop {
	workshopID := strings.TrimSpace(file.WorkshopID)
	snapshot := groupingCatalogWorkshop{}
	if meta, err := LoadWorkshopMeta(file.Path); err == nil && meta != nil {
		if id := strings.TrimSpace(meta.WorkshopID); id != "" {
			workshopID = id
		}
		snapshot.Title = strings.TrimSpace(meta.Title)
		snapshot.Author = strings.TrimSpace(meta.Author)
		snapshot.Desc = strings.TrimSpace(meta.Description)
		snapshot.Tags = meta.Tags
		snapshot.PreviewURL = strings.TrimSpace(meta.PreviewURL)
		snapshot.DownloadedAt = strings.TrimSpace(meta.DownloadedAt)
		snapshot.TimeUpdated = strings.TrimSpace(meta.TimeUpdated)
	}
	snapshot.ID = workshopID
	if workshopID == "" {
		return nil
	}
	snapshot.URL = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + workshopID
	if item, ok := watchLater[workshopID]; ok {
		snapshot.InWatchLater = true
		snapshot.Views = item.Views
		snapshot.Subscriptions = item.Subscriptions
		snapshot.Favorited = item.Favorited
		snapshot.FileType = item.FileType
		if snapshot.Title == "" {
			snapshot.Title = strings.TrimSpace(item.Title)
		}
	}
	_ = mod
	return &snapshot
}

// managementSnapshot 汇总"本地管理记录"：策略组、启用方案、依赖、冲突忽略清单、工坊合集。
func (a *App) managementSnapshot() map[string]*groupingCatalogManagement {
	result := make(map[string]*groupingCatalogManagement)
	ensure := func(key string) *groupingCatalogManagement {
		normalized := normalizeAddonListKey(key)
		if normalized == "" {
			return nil
		}
		record, ok := result[normalized]
		if !ok {
			record = &groupingCatalogManagement{}
			result[normalized] = record
		}
		return record
	}

	if groups, err := a.ListModStrategyGroups(); err == nil {
		for _, group := range groups {
			for _, member := range group.Members {
				if record := ensure(member.Key); record != nil {
					record.Groups = append(record.Groups, group.Name)
				}
			}
		}
	}
	if profiles, err := a.ListModEnableProfiles(); err == nil {
		for _, profile := range profiles {
			for _, entry := range profile.Entries {
				if record := ensure(entry.Name); record != nil {
					record.Profiles = append(record.Profiles, profile.Name)
				}
			}
		}
	}
	if records, err := a.ListModDependencies(); err == nil {
		for _, record := range records {
			if target := ensure(record.Key); target != nil {
				for _, dependency := range record.Dependencies {
					label := strings.TrimSpace(dependency.Name)
					if label == "" {
						label = strings.TrimSpace(dependency.Key)
					}
					if label != "" {
						target.Dependencies = append(target.Dependencies, label)
					}
				}
			}
		}
	}
	if store, err := a.readModIgnoreStore(); err == nil {
		for _, record := range store.Records {
			if target := ensure(record.Key); target != nil && len(record.Files) > 0 {
				target.IgnoredFiles = append(target.IgnoredFiles, record.Files...)
			}
		}
	}
	if store, err := a.readWorkshopCollectionStore(); err == nil {
		for _, link := range store.Links {
			label := strings.TrimSpace(link.Title)
			if label == "" {
				label = "工坊合集 " + link.CollectionID
			}
			for _, member := range link.Members {
				if record := ensure(workshopAddonListKey(member.WorkshopID)); record != nil {
					record.Collections = append(record.Collections, label)
				}
			}
		}
	}

	for key, record := range result {
		if record.Groups == nil && record.Profiles == nil && record.Dependencies == nil &&
			record.IgnoredFiles == nil && record.Collections == nil {
			delete(result, key)
		}
	}
	return result
}

// catalogScope 说明本次扫描覆盖与排除的范围（当前固定为 root + workshop + disabled）。
func (a *App) catalogScope() *groupingCatalogScope {
	scope := &groupingCatalogScope{
		Included: []string{"addons 根目录（不递归）", "addons\\workshop（递归）", "addons\\disabled（递归）"},
		Excluded: []string{"addons 下其它子目录（用户自建套件/库目录，不扫描）"},
		Note:     "范围与游戏实际加载位置一致；其它子目录里的 VPK 不参与列表、冲突分析与分组推导。",
	}
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return scope
	}
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return scope
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if name == "workshop" || name == "disabled" || strings.HasPrefix(name, ".") {
			continue
		}
		scope.ExcludedDirCount++
		scope.ExcludedVpkCount += countVPKFiles(filepath.Join(rootDir, entry.Name()), 0)
	}
	return scope
}

// countVPKFiles 统计目录下的 .vpk 数量（带深度上限，避免异常目录拖慢导出）。
// entryIDIndex 建立 key → entryId 的索引（同键多位置时取第一条；
// 需要精确寻址的地方应直接使用 entry.EntryID）。
func entryIDIndex(entries []groupingCatalogEntry) map[string]string {
	index := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.Key == "" || entry.EntryID == "" {
			continue
		}
		if _, ok := index[entry.Key]; !ok {
			index[entry.Key] = entry.EntryID
		}
	}
	return index
}

func countVPKFiles(dir string, depth int) int {
	if depth > 8 {
		return 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	total := 0
	for _, entry := range entries {
		if entry.IsDir() {
			total += countVPKFiles(filepath.Join(dir, entry.Name()), depth+1)
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
			total++
		}
	}
	return total
}

// ungroupedKeys 找出没有被任何 clusterHint 覆盖的 Mod 键。
func ungroupedKeys(entries []groupingCatalogEntry, clusters []groupingCatalogCluster) []string {
	covered := make(map[string]struct{}, len(entries))
	for _, cluster := range clusters {
		for _, key := range cluster.MemberKeys {
			covered[key] = struct{}{}
		}
	}
	ungrouped := make([]string, 0, len(entries))
	for _, entry := range entries {
		if _, ok := covered[entry.Key]; ok {
			continue
		}
		ungrouped = append(ungrouped, entry.Key)
	}
	return ungrouped
}

// themeTokenStopWords 是"主题名"提取时要跳过的通用词。
var themeTokenStopWords = map[string]struct{}{
	"重置": {}, "修复": {}, "版本": {}, "最终版": {}, "完整版": {}, "无": {}, "版": {},
	"替换": {}, "主题": {}, "皮肤": {}, "模型": {}, "武器": {}, "材质": {}, "贴图": {},
}

// buildThemeHints 找"同一主题名横跨多个替换目标"的套装候选。
func buildThemeHints(entries []groupingCatalogEntry, entryIDByKey map[string]string) []groupingCatalogThemeHint {
	type bucket struct {
		keys     []string
		entryIDs []string
		subjects map[string]struct{}
	}
	buckets := make(map[string]*bucket)
	for _, entry := range entries {
		if entry.SubjectSummary == "" {
			continue
		}
		for _, token := range themeTokens(entry.Title) {
			item, ok := buckets[token]
			if !ok {
				item = &bucket{subjects: map[string]struct{}{}}
				buckets[token] = item
			}
			item.keys = append(item.keys, entry.Key)
			entryID := entry.EntryID
			if entryID == "" {
				entryID = entry.Key
			}
			item.entryIDs = append(item.entryIDs, entryID)
			item.subjects[entry.SubjectSummary] = struct{}{}
		}
	}
	_ = entryIDByKey
	hints := make([]groupingCatalogThemeHint, 0, len(buckets))
	for token, item := range buckets {
		// 只有横跨多个替换目标才叫"套装"；单目标同主题属于普通替换组。
		if len(item.keys) < 3 || len(item.subjects) < 2 {
			continue
		}
		subjects := make([]string, 0, len(item.subjects))
		for subject := range item.subjects {
			subjects = append(subjects, subject)
		}
		sort.Strings(subjects)
		sort.Strings(item.entryIDs)
		sort.Strings(item.keys)
		hints = append(hints, groupingCatalogThemeHint{
			Theme:          token,
			MemberEntryIDs: item.entryIDs,
			MemberKeys:     item.keys,
			Subjects:       subjects,
			Reason:         fmt.Sprintf("标题里都出现「%s」，且覆盖 %d 个不同替换目标（可能是跨槽位套装，建议 all）", token, len(item.subjects)),
		})
	}
	sort.Slice(hints, func(i, j int) bool {
		if len(hints[i].MemberKeys) != len(hints[j].MemberKeys) {
			return len(hints[i].MemberKeys) > len(hints[j].MemberKeys)
		}
		return hints[i].Theme < hints[j].Theme
	})
	if len(hints) > 60 {
		hints = hints[:60]
	}
	return hints
}

// themeTokens 从标题里提取可用于"主题名"的短词：优先中英文连续片段，
// 过滤通用词与纯数字/版本号。
func themeTokens(title string) []string {
	value := strings.TrimSpace(title)
	if value == "" {
		return nil
	}
	// 去掉常见的括号/书名号内容，避免把"（重置）"当主题。
	replaced := strings.NewReplacer("【", " ", "】", " ", "（", " ", "）", " ", "(", " ", ")", " ",
		"[", " ", "]", " ", "「", " ", "」", " ", ":", " ", "：", " ", "-", " ", "_", " ").Replace(value)
	fields := strings.Fields(replaced)
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.TrimSpace(field)
		if len([]rune(token)) < 2 || len([]rune(token)) > 20 {
			continue
		}
		if _, skip := themeTokenStopWords[token]; skip {
			continue
		}
		if isNumericToken(token) {
			continue
		}
		tokens = append(tokens, strings.ToLower(token))
	}
	return tokens
}

func isNumericToken(token string) bool {
	for _, r := range token {
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '.' || r == 'v' || r == 'V' {
			continue
		}
		return false
	}
	return true
}

// buildPreloadHints 找"可能是前置库"的 Mod（名称里带库/前置/Base/lib/KSEP/xdReanims 等特征）。
func buildPreloadHints(entries []groupingCatalogEntry) []groupingCatalogPreloadHint {
	keywords := []string{"音频库", "音效库", "前置", "依赖", "基础库", "工具链", "框架", "xdreanims", "ksep", "base"}
	hints := make([]groupingCatalogPreloadHint, 0, 16)
	for _, entry := range entries {
		haystack := strings.ToLower(entry.Title + " " + entry.Name)
		for _, keyword := range keywords {
			if !strings.Contains(haystack, keyword) {
				continue
			}
			hints = append(hints, groupingCatalogPreloadHint{
				Key:    entry.Key,
				Title:  entry.Title,
				Reason: fmt.Sprintf("名称包含「%s」，可能是被其它 Mod 依赖的前置库（建议单独成组并用 all 常开）", keyword),
			})
			break
		}
		if len(hints) >= 80 {
			break
		}
	}
	return hints
}

// loadOrderSnapshot 返回 addonlist 键 → 1 基顺序号（未记录的不出现在表里）。
func (a *App) loadOrderSnapshot() map[string]int {
	result := make(map[string]int)
	table := a.conflictLoadOrderTable()
	for key, entry := range table.entries {
		result[key] = entry.Index + 1
	}
	return result
}

// cachedVPKFileByPath 按物理路径取缓存里的 VPKFile（找不到时返回零值）。
// 必须用路径而不是 addonlist 键：root 与 disabled 的同名文件共享同一个键。
func (a *App) cachedVPKFileByPath(path string) parser.VPKFile {
	if strings.TrimSpace(path) == "" {
		return parser.VPKFile{}
	}
	if value, ok := a.vpkCache.Load(path); ok {
		if cache, ok := value.(*VPKFileCache); ok && cache != nil {
			return cache.File
		}
	}
	return parser.VPKFile{}
}

// catalogStructureFor 把 VPK 结构摘要转成清单里的结构对象。
func catalogStructureFor(file parser.VPKFile) *groupingCatalogStructure {
	if file.StructureFileCount == 0 && len(file.StructureTopDirs) == 0 {
		return nil
	}
	return &groupingCatalogStructure{
		TopDirs:     file.StructureTopDirs,
		FileCount:   file.StructureFileCount,
		TotalSize:   file.StructureTotalSize,
		SamplePaths: file.StructureSamplePaths,
		Targets:     file.StructureTargets,
	}
}

// buildDuplicateGroups 预计算"疑似同一 Mod 的多个副本"：
// 同名（含根目录 + workshop 两份）、同名且同体积（几乎肯定是同一份文件）。
func buildDuplicateGroups(entries []groupingCatalogEntry) []groupingCatalogDuplicateGroup {
	byName := map[string][]groupingCatalogEntry{}
	byNameSize := map[string][]groupingCatalogEntry{}
	entryIDByKey := map[string]string{}
	for _, entry := range entries {
		name := strings.ToLower(strings.TrimSpace(entry.Name))
		if name == "" {
			continue
		}
		if _, ok := entryIDByKey[entry.Key]; !ok {
			entryIDByKey[entry.Key] = entry.EntryID
		}
		byName[name] = append(byName[name], entry)
		key := fmt.Sprintf("%s\x00%d", name, entry.Size)
		byNameSize[key] = append(byNameSize[key], entry)
	}

	groups := make([]groupingCatalogDuplicateGroup, 0, 32)
	// 同一对副本可能同时命中"同名同体积"和"同名不同位置"：按成员集合去重，
	// 只保留更强的判据（同名同体积优先）。
	seen := make(map[string]struct{}, 64)
	appendGroups := func(buckets map[string][]groupingCatalogEntry, reason string) {
		keys := make([]string, 0, len(buckets))
		for key := range buckets {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			bucket := buckets[key]
			if len(bucket) < 2 {
				continue
			}
			entryIDs := make([]string, 0, len(bucket))
			memberKeys := make([]string, 0, len(bucket))
			names := make([]string, 0, len(bucket))
			for _, entry := range bucket {
				memberKeys = append(memberKeys, entry.Key)
				names = append(names, entry.Title)
				entryIDs = append(entryIDs, entry.EntryID)
			}
			sort.Strings(entryIDs)
			signature := strings.Join(entryIDs, "|")
			if _, ok := seen[signature]; ok {
				continue
			}
			seen[signature] = struct{}{}
			sort.Strings(memberKeys)
			groups = append(groups, groupingCatalogDuplicateGroup{
				Reason:   reason,
				EntryIDs: entryIDs,
				Keys:     memberKeys,
				Names:    names,
			})
			if len(groups) >= 400 {
				return
			}
		}
	}
	appendGroups(byNameSize, "同名同体积")
	appendGroups(byName, "同名不同位置")
	return groups
}

type groupingCatalogEntry struct {
	Key string `json:"key"`
	// EntryID 是稳定且唯一的寻址标识（`<location>/<key>`，例如 `root/123.vpk`、
	// `disabled/123.vpk`）：addonlist 键在同一文件同时存在于根目录与 disabled 时会重复，
	// 只有 EntryID 能唯一指向其中一条记录。
	EntryID           string   `json:"entryId"`
	Name              string   `json:"name"`
	Title             string   `json:"title"`
	Author            string   `json:"author,omitempty"`
	PrimaryTag        string   `json:"primaryTag,omitempty"`
	SecondaryTags     []string `json:"secondaryTags,omitempty"`
	SubjectSummary    string   `json:"subjectSummary,omitempty"`
	SubjectConfidence string   `json:"subjectConfidence,omitempty"`
	ContentSubjects   []string `json:"contentSubjects,omitempty"`
	VoiceCharacters   []string `json:"voiceCharacters,omitempty"`
	XDRSummary        string   `json:"xdrSummary,omitempty"`
	ModelCount        int      `json:"modelCount,omitempty"`
	ModelTriangles    int      `json:"modelTriangles,omitempty"`
	Campaign          string   `json:"campaign,omitempty"`
	WorkshopID        string   `json:"workshopId,omitempty"`
	Folder            string   `json:"folder,omitempty"`
	Location          string   `json:"location,omitempty"`
	// RelativePath 是相对 addons 根目录的物理路径（保留 disabled / workshop 前缀）。
	RelativePath      string   `json:"relativePath,omitempty"`
	GameEnabled       bool     `json:"gameEnabled"`
	GameStateKnown    bool     `json:"gameStateKnown"`
	Size              int64    `json:"size,omitempty"`
	LastModified      string   `json:"lastModified,omitempty"`
	LoadOrder         int      `json:"loadOrder,omitempty"`
	EffectiveLayer    *int     `json:"effectiveLayer,omitempty"`
	PrioritySource    string   `json:"prioritySource,omitempty"`
	// Structure 是 VPK 内部结构摘要（顶层目录、条目数、体积、代表性资源路径）。
	Structure *groupingCatalogStructure `json:"structure,omitempty"`
	// AddonInfo 来自 VPK 内的 addoninfo.txt（版本、描述、主页、更新标记等）。
	AddonInfo *groupingCatalogAddonInfo `json:"addonInfo,omitempty"`
	// Workshop 来自同名 .meta（工坊标题 / 作者 / 简介 / 标签 / 预览图 / 收藏数据）。
	Workshop *groupingCatalogWorkshop `json:"workshop,omitempty"`
	// Management 是 LytVPK 里与该 Mod 相关的本地管理记录。
	Management *groupingCatalogManagement `json:"management,omitempty"`
	// XDRSlots 是 xdReanims 骨骼槽证据（角色 / 模型 / 槽位 / 动作），角色模型分组的关键依据。
	XDRSlots []groupingCatalogXDRSlot `json:"xdrSlots,omitempty"`
}

type groupingCatalogXDRSlot struct {
	Character  string   `json:"character,omitempty"`
	Model      string   `json:"model,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	Slot       int      `json:"slot,omitempty"`
	SlotLabel  string   `json:"slotLabel,omitempty"`
	Actions    []string `json:"actions,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
}

type groupingCatalogAddonInfo struct {
	Version   string `json:"version,omitempty"`
	Desc      string `json:"desc,omitempty"`
	URL       string `json:"url,omitempty"`
	HasUpdate bool   `json:"hasUpdate,omitempty"`
	Chapters  int    `json:"chapters,omitempty"`
	Mode      string `json:"mode,omitempty"`
}

type groupingCatalogWorkshop struct {
	ID            string   `json:"id,omitempty"`
	Title         string   `json:"title,omitempty"`
	Author        string   `json:"author,omitempty"`
	Desc          string   `json:"desc,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	URL           string   `json:"url,omitempty"`
	PreviewURL    string   `json:"previewUrl,omitempty"`
	DownloadedAt  string   `json:"downloadedAt,omitempty"`
	TimeUpdated   string   `json:"timeUpdated,omitempty"`
	InWatchLater  bool     `json:"inWatchLater,omitempty"`
	Views         int      `json:"views,omitempty"`
	Subscriptions int      `json:"subscriptions,omitempty"`
	Favorited     int      `json:"favorited,omitempty"`
	FileType      int      `json:"fileType,omitempty"`
}

type groupingCatalogManagement struct {
	Groups       []string `json:"groups,omitempty"`
	Profiles     []string `json:"profiles,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	IgnoredFiles []string `json:"ignoredFiles,omitempty"`
	Collections  []string `json:"collections,omitempty"`
}

type groupingCatalogStructure struct {
	TopDirs     []string `json:"topDirs,omitempty"`
	FileCount   int      `json:"fileCount"`
	TotalSize   int64    `json:"totalSize"`
	SamplePaths []string `json:"samplePaths,omitempty"`
	// Targets 是压缩后的"替换目标"（如 props_interiors/medicalcabinet02），
	// 比原始路径更适合直接给大模型比较。
	Targets []string `json:"targets,omitempty"`
}

type groupingCatalogDuplicateGroup struct {
	Reason string   `json:"reason"`
	// EntryIDs 用 entryId 寻址；Keys 仅作为兼容展示。
	EntryIDs []string `json:"entryIds"`
	Keys     []string `json:"keys,omitempty"`
	Names    []string `json:"names,omitempty"`
}
