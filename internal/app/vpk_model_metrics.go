package app

import (
	"sync"
	"sync/atomic"

	"vpk-manager/internal/parser"
)

// VPKModelMetric is the cached LOD0 geometry summary used by the list sort.
type VPKModelMetric struct {
	Path           string `json:"path"`
	ModelCount     int    `json:"modelCount"`
	TotalVertices  int    `json:"totalVertices"`
	TotalTriangles int    `json:"totalTriangles"`
	Error          string `json:"error,omitempty"`
}

// GetVPKModelMetrics analyzes requested cached VPK files and returns their LOD0 model complexity.
// It is intentionally on-demand: normal directory scans do not parse geometry for every VPK.
func (a *App) GetVPKModelMetrics(filePaths []string) []VPKModelMetric {
	metrics := make([]VPKModelMetric, len(filePaths))
	var waitGroup sync.WaitGroup
	var analyzedCount int64

	for index, filePath := range filePaths {
		cached, ok := a.vpkCache.Load(filePath)
		if !ok {
			metrics[index] = VPKModelMetric{Path: filePath, Error: "文件未找到"}
			continue
		}

		cache := cached.(*VPKFileCache)
		file := cache.File
		if file.ModelStatsKnown {
			metrics[index] = VPKModelMetric{
				Path:           file.Path,
				ModelCount:     file.ModelCount,
				TotalVertices:  file.ModelVertices,
				TotalTriangles: file.ModelTriangles,
			}
			continue
		}

		waitGroup.Add(1)
		metricIndex := index
		targetPath := file.Path
		a.submitPoolTask(func() {
			defer waitGroup.Done()

			stats, err := parser.AnalyzeVPKModelStats(targetPath)
			if err != nil {
				metrics[metricIndex] = VPKModelMetric{Path: targetPath, Error: err.Error()}
				return
			}

			metric := VPKModelMetric{
				Path:           targetPath,
				ModelCount:     stats.ModelCount,
				TotalVertices:  stats.TotalVertices,
				TotalTriangles: stats.TotalTriangles,
			}
			metrics[metricIndex] = metric

			if current, found := a.vpkCache.Load(targetPath); found {
				currentCache := current.(*VPKFileCache)
				currentCache.File.ModelStatsKnown = true
				currentCache.File.ModelCount = metric.ModelCount
				currentCache.File.ModelVertices = metric.TotalVertices
				currentCache.File.ModelTriangles = metric.TotalTriangles
				a.vpkCache.Store(targetPath, currentCache)
			}
			atomic.AddInt64(&analyzedCount, 1)
		})
	}

	waitGroup.Wait()
	// 模型指标就存在 cache.File（parser.VPKFile 的 modelStats* 字段）里，顺手触发一次
	// 扫描缓存落盘：下次冷启动直接命中，不再为整库重扫模型（真机 2912 个 Mod 首次 43s）。
	// 失效判据仍由扫描缓存的 size+mtime+版本 负责，不新增规则。
	if atomic.LoadInt64(&analyzedCount) > 0 {
		a.saveVPKScanCacheAsync()
	}
	return metrics
}
