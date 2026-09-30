package app

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// TestCreateUniqueFileAvoidsConcurrentCollision 覆盖"并行解压同名条目"的竞态：
// 原来的写法是"先 os.Stat 判断不存在 → 再 os.Create"，两个 goroutine 会同时通过检查，
// 后一个把前一个刚写好的文件截断。createUniqueFile 用 O_CREATE|O_EXCL 占名。
func TestCreateUniqueFileAvoidsConcurrentCollision(t *testing.T) {
	dir := t.TempDir()
	const workers = 16

	var wg sync.WaitGroup
	paths := make(chan string, workers)
	for index := 0; index < workers; index++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			file, path, err := createUniqueFile(dir, "same.vpk")
			if err != nil {
				t.Errorf("第 %d 个写入者没拿到文件名: %v", i, err)
				return
			}
			if _, err := file.Write([]byte(fmt.Sprintf("payload-%d", i))); err != nil {
				t.Errorf("第 %d 个写入者写失败: %v", i, err)
			}
			_ = file.Close()
			paths <- path
		}(index)
	}
	wg.Wait()
	close(paths)

	seen := make(map[string]bool, workers)
	for path := range paths {
		if seen[path] {
			t.Fatalf("两个写入者拿到了同一个目标文件：%s", path)
		}
		seen[path] = true
	}
	if len(seen) != workers {
		t.Fatalf("只写出了 %d 个文件，期望 %d", len(seen), workers)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != workers {
		t.Fatalf("目录里有 %d 个文件，期望 %d（有内容被覆盖了）", len(entries), workers)
	}
}

// TestExtractVPKFromZipHandlesDuplicateBasenames 是上面那个竞态的用户可见版本：
// 压缩包里 dir1/x.vpk 与 dir2/x.vpk 必须都解出来（x.vpk 与 x(1).vpk），不能只剩一个。
func TestExtractVPKFromZipHandlesDuplicateBasenames(t *testing.T) {
	base := t.TempDir()
	destDir := filepath.Join(base, "addons")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(base, "dupes.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, item := range []struct{ name, payload string }{
		{"modA/same.vpk", "payload-a"},
		{"modB/same.vpk", "payload-b"},
	} {
		entry, err := writer.Create(item.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(item.payload)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	if err := app.ExtractVPKFromZip(zipPath, destDir); err != nil {
		t.Fatalf("解压失败: %v", err)
	}

	payloads := map[string]bool{}
	for _, name := range []string{"same.vpk", "same(1).vpk"} {
		raw, err := os.ReadFile(filepath.Join(destDir, name))
		if err != nil {
			t.Fatalf("缺少 %s（同名条目被覆盖了）：%v", name, err)
		}
		payloads[string(raw)] = true
	}
	if !payloads["payload-a"] || !payloads["payload-b"] {
		t.Fatalf("两个同名条目的内容没有都保留下来：%#v", payloads)
	}
}
