package app

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"vpk-manager/internal/parser"
)

const (
	dropImportKindVPK         = "vpk"
	dropImportKindArchive     = "archive"
	dropImportKindFolder      = "folder"
	dropImportKindDump        = "dump"
	dropImportKindUnsupported = "unsupported"
)

// DropImportResult is returned after processing paths dropped or selected by the user.
type DropImportResult struct {
	Total             int                    `json:"total"`
	Succeeded         int                    `json:"succeeded"`
	Failed            int                    `json:"failed"`
	Items             []DropImportItemResult `json:"items"`
	HasInstallChanges bool                   `json:"hasInstallChanges"`
}

type DropImportItemResult struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	OutputPath string `json:"outputPath"`
}

type DropImportProgress struct {
	Current     int      `json:"current"`
	Total       int      `json:"total"`
	Percent     int      `json:"percent"`
	Phase       string   `json:"phase"`
	Path        string   `json:"path"`
	Name        string   `json:"name"`
	Message     string   `json:"message"`
	ActiveNames []string `json:"activeNames"`
}

type dropImportProgressFunc = func(percent int, message string)
type archiveProgressFunc = func(percent int, message string, activeNames []string)

type dropZipEntry struct {
	file        *zip.File
	decodedName string
	size        int64
}

type dropRarEntry struct {
	name string
	size int64
}

type drop7zEntry struct {
	file *sevenzip.File
	name string
	size int64
}

func (a *App) HandleFileDrop(paths []string) (DropImportResult, error) {
	result := DropImportResult{
		Total: len(paths),
		Items: make([]DropImportItemResult, 0, len(paths)),
	}

	for index, rawPath := range paths {
		item := a.handleDroppedPath(index+1, len(paths), strings.TrimSpace(rawPath))
		result.Items = append(result.Items, item)
		if item.Success {
			result.Succeeded++
			if item.Kind == dropImportKindVPK || item.Kind == dropImportKindArchive || item.Kind == dropImportKindFolder {
				result.HasInstallChanges = true
			}
		} else {
			result.Failed++
		}
	}

	if result.HasInstallChanges && a.ctx != nil {
		runtime.EventsEmit(a.ctx, "refresh_files", nil)
	}
	return result, nil
}

