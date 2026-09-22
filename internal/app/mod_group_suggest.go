package app

import (
	"path/filepath"
	"strings"

	"vpk-manager/internal/grouping"
)

// 这一层只做"适配"：把 App 的缓存 / 本地记录翻译成推导引擎的输入，
// 再把引擎输出翻译成前端模型。分组算法本身在 internal/grouping（纯逻辑、可基准测试）。

// ModGroupSuggestion 是一条"建议的分组"，用户可以勾选/调整后再保存成策略组。
type ModGroupSuggestion struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Reason      string   `json:"reason"`
	Confidence  string   `json:"confidence"` // high | medium | low
	Score       int      `json:"score"`
	Signals     []string `json:"signals"`
	MemberKeys  []string `json:"memberKeys"`
	MemberNames []string `json:"memberNames"`
	// ExistingGroupID 非空表示这批 Mod 已经属于同一个策略组，无需重复创建。
	ExistingGroupID string `json:"existingGroupId,omitempty"`
	// Source 说明建议来源：空 = 内置推导，"external" = 外部建议文件（模型 / 人工）。
	Source string `json:"source,omitempty"`
	// Strategy 是外部建议给出的建组策略（single/single_random/all/off），空表示默认互斥单选。
	Strategy string `json:"strategy,omitempty"`
}

// 信号名与限制值：与 internal/grouping 的信号目录保持一致，
// 这里保留同名常量是为了让 App 层代码与测试可读（有测试守住二者不漂移）。
const (
	modGroupSignalCollection     = "工坊合集成员"
	modGroupSignalFilenamePrefix = "文件名前缀"
	modGroupSignalSharedTags     = "共同标签"
	modGroupSignalSameAuthor     = "同一作者"
	modGroupSignalSubject        = "主体识别"
	modGroupSignalVoice          = "语音角色"
	modGroupSignalFolder         = "同一文件夹"
	modGroupSignalSameFileName   = "同名文件（不同目录）"

	modGroupSuggestionLimit             = 40
	modGroupSuggestionMinMembers        = 2
	modGroupSuggestionMaxMembers        = 60
	modGroupSuggestionMaxCuratedMembers = 300
	modGroupSuggestionMaxAuthorMembers  = 12
	// 导入建议很多时放宽弹窗展示上限（避免"文件有 195 条却只看到 40 条"）。
	modGroupSuggestionMaxDisplayLimit = 240

	// 文件夹建议的基准分（与 internal/grouping 的目录保持一致，有测试守住）。
	modGroupSuggestionFolderScore = grouping.FolderScore
)

