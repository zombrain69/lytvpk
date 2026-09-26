package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// rangeTestServer 是一个支持 Range 的文件服务器，并记录每个请求的字节区间。
type rangeTestServer struct {
	server   *httptest.Server
	payload  []byte
	mu       *sync.Mutex
	ranges   *[]string
	onServed func(index int)
}

func newRangeTestServer(t *testing.T, size int) *rangeTestServer {
	t.Helper()
	payload := make([]byte, size)
	for index := range payload {
		payload[index] = byte(index % 251)
	}
	ranges := make([]string, 0)
	mu := &sync.Mutex{}
	served := 0
	holder := &rangeTestServer{payload: payload, mu: mu, ranges: &ranges}

	holder.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := -1
		mu.Lock()
		served++
		index = served
		mu.Unlock()

		header := strings.TrimSpace(r.Header.Get("Range"))
		if header == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(holder.payload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(holder.payload)
		} else {
			start, end, err := parseRangeHeader(header, int64(len(holder.payload)))
			if err != nil {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			mu.Lock()
			ranges = append(ranges, fmt.Sprintf("%d-%d", start, end))
			mu.Unlock()
			chunk := holder.payload[start : end+1]
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(chunk)
		}
		if holder.onServed != nil {
			holder.onServed(index)
		}
	}))
	t.Cleanup(holder.server.Close)
	return holder
}

// parseRangeHeader 解析 `bytes=start-end`。
func parseRangeHeader(header string, total int64) (int64, int64, error) {
	value := strings.TrimPrefix(header, "bytes=")
	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("非法 Range: %s", header)
	}
	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	end := total - 1
	if strings.TrimSpace(parts[1]) != "" {
		end, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil {
			return 0, 0, err
		}
	}
	if start < 0 || end >= total || start > end {
		return 0, 0, fmt.Errorf("Range 越界: %s", header)
	}
	return start, end, nil
}

func (s *rangeTestServer) requestedRanges() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), (*s.ranges)...)
}

// 完整下载：临时文件内容的 hash 必须等于源数据，收尾后不留下检查点。
func TestChunkedDownloadWritesExactBytes(t *testing.T) {
	const blockSize = int64(5 * 1024 * 1024)
	size := int(blockSize*2 + 1024)
	server := newRangeTestServer(t, size)
	tempDir := t.TempDir()

	task := &DownloadTask{ID: "resume-full", Filename: "full.vpk"}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	finalPath, err := newTestApp().processChunkedDownload(
		context.Background(), task, server.server.URL, "", int64(size), 2, tempDir,
	)
	if err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(data) != sha256.Sum256(server.payload) {
		t.Fatal("下载内容与源数据不一致")
	}
	if _, err := os.Stat(downloadBlockCheckpointPath(finalPath)); !os.IsNotExist(err) {
		t.Fatal("成功后不应留下检查点")
	}
}

