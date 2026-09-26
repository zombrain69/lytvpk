package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// newPriorityTestApp 建立一份真实的最小游戏目录（含未记录条目）：
// left4dead2/addons/{a.vpk,b.vpk,c.vpk,workshop/123.vpk} + addonlist.txt。
// 其中 c.vpk 不在 addonlist.txt 中，用来覆盖"未记录参与者 → 冲突"的分支。
func newPriorityTestApp(t *testing.T) (*App, string) {
	t.Helper()

	base := t.TempDir()
	gameDir := filepath.Join(base, "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: `workshop\123.vpk`, Value: "1"},
		{Name: "b.vpk", Value: "1"},
	})
	writeTestVPK(t, filepath.Join(addonsDir, "a.vpk"), map[string][]byte{
		"materials/shared.vtf":  []byte("a"),
		"materials/decided.vtf": []byte("a"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "b.vpk"), map[string][]byte{
		"materials/shared.vtf": []byte("b"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "c.vpk"), map[string][]byte{
		"materials/shared.vtf": []byte("c"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "workshop", "123.vpk"), map[string][]byte{
		"materials/shared.vtf":  []byte("w"),
		"materials/decided.vtf": []byte("w"),
	})

	return &App{rootDir: addonsDir, configDir: filepath.Join(base, "config")}, addonsDir
}

// conflictGoldenVPK 只保留与优先级判定有关、且与临时目录无关的字段，
// 这样黄金快照可以在任意机器上逐字节复现。
type conflictGoldenVPK struct {
	Name     string `json:"name"`
	Location string `json:"location"`
	Order    int    `json:"order"`
}

type conflictGoldenGroup struct {
	VpkFiles       []conflictGoldenVPK `json:"vpkFiles"`
	Files          []string            `json:"files"`
	FileCount      int                 `json:"fileCount"`
	FilesTruncated bool                `json:"filesTruncated"`
	Severity       string              `json:"severity"`
	Winner         *conflictGoldenVPK  `json:"winner,omitempty"`
}

type conflictGoldenResult struct {
	TotalConflicts int                   `json:"totalConflicts"`
	TotalOverrides int                   `json:"totalOverrides"`
	ConflictGroups []conflictGoldenGroup `json:"conflictGroups"`
	OverrideGroups []conflictGoldenGroup `json:"overrideGroups"`
}

func conflictGoldenVPKs(files []ConflictVPKFile) []conflictGoldenVPK {
	result := make([]conflictGoldenVPK, 0, len(files))
	for _, file := range files {
		result = append(result, conflictGoldenVPK{
			Name:     file.Name,
			Location: file.Location,
			Order:    file.Order,
		})
	}
	return result
}

func conflictGoldenGroupFrom(group ConflictGroup) conflictGoldenGroup {
	return conflictGoldenGroup{
		VpkFiles:       conflictGoldenVPKs(group.VpkFiles),
		Files:          group.Files,
		FileCount:      group.FileCount,
		FilesTruncated: group.FilesTruncated,
		Severity:       group.Severity,
	}
}

func conflictGoldenSnapshot(result *ConflictResult) conflictGoldenResult {
	snapshot := conflictGoldenResult{
		TotalConflicts: result.TotalConflicts,
		TotalOverrides: result.TotalOverrides,
		ConflictGroups: make([]conflictGoldenGroup, 0, len(result.ConflictGroups)),
		OverrideGroups: make([]conflictGoldenGroup, 0, len(result.OverrideGroups)),
	}
	for _, group := range result.ConflictGroups {
		snapshot.ConflictGroups = append(snapshot.ConflictGroups, conflictGoldenGroupFrom(group))
	}
	for _, group := range result.OverrideGroups {
		converted := conflictGoldenGroupFrom(ConflictGroup{
			VpkFiles:       group.VpkFiles,
			Files:          group.Files,
			FileCount:      group.FileCount,
			FilesTruncated: group.FilesTruncated,
			Severity:       group.Severity,
		})
		winner := conflictGoldenVPKs([]ConflictVPKFile{group.Winner})[0]
		converted.Winner = &winner
		snapshot.OverrideGroups = append(snapshot.OverrideGroups, converted)
	}
	sort.SliceStable(snapshot.ConflictGroups, func(i, j int) bool {
		return snapshot.ConflictGroups[i].Files[0] < snapshot.ConflictGroups[j].Files[0]
	})
	sort.SliceStable(snapshot.OverrideGroups, func(i, j int) bool {
		return snapshot.OverrideGroups[i].Files[0] < snapshot.OverrideGroups[j].Files[0]
	})
	return snapshot
}

// TestConflictPriorityGoldenRegressionWithoutLayers 是本设计的硬约束：
// priority.json 不存在、所有策略组 Tier 未设置时，优先级感知分析的结果
// 必须与"引入分层之前"的实现逐字节一致。
func TestConflictPriorityGoldenRegressionWithoutLayers(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{
		FullScan:      true,
		PriorityAware: true,
	})
	if err != nil {
		t.Fatalf("check conflicts: %v", err)
	}

	snapshot := conflictGoldenSnapshot(result)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	encoded = append(encoded, '\n')

	goldenPath := filepath.Join("testdata", "conflict-priority-golden.json")
	if os.Getenv("UPDATE_PRIORITY_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, encoded, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (先运行 UPDATE_PRIORITY_GOLDEN=1 go test ./internal/app -run TestConflictPriorityGoldenRegressionWithoutLayers 生成基线): %v", err)
	}
	// 仓库没有 .gitattributes，Windows 检出（core.autocrlf=true）会把 testdata 转成 CRLF，
	// 因此比较前统一换行，避免"本地通过、CI 失败"这种与内容无关的差异。
	if normalizeGoldenNewlines(string(want)) != normalizeGoldenNewlines(string(encoded)) {
		t.Fatalf("未分层时的冲突结果与黄金基线不一致:\n--- want ---\n%s\n--- got ---\n%s", want, encoded)
	}
}

