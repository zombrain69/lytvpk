package grouping

import (
	"sort"
	"strings"
)

// 标签 × 分组建议的互补判定。
//
// 背景：LytVPK 的标签（一级 / 二级）直接写在文件名（`.meta`）里，本来就是"用户自己维护的
// 分类"；而分组推导会给同一批 Mod 再产出一条"共同标签"建议。如果某个标签已经**恰好**
// 只圈住这一批 Mod，那这条建议对用户没有增量价值 —— 直接用标签筛选即可，不必再建一个组。
//
// 反过来，外部智能体导入的高精度分组，是正确的"这堆 Mod 属于同一类"的证据，
// 可以反过来给这批 Mod 补上统一的标签（见 app 层的 GetGroupTagSuggestions）。
//
// 这里的判定是纯逻辑：只看 Mod 的标签与成员集合，不碰任何文件。

const (
	// TagScopeExact 表示该标签恰好只选中这个集合（集合内全部命中，集合外没有同标签的 Mod）。
	TagScopeExact = "exact"
	// TagScopePartial 表示集合内多数成员带该标签，但集合外也有同标签的 Mod。
	TagScopePartial = "partial"
	// TagScopeWide 表示标签在集合内覆盖不足一半（只能算"沾边"）。
	TagScopeWide = "wide"

	// tagCoverageMinRatio 是"值得展示"的最低集合内覆盖率。
	tagCoverageMinRatio = 0.6
)

// TagCoverage 描述"某个标签能在多大程度上选出同一批 Mod"。
type TagCoverage struct {
	Tag     string `json:"tag"`
	Scope   string `json:"scope"`
	InSet   int    `json:"inSet"`   // 集合内带该标签的成员数
	SetSize int    `json:"setSize"` // 集合大小
	Outside int    `json:"outside"` // 集合外带该标签的 Mod 数（0 且全覆盖 = 精确）
}

// TagFrequency 是集合内某个标签的出现次数（用于挑选"最能代表这一组"的标签）。
type TagFrequency struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// tagKeysOf 返回一个 Mod 的全部标签（一级 + 二级，去重保序）。
func tagKeysOf(mod Mod) []string {
	result := make([]string, 0, len(mod.SecondaryTags)+1)
	seen := make(map[string]struct{}, len(mod.SecondaryTags)+1)
	appendTag := func(raw string) {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			return
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	appendTag(mod.PrimaryTag)
	for _, tag := range mod.SecondaryTags {
		appendTag(tag)
	}
	return result
}

// TagFrequenciesForSet 统计集合内的标签频次（按次数降序，其次按标签名）。
func TagFrequenciesForSet(mods []Mod, keys []string) []TagFrequency {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" {
			set[key] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	counts := make(map[string]int)
	display := make(map[string]string)
	for _, mod := range mods {
		if _, inSet := set[strings.ToLower(strings.TrimSpace(mod.Key))]; !inSet {
			continue
		}
		for _, tag := range tagKeysOf(mod) {
			key := strings.ToLower(tag)
			counts[key]++
			if _, exists := display[key]; !exists {
				display[key] = tag
			}
		}
	}
	result := make([]TagFrequency, 0, len(counts))
	for key, count := range counts {
		result = append(result, TagFrequency{Tag: display[key], Count: count})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return strings.ToLower(result[i].Tag) < strings.ToLower(result[j].Tag)
	})
	return result
}

// TagCoverageFor 选出"最能代表这个集合"的标签。
//
// 选择顺序：
//  1. 精确覆盖（集合内全命中、集合外没有）——集合本身就可以用标签筛出来；
//  2. 覆盖率 ≥ 60% 且外溢不超过集合内命中数 —— 只能算"部分覆盖"；
//  3. 集合内命中 ≥ 2 的最佳标签 —— "沾边"（用于提示，不用于替代分组）。
//
// 找不到任何在集合内命中 ≥ 2 的标签时返回 ok=false。
func TagCoverageFor(mods []Mod, keys []string) (TagCoverage, bool) {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" {
			set[key] = struct{}{}
		}
	}
	if len(set) == 0 {
		return TagCoverage{}, false
	}

	type stats struct {
		display string
		inSet   int
		total   int
	}
	byTag := make(map[string]*stats)
	for _, mod := range mods {
		_, inSet := set[strings.ToLower(strings.TrimSpace(mod.Key))]
		for _, tag := range tagKeysOf(mod) {
			key := strings.ToLower(tag)
			entry, exists := byTag[key]
			if !exists {
				entry = &stats{display: tag}
				byTag[key] = entry
			}
			entry.total++
			if inSet {
				entry.inSet++
			}
		}
	}

	var best *TagCoverage
	better := func(candidate TagCoverage) bool {
		if best == nil {
			return true
		}
		if candidate.InSet != best.InSet {
			return candidate.InSet > best.InSet
		}
		if candidate.Outside != best.Outside {
			return candidate.Outside < best.Outside
		}
		return len(candidate.Tag) < len(best.Tag)
	}

	for _, entry := range byTag {
		if entry.inSet < 2 {
			continue
		}
		coverage := TagCoverage{
			Tag:     entry.display,
			InSet:   entry.inSet,
			SetSize: len(set),
			Outside: entry.total - entry.inSet,
		}
		switch {
		case entry.inSet == len(set) && coverage.Outside == 0:
			coverage.Scope = TagScopeExact
		case float64(entry.inSet) >= tagCoverageMinRatio*float64(len(set)) &&
			coverage.Outside <= entry.inSet:
			coverage.Scope = TagScopePartial
		default:
			coverage.Scope = TagScopeWide
		}
		// 精确覆盖永远优先；同为精确时选更短的标签名（更贴近"分类名"而不是长描述）。
		if coverage.Scope == TagScopeExact {
			if best == nil || best.Scope != TagScopeExact || better(coverage) {
				copied := coverage
				best = &copied
			}
			continue
		}
		// 精确覆盖已经找到时，部分覆盖不再考虑。
		if best != nil && best.Scope == TagScopeExact {
			continue
		}
		if best == nil || better(coverage) {
			copied := coverage
			best = &copied
		}
	}
	if best == nil {
		return TagCoverage{}, false
	}
	return *best, true
}
