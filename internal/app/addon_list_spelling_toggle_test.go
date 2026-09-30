package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真机复现（与 community.25 修的是同一类问题，但走的是更常见的「禁用 → 启用」）：
//
//	磁盘上：ZzTest_MixedCase.VPK
//	禁用时：条目被移除（正常）
//	启用后：条目被重新插入成 "zztest_mixedcase.vpk" —— 丢了磁盘上的真实拼写
//
// 原因是 ToggleVPKFile 传的是 addonListKeyForManagedVPKPathFromRoot（规范化小写键），
// 而 updateAddonListEntries 把 values 的键同时当成"匹配键"和"新条目名"。
// 项目规则（addonListDisplayKeyForVPKPath）是：匹配用小写键，落盘用磁盘真实拼写。
func TestToggleVPKFileKeepsDiskSpellingWhenReenabling(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const name = "ZzTest_MixedCase.VPK"
	path := filepath.Join(addonsDir, name)
	if err := os.WriteFile(path, []byte("vpk"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{{Name: name, Value: "1"}})

	app := &App{rootDir: addonsDir, configDir: filepath.Join(gameDir, "config")}
	app.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
		Path: path, Name: name, Location: "root", Enabled: true,
	}})

	if err := app.ToggleVPKFile(path); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	disabledPath := filepath.Join(addonsDir, "disabled", name)
	if _, err := os.Stat(disabledPath); err != nil {
		t.Fatalf("文件没有进 disabled: %v", err)
	}
	if err := app.ToggleVPKFile(disabledPath); err != nil {
		t.Fatalf("启用失败: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(gameDir, "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, `"`+name+`"`) {
		t.Fatalf("重新启用的条目应保留磁盘上的真实拼写 %q，实际内容：\n%s", name, content)
	}
	if strings.Contains(content, `"zztest_mixedcase.vpk"`) {
		t.Fatalf("不该把条目写成规范化的小写键，实际内容：\n%s", content)
	}
}

// 同一类问题：把工坊文件「复制到 addons」时新建的 root 条目也必须保留真实拼写。
// 真机场景：订阅目录里的文件叫 ZzTest_Workshop_Copy.VPK，复制到根目录后
// addonlist 里出现的却是 zztest_workshop_copy.vpk。
func TestMoveWorkshopToAddonsKeepsDiskSpellingForNewEntry(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: `workshop\999000111.vpk`, Value: "1"},
	})

	const name = "ZzTest_Workshop_Copy.VPK"
	workshopPath := filepath.Join(workshopDir, name)
	if err := os.WriteFile(workshopPath, []byte("vpk"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: addonsDir, configDir: filepath.Join(gameDir, "config")}
	app.vpkCache.Store(workshopPath, &VPKFileCache{File: VPKFile{
		Path: workshopPath, Name: name, Location: "workshop", Enabled: true, WorkshopID: "999000111",
	}})
	if err := app.MoveWorkshopToAddons(workshopPath); err != nil {
		t.Fatalf("复制到 addons 失败: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(gameDir, "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, `"`+name+`"`) {
		t.Fatalf("新建的 root 条目应保留磁盘拼写 %q，实际内容：\n%s", name, content)
	}
	if strings.Contains(content, `"zztest_workshop_copy.vpk"`) {
		t.Fatalf("不该把条目写成规范化的小写键，实际内容：\n%s", content)
	}
}