// normalizeGoldenNewlines 把 CRLF 归一化为 LF，供黄金快照比较使用。
func normalizeGoldenNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func intPointer(value int) *int {
	return &value
}

func TestComputeEffectivePriorityFallsBackToOrderAndTakesGroupMinimum(t *testing.T) {
	if got := computeEffectivePriority(7, nil, nil); got != 7 {
		t.Fatalf("未设置分层时有效分层应等于顺序号, got %d", got)
	}
	if got := computeEffectivePriority(7, intPointer(3), nil); got != 3 {
		t.Fatalf("显式分层应覆盖顺序号, got %d", got)
	}
	if got := computeEffectivePriority(7, nil, intPointer(2)); got != 2 {
		t.Fatalf("组权重应抬高优先级, got %d", got)
	}
	if got := computeEffectivePriority(7, intPointer(5), intPointer(9)); got != 5 {
		t.Fatalf("组权重更小时应取组权重, got %d", got)
	}
	if got := computeEffectivePriority(7, intPointer(2), intPointer(9)); got != 2 {
		t.Fatalf("组权重更大时不应降低优先级, got %d", got)
	}
	if got := computeEffectivePriority(-1, nil, nil); got != -1 {
		t.Fatalf("未记录条目的有效分层应保持 -1, got %d", got)
	}
	if got := effectivePrioritySource(7, nil, nil); got != prioritySourceOrder {
		t.Fatalf("source = %q, want %q", got, prioritySourceOrder)
	}
	if got := effectivePrioritySource(7, intPointer(3), intPointer(1)); got != prioritySourceGroup {
		t.Fatalf("source = %q, want %q", got, prioritySourceGroup)
	}
}

