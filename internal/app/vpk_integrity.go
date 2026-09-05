package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// InspectVPKIntegrity scans one VPK for structural, checksum, encoding and
// game-facing addoninfo.txt problems.
func (a *App) InspectVPKIntegrity(filePath string) (parser.VPKIntegrityReport, error) {
	return parser.InspectVPKIntegrity(filePath)
}

// InspectVPKIntegrityBatch checks each selected VPK independently and keeps
// processing after an individual path fails.
func (a *App) InspectVPKIntegrityBatch(filePaths []string) []VPKIntegrityBatchResult {
	paths := uniqueVPKIntegrityPaths(filePaths)
	results := make([]VPKIntegrityBatchResult, 0, len(paths))
	for _, filePath := range paths {
		report, err := parser.InspectVPKIntegrity(filePath)
		item := VPKIntegrityBatchResult{Path: filePath, Report: report}
		if err != nil {
			item.Error = err.Error()
		}
		results = append(results, item)
	}
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
