package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type conflictTestAddonListEntry struct {
	Name  string
	Value string
}

func writeConflictTestAddonList(t *testing.T, rootDir string, entries []conflictTestAddonListEntry) {
	t.Helper()

	var builder strings.Builder
	builder.WriteString("\"AddonList\"\n{\n")
	for _, entry := range entries {
		builder.WriteString("\t\"" + entry.Name + "\"\t\t\"" + entry.Value + "\"\n")
	}
	builder.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(rootDir, "addonlist.txt"), []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("write addonlist fixture: %v", err)
	}
}

func TestDecideConflictOwnersTreatsUnknownOrderAsUndecidedConflict(t *testing.T) {
	owners := []conflictOwner{
		{Path: filepath.Join("C:\\mods", "a.vpk"), Index: 0, Known: true, Enabled: true},
		{Path: filepath.Join("C:\\mods", "b.vpk"), Index: -1, Known: false},
	}

	decision := decideConflictOwners(owners)

	if decision.Kind != conflictDecisionConflict {
		t.Fatalf("expected an undecided conflict, got %q", decision.Kind)
	}
	if len(decision.Owners) != 2 {
		t.Fatalf("expected both owners to stay in the conflict, got %#v", decision.Owners)
	}
}

func TestDecideConflictOwnersReportsHighestLoadOrderAsWinner(t *testing.T) {
	owners := []conflictOwner{
		{Path: "a.vpk", Index: 0, Known: true, Enabled: true},
		{Path: "b.vpk", Index: 3, Known: true, Enabled: true},
		{Path: "c.vpk", Index: 1, Known: true, Enabled: true},
	}

	decision := decideConflictOwners(owners)

	if decision.Kind != conflictDecisionOverride {
		t.Fatalf("expected a decided override, got %q", decision.Kind)
	}
	if decision.WinnerPath != "b.vpk" || decision.WinnerIndex != 3 {
		t.Fatalf("expected b.vpk (order 3) to win, got %q (order %d)", decision.WinnerPath, decision.WinnerIndex)
	}
	if len(decision.Owners) != 3 {
		t.Fatalf("expected every loaded owner to stay in the override group, got %#v", decision.Owners)
	}
}

func TestDecideConflictOwnersDropsDisabledParticipants(t *testing.T) {
	owners := []conflictOwner{
		{Path: "a.vpk", Index: 0, Known: true, Enabled: true},
		{Path: "b.vpk", Index: 1, Known: true, Enabled: false},
	}

	decision := decideConflictOwners(owners)

	if decision.Kind != conflictDecisionIgnore {
		t.Fatalf("expected a disabled participant to leave no conflict, got %q", decision.Kind)
	}
}

func TestConflictIgnoreSetMatchesExactPathsAndDirectoryPrefixes(t *testing.T) {
	set := newConflictIgnoreSet([]string{" Materials/Shared.VTF ", "models/", "  "})

	if !set.ShouldIgnore("materials/shared.vtf") {
		t.Fatal("expected an exact path to be ignored regardless of case and spacing")
	}
	if !set.ShouldIgnore("models/foo/bar.mdl") {
		t.Fatal("expected a directory prefix entry to ignore nested files")
	}
	if set.ShouldIgnore("materials/other.vtf") {
		t.Fatal("expected unrelated paths to stay reportable")
	}
	if set.ShouldIgnore("modelsx/foo.mdl") {
		t.Fatal("expected prefix matching to respect the directory boundary")
	}
}

