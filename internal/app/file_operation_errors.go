package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// 移动 / 改名失败时，Go 抛给用户的是这样的原文：
//
//	rename C:\...\addons\a.vpk C:\...\addons\disabled\a.vpk:
//	The process cannot access the file because it is being used by another process.
//
// 同一句话里既有两条完整路径又有英文系统错误，用户既看不懂也不知道下一步做什么。
// 这里把「哪个文件、为什么、怎么办」翻译成中文；认不出来的错误仍然保留原文，
// 免得把真正的原因吞掉。
//
// 错误码按数值对照：syscall 只导出了其中一部分，而 CI 与发布都在 Windows 上跑；
// 非 Windows 平台不会命中这些分支，会自动退回到下面的通用判断。
const (
	winErrAccessDenied     syscall.Errno = 5   // ERROR_ACCESS_DENIED
	winErrSharingViolation syscall.Errno = 32  // ERROR_SHARING_VIOLATION
	winErrLockViolation    syscall.Errno = 33  // ERROR_LOCK_VIOLATION
	winErrDiskFull         syscall.Errno = 112 // ERROR_DISK_FULL
	winErrAlreadyExists    syscall.Errno = 183 // ERROR_ALREADY_EXISTS
	winErrFilenameExceeded syscall.Errno = 206 // ERROR_FILENAME_EXCED_RANGE
)

// describeFileMoveFailure 返回可行动的中文原因；认不出来时返回空串。
func describeFileMoveFailure(err error) string {
	if err == nil {
		return ""
	}
	if runtime.GOOS == "windows" {
		switch {
		case errors.Is(err, winErrSharingViolation), errors.Is(err, winErrLockViolation):
			return "文件正被其它程序占用（游戏、杀毒软件、资源管理器预览都可能占用它），关闭占用的程序后重试"
		case errors.Is(err, winErrAccessDenied):
			return "没有权限修改这个文件（可能被设为只读，或被安全软件拦截）"
		case errors.Is(err, winErrAlreadyExists):
			return "目标目录里已经有同名文件，请先改名或删除它"
		case errors.Is(err, winErrFilenameExceeded):
			return "路径超过 Windows 的 260 字符上限，请缩短名称或把 Mod 放到更浅的目录"
		case errors.Is(err, winErrDiskFull):
			return "磁盘空间不足"
		}
	}
	switch {
	case errors.Is(err, os.ErrPermission):
		return "没有权限修改这个文件（可能被设为只读，或被安全软件拦截）"
	case errors.Is(err, os.ErrNotExist):
		return "找不到这个文件，可能已被移动或删除，请刷新列表后重试"
	}
	return ""
}

// formatFileMoveError 给移动 / 改名失败补上文件名与可行动的原因。
//
// action 用中文动词（禁用 / 启用 / 重命名 / 复制），path 传源文件路径：
// 错误里只留文件名，完整路径对用户没有帮助，还会把提示撑得很长。
func formatFileMoveError(action, path string, err error) error {
	name := filepath.Base(path)
	if reason := describeFileMoveFailure(err); reason != "" {
		return fmt.Errorf("%s %s 失败：%s", action, name, reason)
	}
	return fmt.Errorf("%s %s 失败：%v", action, name, err)
}
