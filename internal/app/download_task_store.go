package app

import (
	"crypto/sha256"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 下载任务的「入队去重 + 进度节流落盘」对齐 FireAxe：
//   - FireAxe.Core/DownloadService.cs:11 定义 SaveDownloadProgressIntervalMs = 1000，
//     进度回调里只在距上次写盘超过 1s 时才落盘（同文件 374-384 行）。
//   - FireAxe.Core/WorkshopVpkAddon.cs:428-450 的 CheckDownloadAsync 把进行中的
//     检查任务存进 DownloadCheckTask 字段，重复调用直接复用同一个 Task。
//
// 本项目的下载任务原本只活在内存里，应用重启后进度全丢。这里补一份带
// schemaVersion 的快照，并保持「内存状态实时、磁盘快照最多每秒一次」的节奏。
const (
	downloadTaskSnapshotFileName      = "download_tasks.json"
	downloadTaskSnapshotSchemaVersion = 1
	// downloadTaskPersistInterval 与 FireAxe 的 SaveDownloadProgressIntervalMs 对齐。
	downloadTaskPersistInterval = time.Second
	// downloadTaskSnapshotLimit 限制快照里保留的任务条数，避免文件无限增长。
	downloadTaskSnapshotLimit = 200
	// downloadTaskInterruptedMessage 是重启后未完成任务显示的原因。
	downloadTaskInterruptedMessage = "应用退出时该下载还没完成，可点击重试继续。"
)

// isTerminalDownloadStatus 判断状态是否已经结束（结束后不再占用去重名额）。
func isTerminalDownloadStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// downloadTaskDedupeKey 计算「同一份下载」的去重键。
// 工坊任务用 publishedFileId（同一件作品同时只允许一个任务）；
// 直链任务没有稳定 ID，用 URL 或最终文件名兜底。返回空串表示无法判定，不参与去重。
func downloadTaskDedupeKey(workshopID string, fileURL string, filename string) string {
	if id := strings.TrimSpace(workshopID); id != "" && !strings.HasPrefix(strings.ToLower(id), "direct-") {
		return "workshop:" + strings.ToLower(id)
	}
	if url := strings.TrimSpace(fileURL); url != "" {
		return "url:" + url
	}
	name := strings.TrimSpace(filename)
	if name == "" {
		return ""
	}
	return "file:" + strings.ToLower(filepath.Clean(name))
}

// persistedDownloadTask 是落盘用的任务快照，字段比内存结构保守，只保留可恢复的部分。
type persistedDownloadTask struct {
	ID                 string `json:"id"`
	WorkshopID         string `json:"workshopId,omitempty"`
	Title              string `json:"title,omitempty"`
	Filename           string `json:"filename,omitempty"`
	FilePath           string `json:"filePath,omitempty"`
	PreviewUrl         string `json:"previewUrl,omitempty"`
	FileUrl            string `json:"fileUrl,omitempty"`
	UseOptimizedIP     bool   `json:"useOptimizedIp,omitempty"`
	Status             string `json:"status"`
	Progress           int    `json:"progress"`
	TotalSize          int64  `json:"totalSize"`
	DownloadedSize     int64  `json:"downloadedSize"`
	Error              string `json:"error,omitempty"`
	Description        string `json:"description,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	AutoRedownload     bool   `json:"autoRedownload,omitempty"`
	RedownloadAttempts int    `json:"redownloadAttempts,omitempty"`
}

// downloadTaskSnapshot 是 download_tasks.json 的顶层结构。
type downloadTaskSnapshot struct {
	SchemaVersion int                     `json:"schemaVersion"`
	SavedAt       string                  `json:"savedAt,omitempty"`
	Tasks         []persistedDownloadTask `json:"tasks"`
}

func downloadTaskToPersisted(task *DownloadTask) persistedDownloadTask {
	return persistedDownloadTask{
		ID:                 task.ID,
		WorkshopID:         task.WorkshopID,
		Title:              task.Title,
		Filename:           task.Filename,
		FilePath:           task.FilePath,
		PreviewUrl:         task.PreviewUrl,
		FileUrl:            task.FileUrl,
		UseOptimizedIP:     task.UseOptimizedIP,
		Status:             task.Status,
		Progress:           task.Progress,
		TotalSize:          task.TotalSize,
		DownloadedSize:     task.DownloadedSize,
		Error:              task.Error,
		Description:        task.Description,
		CreatedAt:          task.CreatedAt,
		AutoRedownload:     task.AutoRedownload,
		RedownloadAttempts: task.RedownloadAttempts,
	}
}

func persistedToDownloadTask(saved persistedDownloadTask) *DownloadTask {
	return &DownloadTask{
		ID:                 saved.ID,
		WorkshopID:         saved.WorkshopID,
		Title:              saved.Title,
		Filename:           saved.Filename,
		FilePath:           saved.FilePath,
		PreviewUrl:         saved.PreviewUrl,
		FileUrl:            saved.FileUrl,
		UseOptimizedIP:     saved.UseOptimizedIP,
		Status:             saved.Status,
		Progress:           saved.Progress,
		TotalSize:          saved.TotalSize,
		DownloadedSize:     saved.DownloadedSize,
		Error:              saved.Error,
		Description:        saved.Description,
		CreatedAt:          saved.CreatedAt,
		AutoRedownload:     saved.AutoRedownload,
		RedownloadAttempts: saved.RedownloadAttempts,
	}
}

// buildDownloadTaskSnapshot 生成确定有序的快照；条数按创建时间倒序截断。
func buildDownloadTaskSnapshot(tasks []*DownloadTask, savedAt time.Time) downloadTaskSnapshot {
	ordered := make([]*DownloadTask, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			ordered = append(ordered, task)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt != ordered[j].CreatedAt {
			return ordered[i].CreatedAt > ordered[j].CreatedAt
		}
		return ordered[i].ID > ordered[j].ID
	})

	snapshot := downloadTaskSnapshot{
		SchemaVersion: downloadTaskSnapshotSchemaVersion,
		SavedAt:       savedAt.Format(time.RFC3339),
		Tasks:         make([]persistedDownloadTask, 0, len(ordered)),
	}
	for _, task := range ordered {
		if len(snapshot.Tasks) >= downloadTaskSnapshotLimit {
			break
		}
		snapshot.Tasks = append(snapshot.Tasks, downloadTaskToPersisted(task))
	}
	return snapshot
}

// mergeDownloadTaskSnapshot 把磁盘快照与内存任务合并：内存任务优先，
// 快照里非终态的任务统一降级成 interrupted（重启后不会被自动续跑）。
func mergeDownloadTaskSnapshot(snapshot downloadTaskSnapshot, live map[string]*DownloadTask) []*DownloadTask {
	merged := make([]*DownloadTask, 0, len(snapshot.Tasks)+len(live))
	seen := make(map[string]bool, len(snapshot.Tasks)+len(live))

	for _, saved := range snapshot.Tasks {
		if saved.ID == "" || seen[saved.ID] {
			continue
		}
		if _, exists := live[saved.ID]; exists {
			continue
		}
		seen[saved.ID] = true
		task := persistedToDownloadTask(saved)
		// 「已暂停」是用户意图，重启后要保持可继续（FireAxe 的 Resume 语义）；
		// 只有"还在跑但应用退出了"的任务才降级成「已中断」。
		if !isTerminalDownloadStatus(task.Status) && task.Status != "paused" {
			task.Status = "interrupted"
			task.Speed = ""
			task.Error = downloadTaskInterruptedMessage
		}
		merged = append(merged, task)
	}

	for id, task := range live {
		if task == nil || seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, task)
	}

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].CreatedAt != merged[j].CreatedAt {
			return merged[i].CreatedAt > merged[j].CreatedAt
		}
		return merged[i].ID > merged[j].ID
	})
	return merged
}

// writeFileAtomically 先写同目录临时文件再原子替换，避免半截 JSON 留在磁盘上。
func writeFileAtomically(path string, content []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, content, 0644); err != nil {
		return err
	}
	if err := replaceFile(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}
	return nil
}

// downloadTaskPersister 负责节流：内容相同跳过、距上次写盘不足 1s 跳过（force 例外）。
type downloadTaskPersister struct {
	mu        sync.Mutex
	lastPath  string
	lastWrite time.Time
	lastHash  [sha256.Size]byte
	hasHash   bool
	wroteOnce bool
	writes    int
	skips     int
}

var downloadTaskPersist = &downloadTaskPersister{}

func (p *downloadTaskPersister) save(path string, content []byte, now time.Time, force bool) (bool, error) {
	if path == "" {
		return false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !force && p.wroteOnce && now.Sub(p.lastWrite) < downloadTaskPersistInterval {
		p.skips++
		return false, nil
	}

	hash := sha256.Sum256(content)
	// 只有"同一个文件 + 内容相同"才跳过；换了路径时必须真的写出去。
	if p.hasHash && hash == p.lastHash && p.lastPath == path {
		p.skips++
		return false, nil
	}

	if err := writeFileAtomically(path, content); err != nil {
		return false, err
	}

	p.lastWrite = now
	p.lastPath = path
	p.lastHash = hash
	p.hasHash = true
	p.wroteOnce = true
	p.writes++
	return true, nil
}

func (a *App) ensureDownloadTasksPath() string {
	a.ensureConfigPaths()
	if a.downloadTasksPath == "" && a.configDir != "" {
		a.downloadTasksPath = filepath.Join(a.configDir, downloadTaskSnapshotFileName)
	}
	return a.downloadTasksPath
}

// persistDownloadTasks 把当前任务表按节流策略写盘；force 用于创建、终态等关键节点。
func (a *App) persistDownloadTasks(force bool) error {
	path := a.ensureDownloadTasksPath()
	if path == "" {
		return nil
	}

	taskManager.mu.RLock()
	tasks := make([]*DownloadTask, 0, len(taskManager.tasks))
	for _, task := range taskManager.tasks {
		tasks = append(tasks, task)
	}
	taskManager.mu.RUnlock()

	if len(tasks) == 0 {
		// 没有任务时不创建空文件；已有文件保持原样，交由 ClearCompletedTasks 覆盖。
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil
		}
	}

	content, err := json.MarshalIndent(buildDownloadTaskSnapshot(tasks, time.Now()), "", "  ")
	if err != nil {
		return err
	}

	written, err := downloadTaskPersist.save(path, content, time.Now(), force)
	if err != nil {
		log.Printf("写入下载任务快照失败: %v", err)
		return err
	}
	if written {
		log.Printf("已写入下载任务快照: %s", path)
	}
	return nil
}

// persistDownloadTasksThrottled 是进度/状态更新的统一落盘入口（最多每秒一次）。
func (a *App) persistDownloadTasksThrottled() {
	_ = a.persistDownloadTasks(false)
}

// emitTaskProgress 广播进度并按节流策略落盘，替代裸的 runtime.EventsEmit。
func (a *App) emitTaskProgress(task *DownloadTask) {
	if a.ctx != nil && task != nil {
		runtime.EventsEmit(a.ctx, "task_progress", task)
	}
	a.persistDownloadTasksThrottled()
}

// readDownloadTaskSnapshot 读取磁盘快照；文件缺失或损坏时返回零值。
func (a *App) readDownloadTaskSnapshot() downloadTaskSnapshot {
	path := a.ensureDownloadTasksPath()
	if path == "" {
		return downloadTaskSnapshot{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return downloadTaskSnapshot{}
	}
	var snapshot downloadTaskSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		log.Printf("下载任务快照损坏，已忽略: %v", err)
		return downloadTaskSnapshot{}
	}
	if snapshot.SchemaVersion > downloadTaskSnapshotSchemaVersion {
		// 更新的版本仍然宽容读取，只记录一条日志，避免降级后直接丢数据。
		log.Printf("下载任务快照版本 %d 高于当前支持版本 %d", snapshot.SchemaVersion, downloadTaskSnapshotSchemaVersion)
	}
	return snapshot
}

// loadDownloadTasksSnapshot 返回磁盘上记录的下载任务（非终态标记为 interrupted）。
func (a *App) loadDownloadTasksSnapshot() []*DownloadTask {
	return mergeDownloadTaskSnapshot(a.readDownloadTaskSnapshot(), nil)
}

// restoreDownloadTasksSnapshotTasks 把快照任务补进内存任务表（内存优先），
// 让重试、清空、自动重下开关等既有接口对恢复出来的任务同样可用。
func (a *App) restoreDownloadTasksSnapshotTasks() {
	snapshot := a.readDownloadTaskSnapshot()
	if len(snapshot.Tasks) == 0 {
		return
	}
	restored := mergeDownloadTaskSnapshot(snapshot, nil)
	if len(restored) == 0 {
		return
	}

	taskManager.mu.Lock()
	for _, task := range restored {
		if _, exists := taskManager.tasks[task.ID]; exists {
			continue
		}
		taskManager.tasks[task.ID] = task
	}
	taskManager.mu.Unlock()
}

// findActiveDuplicateDownloadTaskLocked 在任务表里找同一份下载的活跃任务（调用方需持写锁）。
func findActiveDuplicateDownloadTaskLocked(key string) *DownloadTask {
	if key == "" {
		return nil
	}
	var found *DownloadTask
	for _, task := range taskManager.tasks {
		taskKey := downloadTaskDedupeKey(task.WorkshopID, task.FileUrl, task.Filename)
		if taskKey != key || isTerminalDownloadStatus(task.Status) {
			continue
		}
		if found == nil || task.CreatedAt < found.CreatedAt || (task.CreatedAt == found.CreatedAt && task.ID < found.ID) {
			found = task
		}
	}
	return found
}
