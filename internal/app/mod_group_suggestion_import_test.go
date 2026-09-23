package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSuggestionFile(t *testing.T, dir string, payload map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, "suggestions.json")
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func suggestionsPayload(suggestions ...map[string]any) map[string]any {
	list := make([]any, 0, len(suggestions))
	for _, item := range suggestions {
		list = append(list, item)
	}
	return map[string]any{
		"version":     1,
		"generator":   "codex",
		"suggestions": list,
	}
}

// 外部建议文件可以用 addonlist 键、裸文件名或绝对路径三种写法描述成员。
func TestImportGroupSuggestionsResolvesKeysNamesAndPaths(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Title: "甲", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Title: "乙", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "c.vpk"), VPKFile{Name: "c.vpk", Title: "丙", Location: "root"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":      "模型三件套",
		"reason":     "同一角色的模型替换",
		"confidence": "high",
		"members": []string{
			"a.vpk",
			"b.vpk",
			filepath.Join(addonsDir, "c.vpk"),
		},
	}))

	result, err := a.ImportGroupSuggestionsFromFile(source)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 || result.MemberCount != 3 {
		t.Fatalf("导入结果异常: %#v", result)
	}

	suggestions, err := a.GetExternalGroupSuggestions()
	if err != nil {
		t.Fatalf("read inbox: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("应读取到 1 条建议: %#v", suggestions)
	}
	got := suggestions[0]
	if got.Label != "模型三件套" || got.Source != modGroupSuggestionSourceExternal || got.Confidence != "high" {
		t.Fatalf("建议结构异常: %#v", got)
	}
	if len(got.MemberKeys) != 3 || got.MemberKeys[0] != "a.vpk" || got.MemberKeys[2] != "c.vpk" {
		t.Fatalf("成员键解析异常: %#v", got.MemberKeys)
	}
	if len(got.Signals) == 0 || got.Signals[0] != modGroupSignalExternal {
		t.Fatalf("外部建议必须带外部信号: %#v", got.Signals)
	}
}

// 匹配不到的成员只记警告，不阻断其它成员；整条都匹配不上时跳过并说明原因。
func TestImportGroupSuggestionsReportsUnmatchedMembers(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Title: "甲", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Title: "乙", Location: "root"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(
		map[string]any{
			"label":   "部分匹配",
			"members": []string{"a.vpk", "b.vpk", "不存在的.vpk"},
		},
		map[string]any{
			"label":   "全部不匹配",
			"members": []string{"缺失甲.vpk", "缺失乙.vpk"},
		},
	))

	result, err := a.ImportGroupSuggestionsFromFile(source)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.Imported != 1 || result.Skipped != 1 {
		t.Fatalf("应导入 1 条、跳过 1 条: %#v", result)
	}
	warnings := joinWarnings(result.Warnings)
	if !containsAll(warnings, "未在当前列表里", "有效成员不足") {
		t.Fatalf("警告内容不完整: %#v", result.Warnings)
	}
	if len(result.Warnings) == 0 || result.Total != 2 {
		t.Fatalf("统计字段异常: %#v", result)
	}
}

// 裸文件名同时命中根目录与 workshop 时，两个位置都会被纳入，并给出提示。
func TestImportGroupSuggestionsAmbiguousNameIncludesAllMatches(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "123.vpk"), VPKFile{Name: "123.vpk", Title: "根目录副本", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "123.vpk"), VPKFile{Name: "123.vpk", Title: "工坊原件", Location: "workshop"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Title: "乙", Location: "root"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":   "重复副本",
		"members": []string{"123.vpk", "b.vpk"},
	}))

	result, err := a.ImportGroupSuggestionsFromFile(source)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.MemberCount != 3 {
		t.Fatalf("裸文件名应命中两个位置: %#v", result)
	}
	if warnings := joinWarnings(result.Warnings); !containsAll(warnings, "命中多个位置") {
		t.Fatalf("应提示命中多处: %#v", result.Warnings)
	}
}

