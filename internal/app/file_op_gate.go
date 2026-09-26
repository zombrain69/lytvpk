package app

import (
	"errors"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 文件操作的"互斥闸门"（对齐 FireAxe `WorkshopVpkAddon.BlockMove()`）。
//
// FireAxe 在长任务运行期间会 BlockMove，避免用户在任务途中移动节点；
// 本项目的移动 / 删除 / 打包都是"逐个文件改真实磁盘"的批量操作，两种操作同时跑会出现：
//   - 同一个文件被两次操作争抢，其中一次以"源文件不存在"失败，用户看到莫名其妙的错误；
//   - 批量操作半途被另一批操作插队，结果计数与用户预期不符。
// 这里用 TryLock 让**同一时间只允许一个文件操作**，后来者拿到一句清楚的提示而不是排队死等。
//
// 注意（为什么只锁这几个入口）：
//   - `RepairVPKIntegrity` 内部会调用 `UnpackVPKFile`，`RepairVPKIntegrityBatch` 又会调用
//     `RepairVPKIntegrity`，所以这两个**不能**加锁（否则自锁死）；修复流程本身写的是临时目录 +
//     `.repaired.vpk` 新文件，不参与"移动/删除/打包"的争抢。
//   - `UnpackVPKFile` 直接写用户指定的输出目录，同理不加锁。
var errFileOperationBusy = errors.New("另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试")

// beginFileOperation 尝试进入"文件操作"临界区；已被占用时返回可读错误。
func (a *App) beginFileOperation() error {
	if !a.fileOpsMu.TryLock() {
		return errFileOperationBusy
	}
	a.emitFileOperationState(true)
	return nil
}

// endFileOperation 退出临界区（必须与 beginFileOperation 成对出现）。
func (a *App) endFileOperation() {
	a.fileOpsMu.Unlock()
	a.emitFileOperationState(false)
}

// emitFileOperationState 把忙碌状态推给界面。
//
// 为什么用事件而不是让界面轮询：一次打包/移动可能只持续几百毫秒，
// 1 秒一次的轮询会整段错过（真机验收时就是这样：徽标一次都没出现）。
// 状态变化是后端最清楚，所以由后端主动推；界面那边仍保留低频轮询兜底
// （覆盖"界面比事件晚启动"等边界）。
func (a *App) emitFileOperationState(busy bool) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "file_operation_state", busy)
}

// IsFileOperationBusy 让界面可以问"现在有没有文件操作在跑"（用于禁用按钮 / 显示进度文案）。
func (a *App) IsFileOperationBusy() bool {
	if a.fileOpsMu.TryLock() {
		a.fileOpsMu.Unlock()
		return false
	}
	return true
}

// fileOpsMu 放在 App 上（见 app.go），这里只补一个编译期断言，避免以后被改成指针字段时忘记初始化。
var _ = func(a *App) *sync.Mutex { return &a.fileOpsMu }
