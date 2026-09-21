package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func newCollectionTestApp(t *testing.T, catalog map[string]WorkshopFileDetails) (*App, *[][]string) {
	t.Helper()
	a, addonsDir := newPriorityTestApp(t)
	fetcher, requests := newFakeWorkshopFetcher(t, catalog)
	a.workshopDetailsFetcher = fetcher
	_ = addonsDir
	return a, requests
}

func TestDiffWorkshopCollectionMembers(t *testing.T) {
	stored := []WorkshopCollectionMember{
		{WorkshopID: "1", Title: "A"},
		{WorkshopID: "2", Title: "B"},
	}
	current := []WorkshopCollectionMember{
		{WorkshopID: "2", Title: "B"},
		{WorkshopID: "3", Title: "C"},
	}
	diff := diffWorkshopCollectionMembers(stored, current)
	if len(diff.Added) != 1 || diff.Added[0].WorkshopID != "3" {
		t.Fatalf("新增成员 = %#v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].WorkshopID != "1" {
		t.Fatalf("移除成员 = %#v", diff.Removed)
	}
	if len(diff.Unchanged) != 1 || diff.Unchanged[0].WorkshopID != "2" {
		t.Fatalf("未变化成员 = %#v", diff.Unchanged)
	}
}

// TestCaptureWorkshopCollectionPersistsMembers 覆盖"合集实体化"。
func TestCaptureWorkshopCollectionPersistsMembers(t *testing.T) {
	a, _ := newCollectionTestApp(t, map[string]WorkshopFileDetails{
		"100": workshopDetailWithChildren("100", "1", "2", "200"),
		"200": workshopDetailWithChildren("200", "3"),
		"1":   {Result: 1, PublishedFileId: "1", Title: "物品一", Filename: "1.vpk", FileUrl: "https://example.invalid/1.vpk"},
		"2":   {Result: 1, PublishedFileId: "2", Title: "物品二", Filename: "2.vpk", FileUrl: "https://example.invalid/2.vpk"},
		"3":   {Result: 1, PublishedFileId: "3", Title: "物品三", Filename: "3.vpk", FileUrl: "https://example.invalid/3.vpk"},
	})

	link, err := a.CaptureWorkshopCollection("100")
	if err != nil {
		t.Fatalf("capture collection: %v", err)
	}
	if link.CollectionID != "100" || link.ID == "" {
		t.Fatalf("合集记录 = %#v", link)
	}
	gotIDs := make([]string, 0, len(link.Members))
	for _, member := range link.Members {
		gotIDs = append(gotIDs, member.WorkshopID)
		if member.Present {
			t.Fatalf("本地没有下载过，Present 应为 false: %#v", member)
		}
	}
	if want := []string{"1", "2", "3"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("合成员 = %#v, want %#v", gotIDs, want)
	}

	// 重复保存同一合集应报错。
	if _, err := a.CaptureWorkshopCollection("100"); err == nil {
		t.Fatal("重复保存同一合集应报错")
	}

	// 本地已有成员会被标记为 Present。
	addonsDir := a.rootDirectorySnapshot()
	writeTestVPK(t, filepath.Join(addonsDir, "workshop", "2.vpk"), map[string][]byte{"materials/x.vtf": []byte("x")})
	links, err := a.ListWorkshopCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("合集列表 = %#v", links)
	}
	present := map[string]bool{}
	for _, member := range links[0].Members {
		present[member.WorkshopID] = member.Present
	}
	if !present["2"] || present["1"] || present["3"] {
		t.Fatalf("本地存在的成员应标记 Present: %#v", present)
	}
}

// TestRefreshWorkshopCollectionFollowsNodes 覆盖"更新跟随节点"。
func TestRefreshWorkshopCollectionFollowsNodes(t *testing.T) {
	catalog := map[string]WorkshopFileDetails{
		"100": workshopDetailWithChildren("100", "1", "2"),
		"1":   {Result: 1, PublishedFileId: "1", Title: "物品一", Filename: "1.vpk", FileUrl: "https://example.invalid/1.vpk"},
		"2":   {Result: 1, PublishedFileId: "2", Title: "物品二", Filename: "2.vpk", FileUrl: "https://example.invalid/2.vpk"},
	}
	a, _ := newCollectionTestApp(t, catalog)
	link, err := a.CaptureWorkshopCollection("100")
	if err != nil {
		t.Fatal(err)
	}

	// 合集作者新增了一个成员、下架了一个成员。
	catalog["100"] = workshopDetailWithChildren("100", "2", "3")
	catalog["3"] = WorkshopFileDetails{Result: 1, PublishedFileId: "3", Title: "物品三", Filename: "3.vpk", FileUrl: "https://example.invalid/3.vpk"}

	result, err := a.RefreshWorkshopCollection(link.ID)
	if err != nil {
		t.Fatalf("refresh collection: %v", err)
	}
	if result.AddedCount != 1 || result.RemovedCount != 1 || result.TotalCount != 2 {
		t.Fatalf("刷新结果 = %#v", result)
	}
	if !reflect.DeepEqual(result.AddedTitles, []string{"物品三"}) {
		t.Fatalf("新增标题 = %#v", result.AddedTitles)
	}
	if !reflect.DeepEqual(result.RemovedTitles, []string{"物品一"}) {
		t.Fatalf("移除标题 = %#v", result.RemovedTitles)
	}
	if result.MissingCount != 2 {
		t.Fatalf("本地都还没下载，MissingCount 应为 2: %#v", result)
	}

	// 刷新后本地记录应同步。
	links, err := a.ListWorkshopCollections()
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(links[0].Members))
	for _, member := range links[0].Members {
		ids = append(ids, member.WorkshopID)
	}
	if want := []string{"2", "3"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("刷新后的成员 = %#v, want %#v", ids, want)
	}
	if links[0].LastCheckedAt == "" {
		t.Fatal("刷新后应记录检查时间")
	}
}

