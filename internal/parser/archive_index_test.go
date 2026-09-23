package parser

import (
	"reflect"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

func TestArchivePathIndexClassifiesUppercaseSurvivorPath(t *testing.T) {
	archive := testArchive("MODELS\\SURVIVORS", "SURVIVOR_NAMVET", "MDL")
	index := buildArchivePathIndex(archive)

	if got := determineVPKType(index); got != "人物" {
		t.Fatalf("expected 人物, got %q", got)
	}

	tags := make(map[string]bool)
	file := &VPKFile{}
	ProcessCharacterVPK(index, file, tags)
	if file.PrimaryTag != "人物" || !tags["Bill"] {
		t.Fatalf("expected 人物/Bill, got primary=%q tags=%v", file.PrimaryTag, tags)
	}
}

func TestArchivePathIndexRejectsUICharacterMention(t *testing.T) {
	archive := testArchive("RESOURCE/UI", "SURVIVOR_HUD", "RES")
	if got := determineVPKType(buildArchivePathIndex(archive)); got != "其他" {
		t.Fatalf("UI resource must not classify VPK as a character mod, got %q", got)
	}
}

func TestWeaponPathRulesPreferDesertEagleOverDesertRifle(t *testing.T) {
	archive := testArchive("models/weapons", "w_desert_eagle", "mdl")
	index := buildArchivePathIndex(archive)

	if got := determineVPKType(index); got != "武器" {
		t.Fatalf("expected 武器, got %q", got)
	}

	tags := make(map[string]bool)
	ProcessWeaponVPK(index, &VPKFile{}, tags)
	if !tags["马格南"] || tags["三连发"] {
		t.Fatalf("w_desert_eagle must be 马格南 only, got %v", tags)
	}
}

func TestArchivePathIndexRecognizesRealViewModelDirectory(t *testing.T) {
	archive := testArchive("MODELS/V_MODELS/WEAPONS/DESERTEAGLE", "v_deserteagle", "MDL")
	index := buildArchivePathIndex(archive)

	if got := determineVPKType(index); got != "武器" {
		t.Fatalf("expected 武器 from v_models/weapons, got %q", got)
	}

	tags := make(map[string]bool)
	ProcessWeaponVPK(index, &VPKFile{}, tags)
	if !tags["马格南"] {
		t.Fatalf("expected deserteagle view model to be 马格南, got %v", tags)
	}
}

func TestArchivePathIndexRecognizesWeaponMaterialAndStandaloneScript(t *testing.T) {
	tests := []struct {
		name string
		dir  string
		base string
		ext  string
	}{
		{name: "weapon material", dir: "MATERIALS\\MODELS\\WEAPONS", base: "v_rifle_ak47", ext: "VMT"},
		{name: "standalone weapon script", dir: "SCRIPTS", base: "weapon_smg", ext: "TXT"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := determineVPKType(buildArchivePathIndex(testArchive(test.dir, test.base, test.ext))); got != "武器" {
				t.Fatalf("expected 武器, got %q", got)
			}
		})
	}
}

func TestEquipmentUnderWeaponDirectoryUsesLocalizedItemTag(t *testing.T) {
	archive := testArchiveFiles(
		vpk.File{Dir: "models/w_models/weapons", Base: "eq_medkit", Ext: "mdl"},
		vpk.File{Dir: "materials/vgui/hud", Base: "healthbar", Ext: "vtf"},
	)
	index := buildArchivePathIndex(archive)

	if got := determineVPKType(index); got != "其他" {
		t.Fatalf("eq_medkit must not be classified as 武器, got %q", got)
	}
	if !index.contentTags["医疗包"] || !index.contentTags["HUD"] || !index.contentTags["血条"] {
		t.Fatalf("expected localized item and HUD tags, got %v", index.contentTags)
	}
}

