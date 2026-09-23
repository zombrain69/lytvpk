package grouping

import (
	"fmt"
	"strings"
	"testing"
)

func keys(suggestion Suggestion) string {
	return strings.Join(suggestion.MemberKeys, "|")
}

func findSuggestion(suggestions []Suggestion, signal string) *Suggestion {
	for index := range suggestions {
		for _, value := range suggestions[index].Signals {
			if value == signal {
				return &suggestions[index]
			}
		}
	}
	return nil
}

func TestSuggestProducesEveryDocumentedSignal(t *testing.T) {
	mods := []Mod{
		// folder：同一套件目录（含子目录）
		{Key: `pack\基础\a.vpk`, Name: "a.vpk", Title: "整套甲", Folder: `pack\基础`},
		{Key: `pack\开关\b.vpk`, Name: "b.vpk", Title: "整套乙", Folder: `pack\开关`},
		// voice：同一角色语音
		{Key: "voice1.vpk", Name: "voice1.vpk", Title: "Nick 语音一", VoiceCharacters: []string{"Nick"}, PrimaryTag: "人物"},
		{Key: "voice2.vpk", Name: "voice2.vpk", Title: "Nick 语音二", VoiceCharacters: []string{"Nick"}, PrimaryTag: "人物"},
		// subject：同一替换目标
		{Key: "sub1.vpk", Name: "sub1.vpk", Title: "M16 甲", Subject: "主体：M16 武器", SubjectConfidence: "高", PrimaryTag: "武器"},
		{Key: "sub2.vpk", Name: "sub2.vpk", Title: "M16 乙", Subject: "主体：M16 武器", SubjectConfidence: "高", PrimaryTag: "武器"},
		// 同名不同目录（同一套件内）
		{Key: `pack2\动画\bill.vpk`, Name: "bill.vpk", Title: "Bill 动画版", Folder: `pack2\动画`},
		{Key: `pack2\静态\bill.vpk`, Name: "bill.vpk", Title: "Bill 静态版", Folder: `pack2\静态`},
		// 文件名前缀 + 共同标签 + 同一作者
		{Key: "series_a.vpk", Name: "series_a.vpk", Title: "Series 甲", Author: "某人", PrimaryTag: "武器", SecondaryTags: []string{"AK47", "皮肤"}},
		{Key: "series_b.vpk", Name: "series_b.vpk", Title: "Series 乙", Author: "某人", PrimaryTag: "武器", SecondaryTags: []string{"AK47", "皮肤"}},
		{Key: "series_c.vpk", Name: "series_c.vpk", Title: "Series 丙", Author: "某人", PrimaryTag: "武器", SecondaryTags: []string{"皮肤", "AK47"}},
	}

	suggestions, stats := Suggest(mods, DefaultOptions())
	if stats.Mods != len(mods) || stats.Candidates == 0 {
		t.Fatalf("统计信息异常: %#v", stats)
	}
	for _, signal := range []string{"同一文件夹", "同名文件（不同目录）", "文件名前缀", "共同标签", "同一作者", "语音角色", "主体识别"} {
		if findSuggestion(suggestions, signal) == nil {
			t.Fatalf("缺少信号 %s 的建议: %#v", signal, suggestions)
		}
	}
	if stats.ByProvider["subject"] == 0 || stats.ByProvider["voice-character"] == 0 {
		t.Fatalf("信号统计缺失: %#v", stats.ByProvider)
	}
}

func TestSuggestDedupesSameMemberSetAndKeepsStrongestLabel(t *testing.T) {
	mods := []Mod{
		{Key: "a.vpk", Name: "a.vpk", Title: "甲", Author: "同一人", Subject: "主体：AK47 武器", SubjectConfidence: "高", PrimaryTag: "武器", SecondaryTags: []string{"AK47", "皮肤", "涂装"}},
		{Key: "b.vpk", Name: "b.vpk", Title: "乙", Author: "同一人", Subject: "主体：AK47 武器", SubjectConfidence: "高", PrimaryTag: "武器", SecondaryTags: []string{"AK47", "皮肤", "涂装"}},
	}
	suggestions, _ := Suggest(mods, DefaultOptions())
	if len(suggestions) != 1 {
		t.Fatalf("同一批成员应合并成一条: %#v", suggestions)
	}
	merged := suggestions[0]
	// 同一批成员命中"同一作者 + 主体识别"两个信号（标签对只有 2 个成员，不到 3 个）。
	if len(merged.Signals) < 2 {
		t.Fatalf("合并后应保留全部信号: %#v", merged.Signals)
	}
	// 主体（50）分数高于作者 / 标签（45），标签应来自主体。
	if merged.Label != "AK47 武器" {
		t.Fatalf("应保留分数最高的理由对应的组名: %#v", merged)
	}
}

