package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSnapshotTestApp 造一个"有真实文件布局"的库：
// <tmp>/LytVPK（配置目录） + <tmp>/left4dead2/addons（库） + addonlist.txt。
func newSnapshotTestApp(t *testing.T, addonListContent string) (*App, string) {
	t.Helper()
	base := t.TempDir()
	addonsRoot := filepath.Join(base, "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsRoot, "disabled"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(addonsRoot, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(base, "LytVPK")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if addonListContent != "" {
		path := filepath.Join(base, "left4dead2", "addonlist.txt")
		if err := os.WriteFile(path, []byte(addonListContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &App{rootDir: addonsRoot, configDir: configDir}, addonsRoot
}

func writeSnapshotTestFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("v", size)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func planActions(plan ModSnapshotRestorePlan) map[string]string {
	actions := make(map[string]string, len(plan.Items))
	for _, item := range plan.Items {
		actions[item.Key] = item.Action
	}
	return actions
}

// 文件名快照：记录 addonlist 与 Mod 清单；恢复时能把"被禁用的移回来 / 新增的移出去 / 改过的 addonlist 还原"。
func TestModSnapshotNamesRoundTrip(t *testing.T) {
	content := "\"AddonList\"\n{\n\t\"a.vpk\"\t\t\"1\"\n\t\"b.vpk\"\t\t\"0\"\n}\n"
	app, root := newSnapshotTestApp(t, content)
	writeSnapshotTestFile(t, filepath.Join(root, "a.vpk"), 10)
	writeSnapshotTestFile(t, filepath.Join(root, "disabled", "b.vpk"), 10)

	meta, err := app.CreateModSnapshot("整理前", "names")
	if err != nil {
		t.Fatalf("创建快照失败: %v", err)
	}
	if meta.Mode != "names" || meta.ModCount != 2 || meta.AddonListEntries != 2 {
		t.Fatalf("快照元信息不对: %#v", meta)
	}
	list, err := app.ListModSnapshots()
	if err != nil || len(list) != 1 {
		t.Fatalf("列表应有一条，实际 %#v (err=%v)", list, err)
	}

	// 快照之后把 a 禁用、新增 c、并改掉 addonlist。
	if err := os.Rename(filepath.Join(root, "a.vpk"), filepath.Join(root, "disabled", "a.vpk")); err != nil {
		t.Fatal(err)
	}
	writeSnapshotTestFile(t, filepath.Join(root, "c.vpk"), 10)
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "addonlist.txt"),
		[]byte("\"AddonList\"\n{\n\t\"b.vpk\"\t\t\"1\"\n\t\"c.vpk\"\t\t\"1\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := app.PreviewModSnapshotRestore(meta.ID)
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	actions := planActions(plan)
	if actions["a.vpk"] != "enable" {
		t.Fatalf("a.vpk 应计划启用，实际 %#v", actions)
	}
	if actions["b.vpk"] != "skip" {
		t.Fatalf("b.vpk 已经一致，应跳过，实际 %#v", actions)
	}
	if actions["c.vpk"] != "disable" {
		t.Fatalf("新增的 c.vpk 应计划禁用，实际 %#v", actions)
	}

	result, err := app.RestoreModSnapshot(meta.ID)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if result.Enabled != 1 || result.Disabled != 1 || !result.AddonListWrote {
		t.Fatalf("恢复结果不对: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "a.vpk")); err != nil {
		t.Fatalf("a.vpk 应回到根目录: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "disabled", "c.vpk")); err != nil {
		t.Fatalf("c.vpk 应被移到 disabled: %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(filepath.Dir(root), "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(restored), "a.vpk") || strings.Contains(string(restored), "c.vpk") {
		t.Fatalf("addonlist 应回到快照内容，实际：%s", restored)
	}
}

// 完整备份：文件被删除后能补回来；大小不同的文件会被覆盖；执行前留 addonlist 备份。
func TestModSnapshotFullRestoresDeletedFiles(t *testing.T) {
	content := "\"AddonList\"\n{\n\t\"keep.vpk\"\t\t\"1\"\n}\n"
	app, root := newSnapshotTestApp(t, content)
	writeSnapshotTestFile(t, filepath.Join(root, "keep.vpk"), 32)
	writeSnapshotTestFile(t, filepath.Join(root, "keep.jpg"), 4)

	meta, err := app.CreateModSnapshot("完整备份", "full")
	if err != nil {
		t.Fatalf("创建完整快照失败: %v", err)
	}
	if meta.BackupSize == 0 {
		t.Fatal("完整备份应记录备份占用")
	}

	if err := os.Remove(filepath.Join(root, "keep.vpk")); err != nil {
		t.Fatal(err)
	}
	plan, err := app.PreviewModSnapshotRestore(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actions := planActions(plan); actions["keep.vpk"] != "restore" {
		t.Fatalf("删除的文件应计划从快照补回，实际 %#v", actions)
	}
	result, err := app.RestoreModSnapshot(meta.ID)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if result.Restored != 1 {
		t.Fatalf("应补回 1 个文件，实际 %#v", result)
	}
	restored, err := os.Stat(filepath.Join(root, "keep.vpk"))
	if err != nil || restored.Size() != 32 {
		t.Fatalf("文件内容没补回来: %v size=%d", err, restored.Size())
	}
	if _, err := os.Stat(filepath.Join(root, "keep.jpg")); err != nil {
		t.Fatalf("同名图片也应一起补回: %v", err)
	}

	backups, err := app.ListAddonListBackups()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, backup := range backups {
		if backup.Kind == "before-snapshot-restore" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("恢复前应留 addonlist 备份，实际 %#v", backups)
	}
}

// 删除快照：移入回收站，Mod 文件不受影响；损坏的快照会被列表跳过而不是让整页报错。
func TestModSnapshotDeleteAndCorruptTolerance(t *testing.T) {
	app, root := newSnapshotTestApp(t, "\"AddonList\"\n{\n\t\"x.vpk\"\t\t\"1\"\n}\n")
	writeSnapshotTestFile(t, filepath.Join(root, "x.vpk"), 8)
	meta, err := app.CreateModSnapshot("删除我", "names")
	if err != nil {
		t.Fatal(err)
	}

	broken := filepath.Join(app.modSnapshotsRoot(), "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "snapshot.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := app.ListModSnapshots()
	if err != nil {
		t.Fatalf("损坏快照不应让列表失败: %v", err)
	}
	if len(list) != 1 || list[0].ID != meta.ID {
		t.Fatalf("应只列出健康快照，实际 %#v", list)
	}

	if err := app.DeleteModSnapshot(meta.ID); err != nil {
		t.Fatalf("删除快照失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "x.vpk")); err != nil {
		t.Fatalf("删除快照不该动 Mod 文件: %v", err)
	}
	if list, _ := app.ListModSnapshots(); len(list) != 0 {
		t.Fatalf("快照应已删除，实际 %#v", list)
	}
}
