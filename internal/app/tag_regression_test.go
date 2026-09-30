package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"vpk-manager/internal/grouping"
)

func baselineMod(entryID, name string, tags ...string) tagBaselineMod {
	return tagBaselineMod{
		EntryID:       entryID,
		Name:          name,
		SecondaryTags: normalizedTagSlice(tags),
	}
}

func baselineOf(mods ...tagBaselineMod) tagBaselineFile {
	return tagBaselineFile{Version: tagBaselineVersion, Mods: mods}
}

func TestDiffTagBaselineAllowsAddedTags(t *testing.T) {
	baseline := baselineOf(baselineMod("root/a.vpk", "a.vpk", "贴图", "模型"))
	current := []tagBaselineMod{baselineMod("root/a.vpk", "a.vpk", "贴图", "模型", "AK47")}

	report := diffTagBaseline(baseline, current, tagRegressionAllowlist{})
	if !report.OK {
		t.Fatalf("新增标签必须放行，report=%+v", report)
	}
	if report.AddedTags != 1 || report.RemovedTags != 0 {
		t.Fatalf("added=%d removed=%d，期望 1/0", report.AddedTags, report.RemovedTags)
	}
	if len(report.Changes) != 1 || !reflect.DeepEqual(report.Changes[0].Added, []string{"AK47"}) {
		t.Fatalf("changes=%+v", report.Changes)
	}
}

func TestDiffTagBaselineFailsOnRemovedTag(t *testing.T) {
	baseline := baselineOf(baselineMod("root/a.vpk", "a.vpk", "贴图", "铁喷"))
	current := []tagBaselineMod{baselineMod("root/a.vpk", "a.vpk", "贴图")}

	report := diffTagBaseline(baseline, current, tagRegressionAllowlist{})
	if report.OK {
		t.Fatalf("标签消失必须判为回归")
	}
	if report.RemovedTags != 1 || !reflect.DeepEqual(report.Changes[0].Unexcused, []string{"铁喷"}) {
		t.Fatalf("report=%+v", report)
	}
}

func TestDiffTagBaselineAllowlistExcusesRemoval(t *testing.T) {
	baseline := baselineOf(baselineMod("disabled/3300814140.vpk", "3300814140.vpk", "木喷", "贴图"))
	current := []tagBaselineMod{baselineMod("disabled/3300814140.vpk", "3300814140.vpk", "贴图", "铁喷")}
	allowlist := tagRegressionAllowlist{Entries: []tagRegressionAllowEntry{{
		EntryID:  "disabled/3300814140.vpk",
		Tag:      "木喷",
		Reason:   "w_pumpshotgun_A.mdl 是 Chrome 连喷的世界模型，木喷是错误标签",
		Evidence: "scripts/weapon_shotgun_chrome.txt → models/w_models/weapons/w_pumpshotgun_A.mdl",
	}}}

	report := diffTagBaseline(baseline, current, allowlist)
	if !report.OK {
		t.Fatalf("allowlist 内的消失必须放行，report=%+v", report)
	}
	if report.AllowlistedRemovals != 1 || report.RemovedTags != 0 {
		t.Fatalf("allowlisted=%d removed=%d，期望 1/0", report.AllowlistedRemovals, report.RemovedTags)
	}
	if report.AddedTags != 1 {
		t.Fatalf("新增的「铁喷」应被记录，added=%d", report.AddedTags)
	}
}

func TestDiffTagBaselineAllowlistMatchesNameCaseInsensitively(t *testing.T) {
	baseline := baselineOf(baselineMod("root/MOD.vpk", "MOD.VPK", "旧标签"))
	current := []tagBaselineMod{baselineMod("root/MOD.vpk", "MOD.VPK")}
	allowlist := tagRegressionAllowlist{Entries: []tagRegressionAllowEntry{{
		Name: "mod.vpk",
		Tag:  "旧标签",
	}}}
	if report := diffTagBaseline(baseline, current, allowlist); !report.OK {
		t.Fatalf("按名称匹配应忽略大小写，report=%+v", report)
	}
}

func TestDiffTagBaselineMissingAndNewModsAreNotRegressions(t *testing.T) {
	baseline := baselineOf(
		baselineMod("root/gone.vpk", "gone.vpk", "贴图"),
		baselineMod("root/keep.vpk", "keep.vpk", "模型"),
	)
	current := []tagBaselineMod{
		baselineMod("root/keep.vpk", "keep.vpk", "模型"),
		baselineMod("root/fresh.vpk", "fresh.vpk", "声音"),
	}

	report := diffTagBaseline(baseline, current, tagRegressionAllowlist{})
	if !report.OK {
		t.Fatalf("卸载/新增 Mod 不算少标，report=%+v", report)
	}
	if !reflect.DeepEqual(report.MissingMods, []string{"root/gone.vpk"}) {
		t.Fatalf("missing=%v", report.MissingMods)
	}
	if !reflect.DeepEqual(report.NewMods, []string{"root/fresh.vpk"}) {
		t.Fatalf("new=%v", report.NewMods)
	}
}

