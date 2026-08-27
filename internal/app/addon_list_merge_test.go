package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddonListPathUsesGameParentOnlyForRealGameAddonsDirectory(t *testing.T) {
	gameAddons := filepath.Join(t.TempDir(), "left4dead2", "addons")
	if err := os.MkdirAll(gameAddons, 0755); err != nil {
		t.Fatal(err)
	}
	gameApp := &App{rootDir: gameAddons}
	gamePath, err := gameApp.addonListPath()
	if err != nil {
		t.Fatal(err)
	}
	wantGamePath := filepath.Join(filepath.Dir(gameAddons), "addonlist.txt")
	if gamePath != wantGamePath {
		t.Fatalf("game addonlist path = %q, want %q", gamePath, wantGamePath)
	}

	customRoot := filepath.Join(t.TempDir(), "my-mod-collection")
	if err := os.MkdirAll(customRoot, 0755); err != nil {
		t.Fatal(err)
	}
	customApp := &App{rootDir: customRoot}
	customPath, err := customApp.addonListPath()
	if err != nil {
		t.Fatal(err)
	}
	wantCustomPath := filepath.Join(customRoot, "addonlist.txt")
	if customPath != wantCustomPath {
		t.Fatalf("custom addonlist path = %q, want %q", customPath, wantCustomPath)
	}
}

func TestAddonListPathAcceptsSteamGameRoot(t *testing.T) {
	base := t.TempDir()
	gameRoot := filepath.Join(base, "Left 4 Dead 2")
	gameDir := filepath.Join(gameRoot, "left4dead2")
	if err := os.MkdirAll(filepath.Join(gameDir, "addons"), 0755); err != nil {
		t.Fatal(err)
	}
	app := &App{rootDir: gameRoot}
	got, err := app.addonListPath()
	if err != nil {
		t.Fatalf("addonlist path for Steam root: %v", err)
	}
	want := filepath.Join(gameDir, "addonlist.txt")
	if got != want {
		t.Fatalf("addonlist path for Steam root = %q, want %q", got, want)
	}
}

func TestAddonListMergeKeepsCurrentByDefaultAndCreatesCustomFolderConfig(t *testing.T) {
	customRoot := filepath.Join(t.TempDir(), "my-mod-collection")
	sourceRoot := filepath.Join(t.TempDir(), "other-mod-collection")
	if err := os.MkdirAll(customRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatal(err)
	}

	targetPath := filepath.Join(customRoot, "addonlist.txt")
	if err := os.WriteFile(targetPath, []byte("\"AddonList\"\n{\n\t\"shared.vpk\"\t\t\"1\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(sourceRoot, "addonlist.txt")
	if err := os.WriteFile(sourcePath, []byte("\"AddonList\"\n{\n\t\"shared.vpk\"\t\t\"0\"\n\t\"from-source.vpk\"\t\t\"1\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: customRoot}
	preview, err := app.PreviewAddonListMerge(sourcePath)
	if err != nil {
		t.Fatalf("preview merge: %v", err)
	}
	if preview.TargetPath != targetPath {
		t.Fatalf("preview target = %q, want %q", preview.TargetPath, targetPath)
	}
	if len(preview.Added) != 1 || preview.Added[0].Name != "from-source.vpk" {
		t.Fatalf("unexpected added items: %#v", preview.Added)
	}
	if len(preview.Conflicts) != 1 || preview.Conflicts[0].Key != "shared.vpk" || !preview.Conflicts[0].CurrentEnabled || preview.Conflicts[0].SourceEnabled {
		t.Fatalf("unexpected conflicts: %#v", preview.Conflicts)
	}

	if err := app.ApplyAddonListMerge(sourcePath, nil); err != nil {
		t.Fatalf("apply default merge: %v", err)
	}
	items, _, err := app.readAddonList()
	if err != nil {
		t.Fatalf("read merged config: %v", err)
	}
	states := addonListItemsByKey(items)
	if states["shared.vpk"].Value != "1" || states["from-source.vpk"].Value != "1" {
		t.Fatalf("default merge states = %#v", states)
	}

	if err := app.ApplyAddonListMerge(sourcePath, []string{"shared.vpk"}); err != nil {
		t.Fatalf("apply source-wins merge: %v", err)
	}
	items, _, err = app.readAddonList()
	if err != nil {
		t.Fatalf("read source-wins config: %v", err)
	}
	states = addonListItemsByKey(items)
	if states["shared.vpk"].Value != "0" {
		t.Fatalf("source-wins value = %q, want 0", states["shared.vpk"].Value)
	}
	if _, err := os.Stat(targetPath + ".lytvpk.bak"); err != nil {
		t.Fatalf("original target config backup was not preserved: %v", err)
	}
}

func TestAddonListMergeCreatesMissingCustomFolderConfig(t *testing.T) {
	customRoot := filepath.Join(t.TempDir(), "empty-mod-collection")
	sourceRoot := filepath.Join(t.TempDir(), "source-mod-collection")
	if err := os.MkdirAll(customRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(sourceRoot, "addonlist.txt")
	if err := os.WriteFile(sourcePath, []byte("\"AddonList\"\n{\n\t\"new.vpk\"\t\t\"0\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: customRoot}
	if err := app.ApplyAddonListMerge(sourcePath, nil); err != nil {
		t.Fatalf("apply merge into missing config: %v", err)
	}
	items, path, err := app.readAddonList()
	if err != nil {
		t.Fatalf("read newly created config: %v", err)
	}
	if path != filepath.Join(customRoot, "addonlist.txt") {
		t.Fatalf("new config path = %q", path)
	}
	if len(items) != 1 || items[0].Name != "new.vpk" || items[0].Value != "0" {
		t.Fatalf("new config items = %#v", items)
	}
}

func TestWorkshopLoadOrderUsesRelativeWorkshopKey(t *testing.T) {
	addonsDir := filepath.Join(t.TempDir(), "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0755); err != nil {
		t.Fatal(err)
	}
	addonListPath := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	content := "\"AddonList\"\n{\n\t\"root.vpk\"\t\t\"1\"\n\t\"workshop\\3153860853.vpk\"\t\t\"0\"\n}\n"
	if err := os.WriteFile(addonListPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	workshopPath := filepath.Join(addonsDir, "workshop", "3153860853.vpk")
	app := &App{rootDir: addonsDir}

	order, err := app.GetVPKLoadOrder(workshopPath)
	if err != nil {
		t.Fatalf("get workshop order: %v", err)
	}
	if order != 2 {
		t.Fatalf("workshop order = %d, want 2", order)
	}
	if err := app.SetVPKLoadOrder(workshopPath, 1); err != nil {
		t.Fatalf("set workshop order: %v", err)
	}
	items, _, err := app.readAddonList()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "workshop\\3153860853.vpk" || items[0].Value != "0" {
		t.Fatalf("workshop load order was not preserved: %#v", items)
	}
}
