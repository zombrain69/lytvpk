package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 工坊合集实体化
//
// 现有的合集解析（`getWorkshopDetailsGroup`）只在浏览器里"看一眼"合集内容。
// 这里把合集保存成一条本地实体记录：保存合集 ID 与展开结果，
// 之后可以"跟随节点"下载缺失成员、检查合集是否有新增/下架成员。
//
// 所有网络访问都经过可注入的 `workshopDetailsFetcher`，因此单元测试不需要真实工坊链接。

// WorkshopCollectionMember 是合集里的一个可下载条目。
type WorkshopCollectionMember struct {
	WorkshopID string `json:"workshopId"`
	Title      string `json:"title"`
	Filename   string `json:"filename"`
	FileUrl    string `json:"fileUrl,omitempty"`
	FileSize   string `json:"fileSize,omitempty"`
	// Present 表示本地 workshop 目录下已经有这个 VPK。
	Present bool `json:"present"`
}

// WorkshopCollectionLink 是一条本地保存的"工坊合集"记录。
type WorkshopCollectionLink struct {
	ID           string                     `json:"id"`
	CollectionID string                     `json:"collectionId"`
	Title        string                     `json:"title"`
	Members      []WorkshopCollectionMember `json:"members"`
	// ChildCollectionsTruncated 透传解析时的嵌套展开截断标记。
	ChildCollectionsTruncated bool   `json:"childCollectionsTruncated,omitempty"`
	CreatedAt                 string `json:"createdAt"`
	UpdatedAt                 string `json:"updatedAt"`
	LastCheckedAt             string `json:"lastCheckedAt,omitempty"`
}

// WorkshopCollectionRefreshResult 描述一次"跟随节点"检查的结果。
type WorkshopCollectionRefreshResult struct {
	LinkID        string   `json:"linkId"`
	CollectionID  string   `json:"collectionId"`
	Title         string   `json:"title"`
	AddedCount    int      `json:"addedCount"`
	RemovedCount  int      `json:"removedCount"`
	TotalCount    int      `json:"totalCount"`
	MissingCount  int      `json:"missingCount"`
	AddedTitles   []string `json:"addedTitles"`
	RemovedTitles []string `json:"removedTitles"`
	// Error 只用于"检查全部更新"：单条合集解析失败时不再让整轮检查失败，
	// 而是把原因带回来，界面汇总成「N 个正常 / M 个失败：<原因>」。
	Error string `json:"error,omitempty"`
}

// WorkshopCollectionDiff 是合成员集合的变化。
type WorkshopCollectionDiff struct {
	Added     []WorkshopCollectionMember
	Removed   []WorkshopCollectionMember
	Unchanged []WorkshopCollectionMember
}

type workshopCollectionStore struct {
	Links []WorkshopCollectionLink `json:"links"`
}

func (a *App) ensureCollectionsPath() string {
	a.ensureConfigPaths()
	if a.collectionsPath == "" && a.configDir != "" {
		a.collectionsPath = filepath.Join(a.configDir, "collections.json")
	}
	return a.collectionsPath
}

func (a *App) readWorkshopCollectionStore() (workshopCollectionStore, error) {
	path := a.ensureCollectionsPath()
	if path == "" {
		return workshopCollectionStore{}, nil
	}
	var store workshopCollectionStore
	if err := readJSONFile(path, &store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workshopCollectionStore{}, nil
		}
		return workshopCollectionStore{}, fmt.Errorf("无法读取工坊合集记录: %w", err)
	}
	return store, nil
}

func (a *App) writeWorkshopCollectionStore(store workshopCollectionStore) error {
	path := a.ensureCollectionsPath()
	if path == "" {
		return fmt.Errorf("未配置配置目录，无法保存工坊合集记录")
	}
	if store.Links == nil {
		store.Links = []WorkshopCollectionLink{}
	}
	a.backupLocalStoreFileIfNeeded(path)
	return writeJSONFile(a.configDir, path, store)
}

// workshopDetailsFetcherOrDefault 返回可注入的工坊详情获取器。
func (a *App) workshopDetailsFetcherOrDefault() workshopDetailFetcher {
	if a.workshopDetailsFetcher != nil {
		return a.workshopDetailsFetcher
	}
	return a.fetchWorkshopDetails
}

