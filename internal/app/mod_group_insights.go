package app

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Mod 分组洞察：把"策略组"从设置页里的后台记录，变成 Mod 管理页能直接用的东西：
//   - 组归属查询（列表徽标 / 按组筛选 / 组内一眼可辨）；
//   - 整组一键开关；
//   - 整组优先级平移（保持组内相对顺序，整组一起前后移动）；
//   - 分组推导建议（根据工坊合集、文件名前缀、共同标签、同一作者、相同主体给出候选组，
//     用户确认后才落盘）。
//
// 所有推导都只读磁盘与缓存，绝不改动 addonlist.txt 或任何 Mod 文件。

// ModGroupMembership 描述某个 Mod 的组归属，供列表展示与按组筛选使用。
type ModGroupMembership struct {
	Key         string `json:"key"`
	GroupID     string `json:"groupId"`
	GroupName   string `json:"groupName"`
	Strategy    string `json:"strategy"`
	Enforce     bool   `json:"enforce"`
	Tier        *int   `json:"tier,omitempty"`
	MemberCount int    `json:"memberCount"`
	// ParentID 是上级分组（留空表示顶层）。层级只影响展示与组织，不影响优先级；
	// 前端「按分组筛选」会用它把子组排在父组下面。
	ParentID string `json:"parentId,omitempty"`
	// Missing 为 true 表示这条成员记录指向的文件当前不在受管列表里
	// （已被删除 / 移出目录）：组里仍保留它，界面会给"缺失"标记。
	Missing bool `json:"missing,omitempty"`
}

// GetModGroupMembership 返回所有策略组成员的归属信息（一个 Mod 可以属于多个组）。
func (a *App) GetModGroupMembership() ([]ModGroupMembership, error) {
	groups, err := a.ListModStrategyGroups()
	if err != nil {
		return nil, err
	}
	result := make([]ModGroupMembership, 0)
	for _, group := range groups {
		for _, member := range group.Members {
			key := normalizeAddonListKey(member.Key)
			if key == "" {
				continue
			}
			result = append(result, ModGroupMembership{
				Key:         key,
				GroupID:     group.ID,
				GroupName:   group.Name,
				Strategy:    group.Strategy,
				Enforce:     group.Enforce,
				Tier:        group.Tier,
				MemberCount: len(group.Members),
				ParentID:    group.ParentID,
				Missing:     !a.modKeyExistsInVault(key),
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].GroupName != result[j].GroupName {
			return result[i].GroupName < result[j].GroupName
		}
		return result[i].Key < result[j].Key
	})
	return result, nil
}

// ModStrategyGroupMissingMembers 描述某个策略组里"文件已不在列表"的成员。
type ModStrategyGroupMissingMembers struct {
	GroupID      string   `json:"groupId"`
	GroupName    string   `json:"groupName"`
	ParentID     string   `json:"parentId,omitempty"`
	MemberCount  int      `json:"memberCount"`
	// MissingCount / MissingNames 只统计**本组自己**的缺失成员。
	MissingCount int      `json:"missingCount"`
	MissingNames []string `json:"missingNames"`
	// SubtreeMissingCount / AffectedChildCount 是子孙组里的缺失汇总
	// （对齐 FireAxe 的 AddonChildrenProblem：子节点有问题，父节点也要看得到）。
	SubtreeMissingCount int `json:"subtreeMissingCount,omitempty"`
	AffectedChildCount  int `json:"affectedChildCount,omitempty"`
}

// GetModStrategyGroupMissingMembers 汇总每个策略组的缺失成员，
// 供界面提示"哪一组少了文件、放回同名文件即可自动回到组里"。
// 同时把子孙组的缺失汇总到父组上：否则父组行看上去一切正常，问题只藏在展开后的子组里。
func (a *App) GetModStrategyGroupMissingMembers() ([]ModStrategyGroupMissingMembers, error) {
	groups, err := a.ListModStrategyGroups()
	if err != nil {
		return nil, err
	}

	selfMissing := make(map[string][]string, len(groups))
	children := make(map[string][]string, len(groups))
	for _, group := range groups {
		missing := make([]string, 0, 4)
		for _, member := range group.Members {
			key := normalizeAddonListKey(member.Key)
			if key == "" || a.modKeyExistsInVault(key) {
				continue
			}
			missing = append(missing, groupMemberDisplayName(member))
		}
		if len(missing) == 0 {
			continue
		}
		sort.Strings(missing)
		selfMissing[group.ID] = missing
	}
	for _, group := range groups {
		if parentID := strings.TrimSpace(group.ParentID); parentID != "" {
			children[parentID] = append(children[parentID], group.ID)
		}
	}

	// 记忆化 DFS（带访问集合，防脏数据成环时无限递归）。
	subtreeCache := make(map[string][2]int, len(groups))
	visiting := make(map[string]bool, len(groups))
	var subtreeStats func(id string) (int, int)
	subtreeStats = func(id string) (int, int) {
		if cached, ok := subtreeCache[id]; ok {
			return cached[0], cached[1]
		}
		if visiting[id] {
			return 0, 0
		}
		visiting[id] = true
		total, affected := 0, 0
		for _, childID := range children[id] {
			childTotal, childAffected := subtreeStats(childID)
			if len(selfMissing[childID]) > 0 {
				childTotal += len(selfMissing[childID])
				childAffected++
			}
			total += childTotal
			affected += childAffected
		}
		visiting[id] = false
		subtreeCache[id] = [2]int{total, affected}
		return total, affected
	}

	result := make([]ModStrategyGroupMissingMembers, 0, len(groups))
	for _, group := range groups {
		missing := selfMissing[group.ID]
		subtreeMissing, affectedChildren := subtreeStats(group.ID)
		if len(missing) == 0 && subtreeMissing == 0 {
			continue
		}
		result = append(result, ModStrategyGroupMissingMembers{
			GroupID:             group.ID,
			GroupName:           group.Name,
			ParentID:            strings.TrimSpace(group.ParentID),
			MemberCount:         len(group.Members),
			MissingCount:        len(missing),
			MissingNames:        missing,
			SubtreeMissingCount: subtreeMissing,
			AffectedChildCount:  affectedChildren,
		})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].GroupName < result[j].GroupName })
	return result, nil
}

