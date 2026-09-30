package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真机复现：把与 addons 里同名的 VPK 拖进应用，原文件被静默覆盖
// （767 → 1848 字节），提示却只有「VPK 安装完成」。
// 导入外来文件不该动用户已有的 Mod：同名要另存为 name(1).vpk 并说明。

func TestUniqueDropImportTargetAvoidsOverwrite(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "map.vpk")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := uniqueDropImportTarget(dir, "map.vpk")
	if first != filepath.Join(dir, "map(1).vpk") {
		t.Fatalf("同名应让位到 map(1).vpk，实际 %s", first)
	}
	if err := os.WriteFile(first, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := uniqueDropImportTarget(dir, "map.vpk")
	if second != filepath.Join(dir, "map(2).vpk") {
		t.Fatalf("再冲突应继续递增，实际 %s", second)
	}

	// 原始文件必须原封不动。
	raw, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "old" {
		t.Fatalf("已有文件被改动了：%q", string(raw))
	}
}

func TestInstallVPKFileKeepsExistingModOnNameCollision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "addons")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(root, "map.vpk")
	if err := os.WriteFile(existing, []byte("old-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	stage := filepath.Join(t.TempDir(), "map.vpk")
	if err := os.WriteFile(stage, []byte("new-content-longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: root}
	written, err := app.installVPKFile(stage, nil)
	if err != nil {
		t.Fatalf("安装失败: %v", err)
	}
	if filepath.Base(written) != "map(1).vpk" {
		t.Fatalf("同名应另存为 map(1).vpk，实际 %s", filepath.Base(written))
	}
	oldRaw, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(oldRaw) != "old-content" {
		t.Fatalf("已有 Mod 被覆盖了：%q", string(oldRaw))
	}
	newRaw, err := os.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	if string(newRaw) != "new-content-longer" {
		t.Fatalf("新文件内容不对：%q", string(newRaw))
	}
}

func TestExtractReaderEntryKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "zz.vpk")
	if err := os.WriteFile(existing, []byte("keep-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	written, err := extractReaderEntryWithProgress(strings.NewReader("incoming"), "zz.vpk", dir, nil)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if filepath.Base(written) != "zz(1).vpk" {
		t.Fatalf("同名应另存为 zz(1).vpk，实际 %s", filepath.Base(written))
	}
	raw, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "keep-me" {
		t.Fatalf("已有文件被覆盖了：%q", string(raw))
	}
}

// 端到端：拖入一个与已有 Mod 同名的 zip 内 VPK，原文件保留、副本另存。
func TestHandleFileDropArchiveKeepsExistingMod(t *testing.T) {
	root := filepath.Join(t.TempDir(), "addons")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(root, "bundle.vpk")
	if err := os.WriteFile(existing, []byte("existing-mod"), 0o644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "bundle.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("bundle.vpk")
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

	app := &App{rootDir: root}
	result, err := app.HandleFileDrop([]string{zipPath})
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("导入应成功：%+v", result.Items)
	}

	raw, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "existing-mod" {
		t.Fatalf("已有 Mod 被覆盖了：%q", string(raw))
	}
	if _, err := os.Stat(filepath.Join(root, "bundle(1).vpk")); err != nil {
		t.Fatalf("应另有副本 bundle(1).vpk：%v", err)
	}
}