func TestImportGroupSuggestionsRejectsInvalidFiles(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	dir := t.TempDir()

	badVersion := writeSuggestionFile(t, dir, map[string]any{
		"version":     99,
		"suggestions": []any{map[string]any{"label": "x", "members": []string{"a.vpk"}}},
	})
	if _, err := a.ImportGroupSuggestionsFromFile(badVersion); err == nil {
		t.Fatal("不支持的版本应被拒绝")
	}

	empty := writeSuggestionFile(t, dir, map[string]any{"version": 1, "suggestions": []any{}})
	if _, err := a.ImportGroupSuggestionsFromFile(empty); err == nil {
		t.Fatal("空建议列表应被拒绝")
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ImportGroupSuggestionsFromFile(broken); err == nil {
		t.Fatal("非法 JSON 应被拒绝")
	}
}

// 导入建议的展示顺序应保留文件里的编排，并且不因为超过 40 条而被截断。
func TestSuggestModGroupsPreservesExternalOrderAndShowsAll(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	names := []string{"e1.vpk", "e2.vpk", "e3.vpk", "e4.vpk", "e5.vpk", "e6.vpk"}
	for _, name := range names {
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{
			Name: name, Title: name, Location: "root", PrimaryTag: "武器",
			SubjectSummary: "主体：M16 武器", SubjectConfidence: "高",
		})
	}
	// 文件顺序故意与"成员数"顺序相反：3 成员的排在 2 成员的前面。
	payload := suggestionsPayload(
		map[string]any{"label": "文件第一组", "members": []string{"e1.vpk", "e2.vpk", "e3.vpk"}},
		map[string]any{"label": "文件第二组", "members": []string{"e4.vpk", "e5.vpk"}},
		map[string]any{"label": "文件第三组", "members": []string{"e5.vpk", "e6.vpk"}},
	)
	source := writeSuggestionFile(t, t.TempDir(), payload)
	if _, err := a.ImportGroupSuggestionsFromFile(source); err != nil {
		t.Fatalf("import: %v", err)
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	external := make([]ModGroupSuggestion, 0, len(suggestions))
	for _, item := range suggestions {
		if item.Source == modGroupSuggestionSourceExternal {
			external = append(external, item)
		}
	}
	if len(external) != 3 {
		t.Fatalf("应展示全部 3 条导入建议: %#v", external)
	}
	want := []string{"文件第一组", "文件第二组", "文件第三组"}
	for index, item := range external {
		if item.Label != want[index] {
			t.Fatalf("导入顺序被改变: %#v", []string{external[0].Label, external[1].Label, external[2].Label})
		}
	}
}

// 导入后的建议必须出现在分组建议列表最前面，并带上来源标记。
func TestSuggestModGroupsIncludesExternalSuggestionsFirst(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Title: "甲", Location: "root", Author: "作者"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Title: "乙", Location: "root", Author: "作者"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":      "模型看过之后写下的组",
		"reason":     "两者替换同一资源",
		"confidence": "high",
		"members":    []string{"a.vpk", "b.vpk"},
	}))
	if _, err := a.ImportGroupSuggestionsFromFile(source); err != nil {
		t.Fatalf("import: %v", err)
	}

	suggestions, err := a.SuggestModGroups()
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatal("应至少返回一条建议")
	}
	if suggestions[0].Source != modGroupSuggestionSourceExternal || suggestions[0].Label != "模型看过之后写下的组" {
		t.Fatalf("外部建议应排在最前面: %#v", suggestions[0])
	}
}

