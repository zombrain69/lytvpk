package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vpk-manager/internal/parser"
	"vpk-manager/internal/platform/protocol"
)

// VPKRepairResult describes a repaired copy. The source archive is never
// overwritten; users can inspect or replace it manually after verification.
type VPKRepairResult struct {
	SourcePath        string                           `json:"sourcePath"`
	OutputPath        string                           `json:"outputPath"`
	OriginalPreserved bool                             `json:"originalPreserved"`
	AddonInfoRepair   parser.VPKAddonInfoRepairSummary `json:"addonInfoRepair"`
	Report            parser.VPKIntegrityReport        `json:"report"`
}

// VPKIntegrityBatchResult keeps one report per requested path. Batch methods
// return item-level errors so one broken or missing file does not hide results
// for the other selected Mods.
type VPKIntegrityBatchResult struct {
	Path   string                    `json:"path"`
	Report parser.VPKIntegrityReport `json:"report"`
	Error  string                    `json:"error,omitempty"`
}

type VPKRepairBatchResult struct {
	SourcePath        string                           `json:"sourcePath"`
	OutputPath        string                           `json:"outputPath"`
	OriginalPreserved bool                             `json:"originalPreserved"`
	AddonInfoRepair   parser.VPKAddonInfoRepairSummary `json:"addonInfoRepair"`
	Report            parser.VPKIntegrityReport        `json:"report"`
	Error             string                           `json:"error,omitempty"`
}

// vpkIntegrityCacheEntry 是一次校验结果的记忆化条目。
//
// 键用 (路径, 大小, 修改时间)：文件没被动过就直接复用上一次的结论。
// 这条缓存是"启用游戏内 Mod 之前先做风险提示"能便宜下来的关键 ——
// 同一个包第二次、第三次启用时是 0 成本，而不是把整包再读一遍。
type vpkIntegrityCacheEntry struct {
	size    int64
	modTime time.Time
	report  parser.VPKIntegrityReport
}

type vpkIntegrityCache struct {
	mu      sync.Mutex
	entries map[string]vpkIntegrityCacheEntry
}

func integrityCacheKey(filePath string) string {
	return strings.ToLower(filepath.Clean(filePath))
}

func (c *vpkIntegrityCache) lookup(filePath string, info os.FileInfo) (parser.VPKIntegrityReport, bool) {
	if info == nil {
		return parser.VPKIntegrityReport{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[integrityCacheKey(filePath)]
	if !ok {
		return parser.VPKIntegrityReport{}, false
	}
	if entry.size != info.Size() || !entry.modTime.Equal(info.ModTime()) {
		return parser.VPKIntegrityReport{}, false
	}
	return cloneIntegrityReport(entry.report), true
}

func (c *vpkIntegrityCache) store(filePath string, info os.FileInfo, report parser.VPKIntegrityReport) {
	if info == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]vpkIntegrityCacheEntry)
	}
	c.entries[integrityCacheKey(filePath)] = vpkIntegrityCacheEntry{
		size:    info.Size(),
		modTime: info.ModTime(),
		report:  cloneIntegrityReport(report),
	}
}

// cloneIntegrityReport 复制一份结果：Issues 是切片，直接返回会让调用方改到缓存里的那份。
func cloneIntegrityReport(report parser.VPKIntegrityReport) parser.VPKIntegrityReport {
	cloned := report
	if len(report.Issues) > 0 {
		cloned.Issues = append([]parser.VPKIntegrityIssue(nil), report.Issues...)
	}
	return cloned
}

// InspectVPKIntegrity scans one VPK for structural, encoding and game-facing
// addoninfo.txt problems，并按 (路径, 大小, 修改时间) 复用上一次的结果。
//
// 索引级校验本身已经是 O(条目数)；缓存再兜住"同一个包被反复启用/禁用"的场景。
func (a *App) InspectVPKIntegrity(filePath string) (parser.VPKIntegrityReport, error) {
	info, statErr := os.Stat(filePath)
	if statErr != nil {
		// 交给 parser 产出统一的中文错误文案（路径为空 / 不是 .vpk / 文件不存在）。
		return parser.InspectVPKIntegrity(filePath)
	}
	if cached, ok := a.integrityCache.lookup(filePath, info); ok {
		return cached, nil
	}
	report, err := parser.InspectVPKIntegrity(filePath)
	if err != nil {
		return report, err
	}
	a.integrityCache.store(filePath, info, report)
	return report, nil
}