// SuggestModGroups 生成分组建议：内置信号 + 工坊合集（本地记录）+ 外部建议文件。
func (a *App) SuggestModGroups() ([]ModGroupSuggestion, error) {
	mods := a.groupingMods()
	if len(mods) < modGroupSuggestionMinMembers {
		return []ModGroupSuggestion{}, nil
	}

	groups, err := a.ListModStrategyGroups()
	if err != nil {
		groups = nil
	}
	existingByKey := make(map[string]string)
	for _, group := range groups {
		for _, member := range group.Members {
			if key := normalizeAddonListKey(member.Key); key != "" {
				existingByKey[key] = group.ID
			}
		}
	}

	options := grouping.DefaultOptions()
	// 引擎层不要提前截断：导入的建议文件可能有上百条，截断要放到下面按"外部建议数"计算，
	// 否则外部建议会在引擎里被默认的 40 条上限吃掉（实测只回 40 条）。
	options.Limit = modGroupSuggestionMaxDisplayLimit
	options.DisableQuotaForIDs = []string{"collection", "external"}
	options.Injected = append(options.Injected, a.collectionCandidates(mods)...)
	if external, err := a.externalSuggestionCandidates(); err == nil {
		options.Injected = append(options.Injected, external...)
	}

	ranked, _ := grouping.Suggest(mods, options)
	suggestions := make([]ModGroupSuggestion, 0, len(ranked))
	for _, item := range ranked {
		suggestions = append(suggestions, ModGroupSuggestion{
			ID:              item.ID,
			Label:           item.Label,
			Reason:          item.Reason,
			Confidence:      item.Confidence,
			Score:           item.Score,
			Signals:         item.Signals,
			MemberKeys:      item.MemberKeys,
			MemberNames:     item.MemberNames,
			ExistingGroupID: existingGroupIDForKeys(existingByKey, item.MemberKeys),
			Source:          item.Source,
			Strategy:        item.Strategy,
		})
	}

	// 已经整组建过的建议只作为"已处理"提示，不能占满上限：
	// 否则用户建完组后列表完全不翻页，看不到后面的新建议。
	sortSuggestionsForDisplay(suggestions)
	limit := modGroupSuggestionLimit
	// 导入的建议文件可能有上百条（实测 195 条），此时放宽展示上限，
	// 否则用户只能看到前 40 条、以为文件没导进来。
	externalCount := 0
	for _, item := range suggestions {
		if item.Source == modGroupSuggestionSourceExternal {
			externalCount++
		}
	}
	if externalCount > limit {
		// 保留全部导入建议，同时给内置启发式留出正常份额。
		limit = externalCount + modGroupSuggestionLimit
		if limit > modGroupSuggestionMaxDisplayLimit {
			limit = modGroupSuggestionMaxDisplayLimit
		}
	}
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}
	return suggestions, nil
}

// sortSuggestionsForDisplay 把"已建组"的建议沉到末尾，其余保持引擎给出的排序。
func sortSuggestionsForDisplay(suggestions []ModGroupSuggestion) {
	stablePartition(suggestions, func(item ModGroupSuggestion) bool { return item.ExistingGroupID != "" })
}

// stablePartition 把满足 moveToEnd 的条目稳定地移到切片末尾。
func stablePartition[T any](items []T, moveToEnd func(T) bool) {
	kept := make([]T, 0, len(items))
	moved := make([]T, 0, len(items))
	for _, item := range items {
		if moveToEnd(item) {
			moved = append(moved, item)
			continue
		}
		kept = append(kept, item)
	}
	copy(items, append(kept, moved...))
}

// groupingMods 把 VPK 缓存翻译成推导引擎的输入（一次遍历）。
func (a *App) groupingMods() []grouping.Mod {
	mods := make([]grouping.Mod, 0, 2048)
	rootDir := a.rootDirectorySnapshot()
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
		mods = append(mods, grouping.Mod{
			Key:               key,
			Name:              file.Name,
			Title:             file.Title,
			Author:            strings.TrimSpace(file.Author),
			PrimaryTag:        strings.TrimSpace(file.PrimaryTag),
			SecondaryTags:     normalizeTagList(file.SecondaryTags),
			Subject:           strings.TrimSpace(file.SubjectSummary),
			SubjectConfidence: strings.TrimSpace(file.SubjectConfidence),
			VoiceCharacters:   normalizeVoiceCharacterList(file.VoiceCharacters),
			Folder:            groupingDirForPath(rootDir, file.Path),
			WorkshopID:        strings.TrimSpace(file.WorkshopID),
			SourcePath:        file.Path,
		})
		return true
	})
	return mods
}

