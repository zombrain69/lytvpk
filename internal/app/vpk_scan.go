package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vpk-manager/internal/parser"
	"vpk-manager/internal/platform/protocol"
)

func (a *App) SetRootDirectory(path string) error {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return fmt.Errorf("目录路径不能为空")
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("目录不存在: %s", path)
		}
		return fmt.Errorf("无法访问目录 %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("目标不是目录: %s", path)
	}

	a.mu.Lock()
	a.rootDir = path
	a.mu.Unlock()
	a.restartAddonListMonitor()
	return nil
}

// GetRootDirectory 获取根目录
func (a *App) GetRootDirectory() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.rootDir
}

// GetAppVersion 获取当前版本号
func (a *App) GetAppVersion() string {
	return AppVersion
}

// ScanVPKFiles 扫描所有VPK文件（智能缓存版本）
func (a *App) ScanVPKFiles() error {
	a.mu.RLock()
	rootDir := a.rootDir
	a.mu.RUnlock()
	if rootDir == "" {
		return fmt.Errorf("请先设置根目录")
	}

	var wg sync.WaitGroup

	// 首先扫描所有VPK文件路径
	vpkPaths := make([]string, 0)

	// 扫描根目录（仅扫描根目录本身的VPK文件，不包含子目录）
	err := a.scanRootDirectory(rootDir, &vpkPaths)
	if err != nil {
		return err
	}

	// 扫描workshop目录
	workshopDir := filepath.Join(rootDir, "workshop")
	if _, err := os.Stat(workshopDir); err == nil {
		err = a.scanDirectory(workshopDir, &vpkPaths)
		if err != nil {
			return err
		}
	}

	// 扫描disabled目录
	disabledDir := filepath.Join(rootDir, "disabled")
	if _, err := os.Stat(disabledDir); err == nil {
		err = a.scanDirectory(disabledDir, &vpkPaths)
		if err != nil {
			return err
		}
	}

	// 创建当前文件路径集合，用于清理不存在的缓存
	currentPaths := make(map[string]bool)
	for _, path := range vpkPaths {
		currentPaths[path] = true
	}

	// 清理缓存中不存在的文件
	a.vpkCache.Range(func(key, value interface{}) bool {
		path := key.(string)
		if !currentPaths[path] {
			a.vpkCache.Delete(path)
			a.deleteVPKPreviewCache(path)
			log.Printf("清理缓存: 文件已删除 %s", path)
		}
		return true
	})

	// 并发处理所有文件（使用智能缓存）。任务池不可用时同步回退，
	// 确保 WaitGroup 不会因拒绝任务而永久等待。
	for _, path := range vpkPaths {
		wg.Add(1)
		filePath := path // 捕获变量
		a.submitPoolTask(func() {
			defer wg.Done()
			a.processVPKFileWithCache(filePath)
		})
	}
	wg.Wait()

	// addonlist.txt 的 "0"/"1" 是游戏内开关；它独立于本程序将文件移入
	// disabled 目录的整理状态。扫描完成后统一合并，避免对每个 VPK 重复读文件。
	a.applyAddonListGameStates()

	return nil
}

// scanRootDirectory 扫描根目录中的VPK文件（不包含子目录）
func (a *App) scanRootDirectory(dir string, vpkPaths *[]string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
			fullPath := filepath.Join(dir, entry.Name())
			*vpkPaths = append(*vpkPaths, fullPath)
		}
	}
	return nil
}

// scanDirectory 扫描指定目录中的VPK文件（递归扫描所有子目录）
func (a *App) scanDirectory(dir string, vpkPaths *[]string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(strings.ToLower(path), ".vpk") {
			*vpkPaths = append(*vpkPaths, path)
		}
		return nil
	})
}

