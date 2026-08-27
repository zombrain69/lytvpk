package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type AddonListItem struct {
	Name  string
	Value string
}

type addonListEncoding uint8

const (
	addonListEncodingUTF8 addonListEncoding = iota
	addonListEncodingGBK
	addonListEncodingWindows1252
	addonListEncodingUTF16LE
	addonListEncodingUTF16BE
)

type addonListDocument struct {
	path       string
	content    string
	encoding   addonListEncoding
	hasUTF8BOM bool
}

var addonListValueLineRegex = regexp.MustCompile(`^(\s*"([^"]+)"\s+")([^"]*)(".*)$`)

// readAddonList 读取并解析 addonlist.txt
func (a *App) readAddonList() ([]AddonListItem, string, error) {
	doc, err := a.readAddonListDocument()
	if err != nil {
		return nil, doc.path, err
	}
	return parseAddonListItems(doc.content), doc.path, nil
}

func parseAddonListItems(content string) []AddonListItem {
	list := make([]AddonListItem, 0)
	lines := strings.Split(content, "\n")
	kvRegex := regexp.MustCompile(`"([^"]+)"\s+"([^"]+)"`)
	inBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.Contains(line, "\"AddonList\"") {
			continue
		}
		if strings.Contains(line, "{") {
			inBlock = true
			continue
		}
		if strings.Contains(line, "}") {
			inBlock = false
			continue
		}

		if inBlock {
			matches := kvRegex.FindStringSubmatch(line)
			if len(matches) == 3 {
				list = append(list, AddonListItem{
					Name:  matches[1],
					Value: matches[2],
				})
			}
		}
	}
	return list
}

// writeAddonList 写入 addonlist.txt
func (a *App) writeAddonList(path string, list []AddonListItem) error {
	var buf bytes.Buffer
	buf.WriteString("\"AddonList\"\n{\n")
	for _, item := range list {
		// root Mod 使用文件名；workshop Mod 必须保留 workshop\\<id>.vpk，
		// 否则会丢失其在 addonlist.txt 中的识别路径。
		name := strings.ReplaceAll(strings.TrimSpace(item.Name), "/", "\\")
		buf.WriteString(fmt.Sprintf("\t\"%s\"\t\t\"%s\"\n", name, item.Value))
	}
	buf.WriteString("}\n")

	// 读取现有文档的编码与 BOM，排序写回时必须保持游戏原文件格式。
	// L4D2 常见的 addonlist.txt 是 GBK/ANSI；少数环境可能使用
	// Windows-1252。强制改成 UTF-8 会让游戏
	// 在启动/保存阶段重新生成文件，从而看起来像是排序被回滚。
	doc, err := readAddonListDocumentAtPath(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		doc = addonListDocument{path: path, encoding: addonListEncodingUTF8}
	}
	return a.writeAddonListDocument(doc, buf.String())
}

// GetVPKLoadOrder 获取 VPK 文件的加载顺序 (1-based index)
// 如果文件不在列表中，返回 -1
func (a *App) GetVPKLoadOrder(filename string) (int, error) {
	list, _, err := a.readAddonList()
	if err != nil {
		// 如果文件不存在，必须返回错误，而不是吞掉错误
		// 这样前端才能区分是"文件不存在"还是"文件不在列表中"
		return 0, err
	}

	targetName, err := a.addonListKeyForReference(filename)
	if err != nil {
		return 0, err
	}
	for i, item := range list {
		if normalizeAddonListKey(item.Name) == targetName {
			return i + 1, nil // 1-based
		}
	}

	return -1, nil
}

