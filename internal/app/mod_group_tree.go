package app

import (
	"fmt"
	"strings"
	"time"
)

// 策略组树形层级（数据层）
//
// 设计约束：
//   - 只是给策略组增加一条可选的"上级分组"关系，**默认没有上级**，
//     因此渲染结果与之前的扁平列表完全一致（扁平视图仍是默认视图）；
//   - 组权重（Tier）已经参与统一优先级模型，树形层级不再额外影响优先级，
//     避免出现"层级 + 权重"两套互相冲突的优先级来源；
//   - 关系必须无环、深度有上限，异常数据在读取时就退化到顶层，
//     不让坏数据把界面变成死循环。

// modStrategyGroupMaxDepth 限制层级深度（根为第 1 层）。
const modStrategyGroupMaxDepth = 4

// ModStrategyGroupTreeNode 是树形视图里的一个节点。
type ModStrategyGroupTreeNode struct {
	Group    ModStrategyGroup           `json:"group"`
	Depth    int                        `json:"depth"`
	Children []ModStrategyGroupTreeNode `json:"children"`
}

// buildModStrategyGroupTree 把扁平列表转成树：
//   - 没有上级、或上级不存在（被删除）的组都作为根节点；
//   - 顺序保持传入顺序（保存顺序），保证"默认扁平视图"的前后关系不变；
//   - 检测到环或超深的节点会被降级为根节点，避免界面递归失控。
func buildModStrategyGroupTree(groups []ModStrategyGroup) []ModStrategyGroupTreeNode {
	byID := make(map[string]ModStrategyGroup, len(groups))
	for _, group := range groups {
		if strings.TrimSpace(group.ID) == "" {
			continue
		}
		byID[group.ID] = group
	}

	childrenByParent := make(map[string][]ModStrategyGroup)
	roots := make([]ModStrategyGroup, 0, len(groups))
	for _, group := range groups {
		parentID := strings.TrimSpace(group.ParentID)
		if parentID == "" {
			roots = append(roots, group)
			continue
		}
		if _, exists := byID[parentID]; !exists {
			// 上级已被删除：降级为根，避免整棵子树消失。
			roots = append(roots, group)
			continue
		}
		if groupDepthInChain(byID, group.ID) > modStrategyGroupMaxDepth {
			roots = append(roots, group)
			continue
		}
		childrenByParent[parentID] = append(childrenByParent[parentID], group)
	}

	var build func(group ModStrategyGroup, depth int) ModStrategyGroupTreeNode
	build = func(group ModStrategyGroup, depth int) ModStrategyGroupTreeNode {
		node := ModStrategyGroupTreeNode{Group: group, Depth: depth}
		children := childrenByParent[group.ID]
		if depth >= modStrategyGroupMaxDepth {
			return node
		}
		for _, child := range children {
			if child.ID == group.ID {
				continue
			}
			node.Children = append(node.Children, build(child, depth+1))
		}
		return node
	}

	tree := make([]ModStrategyGroupTreeNode, 0, len(roots))
	for _, root := range roots {
		tree = append(tree, build(root, 1))
	}
	return tree
}

// groupDepthInChain 返回该组沿"上级链"到根的深度；链上有环时返回一个超过上限的值。
func groupDepthInChain(byID map[string]ModStrategyGroup, id string) int {
	depth := 0
	cursor := id
	seen := make(map[string]struct{}, len(byID))
	for cursor != "" {
		if _, exists := seen[cursor]; exists {
			return modStrategyGroupMaxDepth + 1
		}
		seen[cursor] = struct{}{}
		group, exists := byID[cursor]
		if !exists {
			break
		}
		depth++
		cursor = strings.TrimSpace(group.ParentID)
	}
	return depth
}

// validateModStrategyGroupParent 校验一次"设置上级分组"是否合法。
func validateModStrategyGroupParent(groups []ModStrategyGroup, id string, parentID string) error {
	id = strings.TrimSpace(id)
	parentID = strings.TrimSpace(parentID)
	if id == "" {
		return fmt.Errorf("缺少策略组 ID")
	}
	if parentID == "" {
		return nil
	}
	if parentID == id {
		return fmt.Errorf("策略组不能把自己设为上级分组")
	}

	byID := make(map[string]ModStrategyGroup, len(groups))
	for _, group := range groups {
		byID[group.ID] = group
	}
	if _, exists := byID[parentID]; !exists {
		return fmt.Errorf("上级分组不存在: %s", parentID)
	}
	if _, exists := byID[id]; !exists {
		return fmt.Errorf("策略组不存在: %s", id)
	}

	// 从 parent 往上走：如果能走回自己，说明会形成环。
	cursor := parentID
	steps := 0
	for cursor != "" {
		if cursor == id {
			return fmt.Errorf("不能把策略组移动到自己的下级里（会形成环）")
		}
		group, exists := byID[cursor]
		if !exists {
			break
		}
		steps++
		if steps > modStrategyGroupMaxDepth {
			return fmt.Errorf("分组层级最多 %d 层", modStrategyGroupMaxDepth)
		}
		cursor = strings.TrimSpace(group.ParentID)
	}

	// 目标层级深度 = 上级链深度 + 1（当前节点）+ 自身子树深度。
	depth := steps + 1
	if depth > modStrategyGroupMaxDepth {
		return fmt.Errorf("分组层级最多 %d 层", modStrategyGroupMaxDepth)
	}
	if deepest := deepestDescendantDepth(groups, id); depth+deepest > modStrategyGroupMaxDepth {
		return fmt.Errorf("分组层级最多 %d 层", modStrategyGroupMaxDepth)
	}
	return nil
}

// deepestDescendantDepth 返回以 id 为根的子树最大深度（叶子为 0）。
func deepestDescendantDepth(groups []ModStrategyGroup, id string) int {
	children := make([]ModStrategyGroup, 0)
	for _, group := range groups {
		if strings.TrimSpace(group.ParentID) == id {
			children = append(children, group)
		}
	}
	deepest := 0
	for _, child := range children {
		if depth := 1 + deepestDescendantDepth(groups, child.ID); depth > deepest {
			deepest = depth
		}
	}
	return deepest
}

// ListModStrategyGroupTree 返回策略组树；没有任何上级分组时等价于扁平列表。
func (a *App) ListModStrategyGroupTree() ([]ModStrategyGroupTreeNode, error) {
	groups, err := a.ListModStrategyGroups()
	if err != nil {
		return nil, err
	}
	return buildModStrategyGroupTree(groups), nil
}

// MoveModStrategyGroup 设置（或清除）某个策略组的上级分组。
// parentID 为空表示移动到顶层。
func (a *App) MoveModStrategyGroup(id string, parentID string) (ModStrategyGroup, error) {
	id = strings.TrimSpace(id)
	parentID = strings.TrimSpace(parentID)
	if id == "" {
		return ModStrategyGroup{}, fmt.Errorf("缺少策略组 ID")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	if err := validateModStrategyGroupParent(store.Groups, id, parentID); err != nil {
		return ModStrategyGroup{}, err
	}
	for index := range store.Groups {
		if store.Groups[index].ID != id {
			continue
		}
		store.Groups[index].ParentID = parentID
		store.Groups[index].UpdatedAt = time.Now().Format(time.RFC3339)
		if err := a.writeModStrategyGroupStore(store); err != nil {
			return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
		}
		return store.Groups[index], nil
	}
	return ModStrategyGroup{}, fmt.Errorf("策略组不存在: %s", id)
}
