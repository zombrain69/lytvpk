package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	modHealthKindDependencyMissing  = "dependency_missing"
	modHealthKindDependencyDisabled = "dependency_disabled"
	// modHealthKindDependencyCycle 对齐 FireAxe 的 AddonCircularRefProblem：
	// A 依赖 B、B 又依赖 A（或更长的环）时，配置本身是自相矛盾的。
	modHealthKindDependencyCycle = "dependency_cycle"
)

// ModDependencyRef 指向一个依赖项，按 addonlist 键匹配。
type ModDependencyRef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ModDependencyRecord 记录某个 Mod 声明的依赖。依赖由用户声明，
// LytVPK 不会自动推断。
type ModDependencyRecord struct {
	Key          string             `json:"key"`
	Name         string             `json:"name"`
	Dependencies []ModDependencyRef `json:"dependencies"`
	UpdatedAt    string             `json:"updatedAt"`
}

// ModDependencyEnableResult 描述一次“启用依赖”的结果。
type ModDependencyEnableResult struct {
	MasterName     string   `json:"masterName"`
	MasterCount    int      `json:"masterCount"`
	Enabled        []string `json:"enabled"`
	AlreadyEnabled []string `json:"alreadyEnabled"`
	Missing        []string `json:"missing"`
}

type modDependencyStore struct {
	Records []ModDependencyRecord `json:"records"`
}

func (a *App) ensureDependenciesPath() string {
	a.ensureConfigPaths()
	if a.dependenciesPath == "" && a.configDir != "" {
		a.dependenciesPath = filepath.Join(a.configDir, "dependencies.json")
	}
	return a.dependenciesPath
}

func (a *App) readModDependencyStore() (modDependencyStore, error) {
	path := a.ensureDependenciesPath()
	if path == "" {
		return modDependencyStore{}, fmt.Errorf("未配置配置目录，无法读写依赖")
	}
	var store modDependencyStore
	if err := readJSONFile(path, &store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return modDependencyStore{}, nil
		}
		return modDependencyStore{}, fmt.Errorf("无法读取依赖列表: %w", err)
	}
	return store, nil
}

func (a *App) writeModDependencyStore(store modDependencyStore) error {
	path := a.ensureDependenciesPath()
	if path == "" {
		return fmt.Errorf("未配置配置目录，无法保存依赖")
	}
	if store.Records == nil {
		store.Records = []ModDependencyRecord{}
	}
	a.backupLocalStoreFileIfNeeded(path)
	return writeJSONFile(a.configDir, path, store)
}

// ListModDependencies 返回已声明的依赖记录。
func (a *App) ListModDependencies() ([]ModDependencyRecord, error) {
	a.dependenciesMu.Lock()
	defer a.dependenciesMu.Unlock()

	store, err := a.readModDependencyStore()
	if err != nil {
		return nil, err
	}
	return append([]ModDependencyRecord(nil), store.Records...), nil
}

// SetModDependencies 覆盖某个 Mod 的依赖声明；依赖为空表示删除该记录。
func (a *App) SetModDependencies(targetPath string, dependencyPaths []string) (ModDependencyRecord, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModDependencyRecord{}, fmt.Errorf("未选择L4D2目录")
	}
	target, err := modStrategyGroupMemberFromPath(rootDir, targetPath)
	if err != nil {
		return ModDependencyRecord{}, fmt.Errorf("无法识别主 Mod: %w", err)
	}

	dependencies := make([]ModDependencyRef, 0, len(dependencyPaths))
	seen := map[string]bool{target.Key: true}
	for _, rawPath := range dependencyPaths {
		if strings.TrimSpace(rawPath) == "" {
			continue
		}
		member, memberErr := modStrategyGroupMemberFromPath(rootDir, rawPath)
		if memberErr != nil {
			continue
		}
		if seen[member.Key] {
			continue
		}
		seen[member.Key] = true
		dependencies = append(dependencies, ModDependencyRef{Key: member.Key, Name: member.Name})
	}

	record := ModDependencyRecord{
		Key:          target.Key,
		Name:         target.Name,
		Dependencies: dependencies,
		UpdatedAt:    time.Now().Format(time.RFC3339),
	}

	a.dependenciesMu.Lock()
	defer a.dependenciesMu.Unlock()
	store, err := a.readModDependencyStore()
	if err != nil {
		return ModDependencyRecord{}, err
	}

	next := make([]ModDependencyRecord, 0, len(store.Records)+1)
	replaced := false
	for _, existing := range store.Records {
		if existing.Key == record.Key {
			replaced = true
			continue
		}
		next = append(next, existing)
	}
	if len(dependencies) > 0 {
		next = append(next, record)
	}
	if !replaced && len(dependencies) == 0 {
		// 之前本来就没有记录：什么都不用写。
		return record, nil
	}
	store.Records = next
	if err := a.writeModDependencyStore(store); err != nil {
		return ModDependencyRecord{}, fmt.Errorf("无法保存依赖: %w", err)
	}
	return record, nil
}

