package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 本文件覆盖 docs/development/fireaxe-parity.md §4 的第 10-16 项，
// 与 fireaxe_parity_e2e_test.go 同样的口径：真实 fixture + 导出方法。
// 第 9 项 i18n 是纯前端文案层，由 frontend 的 `node --test` 覆盖。

// 10 自身崩溃上报：上报 → 列表 → 读取 → 删除。
func TestParity10CrashReportEndToEnd(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	name, err := a.ReportFrontendError("e2e：前端渲染异常", "at renderFileList (app.js:1)")
	if err != nil {
		t.Fatalf("上报失败: %v", err)
	}
	if strings.TrimSpace(name) == "" {
		t.Fatal("上报应返回文件名")
	}

	reports, err := a.ListCrashReports()
	if err != nil {
		t.Fatalf("列出报告失败: %v", err)
	}
	found := false
	for _, report := range reports {
		if report.FileName == name {
			found = true
			if report.Kind != crashReportKindUI {
				t.Fatalf("报告类型 = %q, 期望 %q", report.Kind, crashReportKindUI)
			}
		}
	}
	if !found {
		t.Fatalf("列表里找不到刚上报的报告 %s: %+v", name, reports)
	}

	detail, err := a.ReadCrashReport(name)
	if err != nil {
		t.Fatalf("读取报告失败: %v", err)
	}
	if !strings.Contains(detail.Reason, "e2e") || !strings.Contains(detail.Stack, "app.js") {
		t.Fatalf("报告内容不完整: %+v", detail)
	}
	if detail.Version == "" || detail.OS == "" {
		t.Fatalf("报告应带上环境信息: %+v", detail)
	}

	if err := a.DeleteCrashReport(name); err != nil {
		t.Fatalf("删除报告失败: %v", err)
	}
	after, err := a.ListCrashReports()
	if err != nil {
		t.Fatal(err)
	}
	for _, report := range after {
		if report.FileName == name {
			t.Fatalf("删除后仍能看到报告: %+v", report)
		}
	}
}

// 11 push 事务化：受保护快照同步失败时，addonlist.txt 必须回到写前状态。
func TestParity11AddonListTransactionRollbackEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	path := parityAddonListPath(addonsDir)

	// 打开运行时监控（受保护版本），这是"文件已改、快照还是旧的"最容易出问题的场景。
	if _, err := a.SetAddonListGuardEnabled(true); err != nil {
		t.Fatalf("开启监控失败: %v", err)
	}
	snapshotPath := addonListManagedSnapshotPath(path)

	// 用"目录占位"让快照写入必然失败：写盘成功、同步失败。
	if err := os.Remove(snapshotPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("清理快照失败: %v", err)
	}
	if err := os.MkdirAll(snapshotPath, 0o755); err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(snapshotPath)
	})

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SetModPriority("b.vpk", "b.vpk", -5); err != nil {
		t.Fatalf("设置分层失败: %v", err)
	}
	if _, err := a.ApplyModPriorityLayers(); err == nil {
		t.Fatal("快照同步失败时应用分层必须报错，而不是静默留下半新半旧状态")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("失败后 addonlist.txt 应回滚到写前内容:\nbefore=%q\nafter=%q", before, after)
	}

	// 解除占位后同一操作应当成功（说明失败只是被回滚，而不是把状态搞坏）。
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatalf("解除占位失败: %v", err)
	}
	if _, err := a.ApplyModPriorityLayers(); err != nil {
		t.Fatalf("解除占位后应用分层应成功: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"b.vpk"`) {
		t.Fatalf("重排后条目丢失:\n%s", content)
	}
}

