package rules

import (
	"testing"

	"vpk-manager/internal/gamedata"
)

func testIndex() *gamedata.StockIndex {
	return gamedata.NewFromPaths(nil, []string{
		"models/w_models/weapons/w_shotgun.mdl",
		"models/w_models/weapons/w_pumpshotgun_a.mdl",
		"models/weapons/melee/w_golfclub.mdl",
		"models/props_junk/gnome.mdl",
		"models/props_junk/explosive_box001.mdl",
		"materials/vgui/hud/x.vtf",
		"resource/ui/hud.res",
		"sound/player/hunter/voice/warn01.wav",
	})
}

func TestValidateFlagsDeadPrefixAndDeadKeywords(t *testing.T) {
	table := &Table{
		SchemaVersion: 1,
		FileKinds: []FileKind{
			{Tag: "UI", Prefixes: []string{"materials/vgui/", "materials/does-not-exist/"}},
			{Tag: "声音", Prefixes: []string{"sound/"}},
		},
		ContentRules: []ContentRule{
			{Tag: "侏儒", Prefixes: []string{"models/"}, Keywords: []string{"gnome", "not-a-keyword"}},
			// 复刻 D2：关键词 firework 在本体里根本不存在（真实模型叫 explosive_box001）。
			{Tag: "烟花盒", Prefixes: []string{"models/"}, Keywords: []string{"firework"}, Item: true},
		},
		WeaponPathRules: []MatchRule{
			{Keyword: "w_golfclub", Tag: "高尔夫球杆"},
			// 复刻 D8：规则写 golf_club，本体却是 w_golfclub。
			{Keyword: "golf_club", Tag: "高尔夫球杆"},
		},
	}

	report := Validate(table, testIndex(), nil)

	if report.OK {
		t.Fatalf("存在 0 命中前缀时必须判为不通过：%+v", report)
	}
	if len(report.DeadPrefixes) != 1 || report.DeadPrefixes[0].Pattern != "materials/does-not-exist/" {
		t.Fatalf("DeadPrefixes=%+v", report.DeadPrefixes)
	}

	deadByPattern := make(map[string]Resolution)
	for _, item := range report.DeadKeywords {
		deadByPattern[item.Scope+"/"+item.Pattern] = item
	}
	for _, want := range []string{"contentRule/not-a-keyword", "contentRule/firework", "weaponPathRule/golf_club"} {
		if _, ok := deadByPattern[want]; !ok {
			t.Fatalf("应报告死关键词 %q，实际 %v", want, report.DeadKeywords)
		}
	}
	if _, alive := deadByPattern["weaponPathRule/w_golfclub"]; alive {
		t.Fatalf("w_golfclub 在本体里存在，不应判死")
	}
	if _, alive := deadByPattern["contentRule/gnome"]; alive {
		t.Fatalf("gnome 在本体里存在，不应判死")
	}
	if report.DeadTotal != len(report.DeadPrefixes)+len(report.DeadKeywords) {
		t.Fatalf("DeadTotal=%d 与明细不符", report.DeadTotal)
	}
	if !report.WeaponScoped {
		t.Fatalf("武器关键词应按武器目录统计")
	}
	if report.IndexPaths != len(testIndex().Paths) {
		t.Fatalf("IndexPaths=%d", report.IndexPaths)
	}
}

func TestValidateWithoutIndexIsInconclusiveNotFailure(t *testing.T) {
	report := Validate(MustLoad(), nil, nil)
	if report.OK {
		t.Fatalf("没有索引时不能宣称通过")
	}
	if report.IndexPaths != 0 || report.DeadTotal != 0 {
		t.Fatalf("没有索引时不应产生「规则坏了」的明细：%+v", report)
	}
	if report.Notes == "" {
		t.Fatalf("应说明无法校验的原因")
	}
}

// TestValidateAcceptsRegisteredStockAbsentRule 锁定 knownStockAbsent 的语义：
// 登记过的 0 命中前缀不算错误，但要出现在 AcceptedDead 里，仍然可见。
func TestValidateAcceptsRegisteredStockAbsentRule(t *testing.T) {
	table := &Table{
		SchemaVersion: 1,
		ContentRules: []ContentRule{
			{Tag: "载入画面", Prefixes: []string{"materials/vgui/loadingscreen/"}},
		},
	}
	report := Validate(table, testIndex(), nil)
	if !report.OK {
		t.Fatalf("登记过的 0 命中前缀不应判失败：%+v", report)
	}
	if len(report.DeadPrefixes) != 0 {
		t.Fatalf("不应进入 DeadPrefixes：%+v", report.DeadPrefixes)
	}
	if len(report.AcceptedDead) != 1 || report.AcceptedDead[0].Pattern != "materials/vgui/loadingscreen/" {
		t.Fatalf("应进入 AcceptedDead：%+v", report.AcceptedDead)
	}
}

// TestKnownStockAbsentEntriesPointAtRealRules 防止白名单变成垃圾桶：
// 每条登记都必须真的对应规则表里的一条前缀，否则测试失败（规则改了、登记没删）。
func TestKnownStockAbsentEntriesPointAtRealRules(t *testing.T) {
	table := MustLoad()
	existing := make(map[string]struct{})
	for _, kind := range table.FileKinds {
		for _, prefix := range kind.Prefixes {
			existing[resolutionKey(Resolution{Scope: "fileKind", Tag: kind.Tag, Pattern: prefix})] = struct{}{}
		}
	}
	for _, rule := range table.ContentRules {
		for _, prefix := range rule.Prefixes {
			existing[resolutionKey(Resolution{Scope: "contentRule", Tag: rule.Tag, Pattern: prefix})] = struct{}{}
		}
	}
	for key, reason := range knownStockAbsent {
		if _, ok := existing[key]; !ok {
			t.Fatalf("knownStockAbsent 里的 %q 已不对应任何规则（规则表变了？）", key)
		}
		if len([]rune(reason)) < 10 {
			t.Fatalf("knownStockAbsent 的 %q 缺少可读原因", key)
		}
	}
}

func TestEmbeddedTableLoadsAndHasExpectedShape(t *testing.T) {
	table, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if table.SchemaVersion != SchemaVersion {
		t.Fatalf("schemaVersion=%d", table.SchemaVersion)
	}
	if len(table.FileKinds) == 0 || len(table.ContentRules) == 0 || len(table.WeaponPathRules) == 0 {
		t.Fatalf("规则表内容为空: %+v", table)
	}
	for _, kind := range table.FileKinds {
		if kind.Tag == "" || len(kind.Prefixes) == 0 {
			t.Fatalf("文件类别规则不完整: %+v", kind)
		}
	}
	for _, rule := range table.ContentRules {
		if rule.Tag == "" || len(rule.Prefixes) == 0 {
			t.Fatalf("内容规则不完整: %+v", rule)
		}
	}
	for _, rule := range table.WeaponPathRules {
		if rule.Tag == "" || rule.Keyword == "" {
			t.Fatalf("武器规则不完整: %+v", rule)
		}
	}
}
