package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStockWhitelistSuppressesEngineGlueOverlaps 覆盖内置批次的效果。
func TestStockWhitelistSuppressesEngineGlueOverlaps(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	writeTestVPK(t, filepath.Join(addonsDir, "glue-a.vpk"), map[string][]byte{
		"sound/sound.cache":                        []byte("a"),
		"scripts/vscripts/director_base_addon.nut": []byte("a"),
		"materials/real.vtf":                       []byte("a"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "glue-b.vpk"), map[string][]byte{
		"sound/sound.cache":                        []byte("b"),
		"scripts/vscripts/director_base_addon.nut": []byte("b"),
		"materials/real.vtf":                       []byte("b"),
	})

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, group := range result.ConflictGroups {
		for _, file := range group.Files {
			seen[file] = true
		}
	}
	for _, ignored := range []string{"sound/sound.cache", "scripts/vscripts/director_base_addon.nut"} {
		if seen[ignored] {
			t.Fatalf("原版白名单内的路径不应再报冲突: %s: %#v", ignored, seen)
		}
	}
	if !seen["materials/real.vtf"] {
		t.Fatalf("白名单以外的新增重叠必须保留: %#v", seen)
	}
}

// TestStockWhitelistUserBatchesAreAdditiveAndRemovable 覆盖增量批次。
func TestStockWhitelistUserBatchesAreAdditiveAndRemovable(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	writeTestVPK(t, filepath.Join(addonsDir, "e.vpk"), map[string][]byte{"materials/shared.vtf": []byte("e"), "materials/custom.vtf": []byte("e")})
	writeTestVPK(t, filepath.Join(addonsDir, "f.vpk"), map[string][]byte{"materials/shared.vtf": []byte("f"), "materials/custom.vtf": []byte("f")})

	countConflicts := func() map[string]bool {
		t.Helper()
		result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, group := range result.ConflictGroups {
			for _, file := range group.Files {
				seen[file] = true
			}
		}
		return seen
	}

	if !countConflicts()["materials/custom.vtf"] {
		t.Fatal("前置条件：自定义路径默认应报冲突")
	}

	dir := a.stockWhitelistDirectory()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "materials.txt"), []byte("# custom batch\nmaterials/custom.vtf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.ReloadStockWhitelist(); err != nil {
		t.Fatal(err)
	}
	if countConflicts()["materials/custom.vtf"] {
		t.Fatal("用户增量批次应让该路径不再报冲突")
	}

	if err := a.DeleteStockWhitelistBatch("materials.txt"); err != nil {
		t.Fatal(err)
	}
	if !countConflicts()["materials/custom.vtf"] {
		t.Fatal("删除批次后应恢复冲突报告")
	}
}

// TestStockWhitelistDegradesWhenBatchMissing 覆盖"加载失败降级"。
func TestStockWhitelistDegradesWhenBatchMissing(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	writeTestVPK(t, filepath.Join(addonsDir, "g.vpk"), map[string][]byte{"materials/x.vtf": []byte("g")})
	writeTestVPK(t, filepath.Join(addonsDir, "h.vpk"), map[string][]byte{"materials/x.vtf": []byte("h")})

	// 目录不存在：应当正常返回状态（不报错）且仍能检测冲突。
	status, err := a.GetStockWhitelistStatus()
	if err != nil {
		t.Fatalf("白名单目录缺失不应报错: %v", err)
	}
	if status.Degraded {
		t.Fatalf("目录缺失属于正常状态，不应标记为降级: %#v", status)
	}
	if len(status.BuiltinBatches) == 0 || status.TotalPaths == 0 {
		t.Fatalf("内置批次应始终可用: %#v", status)
	}
	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ConflictGroups) == 0 {
		t.Fatal("白名单降级后仍应正常检测冲突")
	}

	// 批次文件不可解析（目录路径）：降级但不阻断。
	if err := os.MkdirAll(filepath.Join(a.stockWhitelistDirectory(), "broken.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	status, err = a.GetStockWhitelistStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Degraded {
		t.Fatalf("目录形式的批次名应被忽略而不是标记降级: %#v", status.UserBatches)
	}
}

// TestGenerateStockWhitelistBatchesFromGame 用夹具 pak01_dir.vpk 覆盖生成流程，
// 并断言不会改动"游戏"文件。
func TestGenerateStockWhitelistBatchesFromGame(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	gameDir := filepath.Dir(addonsDir)
	pakPath := filepath.Join(gameDir, "pak01_dir.vpk")
	writeTestVPK(t, pakPath, map[string][]byte{
		"models/survivor/coach.mdl": []byte("stock"),
		"materials/wood/wall.vtf":   []byte("stock"),
		"iohints.txt":               []byte("stock"),
	})
	before := readAddonListBytes(t, pakPath)

	status, err := a.GenerateStockWhitelistBatchesFromGame()
	if err != nil {
		t.Fatalf("generate batches: %v", err)
	}
	if len(status.UserBatches) != 3 {
		t.Fatalf("应按顶层目录生成 3 个批次: %#v", status.UserBatches)
	}
	names := map[string]bool{}
	for _, batch := range status.UserBatches {
		if batch.Error != "" {
			t.Fatalf("批次 %s 解析失败: %s", batch.Name, batch.Error)
		}
		names[batch.Name] = true
	}
	for _, want := range []string{"models.txt", "materials.txt", "_root.txt"} {
		if !names[want] {
			t.Fatalf("缺少批次 %s: %#v", want, names)
		}
	}

	// 游戏文件必须保持原样。
	after := readAddonListBytes(t, pakPath)
	if string(before) != string(after) {
		t.Fatal("生成白名单不应改动游戏文件")
	}

	// 生成后的批次应真的生效。
	writeTestVPK(t, filepath.Join(addonsDir, "m1.vpk"), map[string][]byte{"models/survivor/coach.mdl": []byte("m1")})
	writeTestVPK(t, filepath.Join(addonsDir, "m2.vpk"), map[string][]byte{"models/survivor/coach.mdl": []byte("m2")})
	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range result.ConflictGroups {
		for _, file := range group.Files {
			if strings.EqualFold(file, "models/survivor/coach.mdl") {
				t.Fatalf("生成的白名单应抑制原版模型路径冲突: %#v", group.Files)
			}
		}
	}
}
