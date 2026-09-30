//go:build windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// lockFileExclusive 用「不允许任何共享」的方式打开文件，模拟游戏 / 杀毒软件 /
// 资源管理器预览正占着这个 VPK。注意 os.OpenFile 在 Windows 上默认允许共享，
// 挡不住重命名，所以这里必须直接调 CreateFile。
func lockFileExclusive(t *testing.T, path string) func() {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, // 不给任何共享权限
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("独占打开测试文件失败: %v", err)
	}
	closed := false
	return func() {
		// 句柄只能关一次：测试里会显式释放一次、再 defer 一次。
		if closed {
			return
		}
		closed = true
		_ = windows.CloseHandle(handle)
	}
}

// TestToggleVPKFileReportsOccupiedFileInChinese 是真机联调里那条链路的固定版本：
// 文件被别的进程占着时，禁用失败必须给出中文、可行动、只带文件名的提示，
// 而不是 Go 的 "rename … The process cannot access the file …"。
func TestToggleVPKFileReportsOccupiedFileInChinese(t *testing.T) {
	root := filepath.Join(t.TempDir(), "addons")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "occupied.vpk")
	if err := os.WriteFile(path, []byte("occupied"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "addonlist.txt"),
		[]byte("\"AddonList\"\n{\n\t\"occupied.vpk\"\t\t\"1\"\n}\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: root}
	app.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
		Path: path, Name: "occupied.vpk", Location: "root", Enabled: true,
	}})

	release := lockFileExclusive(t, path)
	defer release()

	err := app.ToggleVPKFile(path)
	if err == nil {
		t.Fatal("文件被独占占用时禁用应当失败，实际却成功了")
	}
	message := err.Error()
	if !strings.Contains(message, "occupied.vpk") {
		t.Fatalf("提示里要有文件名: %q", message)
	}
	if !strings.Contains(message, "占用") || !strings.Contains(message, "重试") {
		t.Fatalf("提示要说清原因与下一步: %q", message)
	}
	if strings.Contains(message, "The process cannot access") {
		t.Fatalf("不该把原始英文系统错误露给用户: %q", message)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("失败后文件应留在原位: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "disabled", "occupied.vpk")); statErr == nil {
		t.Fatal("失败后不该在 disabled 目录里留下文件")
	}

	// 锁释放后同一调用必须能成功，确认失败没有把缓存 / 状态搞脏。
	release()
	if err := app.ToggleVPKFile(path); err != nil {
		t.Fatalf("解锁后禁用仍然失败: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "disabled", "occupied.vpk")); statErr != nil {
		t.Fatalf("解锁后应能正常禁用: %v", statErr)
	}
}
