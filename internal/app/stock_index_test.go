package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadStockIndexDegradesWithoutGameDir 覆盖硬约束要求的降级路径：
// 没选目录、没有缓存时，本体索引必须是 nil（而不是报错或中断扫描）。
func TestLoadStockIndexDegradesWithoutGameDir(t *testing.T) {
	app := &App{}
	if index := app.loadStockIndex(); index != nil {
		t.Fatalf("没有游戏目录时索引必须为 nil，got=%v", index)
	}

	status := app.GetStockIndexStatus()
	if status.Error == "" {
		t.Fatalf("应给出可读的原因，status=%+v", status)
	}
	if status.PathCount != 0 {
		t.Fatalf("降级时不应报告条目数，status=%+v", status)
	}

	// 索引不可用时查询必须安全失败，不影响调用方。
	result := app.QueryStockIndex("models/**", 5)
	if result.Error == "" || result.Count != 0 {
		t.Fatalf("降级查询应返回错误而不是结果，result=%+v", result)
	}

	// 二次调用不应反复尝试（loaded 标记），且仍然安全。
	if index := app.loadStockIndex(); index != nil {
		t.Fatalf("重复调用仍应为 nil")
	}
}

// TestLoadStockIndexUsesCacheWhenGameRootMissing 覆盖"游戏更新/移动后仍能用缓存"的路径。
func TestLoadStockIndexUsesCacheWhenGameRootMissing(t *testing.T) {
	configDir := t.TempDir()
	cachePath := filepath.Join(configDir, stockIndexCacheFileName)
	if err := os.WriteFile(cachePath, []byte(`{
  "schemaVersion": 1,
  "generatedAt": "2026-09-30T00:00:00+08:00",
  "gameRoot": "E:/game",
  "pathCount": 2,
  "paths": ["models/props_junk/gnome.mdl", "sound/player/hunter/voice/warn01.wav"]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{configDir: configDir}
	index := app.loadStockIndex()
	if index == nil {
		t.Fatalf("有缓存时即使没有 Mod 目录也必须能用缓存")
	}
	if !index.Exists("models/props_junk/gnome.mdl") {
		t.Fatalf("缓存内容未生效")
	}

	query := app.QueryStockIndex("sound/player/*/voice/*.wav", 10)
	if query.Error != "" || query.Count != 1 {
		t.Fatalf("查询结果异常: %+v", query)
	}
	if got := query.Matches[0]; got != "sound/player/hunter/voice/warn01.wav" {
		t.Fatalf("匹配结果异常: %q", got)
	}
}