// processVPKFileWithCache 处理单个VPK文件（智能缓存版本）
func (a *App) processVPKFileWithCache(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil {
		log.Printf("无法读取文件信息: %s, 错误: %v", filePath, err)
		return
	}

	modTime := info.ModTime()
	size := info.Size()

	// 检查外部图片状态
	var imgModTime time.Time
	basePath := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	exts := []string{".jpg", ".png", ".jpeg", ".gif"}
	for _, ext := range exts {
		if imgInfo, statErr := os.Stat(basePath + ext); statErr == nil {
			imgModTime = imgInfo.ModTime()
			break
		}
	}
	previewRevision := buildVPKPreviewRevision(filePath, modTime, size, imgModTime)

	// 检查meta文件状态
	var metaModTime time.Time
	if metaInfo, statErr := os.Stat(basePath + ".meta"); statErr == nil {
		metaModTime = metaInfo.ModTime()
	}

	// 记住上一次成功读取的游戏内开关。VPK 文件被外部程序触碰后重新解析时，
	// addonlist.txt 可能恰好仍被游戏占用；此时 applyAddonListGameStates 会保留
	// 旧状态，不能因为重建 VPK 元数据而先把它丢成“未记录”。
	var previousGameEnabled bool
	var previousGameStateKnown bool
	var hasPreviousGameState bool

	// 检查缓存
	if cached, ok := a.vpkCache.Load(filePath); ok {
		cache := cached.(*VPKFileCache)
		previousGameEnabled = cache.File.GameEnabled
		previousGameStateKnown = cache.File.GameStateKnown
		hasPreviousGameState = true

		// 判断文件是否变化（通过修改时间和大小）以及图片/meta是否变化
		if cache.ModTime.Equal(modTime) && cache.Size == size && cache.ImageModTime.Equal(imgModTime) && cache.MetaModTime.Equal(metaModTime) {
			// 文件、图片和meta都未变化，使用缓存
			// 但需要更新位置信息（因为文件可能被移动）
			location := a.getLocationFromPath(filePath)
			cache.File.Location = location
			cache.File.Enabled = location != "disabled"
			cache.File.Path = filePath // 更新路径（处理移动情况）
			cache.File.PreviewRevision = previewRevision

			// 更新缓存
			a.vpkCache.Store(filePath, cache)
			log.Printf("使用缓存: %s (未变化)", filepath.Base(filePath))
			return
		}

		log.Printf("文件或图片已变化，重新解析: %s", filepath.Base(filePath))
	}

	// 文件不在缓存中或已变化，需要重新解析
	// List scanning intentionally avoids eager Base64 preview extraction. The
	// frontend requests it later only for visible cards or the detail dialog.
	vpkFile, err := parser.ParseVPKFileMetadata(filePath)
	if err != nil {
		a.LogError("VPK解析", describeVPKParseError(filePath, err), filePath)
		return
	}
	if hasPreviousGameState {
		vpkFile.GameEnabled = previousGameEnabled
		vpkFile.GameStateKnown = previousGameStateKnown
	}

	// 设置文件系统相关信息
	location := a.getLocationFromPath(filePath)
	vpkFile.Size = size
	vpkFile.Location = location
	vpkFile.Enabled = location != "disabled"
	vpkFile.LastModified = modTime.Format(time.RFC3339)
	vpkFile.PreviewRevision = previewRevision
	vpkFile.Path = filePath

	// 自定义标签始终从 .meta 读取：它们是本程序的本地分类数据，不能依赖于工坊详情开关。
	// 这样 workshop\123456.vpk 无需通过重命名来保存标签，Steam 仍可识别原始文件名。
	metaEnabled, updateCheckEnabled := a.workshopOptionsSnapshot()
	if meta, err := LoadWorkshopMeta(filePath); meta != nil && err == nil {
		if meta.PrimaryTag != "" || len(meta.SecondaryTags) > 0 {
			vpkFile.PrimaryTag = parser.CanonicalTag(meta.PrimaryTag)
			vpkFile.SecondaryTags = parser.UniqueTagsExcluding(meta.SecondaryTags, vpkFile.PrimaryTag)
		} else if len(meta.Tags) > 0 {
			metaTags := parser.UniqueTagsExcluding(meta.Tags)
			if len(metaTags) > 0 {
				vpkFile.PrimaryTag = parser.CanonicalTag(metaTags[0])
				vpkFile.SecondaryTags = parser.UniqueTagsExcluding(metaTags[1:], vpkFile.PrimaryTag)
			}
		}

		if metaEnabled {
			if meta.Title != "" {
				vpkFile.Title = meta.Title
			}
			if meta.Author != "" {
				vpkFile.Author = meta.Author
			}
			if meta.Description != "" {
				vpkFile.Desc = meta.Description
			}
			if meta.WorkshopID != "" && !strings.HasPrefix(meta.WorkshopID, "direct-") && protocol.IsValidWorkshopID(meta.WorkshopID) {
				vpkFile.WorkshopID = meta.WorkshopID
			}
			if updateCheckEnabled && meta.TimeUpdated != "" && meta.DownloadedAt != "" {
				timeUpdated, tErr := time.Parse(time.RFC3339, meta.TimeUpdated)
				downloadedAt, dErr := time.Parse(time.RFC3339, meta.DownloadedAt)
				if tErr == nil && dErr == nil && timeUpdated.After(downloadedAt) {
					vpkFile.HasUpdate = true
				}
			}
		}
	}

	// 存入缓存
	cache := &VPKFileCache{
		File:         *vpkFile,
		ModTime:      modTime,
		Size:         size,
		ImageModTime: imgModTime,
		MetaModTime:  metaModTime,
		CachedAt:     time.Now(),
	}
	a.vpkCache.Store(filePath, cache)

	log.Printf("已解析并缓存: %s", filepath.Base(filePath))
}

