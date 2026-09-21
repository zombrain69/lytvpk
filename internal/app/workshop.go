package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"vpk-manager/internal/platform/protocol"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type WorkshopChild struct {
	PublishedFileId string `json:"publishedfileid"`
	SortOrder       int    `json:"sortorder"`
	FileType        int    `json:"file_type"`
}

type WorkshopFileDetails struct {
	Result          int    `json:"result"`
	PublishedFileId string `json:"publishedfileid"`
	Creator         string `json:"creator"`
	Filename        string `json:"filename"`
	FileSize        string `json:"file_size"`
	FileUrl         string `json:"file_url"`
	PreviewUrl      string `json:"preview_url"`
	Previews        []struct {
		PreviewUrl  string `json:"preview_url"`
		PreviewType int    `json:"preview_type"`
	} `json:"previews"`
	Title       string          `json:"title"`
	Description string          `json:"file_description"`
	Children    []WorkshopChild `json:"children"`
}

type WorkshopDetailsGroup struct {
	RootID            string                `json:"root_id"`
	Main              WorkshopFileDetails   `json:"main"`
	Items             []WorkshopFileDetails `json:"items"`
	DownloadableItems []WorkshopFileDetails `json:"downloadable_items"`
	// ChildCollectionsTruncated 表示子合集嵌套过深，已达到展开上限。
	ChildCollectionsTruncated bool `json:"child_collections_truncated"`
}

type WorkshopDetailsResult struct {
	Groups []WorkshopDetailsGroup `json:"groups"`
}

const (
	// workshopCollectionExpandLimit 限制单次解析最多展开多少个子合集，
	// 避免异常数据导致大量接口请求。
	workshopCollectionExpandLimit = 20
	// workshopCollectionFetchChunk 控制单次批量请求的 ID 数量。
	workshopCollectionFetchChunk = 50
)

// workshopDetailFetcher 批量获取工坊详情（payload 形如 [1,2,3]）。
type workshopDetailFetcher func(payload string) ([]WorkshopFileDetails, error)

type DownloadTask struct {
	ID             string `json:"id"`
	WorkshopID     string `json:"workshop_id"`
	Title          string `json:"title"`
	Filename       string `json:"filename"`
	FilePath       string `json:"file_path"`
	PreviewUrl     string `json:"preview_url"`
	FileUrl        string `json:"file_url"` // Added for retry
	UseOptimizedIP bool   `json:"use_optimized_ip"`
	Status         string `json:"status"` // "pending", "downloading", "completed", "failed", "cancelled"
	Progress       int    `json:"progress"`
	TotalSize      int64  `json:"total_size"`
	DownloadedSize int64  `json:"downloaded_size"`
	Speed          string `json:"speed"`
	Error          string `json:"error"`
	Description    string `json:"description"`
	CreatedAt      string `json:"created_at"`
	// AutoRedownload 为真时，本任务在失败后自动重试一次；默认关闭。
	// 下载失败仍然会把错误写进任务里，用户随时能看到发生了什么。
	AutoRedownload bool `json:"auto_redownload"`
	// RedownloadAttempts 记录已经自动重试的次数（上限 1，见 shouldAutoRedownload）。
	RedownloadAttempts int                `json:"redownload_attempts"`
	cancelFunc         context.CancelFunc `json:"-"`
}

// shouldAutoRedownload 判定一个失败任务是否应该触发"自动重下一次"。
// 纯函数，便于在不动网络的情况下覆盖全部边界。
func shouldAutoRedownload(task *DownloadTask) bool {
	if task == nil {
		return false
	}
	if !task.AutoRedownload {
		return false
	}
	if task.RedownloadAttempts >= 1 {
		return false
	}
	if task.Status != "failed" {
		return false
	}
	if strings.HasPrefix(task.Error, "Cancelled") {
		// 用户主动取消不应被自动重下覆盖。
		return false
	}
	return true
}

// downloadTaskStarter 是"启动一次下载"的可注入入口，测试用它替换真实网络下载。
// 默认 nil 表示走真实下载；显式赋值（测试）时优先使用注入实现。
var downloadTaskStarter func(a *App, ctx context.Context, task *DownloadTask, url string)

// startDownloadTask 统一入口：默认在协程里跑真实下载，测试可注入替身。
func startDownloadTask(a *App, ctx context.Context, task *DownloadTask, url string) {
	if downloadTaskStarter != nil {
		downloadTaskStarter(a, ctx, task, url)
		return
	}
	go a.processDownloadTask(ctx, task, url)
}

// emitTaskUpdated 广播任务状态；Wails 未就绪（测试或无界面调用）时静默跳过。
func (a *App) emitTaskUpdated(task *DownloadTask) {
	if a.ctx == nil || task == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "task_updated", task)
}

