package app

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 变更驱动的冲突自动复检
//
// 设计要点（对齐 FireAxe 的 Problem / IValidity "失效后按需重算"思路）：
//
//   - 任何会改变游戏侧加载状态的操作（开关、方案应用、策略组应用、依赖启用、
//     文件移动/删除、加载顺序写入）只把复检结果标记为**脏**，并广播事件；
//   - 真正的重算发生在需要结果的时候（列表角标 / 修复建议），
//     复用既有的 conflictIndexCache 与 addonlist 状态映射，避免重复解析 VPK；
//   - 修复建议**只作为建议**返回，绝不自动改写 addonlist.txt 或任何 Mod 文件。

const conflictBadgeSeverityRankUnset = 0

// ConflictBadge 是列表角标使用的单 Mod 冲突摘要。
type ConflictBadge struct {
	Path string `json:"path"`
	Key  string `json:"key"`
	Name string `json:"name"`
	// ConflictFiles 是该 Mod 参与且判定为冲突（胜负未定）的文件数。
	ConflictFiles int `json:"conflictFiles"`
	// OverrideFiles 是该 Mod 参与但胜负已判定的文件数。
	OverrideFiles int `json:"overrideFiles"`
	// Severity 取两者中最高严重度。
	Severity string `json:"severity"`
}

// ConflictFixSuggestion 是一条"用户确认后才执行"的修复建议。
type ConflictFixSuggestion struct {
	Kind    string `json:"kind"` // conflict | override | unknown-order
	Summary string `json:"summary"`
	Action  string `json:"action"` // set-tier | register | review-order
	// TargetKey / TargetName 指向建议调整的 Mod。
	TargetKey  string   `json:"targetKey,omitempty"`
	TargetName string   `json:"targetName,omitempty"`
	Files      []string `json:"files"`
	FileCount  int      `json:"fileCount"`
	// SuggestedTier 仅在 Action=set-tier 时给出，用户可一键采纳后再显式应用分层。
	SuggestedTier *int `json:"suggestedTier,omitempty"`
}

// ConflictRecheckStatus 描述自动复检缓存的状态。
type ConflictRecheckStatus struct {
	Dirty          bool   `json:"dirty"`
	Reason         string `json:"reason,omitempty"`
	Generation     int    `json:"generation"`
	RecomputeCount int    `json:"recomputeCount"`
	LastRun        string `json:"lastRun,omitempty"`
	ScannedVPKs    int    `json:"scannedVpks"`
	BadgeCount     int    `json:"badgeCount"`
}

type conflictRecheckState struct {
	mu             sync.Mutex
	dirty          bool
	reason         string
	generation     int
	recomputeCount int
	lastRun        time.Time
	scannedVPKs    int
	badges         []ConflictBadge
	suggestions    []ConflictFixSuggestion
}

// InvalidateConflictRecheck 标记复检结果为脏；不改动任何文件。
func (a *App) InvalidateConflictRecheck(reason string) {
	a.conflictRecheck.mu.Lock()
	a.conflictRecheck.dirty = true
	a.conflictRecheck.generation++
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		a.conflictRecheck.reason = trimmed
	}
	generation := a.conflictRecheck.generation
	a.conflictRecheck.mu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "conflict_recheck_invalidated", map[string]any{
			"reason":     strings.TrimSpace(reason),
			"generation": generation,
		})
	}
}

// GetConflictRecheckStatus 返回当前缓存状态，不会触发重算。
func (a *App) GetConflictRecheckStatus() ConflictRecheckStatus {
	a.conflictRecheck.mu.Lock()
	defer a.conflictRecheck.mu.Unlock()
	return a.conflictRecheckStatusLocked()
}

