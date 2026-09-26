package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetDownloadTasks() []*DownloadTask {
	taskManager.mu.RLock()
	live := make(map[string]*DownloadTask, len(taskManager.tasks))
	for id, t := range taskManager.tasks {
		live[id] = t
	}
	taskManager.mu.RUnlock()

	// 先把上次退出时留下的快照补进内存（重试、清空等接口对它们同样可用），
	// 再返回内存 + 快照的合并结果。
	a.restoreDownloadTasksSnapshotTasks()

	return mergeDownloadTaskSnapshot(a.readDownloadTaskSnapshot(), live)
}

// ClearCompletedTasks removes completed and failed tasks
func (a *App) ClearCompletedTasks() {
	taskManager.mu.Lock()
	for id, t := range taskManager.tasks {
		if isTerminalDownloadStatus(t.Status) || t.Status == "interrupted" {
			delete(taskManager.tasks, id)
		}
	}
	taskManager.mu.Unlock()

	_ = a.persistDownloadTasks(true)
	runtime.EventsEmit(a.ctx, "tasks_cleared", nil)
}

func normalizeDownloadTaskPath(filePath string) string {
	if strings.TrimSpace(filePath) == "" {
		return ""
	}
	return filepath.Clean(filePath)
}

func isVPKDownloadTaskPath(filePath string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ".vpk")
}

func sameDownloadTaskPath(left string, right string) bool {
	return strings.EqualFold(normalizeDownloadTaskPath(left), normalizeDownloadTaskPath(right))
}

func setDownloadTaskFilePath(task *DownloadTask, filePath string) {
	filePath = normalizeDownloadTaskPath(filePath)
	if filePath == "" || !isVPKDownloadTaskPath(filePath) {
		return
	}

	taskManager.mu.Lock()
	task.FilePath = filePath
	taskManager.mu.Unlock()
}

func (a *App) updateCompletedDownloadTaskPath(oldPath string, newPath string) {
	oldPath = normalizeDownloadTaskPath(oldPath)
	newPath = normalizeDownloadTaskPath(newPath)
	if oldPath == "" || newPath == "" || sameDownloadTaskPath(oldPath, newPath) {
		return
	}

	updatedTasks := make([]DownloadTask, 0)

	taskManager.mu.Lock()
	for _, task := range taskManager.tasks {
		if task.Status == "completed" && sameDownloadTaskPath(task.FilePath, oldPath) {
			task.FilePath = newPath
			updatedTasks = append(updatedTasks, *task)
		}
	}
	taskManager.mu.Unlock()

	for i := range updatedTasks {
		runtime.EventsEmit(a.ctx, "task_updated", &updatedTasks[i])
	}
}

type TaskWriteCounter struct {
	Task        *DownloadTask
	Total       int64
	Current     int64
	Ctx         context.Context
	App         *App
	LastPercent int
	LastTime    time.Time
	LastBytes   int64
}

func (wc *TaskWriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Current += int64(n)

	// Update progress every 1%
	// Update speed every 3 seconds
	now := time.Now()
	duration := now.Sub(wc.LastTime)

	updateProgress := false
	updateSpeed := false

	if wc.Total > 0 {
		percent := int(float64(wc.Current) / float64(wc.Total) * 100)
		if percent > wc.LastPercent {
			wc.LastPercent = percent
			updateProgress = true
		}
	}

	if duration > 3*time.Second {
		updateSpeed = true
	}

	if updateProgress || updateSpeed {
		taskManager.mu.Lock()
		if wc.Total > 0 {
			wc.Task.Progress = wc.LastPercent
		}
		wc.Task.DownloadedSize = wc.Current

		if updateSpeed {
			// Calculate speed
			bytesDelta := wc.Current - wc.LastBytes
			speedBps := float64(bytesDelta) / duration.Seconds()
			wc.Task.Speed = formatSpeed(speedBps)

			// Reset speed counters
			wc.LastTime = now
			wc.LastBytes = wc.Current
		}

		taskManager.mu.Unlock()

		runtime.EventsEmit(wc.Ctx, "task_progress", wc.Task)
		if wc.App != nil {
			// 进度写盘同样按 1s 节流（对齐 FireAxe DownloadService.SaveDownloadProgressIntervalMs）。
			wc.App.persistDownloadTasksThrottled()
		}
	}
	return n, nil
}

func formatSpeed(bytesPerSec float64) string {
	if bytesPerSec < 1024 {
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	} else if bytesPerSec < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	} else {
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
	}
}

// ==================== Dynamic Chunked Download Functions ====================
