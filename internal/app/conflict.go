package app

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	rt "runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"vpk-manager/internal/parser"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ConflictVPKFile struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Title    string `json:"title"`
	Location string `json:"location"`
	// Order 是该 VPK 在 addonlist.txt 中的 0 基顺序号；-1 表示未记录。
	// 只有在优先级感知模式下才会填充真实值。
	Order int `json:"order"`
	// Layer 是该 VPK 的有效分层（0 基，越小越先加载）；-1 表示未计算。
	// 未设置任何分层时 Layer == Order，判定结果因此与历史行为一致。
	Layer int `json:"layer"`
	// Tier 是该 Mod 的显式分层；nil 表示未设置（此时 base = 顺序号）。
	Tier *int `json:"tier,omitempty"`
}

type ConflictGroup struct {
	VpkFiles       []ConflictVPKFile `json:"vpk_files"`
	Files          []string          `json:"files"`
	FileCount      int               `json:"file_count"`
	FilesTruncated bool              `json:"files_truncated"`
	Severity       string            `json:"severity"` // "critical", "warning", "info"
	// Layer 是该组全部参与者共用的有效分层，仅在"同层冲突"时填充；
	// nil 表示冲突来自存在未记录参与者，而不是分层相同。
	Layer *int `json:"layer,omitempty"`
}

// ConflictOverrideGroup 描述一组"胜负已判定"的资源重叠：所有参与者都在
// addonlist.txt 中记录了加载顺序，胜者由顺序号决定，因此不属于冲突。
type ConflictOverrideGroup struct {
	VpkFiles       []ConflictVPKFile `json:"vpk_files"`
	Winner         ConflictVPKFile   `json:"winner"`
	Files          []string          `json:"files"`
	FileCount      int               `json:"file_count"`
	FilesTruncated bool              `json:"files_truncated"`
	Severity       string            `json:"severity"`
}

type ConflictResult struct {
	TotalConflicts int                     `json:"total_conflicts"`
	ConflictGroups []ConflictGroup         `json:"conflict_groups"`
	TotalOverrides int                     `json:"total_overrides"`
	OverrideGroups []ConflictOverrideGroup `json:"override_groups"`
	// ModIgnoreAnnotations 记录"因为某个 Mod 自己的忽略规则而被跳过的重叠"：
	// 这些路径本来会进入冲突/覆盖统计，但该 Mod 用自己的清单声明"我不提供它"。
	// 只有真正被其它 Mod 覆盖到的路径才会出现，避免把纯忽略清单刷屏。
	ModIgnoreAnnotations      []ConflictModIgnoreAnnotation `json:"mod_ignore_annotations"`
	TotalModIgnoreAnnotations int                           `json:"total_mod_ignore_annotations"`
}

// ConflictModIgnoreAnnotation 描述一次"被自身规则忽略"的重叠。
type ConflictModIgnoreAnnotation struct {
	// File 是归档内路径（归一化后的 "/" 分隔小写形式）。
	File string `json:"file"`
	// VpkFiles 是声明了该忽略规则、因而退出判定的 Mod。
	VpkFiles []ConflictVPKFile `json:"vpk_files"`
}

// ConflictBaselineRule describes one condition used to select the Mods that
// scoped conflict analysis compares against. TargetPaths identify the current
// list/filter result; only targets that also satisfy the selected baseline can
// qualify a result group. Supported types are: enabled, not_disabled, tag,
// root, and workshop.
type ConflictBaselineRule struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

// ConflictAnalysisOptions configures a scoped conflict check. MatchMode is
// "and" (all rules) or "or" (any rule). An empty rule list deliberately keeps
// the historical behaviour and uses the currently game-enabled Mods.
type ConflictAnalysisOptions struct {
	TargetPaths   []string               `json:"targetPaths"`
	BaselineRules []ConflictBaselineRule `json:"baselineRules"`
	MatchMode     string                 `json:"matchMode"`
	// PriorityAware 开启"按加载顺序判定胜负"的分析模式：可判定胜负的重叠
	// 归入 OverrideGroups，只有无法判定胜负的重叠才留在冲突里。
	PriorityAware bool `json:"priorityAware"`
	// IgnoreFiles 是调用方附加的忽略清单，元素为归档内路径；
	// 以 "/" 结尾的条目按目录前缀匹配。内置忽略规则始终生效。
	IgnoreFiles []string `json:"ignoreFiles"`
	// FullScan 让调用方在全量扫描（与 CheckConflicts 相同的范围）上使用新选项，
	// 此时 TargetPaths 必须为空；未开启时保持"空 targetPaths = 空结果"的历史行为。
	FullScan bool `json:"fullScan"`
}

// conflictCheckRequest 汇总一次冲突检查的全部输入，避免继续拉长参数列表。
type conflictCheckRequest struct {
	selectedPaths []string
	baselineRules []ConflictBaselineRule
	matchMode     string
	priorityAware bool
	ignoreFiles   conflictIgnoreSet
}

const (
	conflictWorkerLimit        = 4
	conflictGroupFileListLimit = 2000
	// Keep a practical working set for large addon directories. Entries are
	// evicted individually (LRU-style) instead of clearing the whole cache.
	conflictIndexCacheMax = 1024
	// Scoped conflict checks are bounded to keep the list-page switch
	// responsive while allowing large, practical mod collections. The limit
	// applies to selected targets only; the chosen baseline may be larger.
	// 5,000 targets covers large custom collections while retaining a hard
	// guard against accidental unbounded scans. The baseline may still be
	// larger than this target limit.
	scopedConflictMaxVPKs = 5000
)

type conflictGroupAccumulator struct {
	files     []string
	fileCount int
	severity  string
	// layer 记录该组共用的有效分层（同层冲突），未记录参与者导致的冲突为 nil。
	layer *int
}