// 断点续传：预置"第一块已完成"的临时文件 + 检查点，续传时不应再请求第一块的字节。
func TestChunkedDownloadResumesFromCheckpoint(t *testing.T) {
	const blockSize = int64(5 * 1024 * 1024)
	size := int(blockSize*3 + 128)
	server := newRangeTestServer(t, size)
	tempDir := t.TempDir()

	task := &DownloadTask{ID: "resume-half", Filename: "half.vpk"}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	// 预置：临时文件已经预分配好，并且第 0 块已经完整写入（模拟"上次下到一半暂停/退出"）。
	finalPath := filepath.Join(tempDir, task.ID+"_final")
	partial := make([]byte, size)
	copy(partial[:blockSize], server.payload[:blockSize])
	if err := os.WriteFile(finalPath, partial, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveBlockCheckpoint(downloadBlockCheckpointPath(finalPath), task.ID, int64(size), blockSize, []int{0}); err != nil {
		t.Fatal(err)
	}

	_, err := newTestApp().processChunkedDownload(context.Background(), task, server.server.URL, "", int64(size), 2, tempDir)
	if err != nil {
		t.Fatalf("续传失败: %v", err)
	}

	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(data) != sha256.Sum256(server.payload) {
		t.Fatal("续传后的文件与源数据不一致（把已完成的区块拼错了）")
	}

	requested := server.requestedRanges()
	if len(requested) == 0 {
		t.Fatal("续传应该仍然请求缺的区块")
	}
	for _, value := range requested {
		start, _, err := parseRangeHeader("bytes="+value, int64(size))
		if err != nil {
			t.Fatalf("解析请求区间失败: %v", err)
		}
		if start < blockSize {
			t.Fatalf("第 0 块已经完成，不应再请求它：%v（全部请求 %v）", value, requested)
		}
	}
	if len(requested) != 3 {
		t.Fatalf("应只请求剩下的 3 块，实际 %v", requested)
	}
	// 续传时进度不是从 0 开始。
	if task.Progress == 0 && size > 0 {
		t.Fatalf("续传应带上已有进度，实际 progress=%d", task.Progress)
	}
}

// 暂停：保留临时文件与检查点，并把任务留在 paused；继续后能下完整。
func TestChunkedDownloadPauseKeepsPartialData(t *testing.T) {
	const blockSize = int64(5 * 1024 * 1024)
	size := int(blockSize*2 + 1024)
	server := newRangeTestServer(t, size)
	tempDir := t.TempDir()

	task := &DownloadTask{ID: "resume-pause", Filename: "pause.vpk", Status: "downloading"}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	ctx, cancel := context.WithCancel(context.Background())
	// 第 2 个请求进来时"按下暂停"：此时第 0 块已经完整读完并被标记完成
	// （单 worker 是"完成一块才取下一块"），所以检查点里一定有第 0 块。
	done := make(chan struct{})
	server.onServed = func(index int) {
		if index == 2 {
			taskManager.mu.Lock()
			task.Status = "paused"
			taskManager.mu.Unlock()
			cancel()
			close(done)
		}
	}

	_, err := newTestApp().processChunkedDownload(ctx, task, server.server.URL, "", int64(size), 1, tempDir)
	if !errorsIsPaused(err) {
		t.Fatalf("应返回「已暂停」，实际 %v", err)
	}
	<-done

	finalPath := filepath.Join(tempDir, task.ID+"_final")
	if _, statErr := os.Stat(finalPath); statErr != nil {
		t.Fatalf("暂停后临时文件必须保留: %v", statErr)
	}
	checkpoint := loadBlockCheckpoint(downloadBlockCheckpointPath(finalPath), task.ID, int64(size), blockSize)
	if checkpoint == nil || len(checkpoint.Completed) == 0 {
		t.Fatalf("暂停后应留下检查点: %+v", checkpoint)
	}

	// 继续：把状态改回 downloading，续传剩余区块。
	taskManager.mu.Lock()
	task.Status = "downloading"
	taskManager.mu.Unlock()
	if _, err := newTestApp().processChunkedDownload(context.Background(), task, server.server.URL, "", int64(size), 2, tempDir); err != nil {
		t.Fatalf("继续下载失败: %v", err)
	}
	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(data) != sha256.Sum256(server.payload) {
		t.Fatal("继续下载后的文件与源数据不一致")
	}
}

// 残留数据不可信（大小对不上）时必须从头下，而不是把旧数据拼进去。
func TestChunkedDownloadRestartsWhenPartialFileIsTruncated(t *testing.T) {
	const blockSize = int64(5 * 1024 * 1024)
	size := int(blockSize*2 + 32)
	server := newRangeTestServer(t, size)
	tempDir := t.TempDir()

	task := &DownloadTask{ID: "resume-bad", Filename: "bad.vpk"}
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	finalPath := filepath.Join(tempDir, task.ID+"_final")
	if err := os.WriteFile(finalPath, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveBlockCheckpoint(downloadBlockCheckpointPath(finalPath), task.ID, int64(size), blockSize, []int{0}); err != nil {
		t.Fatal(err)
	}

	if _, err := newTestApp().processChunkedDownload(context.Background(), task, server.server.URL, "", int64(size), 2, tempDir); err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, server.payload) {
		t.Fatal("截断的残留文件必须被丢弃并重下")
	}
	// 从头下 ⇒ 第 0 块也会被重新请求。
	requested := server.requestedRanges()
	sawFirst := false
	for _, value := range requested {
		if strings.HasPrefix(value, "0-") {
			sawFirst = true
		}
	}
	if !sawFirst {
		t.Fatalf("应从第 0 块重新下载，实际请求 %v", requested)
	}
}

// newTestApp 造一个只用于下载路径的最小 App（不碰真实配置目录）。
func newTestApp() *App { return &App{} }

func errorsIsPaused(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), errDownloadPaused.Error()) || err == errDownloadPaused
}

