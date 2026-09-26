package app

import (
	"os"
	"path/filepath"
	"testing"
)

func newGroupOrderApp(t *testing.T) *App {
	t.Helper()
	a, _ := newPriorityTestApp(t)
	return a
}

// groupOrderAppWithGroups 建好夹具并返回 app、addons 目录与三个已建组。
func groupOrderAppWithGroups(t *testing.T) (*App, string, ModStrategyGroup, ModStrategyGroup, ModStrategyGroup) {
	t.Helper()
	a, addonsDir := newPriorityTestApp(t)

	capture := func(name string, file string, strategy string) ModStrategyGroup {
		group, err := a.CaptureModStrategyGroup(name, "", strategy, []string{filepath.Join(addonsDir, file)})
		if err != nil {
			t.Fatalf("创建策略组 %s 失败: %v", name, err)
		}
		return group
	}

	first := capture("甲", "a.vpk", modStrategyGroupSingle)
	second := capture("乙", "b.vpk", modStrategyGroupSingle)
	third := capture("丙", "c.vpk", modStrategyGroupSingle)
	return a, addonsDir, first, second, third
}

func groupOrderIDs(t *testing.T, a *App) []string {
	t.Helper()
	groups, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatalf("读取策略组失败: %v", err)
	}
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

// TestReorderModStrategyGroupsPersistsOrder 覆盖拖放排序落盘。
func TestReorderModStrategyGroupsPersistsOrder(t *testing.T) {
	a, _, first, second, third := groupOrderAppWithGroups(t)

	want := []string{third.ID, first.ID, second.ID}
	if _, err := a.ReorderModStrategyGroups(want); err != nil {
		t.Fatalf("重排失败: %v", err)
	}
	if got := groupOrderIDs(t, a); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("重排后的顺序 = %v, 期望 %v", got, want)
	}

	// 重新加载（模拟重启）后顺序必须保持。
	reloaded := &App{rootDir: a.rootDir, configDir: a.configDir}
	if got := groupOrderIDs(t, reloaded); got[0] != want[0] {
		t.Fatalf("重启后顺序丢失: %v", got)
	}
}

// TestReorderModStrategyGroupsKeepsMembersAndTiers 确认排序只动顺序。
func TestReorderModStrategyGroupsKeepsMembersAndTiers(t *testing.T) {
	a, addonsDir, first, second, third := groupOrderAppWithGroups(t)
	if _, err := a.AddModStrategyGroupMembers(first.ID, []string{
		filepath.Join(addonsDir, "b.vpk"),
	}); err != nil {
		t.Fatalf("给甲追加成员失败: %v", err)
	}
	tier := -3
	if _, err := a.SetModStrategyGroupTier(first.ID, &tier); err != nil {
		t.Fatalf("设置权重失败: %v", err)
	}

	if _, err := a.ReorderModStrategyGroups([]string{second.ID, first.ID, third.ID}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}

	groups, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 {
		t.Fatalf("组数 = %d", len(groups))
	}
	if groups[0].ID != second.ID || groups[2].ID != third.ID {
		t.Fatalf("重排结果不是期望的全量顺序: %v", []string{groups[0].ID, groups[1].ID, groups[2].ID})
	}
	moved := groups[1]
	if moved.ID != first.ID {
		t.Fatalf("第二个组应为甲，实际 %s", moved.Name)
	}
	if len(moved.Members) != 2 {
		t.Fatalf("重排后内容被改动: %+v", moved)
	}
	if moved.Tier == nil || *moved.Tier != -3 {
		t.Fatalf("重排后权重丢失: %+v", moved.Tier)
	}
}

// TestReorderModStrategyGroupsRejectsBadInput 保证排序接口不会丢组或加组。
func TestReorderModStrategyGroupsRejectsBadInput(t *testing.T) {
	a, _, first, second, _ := groupOrderAppWithGroups(t)
	before := groupOrderIDs(t, a)
	path := a.ensureGroupsPath()
	beforeRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string][]string{
		"空列表":     {},
		"缺一个组":    {first.ID},
		"含未知 ID":  {first.ID, second.ID, "no-such-group"},
		"重复 ID":   {first.ID, first.ID},
		"含空 ID":   {first.ID, "", second.ID},
	}
	for name, ids := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := a.ReorderModStrategyGroups(ids); err == nil {
				t.Fatalf("非法输入应报错: %v", ids)
			}
			if got := groupOrderIDs(t, a); len(got) != len(before) || got[0] != before[0] {
				t.Fatalf("报错后顺序不应变化: %v", got)
			}
		})
	}

	afterRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterRaw) != string(beforeRaw) {
		t.Fatal("全部非法输入都不应改动 groups.json")
	}
}

// TestReorderModStrategyGroupsSkipsIdenticalOrder 顺序没变时不应重复写盘。
func TestReorderModStrategyGroupsSkipsIdenticalOrder(t *testing.T) {
	a, _, first, second, third := groupOrderAppWithGroups(t)
	path := a.ensureGroupsPath()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReorderModStrategyGroups([]string{first.ID, second.ID, third.ID}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(again) {
		t.Fatal("顺序不变时不应重写 groups.json（否则会白白撑大备份轮转）")
	}
}
