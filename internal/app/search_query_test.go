package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type searchQueryCase struct {
	Query       string     `json:"query"`
	Terms       []string   `json:"terms"`
	IncludeTags [][]string `json:"includeTags"`
	ExcludeTags []string   `json:"excludeTags"`
	Regex       string     `json:"regex"`
	RegexError  bool       `json:"regexError"`
}

func loadSearchQueryCases(t *testing.T) []searchQueryCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "search_query_cases.json"))
	if err != nil {
		t.Fatalf("读取共享用例失败: %v", err)
	}
	var payload struct {
		Cases []searchQueryCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("解析共享用例失败: %v", err)
	}
	if len(payload.Cases) == 0 {
		t.Fatal("共享用例为空")
	}
	return payload.Cases
}

// 与前端共用同一份用例（testdata/search_query_cases.json），保证两边解析结果一致。
func TestParseModSearchQueryMatchesSharedCases(t *testing.T) {
	for _, tc := range loadSearchQueryCases(t) {
		t.Run(tc.Query, func(t *testing.T) {
			spec := parseModSearchQuery(tc.Query)
			if !sameStringList(spec.Terms, tc.Terms) {
				t.Fatalf("terms = %v, 期望 %v", spec.Terms, tc.Terms)
			}

			gotInclude := make([][]string, 0, len(spec.IncludeTags))
			for _, condition := range spec.IncludeTags {
				gotInclude = append(gotInclude, condition.Terms)
			}
			wantInclude := tc.IncludeTags
			if wantInclude == nil {
				wantInclude = [][]string{}
			}
			if !reflect.DeepEqual(gotInclude, wantInclude) {
				t.Fatalf("includeTags = %v, 期望 %v", gotInclude, wantInclude)
			}

			wantExclude := tc.ExcludeTags
			if wantExclude == nil {
				wantExclude = []string{}
			}
			if !sameStringList(spec.ExcludeTags, wantExclude) {
				t.Fatalf("excludeTags = %v, 期望 %v", spec.ExcludeTags, wantExclude)
			}

			if tc.Regex == "" {
				if spec.Regex != nil {
					t.Fatalf("不应有正则，实际 %v", spec.Regex)
				}
			} else if spec.Regex == nil || spec.Regex.String() != "(?i)"+tc.Regex {
				t.Fatalf("正则 = %v, 期望 (?i)%s", spec.Regex, tc.Regex)
			}
			if tc.RegexError && spec.RegexError == "" {
				t.Fatal("应当记录正则错误")
			}
			if !tc.RegexError && spec.RegexError != "" {
				t.Fatalf("不应有正则错误: %s", spec.RegexError)
			}
		})
	}
}

// sameStringList 把 nil 与空切片视为相等（JSON 里写 [] 与 Go 的 nil 语义一致）。
func sameStringList(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// 匹配语义：tag: 的 OR / AND / NOT 与 FireAxe 的三种 TagFilterMode 对齐。
func TestModSearchQueryMatchesFields(t *testing.T) {
	fields := searchableModFields{
		Title:           "AK47 突击步枪",
		Name:            "ak47_替换.vpk",
		PrimaryTag:      "武器",
		SecondaryTags:   []string{"步枪", "材质"},
		SubjectSummary:  "AK47 武器",
		ContentSubjects: []string{"models/weapons/v_rifle_ak47.mdl"},
	}

	cases := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"ak47", true},
		{"ak47 武器", true},
		{"ak47 狙击", false},          // 多个普通词是"且"
		{"tag:武器", true},            // Or/单标签
		{"tag:步枪|狙击", true},        // Or（任一）
		{"tag:武器 tag:步枪", true},    // And（都要）
		{"tag:武器 tag:狙击", false},   // And 缺一个
		{"-tag:材质", false},         // Not：含该标签 ⇒ 排除
		{"-tag:狙击", true},          // Not：不含 ⇒ 保留
		{"tag:武器 -tag:材质 ak47", false},
		{"tag:武器 -tag:狙击 ak47", true},
		{"re:^ak\\d+", true}, // 正则匹配文件名/标题里的 ak47
		{"re:^zzz", false},
		{"-材质", false}, // 普通词减法：命中即排除
		{"-狙击", true},
		// `-排除` 用子串而不是模糊：长路径里"凑字母"不该把记录排掉
		{"-old", true},  // 路径里有 o/l/d 但不含 "old" 这个词
		{"-sniper", true},
	}
	for _, tc := range cases {
		spec := parseModSearchQuery(tc.query)
		if got := spec.matches(fields); got != tc.want {
			t.Fatalf("query %q 匹配结果 = %v, 期望 %v", tc.query, got, tc.want)
		}
	}
}

