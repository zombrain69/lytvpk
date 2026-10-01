package app

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// 扫描不能"每个文件打一行日志"。
//
// 真机背景（2904 个 Mod）：缓存命中路径上原本有一行 `使用缓存: … (未变化)`，
// 每次扫描就是 2904 行 —— 格式化 + 写 stderr，既拖慢扫描，又会把崩溃报告
// 环形缓冲里真正有用的诊断刷掉。现在只保留 ScanVPKFiles 结束时的一行汇总。
func TestScanLogsOneSummaryLineInsteadOfOnePerFile(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	var buffer bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buffer)
	defer log.SetOutput(previous)

	// 第一次扫描：解析并缓存（每个文件一行"已解析并缓存"也应当已经去掉）。
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描失败: %v", err)
	}
	firstLines := countLogLines(buffer.String())
	buffer.Reset()

	// 第二次扫描：全部命中缓存 —— 这正是真机上"刷新/文件操作后重扫"的路径。
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("二次扫描失败: %v", err)
	}
	second := buffer.String()
	secondLines := countLogLines(second)

	if !strings.Contains(second, "扫描完成：共") {
		t.Fatalf("扫描结束要有一行汇总日志，实际输出：%q", second)
	}
	// 汇总行必须带耗时：冷启动（全量解析）与命中缓存（只 stat）的差别就靠它量化。
	if !strings.Contains(second, "耗时 ") {
		t.Fatalf("扫描汇总要带耗时，实际输出：%q", second)
	}
	// 只允许"汇总/缓存"这类每次扫描固定条数的日志；逐个文件的那种必须消失。
	// （批量文件的缓存在，因此行数上限给到 3：汇总 + 载入缓存 + 极端情况下的写盘告警。）
	if secondLines > 3 {
		t.Fatalf("一次扫描的日志条数异常（%d 行）：\n%s", secondLines, second)
	}
	if firstLines > 3 {
		t.Fatalf("首次扫描的日志条数异常（%d 行）：\n%s", firstLines, buffer.String())
	}
	if strings.Contains(second, "使用缓存:") {
		t.Fatalf("不应再逐个文件打「使用缓存」日志：\n%s", second)
	}
	if strings.Contains(second, "已解析并缓存:") {
		t.Fatalf("不应再逐个文件打「已解析并缓存」日志：\n%s", second)
	}
}

func countLogLines(output string) int {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}
