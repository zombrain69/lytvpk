package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"vpk-manager/internal/parser"
)

func xdrTestSlot(character string, slot int) parser.XDRSlotInfo {
	return parser.XDRSlotInfo{
		Character: character,
		Model:     fmt.Sprintf("%s_slot_%03d", strings.ToLower(character), slot),
		Slot:      slot,
		SlotLabel: fmt.Sprintf("%03d", slot),
	}
}

func xdrTestFile(rootDir string, relPath string, title string, size int64, slots ...parser.XDRSlotInfo) VPKFile {
	location := "root"
	if strings.Contains(strings.ToLower(relPath), "workshop") {
		location = "workshop"
	}
	if strings.Contains(strings.ToLower(relPath), "disabled") {
		location = "disabled"
	}
	return VPKFile{
		Name:           filepath.Base(relPath),
		Path:           filepath.Join(rootDir, relPath),
		Size:           size,
		Title:          title,
		Location:       location,
		XDRSlots:       slots,
		XDRSummary:     "XDR",
		GameEnabled:    true,
		GameStateKnown: true,
	}
}

func healthKindsOf(report ModHealthReport) map[string]int {
	counts := make(map[string]int)
	for _, issue := range report.Issues {
		counts[issue.Kind]++
	}
	return counts
}

func TestCheckXDRHealthReportsSameSlotCollision(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	files := []VPKFile{
		xdrTestFile(root, "a.vpk", "Zoey 治疗动作 A", 100, xdrTestSlot("Zoey", 31)),
		xdrTestFile(root, "b.vpk", "Zoey 治疗动作 B", 200, xdrTestSlot("Zoey", 31)),
		xdrTestFile(root, "xdReanimsBase.vpk", "xdReanimsBase", 300),
	}

	report := ModHealthReport{}
	app.checkXDRHealth(files, &report)
	counts := healthKindsOf(report)
	if counts[modHealthKindXDRSlotCollision] != 1 {
		t.Fatalf("期望 1 条同槽冲突，实际 %#v", report.Issues)
	}
	if counts[modHealthKindXDRMissingBase] != 0 {
		t.Fatalf("有基础包时不该报缺基础包：%#v", report.Issues)
	}
	message := report.Issues[0].Message
	for _, want := range []string{"a.vpk", "b.vpk", "随机生效一个", "slot 031", "Zoey"} {
		if !strings.Contains(message, want) {
			t.Fatalf("冲突文案缺少 %q：%s", want, message)
		}
	}
	if report.Issues[0].Severity != "warning" {
		t.Fatalf("同槽冲突应是 warning：%#v", report.Issues[0])
	}
}

// 一个"8 角色通用"的动作包与另一个同样覆盖多角色的包撞车时，只报一行（列出受影响角色），
// 否则体检列表会被同一件事刷爆。
func TestCheckXDRHealthAggregatesMultiCharacterCollision(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	characters := []string{"Bill", "Coach", "Ellis", "Francis", "Louis", "Nick", "Rochelle", "Zoey"}
	slotsFor := func() []parser.XDRSlotInfo {
		result := make([]parser.XDRSlotInfo, 0, len(characters))
		for _, character := range characters {
			result = append(result, xdrTestSlot(character, 8))
		}
		return result
	}
	files := []VPKFile{
		xdrTestFile(root, "pack-a.vpk", "动作包 A", 1000, slotsFor()...),
		xdrTestFile(root, "pack-b.vpk", "动作包 B", 2000, slotsFor()...),
		xdrTestFile(root, "xdReanimsBase.vpk", "xdReanimsBase", 300),
	}

	report := ModHealthReport{}
	app.checkXDRHealth(files, &report)
	if counts := healthKindsOf(report); counts[modHealthKindXDRSlotCollision] != 1 {
		t.Fatalf("多角色同槽应聚合成 1 条：%#v", report.Issues)
	}
	message := report.Issues[0].Message
	if !strings.Contains(message, "8 个角色") && !strings.Contains(message, "等 8 个角色") {
		t.Fatalf("聚合文案应说明影响角色数：%s", message)
	}
	if !strings.Contains(message, "pack-a.vpk") || !strings.Contains(message, "pack-b.vpk") {
		t.Fatalf("聚合文案应列出冲突的 Mod：%s", message)
	}
}

