package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/panjf2000/ants/v2"
)

// TestScanVPKFilesKeepsSameNamedFilesInRootAndWorkshop verifies that the cache
// identity is the full path, not just the VPK basename or workshop ID.
func TestScanVPKFilesKeepsSameNamedFilesInRootAndWorkshop(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "addons")
	workshopDir := filepath.Join(rootDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0755); err != nil {
		t.Fatal(err)
	}

	sourceDir := filepath.Join(tempDir, "3555655120")
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "addon.txt"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	packer := &App{}
	if _, err := packer.PackVPKDirectory(sourceDir, rootDir, false); err != nil {
		t.Fatalf("pack root VPK: %v", err)
	}
	if _, err := packer.PackVPKDirectory(sourceDir, workshopDir, false); err != nil {
		t.Fatalf("pack workshop VPK: %v", err)
	}

	pool, err := ants.NewPool(2)
	if err != nil {
		t.Fatalf("create worker pool: %v", err)
	}
	defer pool.Release()

	app := &App{rootDir: rootDir, goroutinePool: pool}
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("scan VPK files: %v", err)
	}

	files := app.GetVPKFiles()
	if len(files) != 2 {
		t.Fatalf("expected both same-named VPK files, got %d: %+v", len(files), files)
	}

	seen := map[string]bool{}
	for _, file := range files {
		seen[file.Path] = true
	}
	if !seen[filepath.Join(rootDir, "3555655120.vpk")] {
		t.Errorf("root VPK missing from scan: %v", seen)
	}
	if !seen[filepath.Join(workshopDir, "3555655120.vpk")] {
		t.Errorf("workshop VPK missing from scan: %v", seen)
	}

	searchResults := app.SearchVPKFiles("3555655120", "", nil)
	if len(searchResults) != 2 {
		t.Fatalf("expected both same-named VPK files in search results, got %d: %+v", len(searchResults), searchResults)
	}
}

func TestGetLocationFromPathIsCaseInsensitive(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "addons")
	app := &App{rootDir: rootDir}

	cases := []struct {
		path string
		want string
	}{
		{filepath.Join(rootDir, "WORKSHOP", "mod.vpk"), "workshop"},
		{filepath.Join(rootDir, "Disabled", "mod.vpk"), "disabled"},
		{filepath.Join(rootDir, "mod.vpk"), "root"},
	}
	for _, tc := range cases {
		if got := app.getLocationFromPath(tc.path); got != tc.want {
			t.Errorf("getLocationFromPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
