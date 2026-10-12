//go:build !windows

package app

import "fmt"

// openSteamUGCSessionImpl 在非 Windows 平台不可用（steam_api64.dll 是 Windows 动态库；
// 该平台也不运行 Steam 客户端）。状态查询与界面仍可用，仅一键执行被禁用。
func openSteamUGCSessionImpl(string) (steamUGCSession, error) {
	return nil, fmt.Errorf("一键退订目前仅支持 Windows")
}