// conflictOverrideAccumulator 与冲突累加器同构，额外记住胜者。
type conflictOverrideAccumulator struct {
	files      []string
	fileCount  int
	severity   string
	winnerPath string
}

// conflictLoadEntry 是某个 addonlist 条目的加载顺序信息。
type conflictLoadEntry struct {
	Index   int
	Enabled bool
}

// conflictLoadOrderTable 把 addonlist 条目名映射为加载顺序。
type conflictLoadOrderTable struct {
	entries map[string]conflictLoadEntry
}

// conflictOwner 是一次资源重叠中的某个 VPK 参与者。
type conflictOwner struct {
	Path string
	// Index 为 addonlist 顺序号，仅在 Known 为真时有意义。
	Index int
	// Known 表示 addonlist.txt 是否记录了该 VPK。
	Known bool
	// Enabled 表示该条目在 addonlist.txt 中的开关值。
	Enabled bool
	// Tier 是该 Mod 的显式分层；GroupTier 是所属策略组权重的最小值。
	// 两者都为 nil 时有效分层退化为顺序号，保证未分层时行为与历史一致。
	Tier      *int
	GroupTier *int
}

// effectiveLayer 是冲突判定使用的有效分层。
func (o conflictOwner) effectiveLayer() int {
	return computeEffectivePriority(o.Index, o.Tier, o.GroupTier)
}

type conflictDecisionKind string

const (
	conflictDecisionIgnore   conflictDecisionKind = "ignore"
	conflictDecisionConflict conflictDecisionKind = "conflict"
	conflictDecisionOverride conflictDecisionKind = "override"
)

type conflictDecision struct {
	Kind        conflictDecisionKind
	Owners      []conflictOwner
	WinnerIndex int
	WinnerPath  string
	// Layer 仅在"同层冲突"（全部参与者有效分层相同）时填充，供前端显示"分层 T"。
	Layer *int
}

// conflictOrderWins 判定两个有效分层谁最终生效。LytVPK 现有文档的语义是
// “加载顺序越靠后，通常越容易覆盖前面的资源”，因此有效分层更大者获胜。
// 方向尚待在游戏内做受控实验确认；结论出来后只需修改这一处。
func conflictOrderWins(candidate, current int) bool {
	return candidate > current
}

// decideConflictOwners 按有效分层把一次资源重叠归类为忽略、覆盖或冲突。
// 规则：未加载的条目不参与；未记录的参与者会让重叠无法判定胜负；
// 有效分层相同的重叠属于"意图上无法分辨谁该覆盖谁"的真冲突。
func decideConflictOwners(owners []conflictOwner) conflictDecision {
	participants := make([]conflictOwner, 0, len(owners))
	for _, owner := range owners {
		if owner.Known && !owner.Enabled {
			// 游戏未加载的条目不会提供文件，因此不参与覆盖判定。
			continue
		}
		participants = append(participants, owner)
	}
	if len(participants) < 2 {
		return conflictDecision{Kind: conflictDecisionIgnore}
	}

	seen := make(map[int]string, len(participants))
	for _, owner := range participants {
		if !owner.Known {
			return conflictDecision{Kind: conflictDecisionConflict, Owners: participants}
		}
		layer := owner.effectiveLayer()
		if _, duplicate := seen[layer]; duplicate {
			// 有效分层相同意味着先后关系未定义，按真冲突处理。
			shared := layer
			return conflictDecision{Kind: conflictDecisionConflict, Owners: participants, Layer: &shared}
		}
		seen[layer] = owner.Path
	}

	winner := participants[0]
	for _, owner := range participants[1:] {
		if conflictOrderWins(owner.effectiveLayer(), winner.effectiveLayer()) {
			winner = owner
		}
	}
	return conflictDecision{
		Kind:        conflictDecisionOverride,
		Owners:      participants,
		WinnerIndex: winner.Index,
		WinnerPath:  winner.Path,
	}
}

// conflictIgnoreSet 保存调用方传入的忽略清单。
type conflictIgnoreSet struct {
	exact    map[string]struct{}
	prefixes []string
}

// newConflictIgnoreSet 归一化忽略清单：路径大小写、分隔符与首尾空格都不敏感，
// 以 "/" 结尾的条目按目录前缀匹配。
func newConflictIgnoreSet(entries []string) conflictIgnoreSet {
	set := conflictIgnoreSet{}
	for _, entry := range entries {
		normalized := normalizeConflictFilePath(entry)
		if normalized == "" {
			continue
		}
		if strings.HasSuffix(normalized, "/") {
			prefix := strings.TrimSuffix(normalized, "/") + "/"
			if prefix == "/" {
				continue
			}
			set.prefixes = append(set.prefixes, prefix)
			continue
		}
		if set.exact == nil {
			set.exact = make(map[string]struct{})
		}
		set.exact[normalized] = struct{}{}
	}
	return set
}

// ShouldIgnore 的入参必须已经过 normalizeConflictFilePath 归一化。
func (s conflictIgnoreSet) ShouldIgnore(normalizedPath string) bool {
	if normalizedPath == "" {
		return true
	}
	if _, ok := s.exact[normalizedPath]; ok {
		return true
	}
	for _, prefix := range s.prefixes {
		if strings.HasPrefix(normalizedPath, prefix) {
			return true
		}
	}
	return false
}

// IsEmpty 表示当前没有任何额外忽略规则。
func (s conflictIgnoreSet) IsEmpty() bool {
	return len(s.exact) == 0 && len(s.prefixes) == 0
}