// maybeAutoRedownload 在任务失败后按策略自动重下一次。
// 返回 true 表示已经安排了重试。计数与状态判断在同一把锁内完成，避免并发双开。
func (a *App) maybeAutoRedownload(taskID string) bool {
	taskManager.mu.Lock()
	task, exists := taskManager.tasks[taskID]
	if !exists || !shouldAutoRedownload(task) {
		taskManager.mu.Unlock()
		return false
	}
	task.RedownloadAttempts++
	task.Status = "pending"
	task.Progress = 0
	task.DownloadedSize = 0
	task.Error = ""
	task.Speed = ""
	task.FilePath = ""
	ctx, cancel := context.WithCancel(context.Background())
	task.cancelFunc = cancel
	taskManager.mu.Unlock()

	a.emitTaskUpdated(task)
	log.Printf("下载任务 %s 失败，已自动重下一次", taskID)
	startDownloadTask(a, ctx, task, task.FileUrl)
	return true
}

// SetDownloadTaskAutoRedownload 打开/关闭某个任务的自动重下。
func (a *App) SetDownloadTaskAutoRedownload(taskID string, enabled bool) error {
	taskManager.mu.Lock()
	task, exists := taskManager.tasks[taskID]
	if !exists {
		taskManager.mu.Unlock()
		return fmt.Errorf("下载任务不存在: %s", taskID)
	}
	task.AutoRedownload = enabled
	taskManager.mu.Unlock()
	a.emitTaskUpdated(task)
	return nil
}

// GetWorkshopAutoRedownload 返回"新建下载任务是否默认自动重下一次"的全局设置。
func (a *App) GetWorkshopAutoRedownload() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workshopAutoRedownload
}

// SetWorkshopAutoRedownload 保存全局默认值。默认关闭：只有用户显式打开才会自动重下。
func (a *App) SetWorkshopAutoRedownload(enabled bool) error {
	a.mu.Lock()
	a.workshopAutoRedownload = enabled
	a.mu.Unlock()
	a.saveConfig()
	return nil
}

func (a *App) workshopAutoRedownloadSnapshot() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workshopAutoRedownload
}

// TaskManager manages download tasks
type TaskManager struct {
	tasks map[string]*DownloadTask
	mu    sync.RWMutex
}

var taskManager = &TaskManager{
	tasks: make(map[string]*DownloadTask),
}

// HasActiveDownloads checks if there are any active downloads
func (a *App) HasActiveDownloads() bool {
	taskManager.mu.RLock()
	defer taskManager.mu.RUnlock()

	for _, task := range taskManager.tasks {
		if task.Status == "downloading" || task.Status == "pending" {
			return true
		}
	}
	return false
}

// CancelDownloadTask cancels a download task
func (a *App) CancelDownloadTask(taskID string) {
	taskManager.mu.Lock()
	task, exists := taskManager.tasks[taskID]
	if exists && task.cancelFunc != nil && (task.Status == "pending" || task.Status == "downloading") {
		task.cancelFunc()
		task.Status = "cancelled"
		task.Error = "Cancelled by user"
	}
	taskManager.mu.Unlock()

	if exists {
		runtime.EventsEmit(a.ctx, "task_updated", task)
	}
}

// RetryDownloadTask retries a failed or cancelled task
func (a *App) RetryDownloadTask(taskID string) {
	taskManager.mu.Lock()
	task, exists := taskManager.tasks[taskID]
	if !exists {
		taskManager.mu.Unlock()
		return
	}

	// 状态检查与重置必须在同一把锁内完成。否则快速双击“重试”时，
	// 两个调用都可能先读到 failed/cancelled，进而为同一个任务启动两条下载协程。
	if task.Status != "failed" && task.Status != "cancelled" {
		taskManager.mu.Unlock()
		return
	}

	// Reset task state while holding the lock acquired above.
	task.Status = "pending"
	task.Progress = 0
	task.DownloadedSize = 0
	task.Error = ""
	task.Speed = ""
	task.FilePath = ""

	// Create new context
	ctx, cancel := context.WithCancel(context.Background())
	task.cancelFunc = cancel
	taskManager.mu.Unlock()

	runtime.EventsEmit(a.ctx, "task_updated", task)

	downloadTaskStarter(a, ctx, task, task.FileUrl)
}

func parseFileSize(sizeStr string) int64 {
	// Try parsing as simple integer
	if s, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
		return s
	}

	// Try parsing "123 MB" etc. (Simple implementation)
	// This is just a fallback, usually API returns bytes
	return 0
}

