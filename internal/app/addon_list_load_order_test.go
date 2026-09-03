package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
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

func TestPreviewAddonListLoadOrderPolicyDeduplicatesSameStateEntries(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"root-a.vpk\"\t\t\"1\"\n\t\"WORKSHOP/123.vpk\"\t\t\"0\"\n\t\"workshop\\123.vpk\"\t\t\"0\"\n\t\"root-b.vpk\"\t\t\"1\"\n}\n")

	preview, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("preview duplicated addonlist: %v", err)
	}
	if want := []string{"root-a.vpk", "root-b.vpk", "workshop\\123.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("deduplicated preview = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}
	if got := preview.Entries[2].Value; got != "0" {
		t.Fatalf("deduplicated workshop state = %q, want 0", got)
	}
}

func TestGetAddonListLoadOrderEntriesDeduplicatesSameStateEntries(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"root-a.vpk\"\t\t\"1\"\n\t\"WORKSHOP/123.vpk\"\t\t\"0\"\n\t\"workshop\\123.vpk\"\t\t\"0\"\n}\n")

	entries, err := app.GetAddonListLoadOrderEntries()
	if err != nil {
		t.Fatalf("get duplicated addonlist entries: %v", err)
	}
	if want := []string{"root-a.vpk", "workshop\\123.vpk"}; !reflect.DeepEqual(loadOrderKeys(entries), want) {
		t.Fatalf("current entries = %#v, want %#v", loadOrderKeys(entries), want)
	}
}

func TestApplyAddonListLoadOrderPolicyWritesDeduplicatedEntries(t *testing.T) {
	app, path := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"workshop\\123.vpk\"\t\t\"1\"\n\t\"root-a.vpk\"\t\t\"0\"\n\t\"WORKSHOP/123.vpk\"\t\t\"1\"\n}\n")

	preview, err := app.ApplyAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("apply duplicated addonlist: %v", err)
	}
	if want := []string{"root-a.vpk", "workshop\\123.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
		t.Fatalf("applied deduplicated order = %#v, want %#v", loadOrderKeys(preview.Entries), want)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := parseAddonListItems(string(content)); !reflect.DeepEqual(got, []AddonListItem{
		{Name: "root-a.vpk", Value: "0"},
		{Name: "workshop\\123.vpk", Value: "1"},
	}) {
		t.Fatalf("written addonlist = %#v", got)
	}
}

func TestPreviewAddonListLoadOrderPolicyRejectsConflictingDuplicateStates(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"workshop\\123.vpk\"\t\t\"0\"\n\t\"WORKSHOP/123.vpk\"\t\t\"1\"\n}\n")

	_, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{})
	if err == nil {
		t.Fatal("conflicting duplicate states unexpectedly succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "状态冲突") || !strings.Contains(got, "第 1 条") || !strings.Contains(got, "第 2 条") {
		t.Fatalf("conflicting duplicate error = %q, want state conflict with entry positions", got)
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
	if want := []string{"workshop\\200.vpk", "root-a.vpk", "root-b.vpk", "workshop\\100.vpk"}; !reflect.DeepEqual(loadOrderKeys(preview.Entries), want) {
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

func TestAddonListLoadOrderConstraintsMoveSourcesBeforeEarlierAnchor(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"anchor.vpk\"\t\t\"1\"\n\t\"gap-a.vpk\"\t\t\"1\"\n\t\"gap-b.vpk\"\t\t\"0\"\n\t\"source-a.vpk\"\t\t\"1\"\n\t\"source-b.vpk\"\t\t\"0\"\n\t\"source-c.vpk\"\t\t\"1\"\n}\n")
	preview, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{
		Constraints: []AddonListLoadOrderConstraint{
			{Before: "source-a.vpk", After: "anchor.vpk"},
			{Before: "source-b.vpk", After: "anchor.vpk"},
			{Before: "source-c.vpk", After: "anchor.vpk"},
		},
	})
	if err != nil {
		t.Fatalf("preview source-before-anchor: %v", err)
	}
	want := []string{"source-a.vpk", "source-b.vpk", "source-c.vpk", "anchor.vpk", "gap-a.vpk", "gap-b.vpk"}
	if got := loadOrderKeys(preview.Entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("source-before-anchor = %#v, want %#v", got, want)
	}
	for _, entry := range preview.Entries {
		if entry.Key == "anchor.vpk" && entry.Order != 4 {
			t.Fatalf("anchor order = %d, want 4", entry.Order)
		}
	}
}

func TestAddonListLoadOrderAnchorMoveAfterKeepsSourcesAdjacent(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"source-a.vpk\"\t\t\"1\"\n\t\"anchor.vpk\"\t\t\"1\"\n\t\"gap-a.vpk\"\t\t\"1\"\n\t\t\"source-b.vpk\"\t\t\"0\"\n\t\"gap-b.vpk\"\t\t\"1\"\n}\n")
	preview, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{
		Constraints: []AddonListLoadOrderConstraint{
			{Before: "anchor.vpk", After: "source-a.vpk", AnchorMove: "after"},
			{Before: "anchor.vpk", After: "source-b.vpk", AnchorMove: "after"},
		},
	})
	if err != nil {
		t.Fatalf("preview source-after-anchor: %v", err)
	}
	want := []string{"anchor.vpk", "source-a.vpk", "source-b.vpk", "gap-a.vpk", "gap-b.vpk"}
	if got := loadOrderKeys(preview.Entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("source-after-anchor = %#v, want %#v", got, want)
	}
}

