package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// countDownloadTasks 返回当前任务表里的任务数量，测试用。
func countDownloadTasks() int {
	taskManager.mu.RLock()
	defer taskManager.mu.RUnlock()
	return len(taskManager.tasks)
}

func TestDownloadTaskDedupeKeyPrefersWorkshopID(t *testing.T) {
	cases := []struct {
		name       string
		workshopID string
		fileURL    string
		filename   string
		want       string
	}{
		{"工坊任务用 ID", "123456", "https://a/b", "123456.vpk", "workshop:123456"},
		{"工坊 ID 大小写归一", " 98765 ", "", "98765.vpk", "workshop:98765"},
		{"直链任务退回 URL", "direct-1", "https://example.com/a.vpk", "a.vpk", "url:https://example.com/a.vpk"},
		{"直链无 URL 退回文件名", "direct-1", "", "a.vpk", "file:a.vpk"},
		{"路径与大小写归一", "direct-1", "", " DIR\\b.VPK ", "file:" + strings.ToLower(filepath.Clean("DIR\\b.VPK"))},
		{"完全没有可用信息时不参与去重", "", "", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := downloadTaskDedupeKey(tc.workshopID, tc.fileURL, tc.filename)
			if got != tc.want {
				t.Fatalf("去重键 = %q, 期望 %q", got, tc.want)
			}
		})
	}
}

func TestIsTerminalDownloadStatus(t *testing.T) {
	terminal := []string{"completed", "failed", "cancelled"}
	active := []string{"pending", "downloading", "selecting_ip", "interrupted", ""}

	for _, status := range terminal {
		if !isTerminalDownloadStatus(status) {
			t.Fatalf("%s 应视为终态", status)
		}
	}
	for _, status := range active {
		if isTerminalDownloadStatus(status) {
			t.Fatalf("%s 不应视为终态", status)
		}
	}
}

// TestStartDownloadTaskDeduplicatesActiveTask 覆盖"同一件工坊作品只排一个任务"。
func TestStartDownloadTaskDeduplicatesActiveTask(t *testing.T) {
	withDownloadTaskStarter(t, func(a *App, ctx context.Context, task *DownloadTask, url string) {})
	a := &App{}

	details := WorkshopFileDetails{
		PublishedFileId: "777001",
		Filename:        "whatever.vpk",
		FileUrl:         "https://example.com/777001.vpk",
		Title:           "测试作品",
	}

	first := a.StartDownloadTask(details, false)
	t.Cleanup(func() { removeDownloadTask(first) })
	second := a.StartDownloadTask(details, false)

	if first != second {
		t.Fatalf("重复入队应复用已有任务：first=%s second=%s", first, second)
	}
	if got := countDownloadTasks(); got != 1 {
		t.Fatalf("任务数量 = %d, 期望 1", got)
	}

	// 终态任务不再占位，允许重新下载。
	taskManager.mu.Lock()
	taskManager.tasks[first].Status = "completed"
	taskManager.mu.Unlock()

	third := a.StartDownloadTask(details, false)
	t.Cleanup(func() { removeDownloadTask(third) })
	if third == first {
		t.Fatal("已完成的任务不应吞掉新的下载请求")
	}
}

// TestStartDownloadTaskDeduplicatesSameTargetFile 覆盖直链任务按目标文件去重。
func TestStartDownloadTaskDeduplicatesSameTargetFile(t *testing.T) {
	withDownloadTaskStarter(t, func(a *App, ctx context.Context, task *DownloadTask, url string) {})
	a := &App{}

	details := WorkshopFileDetails{
		PublishedFileId: "direct-1",
		Filename:        "我 的 材质.vpk",
		Title:           "直链",
	}

	first := a.StartDownloadTask(details, false)
	t.Cleanup(func() { removeDownloadTask(first) })

	same := details
	same.PublishedFileId = "direct-2"
	second := a.StartDownloadTask(same, false)
	if second != first {
		t.Fatalf("同目标文件的直链任务应复用：first=%s second=%s", first, second)
	}

	other := details
	other.Filename = "别的.vpk"
	third := a.StartDownloadTask(other, false)
	t.Cleanup(func() { removeDownloadTask(third) })
	if third == first {
		t.Fatal("目标文件不同的任务不应被去重")
	}
}

