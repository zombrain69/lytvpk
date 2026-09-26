package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 对齐 FireAxe `AddonFileMissingProblem.FileTypeMismatch`
// （AddonFileMissingProblem.cs:9-14、:26）：本该是文件、磁盘上却是目录时要有自己的诊断，
// 不能混在"文件不存在"里 —— 两种情况的处理方式完全不同。
func TestHealthCheckReportsFileTypeMismatchForDirectoryNamedVpk(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "real.vpk")
	// addons/ghost.vpk 是个文件夹：游戏只加载 *.vpk 文件，这个条目不会生效。
	if err := os.MkdirAll(filepath.Join(root, "ghost.vpk"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "real.vpk", Value: "1"},
		{Name: "ghost.vpk", Value: "1"},
		{Name: "gone.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	counts := healthIssueKinds(report)
	if counts[modHealthKindFileTypeMismatch] != 1 {
		t.Fatalf("expected one file_type_mismatch issue, got %#v", report.Issues)
	}
	if counts[modHealthKindMissingFile] != 1 {
		t.Fatalf("只有 gone.vpk 该算缺失，目录不算缺失：%#v", report.Issues)
	}
	if names := healthIssueNames(report, modHealthKindFileTypeMismatch); names[0] != "ghost.vpk" {
		t.Fatalf("expected the issue to reference ghost.vpk, got %#v", names)
	}
	for _, issue := range report.Issues {
		if issue.Kind != modHealthKindFileTypeMismatch {
			continue
		}
		if !strings.Contains(issue.Message, "文件夹") {
			t.Fatalf("提示里要说明磁盘上是一个文件夹：%q", issue.Message)
		}
	}
}

func TestHealthCheckFileTypeMismatchPointsAtDisabledCopy(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, filepath.Join("disabled", "ghost.vpk"))
	if err := os.MkdirAll(filepath.Join(root, "ghost.vpk"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{{Name: "ghost.vpk", Value: "1"}})
	a := newProfileTestApp(t, root)

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	counts := healthIssueKinds(report)
	if counts[modHealthKindFileTypeMismatch] != 1 || counts[modHealthKindDisabledOnly] != 0 {
		t.Fatalf("目录占位应报类型不符而不是只剩 disabled 副本：%#v", report.Issues)
	}
	for _, issue := range report.Issues {
		if issue.Kind == modHealthKindFileTypeMismatch && !strings.Contains(issue.Message, "disabled") {
			t.Fatalf("disabled 里还有可用副本时要提示出来：%q", issue.Message)
		}
	}
}

// 对齐 FireAxe `InvalidPublishedFileIdProblem`（InvalidPublishedFileIdProblem.cs:5-16）
// 与 `WorkshopVpkMetaInfo.PublishedFileId`（WorkshopVpkMetaInfo.cs:7）：
// 本地工坊记录指向的作品必须和文件本身对得上，否则更新检测会拿另一个作品的时间戳做比较。
func TestHealthCheckReportsWorkshopMetaIDMismatch(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, filepath.Join("workshop", "123.vpk"), filepath.Join("workshop", "456.vpk"))
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: `workshop\123.vpk`, Value: "1"},
		{Name: `workshop\456.vpk`, Value: "1"},
	})
	a := newProfileTestApp(t, root)
	workshopDir := filepath.Join(root, "workshop")

	if err := saveWorkshopMeta(filepath.Join(workshopDir, "123.vpk"), &WorkshopMeta{
		WorkshopID:   "999",
		Title:        "另一个作品",
		DownloadedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveWorkshopMeta(filepath.Join(workshopDir, "456.vpk"), &WorkshopMeta{
		WorkshopID:   "456",
		Title:        "对得上的作品",
		DownloadedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	counts := healthIssueKinds(report)
	if counts[modHealthKindMetaIDMismatch] != 1 {
		t.Fatalf("expected one meta_id_mismatch issue, got %#v", report.Issues)
	}
	if counts[modHealthKindOrphanMeta] != 0 || counts[modHealthKindMissingMeta] != 0 {
		t.Fatalf("ID 不一致不该被算成孤立或缺失：%#v", report.Issues)
	}
	for _, issue := range report.Issues {
		if issue.Kind != modHealthKindMetaIDMismatch {
			continue
		}
		if issue.Name != "123.meta" {
			t.Fatalf("expected the issue to reference 123.meta, got %q", issue.Name)
		}
		if !strings.Contains(issue.Message, "999") || !strings.Contains(issue.Message, "123") {
			t.Fatalf("提示里要写清两个 ID：%q", issue.Message)
		}
	}
}

// 对齐 FireAxe `AddonDependencyProblem`（AddonDependencyProblem.cs:12-26）：
// 上游唯一声明"可自动修复"的问题就是依赖未开启；修复动作是启用全部依赖。
// 本项目因此要求体检结果把可操作目标（addonlist 键）带给界面。
func TestDependencyDisabledIssueCarriesFixTarget(t *testing.T) {
	root := t.TempDir()
	paths := writeModGroupFixture(t, root, "master.vpk", "dep-off.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "master.vpk", Value: "1"},
		{Name: "dep-off.vpk", Value: "0"},
	})
	a := newDependencyTestApp(t, root)
	if _, err := a.SetModDependencies(paths[0], []string{paths[1]}); err != nil {
		t.Fatalf("set master dependencies: %v", err)
	}

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	target := ""
	for _, issue := range report.Issues {
		if issue.Kind == modHealthKindDependencyDisabled {
			target = issue.Target
		}
	}
	if target == "" {
		t.Fatalf("依赖问题要带上可操作目标，界面才能一键修复：%#v", report.Issues)
	}

	// 用界面拿到的目标直接驱动修复入口，等价于用户点「修复」。
	result, err := a.EnableModDependencies(target)
	if err != nil {
		t.Fatalf("用体检目标一键启用依赖: %v", err)
	}
	if len(result.Enabled) != 1 || result.Enabled[0] != "dep-off.vpk" {
		t.Fatalf("expected dep-off.vpk to be enabled, got %#v", result)
	}

	// 体检文档承诺"所有修复写盘前先备份"，依赖修复也必须留下可恢复备份。
	backups, err := a.ListAddonListBackups()
	if err != nil {
		t.Fatalf("list addonlist backups: %v", err)
	}
	found := false
	for _, backup := range backups {
		if strings.HasPrefix(backup.Name, "before-dependency-fix") {
			found = true
		}
	}
	if !found {
		t.Fatalf("依赖修复前必须建立 before-dependency-fix 备份，现有备份：%#v", backups)
	}

	after, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check after fix: %v", err)
	}
	if counts := healthIssueKinds(after); counts[modHealthKindDependencyDisabled] != 0 {
		t.Fatalf("修完之后这条问题应该消失：%#v", after.Issues)
	}
}

// 对齐 FireAxe File Cleaner 的目标之一：工坊条目的冗余 VPK。
// 工坊 VPK 的文件名就是作品 ID，所以 `addons\123.vpk` 与 `addons\workshop\123.vpk`
// 是同一件作品的两份拷贝 —— 只报告，删除交给用户。
func TestHealthCheckReportsDuplicateWorkshopCopies(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root,
		"123.vpk",                        // 根目录副本
		filepath.Join("workshop", "123.vpk"), // 工坊原件
		filepath.Join("workshop", "456.vpk"), // 只有工坊一份：不该报
		filepath.Join("workshop", "not-a-number.vpk"), // 文件名不是 ID：跳过
		"not-a-number.vpk",
	)
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "123.vpk", Value: "1"},
		{Name: `workshop\123.vpk`, Value: "0"},
		{Name: `workshop\456.vpk`, Value: "1"},
	})
	a := newProfileTestApp(t, root)

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	counts := healthIssueKinds(report)
	if counts[modHealthKindDuplicateVPKCopy] != 1 {
		t.Fatalf("应当只报 1 条冗余副本（123.vpk），实际：%#v", report.Issues)
	}
	for _, issue := range report.Issues {
		if issue.Kind != modHealthKindDuplicateVPKCopy {
			continue
		}
		if issue.Name != "123.vpk" || issue.Location != "root" {
			t.Fatalf("冗余副本应指向根目录那份：%#v", issue)
		}
		if !strings.Contains(issue.Message, "只保留一份") {
			t.Fatalf("提示要给出下一步：%q", issue.Message)
		}
	}
	// 体检只读：两份都还在
	if !fileExists(filepath.Join(root, "123.vpk")) || !fileExists(filepath.Join(root, "workshop", "123.vpk")) {
		t.Fatal("体检不该动任何文件")
	}
}