// resolveWorkshopCollectionMembers 展开一个合集，返回其中的可下载条目（不含合集自身）。
func resolveWorkshopCollectionMembers(fetcher workshopDetailFetcher, collectionID string) (WorkshopDetailsGroup, []WorkshopCollectionMember, error) {
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return WorkshopDetailsGroup{}, nil, fmt.Errorf("缺少工坊合集 ID")
	}
	details, err := fetcher(workshopPayload([]string{collectionID}))
	if err != nil {
		return WorkshopDetailsGroup{}, nil, err
	}
	if len(details) == 0 {
		return WorkshopDetailsGroup{}, nil, fmt.Errorf("未找到工坊合集: %s", collectionID)
	}
	main := details[0]
	// 工坊对"已删除 / 私密 / 不存在"的 ID 会回一个空壳（result != 1，没有标题也没有成员）。
	// 以前这里不校验 result，空壳会被当成"合集内容变了：成员全部下架"，
	// 真机上表现为：检查更新显示「下架 13 个」，同时把本地记录的 13 个成员直接清空。
	// （0 表示后端没给这个字段，不能据此判定失败；只有明确的非 1 才拒绝。）
	if main.Result != 0 && main.Result != 1 {
		return WorkshopDetailsGroup{}, nil, fmt.Errorf(
			"这个合集在工坊里已经不存在了（工坊返回 result=%d）：可能被作者删除或设为私密", main.Result,
		)
	}
	items, truncated, err := collectWorkshopGroupItems(collectionID, main, fetcher, workshopCollectionExpandLimit)
	if err != nil {
		return WorkshopDetailsGroup{}, nil, fmt.Errorf("展开合集失败: %w", err)
	}
	children := []WorkshopFileDetails{}
	if len(items) > 1 {
		children = items[1:]
	}
	group := buildWorkshopDetailsGroup(collectionID, main, children)
	group.ChildCollectionsTruncated = truncated

	members := make([]WorkshopCollectionMember, 0, len(group.DownloadableItems))
	for _, item := range group.DownloadableItems {
		if item.PublishedFileId == "" || item.PublishedFileId == collectionID {
			continue
		}
		members = append(members, WorkshopCollectionMember{
			WorkshopID: item.PublishedFileId,
			Title:      item.Title,
			Filename:   item.Filename,
			FileUrl:    item.FileUrl,
			FileSize:   item.FileSize,
		})
	}
	sort.SliceStable(members, func(i, j int) bool { return members[i].WorkshopID < members[j].WorkshopID })
	return group, members, nil
}

// diffWorkshopCollectionMembers 计算"已保存成员"与"最新成员"的差异。
// 纯函数：不做任何网络或磁盘访问，便于直接测试"下载/更新跟随节点"的判定。
func diffWorkshopCollectionMembers(stored []WorkshopCollectionMember, current []WorkshopCollectionMember) WorkshopCollectionDiff {
	storedByID := make(map[string]WorkshopCollectionMember, len(stored))
	for _, member := range stored {
		if member.WorkshopID == "" {
			continue
		}
		storedByID[member.WorkshopID] = member
	}
	currentByID := make(map[string]WorkshopCollectionMember, len(current))
	for _, member := range current {
		if member.WorkshopID == "" {
			continue
		}
		currentByID[member.WorkshopID] = member
	}

	diff := WorkshopCollectionDiff{}
	for _, member := range current {
		if member.WorkshopID == "" {
			continue
		}
		if _, exists := storedByID[member.WorkshopID]; exists {
			diff.Unchanged = append(diff.Unchanged, member)
			continue
		}
		diff.Added = append(diff.Added, member)
	}
	for _, member := range stored {
		if member.WorkshopID == "" {
			continue
		}
		if _, exists := currentByID[member.WorkshopID]; !exists {
			diff.Removed = append(diff.Removed, member)
		}
	}
	return diff
}

// markWorkshopCollectionMembersPresent 按本地 workshop 目录标记成员是否已下载。
func (a *App) markWorkshopCollectionMembersPresent(members []WorkshopCollectionMember) []WorkshopCollectionMember {
	result := make([]WorkshopCollectionMember, 0, len(members))
	rootDir := a.rootDirectorySnapshot()
	for _, member := range members {
		if rootDir != "" && strings.TrimSpace(member.WorkshopID) != "" {
			target := filepath.Join(rootDir, "workshop", member.WorkshopID+".vpk")
			if info, err := os.Stat(target); err == nil && !info.IsDir() {
				member.Present = true
			} else {
				member.Present = false
			}
		}
		result = append(result, member)
	}
	return result
}

// ListWorkshopCollections 返回已保存的工坊合集。
func (a *App) ListWorkshopCollections() ([]WorkshopCollectionLink, error) {
	a.collectionsMu.Lock()
	defer a.collectionsMu.Unlock()

	store, err := a.readWorkshopCollectionStore()
	if err != nil {
		return nil, err
	}
	links := make([]WorkshopCollectionLink, 0, len(store.Links))
	for _, link := range store.Links {
		link.Members = a.markWorkshopCollectionMembersPresent(link.Members)
		links = append(links, link)
	}
	return links, nil
}