func TestSuggestAppliesProviderQuota(t *testing.T) {
	// 30 个互不相关的 Mod，各自独立的"主体"，全部来自 subject 信号。
	mods := make([]Mod, 0, 60)
	for index := 0; index < 30; index++ {
		subject := fmt.Sprintf("主体：目标%02d 武器", index)
		for sub := 0; sub < 2; sub++ {
			mods = append(mods, Mod{
				Key:               fmt.Sprintf("quota_%02d_%d.vpk", index, sub),
				Name:              fmt.Sprintf("quota_%02d_%d.vpk", index, sub),
				Title:             fmt.Sprintf("目标%02d 第%d版", index, sub),
				Subject:           subject,
				SubjectConfidence: "高",
				PrimaryTag:        "武器",
			})
		}
	}
	options := DefaultOptions()
	options.Limit = 40
	options.PerProviderLimit = 5
	suggestions, _ := Suggest(mods, options)
	count := 0
	for _, suggestion := range suggestions {
		if findSignal(suggestion, "主体识别") {
			count++
		}
	}
	if count > options.PerProviderLimit {
		t.Fatalf("单一信号超过配额: %d > %d", count, options.PerProviderLimit)
	}
}

// 回归：配额裁剪曾只改本地切片长度，导致被删除的候选在尾部残留，
// 表现为"同一批成员的建议重复几百次"。这里锁住不变量：
// 结果里不允许出现成员集合完全相同的两条建议。
func TestSuggestNeverReturnsDuplicateMemberSets(t *testing.T) {
	mods := make([]Mod, 0, 120)
	for index := 0; index < 60; index++ {
		subject := fmt.Sprintf("主体：目标%02d 武器", index)
		for sub := 0; sub < 2; sub++ {
			mods = append(mods, Mod{
				Key:               fmt.Sprintf("dup_%02d_%d.vpk", index, sub),
				Name:              fmt.Sprintf("dup_%02d_%d.vpk", index, sub),
				Title:             fmt.Sprintf("目标%02d 第%d版", index, sub),
				Subject:           subject,
				SubjectConfidence: "高",
				PrimaryTag:        "武器",
			})
		}
	}
	options := DefaultOptions()
	options.Limit = 600
	options.PerProviderLimit = 5
	suggestions, _ := Suggest(mods, options)

	seen := make(map[string]struct{}, len(suggestions))
	for _, suggestion := range suggestions {
		key := strings.Join(suggestion.MemberKeys, "|")
		if _, ok := seen[key]; ok {
			t.Fatalf("出现重复成员集合的建议: %s (%#v)", key, suggestion.Label)
		}
		seen[key] = struct{}{}
	}
	if len(suggestions) > options.PerProviderLimit {
		t.Fatalf("配额未生效: %d 条 > %d", len(suggestions), options.PerProviderLimit)
	}
}

func findSignal(suggestion Suggestion, signal string) bool {
	for _, value := range suggestion.Signals {
		if value == signal {
			return true
		}
	}
	return false
}

func TestSuggestIsDeterministic(t *testing.T) {
	mods := []Mod{
		{Key: "b.vpk", Name: "b.vpk", Title: "乙", Subject: "主体：M16 武器", SubjectConfidence: "高", PrimaryTag: "武器"},
		{Key: "a.vpk", Name: "a.vpk", Title: "甲", Subject: "主体：M16 武器", SubjectConfidence: "高", PrimaryTag: "武器"},
		{Key: "c.vpk", Name: "c.vpk", Title: "丙", Subject: "主体：M16 武器", SubjectConfidence: "高", PrimaryTag: "武器"},
	}
	first, _ := Suggest(mods, DefaultOptions())
	second, _ := Suggest(mods, DefaultOptions())
	if len(first) != len(second) {
		t.Fatalf("两次推导结果数量不同: %d vs %d", len(first), len(second))
	}
	for index := range first {
		if first[index].ID != second[index].ID || keys(first[index]) != keys(second[index]) {
			t.Fatalf("结果不确定: %#v vs %#v", first[index], second[index])
		}
	}
}