// SetVPKLoadOrder 设置 VPK 文件的加载顺序
func (a *App) SetVPKLoadOrder(filename string, newOrder int) error {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	list, path, err := a.readAddonList()

	// 如果文件不存在，初始化为空列表，准备新建
	if err != nil && strings.Contains(err.Error(), "不存在") {
		list = []AddonListItem{}
		if path == "" {
			path, err = a.addonListPath()
			if err != nil {
				return err
			}
		}
	} else if err != nil {
		return err
	}

	targetName, err := a.addonListKeyForReference(filename)
	if err != nil {
		return err
	}

	// 1. 先查找并移除已存在的条目
	var existingItem AddonListItem
	found := false
	cleanList := make([]AddonListItem, 0, len(list))

	for _, item := range list {
		if normalizeAddonListKey(item.Name) == targetName {
			existingItem = item
			found = true
		} else {
			cleanList = append(cleanList, item)
		}
	}

	// 如果没找到，创建一个新的，默认为开启状态 "1"
	if !found {
		existingItem = AddonListItem{
			Name:  targetName,
			Value: "1",
		}
	}

	// 2. 确定插入位置
	// newOrder 是 1-based
	// 转换到 0-based slice index
	index := newOrder - 1

	if index < 0 {
		index = 0
	}
	if index > len(cleanList) {
		index = len(cleanList)
	}

	// 3. 插入
	finalList := make([]AddonListItem, 0, len(cleanList)+1)
	finalList = append(finalList, cleanList[:index]...)
	finalList = append(finalList, existingItem)
	finalList = append(finalList, cleanList[index:]...)

	// 4. 写入文件，并在开启监控时同步更新受保护版本。
	if err := a.writeAddonList(path, finalList); err != nil {
		return err
	}
	return a.syncManagedAddonListSnapshotLocked(path)
}

// GetAddonListOrder 读取并解析 addonlist.txt 获取加载顺序
func (a *App) GetAddonListOrder() ([]string, error) {
	list, _, err := a.readAddonList()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("解析 addonlist.txt 失败或未找到任何条目")
	}
	order := make([]string, 0, len(list))
	for _, item := range list {
		order = append(order, item.Name)
	}
	return order, nil
}

func (a *App) addonListPath() (string, error) {
	rootDir := a.rootDirectorySnapshot()
	return addonListPathForRoot(rootDir)
}

func addonListPathForRoot(rootDir string) (string, error) {
	rootDir = filepath.Clean(strings.TrimSpace(rootDir))
	if rootDir == "" || rootDir == "." {
		return "", fmt.Errorf("未选择L4D2目录")
	}

	if strings.EqualFold(filepath.Base(rootDir), "addons") {
		if strings.EqualFold(filepath.Base(filepath.Dir(rootDir)), "left4dead2") {
			rootDir = filepath.Dir(rootDir)
		}
	} else if !strings.EqualFold(filepath.Base(rootDir), "left4dead2") {
		// The picker normally returns left4dead2\addons, but accepting the
		// Steam game root keeps the path tied to the real game files when a
		// user selects E:\\...\\Left 4 Dead 2 instead.
		candidate := filepath.Join(rootDir, "left4dead2")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			rootDir = candidate
		}
	}
	return filepath.Join(rootDir, "addonlist.txt"), nil
}

// readAddonListDocument 读取原始文档，并记住编码与 BOM，供游戏开关原位保真写入。
func (a *App) readAddonListDocument() (addonListDocument, error) {
	path, err := a.addonListPath()
	if err != nil {
		return addonListDocument{}, err
	}
	return readAddonListDocumentAtPath(path)
}

