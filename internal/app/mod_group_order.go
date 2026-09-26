package app

import (
	"fmt"
	"strings"
	"time"
)

// ReorderModStrategyGroups 按传入的 ID 顺序重写 groups.json 里的排列顺序。
//
// 为什么需要单独一个接口：树视图（拖放排序）与「按分组筛选」的默认排列
// 都直接依赖 store.Groups 的数组顺序，而 MoveModStrategyGroup 只负责改上级，
// 不负责同级之间的先后。接口要求传入的 ID 集合与现有组完全一致，
// 既不能漏组也不能加组，避免"拖一下少一个组"这类事故。
func (a *App) ReorderModStrategyGroups(orderedIDs []string) ([]ModStrategyGroup, error) {
	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()

	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("排序列表不能为空")
	}

	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return nil, err
	}
	if len(store.Groups) == 0 {
		return nil, fmt.Errorf("还没有策略组，无法排序")
	}

	byID := make(map[string]ModStrategyGroup, len(store.Groups))
	for _, group := range store.Groups {
		byID[group.ID] = group
	}

	if len(orderedIDs) != len(store.Groups) {
		return nil, fmt.Errorf("排序列表包含 %d 个组，现有 %d 个；请提交全部策略组的 ID", len(orderedIDs), len(store.Groups))
	}

	reordered := make([]ModStrategyGroup, 0, len(orderedIDs))
	seen := make(map[string]bool, len(orderedIDs))
	for _, rawID := range orderedIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, fmt.Errorf("排序列表里存在空的组 ID")
		}
		if seen[id] {
			return nil, fmt.Errorf("排序列表里存在重复的组 ID: %s", id)
		}
		group, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("策略组不存在: %s", id)
		}
		seen[id] = true
		reordered = append(reordered, group)
	}

	if sameGroupOrder(store.Groups, reordered) {
		// 顺序没变时直接返回，不写盘：避免白白触发本地记录备份轮转。
		return append([]ModStrategyGroup(nil), store.Groups...), nil
	}

	now := time.Now().Format(time.RFC3339)
	originalIndex := make(map[string]int, len(store.Groups))
	for index, group := range store.Groups {
		originalIndex[group.ID] = index
	}
	for index := range reordered {
		if originalIndex[reordered[index].ID] != index {
			reordered[index].UpdatedAt = now
		}
	}

	store.Groups = reordered
	if err := a.writeModStrategyGroupStore(store); err != nil {
		return nil, err
	}
	return append([]ModStrategyGroup(nil), reordered...), nil
}

func sameGroupOrder(left []ModStrategyGroup, right []ModStrategyGroup) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID {
			return false
		}
	}
	return true
}
