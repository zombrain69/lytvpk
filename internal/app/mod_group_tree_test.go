package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func testGroup(id string, parentID string) ModStrategyGroup {
	return ModStrategyGroup{ID: id, Name: "组-" + id, Strategy: modStrategyGroupAll, ParentID: parentID}
}

// TestBuildModStrategyGroupTreeKeepsFlatOrderWithoutParents 守住"扁平视图仍是默认"。
func TestBuildModStrategyGroupTreeKeepsFlatOrderWithoutParents(t *testing.T) {
	groups := []ModStrategyGroup{testGroup("a", ""), testGroup("b", ""), testGroup("c", "")}
	tree := buildModStrategyGroupTree(groups)
	if len(tree) != 3 {
		t.Fatalf("无上级分组时应等价于扁平列表: %#v", tree)
	}
	for index, node := range tree {
		if node.Depth != 1 || len(node.Children) != 0 {
			t.Fatalf("节点 %d 不应有层级: %#v", index, node)
		}
	}
	if tree[0].Group.ID != "a" || tree[1].Group.ID != "b" || tree[2].Group.ID != "c" {
		t.Fatalf("顺序应与保存顺序一致: %#v", tree)
	}
}

func TestBuildModStrategyGroupTreeNestsChildren(t *testing.T) {
	groups := []ModStrategyGroup{
		testGroup("parent", ""),
		testGroup("child", "parent"),
		testGroup("grandchild", "child"),
		testGroup("other", ""),
	}
	tree := buildModStrategyGroupTree(groups)
	if len(tree) != 2 {
		t.Fatalf("应有 2 个根节点: %#v", tree)
	}
	if tree[0].Group.ID != "parent" || len(tree[0].Children) != 1 {
		t.Fatalf("父节点结构 = %#v", tree[0])
	}
	if tree[0].Children[0].Group.ID != "child" || tree[0].Children[0].Depth != 2 {
		t.Fatalf("子节点结构 = %#v", tree[0].Children[0])
	}
	if len(tree[0].Children[0].Children) != 1 || tree[0].Children[0].Children[0].Group.ID != "grandchild" {
		t.Fatalf("孙节点结构 = %#v", tree[0].Children[0].Children)
	}
}

// TestBuildModStrategyGroupTreeDegradesBadData 覆盖坏数据降级（不递归、不丢子树）。
func TestBuildModStrategyGroupTreeDegradesBadData(t *testing.T) {
	// 上级已被删除。
	orphan := buildModStrategyGroupTree([]ModStrategyGroup{testGroup("a", "missing")})
	if len(orphan) != 1 || orphan[0].Depth != 1 {
		t.Fatalf("上级不存在时应降级为根节点: %#v", orphan)
	}

	// 环：a -> b -> a，两节点都必须出现，且不能无限递归。
	cycle := buildModStrategyGroupTree([]ModStrategyGroup{testGroup("a", "b"), testGroup("b", "a")})
	if len(cycle) == 0 {
		t.Fatalf("存在环时仍应返回节点: %#v", cycle)
	}
	total := 0
	var walk func(nodes []ModStrategyGroupTreeNode)
	walk = func(nodes []ModStrategyGroupTreeNode) {
		for _, node := range nodes {
			total++
			if node.Depth > modStrategyGroupMaxDepth {
				t.Fatalf("深度不能超过上限: %#v", node)
			}
			walk(node.Children)
		}
	}
	walk(cycle)
	if total > 4 {
		t.Fatalf("环应被限制，不应展开过多节点: %d", total)
	}
}

func TestValidateModStrategyGroupParent(t *testing.T) {
	groups := []ModStrategyGroup{
		testGroup("a", ""),
		testGroup("b", "a"),
		testGroup("c", ""),
	}
	if err := validateModStrategyGroupParent(groups, "c", ""); err != nil {
		t.Fatalf("移动到顶层应合法: %v", err)
	}
	if err := validateModStrategyGroupParent(groups, "c", "b"); err != nil {
		t.Fatalf("移动到已有层级应合法: %v", err)
	}
	if err := validateModStrategyGroupParent(groups, "a", "a"); err == nil {
		t.Fatal("自己不能作为自己的上级")
	}
	if err := validateModStrategyGroupParent(groups, "a", "b"); err == nil {
		t.Fatal("把父节点移到子节点下应被拒绝（成环）")
	}
	if err := validateModStrategyGroupParent(groups, "c", "missing"); err == nil {
		t.Fatal("上级不存在应被拒绝")
	}
}

func TestValidateModStrategyGroupParentEnforcesDepthLimit(t *testing.T) {
	groups := []ModStrategyGroup{
		testGroup("1", ""),
		testGroup("2", "1"),
		testGroup("3", "2"),
		testGroup("4", "3"),
		testGroup("5", ""),
	}
	// 把 5 挂到 4 下面会形成第 5 层，超过上限。
	err := validateModStrategyGroupParent(groups, "5", "4")
	if err == nil {
		t.Fatal("超过层级上限应被拒绝")
	}
	if !strings.Contains(err.Error(), "最多") {
		t.Fatalf("错误信息应说明层级上限: %v", err)
	}
}

// TestMoveModStrategyGroupPersists 覆盖 API 层的持久化与清除。
func TestMoveModStrategyGroupPersists(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	parent, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{filepath.Join(addonsDir, "a.vpk")})
	if err != nil {
		t.Fatal(err)
	}
	child, err := a.CaptureModStrategyGroup("子组", "", modStrategyGroupAll, []string{filepath.Join(addonsDir, "b.vpk")})
	if err != nil {
		t.Fatal(err)
	}

	moved, err := a.MoveModStrategyGroup(child.ID, parent.ID)
	if err != nil {
		t.Fatalf("move group: %v", err)
	}
	if moved.ParentID != parent.ID {
		t.Fatalf("上级分组未保存: %#v", moved)
	}

	tree, err := a.ListModStrategyGroupTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || tree[0].Group.ID != parent.ID || len(tree[0].Children) != 1 {
		t.Fatalf("树结构 = %#v", tree)
	}

	// 回到顶层。
	if _, err := a.MoveModStrategyGroup(child.ID, ""); err != nil {
		t.Fatal(err)
	}
	tree, err = a.ListModStrategyGroupTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 {
		t.Fatalf("清除上级后应回到扁平结构: %#v", tree)
	}

	if _, err := a.MoveModStrategyGroup("missing", parent.ID); err == nil {
		t.Fatal("不存在的策略组应报错")
	}
}
