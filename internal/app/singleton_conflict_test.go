package app

import (
	"reflect"
	"testing"
)

func cacheFile(file VPKFile) *VPKFileCache {
	return &VPKFileCache{File: file}
}

// 角色/具体武器型号算互斥；类别标签（步枪 / 霰弹枪）与物品不算——宁可不报，也不要把整类枪算成互斥。
func TestCheckModEnableConflictOnlyUsesSingletonTags(t *testing.T) {
	app := &App{}
	app.vpkCache.Store("z1.vpk", cacheFile(VPKFile{
		Path: "z1.vpk", Name: "z1.vpk", Enabled: true, GameEnabled: true,
		SecondaryTags: []string{"Zoey", "人物", "贴图"},
	}))
	app.vpkCache.Store("z2.vpk", cacheFile(VPKFile{
		Path: "z2.vpk", Name: "z2.vpk", Enabled: true, GameEnabled: true,
		SecondaryTags: []string{"Zoey", "步枪"},
	}))
	app.vpkCache.Store("rifle.vpk", cacheFile(VPKFile{
		Path: "rifle.vpk", Name: "rifle.vpk", Enabled: true, GameEnabled: true,
		SecondaryTags: []string{"步枪", "贴图"},
	}))
	app.vpkCache.Store("m16.vpk", cacheFile(VPKFile{
		Path: "m16.vpk", Name: "m16.vpk", Enabled: true, GameEnabled: true,
		SecondaryTags: []string{"M16", "步枪"},
	}))
	app.vpkCache.Store("ak.vpk", cacheFile(VPKFile{
		Path: "ak.vpk", Name: "ak.vpk", Enabled: true, GameEnabled: false,
		SecondaryTags: []string{"AK47"},
	}))
	app.vpkCache.Store("disabled-zoey.vpk", cacheFile(VPKFile{
		Path: "disabled-zoey.vpk", Name: "disabled-zoey.vpk", Enabled: false, GameEnabled: false,
		SecondaryTags: []string{"Zoey"},
	}))

	conflict, err := app.CheckModEnableConflict("z1.vpk")
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if conflict.Key != "人物:Zoey" {
		t.Fatalf("互斥键 = %q，期望 人物:Zoey", conflict.Key)
	}
	if len(conflict.Conflicts) != 1 || conflict.Conflicts[0].Path != "z2.vpk" {
		t.Fatalf("只该报告游戏内已启用的同类：%#v", conflict.Conflicts)
	}
	if label := SingletonConflictLabel(conflict.Key); label == "" || label == conflict.Key {
		t.Fatalf("互斥键应能转成人话，实际 %q", label)
	}

	// 类别标签（步枪）不构成互斥：只有"步枪"的包没有任何互斥分类。
	classOnly, err := app.CheckModEnableConflict("rifle.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if len(classOnly.Conflicts) != 0 {
		t.Fatalf("只有类别标签的包不该报互斥：%#v", classOnly)
	}

	// 不同型号不互斥：M16 包不该和 AK47 包算同一类。
	m16Conflict, err := app.CheckModEnableConflict("m16.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if len(m16Conflict.Conflicts) != 0 {
		t.Fatalf("不同武器型号不该互斥：%#v", m16Conflict)
	}

	// 游戏内关闭的同类不算互斥（它本来就不生效）。
	app.vpkCache.Store("ak2.vpk", cacheFile(VPKFile{
		Path: "ak2.vpk", Name: "ak2.vpk", Enabled: true, GameEnabled: true, SecondaryTags: []string{"AK47"},
	}))
	akConflict, err := app.CheckModEnableConflict("ak.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if akConflict.Key != "武器:AK47" || len(akConflict.Conflicts) != 1 || akConflict.Conflicts[0].Path != "ak2.vpk" {
		t.Fatalf("武器互斥判定不对：%#v", akConflict)
	}
}

func TestSingletonTagsForMatchesCuratedLists(t *testing.T) {
	got := singletonTagsFor(VPKFile{SecondaryTags: []string{"Zoey", "AK47", "步枪", "物品"}})
	want := []string{"人物:Zoey", "武器:AK47"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("singletonTagsFor = %#v，期望 %#v", got, want)
	}
	if tags := singletonTagsFor(VPKFile{SecondaryTags: []string{"步枪", "贴图"}}); len(tags) != 0 {
		t.Fatalf("类别/内容标签不应产生互斥键：%#v", tags)
	}
}

