package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// 工具箱「从压缩包提取 VPK」与拖拽导入是同一类操作：目标目录里已有同名文件时
// 不能静默覆盖（用户经常直接把 addons 当目标目录）。
func TestExtractVPKFromZipKeepsExistingMod(t *testing.T) {
	base := t.TempDir()
	destDir := filepath.Join(base, "addons")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(destDir, "map.vpk")
	if err := os.WriteFile(existing, []byte("existing-mod"), 0o644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(base, "bundle.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("map.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("incoming-mod")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	if err := app.ExtractVPKFromZip(zipPath, destDir); err != nil {
		t.Fatalf("提取失败: %v", err)
	}

	raw, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "existing-mod" {
		t.Fatalf("已有 Mod 被覆盖了：%q", string(raw))
	}
	copied, err := os.ReadFile(filepath.Join(destDir, "map(1).vpk"))
	if err != nil {
		t.Fatalf("应另存为 map(1).vpk：%v", err)
	}
	if string(copied) != "incoming-mod" {
		t.Fatalf("副本内容不对：%q", string(copied))
	}
}

func TestExtractZipFileKeepsExistingTarget(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "a.vpk")
	if err := os.WriteFile(existing, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(dir, "src.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("a.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractZipFile(reader.File[0], "a.vpk", dir); err != nil {
		t.Fatalf("解压失败: %v", err)
	}

	raw, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "keep" {
		t.Fatalf("已有文件被覆盖了：%q", string(raw))
	}
	if _, err := os.Stat(filepath.Join(dir, "a(1).vpk")); err != nil {
		t.Fatalf("应另存为 a(1).vpk：%v", err)
	}
}
