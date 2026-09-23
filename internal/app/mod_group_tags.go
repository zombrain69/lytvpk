package app

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"vpk-manager/internal/grouping"
	"vpk-manager/internal/parser"
)

// 标签 × 策略组的互补实现（App 层）。
//
// 两条互补方向：
//  1. 标签能精确选出同一批 Mod 时，分组建议会被标注（TagKey/TagScope），
//     排序降权并沉到后面 —— 用户直接用标签筛选即可，不必再建组；
//  2. 反过来，策略组（尤其是外部智能体导入的高精度建议）可以作为"这批 Mod 属于同一类"
//     的可靠证据，给它们补一个统一标签，让标签筛选也变得精确。
//
// 所有写入都复用既有的 SetVPKTags（标签写在文件名 / 工坊 .meta 里），
// 这里只负责"算哪些 Mod 该打什么标签"。

const (
	// 组名直接当标签时的长度上限（超过就回退到主体识别结果）。
	modGroupTagMaxRunes = 24
	// 标签至少要有两个成员才值得补（单成员的"类别"没有意义）。
	modGroupTagMinMembers = 2
)

// validateSuggestedTag 校验外部建议文件里的标签：返回空串表示可用。
//
// 规则与 LytVPK 的标签写入保持一致（标签写在文件名里，见 parser.SanitizeTag）：
//   - 不能包含 `+`、`,`、`[`、`]` 等会破坏文件名或标签解析的字符；
//   - 长度不超过 modGroupTagMaxRunes（太长会顶掉文件名本体）。
func validateSuggestedTag(tag string) string {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return ""
	}
	if utf8.RuneCountInString(trimmed) > modGroupTagMaxRunes {
		return fmt.Sprintf("tag 超过 %d 个字符：%s", modGroupTagMaxRunes, trimmed)
	}
	if strings.ContainsAny(trimmed, "+,[],<>:\"/\\|?*") {
		return "tag 含有会破坏文件名或标签解析的字符（+ , [ ] < > : \" / \\ | ? *）：" + trimmed
	}
	return ""
}

// ModGroupTagSuggestion 是"给这个组补一个统一标签"的提案。
type ModGroupTagSuggestion struct {
	GroupID      string `json:"groupId,omitempty"`
	SuggestionID string `json:"suggestionId,omitempty"`
	// Source 说明提案来自哪里：group = 已保存的策略组，suggestion = 导入的外部建议。
	Source string `json:"source"`
	Name   string `json:"name"`
	Tag    string `json:"tag"`
	// TagOrigin 说明标签怎么来的：label（组名）、subject（主体识别）、common-tag（已有共同标签）。
	TagOrigin string `json:"tagOrigin"`
	// TagReason 是外部建议给出的"为什么打这个标签"（可选）。
	TagReason     string   `json:"tagReason,omitempty"`
	MemberCount   int      `json:"memberCount"`
	AlreadyTagged int      `json:"alreadyTagged"`
	MissingCount  int      `json:"missingCount"`
	Keys          []string `json:"keys"`
	Names         []string `json:"names"`
	// MemberTags 是外部建议给出的"逐成员标签"（可选）：比整组标签更细，
	// 界面可以按"标签 → 成员"分组逐个应用。
	MemberTags map[string][]string `json:"memberTags,omitempty"`
}

// ModTagApplyResult 是一次"给一批 Mod 打标签"的结果。
type ModTagApplyResult struct {
	Tag     string   `json:"tag"`
	Applied []string `json:"applied"`
	Skipped []string `json:"skipped"`
	Missing []string `json:"missing"`
	Failed  []string `json:"failed"`
	Reasons []string `json:"reasons,omitempty"`
}