// CaptureWorkshopCollection 解析并保存一个工坊合集（"实体化"）。
func (a *App) CaptureWorkshopCollection(collectionID string) (WorkshopCollectionLink, error) {
	group, members, err := resolveWorkshopCollectionMembers(a.workshopDetailsFetcherOrDefault(), collectionID)
	if err != nil {
		return WorkshopCollectionLink{}, err
	}
	// 单个 Mod 也能被解析成"只有自己"的组，但它的成员列表是空的
	// （解析时会把合集自身从成员里排除）。以前这种 ID 会被静默存成一条
	// 0 成员记录：真机上粘贴单个 Mod 链接，界面显示
	// 「已保存合集「XXX」：没有可下载成员」，用户拿到的是一条没用的记录。
	if len(members) == 0 {
		return WorkshopCollectionLink{}, fmt.Errorf(
			"这个 ID 不是工坊合集（或合集里还没有可下载的成员）：请粘贴工坊合集的链接或 ID",
		)
	}
	now := time.Now().Format(time.RFC3339)
	link := WorkshopCollectionLink{
		ID:                        newLocalRecordID(),
		CollectionID:              strings.TrimSpace(collectionID),
		Title:                     strings.TrimSpace(group.Main.Title),
		Members:                   a.markWorkshopCollectionMembersPresent(members),
		ChildCollectionsTruncated: group.ChildCollectionsTruncated,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	if link.Title == "" {
		link.Title = link.CollectionID
	}

	a.collectionsMu.Lock()
	defer a.collectionsMu.Unlock()
	store, err := a.readWorkshopCollectionStore()
	if err != nil {
		return WorkshopCollectionLink{}, err
	}
	for _, existing := range store.Links {
		if existing.CollectionID == link.CollectionID {
			return WorkshopCollectionLink{}, fmt.Errorf("该合集已经保存过: %s", link.CollectionID)
		}
	}
	store.Links = append(store.Links, link)
	if err := a.writeWorkshopCollectionStore(store); err != nil {
		return WorkshopCollectionLink{}, fmt.Errorf("无法保存工坊合集: %w", err)
	}
	return link, nil
}

// DeleteWorkshopCollection 删除一条合集记录（不会删除已下载的 Mod 文件）。
func (a *App) DeleteWorkshopCollection(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("缺少合集记录 ID")
	}
	a.collectionsMu.Lock()
	defer a.collectionsMu.Unlock()
	store, err := a.readWorkshopCollectionStore()
	if err != nil {
		return err
	}
	remaining := make([]WorkshopCollectionLink, 0, len(store.Links))
	found := false
	for _, link := range store.Links {
		if link.ID == id {
			found = true
			continue
		}
		remaining = append(remaining, link)
	}
	if !found {
		return fmt.Errorf("工坊合集记录不存在: %s", id)
	}
	store.Links = remaining
	return a.writeWorkshopCollectionStore(store)
}

