package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 用户反馈：多选之后只能"批量启用/禁用"（在 addons 与 disabled 之间搬文件），
// 不能批量改"游戏内开关"（addonlist.txt 的 0/1）。这里覆盖新的批量实现。
func TestSetVPKGameEnabledBatchWritesOnceAndReportsCounts(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "disabled"), 0755); err != nil {
		t.Fatal(err)
	}
	addonListPath := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	original := "\"AddonList\"\r\n{\r\n\t\"a.vpk\"\t\t\"0\"\r\n\t\"b.vpk\"\t\t\"1\"\r\n}\r\n"
	if err := os.WriteFile(addonListPath, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: addonsDir}
	a := filepath.Join(addonsDir, "a.vpk")
	b := filepath.Join(addonsDir, "b.vpk")
	d := filepath.Join(addonsDir, "disabled", "d.vpk")
	app.vpkCache.Store(a, &VPKFileCache{File: VPKFile{Path: a, Name: "a.vpk", Location: "root", GameStateKnown: true, GameEnabled: false}})
	app.vpkCache.Store(b, &VPKFileCache{File: VPKFile{Path: b, Name: "b.vpk", Location: "root", GameStateKnown: true, GameEnabled: true}})
	app.vpkCache.Store(d, &VPKFileCache{File: VPKFile{Path: d, Name: "d.vpk", Location: "disabled"}})

	result, err := app.SetVPKGameEnabledBatch([]string{a, b, d, a}, true)
	if err != nil {
		t.Fatalf("batch enable: %v", err)
	}
	if len(result.Updated) != 1 || result.Updated[0] != a {
		t.Fatalf("updated = %#v", result.Updated)
	}
	if len(result.Unchanged) != 1 || result.Unchanged[0] != b {
		t.Fatalf("unchanged = %#v", result.Unchanged)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != d {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	if result.Requested != 3 {
		t.Fatalf("重复路径应去重，requested = %d", result.Requested)
	}

	updated, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "\"a.vpk\"\t\t\"1\"") {
		t.Fatalf("a.vpk 应被写成 1:\n%s", updated)
	}
	if !strings.Contains(string(updated), "\"b.vpk\"\t\t\"1\"") {
		t.Fatalf("b.vpk 应保持 1:\n%s", updated)
	}

	// 再点一次：全部已经是目标状态，不应有 Updated。
	again, err := app.SetVPKGameEnabledBatch([]string{a, b}, true)
	if err != nil {
		t.Fatalf("second batch: %v", err)
	}
	if len(again.Updated) != 0 || len(again.Unchanged) != 2 {
		t.Fatalf("第二次应全部 unchanged: %#v", again)
	}

	// 反向：批量关闭。
	off, err := app.SetVPKGameEnabledBatch([]string{a, b}, false)
	if err != nil {
		t.Fatalf("batch disable: %v", err)
	}
	if len(off.Updated) != 2 {
		t.Fatalf("反向批量应有 2 个更新: %#v", off)
	}
	updated, err = os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), "\"a.vpk\"\t\t\"1\"") || strings.Contains(string(updated), "\"b.vpk\"\t\t\"1\"") {
		t.Fatalf("两个条目都应为 0:\n%s", updated)
	}
}

// 未记录在 addonlist.txt 的 Mod 批量开启时，要按"未记录插入位置"设置写进去。
func TestSetVPKGameEnabledBatchInsertsUnrecordedEntries(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(addonsDir, 0755); err != nil {
		t.Fatal(err)
	}
	addonListPath := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	if err := os.WriteFile(addonListPath, []byte("\"AddonList\"\r\n{\r\n}\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app := &App{rootDir: addonsDir, unrecordedModLoadOrderPlacement: addonListUnrecordedPlacementEnd}
	x := filepath.Join(addonsDir, "x.vpk")
	y := filepath.Join(addonsDir, "y.vpk")
	app.vpkCache.Store(x, &VPKFileCache{File: VPKFile{Path: x, Name: "x.vpk", Location: "root"}})
	app.vpkCache.Store(y, &VPKFileCache{File: VPKFile{Path: y, Name: "y.vpk", Location: "root"}})

	result, err := app.SetVPKGameEnabledBatch([]string{x, y}, true)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(result.Updated) != 2 {
		t.Fatalf("两个未记录项都应被写入: %#v", result)
	}
	updated, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "\"x.vpk\"") || !strings.Contains(string(updated), "\"y.vpk\"") {
		t.Fatalf("未记录条目应写进 addonlist.txt:\n%s", updated)
	}
}
