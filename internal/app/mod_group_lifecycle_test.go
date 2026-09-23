package app

import (
	"os"
	"path/filepath"
	"reflect"
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

// 策略组自身的生命周期（重命名 / 改成员 / 删除时的层级收尾）。

func TestRenameModStrategyGroupUpdatesNameAndDescription(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("旧名字", "旧描述", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	renamed, err := a.RenameModStrategyGroup(group.ID, "  新名字  ", " 新描述 ")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name != "新名字" || renamed.Description != "新描述" {
		t.Fatalf("重命名结果不对: %#v", renamed)
	}
	if renamed.ID != group.ID {
		t.Fatalf("重命名不应改变 ID: %q -> %q", group.ID, renamed.ID)
	}
	if len(renamed.Members) != 2 {
		t.Fatalf("重命名不应改动成员: %#v", renamed.Members)
	}

	reloaded, err := a.findModStrategyGroup(group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Name != "新名字" || reloaded.Description != "新描述" {
		t.Fatalf("重命名没有落盘: %#v", reloaded)
	}

	if _, err := a.RenameModStrategyGroup(group.ID, "   ", ""); err == nil {
		t.Fatal("空名字应被拒绝")
	}
	if _, err := a.RenameModStrategyGroup("no-such-group", "名字", ""); err == nil {
		t.Fatal("未知组应被拒绝")
	}
}

func TestModStrategyGroupMemberEditingKeepsUnlistedMembers(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "c.vpk"), VPKFile{Name: "C 模型.vpk"})
	group, err := a.CaptureModStrategyGroup("成员组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// 加入 c.vpk：原有成员保持，显示名从缓存解析。
	added, err := a.AddModStrategyGroupMembers(group.ID, []string{"c.vpk"})
	if err != nil {
		t.Fatalf("add members: %v", err)
	}
	if got := groupMemberKeys(added); !reflect.DeepEqual(got, []string{"a.vpk", "b.vpk", "c.vpk"}) {
		t.Fatalf("加入后成员 = %#v", got)
	}
	if added.Members[2].Name != "C 模型.vpk" {
		t.Fatalf("显示名应来自缓存: %#v", added.Members[2])
	}
	// 重复加入同一个键不会产生重复成员。
	again, err := a.AddModStrategyGroupMembers(group.ID, []string{"c.vpk", "c.vpk"})
	if err != nil {
		t.Fatalf("add duplicate: %v", err)
	}
	if len(again.Members) != 3 {
		t.Fatalf("重复加入不应新增成员: %#v", again.Members)
	}
	if _, err := a.AddModStrategyGroupMembers(group.ID, []string{"   "}); err == nil {
		t.Fatal("没有有效键时应报错")
	}

	// 移除 a.vpk：其余成员（包括"文件已不在列表里"的成员）不受影响。
	missingKey := "ghost.vpk"
	if _, err := a.AddModStrategyGroupMembers(group.ID, []string{missingKey}); err != nil {
		t.Fatalf("add missing: %v", err)
	}
	removed, err := a.RemoveModStrategyGroupMembers(group.ID, []string{"a.vpk"})
	if err != nil {
		t.Fatalf("remove members: %v", err)
	}
	if got := groupMemberKeys(removed); !reflect.DeepEqual(got, []string{"b.vpk", "c.vpk", missingKey}) {
		t.Fatalf("移除后成员 = %#v", got)
	}
	if _, err := a.RemoveModStrategyGroupMembers(group.ID, []string{"b.vpk", "c.vpk", missingKey}); err == nil {
		t.Fatal("移空成员应被拒绝（组至少保留 1 个成员）")
	}
	// 幂等：移除一个本来就不在组里的键不报错，也不改动成员。
	unchanged, err := a.RemoveModStrategyGroupMembers(group.ID, []string{"never-there.vpk"})
	if err != nil {
		t.Fatalf("移除不存在的成员应幂等成功: %v", err)
	}
	if got := groupMemberKeys(unchanged); !reflect.DeepEqual(got, []string{"b.vpk", "c.vpk", missingKey}) {
		t.Fatalf("幂等移除不应改动成员 = %#v", got)
	}
}

func TestSetModStrategyGroupMembersReplacesWholeList(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("替换组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	replaced, err := a.SetModStrategyGroupMembers(group.ID, []string{"workshop\\77.vpk", "workshop\\77.vpk"})
	if err != nil {
		t.Fatalf("set members: %v", err)
	}
	if got := groupMemberKeys(replaced); !reflect.DeepEqual(got, []string{"workshop\\77.vpk"}) {
		t.Fatalf("替换后成员 = %#v", got)
	}
	if replaced.Members[0].Name != "77.vpk" {
		t.Fatalf("workshop 成员显示名 = %q", replaced.Members[0].Name)
	}
	if _, err := a.SetModStrategyGroupMembers(group.ID, nil); err == nil {
		t.Fatal("替换成空列表应被拒绝")
	}
}

func TestDeleteModStrategyGroupReattachesChildGroups(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	parent, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture parent: %v", err)
	}
	child, err := a.CaptureModStrategyGroup("子组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
	})
	if err != nil {
		t.Fatalf("capture child: %v", err)
	}
	if _, err := a.MoveModStrategyGroup(child.ID, parent.ID); err != nil {
		t.Fatalf("move: %v", err)
	}

	if err := a.DeleteModStrategyGroup(parent.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	reloaded, err := a.findModStrategyGroup(child.ID)
	if err != nil {
		t.Fatalf("子组不应被连带删除: %v", err)
	}
	if reloaded.ParentID != "" {
		t.Fatalf("父组删除后子组应回到顶层，实际 parentId=%q", reloaded.ParentID)
	}
	tree, err := a.ListModStrategyGroupTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].Group.ID != child.ID || tree[0].Depth != 1 {
		t.Fatalf("删除后树应只剩顶层子组: %#v", tree)
	}
	if err := a.DeleteModStrategyGroup(parent.ID); err == nil {
		t.Fatal("重复删除应报错")
	}
}

func TestDeleteModStrategyGroupKeepsAddonListUntouched(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("只删记录", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	before, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteModStrategyGroup(group.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	after, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("删除策略组不应改动 addonlist.txt\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if groups, err := a.ListModStrategyGroups(); err != nil || len(groups) != 0 {
		t.Fatalf("删除后不应还有策略组: %#v err=%v", groups, err)
	}
}
func TestMoveModStrategyGroupMembersMovesBetweenGroups(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	source, err := a.CaptureModStrategyGroup("源组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
		filepath.Join(addonsDir, "c.vpk"),
	})
	if err != nil {
		t.Fatalf("capture source: %v", err)
	}
	target, err := a.CaptureModStrategyGroup("目标组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
	})
	if err != nil {
		t.Fatalf("capture target: %v", err)
	}

	// b.vpk、c.vpk 从源组移到目标组；a.vpk 留在源组（它本来就在两边）。
	result, err := a.MoveModStrategyGroupMembers(source.ID, target.ID, []string{"b.vpk", "c.vpk"})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !reflect.DeepEqual(result.Moved, []string{"b.vpk", "c.vpk"}) {
		t.Fatalf("被移动的键 = %#v", result.Moved)
	}
	if !reflect.DeepEqual(result.RemovedFromSource, []string{"b.vpk", "c.vpk"}) ||
		!reflect.DeepEqual(result.AddedToTarget, []string{"b.vpk", "c.vpk"}) {
		t.Fatalf("移出/加入集合不对: %#v", result)
	}
	if len(result.AlreadyInTarget) != 0 {
		t.Fatalf("本次没有已在目标组的键: %#v", result.AlreadyInTarget)
	}
	if result.SourceRemaining != 1 || result.TargetTotal != 3 {
		t.Fatalf("剩余统计不对: %#v", result)
	}

	reloadedSource, err := a.findModStrategyGroup(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := groupMemberKeys(reloadedSource); !reflect.DeepEqual(got, []string{"a.vpk"}) {
		t.Fatalf("源组应只剩 a.vpk: %#v", got)
	}
	reloadedTarget, err := a.findModStrategyGroup(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := groupMemberKeys(reloadedTarget); !reflect.DeepEqual(got, []string{"a.vpk", "b.vpk", "c.vpk"}) {
		t.Fatalf("目标组应含三个成员: %#v", got)
	}

	// 幂等：再移动一次同一批键——它们已经不在源组，且已在目标组里，等于什么都没做。
	again, err := a.MoveModStrategyGroupMembers(source.ID, target.ID, []string{"b.vpk", "c.vpk"})
	if err != nil {
		t.Fatalf("重复移动应幂等: %v", err)
	}
	if len(again.RemovedFromSource) != 0 || len(again.AddedToTarget) != 0 {
		t.Fatalf("重复移动不应再改动: %#v", again)
	}
	if !reflect.DeepEqual(again.AlreadyInTarget, []string{"b.vpk", "c.vpk"}) {
		t.Fatalf("重复移动应报告已在目标组: %#v", again.AlreadyInTarget)
	}

	// 把 a.vpk 也移走会把源组清空：必须被拒绝，且两边都不变。
	if _, err := a.MoveModStrategyGroupMembers(source.ID, target.ID, []string{"a.vpk"}); err == nil {
		t.Fatal("把源组清空时必须报错")
	}
}

func TestMoveModStrategyGroupMembersRefusesToEmptySource(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	source, err := a.CaptureModStrategyGroup("源组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
	})
	if err != nil {
		t.Fatalf("capture source: %v", err)
	}
	target, err := a.CaptureModStrategyGroup("目标组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture target: %v", err)
	}

	if _, err := a.MoveModStrategyGroupMembers(source.ID, target.ID, []string{"a.vpk"}); err == nil {
		t.Fatal("会把源组清空时必须报错（组至少保留 1 个成员）")
	}
	// 拒绝时两边都不能被改动。
	reloadedSource, err := a.findModStrategyGroup(source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloadedSource.Members) != 1 {
		t.Fatalf("被拒绝的移动不应改动源组: %#v", reloadedSource.Members)
	}
	reloadedTarget, err := a.findModStrategyGroup(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := groupMemberKeys(reloadedTarget); !reflect.DeepEqual(got, []string{"b.vpk"}) {
		t.Fatalf("被拒绝的移动不应改动目标组: %#v", got)
	}
}

func TestMoveModStrategyGroupMembersValidatesArguments(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	group, err := a.CaptureModStrategyGroup("唯一组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	other, err := a.CaptureModStrategyGroup("第二个组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture other: %v", err)
	}

	if _, err := a.MoveModStrategyGroupMembers(group.ID, group.ID, []string{"a.vpk"}); err == nil {
		t.Fatal("源组与目标组相同应被拒绝")
	}
	if _, err := a.MoveModStrategyGroupMembers(group.ID, "no-such-group", []string{"a.vpk"}); err == nil {
		t.Fatal("目标组不存在应被拒绝")
	}
	if _, err := a.MoveModStrategyGroupMembers("no-such-group", other.ID, []string{"a.vpk"}); err == nil {
		t.Fatal("源组不存在应被拒绝")
	}
	if _, err := a.MoveModStrategyGroupMembers(group.ID, other.ID, []string{"   "}); err == nil {
		t.Fatal("没有有效键应被拒绝")
	}
}

func TestMoveModStrategyGroupMembersKeepsAddonListUntouched(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	source, err := a.CaptureModStrategyGroup("源组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatalf("capture source: %v", err)
	}
	target, err := a.CaptureModStrategyGroup("目标组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "c.vpk"),
	})
	if err != nil {
		t.Fatalf("capture target: %v", err)
	}
	before, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.MoveModStrategyGroupMembers(source.ID, target.ID, []string{"a.vpk"}); err != nil {
		t.Fatalf("move: %v", err)
	}
	after, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("移动组成员不应改动 addonlist.txt\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
