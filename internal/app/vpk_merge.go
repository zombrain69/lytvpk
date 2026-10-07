package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vpk-manager/internal/parser"

	"l4d2-manager-next/pkg/valve/vpk"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// VPK 合并（导出整合包）。
//
// 用途：把几个 Mod 打包成一个 VPK，方便"只想装一个文件"或对抗模式场景（对齐 Mods4Versus 的思路）。
//
// 重要边界（写进返回值与界面文案）：
//   - 合并是**复制**，不会替代 addonlist：原 Mod 的启停/优先级/冲突分析都不受影响；
//   - 后一个 VPK 的同名条目覆盖前一个（顺序 = 列表顺序）；
//   - 合并结果不参与本应用的 Mod 管理（它只是一次导出），重复文件会变成"一个包内自带"。

type VPKMergeResult struct {
	OutputPath   string `json:"outputPath"`
	SourceCount  int    `json:"sourceCount"`
	TotalEntries int    `json:"totalEntries"`
	Overwritten  int    `json:"overwritten"`
	Size         int64  `json:"size"`
}

// MergeVPKFiles 把 filePaths 按给定顺序合并成一个新 VPK，写到 outputPath。
func (a *App) MergeVPKFiles(filePaths []string, outputPath string) (VPKMergeResult, error) {
	result := VPKMergeResult{}
	target := strings.TrimSpace(outputPath)
	if target == "" {
		return result, fmt.Errorf("请先选择合并后的保存位置")
	}
	if !strings.HasSuffix(strings.ToLower(target), ".vpk") {
		target += ".vpk"
	}
	sources := make([]string, 0, len(filePaths))
	for _, item := range filePaths {
		path := strings.TrimSpace(item)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return result, fmt.Errorf("找不到 VPK 文件: %s", path)
		}
		sources = append(sources, path)
	}
	if len(sources) < 2 {
		return result, fmt.Errorf("至少选择两个 VPK 才能合并（当前 %d 个）", len(sources))
	}
	result.SourceCount = len(sources)

	workDir, err := os.MkdirTemp("", "lytvpk-merge-*")
	if err != nil {
		return result, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(workDir)

	// 按列表顺序解包：后解包的会覆盖同名文件（与游戏"后加载覆盖"的直觉一致）。
	seen := make(map[string]struct{}, 1024)
	for _, source := range sources {
		written, err := extractVPKInto(source, workDir, seen)
		if err != nil {
			return result, fmt.Errorf("解包 %s 失败: %w", filepath.Base(source), err)
		}
		result.TotalEntries += written
	}
	result.Overwritten = result.TotalEntries - len(seen)
	if len(seen) == 0 {
		return result, fmt.Errorf("这些 VPK 里没有任何文件，已取消合并")
	}

	outputDir := filepath.Dir(target)
	baseName := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return result, fmt.Errorf("无法创建输出目录: %w", err)
	}
	// 复用现有打包通道：它负责写 VPK v1 目录结构并给出进度。
	packResult, err := a.packVPKDirectoryWithOptions(workDir, outputDir, false, baseName, nil)
	if err != nil {
		return result, fmt.Errorf("打包合并结果失败: %w", err)
	}
	result.OutputPath = packResult.OutputPath
	if info, err := os.Stat(result.OutputPath); err == nil {
		result.Size = info.Size()
	}
	return result, nil
}

// extractVPKInto 把一个 VPK 解包进 rootDir，返回本次写出的文件数与累计条目集合。
func extractVPKInto(vpkPath string, rootDir string, seen map[string]struct{}) (int, error) {
	opener := vpk.Single(vpkPath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		return 0, err
	}
	written := 0
	for i := range archive.Files {
		file := &archive.Files[i]
		entryName, decodeErr := parser.DecodeVPKEntryName(file.Name())
		if decodeErr != nil {
			return written, fmt.Errorf("解码条目名失败 %s: %w", file.Name(), decodeErr)
		}
		target, pathErr := safeVPKOutputPath(rootDir, entryName)
		if pathErr != nil {
			return written, pathErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return written, err
		}
		// extractVPKEntry 用 O_EXCL（解包工具不覆盖已有文件）；合并需要"后包覆盖前包"，
		// 所以先删掉同名文件再解包。
		if _, statErr := os.Stat(target); statErr == nil {
			if err := os.Remove(target); err != nil {
				return written, fmt.Errorf("覆盖已存在条目失败 %s: %w", entryName, err)
			}
		}
		if err := extractVPKEntry(opener, file, target); err != nil {
			return written, err
		}
		seen[strings.ToLower(filepath.ToSlash(entryName))] = struct{}{}
		written++
	}
	return written, nil
}

// SelectVPKMergeOutputFile 让用户选一个保存位置（默认给 .vpk 后缀）。
func (a *App) SelectVPKMergeOutputFile(defaultName string) (string, error) {
	name := strings.TrimSpace(defaultName)
	if name == "" {
		name = "merged_addon.vpk"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".vpk") {
		name += ".vpk"
	}
	selection, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存合并后的 VPK",
		DefaultFilename: name,
		Filters: []runtime.FileFilter{
			{DisplayName: "VPK 文件 (*.vpk)", Pattern: "*.vpk"},
		},
	})
	if err != nil {
		return "", err
	}
	return selection, nil
}

// DedupeVPKMergeSources 去重但保持传入顺序（合并的覆盖顺序 = 用户看到的列表顺序，不能重排）。
func DedupeVPKMergeSources(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(filepath.ToSlash(trimmed))
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