func TestBuildTagBaselineSortsAndDedupesTags(t *testing.T) {
	mods := []grouping.Mod{
		{Key: "b.vpk", Name: "b.vpk", PrimaryTag: "武器", SecondaryTags: []string{"贴图", " 贴图 ", "AK47", "ak47", ""}},
		{Key: "a.vpk", Name: "a.vpk", PrimaryTag: "人物", SecondaryTags: nil},
	}
	baseline := buildTagBaseline(mods, func(mod grouping.Mod) string { return "root/" + mod.Key })

	if len(baseline.Mods) != 2 || baseline.Mods[0].EntryID != "root/a.vpk" {
		t.Fatalf("基线必须按 entryId 排序：%+v", baseline.Mods)
	}
	if got := baseline.Mods[1].SecondaryTags; !reflect.DeepEqual(got, []string{"AK47", "贴图"}) {
		t.Fatalf("标签应去重且去空白，got=%v", got)
	}
	if baseline.Mods[0].SecondaryTags == nil {
		t.Fatalf("空标签必须序列化成 []，避免 JSON 里出现 null")
	}
	if baseline.Version != tagBaselineVersion || baseline.ModCount != 2 {
		t.Fatalf("baseline=%+v", baseline)
	}
}

// TestAllowlistGlobalScopeNeedsReasonAndEvidence 锁定整类放行的安全边界：
// 只有带原因 + 本体证据的 global 登记才生效，写坏了就 fail-closed（仍然报回归）。
func TestAllowlistGlobalScopeNeedsReasonAndEvidence(t *testing.T) {
	baseline := baselineOf(baselineMod("root/a.vpk", "a.vpk", "木喷", "铁喷"))
	current := []tagBaselineMod{baselineMod("root/a.vpk", "a.vpk", "铁喷")}

	incomplete := tagRegressionAllowlist{Entries: []tagRegressionAllowEntry{{
		Scope: "global", Tag: "木喷", Reason: "", Evidence: "",
	}}}
	if report := diffTagBaseline(baseline, current, incomplete); report.OK {
		t.Fatalf("缺少原因/证据的 global 登记必须无效：%+v", report)
	}

	complete := tagRegressionAllowlist{Entries: []tagRegressionAllowEntry{{
		Scope:    "global",
		Tag:      "木喷",
		Reason:   "w_pumpshotgun_A.mdl 是 Chrome 连喷的世界模型，木喷是错误标签",
		Evidence: "scripts/weapon_shotgun_chrome.txt → models/w_models/weapons/w_pumpshotgun_A.mdl",
	}}}
	report := diffTagBaseline(baseline, current, complete)
	if !report.OK {
		t.Fatalf("完整登记应放行：%+v", report)
	}
	if report.AllowlistedRemovals != 1 || report.RemovedTags != 0 {
		t.Fatalf("allowlisted=%d removed=%d", report.AllowlistedRemovals, report.RemovedTags)
	}
	// 新增的「铁喷」在两边都有，这里不产生 added；关键是"消失"被登记解释掉了。
	if len(report.Changes) != 1 || !reflect.DeepEqual(report.Changes[0].Allowlisted, []string{"木喷"}) {
		t.Fatalf("changes=%+v", report.Changes)
	}
}

// TestRepoAllowlistEntriesAreWellFormed 防止仓库里的登记写坏（缺原因/证据等于无效登记）。
func TestRepoAllowlistEntriesAreWellFormed(t *testing.T) {
	path := filepath.Join("..", "..", "tools", "tag-regression-allowlist.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	var allowlist tagRegressionAllowlist
	if err := json.Unmarshal(raw, &allowlist); err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	for i, entry := range allowlist.Entries {
		if strings.TrimSpace(entry.Tag) == "" || strings.TrimSpace(entry.Reason) == "" || strings.TrimSpace(entry.Evidence) == "" {
			t.Fatalf("第 %d 条登记缺少 tag/reason/evidence：%+v", i, entry)
		}
		isGlobal := strings.EqualFold(strings.TrimSpace(entry.Scope), "global")
		if !isGlobal && entry.EntryID == "" && entry.Name == "" {
			t.Fatalf("第 %d 条登记既不是 global，也没有 entryId/name：%+v", i, entry)
		}
	}
}
