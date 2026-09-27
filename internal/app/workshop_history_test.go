package app

import (
	"encoding/json"
	"os"
	"testing"
)

// 这组用例对应上游 `feat 添加解析历史功能`（b635ea3）的语义，
// 并按本项目约定补了 schemaVersion 断言。测试隔离：每个用例用自己的 configDir。

func newHistoryTestApp(t *testing.T) *App {
	t.Helper()
	return &App{configDir: t.TempDir()}
}

func historyRootIDs(items []WorkshopHistoryItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.RootID)
	}
	return ids
}

func TestWorkshopHistoryKeepsNewestFirstAndDeduplicates(t *testing.T) {
	a := newHistoryTestApp(t)

	if _, err := a.AddWorkshopHistoryEntries([]WorkshopHistoryItem{
		{RootID: "111", Title: "第一个"},
		{RootID: "222", Title: "第二个"},
	}); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}

	storage, err := a.AddWorkshopHistoryEntries([]WorkshopHistoryItem{{RootID: "111", Title: "第一个（更新）"}})
	if err != nil {
		t.Fatalf("再次写入失败: %v", err)
	}

	want := []string{"111", "222"}
	got := historyRootIDs(storage.Items)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("去重置顶结果错误: got %v, want %v", got, want)
	}
	if storage.Items[0].Title != "第一个（更新）" {
		t.Fatalf("重复项未使用最新标题: %q", storage.Items[0].Title)
	}
	if storage.Items[0].ParsedAt == 0 {
		t.Fatal("写入时未补全 ParsedAt")
	}
	if storage.SchemaVersion != workshopHistorySchemaVersion {
		t.Fatalf("存档缺少 schemaVersion: %d", storage.SchemaVersion)
	}

	reloaded := a.GetWorkshopHistory()
	if got := historyRootIDs(reloaded.Items); len(got) != 2 || got[0] != "111" {
		t.Fatalf("持久化后读取结果错误: %v", got)
	}
}

func TestWorkshopHistoryTruncatesToLimit(t *testing.T) {
	a := newHistoryTestApp(t)

	items := make([]WorkshopHistoryItem, 0, workshopHistoryLimit+2)
	for i := 0; i < workshopHistoryLimit+2; i++ {
		items = append(items, WorkshopHistoryItem{RootID: string(rune('a' + i)), Title: "条目"})
	}

	storage, err := a.AddWorkshopHistoryEntries(items)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	if len(storage.Items) != workshopHistoryLimit {
		t.Fatalf("条数未截断: got %d, want %d", len(storage.Items), workshopHistoryLimit)
	}
	if storage.Items[0].RootID != "a" {
		t.Fatalf("最新条目未置顶: %q", storage.Items[0].RootID)
	}
	if last := storage.Items[workshopHistoryLimit-1].RootID; last != string(rune('a'+workshopHistoryLimit-1)) {
		t.Fatalf("最旧条目应被淘汰: %q", last)
	}
}

func TestWorkshopHistoryStoresGroupSnapshot(t *testing.T) {
	a := newHistoryTestApp(t)

	group := WorkshopDetailsGroup{
		RootID: "999",
		Main: WorkshopFileDetails{
			PublishedFileId: "999",
			Title:           "合集示例",
			FileUrl:         "https://example.com/a.vpk",
			Children: []WorkshopChild{
				{PublishedFileId: "888", FileType: 0},
			},
		},
		Items: []WorkshopFileDetails{
			{PublishedFileId: "888", Title: "子物品", FileUrl: "https://example.com/b.vpk"},
		},
	}

	if _, err := a.AddWorkshopHistoryEntries([]WorkshopHistoryItem{{
		RootID:   group.RootID,
		Title:    group.Main.Title,
		FileType: 2, // 2 = 合集（与工坊接口一致）
		Group:    group,
	}}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	reloaded := a.GetWorkshopHistory()
	if len(reloaded.Items) != 1 {
		t.Fatalf("读取条数错误: %d", len(reloaded.Items))
	}
	got := reloaded.Items[0]
	if got.Group.Main.Title != "合集示例" || got.Group.Main.FileUrl != "https://example.com/a.vpk" {
		t.Fatalf("快照主物品丢失: %+v", got.Group.Main)
	}
	if len(got.Group.Items) != 1 || got.Group.Items[0].PublishedFileId != "888" {
		t.Fatalf("快照子物品丢失: %+v", got.Group.Items)
	}
	if got.FileType != 2 {
		t.Fatalf("fileType 未保留: %d", got.FileType)
	}

	raw, err := os.ReadFile(a.workshopHistoryPath)
	if err != nil {
		t.Fatalf("读取历史文件失败: %v", err)
	}
	var onDisk WorkshopHistoryStorage
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("历史文件不是合法 JSON: %v", err)
	}
	if onDisk.SchemaVersion != workshopHistorySchemaVersion {
		t.Fatalf("历史文件缺少 schemaVersion: %d", onDisk.SchemaVersion)
	}
	if len(onDisk.Items) != 1 || onDisk.Items[0].Group.RootID != "999" {
		t.Fatalf("历史文件内容错误: %+v", onDisk)
	}
}

func TestWorkshopHistorySkipsEmptyRootIDAndClears(t *testing.T) {
	a := newHistoryTestApp(t)

	storage, err := a.AddWorkshopHistoryEntries([]WorkshopHistoryItem{
		{RootID: "   "},
		{RootID: "777", Title: "有效"},
	})
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if len(storage.Items) != 1 || storage.Items[0].RootID != "777" {
		t.Fatalf("空 rootId 未被过滤: %+v", storage.Items)
	}

	if err := a.ClearWorkshopHistory(); err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if got := a.GetWorkshopHistory(); len(got.Items) != 0 {
		t.Fatalf("清空后仍有记录: %+v", got.Items)
	}
}

func TestWorkshopHistoryToleratesCorruptFile(t *testing.T) {
	a := newHistoryTestApp(t)
	a.ensureConfigPaths()
	if err := os.WriteFile(a.workshopHistoryPath, []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("准备损坏文件失败: %v", err)
	}

	if got := a.GetWorkshopHistory(); len(got.Items) != 0 {
		t.Fatalf("损坏文件应退化成空历史，实际 %+v", got.Items)
	}
	if _, err := a.AddWorkshopHistoryEntries([]WorkshopHistoryItem{{RootID: "1", Title: "重建"}}); err != nil {
		t.Fatalf("损坏文件后仍应能写入: %v", err)
	}
	if got := a.GetWorkshopHistory(); len(got.Items) != 1 {
		t.Fatalf("重建后读取错误: %+v", got.Items)
	}
}