func TestExportGroupingCatalogWritesModMetadata(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{
		Name: "a.vpk", Title: "甲", Location: "root", Author: "作者甲",
		PrimaryTag: "武器", SecondaryTags: []string{"AK47", "皮肤"},
		SubjectSummary: "主体：AK47 武器", SubjectConfidence: "高",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "123.vpk"), VPKFile{
		Name: "123.vpk", Title: "工坊条目", Location: "workshop", WorkshopID: "123",
		VoiceCharacters: []string{"Nick"}, PrimaryTag: "人物",
	})

	target := filepath.Join(t.TempDir(), "catalog.json")
	path, err := a.ExportGroupingCatalog(target)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if path != target {
		t.Fatalf("导出路径 = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload groupingCatalogFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("解析导出文件失败: %v", err)
	}
	if payload.ModCount != 2 || len(payload.Mods) != 2 {
		t.Fatalf("导出条目数异常: %#v", payload)
	}
	byName := map[string]groupingCatalogEntry{}
	for _, entry := range payload.Mods {
		byName[entry.Name] = entry
	}
	weapon, ok := byName["a.vpk"]
	if !ok {
		t.Fatalf("缺少 a.vpk: %#v", payload.Mods)
	}
	if weapon.PrimaryTag != "武器" || weapon.SubjectConfidence != "高" || len(weapon.SecondaryTags) != 2 {
		t.Fatalf("导出字段不完整: %#v", weapon)
	}
	workshop := byName["123.vpk"]
	if workshop.WorkshopID != "123" || len(workshop.VoiceCharacters) != 1 {
		t.Fatalf("工坊条目字段不完整: %#v", workshop)
	}
}

// 清单要带上 VPK 内部结构（顶层目录 / 条目数 / 代表性路径）与位置、开关、优先级，
// 让外部智能体不需要自己打开 VPK。
func TestExportGroupingCatalogIncludesStructureAndState(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	packPath := filepath.Join(addonsDir, "structure.vpk")
	writeTestVPK(t, packPath, map[string][]byte{
		"models/v_models/v_rifle.mdl":     []byte("model"),
		"materials/models/v_rifle/00.vtf": []byte("texture"),
		"scripts/vscripts/weapon.nut":     []byte("script"),
		"sound/weapons/rifle/fire.wav":    []byte("sound"),
		"addoninfo.txt":                   []byte("\"addontitle\" \"结构测试\"\n"),
	})
	seedCachedMod(a, packPath, VPKFile{Name: "structure.vpk", Title: "结构测试", Location: "root"})
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	target := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := a.ExportGroupingCatalog(target); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var payload groupingCatalogFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Notes == "" || !strings.Contains(payload.Notes, "无需自行打开 VPK") {
		t.Fatalf("清单缺少说明: %q", payload.Notes)
	}

	var entry *groupingCatalogEntry
	for index := range payload.Mods {
		if payload.Mods[index].Name == "structure.vpk" {
			entry = &payload.Mods[index]
			break
		}
	}
	if entry == nil {
		t.Fatalf("清单里没有 structure.vpk: %#v", payload.Mods)
	}
	if entry.Structure == nil {
		t.Fatalf("缺少结构摘要: %#v", entry)
	}
	if entry.Structure.FileCount < 4 {
		t.Fatalf("结构条目数异常: %#v", entry.Structure)
	}
	if entry.Structure.TotalSize <= 0 {
		t.Fatalf("结构体积异常: %#v", entry.Structure)
	}
	joined := strings.Join(entry.Structure.TopDirs, ",")
	for _, dir := range []string{"models(", "materials(", "scripts(", "sound("} {
		if !strings.Contains(joined, dir) {
			t.Fatalf("顶层目录缺少 %s: %#v", dir, entry.Structure.TopDirs)
		}
	}
	paths := strings.Join(entry.Structure.SamplePaths, ",")
	if !strings.Contains(paths, "models/v_models/v_rifle.mdl") {
		t.Fatalf("代表性路径缺少模型条目: %#v", entry.Structure.SamplePaths)
	}
	if strings.Contains(paths, "addoninfo.txt") {
		t.Fatalf("代表性路径不应包含 addoninfo: %#v", entry.Structure.SamplePaths)
	}
	if entry.Location != "root" || entry.Size <= 0 {
		t.Fatalf("位置 / 体积字段异常: %#v", entry)
	}
	if len(entry.Structure.Targets) == 0 {
		t.Fatalf("缺少压缩后的替换目标: %#v", entry.Structure)
	}
	targets := strings.Join(entry.Structure.Targets, ",")
	if !strings.Contains(targets, "props_interiors") && !strings.Contains(targets, "rifle") {
		t.Fatalf("替换目标不合理: %#v", entry.Structure.Targets)
	}
	if entry.Structure.ResourceRoots != nil {
		t.Fatalf("该夹具不应有套件命名空间: %#v", entry.Structure.ResourceRoots)
	}
}