type modDependencyPlan struct {
	results map[string]*ModDependencyEnableResult
	enable  []ModDependencyRef
}

// buildModDependencyPlan 计算需要开启的依赖。
// onlyMasterKey 非空时只处理该主 Mod；requireMasterEnabled 为真时跳过已关闭的主 Mod。
func buildModDependencyPlan(records []ModDependencyRecord, states map[string]bool, rootDir string, onlyMasterKey string, requireMasterEnabled bool) modDependencyPlan {
	plan := modDependencyPlan{results: make(map[string]*ModDependencyEnableResult, len(records))}
	if len(records) == 0 {
		return plan
	}
	queued := make(map[string]bool, 8)

	for _, record := range records {
		if onlyMasterKey != "" && record.Key != onlyMasterKey {
			continue
		}
		if requireMasterEnabled {
			if enabled, known := states[record.Key]; known && !enabled {
				continue
			}
		}

		result := &ModDependencyEnableResult{MasterName: record.Name, MasterCount: 1}
		for _, dependency := range record.Dependencies {
			if !modHealthEntryExists(rootDir, dependency.Name) {
				result.Missing = append(result.Missing, dependency.Name)
				continue
			}
			if enabled, known := states[dependency.Key]; known && enabled {
				result.AlreadyEnabled = append(result.AlreadyEnabled, dependency.Name)
				continue
			}
			result.Enabled = append(result.Enabled, dependency.Name)
			if !queued[dependency.Key] {
				queued[dependency.Key] = true
				plan.enable = append(plan.enable, dependency)
			}
		}
		plan.results[record.Key] = result
	}
	return plan
}

// applyModDependencyEnablement 把计划中的依赖写回 addonlist.txt（未记录的条目按插入位置补写）。
func (a *App) applyModDependencyEnablement(plan modDependencyPlan) error {
	if len(plan.enable) == 0 {
		return nil
	}

	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	doc, err := a.readAddonListDocument()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("无法读取 addonlist.txt: %w", err)
		}
		path, pathErr := a.addonListPath()
		if pathErr != nil {
			return pathErr
		}
		doc = addonListDocument{path: path, content: "\"AddonList\"\n{\n}\n", encoding: addonListEncodingUTF8}
	}

	placement := normalizeAddonListUnrecordedPlacement(a.unrecordedModLoadOrderPlacement)
	updated := doc.content
	changed := false
	for _, ref := range plan.enable {
		var replaced bool
		updated, replaced, err = replaceAddonListValueWithPlacementAndName(updated, ref.Key, ref.Name, "1", placement)
		if err != nil {
			return fmt.Errorf("无法更新 addonlist.txt 条目 %s: %w", ref.Key, err)
		}
		changed = changed || replaced
	}
	if !changed {
		return nil
	}
	// 与其它自动修复保持一致：改 addonlist.txt 之前先留一份带来源标记的可恢复备份。
	if _, err := a.createAddonListPreWriteBackupLocked(doc.path, "before-dependency-fix"); err != nil {
		return err
	}
	// 事务化提交：写盘 → 刷新内存开关状态 → 快照同步（失败回滚）。
	if err := a.commitAddonListDocumentLocked(doc, updated, a.applyAddonListGameStates); err != nil {
		return fmt.Errorf("无法提交 addonlist.txt: %w", err)
	}
	return nil
}

// EnableModDependencies 开启某个主 Mod 的全部依赖（文件缺失的依赖会被跳过并报告）。
// target 可以是 VPK 路径（来自选择）或 addonlist 键（来自依赖列表）。
func (a *App) EnableModDependencies(targetPath string) (ModDependencyEnableResult, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModDependencyEnableResult{}, fmt.Errorf("未选择L4D2目录")
	}
	target, err := resolveModDependencyTarget(rootDir, targetPath)
	if err != nil {
		return ModDependencyEnableResult{}, fmt.Errorf("无法识别主 Mod: %w", err)
	}

	records, err := a.ListModDependencies()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}

	plan := buildModDependencyPlan(records, states, rootDir, target.Key, false)
	result, ok := plan.results[target.Key]
	if !ok {
		return ModDependencyEnableResult{}, fmt.Errorf("%s 还没有声明依赖", target.Name)
	}
	if err := a.applyModDependencyEnablement(plan); err != nil {
		return ModDependencyEnableResult{}, err
	}
	return *result, nil
}

