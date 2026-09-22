package app

import "testing"

// 没有 Wails 上下文时（后台任务、扫描早期、测试）记录错误必须只写日志，
// 不能调用 runtime.EventsEmit：Wails 会用无效上下文直接中断进程。
func TestLogErrorWithoutWailsContextDoesNotAbort(t *testing.T) {
	a := &App{}
	a.LogError("单元测试", "没有 Wails 上下文时也应安全返回", `E:\tmp\demo.vpk`)
}
