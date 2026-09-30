package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 压缩包管理里，卡片上显示的是 info.Error，悬停详情是 info.ErrorDetail。
// 真机复现：损坏的 zip 会显示「无法打开 ZIP: zip: not a valid zip file」——
// 库的英文原文直接摆在了用户面前；7z 分支早就是「中文提示 + 原始细节进悬停」，
// 这里把 zip / rar / tar 拉齐。

func TestScanArchivePackageReportsChineseErrorForBrokenZip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.zip")
	if err := os.WriteFile(path, []byte("PK"+strings.Repeat("x", 400)), 0o644); err != nil {
		t.Fatal(err)
	}

	info := scanArchivePackage(path, "zip", archiveExistingVPKIndex{})
	if info.Error == "" {
		t.Fatal("损坏的 zip 应给出错误提示")
	}
	if !strings.Contains(info.Error, "无法读取 ZIP") {
		t.Fatalf("卡片提示应是中文可行动文案，实际 %q", info.Error)
	}
	if strings.Contains(info.Error, "not a valid zip") {
		t.Fatalf("卡片提示里不该出现库的英文原文：%q", info.Error)
	}
	if !strings.Contains(info.ErrorDetail, "not a valid zip") {
		t.Fatalf("原始错误要保留在 ErrorDetail 里做排查，实际 %q", info.ErrorDetail)
	}
	if info.ErrorKind != archiveErrorKindUnreadable {
		t.Fatalf("错误分类应为不可读，实际 %q", info.ErrorKind)
	}
}

func TestScanArchivePackageReportsChineseErrorForMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone.zip")
	info := scanArchivePackage(path, "zip", archiveExistingVPKIndex{})
	if !strings.Contains(info.Error, "找不到这个压缩包") {
		t.Fatalf("文件不存在时应给中文提示，实际 %q", info.Error)
	}
	if info.ErrorDetail == "" {
		t.Fatal("原始错误要保留在 ErrorDetail")
	}
}

func TestDescribeArchiveReadFailureMentionsPassword(t *testing.T) {
	got := describeArchiveReadFailure("7z", errTestPasswordRequired)
	if !strings.Contains(got, "密码") {
		t.Fatalf("加密包应提示输入密码，实际 %q", got)
	}
}

// TestExtractVPKFromArchiveReportsChineseErrorForBrokenZip 覆盖真机复现：
// 解包路径（工具箱 / 压缩包管理里的"解包"）原来把解码库的英文原文直接拼在
// 「无法打开ZIP文件:」后面 —— 用户看到的是 `zip: not a valid zip file`。
// 现在和扫描路径一样：只给中文可行动说明，原始英文写日志。
func TestExtractVPKFromArchiveReportsChineseErrorForBrokenZip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.zip")
	if err := os.WriteFile(path, []byte("PK"+strings.Repeat("x", 400)), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &App{}
	err := a.ExtractVPKFromArchive(path, dir)
	if err == nil {
		t.Fatal("损坏的 zip 解包应该报错")
	}
	if !strings.Contains(err.Error(), "无法读取 ZIP") {
		t.Fatalf("解包报错应是中文可行动说明，实际 %q", err.Error())
	}
	if strings.Contains(strings.ToLower(err.Error()), "not a valid zip") {
		t.Fatalf("解包报错里不该出现解码库英文原文：%q", err.Error())
	}
}

// TestExtractVPKFromArchiveReportsChineseErrorForBrokenRar：rar 分支同理。
func TestExtractVPKFromArchiveReportsChineseErrorForBrokenRar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.rar")
	if err := os.WriteFile(path, []byte("Rar!"+strings.Repeat("x", 64)), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &App{}
	err := a.ExtractVPKFromArchive(path, dir)
	if err == nil {
		t.Fatal("损坏的 rar 解包应该报错")
	}
	if !strings.Contains(err.Error(), "无法读取 RAR") {
		t.Fatalf("解包报错应是中文可行动说明，实际 %q", err.Error())
	}
}

// errTestPasswordRequired 用库常见的英文写法，确认关键词识别不区分大小写。
var errTestPasswordRequired = &testError{"archive is encrypted: password required"}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }
