package app

import "github.com/wailsapp/wails/v2/pkg/runtime"

// emitEvent 是 runtime.EventsEmit 的判空包装。
//
// 为什么必须统一走它：Wails 对无效（nil / 未就绪）的 ctx 是 log.Fatal，会把整个
// 进程带走 —— 后台任务（下载完成回写、模型扫描进度、镜像测速、IP 优选、Mod 轮换）
// 以及启动早期、CLI 与测试里都可能拿到 nil ctx。仓库原有"每个 emit 先判空"的约定，
// 这里提供统一入口，后续新增事件默认用它就不会再漏。
func (a *App) emitEvent(name string, payload ...any) {
	if a == nil || a.ctx == nil || name == "" {
		return
	}
	runtime.EventsEmit(a.ctx, name, payload...)
}
