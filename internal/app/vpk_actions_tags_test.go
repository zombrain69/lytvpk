package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetVPKTagsStoresWorkshopTagsInMetaWithoutRenamingVPK(t *testing.T) {
	addonsDir := filepath.Join(t.TempDir(), "left4dead2", "addons")
	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(workshopDir, "3153860853.vpk")
	if err := os.WriteFile(vpkPath, []byte("workshop-vpk"), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{
		Name:     filepath.Base(vpkPath),
		Path:     vpkPath,
		Location: "workshop",
	}})
	if err := app.SetVPKTags(vpkPath, "人物", []string{"Bill"}); err != nil {
		t.Fatalf("SetVPKTags: %v", err)
	}

	if _, err := os.Stat(vpkPath); err != nil {
		t.Fatalf("original workshop VPK should remain untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workshopDir, "[人物+Bill]3153860853.vpk")); !os.IsNotExist(err) {
		t.Fatalf("workshop VPK was renamed, stat err = %v", err)
	}
	meta, err := LoadWorkshopMeta(vpkPath)
	if err != nil || meta == nil {
		t.Fatalf("load workshop meta: meta=%#v err=%v", meta, err)
	}
	if meta.PrimaryTag != "人物" || len(meta.SecondaryTags) != 1 || meta.SecondaryTags[0] != "Bill" {
		t.Fatalf("hierarchical tags = %#v", meta)
	}
	if len(meta.Tags) != 2 || meta.Tags[0] != "人物" || meta.Tags[1] != "Bill" {
		t.Fatalf("stored tags = %#v", meta.Tags)
	}
	cached := app.mustCachedVPKFile(t, vpkPath)
	if cached.PrimaryTag != "人物" || len(cached.SecondaryTags) != 1 || cached.SecondaryTags[0] != "Bill" {
		t.Fatalf("cached tags = %#v", cached)
	}
}

func TestScanAppliesWorkshopMetaTagsWhenWorkshopDetailsAreDisabled(t *testing.T) {
	addonsDir := filepath.Join(t.TempDir(), "left4dead2", "addons")
	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(workshopDir, "3153860853.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"scripts/test.txt": []byte("test"),
	})
	if err := saveWorkshopMeta(vpkPath, &WorkshopMeta{
		Tags:          []string{"人物", "Bill"},
		PrimaryTag:    "人物",
		SecondaryTags: []string{"Bill"},
	}); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: addonsDir, workshopMetaEnabled: false}
	app.processVPKFileWithCache(vpkPath)
	cached := app.mustCachedVPKFile(t, vpkPath)
	if cached.PrimaryTag != "人物" || len(cached.SecondaryTags) != 1 || cached.SecondaryTags[0] != "Bill" {
		t.Fatalf("scanner did not apply workshop meta tags: %#v", cached)
	}
}

func TestSetVPKTagsCanonicalizesAndDeduplicatesSecondaryTags(t *testing.T) {
	addonsDir := filepath.Join(t.TempDir(), "left4dead2", "addons")
	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(workshopDir, "987654321.vpk")
	if err := os.WriteFile(vpkPath, []byte("workshop-vpk"), 0644); err != nil {
		t.Fatal(err)
	}
	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{
		Name: filepath.Base(vpkPath), Path: vpkPath, Location: "workshop",
	}})
	if err := app.SetVPKTags(vpkPath, "武器", []string{"榴弹", "榴弹发射器", "榴弹"}); err != nil {
		t.Fatalf("SetVPKTags: %v", err)
	}
	meta, err := LoadWorkshopMeta(vpkPath)
	if err != nil || meta == nil {
		t.Fatalf("load meta: %#v %v", meta, err)
	}
	if len(meta.SecondaryTags) != 1 || meta.SecondaryTags[0] != "榴弹发射器" {
		t.Fatalf("deduplicated meta tags = %#v", meta.SecondaryTags)
	}
	if cached := app.mustCachedVPKFile(t, vpkPath); len(cached.SecondaryTags) != 1 || cached.SecondaryTags[0] != "榴弹发射器" {
		t.Fatalf("deduplicated cache tags = %#v", cached.SecondaryTags)
	}
}
