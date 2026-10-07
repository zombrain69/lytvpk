package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"vpk-manager/internal/parser"
)

// 自定义标签规则（导入 JSON + 预览命中 + 保存后重扫）。
//
// 定位：内置规则表（internal/ruletable/rules.json）是"识别 VPK 里到底装了什么"的通用口径，
// 改动它要走发布流程；但每个人的库都有自己的一套命名习惯（"三角洲""崩铁""写实包"…）。
// 这里让用户按**文件名 / 标题 / 已识别标签**再补一层自定义标签，口径与既有设计一致：
//
//   - **只增不减**：命中就加标签，永远不改/删自动识别结果；
//   - 不参与"互斥/冲突/优先级"判定，只是筛选维度；
//   - 规则文件是纯 JSON（可导入导出），保存后重扫一次即可全库生效。
//
// 为什么不做"正则 + 布尔表达式"（xavier-cai 那种）：那套要配可视化编辑器与语法教学，
// 维护成本高；先做"关键词 + 单条正则"，覆盖绝大多数命名习惯。

const customTagRulesVersion = 1

type CustomTagRule struct {
	// Tag：命中后给 Mod 加的二级标签（必填）。
	Tag string `json:"tag"`
	// NameContains：文件名里包含任意一个关键词即命中（不区分大小写）。
	NameContains []string `json:"nameContains,omitempty"`
	// TitleContains：标题（addoninfo/meta）里包含任意一个关键词即命中。
	TitleContains []string `json:"titleContains,omitempty"`
	// AnyTags：Mod 已有标签里包含任意一个即命中（"给已识别为贴图的包再打一个仓库标签"）。
	AnyTags []string `json:"anyTags,omitempty"`
	// Regex：对"文件名 + 标题"做一次不区分大小写的正则匹配（可选，写错会在保存时报错）。
	Regex string `json:"regex,omitempty"`
	// Enabled：省略 = 启用；显式 false 可以临时停用一条规则。
	Enabled *bool `json:"enabled,omitempty"`
}

type CustomTagRuleFile struct {
	Version int             `json:"version"`
	Rules   []CustomTagRule `json:"rules"`
}

type CustomTagRuleIssue struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}

type CustomTagRulePreviewItem struct {
	Index      int      `json:"index"`
	Tag        string   `json:"tag"`
	MatchCount int      `json:"matchCount"`
	Samples    []string `json:"samples,omitempty"`
}

type CustomTagRulePreview struct {
	RuleCount int                        `json:"ruleCount"`
	Items     []CustomTagRulePreviewItem `json:"items"`
	Issues    []CustomTagRuleIssue       `json:"issues,omitempty"`
}

func (a *App) customTagRulesPath() string {
	a.mu.RLock()
	dir := a.configDir
	a.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	return filepath.Join(dir, "custom_tag_rules.json")
}

// LoadCustomTagRules 读取规则文件（不存在/损坏时返回空规则集，不影响扫描）。
func (a *App) loadCustomTagRules() []CustomTagRule {
	path := a.customTagRulesPath()
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	parsed, _, err := parseCustomTagRules(string(raw))
	if err != nil {
		return nil
	}
	return parsed
}

// GetCustomTagRules 返回规则文件的原始 JSON（供界面编辑）。
func (a *App) GetCustomTagRules() (string, error) {
	path := a.customTagRulesPath()
	if path == "" {
		return "", fmt.Errorf("配置目录不可用")
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		empty := CustomTagRuleFile{Version: customTagRulesVersion, Rules: []CustomTagRule{}}
		encoded, _ := json.MarshalIndent(empty, "", "  ")
		return string(encoded), nil
	}
	if err != nil {
		return "", fmt.Errorf("读取自定义标签规则失败: %w", err)
	}
	return string(raw), nil
}

// ValidateCustomTagRules 只做校验（不落盘、不扫描），返回规则条数与问题清单。
func (a *App) ValidateCustomTagRules(rawJSON string) (CustomTagRulePreview, error) {
	rules, issues, err := parseCustomTagRules(rawJSON)
	if err != nil {
		return CustomTagRulePreview{}, err
	}
	return CustomTagRulePreview{RuleCount: len(rules), Issues: issues}, nil
}

// PreviewCustomTagRules 用当前扫描缓存试算命中：每条规则命中多少 Mod + 最多 5 个样例。
// 不重新扫描（文件名/标题/标签都在缓存里），所以是即时反馈。
func (a *App) PreviewCustomTagRules(rawJSON string) (CustomTagRulePreview, error) {
	rules, issues, err := parseCustomTagRules(rawJSON)
	if err != nil {
		return CustomTagRulePreview{}, err
	}
	preview := CustomTagRulePreview{RuleCount: len(rules), Issues: issues}
	files := a.allVPKFilesSnapshot()
	for index, rule := range rules {
		item := CustomTagRulePreviewItem{Index: index, Tag: rule.Tag}
		for _, file := range files {
			if customTagRuleMatches(rule, file) {
				item.MatchCount++
				if len(item.Samples) < 5 {
					name := file.Name
					if strings.TrimSpace(file.Title) != "" {
						name = file.Title
					}
					item.Samples = append(item.Samples, name)
				}
			}
		}
		preview.Items = append(preview.Items, item)
	}
	return preview, nil
}

