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
