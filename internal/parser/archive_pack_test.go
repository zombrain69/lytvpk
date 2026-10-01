package parser

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 「扩展名是 .vpk、实际是压缩包」这类条目的分类。
// 真实样本（addons\workshop\）：3558049615.vpk = Left4Neko 工具包；
// 3787239937.vpk = 带 dll/bat 的插件包（SKILL.md 是它的说明）。两者都该判成工具/插件包。

func writeZipVPK(t *testing.T, filePath string, entries []string) {
	t.Helper()
	handle, err := os.Create(filePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(handle)
	for _, name := range entries {
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

func TestDescribeArchivePackClassifiesToolkit(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "3558049615.vpk")
	writeZipVPK(t, filePath, []string{
		"bin/left4neko.dll",
		"bin/neko/build_sound_cache.bat",
		"bin/neko/plugins/l4n_plugin.h",
		"bin/neko/survivor_convert.bat",
		"readme.txt",
	})

	info, ok := DescribeArchivePack(filePath)
	if !ok {
		t.Fatal("zip 伪装的 .vpk 应该被识别为压缩包")
	}
	if info.Kind != ArchivePackToolkit {
		t.Fatalf("应判成工具/插件包，实际 %q", info.Kind)
	}
	if info.Format != "zip" || info.EntryCount != 5 {
		t.Fatalf("格式/条目数不对: %q / %d", info.Format, info.EntryCount)
	}
	if len(info.Evidence) == 0 || !strings.Contains(strings.Join(info.Evidence, " "), "left4neko.dll") {
		t.Fatalf("判定依据应点出 dll: %v", info.Evidence)
	}
	if !strings.Contains(info.Note, "不参与 addonlist.txt") {
		t.Fatalf("说明里要写清这类包不参与 addonlist: %q", info.Note)
	}
}

func TestDescribeArchivePackClassifiesModBundle(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "collection.vpk")
	writeZipVPK(t, filePath, []string{"addons/aaa.vpk", "addons/bbb.vpk", "说明.txt"})

	info, ok := DescribeArchivePack(filePath)
	if !ok || info.Kind != ArchivePackModBundle {
		t.Fatalf("装着 .vpk 的包应判成 Mod 压缩包，实际 ok=%v kind=%q", ok, info.Kind)
	}
	if !strings.Contains(info.Note, "放进 addons") {
		t.Fatalf("Mod 压缩包的说明要写清解压后放回 addons: %q", info.Note)
	}
}

func TestDescribeArchivePackClassifiesDocs(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "tutorial.vpk")
	writeZipVPK(t, filePath, []string{
		"DIY教程/SKILL.md",
		"DIY教程/references/schema-and-layout.md",
		"DIY教程/素材/预览.png",
	})

	info, ok := DescribeArchivePack(filePath)
	if !ok || info.Kind != ArchivePackDocs {
		t.Fatalf("只有文档/素材应判成教程/素材包，实际 ok=%v kind=%q", ok, info.Kind)
	}
}

func TestDescribeArchivePackReportsNonZipFormat(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "pack.vpk")
	// 7z 魔数：只报格式，不做文件树分类（rar/7z 的条目列表在 app 包里，parser 不能反依赖）。
	if err := os.WriteFile(filePath, []byte("7z\xbc\xaf\x27\x1c\x00\x04payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, ok := DescribeArchivePack(filePath)
	if !ok || info.Format != "7z" || info.Kind != ArchivePackOther {
		t.Fatalf("7z 应只报格式，实际 ok=%v info=%+v", ok, info)
	}
}

func TestDescribeArchivePackIgnoresRealVPK(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "real.vpk")
	// VPK 魔数 0x55aa1234（小端）——不是压缩包，调用方要走原来的错误路径。
	if err := os.WriteFile(filePath, []byte{0x34, 0x12, 0xaa, 0x55, 0x00, 0x00, 0x00, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := DescribeArchivePack(filePath); ok {
		t.Fatal("真 VPK 不该被当成压缩包")
	}
}

func TestDescribeArchivePackHandlesBrokenZip(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "broken.vpk")
	// 有 zip 魔数但目录损坏：仍要报"这是压缩包"，只是列不出条目、不给类别。
	if err := os.WriteFile(filePath, []byte{'P', 'K', 0x03, 0x04, 0x00, 0x00, 0x00, 0x00, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}
	info, ok := DescribeArchivePack(filePath)
	if !ok {
		t.Fatal("坏 zip 也应被识别为压缩包（不能当成真 VPK）")
	}
	if info.Kind != ArchivePackOther || !strings.Contains(info.Note, "无法列出") {
		t.Fatalf("坏 zip 应说明列不出内容: %+v", info)
	}
}
