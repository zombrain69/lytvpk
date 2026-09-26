package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 策略组"应用前预检"（对齐 FireAxe 的 `AddonGroup.CheckEnableStrategy`）：
// 应用策略前先算清楚"这个策略到底能不能满足"，把问题提前告诉用户，
// 而不是写完 addonlist.txt 之后才发现"两个成员都不在列表里 / 都在 disabled 目录"。
type ModStrategyGroupApplyCheck struct {
	GroupID   string `json:"groupId"`
	GroupName string `json:"groupName"`
	Strategy  string `json:"strategy"`
	// MemberCount 是组里的成员总数。
	MemberCount int `json:"memberCount"`
	// UsableCount 是"文件在 addons / workshop 里、写 0/1 真的生效"的成员数。
	UsableCount int `json:"usableCount"`
	// MissingCount 是文件已不在受管列表里的成员数（应用时会被跳过）。
	MissingCount int      `json:"missingCount"`
	MissingNames []string `json:"missingNames"`
	// BlockedCount 是位于 disabled 目录的成员数：写 0/1 在游戏里不生效。
	BlockedCount int      `json:"blockedCount"`
	BlockedNames []string `json:"blockedNames"`
	// Applicable 为 false 时表示这个策略现在没法执行，界面应直接拦下。
	Applicable bool `json:"applicable"`
	// Reason 是 applicable=false 时的原因。
	Reason string `json:"reason,omitempty"`
	// Warnings 是"能执行但值得提醒"的事项（界面应让用户确认后再继续）。
	Warnings []string `json:"warnings"`
}

// CheckModStrategyGroupApply 预检某个策略组的应用动作。
// options.Strategy 与 ApplyModStrategyGroup 一样可以临时覆盖组策略（"随机单选 / 全关"按钮用）。
func (a *App) CheckModStrategyGroupApply(id string, options ModStrategyGroupApplyOptions) (ModStrategyGroupApplyCheck, error) {
	group, err := a.findModStrategyGroup(id)
	if err != nil {
		return ModStrategyGroupApplyCheck{}, err
	}
	strategy := group.Strategy
	if override := strings.TrimSpace(options.Strategy); override != "" {
		normalized, normalizeErr := normalizeModStrategyGroupStrategy(override)
		if normalizeErr != nil {
			return ModStrategyGroupApplyCheck{}, normalizeErr
		}
		strategy = normalized
	}

	check := ModStrategyGroupApplyCheck{
		GroupID:     group.ID,
		GroupName:   group.Name,
		Strategy:    strategy,
		MemberCount: len(group.Members),
		Warnings:    []string{},
	}
	for _, member := range group.Members {
		name := groupMemberDisplayName(member)
		switch a.modKeyLocation(member.Key) {
		case "root", "workshop":
			check.UsableCount++
		case "disabled":
			check.BlockedCount++
			check.BlockedNames = append(check.BlockedNames, name)
		default:
			check.MissingCount++
			check.MissingNames = append(check.MissingNames, name)
		}
	}

	switch {
	case check.MemberCount == 0:
		check.Applicable = false
		check.Reason = fmt.Sprintf("策略组「%s」还没有成员：先用「加入策略组…」把 Mod 放进来", group.Name)
	case check.UsableCount == 0 && check.MissingCount == check.MemberCount:
		check.Applicable = false
		check.Reason = fmt.Sprintf("策略组「%s」的成员文件都不在当前列表里，应用不会有任何效果", group.Name)
	case check.UsableCount == 0:
		// 成员都在 disabled 目录：游戏本来就不会加载它们。
		// 「全关」这种"关掉"的策略仍然算可执行（写 0 不影响任何东西），其余策略没有意义。
		if strategy == modStrategyGroupOff {
			check.Applicable = true
			check.Warnings = append(check.Warnings,
				"所有成员都在 disabled 目录，游戏内本来就不会加载它们（这次只会把 addonlist.txt 里的开关写成 0）")
		} else {
			check.Applicable = false
			check.Reason = fmt.Sprintf(
				"策略组「%s」的成员都在 disabled 目录，游戏内不会加载：先用「批量启用」把它们放回 addons",
				group.Name,
			)
		}
	default:
		check.Applicable = true
	}

	if check.MissingCount > 0 {
		check.Warnings = append(check.Warnings, fmt.Sprintf(
			"%d 个成员的文件已不在列表里，应用时会被跳过：%s",
			check.MissingCount, summarizeNames(check.MissingNames),
		))
	}
	if check.BlockedCount > 0 && check.UsableCount > 0 {
		check.Warnings = append(check.Warnings, fmt.Sprintf(
			"%d 个成员位于 disabled 目录，游戏内不会加载（写 0/1 不生效）：%s",
			check.BlockedCount, summarizeNames(check.BlockedNames),
		))
	}
	if check.Applicable && check.UsableCount == 1 &&
		(strategy == modStrategyGroupSingle || strategy == modStrategyGroupSingleRandom) {
		check.Warnings = append(check.Warnings, "只有 1 个可用成员，这个「单选」策略实际上没有选择空间")
	}
	return check, nil
}

// modKeyLocation 返回 addonlist 键对应的文件位置：root / workshop / disabled，找不到返回 ""。
func (a *App) modKeyLocation(key string) string {
	target := normalizeAddonListKey(key)
	if target == "" {
		return ""
	}
	rootDir := a.rootDirectorySnapshot()
	if rootDir != "" {
		found := ""
		a.vpkCache.Range(func(_ any, value any) bool {
			cache, ok := value.(*VPKFileCache)
			if !ok || cache == nil {
				return true
			}
			current, err := addonListKeyForManagedVPKPathFromRoot(rootDir, cache.File.Path)
			if err != nil || normalizeAddonListKey(current) != target {
				return true
			}
			found = strings.ToLower(strings.TrimSpace(cache.File.Location))
			return false
		})
		if found != "" {
			return found
		}
		// 缓存可能还没扫到（刚移动完 / 测试夹具）：用物理路径兜底。
		for _, candidate := range []struct {
			location string
			path     string
		}{
			{"root", filepath.Join(rootDir, target)},
			{"workshop", filepath.Join(rootDir, "workshop", target)},
			{"disabled", filepath.Join(rootDir, "disabled", target)},
		} {
			if info, err := os.Stat(candidate.path); err == nil && !info.IsDir() {
				return candidate.location
			}
		}
	}
	return ""
}

// summarizeNames 把成员名压成一行（最多 3 个，其余用"等 N 个"）。
func summarizeNames(names []string) string {
	if len(names) == 0 {
		return ""
	}
	limit := 3
	if len(names) <= limit {
		return strings.Join(names, "、")
	}
	return strings.Join(names[:limit], "、") + fmt.Sprintf(" 等 %d 个", len(names))
}
