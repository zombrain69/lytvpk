package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/panjf2000/ants/v2"
	"vpk-manager/internal/parser"
)

// 「扩展名是 .vpk、实际是压缩包」在扫描路径上的行为：
// 记进 archivePacks（带类别），**不**记进 unreadableMods（那不是异常）。
func TestScanRecordsArchivePackInsteadOfUnreadableMod(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "addons")
	workshopDir := filepath.Join(rootDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 一个真 VPK（用打包器生成，保证能被扫描识别）。
	sourceDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "addon.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&App{}).PackVPKDirectory(sourceDir, rootDir, false); err != nil {
		t.Fatalf("打包测试 VPK 失败: %v", err)
	}

	// 一个 zip 伪装成 .vpk（模仿真机 3558049615.vpk：bin/ + dll + bat）。
	fakePath := filepath.Join(workshopDir, "3558049615.vpk")
	writeToolkitZip(t, fakePath)

	pool, err := ants.NewPool(2)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Release()

	app := &App{rootDir: rootDir, goroutinePool: pool}
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}

	value, ok := app.archivePacks.Load(fakePath)
	if !ok {
		t.Fatal("zip 伪装的 .vpk 应记进 archivePacks")
	}
	pack, ok := value.(parser.ArchivePackInfo)
	if !ok || pack.Kind != parser.ArchivePackToolkit {
		t.Fatalf("archivePacks 里应带类别：%+v", value)
	}
	if _, stillUnreadable := app.unreadableMods.Load(fakePath); stillUnreadable {
		t.Fatal("压缩包类条目不该再进 unreadableMods（它不是异常）")
	}

	// 但**要出现在列表里**：真机上这类包占着 addonlist 的一行（= 占了优先级位置），
	// 用户需要能看到它、能把它关掉。列表条目带 ArchivePack 标记供界面显示徽标。
	cached, ok := app.vpkCache.Load(fakePath)
	if !ok {
		t.Fatal("压缩包类条目应出现在列表（vpkCache）里")
	}
	listed, ok := cached.(*VPKFileCache)
	if !ok || listed.File.ArchivePack == nil {
		t.Fatalf("列表条目要带 ArchivePack 标记：%+v", cached)
	}
	if listed.File.Location != "workshop" || listed.File.Title != "3558049615" {
		t.Fatalf("列表条目要有位置与可读标题：location=%q title=%q",
			listed.File.Location, listed.File.Title)
	}

	// 清单导出里它**不进** mods[]（不是 Mod），只进 archivePacks[]。
	for _, mod := range app.groupingMods() {
		if mod.Name == "3558049615.vpk" {
			t.Fatal("压缩包类条目不该进分组候选/标签基线")
		}
	}

	// 清单导出里也应带上它，且写清类别与说明。
	packs := app.archivePackSnapshot()
	if len(packs) != 1 || packs[0].Name != "3558049615.vpk" {
		t.Fatalf("archivePackSnapshot 应导出这一条：%+v", packs)
	}
	if packs[0].Label == "" || packs[0].Note == "" {
		t.Fatalf("导出项要带 label 与说明：%+v", packs[0])
	}
	for _, item := range app.unreadableModSnapshot() {
		if item.Name == "3558049615.vpk" {
			t.Fatal("unreadableModSnapshot 里不该出现压缩包类条目")
		}
	}
}

func writeToolkitZip(t *testing.T, filePath string) {
	t.Helper()
	handle, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(handle)
	for _, name := range []string{"bin/left4neko.dll", "bin/neko/build_sound_cache.bat", "readme.txt"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
}
