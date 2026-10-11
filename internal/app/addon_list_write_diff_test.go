package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func addonListItemFixture(names ...string) []AddonListItem {
	items := make([]AddonListItem, 0, len(names))
	for _, name := range names {
		items = append(items, AddonListItem{Name: name, Value: "1"})
	}
	return items
}

// 差异计算是写前确认弹窗的唯一数据来源：删除不能把后续条目误报成"整体移动"，
// 大小写不同的同一条目不能算变化，明细列表必须按文件顺序稳定输出。
func TestDiffAddonListForWrite(t *testing.T) {
	// 删除：只有被删的条目算变化，其余保持"未变化"（不应出现"删 1 条 = 全体 -1 位"的噪音）。
	diff := diffAddonListForWrite(
		addonListItemFixture("a.vpk", "ghost.vpk", "b.vpk", "c.vpk"),
		addonListItemFixture("a.vpk", "b.vpk", "c.vpk"),
		0)
	if diff.MovedTotal != 0 || diff.Unchanged != 3 {
		t.Fatalf("删除场景不应产生移动：%#v", diff)
	}
	if !reflect.DeepEqual(diff.Removed, []string{"ghost.vpk"}) {
		t.Fatalf("removed = %#v", diff.Removed)
	}
	if diff.IsEmpty() {
		t.Fatal("有删除就不是空差异")
	}
	if diff.CountBefore != 4 || diff.CountAfter != 3 {
		t.Fatalf("计数 = %d → %d", diff.CountBefore, diff.CountAfter)
	}

	// 交换两条 = 2 条移动，明细带上文件里的实际位置。
	diff = diffAddonListForWrite(addonListItemFixture("a.vpk", "b.vpk"), addonListItemFixture("b.vpk", "a.vpk"), 0)
	if diff.MovedTotal != 2 || diff.Unchanged != 0 {
		t.Fatalf("交换场景 = %#v", diff)
	}
	movedByKey := map[string]AddonListMovedEntry{}
	for _, entry := range diff.Moved {
		movedByKey[entry.Key] = entry
	}
	if movedByKey["a.vpk"].OldOrder != 1 || movedByKey["a.vpk"].NewOrder != 2 {
		t.Fatalf("a.vpk 移动明细错误：%#v", movedByKey["a.vpk"])
	}
	if movedByKey["b.vpk"].OldOrder != 2 || movedByKey["b.vpk"].NewOrder != 1 {
		t.Fatalf("b.vpk 移动明细错误：%#v", movedByKey["b.vpk"])
	}

	// 完全一致 → 空差异；键的大小写不同不算变化（与写回口径一致）。
	if diff = diffAddonListForWrite(addonListItemFixture("a.vpk"), addonListItemFixture("a.vpk"), 0); !diff.IsEmpty() {
		t.Fatalf("无变化应为空差异：%#v", diff)
	}
	if diff = diffAddonListForWrite(addonListItemFixture("A.VPK"), addonListItemFixture("a.vpk"), 0); !diff.IsEmpty() {
		t.Fatalf("大小写不敏感应为空差异：%#v", diff)
	}

	// 新增。
	diff = diffAddonListForWrite(addonListItemFixture("a.vpk"), addonListItemFixture("a.vpk", "b.vpk"), 0)
	if !reflect.DeepEqual(diff.Added, []string{"b.vpk"}) || diff.MovedTotal != 0 {
		t.Fatalf("新增场景 = %#v", diff)
	}

	// 合并重复也要算变化（重复条目会被写回逻辑折叠掉）。
	diff = diffAddonListForWrite(addonListItemFixture("a.vpk"), addonListItemFixture("a.vpk"), 2)
	if diff.DuplicatesFolded != 2 || diff.IsEmpty() {
		t.Fatalf("重复合并 = %#v", diff)
	}
}

// 预览与写入必须共用同一份计划：预览报告"清理 + 重排"，应用后的结果口径一致；
// 应用完成后再次预览应为空差异。
func TestPreviewAddonListLoadOrderPolicyReportsWriteDiff(t *testing.T) {
	app, addonsDir := newPriorityTestApp(t)
	gameDir := filepath.Dir(addonsDir)
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: `workshop\123.vpk`, Value: "1"},
		{Name: "ghost.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
	})

	preview, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.Diff == nil {
		t.Fatal("预览必须携带写前差异")
	}
	if want := []string{"ghost.vpk"}; !reflect.DeepEqual(preview.RemovedStale, want) {
		t.Fatalf("removedStale = %#v, want %#v", preview.RemovedStale, want)
	}
	if preview.Diff.MovedTotal != 2 {
		t.Fatalf("movedTotal = %d, want 2（工坊条目与 b.vpk 换位）", preview.Diff.MovedTotal)
	}
	if len(preview.Diff.Removed) != 1 || preview.Diff.Removed[0] != "ghost.vpk" {
		t.Fatalf("diff.removed = %#v", preview.Diff.Removed)
	}
	if preview.Diff.IsEmpty() {
		t.Fatal("有清理和重排，不应是空差异")
	}
	// 预览不落盘。
	raw, err := os.ReadFile(filepath.Join(gameDir, "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "ghost.vpk") {
		t.Fatal("预览不得改动 addonlist.txt")
	}

	applied, err := app.ApplyAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Diff == nil || applied.Diff.MovedTotal != preview.Diff.MovedTotal ||
		len(applied.Diff.Removed) != len(preview.Diff.Removed) {
		t.Fatalf("应用结果与预览口径不一致：preview=%#v applied=%#v", preview.Diff, applied.Diff)
	}

	again, err := app.PreviewAddonListLoadOrderPolicy(AddonListLoadOrderPolicy{RootFirst: true})
	if err != nil {
		t.Fatalf("preview again: %v", err)
	}
	if again.Diff == nil || !again.Diff.IsEmpty() {
		t.Fatalf("应用后应无差异：%#v", again.Diff)
	}
}