// 12 策略可满足性预检：空组 / 只在 disabled 的组都要在写盘前被拦下。
func TestParity12StrategyApplyPrecheckEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	parent, err := a.CaptureModStrategyGroup("预检父组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := a.CreateModStrategyGroupChild(parent.ID, "空子组", modStrategyGroupSingle, nil)
	if err != nil {
		t.Fatalf("建空子组失败: %v", err)
	}
	check, err := a.CheckModStrategyGroupApply(empty.ID, ModStrategyGroupApplyOptions{})
	if err != nil {
		t.Fatalf("预检失败: %v", err)
	}
	if check.Applicable {
		t.Fatalf("空组不应可执行: %+v", check)
	}
	if !strings.Contains(check.Reason, "还没有成员") {
		t.Fatalf("空组原因不清晰: %q", check.Reason)
	}

	// 成员只剩 disabled 副本：除「全关」外都不能应用。
	group, err := a.CaptureModStrategyGroup("只剩 disabled 副本", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	disabledDir := filepath.Join(addonsDir, "disabled")
	if err := os.MkdirAll(disabledDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(addonsDir, "b.vpk"), filepath.Join(disabledDir, "b.vpk")); err != nil {
		t.Fatalf("移动 b.vpk 到 disabled 失败: %v", err)
	}

	blocked, err := a.CheckModStrategyGroupApply(group.ID, ModStrategyGroupApplyOptions{})
	if err != nil {
		t.Fatalf("预检失败: %v", err)
	}
	if blocked.Applicable {
		t.Fatalf("成员全在 disabled 时「单选」不应可执行: %+v", blocked)
	}
	if !strings.Contains(blocked.Reason, "disabled") || !strings.Contains(blocked.Reason, "批量启用") {
		t.Fatalf("原因应给出解法: %q", blocked.Reason)
	}
	if blocked.BlockedCount != 1 {
		t.Fatalf("BlockedCount = %d, 期望 1", blocked.BlockedCount)
	}

	// 「全关」仍然可执行（写 0 不受 disabled 影响），只给警告。
	off, err := a.CheckModStrategyGroupApply(group.ID, ModStrategyGroupApplyOptions{Strategy: modStrategyGroupOff})
	if err != nil {
		t.Fatal(err)
	}
	if !off.Applicable {
		t.Fatalf("「全关」应可执行: %+v", off)
	}
	if len(off.Warnings) == 0 {
		t.Fatal("「全关」应提示成员都在 disabled")
	}
}

// 13 体检：重复条目被识别为"可自动修复"，一键修复后消失。
func TestParity13HealthAutoFixEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	gameDir := parityGameDir(addonsDir)

	// 制造重复条目（同一键出现两次）。
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
	})

	report, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatalf("体检失败: %v", err)
	}
	hasDuplicate := false
	for _, issue := range report.Issues {
		if issue.Kind == modHealthKindDuplicateEntry {
			hasDuplicate = true
		}
	}
	if !hasDuplicate {
		t.Fatalf("应报出重复条目: %+v", report.Issues)
	}

	removed, err := a.RemoveDuplicateAddonListEntries()
	if err != nil {
		t.Fatalf("一键修复失败: %v", err)
	}
	if removed != 1 {
		t.Fatalf("应删除 1 条重复项，实际 %d", removed)
	}

	after, err := a.RunModHealthCheck(ModHealthCheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range after.Issues {
		if issue.Kind == modHealthKindDuplicateEntry {
			t.Fatalf("修复后不应再有重复条目: %+v", issue)
		}
	}

	// 修复前应留下可恢复备份。
	backups, err := a.ListAddonListBackups()
	if err != nil {
		t.Fatalf("列出备份失败: %v", err)
	}
	if len(backups) == 0 {
		t.Fatal("修复前应建立备份")
	}
}

