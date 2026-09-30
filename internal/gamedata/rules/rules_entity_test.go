package rules

import (
	"testing"

	"vpk-manager/internal/gamedata/entities"
)

// TestValidateEntityAnchorsReportsMissingStockPaths 覆盖实体锚点校验：
// 锚点在本体里不存在 = 错误（该"精确证据"永远不可能命中）。
func TestValidateEntityAnchorsReportsMissingStockPaths(t *testing.T) {
	entityTable := entities.NewTable([]entities.Entity{
		{
			ID: "weapon_shotgun_chrome", Tag: "铁喷",
			Anchors: []string{"models/w_models/weapons/w_pumpshotgun_a.mdl"},
		},
		{
			ID: "melee.golfclub", Tag: "高尔夫球杆",
			Anchors: []string{"models/weapons/melee/w_golfclub.mdl", "models/weapons/melee/v_does_not_exist.mdl"},
		},
	})
	index := testIndex() // 夹具里已有 w_pumpshotgun_a 与 w_golfclub，但没有 v_does_not_exist

	report := Validate(&Table{SchemaVersion: 1}, index, entityTable)
	if report.OK {
		t.Fatalf("存在解析不到的锚点时必须判失败：%+v", report)
	}
	if len(report.DeadPrefixes) != 1 {
		t.Fatalf("应只报告 1 条死亡锚点：%+v", report.DeadPrefixes)
	}
	dead := report.DeadPrefixes[0]
	if dead.Scope != "entity" || dead.Pattern != "models/weapons/melee/v_does_not_exist.mdl" || dead.Tag != "高尔夫球杆" {
		t.Fatalf("死亡锚点内容异常：%+v", dead)
	}
	if report.Checked != 3 {
		t.Fatalf("应检查 3 条锚点，实际 %d", report.Checked)
	}
}

// TestValidateWithoutEntityTableSkipsAnchorChecks 锁定"传 nil 就不校验实体锚点"的语义，
// 让只关心规则表的调用方（与夹具测试）不会被实体表牵制。
func TestValidateWithoutEntityTableSkipsAnchorChecks(t *testing.T) {
	report := Validate(&Table{SchemaVersion: 1}, testIndex(), nil)
	if !report.OK {
		t.Fatalf("没有实体表时应只看规则表本身：%+v", report)
	}
	if report.Checked != 0 {
		t.Fatalf("空规则表应检查 0 条，实际 %d", report.Checked)
	}
}
