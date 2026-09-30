package app

import (
	"errors"
	"strings"
	"testing"
)

// 真机复现：压缩包被占用时，失败条目成了
//
//	移动 zz-archive.zip 失败: 移动 zz-archive.zip 失败：文件正被其它程序占用（…）
//
// 底层已经给过完整提示，外面不该再套一层同样的前缀。
func TestDescribeArchiveMoveFailureDoesNotDoublePrefix(t *testing.T) {
	inner := errors.New("移动 zz-archive.zip 失败：文件正被其它程序占用（游戏、杀毒软件、资源管理器预览都可能占用它），关闭占用的程序后重试")
	got := describeArchiveMoveFailure(`C:\addons\zz-archive.zip`, inner)

	if got != inner.Error() {
		t.Fatalf("不该再包一层前缀，实际 %q", got)
	}
	if strings.Count(got, "移动 zz-archive.zip 失败") != 1 {
		t.Fatalf("「移动 … 失败」只应出现一次，实际 %q", got)
	}
}

// 底层只给了原因（例如冲突提示）时，仍然要带上"移动哪个文件"。
func TestDescribeArchiveMoveFailureAddsFileWhenMissing(t *testing.T) {
	got := describeArchiveMoveFailure(`C:\addons\zz-archive.zip`, errors.New("目标文件已存在"))
	if !strings.HasPrefix(got, "移动 zz-archive.zip 失败: ") {
		t.Fatalf("缺少文件名/动作前缀：%q", got)
	}
	if !strings.Contains(got, "目标文件已存在") {
		t.Fatalf("原因丢了：%q", got)
	}
}