// DeleteModDependencies 删除某个主 Mod 的依赖声明。
func (a *App) DeleteModDependencies(target string) error {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return fmt.Errorf("未选择L4D2目录")
	}
	resolved, err := resolveModDependencyTarget(rootDir, target)
	if err != nil {
		return err
	}

	a.dependenciesMu.Lock()
	defer a.dependenciesMu.Unlock()
	store, err := a.readModDependencyStore()
	if err != nil {
		return err
	}
	next := make([]ModDependencyRecord, 0, len(store.Records))
	found := false
	for _, record := range store.Records {
		if record.Key == resolved.Key {
			found = true
			continue
		}
		next = append(next, record)
	}
	if !found {
		return fmt.Errorf("%s 还没有声明依赖", resolved.Name)
	}
	store.Records = next
	return a.writeModDependencyStore(store)
}

// resolveModDependencyTarget 接受路径或 addonlist 键，统一解析成主 Mod 标识。
func resolveModDependencyTarget(rootDir string, input string) (ModStrategyGroupMember, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ModStrategyGroupMember{}, fmt.Errorf("缺少主 Mod 标识")
	}
	if member, err := modStrategyGroupMemberFromPath(rootDir, input); err == nil {
		return member, nil
	}
	key := normalizeAddonListKey(input)
	if key == "" {
		return ModStrategyGroupMember{}, fmt.Errorf("无法识别主 Mod: %s", input)
	}
	return ModStrategyGroupMember{Key: key, Name: input}, nil
}

// EnableAllMissingModDependencies 为所有“处于开启状态”的主 Mod 补齐依赖。
func (a *App) EnableAllMissingModDependencies() (ModDependencyEnableResult, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModDependencyEnableResult{}, fmt.Errorf("未选择L4D2目录")
	}

	records, err := a.ListModDependencies()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}

	plan := buildModDependencyPlan(records, states, rootDir, "", true)
	combined := ModDependencyEnableResult{}
	masterKeys := make([]string, 0, len(plan.results))
	for key := range plan.results {
		masterKeys = append(masterKeys, key)
	}
	sort.Strings(masterKeys)
	for _, key := range masterKeys {
		result := plan.results[key]
		if len(result.Enabled) == 0 {
			continue
		}
		combined.MasterCount++
		combined.Enabled = append(combined.Enabled, result.Enabled...)
		combined.AlreadyEnabled = append(combined.AlreadyEnabled, result.AlreadyEnabled...)
		combined.Missing = append(combined.Missing, result.Missing...)
	}
	if err := a.applyModDependencyEnablement(plan); err != nil {
		return ModDependencyEnableResult{}, err
	}
	return combined, nil
}

// checkModDependencies 在体检里提示“依赖被关闭 / 依赖文件缺失”。
// 只检查处于开启状态（或状态未记录）的主 Mod。
func (a *App) checkModDependencies(rootDir string, states map[string]bool, report *ModHealthReport) {
	records, err := a.ListModDependencies()
	if err != nil || len(records) == 0 {
		return
	}

	// 依赖成环：配置自相矛盾（对齐 FireAxe 的 AddonCircularRefProblem）。
	for _, cycle := range dependencyCycles(records) {
		names := make([]string, 0, len(cycle)+1)
		for _, key := range cycle {
			names = append(names, dependencyDisplayName(records, key))
		}
		names = append(names, names[0])
		report.addIssue(modHealthKindDependencyCycle, "warning", names[0], "", "",
			fmt.Sprintf("依赖成环：%s（互相等待）：请删掉其中一条依赖声明，否则两边都只能靠手动静止状态",
				strings.Join(names, " → ")))
	}

	for _, record := range records {
		if enabled, known := states[record.Key]; known && !enabled {
			continue
		}
		for _, dependency := range record.Dependencies {
			if !modHealthEntryExists(rootDir, dependency.Name) {
				report.addIssue(modHealthKindDependencyMissing, "warning", record.Name, "", "",
					fmt.Sprintf("%s 依赖的 %s 在磁盘上找不到：请重新下载该依赖，或移除这条依赖声明", record.Name, dependency.Name))
				continue
			}
			if enabled, known := states[dependency.Key]; known && !enabled {
				// Target 是主 Mod 的 addonlist 键：界面拿它调 EnableModDependencies，
				// 等价于 FireAxe `AddonDependencyProblem.OnAutomaticallyFix` 的 EnableAllDependencies。
				report.addIssueWithTarget(record.Key, modHealthKindDependencyDisabled, "warning", record.Name, "", "",
					fmt.Sprintf("%s 已开启，但依赖 %s 处于关闭状态：可以在体检里一键启用依赖", record.Name, dependency.Name))
			}
		}
	}
}