func (a *App) conflictRecheckStatusLocked() ConflictRecheckStatus {
	// 从未重算过（进程刚启动或刚选择游戏目录）时同样视为待复检。
	dirty := a.conflictRecheck.dirty || (a.conflictRecheck.badges == nil && a.conflictRecheck.recomputeCount == 0)
	status := ConflictRecheckStatus{
		Dirty:          dirty,
		Reason:         a.conflictRecheck.reason,
		Generation:     a.conflictRecheck.generation,
		RecomputeCount: a.conflictRecheck.recomputeCount,
		ScannedVPKs:    a.conflictRecheck.scannedVPKs,
		BadgeCount:     len(a.conflictRecheck.badges),
	}
	if !a.conflictRecheck.lastRun.IsZero() {
		status.LastRun = a.conflictRecheck.lastRun.Format(time.RFC3339)
	}
	return status
}

// GetConflictBadges 返回列表角标数据：缓存有效时直接复用，脏或为空时按需重算一次。
func (a *App) GetConflictBadges() ([]ConflictBadge, error) {
	a.conflictRecheck.mu.Lock()
	needsRecompute := a.conflictRecheck.dirty || a.conflictRecheck.badges == nil
	a.conflictRecheck.mu.Unlock()

	if needsRecompute {
		if _, err := a.RecheckConflictsNow(); err != nil {
			return nil, err
		}
	}

	a.conflictRecheck.mu.Lock()
	defer a.conflictRecheck.mu.Unlock()
	return append([]ConflictBadge(nil), a.conflictRecheck.badges...), nil
}

// GetConflictFixSuggestions 返回"需要用户确认"的修复建议。
func (a *App) GetConflictFixSuggestions() ([]ConflictFixSuggestion, error) {
	a.conflictRecheck.mu.Lock()
	needsRecompute := a.conflictRecheck.dirty || a.conflictRecheck.suggestions == nil
	a.conflictRecheck.mu.Unlock()

	if needsRecompute {
		if _, err := a.RecheckConflictsNow(); err != nil {
			return nil, err
		}
	}

	a.conflictRecheck.mu.Lock()
	defer a.conflictRecheck.mu.Unlock()
	return append([]ConflictFixSuggestion(nil), a.conflictRecheck.suggestions...), nil
}

// RecheckConflictsNow 立即重算冲突角标与修复建议（复用既有索引缓存与优先级分层）。
func (a *App) RecheckConflictsNow() (ConflictRecheckStatus, error) {
	result, err := a.checkConflicts(conflictCheckRequest{matchMode: "or", priorityAware: true})
	if err != nil {
		return a.GetConflictRecheckStatus(), err
	}

	badges := buildConflictBadges(result)
	suggestions := buildConflictFixSuggestions(result)

	a.conflictRecheck.mu.Lock()
	a.conflictRecheck.badges = badges
	a.conflictRecheck.suggestions = suggestions
	a.conflictRecheck.dirty = false
	a.conflictRecheck.reason = ""
	a.conflictRecheck.recomputeCount++
	a.conflictRecheck.lastRun = time.Now()
	a.conflictRecheck.scannedVPKs = countConflictResultParticipants(result)
	status := a.conflictRecheckStatusLocked()
	a.conflictRecheck.mu.Unlock()
	return status, nil
}

func countConflictResultParticipants(result *ConflictResult) int {
	if result == nil {
		return 0
	}
	seen := make(map[string]struct{})
	for _, group := range result.ConflictGroups {
		for _, file := range group.VpkFiles {
			seen[file.Path] = struct{}{}
		}
	}
	for _, group := range result.OverrideGroups {
		for _, file := range group.VpkFiles {
			seen[file.Path] = struct{}{}
		}
	}
	return len(seen)
}