// mergeConflictIgnoreSets 合并两份忽略集合：任一份命中即忽略。
// 用于把"全局忽略清单"与"游戏原版文件白名单"取并集。
func mergeConflictIgnoreSets(left conflictIgnoreSet, right conflictIgnoreSet) conflictIgnoreSet {
	if right.IsEmpty() {
		return left
	}
	if left.IsEmpty() {
		return right
	}
	merged := conflictIgnoreSet{
		exact:    make(map[string]struct{}, len(left.exact)+len(right.exact)),
		prefixes: append(append([]string(nil), left.prefixes...), right.prefixes...),
	}
	for key := range left.exact {
		merged.exact[key] = struct{}{}
	}
	for key := range right.exact {
		merged.exact[key] = struct{}{}
	}
	return merged
}

// normalizeConflictIgnoreFileList 归一化用户维护的忽略清单：去掉空行与注释行，
// 统一使用 "/" 分隔符并转为小写（归档路径匹配本身不区分大小写），按序去重。
func normalizeConflictIgnoreFileList(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}
	result := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		normalized := strings.ToLower(strings.ReplaceAll(trimmed, "\\", "/"))
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// conflictLoadOrderTable 读取 addonlist.txt 并建立顺序表。读取失败时返回空表，
// 由调用方安全降级为"全部无法判定"。
func (a *App) conflictLoadOrderTable() conflictLoadOrderTable {
	list, _, err := a.readAddonList()
	if err != nil || len(list) == 0 {
		return conflictLoadOrderTable{}
	}
	table := conflictLoadOrderTable{entries: make(map[string]conflictLoadEntry, len(list))}
	for index, item := range list {
		key := normalizeAddonListKey(item.Name)
		if key == "" {
			continue
		}
		if _, exists := table.entries[key]; exists {
			// 重复条目保留第一条，避免顺序号随写入顺序漂移。
			continue
		}
		table.entries[key] = conflictLoadEntry{
			Index:   index,
			Enabled: strings.TrimSpace(item.Value) == "1",
		}
	}
	return table
}

// ForPath 返回某个 VPK 路径的加载顺序；第二个返回值表示 addonlist 是否记录了它。
func (t conflictLoadOrderTable) ForPath(rootDir, path string) (conflictLoadEntry, bool) {
	if len(t.entries) == 0 || rootDir == "" {
		return conflictLoadEntry{}, false
	}
	key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, path)
	if err != nil {
		return conflictLoadEntry{}, false
	}
	entry, ok := t.entries[key]
	return entry, ok
}

func conflictOwnersFromPaths(rootDir string, paths []string, loadOrder conflictLoadOrderTable, layers modPriorityLayers) []conflictOwner {
	owners := make([]conflictOwner, 0, len(paths))
	for _, path := range paths {
		entry, known := loadOrder.ForPath(rootDir, path)
		var tier *int
		var groupTier *int
		if known {
			if key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, path); err == nil {
				tier = layers.tierFor(key)
				groupTier = layers.groupTierFor(key)
			}
		}
		owners = append(owners, conflictOwner{
			Path:      path,
			Index:     entry.Index,
			Known:     known,
			Enabled:   entry.Enabled,
			Tier:      tier,
			GroupTier: groupTier,
		})
	}
	return owners
}

func conflictOwnerPaths(owners []conflictOwner) []string {
	paths := make([]string, 0, len(owners))
	for _, owner := range owners {
		paths = append(paths, owner.Path)
	}
	return paths
}

// conflictGroupLess 统一冲突组与覆盖组的排序：先看严重度，再看文件数。
func conflictGroupLess(severityA string, countA int, severityB string, countB int) bool {
	si := getConflictSeverityRank(severityA)
	sj := getConflictSeverityRank(severityB)
	if si != sj {
		return si > sj
	}
	return countA > countB
}

// getConflictSeverity 判断文件冲突严重程度
func getConflictSeverity(filePath string) string {
	lower := strings.ToLower(filePath)
	lower = strings.ReplaceAll(lower, "\\", "/")

	// 🔴 严重
	// 完全匹配
	if lower == "particles/particles_manifest.txt" {
		return "critical"
	}
	if lower == "scripts/soundmixers.txt" {
		return "critical"
	}
	// 后缀匹配
	if strings.HasSuffix(lower, ".bsp") || strings.HasSuffix(lower, ".nav") {
		return "critical"
	}
	// 前缀+后缀匹配
	if strings.HasPrefix(lower, "missions/") && strings.HasSuffix(lower, ".txt") {
		return "critical"
	}
	if strings.HasPrefix(lower, "scripts/") && strings.HasSuffix(lower, ".txt") {
		// 特殊情况：vscripts 属于告警
		if strings.HasPrefix(lower, "scripts/vscripts/") {
			return "warning"
		}
		return "critical"
	}

	// 🟡 告警
	if lower == "sound/sound.cache" {
		return "warning"
	}
	if strings.HasSuffix(lower, ".phy") {
		return "warning"
	}
	if strings.HasPrefix(lower, "resource/") && strings.HasSuffix(lower, ".res") {
		return "warning"
	}
	if strings.HasPrefix(lower, "scripts/vscripts/") {
		return "warning"
	}
	if strings.HasSuffix(lower, ".vscript") || strings.HasSuffix(lower, ".nut") || strings.HasSuffix(lower, ".nuc") {
		return "warning"
	}
	if strings.HasSuffix(lower, ".db") {
		return "warning"
	}
	if strings.HasSuffix(lower, ".vtx") || strings.HasSuffix(lower, ".vvd") {
		return "warning"
	}
	if strings.HasSuffix(lower, ".ttf") || strings.HasSuffix(lower, ".otf") {
		return "warning"
	}

	// 🟢 一般 (其他所有文件)
	return "info"
}

func getConflictSeverityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func isIgnoredConflictFile(filePath string) bool {
	if filePath == "" {
		return true
	}
	if filePath == "addoninfo.txt" || filePath == "addonimage.vtf" || filePath == "addonimage.jpg" {
		return true
	}
	return strings.HasPrefix(filePath, "materials/dev/") || strings.HasPrefix(filePath, "materials/temp/")
}

