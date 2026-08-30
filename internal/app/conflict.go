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
}

type ConflictGroup struct {
	VpkFiles       []ConflictVPKFile `json:"vpk_files"`
	Files          []string          `json:"files"`
	FileCount      int               `json:"file_count"`
	FilesTruncated bool              `json:"files_truncated"`
	Severity       string            `json:"severity"` // "critical", "warning", "info"
}

type ConflictResult struct {
	TotalConflicts int             `json:"total_conflicts"`
	ConflictGroups []ConflictGroup `json:"conflict_groups"`
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
	return a.checkConflicts(nil, nil, "or")
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
	return a.checkConflicts(paths, defaultConflictBaselineRules(), "or")
}

// CheckConflictsWithOptions checks the selected target Mods against a baseline
// chosen by the caller. It reuses the VPK metadata and archive file-list
// caches, so changing a range normally avoids reparsing unchanged archives.
func (a *App) CheckConflictsWithOptions(options ConflictAnalysisOptions) (*ConflictResult, error) {
	if len(options.TargetPaths) == 0 {
		return &ConflictResult{}, nil
	}
	if len(options.TargetPaths) > scopedConflictMaxVPKs {
		return nil, fmt.Errorf("当前筛选包含 %d 个 Mod；当前最多支持 %d 个目标，请缩小筛选范围后再分析", len(options.TargetPaths), scopedConflictMaxVPKs)
	}

	rules, matchMode, err := normalizeConflictBaselineRules(options.BaselineRules, options.MatchMode)
	if err != nil {
		return nil, err
	}
	return a.checkConflicts(options.TargetPaths, rules, matchMode)
}

func (a *App) checkConflicts(selectedPaths []string, baselineRules []ConflictBaselineRule, matchMode string) (*ConflictResult, error) {
	rootDir := a.rootDirectorySnapshot()

	if rootDir == "" {
		return nil, fmt.Errorf("未选择L4D2目录")
	}

	if !a.conflictCheckMu.TryLock() {
		return nil, fmt.Errorf("冲突检测正在进行中，请稍候")
	}
	defer a.conflictCheckMu.Unlock()

	var vpkPaths []string
	var targetSet map[string]struct{}
	var baselineSet map[string]struct{}
	if selectedPaths != nil {
		targetPaths, err := a.collectConflictVPKPaths(selectedPaths)
		if err != nil {
			return nil, err
		}
		baselinePaths := a.collectConflictBaselineVPKPaths(baselineRules, matchMode)
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
	var mu sync.Mutex
	var wg sync.WaitGroup
	workerCount := min(conflictWorkerLimit, rt.GOMAXPROCS(0))
	if workerCount < 1 {
		workerCount = 1
	}
	workerSlots := make(chan struct{}, workerCount)

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
				if isIgnoredConflictFile(lowerF) {
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

	// VPK组合 -> 冲突摘要
	// key: "vpkFullPath1|vpkFullPath2" (sorted)
	conflictMap := make(map[string]*conflictGroupAccumulator)

	for f, vpks := range conflictOwners {
		if selectedPaths != nil {
			vpks = scopedConflictOwners(vpks, targetSet, baselineSet)
			if len(vpks) < 2 {
				continue
			}
		}
		sort.Strings(vpks)
		key := strings.Join(vpks, "|")
		acc, ok := conflictMap[key]
		if !ok {
			acc = &conflictGroupAccumulator{
				files:    make([]string, 0, min(conflictGroupFileListLimit, 16)),
				severity: "info",
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

		// 从缓存获取完整VPK信息
		vpkInfos := make([]ConflictVPKFile, 0, len(vpkFullPaths))
		for _, fullPath := range vpkFullPaths {
			if cached, ok := a.vpkCache.Load(fullPath); ok {
				cache := cached.(*VPKFileCache)
				vpkInfos = append(vpkInfos, newConflictVPKFile(cache.File.Name, cache.File.Path, cache.File.Title, cache.File.Location))
			} else {
				// 缓存不存在时的兜底处理
				vpkInfos = append(vpkInfos, newConflictVPKFile(filepath.Base(fullPath), fullPath, filepath.Base(fullPath), a.getLocationFromPath(fullPath)))
			}
		}

		groups = append(groups, ConflictGroup{
			VpkFiles:       vpkInfos,
			Files:          files,
			FileCount:      acc.fileCount,
			FilesTruncated: acc.fileCount > len(files),
			Severity:       acc.severity,
		})
	}

	// 按严重程度和冲突数量排序 groups
	sort.Slice(groups, func(i, j int) bool {
		// 严重程度优先级: critical > warning > info
		si := getConflictSeverityRank(groups[i].Severity)
		sj := getConflictSeverityRank(groups[j].Severity)

		if si != sj {
			return si > sj
		}
		return groups[i].FileCount > groups[j].FileCount
	})

	return &ConflictResult{
		TotalConflicts: len(groups),
		ConflictGroups: groups,
	}, nil
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
	}
}