// normalizeTagForCompare 用于判断"这个 Mod 是否已经有这个标签"。
func normalizeTagForCompare(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

func modHasTag(mod grouping.Mod, tag string) bool {
	wanted := normalizeTagForCompare(tag)
	if wanted == "" {
		return false
	}
	if normalizeTagForCompare(mod.PrimaryTag) == wanted {
		return true
	}
	for _, existing := range mod.SecondaryTags {
		if normalizeTagForCompare(existing) == wanted {
			return true
		}
	}
	return false
}

// deriveTagFromName 把组名压成可用标签：去掉会破坏文件名的字符，过长则放弃。
func deriveTagFromName(name string) string {
	cleaned := strings.TrimSpace(parser.SanitizeTag(strings.TrimSpace(name)))
	cleaned = strings.Trim(cleaned, "_ ")
	if cleaned == "" || utf8.RuneCountInString(cleaned) > modGroupTagMaxRunes {
		return ""
	}
	return cleaned
}

// deriveTagForMemberSet 为一批成员挑一个标签，并说明来源。
//
//	exact 覆盖  -> 返回空（已经能用现有标签筛出来，不需要新增标签）
//	partial 覆盖 -> 用那个共同标签补齐剩下的成员
//	否则        -> 组名（够短时），再退化到成员共同的主体识别
func deriveTagForMemberSet(mods []grouping.Mod, name string, keys []string) (string, string) {
	coverage, ok := grouping.TagCoverageFor(mods, keys)
	if ok {
		if coverage.Scope == grouping.TagScopeExact {
			return "", ""
		}
		if coverage.Scope == grouping.TagScopePartial {
			return coverage.Tag, "common-tag"
		}
	}
	if tag := deriveTagFromName(name); tag != "" {
		return tag, "label"
	}
	if subject := dominantSubject(mods, keys); subject != "" {
		if tag := deriveTagFromName(subject); tag != "" {
			return tag, "subject"
		}
	}
	return "", ""
}

// dominantSubject 找出集合内出现次数最多的主体识别结果（要求至少两个成员一致）。
func dominantSubject(mods []grouping.Mod, keys []string) string {
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if normalized := normalizeAddonListKey(key); normalized != "" {
			wanted[normalized] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return ""
	}
	counts := make(map[string]int)
	display := make(map[string]string)
	for _, mod := range mods {
		if _, inSet := wanted[normalizeAddonListKey(mod.Key)]; !inSet {
			continue
		}
		subject := strings.TrimSpace(mod.Subject)
		if subject == "" {
			continue
		}
		key := strings.ToLower(subject)
		counts[key]++
		if _, exists := display[key]; !exists {
			display[key] = subject
		}
	}
	best := ""
	bestCount := 0
	for key, count := range counts {
		if count < modGroupTagMinMembers {
			continue
		}
		if count > bestCount || (count == bestCount && (best == "" || key < best)) {
			best = key
			bestCount = count
		}
	}
	if best == "" {
		return ""
	}
	return display[best]
}

// buildTagSuggestion 组装一条提案；不需要补标签时返回 ok=false。
//
// preferredTag / preferredReason / memberTags 来自外部建议文件（智能体已经判断过内容）：
// 只要给了合法标签就优先采用，规则推导只在这时缺位时才兜底。
func buildTagSuggestion(
	mods []grouping.Mod,
	byKey map[string]grouping.Mod,
	source, id, name string,
	keys, names []string,
	preferredTag string,
	preferredReason string,
	memberTags map[string][]string,
) (ModGroupTagSuggestion, bool) {
	if len(keys) < modGroupTagMinMembers {
		return ModGroupTagSuggestion{}, false
	}
	tag := strings.TrimSpace(preferredTag)
	origin := ""
	reason := ""
	if tag != "" && validateSuggestedTag(tag) == "" {
		origin = "imported"
		reason = strings.TrimSpace(preferredReason)
	} else {
		tag, origin = deriveTagForMemberSet(mods, name, keys)
	}
	if tag == "" {
		return ModGroupTagSuggestion{}, false
	}
	suggestion := ModGroupTagSuggestion{
		Source:      source,
		Name:        strings.TrimSpace(name),
		Tag:         tag,
		TagOrigin:   origin,
		MemberCount: len(keys),
		Keys:        append([]string(nil), keys...),
		Names:       append([]string(nil), names...),
		TagReason:   reason,
		MemberTags:  memberTags,
	}
	if source == "group" {
		suggestion.GroupID = id
	} else {
		suggestion.SuggestionID = id
	}
	for _, key := range keys {
		mod, exists := byKey[normalizeAddonListKey(key)]
		if !exists {
			suggestion.MissingCount++
			continue
		}
		if modHasTag(mod, tag) {
			suggestion.AlreadyTagged++
		}
	}
	if suggestion.AlreadyTagged >= suggestion.MemberCount {
		// 全都有这个标签了，没有可补的。
		return ModGroupTagSuggestion{}, false
	}
	return suggestion, true
}

// GetGroupTagSuggestions 返回"可以补统一标签"的策略组与导入建议。
//
// 只读：不写任何文件。前端拿它显示"给这组打标签…"的候选与影响范围。
func (a *App) GetGroupTagSuggestions() ([]ModGroupTagSuggestion, error) {
	mods := a.groupingMods()
	byKey := make(map[string]grouping.Mod, len(mods))
	for _, mod := range mods {
		byKey[normalizeAddonListKey(mod.Key)] = mod
	}

	result := make([]ModGroupTagSuggestion, 0, 16)
	groups, err := a.ListModStrategyGroups()
	if err == nil {
		for _, group := range groups {
			keys := make([]string, 0, len(group.Members))
			names := make([]string, 0, len(group.Members))
			for _, member := range group.Members {
				keys = append(keys, member.Key)
				names = append(names, groupMemberDisplayName(member))
			}
			if item, ok := buildTagSuggestion(
				mods, byKey, "group", group.ID, group.Name, keys, names, "", "", nil,
			); ok {
				result = append(result, item)
			}
		}
	}

	// 外部导入的高精度建议同样可以反过来"沉淀"成标签。
	if external, err := a.GetExternalGroupSuggestions(); err == nil {
		for _, suggestion := range external {
			if item, ok := buildTagSuggestion(
				mods,
				byKey,
				"suggestion",
				suggestion.ID,
				suggestion.Label,
				suggestion.MemberKeys,
				suggestion.MemberNames,
				// 智能体在文件里给过标签就优先用它；没给则降级到规则推导。
				suggestion.Tag,
				suggestion.TagReason,
				suggestion.MemberTags,
			); ok {
				result = append(result, item)
			}
		}
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].MemberCount != result[j].MemberCount {
			return result[i].MemberCount > result[j].MemberCount
		}
		if result[i].Source != result[j].Source {
			return result[i].Source == "group"
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// ApplyTagToModKeys 给一批 addonlist 键对应的 Mod 追加同一个二级标签。
//
// 复用 SetVPKTags：标签写进文件名（root / disabled）或工坊 .meta，不改变游戏开关与加载顺序。
// 缺失文件、已经带该标签的文件分别报告，不会因为个别失败而中断整批。
func (a *App) ApplyTagToModKeys(tag string, keys []string) (ModTagApplyResult, error) {
	normalizedTag := strings.TrimSpace(parser.SanitizeTag(strings.TrimSpace(tag)))
	if normalizedTag == "" {
		return ModTagApplyResult{}, fmt.Errorf("标签不能为空")
	}
	targetKeys := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, rawKey := range keys {
		key := normalizeAddonListKey(rawKey)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		targetKeys = append(targetKeys, key)
	}
	if len(targetKeys) == 0 {
		return ModTagApplyResult{}, fmt.Errorf("请至少选择一个 Mod")
	}

	byKey := a.cachedFilesByAddonListKey()
	result := ModTagApplyResult{Tag: normalizedTag}
	for _, key := range targetKeys {
		file, exists := byKey[key]
		if !exists {
			result.Missing = append(result.Missing, key)
			continue
		}
		if modHasTag(grouping.Mod{
			PrimaryTag:    file.PrimaryTag,
			SecondaryTags: file.SecondaryTags,
		}, normalizedTag) {
			result.Skipped = append(result.Skipped, key)
			continue
		}
		secondary := append(append([]string(nil), file.SecondaryTags...), normalizedTag)
		if err := a.SetVPKTags(file.Path, file.PrimaryTag, secondary); err != nil {
			result.Failed = append(result.Failed, key)
			result.Reasons = append(result.Reasons, fmt.Sprintf("%s: %v", file.Name, err))
			continue
		}
		result.Applied = append(result.Applied, key)
	}
	return result, nil
}

// cachedFilesByAddonListKey 建立 addonlist 键 -> 当前缓存文件 的索引。
func (a *App) cachedFilesByAddonListKey() map[string]VPKFile {
	rootDir := a.rootDirectorySnapshot()
	byKey := make(map[string]VPKFile, 2048)
	if rootDir == "" {
		return byKey
	}
	a.vpkCache.Range(func(_ any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, cache.File.Path)
		if err != nil || key == "" {
			return true
		}
		byKey[normalizeAddonListKey(key)] = cache.File
		return true
	})
	return byKey
}
