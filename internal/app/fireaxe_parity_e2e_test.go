package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 本文件是「FireAxe 对照表的端到端验收」：每一项都用真实 fixture
// （真实 VPK 文件 + 真实 addonlist.txt + 真实配置目录）驱动**导出方法**，
// 语义上等价于用户在界面上点一遍，因此比只测内部函数更接近"实际使用"。
//
// 覆盖 docs/development/fireaxe-parity.md §4 的 1-8 项；10-16 项在
// fireaxe_parity_flow_test.go；第 9 项（i18n）是纯前端，由 node --test 覆盖。

// parityGameDir 返回 fixture 里的 left4dead2 目录。
func parityGameDir(addonsDir string) string { return filepath.Dir(addonsDir) }

func parityAddonListPath(addonsDir string) string {
	return filepath.Join(parityGameDir(addonsDir), "addonlist.txt")
}

// parityConflictFiles 把冲突结果里出现过的归档路径收集成集合。
func parityConflictFiles(result *ConflictResult) map[string]bool {
	seen := map[string]bool{}
	if result == nil {
		return seen
	}
	for _, group := range result.ConflictGroups {
		for _, file := range group.Files {
			seen[file] = true
		}
	}
	return seen
}

// parityGroupParticipants 返回"包含指定归档路径的冲突组"里参与其中的 VPK 数量。
func parityGroupParticipants(result *ConflictResult, file string) int {
	if result == nil {
		return 0
	}
	for _, group := range result.ConflictGroups {
		for _, candidate := range group.Files {
			if candidate == file {
				return len(group.VpkFiles)
			}
		}
	}
	return 0
}

// parityDownloadableWorkshopItem 造一个"有下载地址"的工坊条目（叶子节点）。
func parityDownloadableWorkshopItem(id string) WorkshopFileDetails {
	return WorkshopFileDetails{
		Result:          1,
		PublishedFileId: id,
		Title:           "item-" + id,
		Filename:        id + ".vpk",
		FileUrl:         "https://example.invalid/" + id + ".vpk",
		FileSize:        "1024",
	}
}

