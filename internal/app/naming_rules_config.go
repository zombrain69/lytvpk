package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vpk-manager/internal/parser"
)

// 命名规范化的「规则配置」（路线图第 20 项 P3）。
//
// 存储：配置目录下的 naming_rules.json（纯 JSON，可手工编辑 / 备份）。两个分区：
//
//   - subjectAliases 词表替换：把内置词表的替换对象（行名 / 上位名，如「木喷」）映射为
//     自定义显示名（如「泵动霰弹枪」）。批量规范化与单文件「按分类建议填充」都会采用，
//     幂等判定以替换后的名称为准（parser.NamingOptions）。
//   - exemptions 例外表：用户裁定「这个包不要再建议改名」的文件清单（相对 Mod 根目录、
//     斜杠分隔，跨机器可移植）。批量规范化的计划阶段直接跳过（skipReason=exempt），
//     重跑规范库也不受影响。
//
// 写盘纪律：所有增删都走「读改写 + 原子替换（临时文件 + rename）」，并由 namingRulesMu
// 串行化，失败时不动内存缓存、不落半截文件。文件损坏 / 不存在按空配置处理（不影响扫描）。

const namingRulesVersion = 1

// NamingSubjectAlias 是一条词表替换：From = 内置替换对象词，To = 自定义显示名。
type NamingSubjectAlias struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// NamingExemptionEntry 是例外表里的一条：Path 相对 Mod 根目录（斜杠分隔，存储键）。
type NamingExemptionEntry struct {
	Path    string `json:"path"`
	Name    string `json:"name,omitempty"`
	Note    string `json:"note,omitempty"`
	AddedAt string `json:"addedAt,omitempty"`
}

type NamingRulesFile struct {
	Version        int                    `json:"version"`
	SubjectAliases []NamingSubjectAlias   `json:"subjectAliases,omitempty"`
	Exemptions     []NamingExemptionEntry `json:"exemptions,omitempty"`
}

// NamingExemptionView 是例外表条目的展示形态（Exists = 文件当前是否还在库里）。
type NamingExemptionView struct {
	Path    string `json:"path"`
	Name    string `json:"name,omitempty"`
	Note    string `json:"note,omitempty"`
	AddedAt string `json:"addedAt,omitempty"`
	Exists  bool   `json:"exists"`
}

// NamingRulesView 是规则配置的只读视图（设置页消费；改完任一操作都返回最新全量）。
type NamingRulesView struct {
	FilePath       string                `json:"filePath"`
	SubjectAliases []NamingSubjectAlias  `json:"subjectAliases"`
	Exemptions     []NamingExemptionView `json:"exemptions"`
}

// NamingRulesMutationResult 是增删操作的结果：计数 + 跳过明细 + 最新全量视图。
type NamingRulesMutationResult struct {
	Added   int             `json:"added"`
	Removed int             `json:"removed"`
	Skipped []string        `json:"skipped,omitempty"`
	Rules   NamingRulesView `json:"rules"`
}

// namingRulesPath 返回规则文件路径（配置目录不可用时为空串）。
func (a *App) namingRulesPath() string {
	a.ensureConfigPaths()
	a.mu.RLock()
	dir := a.configDir
	a.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	return filepath.Join(dir, "naming_rules.json")
}

// readNamingRulesFile 读取并归一化规则文件；不存在 / 损坏时按空配置处理（只记日志）。
func (a *App) readNamingRulesFile() NamingRulesFile {
	path := a.namingRulesPath()
	if path == "" {
		return NamingRulesFile{Version: namingRulesVersion}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取命名规则配置失败（按空配置处理）: %v", err)
		}
		return NamingRulesFile{Version: namingRulesVersion}
	}
	var payload NamingRulesFile
	if err := json.Unmarshal(raw, &payload); err != nil {
		log.Printf("解析命名规则配置失败（按空配置处理）: %v", err)
		return NamingRulesFile{Version: namingRulesVersion}
	}
	return normalizeNamingRulesFile(payload)
}

