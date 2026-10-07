package app

import (
	"reflect"
	"testing"
)

// 按分类随机：官方开关只看白名单；自定义标签不受主类型限制；只有"当前启用"的标签才参与。
func TestCollectRotationTargetTags(t *testing.T) {
	files := []VPKFile{
		{Path: "a.vpk", Enabled: true, PrimaryTag: "武器", SecondaryTags: []string{"AK47", "贴图"}},
		{Path: "b.vpk", Enabled: true, PrimaryTag: "其他", SecondaryTags: []string{"我的分类"}},
		{Path: "c.vpk", Enabled: false, PrimaryTag: "武器", SecondaryTags: []string{"M16", "我的分类"}},
	}

	// 只开武器轮换：命中的是官方白名单标签，且只统计已启用的 Mod。
	weaponsOnly := collectRotationTargetTags(files, RotationConfig{EnableWeapons: true})
	if !weaponsOnly["AK47"] || weaponsOnly["M16"] {
		t.Fatalf("武器轮换只该收集已启用 Mod 的官方标签: %#v", weaponsOnly)
	}

	// 人物开关打开但没有人物 Mod → 不产生任何标签。
	if got := collectRotationTargetTags(files, RotationConfig{EnableCharacters: true}); len(got) != 0 {
		t.Fatalf("没有已启用的人物 Mod 时不应有目标标签: %#v", got)
	}

	// 自定义分类：不看主类型、不看白名单，只要求该标签下有启用中的 Mod。
	custom := collectRotationTargetTags(files, RotationConfig{Tags: []string{"我的分类"}})
	if !custom["我的分类"] {
		t.Fatalf("自定义分类应参与轮换: %#v", custom)
	}
	if got := collectRotationTargetTags(files, RotationConfig{Tags: []string{"不存在的分类"}}); len(got) != 0 {
		t.Fatalf("没有启用 Mod 的分类不该参与: %#v", got)
	}
}

func TestNormalizeRotationTags(t *testing.T) {
	got := NormalizeRotationTags([]string{" 贴图 ", "贴图", "", "HUD"})
	if want := []string{"贴图", "HUD"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeRotationTags = %#v，期望 %#v", got, want)
	}
	if got := NormalizeRotationTags(nil); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %#v", got)
	}
}

// 扩展配置不能破坏旧行为：只有人物/武器开关、没有自定义标签时，空配置仍然等于"关闭"。
func TestRotationConfigIsZeroKeepsLegacySemantics(t *testing.T) {
	if !rotationConfigIsZero(RotationConfig{}) {
		t.Fatal("空配置应视为未设置")
	}
	if rotationConfigIsZero(RotationConfig{EnableWeapons: true}) {
		t.Fatal("开启武器轮换后不算未设置")
	}
	if rotationConfigIsZero(RotationConfig{Tags: []string{"贴图"}}) {
		t.Fatal("只配了自定义分类也算已设置")
	}
	// 归一化后空标签列表会退化成 nil，从而回到"未设置"。
	if !rotationConfigIsZero(RotationConfig{Tags: NormalizeRotationTags([]string{"  "})}) {
		t.Fatal("全空白的自定义标签应归一化成未设置")
	}
}