// 套件命名空间（l4n 的 airi 包、武器贴图/参数包这种）要一路走到清单里：
// 解析 VPK → 缓存 → grouping 输入 → 清单 structure.resourceRoots。
func TestExportGroupingCatalogIncludesSuiteResourceRoots(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	packPath := filepath.Join(addonsDir, "suite.vpk")
	writeTestVPK(t, packPath, map[string][]byte{
		"materials/models/913limod/airi_evilfall/swtich/lightmainarmglove.vmt": []byte("m"),
		"materials/models/913limod/airi_evilfall/swtich/metalmainarmglove.vmt": []byte("m"),
		"models/survivors/survivor_coach.mdl":                                  []byte("x"),
	})
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	target := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := a.ExportGroupingCatalog(target); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var payload groupingCatalogFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	var entry *groupingCatalogEntry
	for index := range payload.Mods {
		if payload.Mods[index].Name == "suite.vpk" {
			entry = &payload.Mods[index]
			break
		}
	}
	if entry == nil || entry.Structure == nil {
		t.Fatalf("清单里没有 suite.vpk 的结构摘要: %#v", payload.Mods)
	}
	if !containsString(entry.Structure.ResourceRoots, "913limod/airi_evilfall") {
		t.Fatalf("清单必须带上套件命名空间: %#v", entry.Structure.ResourceRoots)
	}
}