// normalizeNamingRulesFile 归一化：去空白、丢空条目、From 去重（后者覆盖前者并保留位置）、
// 例外路径统一斜杠并去重。
func normalizeNamingRulesFile(payload NamingRulesFile) NamingRulesFile {
	normalized := NamingRulesFile{Version: namingRulesVersion}
	for _, alias := range payload.SubjectAliases {
		from := strings.TrimSpace(alias.From)
		to := strings.TrimSpace(alias.To)
		if from == "" || to == "" {
			continue
		}
		replaced := false
		for index := range normalized.SubjectAliases {
			if normalized.SubjectAliases[index].From == from {
				normalized.SubjectAliases[index].To = to
				replaced = true
				break
			}
		}
		if !replaced {
			normalized.SubjectAliases = append(normalized.SubjectAliases, NamingSubjectAlias{From: from, To: to})
		}
	}
	seen := make(map[string]bool)
	for _, entry := range payload.Exemptions {
		key := normalizeNamingRelKey(entry.Path)
		if key == "" || seen[strings.ToLower(key)] {
			continue
		}
		seen[strings.ToLower(key)] = true
		entry.Path = key
		entry.Name = strings.TrimSpace(entry.Name)
		entry.Note = strings.TrimSpace(entry.Note)
		entry.AddedAt = strings.TrimSpace(entry.AddedAt)
		normalized.Exemptions = append(normalized.Exemptions, entry)
	}
	return normalized
}

// normalizeNamingRelKey 把存储键统一成「斜杠分隔的相对路径」形态（只做文本归一，不做语义校验）。
func normalizeNamingRelKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	trimmed = strings.TrimPrefix(trimmed, "./")
	return trimmed
}

// namingRulesSnapshot 返回内存快照（懒加载；增删操作成功后同步更新缓存）。
func (a *App) namingRulesSnapshot() NamingRulesFile {
	a.mu.RLock()
	if a.namingRulesLoaded {
		payload := a.namingRules
		a.mu.RUnlock()
		return payload
	}
	a.mu.RUnlock()

	loaded := a.readNamingRulesFile()
	a.mu.Lock()
	if !a.namingRulesLoaded {
		a.namingRules = loaded
		a.namingRulesLoaded = true
	}
	payload := a.namingRules
	a.mu.Unlock()
	return payload
}

// storeNamingRulesFile 原子写盘 + 同步内存缓存（写失败返回错误且缓存不动）。
func (a *App) storeNamingRulesFile(payload NamingRulesFile) error {
	path := a.namingRulesPath()
	if path == "" {
		return errors.New("配置目录不可用")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload = normalizeNamingRulesFile(payload)
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')

	// 原子替换：同目录临时文件 + rename（Windows 下 os.Rename 覆盖已存在目标）。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	a.mu.Lock()
	a.namingRules = payload
	a.namingRulesLoaded = true
	a.mu.Unlock()
	return nil
}

// namingOptionsSnapshot 把词表替换转成 parser 选项（供建议 / 批量计划消费）。
func (a *App) namingOptionsSnapshot() parser.NamingOptions {
	payload := a.namingRulesSnapshot()
	if len(payload.SubjectAliases) == 0 {
		return parser.NamingOptions{}
	}
	aliases := make(map[string]string, len(payload.SubjectAliases))
	for _, alias := range payload.SubjectAliases {
		aliases[alias.From] = alias.To
	}
	return parser.NamingOptions{SubjectAliases: aliases}
}

// namingRelKeyFor 把受管目录内的绝对路径转成存储键（相对 Mod 根目录、斜杠分隔）。
func (a *App) namingRelKeyFor(path string) (string, error) {
	root := strings.TrimSpace(a.rootDirectorySnapshot())
	if root == "" {
		return "", errors.New("Mod 目录未设置")
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("文件不在受管 Mod 目录内")
	}
	return normalizeNamingRelKey(rel), nil
}

// namingExemptKeySet 返回例外表键集合（小写匹配）。
func (a *App) namingExemptKeySet() map[string]bool {
	payload := a.namingRulesSnapshot()
	if len(payload.Exemptions) == 0 {
		return nil
	}
	keys := make(map[string]bool, len(payload.Exemptions))
	for _, entry := range payload.Exemptions {
		keys[strings.ToLower(entry.Path)] = true
	}
	return keys
}

// isNamingExemptPath 判断某个受管文件是否在例外表里（批量计划阶段跳过）。
func (a *App) isNamingExemptPath(path string) bool {
	keys := a.namingExemptKeySet()
	if len(keys) == 0 {
		return false
	}
	key, err := a.namingRelKeyFor(path)
	if err != nil {
		return false
	}
	return keys[strings.ToLower(key)]
}

// buildNamingRulesView 组装只读视图（例外的 exists 用当前 Mod 根目录实测）。
func (a *App) buildNamingRulesView(payload NamingRulesFile) NamingRulesView {
	view := NamingRulesView{
		FilePath:       a.namingRulesPath(),
		SubjectAliases: make([]NamingSubjectAlias, 0, len(payload.SubjectAliases)),
		Exemptions:     make([]NamingExemptionView, 0, len(payload.Exemptions)),
	}
	view.SubjectAliases = append(view.SubjectAliases, payload.SubjectAliases...)
	root := strings.TrimSpace(a.rootDirectorySnapshot())
	for _, entry := range payload.Exemptions {
		item := NamingExemptionView{
			Path:    entry.Path,
			Name:    entry.Name,
			Note:    entry.Note,
			AddedAt: entry.AddedAt,
		}
		if root != "" {
			abs := filepath.Join(root, filepath.FromSlash(entry.Path))
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				item.Exists = true
			}
		}
		view.Exemptions = append(view.Exemptions, item)
	}
	sort.SliceStable(view.Exemptions, func(i, j int) bool { return view.Exemptions[i].Path < view.Exemptions[j].Path })
	return view
}

