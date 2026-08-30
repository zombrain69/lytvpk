package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"vpk-manager/internal/parser"
)

const (
	windowsMaxPathLength     = 260
	windowsMaxFilenameLength = 255
)

func windowsPathLength(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func validateWindowsRenamePath(path string, filename string) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	filenameLength := windowsPathLength(filename)
	if filenameLength > windowsMaxFilenameLength {
		return fmt.Errorf("文件名过长: %d/%d，请缩短名称", filenameLength, windowsMaxFilenameLength)
	}

	pathLength := windowsPathLength(path)
	if pathLength > windowsMaxPathLength {
		return fmt.Errorf("完整路径过长: %d/%d，请缩短名称或移动 Mod 目录", pathLength, windowsMaxPathLength)
	}

	return nil
}

func (a *App) GetVPKPreviewImage(filePath string) string {
	if _, ok := a.vpkCache.Load(filePath); !ok {
		return ""
	}

	modTime, size, imageModTime, err := previewSourceSignature(filePath)
	if err != nil {
		return ""
	}
	if preview, ok := a.getVPKPreviewCache(filePath, modTime, size, imageModTime); ok {
		return preview
	}

	preview, err := parser.ExtractVPKPreviewImage(filePath)
	if err != nil {
		log.Printf("读取 VPK 预览图失败: %s, 错误: %v", filePath, err)
		return ""
	}
	a.storeVPKPreviewCache(filePath, preview, modTime, size, imageModTime)
	return preview
}

func (a *App) getVPKPreviewCache(filePath string, modTime time.Time, size int64, imageModTime time.Time) (string, bool) {
	a.previewCacheMu.Lock()
	defer a.previewCacheMu.Unlock()

	cached, ok := a.previewCache.Load(filePath)
	if !ok {
		return "", false
	}
	entry := cached.(*VPKPreviewCache)
	if !entry.ModTime.Equal(modTime) || entry.Size != size || !entry.ImageModTime.Equal(imageModTime) {
		a.previewCache.Delete(filePath)
		return "", false
	}
	entry.CachedAt = time.Now()
	return entry.Data, true
}

func (a *App) storeVPKPreviewCache(filePath, preview string, modTime time.Time, size int64, imageModTime time.Time) {
	// Go string stores Base64 bytes verbatim. A single unusually large image is
	// still returned to the caller but is not retained by the backend cache.
	if len(preview) > maxVPKPreviewCacheBytes {
		a.deleteVPKPreviewCache(filePath)
		return
	}

	a.previewCacheMu.Lock()
	defer a.previewCacheMu.Unlock()
	a.previewCache.Store(filePath, &VPKPreviewCache{
		Data:         preview,
		ModTime:      modTime,
		Size:         size,
		ImageModTime: imageModTime,
		CachedAt:     time.Now(),
	})

	var totalBytes int
	var entries int
	for {
		var oldestPath string
		var oldestTime time.Time
		totalBytes = 0
		entries = 0
		a.previewCache.Range(func(key, value any) bool {
			entry := value.(*VPKPreviewCache)
			entries++
			totalBytes += len(entry.Data)
			if oldestPath == "" || entry.CachedAt.Before(oldestTime) {
				oldestPath = key.(string)
				oldestTime = entry.CachedAt
			}
			return true
		})
		if entries <= maxVPKPreviewCacheEntries && totalBytes <= maxVPKPreviewCacheBytes {
			return
		}
		if oldestPath == "" {
			return
		}
		a.previewCache.Delete(oldestPath)
	}
}

func (a *App) deleteVPKPreviewCache(filePath string) {
	a.previewCacheMu.Lock()
	defer a.previewCacheMu.Unlock()
	a.previewCache.Delete(filePath)
}

func previewSourceSignature(filePath string) (time.Time, int64, time.Time, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}, 0, time.Time{}, err
	}

	var imageModTime time.Time
	basePath := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	for _, ext := range []string{".jpg", ".png", ".jpeg", ".gif"} {
		if imageInfo, statErr := os.Stat(basePath + ext); statErr == nil {
			imageModTime = imageInfo.ModTime()
			break
		}
	}
	return info.ModTime(), info.Size(), imageModTime, nil
}

