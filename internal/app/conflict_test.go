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

func TestCollectConflictFilesystemPathsKeepsSpecialDirectorySemanticsForCustomRoots(t *testing.T) {
	for _, rootBase := range []string{"workshop", "disabled"} {
		root := filepath.Join(t.TempDir(), rootBase)
		if err := os.MkdirAll(filepath.Join(root, "workshop", "nested"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "disabled", "nested"), 0o755); err != nil {
			t.Fatal(err)
		}

		rootVPK := filepath.Join(root, "root.vpk")
		workshopVPK := filepath.Join(root, "workshop", "nested", "workshop.vpk")
		disabledVPK := filepath.Join(root, "disabled", "nested", "disabled.vpk")
		for _, path := range []string{rootVPK, workshopVPK, disabledVPK} {
			if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		paths := collectConflictFilesystemPaths(root)
		if len(paths) != 3 {
			t.Fatalf("root %q: expected 3 VPK paths, got %d: %#v", rootBase, len(paths), paths)
		}

		for _, want := range []string{rootVPK, workshopVPK, disabledVPK} {
			if !containsString(paths, want) {
				t.Fatalf("root %q: missing %s in %#v", rootBase, want, paths)
			}
		}

		rootFile := (&App{}).conflictBaselineFile(rootVPK, root, nil, false)
		if rootFile.Location != "root" {
			t.Fatalf("root %q: direct VPK was classified as %q", rootBase, rootFile.Location)
		}
	}
}

func TestConflictBaselineFilesystemFallbackIncludesDiskFilesWhenCacheIsEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(root, "active.vpk")
	workshop := filepath.Join(root, "workshop", "workshop.vpk")
	for _, path := range []string{active, workshop} {
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := &App{rootDir: root}
	paths := a.collectConflictBaselineVPKPaths([]ConflictBaselineRule{{Type: "not_disabled"}}, "or")
	if len(paths) != 2 {
		t.Fatalf("expected disk VPKs even before initial cache scan, got %d: %#v", len(paths), paths)
	}
	if !containsString(paths, active) || !containsString(paths, workshop) {
		t.Fatalf("baseline omitted disk VPK: %#v", paths)
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

func TestScopedConflictOwnersComparesTargetsWithEnabledBaseline(t *testing.T) {
	target := filepath.Join("C:\\mods", "target.vpk")
	active := filepath.Join("C:\\mods", "active.vpk")
	other := filepath.Join("C:\\mods", "other.vpk")
	targets := pathSet([]string{target})
	baseline := pathSet([]string{target, active, other})

	owners := scopedConflictOwners([]string{target, active, other}, targets, baseline)
	if len(owners) != 3 {
		t.Fatalf("expected selected enabled target plus two enabled baseline owners, got %#v", owners)
	}

	// A target that is itself enabled must not be considered a conflict with
	// itself when it is the only owner of a file.
	onlySelf := scopedConflictOwners([]string{target}, pathSet([]string{target}), pathSet([]string{target}))
	if len(onlySelf) != 0 {
		t.Fatalf("expected self-only ownership to be ignored, got %#v", onlySelf)
	}

	// Two filtered disabled targets are not compared with each other; there is
	// no active baseline owner in this case.
	secondTarget := filepath.Join("C:\\mods", "second-target.vpk")
	if owners := scopedConflictOwners([]string{target, secondTarget}, pathSet([]string{target, secondTarget}), map[string]struct{}{}); len(owners) != 0 {
		t.Fatalf("expected target-target-only ownership to be ignored, got %#v", owners)
	}

	// A filtered target that is not in the selected baseline (for example, a
	// game-disabled target while the baseline is "游戏内开启") must not leak
	// into the result or qualify a conflict group.
	disabledTarget := filepath.Join("C:\\mods", "disabled-target.vpk")
	if owners := scopedConflictOwners([]string{disabledTarget, active}, pathSet([]string{disabledTarget}), pathSet([]string{active})); len(owners) != 0 {
		t.Fatalf("expected a disabled target to be excluded from enabled-baseline results, got %#v", owners)
	}

	// Baseline-only conflicts are unrelated to the current filtered target and
	// must stay hidden in a scoped analysis.
	if owners := scopedConflictOwners([]string{active, other}, pathSet([]string{disabledTarget}), baseline); len(owners) != 0 {
		t.Fatalf("expected baseline-only ownership to be ignored, got %#v", owners)
	}
}

func TestCollectEnabledConflictVPKPathsExcludesDisabledAndUnknown(t *testing.T) {
	a := &App{}
	activePath := filepath.Join(t.TempDir(), "active.vpk")
	disabledPath := filepath.Join(t.TempDir(), "disabled.vpk")
	unknownPath := filepath.Join(t.TempDir(), "unknown.vpk")
	for _, path := range []string{activePath, disabledPath, unknownPath} {
		a.vpkCache.Store(path, &VPKFileCache{File: VPKFile{Path: path}})
	}
	active := &VPKFileCache{File: VPKFile{Path: activePath, GameEnabled: true, GameStateKnown: true, Location: "root"}}
	disabled := &VPKFileCache{File: VPKFile{Path: disabledPath, GameEnabled: true, GameStateKnown: true, Location: "disabled"}}
	unknown := &VPKFileCache{File: VPKFile{Path: unknownPath, GameEnabled: false, Location: "root"}}
	a.vpkCache.Store(activePath, active)
	a.vpkCache.Store(disabledPath, disabled)
	a.vpkCache.Store(unknownPath, unknown)

	paths := a.collectEnabledConflictVPKPaths()
	if len(paths) != 1 || filepath.Clean(paths[0]) != filepath.Clean(activePath) {
		t.Fatalf("expected only active root VPK, got %#v", paths)
	}
}

func TestConflictBaselineRulesMatchLocationStateAndTags(t *testing.T) {
	a := &App{}
	rootEnabled := filepath.Join(t.TempDir(), "root-enabled.vpk")
	rootUnknown := filepath.Join(t.TempDir(), "root-unknown.vpk")
	workshopEnabled := filepath.Join(t.TempDir(), "workshop", "workshop-enabled.vpk")
	disabled := filepath.Join(t.TempDir(), "disabled", "disabled.vpk")
	files := []struct {
		path string
		file VPKFile
	}{
		{rootEnabled, VPKFile{Path: rootEnabled, Location: "root", GameStateKnown: true, GameEnabled: true, PrimaryTag: "人物", SecondaryTags: []string{"Bill"}}},
		{rootUnknown, VPKFile{Path: rootUnknown, Location: "root", PrimaryTag: "武器", SecondaryTags: []string{"AK47"}}},
		{workshopEnabled, VPKFile{Path: workshopEnabled, Location: "workshop", GameStateKnown: true, GameEnabled: true, SecondaryTags: []string{"HUD"}}},
		{disabled, VPKFile{Path: disabled, Location: "disabled", GameStateKnown: true, GameEnabled: true, PrimaryTag: "人物", SecondaryTags: []string{"Bill"}}},
	}
	for _, item := range files {
		a.vpkCache.Store(item.path, &VPKFileCache{File: item.file})
	}

	assertPaths := func(name string, rules []ConflictBaselineRule, mode string, want ...string) {
		t.Helper()
		got := a.collectConflictBaselineVPKPaths(rules, mode)
		if len(got) != len(want) {
			t.Fatalf("%s: expected %d paths, got %d: %#v", name, len(want), len(got), got)
		}
		for _, path := range want {
			if !containsString(got, path) {
				t.Fatalf("%s: missing %s in %#v", name, path, got)
			}
		}
	}

	assertPaths("enabled", []ConflictBaselineRule{{Type: "enabled"}}, "or", rootEnabled, workshopEnabled)
	assertPaths("not_disabled", []ConflictBaselineRule{{Type: "not_disabled"}}, "or", rootEnabled, rootUnknown, workshopEnabled)
	assertPaths("root", []ConflictBaselineRule{{Type: "root"}}, "or", rootEnabled, rootUnknown)
	assertPaths("workshop", []ConflictBaselineRule{{Type: "workshop"}}, "or", workshopEnabled)
	assertPaths("tag primary or secondary", []ConflictBaselineRule{{Type: "tag", Value: "Bill"}}, "or", rootEnabled, disabled)
	assertPaths("and enabled root", []ConflictBaselineRule{{Type: "enabled"}, {Type: "root"}}, "and", rootEnabled)
	assertPaths("or enabled or HUD", []ConflictBaselineRule{{Type: "enabled"}, {Type: "tag", Value: "HUD"}}, "or", rootEnabled, workshopEnabled)
}

func TestNormalizeConflictBaselineRulesDefaultsAndRejectsInvalidValues(t *testing.T) {
	rules, mode, err := normalizeConflictBaselineRules(nil, "")
	if err != nil || mode != "or" || len(rules) != 1 || rules[0].Type != "enabled" {
		t.Fatalf("unexpected defaults: rules=%#v mode=%q err=%v", rules, mode, err)
	}
	if _, _, err := normalizeConflictBaselineRules([]ConflictBaselineRule{{Type: "tag"}}, "or"); err == nil {
		t.Fatal("expected tag rule without value to fail")
	}
	if _, _, err := normalizeConflictBaselineRules([]ConflictBaselineRule{{Type: "unknown"}}, "or"); err == nil {
		t.Fatal("expected unknown rule to fail")
	}
	if _, _, err := normalizeConflictBaselineRules([]ConflictBaselineRule{{Type: "enabled"}}, "xor"); err == nil {
		t.Fatal("expected unknown match mode to fail")
	}
	rules, _, err = normalizeConflictBaselineRules([]ConflictBaselineRule{{Type: "workshop"}, {Type: "workshop"}}, "and")
	if err != nil || len(rules) != 1 {
		t.Fatalf("expected duplicate rules to be removed: %#v err=%v", rules, err)
	}
}