// 清单要带上工坊资料（.meta）、本地管理记录（策略组 / 方案 / 依赖 / 忽略 / 合集）与
// addoninfo 字段，这样智能体看到的就是 LytVPK 已经解析好的全部信息。
func TestExportGroupingCatalogIncludesWorkshopAndManagement(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	workshopPath := filepath.Join(addonsDir, "workshop", "2985551390.vpk")
	seedCachedMod(a, workshopPath, VPKFile{
		Name: "2985551390.vpk", Title: "Nick 语音增强", Location: "workshop", WorkshopID: "2985551390",
		PrimaryTag: "人物", SecondaryTags: []string{"Nick", "语音"},
		SubjectSummary: "主体：Nick 语音", SubjectConfidence: "高",
		VoiceCharacters: []string{"Nick"}, Version: "1.2", Desc: "替换 Nick 语音", AddonURL0: "https://example.invalid/nick",
	})
	rootPath := filepath.Join(addonsDir, "ak47.vpk")
	seedCachedMod(a, rootPath, VPKFile{Name: "ak47.vpk", Title: "AK47 皮肤", Location: "root", PrimaryTag: "武器"})

	// 工坊 .meta（模拟 LytVPK 下载时写下的资料）。
	meta := WorkshopMeta{
		WorkshopID: "2985551390", Title: "Nick Voice Overhaul", Author: "某作者",
		Description: "Replaces Nick voice lines", PreviewURL: "https://example.invalid/preview.jpg",
		Tags: []string{"Voice", "Character"}, TimeUpdated: "2026-09-01T00:00:00Z",
	}
	if err := saveWorkshopMeta(workshopPath, &meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}
	// 稍后再看统计。
	if err := a.SaveWorkshopWatchLaterStorage(WorkshopWatchLaterStorage{Items: []WorkshopWatchLaterItem{{
		PublishedFileID: "2985551390", Title: "Nick Voice Overhaul", Views: 12345, Subscriptions: 678, Favorited: 90, FileType: 0,
	}}}); err != nil {
		t.Fatalf("watch later: %v", err)
	}
	// 策略组 + 依赖 + 忽略清单 + 工坊合集。
	if _, err := a.CreateModStrategyGroupFromKeys("语音组", "", modStrategyGroupSingle,
		[]string{"workshop\\2985551390.vpk"}); err != nil {
		t.Fatalf("group: %v", err)
	}
	if _, err := a.SetModDependencies(rootPath, []string{workshopPath}); err != nil {
		t.Fatalf("dependencies: %v", err)
	}
	if _, err := a.SetModIgnoreFiles("ak47.vpk", "AK47 皮肤", []string{"materials/shared.vtf"}); err != nil {
		t.Fatalf("ignore: %v", err)
	}
	if err := a.writeWorkshopCollectionStore(workshopCollectionStore{Links: []WorkshopCollectionLink{{
		ID: "link-1", CollectionID: "999", Title: "整合作者包",
		Members: []WorkshopCollectionMember{{WorkshopID: "2985551390", Title: "Nick 语音增强"}},
	}}}); err != nil {
		t.Fatalf("collection: %v", err)
	}

	target := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := a.ExportGroupingCatalog(target); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var payload groupingCatalogFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]groupingCatalogEntry{}
	for _, entry := range payload.Mods {
		byKey[entry.Key] = entry
	}

	voice, ok := byKey["workshop\\2985551390.vpk"]
	if !ok {
		t.Fatalf("缺少工坊条目: %#v", byKey)
	}
	if voice.Workshop == nil {
		t.Fatalf("缺少工坊资料: %#v", voice)
	}
	if voice.Workshop.Title != "Nick Voice Overhaul" || voice.Workshop.Author != "某作者" {
		t.Fatalf("工坊标题 / 作者未导出: %#v", voice.Workshop)
	}
	if len(voice.Workshop.Tags) != 2 || !voice.Workshop.InWatchLater || voice.Workshop.Views != 12345 {
		t.Fatalf("工坊标签 / 稍后再看统计未导出: %#v", voice.Workshop)
	}
	if !strings.Contains(voice.Workshop.URL, "2985551390") {
		t.Fatalf("缺少工坊详情页链接: %#v", voice.Workshop)
	}
	if voice.AddonInfo == nil || voice.AddonInfo.Version != "1.2" || voice.AddonInfo.Desc == "" {
		t.Fatalf("addoninfo 字段未导出: %#v", voice.AddonInfo)
	}
	if voice.Management == nil || len(voice.Management.Groups) == 0 || len(voice.Management.Collections) == 0 {
		t.Fatalf("管理记录未导出: %#v", voice.Management)
	}

	weapon, ok := byKey["ak47.vpk"]
	if !ok || weapon.Management == nil {
		t.Fatalf("缺少根目录条目的管理记录: %#v", byKey)
	}
	if len(weapon.Management.Dependencies) == 0 || len(weapon.Management.IgnoredFiles) == 0 {
		t.Fatalf("依赖 / 忽略清单未导出: %#v", weapon.Management)
	}
}

