package app

import (
	"os"
	"path/filepath"
	"testing"
)

func newProfileTestApp(t *testing.T, rootDir string) *App {
	t.Helper()
	return &App{rootDir: rootDir, configDir: t.TempDir()}
}

func TestCaptureModEnableProfileRecordsOrderAndStates(t *testing.T) {
	root := t.TempDir()
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: `workshop\\123.vpk`, Value: "0"},
		{Name: "b.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)

	profile, err := a.CaptureModEnableProfile("写实包", "夜间写实")
	if err != nil {
		t.Fatalf("capture profile: %v", err)
	}
	if profile.ID == "" {
		t.Fatal("expected the captured profile to get an id")
	}
	if profile.CreatedAt == "" || profile.UpdatedAt == "" {
		t.Fatal("expected the captured profile to record timestamps")
	}
	if len(profile.Entries) != 3 {
		t.Fatalf("expected all three addonlist entries, got %#v", profile.Entries)
	}
	want := []ModEnableProfileEntry{
		{Name: "a.vpk", Enabled: true},
		{Name: `workshop\\123.vpk`, Enabled: false},
		{Name: "b.vpk", Enabled: true},
	}
	for index, entry := range want {
		if profile.Entries[index] != entry {
			t.Fatalf("entry %d: expected %#v, got %#v", index, entry, profile.Entries[index])
		}
	}
}

func TestCaptureModEnableProfileRequiresAddonList(t *testing.T) {
	a := newProfileTestApp(t, t.TempDir())
	if _, err := a.CaptureModEnableProfile("空方案", ""); err == nil {
		t.Fatal("expected capturing without addonlist.txt to fail")
	}
}

func TestModEnableProfilesPersistAndDelete(t *testing.T) {
	root := t.TempDir()
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{{Name: "a.vpk", Value: "1"}})
	a := newProfileTestApp(t, root)
	profile, err := a.CaptureModEnableProfile("方案一", "")
	if err != nil {
		t.Fatalf("capture profile: %v", err)
	}

	reloaded := newProfileTestApp(t, root)
	reloaded.configDir = a.configDir
	list, err := reloaded.ListModEnableProfiles()
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(list) != 1 || list[0].ID != profile.ID || list[0].Name != "方案一" {
		t.Fatalf("expected the captured profile to persist, got %#v", list)
	}

	if err := reloaded.DeleteModEnableProfile(profile.ID); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	list, err = reloaded.ListModEnableProfiles()
	if err != nil {
		t.Fatalf("list profiles after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected the profile to be gone, got %#v", list)
	}
}

func TestApplyModEnableProfileRestoresStatesAndOrder(t *testing.T) {
	root := t.TempDir()
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "0"},
		{Name: "c.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)
	profile, err := a.CaptureModEnableProfile("写实包", "")
	if err != nil {
		t.Fatalf("capture profile: %v", err)
	}

	// 模拟用户改动：换序、翻转开关、删掉 c.vpk、新增 d.vpk。
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "b.vpk", Value: "1"},
		{Name: "a.vpk", Value: "0"},
		{Name: "d.vpk", Value: "1"},
	})

	result, err := a.ApplyModEnableProfile(profile.ID)
	if err != nil {
		t.Fatalf("apply profile: %v", err)
	}
	if result.AppliedCount != 2 || result.AddedCount != 1 || result.KeptCount != 1 {
		t.Fatalf("unexpected apply counts: %#v", result)
	}
	if result.BackupName == "" {
		t.Fatal("expected applying a profile to create a backup first")
	}

	list, _, err := a.readAddonList()
	if err != nil {
		t.Fatalf("read addonlist: %v", err)
	}
	want := []AddonListItem{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "0"},
		{Name: "c.vpk", Value: "1"},
		{Name: "d.vpk", Value: "1"},
	}
	if len(list) != len(want) {
		t.Fatalf("expected %d entries, got %#v", len(want), list)
	}
	for index, entry := range want {
		if list[index].Name != entry.Name || list[index].Value != entry.Value {
			t.Fatalf("entry %d: expected %#v, got %#v", index, entry, list[index])
		}
	}

	backups, err := a.ListAddonListBackups()
	if err != nil {
		t.Fatalf("list addonlist backups: %v", err)
	}
	if len(backups) == 0 || backups[0].Kind != "before-profile-apply" {
		t.Fatalf("expected a before-profile-apply backup, got %#v", backups)
	}
}

