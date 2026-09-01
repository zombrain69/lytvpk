package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	rt "runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode"
	"l4d2-manager-next/pkg/valve/vpk"
	"vpk-manager/internal/parser"
)

const (
	archiveMaxEntries                = 100000
	archiveMaxVPKDirectoryBytes      = 8 << 20
	archiveMaxVPKFiles               = 100000
	archiveMaxScanDepth              = 64
	archiveVPKInspectionValid        = "valid"
	archiveVPKInspectionLimited      = "limited"
	archiveVPKInspectionInvalid      = "invalid"
	archiveErrorKindPasswordRequired = "password_required"
	archiveErrorKindUnreadable       = "unreadable"
)

type archiveErrorState struct {
	Kind             string
	Message          string
	RequiresPassword bool
}

type archiveExistingVPKState struct {
	RootEnabled  bool
	RootDisabled bool
	Workshop     bool
	GameKnown    bool
	GameEnabled  bool
}

type archiveExistingVPKIndex map[string]archiveExistingVPKState

type archiveFileSignature struct {
	Size    int64
	ModTime time.Time
}

type archiveScanCacheEntry struct {
	Signature              archiveFileSignature
	PasswordHash           string
	ExistingIndexSignature archiveExistingIndexSignature
	Info                   ArchivePackageInfo
}

type archiveScanCache struct {
	mu      sync.RWMutex
	entries map[string]archiveScanCacheEntry
}

type archiveExistingIndexSignature struct {
	Root        string
	AddonList   archiveFileSignature
	RootDir     archiveFileSignature
	DisabledDir archiveFileSignature
	WorkshopDir archiveFileSignature
}

func newArchiveScanCache() *archiveScanCache {
	return &archiveScanCache{entries: make(map[string]archiveScanCacheEntry)}
}

func (c *archiveScanCache) get(path string, signature archiveFileSignature, password string) (ArchivePackageInfo, bool) {
	return c.getWithIndex(path, signature, password, archiveExistingIndexSignature{})
}

func (c *archiveScanCache) getWithIndex(path string, signature archiveFileSignature, password string, indexSignature archiveExistingIndexSignature) (ArchivePackageInfo, bool) {
	if c == nil {
		return ArchivePackageInfo{}, false
	}
	c.mu.RLock()
	entry, ok := c.entries[path]
	c.mu.RUnlock()
	if !ok || entry.Signature != signature || entry.PasswordHash != archivePasswordHash(password) || entry.ExistingIndexSignature != indexSignature {
		return ArchivePackageInfo{}, false
	}
	return cloneArchivePackageInfo(entry.Info), true
}

func (c *archiveScanCache) put(path string, signature archiveFileSignature, password string, info ArchivePackageInfo) {
	c.putWithIndex(path, signature, password, archiveExistingIndexSignature{}, info)
}

func (c *archiveScanCache) putWithIndex(path string, signature archiveFileSignature, password string, indexSignature archiveExistingIndexSignature, info ArchivePackageInfo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]archiveScanCacheEntry)
	}
	c.entries[path] = archiveScanCacheEntry{
		Signature:              signature,
		PasswordHash:           archivePasswordHash(password),
		ExistingIndexSignature: indexSignature,
		Info:                   cloneArchivePackageInfo(info),
	}
	c.mu.Unlock()
}