func TestAddonListLoadOrderAnchorMoveBeforeKeepsSourcesAdjacent(t *testing.T) {
	app, _ := newLoadOrderTestApp(t, "\"AddonList\"\n{\n\t\"source-a.vpk\"\t\t\"1\"\n\t\"gap-a.vpk\"\t\t\"1\"\n\t\"anchor.vpk\"\t\t\"1\"\n\t\"gap-b.vpk\"\t\t\"0\"\n\t\"source-b.vpk\"\t\t\"1\"\n}\n")
	preview, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{
		Constraints: []AddonListLoadOrderConstraint{
			{Before: "source-a.vpk", After: "anchor.vpk", AnchorMove: "before"},
			{Before: "source-b.vpk", After: "anchor.vpk", AnchorMove: "before"},
		},
	})
	if err != nil {
		t.Fatalf("preview source-before-anchor: %v", err)
	}
	want := []string{"gap-a.vpk", "source-a.vpk", "source-b.vpk", "anchor.vpk", "gap-b.vpk"}
	if got := loadOrderKeys(preview.Entries); !reflect.DeepEqual(got, want) {
		t.Fatalf("source-before-anchor = %#v, want %#v", got, want)
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

func TestApplyAddonListLoadOrderPolicyPreservesGBKEncoding(t *testing.T) {
	content := "\"AddonList\"\r\n{\r\n\t\"workshop\\123.vpk\"\t\"1\"\r\n\t\"根目录测试.vpk\"\t\"0\"\r\n}\r\n"
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	app, path := newLoadOrderTestApp(t, "")
	if err := os.WriteFile(path, encoded, 0644); err != nil {
		t.Fatal(err)
	}

	preview, err := app.ApplyAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("apply GBK policy: %v", err)
	}
	if got, want := loadOrderKeys(preview.Entries), []string{"根目录测试.vpk", "workshop\\123.vpk"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered keys = %#v, want %#v", got, want)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.Valid(updated) {
		t.Fatal("sorting rewrote a GBK addonlist.txt as UTF-8")
	}
	decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), updated)
	if err != nil {
		t.Fatal(err)
	}
	if got := parseAddonListItems(string(decoded)); !reflect.DeepEqual(got[0], (AddonListItem{Name: "根目录测试.vpk", Value: "0"})) {
		t.Fatalf("decoded first entry = %#v, want 根目录测试.vpk disabled", got[0])
	}
}