// ---------------------------------------------------------------------------
// 导出方法（Wails 绑定）
// ---------------------------------------------------------------------------

// GetNamingRules 读取命名规则配置（词表替换 + 例外表；只读）。
func (a *App) GetNamingRules() (NamingRulesView, error) {
	payload := a.namingRulesSnapshot()
	return a.buildNamingRulesView(payload), nil
}

// AddNamingRulesAlias 新增 / 覆盖一条词表替换（From 相同则覆盖 To）。
func (a *App) AddNamingRulesAlias(from, to string) (NamingRulesMutationResult, error) {
	result := NamingRulesMutationResult{}
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return result, errors.New("原词与替换名都不能为空")
	}
	if strings.ContainsAny(from, "\r\n") || strings.ContainsAny(to, "\r\n\t") {
		return result, errors.New("原词与替换名不能包含换行 / 制表符")
	}
	if invalid := namingAliasInvalidChars(to); invalid != "" {
		return result, fmt.Errorf("替换名不能包含这些字符：%s", invalid)
	}
	if from == to {
		return result, errors.New("替换名与原词相同，无需配置")
	}

	a.namingRulesMu.Lock()
	defer a.namingRulesMu.Unlock()
	payload := a.namingRulesSnapshot()
	replaced := false
	for index := range payload.SubjectAliases {
		if payload.SubjectAliases[index].From == from {
			payload.SubjectAliases[index].To = to
			replaced = true
			break
		}
	}
	if !replaced {
		payload.SubjectAliases = append(payload.SubjectAliases, NamingSubjectAlias{From: from, To: to})
	}
	if err := a.storeNamingRulesFile(payload); err != nil {
		return NamingRulesMutationResult{}, fmt.Errorf("保存命名规则失败: %w", err)
	}
	result.Added = 1
	result.Rules = a.buildNamingRulesView(a.namingRulesSnapshot())
	return result, nil
}

// RemoveNamingRulesAlias 删除一条词表替换（按 From 精确匹配）。
func (a *App) RemoveNamingRulesAlias(from string) (NamingRulesMutationResult, error) {
	result := NamingRulesMutationResult{}
	from = strings.TrimSpace(from)
	if from == "" {
		return result, errors.New("原词不能为空")
	}
	a.namingRulesMu.Lock()
	defer a.namingRulesMu.Unlock()
	payload := a.namingRulesSnapshot()
	kept := make([]NamingSubjectAlias, 0, len(payload.SubjectAliases))
	removed := 0
	for _, alias := range payload.SubjectAliases {
		if alias.From == from {
			removed++
			continue
		}
		kept = append(kept, alias)
	}
	if removed == 0 {
		return result, fmt.Errorf("词表里没有「%s」这条替换", from)
	}
	payload.SubjectAliases = kept
	if err := a.storeNamingRulesFile(payload); err != nil {
		return NamingRulesMutationResult{}, fmt.Errorf("保存命名规则失败: %w", err)
	}
	result.Removed = removed
	result.Rules = a.buildNamingRulesView(a.namingRulesSnapshot())
	return result, nil
}

// namingAliasInvalidChars 返回替换名里出现的 Windows 非法文件名字符（空串 = 合法）。
func namingAliasInvalidChars(value string) string {
	found := make([]string, 0, 1)
	seen := make(map[rune]bool)
	for _, char := range value {
		switch char {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			if !seen[char] {
				seen[char] = true
				found = append(found, string(char))
			}
		}
	}
	return strings.Join(found, " ")
}

