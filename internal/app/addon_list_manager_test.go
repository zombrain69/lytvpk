package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newAddonListManagerTestApp(t *testing.T, content string) (*App, string) {
	t.Helper()
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(addonsDir, 0755); err != nil {
		t.Fatal(err)
	}
	addonListPath := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	if err := os.WriteFile(addonListPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return &App{rootDir: addonsDir}, addonListPath
}

func TestAddonListLifecycleManagement(t *testing.T) {
	desired := "\"AddonList\"\n{\n\t\"desired.vpk\"\t\t\"1\"\n}\n"
	app, addonListPath := newAddonListManagerTestApp(t, desired)

	info, err := app.SaveAddonListManagedSnapshot()
	if err != nil {
		t.Fatalf("save managed snapshot: %v", err)
	}
	if !info.ManagedSnapshotExists {
		t.Fatal("managed snapshot was not recorded")
	}
	managedBytes, err := os.ReadFile(addonListManagedSnapshotPath(addonListPath))
	if err != nil {
		t.Fatalf("read managed snapshot: %v", err)
	}
	if !bytes.Equal(managedBytes, []byte(desired)) {
		t.Fatal("managed snapshot does not preserve original bytes")
	}

	manual, err := app.CreateAddonListBackup()
	if err != nil {
		t.Fatalf("create manual backup: %v", err)
	}
	if manual.Kind != "manual" {
		t.Fatalf("manual backup kind = %q", manual.Kind)
	}

	old := "\"AddonList\"\n{\n\t\"old.vpk\"\t\t\"0\"\n}\n"
	if err := os.WriteFile(addonListPath, []byte(old), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.RestoreAddonListBackup(manual.Name); err != nil {
		t.Fatalf("restore manual backup: %v", err)
	}
	restored, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, []byte(desired)) {
		t.Fatalf("restored bytes = %q, want %q", restored, desired)
	}

	backups, err := app.ListAddonListBackups()
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if len(backups) < 2 {
		t.Fatalf("backup count = %d, want manual and before-restore", len(backups))
	}
	if err := app.DeleteAddonListBackup(manual.Name); err != nil {
		t.Fatalf("delete manual backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(addonListBackupDirectory(addonListPath), manual.Name)); !os.IsNotExist(err) {
		t.Fatalf("manual backup still exists, stat err = %v", err)
	}

	if err := app.DeleteAddonList(); err != nil {
		t.Fatalf("delete addonlist: %v", err)
	}
	if _, err := os.Stat(addonListPath); !os.IsNotExist(err) {
		t.Fatalf("addonlist.txt still exists, stat err = %v", err)
	}
	if _, err := os.Stat(addonListManagedSnapshotPath(addonListPath)); !os.IsNotExist(err) {
		t.Fatalf("managed snapshot still exists, stat err = %v", err)
	}
	backups, err = app.ListAddonListBackups()
	if err != nil {
		t.Fatalf("list backups after delete: %v", err)
	}
	if !containsAddonListBackupKind(backups, "before") {
		t.Fatalf("before-delete backup missing: %#v", backups)
	}
}

func TestAddonListGuardRestoresStableExternalOverwrite(t *testing.T) {
	desired := "\"AddonList\"\n{\n\t\"desired.vpk\"\t\t\"1\"\n}\n"
	app, addonListPath := newAddonListManagerTestApp(t, desired)
	t.Cleanup(app.stopAddonListMonitor)

	info, err := app.SetAddonListGuardEnabled(true)
	if err != nil {
		t.Fatalf("enable guard: %v", err)
	}
	if !info.GuardEnabled || !info.ManagedSnapshotExists {
		t.Fatalf("unexpected guard info: %#v", info)
	}

	external := "\"AddonList\"\n{\n\t\"old.vpk\"\t\t\"0\"\n}\n"
	if err := os.WriteFile(addonListPath, []byte(external), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		content, readErr := os.ReadFile(addonListPath)
		if readErr == nil && bytes.Equal(content, []byte(desired)) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	content, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, []byte(desired)) {
		t.Fatalf("guard did not restore desired bytes: %q", content)
	}

	backups, err := app.ListAddonListBackups()
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	if !containsAddonListBackupKind(backups, "external") {
		t.Fatalf("external overwrite backup missing: %#v", backups)
	}
	info, err = app.GetAddonListInfo()
	if err != nil {
		t.Fatalf("get guard info: %v", err)
	}
	if info.LastGuardRestore == "" {
		t.Fatal("guard restore time was not recorded")
	}
}

func TestUnrecordedGameStateIsNotRewrittenAfterGameSave(t *testing.T) {
	initial := "\"AddonList\"\n{\n\t\"workshop\\2758492786.vpk\"\t\t\"1\"\n\t\"workshop\\2914817953.vpk\"\t\t\"1\"\n}\n"
	app, addonListPath := newAddonListManagerTestApp(t, initial)
	t.Cleanup(app.stopAddonListMonitor)

	vpkPath := filepath.Join(filepath.Dir(addonListPath), "addons", "AWM Mon3tr.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonSteamAppID \"550\"\n}\n"),
	})
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{
		Path: vpkPath, Name: "AWM Mon3tr.vpk", Location: "root",
	}})
	if err := app.SetVPKGameEnabled(vpkPath, true); err != nil {
		t.Fatalf("SetVPKGameEnabled: %v", err)
	}

	// The optional full-file guard is disabled by default. A game save must
	// remain authoritative; LytVPK must not silently reinsert AWM.
	gameSaved := "\"AddonList\"\n{\n\t\"workshop\\2758492786.vpk\"\t\t\"0\"\n\t\"workshop\\2914817953.vpk\"\t\t\"1\"\n}\n"
	if err := os.WriteFile(addonListPath, []byte(gameSaved), 0644); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != gameSaved {
		t.Fatalf("game save was unexpectedly rewritten:\nwant %q\n got %q", gameSaved, content)
	}
	app.addonListMonitorMu.Lock()
	monitorActive := app.addonListMonitorStop != nil
	app.addonListMonitorMu.Unlock()
	if monitorActive {
		t.Fatal("temporary addonlist protection monitor started without user opt-in")
	}
}

func containsAddonListBackupKind(backups []AddonListBackup, prefix string) bool {
	for _, backup := range backups {
		if strings.HasPrefix(backup.Kind, prefix) {
			return true
		}
	}
	return false
}
