package app

import "strings"

// AddonListMovedEntry 描述一个条目在重排前后的位置（写前差异明细用）。
type AddonListMovedEntry struct {
	Key      string `json:"key"`
	OldOrder int    `json:"oldOrder"`
	NewOrder int    `json:"newOrder"`
}

// AddonListWriteDiff 描述一次 addonlist.txt 写入的条目级差异（写前预览用）。
// 预览与写入共用同一份计划计算（见 buildAddonListWritePlan），因此这里的数字
// 与真正落盘的内容一一对应。
type AddonListWriteDiff struct {
	CountBefore      int                   `json:"countBefore"`
	CountAfter       int                   `json:"countAfter"`
	Added            []string              `json:"added,omitempty"`
	Removed          []string              `json:"removed,omitempty"`
	MovedTotal       int                   `json:"movedTotal"`
	Moved            []AddonListMovedEntry `json:"moved,omitempty"`
	Unchanged        int                   `json:"unchanged"`
	DuplicatesFolded int                   `json:"duplicatesFolded,omitempty"`
}

const addonListWriteDiffMovedLimit = 200

// diffAddonListForWrite 计算「去重后的当前列表 → 将写入列表」的条目级差异。
// 键比较用 normalizeAddonListKey（与写回逻辑同一套口径）；同一键出现多次时只认首次出现。
//
// 「移动」按公共子序列位置判定：先把将删除的条目剔除再比较位置 ——
// 否则"删 1 条导致后面所有条目序号 -1"会被误报成整体移动。
func diffAddonListForWrite(before []AddonListItem, after []AddonListItem, duplicatesFolded int) AddonListWriteDiff {
	diff := AddonListWriteDiff{
		CountBefore:      len(before),
		CountAfter:       len(after),
		DuplicatesFolded: duplicatesFolded,
	}

	beforePos := make(map[string]int, len(before))
	seen := make(map[string]bool, len(before))
	for index, item := range before {
		key := normalizeAddonListKey(item.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		beforePos[key] = index
	}
	afterPos := make(map[string]int, len(after))
	afterName := make(map[string]string, len(after))
	seen = make(map[string]bool, len(after))
	for index, item := range after {
		key := normalizeAddonListKey(item.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		afterPos[key] = index
		afterName[key] = strings.TrimSpace(item.Name)
	}

	// 按文件顺序收集（明细列表必须确定性，不能跟着 map 迭代顺序飘）。
	commonBefore := make([]string, 0, len(before))
	seen = make(map[string]bool, len(before))
	for _, item := range before {
		key := normalizeAddonListKey(item.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if _, keeps := afterPos[key]; keeps {
			commonBefore = append(commonBefore, key)
		} else {
			diff.Removed = append(diff.Removed, strings.TrimSpace(item.Name))
		}
	}
	commonAfter := make([]string, 0, len(after))
	seen = make(map[string]bool, len(after))
	for _, item := range after {
		key := normalizeAddonListKey(item.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if _, had := beforePos[key]; had {
			commonAfter = append(commonAfter, key)
		} else {
			diff.Added = append(diff.Added, strings.TrimSpace(item.Name))
		}
	}

	afterCommonIndex := make(map[string]int, len(commonAfter))
	for index, key := range commonAfter {
		afterCommonIndex[key] = index
	}
	for index, key := range commonBefore {
		newIndex := afterCommonIndex[key]
		if index == newIndex {
			diff.Unchanged++
			continue
		}
		diff.MovedTotal++
		if len(diff.Moved) < addonListWriteDiffMovedLimit {
			name := afterName[key]
			if name == "" {
				name = key
			}
			diff.Moved = append(diff.Moved, AddonListMovedEntry{
				Key:      name,
				OldOrder: beforePos[key] + 1,
				NewOrder: afterPos[key] + 1,
			})
		}
	}
	return diff
}

// IsEmpty 报告这次写入是否没有可观察变化（新增 / 移除 / 移动 / 合并重复都算变化）。
func (d AddonListWriteDiff) IsEmpty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && d.MovedTotal == 0 && d.DuplicatesFolded == 0
}