// buildVPKPreviewRevision identifies the bytes that can affect the card
// preview without including the absolute path. Excluding the path is
// intentional: toggling a Mod between addons and disabled changes its path,
// but not its image, so the frontend can retain the already decoded image.
func buildVPKPreviewRevision(filePath string, modTime time.Time, size int64, imageModTime time.Time) string {
	return fmt.Sprintf("%s|%d|%d|%d",
		strings.ToLower(filepath.Base(filePath)),
		size,
		modTime.UnixNano(),
		imageModTime.UnixNano(),
	)
}

// describeVPKParseError adds a concrete diagnosis for files that merely use a
// .vpk extension. Steam Workshop downloads occasionally leave a ZIP archive in
// place (header PK\x03\x04), which otherwise surfaces as an opaque VPK magic
// error and makes users suspect the addonlist state instead.
func describeVPKParseError(filePath string, parseErr error) string {
	file, err := os.Open(filePath)
	if err != nil {
		return parseErr.Error()
	}
	defer file.Close()

	var header [4]byte
	n, readErr := io.ReadFull(file, header[:])
	if n >= 4 && bytes.Equal(header[:2], []byte{'P', 'K'}) {
		return fmt.Sprintf("文件扩展名为 .vpk，但实际是 ZIP 压缩包（文件头 PK\\x03\\x04）；请重新下载、解压或改正文件后再扫描（原始错误：%v）", parseErr)
	}
	if n >= 4 && !bytes.Equal(header[:], []byte{0x34, 0x12, 0xAA, 0x55}) {
		return fmt.Sprintf("不是有效的 VPK 文件（文件头 % X）；请确认文件未被错误重命名或下载不完整（原始错误：%v）", header, parseErr)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return fmt.Sprintf("无法读取 VPK 文件头：%v（原始错误：%v）", readErr, parseErr)
	}
	return parseErr.Error()
}

// getLocationFromPath 根据文件路径判断位置
func (a *App) getLocationFromPath(filePath string) string {
	rootDir := a.rootDirectorySnapshot()
	rel, _ := filepath.Rel(rootDir, filePath)
	parts := strings.Split(rel, string(filepath.Separator))

	if len(parts) > 0 {
		switch {
		case strings.EqualFold(parts[0], "workshop"):
			return "workshop"
		case strings.EqualFold(parts[0], "disabled"):
			return "disabled"
		default:
			return "root"
		}
	}
	return "root"
}