// dry-run 校验：逐成员解析结果 + 全部警告，供外部智能体脱离 GUI 迭代。
func TestValidateGroupSuggestionsFileReportsPerMemberResults(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "123.vpk"), VPKFile{Name: "123.vpk", Title: "根目录副本", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "disabled", "123.vpk"), VPKFile{Name: "123.vpk", Title: "禁用副本", Location: "disabled"})
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "456.vpk"), VPKFile{Name: "456.vpk", Title: "工坊条目", Location: "workshop", WorkshopID: "456"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Title: "乙", Location: "root"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(
		map[string]any{
			"label":   "entryId 寻址",
			"members": []string{"disabled/123.vpk", "workshop/456.vpk"},
		},
		map[string]any{
			"label":   "含缺失成员",
			"members": []string{"b.vpk", "不存在的.vpk"},
		},
	))

	validation, err := a.ValidateGroupSuggestionsFile(source)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if validation.Total != 2 || validation.Valid != 1 || validation.Invalid != 1 {
		t.Fatalf("统计异常: %#v", validation)
	}
	first := validation.Items[0]
	if !first.Valid || first.MemberCount != 2 {
		t.Fatalf("entryId 寻址应校验通过: %#v", first)
	}
	if first.Members[0].Resolved != "123.vpk" || !first.Members[0].Matched {
		t.Fatalf("disabled 副本未解析: %#v", first.Members[0])
	}
	second := validation.Items[1]
	if second.Valid || len(second.Problems) == 0 {
		t.Fatalf("缺失成员应判为无效: %#v", second)
	}
	if len(validation.Warnings) == 0 || !containsAll(joinWarnings(validation.Warnings), "不存在的.vpk") {
		t.Fatalf("应给出未匹配成员警告: %#v", validation.Warnings)
	}
}

// 同一文件在根目录与 disabled 各有一份时，用键引用不应报歧义（反馈 P0-2）。
func TestResolveDoesNotWarnForSameKeyAcrossLocations(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "3773210949.vpk"), VPKFile{Name: "3773210949.vpk", Title: "根目录", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "disabled", "3773210949.vpk"), VPKFile{Name: "3773210949.vpk", Title: "禁用区", Location: "disabled"})

	validation, err := a.ValidateGroupSuggestionsFile(writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":   "同键多位置",
		"members": []string{"3773210949.vpk"},
	})))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(validation.Warnings) != 0 {
		t.Fatalf("同键多位置不应产生警告: %#v", validation.Warnings)
	}
}

// 反馈第二轮：hints 必须能用 entryId 精确寻址，且不允许出现重复条目。
func TestExportGroupingCatalogHintsUseEntryIdAndAreUnique(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// 同一文件在根目录与 disabled 各一份（共用同一个 key）。
	seedCachedMod(a, filepath.Join(addonsDir, "dup.vpk"), VPKFile{
		Name: "dup.vpk", Title: "重复甲", Location: "root", PrimaryTag: "武器",
		SubjectSummary: "主体：M16 武器", SubjectConfidence: "高",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "disabled", "dup.vpk"), VPKFile{
		Name: "dup.vpk", Title: "重复甲", Location: "disabled", PrimaryTag: "武器",
		SubjectSummary: "主体：M16 武器", SubjectConfidence: "高",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "other.vpk"), VPKFile{
		Name: "other.vpk", Title: "重复乙", Location: "root", PrimaryTag: "武器",
		SubjectSummary: "主体：M16 武器", SubjectConfidence: "高",
	})

	target := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := a.ExportGroupingCatalog(target); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var payload groupingCatalogFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}

	// hints 不得重复（曾经因为配额裁剪残留导致同一批成员重复几百次）。
	seen := map[string]struct{}{}
	for _, hint := range payload.ClusterHints {
		signature := hint.Label + "|" + strings.Join(hint.MemberEntryIDs, ",")
		if _, ok := seen[signature]; ok {
			t.Fatalf("clusterHints 出现重复条目: %#v", hint)
		}
		seen[signature] = struct{}{}
		if len(hint.MemberEntryIDs) != len(hint.MemberKeys) {
			t.Fatalf("entryIds 与 keys 数量不一致: %#v", hint)
		}
	}

	// duplicateGroups：同一对副本只能出现一次，并且带 entryIds。
	duplicateSignatures := map[string]int{}
	for _, group := range payload.DuplicateGroups {
		if len(group.EntryIDs) == 0 {
			t.Fatalf("duplicateGroups 缺少 entryIds: %#v", group)
		}
		duplicateSignatures[strings.Join(group.EntryIDs, "|")]++
	}
	for signature, count := range duplicateSignatures {
		if count > 1 {
			t.Fatalf("duplicateGroups 重复输出同一组: %s x%d", signature, count)
		}
	}

	// themeHints（若有）同样使用 entryIds。
	for _, hint := range payload.ThemeHints {
		if len(hint.MemberEntryIDs) == 0 {
			t.Fatalf("themeHints 缺少 entryIds: %#v", hint)
		}
	}
}