func readAddonListDocumentAtPath(path string) (addonListDocument, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return addonListDocument{path: path}, fmt.Errorf("addonlist.txt 不存在: %w", err)
		}
		return addonListDocument{path: path}, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}

	doc := addonListDocument{path: path, encoding: addonListEncodingUTF8}
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		doc.hasUTF8BOM = true
		content = content[3:]
	}
	if utf8.Valid(content) {
		doc.content = string(content)
		return doc, nil
	}
	if len(content) >= 2 && content[0] == 0xFF && content[1] == 0xFE {
		decoded, decodeErr := decodeAddonListUTF16(content, unicode.LittleEndian)
		if decodeErr != nil {
			return addonListDocument{}, decodeErr
		}
		doc.content = decoded
		doc.encoding = addonListEncodingUTF16LE
		return doc, nil
	}
	if len(content) >= 2 && content[0] == 0xFE && content[1] == 0xFF {
		decoded, decodeErr := decodeAddonListUTF16(content, unicode.BigEndian)
		if decodeErr != nil {
			return addonListDocument{}, decodeErr
		}
		doc.content = decoded
		doc.encoding = addonListEncodingUTF16BE
		return doc, nil
	}

	decoded, _, gbkErr := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), content)
	if gbkErr == nil && !strings.ContainsRune(string(decoded), '\uFFFD') {
		doc.content = string(decoded)
		doc.encoding = addonListEncodingGBK
		return doc, nil
	}
	decoded, _, ansiErr := transform.Bytes(charmap.Windows1252.NewDecoder(), content)
	if ansiErr != nil {
		return addonListDocument{}, fmt.Errorf("无法按 GBK/ANSI 解码 addonlist.txt（GBK: %v；Windows-1252: %w）", gbkErr, ansiErr)
	}
	if strings.ContainsRune(string(decoded), '\uFFFD') {
		return addonListDocument{}, fmt.Errorf("无法按 GBK/ANSI 解码 addonlist.txt：Windows-1252 结果包含替换字符（GBK: %v）", gbkErr)
	}
	doc.content = string(decoded)
	doc.encoding = addonListEncodingWindows1252
	return doc, nil
}

func encodeAddonListDocument(doc addonListDocument, content string) ([]byte, error) {
	if doc.encoding == addonListEncodingGBK {
		encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(content))
		if err != nil {
			return nil, fmt.Errorf("无法按 GBK 编码 addonlist.txt: %w", err)
		}
		return encoded, nil
	}
	if doc.encoding == addonListEncodingWindows1252 {
		encoded, _, err := transform.Bytes(charmap.Windows1252.NewEncoder(), []byte(content))
		if err != nil {
			return nil, fmt.Errorf("无法按 Windows-1252/ANSI 编码 addonlist.txt: %w", err)
		}
		return encoded, nil
	}
	if doc.encoding == addonListEncodingUTF16LE || doc.encoding == addonListEncodingUTF16BE {
		endian := unicode.LittleEndian
		bom := []byte{0xFF, 0xFE}
		if doc.encoding == addonListEncodingUTF16BE {
			endian = unicode.BigEndian
			bom = []byte{0xFE, 0xFF}
		}
		encoded, _, err := transform.Bytes(unicode.UTF16(endian, unicode.IgnoreBOM).NewEncoder(), []byte(content))
		if err != nil {
			return nil, fmt.Errorf("无法按 UTF-16 编码 addonlist.txt: %w", err)
		}
		return append(bom, encoded...), nil
	}

	encoded := []byte(content)
	if doc.hasUTF8BOM {
		encoded = append([]byte{0xEF, 0xBB, 0xBF}, encoded...)
	}
	return encoded, nil
}

func decodeAddonListUTF16(content []byte, endian unicode.Endianness) (string, error) {
	decoded, _, err := transform.Bytes(unicode.UTF16(endian, unicode.ExpectBOM).NewDecoder(), content)
	if err != nil {
		return "", fmt.Errorf("无法按 UTF-16 解码 addonlist.txt: %w", err)
	}
	return string(decoded), nil
}

// backupAddonListDocument 在首次修改前保存原始字节，避免覆盖游戏生成的配置。
// 已存在的备份不会被覆盖，以保留首次写入前的可恢复版本。
func backupAddonListDocument(doc addonListDocument) error {
	original, err := os.ReadFile(doc.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法读取 addonlist.txt 以创建备份: %w", err)
	}

	backupPath := doc.path + ".lytvpk.bak"
	backupFile, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法创建 addonlist.txt 备份: %w", err)
	}

	if _, err := backupFile.Write(original); err != nil {
		backupFile.Close()
		_ = os.Remove(backupPath)
		return fmt.Errorf("无法写入 addonlist.txt 备份: %w", err)
	}
	if err := backupFile.Close(); err != nil {
		return fmt.Errorf("无法关闭 addonlist.txt 备份: %w", err)
	}
	return nil
}