// RefreshWorkshopCollection 重新解析合集并更新本地记录（下载/更新跟随节点）。
func (a *App) RefreshWorkshopCollection(id string) (WorkshopCollectionRefreshResult, error) {
	a.collectionsMu.Lock()
	store, err := a.readWorkshopCollectionStore()
	if err != nil {
		a.collectionsMu.Unlock()
		return WorkshopCollectionRefreshResult{}, err
	}
	index := -1
	for i, link := range store.Links {
		if link.ID == strings.TrimSpace(id) {
			index = i
			break
		}
	}
	if index < 0 {
		a.collectionsMu.Unlock()
		return WorkshopCollectionRefreshResult{}, fmt.Errorf("工坊合集记录不存在: %s", id)
	}
	link := store.Links[index]
	a.collectionsMu.Unlock()

	group, members, err := resolveWorkshopCollectionMembers(a.workshopDetailsFetcherOrDefault(), link.CollectionID)
	if err != nil {
		return WorkshopCollectionRefreshResult{}, err
	}
	// 双保险：即使后端漏了 result 字段（0），也不能把"解析不到任何成员"当成
	// "成员全部下架"写回去 —— 那会把本地记录清空，之后再也没法跟随节点。
	if len(members) == 0 && len(link.Members) > 0 {
		return WorkshopCollectionRefreshResult{}, fmt.Errorf(
			"这次没有解析到任何成员（合集可能已被删除或暂时不可用），已保留原来的 %d 个成员；请稍后重试",
			len(link.Members),
		)
	}
	diff := diffWorkshopCollectionMembers(link.Members, members)
	members = a.markWorkshopCollectionMembersPresent(members)

	result := WorkshopCollectionRefreshResult{
		LinkID:       link.ID,
		CollectionID: link.CollectionID,
		Title:        link.Title,
		AddedCount:   len(diff.Added),
		RemovedCount: len(diff.Removed),
		TotalCount:   len(members),
	}
	for _, member := range diff.Added {
		result.AddedTitles = append(result.AddedTitles, member.Title)
	}
	for _, member := range diff.Removed {
		result.RemovedTitles = append(result.RemovedTitles, member.Title)
	}
	for _, member := range members {
		if !member.Present {
			result.MissingCount++
		}
	}

	link.Members = members
	if title := strings.TrimSpace(group.Main.Title); title != "" {
		link.Title = title
	}
	link.ChildCollectionsTruncated = group.ChildCollectionsTruncated
	link.UpdatedAt = time.Now().Format(time.RFC3339)
	link.LastCheckedAt = link.UpdatedAt

	a.collectionsMu.Lock()
	defer a.collectionsMu.Unlock()
	latest, err := a.readWorkshopCollectionStore()
	if err != nil {
		return WorkshopCollectionRefreshResult{}, err
	}
	for i := range latest.Links {
		if latest.Links[i].ID == link.ID {
			latest.Links[i] = link
		}
	}
	if err := a.writeWorkshopCollectionStore(latest); err != nil {
		return WorkshopCollectionRefreshResult{}, err
	}
	return result, nil
}

// CheckWorkshopCollectionUpdates 检查全部已保存合集的成员变化（"更新跟随节点"）。
func (a *App) CheckWorkshopCollectionUpdates() ([]WorkshopCollectionRefreshResult, error) {
	a.collectionsMu.Lock()
	store, err := a.readWorkshopCollectionStore()
	type pendingCollection struct {
		id           string
		collectionID string
		title        string
	}
	pending := make([]pendingCollection, 0, len(store.Links))
	if err == nil {
		for _, link := range store.Links {
			pending = append(pending, pendingCollection{id: link.ID, collectionID: link.CollectionID, title: link.Title})
		}
	}
	a.collectionsMu.Unlock()
	if err != nil {
		return nil, err
	}

	results := make([]WorkshopCollectionRefreshResult, 0, len(pending))
	for _, item := range pending {
		result, refreshErr := a.RefreshWorkshopCollection(item.id)
		if refreshErr != nil {
			// 单条失败不再中断整轮检查：以前一条坏记录会让「检查全部更新」整个报错，
			// 其余合集的结果也一起丢掉。现在把原因装进结果里，交给界面汇总。
			results = append(results, WorkshopCollectionRefreshResult{
				LinkID:       item.id,
				CollectionID: item.collectionID,
				Title:        item.title,
				Error:        refreshErr.Error(),
			})
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// DownloadWorkshopCollection 下载合集中尚未下载的成员（"下载跟随节点"）。
// 返回本次入队的工坊 ID 列表；已经在本地的成员会被跳过。
func (a *App) DownloadWorkshopCollection(id string) ([]string, error) {
	a.collectionsMu.Lock()
	store, err := a.readWorkshopCollectionStore()
	if err != nil {
		a.collectionsMu.Unlock()
		return nil, err
	}
	var target *WorkshopCollectionLink
	for index := range store.Links {
		if store.Links[index].ID == strings.TrimSpace(id) {
			target = &store.Links[index]
			break
		}
	}
	a.collectionsMu.Unlock()
	if target == nil {
		return nil, fmt.Errorf("工坊合集记录不存在: %s", id)
	}
	collectionID := target.CollectionID

	group, members, err := resolveWorkshopCollectionMembers(a.workshopDetailsFetcherOrDefault(), collectionID)
	if err != nil {
		return nil, err
	}
	members = a.markWorkshopCollectionMembersPresent(members)
	details := make(map[string]WorkshopFileDetails, len(group.DownloadableItems))
	for _, item := range group.DownloadableItems {
		details[item.PublishedFileId] = item
	}

	queued := make([]string, 0, len(members))
	useOptimizedIP := a.GetWorkshopPreferredIP()
	for _, member := range members {
		if member.Present {
			continue
		}
		item, ok := details[member.WorkshopID]
		if !ok || strings.TrimSpace(item.FileUrl) == "" {
			continue
		}
		a.StartDownloadTask(item, useOptimizedIP)
		queued = append(queued, member.WorkshopID)
	}
	return queued, nil
}
