package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// writeUpdateZip 造一个只含统一 EXE 的更新包。
func writeUpdateZip(t *testing.T, zipPath string, payload []byte) {
	t.Helper()
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("创建 zip 失败: %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	entry, err := writer.Create(CommunityForkExecutableName)
	if err != nil {
		t.Fatalf("写 zip 条目失败: %v", err)
	}
	if _, err := entry.Write(payload); err != nil {
		t.Fatalf("写 zip 内容失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 zip 失败: %v", err)
	}
}

// TestInstallUpdateReplacesCurrentExe 常规路径：当前 EXE 备份成 .old，新 EXE 就位。
func TestInstallUpdateReplacesCurrentExe(t *testing.T) {
	dir := t.TempDir()
	currentExe := filepath.Join(dir, "LytVPK-Test.exe")
	if err := os.WriteFile(currentExe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "update.zip")
	writeUpdateZip(t, zipPath, []byte("new-binary"))

	if err := installUpdate(zipPath, currentExe); err != nil {
		t.Fatalf("installUpdate 失败: %v", err)
	}
	if got, _ := os.ReadFile(currentExe); string(got) != "new-binary" {
		t.Fatalf("当前 EXE 未替换: %q", got)
	}
	if got, _ := os.ReadFile(currentExe + ".old"); string(got) != "old-binary" {
		t.Fatalf(".old 备份内容不对: %q", got)
	}
}

// TestInstallUpdateFallsBackWhenOldBackupLocked 旧的 .old 删不掉时改用时间戳备份名，
// 覆盖「上次更新后没重启（.old 是运行中的自身）/ 被占用」导致 Access is denied 的场景。
func TestInstallUpdateFallsBackWhenOldBackupLocked(t *testing.T) {
	dir := t.TempDir()
	currentExe := filepath.Join(dir, "LytVPK-Test.exe")
	if err := os.WriteFile(currentExe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 用一个非空目录占住 .old：os.Remove 无法删除非空目录，模拟「删不掉旧备份」。
	if err := os.Mkdir(currentExe+".old", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(currentExe+".old", "locked.bin"), []byte("locked"), 0o644); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "update.zip")
	writeUpdateZip(t, zipPath, []byte("new-binary"))

	if err := installUpdate(zipPath, currentExe); err != nil {
		t.Fatalf("旧备份删不掉时应改用时间戳备份名而不是报错: %v", err)
	}
	if got, _ := os.ReadFile(currentExe); string(got) != "new-binary" {
		t.Fatalf("当前 EXE 未替换: %q", got)
	}
	matches, err := filepath.Glob(currentExe + ".old-*")
	if err != nil || len(matches) == 0 {
		t.Fatalf("应留下时间戳备份: %v %v", matches, err)
	}
	if got, _ := os.ReadFile(matches[0]); string(got) != "old-binary" {
		t.Fatalf("时间戳备份内容不对: %q", got)
	}
}