func TestHistoricalWeaponPathsStaySpecificAndDeterministic(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "M1014 auto shotgun", path: "models/weapons/w_shotgun_m1014.mdl", want: "一代连喷"},
		{name: "M4 Super auto shotgun", path: "models/weapons/w_autoshot_m4super.mdl", want: "一代连喷"},
		{name: "Chrome shotgun", path: "models/weapons/shotgun_chrome.mdl", want: "铁喷"},
		{name: "Uzi", path: "models/weapons/w_smg_uzi.mdl", want: "乌兹"},
		{name: "Silenced SMG", path: "models/weapons/smg_silenced.mdl", want: "消音"},
		{name: "Fixed gun", path: "models/w_models/50cal/50cal.mdl", want: "固定机关枪"},
		{name: "Riot shield", path: "models/weapons/melee/riot_shield.mdl", want: "防爆盾"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := weaponPathTag(test.path); got != test.want {
				t.Fatalf("weaponPathTag(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}

	if got := weaponPathTag("models/weapons/oscar_statue.mdl"); got != "" {
		t.Fatalf("scar must not match oscar, got %q", got)
	}
}

func TestContentTagRulesRecognizeActualL4D2StylePaths(t *testing.T) {
	archive := testArchiveFiles(
		vpk.File{Dir: "sound/music/flu/jukebox", Base: "song", Ext: "wav"},
		vpk.File{Dir: "models/props_unique", Base: "vending_machine", Ext: "mdl"},
		vpk.File{Dir: "materials/skybox", Base: "city", Ext: "vtf"},
		vpk.File{Dir: "resource/ui", Base: "radial_menu", Ext: "res"},
	)
	tags := buildArchivePathIndex(archive).contentTags
	for _, want := range []string{"唱片机", "售货机", "天空", "人物语音表"} {
		if !tags[want] {
			t.Fatalf("expected %q in %v", want, tags)
		}
	}
}

func TestArchivePathIndexKeepsMapPriority(t *testing.T) {
	archive := &vpk.Archive{Files: []vpk.File{
		{Dir: "maps", Base: "c1m1_hotel", Ext: "BSP"},
		{Dir: "models/weapons", Base: "w_rifle_ak47", Ext: "mdl"},
	}}

	if got := determineVPKType(buildArchivePathIndex(archive)); got != "地图" {
		t.Fatalf("expected 地图 to keep highest priority, got %q", got)
	}
}

func TestSortedTagSetAndSecondaryTagsAreStable(t *testing.T) {
	if got, want := sortedTagSet(map[string]bool{"Zoey": true, "Bill": true, "Coach": true}), []string{"Bill", "Coach", "Zoey"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedTagSet = %v, want %v", got, want)
	}

	files := []VPKFile{
		{PrimaryTag: "人物", SecondaryTags: []string{"Zoey", "Bill"}},
		{PrimaryTag: "人物", SecondaryTags: []string{"Coach", "Bill"}},
	}
	if got, want := GetSecondaryTags(files, "人物"), []string{"Bill", "Coach", "Zoey"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GetSecondaryTags = %v, want %v", got, want)
	}
}

func testArchive(dir, base, ext string) *vpk.Archive {
	return testArchiveFiles(vpk.File{Dir: dir, Base: base, Ext: ext})
}

// 「套件命名空间」：materials/models/<作者>/<套件>/… 或 models/<作者>/<套件>/…
// 是判断"同一套件的多个模块"的关键证据（例如 l4n 的 airi_evilfall、codm 的 ice）。
func TestStructureSuiteNamespaceDetectsAuthorSuiteRoots(t *testing.T) {
	archive := testArchiveFiles(
		vpk.File{Dir: "materials/models/913limod/airi_evilfall/swtich", Base: "lightmainarmglove", Ext: "vmt"},
		vpk.File{Dir: "materials/models/913limod/airi_evilfall/swtich", Base: "metalmainarmglove", Ext: "vmt"},
		vpk.File{Dir: "materials/models/codm/ice", Base: "codm_ice", Ext: "vmt"},
		// 官方目录不能被当成套件命名空间
		vpk.File{Dir: "materials/models/weapons/rifle_ak47", Base: "w_rifle_ak47", Ext: "vmt"},
		vpk.File{Dir: "materials/models/survivors/survivor_coach", Base: "coach", Ext: "vmt"},
		// 目录里只有一层（没有套件段）时不算
		vpk.File{Dir: "materials/models/honkai3", Base: "body", Ext: "vmt"},
	)
	index := buildArchivePathIndex(archive)
	file := &VPKFile{}
	applyStructureSummary(file, index)

	roots := file.StructureResourceRoots
	if !containsString(roots, "913limod/airi_evilfall") {
		t.Fatalf("应识别出 913limod/airi_evilfall: %#v", roots)
	}
	if !containsString(roots, "codm/ice") {
		t.Fatalf("应识别出 codm/ice: %#v", roots)
	}
	for _, banned := range []string{"weapons/rifle_ak47", "survivors/survivor_coach", "honkai3"} {
		if containsString(roots, banned) {
			t.Fatalf("官方目录或只有一层的目录不应出现 %q: %#v", banned, roots)
		}
	}
}

// 内部路径不统一时也要能收敛到同一个套件候选：
//
//	materials/sikushui/mo/white_2.vtf      → sikushui
//	materials/qkl/mo/sikushui/waitao.vtf    → sikushui
//
// （死库水套件的真实形态；没有 models 段）
func TestStructureSuiteNamespaceFromMaterialsBranch(t *testing.T) {
	cases := map[string]string{
		"materials/sikushui/mo/white_2.vtf":             "sikushui",
		"materials/qkl/mo/sikushui/waitao.vtf":          "sikushui",
		"materials/models/913limod/airi_evilfall/a.vmt": "913limod/airi_evilfall",
	}
	for path, want := range cases {
		got := structureSuiteNamespace(path)
		if got != want {
			t.Fatalf("structureSuiteNamespace(%q) = %q, want %q", path, got, want)
		}
	}
	// 官方 materials 子目录不能被当成套件
	for _, path := range []string{
		"materials/models/weapons/rifle_ak47/a.vmt",
		"materials/vgui/hud/crosshair.vtf",
		"materials/props/crates/crate.vtf",
		"materials/particle/smoke.vtf",
	} {
		if got := structureSuiteNamespace(path); got != "" {
			t.Fatalf("官方目录不应产出套件候选 %q => %q", path, got)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func testArchiveFiles(files ...vpk.File) *vpk.Archive {
	return &vpk.Archive{Files: files}
}

func BenchmarkBuildArchivePathIndex1000Entries(b *testing.B) {
	patterns := []vpk.File{
		{Dir: "models/survivors", Base: "survivor_namvet", Ext: "mdl"},
		{Dir: "models/weapons", Base: "w_rifle_ak47", Ext: "mdl"},
		{Dir: "models/w_models/weapons", Base: "eq_medkit", Ext: "mdl"},
		{Dir: "materials/vgui/hud", Base: "crosshair", Ext: "vtf"},
		{Dir: "sound/music/flu/jukebox", Base: "song", Ext: "wav"},
		{Dir: "models/props_unique", Base: "vending_machine", Ext: "mdl"},
		{Dir: "materials/skybox", Base: "city", Ext: "vtf"},
		{Dir: "scripts/weapons", Base: "weapon_smg", Ext: "txt"},
	}
	files := make([]vpk.File, 0, 1000)
	for len(files) < cap(files) {
		files = append(files, patterns[len(files)%len(patterns)])
	}
	archive := &vpk.Archive{Files: files}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildArchivePathIndex(archive)
	}
}
