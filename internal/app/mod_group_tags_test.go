package app

import (
	"path/filepath"
	"strings"
	"testing"

	"vpk-manager/internal/grouping"
)

func TestSuggestModGroupsAnnotatesTagCoverage(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// 两个 Mod 都带同一个二级标签：这一组已经能用标签筛出来。
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{
		Name:          "a.vpk",
		SecondaryTags: []string{"sg552"},
		SubjectSummary: "sg552 武器",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{
		Name:          "b.vpk",
		SecondaryTags: []string{"sg552"},
		SubjectSummary: "sg552 武器",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "c.vpk"), VPKFile{
		Name:          "c.vpk",
		SubjectSummary: "AK47 武器",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	var found *ModGroupSuggestion
	for index := range suggestions {
		if len(suggestions[index].MemberKeys) == 2 {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("没有找到 2 个成员的建议: %#v", suggestions)
	}
	if found.TagKey != "sg552" || found.TagScope != grouping.TagScopeExact {
		t.Fatalf("建议应标注「标签已精确覆盖」: %#v", found)
	}
	if found.TagInSet != 2 || found.TagOutside != 0 {
		t.Fatalf("覆盖统计不对: %#v", found)
	}
}

func TestGetGroupTagSuggestionsProposesTagFromGroupName(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk"})
	group, err := a.CaptureModStrategyGroup("M16 武器", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	suggestions, err := a.GetGroupTagSuggestions()
	if err != nil {
		t.Fatalf("suggestions: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("应有 1 条标签建议: %#v", suggestions)
	}
	item := suggestions[0]
	if item.GroupID != group.ID || item.Source != "group" {
		t.Fatalf("来源信息不对: %#v", item)
	}
	if item.Tag != "M16 武器" || item.TagOrigin != "label" {
		t.Fatalf("标签应来自组名: %#v", item)
	}
	if item.MemberCount != 2 || item.AlreadyTagged != 0 || item.MissingCount != 0 {
		t.Fatalf("成员统计不对: %#v", item)
	}
}

func TestGetGroupTagSuggestionsSkipsFullyTaggedAndThinGroups(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{
		Name:          "a.vpk",
		SecondaryTags: []string{"打包组"},
	})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{
		Name:          "b.vpk",
		SecondaryTags: []string{"打包组"},
	})
	// 两个成员都已经带上了组名对应的标签 → 没有可补的标签。
	if _, err := a.CaptureModStrategyGroup("打包组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	}); err != nil {
		t.Fatalf("capture: %v", err)
	}
	// 单成员组不参与（没有"共同的类别"可言）。
	if _, err := a.CaptureModStrategyGroup("只有一个", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "c.vpk"),
	}); err != nil {
		t.Fatalf("capture second: %v", err)
	}

	suggestions, err := a.GetGroupTagSuggestions()
	if err != nil {
		t.Fatalf("suggestions: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("已全带标签的组与单成员组都不应出现在建议里: %#v", suggestions)
	}
}

func TestGetGroupTagSuggestionsUsesSubjectWhenNameIsTooLong(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{
		Name:           "a.vpk",
		SubjectSummary: "sg552 武器",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{
		Name:           "b.vpk",
		SubjectSummary: "sg552 武器",
	})
	longName := "这是一个特别特别长的组名用来说明不能直接当作标签使用因为它会超出文件名里标签的长度限制"
	if _, err := a.CaptureModStrategyGroup(longName, "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	}); err != nil {
		t.Fatalf("capture: %v", err)
	}

	suggestions, err := a.GetGroupTagSuggestions()
	if err != nil {
		t.Fatalf("suggestions: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("应有 1 条标签建议: %#v", suggestions)
	}
	if suggestions[0].Tag != "sg552 武器" || suggestions[0].TagOrigin != "subject" {
		t.Fatalf("组名过长时应回退到主体识别: %#v", suggestions[0])
	}
}

func TestApplyTagToModKeysAddsTagAndSkipsExisting(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{
		Name:          "b.vpk",
		SecondaryTags: []string{"已经有的标签"},
	})

	result, err := a.ApplyTagToModKeys("M16 武器", []string{"a.vpk", "b.vpk", "missing.vpk"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Applied) != 2 {
		t.Fatalf("应给两个存在的文件打标签: %#v", result)
	}
	if len(result.Missing) != 1 || result.Missing[0] != "missing.vpk" {
		t.Fatalf("缺失文件应被单独报告: %#v", result)
	}

	// 标签写进文件名：重新扫描后两个文件都应带上级标签。
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	// 注意：单个标签会落在"一级标签"位（[标签]名字.vpk），多个标签时第一个是一级。
	// 所以这里两级都要看，不能只看 SecondaryTags。
	tagged := map[string]bool{}
	a.vpkCache.Range(func(_ any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		if cache.File.PrimaryTag == "M16 武器" {
			tagged[cache.File.Path] = true
		}
		for _, tag := range cache.File.SecondaryTags {
			if tag == "M16 武器" {
				tagged[cache.File.Path] = true
			}
		}
		return true
	})
	if len(tagged) != 2 {
		t.Fatalf("两个文件都应带上新标签: %#v", tagged)
	}

	// 打标签会改文件名 → addonlist 键变化：成员键必须跟着改绑，否则会从组里掉出去。
	var pathA, pathB string
	for path := range tagged {
		base := strings.ToLower(filepath.Base(path))
		switch {
		case strings.HasSuffix(base, "a.vpk"):
			pathA = path
		case strings.HasSuffix(base, "b.vpk"):
			pathB = path
		}
	}
	if pathA == "" || pathB == "" {
		t.Fatalf("找不到改名后的两个文件: %#v", tagged)
	}
	group, err := a.CaptureModStrategyGroup("补标签组", "", modStrategyGroupSingle, []string{
		pathA,
		pathB,
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	again, err := a.ApplyTagToModKeys("第二个标签", []string{group.Members[0].Key, group.Members[1].Key})
	if err != nil {
		t.Fatalf("apply again: %v", err)
	}
	if len(again.Applied) != 2 || len(again.Missing) != 0 {
		t.Fatalf("应能给改名后的键继续打标签: %#v", again)
	}
	reloaded, err := a.findModStrategyGroup(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 打标签后文件名再次变化，但组成员键必须同步改绑（否则组会掉成员）。
	for _, member := range reloaded.Members {
		if strings.Contains(member.Key, "第二个标签") {
			continue
		}
		t.Fatalf("组成员键没有跟着标签改名改绑: %#v", reloaded.Members)
	}
	if len(reloaded.Members) != 2 {
		t.Fatalf("组成员应仍是 2 个: %#v", reloaded.Members)
	}
}

func TestApplyTagToModKeysRejectsEmptyTag(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if _, err := a.ApplyTagToModKeys("   ", []string{"a.vpk"}); err == nil {
		t.Fatal("空标签应被拒绝")
	}
	if _, err := a.ApplyTagToModKeys("标签", nil); err == nil {
		t.Fatal("没有目标 Mod 时应被拒绝")
	}
}