// classifySingletonOverlap：文件级定级（复用冲突分析口径）。
func TestClassifySingletonOverlap(t *testing.T) {
	cases := []struct {
		name         string
		target       []string
		other        []string
		wantSeverity string
		wantCount    int
	}{
		{"无重叠", []string{"models/a.mdl"}, []string{"models/b.mdl"}, "normal", 0},
		{
			"仅 HUD 图标级重叠（普通）",
			[]string{"models/w_m60.mdl", "materials/w_m60.vmt", "materials/w_m60.vtf", "scripts/weapon_m60.txt", "materials/vgui/hud/icon_m60.vtf"},
			[]string{"materials/vgui/hud/icon_m60.vtf"},
			"normal", 1,
		},
		{
			"同尺寸替换互撕（严重）",
			[]string{"models/w_m60.mdl", "materials/a.vmt", "materials/b.vmt", "materials/c.vmt"},
			[]string{"models/w_m60.mdl", "materials/a.vmt", "materials/b.vmt"},
			"severe", 3,
		},
		{
			"含 critical 文件（严重）",
			[]string{"scripts/weapon_m60.txt", "models/w_m60.mdl"},
			[]string{"scripts/weapon_m60.txt"},
			"severe", 1,
		},
		{
			"忽略文件不计（addoninfo）",
			[]string{"addoninfo.txt", "models/a.mdl"},
			[]string{"addoninfo.txt"},
			"normal", 0,
		},
		{
			"小体量包被整体覆盖（严重）",
			[]string{"models/x.mdl", "models/x.vvd"},
			[]string{"models/x.mdl", "models/x.vvd"},
			"severe", 2,
		},
	}
	for _, tc := range cases {
		severity, count, _ := classifySingletonOverlap(tc.target, tc.other)
		if severity != tc.wantSeverity || count != tc.wantCount {
			t.Errorf("%s: got %s/%d，期望 %s/%d", tc.name, severity, count, tc.wantSeverity, tc.wantCount)
		}
	}

	// 路径归一化（大小写 / 反斜杠）。
	severity, count, _ := classifySingletonOverlap([]string{"Models\\M60.MDL"}, []string{"models/m60.mdl"})
	if severity != "severe" || count != 1 {
		t.Fatalf("归一化后应整体覆盖（严重/1），实际 %s/%d", severity, count)
	}
}

// CheckModEnableConflict：逐条定级 + 严重在前排序（注入假文件清单，不解析真实 VPK）。
func TestCheckModEnableConflictAnnotatesSeverity(t *testing.T) {
	app := &App{}
	app.vpkCache.Store("m60a.vpk", cacheFile(VPKFile{
		Path: "m60a.vpk", Name: "m60a.vpk", Enabled: true, GameEnabled: true,
		SecondaryTags: []string{"M60"},
	}))
	app.vpkCache.Store("m60b.vpk", cacheFile(VPKFile{
		Path: "m60b.vpk", Name: "m60b.vpk", Enabled: true, GameEnabled: true,
		SecondaryTags: []string{"M60", "贴图"},
	}))
	app.vpkCache.Store("m60hud.vpk", cacheFile(VPKFile{
		Path: "m60hud.vpk", Name: "m60hud.vpk", Enabled: true, GameEnabled: true,
		SecondaryTags: []string{"M60", "HUD"},
	}))

	previous := singletonOverlapFileList
	singletonOverlapFileList = func(_ *App, path string) ([]string, error) {
		switch path {
		case "m60a.vpk":
			return []string{"models/w_m60.mdl", "materials/w_m60.vmt", "materials/w_m60.vtf", "scripts/weapon_m60.txt", "materials/vgui/hud/icon_m60.vtf"}, nil
		case "m60b.vpk":
			return []string{"models/w_m60.mdl", "materials/w_m60.vmt", "materials/w_m60.vtf", "scripts/weapon_m60.txt"}, nil
		case "m60hud.vpk":
			return []string{"materials/vgui/hud/icon_m60.vtf"}, nil
		}
		return nil, nil
	}
	t.Cleanup(func() { singletonOverlapFileList = previous })

	conflict, err := app.CheckModEnableConflict("m60a.vpk")
	if err != nil {
		t.Fatalf("检查失败: %v", err)
	}
	if len(conflict.Conflicts) != 2 {
		t.Fatalf("应报告 2 条同类：%#v", conflict.Conflicts)
	}
	first := conflict.Conflicts[0]
	if first.Path != "m60b.vpk" || first.Severity != "severe" || first.OverlapCount != 4 {
		t.Fatalf("首条应为严重冲突的全量替换：%#v", first)
	}
	if len(first.OverlapFiles) == 0 || first.OverlapFiles[0] != "materials/w_m60.vmt" {
		t.Fatalf("重叠样例应有序返回：%#v", first.OverlapFiles)
	}
	second := conflict.Conflicts[1]
	if second.Path != "m60hud.vpk" || second.Severity != "normal" || second.OverlapCount != 1 {
		t.Fatalf("次条应为仅 HUD 重叠的普通项：%#v", second)
	}
}