// 排除词必须是**子串**命中：字段里有完整路径，模糊匹配会让 `-old`、`-hd`
// 这类短词在长路径里"凑字母"命中，把不相干的记录一起排掉。
func TestNegativeTermUsesSubstringNotFuzzy(t *testing.T) {
	fields := searchableModFields{
		Title:     "AK47 高清",
		Name:      "ak47-hd.vpk",
		PrimaryTag: "武器",
		// 长字段（搜索材料里的路径/主体）是"模糊误伤"的温床：这里用一段含 o/l/d 散落字符的文本
		SubjectSummary: `E:\SteamLibrary\steamapps\common\Left 4 Dead 2\program\lytvpk\internal\.tmp-cua\fixture8\left4dead2\addons\ak47-hd.vpk`,
	}
	// 正向词仍然是模糊的（保持既有手感）：`a4` 能命中 ak47
	if !parseModSearchQuery("a4").matches(fields) {
		t.Fatal("正向词应当保持模糊匹配")
	}
	// 但排除词不再模糊：路径里虽然散落着 o/l/d，却没有 "old" 子串
	if !parseModSearchQuery("-old").matches(fields) {
		t.Fatal("排除词不该用模糊匹配误伤长路径")
	}
	// 真正包含时照旧排除
	if parseModSearchQuery("-ak47").matches(fields) {
		t.Fatal("包含该词的记录仍应被排除")
	}
	if parseModSearchQuery("-高清").matches(fields) {
		t.Fatal("中文排除词同样按子串生效")
	}
}

// 正则写错时：一个都不匹配，并带上原因（界面要显示出来）。
func TestModSearchQueryInvalidRegexMatchesNothing(t *testing.T) {
	spec := parseModSearchQuery("re:[")
	if spec.RegexError == "" {
		t.Fatal("应记录正则错误")
	}
	if spec.matches(searchableModFields{Title: "任何东西"}) {
		t.Fatal("正则无效时不应匹配任何记录")
	}
}

// 端到端：搜索框语法要真的作用在 SearchVPKFiles 上（tag: 过滤 + 排除 + 正则）。
func TestSearchVPKFilesSupportsQueryGrammar(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	// 给夹具打上可预期的标签：a.vpk 是"武器/步枪"，b.vpk 是"武器/贴图"。
	a.vpkCache.Range(func(key any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		switch strings.ToLower(cache.File.Name) {
		case "a.vpk":
			cache.File.PrimaryTag = "武器"
			cache.File.SecondaryTags = []string{"步枪"}
		case "b.vpk":
			cache.File.PrimaryTag = "武器"
			cache.File.SecondaryTags = []string{"贴图"}
		}
		a.vpkCache.Store(key, cache)
		return true
	})
	_ = addonsDir

	names := func(files []VPKFile) []string {
		result := make([]string, 0, len(files))
		for _, file := range files {
			result = append(result, strings.ToLower(file.Name))
		}
		return result
	}

	if got := names(a.SearchVPKFiles("tag:武器 tag:步枪", "", nil)); !reflect.DeepEqual(got, []string{"a.vpk"}) {
		t.Fatalf("tag AND 过滤结果 = %v, 期望 [a.vpk]", got)
	}
	if got := names(a.SearchVPKFiles("tag:武器 -tag:贴图", "", nil)); !reflect.DeepEqual(got, []string{"a.vpk"}) {
		t.Fatalf("tag 排除结果 = %v, 期望 [a.vpk]", got)
	}
	if got := names(a.SearchVPKFiles("re:^[ab]\\.vpk$", "", nil)); len(got) < 2 {
		t.Fatalf("正则搜索应命中 a/b，实际 %v", got)
	}
	if got := names(a.SearchVPKFiles("re:[", "", nil)); len(got) != 0 {
		t.Fatalf("非法正则应返回空结果，实际 %v", got)
	}
}