func (a *App) handleDroppedPath(current int, total int, targetPath string) DropImportItemResult {
	item := DropImportItemResult{
		Path: targetPath,
		Name: filepath.Base(targetPath),
	}
	if targetPath == "" {
		item.Kind = dropImportKindUnsupported
		item.Message = "路径不能为空"
		a.emitDropImportProgress(current, total, item, 100, "failed", item.Message)
		return item
	}

	kind, err := classifyDropImportPath(targetPath)
	item.Kind = kind
	if err != nil {
		item.Message = err.Error()
		a.emitDropImportProgress(current, total, item, 100, "failed", item.Message)
		return item
	}

	if kind == dropImportKindDump {
		item.Success = true
		item.Message = "已交给崩溃转储分析器"
		a.emitDropImportProgress(current, total, item, 100, "complete", item.Message)
		return item
	}

	if kind == dropImportKindUnsupported {
		item.Message = "仅支持 .vpk, .zip, .rar, .7z, 文件夹, .mdmp, .dmp"
		a.emitDropImportProgress(current, total, item, 100, "failed", item.Message)
		return item
	}

	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		item.Message = "请先设置游戏 addons 目录"
		a.emitDropImportProgress(current, total, item, 100, "failed", item.Message)
		return item
	}

	progress := func(phase string) dropImportProgressFunc {
		return func(percent int, message string) {
			a.emitDropImportProgress(current, total, item, percent, phase, message)
		}
	}
	archiveProgress := func(phase string) archiveProgressFunc {
		return func(percent int, message string, activeNames []string) {
			a.emitDropImportProgress(current, total, item, percent, phase, message, activeNames)
		}
	}

	a.emitDropImportProgress(current, total, item, 0, "preparing", "准备处理...")
	switch kind {
	case dropImportKindVPK:
		outputPath, err := a.installVPKFile(targetPath, progress("copying"))
		item.OutputPath = outputPath
		if err != nil {
			item.Message = fmt.Sprintf("安装 VPK 失败: %v", err)
			a.emitDropImportProgress(current, total, item, 100, "failed", item.Message)
			return item
		}
		item.Success = true
		// 同名不覆盖：另存为 name(1).vpk，并把这件事说清楚。
		if filepath.Base(outputPath) != filepath.Base(targetPath) {
			item.Message = fmt.Sprintf("VPK 安装完成（已存在同名 Mod，另存为 %s）", filepath.Base(outputPath))
		} else {
			item.Message = "VPK 安装完成"
		}
		// 仍然照旧复制（不减少已有能力），但"扩展名是 .vpk、内容却不是 VPK"时不能说"安装完成"：
		// 游戏读不了这种文件，用户会以为装好了。压缩包装的情况已经在上面按内容分流去解包了，
		// 走到这里说明文件头既不是 VPK 也不是 zip/rar/7z。
		if container, readable := sniffDropImportContainer(targetPath); readable && container != dropContainerVPK {
			item.Message = "已复制到 addons，但这个文件的文件头不是 VPK 魔数（也不是 zip/rar/7z 压缩包）：游戏不会加载它。可能下载不完整或被改过名，请重新下载；如果它其实是压缩包，改名为 .zip 再拖进来即可解包。"
		}
	case dropImportKindArchive:
		err := a.extractVPKFromArchiveWithProgress(targetPath, rootDir, archiveProgress("extracting"))
		if err != nil {
			item.Message = fmt.Sprintf("解压压缩包失败: %v", err)
			a.emitDropImportProgress(current, total, item, 100, "failed", item.Message)
			return item
		}
		item.Success = true
		item.Message = "压缩包导入完成"
		// 扩展名和实际容器不一致时说清楚（例如工坊条目 <id>.vpk 其实是 zip），
		// 否则用户会以为"我拖的是 VPK，怎么走了压缩包流程"。
		if container, readable := sniffDropImportContainer(targetPath); readable && container != dropContainerUnknown {
			if extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(targetPath)), "."); extension != container {
				item.Message = fmt.Sprintf("压缩包导入完成（文件扩展名是 .%s，实际是 %s 压缩包）", extension, container)
			}
		}
	case dropImportKindFolder:
		packResult, err := a.packVPKDirectoryWithProgress(targetPath, rootDir, true, progress("packing"))
		item.OutputPath = packResult.OutputPath
		if err != nil {
			item.Message = fmt.Sprintf("打包文件夹失败: %v", err)
			a.emitDropImportProgress(current, total, item, 100, "failed", item.Message)
			return item
		}
		item.Success = true
		item.Message = "文件夹已打包为 VPK"
	}

	a.emitDropImportProgress(current, total, item, 100, "complete", item.Message)
	return item
}

func classifyDropImportPath(targetPath string) (string, error) {
	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return dropImportKindUnsupported, fmt.Errorf("路径不存在: %s", targetPath)
		}
		return dropImportKindUnsupported, fmt.Errorf("无法访问路径: %v", err)
	}
	if info.IsDir() {
		return dropImportKindFolder, nil
	}

	extension := strings.ToLower(filepath.Ext(targetPath))
	switch extension {
	case ".mdmp", ".dmp":
		return dropImportKindDump, nil
	}

	// 扩展名会撒谎：真实库里 workshop\3558049615.vpk 就是一个 ZIP（作者上传的是压缩包，
	// Steam 按工坊约定存成 <id>.vpk）。这里按文件头纠正 .vpk ↔ 压缩包 的错配，
	// 其它扩展名保持原判定，不扩大入口面。
	container, _ := sniffDropImportContainer(targetPath)
	switch extension {
	case ".vpk":
		if container != dropContainerVPK && dropImportKindForContainer(container) == dropImportKindArchive {
			return dropImportKindArchive, nil
		}
		return dropImportKindVPK, nil
	case ".zip", ".rar", ".7z":
		if container == dropContainerVPK {
			return dropImportKindVPK, nil
		}
		return dropImportKindArchive, nil
	default:
		return dropImportKindUnsupported, nil
	}
}