func (c *archiveScanCache) pruneUnder(directory string, currentPaths map[string]struct{}) {
	if c == nil {
		return
	}
	directory = filepath.Clean(directory)
	c.mu.Lock()
	for path := range c.entries {
		rel, err := filepath.Rel(directory, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if _, ok := currentPaths[path]; !ok {
			delete(c.entries, path)
		}
	}
	c.mu.Unlock()
}

func archivePasswordHash(password string) string {
	if password == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", sum[:])
}

func archiveFileSignatureForPath(path string) archiveFileSignature {
	info, err := os.Stat(path)
	if err != nil {
		return archiveFileSignature{}
	}
	return archiveFileSignature{Size: info.Size(), ModTime: info.ModTime()}
}

func archiveExistingIndexSignatureForRoot(root string) archiveExistingIndexSignature {
	return archiveExistingIndexSignature{
		Root:        filepath.Clean(root),
		AddonList:   archiveFileSignatureForPath(filepath.Join(root, "addonlist.txt")),
		RootDir:     archiveFileSignatureForPath(root),
		DisabledDir: archiveFileSignatureForPath(filepath.Join(root, "disabled")),
		WorkshopDir: archiveFileSignatureForPath(filepath.Join(root, "workshop")),
	}
}

func cloneArchivePackageInfo(info ArchivePackageInfo) ArchivePackageInfo {
	clone := info
	clone.Entries = append([]ArchiveTreeEntry(nil), info.Entries...)
	clone.VPKs = make([]ArchiveVPKInfo, len(info.VPKs))
	for i, vpkInfo := range info.VPKs {
		clone.VPKs[i] = vpkInfo
		clone.VPKs[i].InternalFiles = append([]string(nil), vpkInfo.InternalFiles...)
		clone.VPKs[i].ExistingLocations = append([]string(nil), vpkInfo.ExistingLocations...)
	}
	return clone
}

func cloneArchiveExistingVPKIndex(index archiveExistingVPKIndex) archiveExistingVPKIndex {
	clone := make(archiveExistingVPKIndex, len(index))
	for name, state := range index {
		clone[name] = state
	}
	return clone
}

// ArchiveTreeEntry describes one item in an archive without extracting it.
type ArchiveTreeEntry struct {
	Name     string `json:"name"`
	IsDir    bool   `json:"isDir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// ArchiveVPKInfo contains the VPK-specific evidence discovered inside an archive.
type ArchiveVPKInfo struct {
	EntryPath         string   `json:"entryPath"`
	Name              string   `json:"name"`
	Size              int64    `json:"size"`
	MatchState        string   `json:"matchState"`
	Valid             bool     `json:"valid"`
	FileCount         int      `json:"fileCount"`
	InternalFiles     []string `json:"internalFiles"`
	InspectionStatus  string   `json:"inspectionStatus"`
	ExistingLocations []string `json:"existingLocations"`
	ExistingGameState string   `json:"existingGameState"`
	Error             string   `json:"error"`
}

// ArchivePackageInfo describes one supported archive found below a selected folder.
type ArchivePackageInfo struct {
	Path             string             `json:"path"`
	Name             string             `json:"name"`
	Format           string             `json:"format"`
	Size             int64              `json:"size"`
	Modified         string             `json:"modified"`
	Entries          []ArchiveTreeEntry `json:"entries"`
	VPKs             []ArchiveVPKInfo   `json:"vpks"`
	ErrorKind        string             `json:"errorKind"`
	RequiresPassword bool               `json:"requiresPassword"`
	ErrorDetail      string             `json:"errorDetail"`
	Error            string             `json:"error"`
}

func archiveFormatForPath(path string) string {
	lower := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	case strings.HasSuffix(lower, ".rar"):
		return "rar"
	case strings.HasSuffix(lower, ".7z"):
		return "7z"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	default:
		return ""
	}
}

func archiveVPKMatchState(entryName string, existing map[string]struct{}) string {
	if !strings.EqualFold(filepath.Ext(entryName), ".vpk") {
		return ""
	}
	name := strings.ToLower(filepath.Base(filepath.Clean(strings.ReplaceAll(entryName, "\\", "/"))))
	if _, ok := existing[name]; ok {
		return "existing"
	}
	return "new"
}

func archiveVPKMatchStateFromIndex(entryName string, existing archiveExistingVPKIndex) string {
	if !strings.EqualFold(filepath.Ext(entryName), ".vpk") {
		return ""
	}
	name := strings.ToLower(filepath.Base(filepath.Clean(strings.ReplaceAll(entryName, "\\", "/"))))
	_, ok := existing[name]
	if !ok {
		return "new"
	}
	return "existing"
}

func archiveVPKExistingDetails(entryName string, existing archiveExistingVPKIndex) ([]string, string) {
	name := strings.ToLower(filepath.Base(filepath.Clean(strings.ReplaceAll(entryName, "\\", "/"))))
	state, ok := existing[name]
	if !ok {
		return nil, ""
	}
	locations := make([]string, 0, 3)
	if state.RootEnabled {
		locations = append(locations, "addons")
	}
	if state.RootDisabled {
		locations = append(locations, "disabled")
	}
	if state.Workshop {
		locations = append(locations, "workshop")
	}
	sort.Strings(locations)
	gameState := ""
	if state.GameKnown {
		if state.GameEnabled {
			gameState = "enabled"
		} else {
			gameState = "disabled"
		}
	}
	return locations, gameState
}

func (a *App) getArchiveScanCache() *archiveScanCache {
	a.archiveScanCacheMu.Lock()
	defer a.archiveScanCacheMu.Unlock()
	if a.archiveScanCache == nil {
		a.archiveScanCache = newArchiveScanCache()
	}
	return a.archiveScanCache
}

func (a *App) existingVPKIndexCached() (archiveExistingVPKIndex, archiveExistingIndexSignature) {
	root := a.rootDirectorySnapshot()
	signature := archiveExistingIndexSignatureForRoot(root)
	a.archiveExistingIndexMu.RLock()
	if a.archiveExistingIndexSet && a.archiveExistingIndexSig == signature {
		index := cloneArchiveExistingVPKIndex(a.archiveExistingIndex)
		a.archiveExistingIndexMu.RUnlock()
		return index, signature
	}
	a.archiveExistingIndexMu.RUnlock()

	index := a.existingVPKIndex()
	a.archiveExistingIndexMu.Lock()
	a.archiveExistingIndex = cloneArchiveExistingVPKIndex(index)
	a.archiveExistingIndexSig = signature
	a.archiveExistingIndexSet = true
	a.archiveExistingIndexMu.Unlock()
	return index, signature
}

// SelectArchiveDirectory opens a folder picker for the archive manager.
func (a *App) SelectArchiveDirectory() (string, error) { return a.SelectDirectory() }

// ScanArchiveDirectory recursively lists supported archives and their VPK entries.
// Archive contents are never extracted to disk.
func (a *App) ScanArchiveDirectory(directory string) ([]ArchivePackageInfo, error) {
	return a.ScanArchiveDirectoryWithPasswords(directory, nil)
}

// ScanArchiveDirectoryWithPasswords rescans supported archives with temporary
// per-file 7Z passwords. Passwords are used only by this call and are never
// persisted in settings or written to disk.
func (a *App) ScanArchiveDirectoryWithPasswords(directory string, passwords map[string]string) ([]ArchivePackageInfo, error) {
	directory = filepath.Clean(strings.TrimSpace(directory))
	if directory == "" || directory == "." {
		return nil, fmt.Errorf("压缩包目录不能为空")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("无法访问压缩包目录: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("压缩包路径不是目录: %s", directory)
	}

	existing, existingSignature := a.existingVPKIndexCached()
	type scanCandidate struct {
		path      string
		format    string
		signature archiveFileSignature
		password  string
	}
	candidates := make([]scanCandidate, 0)
	err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if depth := archivePathDepth(directory, path); depth > archiveMaxScanDepth && path != directory {
				return filepath.SkipDir
			}
			return nil
		}
		format := archiveFormatForPath(path)
		if format == "" {
			return nil
		}
		fileInfo, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		password := ""
		if format == "7z" && passwords != nil {
			password = passwords[path]
		}
		candidates = append(candidates, scanCandidate{
			path:      path,
			format:    format,
			signature: archiveFileSignature{Size: fileInfo.Size(), ModTime: fileInfo.ModTime()},
			password:  password,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描压缩包目录失败: %w", err)
	}

	cache := a.getArchiveScanCache()
	packages := make([]ArchivePackageInfo, 0, len(candidates))
	var packagesMu sync.Mutex
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		candidate := candidate
		if cached, ok := cache.getWithIndex(candidate.path, candidate.signature, candidate.password, existingSignature); ok {
			packages = append(packages, cached)
			continue
		}
		wg.Add(1)
		a.submitPoolTask(func() {
			defer wg.Done()
			pkg := scanArchivePackageWithPassword(candidate.path, candidate.format, existing, candidate.password)
			cache.putWithIndex(candidate.path, candidate.signature, candidate.password, existingSignature, pkg)
			packagesMu.Lock()
			packages = append(packages, pkg)
			packagesMu.Unlock()
		})
	}
	wg.Wait()
	currentPaths := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		currentPaths[candidate.path] = struct{}{}
	}
	cache.pruneUnder(directory, currentPaths)
	sort.Slice(packages, func(i, j int) bool { return strings.ToLower(packages[i].Path) < strings.ToLower(packages[j].Path) })
	return packages, nil
}

func archivePathDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}

func (a *App) existingVPKIndex() archiveExistingVPKIndex {
	set := make(archiveExistingVPKIndex)
	root := a.rootDirectorySnapshot()
	addonStates := make(map[string]bool)
	if list, _, err := a.readAddonList(); err == nil {
		addonStates = addonListStateMap(list)
	}
	add := func(path string) {
		if !strings.EqualFold(filepath.Ext(path), ".vpk") {
			return
		}
		name := strings.ToLower(filepath.Base(path))
		state := set[name]
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return
		}
		parts := strings.Split(rel, string(filepath.Separator))
		location := "root"
		if len(parts) > 0 {
			switch {
			case strings.EqualFold(parts[0], "workshop"):
				location = "workshop"
			case strings.EqualFold(parts[0], "disabled"):
				location = "disabled"
			}
		}
		switch location {
		case "workshop":
			state.Workshop = true
		case "disabled":
			state.RootDisabled = true
		default:
			state.RootEnabled = true
		}
		key, keyErr := addonListKeyForVPKPathFromRoot(root, path)
		if location == "disabled" {
			key, keyErr = addonListKeyForManagedVPKPathFromRoot(root, path)
		}
		if keyErr == nil {
			if enabled, found := addonStates[key]; found {
				state.GameKnown = true
				state.GameEnabled = enabled
			}
		}
		set[name] = state
	}

	a.vpkCache.Range(func(key, value interface{}) bool {
		cache, ok := value.(*VPKFileCache)
		if ok {
			path := cache.File.Path
			if path == "" {
				path, _ = key.(string)
			}
			add(path)
		}
		return true
	})
	if root == "" {
		return set
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".vpk") {
			add(path)
		}
		return nil
	})
	return set
}

func (a *App) existingVPKBasenames() map[string]struct{} {
	set := make(map[string]struct{})
	for name := range a.existingVPKIndex() {
		set[name] = struct{}{}
	}
	return set
}

func scanArchivePackage(path, format string, existing archiveExistingVPKIndex) ArchivePackageInfo {
	return scanArchivePackageWithPassword(path, format, existing, "")
}

func scanArchivePackageWithPassword(path, format string, existing archiveExistingVPKIndex, password string) ArchivePackageInfo {
	info := ArchivePackageInfo{Path: path, Name: filepath.Base(path), Format: format, Entries: make([]ArchiveTreeEntry, 0), VPKs: make([]ArchiveVPKInfo, 0)}
	if stat, err := os.Stat(path); err == nil {
		info.Size = stat.Size()
		info.Modified = stat.ModTime().Format(time.RFC3339)
	} else {
		info.Error = err.Error()
		return info
	}
	var err error
	switch format {
	case "zip":
		err = scanZipArchive(path, &info, existing)
	case "rar":
		err = scanRarArchive(path, &info, existing)
	case "7z":
		err = scan7zArchive(path, password, &info, existing)
	case "tar", "tar.gz":
		err = scanTarArchive(path, format == "tar.gz", &info, existing)
	}
	if err != nil {
		info.ErrorDetail = err.Error()
		if format == "7z" {
			state := classifySevenZipError(err)
			info.ErrorKind = state.Kind
			info.RequiresPassword = state.RequiresPassword
			info.Error = state.Message
		} else {
			info.ErrorKind = archiveErrorKindUnreadable
			info.Error = err.Error()
		}
	}
	return info
}

func classifySevenZipError(err error) archiveErrorState {
	var readErr *sevenzip.ReadError
	if errors.As(err, &readErr) && readErr.Encrypted {
		return archiveErrorState{
			Kind:             archiveErrorKindPasswordRequired,
			RequiresPassword: true,
			Message:          "7Z 文件头已加密；请输入密码后重试，才能读取文件树和 VPK 信息。",
		}
	}
	return archiveErrorState{
		Kind:    archiveErrorKindUnreadable,
		Message: "无法读取 7Z：文件可能已损坏、分卷不完整，或使用了当前解码器不支持的压缩方法。",
	}
}

// ScanArchivePackageWithPassword refreshes one archive without walking the
// selected directory again. It is intended for password retries and inline
// package updates in the archive manager session.
func (a *App) ScanArchivePackageWithPassword(path, password string) (ArchivePackageInfo, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	format := archiveFormatForPath(path)
	if path == "" || path == "." || format == "" {
		return ArchivePackageInfo{}, fmt.Errorf("不支持的压缩格式: %s", filepath.Ext(path))
	}
	info, err := os.Stat(path)
	if err != nil {
		return ArchivePackageInfo{}, fmt.Errorf("无法访问压缩包: %w", err)
	}
	if info.IsDir() {
		return ArchivePackageInfo{}, fmt.Errorf("压缩包路径不是文件: %s", path)
	}
	password = strings.TrimSpace(password)
	signature := archiveFileSignature{Size: info.Size(), ModTime: info.ModTime()}
	existing, existingSignature := a.existingVPKIndexCached()
	cache := a.getArchiveScanCache()
	if cached, ok := cache.getWithIndex(path, signature, password, existingSignature); ok {
		return cached, nil
	}
	result := scanArchivePackageWithPassword(path, format, existing, password)
	cache.putWithIndex(path, signature, password, existingSignature, result)
	return result, nil
}

func appendArchiveEntry(info *ArchivePackageInfo, name string, isDir bool, size int64, modified time.Time, existing archiveExistingVPKIndex, open func() (io.ReadCloser, error)) error {
	if len(info.Entries) >= archiveMaxEntries {
		return fmt.Errorf("压缩包条目超过 %d 个，已停止读取", archiveMaxEntries)
	}
	name = decodeArchiveEntryName(name)
	entry := ArchiveTreeEntry{Name: name, IsDir: isDir, Size: size}
	if !modified.IsZero() {
		entry.Modified = modified.Format(time.RFC3339)
	}
	info.Entries = append(info.Entries, entry)
	if isDir || archiveVPKMatchStateFromIndex(name, existing) == "" || open == nil {
		return nil
	}
	vpkInfo := inspectNestedVPK(name, size, existing, open)
	info.VPKs = append(info.VPKs, vpkInfo)
	return nil
}

func decodeArchiveEntryName(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	if decoded, err := parser.DecodeVPKEntryName(name); err == nil {
		return decoded
	}
	return name
}

func inspectNestedVPK(name string, size int64, existing archiveExistingVPKIndex, open func() (io.ReadCloser, error)) ArchiveVPKInfo {
	result := ArchiveVPKInfo{
		EntryPath:        name,
		Name:             filepath.Base(name),
		Size:             size,
		MatchState:       archiveVPKMatchStateFromIndex(name, existing),
		InspectionStatus: archiveVPKInspectionInvalid,
		InternalFiles:    make([]string, 0),
	}
	result.ExistingLocations, result.ExistingGameState = archiveVPKExistingDetails(name, existing)
	reader, err := open()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer reader.Close()

	// A VPK directory is stored at the beginning of the file. Limit this
	// metadata read rather than the total VPK size: large but normal texture or
	// audio mods no longer become false failures, while malformed directories
	// cannot consume unbounded memory through an archive entry.
	limited := &io.LimitedReader{R: reader, N: archiveMaxVPKDirectoryBytes}
	archive, err := vpk.ReadArchive(limited)
	if err != nil {
		if limited.N == 0 {
			result.InspectionStatus = archiveVPKInspectionLimited
			result.Error = fmt.Sprintf("VPK 目录信息超过 %d MiB 的安全读取上限；未判定为文件损坏", archiveMaxVPKDirectoryBytes>>20)
			return result
		}
		result.Error = err.Error()
		return result
	}
	result.Valid = true
	result.InspectionStatus = archiveVPKInspectionValid
	result.FileCount = len(archive.Files)
	if result.FileCount > archiveMaxVPKFiles {
		result.Error = fmt.Sprintf("VPK 条目超过 %d 个，未展开文件名", archiveMaxVPKFiles)
		result.InternalFiles = nil
		return result
	}
	for i := range archive.Files {
		result.InternalFiles = append(result.InternalFiles, decodeArchiveEntryName(archive.Files[i].Name()))
	}
	sort.Strings(result.InternalFiles)
	return result
}

func scanZipArchive(path string, info *ArchivePackageInfo, existing archiveExistingVPKIndex) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("无法打开 ZIP: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		f := file
		var open func() (io.ReadCloser, error)
		if !f.FileInfo().IsDir() && archiveVPKMatchStateFromIndex(f.Name, existing) != "" {
			open = f.Open
		}
		if err := appendArchiveEntry(info, f.Name, f.FileInfo().IsDir(), int64(f.UncompressedSize64), f.ModTime(), existing, open); err != nil {
			return err
		}
	}
	return nil
}

func scan7zArchive(path, password string, info *ArchivePackageInfo, existing archiveExistingVPKIndex) error {
	reader, err := sevenzip.OpenReaderWithPassword(path, password)
	if err != nil {
		return fmt.Errorf("无法打开 7Z: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		f := file
		isDir := f.FileInfo().IsDir()
		var open func() (io.ReadCloser, error)
		if !isDir && archiveVPKMatchStateFromIndex(f.Name, existing) != "" {
			open = f.Open
		}
		if err := appendArchiveEntry(info, f.Name, isDir, int64(f.UncompressedSize), f.Modified, existing, open); err != nil {
			return err
		}
	}
	return nil
}

func scanRarArchive(path string, info *ArchivePackageInfo, existing archiveExistingVPKIndex) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("无法打开 RAR: %w", err)
	}
	defer file.Close()
	reader, err := rardecode.NewReader(file, "")
	if err != nil {
		return fmt.Errorf("无法创建 RAR 读取器: %w", err)
	}
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("读取 RAR 内容失败: %w", nextErr)
		}
		name := header.Name
		var open func() (io.ReadCloser, error)
		if !header.IsDir && archiveVPKMatchStateFromIndex(name, existing) != "" {
			open = func() (io.ReadCloser, error) { return io.NopCloser(reader), nil }
		}
		if err := appendArchiveEntry(info, name, header.IsDir, header.UnPackedSize, header.ModificationTime, existing, open); err != nil {
			return err
		}
		if open != nil {
			if _, err := io.Copy(io.Discard, reader); err != nil {
				return fmt.Errorf("读取 RAR 中的 VPK 内容失败: %w", err)
			}
		}
	}
	return nil
}

func scanTarArchive(path string, compressed bool, info *ArchivePackageInfo, existing archiveExistingVPKIndex) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("无法打开 TAR: %w", err)
	}
	defer file.Close()
	var input io.Reader = file
	var gz *gzip.Reader
	if compressed {
		gz, err = gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("无法读取 GZIP: %w", err)
		}
		defer gz.Close()
		input = gz
	}
	reader := tar.NewReader(input)
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("读取 TAR 内容失败: %w", nextErr)
		}
		var open func() (io.ReadCloser, error)
		if header.Typeflag == tar.TypeReg && archiveVPKMatchStateFromIndex(header.Name, existing) != "" {
			open = func() (io.ReadCloser, error) { return io.NopCloser(reader), nil }
		}
		if err := appendArchiveEntry(info, header.Name, header.Typeflag != tar.TypeReg, header.Size, header.ModTime, existing, open); err != nil {
			return err
		}
		if open != nil {
			if _, err := io.Copy(io.Discard, reader); err != nil {
				return fmt.Errorf("读取 TAR 中的 VPK 内容失败: %w", err)
			}
		}
	}
	return nil
}

// OpenArchivePackage opens a supported archive with the operating system's
// default application. The path remains local and is never uploaded.
func (a *App) OpenArchivePackage(filePath string) error {
	filePath = filepath.Clean(strings.TrimSpace(filePath))
	if filePath == "" || filePath == "." {
		return fmt.Errorf("压缩包路径不能为空")
	}
	if archiveFormatForPath(filePath) == "" {
		return fmt.Errorf("不支持的压缩格式: %s", filepath.Ext(filePath))
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("无法访问压缩包: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("压缩包路径不是文件: %s", filePath)
	}

	var cmd *exec.Cmd
	switch rt.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", strings.ReplaceAll(filePath, "/", "\\"))
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "linux":
		cmd = exec.Command("xdg-open", filePath)
	default:
		return fmt.Errorf("不支持的操作系统: %s", rt.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开压缩包失败: %w", err)
	}
	return nil
}

// CheckArchiveMoveConflicts checks only archive files, unlike the Mod mover which
// also checks image/meta sidecars.
func (a *App) CheckArchiveMoveConflicts(filePaths []string, destDir string) ([]FileMoveConflict, error) {
	destDir = filepath.Clean(strings.TrimSpace(destDir))
	if destDir == "" || destDir == "." {
		return nil, fmt.Errorf("目标目录不能为空")
	}
	conflicts := make([]FileMoveConflict, 0)
	for _, source := range filePaths {
		source = filepath.Clean(strings.TrimSpace(source))
		if source == "" || archiveFormatForPath(source) == "" {
			continue
		}
		if _, err := os.Stat(source); err != nil {
			return nil, fmt.Errorf("压缩包不存在或无法访问: %s: %w", source, err)
		}
		target := filepath.Join(destDir, filepath.Base(source))
		if filepath.Clean(source) == filepath.Clean(target) {
			continue
		}
		if _, err := os.Stat(target); err == nil {
			conflict, err := fileMoveConflictFromCandidate(moveFileCandidate{source: source, target: target, fileType: "压缩包"})
			if err != nil {
				return nil, err
			}
			conflicts = append(conflicts, conflict)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("无法检查目标文件 %s: %w", target, err)
		}
	}
	return conflicts, nil
}

// MoveArchiveFiles moves selected archives with replace/skip/cancel conflict actions.
func (a *App) MoveArchiveFiles(filePaths []string, destDir, action string) (MoveResult, error) {
	result := MoveResult{}
	var err error
	if action, err = normalizeMoveConflictAction(action); err != nil {
		return result, err
	}
	destDir = filepath.Clean(strings.TrimSpace(destDir))
	if destDir == "" || destDir == "." {
		return result, fmt.Errorf("目标目录不能为空")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return result, fmt.Errorf("无法创建目标目录: %w", err)
	}
	for _, source := range filePaths {
		source = filepath.Clean(strings.TrimSpace(source))
		if source == "" || archiveFormatForPath(source) == "" {
			continue
		}
		target := filepath.Join(destDir, filepath.Base(source))
		err := moveFileWithConflictAction(source, target, action)
		if err == nil {
			result.SuccessCount++
			continue
		}
		if err == errMoveCancelled {
			result.Cancelled = true
			return result, nil
		}
		if err == errMoveSkipped {
			result.SkippedCount++
			continue
		}
		result.FailCount++
		result.Errors = append(result.Errors, fmt.Sprintf("移动 %s 失败: %v", filepath.Base(source), err))
	}
	return result, nil
}
