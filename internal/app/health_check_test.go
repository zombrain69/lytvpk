package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHealthCheckFixture(t *testing.T, rootDir string, relPaths ...string) {
	t.Helper()
	for _, relPath := range relPaths {
		path := filepath.Join(rootDir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestVPK(t, path, map[string][]byte{"materials/" + filepath.Base(relPath) + ".vtf": {1}})
	}
}

func healthIssueKinds(report ModHealthReport) map[string]int {
	counts := make(map[string]int)
	for _, issue := range report.Issues {
		counts[issue.Kind]++
	}
	return counts
}

func healthIssueNames(report ModHealthReport, kind string) []string {
	names := make([]string, 0)
	for _, issue := range report.Issues {
		if issue.Kind == kind {
			names = append(names, issue.Name)
		}
	}
	return names
}

func TestRunModHealthCheckReportsAddonListInconsistencies(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "a.vpk", "dup.vpk", "unrecorded.vpk",
		filepath.Join("workshop", "123.vpk"), filepath.Join("disabled", "disabled-only.vpk"))
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "missing.vpk", Value: "1"},
		{Name: "dup.vpk", Value: "1"},
		{Name: "DUP.vpk", Value: "0"},
		{Name: "disabled-only.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	counts := healthIssueKinds(report)
	if counts["missing_file"] != 1 {
		t.Fatalf("expected one missing_file issue, got %#v", report.Issues)
	}
	if counts["disabled_only"] != 1 {
		t.Fatalf("expected one disabled_only issue, got %#v", report.Issues)
	}
	if counts["duplicate_entry"] != 1 {
		t.Fatalf("expected one duplicate_entry issue, got %#v", report.Issues)
	}
	if counts["unrecorded"] != 2 {
		t.Fatalf("expected two unrecorded files (root + workshop), got %#v", report.Issues)
	}
	if report.TotalIssues != len(report.Issues) || report.TotalIssues != 5 {
		t.Fatalf("expected five issues in total, got %#v", report)
	}

	if names := healthIssueNames(report, "unrecorded"); len(names) != 2 ||
		names[0] != "unrecorded.vpk" || names[1] != `workshop\123.vpk` {
		t.Fatalf("unexpected unrecorded names: %#v", names)
	}
	for _, issue := range report.Issues {
		if issue.Kind == "duplicate_entry" && issue.Severity != "warning" {
			t.Fatalf("expected duplicate entries to be a warning, got %#v", issue)
		}
		if issue.Kind == "missing_file" && issue.Severity != "critical" {
			t.Fatalf("expected a missing file to be critical, got %#v", issue)
		}
	}
	// 严重问题必须排在前面，方便用户先处理会影响加载的条目。
	if report.Issues[0].Severity != "critical" {
		t.Fatalf("expected critical issues first, got %#v", report.Issues)
	}
}

func TestRunModHealthCheckDeepScanFlagsCorruptVPK(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "a.vpk")
	brokenPath := filepath.Join(root, "broken.vpk")
	if err := os.WriteFile(brokenPath, []byte("this is not a vpk"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "broken.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)

	shallow, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("shallow health check: %v", err)
	}
	if kinds := healthIssueKinds(shallow); kinds["invalid_vpk"] != 0 {
		t.Fatalf("expected no invalid_vpk without deep scan, got %#v", shallow.Issues)
	}

	deep, err := a.RunModHealthCheck(ModHealthCheckOptions{DeepScan: true})
	if err != nil {
		t.Fatalf("deep health check: %v", err)
	}
	if !deep.DeepScanned {
		t.Fatal("expected the report to record that a deep scan ran")
	}
	names := healthIssueNames(deep, "invalid_vpk")
	if len(names) != 1 || names[0] != "broken.vpk" {
		t.Fatalf("expected broken.vpk to be reported as invalid, got %#v", deep.Issues)
	}
}

func TestRunModHealthCheckWithoutAddonListSkipsComparison(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "a.vpk", "b.vpk")
	a := newProfileTestApp(t, root)

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Kind != "missing_addonlist" {
		t.Fatalf("expected a single missing_addonlist hint, got %#v", report.Issues)
	}
	if report.Issues[0].Severity != "warning" {
		t.Fatalf("expected a warning severity, got %#v", report.Issues[0])
	}
}

func TestRemoveDuplicateAddonListEntriesKeepsFirstOccurrence(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "a.vpk", "b.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "A.vpk", Value: "0"},
		{Name: "b.vpk", Value: "0"},
	})
	a := newProfileTestApp(t, root)

	removed, err := a.RemoveDuplicateAddonListEntries()
	if err != nil {
		t.Fatalf("remove duplicates: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected exactly one duplicate to be removed, got %d", removed)
	}

	list, _, err := a.readAddonList()
	if err != nil {
		t.Fatalf("read addonlist: %v", err)
	}
	want := []AddonListItem{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "0"},
	}
	if len(list) != len(want) {
		t.Fatalf("expected %d entries, got %#v", len(want), list)
	}
	for index, entry := range want {
		if list[index].Name != entry.Name || list[index].Value != entry.Value {
			t.Fatalf("entry %d: expected %#v, got %#v", index, entry, list[index])
		}
	}

	// 没有重复项时不再改动文件。
	removed, err = a.RemoveDuplicateAddonListEntries()
	if err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected a no-op second run, got %d removals", removed)
	}
}