// 14 本地记录 schema 版本化：旧文件迁移 + 写回盖版本。
func TestParity14LocalStoreSchemaMigrationEndToEnd(t *testing.T) {
	dir := t.TempDir()
	a := &App{configDir: dir}

	legacyGroups := `{
  "groups": [
    {
      "id": "g1",
      "name": "旧版组",
      "strategy": "weird-strategy",
      "members": [
        {"key": "a.vpk", "name": "a.vpk"},
        {"key": "a.vpk", "name": "a.vpk"},
        {"key": "", "name": ""}
      ],
      "parentId": "missing-parent"
    },
    {"id": "g1", "name": "重复 ID", "strategy": "all", "members": []}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "groups.json"), []byte(legacyGroups), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyPriority := `{
  "entries": [
    {"key": "a.vpk", "name": "a.vpk", "tier": 1},
    {"key": "a.vpk", "name": "a.vpk", "tier": 7},
    {"key": "", "name": "空键", "tier": 3}
  ]
}`
	if err := os.WriteFile(filepath.Join(dir, "priority.json"), []byte(legacyPriority), 0o644); err != nil {
		t.Fatal(err)
	}

	groups, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatalf("读取旧版 groups.json 失败: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("重复 ID 应被合并为 1 个组，实际 %d: %+v", len(groups), groups)
	}
	if groups[0].ParentID != "" {
		t.Fatalf("悬空 parentId 应被清空: %+v", groups[0])
	}
	if len(groups[0].Members) != 1 {
		t.Fatalf("重复/空成员应被去掉，实际 %+v", groups[0].Members)
	}
	if groups[0].Strategy != modStrategyGroupSingle {
		t.Fatalf("非法策略应被归一化为默认单选，实际 %q", groups[0].Strategy)
	}

	entries, err := a.ListModPriorities()
	if err != nil {
		t.Fatalf("读取旧版 priority.json 失败: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "a.vpk" || entries[0].Tier != 7 {
		t.Fatalf("同键应保留最后一条、空键丢弃，实际 %+v", entries)
	}

	// 写回时必须盖上当前 schema 版本。
	if _, err := a.SetModPriority("b.vpk", "b.vpk", 2); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "priority.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("写回内容不是合法 JSON: %v", err)
	}
	if stored.SchemaVersion != localStoreSchemaVersion {
		t.Fatalf("写回的 schemaVersion = %d, 期望 %d", stored.SchemaVersion, localStoreSchemaVersion)
	}
}

// 15 下载任务：入队去重 + 1s 节流落盘 + 重启后标记已中断 + 可重试。
func TestParity15DownloadTaskStoreEndToEnd(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	// 隔离全局节流状态：不同测试写不同路径时不应互相影响。
	downloadTaskPersist = &downloadTaskPersister{}
	t.Cleanup(func() { downloadTaskPersist = &downloadTaskPersister{} })

	var started int
	withDownloadTaskStarter(t, func(app *App, ctx context.Context, task *DownloadTask, url string) {
		started++
	})

	details := WorkshopFileDetails{
		PublishedFileId: "test-e2e-15",
		Filename:        "e2e-15.vpk",
		FileUrl:         "https://example.invalid/e2e-15.vpk",
		Title:           "e2e 下载任务",
		FileSize:        "2048",
	}
	first := a.StartDownloadTask(details, false)
	second := a.StartDownloadTask(details, false)
	if first != second {
		t.Fatalf("同一作品重复入队应复用任务：%s vs %s", first, second)
	}
	if started != 1 {
		t.Fatalf("只应启动一次下载，实际 %d", started)
	}
	t.Cleanup(func() { removeDownloadTask(first) })

	// 落盘：带 schemaVersion 的快照文件。
	path := a.ensureDownloadTasksPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("应写入下载任务快照: %v", err)
	}
	var snapshot downloadTaskSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("快照不是合法 JSON: %v", err)
	}
	if snapshot.SchemaVersion != downloadTaskSnapshotSchemaVersion {
		t.Fatalf("schemaVersion = %d", snapshot.SchemaVersion)
	}

	// 1 秒内的第二次写盘应被跳过（mtime 不变）。
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	a.persistDownloadTasksThrottled()
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("1 秒内的重复落盘应被节流：%v → %v", before.ModTime(), after.ModTime())
	}

	// 模拟重启：清空内存任务表后，列表应从快照恢复出「已中断」。
	taskManager.mu.Lock()
	delete(taskManager.tasks, first)
	taskManager.mu.Unlock()

	restored := a.GetDownloadTasks()
	var interrupted *DownloadTask
	for _, task := range restored {
		if task.ID == first {
			interrupted = task
		}
	}
	if interrupted == nil {
		t.Fatalf("重启后应从快照恢复任务: %+v", restored)
	}
	if interrupted.Status != "interrupted" {
		t.Fatalf("恢复状态 = %q, 期望 interrupted", interrupted.Status)
	}
	if interrupted.FileUrl == "" {
		t.Fatal("恢复的任务必须带上 FileUrl，否则无法重试")
	}
	if interrupted.TotalSize != 2048 {
		t.Fatalf("恢复的任务应保留体积信息，实际 %d", interrupted.TotalSize)
	}

	// 中断任务可以重试。
	a.RetryDownloadTask(first)
	taskManager.mu.RLock()
	retried := taskManager.tasks[first]
	status := ""
	if retried != nil {
		status = retried.Status
	}
	taskManager.mu.RUnlock()
	if status != "pending" {
		t.Fatalf("重试后状态 = %q, 期望 pending", status)
	}
	if started != 2 {
		t.Fatalf("重试应再启动一次下载，实际 %d", started)
	}
}

// 16 拖放排序的后端一半：同级顺序写盘并反映到树视图。
func TestParity16StrategyGroupReorderEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	member := func(name string) []string { return []string{filepath.Join(addonsDir, name)} }

	first, err := a.CaptureModStrategyGroup("甲", "", modStrategyGroupSingle, member("a.vpk"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.CaptureModStrategyGroup("乙", "", modStrategyGroupSingle, member("b.vpk"))
	if err != nil {
		t.Fatal(err)
	}
	third, err := a.CaptureModStrategyGroup("丙", "", modStrategyGroupSingle, member("c.vpk"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.ReorderModStrategyGroups([]string{third.ID, first.ID, second.ID}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}
	tree, err := a.ListModStrategyGroupTree()
	if err != nil {
		t.Fatal(err)
	}
	order := make([]string, 0, len(tree))
	for _, node := range tree {
		order = append(order, node.Group.ID)
	}
	if len(order) != 3 || order[0] != third.ID || order[1] != first.ID || order[2] != second.ID {
		t.Fatalf("树视图顺序 = %v, 期望 [丙 甲 乙]", order)
	}

	// 非法输入（漏组）不得改动任何东西。
	if _, err := a.ReorderModStrategyGroups([]string{first.ID}); err == nil {
		t.Fatal("漏组的排序请求应被拒绝")
	}
	tree2, err := a.ListModStrategyGroupTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree2) != 3 || tree2[0].Group.ID != third.ID {
		t.Fatalf("非法请求后顺序不应变化: %+v", tree2)
	}
}