func TestCatalogCoversDocumentedSignals(t *testing.T) {
	catalog := Catalog()
	if len(catalog) < 8 {
		t.Fatalf("信号目录过小: %#v", catalog)
	}
	for _, spec := range catalog {
		if spec.ID == "" || spec.Label == "" {
			t.Fatalf("信号缺少 ID 或展示名: %#v", spec)
		}
		if SignalLabel(spec.ID) != spec.Label {
			t.Fatalf("ID → 展示名映射不一致: %#v", spec)
		}
		if SignalID(spec.Label) != spec.ID {
			t.Fatalf("展示名 → ID 映射不一致: %#v", spec)
		}
		switch spec.Confidence {
		case "high", "medium", "low":
		default:
			t.Fatalf("非法可信度: %#v", spec)
		}
	}
}

func TestConfidenceForSignals(t *testing.T) {
	cases := []struct {
		signals []string
		want    string
	}{
		{[]string{"工坊合集成员"}, "high"},
		{[]string{"同一文件夹"}, "high"},
		{[]string{"外部建议"}, "high"},
		{[]string{"主体识别"}, "medium"},
		{[]string{"共同标签"}, "medium"},
		{[]string{"语音角色"}, "medium"},
		{[]string{"文件名前缀"}, "low"},
		{[]string{"同一作者"}, "low"},
		{[]string{"文件名前缀", "主体识别"}, "medium"},
		{[]string{"未知信号"}, "low"},
	}
	for _, testCase := range cases {
		if got := ConfidenceForSignals(testCase.signals); got != testCase.want {
			t.Fatalf("signals=%v got=%q want=%q", testCase.signals, got, testCase.want)
		}
	}
}

func TestNormalizeAuthorAndSubject(t *testing.T) {
	for _, raw := range []string{"", "AUTHOR_NAME", "Animal33/zmg/momo", "Dazzle + All_calm", "甲，乙", "无", "作者"} {
		if got := NormalizeAuthor(raw); got != "" {
			t.Fatalf("应过滤作者 %q，得到 %q", raw, got)
		}
	}
	if got := NormalizeAuthor("Dazzle_白麒麟"); got != "Dazzle_白麒麟" {
		t.Fatalf("正常作者被过滤: %q", got)
	}
	if got := NormalizeSubject("主体：混合包（AK47 武器、HUD）"); got != "" {
		t.Fatalf("混合包主体应被丢弃: %q", got)
	}
	if got := NormalizeSubject("主体：Nick 模型"); got != "Nick 模型" {
		t.Fatalf("主体前缀未去掉: %q", got)
	}
}

func TestSortScoreCalibration(t *testing.T) {
	options := DefaultOptions()
	subject := SortScore(SubjectScore, []string{"主体识别"}, 3, nil, options)
	tagsBroad := SortScore(TagScore, []string{"共同标签"}, 12, nil, options)
	prefix := SortScore(PrefixScore, []string{"文件名前缀"}, 3, nil, options)
	collection := SortScore(CollectionScore, []string{"工坊合集成员"}, 30, nil, options)

	if subject != 70 {
		t.Fatalf("3 成员主体小组排序分 = %d, want 70", subject)
	}
	if tagsBroad != 49 {
		t.Fatalf("12 成员标签聚类排序分 = %d, want 49", tagsBroad)
	}
	if prefix != 45 {
		t.Fatalf("弱启发式排序分 = %d, want 45", prefix)
	}
	if collection != 120 {
		t.Fatalf("工坊合集不应降权: %d, want 120", collection)
	}
	if !(collection > subject && subject > tagsBroad && tagsBroad > prefix) {
		t.Fatalf("排序配方不符合预期: collection=%d subject=%d tagsBroad=%d prefix=%d", collection, subject, tagsBroad, prefix)
	}
}

