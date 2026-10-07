package parser

import (
	"strings"
	"testing"
)

// 这套用例全部来自真机误标（2026-10-07 用户库实测）：
//   - 「匕首」下面混进了 60 多个"作者把材质放在 knife_* 目录里、实际替换的是警棍/撬棍/高尔夫"的包；
//   - 「M60」被 m600v 这种材质名误命中（HK416 / TAR-21 / 铁喷都被标成 M60）；
//   - 「固定机枪」既没分代，又被地图彩蛋音效 50cal_metalimpact.wav 误命中。
//
// 这些标签的精度靠本文件守住：改动关键词/匹配口径时先看这里。
func TestWeaponTagPrecisionGuards(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "M60：m600v 材质不算命中", path: "materials/models/weapons/hk416d/m600v.vmt", want: ""},
		{name: "M60：本体世界模型命中", path: "models/w_models/weapons/w_m60.mdl", want: "M60"},
		{name: "M60：弹箱音效命中", path: "sound/weapons/spent_mags/light_machinegun/m60_box5.wav", want: "M60"},
		{name: "匕首：作者命名空间里的 knife 目录不算命中", path: "materials/weapons/oe/knife_reborn-a6/zw.vmt", want: ""},
		{name: "匕首：本体小刀世界模型命中", path: "models/w_models/weapons/w_knife_t.mdl", want: "匕首"},
		{name: "匕首：本体小刀材质目录命中", path: "materials/models/weapons/v_models/knife_t/knife_t.vmt", want: "匕首"},
		{name: "匕首：knife_tactical 不能被 v_knife_t 误伤", path: "materials/models/weapons/v_models/knife_tactical/miao.vtf", want: ""},
		{name: "固定机枪：一代是 minigun", path: "models/w_models/weapons/w_minigun.vvd", want: "一代固定机枪"},
		{name: "固定机枪：二代是 50cal", path: "models/w_models/weapons/50cal.vvd", want: "二代固定机枪"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := weaponPathTag(test.path); got != test.want {
				t.Fatalf("weaponPathTag(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}

// 分代标签必须同时补上聚合标签与「所有枪械」，否则筛"固定机关枪"会漏。
func TestFixedGunTagsStayAggregated(t *testing.T) {
	for _, tag := range []string{"一代固定机枪", "二代固定机枪"} {
		tags := map[string]bool{}
		addWeaponTag(tag, tags)
		for _, want := range []string{tag, "固定机关枪", "所有枪械"} {
			if !tags[want] {
				t.Fatalf("addWeaponTag(%q) 缺少聚合标签 %q，实际 %v", tag, want, tags)
			}
		}
	}
}

// 本体 token 通道只解释"作者命名空间里的武器资源"：地图自带音乐/彩蛋里的
// 素材名（sound/music_new/**/50cal_metalimpact.wav）不能把整张地图标成固定机枪。
func TestStockTokenEvidenceSkipsNonWeaponSoundPaths(t *testing.T) {
	tags := map[string]bool{}
	if applyStockTokenEvidence("sound/music_new/nanningcity/easteregg/50cal_metalimpact.wav", tags, newTagEvidenceRecorder()) {
		t.Fatalf("彩蛋音效不应命中物品实体")
	}
	if tags["固定机关枪"] || tags["二代固定机枪"] {
		t.Fatalf("彩蛋音效不应把地图判成固定机枪：%v", tags)
	}

	weaponSoundTags := map[string]bool{}
	applyStockTokenEvidence("sound/weapons/50cal/50cal_shoot.wav", weaponSoundTags, newTagEvidenceRecorder())
	if !weaponSoundTags["二代固定机枪"] {
		t.Fatalf("sound/weapons 下的 50cal 音效仍应判成二代固定机枪，实际 %v", weaponSoundTags)
	}
}

// 标题通道：作者写 "Minigun 替换 M60" 描述的是模型外观，不代表改的是固定机枪；
// 明确写"固定机枪"的标题才给固定机枪标签。
func TestTitleEvidenceRespectsPathOnlyFixedGunRules(t *testing.T) {
	modelStyleTitle := map[string]bool{}
	applyTitleEvidence("【枪械】MW22 Minigun M134 加特林 替换M60", "", "mw22加特林圣宣.vpk", modelStyleTitle)
	if modelStyleTitle["固定机关枪"] || modelStyleTitle["一代固定机枪"] || modelStyleTitle["二代固定机枪"] {
		t.Fatalf("标题只提 Minigun 模型时不应判成固定机枪：%v", modelStyleTitle)
	}
	if !modelStyleTitle["M60"] {
		t.Fatalf("标题里的 M60 仍应命中：%v", modelStyleTitle)
	}

	explicitTitle := map[string]bool{}
	applyTitleEvidence("特效 曳光弹 替换 固定机枪弹道", "", "tracer.vpk", explicitTitle)
	if !explicitTitle["固定机关枪"] {
		t.Fatalf("明确写固定机枪的标题应给聚合标签：%v", explicitTitle)
	}
}

// 「本体独占资源前缀」通道：资源归属由本体索引推导（实体表的 prefixes），
// 而不是手写关键词 —— 音效目录、贴图目录、HUD 图标这类非模型资源靠它归属。
func TestStockOwnerPrefixChannel(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string
	}{
		{name: "小刀音效目录 → 匕首", path: "sound/weapons/knife/knife_deploy.wav", want: []string{"匕首"}},
		{name: "小刀本体材质目录 → 匕首", path: "materials/models/weapons/v_models/knife_t/knife_t.vmt", want: []string{"匕首"}},
		{name: "小刀 HUD 图标 → 匕首", path: "materials/vgui/hud/icon_knife.vmt", want: []string{"匕首"}},
		{name: "平底锅音效目录 → 平底锅", path: "sound/weapons/pan/swing1.wav", want: []string{"平底锅"}},
		{name: "AK47 音效目录 → AK47", path: "sound/weapons/rifle_ak47/gunfire/rifle_fire_1.wav", want: []string{"AK47"}},
		{name: "M60 机器枪音效目录 → M60", path: "sound/weapons/machinegun_m60/gunfire/m60_fire_1.wav", want: []string{"M60"}},
		{name: "作者命名空间（三角洲重塑）不归属", path: "materials/weapons/oe/knife_reborn-a6/zw.vmt", want: nil},
		{name: "作者命名空间（三角洲暗星）不归属", path: "materials/models/weapons/oe/knife_annihil/knife_annihil_u0_002.vmt", want: nil},
		{name: "多把枪共用的 rifle 目录不归属", path: "sound/weapons/rifle/gunfire/rifle_fire_1.wav", want: nil},
		{name: "地图自带音乐目录不归属", path: "sound/music_new/nanningcity/easteregg/50cal_metalimpact.wav", want: nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hits := stockOwnerPrefixHits(test.path)
			got := make([]string, 0, len(hits))
			for _, hit := range hits {
				got = append(got, hit.Tag)
			}
			if len(got) != len(test.want) {
				t.Fatalf("stockOwnerPrefixHits(%q) = %v，期望 %v", test.path, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("stockOwnerPrefixHits(%q) = %v，期望 %v", test.path, got, test.want)
				}
			}
		})
	}
}

// 实体表数据完好性：前缀必须来自本体（小写、"/" 分隔或文件 stem）、数量合理。
func TestStockOwnerPrefixesLookSanitized(t *testing.T) {
	if stockEntities == nil {
		t.Fatal("实体表未加载")
	}
	withPrefixes, total := 0, 0
	allowedKinds := map[string]bool{"sound": true, "script": true, "model": true, "material": true, "ui": true, "other": true}
	for _, entity := range stockEntities.Entities {
		if len(entity.Slots) == 0 {
			continue
		}
		withPrefixes++
		for _, slot := range entity.Slots {
			total++
			if slot.Prefix != strings.ToLower(slot.Prefix) || strings.Contains(slot.Prefix, "\\") || strings.HasPrefix(slot.Prefix, "/") {
				t.Fatalf("%s 的槽位前缀不规范：%q", entity.ID, slot.Prefix)
			}
			if !allowedKinds[slot.Kind] {
				t.Fatalf("%s 的槽位类别非法：%q", entity.ID, slot.Kind)
			}
			if slot.Hits <= 0 {
				t.Fatalf("%s 的槽位 %q 命中数为 %d（构建期应为 0 命中失败）", entity.ID, slot.Prefix, slot.Hits)
			}
		}
	}
	if withPrefixes < 30 || total < 80 {
		t.Fatalf("资源前缀太少（实体 %d / 前缀 %d），实体表可能没重新生成", withPrefixes, total)
	}
}

// 地图包口径：只有 token / 槽位证据的武器·物品标签要被砍掉，
// 本体精确锚点、标题证据与内容标签必须保留，聚合标签按剩下的具体标签重算。
func TestMapPackDropsResourceCoincidenceTags(t *testing.T) {
	recorder := newTagEvidenceRecorder()
	// 地图自带道具（巧合证据）
	recorder.record("二代固定机枪", "stock:weapon_50cal", EvidenceLevelPattern, "models/nanningcity/props/50cal_easter.vvd")
	recorder.record("撬棍", "token:crowbar", EvidenceLevelPattern, "models/nanningcity/props/crowbar.vvd")
	// 真替换本体文件（精确锚点）与内容标签
	recorder.record("AK47", "entity:weapon_rifle_ak47", EvidenceLevelExact, "models/w_models/weapons/w_rifle_ak47.mdl")
	recorder.record("模型", "category:模型", EvidenceLevelPattern, "models/nanningcity/props/50cal_easter.vvd")

	tags := map[string]bool{
		"二代固定机枪": true, "固定机关枪": true, "所有枪械": true,
		"撬棍": true, "近战": true, "官方近战": true, "所有官方近战": true,
		"AK47": true, "步枪": true, "模型": true,
	}
	dropped := dropCoincidenceOnlyTags(tags, recorder)
	if dropped != 2 {
		t.Fatalf("应删掉 2 个巧合标签，实际 %d（%v）", dropped, tags)
	}
	for _, gone := range []string{"二代固定机枪", "固定机关枪", "撬棍"} {
		if tags[gone] {
			t.Fatalf("巧合标签 %s 应被删除：%v", gone, tags)
		}
	}
	for _, kept := range []string{"AK47", "步枪", "所有枪械", "模型"} {
		if !tags[kept] {
			t.Fatalf("非巧合标签 %s 必须保留：%v", kept, tags)
		}
	}
	// 撬棍被删后不应留下孤儿「近战/官方近战」（AK47 是步枪，与近战无关）。
	for _, orphan := range []string{"近战", "官方近战", "所有官方近战"} {
		if tags[orphan] {
			t.Fatalf("孤儿的近战聚合 %s 不应保留：%v", orphan, tags)
		}
	}
}