func (a *App) writeAddonListDocument(doc addonListDocument, content string) error {
	encoded, err := encodeAddonListDocument(doc, content)
	if err != nil {
		return err
	}
	if err := backupAddonListDocument(doc); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(filepath.Dir(doc.path), ".lytvpk-addonlist-*")
	if err != nil {
		return fmt.Errorf("无法创建 addonlist.txt 临时文件: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(encoded); err != nil {
		tempFile.Close()
		return fmt.Errorf("无法写入 addonlist.txt 临时文件: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("无法关闭 addonlist.txt 临时文件: %w", err)
	}
	if err := replaceFile(tempPath, doc.path); err != nil {
		return fmt.Errorf("无法替换 addonlist.txt: %w", err)
	}
	return nil
}

func normalizeAddonListKey(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "\\")
	for strings.HasPrefix(name, ".\\") {
		name = strings.TrimPrefix(name, ".\\")
	}
	return strings.ToLower(name)
}

func (a *App) addonListKeyForVPKPath(filePath string) (string, error) {
	rootDir := a.rootDirectorySnapshot()
	return addonListKeyForVPKPathFromRoot(rootDir, filePath)
}

func addonListKeyForVPKPathFromRoot(rootDir, filePath string) (string, error) {
	if rootDir == "" {
		return "", fmt.Errorf("未选择L4D2目录")
	}

	relativePath, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return "", fmt.Errorf("无法计算 Mod 相对路径: %w", err)
	}
	key := normalizeAddonListKey(relativePath)
	if key == ".." || strings.HasPrefix(key, "..\\") {
		return "", fmt.Errorf("Mod 不在 addons 目录中: %s", filePath)
	}
	return key, nil
}

// addonListKeyForReference 同时接收已扫描 VPK 的绝对路径与兼容旧调用方的相对文件名。
func (a *App) addonListKeyForReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", fmt.Errorf("Mod 路径不能为空")
	}
	if filepath.IsAbs(reference) {
		return a.addonListKeyForVPKPath(reference)
	}
	key := normalizeAddonListKey(reference)
	if key == ".." || strings.HasPrefix(key, "..\\") {
		return "", fmt.Errorf("Mod 不在 addons 目录中: %s", reference)
	}
	return key, nil
}

func addonListStateMap(list []AddonListItem) map[string]bool {
	states := make(map[string]bool, len(list))
	for _, item := range list {
		states[normalizeAddonListKey(item.Name)] = item.Value == "1"
	}
	return states
}

// applyAddonListGameStates 将 addonlist.txt 的游戏内开关状态合并进扫描缓存。
// addonlist.txt 缺失或不可读时不阻断 VPK 扫描，前端会显示为“未记录”。
func (a *App) applyAddonListGameStates() {
	list, _, err := a.readAddonList()
	states := addonListStateMap(list)
	if err != nil {
		states = map[string]bool{}
	}

	a.vpkCache.Range(func(cacheKey, cacheValue interface{}) bool {
		cache := cacheValue.(*VPKFileCache)
		file := cache.File
		file.GameEnabled = false
		file.GameStateKnown = false

		if key, keyErr := a.addonListKeyForVPKPath(file.Path); keyErr == nil {
			if enabled, found := states[key]; found {
				file.GameEnabled = enabled
				file.GameStateKnown = true
			}
		}

		cache.File = file
		a.vpkCache.Store(cacheKey, cache)
		return true
	})
}

