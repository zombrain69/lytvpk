package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckFileMoveConflictsIncludesSidecars(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	source := filepath.Join(sourceDir, "demo.vpk")
	sourcePreview := filepath.Join(sourceDir, "demo.png")
	target := filepath.Join(destDir, "demo.vpk")
	targetPreview := filepath.Join(destDir, "demo.png")
	for path, content := range map[string]string{
		source:        "source-vpk",
		sourcePreview: "source-preview",
		target:        "target-vpk",
		targetPreview: "target-preview",
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	conflicts, err := (&App{}).CheckFileMoveConflicts([]string{source}, destDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 2 {
		t.Fatalf("expected VPK and sidecar conflicts, got %d: %#v", len(conflicts), conflicts)
	}
	if conflicts[0].SourceSize == 0 || conflicts[0].TargetSize == 0 {
		t.Fatalf("expected file sizes in conflict details: %#v", conflicts[0])
	}
}

func TestMoveVpkFilesWithConflictActionReplaceAndSkip(t *testing.T) {
	sourceDir := t.TempDir()
	destDir := t.TempDir()
	source := filepath.Join(sourceDir, "demo.vpk")
	target := filepath.Join(destDir, "demo.vpk")
	if err := os.WriteFile(source, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := (&App{}).MoveVpkFilesWithConflictAction([]string{source}, destDir, "skip")
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedCount != 1 || result.SuccessCount != 0 {
		t.Fatalf("unexpected skip result: %#v", result)
	}
	if content, readErr := os.ReadFile(source); readErr != nil || string(content) != "new" {
		t.Fatalf("skip should retain source, content=%q err=%v", content, readErr)
	}

	result, err = (&App{}).MoveVpkFilesWithConflictAction([]string{source}, destDir, "replace")
	if err != nil {
		t.Fatal(err)
	}
	if result.SuccessCount != 1 || result.FailCount != 0 {
		t.Fatalf("unexpected replace result: %#v", result)
	}
	if content, readErr := os.ReadFile(target); readErr != nil || string(content) != "new" {
		t.Fatalf("replace should update target, content=%q err=%v", content, readErr)
	}
	if _, statErr := os.Stat(source); !os.IsNotExist(statErr) {
		t.Fatalf("replace move should remove source, stat err=%v", statErr)
	}
}
// TestNormalizeMoveConflictActionListsValidValues 防止"传错动作只得到一句不支持"：
// 界面/脚本写错时，错误信息必须能直接看出可用取值。
func TestNormalizeMoveConflictActionListsValidValues(t *testing.T) {
	if _, err := normalizeMoveConflictAction("Replace"); err != nil {
		t.Fatalf("大小写不同的合法取值应被接受: %v", err)
	}
	_, err := normalizeMoveConflictAction("overwrite")
	if err == nil {
		t.Fatal("非法动作应报错")
	}
	message := err.Error()
	for _, want := range []string{"replace", "skip", "cancel"} {
		if !strings.Contains(message, want) {
			t.Fatalf("错误信息里应列出可用取值 %q，实际: %s", want, message)
		}
	}
}