func normalizeConflictFilePath(filePath string) string {
	filePath = strings.ReplaceAll(filePath, "\\", "/")
	filePath = strings.TrimSpace(filePath)
	return strings.ToLower(filePath)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func getVPKFileListSafely(filePath string) (files []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("解析VPK文件时发生异常: %v", r)
		}
	}()

	return parser.GetVPKFileList(filePath)
}

// emitConflictProgress keeps conflict checks usable in tests and headless
// callers where Wails has not assigned an application context yet.
func (a *App) emitConflictProgress(progress ProgressInfo) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "conflict_check_progress", progress)
	}
}

// CheckConflicts 检测VPK文件冲突
func (a *App) CheckConflicts() (*ConflictResult, error) {
	return a.checkConflicts(conflictCheckRequest{matchMode: "or"})
}

// CheckConflictsForPaths treats the supplied paths as analysis targets. Targets
// that satisfy the default game-enabled baseline are compared with every
// currently game-enabled Mod, including Mods outside the current filter. This
// keeps the list badge useful when a filter narrows the targets without hiding
// conflicts against the active load set, while ignoring disabled targets.
func (a *App) CheckConflictsForPaths(paths []string) (*ConflictResult, error) {
	if len(paths) == 0 {
		return &ConflictResult{}, nil
	}
	if len(paths) > scopedConflictMaxVPKs {
		return nil, fmt.Errorf("当前筛选包含 %d 个 Mod；当前最多支持 %d 个目标，请缩小筛选范围后再分析", len(paths), scopedConflictMaxVPKs)
	}
	return a.checkConflicts(conflictCheckRequest{
		selectedPaths: paths,
		baselineRules: defaultConflictBaselineRules(),
		matchMode:     "or",
	})
}

// CheckConflictsWithOptions checks the selected target Mods against a baseline
// chosen by the caller. It reuses the VPK metadata and archive file-list
// caches, so changing a range normally avoids reparsing unchanged archives.
func (a *App) CheckConflictsWithOptions(options ConflictAnalysisOptions) (*ConflictResult, error) {
	if len(options.TargetPaths) == 0 && !options.FullScan {
		return &ConflictResult{}, nil
	}
	if len(options.TargetPaths) > scopedConflictMaxVPKs {
		return nil, fmt.Errorf("当前筛选包含 %d 个 Mod；当前最多支持 %d 个目标，请缩小筛选范围后再分析", len(options.TargetPaths), scopedConflictMaxVPKs)
	}

	rules, matchMode, err := normalizeConflictBaselineRules(options.BaselineRules, options.MatchMode)
	if err != nil {
		return nil, err
	}
	if options.FullScan && len(options.TargetPaths) == 0 {
		// 全量扫描不使用基线规则，行为与 CheckConflicts 的范围保持一致。
		return a.checkConflicts(conflictCheckRequest{
			matchMode:     matchMode,
			priorityAware: options.PriorityAware,
			ignoreFiles:   newConflictIgnoreSet(options.IgnoreFiles),
		})
	}
	return a.checkConflicts(conflictCheckRequest{
		selectedPaths: options.TargetPaths,
		baselineRules: rules,
		matchMode:     matchMode,
		priorityAware: options.PriorityAware,
		ignoreFiles:   newConflictIgnoreSet(options.IgnoreFiles),
	})
}

// conflictIgnoreFilesSnapshot 返回设置页维护的忽略清单副本。
func (a *App) conflictIgnoreFilesSnapshot() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]string(nil), a.conflictIgnoreFiles...)
}