func TestModEnableProfileExportImportAssignsNewID(t *testing.T) {
	sourceRoot := t.TempDir()
	writeConflictTestAddonList(t, sourceRoot, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "0"},
	})
	source := newProfileTestApp(t, sourceRoot)
	profile, err := source.CaptureModEnableProfile("分享包", "给朋友")
	if err != nil {
		t.Fatalf("capture profile: %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), "shared.l4d2profile.json")
	if err := source.ExportModEnableProfileToFile(profile.ID, exportPath); err != nil {
		t.Fatalf("export profile: %v", err)
	}

	targetRoot := t.TempDir()
	writeConflictTestAddonList(t, targetRoot, []conflictTestAddonListEntry{{Name: "a.vpk", Value: "0"}})
	target := newProfileTestApp(t, targetRoot)
	imported, err := target.ImportModEnableProfileFromFile(exportPath)
	if err != nil {
		t.Fatalf("import profile: %v", err)
	}
	if imported.ID == profile.ID || imported.ID == "" {
		t.Fatalf("expected a fresh id on import, got %q (source %q)", imported.ID, profile.ID)
	}
	if imported.Name != "分享包" || imported.Description != "给朋友" {
		t.Fatalf("expected name and description to survive the round trip, got %#v", imported)
	}
	if len(imported.Entries) != 2 || imported.Entries[1].Name != "b.vpk" || imported.Entries[1].Enabled {
		t.Fatalf("expected entries to survive the round trip, got %#v", imported.Entries)
	}

	list, err := target.ListModEnableProfiles()
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(list) != 1 || list[0].ID != imported.ID {
		t.Fatalf("expected the imported profile to be stored, got %#v", list)
	}
}

func TestImportModEnableProfileRejectsInvalidPayload(t *testing.T) {
	target := newProfileTestApp(t, t.TempDir())
	brokenPath := filepath.Join(t.TempDir(), "broken.l4d2profile.json")
	if err := os.WriteFile(brokenPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ImportModEnableProfileFromFile(brokenPath); err == nil {
		t.Fatal("expected malformed json to be rejected")
	}

	emptyPath := filepath.Join(t.TempDir(), "empty.l4d2profile.json")
	if err := os.WriteFile(emptyPath, []byte(`{"name":"没有条目","entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ImportModEnableProfileFromFile(emptyPath); err == nil {
		t.Fatal("expected a profile without entries to be rejected")
	}
}

func TestCaptureModEnableProfileWithAutomationKeepsGroupsAndDependencies(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "master.vpk", "dep.vpk", "other.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "master.vpk", Value: "1"},
		{Name: "dep.vpk", Value: "0"},
		{Name: "other.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)
	if _, err := a.CaptureModStrategyGroup("角色包", "", "single", []string{paths[0], paths[2]}); err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if _, err := a.SetModDependencies(paths[0], []string{paths[1]}); err != nil {
		t.Fatalf("set dependencies: %v", err)
	}

	plain, err := a.CaptureModEnableProfile("不带自动化", "")
	if err != nil {
		t.Fatalf("capture plain profile: %v", err)
	}
	if plain.IncludesAutomation || len(plain.Groups) != 0 || len(plain.Dependencies) != 0 {
		t.Fatalf("expected a plain profile to skip automation, got %#v", plain)
	}

	withAutomation, err := a.CaptureModEnableProfileWithAutomation("整套方案", "", true)
	if err != nil {
		t.Fatalf("capture profile with automation: %v", err)
	}
	if !withAutomation.IncludesAutomation {
		t.Fatal("expected the profile to record that automation was included")
	}
	if len(withAutomation.Groups) != 1 || withAutomation.Groups[0].Name != "角色包" {
		t.Fatalf("expected the strategy group in the profile, got %#v", withAutomation.Groups)
	}
	if len(withAutomation.Dependencies) != 1 || withAutomation.Dependencies[0].Key != "master.vpk" {
		t.Fatalf("expected the dependency record in the profile, got %#v", withAutomation.Dependencies)
	}
}

func TestApplyModEnableProfileRestoresAutomation(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "master.vpk", "dep.vpk", "other.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "master.vpk", Value: "1"},
		{Name: "dep.vpk", Value: "0"},
		{Name: "other.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)
	group, err := a.CaptureModStrategyGroup("角色包", "", "single", []string{paths[0], paths[2]})
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if _, err := a.SetModDependencies(paths[0], []string{paths[1]}); err != nil {
		t.Fatalf("set dependencies: %v", err)
	}
	profile, err := a.CaptureModEnableProfileWithAutomation("整套方案", "", true)
	if err != nil {
		t.Fatalf("capture profile: %v", err)
	}

	// 用户后来删掉了策略组与依赖声明。
	if err := a.DeleteModStrategyGroup(group.ID); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	if err := a.DeleteModDependencies("master.vpk"); err != nil {
		t.Fatalf("delete dependencies: %v", err)
	}

	// 先应用一个不含自动化的方案：不应改动策略组与依赖。
	plain, err := a.CaptureModEnableProfile("仅开关", "")
	if err != nil {
		t.Fatalf("capture plain profile: %v", err)
	}
	if _, err := a.ApplyModEnableProfile(plain.ID); err != nil {
		t.Fatalf("apply plain profile: %v", err)
	}
	if groups, _ := a.ListModStrategyGroups(); len(groups) != 0 {
		t.Fatalf("expected the plain profile not to restore groups, got %#v", groups)
	}

	result, err := a.ApplyModEnableProfile(profile.ID)
	if err != nil {
		t.Fatalf("apply profile with automation: %v", err)
	}
	if result.RestoredGroups != 1 || result.RestoredDependencies != 1 {
		t.Fatalf("expected automation to be restored, got %#v", result)
	}

	groups, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "角色包" || groups[0].Strategy != "single" {
		t.Fatalf("expected the strategy group to come back, got %#v", groups)
	}
	dependencies, err := a.ListModDependencies()
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(dependencies) != 1 || len(dependencies[0].Dependencies) != 1 {
		t.Fatalf("expected the dependency record to come back, got %#v", dependencies)
	}
}

func TestModEnableProfileExportImportCarriesAutomation(t *testing.T) {
	sourceRoot := t.TempDir()
	paths := writeModGroupFixture(t, sourceRoot, "master.vpk", "dep.vpk")
	writeConflictTestAddonList(t, sourceRoot, []conflictTestAddonListEntry{
		{Name: "master.vpk", Value: "1"},
		{Name: "dep.vpk", Value: "1"},
	})
	source := newProfileTestApp(t, sourceRoot)
	if _, err := source.CaptureModStrategyGroup("分享组", "", "all", paths); err != nil {
		t.Fatalf("capture group: %v", err)
	}
	profile, err := source.CaptureModEnableProfileWithAutomation("分享方案", "", true)
	if err != nil {
		t.Fatalf("capture profile: %v", err)
	}

	exportPath := filepath.Join(t.TempDir(), "shared-with-automation.l4d2profile.json")
	if err := source.ExportModEnableProfileToFile(profile.ID, exportPath); err != nil {
		t.Fatalf("export profile: %v", err)
	}

	targetRoot := t.TempDir()
	writeConflictTestAddonList(t, targetRoot, []conflictTestAddonListEntry{{Name: "master.vpk", Value: "0"}})
	target := newProfileTestApp(t, targetRoot)
	imported, err := target.ImportModEnableProfileFromFile(exportPath)
	if err != nil {
		t.Fatalf("import profile: %v", err)
	}
	if !imported.IncludesAutomation || len(imported.Groups) != 1 {
		t.Fatalf("expected automation to survive the round trip, got %#v", imported)
	}
}
