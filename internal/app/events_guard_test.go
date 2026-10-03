package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 没有 Wails 上下文时（后台任务、CLI、启动早期、测试）任何事件都不能调用
// runtime.EventsEmit：Wails 对无效 ctx 是 log.Fatal，会把整个进程带走。
// 这组用例覆盖本轮补上的几处后台路径（Mod 轮换 / 模型扫描 / 下载任务清理）。
func TestEmitEventWithoutWailsContextDoesNotAbort(t *testing.T) {
	app := &App{}
	app.emitEvent("unit_test_event", map[string]string{"hello": "world"})
	app.emitEvent("") // 空事件名也不应崩

	var nilApp *App
	nilApp.emitEvent("unit_test_event", nil) // 防御性：nil 接收者
}

// 轮换链路（rotation_log + refresh_files）在无 ctx 时必须只写日志。
func TestRotationWithoutWailsContextDoesNotAbort(t *testing.T) {
	app := &App{}
	path := filepath.Join(t.TempDir(), "ak47.vpk")
	// 放一个"已启用 + 官方武器标签"的缓存条目：轮换会走到 logMsg 与 refresh_files 两处 emit，
	// 且因为本来就是启用状态不会触发 ToggleVPKFile 的文件操作。
	app.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
		Path:          path,
		Name:          "ak47.vpk",
		Enabled:       true,
		PrimaryTag:    "武器",
		SecondaryTags: []string{"AK47"},
	}})
	if err := app.rotateModsInternal(RotationConfig{EnableWeapons: true}); err != nil {
		t.Fatalf("无 Wails 上下文的轮换不应报错: %v", err)
	}
}

// 模型扫描的进度/完成事件走 emitEvent 包装，无 ctx 时必须静默跳过。
func TestModelStatsScanEmitsWithoutWailsContextDoNotAbort(t *testing.T) {
	app := &App{}
	app.emitModelStatsScanProgress("unit-test", 1, 2, "进行中")
	app.emitModelStatsScanComplete("unit-test", &ModelStatsScanResult{}, "")
}

// 清空下载任务：终态清理后的事件同样不能把进程带走。
func TestClearCompletedTasksWithoutWailsContextDoesNotAbort(t *testing.T) {
	app := &App{}
	app.ClearCompletedTasks()
}

// 静态护栏：任何直接调用 runtime.EventsEmit(a.ctx, ...) 的文件都必须自己出现 nil 判断。
// lifecycle.go 的 ctx 由 Wails 启动钩子保证；events.go 是判空包装本身。
func TestEventsEmitCallsAreNilSafe(t *testing.T) {
	allowlist := map[string]bool{"lifecycle.go": true, "events.go": true}
	entries, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("列目录失败: %v", err)
	}
	for _, name := range entries {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", name, err)
		}
		text := string(source)
		if !strings.Contains(text, "EventsEmit(a.ctx") {
			continue
		}
		if allowlist[name] {
			continue
		}
		if !strings.Contains(text, "a.ctx == nil") && !strings.Contains(text, "a.ctx != nil") {
			t.Errorf(
				"%s 直接调用了 runtime.EventsEmit(a.ctx, ...) 却没有 nil 判断：Wails 对无效 ctx 会 log.Fatal，请改走 a.emitEvent(...) 或加判空",
				name,
			)
		}
	}
}
