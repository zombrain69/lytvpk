package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectConflictVPKPathsForScopedSelection(t *testing.T) {
	root := t.TempDir()
	workshop := filepath.Join(root, "workshop")
	disabled := filepath.Join(root, "disabled")
	if err := os.MkdirAll(workshop, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(disabled, 0o755); err != nil {
		t.Fatal(err)
	}

	rootVPK := filepath.Join(root, "root.vpk")
	workshopVPK := filepath.Join(workshop, "workshop.vpk")
	disabledVPK := filepath.Join(disabled, "disabled.vpk")
	for _, path := range []string{rootVPK, workshopVPK, disabledVPK} {
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := &App{rootDir: root}
	paths, err := a.collectConflictVPKPaths([]string{workshopVPK, rootVPK, workshopVPK, disabledVPK})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 unique scoped paths, got %d: %#v", len(paths), paths)
	}
	for _, want := range []string{rootVPK, workshopVPK, disabledVPK} {
		found := false
		for _, path := range paths {
			if filepath.Clean(path) == filepath.Clean(want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("scoped result is missing %s: %#v", want, paths)
		}
	}
}

func TestCollectConflictVPKPathsRejectsOutsideDirectory(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.vpk")
	if err := os.WriteFile(outside, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &App{rootDir: root}
	if _, err := a.collectConflictVPKPaths([]string{outside}); err == nil {
		t.Fatal("expected a path outside the selected addons directory to be rejected")
	}
}

func TestCheckConflictsForPathsRejectsOversizedSelection(t *testing.T) {
	a := &App{}
	paths := make([]string, scopedConflictMaxVPKs+1)
	if _, err := a.CheckConflictsForPaths(paths); err == nil {
		t.Fatal("expected oversized scoped conflict selection to be rejected")
	}
}
