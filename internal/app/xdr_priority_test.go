package app

import (
	"path/filepath"
	"strings"
	"testing"

	"vpk-manager/internal/parser"
)

func seedXDRPriorityCache(a *App, files ...VPKFile) {
	for _, file := range files {
		a.vpkCache.Store(file.Path, &VPKFileCache{File: file})
	}
}

func xdrPriorityFile(rootDir string, relPath string, title string, size int64, slots ...parser.XDRSlotInfo) VPKFile {
	file := xdrTestFile(rootDir, relPath, title, size, slots...)
	file.XDRSummary = "XDR：测试"
	return file
}

func TestXDRPriorityMarksUniqueSlotAsActive(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	seedXDRPriorityCache(app,
		xdrPriorityFile(root, "only.vpk", "唯一动作", 100, xdrTestSlot("Zoey", 31)),
	)

	index := app.xdrPriorityIndex()
	info, ok := index[filepath.Join(root, "only.vpk")]
	if !ok {
		t.Fatalf("唯一槽位的 Mod 应该有结论：%#v", index)
	}
	if info.State != "active" || info.ActiveSlots != 1 || info.RandomSlots != 0 {
		t.Fatalf("唯一槽位应判为生效：%#v", info)
	}
	if len(info.Slots) != 1 || info.Slots[0].State != "active" {
		t.Fatalf("槽位状态应带 active：%#v", info.Slots)
	}
}

func TestXDRPriorityMarksSameSlotAsRandomWithRivals(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	seedXDRPriorityCache(app,
		xdrPriorityFile(root, "a.vpk", "动作 A", 100, xdrTestSlot("Zoey", 31)),
		xdrPriorityFile(root, "b.vpk", "动作 B", 200, xdrTestSlot("Zoey", 31)),
	)

	index := app.xdrPriorityIndex()
	for _, name := range []string{"a.vpk", "b.vpk"} {
		info, ok := index[filepath.Join(root, name)]
		if !ok {
			t.Fatalf("%s 应该被标出来：%#v", name, index)
		}
		if info.State != "random" || info.RandomSlots != 1 || info.ActiveSlots != 0 {
			t.Fatalf("%s 同槽应判为随机：%#v", name, info)
		}
		if len(info.Slots[0].Rivals) != 1 {
			t.Fatalf("%s 应列出一个同槽对手：%#v", name, info.Slots[0])
		}
		rival := info.Slots[0].Rivals[0].Name
		want := "b.vpk"
		if name == "b.vpk" {
			want = "a.vpk"
		}
		if rival != want {
			t.Fatalf("%s 的对手应为 %s，实际 %s", name, want, rival)
		}
	}
}

// 同一份 Mod 在根目录与 workshop 各一份（同名同大小）是同一件事，不算同槽冲突。
func TestXDRPriorityTreatsRootWorkshopCopyAsOne(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	copyInRoot := xdrPriorityFile(root, "same.vpk", "同一份", 4096, xdrTestSlot("Jockey", 43))
	copyInWorkshop := xdrPriorityFile(root, filepath.Join("workshop", "same.vpk"), "同一份", 4096, xdrTestSlot("Jockey", 43))
	seedXDRPriorityCache(app, copyInRoot, copyInWorkshop)

	index := app.xdrPriorityIndex()
	for _, path := range []string{copyInRoot.Path, copyInWorkshop.Path} {
		info, ok := index[path]
		if !ok {
			t.Fatalf("副本 %s 应该有结论", path)
		}
		if info.State != "active" {
			t.Fatalf("同一份副本不该互相冲突：%s => %#v", path, info)
		}
	}
}