// BenchmarkSuggest 用 2000 个合成 Mod 覆盖全部信号，观察推导耗时与分配。
func BenchmarkSuggest(b *testing.B) {
	mods := make([]Mod, 0, 2000)
	for index := 0; index < 2000; index++ {
		mod := Mod{
			Key:               fmt.Sprintf("bench_%04d.vpk", index),
			Name:              fmt.Sprintf("bench mod %04d.vpk", index),
			Title:             fmt.Sprintf("基准 Mod %04d", index),
			Author:            fmt.Sprintf("作者%02d", index%40),
			PrimaryTag:        "武器",
			SecondaryTags:     []string{fmt.Sprintf("标签%02d", index%30), fmt.Sprintf("类型%02d", index%12), "通用"},
			Subject:           fmt.Sprintf("主体：目标%03d 武器", index%300),
			SubjectConfidence: "高",
			Folder:            fmt.Sprintf("套件%02d/子目录%d", index%25, index%3),
		}
		if index%5 == 0 {
			mod.VoiceCharacters = []string{fmt.Sprintf("角色%02d", index%10)}
		}
		mods = append(mods, mod)
	}
	options := DefaultOptions()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		suggestions, _ := Suggest(mods, options)
		if len(suggestions) == 0 {
			b.Fatal("no suggestions")
		}
	}
}

// 用户实测：`[Milfy]白银审判…` 这一套（角色本体 + 上衣关 + 内衣关，中文名里没有分隔符）
// 明明是同一套，却一条建议都没有——因为老的前缀实现要求"文件名至少两个词"，
// 中文名（`[Milfy]白银审判里内衣关.vpk`）只有一个词，直接被判空。
func TestFilenamePrefixGroupsSeriesWithoutSeparators(t *testing.T) {
	mods := []Mod{
		{Key: "[Milfy]白银审判_Rochelle.vpk", Name: "[Milfy]白银审判_Rochelle.vpk", PrimaryTag: "人物"},
		{Key: "[Milfy]白银审判里内衣关.vpk", Name: "[Milfy]白银审判里内衣关.vpk", PrimaryTag: "人物"},
		{Key: "[Milfy]白银审判上衣关.vpk", Name: "[Milfy]白银审判上衣关.vpk", PrimaryTag: "人物"},
		{Key: "[Milfy]白银审判里内衣关-2.vpk", Name: "[Milfy]白银审判里内衣关-2.vpk", PrimaryTag: "人物"},
	}
	suggestions, _ := Suggest(mods, DefaultOptions())
	prefix := findSuggestion(suggestions, "文件名前缀")
	if prefix == nil {
		t.Fatalf("同一套中文名（无分隔符）应给出文件名前缀建议: %#v", suggestions)
	}
	if len(prefix.MemberKeys) != 4 {
		t.Fatalf("这一套 4 个 Mod 应全部进同一条建议: %#v", prefix)
	}
	if prefix.Label != "白银审判" {
		t.Fatalf("组名应取去掉 [标签] 之后的系列名: %#v", prefix.Label)
	}
}

// 反向保护：`[标签]` 里的工坊作者/标签不能自己变成系列名，
// 否则所有同一标签的 Mod 会被凑成一条超大建议。
func TestFilenamePrefixIgnoresBracketTagAsSeries(t *testing.T) {
	mods := []Mod{
		{Key: "[Milfy]白银审判_Rochelle.vpk", Name: "[Milfy]白银审判_Rochelle.vpk", PrimaryTag: "人物"},
		{Key: "[Milfy]猎魔人_Rochelle.vpk", Name: "[Milfy]猎魔人_Rochelle.vpk", PrimaryTag: "人物"},
		{Key: "[Milfy]泳装_Rochelle.vpk", Name: "[Milfy]泳装_Rochelle.vpk", PrimaryTag: "人物"},
	}
	suggestions, _ := Suggest(mods, DefaultOptions())
	if prefix := findSuggestion(suggestions, "文件名前缀"); prefix != nil && len(prefix.MemberKeys) == 3 {
		t.Fatalf("只共享 [Milfy] 标签的三套不同内容不应被当成同一系列: %#v", prefix)
	}
}