// 清单要给出紧凑的 clusterHints（内置推导的候选簇），
// 智能体可以只读这一小段就开始工作。
func TestExportGroupingCatalogIncludesClusterHints(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "hint_a.vpk"), VPKFile{
		Name: "hint_a.vpk", Title: "M16 甲", Location: "root",
		PrimaryTag: "武器", SubjectSummary: "主体：M16 武器", SubjectConfidence: "高",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "hint_b.vpk"), VPKFile{
		Name: "hint_b.vpk", Title: "M16 乙", Location: "root",
		PrimaryTag: "武器", SubjectSummary: "主体：M16 武器", SubjectConfidence: "高",
	})

	target := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := a.ExportGroupingCatalog(target); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var payload groupingCatalogFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.ClusterHints) == 0 {
		t.Fatalf("缺少 clusterHints: %#v", payload)
	}
	hint := payload.ClusterHints[0]
	if hint.Label == "" || len(hint.MemberKeys) < 2 || hint.Confidence == "" {
		t.Fatalf("簇信息不完整: %#v", hint)
	}
	if !strings.Contains(payload.Notes, "clusterHints") {
		t.Fatalf("notes 未提示如何使用 clusterHints: %q", payload.Notes)
	}
	if payload.Coverage == nil || payload.Coverage.Mods != 2 || payload.Coverage.WithSubject != 2 {
		t.Fatalf("覆盖度统计异常: %#v", payload.Coverage)
	}
	if payload.Coverage.ClusterHints == 0 {
		t.Fatalf("覆盖度未统计簇数量: %#v", payload.Coverage)
	}
}

// 同名不同位置的副本应被预计算进 duplicateGroups。
func TestExportGroupingCatalogPrecomputesDuplicateGroups(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "123.vpk"), VPKFile{Name: "123.vpk", Title: "根目录副本", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "123.vpk"), VPKFile{Name: "123.vpk", Title: "工坊原件", Location: "workshop"})
	seedCachedMod(a, filepath.Join(addonsDir, "other.vpk"), VPKFile{Name: "other.vpk", Title: "无关 Mod", Location: "root"})

	target := filepath.Join(t.TempDir(), "catalog.json")
	if _, err := a.ExportGroupingCatalog(target); err != nil {
		t.Fatalf("export: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var payload groupingCatalogFile
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, group := range payload.DuplicateGroups {
		if len(group.Keys) == 2 && group.Keys[0] == "123.vpk" && group.Keys[1] == "workshop\\123.vpk" {
			found = true
		}
	}
	if !found {
		t.Fatalf("同名副本未被预计算: %#v", payload.DuplicateGroups)
	}
}

func TestClearExternalGroupSuggestionsRemovesInbox(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Title: "甲", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Title: "乙", Location: "root"})

	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label": "要清空的组", "members": []string{"a.vpk", "b.vpk"},
	}))
	if _, err := a.ImportGroupSuggestionsFromFile(source); err != nil {
		t.Fatal(err)
	}
	if err := a.ClearExternalGroupSuggestions(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	suggestions, err := a.GetExternalGroupSuggestions()
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("清空后不应还有外部建议: %#v", suggestions)
	}
}

func joinWarnings(warnings []string) string {
	result := ""
	for _, warning := range warnings {
		result += warning + "\n"
	}
	return result
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
