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
	"sync/atomic"
	"time"

	"vpk-manager/internal/parser"
	"vpk-manager/internal/platform/protocol"
)

func (a *App) SetRootDirectory(path string) error {
	a.addonListGuardMu.Lock()
	restartMonitor := false
	defer func() {
		a.addonListGuardMu.Unlock()
		if restartMonitor {
			a.restartAddonListMonitor()
			// 目录变了：外部改动监听也要跟着换（监听的是"界面会列出来的那三处"）。
			a.restartAddonsWatcher()
		}
	}()

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
	restartMonitor = true
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
	// 扫描阶段计时：这一行是"持久化缓存到底省了多少"的唯一权威证据
	// （冷启动 = 全量解析，命中缓存 = 只剩遍历目录 + 逐个 stat）。
	scanStartedAt := time.Now()

	var wg sync.WaitGroup

	// 首先扫描所有VPK文件路径
	vpkPaths := make([]string, 0)
	// 同名封面图 / .meta 的时间戳直接从目录项里取，避免每个 VPK 再 stat 5 次。
	sidecars := make(map[string]sidecarInfo)

	// 扫描根目录（仅扫描根目录本身的VPK文件，不包含子目录）
	err := a.scanRootDirectory(rootDir, &vpkPaths, sidecars)
	if err != nil {
		return err
	}

	// 扫描workshop目录
	workshopDir := filepath.Join(rootDir, "workshop")
	if _, err := os.Stat(workshopDir); err == nil {
		err = a.scanDirectory(workshopDir, &vpkPaths, sidecars)
		if err != nil {
			return err
		}
	}

	// 扫描disabled目录
	disabledDir := filepath.Join(rootDir, "disabled")
	if _, err := os.Stat(disabledDir); err == nil {
		err = a.scanDirectory(disabledDir, &vpkPaths, sidecars)
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
			a.deleteVPKPreviewCaches(path)
			log.Printf("清理缓存: 文件已删除 %s", path)
		}
		return true
	})

	// 并发处理所有文件（使用智能缓存）。任务池不可用时同步回退，
	// 确保 WaitGroup 不会因拒绝任务而永久等待。
	// 磁盘扫描缓存：冷启动时 2904 个 VPK 全量解析要 1–1.5s，命中缓存就只剩
	// "遍历目录 + 逐个 stat"（真机约 150–250ms）。判据与内存缓存完全一致，
	// 任何异常都会退回全量解析（见 vpk_scan_cache.go）。
	persistedCache, persistedLoaded := a.loadVPKScanCache()
	if persistedLoaded {
		log.Printf("扫描缓存：载入 %d 条记录（%s）", len(persistedCache), AppVersion)
	}
	var cacheHits int64
	var reparsed int64
	for _, path := range vpkPaths {
		wg.Add(1)
		filePath := path // 捕获变量
		a.submitPoolTask(func() {
			defer wg.Done()
			if a.processVPKFileWithCacheAndPersisted(filePath, sidecars, persistedCache) {
				atomic.AddInt64(&cacheHits, 1)
				return
			}
			atomic.AddInt64(&reparsed, 1)
		})
	}
	wg.Wait()
	// 一行汇总代替"每个文件一行"：日志量从 2904 行降到 1 行。
	hits := int(atomic.LoadInt64(&cacheHits))
	reparsedCount := int(atomic.LoadInt64(&reparsed))
	a.setScanStats(ScanStats{
		Total:          len(vpkPaths),
		Unchanged:      hits,
		Reparsed:       reparsedCount,
		CacheLoaded:    persistedLoaded,
		CacheEntrySize: len(persistedCache),
	})
	log.Printf(
		"扫描完成：共 %d 个 Mod（%d 个未变化、%d 个重新解析，耗时 %s）",
		len(vpkPaths),
		hits,
		reparsedCount,
		time.Since(scanStartedAt).Round(time.Millisecond),
	)
	// 有文件被重新解析（新增/改动/删缓存）才重写缓存；全命中时不写盘。
	// 刻意放在 applySuiteTagInheritance 之前：缓存里存的是"纯解析结果"，
	// 继承标签（依赖同命名空间的其它文件）每次扫描现算，语义与冷启动一致。
	if reparsedCount > 0 || !persistedLoaded {
		a.saveVPKScanCacheAsync()
	}

	// addonlist.txt 的 "0"/"1" 是游戏内开关；它独立于本程序将文件移入
	// disabled 目录的整理状态。扫描完成后统一合并，避免对每个 VPK 重复读文件。
	a.applyAddonListGameStates()

	// W4 通道 5：套件命名空间内的标签继承（只增不减，见 tag_inheritance.go）。
	// 必须放在扫描之后、任何清单导出/分组推导之前，这样下游看到的是同一份结果。
	a.applySuiteTagInheritance()

	return nil
}