// 01 统一优先级模型：显式分层参与排序；没有分层时逐字节不变。
func TestParity01UnifiedPriorityModelEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	path := parityAddonListPath(addonsDir)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// 未设置任何分层：应用分层必须完全不写盘（逐字节一致）。
	if _, err := a.ApplyModPriorityLayers(); err != nil {
		t.Fatalf("未分层时应用分层失败: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("未设置分层时 addonlist.txt 不应变化:\nbefore=%q\nafter=%q", before, after)
	}

	// 显式分层：b.vpk 最优先（-5），a.vpk 最后（+5）。
	// 注意只能用 addonlist.txt 里已记录过的条目 —— 只存在于磁盘、没进过 addonlist 的
	// 文件（例如夹具里的 c.vpk）没有"顺序号"，重排时不会被凭空写进去。
	if _, err := a.SetModPriority("b.vpk", "b.vpk", -5); err != nil {
		t.Fatalf("设置 b 分层失败: %v", err)
	}
	if _, err := a.SetModPriority("a.vpk", "a.vpk", 5); err != nil {
		t.Fatalf("设置 a 分层失败: %v", err)
	}

	plan, err := a.GetModPriorityPlan()
	if err != nil {
		t.Fatalf("读取有效分层失败: %v", err)
	}
	staged := map[string]ModEffectivePriority{}
	for _, entry := range plan {
		staged[entry.Key] = entry
	}
	if got := staged["b.vpk"]; got.Effective != -5 || got.Source == "" || got.Tier == nil {
		t.Fatalf("b.vpk 的有效分层异常: %+v", got)
	}
	if got := staged["a.vpk"]; got.Effective != 5 || got.Tier == nil {
		t.Fatalf("a.vpk 的有效分层异常: %+v", got)
	}

	if _, err := a.ApplyModPriorityLayers(); err != nil {
		t.Fatalf("应用分层失败: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	bIndex := strings.Index(text, `"b.vpk"`)
	aIndex := strings.Index(text, `"a.vpk"`)
	if bIndex < 0 || aIndex < 0 {
		t.Fatalf("重排后条目丢失:\n%s", text)
	}
	if bIndex > aIndex {
		t.Fatalf("分层 -5 的 b.vpk 应排在 +5 的 a.vpk 之前:\n%s", text)
	}

	// 清空分层后回到"顺序号"语义。
	if err := a.ClearModPriority("b.vpk"); err != nil {
		t.Fatalf("清除 b 分层失败: %v", err)
	}
	if err := a.ClearModPriority("a.vpk"); err != nil {
		t.Fatalf("清除 a 分层失败: %v", err)
	}
	entries, err := a.ListModPriorities()
	if err != nil {
		t.Fatalf("列出分层失败: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("清空后不应残留分层记录: %+v", entries)
	}
}

// 02 每 Mod 冲突忽略文件：内置规则 ∪ 全局清单 ∪ 单 Mod 清单。
func TestParity02PerModConflictIgnoreEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	baseline, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	if !parityConflictFiles(baseline)["materials/shared.vtf"] {
		t.Fatalf("前置条件失败：夹具本身应有 materials/shared.vtf 冲突: %+v", parityConflictFiles(baseline))
	}

	// 让 a.vpk 声明"我不提供这个文件"。
	if _, err := a.SetModIgnoreFiles("a.vpk", "a.vpk", []string{"materials/shared.vtf"}); err != nil {
		t.Fatalf("写入单 Mod 忽略清单失败: %v", err)
	}
	got, err := a.GetModIgnoreFiles("a.vpk")
	if err != nil {
		t.Fatalf("读取单 Mod 忽略清单失败: %v", err)
	}
	if len(got) != 1 || got[0] != "materials/shared.vtf" {
		t.Fatalf("单 Mod 忽略清单 = %v", got)
	}

	after, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	// 该路径由 a / b / workshop\123 三份 VPK 共同提供：a 声明忽略后，
	// 参与这条冲突的 VPK 应从 3 个降到 2 个（冲突组本身仍然存在）。
	baselineParticipants := parityGroupParticipants(baseline, "materials/shared.vtf")
	afterParticipants := parityGroupParticipants(after, "materials/shared.vtf")
	if baselineParticipants < 2 {
		t.Fatalf("前置条件失败：该路径应有多个参与者，实际 %d", baselineParticipants)
	}
	if afterParticipants >= baselineParticipants {
		t.Fatalf("忽略后参与者应减少：before=%d after=%d", baselineParticipants, afterParticipants)
	}
	if after.TotalModIgnoreAnnotations == 0 {
		t.Fatal("应至少有一条「被自身规则跳过」的标注")
	}

	records, err := a.ListModIgnoreRecords()
	if err != nil {
		t.Fatalf("列出忽略记录失败: %v", err)
	}
	if len(records) != 1 || records[0].Key != "a.vpk" {
		t.Fatalf("忽略记录 = %+v", records)
	}

	// 清空后冲突恢复。
	if _, err := a.SetModIgnoreFiles("a.vpk", "a.vpk", nil); err != nil {
		t.Fatalf("清空忽略清单失败: %v", err)
	}
	restored, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := parityGroupParticipants(restored, "materials/shared.vtf"); got != baselineParticipants {
		t.Fatalf("清空忽略后参与者应恢复：want=%d got=%d", baselineParticipants, got)
	}
	_ = addonsDir
}

// 03 本地记录备份轮转：内容相同跳过 / 数量上限 / 溢出进回收站 / 可列举。
func TestParity03LocalStoreBackupRotationEndToEnd(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	clock := int64(0)
	a.localStoreBackupClock = func() time.Time { return time.Unix(1_700_000_000+clock, 0) }
	a.localStoreBackupInterval = 0 // 关掉"最短间隔"，专心验证内容去重与上限
	a.localStoreBackupMaxFiles = 2
	trashed := make([]string, 0)
	a.localStoreBackupRemove = func(path string) error {
		trashed = append(trashed, path)
		return os.Remove(path)
	}

	// 第一次写入建立文件（无备份），随后每次改内容都会先备份旧内容。
	if _, err := a.SetModPriority("a.vpk", "a.vpk", 1); err != nil {
		t.Fatal(err)
	}
	// 完全相同的内容再写一次：不应产生新备份。
	if _, err := a.SetModPriority("a.vpk", "a.vpk", 1); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 4; step++ {
		// 默认最短间隔是 15 分钟，这里每次推进 1 小时，确保每次改内容都能落一份备份。
		clock += 3600
		if _, err := a.SetModPriority("a.vpk", "a.vpk", 10+step); err != nil {
			t.Fatal(err)
		}
	}

	backups, err := a.ListLocalStoreBackups("priority")
	if err != nil {
		t.Fatalf("列出备份失败: %v", err)
	}
	if len(backups) > 2 {
		t.Fatalf("备份数量应受上限约束（≤2），实际 %d: %v", len(backups), backups)
	}
	if len(backups) == 0 {
		t.Fatal("改内容后应至少产生一份备份")
	}
	if len(trashed) == 0 {
		t.Fatal("超过上限的旧备份应被移出（回收站）")
	}
}

// 04 变更驱动复检：失效 → 重算 → 修复建议。
func TestParity04RecheckInvalidateRecomputeEndToEnd(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	a.InvalidateConflictRecheck("e2e：模拟 Mod 状态变化")
	dirty := a.GetConflictRecheckStatus()
	if !dirty.Dirty {
		t.Fatalf("失效后应标记为脏: %+v", dirty)
	}
	if !strings.Contains(dirty.Reason, "e2e") {
		t.Fatalf("失效原因应被记录: %+v", dirty)
	}

	fresh, err := a.RecheckConflictsNow()
	if err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	if fresh.Dirty {
		t.Fatalf("重算后不应仍是脏的: %+v", fresh)
	}
	if fresh.RecomputeCount <= dirty.RecomputeCount {
		t.Fatalf("重算次数应增加: before=%d after=%d", dirty.RecomputeCount, fresh.RecomputeCount)
	}

	suggestions, err := a.GetConflictFixSuggestions()
	if err != nil {
		t.Fatalf("读取修复建议失败: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatal("夹具存在冲突时应有修复建议")
	}
}

// 05 游戏原版文件白名单：内置批次生效，白名单外的重叠仍然报冲突。
func TestParity05StockWhitelistEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	status, err := a.GetStockWhitelistStatus()
	if err != nil {
		t.Fatalf("读取白名单状态失败: %v", err)
	}
	if len(status.BuiltinBatches) == 0 || status.TotalPaths == 0 {
		t.Fatalf("内置批次应可用: %+v", status)
	}
	if status.Degraded {
		t.Fatalf("内置批次不应处于降级状态: %+v", status)
	}

	// 引擎胶水路径（内置批次内）与自定义路径共存：前者不报，后者要报。
	writeTestVPK(t, filepath.Join(addonsDir, "glue-e2e-a.vpk"), map[string][]byte{
		"sound/sound.cache":  []byte("a"),
		"materials/real.vtf": []byte("a"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "glue-e2e-b.vpk"), map[string][]byte{
		"sound/sound.cache":  []byte("b"),
		"materials/real.vtf": []byte("b"),
	})

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := parityConflictFiles(result)
	if seen["sound/sound.cache"] {
		t.Fatalf("内置白名单路径不应报冲突: %v", seen)
	}
	if !seen["materials/real.vtf"] {
		t.Fatalf("白名单以外的重叠必须保留: %v", seen)
	}

	if err := a.ReloadStockWhitelist(); err != nil {
		t.Fatalf("重新加载白名单失败: %v", err)
	}
}

// 06 自动重下（默认关闭）：失败后只自动重试一次。
func TestParity06AutoRedownloadEndToEnd(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if err := a.SetWorkshopAutoRedownload(true); err != nil {
		t.Fatalf("开启自动重下失败: %v", err)
	}
	if !a.GetWorkshopAutoRedownload() {
		t.Fatal("自动重下开关应已打开")
	}

	var starts int
	withDownloadTaskStarter(t, func(app *App, ctx context.Context, task *DownloadTask, url string) {
		starts++
		taskManager.mu.Lock()
		task.Status = "failed"
		task.Error = "e2e：模拟网络中断"
		taskManager.mu.Unlock()
		app.maybeAutoRedownload(task.ID)
	})

	id := a.StartDownloadTask(WorkshopFileDetails{
		PublishedFileId: "900001",
		Filename:        "900001.vpk",
		FileUrl:         "https://example.invalid/900001.vpk",
		Title:           "e2e 自动重下",
	}, false)
	t.Cleanup(func() { removeDownloadTask(id) })

	taskManager.mu.RLock()
	task := taskManager.tasks[id]
	attempts := 0
	status := ""
	if task != nil {
		attempts = task.RedownloadAttempts
		status = task.Status
	}
	taskManager.mu.RUnlock()

	if starts != 2 {
		t.Fatalf("应恰好启动两次（原始 + 一次自动重下），实际 %d", starts)
	}
	if attempts != 1 {
		t.Fatalf("自动重下次数应记为 1，实际 %d（状态 %s）", attempts, status)
	}
	if status != "failed" {
		t.Fatalf("第二次仍失败时应保持 failed，实际 %s", status)
	}
}

// 07 工坊合集实体化：捕获 → 刷新差异 → 排队下载缺失成员。
func TestParity07WorkshopCollectionEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	catalog := map[string]WorkshopFileDetails{
		"1000": workshopDetailWithChildren("1000", "2001", "2002"),
		// 叶子条目必须带 FileUrl，否则会被判成"没有可下载文件"而不计入成员。
		"2001": parityDownloadableWorkshopItem("2001"),
		"2002": parityDownloadableWorkshopItem("2002"),
	}
	fetcher, _ := newFakeWorkshopFetcher(t, catalog)
	a.workshopDetailsFetcher = fetcher

	link, err := a.CaptureWorkshopCollection("1000")
	if err != nil {
		t.Fatalf("捕获合集失败: %v", err)
	}
	if len(link.Members) != 2 {
		t.Fatalf("合集成员数 = %d, 期望 2", len(link.Members))
	}

	list, err := a.ListWorkshopCollections()
	if err != nil || len(list) != 1 {
		t.Fatalf("列出合集失败: %v %+v", err, list)
	}

	// 把成员 2001 放进 workshop 目录，刷新后应少一个缺失。
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestVPK(t, filepath.Join(addonsDir, "workshop", "2001.vpk"), map[string][]byte{"materials/x.vtf": []byte("x")})

	refresh, err := a.RefreshWorkshopCollection(link.ID)
	if err != nil {
		t.Fatalf("刷新合集失败: %v", err)
	}
	if refresh.TotalCount != 2 || refresh.MissingCount != 1 {
		t.Fatalf("刷新结果异常: %+v", refresh)
	}

	// 下载缺失成员：注入下载启动器避免真联网，只验证排队与去重。
	queued := make([]string, 0)
	withDownloadTaskStarter(t, func(app *App, ctx context.Context, task *DownloadTask, url string) {
		queued = append(queued, task.WorkshopID)
		app.emitTaskUpdated(task)
	})
	ids, err := a.DownloadWorkshopCollection(link.ID)
	if err != nil {
		t.Fatalf("下载合集失败: %v", err)
	}
	for _, id := range ids {
		t.Cleanup(func() { removeDownloadTask(id) })
	}
	if len(ids) == 0 {
		t.Fatal("应至少为缺失成员排队一个下载任务")
	}
	if len(queued) > 1 {
		t.Fatalf("只应下载缺失成员（1 个），实际排队 %v", queued)
	}
}

// 08 树形分组：最多 4 层 + 子组权重按祖先链累加。
func TestParity08StrategyGroupTreeAndTierEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	member := func(name string) []string { return []string{filepath.Join(addonsDir, name)} }

	lv1, err := a.CaptureModStrategyGroup("一级", "", modStrategyGroupSingle, member("a.vpk"))
	if err != nil {
		t.Fatal(err)
	}
	lv2, err := a.CreateModStrategyGroupChild(lv1.ID, "二级", modStrategyGroupSingle, member("b.vpk"))
	if err != nil {
		t.Fatal(err)
	}
	lv3, err := a.CreateModStrategyGroupChild(lv2.ID, "三级", modStrategyGroupSingle, member("c.vpk"))
	if err != nil {
		t.Fatal(err)
	}
	lv4, err := a.CreateModStrategyGroupChild(lv3.ID, "四级", modStrategyGroupSingle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateModStrategyGroupChild(lv4.ID, "五级", modStrategyGroupSingle, nil); err == nil {
		t.Fatal("第 5 层应被拒绝")
	}

	tree, err := a.ListModStrategyGroupTree()
	if err != nil {
		t.Fatal(err)
	}
	depth := 0
	node := tree
	for len(node) > 0 && depth < 6 {
		depth++
		node = node[0].Children
	}
	if depth != 4 {
		t.Fatalf("树深度 = %d, 期望 4", depth)
	}

	// 一级 -2、二级 -3 → 二级成员 b.vpk 的组权重应为 -5。
	parentTier := -2
	if _, err := a.SetModStrategyGroupTier(lv1.ID, &parentTier); err != nil {
		t.Fatal(err)
	}
	childTier := -3
	if _, err := a.SetModStrategyGroupTier(lv2.ID, &childTier); err != nil {
		t.Fatal(err)
	}

	plan, err := a.GetModPriorityPlan()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range plan {
		if entry.Key != "b.vpk" {
			continue
		}
		if entry.GroupTier == nil || *entry.GroupTier != -5 {
			t.Fatalf("子组权重应按祖先链累加为 -5，实际 %+v", entry.GroupTier)
		}
		if entry.Effective != -5 {
			t.Fatalf("有效分层 = %d, 期望 -5", entry.Effective)
		}
		return
	}
	t.Fatal("计划里找不到 b.vpk")
}