func TestMergeDownloadTaskSnapshotMarksInterrupted(t *testing.T) {
	snapshot := downloadTaskSnapshot{
		SchemaVersion: downloadTaskSnapshotSchemaVersion,
		Tasks: []persistedDownloadTask{
			{ID: "a", Status: "downloading", Progress: 42, DownloadedSize: 420, TotalSize: 1000},
			{ID: "b", Status: "completed", Progress: 100},
			{ID: "c", Status: "failed", Error: "网络错误"},
			{ID: "d", Status: "pending"},
		},
	}
	live := map[string]*DownloadTask{"c": {ID: "c", Status: "downloading"}}

	merged := mergeDownloadTaskSnapshot(snapshot, live)
	if len(merged) != 4 {
		t.Fatalf("合并后任务数 = %d, 期望 4", len(merged))
	}

	byID := make(map[string]*DownloadTask, len(merged))
	for _, task := range merged {
		byID[task.ID] = task
	}

	if byID["a"].Status != "interrupted" {
		t.Fatalf("未完成的下载应标记为 interrupted，实际 %q", byID["a"].Status)
	}
	if byID["a"].Progress != 42 || byID["a"].DownloadedSize != 420 {
		t.Fatalf("中断任务应保留已下载进度：progress=%d size=%d", byID["a"].Progress, byID["a"].DownloadedSize)
	}
	if byID["a"].Error == "" {
		t.Fatal("中断任务应带上可读的原因")
	}
	if byID["d"].Status != "interrupted" {
		t.Fatalf("pending 快照在重启后同样应变为 interrupted，实际 %q", byID["d"].Status)
	}
	if byID["b"].Status != "completed" {
		t.Fatalf("终态任务应保持原状态，实际 %q", byID["b"].Status)
	}
	if byID["c"].Status != "downloading" {
		t.Fatalf("内存中的任务应覆盖磁盘快照，实际 %q", byID["c"].Status)
	}
}

func TestDownloadTaskPersisterThrottlesAndSkipsIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "download_tasks.json")
	p := &downloadTaskPersister{}
	base := time.Unix(1_700_000_000, 0)

	written, err := p.save(path, []byte(`{"v":1}`), base, false)
	if err != nil || !written {
		t.Fatalf("首次写入应成功：written=%v err=%v", written, err)
	}

	written, err = p.save(path, []byte(`{"v":2}`), base.Add(500*time.Millisecond), false)
	if err != nil || written {
		t.Fatalf("1 秒内的第二次写入应被节流：written=%v err=%v", written, err)
	}

	written, err = p.save(path, []byte(`{"v":2}`), base.Add(2*time.Second), false)
	if err != nil || !written {
		t.Fatalf("超过间隔后应恢复写入：written=%v err=%v", written, err)
	}

	written, err = p.save(path, []byte(`{"v":2}`), base.Add(4*time.Second), false)
	if err != nil || written {
		t.Fatalf("内容与上一份相同时应跳过：written=%v err=%v", written, err)
	}

	written, err = p.save(path, []byte(`{"v":3}`), base.Add(4*time.Second+100*time.Millisecond), true)
	if err != nil || !written {
		t.Fatalf("force 写入应忽略 1s 节流：written=%v err=%v", written, err)
	}

	written, err = p.save(path, []byte(`{"v":3}`), base.Add(4*time.Second+200*time.Millisecond), false)
	if err != nil || written {
		t.Fatalf("节流窗口内的非 force 写入应被跳过：written=%v err=%v", written, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取落盘内容失败: %v", err)
	}
	if string(data) != `{"v":3}` {
		t.Fatalf("落盘内容 = %s", data)
	}
	if p.writes != 3 || p.skips != 3 {
		t.Fatalf("统计异常：writes=%d skips=%d", p.writes, p.skips)
	}
}