// integrityBatchWorkers 是批量校验的并发度：校验是"读索引 + 少量随机读"，
// 并发几路就能把多个大包的等待重叠起来，又不会把磁盘打满。
const integrityBatchWorkers = 4

// InspectVPKIntegrityBatch checks each selected VPK independently and keeps
// processing after an individual path fails.
//
// 并发执行（老实现是串行 for 循环：选 3 个 1GB 的包就是十几分钟起步），
// 每个路径的结论仍然按 (路径, 大小, 修改时间) 记忆化，结果顺序与请求一致。
func (a *App) InspectVPKIntegrityBatch(filePaths []string) []VPKIntegrityBatchResult {
	paths := uniqueVPKIntegrityPaths(filePaths)
	results := make([]VPKIntegrityBatchResult, len(paths))
	if len(paths) == 0 {
		return results
	}

	workers := integrityBatchWorkers
	if workers > len(paths) {
		workers = len(paths)
	}
	slots := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for index, filePath := range paths {
		wg.Add(1)
		slots <- struct{}{}
		go func(index int, filePath string) {
			defer wg.Done()
			defer func() { <-slots }()
			report, err := a.InspectVPKIntegrity(filePath)
			item := VPKIntegrityBatchResult{Path: filePath, Report: report}
			if err != nil {
				item.Error = err.Error()
			}
			results[index] = item
		}(index, filePath)
	}
	wg.Wait()
	return results
}

// RepairVPKIntegrity repairs only safe addoninfo.txt issues into a new VPK.
// Archives with broken entry data, duplicate paths or unsafe paths are
// reported as non-repairable and are left untouched.
func (a *App) RepairVPKIntegrity(filePath string) (VPKRepairResult, error) {
	result := VPKRepairResult{SourcePath: filepath.Clean(strings.TrimSpace(filePath))}
	workshopMeta, _ := LoadWorkshopMeta(result.SourcePath)
	report, err := parser.InspectVPKIntegrity(filePath)
	if err != nil {
		return result, err
	}
	result.Report = report
	if report.Valid {
		return result, fmt.Errorf("VPK 未发现可修复错误")
	}
	if !report.Repairable {
		return result, fmt.Errorf("检测到不可安全自动修复的问题，请重新下载或手动处理")
	}

	tempRoot, err := os.MkdirTemp("", "lytvpk-vpk-repair-")
	if err != nil {
		return result, fmt.Errorf("无法创建临时修复目录: %w", err)
	}
	defer os.RemoveAll(tempRoot)

	unpacked, err := a.UnpackVPKFile(result.SourcePath, tempRoot)
	if err != nil {
		return result, fmt.Errorf("无法解包待修复 VPK: %w", err)
	}
	addonInfoPath, err := findRootAddonInfoPath(unpacked.OutputDir)
	if err != nil {
		return result, err
	}
	var originalContent string
	if addonInfoPath != "" {
		data, readErr := os.ReadFile(addonInfoPath)
		if readErr != nil {
			return result, fmt.Errorf("无法读取待修复 addoninfo.txt: %w", readErr)
		}
		if decoded, decodeErr := parser.DecodeVPKText(data); decodeErr == nil {
			originalContent = decoded
		}
	} else {
		addonInfoPath = filepath.Join(unpacked.OutputDir, "addoninfo.txt")
	}

	sourceBaseName := strings.TrimSuffix(filepath.Base(result.SourcePath), filepath.Ext(result.SourcePath))
	addonInfoFallbackTitle := sourceBaseName
	metadataFallbacks := make(map[string]string)
	if workshopMeta != nil {
		if strings.TrimSpace(workshopMeta.Title) != "" {
			addonInfoFallbackTitle = workshopMeta.Title
			metadataFallbacks["addontitle"] = workshopMeta.Title
		}
		metadataFallbacks["addonauthor"] = workshopMeta.Author
		metadataFallbacks["addonDescription"] = workshopMeta.Description
		if workshopMeta.WorkshopID != "" && !strings.HasPrefix(workshopMeta.WorkshopID, "direct-") && protocol.IsValidWorkshopID(workshopMeta.WorkshopID) {
			metadataFallbacks["addonURL0"] = "https://steamcommunity.com/sharedfiles/filedetails/?id=" + workshopMeta.WorkshopID
		}
	}
	repairedAddonInfo, addonInfoRepair := parser.BuildRepairedAddonInfoWithMetadata(originalContent, addonInfoFallbackTitle, metadataFallbacks)
	if err := os.WriteFile(addonInfoPath, []byte(repairedAddonInfo), 0644); err != nil {
		return result, fmt.Errorf("无法写入修复后的 addoninfo.txt: %w", err)
	}
	result.AddonInfoRepair = addonInfoRepair

	baseName := sourceBaseName + ".repaired"
	packed, err := a.packVPKDirectoryWithOptions(unpacked.OutputDir, filepath.Dir(result.SourcePath), false, baseName, nil)
	if err != nil {
		return result, fmt.Errorf("无法生成修复后的 VPK: %w", err)
	}
	result.OutputPath = packed.OutputPath
	result.OriginalPreserved = true
	copiedSidecars, err := copyRepairSidecars(result.SourcePath, result.OutputPath, workshopMeta)
	if err != nil {
		_ = os.Remove(result.OutputPath)
		return result, fmt.Errorf("无法保留修复文件的工坊伴随信息: %w", err)
	}
	verified, verifyErr := parser.InspectVPKIntegrity(result.OutputPath)
	if verifyErr != nil {
		_ = os.Remove(result.OutputPath)
		removeFiles(copiedSidecars)
		return result, fmt.Errorf("修复结果无法检查: %w", verifyErr)
	}
	result.Report = verified
	if !verified.Valid {
		_ = os.Remove(result.OutputPath)
		removeFiles(copiedSidecars)
		return result, fmt.Errorf("修复结果仍存在错误，已删除未通过检查的输出文件")
	}
	return result, nil
}

