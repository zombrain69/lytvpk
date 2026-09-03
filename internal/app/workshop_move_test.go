package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveWorkshopFilesToAddonsBatchUsesForkTransferSemantics(t *testing.T) {
	rootDir := t.TempDir()
	workshopDir := filepath.Join(rootDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0755); err != nil {
		t.Fatal(err)
	}
	addonList := `"AddonList"
{
}
`
	if err := os.WriteFile(filepath.Join(rootDir, "addonlist.txt"), []byte(addonList), 0644); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(workshopDir, "3710541769.vpk")
	second := filepath.Join(workshopDir, "3710541770.vpk")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("vpk"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	app := &App{rootDir: rootDir}
	for _, path := range []string{first, second} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		app.vpkCache.Store(path, &VPKFileCache{
			File:    VPKFile{Name: filepath.Base(path), Path: path, Location: "workshop"},
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}

	result, err := app.MoveWorkshopFilesToAddons([]string{first, second, first})
	if err != nil {
		t.Fatalf("MoveWorkshopFilesToAddons returned error: %v", err)
	}
	if result.Total != 2 || result.SuccessCount != 2 || result.FailCount != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, source := range []string{first, second} {
		target := filepath.Join(rootDir, filepath.Base(source))
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("target VPK missing: %v", err)
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatalf("workshop source should be preserved by the Fork transfer semantics: %s (%v)", source, err)
		}
	}
}
