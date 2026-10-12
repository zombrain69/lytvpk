// 命名规范化「规则配置」（路线图第 20 项 P3）的测试：
// 词表替换 / 例外表的增删、落盘与重载、校验与跳过明细、批量计划联动（exempt 跳过 +
// 别名参与建议）。全部在 t.TempDir 下构造，不触碰真实 Mod 库与真实配置。
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamingRulesAliasLifecycle(t *testing.T) {
	app, _ := newNamingBatchTestApp(t)

	// 初始为空。
	view, err := app.GetNamingRules()
	if err != nil {
		t.Fatalf("GetNamingRules: %v", err)
	}
	if len(view.SubjectAliases) != 0 || len(view.Exemptions) != 0 {
		t.Fatalf("初始应为空配置: %+v", view)
	}

	// 新增一条。
	result, err := app.AddNamingRulesAlias("木喷", "泵动霰弹枪")
	if err != nil {
		t.Fatalf("AddNamingRulesAlias: %v", err)
	}
	if result.Added != 1 || len(result.Rules.SubjectAliases) != 1 {
		t.Fatalf("新增结果不对: %+v", result)
	}
	if result.Rules.SubjectAliases[0].From != "木喷" || result.Rules.SubjectAliases[0].To != "泵动霰弹枪" {
		t.Fatalf("别名内容不对: %+v", result.Rules.SubjectAliases)
	}
	if _, err := os.Stat(filepath.Join(app.configDir, "naming_rules.json")); err != nil {
		t.Fatalf("规则文件应已落盘: %v", err)
	}

	// 同 From 覆盖。
	covered, err := app.AddNamingRulesAlias("木喷", "泵动式霰弹枪")
	if err != nil {
		t.Fatalf("覆盖别名: %v", err)
	}
	if len(covered.Rules.SubjectAliases) != 1 || covered.Rules.SubjectAliases[0].To != "泵动式霰弹枪" {
		t.Fatalf("覆盖应替换 To: %+v", covered.Rules.SubjectAliases)
	}

	// 新实例从磁盘重载（懒加载快照）。
	reloaded := &App{rootDir: app.rootDir, configDir: app.configDir}
	reloadedView, err := reloaded.GetNamingRules()
	if err != nil {
		t.Fatalf("重载 GetNamingRules: %v", err)
	}
	if len(reloadedView.SubjectAliases) != 1 || reloadedView.SubjectAliases[0].To != "泵动式霰弹枪" {
		t.Fatalf("重载别名不对: %+v", reloadedView.SubjectAliases)
	}

	// 删除。
	removed, err := app.RemoveNamingRulesAlias("木喷")
	if err != nil {
		t.Fatalf("RemoveNamingRulesAlias: %v", err)
	}
	if removed.Removed != 1 || len(removed.Rules.SubjectAliases) != 0 {
		t.Fatalf("删除结果不对: %+v", removed)
	}
	if _, err := app.RemoveNamingRulesAlias("不存在"); err == nil {
		t.Fatalf("删除不存在的别名应报错")
	}
}

func TestNamingRulesAliasValidation(t *testing.T) {
	app, _ := newNamingBatchTestApp(t)

	if _, err := app.AddNamingRulesAlias("", "X"); err == nil {
		t.Fatalf("空 from 应报错")
	}
	if _, err := app.AddNamingRulesAlias("木喷", "  "); err == nil {
		t.Fatalf("空 to 应报错")
	}
	if _, err := app.AddNamingRulesAlias("木喷", "泵动/霰弹枪"); err == nil {
		t.Fatalf("替换名含非法字符应报错")
	}
	if _, err := app.AddNamingRulesAlias("木喷", "木喷"); err == nil || !strings.Contains(err.Error(), "无需配置") {
		t.Fatalf("替换名与原词相同应报「无需配置」: %v", err)
	}
	if _, err := app.AddNamingRulesAlias("木喷", "泵动\n霰弹枪"); err == nil {
		t.Fatalf("含换行应报错")
	}
}

