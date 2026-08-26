package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToggleVPKFileRemovesDisabledEntryInCustomCollection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "custom-mods")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sample.vpk")
	if err := os.WriteFile(path, []byte("sample"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "addonlist.txt"), []byte("\"AddonList\"\n{\n\t\"sample.vpk\"\t\t\"1\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: root}
	app.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
		Path: path, Name: "sample.vpk", Location: "root", Enabled: true,
	}})
	if err := app.ToggleVPKFile(path); err != nil {
		t.Fatalf("disable custom VPK: %v", err)
	}
	disabledPath := filepath.Join(root, "disabled", "sample.vpk")
	if _, err := os.Stat(disabledPath); err != nil {
		t.Fatalf("disabled VPK missing: %v", err)
	}
	items, _, err := app.readAddonList()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("disabled custom VPK remained in addonlist: %#v", items)
	}

	if err := app.ToggleVPKFile(disabledPath); err != nil {
		t.Fatalf("enable custom VPK: %v", err)
	}
	items, _, err = app.readAddonList()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "sample.vpk" || items[0].Value != "1" {
		t.Fatalf("re-enabled custom VPK state = %#v", items)
	}
}

func TestMoveWorkshopCopiesOriginalWithoutEnablingAddonListGuard(t *testing.T) {
	addons := filepath.Join(t.TempDir(), "left4dead2", "addons")
	workshop := filepath.Join(addons, "workshop")
	if err := os.MkdirAll(workshop, 0755); err != nil {
		t.Fatal(err)
	}
	workshopPath := filepath.Join(workshop, "123456789.vpk")
	if err := os.WriteFile(workshopPath, []byte("workshop"), 0644); err != nil {
		t.Fatal(err)
	}
	app := &App{rootDir: addons}
	app.vpkCache.Store(workshopPath, &VPKFileCache{File: VPKFile{
		Path: workshopPath, Name: "123456789.vpk", Location: "workshop", Enabled: true, WorkshopID: "123456789",
	}})
	if err := app.MoveWorkshopToAddons(workshopPath); err != nil {
		t.Fatalf("copy workshop VPK: %v", err)
	}
	rootPath := filepath.Join(addons, "123456789.vpk")
	for _, path := range []string{workshopPath, rootPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected copied file %s: %v", path, err)
		}
	}
	items, _, err := app.readAddonList()
	if err != nil {
		t.Fatal(err)
	}
	states := addonListItemsByKey(items)
	if states["123456789.vpk"].Value != "1" || states["workshop\\123456789.vpk"].Value != "0" {
		t.Fatalf("copy addonlist states = %#v", states)
	}
	info, err := app.GetAddonListInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.GuardEnabled || info.ManagedSnapshotExists {
		t.Fatalf("copy must not enable addonlist monitoring or create a protected snapshot: %#v", info)
	}
}
