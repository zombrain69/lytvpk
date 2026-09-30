package app

import (
	"context"
	"testing"
	"time"
)

// TestStartDownloadTaskStartsRealWorkerWithoutInjection 回归 ef3bf31：
// 生产代码没有注入 downloadTaskStarter，StartDownloadTask 必须回落到真实下载协程，
// 而不是调用 nil 直接 panic、让任务永远停在 pending。
func TestStartDownloadTaskStartsRealWorkerWithoutInjection(t *testing.T) {
	clearDownloadTaskStateForTest(t)
	// 模拟生产环境：没有任何测试替身。
	withDownloadTaskStarter(t, nil)
	// 未设置根目录时真实 worker 会立刻进入失败分支，无需任何网络访问。
	a := &App{}

	details := WorkshopFileDetails{
		PublishedFileId: "direct-starter-regression",
		Filename:        "starter-regression.vpk",
		FileUrl:         "http://127.0.0.1:1/starter-regression.vpk",
		Title:           "starter-regression",
	}
	id := a.StartDownloadTask(details, false)
	t.Cleanup(func() { removeDownloadTask(id) })

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		taskManager.mu.RLock()
		task := taskManager.tasks[id]
		status, errText := "", ""
		if task != nil {
			status, errText = task.Status, task.Error
		}
		taskManager.mu.RUnlock()

		if status != "" && status != "pending" {
			if status != "failed" || errText != "还没有选择 addons 目录，无法保存下载文件" {
				t.Fatalf("真实 worker 应在未设置根目录时快速失败，实际 status=%s error=%s", status, errText)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("3 秒内任务仍停留在 pending：真实下载启动器没有被调用")
}

// TestShouldUseOptimizedIP 直链任务永远不套用 Steam CDN 优选 IP。
func TestShouldUseOptimizedIP(t *testing.T) {
	workshopTask := &DownloadTask{WorkshopID: "123456"}
	directTask := &DownloadTask{WorkshopID: "direct-1699999999999"}

	cases := []struct {
		name      string
		preferred bool
		task      *DownloadTask
		want      bool
	}{
		{name: "工坊下载开启优选", preferred: true, task: workshopTask, want: true},
		{name: "工坊下载关闭优选", preferred: false, task: workshopTask, want: false},
		{name: "直链下载开启优选", preferred: true, task: directTask, want: false},
		{name: "空任务", preferred: true, task: nil, want: false},
	}
	for _, tc := range cases {
		if got := shouldUseOptimizedIP(tc.preferred, tc.task); got != tc.want {
			t.Fatalf("%s：shouldUseOptimizedIP=%v，期望 %v", tc.name, got, tc.want)
		}
	}
}

// TestStartDownloadTaskDirectDownloadIgnoresOptimizedIPFlag 覆盖前端传 true 的直链场景。
func TestStartDownloadTaskDirectDownloadIgnoresOptimizedIPFlag(t *testing.T) {
	clearDownloadTaskStateForTest(t)
	withDownloadTaskStarter(t, func(a *App, ctx context.Context, task *DownloadTask, url string) {})
	a := &App{}

	details := WorkshopFileDetails{
		PublishedFileId: "direct-optimized-flag",
		Filename:        "direct-optimized.vpk",
		FileUrl:         "http://127.0.0.1:8731/direct-optimized.vpk",
		Title:           "直链",
	}
	id := a.StartDownloadTask(details, true)
	t.Cleanup(func() { removeDownloadTask(id) })

	taskManager.mu.RLock()
	task := taskManager.tasks[id]
	taskManager.mu.RUnlock()
	if task == nil {
		t.Fatal("任务没有入队")
	}
	if task.UseOptimizedIP {
		t.Fatal("直链任务不应使用 Steam 优选 IP")
	}
}