// 容器格式（按文件头判定，与扩展名无关）。
const (
	dropContainerUnknown = ""
	dropContainerVPK     = "vpk"
	dropContainerZIP     = "zip"
	dropContainerRAR     = "rar"
	dropContainer7z      = "7z"
)

// sniffDropImportContainer 只读前 8 字节判断容器格式。
// readable=false 表示文件头读不到（被独占占用、权限不足等）——调用方不要据此拒绝文件。
//
// 文件头表只有一份：internal/parser 的 SniffContainer（扫描分类也用它），
// 这里只把它的容器名映射成本包的常量，避免两份魔术数字表各自漂移。
func sniffDropImportContainer(path string) (container string, readable bool) {
	sniffed, readable := parser.SniffContainer(path)
	switch sniffed {
	case parser.ContainerVPK:
		return dropContainerVPK, readable
	case parser.ContainerZIP:
		return dropContainerZIP, readable
	case parser.ContainerRAR:
		return dropContainerRAR, readable
	case parser.Container7z:
		return dropContainer7z, readable
	default:
		return dropContainerUnknown, readable
	}
}

func dropImportKindForContainer(container string) string {
	switch container {
	case dropContainerVPK:
		return dropImportKindVPK
	case dropContainerZIP, dropContainerRAR, dropContainer7z:
		return dropImportKindArchive
	default:
		return ""
	}
}

func (a *App) emitDropImportProgress(current int, total int, item DropImportItemResult, percent int, phase string, message string, activeNames ...[]string) {
	if a.ctx == nil {
		return
	}
	names := []string(nil)
	if len(activeNames) > 0 {
		names = activeNames[0]
	}
	runtime.EventsEmit(a.ctx, "drop_import_progress", DropImportProgress{
		Current:     current,
		Total:       total,
		Percent:     normalizeProgressPercent(percent),
		Phase:       phase,
		Path:        item.Path,
		Name:        item.Name,
		Message:     message,
		ActiveNames: names,
	})
}

func (a *App) installVPKFile(srcPath string, progress dropImportProgressFunc) (string, error) {
	emit := func(percent int, message string) {
		if progress != nil {
			progress(percent, message)
		}
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return "", err
	}

	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return "", fmt.Errorf("请先设置游戏 addons 目录")
	}
	// 真机复现：拖入与已有 Mod 同名的 VPK 会被静默覆盖（767 → 1848 字节，提示只有"安装完成"）。
	// 导入外来文件不该动用户已有的 Mod：同名时另存为 name(1).vpk。
	destPath := uniqueImportTarget(rootDir, filepath.Base(srcPath))
	dst, err := os.CreateTemp(rootDir, "."+filepath.Base(srcPath)+".tmp-*")
	if err != nil {
		return "", err
	}
	tempPath := dst.Name()

	abort := func(failErr error) (string, error) {
		_ = dst.Close()
		_ = os.Remove(tempPath)
		return destPath, failErr
	}

	emit(0, fmt.Sprintf("正在复制: %s", filepath.Base(srcPath)))
	var copied int64
	if err := copyStreamWithProgress(dst, src, func(delta int64) {
		copied += delta
		emit(scaledProgressPercent(0, 100, copied, info.Size()), fmt.Sprintf("正在复制: %s", filepath.Base(srcPath)))
	}); err != nil {
		return abort(err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tempPath)
		return destPath, err
	}
	if err := replaceFile(tempPath, destPath); err != nil {
		_ = os.Remove(tempPath)
		return destPath, err
	}

	log.Printf("已安装: %s -> %s", srcPath, destPath)
	emit(100, fmt.Sprintf("已复制: %s", filepath.Base(destPath)))
	return destPath, nil
}