func replaceAddonListValue(content, targetKey, value string) (string, bool, error) {
	lineEnding := "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}

	lines := strings.SplitAfter(content, "\n")
	inBlock := false
	blockFound := false
	found := false

	for index, line := range lines {
		lineWithoutEnding := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		trimmed := strings.TrimSpace(lineWithoutEnding)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.Contains(trimmed, "\"AddonList\"") {
			continue
		}
		if strings.Contains(trimmed, "{") {
			inBlock = true
			blockFound = true
			continue
		}
		if strings.Contains(trimmed, "}") {
			if inBlock && !found {
				entry := fmt.Sprintf("\t\"%s\"\t\t\"%s\"%s", targetKey, value, lineEnding)
				lines = append(lines[:index], append([]string{entry}, lines[index:]...)...)
				found = true
			}
			inBlock = false
			continue
		}
		if !inBlock {
			continue
		}

		matches := addonListValueLineRegex.FindStringSubmatchIndex(lineWithoutEnding)
		if len(matches) != 10 || normalizeAddonListKey(lineWithoutEnding[matches[4]:matches[5]]) != targetKey {
			continue
		}

		lines[index] = lineWithoutEnding[:matches[6]] + value + lineWithoutEnding[matches[7]:] + line[len(lineWithoutEnding):]
		found = true
	}

	if !blockFound {
		return "", false, fmt.Errorf("addonlist.txt 中未找到 AddonList 块")
	}
	return strings.Join(lines, ""), found, nil
}

// removeAddonListValue removes every occurrence of targetKey while preserving
// comments, encoding, line endings and the surrounding AddonList block.
func removeAddonListValue(content, targetKey string) (string, bool, error) {
	lines := strings.SplitAfter(content, "\n")
	inBlock := false
	blockFound := false
	removed := false
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		lineWithoutEnding := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		trimmed := strings.TrimSpace(lineWithoutEnding)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			kept = append(kept, line)
			continue
		}
		if strings.Contains(trimmed, "\"AddonList\"") {
			kept = append(kept, line)
			continue
		}
		if strings.Contains(trimmed, "{") {
			inBlock = true
			blockFound = true
			kept = append(kept, line)
			continue
		}
		if strings.Contains(trimmed, "}") {
			inBlock = false
			kept = append(kept, line)
			continue
		}
		if inBlock {
			matches := addonListValueLineRegex.FindStringSubmatch(lineWithoutEnding)
			if len(matches) == 5 && normalizeAddonListKey(matches[2]) == targetKey {
				removed = true
				continue
			}
		}
		kept = append(kept, line)
	}
	if !blockFound {
		return "", false, fmt.Errorf("addonlist.txt 中未找到 AddonList 块")
	}
	return strings.Join(kept, ""), removed, nil
}

// addonListKeyForManagedVPKPath maps a VPK in disabled back to the key it had
// while enabled.  This works for both the real game addons directory and a
// custom collection directory.
func (a *App) addonListKeyForManagedVPKPath(filePath string) (string, error) {
	rootDir := a.rootDirectorySnapshot()
	return addonListKeyForManagedVPKPathFromRoot(rootDir, filePath)
}

func addonListKeyForManagedVPKPathFromRoot(rootDir, filePath string) (string, error) {
	if rootDir == "" {
		return "", fmt.Errorf("未选择L4D2目录")
	}
	relativePath, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		return "", fmt.Errorf("无法计算 Mod 相对路径: %w", err)
	}
	key := normalizeAddonListKey(relativePath)
	if key == ".." || strings.HasPrefix(key, "..\\") {
		return "", fmt.Errorf("Mod 不在 addons 目录中: %s", filePath)
	}
	if strings.HasPrefix(key, "disabled\\") {
		return normalizeAddonListKey(strings.TrimPrefix(key, "disabled\\")), nil
	}
	return key, nil
}

func workshopAddonListKey(workshopID string) string {
	workshopID = strings.TrimSpace(workshopID)
	if workshopID == "" {
		return ""
	}
	return normalizeAddonListKey(filepath.Join("workshop", workshopID+".vpk"))
}