// ToggleVPKFile 切换VPK文件的启用状态（智能缓存版本）
// 注意：workshop文件不能直接启用/禁用，需要先转移到root目录
func (a *App) ToggleVPKFile(filePath string) error {
	a.mu.Lock()

	// 从缓存中获取文件信息
	cached, ok := a.vpkCache.Load(filePath)
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("文件未找到: %s", filePath)
	}

	cache := cached.(*VPKFileCache)
	vpkFile := cache.File
	rootDir := a.rootDir
	if strings.TrimSpace(rootDir) == "" {
		a.mu.Unlock()
		return fmt.Errorf("未选择L4D2目录")
	}

	// workshop文件不能直接启用/禁用
	if vpkFile.Location == "workshop" {
		a.mu.Unlock()
		return fmt.Errorf("workshop文件需要先转移到插件目录才能启用/禁用")
	}
	managedKey, keyErr := addonListKeyForManagedVPKPathFromRoot(rootDir, vpkFile.Path)
	if keyErr != nil {
		a.mu.Unlock()
		return keyErr
	}
	workshopKey := workshopAddonListKey(vpkFile.WorkshopID)

	var newPath string
	var err error

	if vpkFile.Enabled && vpkFile.Location == "root" {
		// 禁用文件：从root移动到disabled目录
		disabledDir := filepath.Join(rootDir, "disabled")
		os.MkdirAll(disabledDir, 0755)

		newPath = filepath.Join(disabledDir, vpkFile.Name)
		err = os.Rename(vpkFile.Path, newPath)
		if err != nil {
			a.mu.Unlock()
			return err
		}
		// 同步移动同名图片
		a.handleSidecarFile(vpkFile.Path, newPath, "move")

		// 更新文件信息
		vpkFile.Path = newPath
		vpkFile.Enabled = false
		vpkFile.Location = "disabled"

	} else if !vpkFile.Enabled && vpkFile.Location == "disabled" {
		// 启用文件：从disabled移动回root目录
		newPath = filepath.Join(rootDir, vpkFile.Name)
		err = os.Rename(vpkFile.Path, newPath)
		if err != nil {
			a.mu.Unlock()
			return err
		}
		// 同步移动同名图片
		a.handleSidecarFile(vpkFile.Path, newPath, "move")

		// 更新文件信息
		vpkFile.Path = newPath
		vpkFile.Enabled = true
		vpkFile.Location = "root"

	} else {
		a.mu.Unlock()
		return fmt.Errorf("无效的文件状态转换")
	}

	// 删除旧路径的缓存
	a.vpkCache.Delete(filePath)
	a.deleteVPKPreviewCache(filePath)

	// 在新路径下存储缓存
	newCache := *cache
	newCache.File = vpkFile
	a.vpkCache.Store(newPath, &newCache)
	a.mu.Unlock()

	values := map[string]string{}
	removals := []string{}
	if vpkFile.Location == "disabled" {
		removals = append(removals, managedKey)
		if workshopKey != "" {
			values[workshopKey] = "0"
		}
	} else {
		values[managedKey] = "1"
		if workshopKey != "" {
			values[workshopKey] = "0"
		}
	}
	if err := a.updateAddonListEntries(values, removals); err != nil {
		return fmt.Errorf("文件已移动，但 addonlist.txt 同步失败: %w", err)
	}

	log.Printf("文件已移动: %s -> %s", filePath, newPath)

	return nil
}

// MoveWorkshopToAddons 将workshop中的VPK移动到addons目录（root目录）
func (a *App) MoveWorkshopToAddons(filePath string) error {
	_, err := a.moveWorkshopToAddonsWithConflictAction(filePath, "")
	return err
}

// MoveWorkshopToAddonsWithConflictAction 将workshop中的VPK复制到addons目录，
// action 可为空、replace、skip 或 cancel。
func (a *App) MoveWorkshopToAddonsWithConflictAction(filePath, action string) (MoveResult, error) {
	return a.moveWorkshopToAddonsWithConflictAction(filePath, action)
}

