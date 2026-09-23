package app

import (
	"path/filepath"
	"reflect"
	"testing"
)

// 组管理的批量操作：一次写盘完成"批量删除 / 批量开关自动联动 / 批量设置或清除权重"。

func batchTestGroups(t *testing.T, a *App, addonsDir string, names ...string) []ModStrategyGroup {
	t.Helper()
	groups := make([]ModStrategyGroup, 0, len(names))
	for _, name := range names {
		group, err := a.CaptureModStrategyGroup(name, "", modStrategyGroupAll, []string{
			filepath.Join(addonsDir, "a.vpk"),
			filepath.Join(addonsDir, "b.vpk"),
		})
		if err != nil {
			t.Fatalf("capture %s: %v", name, err)
		}
		groups = append(groups, group)
	}
	return groups
}

func TestBatchDeleteStrategyGroupsReparentsChildrenAndWritesOnce(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	groups := batchTestGroups(t, a, addonsDir, "组一", "组二", "组三")
	// 组三挂在组一下面；批量删掉组一、组二后，组三必须回到顶层。
	if _, err := a.MoveModStrategyGroup(groups[2].ID, groups[0].ID); err != nil {
		t.Fatalf("move: %v", err)
	}

	result, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID, groups[1].ID, groups[0].ID, "  "},
		modStrategyGroupBatchDelete, nil,
	)
	if err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if !reflect.DeepEqual(result.Deleted, []string{groups[0].ID, groups[1].ID}) {
		t.Fatalf("删除列表不对（应去重保序）: %#v", result.Deleted)
	}
	if len(result.Skipped) != 0 || result.Remaining != 1 {
		t.Fatalf("剩余统计不对: %#v", result)
	}
	if !reflect.DeepEqual(result.DetachedChildren, []string{groups[2].ID}) {
		t.Fatalf("应报告被提升到顶层的子组: %#v", result.DetachedChildren)
	}

	remaining, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != groups[2].ID {
		t.Fatalf("只剩组三: %#v", remaining)
	}
	if remaining[0].ParentID != "" {
		t.Fatalf("被删父组的下级应回到顶层: %#v", remaining[0].ParentID)
	}
}

func TestBatchUpdateStrategyGroupsEnforcement(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	groups := batchTestGroups(t, a, addonsDir, "甲", "乙", "丙")

	on, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID, groups[1].ID}, modStrategyGroupBatchEnforceOn, nil)
	if err != nil {
		t.Fatalf("enforce on: %v", err)
	}
	if len(on.Updated) != 2 || on.Remaining != 3 {
		t.Fatalf("批量开启自动联动结果不对: %#v", on)
	}
	reloaded, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	for index := range reloaded {
		want := index < 2
		if reloaded[index].Enforce != want {
			t.Fatalf("自动联动状态不对: %#v", reloaded[index])
		}
	}

	off, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID, groups[1].ID, groups[2].ID}, modStrategyGroupBatchEnforceOff, nil)
	if err != nil {
		t.Fatalf("enforce off: %v", err)
	}
	if len(off.Updated) != 3 {
		t.Fatalf("批量关闭应覆盖三组: %#v", off)
	}
	reloaded, err = a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range reloaded {
		if group.Enforce {
			t.Fatalf("全部应为关闭: %#v", group)
		}
	}
}

func TestBatchUpdateStrategyGroupsTier(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	groups := batchTestGroups(t, a, addonsDir, "甲", "乙")

	tier := -7
	set, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID, groups[1].ID}, modStrategyGroupBatchSetTier, &tier)
	if err != nil {
		t.Fatalf("set tier: %v", err)
	}
	if set.Tier == nil || *set.Tier != -7 || len(set.Updated) != 2 {
		t.Fatalf("批量设置权重结果不对: %#v", set)
	}
	reloaded, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range reloaded {
		if group.Tier == nil || *group.Tier != -7 {
			t.Fatalf("权重应写进每一组: %#v", group)
		}
	}

	cleared, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID}, modStrategyGroupBatchClearTier, nil)
	if err != nil {
		t.Fatalf("clear tier: %v", err)
	}
	if cleared.Tier != nil {
		t.Fatalf("清除权重后不应带回 tier: %#v", cleared.Tier)
	}
	reloaded, err = a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded[0].Tier != nil || reloaded[1].Tier == nil {
		t.Fatalf("只应清除第一组的权重: %#v", reloaded)
	}

	if _, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID}, modStrategyGroupBatchSetTier, nil); err == nil {
		t.Fatal("设置权重时必须提供 tier")
	}
}

func TestBatchUpdateStrategyGroupsRejectsBadInput(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	groups := batchTestGroups(t, a, addonsDir, "甲")

	if _, err := a.BatchUpdateModStrategyGroups(nil, modStrategyGroupBatchDelete, nil); err == nil {
		t.Fatal("没有选中任何组时应报错")
	}
	if _, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID}, "explode", nil); err == nil {
		t.Fatal("未知动作应报错")
	}
	result, err := a.BatchUpdateModStrategyGroups(
		[]string{"no-such-group"}, modStrategyGroupBatchDelete, nil)
	if err != nil {
		t.Fatalf("未知 ID 应跳过而不是报错: %v", err)
	}
	if len(result.Deleted) != 0 || !reflect.DeepEqual(result.Skipped, []string{"no-such-group"}) {
		t.Fatalf("未知 ID 应出现在 skipped 里: %#v", result)
	}
	if result.Remaining != 1 {
		t.Fatalf("没有任何删除时组数不应变化: %#v", result)
	}
}

func TestBatchDeleteStrategyGroupsKeepsAddonListUntouched(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	groups := batchTestGroups(t, a, addonsDir, "甲", "乙")
	before, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.BatchUpdateModStrategyGroups(
		[]string{groups[0].ID, groups[1].ID}, modStrategyGroupBatchDelete, nil); err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	after, err := a.readAddonListContentForTest()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("批量删除策略组不应改动 addonlist.txt\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
