package app

import (
	"context"
	"log"
	"os"
	"time"

	"vpk-manager/internal/network"
	"vpk-manager/internal/platform/protocol"
	"vpk-manager/internal/platform/urlregistry"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 沙箱只读闸门：启动时先把状态写进日志（只读沙箱实例一眼可辨）。
	logReadonlyLibraryMode()
	// 尽早安装崩溃上报：之后的启动步骤（协议注册、目录恢复、任务扫描）
	// 一旦 panic，都会留下带版本与日志尾部的本地报告。
	a.InstallCrashReporter()
	defer func() {
		// 启动阶段的 panic 同样要变成报告而不是静默退出。
		if r := recover(); r != nil {
			a.runGuarded("启动流程", func() { panic(r) })
		}
	}()

	// 启动单例监听器，接收来自其他实例的URL参数
	singletonMgr, err := StartSingletonListener(a)
	if err != nil {
		log.Printf("启动单例监听器失败: %v", err)
	} else {
		a.singletonMgr = singletonMgr
	}

	// 注册 URL 协议（确保路径变化后始终正确）
	go func() {
		if err := urlregistry.EnsureURLProtocolRegistered(); err != nil {
			log.Printf("注册URL协议失败: %v", err)
		}
	}()

	// 处理启动时的命令行参数（第一个实例自己的参数）
	HandleStartupArgs(a, os.Args)

	// 外部改动自动发现：别的程序往 Mod 目录里增删文件时通知界面静默刷新。
	// rootDir 为空（还没选目录）时这里什么都不做，等 SetRootDirectory 再挂。
	a.restartAddonsWatcher()

	// 清理旧版本文件
	go func() {
		// 稍微延迟一下，确保旧进程完全退出
		time.Sleep(2 * time.Second)

		// 清理过期（7 天）的下载残留：`temp/*_final` 与断点检查点。
		a.cleanupStaleDownloadTempFiles()

		// 如果开启了优选IP，启动时自动触发
		if a.GetWorkshopPreferredIP() {
			a.mu.RLock()
			fixedIP := a.workshopFixedIP
			a.mu.RUnlock()
			if fixedIP != "" {
				log.Printf("检测到优选IP已开启且设置了固定IP: %s，跳过自动优选", fixedIP)
				network.GlobalIPSelector.SetFixedIP(fixedIP)
				runtime.EventsEmit(a.ctx, "ip_selection_end", nil)
			} else {
				log.Println("检测到优选IP已开启，后台启动IP优选...")
				// 设置状态为正在选择
				runtime.EventsEmit(a.ctx, "ip_selection_start", nil)

				go func() {
					// 使用一个典型的工坊图片域名来测试
					network.GlobalIPSelector.GetBestIP("https://steamuserimages-a.akamaihd.net/ugc/test")
					// 完成后通知前端
					runtime.EventsEmit(a.ctx, "ip_selection_end", nil)
				}()
			}
		}

		exe, err := os.Executable()
		if err != nil {
			return
		}
		oldExe := exe + ".old"
		if _, err := os.Stat(oldExe); err == nil {
			if err := os.Remove(oldExe); err != nil {
				log.Printf("清理旧版本失败: %v", err)
			} else {
				log.Printf("已清理旧版本文件: %s", oldExe)
			}
		}
	}()
}

func (a *App) Startup(ctx context.Context) {
	// CUA 调试桥要**尽早**启动：默认构建是空实现，只有 `-tags cua` 构建（配合 LYTVPK_CUA_BRIDGE=1）
	// 才会启动本地求值桥。放在 a.startup(ctx) 之前，首轮扫描（几千个 VPK，可能几十秒）期间
	// 桥就已可用，外部脚本不必盲等。见 docs/development/manual-verification.md 与 cua_bridge_stub.go。
	maybeStartCuaBridge(ctx)
	a.startup(ctx)
}

