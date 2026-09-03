package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	workshopTransferStatusSuccess = "success"
	workshopTransferStatusFailed  = "failed"
	workshopTransferStatusSkipped = "skipped"
)

// WorkshopTransferResult describes a batch workshop-to-root transfer.
// The Fork keeps the original workshop file and synchronizes addonlist.txt;
// this batch API intentionally delegates each item to that existing behavior.
type WorkshopTransferResult struct {
	Total         int                          `json:"total"`
	EligibleCount int                          `json:"eligibleCount"`
	SuccessCount  int                          `json:"successCount"`
	FailCount     int                          `json:"failCount"`
	SkippedCount  int                          `json:"skippedCount"`
	WarningCount  int                          `json:"warningCount"`
	Items         []WorkshopTransferItemResult `json:"items"`
}

type WorkshopTransferItemResult struct {
	SourcePath string   `json:"sourcePath"`
	TargetPath string   `json:"targetPath"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	MetaSaved  bool     `json:"metaSaved"`
	Warnings   []string `json:"warnings"`
	Error      string   `json:"error"`
}

type WorkshopTransferProgress struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

// MoveWorkshopFilesToAddons batches the current Fork's workshop transfer.
// It deduplicates paths, skips non-workshop files, preserves workshop sources,
// copies sidecars, updates addonlist.txt, and reports per-item progress.
func (a *App) MoveWorkshopFilesToAddons(filePaths []string) (WorkshopTransferResult, error) {
	paths := uniqueWorkshopTransferPaths(filePaths)
	result := WorkshopTransferResult{
		Total: len(paths),
		Items: make([]WorkshopTransferItemResult, 0, len(paths)),
	}

	a.mu.RLock()
	rootDir := a.rootDir
	a.mu.RUnlock()
	if strings.TrimSpace(rootDir) == "" {
		return result, fmt.Errorf("请先设置根目录")
	}

	eligiblePaths := make([]string, 0, len(paths))
	for _, filePath := range paths {
		if a.isWorkshopTransferCandidate(filePath) {
			eligiblePaths = append(eligiblePaths, filePath)
		}
	}
	result.EligibleCount = len(eligiblePaths)

	eligibleSet := make(map[string]struct{}, len(eligiblePaths))
	for _, filePath := range eligiblePaths {
		eligibleSet[strings.ToLower(filepath.Clean(filePath))] = struct{}{}
	}
	completed := 0
	for _, filePath := range paths {
		item := WorkshopTransferItemResult{
			SourcePath: filePath,
			Name:       filepath.Base(filePath),
		}
		if _, eligible := eligibleSet[strings.ToLower(filepath.Clean(filePath))]; !eligible {
			item.Status = workshopTransferStatusSkipped
			result.SkippedCount++
			result.Items = append(result.Items, item)
			continue
		}

		a.emitWorkshopTransferProgress(completed, result.EligibleCount, item.Name, "moving", "正在转移创意工坊 Mod")
		moveResult, err := a.moveWorkshopToAddonsWithConflictAction(filePath, "")
		if err != nil {
			item.Status = workshopTransferStatusFailed
			item.Error = err.Error()
			result.FailCount++
			result.Items = append(result.Items, item)
			completed++
			a.emitWorkshopTransferProgress(completed, result.EligibleCount, item.Name, "completed", "转移失败")
			continue
		}
		if moveResult.Cancelled || moveResult.SkippedCount > 0 {
			item.Status = workshopTransferStatusSkipped
			result.SkippedCount++
			result.Items = append(result.Items, item)
			completed++
			a.emitWorkshopTransferProgress(completed, result.EligibleCount, item.Name, "completed", "已跳过")
			continue
		}

		item.Status = workshopTransferStatusSuccess
		item.TargetPath = filepath.Join(rootDir, item.Name)
		result.SuccessCount++
		result.Items = append(result.Items, item)
		completed++
		a.emitWorkshopTransferProgress(completed, result.EligibleCount, item.Name, "completed", "转移完成")
	}

	return result, nil
}

func uniqueWorkshopTransferPaths(filePaths []string) []string {
	paths := make([]string, 0, len(filePaths))
	seen := make(map[string]struct{}, len(filePaths))
	for _, rawPath := range filePaths {
		filePath := strings.TrimSpace(rawPath)
		if filePath == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(filePath))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		paths = append(paths, filePath)
	}
	return paths
}

func (a *App) isWorkshopTransferCandidate(filePath string) bool {
	if cached, ok := a.vpkCache.Load(filePath); ok {
		cache, valid := cached.(*VPKFileCache)
		return valid && cache.File.Location == "workshop"
	}
	return a.getLocationFromPath(filePath) == "workshop"
}

func (a *App) emitWorkshopTransferProgress(current, total int, name, phase, message string) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "workshop_transfer_progress", WorkshopTransferProgress{
		Current: current,
		Total:   total,
		Name:    name,
		Phase:   phase,
		Message: message,
	})
}
