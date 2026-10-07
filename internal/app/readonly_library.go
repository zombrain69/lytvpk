package app

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// readonlyLibraryEnv 是"沙箱只读库"闸门的开关环境变量。
//
// 背景：CUA 桥沙箱实例有两种用途 —— 只读验证（对着真实库读 DOM / 量几何 / 抓像素）
// 和写盘验证（批量开关、移动、删除）。2026-10-03 的 UX 批量探针就是因为把写盘类操作
// 打到了"只读沙箱"实例上（那个实例沿用真实配置 → 真实 addons 目录），一次批量关掉了
// 用户 110 个 Mod 的 addonlist 开关。
//
// 这个闸门把"真实库只读"从"靠自觉"变成"后端强制"：启动时带上
// LYTVPK_READONLY_LIBRARY=1，后端任何会改动 Mod 库的调用都会被当场拒绝。
//
// 默认（不设置该变量）行为与之前完全一致，正式版不受影响。
const readonlyLibraryEnv = "LYTVPK_READONLY_LIBRARY"

// readonlyLibraryMode 判断当前进程是否处于沙箱只读模式。
func readonlyLibraryMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(readonlyLibraryEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// logReadonlyLibraryMode 在启动时把闸门状态写进日志，避免"以为开了其实没开"。
func logReadonlyLibraryMode() {
	if readonlyLibraryMode() {
		log.Printf("沙箱只读模式已启用（%s=1）：拒绝写 addonlist.txt、拒绝移动/重命名/删除 Mod 文件、拒绝导入",
			readonlyLibraryEnv)
	}
}

// rejectReadonlyLibraryWrite 在沙箱只读模式下拒绝写真实库的操作。
// action 是给界面提示用的中文动作描述，会原样出现在错误消息里。
func rejectReadonlyLibraryWrite(action string) error {
	if !readonlyLibraryMode() {
		return nil
	}
	return fmt.Errorf("沙箱只读模式（%s=1）：已拒绝%s。该实例只用于只读验证，写盘请用副本沙箱",
		readonlyLibraryEnv, action)
}