// ParseWorkshopID extracts the ID from a Steam Workshop URL or a direct ID.
func (a *App) ParseWorkshopID(workshopUrl string) (string, error) {
	workshopUrl = strings.TrimSpace(workshopUrl)
	if protocol.IsValidWorkshopID(workshopUrl) {
		return workshopUrl, nil
	}

	u, err := url.Parse(workshopUrl)
	if err != nil {
		return "", fmt.Errorf("invalid URL")
	}

	// 只有 Steam 相关域名才提取 id 参数
	host := strings.ToLower(u.Hostname())
	isSteamHost := strings.HasSuffix(host, "steamcommunity.com") ||
		strings.HasSuffix(host, "steampowered.com") ||
		strings.HasSuffix(host, "steamworkshop.download")

	if isSteamHost {
		q := u.Query()
		id := q.Get("id")
		if id != "" && protocol.IsValidWorkshopID(id) {
			return id, nil
		}
	}

	// 回退：仅当 URL 整体看起来像 Steam 链接时才用正则匹配
	if isSteamHost || strings.Contains(workshopUrl, "steamcommunity.com/sharedfiles") {
		re := regexp.MustCompile(`\d+`)
		matches := re.FindStringSubmatch(workshopUrl)
		if len(matches) > 0 && protocol.IsValidWorkshopID(matches[0]) {
			return matches[0], nil
		}
	}

	return "", fmt.Errorf("could not find valid workshop ID in URL")
}

func (a *App) parseWorkshopIDs(workshopInput string) ([]string, error) {
	workshopInput = strings.TrimSpace(workshopInput)
	if workshopInput == "" {
		return nil, fmt.Errorf("工坊ID不能为空")
	}

	if decoded, err := url.PathUnescape(workshopInput); err == nil {
		workshopInput = strings.TrimSpace(decoded)
	}

	parts := strings.Split(workshopInput, protocol.WorkshopIDDelimiter)
	ids := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("工坊ID列表包含空项")
		}

		id, err := a.ParseWorkshopID(part)
		if err != nil {
			return nil, fmt.Errorf("无效的工坊ID片段 %q: %w", part, err)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, fmt.Errorf("工坊ID不能为空")
	}

	return ids, nil
}

// GetWorkshopDetails fetches details from steamworkshopdownloader.io
func (a *App) GetWorkshopDetails(workshopUrl string) ([]WorkshopFileDetails, error) {
	id, err := a.ParseWorkshopID(workshopUrl)
	if err != nil {
		return nil, err
	}

	payload := fmt.Sprintf(`[%s]`, id)
	details, err := a.fetchWorkshopDetails(payload)
	if err != nil {
		return nil, err
	}

	if len(details) == 0 {
		return nil, fmt.Errorf("no details found")
	}

	// Check if it's a collection (has children)
	if len(details[0].Children) > 0 {
		var childIDs []string
		for _, child := range details[0].Children {
			childIDs = append(childIDs, child.PublishedFileId)
		}

		// Fetch details for all children
		childPayload := "[" + strings.Join(childIDs, ",") + "]"
		childrenDetails, err := a.fetchWorkshopDetails(childPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch children details: %v", err)
		}

		// If the parent item is also a valid file, include it
		if details[0].Result == 1 {
			childrenDetails = append([]WorkshopFileDetails{details[0]}, childrenDetails...)
		}

		return a.processDetails(childrenDetails)
	}

	return a.processDetails(details)
}

func (a *App) GetWorkshopDetailsGrouped(workshopInput string) (WorkshopDetailsResult, error) {
	rootIDs, err := a.parseWorkshopIDs(workshopInput)
	if err != nil {
		return WorkshopDetailsResult{}, err
	}

	groups := make([]WorkshopDetailsGroup, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		group, err := a.getWorkshopDetailsGroup(rootID)
		if err != nil {
			return WorkshopDetailsResult{}, fmt.Errorf("解析工坊ID %s 失败: %w", rootID, err)
		}
		groups = append(groups, group)
	}

	return WorkshopDetailsResult{Groups: groups}, nil
}

func (a *App) getWorkshopDetailsGroup(rootID string) (WorkshopDetailsGroup, error) {
	details, err := a.fetchWorkshopDetails(workshopPayload([]string{rootID}))
	if err != nil {
		return WorkshopDetailsGroup{}, err
	}

	if len(details) == 0 {
		return WorkshopDetailsGroup{}, fmt.Errorf("no details found")
	}

	main := details[0]
	items, truncated, err := collectWorkshopGroupItems(rootID, main, a.fetchWorkshopDetails, workshopCollectionExpandLimit)
	if err != nil {
		return WorkshopDetailsGroup{}, fmt.Errorf("failed to fetch children details: %v", err)
	}

	childrenDetails := []WorkshopFileDetails{}
	if len(items) > 1 {
		childrenDetails = items[1:]
	}
	group := buildWorkshopDetailsGroup(rootID, main, childrenDetails)
	group.ChildCollectionsTruncated = truncated
	return group, nil
}