// AddNamingExemptions 把所选文件加入例外表（已在表内 / 无效的逐条跳过并回报）。
func (a *App) AddNamingExemptions(paths []string) (NamingRulesMutationResult, error) {
	result := NamingRulesMutationResult{}
	if len(paths) == 0 {
		return result, errors.New("没有可加入例外表的文件")
	}
	a.namingRulesMu.Lock()
	defer a.namingRulesMu.Unlock()
	payload := a.namingRulesSnapshot()
	existing := make(map[string]bool, len(payload.Exemptions))
	for _, entry := range payload.Exemptions {
		existing[strings.ToLower(entry.Path)] = true
	}

	added := 0
	skipped := make([]string, 0)
	seenRequest := make(map[string]bool)
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			skipped = append(skipped, "空路径")
			continue
		}
		abs := filepath.Clean(trimmed)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			skipped = append(skipped, filepath.Base(abs)+"：找不到文件")
			continue
		}
		if !strings.EqualFold(filepath.Ext(abs), ".vpk") {
			skipped = append(skipped, filepath.Base(abs)+"：不是 .vpk 文件")
			continue
		}
		key, err := a.namingRelKeyFor(abs)
		if err != nil {
			skipped = append(skipped, filepath.Base(abs)+"：不在受管 Mod 目录内")
			continue
		}
		lower := strings.ToLower(key)
		if existing[lower] || seenRequest[lower] {
			skipped = append(skipped, filepath.Base(abs)+"：已在例外表")
			continue
		}
		seenRequest[lower] = true
		payload.Exemptions = append(payload.Exemptions, NamingExemptionEntry{
			Path:    key,
			Name:    filepath.Base(abs),
			AddedAt: time.Now().Format(time.RFC3339),
		})
		added++
	}
	if added == 0 {
		result.Skipped = skipped
		result.Rules = a.buildNamingRulesView(payload)
		return result, nil
	}
	if err := a.storeNamingRulesFile(payload); err != nil {
		return NamingRulesMutationResult{}, fmt.Errorf("保存命名规则失败: %w", err)
	}
	result.Added = added
	result.Skipped = skipped
	result.Rules = a.buildNamingRulesView(a.namingRulesSnapshot())
	return result, nil
}

// RemoveNamingExemptions 从例外表移除文件。输入接受存储键（相对路径，设置页回传）或
// 绝对路径（右键菜单场景），按小写比较匹配。
func (a *App) RemoveNamingExemptions(paths []string) (NamingRulesMutationResult, error) {
	result := NamingRulesMutationResult{}
	if len(paths) == 0 {
		return result, errors.New("没有可移出例外表的文件")
	}
	a.namingRulesMu.Lock()
	defer a.namingRulesMu.Unlock()
	payload := a.namingRulesSnapshot()

	targets := make(map[string]bool, len(paths))
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if filepath.IsAbs(trimmed) {
			if key, err := a.namingRelKeyFor(filepath.Clean(trimmed)); err == nil {
				targets[strings.ToLower(key)] = true
			}
			continue
		}
		targets[strings.ToLower(normalizeNamingRelKey(trimmed))] = true
	}
	if len(targets) == 0 {
		return result, errors.New("没有可移出例外表的文件")
	}

	kept := make([]NamingExemptionEntry, 0, len(payload.Exemptions))
	removed := 0
	for _, entry := range payload.Exemptions {
		if targets[strings.ToLower(entry.Path)] {
			removed++
			continue
		}
		kept = append(kept, entry)
	}
	skipped := make([]string, 0)
	if removed < len(targets) {
		remaining := make(map[string]bool)
		for key := range targets {
			remaining[key] = true
		}
		for _, entry := range payload.Exemptions {
			delete(remaining, strings.ToLower(entry.Path))
		}
		for key := range remaining {
			skipped = append(skipped, key+"：不在例外表")
		}
		sort.Strings(skipped)
	}
	if removed == 0 {
		result.Skipped = skipped
		result.Rules = a.buildNamingRulesView(payload)
		return result, nil
	}
	payload.Exemptions = kept
	if err := a.storeNamingRulesFile(payload); err != nil {
		return NamingRulesMutationResult{}, fmt.Errorf("保存命名规则失败: %w", err)
	}
	result.Removed = removed
	result.Skipped = skipped
	result.Rules = a.buildNamingRulesView(a.namingRulesSnapshot())
	return result, nil
}
