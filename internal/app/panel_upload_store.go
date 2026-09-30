package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// 面板地图上传的任务快照。
//
// 下载任务早就有一份 download_tasks.json（见 download_task_store.go），上传这边一直
// 只活在内存里：上传到一半关掉应用，任务记录直接消失，用户既看不到"上次有个上传被中断"，
// 也没法点重试 —— 而上传本身是支持按服务端已收分片续传的（processPanelMapUpload 里
// UploadID 非空时会先 fetchPanelMapUploadStatus 再补传）。这里补上同一套快照，
// 重启后把未完成的任务标成 interrupted，交给既有的「重试」入口续传。

const (
	panelUploadSnapshotFileName      = "upload_tasks.json"
	panelUploadSnapshotSchemaVersion = 1
	// panelUploadSnapshotLimit 限制快照里保留的任务条数，避免文件无限增长。
	panelUploadSnapshotLimit = 200
	// panelUploadInterruptedMessage 是重启后未完成任务显示的原因。
	panelUploadInterruptedMessage = "应用退出时该上传还没完成，可点击重试继续（已上传的分片会跳过）。"
)

// panelUploadStatusInterrupted 表示"上次退出时没跑完"，不是终态，但也不在跑。
const panelUploadStatusInterrupted = "interrupted"

// persistedPanelUploadTask 是落盘用的任务快照，字段比内存结构保守。
type persistedPanelUploadTask struct {
	ID               string `json:"id"`
	ServerID         string `json:"serverId"`
	ServerName       string `json:"serverName,omitempty"`
	FilePath         string `json:"filePath"`
	OriginalFilePath string `json:"originalFilePath,omitempty"`
	Filename         string `json:"filename,omitempty"`
	UploadID         string `json:"uploadId,omitempty"`
	Status           string `json:"status"`
	Progress         int    `json:"progress"`
	TotalChunks      int    `json:"totalChunks"`
	UploadedChunks   []int  `json:"uploadedChunks,omitempty"`
	TotalSize        int64  `json:"totalSize"`
	UploadedSize     int64  `json:"uploadedSize"`
	Error            string `json:"error,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
}

type panelUploadSnapshot struct {
	SchemaVersion int                        `json:"schemaVersion"`
	SavedAt       string                     `json:"savedAt,omitempty"`
	Tasks         []persistedPanelUploadTask `json:"tasks"`
}

func panelUploadTaskToPersisted(task *PanelMapUploadTask) persistedPanelUploadTask {
	return persistedPanelUploadTask{
		ID:               task.ID,
		ServerID:         task.ServerID,
		ServerName:       task.ServerName,
		FilePath:         task.FilePath,
		OriginalFilePath: task.OriginalFilePath,
		Filename:         task.Filename,
		UploadID:         task.UploadID,
		Status:           task.Status,
		Progress:         task.Progress,
		TotalChunks:      task.TotalChunks,
		UploadedChunks:   append([]int(nil), task.UploadedChunks...),
		TotalSize:        task.TotalSize,
		UploadedSize:     task.UploadedSize,
		Error:            task.Error,
		CreatedAt:        task.CreatedAt,
	}
}

// persistedToPanelUploadTask 还原任务；非终态一律标成 interrupted，
// 并把 FilePath 还原成原始源文件（VPK 任务跑到一半时 FilePath 会被换成临时 ZIP，
// 那个临时文件退出时已经删掉了，只有原始 VPK 还能重新压缩上传）。
func persistedToPanelUploadTask(saved persistedPanelUploadTask) *PanelMapUploadTask {
	task := &PanelMapUploadTask{
		ID:               saved.ID,
		ServerID:         saved.ServerID,
		ServerName:       saved.ServerName,
		FilePath:         saved.FilePath,
		OriginalFilePath: saved.OriginalFilePath,
		Filename:         saved.Filename,
		UploadID:         saved.UploadID,
		Status:           saved.Status,
		Progress:         saved.Progress,
		TotalChunks:      saved.TotalChunks,
		UploadedChunks:   normalizePanelUploadedChunks(saved.UploadedChunks, saved.TotalChunks),
		TotalSize:        saved.TotalSize,
		UploadedSize:     saved.UploadedSize,
		Error:            saved.Error,
		CreatedAt:        saved.CreatedAt,
	}
	if strings.TrimSpace(task.OriginalFilePath) != "" {
		task.FilePath = task.OriginalFilePath
		task.Filename = filepath.Base(task.OriginalFilePath)
	}
	if isActivePanelUploadStatus(task.Status) || task.Status == "pending" || task.Status == "" {
		task.Status = panelUploadStatusInterrupted
		task.Error = panelUploadInterruptedMessage
		task.Progress = panelProgress(task.UploadedSize, task.TotalSize)
	}
	return task
}

// buildPanelUploadSnapshot 组装快照；任务按创建时间倒序截断，最多保留 panelUploadSnapshotLimit 条。
func buildPanelUploadSnapshot(tasks []*PanelMapUploadTask, now time.Time) panelUploadSnapshot {
	snapshot := panelUploadSnapshot{
		SchemaVersion: panelUploadSnapshotSchemaVersion,
		SavedAt:       now.Format(time.RFC3339),
		Tasks:         make([]persistedPanelUploadTask, 0, len(tasks)),
	}
	sorted := make([]*PanelMapUploadTask, 0, len(tasks))
	for _, task := range tasks {
		if task != nil {
			sorted = append(sorted, task)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt == sorted[j].CreatedAt {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CreatedAt > sorted[j].CreatedAt
	})
	for _, task := range sorted {
		if len(snapshot.Tasks) >= panelUploadSnapshotLimit {
			break
		}
		snapshot.Tasks = append(snapshot.Tasks, panelUploadTaskToPersisted(task))
	}
	return snapshot
}

// panelUploadPersistState 记录上次写盘时间，用来把进度更新限制在最多每秒一次。
var panelUploadPersistState struct {
	mu        sync.Mutex
	lastWrite time.Time
}

func (a *App) panelUploadTasksSnapshotPath() string {
	a.ensureConfigPaths()
	if a.configDir == "" {
		return ""
	}
	return filepath.Join(a.configDir, panelUploadSnapshotFileName)
}

// persistPanelUploadTasks 按节流策略写盘；force 用于创建 / 状态变更 / 清空等关键节点。
func (a *App) persistPanelUploadTasks(force bool) {
	path := a.panelUploadTasksSnapshotPath()
	if path == "" {
		return
	}

	panelUploadPersistState.mu.Lock()
	if !force && !panelUploadPersistState.lastWrite.IsZero() &&
		time.Since(panelUploadPersistState.lastWrite) < time.Second {
		panelUploadPersistState.mu.Unlock()
		return
	}
	panelUploadPersistState.lastWrite = time.Now()
	panelUploadPersistState.mu.Unlock()

	panelUploads.mu.RLock()
	tasks := make([]*PanelMapUploadTask, 0, len(panelUploads.tasks))
	for _, task := range panelUploads.tasks {
		tasks = append(tasks, task)
	}
	panelUploads.mu.RUnlock()

	content, err := json.MarshalIndent(buildPanelUploadSnapshot(tasks, time.Now()), "", "  ")
	if err != nil {
		log.Printf("序列化上传任务快照失败: %v", err)
		return
	}
	if err := writeFileAtomically(path, content); err != nil {
		log.Printf("写入上传任务快照失败: %v", err)
	}
}

// persistPanelUploadTasksThrottled 是进度更新的统一落盘入口（最多每秒一次）。
func (a *App) persistPanelUploadTasksThrottled() {
	a.persistPanelUploadTasks(false)
}

// readPanelUploadSnapshot 读取磁盘快照；文件缺失或损坏时返回零值。
func (a *App) readPanelUploadSnapshot() panelUploadSnapshot {
	path := a.panelUploadTasksSnapshotPath()
	if path == "" {
		return panelUploadSnapshot{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return panelUploadSnapshot{}
	}
	var snapshot panelUploadSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		log.Printf("上传任务快照损坏，已忽略: %v", err)
		return panelUploadSnapshot{}
	}
	if snapshot.SchemaVersion > panelUploadSnapshotSchemaVersion {
		// 降级场景：仍然宽容读取，只记一条日志。
		log.Printf("上传任务快照版本 %d 高于当前支持版本 %d", snapshot.SchemaVersion, panelUploadSnapshotSchemaVersion)
	}
	return snapshot
}

// restorePanelUploadSnapshotTasks 把快照任务补进内存任务表（内存里已有的优先），
// 让「重试 / 取消 / 清空已完成」这些既有入口对恢复出来的任务同样可用。
func (a *App) restorePanelUploadSnapshotTasks() {
	snapshot := a.readPanelUploadSnapshot()
	if len(snapshot.Tasks) == 0 {
		return
	}

	restored := make([]*PanelMapUploadTask, 0, len(snapshot.Tasks))
	for _, saved := range snapshot.Tasks {
		task := persistedToPanelUploadTask(saved)
		if task.ID == "" {
			continue
		}
		restored = append(restored, task)
	}
	if len(restored) == 0 {
		return
	}

	panelUploads.mu.Lock()
	for _, task := range restored {
		if _, exists := panelUploads.tasks[task.ID]; exists {
			continue
		}
		panelUploads.tasks[task.ID] = task
	}
	panelUploads.mu.Unlock()
}