// 同一个文件在 root 与 workshop 各一份（真实库常见）只算一次；disabled 里的不参与冲突。
func TestCheckXDRHealthSkipsDuplicateCopiesAndDisabled(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	files := []VPKFile{
		xdrTestFile(root, "same.vpk", "同一份", 4096, xdrTestSlot("Jockey", 43)),
		xdrTestFile(root, filepath.Join("workshop", "same.vpk"), "同一份", 4096, xdrTestSlot("Jockey", 43)),
		xdrTestFile(root, filepath.Join("disabled", "old.vpk"), "关掉的那份", 999, xdrTestSlot("Jockey", 43)),
		xdrTestFile(root, "xdReanimsBase.vpk", "xdReanimsBase", 300),
	}

	report := ModHealthReport{}
	app.checkXDRHealth(files, &report)
	if counts := healthKindsOf(report); counts[modHealthKindXDRSlotCollision] != 0 {
		t.Fatalf("副本与 disabled 不该算冲突：%#v", report.Issues)
	}
}

func TestCheckXDRHealthReportsMissingBase(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)

	report := ModHealthReport{}
	app.checkXDRHealth([]VPKFile{
		xdrTestFile(root, "a.vpk", "动作", 100, xdrTestSlot("Zoey", 32)),
	}, &report)
	if counts := healthKindsOf(report); counts[modHealthKindXDRMissingBase] != 1 {
		t.Fatalf("缺基础包要报 1 条：%#v", report.Issues)
	}

	// 基础包在游戏内被关掉：同样报缺基础包，但文案要点明"游戏内"。
	base := xdrTestFile(root, "xdReanimsBase.vpk", "xdReanimsBase", 300)
	base.GameEnabled = false
	report = ModHealthReport{}
	app.checkXDRHealth([]VPKFile{
		xdrTestFile(root, "a.vpk", "动作", 100, xdrTestSlot("Zoey", 32)),
		base,
	}, &report)
	counts := healthKindsOf(report)
	if counts[modHealthKindXDRMissingBase] != 1 {
		t.Fatalf("基础包被关掉也要报：%#v", report.Issues)
	}
	if !strings.Contains(report.Issues[0].Message, "游戏内") {
		t.Fatalf("文案应说明是游戏内开关的问题：%s", report.Issues[0].Message)
	}

	// 没有任何 XDR 动作 Mod 时，一条都不该报。
	report = ModHealthReport{}
	app.checkXDRHealth([]VPKFile{xdrTestFile(root, "plain.vpk", "普通 Mod", 100)}, &report)
	if report.TotalIssues != 0 {
		t.Fatalf("没有动作 Mod 时不该报 XDR 问题：%#v", report.Issues)
	}
}

func TestCheckXDRVariantPackDetection(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)

	packEntries := []string{
		"models/xdreanims/survivor_die.mdl",
		"models/xdreanims/die_default.mdl",
		"models/xdreanims/die_gangnam.mdl",
		"models/xdreanims/die_shuangrennv1.mdl",
		"models/xdreanims/die_shuangrennv2.mdl",
		"models/xdreanims/die_shuangrennv3.mdl",
	}
	report := ModHealthReport{}
	seen := map[string]bool{}
	app.checkXDRVariantPack("pack.vpk", filepath.Join(root, "pack.vpk"), "root", 4096, packEntries, seen, &report)
	if counts := healthKindsOf(report); counts[modHealthKindXDRVariantPack] != 1 {
		t.Fatalf("动作包要报 1 条：%#v", report.Issues)
	}
	if report.Issues[0].Severity != "info" {
		t.Fatalf("动作包应是 info：%#v", report.Issues[0])
	}

	// slot 型 Mod 不是动作包。
	report = ModHealthReport{}
	app.checkXDRVariantPack("slot.vpk", filepath.Join(root, "slot.vpk"), "root", 4096,
		append(packEntries, "models/xdreanims/zoey_slot_031.mdl"), seen, &report)
	if report.TotalIssues != 0 {
		t.Fatalf("slot 型 Mod 不该报动作包：%#v", report.Issues)
	}

	// 模型太少时不当动作包（避免误报普通 Mod）。
	report = ModHealthReport{}
	app.checkXDRVariantPack("small.vpk", filepath.Join(root, "small.vpk"), "root", 4096,
		[]string{"models/xdreanims/a.mdl", "models/xdreanims/b.mdl"}, seen, &report)
	if report.TotalIssues != 0 {
		t.Fatalf("模型过少不该报动作包：%#v", report.Issues)
	}

	// 同一个包在根目录与 workshop 各一份：只报一次。
	report = ModHealthReport{}
	dedupe := map[string]bool{}
	app.checkXDRVariantPack("pack.vpk", filepath.Join(root, "pack.vpk"), "root", 4096, packEntries, dedupe, &report)
	app.checkXDRVariantPack("pack.vpk", filepath.Join(root, "workshop", "pack.vpk"), "workshop", 4096, packEntries, dedupe, &report)
	if counts := healthKindsOf(report); counts[modHealthKindXDRVariantPack] != 1 {
		t.Fatalf("同一份动作包只该报一次：%#v", report.Issues)
	}
}