func (a *App) moveWorkshopToAddonsWithConflictAction(filePath, action string) (MoveResult, error) {
	result := MoveResult{}
	var err error
	if action, err = normalizeMoveConflictAction(action); err != nil {
		return result, err
	}
	a.mu.Lock()

	// 从缓存中获取文件信息
	cached, ok := a.vpkCache.Load(filePath)
	if !ok {
		a.mu.Unlock()
		return result, fmt.Errorf("文件未找到: %s", filePath)
	}

	cache := cached.(*VPKFileCache)
	vpkFile := cache.File
	rootDir := a.rootDir
	if strings.TrimSpace(rootDir) == "" {
		a.mu.Unlock()
		return result, fmt.Errorf("未选择L4D2目录")
	}

	if vpkFile.Location != "workshop" {
		a.mu.Unlock()
		return result, fmt.Errorf("只能转移workshop文件")
	}

	newPath := filepath.Join(rootDir, vpkFile.Name)
	err = copyRegularFileWithConflictAction(vpkFile.Path, newPath, action)
	if errors.Is(err, errMoveSkipped) {
		a.mu.Unlock()
		result.SkippedCount = 1
		return result, nil
	}
	if errors.Is(err, errMoveCancelled) {
		a.mu.Unlock()
		result.Cancelled = true
		return result, nil
	}
	if err != nil {
		a.mu.Unlock()
		return result, err
	}
	if err := copyWorkshopSidecarsWithConflictAction(vpkFile.Path, newPath, action); err != nil {
		_ = os.Remove(newPath)
		a.mu.Unlock()
		if errors.Is(err, errMoveCancelled) {
			result.Cancelled = true
			return result, nil
		}
		return result, fmt.Errorf("复制工坊 Mod 伴随文件失败: %w", err)
	}

	// 转移到root目录后，文件默认为启用状态
	vpkFile.Path = newPath
	vpkFile.Location = "root"
	vpkFile.Enabled = true

	// 删除旧路径的缓存
	a.vpkCache.Delete(filePath)
	a.deleteVPKPreviewCache(filePath)

	// 在新路径下存储缓存
	newCache := *cache
	newCache.File = vpkFile
	a.vpkCache.Store(newPath, &newCache)
	a.mu.Unlock()

	workshopKey := normalizeAddonListKey(filepath.Join("workshop", filepath.Base(filePath)))
	rootKey, keyErr := addonListKeyForManagedVPKPathFromRoot(rootDir, newPath)
	if keyErr != nil {
		return result, keyErr
	}
	if err := a.updateAddonListEntries(map[string]string{
		rootKey:     "1",
		workshopKey: "0",
	}, nil); err != nil {
		return result, fmt.Errorf("已复制到 addons，但 addonlist.txt 同步失败: %w", err)
	}
	// 保留 workshop 原件意味着 Steam/游戏可能在下次启动时重新写入 1。
	// 防覆盖监控是用户明确选择的功能：复制操作只同步当前状态，绝不隐式开启监控。
	// 用户可在“设置 > addonlist.txt 生命周期管理”中按需启用监控并自动恢复。

	log.Printf("文件已复制: %s -> %s（保留 workshop 原文件并显式关闭）", filePath, newPath)

	result.SuccessCount = 1
	return result, nil
}

func (a *App) ToggleVPKVisibility(filePath string) (string, error) {
	dir := filepath.Dir(filePath)
	filename := filepath.Base(filePath)

	var newFilename string
	if strings.HasPrefix(filename, "_") {
		// Unhide: remove prefix
		newFilename = strings.TrimPrefix(filename, "_")
	} else {
		// Hide: add prefix
		newFilename = "_" + filename
	}

	newPath := filepath.Join(dir, newFilename)

	// Check if target exists
	if _, err := os.Stat(newPath); err == nil {
		return "", fmt.Errorf("目标文件已存在: %s", newFilename)
	}

	err := os.Rename(filePath, newPath)
	if err != nil {
		return "", err
	}
	// 同步重命名同名图片
	a.handleSidecarFile(filePath, newPath, "move")

	return newPath, nil
}

