package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func newLoadOrderTestApp(t *testing.T, content string) (*App, string) {
	t.Helper()
	addonsDir := filepath.Join(t.TempDir(), "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return &App{rootDir: addonsDir}, path
}

func loadOrderKeys(entries []AddonListLoadOrderEntry) []string {
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return keys
}

func TestPreviewAddonListLoadOrderPolicyKeepsStableGroups(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"root-a.vpk\"\t\t\"1\"\n\t\"workshop\\100.vpk\"\t\t\"0\"\n\t\"root-b.vpk\"\t\t\"1\"\n\t\"workshop\\200.vpk\"\t\t\"1\"\n\t\"root-c.vpk\"\t\t\"0\"\n}\n")

	grouped, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{GroupWorkshop: true})
	if err != nil {
		t.Fatalf("preview grouped: %v", err)
	}
	if want := []string{"root-a.vpk", "workshop\\100.vpk", "workshop\\200.vpk", "root-b.vpk", "root-c.vpk"}; !reflect.DeepEqual(loadOrderKeys(grouped.Entries), want) {
		t.Fatalf("grouped = %#v, want %#v", loadOrderKeys(grouped.Entries), want)
	}

	rootFirst, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("preview root first: %v", err)
	}
	if want := []string{"root-a.vpk", "root-b.vpk", "root-c.vpk", "workshop\\100.vpk", "workshop\\200.vpk"}; !reflect.DeepEqual(loadOrderKeys(rootFirst.Entries), want) {
		t.Fatalf("root first = %#v, want %#v", loadOrderKeys(rootFirst.Entries), want)
	}
}

func TestRootFirstDoesNotPromoteOtherSubdirectories(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"nested\\custom.vpk\"\t\t\"1\"\n\t\"workshop\\100.vpk\"\t\t\"1\"\n\t\"root-a.vpk\"\t\t\"1\"\n}\n")
	preview, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("preview root first: %v", err)
	}
	if want := []string{"root-a.vpk", "nested\\custom.vpk", "workshop\\100.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("root first = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}
	if preview.Entries[1].IsRoot || preview.Entries[1].IsWorkshop {
		t.Fatalf("nested mod location flags = %#v, want neither root nor workshop", preview.Entries[1])
	}
}

func TestAddonListLoadOrderPolicyStableStateOrderingPreservesValues(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"root-a.vpk\"\t\t\"1\"\n\t\"workshop\\100.vpk\"\t\t\"0\"\n\t\"root-b.vpk\"\t\t\"1\"\n\t\"workshop\\200.vpk\"\t\t\"1\"\n\t\"root-c.vpk\"\t\t\"0\"\n}\n")

	enabledFirst, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{StateOrder: addonListStateOrderEnabledFirst})
	if err != nil {
		t.Fatalf("preview enabled-first: %v", err)
	}
	if want := []string{"root-a.vpk", "root-b.vpk", "workshop\\200.vpk", "workshop\\100.vpk", "root-c.vpk"}; !reflect.DeepEqual(loadOrderKeys(enabledFirst.Entries), want) {
		t.Fatalf("enabled-first = %#v, want %#v", loadOrderKeys(enabledFirst.Entries), want)
	}
	for _, entry := range enabledFirst.Entries {
		if entry.Key == "workshop\\100.vpk" && entry.Value != "0" {
			t.Fatalf("disabled workshop value = %q, want 0", entry.Value)
		}
		if entry.Key == "workshop\\200.vpk" && entry.Value != "1" {
			t.Fatalf("enabled workshop value = %q, want 1", entry.Value)
		}
	}

	disabledFirst, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{StateOrder: addonListStateOrderDisabledFirst})
	if err != nil {
		t.Fatalf("preview disabled-first: %v", err)
	}
	if want := []string{"workshop\\100.vpk", "root-c.vpk", "root-a.vpk", "root-b.vpk", "workshop\\200.vpk"}; !reflect.DeepEqual(loadOrderKeys(disabledFirst.Entries), want) {
		t.Fatalf("disabled-first = %#v, want %#v", loadOrderKeys(disabledFirst.Entries), want)
	}

	if _, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{StateOrder: "unexpected"}); err == nil {
		t.Fatal("unexpected state order should fail")
	}
}

func TestAddonListLoadOrderConstraintsAreStableAndRejectCycles(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"root-a.vpk\"\t\t\"1\"\n\t\"workshop\\100.vpk\"\t\t\"0\"\n\t\"root-b.vpk\"\t\t\"1\"\n\t\"workshop\\200.vpk\"\t\t\"1\"\n}\n")
	preview, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{
		RootFirst: true,
		Constraints: []AddonListLoadOrderConstraint{{
			Before: "workshop\\200.vpk",
			After:  "root-a.vpk",
		}},
	})
	if err != nil {
		t.Fatalf("preview constrained: %v", err)
	}
	if want := []string{"root-b.vpk", "workshop\\100.vpk", "workshop\\200.vpk", "root-a.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("constrained = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}

	_, err = app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{Constraints: []AddonListLoadOrderConstraint{
		{Before: "root-a.vpk", After: "root-b.vpk"},
		{Before: "root-b.vpk", After: "root-a.vpk"},
	}})
	if err == nil {
		t.Fatal("cycle policy unexpectedly succeeded")
	}
}

func TestApplyAddonListLoadOrderPolicyWritesAndSyncsProtectedSnapshot(t *testing.T) {
	app, path := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"workshop\\100.vpk\"\t\t\"0\"\n\t\"root-a.vpk\"\t\t\"1\"\n}\n")
	app.addonListGuardEnabled = true
	preview, err := app.ApplyAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("apply policy: %v", err)
	}
	if want := []string{"root-a.vpk", "workshop\\100.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("applied = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(addonListManagedSnapshotPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(snapshot) != string(content) {
		t.Fatal("managed snapshot was not synchronized")
	}
	for _, entry := range preview.Entries {
		if entry.Key == "root-a.vpk" && entry.Value != "1" {
			t.Fatalf("root-a state = %q, want 1", entry.Value)
		}
		if entry.Key == "workshop\\100.vpk" && entry.Value != "0" {
			t.Fatalf("workshop state = %q, want 0", entry.Value)
		}
	}
}