func TestCheckConflictsPriorityAwareSplitsOverridesFromUndecidedConflicts(t *testing.T) {
	root := t.TempDir()
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
	})

	aPath := filepath.Join(root, "a.vpk")
	bPath := filepath.Join(root, "b.vpk")
	cPath := filepath.Join(root, "c.vpk")
	writeTestVPK(t, aPath, map[string][]byte{
		"materials/shared.vtf":    {1},
		"materials/undecided.vtf": {2},
	})
	writeTestVPK(t, bPath, map[string][]byte{"materials/shared.vtf": {3}})
	// c.vpk 没有 addonlist 条目：顺序未知，因此与 a.vpk 的重叠无法判定胜负。
	writeTestVPK(t, cPath, map[string][]byte{"materials/undecided.vtf": {4}})

	a := &App{rootDir: root}

	legacy, err := a.checkConflicts(conflictCheckRequest{})
	if err != nil {
		t.Fatalf("legacy conflict check failed: %v", err)
	}
	if legacy.TotalConflicts != 2 || legacy.TotalOverrides != 0 {
		t.Fatalf("expected the default mode to keep both overlaps as conflicts, got %d conflicts / %d overrides",
			legacy.TotalConflicts, legacy.TotalOverrides)
	}

	result, err := a.checkConflicts(conflictCheckRequest{priorityAware: true})
	if err != nil {
		t.Fatalf("priority-aware conflict check failed: %v", err)
	}

	if result.TotalOverrides != 1 || len(result.OverrideGroups) != 1 {
		t.Fatalf("expected exactly one decided override, got %d / %#v", result.TotalOverrides, result.OverrideGroups)
	}
	override := result.OverrideGroups[0]
	if override.Winner.Path != bPath {
		t.Fatalf("expected b.vpk (recorded later in addonlist) to win, got %q", override.Winner.Path)
	}
	if override.Winner.Order != 1 {
		t.Fatalf("expected the winner order to be 1, got %d", override.Winner.Order)
	}
	if len(override.VpkFiles) != 2 {
		t.Fatalf("expected both participants in the override group, got %#v", override.VpkFiles)
	}
	if len(override.Files) != 1 || override.Files[0] != "materials/shared.vtf" {
		t.Fatalf("expected only the decided file in the override group, got %#v", override.Files)
	}

	if result.TotalConflicts != 1 || len(result.ConflictGroups) != 1 {
		t.Fatalf("expected exactly one undecided conflict, got %d / %#v", result.TotalConflicts, result.ConflictGroups)
	}
	conflict := result.ConflictGroups[0]
	if len(conflict.VpkFiles) != 2 {
		t.Fatalf("expected the conflict to keep a.vpk and c.vpk, got %#v", conflict.VpkFiles)
	}
	paths := map[string]bool{}
	for _, file := range conflict.VpkFiles {
		paths[file.Path] = true
	}
	if !paths[aPath] || !paths[cPath] {
		t.Fatalf("expected the undecided conflict to reference a.vpk and c.vpk, got %#v", conflict.VpkFiles)
	}
	if len(conflict.Files) != 1 || conflict.Files[0] != "materials/undecided.vtf" {
		t.Fatalf("expected only the undecided file in the conflict group, got %#v", conflict.Files)
	}
}

func TestCheckConflictsPriorityAwareHonoursExtraIgnoreFiles(t *testing.T) {
	root := t.TempDir()
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
	})
	writeTestVPK(t, filepath.Join(root, "a.vpk"), map[string][]byte{"materials/shared.vtf": {1}})
	writeTestVPK(t, filepath.Join(root, "b.vpk"), map[string][]byte{"materials/shared.vtf": {2}})

	a := &App{rootDir: root}
	result, err := a.checkConflicts(conflictCheckRequest{
		priorityAware: true,
		ignoreFiles:   newConflictIgnoreSet([]string{"materials/"}),
	})
	if err != nil {
		t.Fatalf("priority-aware conflict check failed: %v", err)
	}
	if result.TotalConflicts != 0 || result.TotalOverrides != 0 {
		t.Fatalf("expected the extra ignore list to suppress the overlap, got %d conflicts / %d overrides",
			result.TotalConflicts, result.TotalOverrides)
	}
}

