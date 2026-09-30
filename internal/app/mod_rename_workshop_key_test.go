package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真机场景：工坊里的 VPK 叫 999000111.vpk，addonlist 记的是 "workshop\999000111.vpk"。
// 在卡片菜单里「重命名」之后，文件名变了，addonlist 的键也该跟着变——
// 但必须仍然是 workshop\ 下的键，不能变成裸文件名（那会指向一个不存在的根目录文件，
// 游戏侧这条记录直接失效、Mod 静默不加载）。
func TestRenameWorkshopVPKKeepsWorkshopPrefixInAddonList(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldName := "999000111.vpk"
	newName := "999000111_备用.vpk"
	oldPath := filepath.Join(workshopDir, oldName)
	if err := os.WriteFile(oldPath, []byte("vpk"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: `workshop\` + oldName, Value: "1"},
		{Name: "zztest_rifle_a.vpk", Value: "1"},
	})

	app := &App{rootDir: addonsDir, configDir: filepath.Join(gameDir, "config")}
	app.vpkCache.Store(oldPath, &VPKFileCache{File: VPKFile{
		Path: oldPath, Name: oldName, Location: "workshop", Enabled: true, WorkshopID: "999000111",
	}})
	if _, err := app.RenameVPKFile(oldPath, newName); err != nil {
		t.Fatalf("重命名失败: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(gameDir, "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, `"workshop\`+newName+`"`) {
		t.Fatalf("改名后的条目应保留 workshop\\ 前缀，实际内容：\n%s", content)
	}
	if strings.Contains(content, "\t\""+newName+"\"") {
		t.Fatalf("不该把工坊条目写成裸文件名（会指向不存在的根目录文件）：\n%s", content)
	}
	if !strings.Contains(content, `"zztest_rifle_a.vpk"`) {
		t.Fatalf("其它条目被误改：\n%s", content)
	}
}

// 同一类：工坊文件「隐藏」（加 _ 前缀）后，键也必须仍是 workshop\ 下的键。
func TestHideWorkshopVPKKeepsWorkshopPrefixInAddonList(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldName := "555000222.vpk"
	oldPath := filepath.Join(workshopDir, oldName)
	if err := os.WriteFile(oldPath, []byte("vpk"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: `workshop\` + oldName, Value: "1"},
	})

	app := &App{rootDir: addonsDir, configDir: filepath.Join(gameDir, "config")}
	app.vpkCache.Store(oldPath, &VPKFileCache{File: VPKFile{
		Path: oldPath, Name: oldName, Location: "workshop", Enabled: true, WorkshopID: "555000222",
	}})
	newPath, err := app.ToggleVPKVisibility(oldPath)
	if err != nil {
		t.Fatalf("隐藏失败: %v", err)
	}
	if filepath.Base(newPath) != "_"+oldName {
		t.Fatalf("隐藏后的文件名不对: %s", newPath)
	}

	raw, err := os.ReadFile(filepath.Join(gameDir, "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, `"workshop\_`+oldName+`"`) {
		t.Fatalf("隐藏后的条目应保留 workshop\\ 前缀，实际内容：\n%s", content)
	}
	if strings.Contains(content, "\t\"_"+oldName+"\"") {
		t.Fatalf("不该把工坊条目写成裸文件名：\n%s", content)
	}
}

// 真机联调里发现的另一处不一致：RenameVPKFile 会顺手更新扫描缓存，
// ToggleVPKVisibility 不会 —— 隐藏成功后立刻读 GetVPKFiles() 还是旧路径/旧名字，
// 只有等到下一次全量重扫才对。补上缓存同步，保证两个改名入口行为一致。
func TestToggleVPKVisibilityUpdatesScanCache(t *testing.T) {
	gameDir := filepath.Join(t.TempDir(), "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const name = "zztest_hide_me.vpk"
	path := filepath.Join(addonsDir, name)
	if err := os.WriteFile(path, []byte("vpk"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{{Name: name, Value: "1"}})

	app := &App{rootDir: addonsDir, configDir: filepath.Join(gameDir, "config")}
	app.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
		Path: path, Name: name, Location: "root", Enabled: true,
	}})

	newPath, err := app.ToggleVPKVisibility(path)
	if err != nil {
		t.Fatalf("隐藏失败: %v", err)
	}
	if _, ok := app.vpkCache.Load(path); ok {
		t.Fatalf("隐藏后旧路径 %s 还留在扫描缓存里", path)
	}
	cached, ok := app.vpkCache.Load(newPath)
	if !ok {
		t.Fatalf("隐藏后的新路径 %s 没有进扫描缓存", newPath)
	}
	file := cached.(*VPKFileCache).File
	if file.Name != filepath.Base(newPath) || file.Path != newPath {
		t.Fatalf("缓存里的文件名/路径没更新: %#v", file)
	}
}