// SetModStrategyGroupEnabled 整组一键开关：开启 = 全部启用，关闭 = 全部关闭。
// 内部复用既有的 ApplyModStrategyGroup（只改组成员开关，不重排、不动其它 Mod）。
func (a *App) SetModStrategyGroupEnabled(id string, enabled bool) (ModStrategyGroupApplyResult, error) {
	strategy := modStrategyGroupOff
	if enabled {
		strategy = modStrategyGroupAll
	}
	return a.ApplyModStrategyGroup(id, ModStrategyGroupApplyOptions{Strategy: strategy})
}

// ModPriorityShiftItem 记录单个成员在整组平移中的分层变化。
type ModPriorityShiftItem struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	From int    `json:"from"`
	To   int    `json:"to"`
}

// ModStrategyGroupPriorityShift 是整组优先级平移的结果。
type ModStrategyGroupPriorityShift struct {
	GroupID   string                 `json:"groupId"`
	GroupName string                 `json:"groupName"`
	Delta     int                    `json:"delta"`
	Moved     []ModPriorityShiftItem `json:"moved"`
	// Skipped 是既没有分层、也不在 addonlist.txt 里的成员（无法判断"当前在第几层"）。
	Skipped []string `json:"skipped"`
}

// ShiftModStrategyGroupPriorities 整组平移优先级：把每个成员的有效分层整体 +delta，
// 组内相对顺序保持不变。只写 priority.json，不重排 addonlist.txt、不改任何 Mod 文件。
func (a *App) ShiftModStrategyGroupPriorities(id string, delta int) (ModStrategyGroupPriorityShift, error) {
	group, err := a.findModStrategyGroup(id)
	if err != nil {
		return ModStrategyGroupPriorityShift{}, err
	}
	if delta == 0 {
		return ModStrategyGroupPriorityShift{
			GroupID:   group.ID,
			GroupName: group.Name,
			Delta:     0,
			Moved:     []ModPriorityShiftItem{},
		}, nil
	}

	layers, err := a.loadModPriorityLayers()
	if err != nil {
		return ModStrategyGroupPriorityShift{}, err
	}
	loadOrder := a.conflictLoadOrderTable()

	type target struct {
		key  string
		name string
		from int
		to   int
	}
	targets := make([]target, 0, len(group.Members))
	skipped := make([]string, 0)
	for _, member := range group.Members {
		key := normalizeAddonListKey(member.Key)
		if key == "" {
			continue
		}
		name := strings.TrimSpace(member.Name)
		if name == "" {
			name = key
		}
		own := layers.tierFor(key)
		entry, known := loadOrder.entries[key]
		base := 0
		switch {
		case own != nil:
			base = *own
		case known:
			base = entry.Index
		default:
			skipped = append(skipped, name)
			continue
		}
		targets = append(targets, target{key: key, name: name, from: base, to: base + delta})
	}

	result := ModStrategyGroupPriorityShift{
		GroupID:   group.ID,
		GroupName: group.Name,
		Delta:     delta,
		Moved:     make([]ModPriorityShiftItem, 0, len(targets)),
		Skipped:   skipped,
	}
	if len(targets) == 0 {
		return result, nil
	}

	// 一次性写盘：先读出全部记录，替换/追加后写回，避免每个成员写一次文件。
	a.priorityMu.Lock()
	defer a.priorityMu.Unlock()
	store, err := a.readModPriorityStore()
	if err != nil {
		return result, err
	}
	now := time.Now().Format(time.RFC3339)
	entries := make([]ModPriorityEntry, 0, len(store.Entries)+len(targets))
	replaced := make(map[string]struct{}, len(targets))
	byKey := make(map[string]target, len(targets))
	for _, item := range targets {
		byKey[item.key] = item
	}
	for _, existing := range store.Entries {
		normalized, ok := normalizeModPriorityEntry(existing)
		if !ok {
			continue
		}
		if item, hit := byKey[normalized.Key]; hit {
			entries = append(entries, ModPriorityEntry{
				Key:       item.key,
				Name:      item.name,
				Tier:      item.to,
				UpdatedAt: now,
			})
			replaced[item.key] = struct{}{}
			continue
		}
		entries = append(entries, normalized)
	}
	for _, item := range targets {
		if _, ok := replaced[item.key]; ok {
			continue
		}
		entries = append(entries, ModPriorityEntry{
			Key:       item.key,
			Name:      item.name,
			Tier:      item.to,
			UpdatedAt: now,
		})
	}
	if err := a.writeModPriorityStore(modPriorityStore{Entries: entries}); err != nil {
		return result, fmt.Errorf("无法保存整组分层: %w", err)
	}

	items := make([]ModPriorityShiftItem, 0, len(targets))
	for _, item := range targets {
		items = append(items, ModPriorityShiftItem{Key: item.key, Name: item.name, From: item.from, To: item.to})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].From < items[j].From })
	result.Moved = items
	return result, nil
}

