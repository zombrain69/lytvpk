package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

// packTestVPK 用真实打包通道造一个小 VPK：dir 下放 <entryName> 内容，输出到 outputDir。
func packTestVPK(t *testing.T, app *App, dir string, entryName string, content string, outputBase string) string {
	t.Helper()
	target := filepath.Join(dir, filepath.FromSlash(entryName))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	outputDir, err := os.MkdirTemp("", "lytvpk-merge-out-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(outputDir) })
	result, err := app.packVPKDirectoryWithOptions(dir, outputDir, false, outputBase, nil)
	if err != nil {
		t.Fatalf("打包测试 VPK 失败: %v", err)
	}
	return result.OutputPath
}

func readVPKEntry(t *testing.T, vpkPath string, entryName string) (string, bool) {
	t.Helper()
	opener := vpk.Single(vpkPath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		t.Fatalf("读取 VPK 失败: %v", err)
	}
	for i := range archive.Files {
		file := &archive.Files[i]
		if !strings.EqualFold(filepath.ToSlash(file.Name()), filepath.ToSlash(entryName)) {
			continue
		}
		reader, err := file.Open(opener)
		if err != nil {
			t.Fatalf("打开条目失败: %v", err)
		}
		defer reader.Close()
		buffer := make([]byte, file.Size())
		if _, err := reader.Read(buffer); err != nil && err.Error() != "EOF" {
			t.Fatalf("读取条目失败: %v", err)
		}
		return string(buffer), true
	}
	return "", false
}

// 合并：两个 VPK 的条目都要在结果里；同名条目按"后一个覆盖前一个"。
func TestMergeVPKFilesOverlaysLaterSource(t *testing.T) {
	app := &App{}
	workA := t.TempDir()
	workB := t.TempDir()
	first := packTestVPK(t, app, filepath.Join(workA, "src"), "materials/a.vmt", "from-first", "first")
	// 第二个包：同名条目（内容不同）+ 一个独有的条目，一次打包进去。
	secondSrc := filepath.Join(workB, "src")
	if err := os.MkdirAll(filepath.Join(secondSrc, "materials"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondSrc, "materials", "a.vmt"), []byte("from-second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(secondSrc, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondSrc, "scripts", "b.nut"), []byte("nut"), 0o644); err != nil {
		t.Fatal(err)
	}
	secondOut, err := os.MkdirTemp("", "lytvpk-merge-second-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(secondOut) })
	second, err := app.packVPKDirectoryWithOptions(secondSrc, secondOut, false, "second", nil)
	if err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "merged.vpk")
	result, err := app.MergeVPKFiles([]string{first, second.OutputPath}, output)
	if err != nil {
		t.Fatalf("合并失败: %v", err)
	}
	if result.SourceCount != 2 || result.OutputPath != output {
		t.Fatalf("合并结果元信息不对: %#v", result)
	}
	if value, ok := readVPKEntry(t, output, "materials/a.vmt"); !ok || value != "from-second" {
		t.Fatalf("同名条目应被后一个包覆盖，实际 %q (ok=%v)", value, ok)
	}
	if _, ok := readVPKEntry(t, output, "scripts/b.nut"); !ok {
		t.Fatal("第二个包独有的条目应该出现在合并结果里")
	}
}

func TestMergeVPKFilesRejectsTooFewSources(t *testing.T) {
	app := &App{}
	single := packTestVPK(t, app, filepath.Join(t.TempDir(), "src"), "a.txt", "a", "single")
	if _, err := app.MergeVPKFiles([]string{single}, filepath.Join(t.TempDir(), "out.vpk")); err == nil {
		t.Fatal("只有一个来源时必须报错")
	}
	if _, err := app.MergeVPKFiles(nil, ""); err == nil {
		t.Fatal("缺少输出路径时必须报错")
	}
}

func TestDedupeVPKMergeSourcesKeepsOrder(t *testing.T) {
	got := DedupeVPKMergeSources([]string{" b.vpk ", "a.vpk", "A.VPK", ""})
	want := []string{"b.vpk", "a.vpk"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("去重结果 = %#v，期望 %#v", got, want)
	}
}