func (a *App) extractVPKFromArchiveWithProgress(archivePath string, destDir string, progress archiveProgressFunc) error {
	// 先按文件头选解压器：.vpk 里其实是压缩包的条目按扩展名分发会直接报
	// "不支持的压缩格式: .vpk"，用户拿不到解包结果。
	format, readable := sniffDropImportContainer(archivePath)
	if !readable || format == dropContainerUnknown || format == dropContainerVPK {
		format = ""
		switch strings.ToLower(filepath.Ext(archivePath)) {
		case ".zip":
			format = dropContainerZIP
		case ".rar":
			format = dropContainerRAR
		case ".7z":
			format = dropContainer7z
		}
	}
	switch format {
	case dropContainerZIP:
		return a.extractVPKFromZipWithProgress(archivePath, destDir, progress)
	case dropContainerRAR:
		return extractVPKFromRarWithProgress(archivePath, destDir, progress)
	case dropContainer7z:
		return a.extractVPKFrom7zWithProgress(archivePath, destDir, progress)
	default:
		return fmt.Errorf("不支持的压缩格式: %s", filepath.Ext(archivePath))
	}
}

func (a *App) extractVPKFromZipWithProgress(zipPath string, destDir string, progress archiveProgressFunc) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return describeArchiveOpenFailure(zipPath, "zip", err)
	}
	defer r.Close()

	allEntries := make([]dropZipEntry, 0, len(r.File))
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := decodeZipEntryName(f)
		allEntries = append(allEntries, dropZipEntry{file: f, decodedName: name, size: int64(f.UncompressedSize64)})
	}

	vpkBases := make(map[string]bool)
	for _, entry := range allEntries {
		if isVPKName(entry.decodedName) {
			vpkBases[entryBaseWithoutExt(entry.decodedName)] = true
		}
	}
	if len(vpkBases) == 0 {
		return fmt.Errorf("ZIP文件中未找到VPK文件")
	}

	selected := make([]dropZipEntry, 0)
	for _, entry := range allEntries {
		if isVPKName(entry.decodedName) {
			selected = append(selected, entry)
		}
	}
	for _, entry := range allEntries {
		if isVPKName(entry.decodedName) {
			continue
		}
		if vpkBases[entryBaseWithoutExt(entry.decodedName)] && isSupportedSidecarName(entry.decodedName) {
			selected = append(selected, entry)
		}
	}

	return a.extractZipEntriesParallel(selected, destDir, progress)
}

