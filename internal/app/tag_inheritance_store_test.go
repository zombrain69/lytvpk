package app

import (
	"os"
	"path/filepath"
	"testing"
)

// 套件继承快照：并集语义必须"只增不减"——用户增删同套件成员后，已继承的标签不能掉。
func TestMergeSuiteInheritanceTagsKeepsStored(t *testing.T) {
	// 当轮因为成员变化只算出 AK47，但快照里还有「步枪」→ 合并结果必须保留两者。
	merged, changed := mergeSuiteInheritanceTags([]string{"AK47", "步枪"}, []string{"AK47"})
	if changed {
		t.Fatalf("快照已包含全部标签时不应报告变化（merged=%v）", merged)
	}
	if len(merged) != 2 || merged[0] != "AK47" || merged[1] != "步枪" {
		t.Fatalf("并集应保留快照里的标签，实际 %v", merged)
	}
	// 完全相同 → 不算变化（避免每次扫描都写盘）。
	if _, changed := mergeSuiteInheritanceTags([]string{"AK47"}, []string{"AK47"}); changed {
		t.Fatalf("相同集合不应报告变化")
	}
	// 新增标签 → 变化 + 去重 + 大小写不敏感。
	merged, changed = mergeSuiteInheritanceTags([]string{"AK47"}, []string{"ak47", "步枪"})
	if !changed || len(merged) != 2 {
		t.Fatalf("新增标签应合并进去且去重，实际 changed=%v merged=%v", changed, merged)
	}
}

// 快照落盘/读取往返：路径跟着 configDir 走，损坏或缺失时从空开始（只少继承、不少标注）。
func TestSuiteInheritanceStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	app := &App{configDir: dir}
	if got := app.suiteInheritanceStorePath(); got != filepath.Join(dir, "suite_inheritance.json") {
		t.Fatalf("快照路径不对：%s", got)
	}
	store := app.loadSuiteInheritanceStore()
	if len(store.Namespaces) != 0 {
		t.Fatalf("首次加载应为空，实际 %v", store.Namespaces)
	}
	store.Namespaces["materials/models/codm/ice"] = []string{"AK47", "步枪"}
	app.saveSuiteInheritanceStore(store)
	if _, err := os.Stat(filepath.Join(dir, "suite_inheritance.json")); err != nil {
		t.Fatalf("快照未落盘: %v", err)
	}
	reloaded := app.loadSuiteInheritanceStore()
	if got := reloaded.Namespaces["materials/models/codm/ice"]; len(got) != 2 {
		t.Fatalf("快照未正确读回: %v", reloaded.Namespaces)
	}
	// 损坏文件 → 从空开始时不能 panic。
	if err := os.WriteFile(filepath.Join(dir, "suite_inheritance.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if broken := app.loadSuiteInheritanceStore(); len(broken.Namespaces) != 0 {
		t.Fatalf("损坏快照应降级为空: %v", broken.Namespaces)
	}
}