// 暂停/继续的状态机：状态转换、持久化、恢复后仍是「已暂停」。
func TestPauseAndResumeDownloadTaskStateMachine(t *testing.T) {
	downloadTaskPersist = &downloadTaskPersister{}
	t.Cleanup(func() { downloadTaskPersist = &downloadTaskPersister{} })

	a := &App{}
	task := &DownloadTask{
		ID:             "pause-state",
		WorkshopID:     "123456",
		Filename:       "123456.vpk",
		FileUrl:        "https://example.invalid/123456.vpk",
		Status:         "downloading",
		DownloadedSize: 1024,
		CreatedAt:      "2026-09-26 10:00:00",
	}
	ctx, cancel := context.WithCancel(context.Background())
	task.cancelFunc = cancel
	seedDownloadTask(task)
	t.Cleanup(func() { removeDownloadTask(task.ID) })

	a.PauseDownloadTask(task.ID)
	taskManager.mu.RLock()
	pausedStatus := taskManager.tasks[task.ID].Status
	taskManager.mu.RUnlock()
	if pausedStatus != "paused" {
		t.Fatalf("暂停后状态 = %q", pausedStatus)
	}
	if ctx.Err() == nil {
		t.Fatal("暂停应真的取消底层下载上下文")
	}

	// 快照恢复：暂停状态必须保留（不能变成 interrupted）。
	snapshot := buildDownloadTaskSnapshot([]*DownloadTask{task}, time.Now())
	restored := mergeDownloadTaskSnapshot(snapshot, nil)
	if len(restored) != 1 || restored[0].Status != "paused" {
		t.Fatalf("重启后暂停任务应保持 paused，实际 %+v", restored)
	}

	// 继续：状态回到 pending 并重新启动下载（测试里用注入器接住，不触网）。
	var started int
	withDownloadTaskStarter(t, func(app *App, ctx context.Context, task *DownloadTask, url string) {
		started++
	})
	a.ResumeDownloadTask(task.ID)
	taskManager.mu.RLock()
	resumed := taskManager.tasks[task.ID]
	resumedStatus := resumed.Status
	resumedSize := resumed.DownloadedSize
	taskManager.mu.RUnlock()
	if resumedStatus != "pending" {
		t.Fatalf("继续后状态 = %q", resumedStatus)
	}
	if started != 1 {
		t.Fatalf("继续应重新启动下载，实际 %d 次", started)
	}
	if resumedSize != 1024 {
		t.Fatalf("继续时应保留已下载字节数，实际 %d", resumedSize)
	}

	// 暂停中的任务也能被「重试」按钮拉起（等价于继续），且不清零进度。
	a.PauseDownloadTask(task.ID)
	a.RetryDownloadTask(task.ID)
	taskManager.mu.RLock()
	retriedStatus := taskManager.tasks[task.ID].Status
	retriedSize := taskManager.tasks[task.ID].DownloadedSize
	taskManager.mu.RUnlock()
	if retriedStatus != "pending" || retriedSize != 1024 {
		t.Fatalf("重试暂停任务：状态 %q 进度 %d", retriedStatus, retriedSize)
	}

	// 取消：丢弃断点（临时文件清理走 rootDir，这里断言状态与错误文案）。
	a.CancelDownloadTask(task.ID)
	taskManager.mu.RLock()
	cancelled := taskManager.tasks[task.ID]
	cancelledStatus := cancelled.Status
	taskManager.mu.RUnlock()
	if cancelledStatus != "cancelled" {
		t.Fatalf("取消后状态 = %q", cancelledStatus)
	}
}
