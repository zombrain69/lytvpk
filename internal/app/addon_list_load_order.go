package app

import (
	"fmt"
	"sort"
	"strings"
)

const (
	addonListStateOrderKeep          = "keep"
	addonListStateOrderEnabledFirst  = "enabled-first"
	addonListStateOrderDisabledFirst = "disabled-first"
)

// AddonListLoadOrderEntry 是 addonlist.txt 中一个可排序的 Mod 条目。
// Key 始终是可直接写回 addonlist.txt 的相对键，例如 workshop\\123.vpk。
type AddonListLoadOrderEntry struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	Order      int    `json:"order"`
	IsWorkshop bool   `json:"isWorkshop"`
	IsRoot     bool   `json:"isRoot"`
}

// AddonListLoadOrderConstraint 表示 Before 必须排在 After 之前。
// 两端可使用 addonlist.txt 键或当前扫描到的 VPK 绝对路径。
type AddonListLoadOrderConstraint struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// AddonListLoadOrderPolicy 是一次可选择的加载顺序优化策略。
// GroupWorkshop 会把工坊条目稳定聚集在原先第一个工坊条目的位置；
// RootFirst 会把根目录条目稳定放到工坊条目之前；StateOrder 可稳定地把
// 游戏内开启或关闭的条目排在前面。约束最后执行，因此明确的“前/后”规则
// 可以覆盖自动分组的默认顺序。所有策略只调整条目顺序，不会修改 Value。
type AddonListLoadOrderPolicy struct {
	RootFirst     bool                           `json:"rootFirst"`
	GroupWorkshop bool                           `json:"groupWorkshop"`
	StateOrder    string                         `json:"stateOrder"`
	Constraints   []AddonListLoadOrderConstraint `json:"constraints"`
}

// AddonListLoadOrderPreview 既用于预览，也作为应用成功后的最终顺序返回值。
type AddonListLoadOrderPreview struct {
	Entries []AddonListLoadOrderEntry `json:"entries"`
}

// GetAddonListLoadOrderEntries 返回当前 addonlist.txt 的完整顺序。
func (a *App) GetAddonListLoadOrderEntries() ([]AddonListLoadOrderEntry, error) {
	list, _, err := a.readAddonList()
	if err != nil {
		return nil, err
	}
	return makeAddonListLoadOrderEntries(list), nil
}

// PreviewAddonListLoadOrderPolicy 仅计算排序结果，不会改写 addonlist.txt。
func (a *App) PreviewAddonListLoadOrderPolicy(policy AddonListLoadOrderPolicy) (AddonListLoadOrderPreview, error) {
	list, _, err := a.readAddonList()
	if err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	ordered, err := a.applyAddonListLoadOrderPolicy(list, policy)
	if err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	return AddonListLoadOrderPreview{Entries: makeAddonListLoadOrderEntries(ordered)}, nil
}

// ApplyAddonListLoadOrderPolicy 校验策略、原子写回 addonlist.txt，并同步运行时保护快照。
func (a *App) ApplyAddonListLoadOrderPolicy(policy AddonListLoadOrderPolicy) (AddonListLoadOrderPreview, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	list, path, err := a.readAddonList()
	if err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	ordered, err := a.applyAddonListLoadOrderPolicy(list, policy)
	if err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	if err := a.writeAddonList(path, ordered); err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	if err := a.syncManagedAddonListSnapshotLocked(path); err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	return AddonListLoadOrderPreview{Entries: makeAddonListLoadOrderEntries(ordered)}, nil
}

func makeAddonListLoadOrderEntries(list []AddonListItem) []AddonListLoadOrderEntry {
	entries := make([]AddonListLoadOrderEntry, 0, len(list))
	for index, item := range list {
		key := normalizeAddonListKey(item.Name)
		entries = append(entries, AddonListLoadOrderEntry{
			Key:        key,
			Value:      item.Value,
			Order:      index + 1,
			IsWorkshop: isWorkshopAddonListKey(key),
			IsRoot:     isRootAddonListKey(key),
		})
	}
	return entries
}

func isWorkshopAddonListKey(key string) bool {
	return strings.HasPrefix(normalizeAddonListKey(key), "workshop\\")
}

func isRootAddonListKey(key string) bool {
	return !strings.Contains(normalizeAddonListKey(key), "\\")
}

func (a *App) applyAddonListLoadOrderPolicy(list []AddonListItem, policy AddonListLoadOrderPolicy) ([]AddonListItem, error) {
	if err := validateUniqueAddonListKeys(list); err != nil {
		return nil, err
	}

	ordered := append([]AddonListItem(nil), list...)
	if policy.GroupWorkshop {
		ordered = groupWorkshopAddonListItems(ordered)
	}
	if policy.RootFirst {
		ordered = rootFirstAddonListItems(ordered)
	}
	stateOrder, err := normalizeAddonListStateOrder(policy.StateOrder)
	if err != nil {
		return nil, err
	}
	if stateOrder != addonListStateOrderKeep {
		ordered = orderAddonListItemsByState(ordered, stateOrder)
	}
	return a.applyAddonListOrderConstraints(ordered, policy.Constraints)
}

func normalizeAddonListStateOrder(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return addonListStateOrderKeep, nil
	}
	switch normalized {
	case addonListStateOrderKeep, addonListStateOrderEnabledFirst, addonListStateOrderDisabledFirst:
		return normalized, nil
	default:
		return "", fmt.Errorf("未知的游戏内开关排序规则 %q", value)
	}
}

