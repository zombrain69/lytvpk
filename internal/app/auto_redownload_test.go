package app

import (
	"context"
	"testing"
)

// withDownloadTaskStarter 临时替换下载启动器，并在测试结束时恢复。
func withDownloadTaskStarter(t *testing.T, starter func(a *App, ctx context.Context, task *DownloadTask, url string)) {
	t.Helper()
	previous := downloadTaskStarter
	downloadTaskStarter = starter
	t.Cleanup(func() { downloadTaskStarter = previous })
}

func seedDownloadTask(task *DownloadTask) {
	taskManager.mu.Lock()
	taskManager.tasks[task.ID] = task
	taskManager.mu.Unlock()
}

func removeDownloadTask(id string) {
	taskManager.mu.Lock()
	delete(taskManager.tasks, id)
	taskManager.mu.Unlock()
}

func TestShouldAutoRedownloadDecision(t *testing.T) {
	base := &DownloadTask{Status: "failed", AutoRedownload: true}
	if !shouldAutoRedownload(base) {
		t.Fatal("开启自动重下的失败任务应触发重试")
	}
	cases := map[string]*DownloadTask{
		"未开启":    {Status: "failed", AutoRedownload: false},
		"已重试过":   {Status: "failed", AutoRedownload: true, RedownloadAttempts: 1},
		"非失败状态":  {Status: "downloading", AutoRedownload: true},
		"用户主动取消": {Status: "failed", AutoRedownload: true, Error: "Cancelled by user"},
		"空任务":    nil,
	}
	for name, task := range cases {
		if shouldAutoRedownload(task) {
			t.Fatalf("%s：不应触发自动重下", name)
		}
	}
}

// TestMaybeAutoRedownloadRetriesExactlyOnce 覆盖"失败后自动重试一次"的完整接线。
func TestMaybeAutoRedownloadRetriesExactlyOnce(t *testing.T) {
	a := &App{}
	started := make([]string, 0)
	withDownloadTaskStarter(t, func(_ *App, _ context.Context, task *DownloadTask, _ string) {
		started = append(started, task.ID)
	})

	task := &DownloadTask{
		ID:             "auto-retry-1",
		Status:         "failed",
		Error:          "network down",
		FileUrl:        "https://example.invalid/1.vpk",
		AutoRedownload: true,
	}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	if !a.maybeAutoRedownload(task.ID) {
		t.Fatal("首次失败应触发一次自动重下")
	}
	if len(started) != 1 || started[0] != task.ID {
		t.Fatalf("应只启动一次重试，实际 %#v", started)
	}

	taskManager.mu.RLock()
	attempts := taskManager.tasks[task.ID].RedownloadAttempts
	status := taskManager.tasks[task.ID].Status
	taskManager.mu.RUnlock()
	if attempts != 1 || status != "pending" {
		t.Fatalf("重试后任务状态 = %s / attempts=%d", status, attempts)
	}

	// 第二次失败不应再重试。
	taskManager.mu.Lock()
	taskManager.tasks[task.ID].Status = "failed"
	taskManager.tasks[task.ID].Error = "network down again"
	taskManager.mu.Unlock()
	if a.maybeAutoRedownload(task.ID) {
		t.Fatal("自动重下只允许一次")
	}
	if len(started) != 1 {
		t.Fatalf("不应再次启动下载: %#v", started)
	}
}

// TestMaybeAutoRedownloadIgnoresDisabledTasks 保证默认关闭时行为与现状一致。
func TestMaybeAutoRedownloadIgnoresDisabledTasks(t *testing.T) {
	a := &App{}
	started := 0
	withDownloadTaskStarter(t, func(_ *App, _ context.Context, _ *DownloadTask, _ string) {
		started++
	})

	task := &DownloadTask{ID: "auto-retry-off", Status: "failed", Error: "boom"}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	if a.maybeAutoRedownload(task.ID) {
		t.Fatal("未开启自动重下的任务不应被重试")
	}
	if started != 0 {
		t.Fatalf("不应启动下载: %d", started)
	}
}

// TestWorkshopAutoRedownloadDefaultsOffAndPersists 覆盖全局默认开关（默认关闭）。
func TestWorkshopAutoRedownloadDefaultsOffAndPersists(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if a.GetWorkshopAutoRedownload() {
		t.Fatal("自动重下默认必须关闭")
	}
	if err := a.SetWorkshopAutoRedownload(true); err != nil {
		t.Fatal(err)
	}
	if !a.GetWorkshopAutoRedownload() {
		t.Fatal("设置后应返回开启")
	}
	if !a.workshopAutoRedownloadSnapshot() {
		t.Fatal("任务创建时读取的快照应为开启")
	}

	reloaded := &App{configDir: a.configDir}
	reloaded.loadConfig()
	if !reloaded.GetWorkshopAutoRedownload() {
		t.Fatal("重新加载配置后应保持开启")
	}
}

// TestSetDownloadTaskAutoRedownloadTogglesTask 覆盖单个任务的开关。
func TestSetDownloadTaskAutoRedownloadTogglesTask(t *testing.T) {
	a := &App{}
	task := &DownloadTask{ID: "toggle-1", Status: "failed"}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	if err := a.SetDownloadTaskAutoRedownload(task.ID, true); err != nil {
		t.Fatal(err)
	}
	taskManager.mu.RLock()
	enabled := taskManager.tasks[task.ID].AutoRedownload
	taskManager.mu.RUnlock()
	if !enabled {
		t.Fatal("任务开关应被打开")
	}

	if err := a.SetDownloadTaskAutoRedownload("missing", true); err == nil {
		t.Fatal("不存在的任务应报错")
	}
}
