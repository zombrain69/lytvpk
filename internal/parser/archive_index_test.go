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