// CreateModStrategyGroupFromKeys 用"addonlist 键"创建策略组（建议列表点确认时使用）。
func (a *App) CreateModStrategyGroupFromKeys(name string, description string, strategy string, keys []string) (ModStrategyGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ModStrategyGroup{}, fmt.Errorf("策略组名称不能为空")
	}
	normalizedStrategy, err := normalizeModStrategyGroupStrategy(strategy)
	if err != nil {
		return ModStrategyGroup{}, err
	}

	// 键 -> 成员的翻译集中在 mod_group_edit.go，创建与编辑共用同一套显示名规则。
	members := a.modStrategyGroupMembersFromKeys(keys)
	if len(members) == 0 {
		return ModStrategyGroup{}, fmt.Errorf("请至少选择一个有效的 Mod")
	}

	now := time.Now().Format(time.RFC3339)
	group := ModStrategyGroup{
		ID:          newLocalRecordID(),
		Name:        name,
		Description: strings.TrimSpace(description),
		Strategy:    normalizedStrategy,
		Members:     members,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	// 与其它建组入口一致：重名自动加序号。
	group.Name = uniqueModStrategyGroupName(modStrategyGroupNames(store.Groups), group.Name)
	store.Groups = append(store.Groups, group)
	if err := a.writeModStrategyGroupStore(store); err != nil {
		return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
	}
	return group, nil
}

// modStrategyGroupDisplayNameForKey 在缓存里找不到文件时，按键推导显示名：
// root 用文件名，workshop\<id>.vpk 用 <id>.vpk。
func modStrategyGroupDisplayNameForKey(key string) string {
	normalized := normalizeAddonListKey(key)
	if strings.HasPrefix(normalized, "workshop\\") {
		return strings.TrimPrefix(normalized, "workshop\\")
	}
	return normalized
}
