package app

import (
	"os"
	"path/filepath"
	"testing"
)

func writeModGroupFixture(t *testing.T, rootDir string, relPaths ...string) []string {
	t.Helper()
	paths := make([]string, 0, len(relPaths))
	for _, relPath := range relPaths {
		path := filepath.Join(rootDir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

func TestCaptureModStrategyGroupMapsSelectedPaths(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "A.vpk", filepath.Join("workshop", "123.vpk"), filepath.Join("disabled", "C.vpk"))
	a := newProfileTestApp(t, root)

	group, err := a.CaptureModStrategyGroup("角色包", "轮换用", "single", append(paths, paths[0]))
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if group.Strategy != "single" {
		t.Fatalf("expected strategy single, got %q", group.Strategy)
	}
	if len(group.Members) != 3 {
		t.Fatalf("expected duplicate paths to collapse into three members, got %#v", group.Members)
	}
	want := []ModStrategyGroupMember{
		{Key: "a.vpk", Name: "A.vpk"},
		{Key: `workshop\123.vpk`, Name: `workshop\123.vpk`},
		{Key: "c.vpk", Name: "C.vpk"},
	}
	for index, member := range want {
		if group.Members[index] != member {
			t.Fatalf("member %d: expected %#v, got %#v", index, member, group.Members[index])
		}
	}
}

func TestCaptureModStrategyGroupValidatesInput(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk")
	a := newProfileTestApp(t, root)

	if _, err := a.CaptureModStrategyGroup("", "", "all", paths); err == nil {
		t.Fatal("expected an empty name to be rejected")
	}
	if _, err := a.CaptureModStrategyGroup("空组", "", "all", nil); err == nil {
		t.Fatal("expected an empty member list to be rejected")
	}
	if _, err := a.CaptureModStrategyGroup("怪策略", "", "sometimes", paths); err == nil {
		t.Fatal("expected an unknown strategy to be rejected")
	}
	if _, err := a.CaptureModStrategyGroup("越界", "", "all", []string{filepath.Join(t.TempDir(), "outside.vpk")}); err == nil {
		t.Fatal("expected paths outside the addons directory to be rejected")
	}
}

func TestModStrategyGroupsPersistAndDelete(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk")
	a := newProfileTestApp(t, root)
	group, err := a.CaptureModStrategyGroup("组合一", "", "single_random", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}

	reloaded := newProfileTestApp(t, root)
	reloaded.configDir = a.configDir
	list, err := reloaded.ListModStrategyGroups()
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(list) != 1 || list[0].ID != group.ID || list[0].Strategy != "single_random" {
		t.Fatalf("expected the captured group to persist, got %#v", list)
	}

	if err := reloaded.DeleteModStrategyGroup(group.ID); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	list, err = reloaded.ListModStrategyGroups()
	if err != nil {
		t.Fatalf("list groups after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected the group to be gone, got %#v", list)
	}
}

func TestApplyModStrategyGroupAllAndOff(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk", "c.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "0"},
		{Name: "b.vpk", Value: "0"},
	})
	a := newProfileTestApp(t, root)
	group, err := a.CaptureModStrategyGroup("全部", "", "all", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}

	result, err := a.ApplyModStrategyGroup(group.ID, ModStrategyGroupApplyOptions{})
	if err != nil {
		t.Fatalf("apply all: %v", err)
	}
	if result.Strategy != "all" || len(result.Enabled) != 3 {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	list, _, err := a.readAddonList()
	if err != nil {
		t.Fatalf("read addonlist: %v", err)
	}
	want := []AddonListItem{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
		{Name: "c.vpk", Value: "1"},
	}
	if len(list) != len(want) {
		t.Fatalf("expected %d entries, got %#v", len(want), list)
	}
	for index, entry := range want {
		if list[index].Name != entry.Name || list[index].Value != entry.Value {
			t.Fatalf("entry %d: expected %#v, got %#v", index, entry, list[index])
		}
	}

	if _, err := a.ApplyModStrategyGroup(group.ID, ModStrategyGroupApplyOptions{Strategy: "off"}); err != nil {
		t.Fatalf("apply off: %v", err)
	}
	list, _, err = a.readAddonList()
	if err != nil {
		t.Fatalf("read addonlist after off: %v", err)
	}
	for _, entry := range list {
		if entry.Value != "0" {
			t.Fatalf("expected every entry to be disabled, got %#v", list)
		}
	}
	if len(list) != 3 {
		t.Fatalf("expected the order to stay intact, got %#v", list)
	}
}