func extractVPKFromRarWithProgress(rarPath string, destDir string, progress archiveProgressFunc) error {
	allEntries, err := listRarEntries(rarPath)
	if err != nil {
		return err
	}

	vpkBases := make(map[string]bool)
	for _, entry := range allEntries {
		if isVPKName(entry.name) {
			vpkBases[entryBaseWithoutExt(entry.name)] = true
		}
	}
	if len(vpkBases) == 0 {
		return fmt.Errorf("RAR文件中未找到VPK文件")
	}

	selected := make(map[string]dropRarEntry)
	for _, entry := range allEntries {
		if isVPKName(entry.name) || (vpkBases[entryBaseWithoutExt(entry.name)] && isSupportedSidecarName(entry.name)) {
			selected[entry.name] = entry
		}
	}

	f, err := os.Open(rarPath)
	if err != nil {
		return describeArchiveOpenFailure(rarPath, "rar", err)
	}
	defer f.Close()

	r, err := rardecode.NewReader(f, "")
	if err != nil {
		return describeArchiveOpenFailure(rarPath, "rar", err)
	}

	totalBytes := totalRarEntryBytes(selected)
	progress(0, fmt.Sprintf("准备解压 %d 个文件", len(selected)), nil)
	var completedBytes int64
	completedEntries := 0
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取RAR内容失败: %v", err)
		}
		if header.IsDir {
			continue
		}
		name := header.Name
		if decoded, decodeErr := parser.DecodeVPKEntryName(name); decodeErr == nil {
			name = decoded
		}
		entry, ok := selected[name]
		if !ok {
			continue
		}
		writtenPath, err := extractReaderEntryWithProgress(r, entry.name, destDir, func(delta int64) {
			completedBytes += delta
			progress(archivePercent(completedBytes, totalBytes, completedEntries, len(selected)), fmt.Sprintf("正在解压: %s", filepath.Base(entry.name)), nil)
		})
		if err != nil {
			return fmt.Errorf("解压 %s 失败: %v", entry.name, err)
		}
		completedEntries++
		progress(archivePercent(completedBytes, totalBytes, completedEntries, len(selected)), fmt.Sprintf("已解压: %s", filepath.Base(writtenPath)), nil)
	}

	if completedEntries == 0 {
		return fmt.Errorf("RAR文件中未找到VPK文件")
	}
	progress(100, fmt.Sprintf("已解压 %d 个文件", completedEntries), nil)
	return nil
}

func (a *App) extractVPKFrom7zWithProgress(sevenZPath string, destDir string, progress archiveProgressFunc) error {
	r, err := sevenzip.OpenReader(sevenZPath)
	if err != nil {
		return describeArchiveOpenFailure(sevenZPath, "7z", err)
	}
	defer r.Close()

	allEntries := make([]drop7zEntry, 0, len(r.File))
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := f.Name
		if decoded, decodeErr := parser.DecodeVPKEntryName(name); decodeErr == nil {
			name = decoded
		}
		allEntries = append(allEntries, drop7zEntry{file: f, name: name, size: f.FileInfo().Size()})
	}

	vpkBases := make(map[string]bool)
	for _, entry := range allEntries {
		if isVPKName(entry.name) {
			vpkBases[entryBaseWithoutExt(entry.name)] = true
		}
	}
	if len(vpkBases) == 0 {
		return fmt.Errorf("7z文件中未找到VPK文件")
	}

	selected := make([]drop7zEntry, 0)
	for _, entry := range allEntries {
		if isVPKName(entry.name) {
			selected = append(selected, entry)
		}
	}
	for _, entry := range allEntries {
		if isVPKName(entry.name) {
			continue
		}
		if vpkBases[entryBaseWithoutExt(entry.name)] && isSupportedSidecarName(entry.name) {
			selected = append(selected, entry)
		}
	}

	return a.extract7zEntriesParallel(selected, destDir, progress)
}

