package app

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// 策略组的"编辑"动作：重命名 / 增删成员。
//
// 只写 groups.json：
//   - 不改动任何 Mod 文件；
//   - 不改动 addonlist.txt（成员变化只影响"按策略应用"后的结果）；
//   - 已经不在磁盘上的成员（缺失成员）不会被编辑动作顺手清掉，
//     否则"改个组名"就会把用户攒了很久的组打散。

const (
	modStrategyGroupNameMaxRunes        = 60
	modStrategyGroupDescriptionMaxRunes = 500
)

func normalizeModStrategyGroupName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("策略组名称不能为空")
	}
	if utf8.RuneCountInString(trimmed) > modStrategyGroupNameMaxRunes {
		return "", fmt.Errorf("策略组名称最多 %d 个字符", modStrategyGroupNameMaxRunes)
	}
	return trimmed, nil
}

func normalizeModStrategyGroupDescription(description string) (string, error) {
	trimmed := strings.TrimSpace(description)
	if utf8.RuneCountInString(trimmed) > modStrategyGroupDescriptionMaxRunes {
		return "", fmt.Errorf("策略组描述最多 %d 个字符", modStrategyGroupDescriptionMaxRunes)
	}
	return trimmed, nil
}

// modStrategyGroupMembersFromKeys 把 addonlist 键翻译成组成员（去重、保序）。
//
//	键可以是 "a.vpk"、"workshop\123.vpk"；显示名优先取当前 VPK 缓存里的真实文件名，
//	缓存里没有（例如文件已被删除的缺失成员）则按键推导，保证组数据稳定可复用。
func (a *App) modStrategyGroupMembersFromKeys(keys []string) []ModStrategyGroupMember {
	rootDir := a.rootDirectorySnapshot()
	displayNameByKey := make(map[string]string)
	if rootDir != "" {
		a.vpkCache.Range(func(_ any, value any) bool {
			cache, ok := value.(*VPKFileCache)
			if !ok || cache == nil {
				return true
			}
			key, keyErr := addonListKeyForManagedVPKPathFromRoot(rootDir, cache.File.Path)
			if keyErr == nil && key != "" {
				displayNameByKey[key] = cache.File.Name
			}
			return true
		})
	}

	members := make([]ModStrategyGroupMember, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, rawKey := range keys {
		key := normalizeAddonListKey(rawKey)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		display := displayNameByKey[key]
		if display == "" {
			display = modStrategyGroupDisplayNameForKey(key)
		}
		members = append(members, ModStrategyGroupMember{Key: key, Name: display})
	}
	return members
}

// RenameModStrategyGroup 修改策略组的名称与描述（成员、策略、权重、层级都不受影响）。
func (a *App) RenameModStrategyGroup(id string, name string, description string) (ModStrategyGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModStrategyGroup{}, fmt.Errorf("缺少策略组 ID")
	}
	normalizedName, err := normalizeModStrategyGroupName(name)
	if err != nil {
		return ModStrategyGroup{}, err
	}
	normalizedDescription, err := normalizeModStrategyGroupDescription(description)
	if err != nil {
		return ModStrategyGroup{}, err
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	for index := range store.Groups {
		if store.Groups[index].ID != id {
			continue
		}
		store.Groups[index].Name = normalizedName
		store.Groups[index].Description = normalizedDescription
		store.Groups[index].UpdatedAt = time.Now().Format(time.RFC3339)
		if err := a.writeModStrategyGroupStore(store); err != nil {
			return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
		}
		return store.Groups[index], nil
	}
	return ModStrategyGroup{}, fmt.Errorf("策略组不存在: %s", id)
}

// AddModStrategyGroupMembers 把若干 addonlist 键加入组（已存在的键忽略）。
// 显示名会按当前缓存刷新，但不会删除任何已有成员。
func (a *App) AddModStrategyGroupMembers(id string, keys []string) (ModStrategyGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModStrategyGroup{}, fmt.Errorf("缺少策略组 ID")
	}
	incoming := a.modStrategyGroupMembersFromKeys(keys)
	if len(incoming) == 0 {
		return ModStrategyGroup{}, fmt.Errorf("请至少选择一个有效的 Mod")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	for index := range store.Groups {
		if store.Groups[index].ID != id {
			continue
		}
		existing := make(map[string]struct{}, len(store.Groups[index].Members))
		for _, member := range store.Groups[index].Members {
			existing[normalizeAddonListKey(member.Key)] = struct{}{}
		}
		added := 0
		for _, member := range incoming {
			if _, duplicate := existing[normalizeAddonListKey(member.Key)]; duplicate {
				continue
			}
			existing[normalizeAddonListKey(member.Key)] = struct{}{}
			store.Groups[index].Members = append(store.Groups[index].Members, member)
			added++
		}
		// 幂等：选中的 Mod 已经都在组里时不报错、也不写盘，
		// 由调用方按"成员数有没有变化"给出提示。
		if added == 0 {
			return store.Groups[index], nil
		}
		store.Groups[index].UpdatedAt = time.Now().Format(time.RFC3339)
		if err := a.writeModStrategyGroupStore(store); err != nil {
			return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
		}
		return store.Groups[index], nil
	}
	return ModStrategyGroup{}, fmt.Errorf("策略组不存在: %s", id)
}

// RemoveModStrategyGroupMembers 把若干键移出组；组至少保留 1 个成员，
// 传进来的键必须确实在组里（避免"以为移掉了其实没动"）。
func (a *App) RemoveModStrategyGroupMembers(id string, keys []string) (ModStrategyGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModStrategyGroup{}, fmt.Errorf("缺少策略组 ID")
	}
	remove := make(map[string]struct{}, len(keys))
	for _, rawKey := range keys {
		if key := normalizeAddonListKey(rawKey); key != "" {
			remove[key] = struct{}{}
		}
	}
	if len(remove) == 0 {
		return ModStrategyGroup{}, fmt.Errorf("请至少选择一个要移出组的 Mod")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	for index := range store.Groups {
		if store.Groups[index].ID != id {
			continue
		}
		kept := make([]ModStrategyGroupMember, 0, len(store.Groups[index].Members))
		removed := 0
		for _, member := range store.Groups[index].Members {
			if _, drop := remove[normalizeAddonListKey(member.Key)]; drop {
				removed++
				continue
			}
			kept = append(kept, member)
		}
		// 幂等：选中的 Mod 都不在组里时不报错、也不写盘。
		if removed == 0 {
			return store.Groups[index], nil
		}
		if len(kept) == 0 {
			// 组被清空后就失去意义：提示用户改用"删除策略组"。
			return ModStrategyGroup{}, fmt.Errorf("不能把成员全部移出；如果不再需要这个组，请直接删除策略组")
		}
		store.Groups[index].Members = kept
		store.Groups[index].UpdatedAt = time.Now().Format(time.RFC3339)
		if err := a.writeModStrategyGroupStore(store); err != nil {
			return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
		}
		return store.Groups[index], nil
	}
	return ModStrategyGroup{}, fmt.Errorf("策略组不存在: %s", id)
}

// SetModStrategyGroupMembers 用给定键整体替换成员列表（用于"重建成员"）。
func (a *App) SetModStrategyGroupMembers(id string, keys []string) (ModStrategyGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModStrategyGroup{}, fmt.Errorf("缺少策略组 ID")
	}
	members := a.modStrategyGroupMembersFromKeys(keys)
	if len(members) == 0 {
		return ModStrategyGroup{}, fmt.Errorf("请至少选择一个有效的 Mod")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	for index := range store.Groups {
		if store.Groups[index].ID != id {
			continue
		}
		store.Groups[index].Members = members
		store.Groups[index].UpdatedAt = time.Now().Format(time.RFC3339)
		if err := a.writeModStrategyGroupStore(store); err != nil {
			return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
		}
		return store.Groups[index], nil
	}
	return ModStrategyGroup{}, fmt.Errorf("策略组不存在: %s", id)
}

// ModStrategyGroupMoveResult 描述一次"把 Mod 从 A 组移到 B 组"的结果。
//
// 这是前端「移动到其它策略组…」的返回值：一次写盘完成"移出 + 加入"，
// 不会出现"已经加入新组、但还没从旧组移出"的中间状态。
type ModStrategyGroupMoveResult struct {
	SourceID   string `json:"sourceId"`
	SourceName string `json:"sourceName"`
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName"`
	// RemovedFromSource 是这次真正从源组移除的键。
	RemovedFromSource []string `json:"removedFromSource"`
	// AddedToTarget 是这次新加入目标组的键（本来就在目标组里的不算）。
	AddedToTarget []string `json:"addedToTarget"`
	// Moved 是两者的交集：既从源组移出、又新加入目标组。
	Moved []string `json:"moved"`
	// AlreadyInTarget 是调用前就已在目标组里的键（没有新增动作）。
	AlreadyInTarget []string `json:"alreadyInTarget"`
	SourceRemaining int      `json:"sourceRemaining"`
	TargetTotal     int      `json:"targetTotal"`
}

// MoveModStrategyGroupMembers 把若干键从源组移到目标组（只写 groups.json 一次）。
//
// 规则：
//   - 源组与目标组必须存在且不同；
//   - 已经在目标组里的键会被跳过（幂等）；
//   - 如果这次移动会把源组的成员全部移空，直接整体拒绝（不写盘、两边都不变），
//     提示用户改用"删除策略组"；
//   - 只有"确实从源组移出"的键才计入 Moved；只加入目标组的键计入 AddedOnly。
func (a *App) MoveModStrategyGroupMembers(sourceID string, targetID string, keys []string) (ModStrategyGroupMoveResult, error) {
	sourceID = strings.TrimSpace(sourceID)
	targetID = strings.TrimSpace(targetID)
	if sourceID == "" {
		return ModStrategyGroupMoveResult{}, fmt.Errorf("缺少源策略组 ID")
	}
	if targetID == "" {
		return ModStrategyGroupMoveResult{}, fmt.Errorf("缺少目标策略组 ID")
	}
	if sourceID == targetID {
		return ModStrategyGroupMoveResult{}, fmt.Errorf("源策略组与目标策略组不能相同")
	}
	moving := a.modStrategyGroupMembersFromKeys(keys)
	if len(moving) == 0 {
		return ModStrategyGroupMoveResult{}, fmt.Errorf("请至少选择一个有效的 Mod")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroupMoveResult{}, err
	}
	sourceIndex := -1
	targetIndex := -1
	for index := range store.Groups {
		switch store.Groups[index].ID {
		case sourceID:
			sourceIndex = index
		case targetID:
			targetIndex = index
		}
	}
	if sourceIndex < 0 {
		return ModStrategyGroupMoveResult{}, fmt.Errorf("源策略组不存在: %s", sourceID)
	}
	if targetIndex < 0 {
		return ModStrategyGroupMoveResult{}, fmt.Errorf("目标策略组不存在: %s", targetID)
	}

	requested := make([]string, 0, len(moving))
	for _, member := range moving {
		requested = append(requested, normalizeAddonListKey(member.Key))
	}
	requestedSet := make(map[string]struct{}, len(requested))
	for _, key := range requested {
		requestedSet[key] = struct{}{}
	}

	targetExisting := make(map[string]struct{}, len(store.Groups[targetIndex].Members))
	for _, member := range store.Groups[targetIndex].Members {
		targetExisting[normalizeAddonListKey(member.Key)] = struct{}{}
	}

	keptSource := make([]ModStrategyGroupMember, 0, len(store.Groups[sourceIndex].Members))
	removedFromSource := make([]string, 0, len(requested))
	for _, member := range store.Groups[sourceIndex].Members {
		key := normalizeAddonListKey(member.Key)
		if _, drop := requestedSet[key]; !drop {
			keptSource = append(keptSource, member)
			continue
		}
		removedFromSource = append(removedFromSource, key)
	}
	if len(removedFromSource) > 0 && len(keptSource) == 0 {
		return ModStrategyGroupMoveResult{}, fmt.Errorf(
			"不能把「%s」的成员全部移出；如果不再需要这个组，请直接删除策略组",
			store.Groups[sourceIndex].Name,
		)
	}

	addedToTarget := make([]string, 0, len(requested))
	alreadyInTarget := make([]string, 0, len(requested))
	for _, key := range requested {
		if _, exists := targetExisting[key]; exists {
			alreadyInTarget = append(alreadyInTarget, key)
			continue
		}
		targetExisting[key] = struct{}{}
		addedToTarget = append(addedToTarget, key)
	}
	moved := make([]string, 0, len(addedToTarget))
	for _, key := range addedToTarget {
		if containsString(removedFromSource, key) {
			moved = append(moved, key)
		}
	}

	now := time.Now().Format(time.RFC3339)
	store.Groups[sourceIndex].Members = keptSource
	store.Groups[sourceIndex].UpdatedAt = now
	// 按请求顺序补齐目标组成员（保持用户勾选顺序，显示名沿用现有规则）。
	addedMembers := make([]ModStrategyGroupMember, 0, len(addedToTarget))
	for _, member := range moving {
		key := normalizeAddonListKey(member.Key)
		if !containsString(addedToTarget, key) {
			continue
		}
		addedMembers = append(addedMembers, member)
	}
	store.Groups[targetIndex].Members = append(store.Groups[targetIndex].Members, addedMembers...)
	store.Groups[targetIndex].UpdatedAt = now

	if len(removedFromSource) == 0 && len(addedToTarget) == 0 {
		// 全部已经在目标组里：不写盘，直接返回现状。
		return ModStrategyGroupMoveResult{
			SourceID:          store.Groups[sourceIndex].ID,
			SourceName:        store.Groups[sourceIndex].Name,
			TargetID:          store.Groups[targetIndex].ID,
			TargetName:        store.Groups[targetIndex].Name,
			RemovedFromSource: []string{},
			AddedToTarget:     []string{},
			Moved:             []string{},
			AlreadyInTarget:   alreadyInTarget,
			SourceRemaining:   len(store.Groups[sourceIndex].Members),
			TargetTotal:       len(store.Groups[targetIndex].Members),
		}, nil
	}

	if err := a.writeModStrategyGroupStore(store); err != nil {
		return ModStrategyGroupMoveResult{}, fmt.Errorf("无法保存策略组: %w", err)
	}
	return ModStrategyGroupMoveResult{
		SourceID:          store.Groups[sourceIndex].ID,
		SourceName:        store.Groups[sourceIndex].Name,
		TargetID:          store.Groups[targetIndex].ID,
		TargetName:        store.Groups[targetIndex].Name,
		RemovedFromSource: removedFromSource,
		AddedToTarget:     addedToTarget,
		Moved:             moved,
		AlreadyInTarget:   alreadyInTarget,
		SourceRemaining:   len(store.Groups[sourceIndex].Members),
		TargetTotal:       len(store.Groups[targetIndex].Members),
	}, nil
}
