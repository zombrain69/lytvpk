package app

import (
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// OpenURLInSteam 用 Steam 客户端的内置浏览器打开一个 http(s) 链接（steam://openurl/）。
//
// 用途：退订引导这类需要"已登录会话"的页面，在 Steam 内置浏览器里打开可以直接用
// 用户当前的 Steam 登录态，比系统浏览器（可能未登录 steamcommunity）更顺手。
// 只接受 http(s) 前缀：拒绝其它协议，避免被当成任意协议跳板。
func (a *App) OpenURLInSteam(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://") {
		return fmt.Errorf("仅支持 http(s) 链接")
	}
	runtime.BrowserOpenURL(a.ctx, "steam://openurl/"+rawURL)
	return nil
}
