package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vpk-manager/internal/parser"
)

func newCustomRulesTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	return &App{configDir: dir}
}

func TestParseCustomTagRulesValidation(t *testing.T) {
	rules, issues, err := parseCustomTagRules(`{"version":1,"rules":[
		{"tag":"三角洲","nameContains":["三角洲","delta"]},
		{"tag":"","nameContains":["x"]},
		{"tag":"坏正则","regex":"([unclosed"},
		{"tag":"没条件"},
		{"tag":"停用的","nameContains":["x"],"enabled":false}
	]}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(rules) != 1 || rules[0].Tag != "三角洲" {
		t.Fatalf("只有第一条是合法规则，实际 %#v", rules)
	}
	if len(issues) != 3 {
		t.Fatalf("应报出 3 条问题（缺 tag / 正则非法 / 无条件），实际 %#v", issues)
	}

	// 也接受"直接一个数组"的写法，并拒绝版本不符。
	if bare, _, err := parseCustomTagRules(`[{"tag":"x","nameContains":["y"]}]`); err != nil || len(bare) != 1 {
		t.Fatalf("数组写法应被接受: %#v err=%v", bare, err)
	}
	if _, _, err := parseCustomTagRules(`{"version":99,"rules":[]}`); err == nil {
		t.Fatal("版本不符必须报错")
	}
	if _, _, err := parseCustomTagRules(`{not json`); err == nil {
		t.Fatal("非法 JSON 必须报错")
	}
}

func TestCustomTagRuleMatches(t *testing.T) {
	file := parser.VPKFile{
		Name:          "三角洲-重塑-AK47.vpk",
		Title:         "Delta Reshape",
		SecondaryTags: []string{"AK47", "贴图"},
	}
	if !customTagRuleMatches(CustomTagRule{Tag: "t", NameContains: []string{"三角洲"}}, file) {
		t.Fatal("文件名关键词应命中")
	}
	if !customTagRuleMatches(CustomTagRule{Tag: "t", TitleContains: []string{"delta"}}, file) {
		t.Fatal("标题关键词应不区分大小写命中")
	}
	if !customTagRuleMatches(CustomTagRule{Tag: "t", AnyTags: []string{"ak47"}}, file) {
		t.Fatal("已有标签应不区分大小写命中")
	}
	if !customTagRuleMatches(CustomTagRule{Tag: "t", Regex: `重塑-\w+47`}, file) {
		t.Fatal("正则应命中")
	}
	if customTagRuleMatches(CustomTagRule{Tag: "t", NameContains: []string{"不存在"}}, file) {
		t.Fatal("不匹配的关键词不该命中")
	}
}

// 保存 + 重新载入：规则文件落盘，重启后仍然生效；扫描时只增不减地补标签。
func TestSaveCustomTagRulesAppliesAdditively(t *testing.T) {
	app := newCustomRulesTestApp(t)
	count, err := app.SaveCustomTagRules(`{"version":1,"rules":[{"tag":"三角洲","nameContains":["三角洲"]},{"tag":"贴图包","anyTags":["贴图"]}]}`)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("规则数 = %d，期望 2", count)
	}
	if _, err := os.Stat(filepath.Join(app.configDir, "custom_tag_rules.json")); err != nil {
		t.Fatalf("规则文件应落盘: %v", err)
	}

	// 模拟重启：新 App 从同一目录载入。
	reloaded := &App{configDir: app.configDir}
	reloaded.customTagRules = reloaded.loadCustomTagRules()
	if len(reloaded.customTagRules) != 2 {
		t.Fatalf("重启后应载入 2 条规则，实际 %#v", reloaded.customTagRules)
	}

	file := parser.VPKFile{Name: "三角洲-重塑.vpk", SecondaryTags: []string{"贴图"}}
	reloaded.applyCustomTagRulesFor(&file)
	joined := strings.Join(file.SecondaryTags, ",")
	if !strings.Contains(joined, "三角洲") || !strings.Contains(joined, "贴图包") {
		t.Fatalf("规则应补上两个标签: %#v", file.SecondaryTags)
	}
	if !strings.Contains(joined, "贴图") {
		t.Fatalf("自动识别的标签必须保留: %#v", file.SecondaryTags)
	}

	// 再次应用不产生重复标签（幂等）。
	reloaded.applyCustomTagRulesFor(&file)
	occurrences := strings.Count(strings.Join(file.SecondaryTags, ","), "三角洲")
	if occurrences != 1 {
		t.Fatalf("重复应用不该产生重复标签: %#v", file.SecondaryTags)
	}
}

func TestValidateCustomTagRulesReportsIssues(t *testing.T) {
	app := newCustomRulesTestApp(t)
	report, err := app.ValidateCustomTagRules(`{"version":1,"rules":[{"tag":"ok","nameContains":["a"]},{"tag":"bad"}]}`)
	if err != nil {
		t.Fatalf("校验不该整体失败: %v", err)
	}
	if report.RuleCount != 1 || len(report.Issues) != 1 {
		t.Fatalf("校验结果不对: %#v", report)
	}
	if _, err := app.SaveCustomTagRules(`{"version":1,"rules":[{"tag":"bad"}]}`); err == nil {
		t.Fatal("保存有问题的规则必须报错")
	}
}
