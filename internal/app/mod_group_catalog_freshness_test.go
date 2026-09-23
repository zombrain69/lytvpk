package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 「准备给智能体的材料」导出的清单必须是"当前状态"：
// 往 mod 目录里加/删文件并重新扫描后，再点一次按钮，清单里也应随之增/删。
// 这条测试就是回答"重复点按钮会不会更新"的回归守卫。

type catalogSnapshot struct {
	GeneratedAt string `json:"generatedAt"`
	ModCount    int    `json:"modCount"`
	Mods        []struct {
		Key string `json:"key"`
	} `json:"mods"`
}

func readCatalogSnapshot(t *testing.T, path string) catalogSnapshot {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取清单失败: %v", err)
	}
	var snapshot catalogSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("解析清单失败: %v", err)
	}
	return snapshot
}

func catalogKeys(snapshot catalogSnapshot) map[string]bool {
	keys := make(map[string]bool, len(snapshot.Mods))
	for _, mod := range snapshot.Mods {
		keys[mod.Key] = true
	}
	return keys
}

func TestGroupingCatalogReflectsAddedAndRemovedModsAfterRescan(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	firstPath := filepath.Join(t.TempDir(), "catalog-first.json")
	if _, err := a.ExportGroupingCatalog(firstPath); err != nil {
		t.Fatalf("首次导出: %v", err)
	}
	first := readCatalogSnapshot(t, firstPath)
	firstKeys := catalogKeys(first)
	if !firstKeys["c.vpk"] {
		t.Fatalf("前置条件：首次清单里应有 c.vpk: %#v", firstKeys)
	}
	if first.ModCount != len(first.Mods) || first.GeneratedAt == "" {
		t.Fatalf("清单缺少 modCount / generatedAt: %#v", first)
	}

	// 磁盘上真实地"加一个 / 删一个"，然后重新扫描（等价于界面上的"刷新"）。
	writeTestVPK(t, filepath.Join(addonsDir, "new.vpk"), map[string][]byte{
		"materials/new.vtf": []byte("n"),
	})
	if err := os.Remove(filepath.Join(addonsDir, "c.vpk")); err != nil {
		t.Fatalf("删除 c.vpk: %v", err)
	}
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("重新扫描: %v", err)
	}

	secondPath := filepath.Join(t.TempDir(), "catalog-second.json")
	if _, err := a.ExportGroupingCatalog(secondPath); err != nil {
		t.Fatalf("再次导出: %v", err)
	}
	second := readCatalogSnapshot(t, secondPath)
	secondKeys := catalogKeys(second)
	if !secondKeys["new.vpk"] {
		t.Fatalf("新增的 Mod 应出现在清单里: %#v", secondKeys)
	}
	if secondKeys["c.vpk"] {
		t.Fatalf("已删除的 Mod 不应再出现在清单里: %#v", secondKeys)
	}
	if second.ModCount != first.ModCount {
		// 一增一删，数量不变；这里只是给出更直观的失败信息。
		t.Logf("modCount: first=%d second=%d", first.ModCount, second.ModCount)
	}
	if second.ModCount != len(second.Mods) {
		t.Fatalf("modCount 与实际条目不一致: %#v", second.ModCount)
	}
}

func TestPrepareGroupingWorkspaceUsesLatestScan(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	workspace, err := a.PrepareGroupingWorkspace()
	if err != nil {
		t.Fatalf("准备材料: %v", err)
	}
	beforeCount := workspace.ModCount
	catalogPath := workspace.CatalogPath

	writeTestVPK(t, filepath.Join(addonsDir, "extra.vpk"), map[string][]byte{
		"materials/extra.vtf": []byte("e"),
	})
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("重新扫描: %v", err)
	}
	again, err := a.PrepareGroupingWorkspace()
	if err != nil {
		t.Fatalf("再次准备材料: %v", err)
	}
	if again.ModCount != beforeCount+1 {
		t.Fatalf("materials 里的 Mod 数应跟着扫描结果更新: before=%d after=%d", beforeCount, again.ModCount)
	}
	if again.CatalogPath != catalogPath {
		t.Fatalf("清单路径应保持稳定: %q -> %q", catalogPath, again.CatalogPath)
	}
	// 清单文件被覆盖成最新的一份（同一个路径，内容含新增的 Mod）。
	snapshot := readCatalogSnapshot(t, again.CatalogPath)
	if !catalogKeys(snapshot)["extra.vpk"] {
		t.Fatalf("覆盖后的清单应包含新增 Mod: %#v", catalogKeys(snapshot))
	}
}

// 面向用户的导出（「导出 Mod 清单…」/「准备给智能体的材料」内部走的这条）必须**自己先扫一遍**：
// 用户往目录里丢/删文件后即使忘了点刷新，材料也不能是旧的。
func TestExportGroupingCatalogFreshScansBeforeExport(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	firstPath := filepath.Join(t.TempDir(), "catalog-before.json")
	if _, err := a.exportGroupingCatalogFresh(firstPath); err != nil {
		t.Fatalf("首次导出: %v", err)
	}
	first := readCatalogSnapshot(t, firstPath)

	// 只改磁盘，**不手动重新扫描**，再导出一次。
	writeTestVPK(t, filepath.Join(addonsDir, "fresh.vpk"), map[string][]byte{
		"materials/fresh.vtf": []byte("f"),
	})
	if err := os.Remove(filepath.Join(addonsDir, "c.vpk")); err != nil {
		t.Fatalf("删除 c.vpk: %v", err)
	}

	secondPath := filepath.Join(t.TempDir(), "catalog-after.json")
	if _, err := a.exportGroupingCatalogFresh(secondPath); err != nil {
		t.Fatalf("再次导出: %v", err)
	}
	second := readCatalogSnapshot(t, secondPath)
	secondKeys := catalogKeys(second)
	if !secondKeys["fresh.vpk"] {
		t.Fatalf("导出前应自动重新扫描，新增的 Mod 必须在清单里: %#v", secondKeys)
	}
	if secondKeys["c.vpk"] {
		t.Fatalf("导出前应自动重新扫描，已删除的 Mod 不应还在清单里: %#v", secondKeys)
	}
	if second.ModCount != len(second.Mods) || second.ModCount != first.ModCount {
		t.Fatalf("一增一删后条目数应保持不变且自洽: first=%d second=%d mods=%d",
			first.ModCount, second.ModCount, len(second.Mods))
	}
}

func TestPrepareGroupingWorkspaceScansWithoutManualRefresh(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	before, err := a.PrepareGroupingWorkspace()
	if err != nil {
		t.Fatalf("准备材料: %v", err)
	}

	// 模拟"用户忘了点刷新"：只把文件放进目录。
	writeTestVPK(t, filepath.Join(addonsDir, "dropped.vpk"), map[string][]byte{
		"materials/dropped.vtf": []byte("d"),
	})
	after, err := a.PrepareGroupingWorkspace()
	if err != nil {
		t.Fatalf("再次准备材料: %v", err)
	}
	if after.ModCount != before.ModCount+1 {
		t.Fatalf("没有手动刷新时，材料也应自己扫描到新文件: before=%d after=%d",
			before.ModCount, after.ModCount)
	}
	if !catalogKeys(readCatalogSnapshot(t, after.CatalogPath))["dropped.vpk"] {
		t.Fatalf("材料清单里应包含刚放进去的 Mod")
	}
}
