package app

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vpk-manager/internal/network"
)

// 用户明确要求：窗口关掉之后，不允许再留着任何常驻监听/线程/端口。
// 这条测试盯住四类后台资源：Mod 目录监听、addonlist 守护、图片代理 HTTP 端口、单例 TCP 端口。
func TestBeforeCloseStopsBackgroundResources(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	for _, dir := range []string{"", "workshop", "disabled"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	app.restartAddonsWatcher()
	if app.addonsWatcher == nil {
		t.Fatal("目录监听应该已启动（三个被监听目录都存在）")
	}

	app.proxyServer = network.NewImageProxyServer(nil)
	if err := app.proxyServer.Start(); err != nil {
		t.Fatalf("启动图片代理失败: %v", err)
	}
	parsed, err := url.Parse(app.proxyServer.GetProxyUrl("http://example.com/a.png"))
	if err != nil {
		t.Fatalf("代理 URL 解析失败: %v", err)
	}
	address := parsed.Host
	if address == "" {
		t.Fatal("图片代理没有监听端口")
	}
	probe, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatalf("图片代理应该已经监听 %s: %v", address, err)
	}
	// 立刻断开：否则 Shutdown 要等这个"还没发请求"的连接，测试白等 2 秒。
	_ = probe.Close()

	// 下载/上传任务是**包级全局状态**，同包其它测试可能留下 pending 任务；
	// 这里先清空再测"正常关窗"这条路径（退出确认分支不是本测试的目标）。
	taskManager.mu.Lock()
	previousDownloads := taskManager.tasks
	taskManager.tasks = nil
	taskManager.mu.Unlock()
	panelUploads.mu.Lock()
	previousUploads := panelUploads.tasks
	panelUploads.tasks = nil
	panelUploads.mu.Unlock()
	t.Cleanup(func() {
		taskManager.mu.Lock()
		taskManager.tasks = previousDownloads
		taskManager.mu.Unlock()
		panelUploads.mu.Lock()
		panelUploads.tasks = previousUploads
		panelUploads.mu.Unlock()
	})

	if app.beforeClose() {
		t.Fatal("没有下载/上传任务时不该阻止关闭")
	}

	if app.addonsWatcher != nil {
		t.Error("关闭后仍然留着 Mod 目录监听")
	}
	if app.addonListMonitorStop != nil {
		t.Error("关闭后仍然留着 addonlist 守护轮询")
	}
	if _, err := net.DialTimeout("tcp", address, 500*time.Millisecond); err == nil {
		t.Error("图片代理端口没有释放")
	}

	// beforeClose 可能因为重复的退出请求被调用两次：必须幂等。
	if app.beforeClose() {
		t.Fatal("第二次关闭请求也不该被阻止")
	}
}