func (a *App) checkConflicts(req conflictCheckRequest) (*ConflictResult, error) {
	rootDir := a.rootDirectorySnapshot()

	if rootDir == "" {
		return nil, fmt.Errorf("未选择L4D2目录")
	}

	if !a.conflictCheckMu.TryLock() {
		return nil, fmt.Errorf("冲突检测正在进行中，请稍候")
	}
	defer a.conflictCheckMu.Unlock()

	// 设置页维护的忽略清单对所有冲突检测入口生效；调用方显式传入的清单优先。
	ignoreFiles := req.ignoreFiles
	if ignoreFiles.IsEmpty() {
		ignoreFiles = newConflictIgnoreSet(a.conflictIgnoreFilesSnapshot())
	}
	// 游戏原版文件白名单同样对所有入口生效：只用于忽略，不改写任何文件。
	// 批次缺失或解析失败时自动降级（见 loadStockWhitelist）。
	stockWhitelist, _ := a.loadStockWhitelist(false)
	ignoreFiles = mergeConflictIgnoreSets(ignoreFiles, stockWhitelist)

	var vpkPaths []string
	var targetSet map[string]struct{}
	var baselineSet map[string]struct{}
	if req.selectedPaths != nil {
		targetPaths, err := a.collectConflictVPKPaths(req.selectedPaths)
		if err != nil {
			return nil, err
		}
		baselinePaths := a.collectConflictBaselineVPKPaths(req.baselineRules, req.matchMode)
		targetSet = pathSet(targetPaths)
		baselineSet = pathSet(baselinePaths)
		vpkPaths = mergeConflictPaths(targetPaths, baselinePaths)
	} else {
		var err error
		vpkPaths, err = a.collectConflictVPKPaths(nil)
		if err != nil {
			return nil, err
		}
	}

	totalFiles := len(vpkPaths)
	if totalFiles == 0 {
		return &ConflictResult{}, nil
	}

	// 发送开始事件
	a.emitConflictProgress(ProgressInfo{
		Current: 0,
		Total:   totalFiles,
		Message: "开始扫描冲突...",
	})

	// 文件路径 -> VPK列表（使用完整路径）
	fileFirstOwner := make(map[string]string)
	conflictOwners := make(map[string][]string)
	// 归档路径 -> 因为"自己的忽略清单"而放弃提供该文件的 VPK 列表。
	modIgnoredOwners := make(map[string][]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	workerCount := min(conflictWorkerLimit, rt.GOMAXPROCS(0))
	if workerCount < 1 {
		workerCount = 1
	}
	workerSlots := make(chan struct{}, workerCount)

	// 单 Mod 忽略清单在本次检测开始时读取一次，避免在并发扫描里重复读盘。
	modIgnoreSets := a.modIgnoreSetsByPath(rootDir, vpkPaths)

	// 进度计数器
	var processedCount int
	var countMu sync.Mutex

	// 使用协程池并发处理
	for _, path := range vpkPaths {
		wg.Add(1)
		p := path // capture loop variable

		a.submitPoolTask(func() {
			defer wg.Done()
			workerSlots <- struct{}{}
			defer func() { <-workerSlots }()

			files, err := a.getConflictFileList(p)

			countMu.Lock()
			processedCount++
			current := processedCount
			countMu.Unlock()

			// 每5个文件或者最后一个文件发送一次进度，避免事件过多
			if current%5 == 0 || current == totalFiles {
				a.emitConflictProgress(ProgressInfo{
					Current: current,
					Total:   totalFiles,
					Message: fmt.Sprintf("正在分析: %s", filepath.Base(p)),
				})
			}

			if err != nil {
				log.Printf("冲突检测跳过VPK: %s, 错误: %v", p, err)
				return
			}

			mu.Lock()
			for _, f := range files {
				lowerF := normalizeConflictFilePath(f)
				if isIgnoredConflictFile(lowerF) || ignoreFiles.ShouldIgnore(lowerF) {
					continue
				}
				if ownSet, ok := modIgnoreSets[filepath.Clean(p)]; ok && ownSet.ShouldIgnore(lowerF) {
					// 该 Mod 自己声明"不提供这个文件"：它退出本次重叠判定。
					if !containsString(modIgnoredOwners[lowerF], p) {
						modIgnoredOwners[lowerF] = append(modIgnoredOwners[lowerF], p)
					}
					continue
				}

				firstOwner, ok := fileFirstOwner[lowerF]
				if !ok {
					fileFirstOwner[lowerF] = p
					continue
				}
				if firstOwner == p {
					continue
				}

				owners := conflictOwners[lowerF]
				if len(owners) == 0 {
					conflictOwners[lowerF] = []string{firstOwner, p}
					continue
				}
				if !containsString(owners, p) {
					conflictOwners[lowerF] = append(owners, p)
				}
			}
			mu.Unlock()
		})
	}

	wg.Wait()

	// 分析冲突
	a.emitConflictProgress(ProgressInfo{
		Current: totalFiles,
		Total:   totalFiles,
		Message: "正在整理冲突结果...",
	})

	// VPK组合 -> 冲突 / 覆盖摘要
	// key: "vpkFullPath1|vpkFullPath2" (sorted)
	conflictMap := make(map[string]*conflictGroupAccumulator)
	overrideMap := make(map[string]*conflictOverrideAccumulator)

	// 只有开启优先级感知时才读取 addonlist 与分层记录，默认路径保持零额外 I/O。
	var loadOrder conflictLoadOrderTable
	var priorityLayers modPriorityLayers
	if req.priorityAware {
		loadOrder = a.conflictLoadOrderTable()
		priorityLayers = a.loadModPriorityLayersBestEffort()
	}

	for f, vpks := range conflictOwners {
		if req.selectedPaths != nil {
			vpks = scopedConflictOwners(vpks, targetSet, baselineSet)
			if len(vpks) < 2 {
				continue
			}
		}
		sort.Strings(vpks)

		if req.priorityAware {
			decision := decideConflictOwners(conflictOwnersFromPaths(rootDir, vpks, loadOrder, priorityLayers))
			switch decision.Kind {
			case conflictDecisionIgnore:
				continue
			case conflictDecisionOverride:
				ownerPaths := conflictOwnerPaths(decision.Owners)
				key := strings.Join(ownerPaths, "|")
				acc, ok := overrideMap[key]
				if !ok {
					acc = &conflictOverrideAccumulator{
						files:      make([]string, 0, min(conflictGroupFileListLimit, 16)),
						severity:   "info",
						winnerPath: decision.WinnerPath,
					}
					overrideMap[key] = acc
				}
				acc.fileCount++
				if len(acc.files) < conflictGroupFileListLimit {
					acc.files = append(acc.files, f)
				}
				if s := getConflictSeverity(f); getConflictSeverityRank(s) > getConflictSeverityRank(acc.severity) {
					acc.severity = s
				}
				continue
			default:
				// 未判定的重叠仍按冲突处理，但参与者已剔除未加载条目。
				vpks = conflictOwnerPaths(decision.Owners)
				if len(vpks) < 2 {
					continue
				}
			}
		}

		key := strings.Join(vpks, "|")
		acc, ok := conflictMap[key]
		if !ok {
			acc = &conflictGroupAccumulator{
				files:    make([]string, 0, min(conflictGroupFileListLimit, 16)),
				severity: "info",
			}
			if req.priorityAware {
				owners := conflictOwnersFromPaths(rootDir, vpks, loadOrder, priorityLayers)
				decision := decideConflictOwners(owners)
				if decision.Kind == conflictDecisionConflict && decision.Layer != nil {
					layer := *decision.Layer
					acc.layer = &layer
				}
			}
			conflictMap[key] = acc
		}
		acc.fileCount++
		if len(acc.files) < conflictGroupFileListLimit {
			acc.files = append(acc.files, f)
		}
		if s := getConflictSeverity(f); getConflictSeverityRank(s) > getConflictSeverityRank(acc.severity) {
			acc.severity = s
		}
	}

	var groups []ConflictGroup
	for key, acc := range conflictMap {
		files := acc.files
		vpkFullPaths := strings.Split(key, "|")
		sort.Strings(files) // 文件列表也排序

		groups = append(groups, ConflictGroup{
			VpkFiles:       a.conflictVPKFileInfos(rootDir, vpkFullPaths, loadOrder, priorityLayers),
			Files:          files,
			FileCount:      acc.fileCount,
			FilesTruncated: acc.fileCount > len(files),
			Severity:       acc.severity,
			Layer:          acc.layer,
		})
	}

	overrideGroups := make([]ConflictOverrideGroup, 0, len(overrideMap))
	for key, acc := range overrideMap {
		files := acc.files
		vpkFullPaths := strings.Split(key, "|")
		sort.Strings(files) // 文件列表也排序

		vpkInfos := a.conflictVPKFileInfos(rootDir, vpkFullPaths, loadOrder, priorityLayers)
		winner := ConflictVPKFile{Order: -1, Layer: -1}
		for _, info := range vpkInfos {
			if strings.EqualFold(filepath.Clean(info.Path), filepath.Clean(acc.winnerPath)) {
				winner = info
				break
			}
		}

		overrideGroups = append(overrideGroups, ConflictOverrideGroup{
			VpkFiles:       vpkInfos,
			Winner:         winner,
			Files:          files,
			FileCount:      acc.fileCount,
			FilesTruncated: acc.fileCount > len(files),
			Severity:       acc.severity,
		})
	}

	// 按严重程度和文件数量排序冲突组与覆盖组
	sort.Slice(groups, func(i, j int) bool {
		return conflictGroupLess(groups[i].Severity, groups[i].FileCount, groups[j].Severity, groups[j].FileCount)
	})
	sort.Slice(overrideGroups, func(i, j int) bool {
		return conflictGroupLess(overrideGroups[i].Severity, overrideGroups[i].FileCount, overrideGroups[j].Severity, overrideGroups[j].FileCount)
	})

	annotations := a.conflictModIgnoreAnnotations(rootDir, fileFirstOwner, modIgnoredOwners, loadOrder, priorityLayers)

	return &ConflictResult{
		TotalConflicts:            len(groups),
		ConflictGroups:            groups,
		TotalOverrides:            len(overrideGroups),
		OverrideGroups:            overrideGroups,
		ModIgnoreAnnotations:      annotations,
		TotalModIgnoreAnnotations: len(annotations),
	}, nil
}

// conflictModIgnoreAnnotationLimit 限制单次检测返回的标注数量，
// 避免一份过宽的忽略清单把结果刷屏；总数仍通过 TotalModIgnoreAnnotations 给出。
const conflictModIgnoreAnnotationLimit = 200

// conflictModIgnoreAnnotations 把"被自身规则忽略"的重叠整理成稳定顺序的标注：
// 只保留确实被其它 Mod 提供、且被该 Mod 自己跳过的路径。
func (a *App) conflictModIgnoreAnnotations(
	rootDir string,
	fileFirstOwner map[string]string,
	modIgnoredOwners map[string][]string,
	loadOrder conflictLoadOrderTable,
	layers modPriorityLayers,
) []ConflictModIgnoreAnnotation {
	if len(modIgnoredOwners) == 0 {
		return nil
	}
	paths := make([]string, 0, len(modIgnoredOwners))
	for path := range modIgnoredOwners {
		if _, provided := fileFirstOwner[path]; !provided {
			// 没有任何其它 Mod 提供该文件时不构成"重叠"，不产生噪音。
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil
	}
	sort.Strings(paths)
	if len(paths) > conflictModIgnoreAnnotationLimit {
		paths = paths[:conflictModIgnoreAnnotationLimit]
	}

	annotations := make([]ConflictModIgnoreAnnotation, 0, len(paths))
	for _, path := range paths {
		owners := append([]string(nil), modIgnoredOwners[path]...)
		sort.Strings(owners)
		annotations = append(annotations, ConflictModIgnoreAnnotation{
			File:     path,
			VpkFiles: a.conflictVPKFileInfos(rootDir, owners, loadOrder, layers),
		})
	}
	return annotations
}

// conflictVPKFileInfos 从缓存（或兜底元数据）构造前端展示用的 VPK 信息，
// 并在优先级感知模式下附带 addonlist 顺序号与有效分层。
func (a *App) conflictVPKFileInfos(rootDir string, fullPaths []string, loadOrder conflictLoadOrderTable, layers modPriorityLayers) []ConflictVPKFile {
	infos := make([]ConflictVPKFile, 0, len(fullPaths))
	for _, fullPath := range fullPaths {
		var info ConflictVPKFile
		if cached, ok := a.vpkCache.Load(fullPath); ok {
			if cache, valid := cached.(*VPKFileCache); valid && cache != nil {
				info = newConflictVPKFile(cache.File.Name, cache.File.Path, cache.File.Title, cache.File.Location)
			}
		}
		if info.Path == "" {
			// 缓存不存在时的兜底处理
			info = newConflictVPKFile(filepath.Base(fullPath), fullPath, filepath.Base(fullPath), a.getLocationFromPath(fullPath))
		}
		info.Order = -1
		info.Layer = -1
		if entry, known := loadOrder.ForPath(rootDir, fullPath); known {
			info.Order = entry.Index
			if key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, fullPath); err == nil {
				info.Tier = layers.tierFor(key)
				info.Layer = layers.effective(key, entry.Index)
			} else {
				info.Layer = entry.Index
			}
		}
		infos = append(infos, info)
	}
	return infos
}

func defaultConflictBaselineRules() []ConflictBaselineRule {
	return []ConflictBaselineRule{{Type: "enabled"}}
}

func normalizeConflictBaselineRules(rules []ConflictBaselineRule, matchMode string) ([]ConflictBaselineRule, string, error) {
	mode := strings.ToLower(strings.TrimSpace(matchMode))
	if mode == "" {
		mode = "or"
	}
	if mode != "and" && mode != "or" {
		return nil, "", fmt.Errorf("不支持的冲突分析条件组合方式: %s", matchMode)
	}

	if len(rules) == 0 {
		return defaultConflictBaselineRules(), mode, nil
	}

	result := make([]ConflictBaselineRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		typeName := strings.ToLower(strings.TrimSpace(rule.Type))
		typeName = strings.ReplaceAll(typeName, "-", "_")
		switch typeName {
		case "enabled", "not_disabled", "root", "workshop":
			// no extra value required
		case "tag":
			rule.Value = strings.TrimSpace(rule.Value)
			if rule.Value == "" {
				return nil, "", fmt.Errorf("“拥有标签”条件需要选择一个标签")
			}
		default:
			return nil, "", fmt.Errorf("不支持的冲突分析条件: %s", rule.Type)
		}

		key := typeName + "\x00" + strings.ToLower(rule.Value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, ConflictBaselineRule{Type: typeName, Value: rule.Value})
	}
	if len(result) == 0 {
		return defaultConflictBaselineRules(), mode, nil
	}
	return result, mode, nil
}

// collectEnabledConflictVPKPaths is retained for package callers and tests.
// It returns the historical baseline: only archives currently game-enabled and
// physically outside disabled.
func (a *App) collectEnabledConflictVPKPaths() []string {
	return a.collectConflictBaselineVPKPaths(defaultConflictBaselineRules(), "or")
}

func (a *App) collectConflictBaselineVPKPaths(rules []ConflictBaselineRule, matchMode string) []string {
	// Do not rely solely on vpkCache here. A user can click conflict analysis
	// while the initial directory scan is still running; in that case the old
	// implementation silently produced an incomplete baseline. Enumerate the
	// filesystem first, then enrich entries from cache (or a minimal metadata
	// parse for tag rules).
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		// Package tests and headless callers may provide only the metadata cache.
		// Preserve that supported fallback when no directory has been selected.
		paths := make([]string, 0)
		a.vpkCache.Range(func(key, value interface{}) bool {
			cache, ok := value.(*VPKFileCache)
			if ok && cache != nil && strings.HasSuffix(strings.ToLower(cache.File.Path), ".vpk") && conflictBaselineMatches(cache.File, rules, matchMode) {
				paths = append(paths, cache.File.Path)
			}
			return true
		})
		return dedupeConflictPaths(paths)
	}
	allPaths := collectConflictFilesystemPaths(rootDir)
	stateMap := a.conflictAddonListStateMap()
	needsTags := false
	for _, rule := range rules {
		if strings.EqualFold(rule.Type, "tag") {
			needsTags = true
			break
		}
	}

	paths := make([]string, 0, len(allPaths))
	for _, path := range allPaths {
		file := a.conflictBaselineFile(path, rootDir, stateMap, needsTags)
		if conflictBaselineMatches(file, rules, matchMode) {
			paths = append(paths, path)
		}
	}
	return dedupeConflictPaths(paths)
}

