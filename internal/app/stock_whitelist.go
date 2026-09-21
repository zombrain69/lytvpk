package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"vpk-manager/internal/parser"
	"vpk-manager/internal/stockfiles"
)

// 游戏原版文件白名单
//
// 用途：很多 Mod 会附带原版文件的完整副本（引擎胶水文件、原版脚本等），
// 两个这样的 Mod 在同一条原版路径上"重叠"并不代表真的冲突，属于典型误报。
//
// 三条约束：
//  1. 只用于忽略，**绝不改写**任何 Mod 文件或游戏文件；
//  2. 分批、可增量：内置批次 + 配置目录 `stock-files/*.txt` 用户批次；
//  3. 任何批次缺失或解析失败都只跳过该批次，降级为"内置忽略规则 + 其余批次"，
//     绝不阻断冲突检测。

const stockWhitelistDirName = "stock-files"

// StockWhitelistStatus 描述当前白名单加载情况。
type StockWhitelistStatus struct {
	// Directory 是用户批次目录。
	Directory string `json:"directory"`
	// BuiltinBatches / UserBatches 分别是内置与用户批次。
	BuiltinBatches []stockfiles.Batch `json:"builtinBatches"`
	UserBatches    []stockfiles.Batch `json:"userBatches"`
	// TotalPaths 是去重后的有效路径总数。
	TotalPaths int `json:"totalPaths"`
	// Degraded 为真表示存在解析失败的批次（已跳过，功能不中断）。
	Degraded bool `json:"degraded"`
}

type stockWhitelistCache struct {
	mu     sync.Mutex
	loaded bool
	set    conflictIgnoreSet
	status StockWhitelistStatus
}

func (a *App) stockWhitelistDirectory() string {
	a.ensureConfigPaths()
	if a.configDir == "" {
		return ""
	}
	return filepath.Join(a.configDir, stockWhitelistDirName)
}

// loadStockWhitelist 加载并缓存白名单；`force` 为真时重新读盘。
func (a *App) loadStockWhitelist(force bool) (conflictIgnoreSet, StockWhitelistStatus) {
	a.stockWhitelist.mu.Lock()
	defer a.stockWhitelist.mu.Unlock()

	if a.stockWhitelist.loaded && !force {
		return a.stockWhitelist.set, a.stockWhitelist.status
	}

	status := StockWhitelistStatus{}
	paths := make([]string, 0)

	builtinBatches, builtinDescriptions := stockfiles.LoadBuiltin()
	status.BuiltinBatches = builtinDescriptions
	for _, description := range builtinDescriptions {
		if description.Error != "" {
			status.Degraded = true
			log.Printf("原版白名单内置批次解析失败（已跳过）: %s: %s", description.Name, description.Error)
			continue
		}
		paths = append(paths, builtinBatches[description.Name]...)
	}

	dir := a.stockWhitelistDirectory()
	status.Directory = dir
	userBatches, userDescriptions, err := stockfiles.LoadDirectory(dir)
	if err != nil {
		// 目录整体读取失败：降级为仅内置批次。
		status.Degraded = true
		log.Printf("原版白名单目录读取失败，降级为内置批次: %v", err)
	} else {
		status.UserBatches = userDescriptions
		for _, description := range userDescriptions {
			if description.Error != "" {
				status.Degraded = true
				log.Printf("原版白名单批次解析失败（已跳过）: %s: %s", description.Name, description.Error)
				continue
			}
			paths = append(paths, userBatches[description.Name]...)
		}
	}

	set := newConflictIgnoreSet(paths)
	status.TotalPaths = len(set.exact) + len(set.prefixes)

	a.stockWhitelist.set = set
	a.stockWhitelist.status = status
	a.stockWhitelist.loaded = true
	return set, status
}

// GetStockWhitelistStatus 返回白名单加载情况，不会改写任何文件。
func (a *App) GetStockWhitelistStatus() (StockWhitelistStatus, error) {
	_, status := a.loadStockWhitelist(true)
	return status, nil
}

// ReloadStockWhitelist 让下次冲突检测重新读取批次文件。
func (a *App) ReloadStockWhitelist() error {
	a.loadStockWhitelist(true)
	return nil
}

// GenerateStockWhitelistBatchesFromGame 从本机原版 `pak01_dir.vpk` 提取路径，
// 按顶层目录类别写成增量批次（每个类别一个 txt）。只写配置目录，不动游戏文件。
func (a *App) GenerateStockWhitelistBatchesFromGame() (StockWhitelistStatus, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return StockWhitelistStatus{}, fmt.Errorf("未选择L4D2目录")
	}
	gameDir := filepath.Dir(filepath.Clean(rootDir))
	pakPath := filepath.Join(gameDir, "pak01_dir.vpk")
	if info, err := os.Stat(pakPath); err != nil || info.IsDir() {
		return StockWhitelistStatus{}, fmt.Errorf("未找到原版资源包: %s", pakPath)
	}

	files, err := parser.GetVPKFileList(pakPath)
	if err != nil {
		return StockWhitelistStatus{}, fmt.Errorf("无法解析原版资源包: %w", err)
	}

	byCategory := make(map[string][]string)
	for _, file := range files {
		category := stockfiles.CategoryName(file)
		if category == "" {
			continue
		}
		byCategory[category] = append(byCategory[category], file)
	}

	dir := a.stockWhitelistDirectory()
	if dir == "" {
		return StockWhitelistStatus{}, fmt.Errorf("未配置配置目录，无法写出白名单批次")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return StockWhitelistStatus{}, err
	}

	for category, paths := range byCategory {
		sort.Slice(paths, func(i, j int) bool {
			return strings.ToLower(paths[i]) < strings.ToLower(paths[j])
		})
		var builder strings.Builder
		builder.WriteString("# 由 LytVPK 从本机原版 pak01_dir.vpk 生成，仅用于降低冲突误报；不会改动任何游戏文件。\n")
		builder.WriteString("# 类别: " + strings.TrimSuffix(category, ".txt") + "；如需停用请删除该文件。\n")
		for _, path := range paths {
			builder.WriteString(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")))
			builder.WriteString("\n")
		}
		if err := os.WriteFile(filepath.Join(dir, category), []byte(builder.String()), 0o644); err != nil {
			return StockWhitelistStatus{}, fmt.Errorf("写入白名单批次失败 %s: %w", category, err)
		}
	}

	_, status := a.loadStockWhitelist(true)
	return status, nil
}

// DeleteStockWhitelistBatch 删除一个用户批次（只删配置目录里的 txt）。
func (a *App) DeleteStockWhitelistBatch(name string) error {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fmt.Errorf("缺少批次名")
	}
	if !strings.HasSuffix(strings.ToLower(name), ".txt") {
		return fmt.Errorf("只能删除 .txt 白名单批次: %s", name)
	}
	dir := a.stockWhitelistDirectory()
	if dir == "" {
		return fmt.Errorf("未配置配置目录，无法删除白名单批次")
	}
	path := filepath.Join(dir, name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	a.loadStockWhitelist(true)
	return nil
}
