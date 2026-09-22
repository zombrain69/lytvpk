package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 组与 addonlist 是两套独立状态：把成员从 addonlist 里移除（例如整组关闭）
// 不会删除组成员，之后再次应用仍然能把它写回去。
func TestGroupMembersSurviveAddonListRemovalAndComeBack(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("存活组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// 先整组打开，再整组关闭：成员不会从组里消失。
	if _, err := a.SetModStrategyGroupEnabled(group.ID, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, err := a.SetModStrategyGroupEnabled(group.ID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	reloaded, err := a.findModStrategyGroup(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Members) != 2 {
		t.Fatalf("整组关闭后成员不应被移除: %#v", reloaded.Members)
	}

	// 再次打开：即使 addonlist 条目被删掉，也能按组重新写回。
	content, removed, err := removeAddonListEntriesForTest(a, []string{"a.vpk"})
	if err != nil {
		t.Fatalf("remove entry: %v", err)
	}
	if !removed {
		t.Fatalf("测试前置失败：addonlist 里没有 a.vpk\n%s", content)
	}
	result, err := a.SetModStrategyGroupEnabled(group.ID, true)
	if err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if len(result.Enabled) != 2 {
		t.Fatalf("重新应用应写回全部成员: %#v", result)
	}
	after, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, `"a.vpk"`) {
		t.Fatalf("addonlist 应重新包含 a.vpk:\n%s", after)
	}
}

// 文件被删除（不在受管范围内）时，应用组必须跳过它，而不是往 addonlist 写幽灵条目。
func TestApplyModStrategyGroupSkipsMembersWhoseFileIsGone(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("含缺失成员", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// 把 b.vpk 从磁盘与缓存里移除（模拟用户删除/移走文件）。
	bPath := filepath.Join(addonsDir, "b.vpk")
	if err := os.Remove(bPath); err != nil {
		t.Fatal(err)
	}
	a.vpkCache.Delete(bPath)

	result, err := a.SetModStrategyGroupEnabled(group.ID, true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != "b.vpk" {
		t.Fatalf("应报告被跳过的成员: %#v", result)
	}
	if len(result.Enabled) != 1 {
		t.Fatalf("只应处理仍然存在的成员: %#v", result)
	}
	content, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	// 跳过 = 完全不碰它的既有条目（既不改值、也不新增幽灵条目）。
	if count := strings.Count(strings.ToLower(content), `"b.vpk"`); count != 1 {
		t.Fatalf("缺失成员的 addonlist 条目数应保持 1: %d\n%s", count, content)
	}

	// 把文件放回来后，同一个组依然能作用到它（键没有变）。
	writeTestVPK(t, bPath, map[string][]byte{"materials/back.vtf": []byte("back")})
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("rescan: %v", err)
	}
	restored, err := a.SetModStrategyGroupEnabled(group.ID, true)
	if err != nil {
		t.Fatalf("apply after restore: %v", err)
	}
	if len(restored.Enabled) != 2 || len(restored.Skipped) != 0 {
		t.Fatalf("文件放回后应重新纳入整组: %#v", restored)
	}
}

// 界面用的"缺失成员"数据：组归属带 missing 标记，并有按组汇总的缺失清单。
func TestGroupMembershipReportsMissingMembers(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("含缺失", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	bPath := filepath.Join(addonsDir, "b.vpk")
	if err := os.Remove(bPath); err != nil {
		t.Fatal(err)
	}
	a.vpkCache.Delete(bPath)

	membership, err := a.GetModGroupMembership()
	if err != nil {
		t.Fatal(err)
	}
	missingByKey := map[string]bool{}
	for _, item := range membership {
		missingByKey[item.Key] = item.Missing
	}
	if missingByKey["a.vpk"] {
		t.Fatalf("a.vpk 存在，不应标记缺失: %#v", membership)
	}
	if !missingByKey["b.vpk"] {
		t.Fatalf("b.vpk 已被删除，应标记缺失: %#v", membership)
	}

	summary, err := a.GetModStrategyGroupMissingMembers()
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 1 || summary[0].GroupID != group.ID {
		t.Fatalf("缺失汇总异常: %#v", summary)
	}
	if summary[0].MissingCount != 1 || summary[0].MemberCount != 2 || len(summary[0].MissingNames) != 1 {
		t.Fatalf("缺失汇总字段异常: %#v", summary[0])
	}
	if summary[0].MissingNames[0] != "b.vpk" {
		t.Fatalf("缺失成员名异常: %#v", summary[0].MissingNames)
	}

	// 把文件放回来：缺失标记消失。
	writeTestVPK(t, bPath, map[string][]byte{"materials/back.vtf": []byte("back")})
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	summary, err = a.GetModStrategyGroupMissingMembers()
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 0 {
		t.Fatalf("文件放回后不应再报缺失: %#v", summary)
	}
}
// 改名后，策略组 / 分层 / 依赖 / 忽略清单里的旧键都要跟着迁移。
func TestRenameVPKFileRebindsLocalRecords(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	oldPath := filepath.Join(addonsDir, "a.vpk")
	newPath := filepath.Join(addonsDir, "renamed.vpk")

	if _, err := a.CaptureModStrategyGroup("改名组", "", modStrategyGroupSingle, []string{oldPath}); err != nil {
		t.Fatalf("group: %v", err)
	}
	if _, err := a.SetModPriority("a.vpk", "a.vpk", 7); err != nil {
		t.Fatalf("priority: %v", err)
	}
	if _, err := a.SetModDependencies(oldPath, []string{filepath.Join(addonsDir, "b.vpk")}); err != nil {
		t.Fatalf("dependencies: %v", err)
	}
	if _, err := a.SetModIgnoreFiles("a.vpk", "a.vpk", []string{"materials/shared.vtf"}); err != nil {
		t.Fatalf("ignore: %v", err)
	}

	if _, err := a.RenameVPKFile(oldPath, "renamed.vpk"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("重命名后的文件不存在: %v", err)
	}

	groups, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, group := range groups {
		for _, member := range group.Members {
			if member.Key == "renamed.vpk" {
				found = true
			}
			if member.Key == "a.vpk" {
				t.Fatalf("策略组仍指向旧键: %#v", member)
			}
		}
	}
	if !found {
		t.Fatal("策略组没有跟到新键")
	}

	entries, err := a.ListModPriorities()
	if err != nil {
		t.Fatal(err)
	}
	priorityKeys := map[string]int{}
	for _, entry := range entries {
		priorityKeys[entry.Key] = entry.Tier
	}
	if priorityKeys["renamed.vpk"] != 7 {
		t.Fatalf("优先级分层没有跟到新键: %#v", priorityKeys)
	}

	records, err := a.ListModDependencies()
	if err != nil {
		t.Fatal(err)
	}
	rebound := false
	for _, record := range records {
		if record.Key == "a.vpk" {
			t.Fatalf("依赖记录仍指向旧键: %#v", record)
		}
		if record.Key == "renamed.vpk" {
			rebound = true
		}
	}
	if !rebound {
		t.Fatal("依赖记录没有跟到新键")
	}

	ignore, err := a.readModIgnoreStore()
	if err != nil {
		t.Fatal(err)
	}
	ignoreRebound := false
	for _, record := range ignore.Records {
		if record.Key == "a.vpk" {
			t.Fatalf("忽略清单仍指向旧键: %#v", record)
		}
		if record.Key == "renamed.vpk" {
			ignoreRebound = true
		}
	}
	if !ignoreRebound {
		t.Fatal("忽略清单没有跟到新键")
	}
}

// removeAddonListEntriesForTest 直接删掉 addonlist.txt 里的某些条目。
func removeAddonListEntriesForTest(a *App, keys []string) (string, bool, error) {
	doc, err := a.readAddonListDocument()
	if err != nil {
		return "", false, err
	}
	lines := strings.SplitAfter(doc.content, "\n")
	targets := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		targets[normalizeAddonListKey(key)] = struct{}{}
	}
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if matches := addonListValueLineRegex.FindStringSubmatch(trimmed); len(matches) == 5 {
			if _, ok := targets[normalizeAddonListKey(matches[2])]; ok {
				removed = true
				continue
			}
		}
		kept = append(kept, line)
	}
	if !removed {
		return doc.content, false, nil
	}
	doc.content = strings.Join(kept, "")
	if err := a.writeAddonListDocument(doc, doc.content); err != nil {
		return "", false, err
	}
	return doc.content, true, nil
}

func (a *App) readAddonListContentForTest() (string, error) {
	doc, err := a.readAddonListDocument()
	if err != nil {
		return "", err
	}
	return doc.content, nil
}
