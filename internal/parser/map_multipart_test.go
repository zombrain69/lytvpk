package parser

import (
	"path/filepath"
	"testing"
)

// 多分片地图配套包（路线图第 8 项①）回归用例。
//
// 样本取自 2026-10-12 只读全库扫描（Dead Before Dawn (Extended) Part 2–4、
// 连接 Connection part 2、宜昌2 贴图包、3583381403 素材包；细节见 map_multipart.go）。

const multipartMissionFixture = `"mission"
{
	"DisplayTitle" "Dead Before Dawn (Extended)"
	"modes"
	{
		"coop" {
			"1"
			{
				"Map" "l4d_dbdext_the_mall"
				"DisplayName" "The Mall"
			}
		}
	}
}`

func TestMultipartMapCompanionPromotesToMap(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "2083814432.vpk")
	buildSingleFileVPK(t, filePath, []testVPKEntry{
		{name: "addoninfo.txt", content: []byte("\"AddonInfo\"\n{\n\t\"addontitle\" \"Dead Before Dawn (Extended) (Part 2)\"\n}\n")},
		{name: "models/survivors/survivor_coach.mdl", content: []byte("mdl-coach")},
		{name: "materials/models/deadb4dawn/wall.vmt", content: []byte("vmt-wall")},
		{name: "missions/dbdextended.txt", content: []byte(multipartMissionFixture)},
	})

	got, err := ParseVPKFileMetadata(filePath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.PrimaryTag != "地图" {
		t.Fatalf("分片包主分类应为「地图」，实际 %q（secondary=%v）", got.PrimaryTag, got.SecondaryTags)
	}
	if !containsTag(got.SecondaryTags, mapCompanionTag) {
		t.Fatalf("缺少「%s」标记: %v", mapCompanionTag, got.SecondaryTags)
	}
	if !containsTag(got.SecondaryTags, mapConfigTag) {
		t.Fatalf("缺少「%s」兜底: %v", mapConfigTag, got.SecondaryTags)
	}
	if got.Campaign != "Dead Before Dawn (Extended)" {
		t.Fatalf("战役字段不对: %q", got.Campaign)
	}
	if !containsTag(got.SecondaryTags, "战役模式") {
		t.Fatalf("缺少模式标签: %v", got.SecondaryTags)
	}
	// 资产标签必须保留：分片包不执行地图口径的场景道具豁免（只增不减）。
	if !containsTag(got.SecondaryTags, "模型") || !containsTag(got.SecondaryTags, "贴图") {
		t.Fatalf("资产标签被丢掉: %v", got.SecondaryTags)
	}
}