// updateAddonListEntriesLocked applies removals and value updates in one
// preserved-format write. The caller must hold addonListGuardMu.
func (a *App) updateAddonListEntriesLocked(values map[string]string, removals []string) error {
	path, err := a.addonListPath()
	if err != nil {
		return err
	}
	doc, err := readAddonListDocumentAtPath(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if len(values) == 0 {
			return nil
		}
		doc = addonListDocument{path: path, content: "\"AddonList\"\n{\n}\n", encoding: addonListEncodingUTF8}
	}

	updated := doc.content
	changed := false
	for _, key := range removals {
		key = normalizeAddonListKey(key)
		if key == "" {
			continue
		}
		var removed bool
		updated, removed, err = removeAddonListValue(updated, key)
		if err != nil {
			return err
		}
		changed = changed || removed
	}
	normalizedValues := make(map[string]string, len(values))
	for key, value := range values {
		if normalized := normalizeAddonListKey(key); normalized != "" {
			normalizedValues[normalized] = value
		}
	}
	keys := make([]string, 0, len(normalizedValues))
	for key := range normalizedValues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := normalizedValues[key]
		var replaced bool
		updated, replaced, err = replaceAddonListValue(updated, key, value)
		if err != nil {
			return err
		}
		if replaced {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := a.writeAddonListDocument(doc, updated); err != nil {
		return err
	}
	if err := a.syncManagedAddonListSnapshotLocked(path); err != nil {
		return err
	}
	a.applyAddonListGameStates()
	return nil
}

func (a *App) updateAddonListEntries(values map[string]string, removals []string) error {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()
	return a.updateAddonListEntriesLocked(values, removals)
}

func (a *App) cleanupAddonListForRemovedVPK(filePath string, cachedFile *VPKFile) error {
	key, err := a.addonListKeyForManagedVPKPath(filePath)
	if err != nil {
		return err
	}
	removals := []string{key}
	values := map[string]string{}
	if cachedFile != nil && cachedFile.Location == "workshop" {
		// 删除 workshop 原文件时允许游戏重新生成它，不能留下旧的 0 状态。
		removals = append(removals, normalizeAddonListKey(filepath.Join("workshop", filepath.Base(filePath))))
	} else if cachedFile != nil {
		// root/disabled 中的工坊副本删除后，仍保持原 workshop 订阅处于关闭状态。
		if workshopKey := workshopAddonListKey(cachedFile.WorkshopID); workshopKey != "" {
			values[workshopKey] = "0"
		}
	}
	a.vpkCache.Delete(filePath)
	return a.updateAddonListEntries(values, removals)
}

// SetVPKGameEnabled 更新 addonlist.txt 中某个已扫描 VPK 的游戏内开关。
// 该操作不会移动文件，也不会改变本程序 disabled 目录的文件管理状态。
func (a *App) SetVPKGameEnabled(filePath string, enabled bool) error {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	a.mu.RLock()
	cached, ok := a.vpkCache.Load(filePath)
	if !ok {
		a.mu.RUnlock()
		return fmt.Errorf("文件未找到: %s", filePath)
	}
	cache := cached.(*VPKFileCache)
	if cache.File.Location == "disabled" {
		a.mu.RUnlock()
		return fmt.Errorf("文件位于 disabled 目录，无法设置游戏内开关")
	}
	vpkPath := cache.File.Path
	a.mu.RUnlock()

	targetKey, err := a.addonListKeyForVPKPath(vpkPath)
	if err != nil {
		return err
	}

	doc, err := a.readAddonListDocument()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		path, pathErr := a.addonListPath()
		if pathErr != nil {
			return pathErr
		}
		doc = addonListDocument{
			path:     path,
			content:  "\"AddonList\"\n{\n}\n",
			encoding: addonListEncodingUTF8,
		}
	}

	value := "0"
	if enabled {
		value = "1"
	}
	updatedContent, _, err := replaceAddonListValue(doc.content, targetKey, value)
	if err != nil {
		return err
	}
	if err := a.writeAddonListDocument(doc, updatedContent); err != nil {
		return err
	}

	a.mu.Lock()
	if latest, found := a.vpkCache.Load(filePath); found {
		cache = latest.(*VPKFileCache)
	}
	cache.File.GameEnabled = enabled
	cache.File.GameStateKnown = true
	a.vpkCache.Store(filePath, cache)
	a.mu.Unlock()
	return a.syncManagedAddonListSnapshotLocked(doc.path)
}
