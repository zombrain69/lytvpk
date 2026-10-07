package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 沙箱只读闸门：设置环境变量后，addonlist 写盘与文件操作都必须当场被拒绝。
func TestReadonlyLibraryGateBlocksWrites(t *testing.T) {
	t.Setenv(readonlyLibraryEnv, "1")

	if !readonlyLibraryMode() {
		t.Fatalf("readonlyLibraryMode() = false，期望 true（%s=1）", readonlyLibraryEnv)
	}

	app := &App{}

	// 1) 文件操作闸门（移动 / 删除 / 打包 / 归档）
	if err := app.beginFileOperation(); err == nil {
		t.Fatalf("beginFileOperation() 未被只读闸门拦住")
	} else if !strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("beginFileOperation() 错误消息不含闸门说明: %v", err)
	}
	// 被拒绝时不应留下"忙碌"状态（否则界面会一直显示文件操作中）。
	if app.IsFileOperationBusy() {
		t.Fatalf("被只读闸门拒绝后 fileOps 仍处于忙碌状态")
	}

	// 2) addonlist.txt 写盘（列表式 / 文档式都走同一个事务函数）
	path := filepath.Join(t.TempDir(), "addonlist.txt")
	original := "\"AddonList\"\n{\n\t\"a.vpk\"\t\t\"1\"\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	err := app.commitAddonListItemsLocked(path, []AddonListItem{{Name: "a.vpk", Value: "0"}}, nil)
	if err == nil {
		t.Fatalf("commitAddonListItemsLocked 未被只读闸门拦住")
	}
	if !strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("addonlist 写入错误消息不含闸门说明: %v", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != original {
		t.Fatalf("只读模式下 addonlist.txt 被改写：\n%q", string(content))
	}

	// 3) 单文件删除 / 重命名 / 导入入口
	if err := app.DeleteVPKFile(filepath.Join(t.TempDir(), "a.vpk")); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("DeleteVPKFile 未被只读闸门拦住: %v", err)
	}
	if _, err := app.ToggleVPKVisibility(filepath.Join(t.TempDir(), "a.vpk")); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("ToggleVPKVisibility 未被只读闸门拦住: %v", err)
	}
	if _, err := app.RenameVPKFile(filepath.Join(t.TempDir(), "a.vpk"), "b.vpk"); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("RenameVPKFile 未被只读闸门拦住: %v", err)
	}
	if _, err := app.HandleFileDrop([]string{filepath.Join(t.TempDir(), "a.vpk")}); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("HandleFileDrop 未被只读闸门拦住: %v", err)
	}
	if err := app.ToggleVPKFile(filepath.Join(t.TempDir(), "a.vpk")); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("ToggleVPKFile（游戏内启用/禁用）未被只读闸门拦住: %v", err)
	}
	if err := app.MoveWorkshopToAddons(filepath.Join(t.TempDir(), "a.vpk")); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("MoveWorkshopToAddons 未被只读闸门拦住: %v", err)
	}
	if _, err := app.MoveWorkshopFilesToAddons([]string{filepath.Join(t.TempDir(), "a.vpk")}); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("MoveWorkshopFilesToAddons 未被只读闸门拦住: %v", err)
	}
	if err := app.ExtractVPKFromArchive(filepath.Join(t.TempDir(), "a.zip"), t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("ExtractVPKFromArchive 未被只读闸门拦住: %v", err)
	}

	// 4) addonlist 备份 / 恢复 / 删除，以及底层原子写入口
	if _, err := app.CreateAddonListBackup(); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("CreateAddonListBackup 未被只读闸门拦住: %v", err)
	}
	if _, err := app.RestoreAddonListBackup("manual-20260101T000000.000000000Z.txt"); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("RestoreAddonListBackup 未被只读闸门拦住: %v", err)
	}
	if err := app.DeleteAddonListBackup("manual-20260101T000000.000000000Z.txt"); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("DeleteAddonListBackup 未被只读闸门拦住: %v", err)
	}
	if err := app.DeleteAddonList(); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("DeleteAddonList 未被只读闸门拦住: %v", err)
	}
	atomicTarget := filepath.Join(t.TempDir(), "addonlist.txt")
	if err := writeAddonListBytesAtomically(atomicTarget, []byte(original)); err == nil ||
		!strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("writeAddonListBytesAtomically 未被只读闸门拦住: %v", err)
	}
	if _, statErr := os.Stat(atomicTarget); !os.IsNotExist(statErr) {
		t.Fatalf("只读模式下底层写入口竟然创建了文件: %v", statErr)
	}
}

// 默认（不设变量）时闸门必须完全透明：文件操作闸门照常放行。
func TestReadonlyLibraryGateIsInertByDefault(t *testing.T) {
	t.Setenv(readonlyLibraryEnv, "")
	if readonlyLibraryMode() {
		t.Fatalf("未设置 %s 时 readonlyLibraryMode() 应为 false", readonlyLibraryEnv)
	}
	app := &App{}
	if err := app.beginFileOperation(); err != nil {
		t.Fatalf("默认模式下 beginFileOperation() 失败: %v", err)
	}
	app.endFileOperation()
}

// 常见真值写法都要能打开闸门（launcher 用 '1'，手工跑可能写 true/on）。
func TestReadonlyLibraryModeTruthyValues(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", "yes", "on", " On "} {
		t.Setenv(readonlyLibraryEnv, value)
		if !readonlyLibraryMode() {
			t.Fatalf("%s=%q 应判定为只读模式", readonlyLibraryEnv, value)
		}
	}
	for _, value := range []string{"", "0", "false", "off", "no"} {
		t.Setenv(readonlyLibraryEnv, value)
		if readonlyLibraryMode() {
			t.Fatalf("%s=%q 不应判定为只读模式", readonlyLibraryEnv, value)
		}
	}
}
