package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 模型指标属于扫描缓存内容的一部分（parser.VPKFile 自带 modelStats* 字段）：
// 分析完必须触发缓存落盘，否则每次冷启动都要为整库重扫模型 ——
// 真机 2912 个 Mod 首次实测 43s。
//
// 这里用"没有模型的普通 VPK"验证写盘/回读接线（AnalyzeVPKModelStats 对无模型文件
// 返回 0 计数且不报错，足以覆盖持久化路径）。
func TestGetVPKModelMetricsPersistsToScanCache(t *testing.T) {
	app, addonsDir := newScanCacheTestApp(t)
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描失败: %v", err)
	}
	target := filepath.Join(addonsDir, "a.vpk")

	metrics := app.GetVPKModelMetrics([]string{target})
	if len(metrics) != 1 || metrics[0].Error != "" {
		t.Fatalf("模型指标分析失败: %#v", metrics)
	}

	raw, err := os.ReadFile(filepath.Join(app.configDir, vpkScanCacheFileName))
	if err != nil {
		t.Fatalf("读扫描缓存失败: %v", err)
	}
	var payload vpkScanCachePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("扫描缓存 JSON 损坏: %v", err)
	}
	found := false
	for _, entry := range payload.Entries {
		if entry.Path == target {
			found = entry.File.ModelStatsKnown
		}
	}
	if !found {
		t.Fatalf("模型指标没有写进扫描缓存（下次冷启动会重扫）: %#v", payload.Entries)
	}

	// 新进程语义：内存缓存为空，只从磁盘缓存恢复 —— 指标必须已经就绪。
	restarted := appWithSameStores(t, app)
	if err := restarted.ScanVPKFiles(); err != nil {
		t.Fatalf("重启后的扫描失败: %v", err)
	}
	cached, ok := restarted.vpkCache.Load(target)
	if !ok {
		t.Fatalf("重启后缓存里应有 %s", target)
	}
	if !cached.(*VPKFileCache).File.ModelStatsKnown {
		t.Fatalf("重启后模型指标没有从磁盘缓存恢复")
	}
}