func TestApplyModStrategyGroupSingleKeepsRequestedMember(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk", "c.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
		{Name: "c.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)
	group, err := a.CaptureModStrategyGroup("角色包", "", "single", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}

	result, err := a.ApplyModStrategyGroup(group.ID, ModStrategyGroupApplyOptions{PickKey: "b.vpk"})
	if err != nil {
		t.Fatalf("apply single: %v", err)
	}
	if result.PickedName != "b.vpk" || len(result.Enabled) != 1 || len(result.Disabled) != 2 {
		t.Fatalf("unexpected single result: %#v", result)
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["b.vpk"] || states["a.vpk"] || states["c.vpk"] {
		t.Fatalf("expected only b.vpk to stay enabled, got %#v", states)
	}

	// 未指定成员时，优先保留当前已启用的那一个。
	if _, err := a.ApplyModStrategyGroup(group.ID, ModStrategyGroupApplyOptions{}); err != nil {
		t.Fatalf("apply single without pick: %v", err)
	}
	states, err = a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["b.vpk"] || states["a.vpk"] || states["c.vpk"] {
		t.Fatalf("expected the enabled member to be kept, got %#v", states)
	}
}

func TestApplyModStrategyGroupSingleRandomPicksExactlyOne(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk", "c.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
		{Name: "c.vpk", Value: "0"},
	})
	a := newProfileTestApp(t, root)
	group, err := a.CaptureModStrategyGroup("随机角色", "", "all", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}

	result, err := a.ApplyModStrategyGroup(group.ID, ModStrategyGroupApplyOptions{Strategy: "single_random"})
	if err != nil {
		t.Fatalf("apply single_random: %v", err)
	}
	if result.Strategy != "single_random" || len(result.Enabled) != 1 || len(result.Disabled) != 2 {
		t.Fatalf("unexpected single_random result: %#v", result)
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	enabledCount := 0
	for _, member := range group.Members {
		if states[member.Key] {
			enabledCount++
		}
	}
	if enabledCount != 1 {
		t.Fatalf("expected exactly one enabled member, got %#v", states)
	}
}

func TestApplyModStrategyGroupRejectsUnknownGroupAndPick(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{{Name: "a.vpk", Value: "1"}})
	a := newProfileTestApp(t, root)
	group, err := a.CaptureModStrategyGroup("双人组", "", "single", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}

	if _, err := a.ApplyModStrategyGroup("missing-id", ModStrategyGroupApplyOptions{}); err == nil {
		t.Fatal("expected an unknown group id to fail")
	}
	if _, err := a.ApplyModStrategyGroup(group.ID, ModStrategyGroupApplyOptions{PickKey: "not-a-member.vpk"}); err == nil {
		t.Fatal("expected an unknown pick key to fail")
	}
	if _, err := a.ApplyModStrategyGroup(group.ID, ModStrategyGroupApplyOptions{Strategy: "sometimes"}); err == nil {
		t.Fatal("expected an unknown strategy override to fail")
	}
}

func cacheModGroupFiles(a *App, paths ...string) {
	for _, path := range paths {
		a.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
			Name:           filepath.Base(path),
			Path:           path,
			Location:       "root",
			GameStateKnown: true,
			GameEnabled:    false,
		}})
	}
}

