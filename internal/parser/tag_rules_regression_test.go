package parser

import (
	pathpkg "path"
	"reflect"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

func TestWeaponPathRuleRegressions(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantTag      string
		wantCategory string
	}{
		{
			name:         "M1014 must remain first-generation auto shotgun",
			path:         "models/w_models/weapons/w_shotgun_m1014.mdl",
			wantTag:      "一代连喷",
			wantCategory: "霰弹枪",
		},
		{
			name:         "scar is a whole path token",
			path:         "models/weapons/rifle_scar.mdl",
			wantTag:      "三连发",
			wantCategory: "步枪",
		},
		{
			name:    "oscar must not be a scar rifle",
			path:    "models/weapons/oscar_statue.mdl",
			wantTag: "",
		},
		{
			name:    "gauge texture must not be guessed as AUG",
			path:    "materials/models/weapons/97-1/gauge_n.vtf",
			wantTag: "",
		},
		{
			name:    "AA12 texture name must not be guessed as UMP45",
			path:    "materials/models/weapons/codol/aa12ss/mtl_scrolling_blackgold_ump45ss.vtf",
			wantTag: "",
		},
		{
			name:         "Desert Eagle is a Magnum slot replacement",
			path:         "models/w_models/weapons/w_desert_eagle.mdl",
			wantTag:      "马格南",
			wantCategory: "手枪",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := weaponPathTag(test.path); got != test.wantTag {
				t.Fatalf("weaponPathTag(%q) = %q, want %q", test.path, got, test.wantTag)
			}

			tags := make(map[string]bool)
			DetectWeaponType(test.path, tags)
			if test.wantTag == "" {
				if len(tags) != 0 {
					t.Fatalf("unexpected tags for %q: %v", test.path, tags)
				}
				return
			}
			if !tags[test.wantTag] || !tags[test.wantCategory] {
				t.Fatalf("expected %q and %q, got %v", test.wantTag, test.wantCategory, tags)
			}
		})
	}
}

func TestWeaponMetadataRuleRegressions(t *testing.T) {
	tests := []struct {
		name         string
		metadata     string
		wantTag      string
		wantCategory string
	}{
		{
			name:         "M1014 metadata",
			metadata:     "Classic M1014 auto shotgun",
			wantTag:      "一代连喷",
			wantCategory: "霰弹枪",
		},
		{
			name:         "SCAR word boundary",
			metadata:     "SCAR replacement",
			wantTag:      "三连发",
			wantCategory: "步枪",
		},
		{
			name:     "Oscar is not SCAR",
			metadata: "Oscar statue texture",
			wantTag:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tags := make(map[string]bool)
			DetectWeaponTypeFromMetadata(test.metadata, tags)
			if test.wantTag == "" {
				if len(tags) != 0 {
					t.Fatalf("unexpected metadata tags for %q: %v", test.metadata, tags)
				}
				return
			}
			if !tags[test.wantTag] || !tags[test.wantCategory] {
				t.Fatalf("expected %q and %q, got %v", test.wantTag, test.wantCategory, tags)
			}
		})
	}
}

func TestWorkshopContentCategoriesUseSourcePaths(t *testing.T) {
	archive := testArchiveFiles(
		vpkFile("resource/ui", "hud_layout", "res"),
		vpkFile("sound/player/survivor", "voice_line", "wav"),
		vpkFile("scripts", "weapon_test", "txt"),
		vpkFile("models/props_unique", "prop", "mdl"),
		vpkFile("materials/models/props_unique", "prop", "vmt"),
		vpkFile("models/w_models/weapons", "eq_medkit", "mdl"),
		vpkFile("models/w_models/weapons", "w_eq_pipebomb", "mdl"),
	)

	tags := buildArchivePathIndex(archive).contentTags
	for _, want := range []string{"UI", "声音", "脚本", "模型", "贴图", "物品", "投掷物", "医疗包", "土制炸弹"} {
		if !tags[want] {
			t.Fatalf("expected %q in %v", want, tags)
		}
	}
}

func TestOfficialItemAliasesAndWorkshopCategories(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "dynamic ammo stack",
			path: "models/props/terror/ammo_stack.mdl",
			want: []string{"二代子弹堆", "弹药堆", "盒子", "物品", "模型"},
		},
		{
			name: "coffee ammo pile",
			path: "models/props_unique/spawn_apartment/coffeeammo.mdl",
			want: []string{"二代子弹堆", "弹药堆", "盒子", "物品", "模型"},
		},
		{
			name: "legacy ammo can",
			path: "models/props/de_prodigy/ammo_can_03.mdl",
			want: []string{"一代子弹堆", "弹药堆", "盒子", "物品", "模型"},
		},
		{
			name: "incendiary upgrade pack",
			path: "models/v_models/v_incendiary_ammopack.mdl",
			want: []string{"燃烧弹盒", "燃烧弹", "盒子", "物品", "模型"},
		},
		{
			name: "explosive upgrade pack",
			path: "models/w_models/weapons/w_eq_explosive_ammopack.mdl",
			want: []string{"高爆弹盒", "高爆弹", "盒子", "物品", "模型"},
		},
		{
			name: "laser sights",
			path: "models/w_models/weapons/w_laser_sights.mdl",
			want: []string{"激光瞄准盒", "镭射", "盒子", "物品", "模型"},
		},
		{
			name: "adrenaline medical item",
			path: "models/v_models/v_adrenaline.mdl",
			want: []string{"肾上腺", "医疗物品", "所有医疗物品", "物品", "模型"},
		},
		{
			name: "bile throwable",
			path: "models/w_models/weapons/w_eq_bile_flask.mdl",
			want: []string{"胆汁", "投掷物", "所有投掷物品", "物品", "模型"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tags := buildArchivePathIndex(testArchiveFiles(vpkFileFromPath(test.path))).contentTags
			for _, want := range test.want {
				if !tags[want] {
					t.Fatalf("path %q missing %q in %v", test.path, want, tags)
				}
			}
		})
	}
}