func validateUniqueAddonListKeys(list []AddonListItem) error {
	seen := make(map[string]struct{}, len(list))
	for _, item := range list {
		key := normalizeAddonListKey(item.Name)
		if key == "" {
			return fmt.Errorf("addonlist.txt 含有空的 Mod 条目，无法排序")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("addonlist.txt 含有重复条目 %q，无法确定约束目标，请先保留其中一个", item.Name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// groupWorkshopAddonListItems 保持工坊条目及其余条目的原有相对顺序，
// 只把全部工坊条目集中到原先首个工坊条目的位置。
func groupWorkshopAddonListItems(list []AddonListItem) []AddonListItem {
	firstWorkshop := -1
	workshopItems := make([]AddonListItem, 0)
	for index, item := range list {
		if isWorkshopAddonListKey(item.Name) {
			if firstWorkshop == -1 {
				firstWorkshop = index
			}
			workshopItems = append(workshopItems, item)
		}
	}
	if firstWorkshop == -1 {
		return append([]AddonListItem(nil), list...)
	}

	grouped := make([]AddonListItem, 0, len(list))
	for index, item := range list {
		if index == firstWorkshop {
			grouped = append(grouped, workshopItems...)
		}
		if !isWorkshopAddonListKey(item.Name) {
			grouped = append(grouped, item)
		}
	}
	return grouped
}

func rootFirstAddonListItems(list []AddonListItem) []AddonListItem {
	ordered := make([]AddonListItem, 0, len(list))
	for _, item := range list {
		if isRootAddonListKey(item.Name) {
			ordered = append(ordered, item)
		}
	}
	for _, item := range list {
		if !isRootAddonListKey(item.Name) {
			ordered = append(ordered, item)
		}
	}
	return ordered
}

// orderAddonListItemsByState 稳定地按游戏内开关状态重排，绝不改写 Value。
func orderAddonListItemsByState(list []AddonListItem, stateOrder string) []AddonListItem {
	preferEnabled := stateOrder == addonListStateOrderEnabledFirst
	ordered := make([]AddonListItem, 0, len(list))
	for _, item := range list {
		if (item.Value == "1") == preferEnabled {
			ordered = append(ordered, item)
		}
	}
	for _, item := range list {
		if (item.Value == "1") != preferEnabled {
			ordered = append(ordered, item)
		}
	}
	return ordered
}

func (a *App) applyAddonListOrderConstraints(list []AddonListItem, constraints []AddonListLoadOrderConstraint) ([]AddonListItem, error) {
	if len(constraints) == 0 {
		return append([]AddonListItem(nil), list...), nil
	}

	indexByKey := make(map[string]int, len(list))
	for index, item := range list {
		indexByKey[normalizeAddonListKey(item.Name)] = index
	}

	adjacent := make([][]int, len(list))
	predecessors := make([][]int, len(list))
	inDegree := make([]int, len(list))
	edges := make(map[[2]int]struct{})
	for _, constraint := range constraints {
		before, err := a.addonListKeyForReference(constraint.Before)
		if err != nil {
			return nil, fmt.Errorf("无效的‘始终在前’ Mod: %w", err)
		}
		after, err := a.addonListKeyForReference(constraint.After)
		if err != nil {
			return nil, fmt.Errorf("无效的‘始终在后’ Mod: %w", err)
		}
		if before == after {
			return nil, fmt.Errorf("同一个 Mod 不能同时要求排在自身之前: %s", before)
		}
		beforeIndex, beforeExists := indexByKey[before]
		afterIndex, afterExists := indexByKey[after]
		if !beforeExists {
			return nil, fmt.Errorf("约束中的 Mod 不在 addonlist.txt: %s", before)
		}
		if !afterExists {
			return nil, fmt.Errorf("约束中的 Mod 不在 addonlist.txt: %s", after)
		}

		edge := [2]int{beforeIndex, afterIndex}
		if _, exists := edges[edge]; exists {
			continue
		}
		edges[edge] = struct{}{}
		adjacent[beforeIndex] = append(adjacent[beforeIndex], afterIndex)
		predecessors[afterIndex] = append(predecessors[afterIndex], beforeIndex)
		inDegree[afterIndex]++
	}

	// 保持每个锚点的“前置 Mod”整体搬到锚点前面，而不是让锚点
	// 被原列表中所有可用条目一路推迟到末尾。例如：锚点原在第 2 位，
	// 来源 Mod 原在第 249~251 位，要求来源排在锚点前时，结果应为
	// 来源进入第 2~4 位、锚点顺延，而不是锚点变成第 254 位。
	// 仍使用拓扑约束保证所有前后关系成立；递归展开依赖时按原始顺序
	// 处理兄弟节点，未涉及约束的条目继续保持原有相对顺序。
	for index := range predecessors {
		sort.SliceStable(predecessors[index], func(i, j int) bool {
			return predecessors[index][i] < predecessors[index][j]
		})
	}

	ordered := make([]AddonListItem, 0, len(list))
	used := make([]bool, len(list))
	visiting := make([]bool, len(list))
	var emit func(int) error
	emit = func(index int) error {
		if used[index] {
			return nil
		}
		if visiting[index] {
			return fmt.Errorf("加载顺序约束存在循环，未写入 addonlist.txt")
		}
		visiting[index] = true
		for _, predecessor := range predecessors[index] {
			if err := emit(predecessor); err != nil {
				return err
			}
		}
		if inDegree[index] != 0 {
			// 所有前置节点都应已输出；不满足时说明约束图存在异常。
			return fmt.Errorf("加载顺序约束无法满足，未写入 addonlist.txt")
		}
		visiting[index] = false
		used[index] = true
		ordered = append(ordered, list[index])
		for _, dependent := range adjacent[index] {
			inDegree[dependent]--
		}
		return nil
	}

	for index := range list {
		if used[index] {
			continue
		}
		if err := emit(index); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}