// TestCheckWorkshopCollectionUpdatesCoversAllLinks 覆盖批量检查入口。
func TestCheckWorkshopCollectionUpdatesCoversAllLinks(t *testing.T) {
	catalog := map[string]WorkshopFileDetails{
		"100": workshopDetailWithChildren("100", "1"),
		"101": workshopDetailWithChildren("101", "2"),
		"1":   {Result: 1, PublishedFileId: "1", Title: "物品一", Filename: "1.vpk", FileUrl: "https://example.invalid/1.vpk"},
		"2":   {Result: 1, PublishedFileId: "2", Title: "物品二", Filename: "2.vpk", FileUrl: "https://example.invalid/2.vpk"},
	}
	a, _ := newCollectionTestApp(t, catalog)
	if _, err := a.CaptureWorkshopCollection("100"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CaptureWorkshopCollection("101"); err != nil {
		t.Fatal(err)
	}

	results, err := a.CheckWorkshopCollectionUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("应检查两条合集记录: %#v", results)
	}
	for _, result := range results {
		if result.AddedCount != 0 || result.RemovedCount != 0 {
			t.Fatalf("无变化时不应报告增删: %#v", result)
		}
	}
}

// TestDownloadWorkshopCollectionSkipsPresentMembers 覆盖"下载跟随节点"。
func TestDownloadWorkshopCollectionSkipsPresentMembers(t *testing.T) {
	a, _ := newCollectionTestApp(t, map[string]WorkshopFileDetails{
		"100": workshopDetailWithChildren("100", "1", "2"),
		"1":   {Result: 1, PublishedFileId: "1", Title: "物品一", Filename: "1.vpk", FileUrl: "https://example.invalid/1.vpk"},
		"2":   {Result: 1, PublishedFileId: "2", Title: "物品二", Filename: "2.vpk", FileUrl: "https://example.invalid/2.vpk"},
	})
	// 本地已经有成员 2。
	writeTestVPK(t, filepath.Join(a.rootDirectorySnapshot(), "workshop", "2.vpk"), map[string][]byte{"materials/y.vtf": []byte("y")})
	link, err := a.CaptureWorkshopCollection("100")
	if err != nil {
		t.Fatal(err)
	}

	// 下载启动器替换为纯记录替身，避免真实网络。
	started := make([]string, 0)
	withDownloadTaskStarter(t, func(_ *App, _ context.Context, task *DownloadTask, _ string) {
		started = append(started, task.WorkshopID)
	})

	queued, err := a.DownloadWorkshopCollection(link.ID)
	if err != nil {
		t.Fatalf("download collection: %v", err)
	}
	if !reflect.DeepEqual(queued, []string{"1"}) {
		t.Fatalf("应只下载缺失成员 1，实际 %#v", queued)
	}
	if !reflect.DeepEqual(started, []string{"1"}) {
		t.Fatalf("启动的下载 = %#v", started)
	}
}

// TestDeleteWorkshopCollectionKeepsFiles 删除记录不应删除已下载文件。
func TestDeleteWorkshopCollectionKeepsFiles(t *testing.T) {
	a, _ := newCollectionTestApp(t, map[string]WorkshopFileDetails{
		"100": workshopDetailWithChildren("100", "1"),
		"1":   {Result: 1, PublishedFileId: "1", Title: "物品一", Filename: "1.vpk", FileUrl: "https://example.invalid/1.vpk"},
	})
	path := filepath.Join(a.rootDirectorySnapshot(), "workshop", "1.vpk")
	writeTestVPK(t, path, map[string][]byte{"materials/z.vtf": []byte("z")})

	link, err := a.CaptureWorkshopCollection("100")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteWorkshopCollection(link.ID); err != nil {
		t.Fatal(err)
	}
	links, err := a.ListWorkshopCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("删除后不应有记录: %#v", links)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("删除合集记录不应删除已下载的 VPK: %v", err)
	}
}