// collectWorkshopGroupItems 从根条目出发递归展开子合集，返回去重后的全部条目
// （含根条目与子合集本身）。seen 集合同时承担循环引用保护。
func collectWorkshopGroupItems(rootID string, root WorkshopFileDetails, fetch workshopDetailFetcher, maxCollections int) ([]WorkshopFileDetails, bool, error) {
	items := []WorkshopFileDetails{root}
	seen := map[string]bool{}
	if id := strings.TrimSpace(root.PublishedFileId); id != "" {
		seen[id] = true
	} else if id := strings.TrimSpace(rootID); id != "" {
		seen[id] = true
	}

	queue := make([]string, 0, len(root.Children))
	for _, child := range root.Children {
		queue = appendUniqueWorkshopID(queue, seen, child.PublishedFileId)
	}

	expanded := 0
	truncated := false
	for len(queue) > 0 {
		next := make([]string, 0)
		for start := 0; start < len(queue); start += workshopCollectionFetchChunk {
			end := min(start+workshopCollectionFetchChunk, len(queue))
			details, err := fetch(workshopPayload(queue[start:end]))
			if err != nil {
				return items, truncated, err
			}
			for _, detail := range details {
				id := strings.TrimSpace(detail.PublishedFileId)
				if id == "" || seen[id] {
					continue
				}
				seen[id] = true
				items = append(items, detail)

				if len(detail.Children) == 0 {
					continue
				}
				if expanded >= maxCollections {
					// 达到展开上限：保留条目本身，但不再继续向下展开。
					truncated = true
					continue
				}
				expanded++
				for _, child := range detail.Children {
					next = appendUniqueWorkshopID(next, seen, child.PublishedFileId)
				}
			}
		}
		queue = next
	}
	return items, truncated, nil
}

// appendUniqueWorkshopID 把尚未见过的 ID 追加到待处理列表。
func appendUniqueWorkshopID(target []string, seen map[string]bool, id string) []string {
	id = strings.TrimSpace(id)
	if id == "" || seen[id] {
		return target
	}
	for _, existing := range target {
		if existing == id {
			return target
		}
	}
	return append(target, id)
}

func workshopPayload(ids []string) string {
	return "[" + strings.Join(ids, ",") + "]"
}

func buildWorkshopDetailsGroup(rootID string, main WorkshopFileDetails, children []WorkshopFileDetails) WorkshopDetailsGroup {
	main = prepareWorkshopDetail(main)
	if main.PublishedFileId == "" {
		main.PublishedFileId = rootID
	}

	items := []WorkshopFileDetails{main}
	seen := map[string]bool{
		main.PublishedFileId: true,
	}

	for _, child := range children {
		child = prepareWorkshopDetail(child)
		if child.PublishedFileId == "" || seen[child.PublishedFileId] {
			continue
		}
		seen[child.PublishedFileId] = true
		items = append(items, child)
	}

	downloadableItems := make([]WorkshopFileDetails, 0, len(items))
	for _, item := range items {
		if isDownloadableWorkshopDetail(item) {
			downloadableItems = append(downloadableItems, item)
		}
	}

	return WorkshopDetailsGroup{
		RootID:            rootID,
		Main:              main,
		Items:             items,
		DownloadableItems: downloadableItems,
	}
}

func (a *App) fetchWorkshopDetails(payload string) ([]WorkshopFileDetails, error) {
	apiUrl := "https://l4d2-workshop-parse.laoyutang.cn"

	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	var details []WorkshopFileDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, err
	}

	return details, nil
}

func (a *App) processDetails(details []WorkshopFileDetails) ([]WorkshopFileDetails, error) {
	var validDetails []WorkshopFileDetails
	for i := range details {
		if details[i].Result == 1 {
			validDetails = append(validDetails, prepareWorkshopDetail(details[i]))
		}
	}

	if len(validDetails) == 0 {
		return nil, fmt.Errorf("no valid details found")
	}

	return validDetails, nil
}

func prepareWorkshopDetail(detail WorkshopFileDetails) WorkshopFileDetails {
	detail.Creator = ""
	detail.Filename = cleanFilename(detail.Filename)
	return detail
}

func isDownloadableWorkshopDetail(detail WorkshopFileDetails) bool {
	return detail.Result == 1 && strings.TrimSpace(detail.FileUrl) != ""
}

func cleanFilename(filename string) string {
	// First, get the base name to handle paths like "myl4d2addons/file.vpk"
	filename = strings.ReplaceAll(filename, "\\", "/")
	if idx := strings.LastIndex(filename, "/"); idx != -1 {
		filename = filename[idx+1:]
	}

	lowerName := strings.ToLower(filename)
	prefixes := []string{"my l4d2addons", "myl4d2addons"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(lowerName, prefix) {
			filename = filename[len(prefix):]
			// Trim spaces, underscores and dashes from the beginning
			filename = strings.TrimLeft(filename, " _-")
			lowerName = strings.ToLower(filename)
		}
	}
	return filename
}