func TestNamingExemptionsLifecycle(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)
	packPath := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, packPath)
	otherPath := filepath.Join(addonsDir, "Map.vpk")
	writeMapPackVPK(t, otherPath)
	notVPK := filepath.Join(addonsDir, "notes.txt")
	if err := os.WriteFile(notVPK, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "Outside.vpk")
	writeCoachPackVPK(t, outside)

	result, err := app.AddNamingExemptions([]string{packPath, notVPK, outside, filepath.Join(addonsDir, "missing.vpk")})
	if err != nil {
		t.Fatalf("AddNamingExemptions: %v", err)
	}
	if result.Added != 1 || len(result.Skipped) != 3 {
		t.Fatalf("加入结果不对: added=%d skipped=%+v", result.Added, result.Skipped)
	}
	if len(result.Rules.Exemptions) != 1 {
		t.Fatalf("例外表应 1 条: %+v", result.Rules.Exemptions)
	}
	entry := result.Rules.Exemptions[0]
	if entry.Path != "Airi.vpk" || entry.Name != "Airi.vpk" || !entry.Exists {
		t.Fatalf("例外条目不对: %+v", entry)
	}

	// 重复加入 → 跳过。
	again, err := app.AddNamingExemptions([]string{packPath})
	if err != nil {
		t.Fatalf("重复加入: %v", err)
	}
	if again.Added != 0 || len(again.Skipped) != 1 || !strings.Contains(again.Skipped[0], "已在例外表") {
		t.Fatalf("重复加入应跳过: %+v", again)
	}

	// 子目录文件的存储键为斜杠相对路径。
	sub := filepath.Join(addonsDir, "sub", "Nested.vpk")
	if err := os.MkdirAll(filepath.Dir(sub), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCoachPackVPK(t, sub)
	if _, err := app.AddNamingExemptions([]string{sub}); err != nil {
		t.Fatalf("加入子目录文件: %v", err)
	}
	view, err := app.GetNamingRules()
	if err != nil {
		t.Fatalf("GetNamingRules: %v", err)
	}
	if len(view.Exemptions) != 2 || view.Exemptions[1].Path != "sub/Nested.vpk" {
		t.Fatalf("存储键应为斜杠相对路径: %+v", view.Exemptions)
	}

	// 按存储键移除 + 按绝对路径移除 + 不存在条目的跳过明细。
	removed, err := app.RemoveNamingExemptions([]string{"Airi.vpk"})
	if err != nil {
		t.Fatalf("按存储键移除: %v", err)
	}
	if removed.Removed != 1 {
		t.Fatalf("按存储键移除应为 1: %+v", removed)
	}
	byAbs, err := app.RemoveNamingExemptions([]string{sub})
	if err != nil {
		t.Fatalf("按绝对路径移除: %v", err)
	}
	if byAbs.Removed != 1 || len(byAbs.Rules.Exemptions) != 0 {
		t.Fatalf("按绝对路径移除应为 1 且清空: %+v", byAbs)
	}
	missing, err := app.RemoveNamingExemptions([]string{"Nope.vpk"})
	if err != nil {
		t.Fatalf("移除不存在条目: %v", err)
	}
	if missing.Removed != 0 || len(missing.Skipped) != 1 {
		t.Fatalf("移除不存在条目应回报跳过: %+v", missing)
	}
}

func TestNamingRulesCorruptFileToleratedAndRepaired(t *testing.T) {
	app, _ := newNamingBatchTestApp(t)
	path := filepath.Join(app.configDir, "naming_rules.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	view, err := app.GetNamingRules()
	if err != nil {
		t.Fatalf("损坏文件应容忍: %v", err)
	}
	if len(view.SubjectAliases) != 0 || len(view.Exemptions) != 0 {
		t.Fatalf("损坏文件应按空配置: %+v", view)
	}
	if _, err := app.AddNamingRulesAlias("木喷", "泵动霰弹枪"); err != nil {
		t.Fatalf("写回应修复损坏文件: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "泵动霰弹枪") {
		t.Fatalf("修复后的文件应包含新配置: %s", raw)
	}
}

func TestNamingBatchPlanHonorsExemptionsAndAliases(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)
	exemptPath := filepath.Join(addonsDir, "Exempt.vpk")
	writeCoachPackVPK(t, exemptPath)
	normalPath := filepath.Join(addonsDir, "Normal.vpk")
	writeCoachPackVPK(t, normalPath)

	// 例外表 + 词表替换各来一条。
	if _, err := app.AddNamingExemptions([]string{exemptPath}); err != nil {
		t.Fatalf("AddNamingExemptions: %v", err)
	}
	if _, err := app.AddNamingRulesAlias("Coach", "Coach 教练"); err != nil {
		t.Fatalf("AddNamingRulesAlias: %v", err)
	}

	plan, err := app.PlanNamingNormalization([]string{exemptPath, normalPath})
	if err != nil {
		t.Fatalf("PlanNamingNormalization: %v", err)
	}
	if plan.Exempted != 1 || plan.Skipped != 1 {
		t.Fatalf("计划应含 1 条豁免（计入跳过）: %+v", plan)
	}
	exemptItem := namingItemByPath(t, plan, exemptPath)
	if exemptItem.SkipReason != "exempt" || exemptItem.Actionable {
		t.Fatalf("豁免项应跳过: %+v", exemptItem)
	}
	normalItem := namingItemByPath(t, plan, normalPath)
	if !normalItem.Actionable {
		t.Fatalf("未豁免项应可执行: %+v", normalItem)
	}
	if normalItem.NewName != "生还者-Coach 教练-Normal.vpk" {
		t.Fatalf("词表替换应参与新名组装: %q", normalItem.NewName)
	}

	// 单文件建议入口同样吃词表替换。
	suggestion, err := app.SuggestNormalizedName(normalPath)
	if err != nil {
		t.Fatalf("SuggestNormalizedName: %v", err)
	}
	if suggestion.Stem != "生还者-Coach 教练-Normal" {
		t.Fatalf("单文件建议应套用别名: %+v", suggestion)
	}

	// 移出例外后重跑计划 → 不再跳过。
	if _, err := app.RemoveNamingExemptions([]string{exemptPath}); err != nil {
		t.Fatalf("RemoveNamingExemptions: %v", err)
	}
	replan, err := app.PlanNamingNormalization([]string{exemptPath, normalPath})
	if err != nil {
		t.Fatalf("重跑计划: %v", err)
	}
	if replan.Exempted != 0 {
		t.Fatalf("移出例外后不应再豁免: %+v", replan)
	}
	if item := namingItemByPath(t, replan, exemptPath); item.SkipReason == "exempt" {
		t.Fatalf("移出例外后应回到正常判定: %+v", item)
	}
}
