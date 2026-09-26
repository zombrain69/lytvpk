package app

import (
	"path/filepath"
	"testing"
)

// 用户要的快捷入口：在某个策略组下面直接新建子组（子组成员可以稍后再加）。
func TestCreateModStrategyGroupChildCreatesNestedGroup(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	bPath := filepath.Join(addonsDir, "b.vpk")

	parent, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{bPath})
	if err != nil {
		t.Fatalf("capture parent: %v", err)
	}

	// ① 带成员建子组：父组自动成为上级。
	child, err := a.CreateModStrategyGroupChild(parent.ID, "子组", modStrategyGroupSingle, []string{bPath})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("子组应挂在父组下: %#v", child)
	}
	if len(child.Members) != 1 || child.Members[0].Key != "b.vpk" {
		t.Fatalf("成员应被带上: %#v", child.Members)
	}

	// ② 不带成员也允许（快捷建组后可以用选择器再加成员）。
	empty, err := a.CreateModStrategyGroupChild(parent.ID, "空子组", "", nil)
	if err != nil {
		t.Fatalf("create empty child: %v", err)
	}
	if len(empty.Members) != 0 || empty.ParentID != parent.ID {
		t.Fatalf("空子组也应挂在父组下: %#v", empty)
	}
	if empty.Strategy != modStrategyGroupSingle {
		t.Fatalf("未指定策略时应回落到互斥单选: %#v", empty.Strategy)
	}

	tree, err := a.ListModStrategyGroupTree()
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	var parentNode *ModStrategyGroupTreeNode
	for index := range tree {
		if tree[index].Group.ID == parent.ID {
			parentNode = &tree[index]
		}
	}
	if parentNode == nil || len(parentNode.Children) != 2 {
		t.Fatalf("父组下应有 2 个子组: %#v", tree)
	}
	for _, node := range parentNode.Children {
		if node.Depth != 2 {
			t.Fatalf("子组深度应为 2: %#v", node)
		}
	}
}

func TestCreateModStrategyGroupChildRejectsBadInput(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	bPath := filepath.Join(addonsDir, "b.vpk")
	parent, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{bPath})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := a.CreateModStrategyGroupChild("", "子组", "", nil); err == nil {
		t.Fatal("缺少父组 ID 应报错")
	}
	if _, err := a.CreateModStrategyGroupChild("不存在的组", "子组", "", nil); err == nil {
		t.Fatal("父组不存在应报错")
	}
	if _, err := a.CreateModStrategyGroupChild(parent.ID, "   ", "", nil); err == nil {
		t.Fatal("空名称应报错")
	}

	// 深度上限 4 层：父(1) → 子(2) → 孙(3) → 曾孙(4) 之后不能再建。
	current := parent.ID
	for depth := 2; depth <= 4; depth++ {
		child, err := a.CreateModStrategyGroupChild(current, "第"+string(rune('0'+depth))+"层", "", nil)
		if err != nil {
			t.Fatalf("第 %d 层应能创建: %v", depth, err)
		}
		current = child.ID
	}
	if _, err := a.CreateModStrategyGroupChild(current, "第5层", "", nil); err == nil {
		t.Fatal("超过 4 层应被拒绝")
	}
}