// copyRepairSidecars preserves external metadata under the repaired basename.
// Invalid or missing .meta files are ignored by the caller's nil metadata
// value, while image sidecars are copied independently when present.
func copyRepairSidecars(sourcePath, outputPath string, meta *WorkshopMeta) ([]string, error) {
	sourceBase := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath))
	outputBase := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	copied := make([]string, 0, 5)
	cleanup := func(err error) ([]string, error) {
		removeFiles(copied)
		return nil, err
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif"} {
		sourceSidecar := sourceBase + ext
		if _, err := os.Stat(sourceSidecar); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return cleanup(err)
		}
		outputSidecar := outputBase + ext
		if err := copyRegularFile(sourceSidecar, outputSidecar); err != nil {
			return cleanup(err)
		}
		copied = append(copied, outputSidecar)
	}
	if meta != nil {
		outputMetaPath := GetMetaFilePath(outputPath)
		if err := copyRegularFile(GetMetaFilePath(sourcePath), outputMetaPath); err != nil {
			return cleanup(err)
		}
		copied = append(copied, outputMetaPath)
	}
	return copied, nil
}

func removeFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

// RepairVPKIntegrityBatch repairs only the selected archives that pass the
// same safety checks as RepairVPKIntegrity. Results are independent so a
// non-repairable archive does not prevent other selected archives from being
// repaired.
func (a *App) RepairVPKIntegrityBatch(filePaths []string) []VPKRepairBatchResult {
	paths := uniqueVPKIntegrityPaths(filePaths)
	results := make([]VPKRepairBatchResult, 0, len(paths))
	for _, filePath := range paths {
		result, err := a.RepairVPKIntegrity(filePath)
		item := VPKRepairBatchResult{
			SourcePath:        result.SourcePath,
			OutputPath:        result.OutputPath,
			OriginalPreserved: result.OriginalPreserved,
			AddonInfoRepair:   result.AddonInfoRepair,
			Report:            result.Report,
		}
		if err != nil {
			item.Error = err.Error()
		}
		results = append(results, item)
	}
	return results
}

func uniqueVPKIntegrityPaths(filePaths []string) []string {
	seen := make(map[string]struct{}, len(filePaths))
	paths := make([]string, 0, len(filePaths))
	for _, filePath := range filePaths {
		clean := filepath.Clean(strings.TrimSpace(filePath))
		if clean == "" || clean == "." {
			continue
		}
		key := strings.ToLower(clean)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		paths = append(paths, clean)
	}
	return paths
}

func findRootAddonInfoPath(root string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if strings.EqualFold(filepath.ToSlash(rel), "addoninfo.txt") {
			found = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("无法定位 addoninfo.txt: %w", err)
	}
	return found, nil
}