func (a *App) SetVPKTags(filePath string, primaryTag string, secondaryTags []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, err := os.Stat(filePath); err != nil {
		return err
	}

	filename := filepath.Base(filePath)

	// 解析原文件名获取 "real name" 部分（包含可能的 _ 前缀）
	_, _, realName, _ := parser.ParseFilenameTags(filename)

	// 拆分 _ 前缀
	prefix := ""
	baseName := realName
	if strings.HasPrefix(realName, "_") {
		prefix = "_"
		baseName = strings.TrimPrefix(realName, "_")
	}

	// 清理特殊字符，避免破坏文件名或标签解析
	primaryTag = parser.CanonicalTag(parser.SanitizeTag(primaryTag))
	sanitizedSecondary := make([]string, 0, len(secondaryTags))
	for _, t := range secondaryTags {
		if cleaned := parser.SanitizeTag(t); cleaned != "" {
			sanitizedSecondary = append(sanitizedSecondary, cleaned)
		}
	}
	secondaryTags = parser.UniqueTagsExcluding(sanitizedSecondary, primaryTag)

	// 组合新标签
	allTags := make([]string, 0)
	if primaryTag != "" {
		allTags = append(allTags, primaryTag)
	}
	allTags = append(allTags, secondaryTags...)

	// Steam Workshop 目录中的文件名通常就是发布 ID。给它添加 [标签] 前缀会使
	// Steam 与游戏失去对该文件的识别，因此标签只写进同名 .meta 伴随文件。
	if a.isWorkshopVPKPath(filePath) {
		return a.setWorkshopVPKTagsLocked(filePath, primaryTag, secondaryTags, allTags)
	}

	var newFilename string
	if len(allTags) == 0 {
		newFilename = prefix + baseName
	} else {
		// 使用 + 作为分隔符，避免使用逗号导致 Windows Explorer /select, 失效
		tagStr := strings.Join(allTags, "+")
		newFilename = fmt.Sprintf("%s[%s]%s", prefix, tagStr, baseName)
	}

	dir := filepath.Dir(filePath)
	newPath := filepath.Join(dir, newFilename)

	if newPath == filePath {
		return nil
	}

	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("目标文件已存在: %s", newFilename)
	}

	if err := os.Rename(filePath, newPath); err != nil {
		return err
	}
	// 同步重命名同名图片
	a.handleSidecarFile(filePath, newPath, "move")

	// Update cache
	// 如果是清除标签操作（len(allTags) == 0），则不复用旧缓存，而是强制重新解析
	// 这样可以恢复文件本身的自动检测标签（如地图、人物等）
	cachedVal, loaded := a.vpkCache.Load(filePath)
	if loaded {
		a.vpkCache.Delete(filePath)
		a.deleteVPKPreviewCache(filePath)
	}

	if loaded && len(allTags) > 0 {
		cache := cachedVal.(*VPKFileCache)
		cache.File.Path = newPath
		cache.File.Name = filepath.Base(newPath)
		cache.File.PrimaryTag = primaryTag
		cache.File.SecondaryTags = secondaryTags

		a.vpkCache.Store(newPath, cache)
	} else {
		// 缓存未命中，或者清除了标签需要重新探测内容
		a.processVPKFileWithCache(newPath)
	}

	a.updateCompletedDownloadTaskPath(filePath, newPath)
	return nil
}

func (a *App) isWorkshopVPKPath(filePath string) bool {
	if a.rootDir == "" {
		return false
	}
	relativePath, err := filepath.Rel(a.rootDir, filePath)
	if err != nil {
		return false
	}
	relativePath = filepath.Clean(relativePath)
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false
	}
	parts := strings.Split(relativePath, string(filepath.Separator))
	return len(parts) > 1 && strings.EqualFold(parts[0], "workshop")
}