func readAddonListBytes(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// TestModPriorityWritesOnlyPriorityStore 守住"只有显式应用分层才允许重排"：
// 设置/清除分层不得触碰 addonlist.txt 一个字节。
func TestModPriorityWritesOnlyPriorityStore(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	addonListPath := filepath.Join(filepath.Dir(a.rootDir), "addonlist.txt")
	before := readAddonListBytes(t, addonListPath)

	if _, err := a.SetModPriority("b.vpk", "b.vpk", -4); err != nil {
		t.Fatalf("set priority: %v", err)
	}
	if got := readAddonListBytes(t, addonListPath); string(got) != string(before) {
		t.Fatalf("设置分层改动了 addonlist.txt:\n%s", got)
	}

	entries, err := a.ListModPriorities()
	if err != nil {
		t.Fatalf("list priorities: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "b.vpk" || entries[0].Tier != -4 {
		t.Fatalf("持久化的分层记录 = %#v", entries)
	}

	// 同一 Mod 重复设置：后者覆盖前者。
	if _, err := a.SetModPriority("b.vpk", "b.vpk", 12); err != nil {
		t.Fatalf("overwrite priority: %v", err)
	}
	entries, err = a.ListModPriorities()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Tier != 12 {
		t.Fatalf("重复设置分层后 = %#v", entries)
	}

	plan, err := a.GetModPriorityPlan()
	if err != nil {
		t.Fatalf("priority plan: %v", err)
	}
	var bEntry *ModEffectivePriority
	for index := range plan {
		if plan[index].Key == "b.vpk" {
			bEntry = &plan[index]
		}
	}
	if bEntry == nil {
		t.Fatalf("plan 缺少 b.vpk: %#v", plan)
	}
	if bEntry.Order != 3 || bEntry.Effective != 12 || bEntry.Source != prioritySourceTier {
		t.Fatalf("b.vpk 的有效分层 = %#v", *bEntry)
	}

	if err := a.ClearModPriority("b.vpk"); err != nil {
		t.Fatalf("clear priority: %v", err)
	}
	plan, err = a.GetModPriorityPlan()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range plan {
		if entry.Key == "b.vpk" && (entry.Tier != nil || entry.Effective != 2 || entry.Source != prioritySourceOrder) {
			t.Fatalf("清除分层后应回到顺序号: %#v", entry)
		}
	}
	if got := readAddonListBytes(t, addonListPath); string(got) != string(before) {
		t.Fatalf("清除分层改动了 addonlist.txt:\n%s", got)
	}
}

// TestApplyModPriorityLayersSortsStablyAndSkipsNoopWrites 覆盖显式"应用分层"。
func TestApplyModPriorityLayersSortsStablyAndSkipsNoopWrites(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	addonListPath := filepath.Join(filepath.Dir(a.rootDir), "addonlist.txt")
	before := readAddonListBytes(t, addonListPath)

	// 没有任何分层：明确的重排入口也不得写盘（未分层行为逐字节一致）。
	if _, err := a.ApplyModPriorityLayers(); err != nil {
		t.Fatalf("apply without layers: %v", err)
	}
	if got := readAddonListBytes(t, addonListPath); string(got) != string(before) {
		t.Fatalf("未设置分层时应用分层改动了 addonlist.txt:\n%s", got)
	}

	if _, err := a.SetModPriority("a.vpk", "a.vpk", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModPriority(`workshop\123.vpk`, `workshop\123.vpk`, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModPriority("b.vpk", "b.vpk", 1); err != nil {
		t.Fatal(err)
	}
	preview, err := a.ApplyModPriorityLayers()
	if err != nil {
		t.Fatalf("apply layers: %v", err)
	}
	// 三者同层：保持既有相对顺序（a, workshop\123, b）。
	if want := []string{"a.vpk", `workshop\123.vpk`, "b.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("同层稳定排序 = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}

	// b.vpk 抬高到最前，其余保持相对顺序。
	if _, err := a.SetModPriority("b.vpk", "b.vpk", -1); err != nil {
		t.Fatal(err)
	}
	preview, err = a.ApplyModPriorityLayers()
	if err != nil {
		t.Fatalf("apply layers again: %v", err)
	}
	if want := []string{"b.vpk", "a.vpk", `workshop\123.vpk`}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("按分层排序 = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}

	// 写盘后仍然能被现有解析器读回，且开关状态没有被改写。
	parsed := parseAddonListItems(string(readAddonListBytes(t, addonListPath)))
	if len(parsed) != 3 || parsed[0].Name != "b.vpk" || parsed[0].Value != "1" || parsed[1].Value != "1" {
		t.Fatalf("写回后的 addonlist 内容 = %#v", parsed)
	}
}

func TestApplyModPriorityLayersPreservesGBKEncoding(t *testing.T) {
	content := "\"AddonList\"\r\n{\r\n\t\"根目录测试.vpk\"\t\"1\"\r\n\t\"workshop\\123.vpk\"\t\"1\"\r\n}\r\n"
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := newPriorityTestApp(t)
	addonListPath := filepath.Join(filepath.Dir(a.rootDir), "addonlist.txt")
	if err := os.WriteFile(addonListPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModPriority(`workshop\123.vpk`, `workshop\123.vpk`, -3); err != nil {
		t.Fatal(err)
	}

	preview, err := a.ApplyModPriorityLayers()
	if err != nil {
		t.Fatalf("apply GBK layers: %v", err)
	}
	if want := []string{`workshop\123.vpk`, "根目录测试.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("ordered keys = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}

	updated := readAddonListBytes(t, addonListPath)
	if utf8.Valid(updated) {
		t.Fatal("按分层重排把 GBK addonlist.txt 写成了 UTF-8")
	}
	decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), updated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decoded), "根目录测试.vpk") {
		t.Fatalf("GBK 内容丢失: %q", decoded)
	}
}

// TestConflictUsesEffectiveLayerForSameLayerConflictAndOverride 是本设计第 4 节的核心语义。
func TestConflictUsesEffectiveLayerForSameLayerConflictAndOverride(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// 移除未记录条目，聚焦"分层相同 = 真冲突 / 分层不同 = 覆盖"。
	if err := os.Remove(filepath.Join(addonsDir, "c.vpk")); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"a.vpk", "b.vpk"} {
		if _, err := a.SetModPriority(key, key, 5); err != nil {
			t.Fatal(err)
		}
	}

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatalf("check conflicts: %v", err)
	}
	if len(result.ConflictGroups) != 1 {
		t.Fatalf("expected one same-layer conflict, got %d groups: %#v", len(result.ConflictGroups), result.ConflictGroups)
	}
	group := result.ConflictGroups[0]
	if group.Layer == nil || *group.Layer != 5 {
		t.Fatalf("同层冲突应报告共用分层 5: %#v", group.Layer)
	}
	if len(group.VpkFiles) != 3 {
		t.Fatalf("expected 3 participants, got %#v", group.VpkFiles)
	}
	for _, file := range group.VpkFiles {
		switch file.Name {
		case "a.vpk":
			if file.Layer != 5 || file.Tier == nil || *file.Tier != 5 || file.Order != 0 {
				t.Fatalf("a.vpk 分层信息 = %#v", file)
			}
		case "b.vpk":
			if file.Layer != 5 || file.Order != 2 {
				t.Fatalf("b.vpk 分层信息 = %#v", file)
			}
		case "123.vpk":
			if file.Layer != 1 || file.Tier != nil {
				t.Fatalf("未设置分层的工坊条目应回退到顺序号: %#v", file)
			}
		default:
			t.Fatalf("unexpected participant %#v", file)
		}
	}

	// decided.vtf: a.vpk(5) vs workshop\123.vpk(1) → 覆盖，胜者为有效分层更大的 a.vpk。
	if len(result.OverrideGroups) != 1 {
		t.Fatalf("expected one override group, got %#v", result.OverrideGroups)
	}
	override := result.OverrideGroups[0]
	if override.Winner.Name != "a.vpk" || override.Winner.Layer != 5 || override.Winner.Order != 0 {
		t.Fatalf("覆盖胜者 = %#v", override.Winner)
	}
}

// TestGroupTierRaisesMemberPriority 覆盖"组权重叠加"与"重叠组取 min"。
func TestGroupTierRaisesMemberPriority(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	bPath := filepath.Join(addonsDir, "b.vpk")
	group, err := a.CaptureModStrategyGroup("轻量包", "", modStrategyGroupAll, []string{bPath})
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if group.Tier != nil {
		t.Fatalf("新建策略组默认不应带权重: %#v", group.Tier)
	}
	if _, err := a.SetModStrategyGroupTier(group.ID, intPointer(0)); err != nil {
		t.Fatalf("set group tier: %v", err)
	}

	second, err := a.CaptureModStrategyGroup("更靠前", "", modStrategyGroupAll, []string{bPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModStrategyGroupTier(second.ID, intPointer(-6)); err != nil {
		t.Fatal(err)
	}

	plan, err := a.GetModPriorityPlan()
	if err != nil {
		t.Fatalf("priority plan: %v", err)
	}
	for _, entry := range plan {
		if entry.Key != "b.vpk" {
			continue
		}
		if entry.GroupTier == nil || *entry.GroupTier != -6 {
			t.Fatalf("重叠策略组权重应取最小值 -6: %#v", entry.GroupTier)
		}
		if entry.Effective != -6 || entry.Source != prioritySourceGroup {
			t.Fatalf("组权重应抬高优先级: %#v", entry)
		}
	}

	// 清除组权重后回到顺序号。
	if _, err := a.SetModStrategyGroupTier(second.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModStrategyGroupTier(group.ID, nil); err != nil {
		t.Fatal(err)
	}
	plan, err = a.GetModPriorityPlan()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range plan {
		if entry.Key == "b.vpk" && (entry.GroupTier != nil || entry.Effective != 2 || entry.Source != prioritySourceOrder) {
			t.Fatalf("清除组权重后应回到顺序号: %#v", entry)
		}
	}
}

// TestProfileSnapshotCarriesPriorities 覆盖方案快照携带分层并在应用后恢复。
// TestGroupTierAccumulatesAncestorTiers 覆盖"更接近 FireAxe 的层级累加"：
//   - 子组会继承全部上级分组的权重（父 -2、子未设 → 子组成员拿到 -2）；
//   - 父子都设权重时**相加**（父 -2 + 子 -3 → -5）；
//   - 跨链仍然取 min（可重叠集合的唯一化规则）。
func TestGroupTierAccumulatesAncestorTiers(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	bPath := filepath.Join(addonsDir, "b.vpk")

	parent, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{bPath})
	if err != nil {
		t.Fatalf("capture parent: %v", err)
	}
	child, err := a.CaptureModStrategyGroup("子组", "", modStrategyGroupAll, []string{bPath})
	if err != nil {
		t.Fatalf("capture child: %v", err)
	}
	if _, err := a.MoveModStrategyGroup(child.ID, parent.ID); err != nil {
		t.Fatalf("move child under parent: %v", err)
	}
	if _, err := a.SetModStrategyGroupTier(parent.ID, intPointer(-2)); err != nil {
		t.Fatal(err)
	}

	// ① 只有父组有权重：子组成员直接继承（以前这里是 nil → 拿顺序号）。
	plan, err := a.GetModPriorityPlan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if entry := findPlanEntry(t, plan, "b.vpk"); entry.GroupTier == nil || *entry.GroupTier != -2 {
		t.Fatalf("父组权重应被下级继承: %#v", entry)
	}

	// ② 子组再设权重：沿链累加 → -5。
	if _, err := a.SetModStrategyGroupTier(child.ID, intPointer(-3)); err != nil {
		t.Fatal(err)
	}
	plan, err = a.GetModPriorityPlan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if entry := findPlanEntry(t, plan, "b.vpk"); entry.GroupTier == nil || *entry.GroupTier != -5 {
		t.Fatalf("父子权重应沿链累加为 -5: %#v", entry)
	} else if entry.Effective != -5 || entry.Source != prioritySourceGroup {
		t.Fatalf("累加后的组权重应成为有效分层: %#v", entry)
	}

	// ③ 清除整条链上的权重 → 回到顺序号（未设分层时行为不变）。
	if _, err := a.SetModStrategyGroupTier(child.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModStrategyGroupTier(parent.ID, nil); err != nil {
		t.Fatal(err)
	}
	plan, err = a.GetModPriorityPlan()
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if entry := findPlanEntry(t, plan, "b.vpk"); entry.GroupTier != nil || entry.Source != prioritySourceOrder {
		t.Fatalf("清空权重后应回到顺序号: %#v", entry)
	}
}

// findPlanEntry 取某个 key 的有效分层明细。
func findPlanEntry(t *testing.T, plan []ModEffectivePriority, key string) ModEffectivePriority {
	t.Helper()
	for _, entry := range plan {
		if entry.Key == key {
			return entry
		}
	}
	t.Fatalf("优先级明细里找不到 %s: %#v", key, plan)
	return ModEffectivePriority{}
}

func TestProfileSnapshotCarriesPriorities(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if _, err := a.SetModPriority("a.vpk", "a.vpk", -2); err != nil {
		t.Fatal(err)
	}

	profile, err := a.CaptureModEnableProfileWithAutomation("带分层", "", true)
	if err != nil {
		t.Fatalf("capture profile: %v", err)
	}
	if len(profile.Priorities) != 1 || profile.Priorities[0].Key != "a.vpk" || profile.Priorities[0].Tier != -2 {
		t.Fatalf("方案未携带分层: %#v", profile.Priorities)
	}

	if err := a.ClearModPriority("a.vpk"); err != nil {
		t.Fatal(err)
	}
	result, err := a.ApplyModEnableProfile(profile.ID)
	if err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if result.RestoredPriorities != 1 {
		t.Fatalf("恢复分层计数 = %d, want 1", result.RestoredPriorities)
	}
	entries, err := a.ListModPriorities()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Key != "a.vpk" || entries[0].Tier != -2 {
		t.Fatalf("恢复后的分层 = %#v", entries)
	}

	// 导入他人方案时按既有校验规则过滤非法条目。
	imported, err := normalizeModEnableProfile(ModEnableProfile{
		Name:               "导入",
		Entries:            []ModEnableProfileEntry{{Name: "a.vpk", Enabled: true}},
		IncludesAutomation: true,
		Priorities: []ModPriorityEntry{
			{Key: "", Tier: 1},
			{Key: "A.VPK", Name: "a.vpk", Tier: 3},
			{Key: "a.vpk", Tier: 9},
			{Key: "b.vpk", Tier: 4},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Priorities) != 2 {
		t.Fatalf("导入后应去重并丢弃空键: %#v", imported.Priorities)
	}
	if imported.Priorities[0].Key != "a.vpk" || imported.Priorities[0].Tier != 3 {
		t.Fatalf("导入后应保留首次出现的分层: %#v", imported.Priorities)
	}
}
