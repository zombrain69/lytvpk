package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 改名 / 打标签 / 隐藏都会改文件名，也就改了 addonlist 的键。
//
// 真机实测的缺口（community.15 修）：给 zztest_rifle_a.vpk 打标签后文件名变成
// [标签]zztest_rifle_a.vpk，addonlist.txt 却一个字节没动 —— 游戏侧那条记录成了
// 指向不存在文件的悬空条目，界面还会因为缓存显示"游戏内开启"。
// 下面三条钉住"三种改名方式都要跟着改键，且保序保值"。

func readAddonListFile(t *testing.T, rootDir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(rootDir, "addonlist.txt"))
	if err != nil {
		t.Fatalf("读取 addonlist.txt: %v", err)
	}
	return string(raw)
}

func assertAddonListEntry(t *testing.T, rootDir, wantKey, wantValue string) {
	t.Helper()
	content := readAddonListFile(t, rootDir)
	items := parseAddonListItems(content)
	for _, item := range items {
		if normalizeAddonListKey(item.Name) == normalizeAddonListKey(wantKey) {
			if item.Value != wantValue {
				t.Fatalf("键 %s 的开关值应为 %s，实际 %s（内容：\n%s）", wantKey, wantValue, item.Value, content)
			}
			return
		}
	}
	t.Fatalf("addonlist.txt 里找不到键 %s（内容：\n%s）", wantKey, content)
}

func assertNoAddonListEntry(t *testing.T, rootDir, unwantedKey string) {
	t.Helper()
	content := readAddonListFile(t, rootDir)
	for _, item := range parseAddonListItems(content) {
		if normalizeAddonListKey(item.Name) == normalizeAddonListKey(unwantedKey) {
			t.Fatalf("addonlist.txt 里还留着旧键 %s（内容：\n%s）", unwantedKey, content)
		}
	}
}

func TestRenameVPKFileRebindsAddonListEntry(t *testing.T) {
	app, addonsDir := newPriorityTestApp(t)
	oldPath := filepath.Join(addonsDir, "a.vpk")
	newPath := filepath.Join(addonsDir, "renamed.vpk")

	if _, err := app.RenameVPKFile(oldPath, "renamed.vpk"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("重命名后的文件不存在: %v", err)
	}

	assertAddonListEntry(t, filepath.Dir(addonsDir), "renamed.vpk", "1")
	assertNoAddonListEntry(t, filepath.Dir(addonsDir), "a.vpk")

	// 顺序也要保住：a.vpk 原本排在 b.vpk 前面，改键之后仍应在前。
	items := parseAddonListItems(readAddonListFile(t, filepath.Dir(addonsDir)))
	if len(items) < 2 || normalizeAddonListKey(items[0].Name) != "renamed.vpk" {
		t.Fatalf("改键后条目顺序变了: %#v", items)
	}
}

func TestSetVPKTagsRebindsAddonListEntry(t *testing.T) {
	app, addonsDir := newPriorityTestApp(t)
	path := filepath.Join(addonsDir, "a.vpk")

	if err := app.SetVPKTags(path, "", []string{"武器"}); err != nil {
		t.Fatalf("SetVPKTags: %v", err)
	}
	taggedPath := filepath.Join(addonsDir, "[武器]a.vpk")
	if _, err := os.Stat(taggedPath); err != nil {
		t.Fatalf("打标签后的文件不存在: %v", err)
	}

	assertAddonListEntry(t, filepath.Dir(addonsDir), "[武器]a.vpk", "1")
	assertNoAddonListEntry(t, filepath.Dir(addonsDir), "a.vpk")

	// 清标签后要回到原键，值也不能丢。
	if err := app.SetVPKTags(taggedPath, "", nil); err != nil {
		t.Fatalf("清除标签: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("清标签后名字没恢复: %v", err)
	}
	assertAddonListEntry(t, filepath.Dir(addonsDir), "a.vpk", "1")
	assertNoAddonListEntry(t, filepath.Dir(addonsDir), "[武器]a.vpk")
}

func TestToggleVPKVisibilityRebindsAddonListEntry(t *testing.T) {
	app, addonsDir := newPriorityTestApp(t)
	path := filepath.Join(addonsDir, "a.vpk")
	hiddenName := "_a.vpk"

	if _, err := app.ToggleVPKVisibility(path); err != nil {
		t.Fatalf("隐藏: %v", err)
	}
	assertAddonListEntry(t, filepath.Dir(addonsDir), hiddenName, "1")
	assertNoAddonListEntry(t, filepath.Dir(addonsDir), "a.vpk")

	if _, err := app.ToggleVPKVisibility(filepath.Join(addonsDir, hiddenName)); err != nil {
		t.Fatalf("显示: %v", err)
	}
	assertAddonListEntry(t, filepath.Dir(addonsDir), "a.vpk", "1")
	assertNoAddonListEntry(t, filepath.Dir(addonsDir), hiddenName)
}

// 开启「监控并自动恢复」时，改键必须走完整事务（含受保护快照同步），
// 而那条路径要取 a.mu —— 键迁移必须在锁外做，否则这里会直接卡死。
func TestSetVPKTagsWithAddonListGuardEnabledDoesNotDeadlock(t *testing.T) {
	app, addonsDir := newPriorityTestApp(t)
	addonListPath := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	app.setAddonListGuardEnabled(true)
	t.Cleanup(func() { app.setAddonListGuardEnabled(false) })

	// 先放一份"旧"快照，改键之后它必须被同步成新内容。
	oldContent, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAddonListBytesAtomically(addonListManagedSnapshotPath(addonListPath), oldContent); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- app.SetVPKTags(filepath.Join(addonsDir, "a.vpk"), "", []string{"武器"}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SetVPKTags: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("开启 addonlist 保护后 SetVPKTags 卡住了：键迁移很可能又跑进了 a.mu 里面")
	}

	assertAddonListEntry(t, filepath.Dir(addonsDir), "[武器]a.vpk", "1")

	snapshot, err := os.ReadFile(addonListManagedSnapshotPath(addonListPath))
	if err != nil {
		t.Fatalf("受保护版本不存在: %v", err)
	}
	if !strings.Contains(string(snapshot), "[武器]a.vpk") {
		t.Fatalf("受保护版本没有跟着同步（会被监控还原成旧键）:\n%s", string(snapshot))
	}
}
