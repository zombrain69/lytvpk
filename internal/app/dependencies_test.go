package app

import (
	"os"
	"path/filepath"
	"testing"
)

func newDependencyTestApp(t *testing.T, rootDir string) *App {
	t.Helper()
	return newProfileTestApp(t, rootDir)
}

func TestSetModDependenciesStoresAndRemovesRecords(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "master.vpk", "dep-a.vpk", "dep-b.vpk", "other.vpk")
	a := newDependencyTestApp(t, root)

	record, err := a.SetModDependencies(paths[0], []string{paths[1], paths[2], paths[1]})
	if err != nil {
		t.Fatalf("set dependencies: %v", err)
	}
	if record.Key != "master.vpk" || record.Name != "master.vpk" {
		t.Fatalf("unexpected master record: %#v", record)
	}
	if len(record.Dependencies) != 2 ||
		record.Dependencies[0].Key != "dep-a.vpk" || record.Dependencies[1].Key != "dep-b.vpk" {
		t.Fatalf("expected two de-duplicated dependencies, got %#v", record.Dependencies)
	}

	reloaded := newDependencyTestApp(t, root)
	reloaded.configDir = a.configDir
	list, err := reloaded.ListModDependencies()
	if err != nil {
		t.Fatalf("list dependencies: %v", err)
	}
	if len(list) != 1 || list[0].Key != "master.vpk" || len(list[0].Dependencies) != 2 {
		t.Fatalf("expected the record to persist, got %#v", list)
	}

	// 目标自己不能成为自己的依赖。
	record, err = a.SetModDependencies(paths[0], []string{paths[0]})
	if err != nil {
		t.Fatalf("set self dependency: %v", err)
	}
	if len(record.Dependencies) != 0 {
		t.Fatalf("expected a self reference to be dropped, got %#v", record.Dependencies)
	}

	// 空依赖列表等价于删除该记录。
	list, err = a.ListModDependencies()
	if err != nil {
		t.Fatalf("list after clearing: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected the record to be removed, got %#v", list)
	}

	// 也可以在依赖列表里按 addonlist 键删除。
	if _, err := a.SetModDependencies(paths[0], []string{paths[1]}); err != nil {
		t.Fatalf("re-add dependencies: %v", err)
	}
	if err := a.DeleteModDependencies("master.vpk"); err != nil {
		t.Fatalf("delete dependencies by key: %v", err)
	}
	list, err = a.ListModDependencies()
	if err != nil {
		t.Fatalf("list after delete by key: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected the record to be deleted by key, got %#v", list)
	}
	if err := a.DeleteModDependencies("master.vpk"); err == nil {
		t.Fatal("expected deleting a missing record to fail")
	}
}

func TestEnableModDependenciesEnablesDisabledDependencies(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "master.vpk", "dep-a.vpk", "dep-b.vpk")
	// 声明一个磁盘上并不存在的依赖，用于验证“缺失依赖”不会被写入 addonlist。
	missingPath := filepath.Join(root, "dep-missing.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "master.vpk", Value: "1"},
		{Name: "dep-a.vpk", Value: "0"},
		{Name: "dep-b.vpk", Value: "1"},
	})
	a := newDependencyTestApp(t, root)
	if _, err := a.SetModDependencies(paths[0], []string{paths[1], paths[2], missingPath}); err != nil {
		t.Fatalf("set dependencies: %v", err)
	}

	result, err := a.EnableModDependencies(paths[0])
	if err != nil {
		t.Fatalf("enable dependencies: %v", err)
	}
	if len(result.Enabled) != 1 || result.Enabled[0] != "dep-a.vpk" {
		t.Fatalf("expected only dep-a.vpk to be enabled, got %#v", result)
	}
	if len(result.AlreadyEnabled) != 1 || result.AlreadyEnabled[0] != "dep-b.vpk" {
		t.Fatalf("expected dep-b.vpk to be reported as already enabled, got %#v", result)
	}
	if len(result.Missing) != 1 || result.Missing[0] != "dep-missing.vpk" {
		t.Fatalf("expected the missing dependency to be reported, got %#v", result)
	}

	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["dep-a.vpk"] || !states["dep-b.vpk"] {
		t.Fatalf("expected both existing dependencies to be enabled, got %#v", states)
	}
}

func TestRunModHealthCheckReportsDependencyProblems(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "master.vpk", "dep-disabled.vpk", "dep-lost.vpk", "idle.vpk", "dep-idle.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "master.vpk", Value: "1"},
		{Name: "dep-disabled.vpk", Value: "0"},
		{Name: "dep-lost.vpk", Value: "1"},
		{Name: "idle.vpk", Value: "0"},
		{Name: "dep-idle.vpk", Value: "0"},
	})
	a := newDependencyTestApp(t, root)
	if _, err := a.SetModDependencies(paths[0], []string{paths[1], paths[2]}); err != nil {
		t.Fatalf("set master dependencies: %v", err)
	}
	// 依赖文件被手动删除，模拟“依赖缺失”。
	if err := os.Remove(paths[2]); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModDependencies(paths[3], []string{paths[4]}); err != nil {
		t.Fatalf("set idle dependencies: %v", err)
	}

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	counts := healthIssueKinds(report)
	if counts["dependency_disabled"] != 1 {
		t.Fatalf("expected one disabled dependency issue, got %#v", report.Issues)
	}
	if counts["dependency_missing"] != 1 {
		t.Fatalf("expected one missing dependency issue, got %#v", report.Issues)
	}
	// 主 Mod 本身处于关闭状态时不应提示依赖问题。
	for _, issue := range report.Issues {
		if issue.Kind == "dependency_disabled" && issue.Name != "master.vpk" {
			t.Fatalf("expected issues to reference the master mod, got %#v", issue)
		}
	}
	if len(healthIssueNames(report, "dependency_disabled")) > 1 {
		t.Fatalf("expected only the enabled master to be reported, got %#v", report.Issues)
	}
}

func TestEnableAllMissingModDependenciesSkipsDisabledMasters(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "master.vpk", "dep.vpk", "idle.vpk", "idle-dep.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "master.vpk", Value: "1"},
		{Name: "dep.vpk", Value: "0"},
		{Name: "idle.vpk", Value: "0"},
		{Name: "idle-dep.vpk", Value: "0"},
	})
	a := newDependencyTestApp(t, root)
	if _, err := a.SetModDependencies(paths[0], []string{paths[1]}); err != nil {
		t.Fatalf("set master dependencies: %v", err)
	}
	if _, err := a.SetModDependencies(paths[2], []string{paths[3]}); err != nil {
		t.Fatalf("set idle dependencies: %v", err)
	}

	result, err := a.EnableAllMissingModDependencies()
	if err != nil {
		t.Fatalf("enable all dependencies: %v", err)
	}
	if len(result.Enabled) != 1 || result.Enabled[0] != "dep.vpk" || result.MasterCount != 1 {
		t.Fatalf("expected only the enabled master's dependency to change, got %#v", result)
	}

	states, err := a.modStrategyGroupStates()
	if err != nil {
		t.Fatalf("read states: %v", err)
	}
	if !states["dep.vpk"] || states["idle-dep.vpk"] {
		t.Fatalf("expected the disabled master's dependency to stay off, got %#v", states)
	}
}
