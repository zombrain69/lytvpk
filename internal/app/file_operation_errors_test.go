package app

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// 「文件被占用」这类失败以前是把 Go 的原始错误直接甩出去：
//
//	rename C:\...\a.vpk C:\...\disabled\a.vpk: The process cannot access the file
//	because it is being used by another process.
//
// 用户既看不懂也不知道下一步做什么。下面是翻译层的回归测试。

func TestDescribeFileMoveFailureWindowsCodes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("错误码按 Windows 语义对照，其它平台不适用")
	}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"文件被占用", &os.PathError{Op: "rename", Path: "a.vpk", Err: syscall.Errno(32)}, "占用"},
		{"区域被锁定", syscall.Errno(33), "占用"},
		{"拒绝访问", syscall.Errno(5), "没有权限"},
		{"同名文件", syscall.Errno(183), "同名文件"},
		{"路径过长", syscall.Errno(206), "260"},
		{"磁盘已满", syscall.Errno(112), "磁盘空间不足"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := describeFileMoveFailure(testCase.err)
			if !strings.Contains(got, testCase.want) {
				t.Fatalf("describeFileMoveFailure(%v) = %q，期望包含 %q", testCase.err, got, testCase.want)
			}
		})
	}
}

func TestFormatFileMoveErrorReplacesRawSystemText(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("错误码按 Windows 语义对照，其它平台不适用")
	}
	raw := &os.PathError{
		Op:   "rename",
		Path: `C:\addons\zztest_hud_only.vpk`,
		Err:  syscall.Errno(32),
	}
	got := formatFileMoveError("禁用", `C:\addons\zztest_hud_only.vpk`, raw).Error()

	if !strings.Contains(got, "禁用 zztest_hud_only.vpk 失败") {
		t.Fatalf("提示要说清动作与文件: %q", got)
	}
	if !strings.Contains(got, "占用") || !strings.Contains(got, "重试") {
		t.Fatalf("提示要给出原因与下一步: %q", got)
	}
	if strings.Contains(got, "The process cannot access") {
		t.Fatalf("不该把原始英文系统错误露给用户: %q", got)
	}
	if strings.Contains(got, `C:\addons`) {
		t.Fatalf("完整路径对用户没有帮助，只留文件名: %q", got)
	}
}

func TestFormatFileMoveErrorKeepsUnknownCause(t *testing.T) {
	raw := errors.New("some unknown filesystem failure")
	got := formatFileMoveError("移动", `C:\addons\a.vpk`, raw).Error()
	if !strings.Contains(got, "a.vpk") || !strings.Contains(got, "some unknown filesystem failure") {
		t.Fatalf("认不出的原因要保留原文，实际 %q", got)
	}
}

func TestDescribeFileMoveFailureRecognizesMissingSource(t *testing.T) {
	raw := &os.PathError{Op: "rename", Path: "gone.vpk", Err: os.ErrNotExist}
	got := describeFileMoveFailure(raw)
	if !strings.Contains(got, "刷新") {
		t.Fatalf("源文件消失时要提示刷新列表，实际 %q", got)
	}
}