// 游戏内关闭（addonlist=0）与 disabled 目录里的 Mod 都不参与播放，不能算对手，也不给结论。
func TestXDRPriorityIgnoresInactiveMods(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	gameDisabled := xdrPriorityFile(root, "off.vpk", "游戏内关闭", 100, xdrTestSlot("Zoey", 31))
	gameDisabled.GameStateKnown = true
	gameDisabled.GameEnabled = false
	disabledDir := xdrPriorityFile(root, filepath.Join("disabled", "old.vpk"), "已禁用", 200, xdrTestSlot("Zoey", 31))
	active := xdrPriorityFile(root, "on.vpk", "在用", 300, xdrTestSlot("Zoey", 31))
	seedXDRPriorityCache(app, gameDisabled, disabledDir, active)

	index := app.xdrPriorityIndex()
	info := index[active.Path]
	if info.State != "active" {
		t.Fatalf("关掉的那两个不该算对手：%#v", info)
	}
	if _, ok := index[gameDisabled.Path]; ok {
		t.Fatal("游戏内关闭的 Mod 不该有'会生效'之类的结论")
	}
	if _, ok := index[disabledDir.Path]; ok {
		t.Fatal("disabled 目录里的 Mod 不该有结论")
	}
}

func TestXDRPriorityMixedSlotsBecomePartial(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	seedXDRPriorityCache(app,
		xdrPriorityFile(root, "multi.vpk", "多角色动作", 100,
			xdrTestSlot("Zoey", 31), xdrTestSlot("Coach", 31)),
		xdrPriorityFile(root, "rival.vpk", "只抢 Zoey", 200, xdrTestSlot("Zoey", 31)),
	)

	info := app.xdrPriorityIndex()[filepath.Join(root, "multi.vpk")]
	if info.State != "partial" {
		t.Fatalf("部分槽位冲突应判为 partial：%#v", info)
	}
	if info.ActiveSlots != 1 || info.RandomSlots != 1 {
		t.Fatalf("应有 1 个生效槽 + 1 个随机槽：%#v", info)
	}
	randomSlot := info.Slots[1]
	if randomSlot.Character != "Zoey" || randomSlot.State != "random" {
		t.Fatalf("Zoey 槽应为随机：%#v", info.Slots)
	}
	if len(randomSlot.Rivals) != 1 || randomSlot.Rivals[0].Name != "rival.vpk" {
		t.Fatalf("应列出对手：%#v", randomSlot)
	}
}

// GetVPKFiles 要把结论带到列表里（前端据此渲染角标）。
func TestGetVPKFilesAttachesXDRPriority(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	seedXDRPriorityCache(app,
		xdrPriorityFile(root, "a.vpk", "动作 A", 100, xdrTestSlot("Zoey", 31)),
		xdrPriorityFile(root, "b.vpk", "动作 B", 200, xdrTestSlot("Zoey", 31)),
	)

	files := app.GetVPKFiles()
	found := false
	for _, file := range files {
		if strings.HasSuffix(file.Path, "a.vpk") {
			found = true
			if file.XDRPriority == nil || file.XDRPriority.State != "random" {
				t.Fatalf("列表里的 XDR 结论缺失：%#v", file.XDRPriority)
			}
		}
	}
	if !found {
		t.Fatal("列表里没有 a.vpk")
	}
}

// 带筛选时走的是 SearchVPKFiles：对手来自全量列表，筛选结果同样要带结论。
func TestSearchVPKFilesAttachesXDRPriority(t *testing.T) {
	root := t.TempDir()
	app := newProfileTestApp(t, root)
	seedXDRPriorityCache(app,
		xdrPriorityFile(root, "a.vpk", "动作 A", 100, xdrTestSlot("Zoey", 31)),
		xdrPriorityFile(root, "b.vpk", "动作 B", 200, xdrTestSlot("Zoey", 31)),
	)

	results := app.SearchVPKFiles("a.vpk", "", nil)
	if len(results) == 0 {
		t.Fatalf("搜索没有结果：%#v", results)
	}
	if results[0].XDRPriority == nil || results[0].XDRPriority.State != "random" {
		t.Fatalf("筛选结果里的 XDR 结论缺失：%#v", results[0].XDRPriority)
	}
	if len(results[0].XDRPriority.Slots[0].Rivals) != 1 {
		t.Fatalf("筛选结果也要带同槽对手：%#v", results[0].XDRPriority.Slots)
	}
}