// GetVPKFiles 获取所有VPK文件（从缓存中读取）
func (a *App) GetVPKFiles() []VPKFile {
	result := make([]VPKFile, 0)

	a.vpkCache.Range(func(key, value interface{}) bool {
		cache := value.(*VPKFileCache)
		file := cache.File
		// 性能优化：列表请求不返回预览图数据，由前端按需加载
		file.PreviewImage = ""
		result = append(result, file)
		return true
	})

	return result
}

func (a *App) SearchVPKFiles(query string, primaryTag string, secondaryTags []string) []VPKFile {
	result := make([]VPKFile, 0)
	query = strings.ToLower(query)

	a.vpkCache.Range(func(key, value interface{}) bool {
		cache := value.(*VPKFileCache)
		vpkFile := cache.File

		// 搜索文本匹配：标题、文件名或标签名
		textMatch := query == ""
		if query != "" {
			// 匹配标题
			if fuzzyMatch(query, strings.ToLower(vpkFile.Title)) {
				textMatch = true
			}
			// 匹配文件名
			if !textMatch && fuzzyMatch(query, strings.ToLower(vpkFile.Name)) {
				textMatch = true
			}
			// 匹配主标签
			if !textMatch && fuzzyMatch(query, strings.ToLower(vpkFile.PrimaryTag)) {
				textMatch = true
			}
			// 匹配二级标签
			if !textMatch {
				for _, tag := range vpkFile.SecondaryTags {
					if fuzzyMatch(query, strings.ToLower(tag)) {
						textMatch = true
						break
					}
				}
			}
			// 匹配结构化主体摘要；主体是面向用户的实际替换对象，
			// 例如“Coach 语音”“M16 武器”“医疗包”。
			if !textMatch && fuzzyMatch(query, strings.ToLower(vpkFile.SubjectSummary)) {
				textMatch = true
			}
			if !textMatch {
				for _, subject := range vpkFile.ContentSubjects {
					if fuzzyMatch(query, strings.ToLower(subject)) {
						textMatch = true
						break
					}
				}
			}
		}

		// 主标签筛选匹配
		primaryMatch := primaryTag == "" || vpkFile.PrimaryTag == primaryTag

		// 二级标签筛选匹配
		secondaryMatch := len(secondaryTags) == 0
		if len(secondaryTags) > 0 {
			for _, tag := range secondaryTags {
				for _, vpkTag := range vpkFile.SecondaryTags {
					if vpkTag == tag {
						secondaryMatch = true
						break
					}
				}
				if secondaryMatch {
					break
				}
			}
		}

		if textMatch && primaryMatch && secondaryMatch {
			// 性能优化：列表请求不返回预览图数据，由前端按需加载
			vpkFile.PreviewImage = ""
			result = append(result, vpkFile)
		}

		return true
	})

	return result
}

// GetPrimaryTags 获取所有主要标签
func (a *App) GetPrimaryTags() []string {
	return parser.GetPrimaryTags()
}

// GetSecondaryTags 获取指定主标签下的所有二级标签（从缓存中获取）
func (a *App) GetSecondaryTags(primaryTag string) []string {
	// 从缓存中收集所有文件
	vpkFiles := make([]VPKFile, 0)
	a.vpkCache.Range(func(key, value interface{}) bool {
		cache := value.(*VPKFileCache)
		vpkFiles = append(vpkFiles, cache.File)
		return true
	})

	return parser.GetSecondaryTags(vpkFiles, primaryTag)
}

func fuzzyMatch(source, target string) bool {
	// 转换为 rune 数组以支持 Unicode
	srcRunes := []rune(source)
	tgtRunes := []rune(target)

	sIdx := 0
	tIdx := 0

	for sIdx < len(srcRunes) && tIdx < len(tgtRunes) {
		if srcRunes[sIdx] == tgtRunes[tIdx] {
			sIdx++
		}
		tIdx++
	}

	return sIdx == len(srcRunes)
}
