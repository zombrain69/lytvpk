package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 建议文件里的标签：整组统一标签（tag）+ 逐成员标签（memberTags）。
// 导入后必须能透出给界面，并且"给这组打标签"优先用导入的标签（比规则推导更准）。

func TestImportGroupSuggestionsCarriesTags(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Location: "root"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":      "SG552 武器",
		"reason":     "都替换 SG552",
		"confidence": "high",
		"members":    []string{"a.vpk", "b.vpk"},
		"tag":        "sg552",
		"tagReason":  "同一把枪的替换，打上 sg552 以后可以直接用标签筛选",
		"memberTags": map[string][]string{
			"a.vpk": {"sg552", "皮肤"},
		},
	}))
	if _, err := a.ImportGroupSuggestionsFromFile(source); err != nil {
		t.Fatalf("import: %v", err)
	}

	external, err := a.GetExternalGroupSuggestions()
	if err != nil {
		t.Fatalf("read external: %v", err)
	}
	if len(external) != 1 {
		t.Fatalf("应有 1 条导入建议: %#v", external)
	}
	if external[0].Tag != "sg552" {
		t.Fatalf("整组标签没有导入: %#v", external[0])
	}
	if external[0].TagReason == "" {
		t.Fatalf("标签理由没有导入: %#v", external[0])
	}
	if len(external[0].MemberTags["a.vpk"]) != 2 {
		t.Fatalf("逐成员标签没有导入: %#v", external[0].MemberTags)
	}

	// 「给这组打标签」要优先用导入的标签，并标明来源是导入文件。
	proposals, err := a.GetGroupTagSuggestions()
	if err != nil {
		t.Fatalf("tag suggestions: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("应有 1 条标签提案: %#v", proposals)
	}
	item := proposals[0]
	if item.Source != "suggestion" || item.Tag != "sg552" || item.TagOrigin != "imported" {
		t.Fatalf("应优先采用导入标签: %#v", item)
	}
	if item.MemberCount != 2 {
		t.Fatalf("成员数不对: %#v", item)
	}
	if len(item.MemberTags["a.vpk"]) != 2 {
		t.Fatalf("逐成员标签应透出: %#v", item.MemberTags)
	}
}

func TestGroupTagSuggestionsFallBackWhenImportHasNoTag(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{
		Name: "a.vpk", Location: "root", SubjectSummary: "sg552 武器",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{
		Name: "b.vpk", Location: "root", SubjectSummary: "sg552 武器",
	})

	// 智能体没有给标签：降级用规则（组名 → 成员共同主体识别）。
	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":      "SG 套装：一整套改名用的特别特别特别特别特别长的组名示例（不该被当成标签）",
		"confidence": "high",
		"members":    []string{"a.vpk", "b.vpk"},
	}))
	if _, err := a.ImportGroupSuggestionsFromFile(source); err != nil {
		t.Fatalf("import: %v", err)
	}

	proposals, err := a.GetGroupTagSuggestions()
	if err != nil {
		t.Fatalf("tag suggestions: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("应有 1 条标签提案: %#v", proposals)
	}
	if proposals[0].TagOrigin != "subject" || proposals[0].Tag != "sg552 武器" {
		t.Fatalf("没有导入标签时应降级到主体识别: %#v", proposals[0])
	}
}

func TestValidateGroupSuggestionsReportsTagCoverage(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Location: "root"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(
		map[string]any{
			"label":   "带标签的组",
			"members": []string{"a.vpk", "b.vpk"},
			"tag":     "sg552",
		},
		map[string]any{
			"label":   "标签非法的组",
			"members": []string{"a.vpk", "b.vpk"},
			"tag":     "含+号且特别特别特别特别特别特别特别特别长的标签名称会破坏文件名",
		},
	))

	validation, err := a.ValidateGroupSuggestionsFile(source)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	// WithTags 只统计**合法**标签（非法的那条会在 problems 里报出来）。
	if validation.WithTags != 1 {
		t.Fatalf("应统计出 1 条带合法标签的建议: %#v", validation)
	}
	if validation.Valid != 1 || validation.Invalid != 1 {
		t.Fatalf("非法标签应让该条失效: %#v", validation)
	}
	var tagged *GroupSuggestionValidationItem
	for index := range validation.Items {
		if validation.Items[index].Label == "标签非法的组" {
			tagged = &validation.Items[index]
		}
	}
	if tagged == nil {
		t.Fatalf("缺少「标签非法的组」: %#v", validation.Items)
	}
	joined := strings.Join(append([]string(nil), tagged.Problems...), " | ")
	if !strings.Contains(joined, "tag") {
		t.Fatalf("非法标签应给出 tag 相关提示: %#v", tagged.Problems)
	}
}

func TestValidateGroupSuggestionsAcceptsOldFileWithoutTags(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Location: "root"})
	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":   "没有标签的旧文件",
		"members": []string{"a.vpk", "b.vpk"},
	}))
	validation, err := a.ValidateGroupSuggestionsFile(source)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if validation.WithTags != 0 || validation.Valid != 1 {
		t.Fatalf("旧格式（没有 tag 字段）必须继续有效: %#v", validation)
	}
}

func TestGroupingCatalogExportsExistingTags(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{
		Name: "a.vpk", Location: "root", PrimaryTag: "武器", SecondaryTags: []string{"sg552"},
	})
	target := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := a.ExportGroupingCatalog(target); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Mods []struct {
			PrimaryTag    string   `json:"primaryTag"`
			SecondaryTags []string `json:"secondaryTags"`
		} `json:"mods"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Mods) != 1 || parsed.Mods[0].PrimaryTag != "武器" ||
		len(parsed.Mods[0].SecondaryTags) != 1 || parsed.Mods[0].SecondaryTags[0] != "sg552" {
		t.Fatalf("清单必须带上现有标签（智能体据此避免重复打标签）: %#v", parsed.Mods)
	}
}
