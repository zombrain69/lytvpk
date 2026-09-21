package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunGuardedWritesCrashReport 覆盖 panic → 本地报告（核心行为）。
func TestRunGuardedWritesCrashReport(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	a.InstallCrashReporter()
	t.Cleanup(a.CloseCrashReporter)
	log.Printf("崩溃前的日志行")

	recovered := a.runGuarded("单元测试", func() {
		panic("测试用崩溃")
	})
	if !recovered {
		t.Fatal("runGuarded 应捕获 panic")
	}

	reports, err := a.ListCrashReports()
	if err != nil {
		t.Fatalf("list crash reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("应生成 1 份崩溃报告: %#v", reports)
	}
	report := reports[0]
	if report.Kind != crashReportKindPanic || report.Source != "单元测试" {
		t.Fatalf("报告来源 = %#v", report)
	}
	if !strings.Contains(report.Reason, "测试用崩溃") {
		t.Fatalf("报告原因 = %q", report.Reason)
	}
	if !strings.Contains(report.Stack, "crash_reporter_test.go") {
		t.Fatalf("报告应包含堆栈: %q", report.Stack)
	}
	if report.Version == "" || report.GoVersion == "" || report.OS == "" || report.Arch == "" {
		t.Fatalf("报告应包含环境信息: %#v", report)
	}
	if report.CreatedAt == "" {
		t.Fatal("报告应包含时间")
	}
}

// TestCrashReportIncludesLogTail 覆盖"日志尾部"。
func TestCrashReportIncludesLogTail(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	a.InstallCrashReporter()
	t.Cleanup(a.CloseCrashReporter)
	for index := 0; index < 3; index++ {
		log.Printf("崩溃日志样本 %d", index)
	}
	if _, err := a.ReportFrontendError("前端炸了", "at foo (bar.js:1)"); err != nil {
		t.Fatal(err)
	}
	reports, err := a.ListCrashReports()
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("应有 1 份报告: %#v", reports)
	}
	tail := strings.Join(reports[0].LogTail, "\n")
	if !strings.Contains(tail, "崩溃日志样本 2") {
		t.Fatalf("报告应包含日志尾部: %q", tail)
	}
	if reports[0].Kind != crashReportKindUI || reports[0].Reason != "前端炸了" {
		t.Fatalf("前端报告内容 = %#v", reports[0])
	}
}

func TestLogRingBufferKeepsOnlyRecentLines(t *testing.T) {
	buffer := newLogRingBuffer(3)
	for index := 0; index < 6; index++ {
		if _, err := buffer.Write([]byte("line-" + string(rune('a'+index)) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	tail := buffer.Tail()
	if len(tail) != 3 {
		t.Fatalf("环形缓冲长度 = %d, want 3: %#v", len(tail), tail)
	}
	if tail[0] != "line-d" || tail[2] != "line-f" {
		t.Fatalf("环形缓冲内容 = %#v", tail)
	}
}

// TestCrashReportsPruneAndDelete 覆盖数量上限与删除。
func TestCrashReportsPruneAndDelete(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	dir := a.crashDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < crashReportMaxKeep+5; index++ {
		name := filepath.Join(dir, "crash-20260922-0000"+string(rune('a'+index%26))+".json")
		if _, err := a.writeCrashReport(CrashReport{Kind: crashReportKindPanic, Reason: name}); err != nil {
			t.Fatal(err)
		}
	}
	files := listCrashReportFiles(dir)
	if len(files) > crashReportMaxKeep {
		t.Fatalf("报告数量应受上限约束: %d", len(files))
	}

	reports, err := a.ListCrashReports()
	if err != nil {
		t.Fatal(err)
	}
	target := reports[0].FileName
	if err := a.DeleteCrashReport(target); err != nil {
		t.Fatal(err)
	}
	after, err := a.ListCrashReports()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(reports)-1 {
		t.Fatalf("删除后数量 = %d, want %d", len(after), len(reports)-1)
	}

	// 路径穿越与非法文件名必须被拒绝。
	if err := a.DeleteCrashReport("../config.json"); err == nil {
		t.Fatal("应拒绝非法文件名")
	}
	if _, err := a.ReadCrashReport("../config.json"); err == nil {
		t.Fatal("应拒绝读取非法文件名")
	}
}