func TestSetModStrategyGroupEnforcementPersists(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk")
	a := newProfileTestApp(t, root)
	group, err := a.CaptureModStrategyGroup("角色包", "", "single", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if group.Enforce {
		t.Fatal("expected enforcement to default to off")
	}

	updated, err := a.SetModStrategyGroupEnforcement(group.ID, true)
	if err != nil {
		t.Fatalf("enable enforcement: %v", err)
	}
	if !updated.Enforce {
		t.Fatal("expected the returned group to be enforced")
	}

	reloaded := newProfileTestApp(t, root)
	reloaded.configDir = a.configDir
	list, err := reloaded.ListModStrategyGroups()
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(list) != 1 || !list[0].Enforce {
		t.Fatalf("expected enforcement to persist, got %#v", list)
	}
}

func TestSetVPKGameEnabledEnforcesSingleStrategyGroup(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "0"},
		{Name: "b.vpk", Value: "0"},
	})
	a := newProfileTestApp(t, root)
	cacheModGroupFiles(a, paths...)
	group, err := a.CaptureModStrategyGroup("角色包", "", "single", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if _, err := a.SetModStrategyGroupEnforcement(group.ID, true); err != nil {
		t.Fatalf("enable enforcement: %v", err)
	}

	if _, err := a.SetVPKGameEnabled(paths[0], true); err != nil {
		t.Fatalf("enable a.vpk: %v", err)
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["a.vpk"] || states["b.vpk"] {
		t.Fatalf("expected only a.vpk to be enabled after enforcement, got %#v", states)
	}

	// 反过来启用 b.vpk 时，a.vpk 必须被关闭。
	if _, err := a.SetVPKGameEnabled(paths[1], true); err != nil {
		t.Fatalf("enable b.vpk: %v", err)
	}
	states, err = a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if states["a.vpk"] || !states["b.vpk"] {
		t.Fatalf("expected only b.vpk to stay enabled, got %#v", states)
	}
}

func TestSetVPKGameEnabledEnforcesAllStrategyGroupWithUnrecordedMember(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk", "c.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "0"},
		{Name: "b.vpk", Value: "0"},
	})
	a := newProfileTestApp(t, root)
	cacheModGroupFiles(a, paths...)
	group, err := a.CaptureModStrategyGroup("全套", "", "all", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	if _, err := a.SetModStrategyGroupEnforcement(group.ID, true); err != nil {
		t.Fatalf("enable enforcement: %v", err)
	}

	if _, err := a.SetVPKGameEnabled(paths[0], true); err != nil {
		t.Fatalf("enable a.vpk: %v", err)
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["a.vpk"] || !states["b.vpk"] || !states["c.vpk"] {
		t.Fatalf("expected the whole group to follow, got %#v", states)
	}

	if _, err := a.SetVPKGameEnabled(paths[0], false); err != nil {
		t.Fatalf("disable a.vpk: %v", err)
	}
	states, err = a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if states["a.vpk"] || states["b.vpk"] || states["c.vpk"] {
		t.Fatalf("expected the whole group to be disabled, got %#v", states)
	}
}

func TestSetVPKGameEnabledSkipsGroupsWithoutEnforcement(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "0"},
		{Name: "b.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)
	cacheModGroupFiles(a, paths...)
	group, err := a.CaptureModStrategyGroup("角色包", "", "single", paths)
	if err != nil {
		t.Fatalf("capture group: %v", err)
	}
	_ = group

	if _, err := a.SetVPKGameEnabled(paths[0], true); err != nil {
		t.Fatalf("enable a.vpk: %v", err)
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["a.vpk"] || !states["b.vpk"] {
		t.Fatalf("expected b.vpk to be left alone without enforcement, got %#v", states)
	}
}

func TestSetVPKGameEnabledDoesNotCascadeAcrossGroups(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "a.vpk", "b.vpk", "c.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "0"},
		{Name: "b.vpk", Value: "0"},
		{Name: "c.vpk", Value: "0"},
	})
	a := newProfileTestApp(t, root)
	cacheModGroupFiles(a, paths...)
	first, err := a.CaptureModStrategyGroup("第一组", "", "all", paths[0:2])
	if err != nil {
		t.Fatalf("capture first group: %v", err)
	}
	second, err := a.CaptureModStrategyGroup("第二组", "", "all", paths[1:3])
	if err != nil {
		t.Fatalf("capture second group: %v", err)
	}
	for _, id := range []string{first.ID, second.ID} {
		if _, err := a.SetModStrategyGroupEnforcement(id, true); err != nil {
			t.Fatalf("enable enforcement for %s: %v", id, err)
		}
	}

	if _, err := a.SetVPKGameEnabled(paths[0], true); err != nil {
		t.Fatalf("enable a.vpk: %v", err)
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["a.vpk"] || !states["b.vpk"] {
		t.Fatalf("expected the first group to follow, got %#v", states)
	}
	if states["c.vpk"] {
		t.Fatalf("expected overlapping groups not to cascade, got %#v", states)
	}
}
