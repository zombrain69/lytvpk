package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestUniqueModStrategyGroupName(t *testing.T) {
	existing := []string{"角色包", "武器包", "Role Pack"}
	cases := []struct {
		desired string
		want    string
	}{
		{"新组", "新组"},
		{"角色包", "角色包 (2)"},
		{" 角色包 ", "角色包 (2)"}, // 去空白后同名
		{"role pack", "role pack (2)"},
		{"", "未命名策略组"},
	}
	for _, tc := range cases {
		if got := uniqueModStrategyGroupName(existing, tc.desired); got != tc.want {
			t.Fatalf("unique(%q) = %q, 期望 %q", tc.desired, got, tc.want)
		}
	}

	// 连续撞名时依次加序号。
	many := []string{"组", "组 (2)", "组 (3)"}
	if got := uniqueModStrategyGroupName(many, "组"); got != "组 (4)" {
		t.Fatalf("连续撞名 = %q, 期望 组 (4)", got)
	}

	// 名字很长时也不能超过上限。
	long := strings.Repeat("长", modStrategyGroupNameMaxRunes)
	got := uniqueModStrategyGroupName([]string{long}, long)
	if len([]rune(got)) > modStrategyGroupNameMaxRunes {
		t.Fatalf("自动改名后超长：%d 字", len([]rune(got)))
	}
	if got == long {
		t.Fatal("撞名时必须换一个名字")
	}
}

func TestFindModStrategyGroupNameConflict(t *testing.T) {
	groups := []ModStrategyGroup{
		{ID: "a", Name: "角色包"},
		{ID: "b", Name: "Weapon"},
	}
	if got := findModStrategyGroupNameConflict(groups, "角色包", ""); got != "角色包" {
		t.Fatalf("应检出重名，实际 %q", got)
	}
	if got := findModStrategyGroupNameConflict(groups, "weapon", "c"); got != "Weapon" {
		t.Fatalf("应大小写不敏感地检出重名，实际 %q", got)
	}
	// 改成自己原来的名字不算冲突。
	if got := findModStrategyGroupNameConflict(groups, "角色包", "a"); got != "" {
		t.Fatalf("改成自己的名字不应报冲突，实际 %q", got)
	}
	if got := findModStrategyGroupNameConflict(groups, "新名字", ""); got != "" {
		t.Fatalf("没有冲突时应返回空串，实际 %q", got)
	}
}

// 端到端：新建同名组自动加序号；改名撞名被拒绝且不改动原数据。
func TestModStrategyGroupNamesEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	first, err := a.CaptureModStrategyGroup("角色包", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "a.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "角色包" {
		t.Fatalf("第一个组名 = %q", first.Name)
	}

	second, err := a.CaptureModStrategyGroup("角色包", "", modStrategyGroupSingle, []string{
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Name != "角色包 (2)" {
		t.Fatalf("同名组应自动加序号，实际 %q", second.Name)
	}

	child, err := a.CreateModStrategyGroupChild(first.ID, "角色包", modStrategyGroupSingle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if child.Name != "角色包 (3)" {
		t.Fatalf("子组同样应自动加序号，实际 %q", child.Name)
	}

	// 改名撞名：报错，并且两个组名都不变。
	if _, err := a.RenameModStrategyGroup(second.ID, "角色包", ""); err == nil {
		t.Fatal("改名撞名应报错")
	} else if !strings.Contains(err.Error(), "同名") {
		t.Fatalf("报错应说明撞名原因，实际 %v", err)
	}
	groups, err := a.ListModStrategyGroups()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, group := range groups {
		names[group.ID] = group.Name
	}
	if names[first.ID] != "角色包" || names[second.ID] != "角色包 (2)" {
		t.Fatalf("改名失败后不应改动任何组名：%+v", names)
	}

	// 改成自己原来的名字（含大小写变化）允许通过。
	if _, err := a.RenameModStrategyGroup(second.ID, "角色包 (2)", ""); err != nil {
		t.Fatalf("改成自己的名字应允许: %v", err)
	}
}