func TestOfficialWeaponAggregateTags(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "rifle", path: "models/w_models/weapons/w_rifle_ak47.mdl", want: []string{"AK47", "步枪", "所有枪械"}},
		{name: "melee", path: "models/w_models/weapons/w_katana.mdl", want: []string{"武士刀", "近战", "官方近战", "所有官方近战"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tags := make(map[string]bool)
			DetectWeaponType(test.path, tags)
			for _, want := range test.want {
				if !tags[want] {
					t.Fatalf("path %q missing %q in %v", test.path, want, tags)
				}
			}
		})
	}
}

func TestCanonicalTagsAreUniqueAndAliasFree(t *testing.T) {
	got := UniqueTagsExcluding([]string{"榴弹", "榴弹发射器", "  榴弹  ", "AK47", "ak47", ""}, "")
	want := []string{"榴弹发射器", "AK47"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UniqueTagsExcluding() = %#v, want %#v", got, want)
	}

	tags := make(map[string]bool)
	DetectWeaponType("models/w_models/weapons/w_grenade_launcher.mdl", tags)
	if tags["榴弹"] || !tags["榴弹发射器"] {
		t.Fatalf("grenade aliases were not canonicalized: %v", tags)
	}
}

func TestAggregateTagFamiliesCoverExpectedChildren(t *testing.T) {
	families := map[string][]string{
		"所有投掷物品": {"土制炸弹", "燃烧瓶", "胆汁"},
		"所有医疗物品": {"医疗包", "电击器", "止痛药", "肾上腺"},
		"所有枪械":   {"小手枪", "马格南", "AK47", "M16", "三连发", "sg552", "M60", "大狙", "猎枪", "军狙", "鸟狙", "木喷", "一代连喷", "铁喷", "二代连喷", "乌兹", "消音", "MP5", "榴弹发射器", "固定机关枪"},
		"所有官方近战": {"棒球棍", "板球拍", "吉他", "平底锅", "高尔夫球杆", "消防斧", "砍刀", "武士刀", "电锯", "撬棍", "草叉", "铁铲", "警棍"},
	}
	for aggregate, children := range families {
		t.Run(aggregate, func(t *testing.T) {
			for _, child := range children {
				tags := make(map[string]bool)
				if strings.Contains(aggregate, "投掷") {
					pathByTag := map[string]string{"土制炸弹": "models/w_models/weapons/w_eq_pipebomb.mdl", "燃烧瓶": "models/w_models/weapons/w_eq_molotov.mdl", "胆汁": "models/w_models/weapons/w_eq_bile_flask.mdl"}
					collectContentTags(pathByTag[child], tags)
				} else if strings.Contains(aggregate, "医疗") {
					pathByTag := map[string]string{"医疗包": "models/w_models/weapons/eq_medkit.mdl", "电击器": "models/w_models/weapons/eq_defibrillator.mdl", "止痛药": "models/w_models/weapons/eq_painpills.mdl", "肾上腺": "models/v_models/v_adrenaline.mdl"}
					collectContentTags(pathByTag[child], tags)
				} else if aggregate == "所有官方近战" {
					pathByTag := map[string]string{"棒球棍": "models/w_models/weapons/w_bat.mdl", "板球拍": "models/w_models/weapons/w_cricket_bat.mdl", "吉他": "models/w_models/weapons/w_guitar.mdl", "平底锅": "models/w_models/weapons/w_frying_pan.mdl", "高尔夫球杆": "models/w_models/weapons/w_golf_club.mdl", "消防斧": "models/w_models/weapons/w_fireaxe.mdl", "砍刀": "models/w_models/weapons/w_machete.mdl", "武士刀": "models/w_models/weapons/w_katana.mdl", "电锯": "models/w_models/weapons/w_chainsaw.mdl", "撬棍": "models/w_models/weapons/w_crowbar.mdl", "草叉": "models/w_models/weapons/w_pitchfork.mdl", "铁铲": "models/w_models/weapons/w_shovel.mdl", "警棍": "models/w_models/weapons/w_tonfa.mdl"}
					DetectWeaponType(pathByTag[child], tags)
				} else {
					pathByTag := map[string]string{"小手枪": "models/w_models/weapons/w_pistol.mdl", "马格南": "models/w_models/weapons/w_desert_eagle.mdl", "AK47": "models/w_models/weapons/w_rifle_ak47.mdl", "M16": "models/w_models/weapons/w_rifle_m16.mdl", "三连发": "models/w_models/weapons/rifle_desert.mdl", "sg552": "models/w_models/weapons/w_rifle_sg552.mdl", "M60": "models/w_models/weapons/w_rifle_m60.mdl", "大狙": "models/w_models/weapons/sniper_awp.mdl", "猎枪": "models/w_models/weapons/w_sniper_mini14.mdl", "军狙": "models/w_models/weapons/sniper_military.mdl", "鸟狙": "models/w_models/weapons/sniper_scout.mdl", "木喷": "models/w_models/weapons/w_shotgun.mdl", "一代连喷": "models/w_models/weapons/w_shotgun_m1014.mdl", "铁喷": "models/w_models/weapons/shotgun_chrome.mdl", "二代连喷": "models/w_models/weapons/shotgun_spas.mdl", "乌兹": "models/w_models/weapons/smg_uzi.mdl", "消音": "models/w_models/weapons/smg_silenced.mdl", "MP5": "models/w_models/weapons/w_smg_mp5.mdl", "榴弹发射器": "models/w_models/weapons/grenade_launcher.mdl", "固定机关枪": "models/w_models/weapons/minigun.mdl"}
					DetectWeaponType(pathByTag[child], tags)
				}
				if !tags[aggregate] || !tags[child] {
					t.Fatalf("aggregate %q missing child %q: %v", aggregate, child, tags)
				}
			}
		})
	}
}