// buildConflictBadges 把冲突结果压缩成"每个 Mod 一行"的角标数据。
func buildConflictBadges(result *ConflictResult) []ConflictBadge {
	if result == nil {
		return []ConflictBadge{}
	}
	byPath := make(map[string]*ConflictBadge)
	ensure := func(file ConflictVPKFile) *ConflictBadge {
		badge, ok := byPath[file.Path]
		if !ok {
			badge = &ConflictBadge{Path: file.Path, Key: conflictBadgeKey(file), Name: file.Name, Severity: "info"}
			byPath[file.Path] = badge
		}
		return badge
	}
	for _, group := range result.ConflictGroups {
		for _, file := range group.VpkFiles {
			badge := ensure(file)
			badge.ConflictFiles += group.FileCount
			badge.Severity = maxConflictSeverity(badge.Severity, group.Severity)
		}
	}
	for _, group := range result.OverrideGroups {
		for _, file := range group.VpkFiles {
			badge := ensure(file)
			badge.OverrideFiles += group.FileCount
			badge.Severity = maxConflictSeverity(badge.Severity, group.Severity)
		}
	}

	badges := make([]ConflictBadge, 0, len(byPath))
	for _, badge := range byPath {
		badges = append(badges, *badge)
	}
	sort.SliceStable(badges, func(i, j int) bool {
		leftRank := getConflictSeverityRank(badges[i].Severity)
		rightRank := getConflictSeverityRank(badges[j].Severity)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		leftTotal := badges[i].ConflictFiles + badges[i].OverrideFiles
		rightTotal := badges[j].ConflictFiles + badges[j].OverrideFiles
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return badges[i].Name < badges[j].Name
	})
	return badges
}

func conflictBadgeKey(file ConflictVPKFile) string {
	name := strings.ReplaceAll(strings.TrimSpace(file.Name), "/", "\\")
	if strings.EqualFold(file.Location, "workshop") {
		return strings.ToLower("workshop\\" + name)
	}
	return strings.ToLower(name)
}

func maxConflictSeverity(left string, right string) string {
	if getConflictSeverityRankUnset(right) > getConflictSeverityRankUnset(left) {
		return right
	}
	return left
}

// getConflictSeverityRankUnset 与 getConflictSeverityRank 一致，但把空串视为最低等级。
func getConflictSeverityRankUnset(severity string) int {
	if strings.TrimSpace(severity) == "" {
		return conflictBadgeSeverityRankUnset
	}
	return getConflictSeverityRank(severity)
}

const conflictFixSuggestionLimit = 50

// buildConflictFixSuggestions 生成"需要用户确认"的修复建议。
// 只输出建议，不写文件：用户确认后由既有 API（SetModPriority / SetVPKGameEnabled 等）执行。
func buildConflictFixSuggestions(result *ConflictResult) []ConflictFixSuggestion {
	if result == nil {
		return []ConflictFixSuggestion{}
	}
	suggestions := make([]ConflictFixSuggestion, 0, len(result.ConflictGroups)+len(result.OverrideGroups))

	for _, group := range result.ConflictGroups {
		if group.Layer == nil {
			// 冲突来自"有参与者没写进 addonlist.txt"：建议先把该 Mod 写入 addonlist，
			// 让先后关系变得可判定，而不是替用户猜一个分层。
			target := pickUnregisteredConflictTarget(group.VpkFiles)
			if target == nil {
				continue
			}
			suggestions = append(suggestions, ConflictFixSuggestion{
				Kind:       "unknown-order",
				Summary:    fmt.Sprintf("“%s”尚未写入 addonlist.txt，导致 %d 个重叠文件无法判定胜负；建议先在游戏内启用它（或执行一次“按分层应用”）把它记录进 addonlist.txt。", target.Name, group.FileCount),
				Action:     "register",
				TargetKey:  conflictBadgeKey(*target),
				TargetName: target.Name,
				Files:      append([]string(nil), group.Files...),
				FileCount:  group.FileCount,
			})
			if len(suggestions) >= conflictFixSuggestionLimit {
				return suggestions
			}
			continue
		}
		// 同层冲突：建议把其中一个 Mod 抬到更靠前的一层，让先后关系变得可判定。
		target := pickConflictSuggestionTarget(group.VpkFiles)
		if target == nil {
			continue
		}
		tier := *group.Layer - 1
		suggestions = append(suggestions, ConflictFixSuggestion{
			Kind:          "conflict",
			Summary:       fmt.Sprintf("“%s”等 %d 个 Mod 的有效分层都是 %d，先后关系未定；建议把其中一个移到更靠前的一层。", target.Name, len(group.VpkFiles), *group.Layer),
			Action:        "set-tier",
			TargetKey:     conflictBadgeKey(*target),
			TargetName:    target.Name,
			Files:         append([]string(nil), group.Files...),
			FileCount:     group.FileCount,
			SuggestedTier: &tier,
		})
		if len(suggestions) >= conflictFixSuggestionLimit {
			return suggestions
		}
	}

	for _, group := range result.OverrideGroups {
		loser := pickConflictSuggestionTarget(filterConflictWinners(group.VpkFiles, group.Winner.Path))
		if loser == nil {
			continue
		}
		tier := group.Winner.Layer + 1
		if group.Winner.Layer < 0 {
			// 胜者未记录顺序时不做分层建议，改为提示先写入 addonlist。
			suggestions = append(suggestions, ConflictFixSuggestion{
				Kind:       "unknown-order",
				Summary:    fmt.Sprintf("“%s”提供 %d 个与其它 Mod 重叠的文件，但尚未写入 addonlist.txt，胜负无法判定。", loser.Name, group.FileCount),
				Action:     "register",
				TargetKey:  conflictBadgeKey(*loser),
				TargetName: loser.Name,
				Files:      append([]string(nil), group.Files...),
				FileCount:  group.FileCount,
			})
			continue
		}
		suggestions = append(suggestions, ConflictFixSuggestion{
			Kind:          "override",
			Summary:       fmt.Sprintf("“%s”当前覆盖“%s”的 %d 个文件；若你希望反过来生效，可把“%s”抬到分层 %d 之后再显式应用分层。", group.Winner.Name, loser.Name, group.FileCount, loser.Name, tier),
			Action:        "set-tier",
			TargetKey:     conflictBadgeKey(*loser),
			TargetName:    loser.Name,
			Files:         append([]string(nil), group.Files...),
			FileCount:     group.FileCount,
			SuggestedTier: &tier,
		})
		if len(suggestions) >= conflictFixSuggestionLimit {
			return suggestions
		}
	}

	return suggestions
}