func TestPersistDownloadTasksWritesVersionedSnapshot(t *testing.T) {
	dir := t.TempDir()
	a := &App{configDir: dir}

	task := &DownloadTask{
		ID:         "persist-1",
		WorkshopID: "555",
		Title:      "落盘测试",
		Filename:   "555.vpk",
		FilePath:   filepath.Join(dir, "555.vpk"),
		Status:     "downloading",
		Progress:   10,
		TotalSize:  1000,
		CreatedAt:  "2026-09-24 10:00:00",
	}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	if err := a.persistDownloadTasks(true); err != nil {
		t.Fatalf("落盘失败: %v", err)
	}

	raw, err := os.ReadFile(a.downloadTasksPath)
	if err != nil {
		t.Fatalf("读取快照失败: %v", err)
	}
	var snapshot downloadTaskSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("快照不是合法 JSON: %v", err)
	}
	if snapshot.SchemaVersion != downloadTaskSnapshotSchemaVersion {
		t.Fatalf("schemaVersion = %d, 期望 %d", snapshot.SchemaVersion, downloadTaskSnapshotSchemaVersion)
	}
	if len(snapshot.Tasks) != 1 || snapshot.Tasks[0].ID != "persist-1" {
		t.Fatalf("快照任务异常: %+v", snapshot.Tasks)
	}

	// 内存里清空后，列表应从快照恢复出 interrupted 任务。
	removeDownloadTask(task.ID)
	restored := a.loadDownloadTasksSnapshot()
	if len(restored) != 1 {
		t.Fatalf("恢复任务数 = %d, 期望 1", len(restored))
	}
	if restored[0].Status != "interrupted" {
		t.Fatalf("恢复状态 = %q, 期望 interrupted", restored[0].Status)
	}
}

func TestDownloadTasksSnapshotSkipsMissingFile(t *testing.T) {
	a := &App{configDir: t.TempDir()}
	if got := a.loadDownloadTasksSnapshot(); len(got) != 0 {
		t.Fatalf("没有快照时应返回空列表，实际 %d 条", len(got))
	}
}

// TestCheckModUpdatesReusesInFlightCheck 覆盖"检查更新"在飞行中只跑一次。
func TestCheckModUpdatesReusesInFlightCheck(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls int
	var callsMu sync.Mutex

	previous := modUpdateCheckRunner
	modUpdateCheckRunner = func(a *App) UpdateCheckResult {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		close(started)
		<-release
		return UpdateCheckResult{TotalUpdates: 5, NewDetected: 2}
	}
	t.Cleanup(func() {
		modUpdateCheckRunner = previous
		resetModUpdateCheckState()
	})
	resetModUpdateCheckState()

	a := &App{workshopUpdateCheckEnabled: true, workshopMetaEnabled: true}

	var wg sync.WaitGroup
	results := make([]UpdateCheckResult, 2)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = a.CheckModUpdates()
		}(i)
	}

	<-started
	// 让第二个调用进入等待路径，再放行第一次检查。
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	callsMu.Lock()
	got := calls
	callsMu.Unlock()
	if got != 1 {
		t.Fatalf("同一时间只应跑一次检查，实际 %d 次", got)
	}
	for i, res := range results {
		if res.TotalUpdates != 5 || res.NewDetected != 2 {
			t.Fatalf("第 %d 个调用应复用结果，实际 %+v", i, res)
		}
	}
}

func TestCheckModUpdatesCanRunAgainAfterFinish(t *testing.T) {
	var calls int
	previous := modUpdateCheckRunner
	modUpdateCheckRunner = func(a *App) UpdateCheckResult {
		calls++
		return UpdateCheckResult{TotalUpdates: calls}
	}
	t.Cleanup(func() {
		modUpdateCheckRunner = previous
		resetModUpdateCheckState()
	})
	resetModUpdateCheckState()

	a := &App{workshopUpdateCheckEnabled: true, workshopMetaEnabled: true}
	if got := a.CheckModUpdates(); got.TotalUpdates != 1 {
		t.Fatalf("第一次结果 = %+v", got)
	}
	if got := a.CheckModUpdates(); got.TotalUpdates != 2 {
		t.Fatalf("第二次结果 = %+v", got)
	}
}

func TestCheckModUpdatesDisabledSkipsRunner(t *testing.T) {
	called := false
	previous := modUpdateCheckRunner
	modUpdateCheckRunner = func(a *App) UpdateCheckResult {
		called = true
		return UpdateCheckResult{}
	}
	t.Cleanup(func() {
		modUpdateCheckRunner = previous
		resetModUpdateCheckState()
	})
	resetModUpdateCheckState()

	a := &App{}
	if got := a.CheckModUpdates(); got.TotalUpdates != 0 {
		t.Fatalf("关闭时应返回空结果，实际 %+v", got)
	}
	if called {
		t.Fatal("关闭时不应执行检查")
	}
}
