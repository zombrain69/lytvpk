package app

import (
	"vpk-manager/internal/grouping"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func seedCachedMod(a *App, path string, file VPKFile) {
	file.Path = path
	a.vpkCache.Store(path, &VPKFileCache{File: file})
}

func groupMemberKeys(group ModStrategyGroup) []string {
	keys := make([]string, 0, len(group.Members))
	for _, member := range group.Members {
		keys = append(keys, member.Key)
	}
	return keys
}

func TestGetModGroupMembershipReportsGroupInfo(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("打包组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if _, err := a.SetModStrategyGroupTier(group.ID, intPointer(-3)); err != nil {
		t.Fatal(err)
	}

	membership, err := a.GetModGroupMembership()
	if err != nil {
		t.Fatalf("membership: %v", err)
	}
	if len(membership) != 2 {
		t.Fatalf("应有 2 条归属记录: %#v", membership)
	}
	for _, item := range membership {
		if item.GroupID != group.ID || item.GroupName != "打包组" || item.Strategy != modStrategyGroupSingle {
			t.Fatalf("归属信息不完整: %#v", item)
		}
		if item.Tier == nil || *item.Tier != -3 {
			t.Fatalf("组权重应透出: %#v", item)
		}
		if item.MemberCount != 2 {
			t.Fatalf("成员数应为 2: %#v", item)
		}
	}
	keys := []string{membership[0].Key, membership[1].Key}
	if !reflect.DeepEqual(keys, []string{"a.vpk", "b.vpk"}) {
		t.Fatalf("成员键 = %#v", keys)
	}
}

func TestSetModStrategyGroupEnabledTogglesWholeGroup(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("打包组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.SetModStrategyGroupEnabled(group.ID, false)
	if err != nil {
		t.Fatalf("disable group: %v", err)
	}
	if len(result.Disabled) != 2 {
		t.Fatalf("整组关闭应报告 2 个成员: %#v", result)
	}
	states := addonListStateMap(mustReadAddonList(t, a))
	if states["a.vpk"] || states["b.vpk"] {
		t.Fatalf("组内成员应全部关闭: %#v", states)
	}
	// 非组成员不受影响（工坊条目原本是开启的）。
	if !states[`workshop\123.vpk`] {
		t.Fatalf("非组成员不应被改动: %#v", states)
	}

	result, err = a.SetModStrategyGroupEnabled(group.ID, true)
	if err != nil {
		t.Fatalf("enable group: %v", err)
	}
	if len(result.Enabled) != 2 {
		t.Fatalf("整组开启应报告 2 个成员: %#v", result)
	}
	states = addonListStateMap(mustReadAddonList(t, a))
	if !states["a.vpk"] || !states["b.vpk"] {
		t.Fatalf("组内成员应全部开启: %#v", states)
	}
}

func mustReadAddonList(t *testing.T, a *App) []AddonListItem {
	t.Helper()
	list, _, err := a.readAddonList()
	if err != nil {
		t.Fatalf("read addonlist: %v", err)
	}
	return list
}

func TestShiftModStrategyGroupPrioritiesMovesWholeGroup(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// a.vpk 顺序号 0、b.vpk 顺序号 2；两者同组。
	group, err := a.CaptureModStrategyGroup("打包组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}

	shift, err := a.ShiftModStrategyGroupPriorities(group.ID, 5)
	if err != nil {
		t.Fatalf("shift group: %v", err)
	}
	if len(shift.Moved) != 2 || len(shift.Skipped) != 0 {
		t.Fatalf("整组平移结果 = %#v", shift)
	}
	// 相对顺序保持：a(0→5) 仍在 b(2→7) 之前。
	if shift.Moved[0].Key != "a.vpk" || shift.Moved[0].From != 0 || shift.Moved[0].To != 5 {
		t.Fatalf("第一个成员 = %#v", shift.Moved[0])
	}
	if shift.Moved[1].Key != "b.vpk" || shift.Moved[1].From != 2 || shift.Moved[1].To != 7 {
		t.Fatalf("第二个成员 = %#v", shift.Moved[1])
	}

	entries, err := a.ListModPriorities()
	if err != nil {
		t.Fatal(err)
	}
	tiers := map[string]int{}
	for _, entry := range entries {
		tiers[entry.Key] = entry.Tier
	}
	if tiers["a.vpk"] != 5 || tiers["b.vpk"] != 7 {
		t.Fatalf("分层应写入 priority.json: %#v", tiers)
	}

	// 再次平移应基于新的分层继续移动。
	if _, err := a.ShiftModStrategyGroupPriorities(group.ID, -2); err != nil {
		t.Fatal(err)
	}
	entries, err = a.ListModPriorities()
	if err != nil {
		t.Fatal(err)
	}
	tiers = map[string]int{}
	for _, entry := range entries {
		tiers[entry.Key] = entry.Tier
	}
	if tiers["a.vpk"] != 3 || tiers["b.vpk"] != 5 {
		t.Fatalf("第二次平移结果 = %#v", tiers)
	}

	// addonlist.txt 不能被整组平移改写。
	if content := readAddonListBytes(t, filepath.Join(filepath.Dir(a.rootDir), "addonlist.txt")); !strings.Contains(string(content), `"a.vpk"`) {
		t.Fatalf("addonlist 内容异常: %s", content)
	}
}

func TestShiftModStrategyGroupPrioritiesSkipsUnknownMembers(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// c.vpk 不在 addonlist.txt 中，也没有分层 → 无法判断层级。
	group, err := a.CaptureModStrategyGroup("混合组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "c.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	shift, err := a.ShiftModStrategyGroupPriorities(group.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(shift.Moved) != 1 || len(shift.Skipped) != 1 {
		t.Fatalf("应移动 1 个、跳过 1 个: %#v", shift)
	}
	if shift.Skipped[0] != "c.vpk" {
		t.Fatalf("跳过项 = %#v", shift.Skipped)
	}
}

func TestSuggestModGroupsFromFilenamePrefixAndTags(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{
		Name: "Hit Marker Overhaul.vpk", Title: "Hit Marker Overhaul", Location: "root",
		SecondaryTags: []string{"HUD", "UI"}, SubjectSummary: "主体：HUD",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{
		Name: "Hit Marker Overhaul Image Support.vpk", Title: "Hit Marker Overhaul Image Support", Location: "root",
		SecondaryTags: []string{"HUD", "UI"}, SubjectSummary: "主体：HUD",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatal("应给出至少一条建议")
	}
	var merged *ModGroupSuggestion
	for index := range suggestions {
		item := &suggestions[index]
		if len(item.MemberKeys) != 2 || item.Confidence == "" || item.ID == "" {
			t.Fatalf("建议内容不完整: %#v", item)
		}
		merged = item
	}
	if merged == nil {
		t.Fatal("应给出至少一条建议")
	}
	// 同一批成员命中多个信号时应合并成一条，并保留最高分（主体 50 > 前缀 45）的理由。
	if merged.Score != 50 || !strings.Contains(merged.Reason, "主体相同") {
		t.Fatalf("合并后应保留最高分理由: %#v", merged)
	}
	signals := strings.Join(merged.Signals, ",")
	if !strings.Contains(signals, "主体识别") || !strings.Contains(signals, "文件名前缀") {
		t.Fatalf("合并后应包含全部信号: %#v", merged.Signals)
	}
	if len(suggestions) != 1 {
		t.Fatalf("同一批成员应合并为一条建议: %#v", suggestions)
	}
}

func TestSuggestModGroupsFromWorkshopCollectionAndExistingGroup(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	workshopDir := filepath.Join(addonsDir, "workshop")
	seedCachedMod(a, filepath.Join(workshopDir, "111.vpk"), VPKFile{
		Name: "111.vpk", Title: "合集物品一", Location: "workshop", WorkshopID: "111",
	})
	seedCachedMod(a, filepath.Join(workshopDir, "222.vpk"), VPKFile{
		Name: "222.vpk", Title: "合集物品二", Location: "workshop", WorkshopID: "222",
	})
	if err := a.writeWorkshopCollectionStore(workshopCollectionStore{Links: []WorkshopCollectionLink{{
		ID: "link-1", CollectionID: "999", Title: "整合作者包",
		Members: []WorkshopCollectionMember{
			{WorkshopID: "111", Title: "合集物品一"},
			{WorkshopID: "222", Title: "合集物品二"},
		},
	}}}); err != nil {
		t.Fatal(err)
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatal(err)
	}
	var collection *ModGroupSuggestion
	for index := range suggestions {
		if strings.Contains(suggestions[index].Reason, "工坊合集") {
			collection = &suggestions[index]
			break
		}
	}
	if collection == nil {
		t.Fatalf("应给出工坊合集建议: %#v", suggestions)
	}
	if collection.Confidence != "high" || len(collection.MemberKeys) != 2 {
		t.Fatalf("合集建议应为高置信度且含 2 个成员: %#v", collection)
	}
	if collection.ExistingGroupID != "" {
		t.Fatalf("尚未建组时不应标记已存在: %#v", collection)
	}

	// 按建议创建组后，再推导应标记为"已存在"。
	if _, err := a.CreateModStrategyGroupFromKeys("整合作者包", "来自工坊合集建议", modStrategyGroupAll, collection.MemberKeys); err != nil {
		t.Fatalf("create group from suggestion: %v", err)
	}
	suggestions, err = a.SuggestModGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range suggestions {
		if strings.Contains(item.Reason, "工坊合集") && item.ExistingGroupID == "" {
			t.Fatalf("已建组后应标记 ExistingGroupID: %#v", item)
		}
	}
}

func TestCreateModStrategyGroupFromKeysPreservesDisplayNames(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "123.vpk"), VPKFile{
		Name: "123.vpk", Title: "工坊条目", Location: "workshop", WorkshopID: "123",
	})
	group, err := a.CreateModStrategyGroupFromKeys("键组", "", modStrategyGroupSingle, []string{
		"A.VPK",
		`workshop\123.vpk`,
		"A.VPK", // 重复键应被去重
	})
	if err != nil {
		t.Fatalf("create from keys: %v", err)
	}
	if got := groupMemberKeys(group); !reflect.DeepEqual(got, []string{"a.vpk", `workshop\123.vpk`}) {
		t.Fatalf("成员键 = %#v", got)
	}
	if group.Members[0].Name != "A.VPK" && group.Members[0].Name != "a.vpk" {
		t.Fatalf("显示名应保留原始拼写或缓存名: %#v", group.Members[0])
	}
	if group.Members[1].Name != "123.vpk" {
		t.Fatalf("工坊条目显示名 = %q", group.Members[1].Name)
	}

	if _, err := a.CreateModStrategyGroupFromKeys("", "", modStrategyGroupAll, []string{"a.vpk"}); err == nil {
		t.Fatal("空名称应被拒绝")
	}
	if _, err := a.CreateModStrategyGroupFromKeys("空组", "", modStrategyGroupAll, nil); err == nil {
		t.Fatal("没有有效成员应被拒绝")
	}
	_ = os.Remove
}

// 「主体：混合包（…）」表示该 Mod 同时覆盖多类内容，成员之间并不构成
// 一个可以整体开关或整体排优先级的组，因此不应作为分组建议输出。
func TestSuggestModGroupsSkipsMixedSubjectClusters(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "ak47_a.vpk"), VPKFile{
		Name: "ak47_a.vpk", Title: "AK47 替换甲", Location: "root",
		SubjectSummary: "主体：混合包（AK47 武器、HUD）",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "m16_b.vpk"), VPKFile{
		Name: "m16_b.vpk", Title: "M16 替换乙", Location: "root",
		SubjectSummary: "主体：混合包（AK47 武器、HUD）",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("混合包主体不应产生分组建议: %#v", suggestions)
	}
}

// 主体标签已经带了"主体："前缀，直接用作组名会出现"主体相同：主体：xxx"这种重复文案。
func TestSuggestModGroupsTrimsSubjectPrefixFromLabel(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "aaa.vpk"), VPKFile{
		Name: "aaa.vpk", Title: "Nick 模型甲", Location: "root", SubjectSummary: "主体：Nick 模型",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "bbb.vpk"), VPKFile{
		Name: "bbb.vpk", Title: "Nick 模型乙", Location: "root", SubjectSummary: "主体：Nick 模型",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("应只有一条主体建议: %#v", suggestions)
	}
	got := suggestions[0]
	if got.Label != "Nick 模型" {
		t.Fatalf("组名应去掉主体前缀: %q", got.Label)
	}
	if strings.Contains(got.Reason, "主体：主体") || !strings.Contains(got.Reason, "Nick 模型") {
		t.Fatalf("理由文案重复或缺失: %q", got.Reason)
	}
	if got.Confidence != "medium" {
		t.Fatalf("主体推导属于内容证据，应标记为中置信度: %#v", got)
	}
}

func TestModGroupSuggestionConfidenceFollowsSignalType(t *testing.T) {
	cases := []struct {
		signals []string
		want    string
	}{
		{[]string{modGroupSignalCollection}, "high"},
		{[]string{modGroupSignalFilenamePrefix}, "low"},
		{[]string{modGroupSignalSharedTags}, "medium"},
		{[]string{modGroupSignalSameAuthor}, "low"},
		{[]string{modGroupSignalSubject}, "medium"},
		{[]string{modGroupSignalSubject, modGroupSignalFilenamePrefix}, "medium"},
		{[]string{modGroupSignalFolder}, "high"},
		{[]string{modGroupSignalSameFileName}, "medium"},
	}
	for _, testCase := range cases {
		if got := modGroupSuggestionConfidenceForSignals(testCase.signals); got != testCase.want {
			t.Fatalf("signals=%v 置信度 = %q, want %q", testCase.signals, got, testCase.want)
		}
	}
}

// 成员很多的粗糙聚类（例如 12 个成员只共享两个通用标签）比 3 个成员的小组
// 更难核实，排序时应让可操作的小组排在前面。
func TestSuggestModGroupsRanksActionableGroupsFirst(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	for index := 0; index < 12; index++ {
		seedCachedMod(a, filepath.Join(addonsDir, fmt.Sprintf("tag_%02d.vpk", index)), VPKFile{
			Name: fmt.Sprintf("tag_%02d.vpk", index), Title: fmt.Sprintf("标签条目 %02d", index),
			Location: "root", SecondaryTags: []string{"HUD", "UI"},
		})
	}
	for index := 0; index < 3; index++ {
		seedCachedMod(a, filepath.Join(addonsDir, fmt.Sprintf("fm_alpha_%d.vpk", index)), VPKFile{
			Name: fmt.Sprintf("fm_alpha_%d.vpk", index), Title: fmt.Sprintf("FM Alpha %d", index),
			Location: "root",
		})
	}
	for index := 0; index < 3; index++ {
		seedCachedMod(a, filepath.Join(addonsDir, fmt.Sprintf("z%d.vpk", index)), VPKFile{
			Name: fmt.Sprintf("z%d.vpk", index), Title: fmt.Sprintf("Zoey 模型 %d", index),
			Location: "root", SubjectSummary: "主体：Zoey 模型",
		})
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) < 3 {
		t.Fatalf("应给出三类建议: %#v", suggestions)
	}
	// 12 个成员、只有单一信号（共同标签）的粗糙聚类，必须排在
	// 3 个成员但来自内容证据（主体识别）的小组之后。
	broadIndex, contentIndex := -1, -1
	for index, item := range suggestions {
		if len(item.MemberKeys) == 12 {
			broadIndex = index
		}
		if len(item.MemberKeys) == 3 && strings.Contains(strings.Join(item.Signals, ","), modGroupSignalSubject) {
			if contentIndex == -1 {
				contentIndex = index
			}
		}
	}
	if broadIndex == -1 || contentIndex == -1 {
		t.Fatalf("缺少预期建议: %#v", suggestions)
	}
	if broadIndex < contentIndex {
		t.Fatalf("粗糙的大聚类不应排在精确小组之前: %#v", suggestions)
	}
}

// 建完组之后，同一批成员的建议必须沉到最后（而不是继续占据列表开头），
// 否则用户看不到后面的新建议。
func TestSuggestModGroupsDemotesAlreadyGroupedSuggestions(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	for index, name := range []string{"aaa.vpk", "bbb.vpk", "ccc.vpk"} {
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{
			Name: name, Title: fmt.Sprintf("Nick 语音 %d", index), Location: "root",
			SecondaryTags: []string{"语音", "人物"}, SubjectSummary: "主体：Nick 语音",
		})
	}
	seedCachedMod(a, filepath.Join(addonsDir, "zzz.vpk"), VPKFile{
		Name: "zzz.vpk", Title: "Ellis 语音甲", Location: "root", SubjectSummary: "主体：Ellis 语音",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "yyy.vpk"), VPKFile{
		Name: "yyy.vpk", Title: "Ellis 语音乙", Location: "root", SubjectSummary: "主体：Ellis 语音",
	})

	if _, err := a.CreateModStrategyGroupFromKeys("Nick 语音", "", modStrategyGroupSingle,
		[]string{"aaa.vpk", "bbb.vpk", "ccc.vpk"}); err != nil {
		t.Fatalf("create group: %v", err)
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) < 2 {
		t.Fatalf("应至少两条建议: %#v", suggestions)
	}
	last := suggestions[len(suggestions)-1]
	if last.ExistingGroupID == "" {
		t.Fatalf("已建组的建议应排在最后: %#v", suggestions)
	}
	if len(last.MemberKeys) != 3 {
		t.Fatalf("最后一条应是 3 个成员的已建组建议: %#v", last)
	}
}

// 用户把一整套 Mod 放进 addons 下的文件夹（例如
// addons\Airi初代恶堕战斗员八人\开关\*.vpk）时，文件夹结构本身就是最强的分组证据：
// 既要给出整套（含子目录）的建议，也要给出各个下级文件夹（开关、角色动画版…）的建议。
func TestSuggestModGroupsFromSharedFolderAndSubfolder(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	packDir := filepath.Join(addonsDir, "Airi初代恶堕战斗员八人")

	seedCachedMod(a, filepath.Join(packDir, "基础材质", "基础材质.vpk"), VPKFile{
		Name: "基础材质.vpk", Title: "Airi 基础材质", Location: "root",
	})
	seedCachedMod(a, filepath.Join(packDir, "开关", "臂甲.vpk"), VPKFile{
		Name: "臂甲.vpk", Title: "Airi 臂甲", Location: "root",
	})
	seedCachedMod(a, filepath.Join(packDir, "开关", "腿环.vpk"), VPKFile{
		Name: "腿环.vpk", Title: "Airi 腿环", Location: "root",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	byLabel := map[string]ModGroupSuggestion{}
	for _, item := range suggestions {
		byLabel[item.Label] = item
	}

	pack, ok := byLabel["Airi初代恶堕战斗员八人"]
	if !ok {
		t.Fatalf("应给出整套文件夹建议: %#v", suggestions)
	}
	if len(pack.MemberKeys) != 3 {
		t.Fatalf("整套应包含全部 3 个子目录成员: %#v", pack.MemberKeys)
	}
	if pack.Confidence != "high" || !strings.Contains(strings.Join(pack.Signals, ","), modGroupSignalFolder) {
		t.Fatalf("文件夹建议应为高置信度且带文件夹信号: %#v", pack)
	}

	sub, ok := byLabel["Airi初代恶堕战斗员八人 / 开关"]
	if !ok {
		t.Fatalf("应给出下级文件夹建议: %#v", byLabel)
	}
	if len(sub.MemberKeys) != 2 || sub.Score < pack.Score {
		t.Fatalf("下级文件夹应更精确（成员更少、分数不低于整套）: %#v", sub)
	}

	// 只有 1 个成员的目录不该单独成组（否则会淹没在单文件建议里）。
	if _, ok := byLabel["Airi初代恶堕战斗员八人 / 基础材质"]; ok {
		t.Fatalf("单成员目录不应给出建议: %#v", byLabel)
	}
}

// workshop / disabled 是"位置"而不是"套件"，不应该作为分组建议。
func TestSuggestModGroupsSkipsLocationFolders(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "111.vpk"), VPKFile{
		Name: "111.vpk", Title: "工坊条目一", Location: "workshop", WorkshopID: "111",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "222.vpk"), VPKFile{
		Name: "222.vpk", Title: "工坊条目二", Location: "workshop", WorkshopID: "222",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "disabled", "aaa.vpk"), VPKFile{
		Name: "aaa.vpk", Title: "禁用条目一", Location: "disabled",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "disabled", "bbb.vpk"), VPKFile{
		Name: "bbb.vpk", Title: "禁用条目二", Location: "disabled",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	for _, item := range suggestions {
		if strings.Contains(strings.Join(item.Signals, ","), modGroupSignalFolder) {
			t.Fatalf("workshop / disabled 不应产生文件夹建议: %#v", item)
		}
	}
}

// 不同套件里各自有一个同名文件只是重名，不能凑成一组。
func TestSuggestModGroupsSameFileNameRequiresSamePack(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "甲包", "目录A", "目镜.vpk"), VPKFile{
		Name: "目镜.vpk", Title: "甲包目镜", Location: "root",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "乙包", "目录B", "目镜.vpk"), VPKFile{
		Name: "目镜.vpk", Title: "乙包目镜", Location: "root",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	for _, item := range suggestions {
		if strings.Contains(strings.Join(item.Signals, ","), modGroupSignalSameFileName) {
			t.Fatalf("跨套件的同名文件不应产生建议: %#v", item)
		}
	}
}

// 同一 Mod 的不同版本常常分别放在"角色动画版 / 角色无动画"这类目录里，
// 文件名相同 ⇒ 它们覆盖同一批资源，天然适合做互斥单选组。
func TestSuggestModGroupsFromSameFileNameAcrossFolders(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	packDir := filepath.Join(addonsDir, "Airi初代恶堕战斗员八人")
	seedCachedMod(a, filepath.Join(packDir, "角色动画版", "bill.vpk"), VPKFile{
		Name: "bill.vpk", Title: "Airi Bill 动画版", Location: "root",
	})
	seedCachedMod(a, filepath.Join(packDir, "角色无动画", "bill.vpk"), VPKFile{
		Name: "bill.vpk", Title: "Airi Bill 静态版", Location: "root",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	var found *ModGroupSuggestion
	for index := range suggestions {
		signals := strings.Join(suggestions[index].Signals, ",")
		if strings.Contains(signals, modGroupSignalSameFileName) {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("应给出同名不同目录建议: %#v", suggestions)
	}
	// 这两个成员同时命中"同一文件夹"和"同名文件"两个信号，
	// 按成员集合合并成一条建议，并保留更强的文件夹证据（高置信度）。
	if len(found.MemberKeys) != 2 {
		t.Fatalf("同名版本建议应包含 2 个成员: %#v", found)
	}
	signals := strings.Join(found.Signals, ",")
	if !strings.Contains(signals, modGroupSignalSameFileName) || !strings.Contains(signals, modGroupSignalFolder) {
		t.Fatalf("合并后应同时列出两个信号: %#v", found.Signals)
	}
	if found.Confidence != "high" {
		t.Fatalf("同一批成员命中文件夹证据时按更强证据定级: %#v", found)
	}
}

// 文件夹是你亲自整理出来的结构，和工坊合集一样不因为成员多而降权。
func TestModGroupSuggestionFolderSignalsExemptFromSizePenalty(t *testing.T) {
	folder := ModGroupSuggestion{
		Confidence: "high", Score: modGroupSuggestionFolderScore,
		Signals: []string{modGroupSignalFolder}, MemberKeys: make([]string, 24),
	}
	got := modGroupSuggestionSortScore(folder)
	want := modGroupSuggestionConfidenceBonus("high") + modGroupSuggestionFolderScore
	if got != want {
		t.Fatalf("文件夹建议不应被规模降权: got %d, want %d", got, want)
	}
}

// addoninfo 的 addonauthor 常被填成占位符或角色列表（AUTHOR_NAME、
// Animal33/zmg/momo），这类"作者"不是有效的分组证据，必须过滤。
func TestSuggestModGroupsSkipsPlaceholderAuthors(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "aaa.vpk"), VPKFile{
		Name: "aaa.vpk", Title: "占位符甲", Location: "root", Author: "AUTHOR_NAME",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "bbb.vpk"), VPKFile{
		Name: "bbb.vpk", Title: "占位符乙", Location: "root", Author: "AUTHOR_NAME",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "ccc.vpk"), VPKFile{
		Name: "ccc.vpk", Title: "角色列表甲", Location: "root", Author: "Animal33/zmg/momo",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "ddd.vpk"), VPKFile{
		Name: "ddd.vpk", Title: "角色列表乙", Location: "root", Author: "Animal33/zmg/momo",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	for _, item := range suggestions {
		if strings.Contains(strings.Join(item.Signals, ","), modGroupSignalSameAuthor) {
			t.Fatalf("占位符 / 角色列表作者不应产生建议: %#v", item)
		}
	}
}

// 高产作者的 Mod 之间往往没有实际关系，"同一作者"只在规模较小时才有分组价值。
func TestSuggestModGroupsSkipsProlificAuthorBuckets(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	for index := 0; index < 13; index++ {
		name := fmt.Sprintf("prod_%02d.vpk", index)
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{
			Name: name, Title: fmt.Sprintf("高产作者作品 %02d", index), Location: "root", Author: "ProlificAuthor",
		})
	}
	for index := 0; index < 3; index++ {
		name := fmt.Sprintf("small_%d.vpk", index)
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{
			Name: name, Title: fmt.Sprintf("小作者作品 %d", index), Location: "root", Author: "SmallAuthor",
		})
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	foundSmall := false
	for _, item := range suggestions {
		signals := strings.Join(item.Signals, ",")
		if !strings.Contains(signals, modGroupSignalSameAuthor) {
			continue
		}
		if len(item.MemberKeys) > modGroupSuggestionMaxAuthorMembers {
			t.Fatalf("高产作者不应成组: %#v", item)
		}
		foundSmall = true
	}
	if !foundSmall {
		t.Fatalf("小规模作者建议应保留: %#v", suggestions)
	}
}

// 联名 / 多人署名（"A + B"、"甲，乙，丙"）不是"同一个人做的一组"。
func TestSuggestModGroupsAuthorSkipsMultiCreditNames(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	for index, name := range []string{"m1.vpk", "m2.vpk"} {
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{
			Name: name, Title: fmt.Sprintf("联名作品 %d", index), Location: "root",
			Author: "Dazzle_白麒麟 + All_calm",
		})
	}
	for index, name := range []string{"n1.vpk", "n2.vpk"} {
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{
			Name: name, Title: fmt.Sprintf("多人作品 %d", index), Location: "root",
			Author: "CY火余，六道花凛，MOMO",
		})
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	for _, item := range suggestions {
		if strings.Contains(strings.Join(item.Signals, ","), modGroupSignalSameAuthor) {
			t.Fatalf("联名作者不应产生建议: %#v", item)
		}
	}
}

// "武器是武器，角色是角色"：同一作者 / 同一文件名前缀也要按主分类分开，
// 不能把武器 Mod 和 HUD Mod 凑成一组。
func TestSuggestModGroupsKeepsPrimaryCategoriesSeparate(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "weapon_a.vpk"), VPKFile{
		Name: "weapon_a.vpk", Title: "系列武器甲", Location: "root",
		Author: "SameAuthor", PrimaryTag: "武器",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "weapon_b.vpk"), VPKFile{
		Name: "weapon_b.vpk", Title: "系列武器乙", Location: "root",
		Author: "SameAuthor", PrimaryTag: "武器",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "hud_c.vpk"), VPKFile{
		Name: "hud_c.vpk", Title: "系列HUD丙", Location: "root",
		Author: "SameAuthor", PrimaryTag: "其他",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	for _, item := range suggestions {
		if !strings.Contains(strings.Join(item.Signals, ","), modGroupSignalSameAuthor) {
			continue
		}
		// 作者桶里混了"武器"和"其他"，不允许整桶成组。
		if len(item.MemberKeys) >= 3 {
			t.Fatalf("跨主分类的作者建议不应输出: %#v", item)
		}
	}
}

// 语音替换是精确的内容证据：这些 Mod 都替换了同一个角色的语音。
func TestSuggestModGroupsFromVoiceCharacters(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "voice_a.vpk"), VPKFile{
		Name: "voice_a.vpk", Title: "Nick 语音增强", Location: "root",
		PrimaryTag: "人物", VoiceCharacters: []string{"Nick"},
	})
	seedCachedMod(a, filepath.Join(addonsDir, "voice_b.vpk"), VPKFile{
		Name: "voice_b.vpk", Title: "Nick 战斗语音", Location: "root",
		PrimaryTag: "人物", VoiceCharacters: []string{"Nick"},
	})
	seedCachedMod(a, filepath.Join(addonsDir, "voice_c.vpk"), VPKFile{
		Name: "voice_c.vpk", Title: "Zoey 语音", Location: "root",
		PrimaryTag: "人物", VoiceCharacters: []string{"Zoey"},
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	var found *ModGroupSuggestion
	for index := range suggestions {
		if strings.Contains(strings.Join(suggestions[index].Signals, ","), modGroupSignalVoice) {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("应给出语音角色建议: %#v", suggestions)
	}
	if len(found.MemberKeys) != 2 || found.Confidence != "medium" {
		t.Fatalf("语音角色建议应包含 2 个成员且为中置信度: %#v", found)
	}
	if !strings.Contains(found.Label, "Nick") {
		t.Fatalf("语音建议应带上角色名: %#v", found)
	}
}

// 解析器明确标注"低置信度"的主体（例如"脚本资源（无法确认具体对象）"）
// 不能作为分组依据，否则会生成一批名字无意义的分组。
func TestSuggestModGroupsSkipsLowConfidenceSubjects(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "low_a.vpk"), VPKFile{
		Name: "low_a.vpk", Title: "脚本资源甲", Location: "root",
		PrimaryTag: "其他", SubjectSummary: "主体：脚本资源（无法确认具体对象）", SubjectConfidence: "低",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "low_b.vpk"), VPKFile{
		Name: "low_b.vpk", Title: "脚本资源乙", Location: "root",
		PrimaryTag: "其他", SubjectSummary: "主体：脚本资源（无法确认具体对象）", SubjectConfidence: "低",
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	for _, item := range suggestions {
		if strings.Contains(strings.Join(item.Signals, ","), modGroupSignalSubject) {
			t.Fatalf("低置信度主体不应产生建议: %#v", item)
		}
	}
}

// 共同标签不能只看前两个标签：任意两个标签相同都应能成为候选，
// 否则大量"同套件但标签顺序不同"的 Mod 会被漏掉。
func TestSuggestModGroupsFromAnySharedTagPair(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "t1.vpk"), VPKFile{
		Name: "t1.vpk", Title: "AK47 甲乙一", Location: "root", PrimaryTag: "武器",
		SecondaryTags: []string{"AK47", "皮肤", "涂装"},
	})
	seedCachedMod(a, filepath.Join(addonsDir, "t2.vpk"), VPKFile{
		Name: "t2.vpk", Title: "AK47 甲乙二", Location: "root", PrimaryTag: "武器",
		SecondaryTags: []string{"皮肤", "AK47", "细节"},
	})
	seedCachedMod(a, filepath.Join(addonsDir, "t3.vpk"), VPKFile{
		Name: "t3.vpk", Title: "AK47 甲乙三", Location: "root", PrimaryTag: "武器",
		SecondaryTags: []string{"涂装", "AK47", "皮肤"},
	})

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	found := false
	for _, item := range suggestions {
		if strings.Contains(strings.Join(item.Signals, ","), modGroupSignalSharedTags) {
			found = true
			if len(item.MemberKeys) != 3 {
				t.Fatalf("共同标签建议应包含 3 个成员: %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("任意两标签相同的 Mod 应能成组: %#v", suggestions)
	}
}

// 主分类里可能混进解析脏值（"三角洲"、"Milfy"）：不能因为脏值不同就把同套件 Mod 拆开。
func TestGroupingBucketSharesPrimaryTagIgnoresUnknownValues(t *testing.T) {
	bucket := []grouping.Mod{
		{Name: "a.vpk", PrimaryTag: "武器"},
		{Name: "b.vpk", PrimaryTag: "三角洲"},
		{Name: "c.vpk", PrimaryTag: ""},
	}
	if !grouping.SharesPrimaryTag(bucket) {
		t.Fatal("非标准主分类应视为未知，而不是与标准分类冲突")
	}
	if grouping.SharesPrimaryTag([]grouping.Mod{{PrimaryTag: "武器"}, {PrimaryTag: "人物"}}) {
		t.Fatal("武器与人物应判定为不同主分类")
	}
}

// 排序分数是"可信度 + 规模"的显式配方，锁住它避免以后调参时互相打架。
func TestModGroupSuggestionSortScoreCalibration(t *testing.T) {
	// 置信度由信号目录判定：主体 / 共同标签属于内容证据（medium），
	// 文件名前缀属于弱启发式（low），工坊合集是用户维护结构（high）。
	subjectSmall := ModGroupSuggestion{Score: 50, Signals: []string{modGroupSignalSubject}, MemberKeys: make([]string, 3)}
	tagsBroad := ModGroupSuggestion{Score: 45, Signals: []string{modGroupSignalSharedTags}, MemberKeys: make([]string, 12)}
	prefixSmall := ModGroupSuggestion{Score: 45, Signals: []string{modGroupSignalFilenamePrefix}, MemberKeys: make([]string, 3)}
	collectionLarge := ModGroupSuggestion{Score: 80, Signals: []string{modGroupSignalCollection}, MemberKeys: make([]string, 30)}

	if got, want := modGroupSuggestionSortScore(subjectSmall), 70; got != want {
		t.Fatalf("3 个成员的主体小组排序分 = %d, want %d", got, want)
	}
	if got, want := modGroupSuggestionSortScore(tagsBroad), 49; got != want {
		t.Fatalf("12 个成员的通用标签聚类排序分 = %d, want %d", got, want)
	}
	if modGroupSuggestionSortScore(tagsBroad) >= modGroupSuggestionSortScore(subjectSmall) {
		t.Fatal("粗糙的大聚类不应排在精确小组之前")
	}
	// 内容证据（主体识别）必须排在弱启发式（文件名前缀）之前。
	if modGroupSuggestionSortScore(prefixSmall) >= modGroupSuggestionSortScore(subjectSmall) {
		t.Fatal("内容证据应排在弱启发式之前")
	}
	if got, want := modGroupSuggestionSortScore(collectionLarge), 120; got != want {
		t.Fatalf("工坊合集不因成员多而降权: %d, want %d", got, want)
	}
}

// 同为小组时，带有客观元数据（共同标签/同一作者/文件名前缀）的建议比只有
// 主体推断的建议更值得先看，即使主体推断那一组成员更少。
func TestSuggestModGroupsRanksObjectiveSignalsAboveSubjectOnly(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "aaa.vpk"), VPKFile{
		Name: "aaa.vpk", Title: "Ellis 语音甲", Location: "root", SubjectSummary: "主体：Ellis 语音",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "bbb.vpk"), VPKFile{
		Name: "bbb.vpk", Title: "Ellis 语音乙", Location: "root", SubjectSummary: "主体：Ellis 语音",
	})
	for index, name := range []string{"ccc.vpk", "ddd.vpk", "eee.vpk"} {
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{
			Name: name, Title: fmt.Sprintf("Nick 语音 %d", index), Location: "root",
			SecondaryTags: []string{"语音", "人物"}, SubjectSummary: "主体：Nick 语音",
		})
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) < 2 {
		t.Fatalf("应给出两条建议: %#v", suggestions)
	}
	// 多信号命中（共同标签 + 主体识别）的内容组比只有一个主体信号的组更可信。
	if len(suggestions[0].Signals) < 2 {
		t.Fatalf("多信号内容组应排在前面: %#v", suggestions[0])
	}
	for _, item := range suggestions {
		if item.Confidence == "low" {
			t.Fatalf("内容证据（主体识别）不应再被标为低置信度: %#v", item)
		}
	}
}