func (a *App) extractZipEntriesParallel(entries []dropZipEntry, destDir string, progress archiveProgressFunc) error {
	totalBytes := totalZipEntryBytes(entries)
	var progressMu sync.Mutex
	emitProgress := func(percent int, message string, activeNames []string) {
		if progress == nil {
			return
		}
		// 进度回调可能更新前端状态或测试切片；并行解压时统一串行派发，
		// 避免调用方必须自行处理多个 goroutine 同时回调。
		progressMu.Lock()
		defer progressMu.Unlock()
		progress(percent, message, activeNames)
	}
	emitProgress(0, fmt.Sprintf("准备并行解压 %d 个文件", len(entries)), nil)

	var mu sync.Mutex
	var wg sync.WaitGroup
	var completedBytes int64
	var completedEntries int
	var errs []string
	activeVPKs := make(map[string]bool)

	run := func(entry dropZipEntry) {
		defer wg.Done()
		activeName := filepath.Base(entry.decodedName)
		if isVPKName(entry.decodedName) {
			mu.Lock()
			activeVPKs[activeName] = true
			percent := archivePercent(completedBytes, totalBytes, completedEntries, len(entries))
			activeNames := activeArchiveNames(activeVPKs)
			mu.Unlock()
			emitProgress(percent, "正在并行解压 VPK", activeNames)
		}
		writtenPath, err := extractZipEntryWithProgress(entry.file, entry.decodedName, destDir, func(delta int64) {
			mu.Lock()
			completedBytes += delta
			percent := archivePercent(completedBytes, totalBytes, completedEntries, len(entries))
			message := parallelArchiveProgressMessage(entry.decodedName)
			activeNames := activeArchiveNames(activeVPKs)
			mu.Unlock()
			emitProgress(percent, message, activeNames)
		})
		if err != nil {
			mu.Lock()
			delete(activeVPKs, activeName)
			errs = append(errs, fmt.Sprintf("%s: %v", entry.decodedName, err))
			mu.Unlock()
			return
		}
		mu.Lock()
		delete(activeVPKs, activeName)
		completedEntries++
		percent := archivePercent(completedBytes, totalBytes, completedEntries, len(entries))
		message := fmt.Sprintf("已解压: %s", filepath.Base(writtenPath))
		activeNames := activeArchiveNames(activeVPKs)
		mu.Unlock()
		emitProgress(percent, message, activeNames)
	}

	for _, entry := range entries {
		wg.Add(1)
		entry := entry
		if a.goroutinePool == nil {
			run(entry)
			continue
		}
		if err := a.goroutinePool.Submit(func() { run(entry) }); err != nil {
			wg.Done()
			mu.Lock()
			errs = append(errs, fmt.Sprintf("提交解压任务失败 %s: %v", entry.decodedName, err))
			mu.Unlock()
		}
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("部分ZIP文件解压失败:\n%s", strings.Join(errs, "\n"))
	}
	emitProgress(100, fmt.Sprintf("并行解压完成，共 %d 个文件", len(entries)), nil)
	return nil
}

func (a *App) extract7zEntriesParallel(entries []drop7zEntry, destDir string, progress archiveProgressFunc) error {
	totalBytes := total7zEntryBytes(entries)
	var progressMu sync.Mutex
	emitProgress := func(percent int, message string, activeNames []string) {
		if progress == nil {
			return
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		progress(percent, message, activeNames)
	}
	emitProgress(0, fmt.Sprintf("准备并行解压 %d 个文件", len(entries)), nil)

	var mu sync.Mutex
	var wg sync.WaitGroup
	var completedBytes int64
	var completedEntries int
	var errs []string
	activeVPKs := make(map[string]bool)

	run := func(entry drop7zEntry) {
		defer wg.Done()
		activeName := filepath.Base(entry.name)
		if isVPKName(entry.name) {
			mu.Lock()
			activeVPKs[activeName] = true
			percent := archivePercent(completedBytes, totalBytes, completedEntries, len(entries))
			activeNames := activeArchiveNames(activeVPKs)
			mu.Unlock()
			emitProgress(percent, "正在并行解压 VPK", activeNames)
		}
		writtenPath, err := extract7zEntryWithProgress(entry.file, entry.name, destDir, func(delta int64) {
			mu.Lock()
			completedBytes += delta
			percent := archivePercent(completedBytes, totalBytes, completedEntries, len(entries))
			message := parallelArchiveProgressMessage(entry.name)
			activeNames := activeArchiveNames(activeVPKs)
			mu.Unlock()
			emitProgress(percent, message, activeNames)
		})
		if err != nil {
			mu.Lock()
			delete(activeVPKs, activeName)
			errs = append(errs, fmt.Sprintf("%s: %v", entry.name, err))
			mu.Unlock()
			return
		}
		mu.Lock()
		delete(activeVPKs, activeName)
		completedEntries++
		percent := archivePercent(completedBytes, totalBytes, completedEntries, len(entries))
		message := fmt.Sprintf("已解压: %s", filepath.Base(writtenPath))
		activeNames := activeArchiveNames(activeVPKs)
		mu.Unlock()
		emitProgress(percent, message, activeNames)
	}

	for _, entry := range entries {
		wg.Add(1)
		entry := entry
		if a.goroutinePool == nil {
			run(entry)
			continue
		}
		if err := a.goroutinePool.Submit(func() { run(entry) }); err != nil {
			wg.Done()
			mu.Lock()
			errs = append(errs, fmt.Sprintf("提交解压任务失败 %s: %v", entry.name, err))
			mu.Unlock()
		}
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("部分7z文件解压失败:\n%s", strings.Join(errs, "\n"))
	}
	emitProgress(100, fmt.Sprintf("并行解压完成，共 %d 个文件", len(entries)), nil)
	return nil
}

func listRarEntries(rarPath string) ([]dropRarEntry, error) {
	f, err := os.Open(rarPath)
	if err != nil {
		return nil, describeArchiveOpenFailure(rarPath, "rar", err)
	}
	defer f.Close()

	r, err := rardecode.NewReader(f, "")
	if err != nil {
		return nil, describeArchiveOpenFailure(rarPath, "rar", err)
	}

	var entries []dropRarEntry
	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取RAR内容失败: %v", err)
		}
		if header.IsDir {
			continue
		}
		name := header.Name
		if decoded, decodeErr := parser.DecodeVPKEntryName(name); decodeErr == nil {
			name = decoded
		}
		entries = append(entries, dropRarEntry{name: name, size: header.UnPackedSize})
	}
	return entries, nil
}