func collectConflictFilesystemPaths(rootDir string) []string {
	paths := make([]string, 0)
	dirs := []struct {
		path      string
		recursive bool
	}{
		{path: rootDir, recursive: false},
		{path: filepath.Join(rootDir, "workshop"), recursive: true},
		{path: filepath.Join(rootDir, "disabled"), recursive: true},
	}
	for _, item := range dirs {
		dir := item.path
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		if !item.recursive {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
					paths = append(paths, filepath.Join(dir, entry.Name()))
				}
			}
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
				paths = append(paths, path)
			}
			return nil
		})
	}
	return dedupeConflictPaths(paths)
}

func (a *App) conflictAddonListStateMap() map[string]bool {
	list, _, err := a.readAddonList()
	if err != nil {
		return map[string]bool{}
	}
	return addonListStateMap(list)
}

func (a *App) conflictBaselineFile(path, rootDir string, stateMap map[string]bool, needsTags bool) VPKFile {
	location := "root"
	if rel, err := filepath.Rel(rootDir, path); err == nil {
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 0 {
			switch strings.ToLower(parts[0]) {
			case "workshop":
				location = "workshop"
			case "disabled":
				location = "disabled"
			}
		}
	}
	file := VPKFile{Name: filepath.Base(path), Path: path, Location: location, Enabled: location != "disabled"}
	if key, err := addonListKeyForRootAndPath(rootDir, path); err == nil {
		if enabled, ok := stateMap[key]; ok {
			file.GameEnabled = enabled
			file.GameStateKnown = true
		}
	}
	if cached, ok := a.vpkCache.Load(path); ok {
		if cache, valid := cached.(*VPKFileCache); valid && cache != nil {
			file = cache.File
			file.Path = path
			file.Location = location
			file.Enabled = location != "disabled"
			if key, err := addonListKeyForRootAndPath(rootDir, path); err == nil {
				if enabled, ok := stateMap[key]; ok {
					file.GameEnabled = enabled
					file.GameStateKnown = true
				} else {
					file.GameEnabled = false
					file.GameStateKnown = false
				}
			}
			return file
		}
	}
	if needsTags {
		if parsed, err := parser.ParseVPKFileMetadata(path); err == nil && parsed != nil {
			file.PrimaryTag = parsed.PrimaryTag
			file.SecondaryTags = parsed.SecondaryTags
		}
		if meta, err := LoadWorkshopMeta(path); err == nil && meta != nil {
			if meta.PrimaryTag != "" {
				file.PrimaryTag = meta.PrimaryTag
			}
			if len(meta.SecondaryTags) > 0 {
				file.SecondaryTags = meta.SecondaryTags
			}
			if len(meta.Tags) > 0 && file.PrimaryTag == "" && len(file.SecondaryTags) == 0 {
				file.PrimaryTag = meta.Tags[0]
				file.SecondaryTags = meta.Tags[1:]
			}
		}
	}
	return file
}