func TestCheckConflictsWithOptionsFullScanEnablesPriorityModeWithoutTargets(t *testing.T) {
	root := t.TempDir()
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
	})
	writeTestVPK(t, filepath.Join(root, "a.vpk"), map[string][]byte{"materials/shared.vtf": {1}})
	writeTestVPK(t, filepath.Join(root, "b.vpk"), map[string][]byte{"materials/shared.vtf": {2}})

	a := &App{rootDir: root}

	// 不带 targetPaths 且未开启 fullScan 时保持历史行为：直接返回空结果。
	empty, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{})
	if err != nil {
		t.Fatalf("empty options must keep the historical empty result: %v", err)
	}
	if empty.TotalConflicts != 0 || empty.TotalOverrides != 0 || len(empty.ConflictGroups) != 0 {
		t.Fatalf("expected empty options to stay a no-op, got %#v", empty)
	}

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatalf("full-scan priority-aware check failed: %v", err)
	}
	if result.TotalConflicts != 0 || result.TotalOverrides != 1 {
		t.Fatalf("expected the full scan to reclassify the overlap as an override, got %d conflicts / %d overrides",
			result.TotalConflicts, result.TotalOverrides)
	}
}

func TestConfiguredIgnoreFilesApplyToEveryConflictCheck(t *testing.T) {
	root := t.TempDir()
	writeTestVPK(t, filepath.Join(root, "a.vpk"), map[string][]byte{
		"materials/shared.vtf": {1},
		"materials/kept.vtf":   {2},
	})
	writeTestVPK(t, filepath.Join(root, "b.vpk"), map[string][]byte{
		"materials/shared.vtf": {3},
		"materials/kept.vtf":   {4},
	})

	unconfigured := &App{rootDir: root}
	before, err := unconfigured.checkConflicts(conflictCheckRequest{})
	if err != nil {
		t.Fatalf("unconfigured conflict check failed: %v", err)
	}
	if before.TotalConflicts != 1 || before.ConflictGroups[0].FileCount != 2 {
		t.Fatalf("expected both overlapping files to be reported without configuration, got %#v", before.ConflictGroups)
	}

	configured := &App{rootDir: root, conflictIgnoreFiles: []string{"materials/shared.vtf"}}
	after, err := configured.checkConflicts(conflictCheckRequest{})
	if err != nil {
		t.Fatalf("configured conflict check failed: %v", err)
	}
	if after.TotalConflicts != 1 {
		t.Fatalf("expected the remaining overlap to stay reported, got %d groups", after.TotalConflicts)
	}
	if files := after.ConflictGroups[0].Files; len(files) != 1 || files[0] != "materials/kept.vtf" {
		t.Fatalf("expected the configured ignore list to hide only materials/shared.vtf, got %#v", files)
	}
}

func TestConflictAnalysisSettingsSurviveConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	priorityAware := true
	saved := &App{configDir: dir}
	if err := saved.SaveAppConfig(ConfigFile{
		ConflictPriorityAware: &priorityAware,
		ConflictIgnoreFiles:   []string{" Materials/Shared.VTF ", "", "scripts/", "materials/shared.vtf"},
	}); err != nil {
		t.Fatalf("save app config: %v", err)
	}

	reloaded := &App{configDir: dir}
	reloaded.loadConfig()

	if !reloaded.conflictPriorityAware {
		t.Fatal("expected the priority-aware flag to survive a config reload")
	}
	if got := reloaded.conflictIgnoreFiles; len(got) != 2 || got[0] != "materials/shared.vtf" || got[1] != "scripts/" {
		t.Fatalf("expected a normalized, de-duplicated ignore list, got %#v", got)
	}

	snapshot := reloaded.snapshotConfig()
	if snapshot.ConflictPriorityAware == nil || !*snapshot.ConflictPriorityAware {
		t.Fatalf("expected the snapshot to expose the priority-aware flag, got %#v", snapshot.ConflictPriorityAware)
	}
	if len(snapshot.ConflictIgnoreFiles) != 2 {
		t.Fatalf("expected the snapshot to expose the ignore list, got %#v", snapshot.ConflictIgnoreFiles)
	}
}
