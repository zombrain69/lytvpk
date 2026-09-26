package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 应用前预检：能执行 / 不能执行 / 有提醒三种情况都要说清楚。
func TestCheckModStrategyGroupApplyReportsBlockersAndWarnings(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// 一个 disabled 目录里的成员（写 0/1 不生效）
	disabledDir := filepath.Join(addonsDir, "disabled")
	if err := os.MkdirAll(disabledDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestVPK(t, filepath.Join(disabledDir, "parked.vpk"), map[string][]byte{"materials/x.vtf": []byte("x")})

	workshopPath := filepath.Join(addonsDir, "workshop", "123.vpk")
	group, err := a.CaptureModStrategyGroup("混合组", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
		workshopPath,
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	// 再手工塞两个成员：一个在 disabled 目录、一个文件根本不存在。
	if _, err := a.AddModStrategyGroupMembers(group.ID, []string{"parked.vpk", "already-gone.vpk"}); err != nil {
		t.Fatalf("add members: %v", err)
	}

	check, err := a.CheckModStrategyGroupApply(group.ID, ModStrategyGroupApplyOptions{})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !check.Applicable {
		t.Fatalf("有可用成员时应可执行: %#v", check)
	}
	if check.MemberCount != 4 || check.UsableCount != 2 || check.BlockedCount != 1 || check.MissingCount != 1 {
		t.Fatalf("计数不对: %#v", check)
	}
	joined := strings.Join(check.Warnings, " | ")
	if !strings.Contains(joined, "已不在列表里") || !strings.Contains(joined, "disabled 目录") {
		t.Fatalf("两类提醒都要出现: %#v", check.Warnings)
	}
	if strings.Contains(joined, "只有 1 个可用成员") {
		t.Fatalf("2 个可用成员时不应提示单选没有空间: %#v", check.Warnings)
	}

	// 「全关」也应该可执行（它只是把开关写成 0）。
	offCheck, err := a.CheckModStrategyGroupApply(group.ID, ModStrategyGroupApplyOptions{Strategy: modStrategyGroupOff})
	if err != nil || !offCheck.Applicable {
		t.Fatalf("全关策略应可执行: %#v err=%v", offCheck, err)
	}
}

func TestCheckModStrategyGroupApplyBlocksEmptyOrUnloadedGroups(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	parent, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{filepath.Join(addonsDir, "a.vpk")})
	if err != nil {
		t.Fatal(err)
	}
	// 空子组：直接拦下并说明怎么加成员。
	empty, err := a.CreateModStrategyGroupChild(parent.ID, "空子组", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	check, err := a.CheckModStrategyGroupApply(empty.ID, ModStrategyGroupApplyOptions{})
	if err != nil {
		t.Fatalf("check empty: %v", err)
	}
	if check.Applicable {
		t.Fatalf("空组不应可执行: %#v", check)
	}
	if !strings.Contains(check.Reason, "还没有成员") {
		t.Fatalf("原因应说明没有成员: %#v", check.Reason)
	}

	// 成员文件全都不在：也应该拦下。
	ghost, err := a.CaptureModStrategyGroup("幽灵组", "", modStrategyGroupAll, []string{filepath.Join(addonsDir, "a.vpk")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddModStrategyGroupMembers(ghost.ID, []string{"ghost-a.vpk", "ghost-b.vpk"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.RemoveModStrategyGroupMembers(ghost.ID, []string{"a.vpk"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	ghostCheck, err := a.CheckModStrategyGroupApply(ghost.ID, ModStrategyGroupApplyOptions{})
	if err != nil {
		t.Fatalf("check ghost: %v", err)
	}
	if ghostCheck.Applicable || !strings.Contains(ghostCheck.Reason, "都不在当前列表里") {
		t.Fatalf("成员全丢时应拦下并说明: %#v", ghostCheck)
	}
}