func addonListKeyForRootAndPath(rootDir, filePath string) (string, error) {
	return addonListKeyForManagedVPKPathFromRoot(rootDir, filePath)
}

func conflictBaselineMatches(file VPKFile, rules []ConflictBaselineRule, matchMode string) bool {
	if len(rules) == 0 {
		rules = defaultConflictBaselineRules()
	}

	matchAll := strings.EqualFold(matchMode, "and")
	for _, rule := range rules {
		matched := false
		switch rule.Type {
		case "enabled":
			matched = file.GameStateKnown && file.GameEnabled && file.Location != "disabled"
		case "not_disabled":
			matched = file.Location != "disabled"
		case "root":
			matched = file.Location == "root"
		case "workshop":
			matched = file.Location == "workshop"
		case "tag":
			matched = conflictFileHasTag(file, rule.Value)
		}

		if matchAll && !matched {
			return false
		}
		if !matchAll && matched {
			return true
		}
	}
	return matchAll
}

func conflictFileHasTag(file VPKFile, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(file.PrimaryTag), wanted) {
		return true
	}
	for _, tag := range file.SecondaryTags {
		if strings.EqualFold(strings.TrimSpace(tag), wanted) {
			return true
		}
	}
	return false
}

func pathSet(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		set[strings.ToLower(filepath.Clean(path))] = struct{}{}
	}
	return set
}