func extractZipEntryWithProgress(file *zip.File, name string, destDir string, onDelta func(int64)) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return extractReaderEntryWithProgress(rc, name, destDir, onDelta)
}

func extract7zEntryWithProgress(file *sevenzip.File, name string, destDir string, onDelta func(int64)) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return extractReaderEntryWithProgress(rc, name, destDir, onDelta)
}

// extractReaderEntryWithProgress 写出一个条目，返回实际写入的路径。
// 同名时另存为 name(1).ext —— 导入外来文件不该覆盖用户已有的 Mod。
func extractReaderEntryWithProgress(reader io.Reader, name string, destDir string, onDelta func(int64)) (string, error) {
	// O_EXCL 占名：并发解压同名条目时不会互相覆盖（见 createUniqueFile 注释）。
	outFile, targetPath, err := createUniqueFile(destDir, filepath.Base(name))
	if err != nil {
		return "", err
	}

	if err := copyStreamWithProgress(outFile, reader, onDelta); err != nil {
		_ = outFile.Close()
		_ = os.Remove(targetPath)
		return "", err
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(targetPath)
		return "", err
	}
	return targetPath, nil
}

// uniqueImportTarget 返回 destDir 下不冲突的目标路径：
// 同名时按 "名字(1).ext / 名字(2).ext" 递增（与打包器同一套约定）。
func uniqueImportTarget(destDir, baseName string) string {
	baseName = strings.TrimSpace(baseName)
	if baseName == "" {
		baseName = "imported.vpk"
	}
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	if stem == "" {
		stem = "imported"
	}
	for index := 0; index < 10000; index++ {
		name := baseName
		if index > 0 {
			name = fmt.Sprintf("%s(%d)%s", stem, index, ext)
		}
		candidate := filepath.Join(destDir, name)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
	return filepath.Join(destDir, baseName)
}

// createUniqueFile 原子地占住 destDir 下一个不冲突的文件名，并返回已打开的文件。
//
// 为什么不能"先 uniqueImportTarget 再 os.Create"：解压是并发跑的（协程池），
// 压缩包里 dir1/x.vpk 与 dir2/x.vpk 这类**同名不同目录**的条目会同时通过
// "目标不存在"的检查，然后其中一个 os.Create 把另一个刚写好的文件截断 ——
// 用户看到"解压完成"，实际只留下一个 Mod（甚至两个写入者交错写同一个文件）。
// 这里用 O_CREATE|O_EXCL 逐个试名字：拿到就用，拿不到（已被别的 goroutine 抢先）
// 就递增到 x(1).vpk，天然没有竞态，命名规则与 uniqueImportTarget 保持一致。
func createUniqueFile(destDir, baseName string) (*os.File, string, error) {
	baseName = strings.TrimSpace(filepath.Base(baseName))
	if baseName == "" || baseName == "." || baseName == string(filepath.Separator) {
		baseName = "imported.vpk"
	}
	ext := filepath.Ext(baseName)
	stem := strings.TrimSuffix(baseName, ext)
	if stem == "" {
		stem = "imported"
	}
	for index := 0; index < 10000; index++ {
		name := baseName
		if index > 0 {
			name = fmt.Sprintf("%s(%d)%s", stem, index, ext)
		}
		candidate := filepath.Join(destDir, name)
		file, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			return file, candidate, nil
		}
		if os.IsExist(err) {
			continue
		}
		// Windows 上"目标是个目录 / 权限不足"未必回报 IsExist：
		// 只要这个路径确实已经被占用，就当冲突继续试下一个名字。
		if _, statErr := os.Stat(candidate); statErr == nil {
			continue
		}
		return nil, "", err
	}
	return nil, "", fmt.Errorf("无法在 %s 下生成不冲突的文件名：%s", destDir, baseName)
}