// scanRootDirectory 扫描根目录中的VPK文件（不包含子目录）。
// 顺手把同名封面图 / .meta 记进 sidecars：这些目录项本来就已经读出来了。
func (a *App) scanRootDirectory(dir string, vpkPaths *[]string, sidecars map[string]sidecarInfo) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
			*vpkPaths = append(*vpkPaths, fullPath)
			continue
		}
		recordSidecar(fullPath, entry, sidecars)
	}
	return nil
}

// scanDirectory 扫描指定目录中的VPK文件（递归扫描所有子目录），并顺手收集侧车文件。
func (a *App) scanDirectory(dir string, vpkPaths *[]string, sidecars map[string]sidecarInfo) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".vpk") {
			*vpkPaths = append(*vpkPaths, path)
			return nil
		}
		recordSidecar(path, d, sidecars)
		return nil
	})
}

// sidecarInfo 记录与某个 VPK 同名的外部封面图 / .meta 的时间戳。
//
// 为什么需要它：扫描时目录项**本来就已经读出来了**（ReadDir / WalkDir），
// 却还要为每个 VPK 再 os.Stat 5 次（jpg/png/jpeg/gif + meta）。真机 2904 个 Mod
// 就是约 14,500 次系统调用，实测占整轮扫描耗时的 60%（908ms → 362ms）。
type sidecarInfo struct {
	hasImage     bool
	imageRank    int
	imageModTime time.Time
	metaModTime  time.Time
}

// sidecarBaseKey 侧车文件与 VPK 的公共键（Windows 路径大小写不敏感）。
func sidecarBaseKey(path string) string {
	return strings.ToLower(strings.TrimSuffix(path, filepath.Ext(path)))
}

// sidecarImageRank 外部封面图的优先级，必须与原来的 stat 顺序一致（.jpg 最先命中）。
func sidecarImageRank(ext string) int {
	switch strings.ToLower(ext) {
	case ".jpg":
		return 0
	case ".png":
		return 1
	case ".jpeg":
		return 2
	case ".gif":
		return 3
	default:
		return -1
	}
}

// recordSidecar 把目录项里的侧车文件记进索引（只有真的存在才需要取时间戳）。
func recordSidecar(path string, entry fs.DirEntry, out map[string]sidecarInfo) {
	if entry.IsDir() {
		return
	}
	ext := strings.ToLower(filepath.Ext(path))
	rank := sidecarImageRank(ext)
	isMeta := ext == ".meta"
	if rank < 0 && !isMeta {
		return
	}
	info, err := entry.Info()
	if err != nil {
		return
	}
	key := sidecarBaseKey(path)
	current := out[key]
	if isMeta {
		current.metaModTime = info.ModTime()
	} else if !current.hasImage || rank < current.imageRank {
		current.hasImage = true
		current.imageRank = rank
		current.imageModTime = info.ModTime()
	}
	out[key] = current
}

// processVPKFileWithCache 处理单个VPK文件（智能缓存版本）。
//
// 返回值表示"命中缓存、文件未变化"——调用方用它做汇总统计；
// 只关心副作用的调用方可以忽略返回值（Go 允许忽略返回值）。
//
// sidecars 是扫描时顺手收集的"同名封面图 / .meta"索引（可为 nil：单文件刷新
// 的调用方没有目录清单，此时退回逐个 os.Stat 的老逻辑，语义完全一致）。
func (a *App) processVPKFileWithCache(filePath string, sidecars map[string]sidecarInfo) bool {
	return a.processVPKFileWithCacheAndPersisted(filePath, sidecars, nil)
}