// 按分层应用同样要有写前差异；它不做失效清理（与 Apply 行为一致）。
func TestPreviewModPriorityLayersReportsDiff(t *testing.T) {
	app, addonsDir := newPriorityTestApp(t)
	if _, err := app.SetModPriority("b.vpk", "b.vpk", -5); err != nil {
		t.Fatalf("set priority: %v", err)
	}

	preview, err := app.PreviewModPriorityLayers()
	if err != nil {
		t.Fatalf("preview priority layers: %v", err)
	}
	if preview.Diff == nil || preview.Diff.MovedTotal == 0 {
		t.Fatalf("按分层预览应报告移动：%#v", preview.Diff)
	}
	if len(preview.Diff.Removed) != 0 || preview.Diff.DuplicatesFolded != 0 {
		t.Fatalf("按分层应用不做清理/合并：%#v", preview.Diff)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(addonsDir), "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"a.vpk"`) {
		t.Fatal("预览不得改动 addonlist.txt")
	}

	applied, err := app.ApplyModPriorityLayers()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Diff == nil || applied.Diff.MovedTotal == 0 {
		t.Fatalf("应用应返回同样口径的差异：%#v", applied.Diff)
	}

	again, err := app.PreviewModPriorityLayers()
	if err != nil {
		t.Fatalf("preview again: %v", err)
	}
	if again.Diff == nil || !again.Diff.IsEmpty() {
		t.Fatalf("应用后应无差异：%#v", again.Diff)
	}
}

// 「恢复上一份备份」的候选选择（首次编辑备份 vs 受管备份取更新者）与首次编辑备份恢复。
func TestAddonListRestoreCandidateAndFirstEditRestore(t *testing.T) {
	original := "\"AddonList\"\n{\n\t\"orig.vpk\"\t\t\"1\"\n}\n"
	app, addonListPath := newAddonListManagerTestApp(t, original)

	// 无任何备份时明确报告"没有可恢复的备份"。
	empty, err := app.GetAddonListRestoreCandidate()
	if err != nil {
		t.Fatalf("candidate(empty): %v", err)
	}
	if empty.Found {
		t.Fatalf("不应有候选：%#v", empty)
	}

	// 模拟 LytVPK 首次改写留下的 .lytvpk.bak（时间较早）与当前内容变化。
	firstEditTime := time.Now().Add(-2 * time.Hour)
	if err := os.WriteFile(addonListPath+".lytvpk.bak", []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(addonListPath+".lytvpk.bak", firstEditTime, firstEditTime); err != nil {
		t.Fatal(err)
	}
	current := "\"AddonList\"\n{\n\t\"orig.vpk\"\t\t\"1\"\n\t\"new.vpk\"\t\t\"1\"\n}\n"
	if err := os.WriteFile(addonListPath, []byte(current), 0644); err != nil {
		t.Fatal(err)
	}

	candidate, err := app.GetAddonListRestoreCandidate()
	if err != nil {
		t.Fatalf("candidate(first-edit): %v", err)
	}
	if !candidate.Found || candidate.Source != "first-edit" || candidate.Kind != "first-edit" {
		t.Fatalf("应选中首次编辑备份：%#v", candidate)
	}
	if candidate.Diff == nil || !reflect.DeepEqual(candidate.Diff.Removed, []string{"new.vpk"}) {
		t.Fatalf("恢复差异应为「移除 new.vpk」：%#v", candidate.Diff)
	}

	// 新建一份受管备份（时间更新）→ 候选应改选它。
	if _, err := app.CreateAddonListBackup(); err != nil {
		t.Fatalf("create managed backup: %v", err)
	}
	candidate, err = app.GetAddonListRestoreCandidate()
	if err != nil {
		t.Fatalf("candidate(managed): %v", err)
	}
	if candidate.Source != "managed" || candidate.Name == "" {
		t.Fatalf("更新时间更近的受管备份应胜出：%#v", candidate)
	}

	// 恢复首次编辑备份：文件回到原始内容，当前内容留档为 before-restore，受保护快照同步更新。
	if _, err := app.RestoreAddonListFirstEditBackup(); err != nil {
		t.Fatalf("restore first-edit: %v", err)
	}
	restored, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != original {
		t.Fatalf("恢复结果 = %q, want %q", restored, original)
	}
	snapshot, err := os.ReadFile(addonListManagedSnapshotPath(addonListPath))
	if err != nil {
		t.Fatalf("read managed snapshot: %v", err)
	}
	if string(snapshot) != original {
		t.Fatalf("受保护快照应同步为恢复内容")
	}
	backups, err := app.ListAddonListBackups()
	if err != nil {
		t.Fatalf("list backups: %v", err)
	}
	foundBeforeRestore := false
	for _, backup := range backups {
		if backup.Kind == "before-restore" {
			foundBeforeRestore = true
		}
	}
	if !foundBeforeRestore {
		t.Fatalf("恢复前必须留 before-restore 备份：%#v", backups)
	}
}
