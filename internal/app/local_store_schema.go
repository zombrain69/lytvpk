package app

import (
	"log"
	"strings"
	"time"
)

// 本地记录（groups.json / priority.json 等）的 schema 版本与迁移。
//
// 对齐 FireAxe 的存档版本化思路（`AddonRootSave.cs:6-13` 把版本号写进存档、读档时按版本迁移）：
//   - 缺 `schemaVersion` 的旧文件按 v1 处理，读取时就地迁移（去重、补时间戳、丢弃悬空父级…），
//     下次写盘时会盖上当前版本号；
//   - 读到**比本程序更新**的版本号时不做任何降级：数据照常读，写回时保留原版本号，
//     避免旧版程序把新版存档"改回旧结构"。
const localStoreSchemaVersion = 2

// normalizeLocalStoreVersion 把"缺字段 / 0 / 负数"统一视作 v1。
func normalizeLocalStoreVersion(raw int) int {
	if raw <= 0 {
		return 1
	}
	return raw
}

// localStoreWriteVersion 决定写盘时写入的版本号：正常情况下升到当前版本，
// 只有当磁盘上是"更新的版本"时才原样保留。
func localStoreWriteVersion(existing int) int {
	if normalizeLocalStoreVersion(existing) > localStoreSchemaVersion {
		return existing
	}
	return localStoreSchemaVersion
}

// migrateModStrategyGroupStore 把策略组存档迁移到当前结构。
// v1 → v2：
//   - 补 CreatedAt / UpdatedAt；
//   - 丢弃重复的组 ID（保留第一条）与自指父级；
//   - 丢弃指向不存在组的 parentId（避免界面上出现永远展开不了的层级）；
//   - 组内成员键去重，策略名归一化。
func migrateModStrategyGroupStore(store *modStrategyGroupStore, from int) {
	if store == nil {
		return
	}

	// 与版本无关的读时修复：未知策略在运行时本来就会被 decideModStrategyGroupTargets
	// 兜底成「互斥单选」执行，这里把它写成显式值，避免存档里长期留着一个不存在的策略名
	// （这类值通常来自手改文件或跨版本写入）。
	for index := range store.Groups {
		value := store.Groups[index].Strategy
		if _, err := normalizeModStrategyGroupStrategy(value); err != nil {
			log.Printf("策略组存档修复：策略 %q 不识别，已按「互斥单选」处理（组 %s）",
				value, store.Groups[index].ID)
			store.Groups[index].Strategy = modStrategyGroupSingle
		}
	}

	if normalizeLocalStoreVersion(from) >= localStoreSchemaVersion {
		return
	}
	now := time.Now().Format(time.RFC3339)
	knownIDs := make(map[string]struct{}, len(store.Groups))
	for _, group := range store.Groups {
		if id := strings.TrimSpace(group.ID); id != "" {
			knownIDs[id] = struct{}{}
		}
	}
	seenIDs := make(map[string]struct{}, len(store.Groups))
	groups := make([]ModStrategyGroup, 0, len(store.Groups))
	for _, group := range store.Groups {
		id := strings.TrimSpace(group.ID)
		if id == "" {
			continue
		}
		if _, duplicated := seenIDs[id]; duplicated {
			log.Printf("策略组存档迁移：跳过重复的组 ID %s", id)
			continue
		}
		seenIDs[id] = struct{}{}

		group.ID = id
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" {
			group.Name = id
		}
		if strategy, err := normalizeModStrategyGroupStrategy(group.Strategy); err == nil {
			group.Strategy = strategy
		}
		if strings.TrimSpace(group.CreatedAt) == "" {
			group.CreatedAt = now
		}
		group.UpdatedAt = now

		parentID := strings.TrimSpace(group.ParentID)
		if parentID == id {
			parentID = ""
		}
		if parentID != "" {
			if _, ok := knownIDs[parentID]; !ok {
				parentID = ""
			}
		}
		group.ParentID = parentID

		members := make([]ModStrategyGroupMember, 0, len(group.Members))
		seenKeys := make(map[string]struct{}, len(group.Members))
		for _, member := range group.Members {
			key := strings.TrimSpace(member.Key)
			if key == "" {
				continue
			}
			if _, duplicated := seenKeys[key]; duplicated {
				continue
			}
			seenKeys[key] = struct{}{}
			member.Key = key
			if strings.TrimSpace(member.Name) == "" {
				member.Name = key
			}
			members = append(members, member)
		}
		group.Members = members
		groups = append(groups, group)
	}
	store.Groups = groups
}

// migrateModPriorityStore 把分层存档迁移到当前结构。
// v1 → v2：丢掉空键、同键只保留最后一条、补 UpdatedAt。
func migrateModPriorityStore(store *modPriorityStore, from int) {
	if store == nil || normalizeLocalStoreVersion(from) >= localStoreSchemaVersion {
		return
	}
	now := time.Now().Format(time.RFC3339)
	indexByKey := make(map[string]int, len(store.Entries))
	entries := make([]ModPriorityEntry, 0, len(store.Entries))
	for _, entry := range store.Entries {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			continue
		}
		entry.Key = key
		if strings.TrimSpace(entry.UpdatedAt) == "" {
			entry.UpdatedAt = now
		}
		if index, exists := indexByKey[key]; exists {
			entries[index] = entry
			continue
		}
		indexByKey[key] = len(entries)
		entries = append(entries, entry)
	}
	store.Entries = entries
}
