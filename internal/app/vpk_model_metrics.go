package app

import (
	"sync"

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
		})
	}

	waitGroup.Wait()
	return metrics
}
