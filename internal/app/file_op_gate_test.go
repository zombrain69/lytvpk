package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 对齐 FireAxe `BlockMove`：文件操作进行中时，另一批文件操作要被挡下并给出可读提示，
// 而不是两批操作互相插队（结果计数对不上、或同一文件被抢两次）。
func TestFileOperationsAreMutuallyExclusive(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	destDir := filepath.Join(filepath.Dir(addonsDir), "moved")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(addonsDir, "a.vpk")

	// 模拟"另一批文件操作正在跑"：手动占住闸门。
	if err := a.beginFileOperation(); err != nil {
		t.Fatalf("闸门应可用：%v", err)
	}

	if !a.IsFileOperationBusy() {
		t.Fatal("占住闸门后 IsFileOperationBusy 应为 true")
	}

	// ① 移动被挡下，且**一个文件都没动**。
	result, err := a.MoveVpkFiles([]string{sourcePath}, destDir)
	if err == nil {
		t.Fatal("文件操作进行中时移动应被挡下")
	}
	if !strings.Contains(err.Error(), "另一个文件操作正在进行") {
		t.Fatalf("提示应说明原因，实际 %v", err)
	}
	if result.SuccessCount != 0 || result.FailCount != 0 {
		t.Fatalf("被挡下的移动不应产生计数: %+v", result)
	}
	if _, statErr := os.Stat(sourcePath); statErr != nil {
		t.Fatalf("被挡下时源文件必须原样保留: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "a.vpk")); !os.IsNotExist(statErr) {
		t.Fatal("被挡下时不应有文件被移过去")
	}

	// ② 带冲突动作的移动同样被挡下。
	if _, err := a.MoveVpkFilesWithConflictAction([]string{sourcePath}, destDir, "skip"); err == nil {
		t.Fatal("MoveVpkFilesWithConflictAction 也应被挡下")
	}

	// ③ 删除被挡下。
	if err := a.DeleteVPKFiles([]string{sourcePath}); err == nil {
		t.Fatal("删除也应被挡下")
	}
	if _, statErr := os.Stat(sourcePath); statErr != nil {
		t.Fatalf("被挡下的删除不应删掉文件: %v", statErr)
	}

	// ④ 打包被挡下。
	packDir := filepath.Join(filepath.Dir(addonsDir), "packme")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PackVPKDirectory(packDir, destDir, false); err == nil {
		t.Fatal("打包也应被挡下")
	}

	// 释放闸门后一切照常。
	a.endFileOperation()
	if a.IsFileOperationBusy() {
		t.Fatal("释放后不应再是 busy")
	}
	result, err = a.MoveVpkFiles([]string{sourcePath}, destDir)
	if err != nil {
		t.Fatalf("释放后移动应成功: %v", err)
	}
	if result.SuccessCount != 1 {
		t.Fatalf("释放后应移动 1 个文件: %+v", result)
	}
	if _, statErr := os.Stat(filepath.Join(destDir, "a.vpk")); statErr != nil {
		t.Fatalf("移动后目标文件应存在: %v", statErr)
	}
}

// 修复流程内部会打包/解包，不能加同一把锁（否则自锁死）；这条测试守住"修复仍然可用"。
func TestRepairFlowStillRunsWhileGateIsFree(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	broken := filepath.Join(addonsDir, "broken.vpk")
	writeTestVPK(t, broken, map[string][]byte{
		"scripts/addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonSteamAppID \"550\"\n\taddonDescription \"\n未闭合\n}\n"),
	})

	if _, err := a.RepairVPKIntegrity(broken); err != nil {
		t.Fatalf("闸门空闲时修复应正常跑完（修复内部会调用解包/打包，不能再加同一把锁）: %v", err)
	}
	if a.IsFileOperationBusy() {
		t.Fatal("修复结束后闸门应已释放")
	}
}

// 状态事件在没有 Wails 上下文（测试 / 无界面调用）时必须是安全的空操作：
// beginFileOperation 会调用它，测试里 a.ctx 为 nil，不能因此 panic。
func TestEmitFileOperationStateWithoutContext(t *testing.T) {
	a := &App{}
	a.emitFileOperationState(true)
	a.emitFileOperationState(false)
	if err := a.beginFileOperation(); err != nil {
		t.Fatalf("闸门应可用: %v", err)
	}
	a.endFileOperation()
}
