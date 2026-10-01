package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"vpk-manager/internal/parser"
)

// formatVPKFileForCacheCompare 把解析结果序列化成可比较的字符串（时间戳之外的字段都在内）。
func formatVPKFileForCacheCompare(file parser.VPKFile) string {
	raw, err := json.Marshal(file)
	if err != nil {
		return ""
	}
	return string(raw)
}

// newScanCacheTestApp 造一个"和 newPriorityTestApp 同源、但每次都是新进程语义"的 App：
// rootDir/configDir 一致、内存缓存为空 —— 冷启动就是这个状态。
func newScanCacheTestApp(t *testing.T) (*App, string) {
	t.Helper()
	base := t.TempDir()
	gameDir := filepath.Join(base, "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
		{Name: `workshop\123.vpk`, Value: "1"},
	})
	writeTestVPK(t, filepath.Join(addonsDir, "a.vpk"), map[string][]byte{
		"materials/shared.vtf":                     []byte("a"),
		"models/w_models/weapons/w_rifle_ak47.mdl": []byte("ak"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "b.vpk"), map[string][]byte{
		"materials/shared.vtf": []byte("b"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "workshop", "123.vpk"), map[string][]byte{
		"materials/decided.vtf": []byte("w"),
	})
	configDir := filepath.Join(base, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &App{rootDir: addonsDir, configDir: configDir}, addonsDir
}

func appWithSameStores(t *testing.T, source *App) *App {
	t.Helper()
	return &App{rootDir: source.rootDir, configDir: source.configDir}
}

func cacheFileFingerprint(t *testing.T, app *App) map[string]string {
	t.Helper()
	out := make(map[string]string)
	app.vpkCache.Range(func(_ any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		// 只比较"解析出来的内容"（时间戳存缓存里、不参与比较）。
		out[cache.File.Path] = formatVPKFileForCacheCompare(cache.File)
		return true
	})
	return out
}

// 冷启动命中扫描缓存：不重新解析，而且解析结果与真正解析一遍完全一致。
func TestScanCacheSkipsReparseAfterRestart(t *testing.T) {
	first, addonsDir := newScanCacheTestApp(t)
	if err := first.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描失败: %v", err)
	}
	firstStats := first.GetScanStats()
	if firstStats.Reparsed != firstStats.Total {
		t.Fatalf("首次扫描应当全部重新解析：%#v", firstStats)
	}
	if _, err := os.Stat(filepath.Join(first.configDir, vpkScanCacheFileName)); err != nil {
		t.Fatalf("首次扫描后应写出缓存文件: %v", err)
	}
	parsedFingerprint := cacheFileFingerprint(t, first)

	// 新进程：内存缓存为空，只有磁盘缓存。
	second := appWithSameStores(t, first)
	if err := second.ScanVPKFiles(); err != nil {
		t.Fatalf("二次扫描失败: %v", err)
	}
	secondStats := second.GetScanStats()
	if !secondStats.CacheLoaded {
		t.Fatalf("第二次应当载入磁盘缓存：%#v", secondStats)
	}
	if secondStats.Reparsed != 0 {
		t.Fatalf("缓存命中时不应重新解析：%#v", secondStats)
	}
	if secondStats.Unchanged != secondStats.Total {
		t.Fatalf("全部条目都应命中缓存：%#v", secondStats)
	}
	if got := cacheFileFingerprint(t, second); !reflect.DeepEqual(got, parsedFingerprint) {
		t.Fatalf("缓存命中的解析结果与真正解析不一致\n缓存命中: %#v\n真正解析: %#v", got, parsedFingerprint)
	}
	// 同目录下第二次扫描也要能看到同一批文件（含 workshop 子目录）。
	if len(parsedFingerprint) != 3 {
		t.Fatalf("夹具应有 3 个 Mod，实际 %d", len(parsedFingerprint))
	}
	_ = addonsDir
}

// 逐个失效通道：改动 VPK、改动同名封面图，都必须让那一个文件重新解析（其它仍命中）。
func TestScanCacheInvalidatesChangedFileAndSidecar(t *testing.T) {
	first, addonsDir := newScanCacheTestApp(t)
	if err := first.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描失败: %v", err)
	}

	// ① 改 VPK 的 mtime
	vpkPath := filepath.Join(addonsDir, "a.vpk")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(vpkPath, later, later); err != nil {
		t.Fatal(err)
	}
	second := appWithSameStores(t, first)
	if err := second.ScanVPKFiles(); err != nil {
		t.Fatalf("二次扫描失败: %v", err)
	}
	stats := second.GetScanStats()
	if stats.Reparsed != 1 || stats.Unchanged != stats.Total-1 {
		t.Fatalf("只应有 1 个文件重新解析：%#v", stats)
	}

	// ② 改同名封面图的 mtime（.jpg 与 VPK 同名）
	imagePath := filepath.Join(addonsDir, "b.jpg")
	if err := os.WriteFile(imagePath, []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	imageTime := later.Add(5 * time.Second)
	if err := os.Chtimes(imagePath, imageTime, imageTime); err != nil {
		t.Fatal(err)
	}
	third := appWithSameStores(t, second)
	if err := third.ScanVPKFiles(); err != nil {
		t.Fatalf("三次扫描失败: %v", err)
	}
	stats = third.GetScanStats()
	if stats.Reparsed != 1 {
		t.Fatalf("新增封面图只应让 b.vpk 重新解析：%#v", stats)
	}
	// 清掉封面图后，对应文件也必须重新解析（回到"没有封面"的状态）。
	if err := os.Remove(imagePath); err != nil {
		t.Fatal(err)
	}
	fourth := appWithSameStores(t, third)
	if err := fourth.ScanVPKFiles(); err != nil {
		t.Fatalf("四次扫描失败: %v", err)
	}
	if stats := fourth.GetScanStats(); stats.Reparsed != 1 {
		t.Fatalf("删掉封面图只应让 b.vpk 重新解析：%#v", stats)
	}
}

// 坏缓存 / 版本不符都要退回全量解析，且不报错。
func TestScanCacheDegradesSafely(t *testing.T) {
	first, _ := newScanCacheTestApp(t)
	if err := first.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(first.configDir, vpkScanCacheFileName)

	// ① 损坏的 JSON
	if err := os.WriteFile(cachePath, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := appWithSameStores(t, first)
	if err := broken.ScanVPKFiles(); err != nil {
		t.Fatalf("坏缓存不应让扫描失败: %v", err)
	}
	if stats := broken.GetScanStats(); stats.CacheLoaded || stats.Reparsed != stats.Total {
		t.Fatalf("坏缓存应退回全量解析：%#v", stats)
	}

	// ② 版本不符（模拟换了一个程序版本）
	if err := os.WriteFile(cachePath, []byte(`{"schemaVersion":1,"appVersion":"0.0.0-old","entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	foreign := appWithSameStores(t, first)
	if err := foreign.ScanVPKFiles(); err != nil {
		t.Fatalf("版本不符不应让扫描失败: %v", err)
	}
	if stats := foreign.GetScanStats(); stats.CacheLoaded || stats.Reparsed != stats.Total {
		t.Fatalf("版本不符应退回全量解析：%#v", stats)
	}

	// ③ 格式版本不符
	if err := os.WriteFile(cachePath, []byte(`{"schemaVersion":999,"appVersion":"`+AppVersion+`","entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	wrongSchema := appWithSameStores(t, first)
	if err := wrongSchema.ScanVPKFiles(); err != nil {
		t.Fatalf("格式版本不符不应让扫描失败: %v", err)
	}
	if stats := wrongSchema.GetScanStats(); stats.CacheLoaded {
		t.Fatalf("格式版本不符应忽略缓存：%#v", stats)
	}
}

// 文件被删掉后：内存缓存与磁盘缓存都不能再留着它（否则会"复活"已删的 Mod）。
func TestScanCacheDropsDeletedFiles(t *testing.T) {
	first, addonsDir := newScanCacheTestApp(t)
	if err := first.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	removedPath := filepath.Join(addonsDir, "b.vpk")
	if err := os.Remove(removedPath); err != nil {
		t.Fatal(err)
	}
	second := appWithSameStores(t, first)
	if err := second.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	if _, ok := second.vpkCache.Load(removedPath); ok {
		t.Fatalf("已删除的文件不应出现在缓存里")
	}
	if fingerprint := cacheFileFingerprint(t, second); len(fingerprint) != 2 {
		t.Fatalf("删掉一个文件后应只剩 2 个：%#v", fingerprint)
	}
	// 落盘的那份也要同步（第二次扫描有重新解析 → 会重写缓存；等写完后校验）
	third := appWithSameStores(t, second)
	if err := third.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	if _, ok := third.vpkCache.Load(removedPath); ok {
		t.Fatalf("磁盘缓存里也不应留下已删除的文件")
	}
}

// 用户最常见的用法：**开着程序时**往 addons 里丢新 Mod / 删掉旧 Mod。
// 缓存必须一条条对账：新文件老实解析，旧文件立刻消失，没动过的继续命中。
func TestScanCacheFollowsLiveAddAndRemoveWhileRunning(t *testing.T) {
	a, addonsDir := newScanCacheTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	// 记下老文件的解析结果，最后用来确认"没动过的没有被缓存改写"。
	beforeA := a.mustCachedVPKFile(t, filepath.Join(addonsDir, "a.vpk"))

	// ① 运行中新增一个 Mod（模拟用户直接往 addons 里拷文件）
	newPath := filepath.Join(addonsDir, "c.vpk")
	writeTestVPK(t, newPath, map[string][]byte{
		"materials/new.vtf": []byte("c"),
	})
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	stats := a.GetScanStats()
	if stats.Reparsed != 1 || stats.Unchanged != stats.Total-1 {
		t.Fatalf("新增 1 个文件时应只解析它：%#v", stats)
	}
	if _, ok := a.vpkCache.Load(newPath); !ok {
		t.Fatalf("新增的文件必须进缓存")
	}

	// ② 运行中删除刚才那个 Mod
	if err := os.Remove(newPath); err != nil {
		t.Fatal(err)
	}
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.vpkCache.Load(newPath); ok {
		t.Fatalf("删除后不应再留在缓存里")
	}

	// ③ 没动过的 a.vpk：解析结果与第一次完全一致（没被缓存"改写"）
	afterA := a.mustCachedVPKFile(t, filepath.Join(addonsDir, "a.vpk"))
	if formatVPKFileForCacheCompare(beforeA) != formatVPKFileForCacheCompare(afterA) {
		t.Fatalf("未改动的文件解析结果不应变化\n前: %s\n后: %s",
			formatVPKFileForCacheCompare(beforeA), formatVPKFileForCacheCompare(afterA))
	}
}

// 开着程序时改 addonlist.txt：命中扫描缓存的那条也必须拿到**当前**的游戏内开关
// （缓存里存的是解析结果，开关来自 addonlist，每次扫描都要重新套用）。
func TestScanCacheStillAppliesFreshAddonListState(t *testing.T) {
	first, _ := newScanCacheTestApp(t)
	if err := first.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}

	// 外部把 a.vpk 改成 0（关闭）
	gameDir := filepath.Dir(first.rootDir)
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "0"},
		{Name: "b.vpk", Value: "1"},
		{Name: `workshop\123.vpk`, Value: "1"},
	})

	second := appWithSameStores(t, first)
	if err := second.ScanVPKFiles(); err != nil {
		t.Fatalf("二次扫描失败: %v", err)
	}
	if stats := second.GetScanStats(); stats.CacheLoaded {
		// 缓存命中是预期路径：这时开关必须来自新读的 addonlist。
		file := second.mustCachedVPKFile(t, filepath.Join(second.rootDir, "a.vpk"))
		if !file.GameStateKnown || file.GameEnabled {
			t.Fatalf("命中缓存时也必须套用最新的 addonlist 状态：%#v", file)
		}
	}
}

// 不同库（用户切目录）之间不能互相冲掉缓存。
func TestScanCacheKeepsOtherRoots(t *testing.T) {
	a, addonsDir := newScanCacheTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	// 换到第二个库（同一份配置目录）
	secondRoot := filepath.Join(filepath.Dir(addonsDir), "addons2")
	if err := os.MkdirAll(secondRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestVPK(t, filepath.Join(secondRoot, "x.vpk"), map[string][]byte{"materials/x.vtf": []byte("x")})
	second := &App{rootDir: secondRoot, configDir: a.configDir}
	if err := second.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}

	// 回到第一个库：应当仍是全命中（其它库的条目没有被冲掉）
	back := appWithSameStores(t, a)
	if err := back.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	stats := back.GetScanStats()
	if !stats.CacheLoaded || stats.Reparsed != 0 {
		t.Fatalf("切回原来的库应全命中缓存：%#v", stats)
	}
}
