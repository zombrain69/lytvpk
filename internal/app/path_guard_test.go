package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 对齐 FireAxe `FileSystemUtils.ThrowIfPathInvalid` / `FileOutOfAddonRootException`
// （FileSystemUtils.cs:90-113、AddonNode.cs:405-413）：改动受管文件之前先确认
// 路径合法且落在受管根目录内 —— 否则一个过期路径（用户中途换过 addons 目录、
// 或前端状态陈旧）就可能去动根本不属于本工具管的文件。
func TestManagedFilePathProblemDetectsOutsidePaths(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name        string
		path        string
		wantBlocked bool
	}{
		{name: "addons 根目录下的 Mod", path: filepath.Join(root, "a.vpk")},
		{name: "workshop 子目录", path: filepath.Join(root, "workshop", "123.vpk")},
		{name: "workshop 的更深一层", path: filepath.Join(root, "workshop", "sub", "123.vpk")},
		{name: "disabled 子目录", path: filepath.Join(root, "disabled", "b.vpk")},
		{name: "上级目录的文件", path: filepath.Join(filepath.Dir(root), "outside.vpk"), wantBlocked: true},
		{name: "用 .. 逃出去", path: filepath.Join(root, "..", "other", "c.vpk"), wantBlocked: true},
		{name: "受管根目录本身", path: root, wantBlocked: true},
		{name: "workshop 目录本身", path: filepath.Join(root, "workshop"), wantBlocked: true},
		{name: "空路径", path: "", wantBlocked: true},
		{name: "相对路径", path: "a.vpk", wantBlocked: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problem := managedFilePathProblem(root, testCase.path)
			if testCase.wantBlocked && problem == "" {
				t.Fatalf("%s 应当被拦下", testCase.path)
			}
			if !testCase.wantBlocked && problem != "" {
				t.Fatalf("%s 不该被拦下: %s", testCase.path, problem)
			}
		})
	}

	// 大小写不同也算同一个目录（Windows 路径不区分大小写）。
	upper := strings.ToUpper(filepath.Join(root, "workshop", "123.vpk"))
	if problem := managedFilePathProblem(root, upper); problem != "" {
		t.Fatalf("大小写不同不该被当成越界: %s", problem)
	}
	// 还没选目录时一律不放过。
	if managedFilePathProblem("", filepath.Join(root, "a.vpk")) == "" {
		t.Fatal("还没选 addons 目录时不该放行任何文件操作")
	}
}

func TestDeleteVPKFileRefusesPathsOutsideManagedRoots(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "keep-me.vpk")
	if err := os.WriteFile(outsideFile, []byte("not managed"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := a.DeleteVPKFile(outsideFile)
	if err == nil {
		t.Fatal("受管目录外的文件不该被删除")
	}
	if !strings.Contains(err.Error(), "受管") {
		t.Fatalf("错误提示要说清原因: %v", err)
	}
	if !fileExists(outsideFile) {
		t.Fatal("被拦下的文件必须原样留在原处")
	}

	// 受管目录里的删除必须照常工作（守卫不能误伤正常流程）。
	insideFile := filepath.Join(addonsDir, "a.vpk")
	if err := a.DeleteVPKFile(insideFile); err != nil {
		t.Fatalf("受管目录里的文件应当能删: %v", err)
	}
	if fileExists(insideFile) {
		t.Fatal("受管目录里的文件应该已经被移入回收站")
	}
}

func TestToggleVPKVisibilityRefusesPathsOutsideManagedRoots(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.vpk")
	if err := os.WriteFile(outsideFile, []byte("not managed"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := a.ToggleVPKVisibility(outsideFile); err == nil {
		t.Fatal("受管目录外的文件不该被改名")
	}
	if !fileExists(outsideFile) {
		t.Fatal("被拦下的文件必须原样留在原处（不能加上 _ 前缀）")
	}
	if fileExists(filepath.Join(outsideDir, "_outside.vpk")) {
		t.Fatal("不该产生改名后的文件")
	}
}

// 源文件必须在受管目录内；但"移动到任意用户选定目录"是既有能力，不能被守卫退化。
func TestMoveVpkFilesKeepsFreeDestinationButGuardsSources(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "keep-me.vpk")
	if err := os.WriteFile(outsideFile, []byte("not managed"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := a.MoveVpkFiles([]string{outsideFile}, filepath.Join(addonsDir, "disabled"))
	if err != nil {
		t.Fatalf("单项失败不该让整批报错: %v", err)
	}
	if result.FailCount != 1 || len(result.Errors) != 1 {
		t.Fatalf("受管目录外的源文件应当逐项失败: %#v", result)
	}
	if !strings.Contains(result.Errors[0], "受管") {
		t.Fatalf("失败原因要说清是「受管目录」问题: %q", result.Errors[0])
	}
	if !fileExists(outsideFile) {
		t.Fatal("被拦下的源文件必须留在原处")
	}

	// 正常流程：受管目录里的 Mod 仍然可以移动到用户自己选的目录。
	source := filepath.Join(addonsDir, "a.vpk")
	userChosenDir := filepath.Join(t.TempDir(), "我的备份")
	moved, err := a.MoveVpkFiles([]string{source}, userChosenDir)
	if err != nil {
		t.Fatalf("移动到用户选定目录不该被拦: %v", err)
	}
	if moved.SuccessCount != 1 {
		t.Fatalf("应当成功移动 1 个文件: %#v", moved)
	}
	if !fileExists(filepath.Join(userChosenDir, "a.vpk")) {
		t.Fatal("文件应当出现在用户选定的目录里")
	}
}

// addonlist.txt 是游戏侧唯一权威，但条目本身也可能"指到受管目录外面"
// （手改文件、别的工具写过、或旧版本留下的 `..\` 前缀）。这种情况游戏不会加载，
// 而旧体检会按"文件存在"放过去 —— 现在单独报一类。
func TestHealthCheckReportsAddonListEntriesOutsideManagedRoots(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "ok.vpk")
	// 受管目录之外的真实文件：条目指向它时，游戏侧不会加载，
	// 但旧实现会因为"文件存在"而不报任何问题。
	outsideFile := filepath.Join(filepath.Dir(root), "outside.vpk")
	writeHealthCheckFixture(t, filepath.Dir(root), "outside.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "ok.vpk", Value: "1"},
		{Name: `..\outside.vpk`, Value: "1"},
	})
	a := newProfileTestApp(t, root)

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("health check: %v", err)
	}

	counts := healthIssueKinds(report)
	if counts[modHealthKindOutsideRoot] != 1 {
		t.Fatalf("应当报 1 条「条目指向受管目录之外」，实际：%#v", report.Issues)
	}
	if counts[modHealthKindMissingFile] != 0 {
		t.Fatalf("越界条目不该同时算成缺失（它就是那些问题的根因）：%#v", report.Issues)
	}
	if !fileExists(outsideFile) {
		t.Fatal("体检只读，不该动任何文件")
	}
}