// setWorkshopVPKTagsLocked 保存工坊 VPK 的标签而不改动其文件名。调用方必须持有 a.mu。
func (a *App) setWorkshopVPKTagsLocked(filePath, primaryTag string, secondaryTags, allTags []string) error {
	meta, err := LoadWorkshopMeta(filePath)
	if err != nil {
		return fmt.Errorf("无法读取工坊 Mod 的 .meta 标签文件: %w", err)
	}
	if meta == nil {
		meta = &WorkshopMeta{}
	}
	meta.Tags = append([]string(nil), allTags...)
	meta.PrimaryTag = primaryTag
	meta.SecondaryTags = append([]string(nil), secondaryTags...)
	if err := saveWorkshopMeta(filePath, meta); err != nil {
		return fmt.Errorf("无法保存工坊 Mod 的 .meta 标签文件: %w", err)
	}

	cachedVal, loaded := a.vpkCache.Load(filePath)
	if loaded && len(allTags) > 0 {
		cache := cachedVal.(*VPKFileCache)
		cache.File.PrimaryTag = primaryTag
		cache.File.SecondaryTags = append([]string(nil), secondaryTags...)
		if metaInfo, statErr := os.Stat(GetMetaFilePath(filePath)); statErr == nil {
			cache.MetaModTime = metaInfo.ModTime()
		}
		a.vpkCache.Store(filePath, cache)
		return nil
	}

	// 清空标签时重新解析，恢复 VPK 自身可推断出的自动标签。
	if loaded {
		a.vpkCache.Delete(filePath)
		a.deleteVPKPreviewCache(filePath)
	}
	a.processVPKFileWithCache(filePath)
	return nil
}

// RenameVPKFile 重命名VPK文件
func (a *App) RenameVPKFile(filePath string, newFilename string) (string, error) {
	// 尝试保留自定义标签
	oldName := filepath.Base(filePath)
	pTag, sTags, _, oldHasTags := parser.ParseFilenameTags(oldName)

	_, _, _, newHasTags := parser.ParseFilenameTags(newFilename)

	finalFilename := newFilename

	// 如果旧名字有标签，且新名字里没有标签，则将旧标签注入到新名字中
	if oldHasTags && !newHasTags {
		prefix := ""
		body := newFilename
		// 检查新文件名是否有 _ 前缀
		if strings.HasPrefix(newFilename, "_") {
			prefix = "_"
			body = strings.TrimPrefix(newFilename, "_")
		}

		// 组合标签
		allTags := make([]string, 0)
		if pTag != "" {
			allTags = append(allTags, pTag)
		}
		allTags = append(allTags, sTags...)

		if len(allTags) > 0 {
			// 使用 + 作为分隔符
			tagStr := strings.Join(allTags, "+")
			finalFilename = fmt.Sprintf("%s[%s]%s", prefix, tagStr, body)
		}
	}

	dir := filepath.Dir(filePath)

	// 确保新文件名以 .vpk 结尾
	if !strings.HasSuffix(strings.ToLower(finalFilename), ".vpk") {
		finalFilename += ".vpk"
	}

	newPath := filepath.Join(dir, finalFilename)

	if err := validateWindowsRenamePath(newPath, finalFilename); err != nil {
		return "", err
	}

	// Check if target exists
	if _, err := os.Stat(newPath); err == nil {
		return "", fmt.Errorf("目标文件已存在: %s", finalFilename)
	}

	err := os.Rename(filePath, newPath)
	if err != nil {
		return "", err
	}
	// 同步重命名同名图片
	a.handleSidecarFile(filePath, newPath, "move")

	// 更新缓存
	if cached, ok := a.vpkCache.Load(filePath); ok {
		cache := cached.(*VPKFileCache)
		cache.File.Path = newPath
		cache.File.Name = filepath.Base(newPath)
		// Location 应该不变，因为是在同目录下重命名

		a.vpkCache.Delete(filePath)
		a.deleteVPKPreviewCache(filePath)
		a.vpkCache.Store(newPath, cache)
	} else {
		// 如果不在缓存中，重新处理
		a.processVPKFileWithCache(newPath)
	}

	a.updateCompletedDownloadTaskPath(filePath, newPath)
	return newPath, nil
}