// HandleProtocolURL 处理 lytvpk:// 协议URL
// 当从浏览器或其他来源收到URL时调用
func (a *App) HandleProtocolURL(url string) {
	log.Printf("收到协议URL: %s", url)

	// 解析URL
	protocolURL, err := protocol.ParseProtocolURL(url)
	if err != nil {
		log.Printf("解析协议URL失败: %v", err)
		// 发送错误事件给前端
		runtime.EventsEmit(a.ctx, "protocol:error", map[string]string{
			"url":     url,
			"message": err.Error(),
		})
		return
	}

	// 根据操作类型发送不同事件
	switch protocolURL.Action {
	case protocol.ProtocolActionParse:
		// 解析工坊ID
		log.Printf("触发解析工坊ID: %s", protocolURL.WorkshopID)
		runtime.EventsEmit(a.ctx, "protocol:parse", map[string]string{
			"workshopId": protocolURL.WorkshopID,
		})

	case protocol.ProtocolActionWorkshop:
		// 在管理器中打开工坊页面
		log.Printf("触发打开工坊页面: %s", protocolURL.WorkshopID)
		runtime.EventsEmit(a.ctx, "protocol:workshop", map[string]string{
			"workshopId": protocolURL.WorkshopID,
		})

	case protocol.ProtocolActionFavoriteServer:
		// 外部程序 / 网页把服务器推进收藏（对齐上游 c1b4972）。
		added, err := a.addFavoriteServer(protocolURL.ServerName, protocolURL.ServerAddress)
		if err != nil {
			log.Printf("添加收藏服务器失败: %v", err)
			runtime.EventsEmit(a.ctx, "protocol:error", map[string]string{
				"url":     url,
				"message": err.Error(),
			})
			return
		}
		log.Printf("收藏服务器: %s (%s) 新增=%v", protocolURL.ServerName, protocolURL.ServerAddress, added)
		runtime.EventsEmit(a.ctx, "protocol:favoriteServer", map[string]interface{}{
			"name":    protocolURL.ServerName,
			"address": protocolURL.ServerAddress,
			"added":   added,
		})

	default:
		log.Printf("未知的协议操作: %s", protocolURL.Action)
		runtime.EventsEmit(a.ctx, "protocol:error", map[string]string{
			"url":     url,
			"message": "未知的协议操作",
		})
	}
}

// ForceExit forces the application to exit
func (a *App) ForceExit() {
	a.forceClose = true
	runtime.Quit(a.ctx)
}

// beforeClose is called when the application is about to close
func (a *App) beforeClose() (prevent bool) {
	if a.forceClose {
		a.stopBackgroundResources()
		return false
	}

	if a.HasActiveDownloads() || a.HasActivePanelUploads() {
		// 与后台路径同一约定：没有 Wails 上下文（CLI / 测试）时只能记日志，不能直接调用
		// runtime（它会报 "An invalid context was passed" 并把进程带走）。
		a.emitEvent("show_exit_confirmation", nil)
		return true
	}

	a.stopBackgroundResources()
	return false
}

// stopBackgroundResources 关闭所有"只在前端窗口显示期间才允许存在"的后台资源。
//
// 约定（用户明确要求）：窗口关掉之后，进程里不允许再留着任何常驻监听/线程/端口 ——
// 包括 Mod 目录监听（fsnotify）、addonlist 守护轮询、图片代理 HTTP 监听、单例 TCP 监听
// 与崩溃上报管道。每一项都必须幂等：beforeClose 可能因为多次退出请求被调用两次，
// 「重启应用」也会先调用一次再走退出流程。
func (a *App) stopBackgroundResources() {
	a.stopAddonListMonitor()
	a.stopAddonsWatcher()
	a.stopImageProxy()
	if a.singletonMgr != nil {
		_ = a.singletonMgr.Close()
		a.singletonMgr = nil
	}
	a.CloseCrashReporter()
	log.Printf("已释放后台监听与端口（Mod 目录监听 / addonlist 守护 / 图片代理 / 单例端口）")
}

// stopImageProxy 关闭本地图片代理（它是唯一一个 HTTP 监听型后台资源）。
func (a *App) stopImageProxy() {
	if a.proxyServer != nil {
		a.proxyServer.Close()
	}
}

// restartBackgroundResources 把 stopBackgroundResources 关掉的资源重新拉起来。
// 只用在"重启应用时新进程启动失败"这条路径上，避免应用停在半残状态。
func (a *App) restartBackgroundResources() {
	a.restartAddonListMonitor()
	a.restartAddonsWatcher()
	if a.proxyServer != nil {
		if err := a.proxyServer.Start(); err != nil {
			log.Printf("恢复图片代理失败: %v", err)
		}
	}
	if a.ctx != nil {
		if manager, err := StartSingletonListener(a); err != nil {
			log.Printf("恢复单例监听失败: %v", err)
		} else {
			a.singletonMgr = manager
		}
	}
	a.InstallCrashReporter()
}

func (a *App) BeforeClose(ctx context.Context) (prevent bool) {
	return a.beforeClose()
}

// SetRootDirectory 设置根目录