func TestHiddenMeleeDoesNotEnterOfficialMeleeAggregate(t *testing.T) {
	for _, path := range []string{
		"models/w_models/weapons/w_knife_t.mdl",
		"models/w_models/weapons/w_riot_shield.mdl",
	} {
		t.Run(path, func(t *testing.T) {
			tags := make(map[string]bool)
			DetectWeaponType(path, tags)
			if !tags["近战"] {
				t.Fatalf("expected hidden melee category for %q, got %v", path, tags)
			}
			if tags["官方近战"] || tags["所有官方近战"] {
				t.Fatalf("hidden melee must not enter official aggregate for %q: %v", path, tags)
			}
		})
	}
}

func TestMixedPackageKeepsPrimaryTypeAndExposesAdditionalTags(t *testing.T) {
	archive := testArchiveFiles(
		vpkFile("models/survivors", "survivor_coach", "mdl"),
		vpkFile("models/w_models/weapons", "w_rifle_ak47", "mdl"),
		vpkFile("materials/vgui/hud", "crosshair", "vtf"),
	)
	index := buildArchivePathIndex(archive)
	if got := determineVPKType(index); got != "人物" {
		t.Fatalf("mixed package primary type = %q, want 人物", got)
	}

	tags := make(map[string]bool)
	file := &VPKFile{}
	ProcessCharacterVPK(index, file, tags)
	collectSupplementaryTypeTags(index, "人物", tags)
	mergeTagSet(tags, index.contentTags)
	delete(tags, file.PrimaryTag)

	if file.PrimaryTag != "人物" {
		t.Fatalf("primary type changed to %q", file.PrimaryTag)
	}
	for _, want := range []string{"Coach", "幸存者", "AK47", "步枪", "武器", "HUD", "UI", "模型", "贴图"} {
		if !tags[want] {
			t.Fatalf("expected mixed-package tag %q in %v", want, tags)
		}
	}

	files := []VPKFile{{PrimaryTag: file.PrimaryTag, SecondaryTags: sortedTagSet(tags)}}
	available := GetSecondaryTags(files, "人物")
	if !containsTag(available, "AK47") {
		t.Fatalf("人物 secondary filters must expose AK47, got %v", available)
	}
}

func TestWorkshopCharacterCategories(t *testing.T) {
	archive := testArchiveFiles(
		vpkFile("models/survivors", "survivor_namvet", "mdl"),
		vpkFile("models/infected", "boomer", "mdl"),
		vpkFile("models/infected", "common_male", "mdl"),
	)
	tags := make(map[string]bool)
	ProcessCharacterVPK(buildArchivePathIndex(archive), &VPKFile{}, tags)

	for _, want := range []string{"Bill", "幸存者", "boomer", "特殊感染者", "common", "普通感染者"} {
		if !tags[want] {
			t.Fatalf("expected %q in %v", want, tags)
		}
	}
}

func vpkFile(dir, base, ext string) vpk.File {
	return vpk.File{Dir: dir, Base: base, Ext: ext}
}

func vpkFileFromPath(path string) vpk.File {
	path = strings.ReplaceAll(path, `\`, "/")
	dir, base := pathpkg.Split(path)
	baseName := strings.TrimSuffix(base, pathpkg.Ext(base))
	ext := strings.TrimPrefix(pathpkg.Ext(base), ".")
	return vpk.File{Dir: strings.TrimSuffix(dir, "/"), Base: baseName, Ext: ext}
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
