package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 真机背景：上传到一半关掉应用，任务记录直接消失 —— 既看不到"上次有个上传被中断"，
// 也没法点重试（而上传本身支持按服务端已收分片续传）。这里补的是任务快照。
func TestPanelUploadSnapshotRestoresInterruptedTasks(t *testing.T) {
	withIsolatedPanelUploadTasks(t)

	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	sourcePath := filepath.Join(dir, "campaign.vpk")
	if err := os.WriteFile(sourcePath, []byte("vpk"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{configDir: configDir}
	// 模拟"上传到一半退出"：内存里是 uploading，FilePath 已被换成临时 ZIP，
	// 原始 VPK 路径只留在 OriginalFilePath 上。
	panelUploads.mu.Lock()
	panelUploads.tasks["t1"] = &PanelMapUploadTask{
		ID:               "t1",
		ServerID:         "srv",
		ServerName:       "Panel",
		FilePath:         filepath.Join(dir, "temp-upload.zip"),
		Filename:         "campaign.zip",
		OriginalFilePath: sourcePath,
		UploadID:         "upload-1",
		Status:           "uploading",
		Progress:         50,
		TotalChunks:      4,
		UploadedChunks:   []int{0, 1, 2},
		TotalSize:        4 * panelMapUploadChunkSize,
		UploadedSize:     3 * panelMapUploadChunkSize,
		CreatedAt:        "2026-09-30 10:00:00",
	}
	panelUploads.tasks["done"] = &PanelMapUploadTask{
		ID:        "done",
		ServerID:  "srv",
		FilePath:  sourcePath,
		Filename:  "campaign.vpk",
		Status:    "completed",
		CreatedAt: "2026-09-30 09:00:00",
	}
	panelUploads.mu.Unlock()

	app.persistPanelUploadTasks(true)
	snapshotPath := filepath.Join(configDir, panelUploadSnapshotFileName)
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("快照没有写出来: %v", err)
	}

	// 模拟重启：清空内存再恢复
	panelUploads.mu.Lock()
	panelUploads.tasks = make(map[string]*PanelMapUploadTask)
	panelUploads.mu.Unlock()
	app.restorePanelUploadSnapshotTasks()

	panelUploads.mu.RLock()
	restored := panelUploads.tasks["t1"]
	done := panelUploads.tasks["done"]
	panelUploads.mu.RUnlock()

	if restored == nil {
		t.Fatalf("未完成的任务没有被恢复到内存里")
	}
	if restored.Status != panelUploadStatusInterrupted {
		t.Fatalf("重启后未完成任务应标成 interrupted，实际 %q", restored.Status)
	}
	if restored.Error != panelUploadInterruptedMessage {
		t.Fatalf("中断原因不对: %q", restored.Error)
	}
	if restored.FilePath != sourcePath {
		t.Fatalf("FilePath 应回到原始源文件，实际 %q", restored.FilePath)
	}
	if restored.Filename != "campaign.vpk" {
		t.Fatalf("Filename 应回到原始文件名，实际 %q", restored.Filename)
	}
	if restored.UploadID != "upload-1" {
		t.Fatalf("UploadID 要保留才能续传，实际 %q", restored.UploadID)
	}
	if len(restored.UploadedChunks) != 3 {
		t.Fatalf("已上传分片应保留，实际 %#v", restored.UploadedChunks)
	}
	if !isClearablePanelUploadStatus(restored.Status) {
		t.Fatalf("中断任务应该可以清空")
	}
	if done == nil || done.Status != "completed" {
		t.Fatalf("已完成的任务状态被改掉了: %#v", done)
	}

	// 中断的任务要能被「重试」接手（之前只接受 failed / cancelled）
	app.RetryPanelMapUpload("t1")
	panelUploads.mu.RLock()
	afterRetry := panelUploads.tasks["t1"].Status
	panelUploads.mu.RUnlock()
	if afterRetry == panelUploadStatusInterrupted {
		t.Fatalf("重试被忽略了，状态仍是 interrupted")
	}

	// 已完成的任务不该被重试改动
	app.RetryPanelMapUpload("done")
	panelUploads.mu.RLock()
	doneStatus := panelUploads.tasks["done"].Status
	panelUploads.mu.RUnlock()
	if doneStatus != "completed" {
		t.Fatalf("已完成的任务被重试改成了 %q", doneStatus)
	}

	// 上面那次重试会拉起后台上传协程（这里没有可用面板服务器，很快失败收尾）。
	// 必须等它结束再退出测试：协程失败后还要写一次任务快照，
	// 否则它会在 t.TempDir() 清理之后继续往 config 目录写文件 ——
	// CI 上偶发的 "TempDir RemoveAll cleanup: ... directory is not empty" 就是这么来的。
	if !waitPanelUploadWorkersIdle(15 * time.Second) {
		t.Fatal("后台上传协程没有在 15s 内收尾，测试无法安全清理临时目录")
	}
}

func TestPanelUploadSnapshotWithoutTasksKeepsFileStable(t *testing.T) {
	withIsolatedPanelUploadTasks(t)

	app := &App{configDir: t.TempDir()}
	app.persistPanelUploadTasks(true)

	snapshot := app.readPanelUploadSnapshot()
	if snapshot.SchemaVersion != panelUploadSnapshotSchemaVersion {
		t.Fatalf("schemaVersion = %d", snapshot.SchemaVersion)
	}
	if len(snapshot.Tasks) != 0 {
		t.Fatalf("空任务表不该写出任务: %#v", snapshot.Tasks)
	}
}