// collectionCandidates 把本地保存的"工坊合集"记录翻译成注入候选。
func (a *App) collectionCandidates(mods []grouping.Mod) []grouping.Candidate {
	store, err := a.readWorkshopCollectionStore()
	if err != nil || len(store.Links) == 0 {
		return nil
	}
	byWorkshopID := make(map[string]grouping.Mod, len(mods))
	for _, mod := range mods {
		if mod.WorkshopID != "" {
			byWorkshopID[mod.WorkshopID] = mod
		}
	}
	candidates := make([]grouping.Candidate, 0, len(store.Links))
	for _, link := range store.Links {
		keys := make([]string, 0, len(link.Members))
		names := make([]string, 0, len(link.Members))
		for _, member := range link.Members {
			mod, ok := byWorkshopID[strings.TrimSpace(member.WorkshopID)]
			if !ok {
				continue
			}
			keys = append(keys, mod.Key)
			names = append(names, collectionMemberDisplayName(mod))
		}
		if len(keys) < modGroupSuggestionMinMembers {
			continue
		}
		label := strings.TrimSpace(link.Title)
		if label == "" {
			label = "工坊合集 " + link.CollectionID
		}
		candidates = append(candidates, grouping.Candidate{
			Provider: "collection",
			Label:    label,
			Reason:   "同属工坊合集「" + label + "」",
			Score:    80,
			Signals:  []string{modGroupSignalCollection},
			Keys:     keys,
			Names:    names,
		})
	}
	return candidates
}

func collectionMemberDisplayName(mod grouping.Mod) string {
	if strings.TrimSpace(mod.Title) != "" {
		return mod.Title
	}
	if strings.TrimSpace(mod.Name) != "" {
		return mod.Name
	}
	return grouping.BaseName(mod.Key)
}

// resolveModKey 把外部建议里的成员写法解析成 addons 相对键（供导入模块复用）。
func (a *App) resolveModKey(raw string) (string, error) {
	index := a.buildModKeyIndex()
	matches, _ := index.resolve(raw)
	if len(matches) == 0 {
		return "", errUnknownModKey(raw)
	}
	return matches[0], nil
}

// normalizeTagList 去重并保持顺序。
func normalizeTagList(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		value := strings.TrimSpace(tag)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

// groupingDirForPath 取 Mod 相对 addons 根目录的目录（保留原始大小写，用于展示）。
func groupingDirForPath(rootDir, filePath string) string {
	if rootDir == "" || filePath == "" {
		return ""
	}
	relative, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return ""
	}
	relative = filepath.ToSlash(relative)
	index := strings.LastIndex(relative, "/")
	if index <= 0 {
		return ""
	}
	return filepath.FromSlash(relative[:index])
}

// normalizeVoiceCharacterList 规范化语音角色名（去重、去空、保留原始大小写）。
func normalizeVoiceCharacterList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}

// existingGroupIDForKeys 判断这批键是否已经完整属于同一个策略组。
func existingGroupIDForKeys(existingByKey map[string]string, keys []string) string {
	existingGroupID := ""
	for _, key := range keys {
		id, ok := existingByKey[key]
		if !ok {
			return ""
		}
		if existingGroupID == "" {
			existingGroupID = id
			continue
		}
		if existingGroupID != id {
			return ""
		}
	}
	return existingGroupID
}

// modGroupSuggestionID 生成稳定 ID（同一批成员无论顺序都得到同一个 ID）。
func modGroupSuggestionID(keys []string) string {
	return "sug-" + groupingSignature(keys)
}

// 以下三个包装函数让 App 层测试可以直接断言引擎的评分口径，
// 避免测试与 internal/grouping 的实现细节耦合。
func modGroupSuggestionConfidenceForSignals(signals []string) string {
	return grouping.ConfidenceForSignals(signals)
}

func modGroupSuggestionConfidenceBonus(level string) int {
	return grouping.ConfidenceBonus(level)
}

func modGroupSuggestionSortScore(suggestion ModGroupSuggestion) int {
	return grouping.SortScore(suggestion.Score, suggestion.Signals, len(suggestion.MemberKeys), nil, grouping.DefaultOptions())
}

func groupingSignature(keys []string) string {
	return grouping.Signature(keys)
}

func errUnknownModKey(raw string) error {
	return &unknownModKeyError{raw: raw}
}

type unknownModKeyError struct{ raw string }

func (e *unknownModKeyError) Error() string {
	return "无法在当前 Mod 列表中匹配：" + e.raw
}