func decodeZipEntryName(file *zip.File) string {
	name := file.Name
	if decoded, err := parser.DecodeVPKEntryName(name); err == nil {
		return decoded
	}
	return name
}

func copyStreamWithProgress(dst io.Writer, src io.Reader, onDelta func(int64)) error {
	buffer := make([]byte, 1024*1024)
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			if _, writeErr := dst.Write(buffer[:n]); writeErr != nil {
				return writeErr
			}
			if onDelta != nil {
				onDelta(int64(n))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

func isVPKName(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".vpk")
}

func isSupportedSidecarName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".meta":
		return true
	default:
		return false
	}
}

func entryBaseWithoutExt(name string) string {
	base := strings.ToLower(filepath.Base(name))
	return strings.TrimSuffix(base, strings.ToLower(filepath.Ext(base)))
}

func totalZipEntryBytes(entries []dropZipEntry) int64 {
	var total int64
	for _, entry := range entries {
		if entry.size > 0 {
			total += entry.size
		}
	}
	return total
}

func totalRarEntryBytes(entries map[string]dropRarEntry) int64 {
	var total int64
	for _, entry := range entries {
		if entry.size > 0 {
			total += entry.size
		}
	}
	return total
}

func total7zEntryBytes(entries []drop7zEntry) int64 {
	var total int64
	for _, entry := range entries {
		if entry.size > 0 {
			total += entry.size
		}
	}
	return total
}

func archivePercent(completedBytes int64, totalBytes int64, completedEntries int, totalEntries int) int {
	if totalBytes > 0 {
		return scaledProgressPercent(0, 100, completedBytes, totalBytes)
	}
	if totalEntries <= 0 {
		return 100
	}
	return normalizeProgressPercent(completedEntries * 100 / totalEntries)
}

func parallelArchiveProgressMessage(name string) string {
	return fmt.Sprintf("正在并行解压: %s", filepath.Base(name))
}

func activeArchiveNames(active map[string]bool) []string {
	names := make([]string, 0, len(active))
	for name := range active {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func scaledProgressPercent(start int, end int, current int64, total int64) int {
	if total <= 0 {
		return normalizeProgressPercent(end)
	}
	span := end - start
	value := start + int(float64(span)*float64(current)/float64(total))
	return normalizeProgressPercent(value)
}

func normalizeProgressPercent(percent int) int {
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}