func TestRemoveMissingFileAddonListEntriesKeepsResolvableEntries(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "a.vpk", filepath.Join("disabled", "disabled-only.vpk"),
		filepath.Join("workshop", "123.vpk"))
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "missing.vpk", Value: "1"},
		{Name: "disabled-only.vpk", Value: "1"},
		{Name: `workshop\123.vpk`, Value: "0"},
		{Name: "missing2.vpk", Value: "0"},
	})
	a := newProfileTestApp(t, root)

	removed, err := a.RemoveMissingFileAddonListEntries()
	if err != nil {
		t.Fatalf("remove missing entries: %v", err)
	}
	if removed != 2 {
		t.Fatalf("expected two stale entries to be removed, got %d", removed)
	}

	list, _, err := a.readAddonList()
	if err != nil {
		t.Fatalf("read addonlist: %v", err)
	}
	want := []AddonListItem{
		{Name: "a.vpk", Value: "1"},
		{Name: "disabled-only.vpk", Value: "1"},
		{Name: `workshop\123.vpk`, Value: "0"},
	}
	if len(list) != len(want) {
		t.Fatalf("expected %d entries, got %#v", len(want), list)
	}
	for index, entry := range want {
		if list[index].Name != entry.Name || list[index].Value != entry.Value {
			t.Fatalf("entry %d: expected %#v, got %#v", index, entry, list[index])
		}
	}

	removed, err = a.RemoveMissingFileAddonListEntries()
	if err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected a no-op second run, got %d removals", removed)
	}
}

func TestRunModHealthCheckReportsWorkshopMetaProblems(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root,
		filepath.Join("workshop", "111.vpk"),
		filepath.Join("workshop", "222.vpk"),
		filepath.Join("workshop", "333.vpk"),
		"444.vpk",
	)
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: `workshop\111.vpk`, Value: "1"},
		{Name: `workshop\222.vpk`, Value: "1"},
		{Name: `workshop\333.vpk`, Value: "1"},
		{Name: "444.vpk", Value: "1"},
	})
	// 111 有配套 meta；222 缺 meta；333 的 meta 存在但 VPK 被手动删掉，这里换个做法：
	// 直接写一个没有对应 VPK 的孤立 meta。
	for _, path := range []string{
		filepath.Join(root, "workshop", "111.meta"),
		filepath.Join(root, "workshop", "orphan.meta"),
	} {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := newProfileTestApp(t, root)
	a.workshopMetaEnabled = false
	withoutMeta := func(report ModHealthReport) bool {
		for _, issue := range report.Issues {
			if issue.Kind == "missing_meta" {
				return true
			}
		}
		return false
	}

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if counts := healthIssueKinds(report); counts["orphan_meta"] != 1 {
		t.Fatalf("expected one orphan meta file, got %#v", report.Issues)
	}
	if withoutMeta(report) {
		t.Fatalf("did not expect missing_meta hints while workshop meta storage is off: %#v", report.Issues)
	}

	a.workshopMetaEnabled = true
	report, err = a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check with meta enabled: %v", err)
	}
	counts := healthIssueKinds(report)
	if counts["orphan_meta"] != 1 {
		t.Fatalf("expected the orphan meta file to stay reported, got %#v", report.Issues)
	}
	if counts["missing_meta"] != 2 {
		t.Fatalf("expected two workshop files without meta, got %#v", report.Issues)
	}
	if names := healthIssueNames(report, "missing_meta"); names[0] != "222.vpk" || names[1] != "333.vpk" {
		t.Fatalf("unexpected missing_meta names: %#v", names)
	}
}

func TestSaveModHealthReportWritesMarkdownFile(t *testing.T) {
	a := &App{configDir: t.TempDir()}
	markdown := "# LytVPK Mod 体检报告\n\n- 问题总数：0\n"

	path, err := a.SaveModHealthReport(markdown)
	if err != nil {
		t.Fatalf("save health report: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		t.Fatalf("expected a markdown file, got %s", path)
	}
	if !strings.Contains(path, "reports") {
		t.Fatalf("expected the report to live in a reports folder, got %s", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(content), "问题总数：0") {
		t.Fatalf("unexpected report content: %s", content)
	}

	if _, err := a.SaveModHealthReport("   "); err == nil {
		t.Fatal("expected an empty report to be rejected")
	}
}
