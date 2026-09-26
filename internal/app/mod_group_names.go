package app

import (
	"fmt"
	"strings"
	"time"
)

// 策略组重名处理（对齐 FireAxe `AddonNodeContainerService.GetUniqueChildName` / `NameExists`）。
//
// FireAxe 的容器不允许同级重名：新建时自动加后缀（GetUniqueChildName），
// 改名撞名则由容器拒绝（ThrowIfChildNewNameDisallowed）。
// 本项目的策略组是"扁平集合 + 可选树形层级"，重名同样会让
// 「按分组筛选」「整组应用」「通知提示」变得含糊（两个「角色包」到底是哪个），所以沿用同一套规则：
//   - 新建（建组 / ＋子组）自动改名，保证结果唯一；
//   - 重命名直接报错，避免"我输入的名字和最后生效的名字不一样"。
const modStrategyGroupNameMaxAttempts = 99

// modStrategyGroupNameKey 是"是否同名"的比较键：去首尾空白 + 不区分大小写。
func modStrategyGroupNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// modStrategyGroupNames 抽出所有组名。
func modStrategyGroupNames(groups []ModStrategyGroup) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		if name := strings.TrimSpace(group.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// uniqueModStrategyGroupName 在已有名字里找一个不冲突的名字：
// 冲突时依次尝试「名字 (2)」「名字 (3)」…，全部撞上时退化到带时间戳的名字。
// 返回值保证不超过名称长度上限（必要时先截断基础名再加后缀）。
func uniqueModStrategyGroupName(existing []string, desired string) string {
	base := strings.TrimSpace(desired)
	if base == "" {
		base = "未命名策略组"
	}

	used := make(map[string]bool, len(existing))
	for _, name := range existing {
		if key := modStrategyGroupNameKey(name); key != "" {
			used[key] = true
		}
	}
	if !used[modStrategyGroupNameKey(base)] {
		return base
	}

	// 给后缀留位置：` (99)` 最多 5 个字符。
	trimmedBase := truncateRunes(base, modStrategyGroupNameMaxRunes-5)
	for index := 2; index <= modStrategyGroupNameMaxAttempts; index++ {
		candidate := fmt.Sprintf("%s (%d)", trimmedBase, index)
		if !used[modStrategyGroupNameKey(candidate)] {
			return candidate
		}
	}
	return truncateRunes(fmt.Sprintf("%s (%d)", trimmedBase, time.Now().Unix()), modStrategyGroupNameMaxRunes)
}

// findModStrategyGroupNameConflict 返回与 desired 重名的**其它**组的名字；没有冲突时返回空串。
// exceptID 用于"改名成自己原来的名字"这种情况（不算冲突）。
func findModStrategyGroupNameConflict(groups []ModStrategyGroup, desired string, exceptID string) string {
	key := modStrategyGroupNameKey(desired)
	if key == "" {
		return ""
	}
	for _, group := range groups {
		if exceptID != "" && group.ID == exceptID {
			continue
		}
		if modStrategyGroupNameKey(group.Name) == key {
			return group.Name
		}
	}
	return ""
}

// truncateRunes 按字符（不是字节）截断，避免把中文名字切坏。
func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
