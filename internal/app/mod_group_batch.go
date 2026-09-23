package app

import (
	"fmt"
	"strings"
	"time"
)

// 策略组的批量管理：一次写盘完成"批量删除 / 批量开关自动联动 / 批量设置或清除权重"。
//
// 只写 groups.json：
//   - 不改动任何 Mod 文件，也不改 addonlist.txt；
//   - 批量删除时，被删组的下级分组会被提升到顶层（与单个删除一致），不会被连带删除。

const (
	modStrategyGroupBatchDelete     = "delete"
	modStrategyGroupBatchEnforceOn  = "enforce_on"
	modStrategyGroupBatchEnforceOff = "enforce_off"
	modStrategyGroupBatchSetTier    = "set_tier"
	modStrategyGroupBatchClearTier  = "clear_tier"
)

// ModStrategyGroupBatchResult 汇总一次批量操作的结果，供界面提示。
type ModStrategyGroupBatchResult struct {
	Action  string   `json:"action"`
	Updated []string `json:"updated"`
	Deleted []string `json:"deleted"`
	// Skipped 是传入但不存在的组 ID（不报错，只报告）。
	Skipped []string `json:"skipped"`
	// DetachedChildren 是批量删除时被提升到顶层的下级分组。
	DetachedChildren []string `json:"detachedChildren,omitempty"`
	Remaining        int      `json:"remaining"`
	Tier             *int     `json:"tier,omitempty"`
}

// BatchUpdateModStrategyGroups 对选中的策略组执行同一个动作。
//
// action 取值：delete / enforce_on / enforce_off / set_tier（需带 tier）/ clear_tier。
// 传入的 ID 会去重并保持顺序；不存在的 ID 记进 Skipped；没有任何组被改动时不写盘。
func (a *App) BatchUpdateModStrategyGroups(ids []string, action string, tier *int) (ModStrategyGroupBatchResult, error) {
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	switch normalizedAction {
	case modStrategyGroupBatchDelete,
		modStrategyGroupBatchEnforceOn,
		modStrategyGroupBatchEnforceOff,
		modStrategyGroupBatchSetTier,
		modStrategyGroupBatchClearTier:
	default:
		return ModStrategyGroupBatchResult{}, fmt.Errorf("不支持的批量操作: %s", action)
	}
	if normalizedAction == modStrategyGroupBatchSetTier && tier == nil {
		return ModStrategyGroupBatchResult{}, fmt.Errorf("批量设置权重时必须提供权重值")
	}

	targets := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return ModStrategyGroupBatchResult{}, fmt.Errorf("请先选择要操作的策略组")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroupBatchResult{}, err
	}

	targetSet := make(map[string]struct{}, len(targets))
	for _, id := range targets {
		targetSet[id] = struct{}{}
	}
	result := ModStrategyGroupBatchResult{
		Action:  normalizedAction,
		Updated: []string{},
		Deleted: []string{},
		Skipped: []string{},
	}
	if normalizedAction == modStrategyGroupBatchSetTier {
		value := *tier
		result.Tier = &value
	}

	now := time.Now().Format(time.RFC3339)
	changed := false

	if normalizedAction == modStrategyGroupBatchDelete {
		remaining := make([]ModStrategyGroup, 0, len(store.Groups))
		matched := make(map[string]struct{}, len(targets))
		for _, group := range store.Groups {
			if _, drop := targetSet[group.ID]; drop {
				matched[group.ID] = struct{}{}
				result.Deleted = append(result.Deleted, group.ID)
				changed = true
				continue
			}
			remaining = append(remaining, group)
		}
		// 被删组的下级分组提升到顶层（与单个删除保持同样的收尾语义）。
		for index := range remaining {
			parentID := strings.TrimSpace(remaining[index].ParentID)
			if parentID == "" {
				continue
			}
			if _, removed := matched[parentID]; !removed {
				continue
			}
			remaining[index].ParentID = ""
			remaining[index].UpdatedAt = now
			result.DetachedChildren = append(result.DetachedChildren, remaining[index].ID)
			changed = true
		}
		store.Groups = remaining
		for _, id := range targets {
			if _, ok := matched[id]; !ok {
				result.Skipped = append(result.Skipped, id)
			}
		}
	} else {
		for index := range store.Groups {
			group := &store.Groups[index]
			if _, target := targetSet[group.ID]; !target {
				continue
			}
			switch normalizedAction {
			case modStrategyGroupBatchEnforceOn:
				if group.Enforce {
					result.Updated = append(result.Updated, group.ID)
					continue
				}
				group.Enforce = true
			case modStrategyGroupBatchEnforceOff:
				if !group.Enforce {
					result.Updated = append(result.Updated, group.ID)
					continue
				}
				group.Enforce = false
			case modStrategyGroupBatchSetTier:
				value := *tier
				group.Tier = &value
			case modStrategyGroupBatchClearTier:
				group.Tier = nil
			}
			group.UpdatedAt = now
			result.Updated = append(result.Updated, group.ID)
			changed = true
		}
		for _, id := range targets {
			if _, ok := targetSet[id]; !ok {
				continue
			}
			found := false
			for index := range store.Groups {
				if store.Groups[index].ID == id {
					found = true
					break
				}
			}
			if !found {
				result.Skipped = append(result.Skipped, id)
			}
		}
	}

	result.Remaining = len(store.Groups)
	if changed {
		if err := a.writeModStrategyGroupStore(store); err != nil {
			return ModStrategyGroupBatchResult{}, fmt.Errorf("无法保存策略组: %w", err)
		}
	}
	return result, nil
}
