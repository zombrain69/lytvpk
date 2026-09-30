//go:build windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真机复现：把工坊源文件用独占句柄锁住，再走「复制到 addons」，
// 结果面板显示的是
//
//	open …\workshop\zztest_ws_dep.vpk: The process cannot access the file because
//	it is being used by another process.
//
// 复制路径（copyRegularFile / copyFileOverwrite）当时没有做中文化，这里钉住它。
func TestMoveWorkshopFilesToAddonsReportsOccupiedSourceInChinese(t *testing.T) {
	app, addonsDir := newPriorityTestApp(t)
	workshopPath := filepath.Join(addonsDir, "workshop", "123.vpk")
	if _, err := os.Stat(workshopPath); err != nil {
		t.Fatalf("夹具里应有工坊样本: %v", err)
	}
	app.vpkCache.Store(workshopPath, &VPKFileCache{File: VPKFile{
		Path: workshopPath, Name: "123.vpk", Location: "workshop", Enabled: true, WorkshopID: "123",
	}})

	release := lockFileExclusive(t, workshopPath)
	defer release()

	result, err := app.MoveWorkshopFilesToAddons([]string{workshopPath})
	if err != nil {
		t.Fatalf("批量转移不应整体失败: %v", err)
	}
	if result.FailCount != 1 || len(result.Items) != 1 {
		t.Fatalf("应恰好 1 个失败条目，实际 fail=%d items=%d", result.FailCount, len(result.Items))
	}
	item := result.Items[0]
	if !strings.Contains(item.Error, "占用") || !strings.Contains(item.Error, "重试") {
		t.Fatalf("失败原因应是中文可行动提示，实际 %q", item.Error)
	}
	if strings.Contains(item.Error, "The process cannot access") {
		t.Fatalf("失败原因里不该出现 Windows 英文原文：%q", item.Error)
	}
	if !strings.Contains(item.Error, "123.vpk") {
		t.Fatalf("提示里要带文件名，实际 %q", item.Error)
	}
	// 失败后不该在 addons 根目录留下半成品。
	if _, statErr := os.Stat(filepath.Join(addonsDir, "123.vpk")); statErr == nil {
		t.Fatal("复制失败后不该留下目标文件")
	}

	// 解锁后同一调用应当成功，确认失败没有污染状态。
	release()
	okResult, err := app.MoveWorkshopFilesToAddons([]string{workshopPath})
	if err != nil {
		t.Fatalf("解锁后转移失败: %v", err)
	}
	if okResult.SuccessCount != 1 {
		t.Fatalf("解锁后应成功 1 个，实际 %d（%#v）", okResult.SuccessCount, okResult.Items)
	}
	if _, statErr := os.Stat(filepath.Join(addonsDir, "123.vpk")); statErr != nil {
		t.Fatalf("解锁后应生成目标文件: %v", statErr)
	}
}
