package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"vpk-manager/internal/gamedata"
)

// 游戏本体文件索引（StockIndex）的应用层接线。
//
// 职责边界：
//   - internal/gamedata 负责"怎么扫、怎么查"（纯逻辑，可单测）；
//   - 本文件负责"缓存放哪、什么时候建、失败怎么降级"。
//
// 降级原则（对应硬约束"可以多标、不能少标"）：
// 任何一步失败都只让索引为 nil，解析器据此跳过依赖本体的新证据通道，
// **既有标签产出必须一字不变**。所以这里绝不因为索引问题中断扫描或返回错误。

const (
	stockIndexCacheFileName = "stock_index.json"
	// stockIndexMaxAge <= 0：缓存永不过期。游戏更新后由用户点「重建」或 CLI 显式重建，
	// 避免每次启动都重新遍历游戏目录。
	stockIndexMaxAgeHours = 0
)

type stockIndexCache struct {
	mu      sync.Mutex
	loaded  bool
	index   *gamedata.StockIndex
	lastErr string
}

// StockIndexStatus 是给 UI / CLI 的索引状态快照。
type StockIndexStatus struct {
	CachePath string               `json:"cachePath"`
	GameRoot  string               `json:"gameRoot"`
	PathCount int                  `json:"pathCount"`
	Mounts    []gamedata.MountStat `json:"mounts"`
	// Degraded 为真表示存在跳过或报错的挂载点（索引仍可用，只是不完整）。
	Degraded bool   `json:"degraded"`
	Error    string `json:"error,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

func (a *App) stockIndexCachePath() string {
	a.ensureConfigPaths()
	if a.configDir == "" {
		return ""
	}
	return filepath.Join(a.configDir, stockIndexCacheFileName)
}

// gameRootForStockIndex 从当前选中的 addons 目录推出游戏安装根。
func (a *App) gameRootForStockIndex() (string, error) {
	rootDir := strings.TrimSpace(a.rootDirectorySnapshot())
	if rootDir == "" {
		return "", fmt.Errorf("未选择 Mod 目录（addons）")
	}
	return gamedata.NormalizeGameRoot(rootDir)
}

// loadStockIndex 懒加载索引：优先缓存，缓存不可用则重建；任何失败都只记日志并返回 nil。
func (a *App) loadStockIndex() *gamedata.StockIndex {
	a.stockIndex.mu.Lock()
	defer a.stockIndex.mu.Unlock()

	if a.stockIndex.loaded {
		return a.stockIndex.index
	}
	a.stockIndex.loaded = true

	cachePath := a.stockIndexCachePath()
	gameRoot, err := a.gameRootForStockIndex()
	if err != nil {
		// 游戏目录不可用：尝试仅用缓存（缓存里已有完整清单）。
		if cachePath != "" {
			if index, loadErr := gamedata.Load(cachePath); loadErr == nil {
				a.stockIndex.index = index
				return index
			}
		}
		a.stockIndex.lastErr = err.Error()
		log.Printf("本体索引不可用（标签识别将跳过依赖本体的证据通道）: %v", err)
		return nil
	}

	index, err := gamedata.LoadOrBuild(cachePath, gameRoot, 0)
	if err != nil {
		a.stockIndex.lastErr = err.Error()
		log.Printf("本体索引构建失败（标签识别将跳过依赖本体的证据通道）: %v", err)
		return nil
	}
	a.stockIndex.index = index
	return index
}

// StockIndexStatus 返回当前索引状态（会触发一次懒加载）。供设置页/CLI 展示。
func (a *App) GetStockIndexStatus() StockIndexStatus {
	index := a.loadStockIndex()
	cachePath := a.stockIndexCachePath()
	if index == nil {
		a.stockIndex.mu.Lock()
		lastErr := a.stockIndex.lastErr
		a.stockIndex.mu.Unlock()
		return StockIndexStatus{
			CachePath: cachePath,
			Error:     lastErr,
			Notes:     "本体索引不可用时，标签识别仍按既有规则工作，只是少了『精确替换目标』这一类证据。",
		}
	}
	return stockIndexStatusFor(index, cachePath)
}

// StockIndexQueryResult 是一次 glob 查询的结果（维护者/CLI 用）。
type StockIndexQueryResult struct {
	Pattern string   `json:"pattern"`
	Count   int      `json:"count"`
	Matches []string `json:"matches,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// QueryStockIndex 在索引里执行一次 glob 查询；limit<=0 时只返回计数。
func (a *App) QueryStockIndex(pattern string, limit int) StockIndexQueryResult {
	result := StockIndexQueryResult{Pattern: pattern}
	index := a.loadStockIndex()
	if index == nil {
		result.Error = "本体索引不可用"
		return result
	}
	matches := index.Glob(pattern)
	result.Count = len(matches)
	if limit > 0 && len(matches) > limit {
		result.Matches = matches[:limit]
	} else {
		result.Matches = matches
	}
	return result
}

// BuildStockIndex 强制重建索引（游戏更新后使用）。gameDirOverride 为空时用当前配置目录。
func (a *App) BuildStockIndex(gameDirOverride string) (StockIndexStatus, error) {
	gameRoot := ""
	if strings.TrimSpace(gameDirOverride) != "" {
		normalized, err := gamedata.NormalizeGameRoot(gameDirOverride)
		if err != nil {
			return StockIndexStatus{}, err
		}
		gameRoot = normalized
	} else {
		resolved, err := a.gameRootForStockIndex()
		if err != nil {
			return StockIndexStatus{}, err
		}
		gameRoot = resolved
	}

	index, err := gamedata.Build(gameRoot)
	if err != nil {
		return StockIndexStatus{}, err
	}
	cachePath := a.stockIndexCachePath()
	if cachePath != "" {
		if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr == nil {
			if saveErr := index.Save(cachePath); saveErr != nil {
				log.Printf("本体索引缓存写入失败（不影响本次使用）: %v", saveErr)
			}
		}
	}

	a.stockIndex.mu.Lock()
	a.stockIndex.loaded = true
	a.stockIndex.index = index
	a.stockIndex.lastErr = ""
	a.stockIndex.mu.Unlock()

	return stockIndexStatusFor(index, cachePath), nil
}

func stockIndexStatusFor(index *gamedata.StockIndex, cachePath string) StockIndexStatus {
	status := StockIndexStatus{
		CachePath: cachePath,
		GameRoot:  index.GameRoot,
		PathCount: index.PathCount,
		Mounts:    index.Mounts,
	}
	skipped := 0
	for _, mount := range index.Mounts {
		if mount.Skipped {
			skipped++
		}
		if mount.Error != "" {
			status.Degraded = true
		}
	}
	if skipped > 0 {
		// 只有 base 挂载点缺失才算异常；dlc/update 缺失是正常安装形态。
		status.Notes = fmt.Sprintf("有 %d 个挂载点不存在（DLC/update 缺失属正常）", skipped)
	}
	return status
}