// dependencyDisplayName 取某个依赖键的显示名：优先用声明它的记录名，其次用依赖引用里的名字。
func dependencyDisplayName(records []ModDependencyRecord, key string) string {
	for _, record := range records {
		if normalizeAddonListKey(record.Key) != key {
			continue
		}
		if name := strings.TrimSpace(record.Name); name != "" {
			return name
		}
	}
	for _, record := range records {
		for _, dependency := range record.Dependencies {
			if normalizeAddonListKey(dependency.Key) != key {
				continue
			}
			if name := strings.TrimSpace(dependency.Name); name != "" {
				return name
			}
		}
	}
	return key
}

// dependencyCycles 找出依赖图里的所有环，每个环用一组规范化的键表示。
//
// 纯函数（不读盘、不依赖 App），方便直接测试：
//   - 环会被旋转到"最小键开头"并去重，所以 a→b→a 与 b→a→b 只报一次；
//   - 结果按环的第一个键排序，保证同样的输入永远得到同样的报告顺序；
//   - 自环（a 依赖 a）也会被找出来。
func dependencyCycles(records []ModDependencyRecord) [][]string {
	const (
		colorWhite = 0
		colorGray  = 1
		colorBlack = 2
	)

	adjacency := make(map[string][]string)
	for _, record := range records {
		key := normalizeAddonListKey(record.Key)
		if key == "" {
			continue
		}
		if _, exists := adjacency[key]; !exists {
			adjacency[key] = nil
		}
		for _, dependency := range record.Dependencies {
			dependencyKey := normalizeAddonListKey(dependency.Key)
			if dependencyKey == "" {
				continue
			}
			if _, exists := adjacency[dependencyKey]; !exists {
				adjacency[dependencyKey] = nil
			}
			adjacency[key] = append(adjacency[key], dependencyKey)
		}
	}

	color := make(map[string]int, len(adjacency))
	starts := make([]string, 0, len(adjacency))
	for key := range adjacency {
		starts = append(starts, key)
	}
	sort.Strings(starts)

	cycles := make([][]string, 0)
	seen := make(map[string]bool)

	for _, start := range starts {
		if color[start] != colorWhite {
			continue
		}

		type frame struct {
			node string
			next int
		}
		stack := []frame{{node: start}}
		color[start] = colorGray
		path := []string{start}

		for len(stack) > 0 {
			topIndex := len(stack) - 1
			node := stack[topIndex].node
			if stack[topIndex].next < len(adjacency[node]) {
				next := adjacency[node][stack[topIndex].next]
				stack[topIndex].next++
				switch color[next] {
				case colorWhite:
					color[next] = colorGray
					stack = append(stack, frame{node: next})
					path = append(path, next)
				case colorGray:
					index := -1
					for i, key := range path {
						if key == next {
							index = i
							break
						}
					}
					if index < 0 {
						continue
					}
					cycle := normalizeDependencyCycle(path[index:])
					signature := strings.Join(cycle, "->")
					if seen[signature] {
						continue
					}
					seen[signature] = true
					cycles = append(cycles, cycle)
				}
				continue
			}

			color[node] = colorBlack
			stack = stack[:topIndex]
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		}
	}

	sort.SliceStable(cycles, func(i, j int) bool {
		return strings.Join(cycles[i], "->") < strings.Join(cycles[j], "->")
	})
	return cycles
}

// normalizeDependencyCycle 把环旋转成"最小键开头"，让同一个环只有一种写法。
func normalizeDependencyCycle(cycle []string) []string {
	if len(cycle) == 0 {
		return nil
	}
	minIndex := 0
	for index, key := range cycle {
		if key < cycle[minIndex] {
			minIndex = index
		}
	}
	normalized := make([]string, 0, len(cycle))
	normalized = append(normalized, cycle[minIndex:]...)
	normalized = append(normalized, cycle[:minIndex]...)
	return normalized
}