// pickUnregisteredConflictTarget 选择一条"未写入 addonlist"的参与者作为建议目标。
func pickUnregisteredConflictTarget(files []ConflictVPKFile) *ConflictVPKFile {
	candidates := make([]ConflictVPKFile, 0, len(files))
	for _, file := range files {
		if file.Order < 0 {
			candidates = append(candidates, file)
		}
	}
	return pickConflictSuggestionTarget(candidates)
}

func filterConflictWinners(files []ConflictVPKFile, winnerPath string) []ConflictVPKFile {
	losers := make([]ConflictVPKFile, 0, len(files))
	for _, file := range files {
		if strings.EqualFold(file.Path, winnerPath) {
			continue
		}
		losers = append(losers, file)
	}
	return losers
}

// pickConflictSuggestionTarget 选择建议里优先呈现的 Mod：
// 优先取"未记录顺序"的条目，其次按名称稳定排序的第一个。
func pickConflictSuggestionTarget(files []ConflictVPKFile) *ConflictVPKFile {
	if len(files) == 0 {
		return nil
	}
	sorted := append([]ConflictVPKFile(nil), files...)
	sort.SliceStable(sorted, func(i, j int) bool {
		leftUnknown := sorted[i].Order < 0
		rightUnknown := sorted[j].Order < 0
		if leftUnknown != rightUnknown {
			return leftUnknown
		}
		return sorted[i].Name < sorted[j].Name
	})
	target := sorted[0]
	return &target
}

// markConflictRecheckDirtyAfterAddonListWrite 供 addonlist 写入路径调用。
// 写盘失败时不应标记为脏，因此调用方只在成功后调用本函数。
func (a *App) markConflictRecheckDirtyAfterAddonListWrite() {
	a.InvalidateConflictRecheck("addonlist.txt 已更新")
}

// logConflictRecheckError 统一记录自动复检失败，不影响调用方主流程。
func logConflictRecheckError(err error) {
	if err != nil {
		log.Printf("冲突自动复检失败: %v", err)
	}
}
