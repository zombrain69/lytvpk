package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetVPKOperationWarningReportsMissingAddonInfoWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(addonsDir, 0755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(addonsDir, "missing-addoninfo.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"materials/test.vtf": []byte("not really a vtf"),
	})

	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{
		Path: vpkPath, Name: "missing-addoninfo.vpk", Location: "root",
	}})

	result, err := app.GetVPKOperationWarning(vpkPath)
	if err != nil {
		t.Fatalf("GetVPKOperationWarning: %v", err)
	}
	if !result.HasWarning || !result.Repairable {
		t.Fatalf("result = %+v, want warning and repairable", result)
	}
	if !strings.Contains(result.Detail, "缺少根目录 addoninfo.txt") {
		t.Fatalf("detail = %q, want missing addoninfo reason", result.Detail)
	}
}

func TestGetVPKOperationWarningReportsMalformedAddonInfoWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	vpkPath := filepath.Join(root, "malformed-addoninfo.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonDescription \"\nunclosed\n}\n"),
	})

	app := &App{}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{Path: vpkPath, Name: filepath.Base(vpkPath), Location: "root"}})

	result, err := app.GetVPKOperationWarning(vpkPath)
	if err != nil {
		t.Fatalf("GetVPKOperationWarning: %v", err)
	}
	if !result.HasWarning || !result.Repairable || !strings.Contains(result.Detail, "addoninfo.txt") {
		t.Fatalf("result = %+v, want malformed addoninfo warning", result)
	}
}

func TestGetVPKOperationWarningAcceptsValidAddonInfo(t *testing.T) {
	root := t.TempDir()
	vpkPath := filepath.Join(root, "valid-addoninfo.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonSteamAppID \"550\"\n}\n"),
	})

	app := &App{}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{Path: vpkPath, Name: filepath.Base(vpkPath), Location: "root"}})

	result, err := app.GetVPKOperationWarning(vpkPath)
	if err != nil {
		t.Fatalf("GetVPKOperationWarning: %v", err)
	}
	if result.HasWarning {
		t.Fatalf("result = %+v, want no warning", result)
	}
}

func TestGetVPKOperationWarningReportsMalformedAddonInfo(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(addonsDir, 0755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(addonsDir, "malformed-addoninfo.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonDescription \"\nunclosed\n}\n"),
	})

	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{
		Path: vpkPath, Name: "malformed-addoninfo.vpk", Location: "root",
	}})

	result, err := app.GetVPKOperationWarning(vpkPath)
	if err != nil {
		t.Fatalf("GetVPKOperationWarning: %v", err)
	}
	if !result.HasWarning || !result.Repairable || !strings.Contains(result.Detail, "addoninfo.txt") {
		t.Fatalf("result = %+v, want malformed addoninfo warning", result)
	}
}

func TestGetVPKOperationWarningSkipsValidFilesButChecksWorkshopFiles(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0755); err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(addonsDir, "valid.vpk")
	workshopPath := filepath.Join(addonsDir, "workshop", "123.vpk")
	writeTestVPK(t, validPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonSteamAppID \"550\"\n}\n"),
	})
	writeTestVPK(t, workshopPath, map[string][]byte{
		"materials/test.vtf": []byte("not really a vtf"),
	})

	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(validPath, &VPKFileCache{File: VPKFile{Path: validPath, Location: "root"}})
	app.vpkCache.Store(workshopPath, &VPKFileCache{File: VPKFile{Path: workshopPath, Location: "workshop"}})

	valid, err := app.GetVPKOperationWarning(validPath)
	if err != nil {
		t.Fatalf("valid warning check: %v", err)
	}
	if valid.HasWarning {
		t.Fatalf("valid result = %+v, want no warning", valid)
	}
	workshop, err := app.GetVPKOperationWarning(workshopPath)
	if err != nil {
		t.Fatalf("workshop warning check: %v", err)
	}
	if !workshop.HasWarning || !strings.Contains(workshop.Detail, "addoninfo.txt") {
		t.Fatalf("workshop result = %+v, want integrity warning", workshop)
	}
}
