// Package stockfiles 负责加载"游戏原版文件白名单"批次。
//
// 白名单只用于降低冲突误报，**不会改写任何 Mod 或游戏文件**。
// 批次形式为每行一个归档内路径的纯文本，便于分批、增量维护：
//
//   - 内置批次：编译进二进制（`assets/engine-glue.txt`），始终可用；
//   - 用户批次：配置目录 `stock-files/*.txt`，可由用户或
//     `GenerateStockWhitelistBatchesFromGame` 按类别生成。
//
// 任一批次缺失或解析失败都只跳过该批次，不阻断冲突检测。
package stockfiles

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed assets/*.txt
var builtinAssets embed.FS

// Batch 是一个白名单批次。
type Batch struct {
	// Name 是批次名（内置批次为文件名，用户批次为相对路径）。
	Name string `json:"name"`
	// Builtin 标记该批次是否来自内置资源。
	Builtin bool `json:"builtin"`
	// Path 是用户批次的磁盘路径（内置批次为空）。
	Path string `json:"path,omitempty"`
	// Count 是解析后的有效路径数量。
	Count int `json:"count"`
	// Error 记录该批次的解析失败原因（为空表示成功）。
	Error string `json:"error,omitempty"`
}

// ParseLines 解析白名单文本：去掉空行与注释行，统一为小写 "/" 分隔。
func ParseLines(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		normalized := strings.ToLower(strings.ReplaceAll(line, "\\", "/"))
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

// LoadBuiltin 返回内置批次（名称 -> 路径列表）。内置资源损坏时返回空表而不是报错，
// 让调用方安全降级到"只使用应用内置忽略规则"。
func LoadBuiltin() (map[string][]string, []Batch) {
	batches := make(map[string][]string)
	descriptions := make([]Batch, 0, 4)

	entries, err := fs.ReadDir(builtinAssets, "assets")
	if err != nil {
		return batches, descriptions
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".txt") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		content, readErr := builtinAssets.ReadFile(filepath.ToSlash(filepath.Join("assets", name)))
		if readErr != nil {
			descriptions = append(descriptions, Batch{Name: name, Builtin: true, Error: readErr.Error()})
			continue
		}
		paths := ParseLines(string(content))
		batches[name] = paths
		descriptions = append(descriptions, Batch{Name: name, Builtin: true, Count: len(paths)})
	}
	return batches, descriptions
}

// LoadDirectory 读取目录下的全部 `*.txt` 批次。目录不存在时返回空结果且不报错
// （这是"用户还没有生成任何增量批次"的正常状态）。
func LoadDirectory(dir string) (map[string][]string, []Batch, error) {
	batches := make(map[string][]string)
	descriptions := make([]Batch, 0)
	if strings.TrimSpace(dir) == "" {
		return batches, descriptions, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return batches, descriptions, nil
		}
		return batches, descriptions, fmt.Errorf("无法读取原版白名单目录: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".txt") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			descriptions = append(descriptions, Batch{Name: name, Path: path, Error: readErr.Error()})
			continue
		}
		paths := ParseLines(string(content))
		batches[name] = paths
		descriptions = append(descriptions, Batch{Name: name, Path: path, Count: len(paths)})
	}
	return batches, descriptions, nil
}

// CategoryName 把归档内路径归档到"类别批次名"，用于按类生成增量白名单。
// 顶层目录优先；没有目录的根文件统一归入 `_root.txt`。
func CategoryName(archivePath string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(archivePath), "\\", "/"))
	if normalized == "" {
		return ""
	}
	if index := strings.Index(normalized, "/"); index > 0 {
		category := normalized[:index]
		return sanitizeCategory(category) + ".txt"
	}
	return "_root.txt"
}

func sanitizeCategory(category string) string {
	var builder strings.Builder
	for _, r := range category {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	value := strings.Trim(builder.String(), "_")
	if value == "" {
		return "misc"
	}
	return value
}