func mergeConflictPaths(groups ...[]string) []string {
	paths := make([]string, 0)
	for _, group := range groups {
		paths = append(paths, group...)
	}
	return dedupeConflictPaths(paths)
}

func dedupeConflictPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		key := strings.ToLower(clean)
		if clean == "." || key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, clean)
	}
	sort.Strings(result)
	return result
}

// scopedConflictOwners keeps only owners that satisfy the selected baseline
// rules. A scoped result is useful only when at least one of those matching
// owners is also a selected target; this prevents conflicts between unrelated
// baseline Mods from appearing when the current filter contains no matching
// Mod. Because target membership alone is not enough, a disabled/otherwise
// non-matching target is never shown or used to qualify a conflict group.
func scopedConflictOwners(owners []string, targets, baseline map[string]struct{}) []string {
	result := make([]string, 0, len(owners))
	seen := make(map[string]struct{}, len(owners))
	hasMatchingTarget := false
	for _, owner := range owners {
		key := strings.ToLower(filepath.Clean(owner))
		if _, isBaseline := baseline[key]; !isBaseline {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, owner)
		if _, isTarget := targets[key]; isTarget {
			hasMatchingTarget = true
		}
	}
	if !hasMatchingTarget || len(result) < 2 {
		return nil
	}
	return result
}

func (a *App) getConflictFileList(filePath string) ([]string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	a.conflictIndexMu.Lock()
	if a.conflictIndexCache != nil {
		if cached, ok := a.conflictIndexCache[filePath]; ok && cached.Size == info.Size() && cached.ModTime.Equal(info.ModTime()) {
			files := append([]string(nil), cached.Files...)
			cached.LastUsed = time.Now()
			a.conflictIndexCache[filePath] = cached
			a.conflictIndexMu.Unlock()
			return files, nil
		}
	}
	a.conflictIndexMu.Unlock()

	files, err := getVPKFileListSafely(filePath)
	if err != nil {
		return nil, err
	}
	entry := conflictIndexCacheEntry{ModTime: info.ModTime(), Size: info.Size(), Files: append([]string(nil), files...), LastUsed: time.Now()}
	a.conflictIndexMu.Lock()
	if a.conflictIndexCache == nil {
		a.conflictIndexCache = make(map[string]conflictIndexCacheEntry)
	}
	if len(a.conflictIndexCache) >= conflictIndexCacheMax {
		// Evict only the least recently used entry. A full reset caused cache
		// thrashing whenever a collection exceeded the old 512-entry limit.
		var oldestKey string
		var oldest time.Time
		for key, cached := range a.conflictIndexCache {
			if oldestKey == "" || cached.LastUsed.Before(oldest) {
				oldestKey = key
				oldest = cached.LastUsed
			}
		}
		if oldestKey != "" {
			delete(a.conflictIndexCache, oldestKey)
		}
	}
	a.conflictIndexCache[filePath] = entry
	a.conflictIndexMu.Unlock()
	return files, nil
}

func (a *App) collectConflictVPKPaths(selectedPaths []string) ([]string, error) {
	a.mu.RLock()
	rootDir := a.rootDir
	a.mu.RUnlock()
	if rootDir == "" {
		return nil, fmt.Errorf("未选择L4D2目录")
	}

	addonsDir := filepath.Clean(rootDir)
	if len(selectedPaths) == 0 {
		var paths []string
		for _, dir := range []string{addonsDir, filepath.Join(addonsDir, "workshop")} {
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
					paths = append(paths, filepath.Join(dir, entry.Name()))
				}
			}
		}
		sort.Strings(paths)
		return paths, nil
	}

	paths := make([]string, 0, len(selectedPaths))
	seen := make(map[string]struct{}, len(selectedPaths))
	for _, rawPath := range selectedPaths {
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(addonsDir, path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("无法解析 Mod 路径 %q: %w", rawPath, err)
		}
		if !isPathWithin(path, addonsDir) {
			return nil, fmt.Errorf("Mod 路径不在当前 addons 目录内: %s", rawPath)
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, statErr
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".vpk") {
			continue
		}
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func isPathWithin(path, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func newConflictVPKFile(name, path, title, location string) ConflictVPKFile {
	if title == "" {
		title = name
	}
	return ConflictVPKFile{
		Name:     name,
		Path:     path,
		Title:    title,
		Location: location,
		Order:    -1,
		Layer:    -1,
	}
}