func TestMultipartMapCompanionOverridesWeaponClassification(t *testing.T) {
	// 资产以武器路径为主的分片包（样本 3583381403 风格）：主分类仍升为「地图」，
	// 武器类附加标签照常保留（走 collectSupplementaryTypeTags）。
	filePath := filepath.Join(t.TempDir(), "3583381403.vpk")
	buildSingleFileVPK(t, filePath, []testVPKEntry{
		{name: "models/w_models/weapons/w_rifle_ak47.mdl", content: []byte("mdl-ak47")},
		{name: "missions/gx_ls_lfswj.txt", content: []byte(multipartMissionFixture)},
	})

	got, err := ParseVPKFileMetadata(filePath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.PrimaryTag != "地图" {
		t.Fatalf("分片包主分类应为「地图」，实际 %q（secondary=%v）", got.PrimaryTag, got.SecondaryTags)
	}
	if !containsTag(got.SecondaryTags, mapCompanionTag) {
		t.Fatalf("缺少「%s」标记: %v", mapCompanionTag, got.SecondaryTags)
	}
	if !containsTag(got.SecondaryTags, "AK47") {
		t.Fatalf("武器附加标签被丢掉: %v", got.SecondaryTags)
	}
}

func TestVanillaMissionsOnlyPackStaysUnpromoted(t *testing.T) {
	// D9 场景（样本 2239816060）：原版 campaign<数字> 任务文件的覆盖包不是分片，
	// 不得升级主分类；「地图配置」兜底照旧。
	filePath := filepath.Join(t.TempDir(), "2239816060.vpk")
	buildSingleFileVPK(t, filePath, []testVPKEntry{
		{name: "materials/models/vanilla_fix.vmt", content: []byte("vmt")},
		{name: "missions/campaign9.txt", content: []byte(multipartMissionFixture)},
	})

	got, err := ParseVPKFileMetadata(filePath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.PrimaryTag == "地图" {
		t.Fatalf("原版战役覆盖包不应升级为「地图」: %v", got.SecondaryTags)
	}
	if containsTag(got.SecondaryTags, mapCompanionTag) {
		t.Fatalf("原版战役覆盖包不应带「%s」: %v", mapCompanionTag, got.SecondaryTags)
	}
	if !containsTag(got.SecondaryTags, mapConfigTag) {
		t.Fatalf("missions-only 包应保留「%s」兜底: %v", mapConfigTag, got.SecondaryTags)
	}
}

func TestMultipartMapCompanionNotMarkedWhenBSPPresent(t *testing.T) {
	// 带 BSP 的完整地图（样本 2083812352 主包）即使有自定义任务文件也不是「分片」。
	filePath := filepath.Join(t.TempDir(), "2083812352.vpk")
	buildSingleFileVPK(t, filePath, []testVPKEntry{
		{name: "maps/l4d_dbdext_the_mall.bsp", content: []byte("bsp")},
		{name: "missions/dbdextended.txt", content: []byte(multipartMissionFixture)},
	})

	got, err := ParseVPKFileMetadata(filePath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got.PrimaryTag != "地图" {
		t.Fatalf("主包应为「地图」，实际 %q", got.PrimaryTag)
	}
	if containsTag(got.SecondaryTags, mapCompanionTag) {
		t.Fatalf("完整地图（有 BSP）不应标「%s」: %v", mapCompanionTag, got.SecondaryTags)
	}
}

func TestIsMultipartMapCompanionIndexSignals(t *testing.T) {
	// missions 子目录同样算证据。
	withSubdir := buildArchivePathIndex(testArchiveFiles(
		vpkFile("materials/weapons/x", "blade", "vmt"),
		vpkFile("missions/sub", "custom_map", "txt"),
	))
	if !isMultipartMapCompanion(withSubdir) {
		t.Fatalf("missions/ 子目录任务文件应判为分片证据")
	}

	// 没有任务文件 → 不是分片。
	noMission := buildArchivePathIndex(testArchiveFiles(vpkFile("materials/x", "a", "vmt")))
	if isMultipartMapCompanion(noMission) {
		t.Fatalf("没有任务文件不应判为分片")
	}

	// 有 BSP → 不是分片。
	withBSP := buildArchivePathIndex(testArchiveFiles(
		vpkFile("maps", "m1", "bsp"),
		vpkFile("missions", "custom_map", "txt"),
	))
	if isMultipartMapCompanion(withBSP) {
		t.Fatalf("有 BSP 不应判为分片")
	}

	// 只有原版任务文件 → 不是分片。
	vanillaOnly := buildArchivePathIndex(testArchiveFiles(
		vpkFile("materials/x", "a", "vmt"),
		vpkFile("missions", "campaign3", "txt"),
	))
	if isMultipartMapCompanion(vanillaOnly) {
		t.Fatalf("只有原版任务文件不应判为分片")
	}

	// 原版 + 自定义混合 → 仍是分片（任一自定义任务文件即可）。
	mixed := buildArchivePathIndex(testArchiveFiles(
		vpkFile("materials/x", "a", "vmt"),
		vpkFile("missions", "campaign3", "txt"),
		vpkFile("missions", "my_campaign", "txt"),
	))
	if !isMultipartMapCompanion(mixed) {
		t.Fatalf("存在自定义任务文件时应判为分片")
	}
}

func TestVanillaCampaignStemDetection(t *testing.T) {
	vanilla := []string{"campaign1", "campaign9", "campaign14", "Campaign12", " campaign4 "}
	for _, stem := range vanilla {
		if !isVanillaCampaignStem(stem) {
			t.Fatalf("%q 应判为原版 campaign<数字>", stem)
		}
	}
	custom := []string{"dbdextended", "connection", "yichang2", "gx_ls_lfswj", "campaign", "campaign9x", "campaign_9", "cam9"}
	for _, stem := range custom {
		if isVanillaCampaignStem(stem) {
			t.Fatalf("%q 应判为自定义", stem)
		}
	}
}