// SaveCustomTagRules 校验并保存规则，然后由调用方触发重扫。
func (a *App) SaveCustomTagRules(rawJSON string) (int, error) {
	rules, issues, err := parseCustomTagRules(rawJSON)
	if err != nil {
		return 0, err
	}
	if len(issues) > 0 {
		return 0, fmt.Errorf("规则有问题：#%d %s", issues[0].Index+1, issues[0].Message)
	}
	path := a.customTagRulesPath()
	if path == "" {
		return 0, fmt.Errorf("配置目录不可用")
	}
	payload := CustomTagRuleFile{Version: customTagRulesVersion, Rules: rules}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return 0, fmt.Errorf("保存自定义标签规则失败: %w", err)
	}
	a.mu.Lock()
	a.customTagRules = rules
	a.mu.Unlock()
	return len(rules), nil
}

// applyCustomTagRulesFor 在扫描结果上补齐自定义标签（只增不减）。
func (a *App) applyCustomTagRulesFor(file *parser.VPKFile) {
	if file == nil {
		return
	}
	a.mu.RLock()
	rules := a.customTagRules
	a.mu.RUnlock()
	if len(rules) == 0 {
		return
	}
	existing := make(map[string]bool, len(file.SecondaryTags))
	for _, tag := range file.SecondaryTags {
		existing[strings.ToLower(strings.TrimSpace(tag))] = true
	}
	for _, rule := range rules {
		if customTagRuleMatches(rule, *file) {
			key := strings.ToLower(strings.TrimSpace(rule.Tag))
			if key == "" || existing[key] {
				continue
			}
			existing[key] = true
			file.SecondaryTags = append(file.SecondaryTags, rule.Tag)
		}
	}
	sort.SliceStable(file.SecondaryTags, func(i, j int) bool { return file.SecondaryTags[i] < file.SecondaryTags[j] })
}

// parseCustomTagRules 解析 + 逐条校验；issues 是"这条规则有问题但不影响其它规则"的清单。
func parseCustomTagRules(rawJSON string) ([]CustomTagRule, []CustomTagRuleIssue, error) {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" {
		return nil, nil, nil
	}
	var payload CustomTagRuleFile
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		// 兼容"直接给一个规则数组"的写法。
		var bare []CustomTagRule
		if err2 := json.Unmarshal([]byte(trimmed), &bare); err2 != nil {
			return nil, nil, fmt.Errorf("规则文件不是合法 JSON: %w", err)
		}
		payload = CustomTagRuleFile{Version: customTagRulesVersion, Rules: bare}
	}
	if payload.Version != 0 && payload.Version != customTagRulesVersion {
		return nil, nil, fmt.Errorf("规则文件版本不支持: %d（当前支持 %d）", payload.Version, customTagRulesVersion)
	}
	issues := make([]CustomTagRuleIssue, 0)
	rules := make([]CustomTagRule, 0, len(payload.Rules))
	for index, rule := range payload.Rules {
		rule.Tag = strings.TrimSpace(rule.Tag)
		if rule.Tag == "" {
			issues = append(issues, CustomTagRuleIssue{Index: index, Message: "缺少 tag"})
			continue
		}
		if rule.Enabled != nil && !*rule.Enabled {
			continue
		}
		if len(rule.NameContains) == 0 && len(rule.TitleContains) == 0 && len(rule.AnyTags) == 0 && strings.TrimSpace(rule.Regex) == "" {
			issues = append(issues, CustomTagRuleIssue{Index: index, Message: "至少要写一个匹配条件（nameContains / titleContains / anyTags / regex）"})
			continue
		}
		if regex := strings.TrimSpace(rule.Regex); regex != "" {
			if _, err := regexp.Compile("(?i)" + regex); err != nil {
				issues = append(issues, CustomTagRuleIssue{Index: index, Message: "正则不合法：" + err.Error()})
				continue
			}
		}
		rules = append(rules, rule)
	}
	return rules, issues, nil
}

func customTagRuleMatches(rule CustomTagRule, file parser.VPKFile) bool {
	name := strings.ToLower(file.Name)
	title := strings.ToLower(file.Title)
	if containsAnyKeyword(name, rule.NameContains) || containsAnyKeyword(title, rule.TitleContains) {
		return true
	}
	if len(rule.AnyTags) > 0 {
		existing := make(map[string]struct{}, len(file.SecondaryTags))
		for _, tag := range file.SecondaryTags {
			existing[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
		}
		for _, want := range rule.AnyTags {
			if _, ok := existing[strings.ToLower(strings.TrimSpace(want))]; ok {
				return true
			}
		}
	}
	if regex := strings.TrimSpace(rule.Regex); regex != "" {
		if compiled, err := regexp.Compile("(?i)" + regex); err == nil {
			if compiled.MatchString(file.Name) || compiled.MatchString(file.Title) {
				return true
			}
		}
	}
	return false
}

func containsAnyKeyword(haystack string, keywords []string) bool {
	for _, keyword := range keywords {
		trimmed := strings.ToLower(strings.TrimSpace(keyword))
		if trimmed == "" {
			continue
		}
		if strings.Contains(haystack, trimmed) {
			return true
		}
	}
	return false
}
