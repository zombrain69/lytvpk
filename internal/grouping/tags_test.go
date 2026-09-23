package grouping

import "testing"

func tagTestMods() []Mod {
	return []Mod{
		{Key: "sg552-a.vpk", Name: "sg552-a.vpk", SecondaryTags: []string{"sg552", "皮肤"}, Subject: "sg552 武器"},
		{Key: "sg552-b.vpk", Name: "sg552-b.vpk", SecondaryTags: []string{"sg552"}, Subject: "sg552 武器"},
		{Key: "sg552-c.vpk", Name: "sg552-c.vpk", SecondaryTags: []string{"sg552"}, Subject: "sg552 武器"},
		{Key: "ak47-a.vpk", Name: "ak47-a.vpk", SecondaryTags: []string{"ak47"}, Subject: "AK47 武器"},
		{Key: "other.vpk", Name: "other.vpk", PrimaryTag: "武器", Subject: "小手枪"},
	}
}

func TestTagCoverageExactWhenTagSelectsExactlyTheSet(t *testing.T) {
	mods := tagTestMods()
	coverage, ok := TagCoverageFor(mods, []string{"sg552-a.vpk", "sg552-b.vpk", "sg552-c.vpk"})
	if !ok {
		t.Fatal("应当找到覆盖标签")
	}
	if coverage.Tag != "sg552" || coverage.Scope != TagScopeExact {
		t.Fatalf("应当判定为精确覆盖: %#v", coverage)
	}
	if coverage.InSet != 3 || coverage.Outside != 0 || coverage.SetSize != 3 {
		t.Fatalf("覆盖统计不对: %#v", coverage)
	}
}

func TestTagCoverageWideWhenTagAlsoMatchesOutsideMods(t *testing.T) {
	mods := tagTestMods()
	// 集合只取两个带 sg552 的 Mod，第三个带同一个标签但不在集合里 → 不算精确。
	coverage, ok := TagCoverageFor(mods, []string{"sg552-a.vpk", "sg552-b.vpk"})
	if !ok {
		t.Fatal("应当找到覆盖标签")
	}
	if coverage.Tag != "sg552" {
		t.Fatalf("覆盖标签 = %q", coverage.Tag)
	}
	if coverage.Scope == TagScopeExact {
		t.Fatalf("有集合外的同标签 Mod 时不能算精确覆盖: %#v", coverage)
	}
	if coverage.Outside != 1 || coverage.InSet != 2 {
		t.Fatalf("覆盖统计不对: %#v", coverage)
	}
}

func TestTagCoverageIgnoresTagsBelowThreshold(t *testing.T) {
	mods := tagTestMods()
	// 三个成员里每个标签最多只命中 1 个：这不构成"标签能代表这一组"的证据。
	if coverage, ok := TagCoverageFor(mods, []string{"sg552-a.vpk", "ak47-a.vpk", "other.vpk"}); ok {
		t.Fatalf("单成员命中的标签不应作为覆盖证据: %#v", coverage)
	}
}

func TestTagCoverageEmptySetReturnsFalse(t *testing.T) {
	if _, ok := TagCoverageFor(tagTestMods(), nil); ok {
		t.Fatal("空集合不应有覆盖结论")
	}
}

func TestPrimaryTagCountsAsCoverage(t *testing.T) {
	mods := []Mod{
		{Key: "a.vpk", PrimaryTag: "音乐"},
		{Key: "b.vpk", PrimaryTag: "音乐"},
	}
	coverage, ok := TagCoverageFor(mods, []string{"a.vpk", "b.vpk"})
	if !ok || coverage.Tag != "音乐" || coverage.Scope != TagScopeExact {
		t.Fatalf("一级标签也应参与覆盖判定: %#v ok=%v", coverage, ok)
	}
}

func TestTagFrequenciesForSetPrefersFrequentTags(t *testing.T) {
	frequencies := TagFrequenciesForSet(tagTestMods(), []string{"sg552-a.vpk", "sg552-b.vpk", "sg552-c.vpk"})
	if len(frequencies) == 0 {
		t.Fatal("应当给出标签频次")
	}
	if frequencies[0].Tag != "sg552" || frequencies[0].Count != 3 {
		t.Fatalf("最高频标签 = %#v", frequencies[0])
	}
}

func TestSuggestAnnotatesTagCoveredSuggestions(t *testing.T) {
	mods := []Mod{
		{Key: "sg-a.vpk", Name: "sg-a.vpk", SecondaryTags: []string{"sg552"}},
		{Key: "sg-b.vpk", Name: "sg-b.vpk", SecondaryTags: []string{"sg552"}},
		{Key: "ak-a.vpk", Name: "ak-a.vpk"},
		{Key: "ak-b.vpk", Name: "ak-b.vpk"},
	}
	options := Options{Injected: []Candidate{
		// 两条建议分数完全相同：唯一的差别是"SG 组"已经能被标签精确筛出来。
		{
			Provider: "external",
			Label:    "SG 组",
			Reason:   "外部建议",
			Score:    90,
			Signals:  []string{"外部建议"},
			Keys:     []string{"sg-a.vpk", "sg-b.vpk"},
			Names:    []string{"sg-a.vpk", "sg-b.vpk"},
		},
		{
			Provider: "external",
			Label:    "AK 组",
			Reason:   "外部建议",
			Score:    90,
			Signals:  []string{"外部建议"},
			Keys:     []string{"ak-a.vpk", "ak-b.vpk"},
			Names:    []string{"ak-a.vpk", "ak-b.vpk"},
		},
	}}

	suggestions, _ := Suggest(mods, options)
	if len(suggestions) != 2 {
		t.Fatalf("应有 2 条建议: %#v", suggestions)
	}
	// 引擎只负责"贴标注"：排序语义（谁更像一个真实的组）不受标签影响，
	// "标签已覆盖 → 沉到后面"由 app 层的展示排序负责。
	byLabel := map[string]Suggestion{}
	for _, item := range suggestions {
		byLabel[item.Label] = item
	}
	covered, exists := byLabel["SG 组"]
	if !exists {
		t.Fatalf("缺少 SG 组: %#v", suggestions)
	}
	if covered.TagKey != "sg552" || covered.TagScope != TagScopeExact {
		t.Fatalf("应标注精确覆盖的标签: %#v", covered)
	}
	if covered.TagInSet != 2 || covered.TagOutside != 0 {
		t.Fatalf("覆盖统计不对: %#v", covered)
	}
	if ak := byLabel["AK 组"]; ak.TagScope != "" {
		t.Fatalf("AK 组没有共同标签，不应被标注: %#v", ak)
	}
}

func TestSuggestKeepsTagCoverageFieldsEmptyWithoutTags(t *testing.T) {
	mods := []Mod{
		{Key: "a.vpk", SecondaryTags: []string{"ak47"}},
		{Key: "b.vpk", SecondaryTags: []string{"ak47"}},
	}
	options := Options{Injected: []Candidate{{
		Provider: "external",
		Label:    "AK 组",
		Score:    90,
		Signals:  []string{"外部建议"},
		Keys:     []string{"a.vpk", "b.vpk"},
	}}}
	suggestions, _ := Suggest(mods, options)
	if len(suggestions) != 1 {
		t.Fatalf("应有 1 条建议: %#v", suggestions)
	}
	if suggestions[0].TagScope != TagScopeExact || suggestions[0].TagInSet != 2 {
		t.Fatalf("标签恰好覆盖时应标注 exact: %#v", suggestions[0])
	}
}
