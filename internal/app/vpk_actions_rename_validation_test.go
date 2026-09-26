package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 对齐 FireAxe `FileSystemUtils.SanitizeFileName` / `AddonNode.SanitizeName`：
// 重命名前就把 Windows 不允许的名字挡下来，给出中文原因，
// 而不是把 os.Rename 的原始错误直接甩给用户。
func TestWindowsFileNameProblem(t *testing.T) {
	valid := []string{
		"a.vpk",
		"你好.vpk",
		"a b.vpk",
		"[武器+材质]AK47.vpk",
		"COM10.vpk",
		"concept.vpk",
		"aux-notes.vpk",
		"a.b.vpk",
	}
	for _, name := range valid {
		if problem := windowsFileNameProblem(name); problem != "" {
			t.Fatalf("%q 应该被接受，实际报错: %s", name, problem)
		}
	}

	cases := map[string]string{
		"":            "不能为空",
		"   ":         "不能为空",
		"a:b.vpk":     "不能包含",
		"a?b.vpk":     "不能包含",
		"a*b.vpk":     "不能包含",
		"a\"b.vpk":    "不能包含",
		"a<b.vpk":     "不能包含",
		"a>b.vpk":     "不能包含",
		"a|b.vpk":     "不能包含",
		"a/b.vpk":     "不能包含",
		"a\\b.vpk":    "不能包含",
		"a\x01b.vpk":  "控制字符",
		"name ":       "空格或点",
		"name.":       "空格或点",
		"CON":         "保留设备名",
		"con.vpk":     "保留设备名",
		"NUL.vpk":     "保留设备名",
		"COM1.vpk":    "保留设备名",
		"lpt9.vpk":    "保留设备名",
		"PRN":         "保留设备名",
		"AUX.vpk":     "保留设备名",
	}
	for name, want := range cases {
		problem := windowsFileNameProblem(name)
		if problem == "" {
			t.Fatalf("%q 应该被拒绝", name)
		}
		if !strings.Contains(problem, want) {
			t.Fatalf("%q 的报错应包含 %q，实际 %q", name, want, problem)
		}
	}
}

// TestRenameVPKFileRejectsIllegalNames 覆盖真实重命名入口：报错清晰且不动文件。
func TestRenameVPKFileRejectsIllegalNames(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows 命名规则校验只在 Windows 上生效")
	}
	a, addonsDir := newPriorityTestApp(t)
	path := filepath.Join(addonsDir, "a.vpk")

	for _, bad := range []string{"bad:name.vpk", "CON.vpk", "trailing."} {
		if _, err := a.RenameVPKFile(path, bad); err == nil {
			t.Fatalf("非法文件名 %q 应被拒绝", bad)
		}
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("被拒绝后原文件应保持不变: %v", statErr)
		}
	}

	// 首尾空白会被去掉，但名字本身仍然照常生效（"  合法名字.vpk  " → "合法名字.vpk"）。
	if _, err := a.RenameVPKFile(path, "  合法名字.vpk  "); err != nil {
		t.Fatalf("首尾空白应被去掉后照常重命名: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(addonsDir, "合法名字.vpk")); statErr != nil {
		t.Fatalf("应生成去掉空白后的文件名: %v", statErr)
	}

	// 合法名字仍然可以改。
	if _, err := a.RenameVPKFile(filepath.Join(addonsDir, "合法名字.vpk"), "最终名字.vpk"); err != nil {
		t.Fatalf("合法文件名应可重命名: %v", err)
	}
}