// processVPKFileWithCacheAndPersisted 同上，但额外接受一份"磁盘扫描缓存"：
// 命中（size/mtime/侧车都一致）时直接把它当作内存缓存装进去，跳过整份解析。
// persisted 为 nil 时行为与不带缓存完全一致（单文件刷新走的就是这条）。
func (a *App) processVPKFileWithCacheAndPersisted(
	filePath string,
	sidecars map[string]sidecarInfo,
	persisted map[string]vpkScanCacheEntry,
) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		log.Printf("无法读取文件信息: %s, 错误: %v", filePath, err)
		return false
	}

	modTime := info.ModTime()
	size := info.Size()

	// 检查外部图片状态
	var imgModTime time.Time
	basePath := strings.TrimSuffix(filePath, filepath.Ext(filePath))
	var metaModTime time.Time
	if sidecars != nil {
		if entry, ok := sidecars[sidecarBaseKey(basePath)]; ok {
			imgModTime = entry.imageModTime
			metaModTime = entry.metaModTime
		}
	} else {
		exts := []string{".jpg", ".png", ".jpeg", ".gif"}
		for _, ext := range exts {
			if imgInfo, statErr := os.Stat(basePath + ext); statErr == nil {
				imgModTime = imgInfo.ModTime()
				break
			}
		}
		if metaInfo, statErr := os.Stat(basePath + ".meta"); statErr == nil {
			metaModTime = metaInfo.ModTime()
		}
	}
	previewRevision := buildVPKPreviewRevision(filePath, modTime, size, imgModTime)

	// 磁盘扫描缓存：判据与下面"内存缓存命中"完全一致；命中就直接装进内存缓存，
	// 让后续逻辑走同一条 warm 路径（位置/启用状态/预览版本都会照常刷新）。
	if persisted != nil {
		if _, ok := a.vpkCache.Load(filePath); !ok {
			if entry, found := persisted[filePath]; found &&
				vpkScanCacheEntryMatches(entry, size, modTime, imgModTime, metaModTime) {
				a.vpkCache.Store(filePath, &VPKFileCache{
					File:         entry.File,
					ModTime:      modTime,
					Size:         size,
					ImageModTime: imgModTime,
					MetaModTime:  metaModTime,
					CachedAt:     time.Now(),
				})
			}
		}
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
			// 这里**不要**逐个文件打日志：真机 2904 个 Mod 就是每次扫描 2904 行
			// （格式化 + 写 stderr，实测是扫描耗时里可观的一块），
			// 而且会把崩溃报告环形缓冲里真正有用的诊断刷掉。
			// 只在 ScanVPKFiles 结束时打一行汇总。
			return true
		}

		log.Printf("文件或图片已变化，重新解析: %s", filepath.Base(filePath))
	}

	// 文件不在缓存中或已变化，需要重新解析
	// List scanning intentionally avoids eager Base64 preview extraction. The
	// frontend requests it later only for visible cards or the detail dialog.
	vpkFile, err := parser.ParseVPKFileMetadata(filePath)
	var archivePack *parser.ArchivePackInfo
	if err != nil {
		// 「扩展名是 .vpk、实际是压缩包」不是异常：工坊作者会特意把插件/工具/教程包
		// 打成压缩包上传（首次安装或自动更新用），游戏根本不加载它。以前这里一律当
		// 解析失败，界面上会弹红色「解析错误」——那是误报。
		//
		// 但它**照常进列表**：真机上这两个包都占着 addonlist.txt 的一行（也就是占了
		// 优先级位置），用户需要能看到它、把它关掉或移走。所以这里构造一个只有文件
		// 信息的条目，并盖上 ArchivePack 标记，后续走和普通 Mod 一样的缓存/展示路径。
		if pack, ok := parser.DescribeArchivePack(filePath); ok {
			a.recordArchivePack(filePath, pack)
			a.clearUnreadableMod(filePath)
			archivePack = &pack
			vpkFile = &parser.VPKFile{
				Name: filepath.Base(filePath),
				// 工坊条目用数字 ID 当标题；有 .meta 时后面还会被真实标题覆盖。
				Title: strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath)),
			}
			log.Printf("压缩包类条目进列表: %s（%s / %s，%d 个条目）— %s",
				filepath.Base(filePath), pack.Label, pack.Format, pack.EntryCount, pack.Note)
		} else {
			a.LogError("VPK解析", describeVPKParseError(filePath, err), filePath)
			a.recordUnreadableMod(filePath, describeVPKParseError(filePath, err))
			return false
		}
	} else {
		a.clearUnreadableMod(filePath)
		a.clearArchivePack(filePath)
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
	if archivePack != nil {
		// 标记"这不是 VPK"：前端据此显示显眼徽标，并按"压缩包类条目"处理详情/操作。
		vpkFile.ArchivePack = archivePack
	}

	// 自定义标签始终从 .meta 读取：它们是本程序的本地分类数据，不能依赖于工坊详情开关。
	// 这样 workshop\123456.vpk 无需通过重命名来保存标签，Steam 仍可识别原始文件名。
	metaEnabled, updateCheckEnabled := a.workshopOptionsSnapshot()
	if meta, err := LoadWorkshopMeta(filePath); meta != nil && err == nil {
		if meta.PrimaryTag != "" || len(meta.SecondaryTags) > 0 {
			// 只增不减：自定义标签与自动识别结果合并（自动一级标签降级为二级保留）。
			// 曾经的硬覆盖让 942 个工坊 Mod 的自动识别标签完全不可见（D13）。
			parser.ApplyCustomTagOverride(vpkFile, meta.PrimaryTag, meta.SecondaryTags)
		} else if len(meta.Tags) > 0 {
			metaTags := parser.UniqueTagsExcluding(meta.Tags)
			if len(metaTags) > 0 {
				parser.ApplyCustomTagOverride(vpkFile, metaTags[0], metaTags[1:])
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

	// 同上：逐个文件的"已解析"日志在大库上就是刷屏，改成调用方汇总。
	return false
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
		// 真实库证据：workshop\3558049615.vpk 是 ZIP，里面装的是 Left4Neko 工具包
		// （bin/*.dll、*.exe、*.bat），根本不是 Mod —— 这种条目重下多少次都一样，
		// 所以不能只说"请重新下载"。
		return fmt.Sprintf("文件扩展名为 .vpk，但实际是 ZIP 压缩包（文件头 PK\\x03\\x04）：工坊里有些条目上传的就是压缩包或工具包，游戏不会加载它。如果里面是 Mod，解压后把 .vpk 放回 addons；如果它其实是工具/素材包，建议直接从 addons 里删掉（原始错误：%v）", parseErr)
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
// allVPKFilesSnapshot 返回缓存里的**完整**文件对象（含 tagEvidence / structure*），
// 只给 Go 内部调用（问题扫描、随机轮换、目录快照……）。
// 给前端的 GetVPKFiles / SearchVPKFiles 会先剥掉"列表根本不用"的重字段。
func (a *App) allVPKFilesSnapshot() []VPKFile {
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

// stripListOnlyEvidence 复制一份并清掉"只有详情弹窗 / 清单导出才需要"的重字段。
//
// 真机实测（2904 个 Mod）：完整 payload 6.11MB / 一次 IPC 340ms，其中
//   - tagEvidence          2.15MB（2896 条，平均 742 字节）
//   - structureSamplePaths 623KB
//   - structureTargets     504KB
//   - structureTopDirs     106KB
//
// 合起来占 55%，而前端**列表**一个都不用（tagEvidence 只在详情弹窗里显示，
// structure* 更是只有 Go 侧的分组目录导出在用）。剥掉后详情按需调 GetModEvidence。
//
// 注意：这里复制的是结构体值，清空字段只影响这一份副本，缓存里的原始数据不动。
func stripListOnlyEvidence(files []VPKFile) []VPKFile {
	for index := range files {
		files[index].TagEvidence = nil
		files[index].StructureTopDirs = nil
		files[index].StructureSamplePaths = nil
		files[index].StructureTargets = nil
	}
	return files
}

func (a *App) GetVPKFiles() []VPKFile {
	files := stripListOnlyEvidence(a.allVPKFilesSnapshot())
	return attachXDRPriority(files, a.xdrPriorityIndex())
}

func (a *App) SearchVPKFiles(query string, primaryTag string, secondaryTags []string) []VPKFile {
	result := make([]VPKFile, 0)
	searchSpec := parseModSearchQuery(query)

	a.vpkCache.Range(func(key, value interface{}) bool {
		cache := value.(*VPKFileCache)
		vpkFile := cache.File

		// 搜索匹配：支持搜索框语法（tag: / -tag: / re:，见 search_query.go），
		// 覆盖标题、文件名、标签、结构化主体、发音角色与动作槽。
		textMatch := searchSpec.matches(searchableModFields{
			Title:           vpkFile.Title,
			Name:            vpkFile.Name,
			PrimaryTag:      vpkFile.PrimaryTag,
			SecondaryTags:   vpkFile.SecondaryTags,
			SubjectSummary:  vpkFile.SubjectSummary,
			ContentSubjects: vpkFile.ContentSubjects,
			VoiceCharacters: vpkFile.VoiceCharacters,
			XDRSummary:      vpkFile.XDRSummary,
		})

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
			// 性能优化：列表请求不返回预览图数据、也不返回"依据类"重字段，
			// 两者都由前端按需加载（见 stripListOnlyEvidence 的说明）。
			vpkFile.PreviewImage = ""
			vpkFile.TagEvidence = nil
			vpkFile.StructureTopDirs = nil
			vpkFile.StructureSamplePaths = nil
			vpkFile.StructureTargets = nil
			result = append(result, vpkFile)
		}

		return true
	})

	// 搜索结果同样要带"XDR 动作会不会播"的结论：对手来自全量列表，不是当前筛出来的这一批。
	return attachXDRPriority(result, a.xdrPriorityIndex())
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

// GetSecondaryTagCounts 获取指定主标签下每个二级标签的 Mod 命中数（key 为小写标签）。
// 筛选条默认收起一行：按"常用度"排序才能让收起时看到的是真正高频的子标签。
func (a *App) GetSecondaryTagCounts(primaryTag string) map[string]int {
	vpkFiles := make([]VPKFile, 0)
	a.vpkCache.Range(func(key, value interface{}) bool {
		cache := value.(*VPKFileCache)
		vpkFiles = append(vpkFiles, cache.File)
		return true
	})

	return parser.SecondaryTagCounts(vpkFiles, primaryTag)
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
