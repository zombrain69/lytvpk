//go:build windows

package app

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// steam_api64.dll 运行时加载（ISteamUGC）—— Windows 实现。
//
// 为什么走 syscall 而不是 cgo / go-steamworks 库：
//   - DLL 不能在编译期链接：Valve 许可不允许随包分发（用户自备，路径运行时才知道）；
//   - syscall.NewLazyDLL(path) 支持任意路径的运行时加载，且不受 Wails 构建的 CGO 配置影响；
//   - 官方 Go 封装（hajimehoshi/go-steamworks）当时未暴露 UGC 接口，自写绑定量也很小。
//
// 安全说明：本文件是全仓库除 secret_windows.go（DPAPI）之外唯一使用 unsafe.Pointer 的地方。
// 用途只有一种：把 Go 缓冲区的地址按 SteamAPI C 调用约定传给 DLL（结果缓冲、ID 数组）。
// 所有涉及到的调用面与验证过程见 docs/development/workshop-subscription-eval.md §5。
//
// 参考实现（已验通）：.tmp-cua/steam-ugc-spike/（只读版 + 写闭环版；这是"照此模式实现"的样例）。

// steamUGCSessionDLL 是 steam_api64.dll 的真实会话。
// 所有 ISteamUGC 调用都是进程内的同步接口；只有"等待生效"需要 RunCallbacks 泵消息。
type steamUGCSessionDLL struct {
	steam        *syscall.LazyDLL
	ugc          uintptr
	runCallbacks *syscall.LazyProc
	numSubs      *syscall.LazyProc
	getSubs      *syscall.LazyProc
	unsubscribe  *syscall.LazyProc
	subscribe    *syscall.LazyProc
	shutdown     *syscall.LazyProc
	closed       bool
}

func steamUGCProc(steam *syscall.LazyDLL, name string) *syscall.LazyProc {
	return steam.NewProc(name)
}

// openSteamUGCSessionImpl 加载 DLL、以 L4D2（app 550）身份初始化、取 ISteamUGC 句柄。
func openSteamUGCSessionImpl(dllPath string) (steamUGCSession, error) {
	dllPath = strings.TrimSpace(dllPath)
	if dllPath == "" {
		return nil, fmt.Errorf("DLL 路径为空")
	}
	if _, err := os.Stat(dllPath); err != nil {
		return nil, fmt.Errorf("找不到 steam_api64.dll：%v", err)
	}

	// Steam 上下文：以 L4D2（550）身份初始化；必须在 Load/Init 之前设置。
	_ = os.Setenv("SteamAppId", "550")
	_ = os.Setenv("SteamGameId", "550")

	steam := syscall.NewLazyDLL(dllPath)
	if err := steam.Load(); err != nil {
		return nil, fmt.Errorf("加载 steam_api64.dll 失败：%v", err)
	}

	if err := steamAPIInit(steam); err != nil {
		return nil, err
	}

	ugc, err := steamUGCInterface(steam)
	if err != nil {
		// 初始化完成但拿不到接口：撤掉，不留半初始化状态。
		steamUGCProc(steam, "SteamAPI_Shutdown").Call()
		return nil, err
	}

	for _, required := range []string{
		"SteamAPI_ISteamUGC_GetNumSubscribedItems",
		"SteamAPI_ISteamUGC_GetSubscribedItems",
		"SteamAPI_ISteamUGC_UnsubscribeItem",
		"SteamAPI_ISteamUGC_SubscribeItem",
		"SteamAPI_RunCallbacks",
		"SteamAPI_Shutdown",
	} {
		if err := steamUGCProc(steam, required).Find(); err != nil {
			steamUGCProc(steam, "SteamAPI_Shutdown").Call()
			return nil, fmt.Errorf("DLL 缺少接口 %s（文件损坏或版本过旧？）", required)
		}
	}

	return &steamUGCSessionDLL{
		steam:        steam,
		ugc:          ugc,
		runCallbacks: steamUGCProc(steam, "SteamAPI_RunCallbacks"),
		numSubs:      steamUGCProc(steam, "SteamAPI_ISteamUGC_GetNumSubscribedItems"),
		getSubs:      steamUGCProc(steam, "SteamAPI_ISteamUGC_GetSubscribedItems"),
		unsubscribe:  steamUGCProc(steam, "SteamAPI_ISteamUGC_UnsubscribeItem"),
		subscribe:    steamUGCProc(steam, "SteamAPI_ISteamUGC_SubscribeItem"),
		shutdown:     steamUGCProc(steam, "SteamAPI_Shutdown"),
	}, nil
}

// steamAPIInit 优先新版 InitFlat（带错误信息缓冲），旧版 Init 兜底。
func steamAPIInit(steam *syscall.LazyDLL) error {
	if proc := steamUGCProc(steam, "SteamAPI_InitFlat"); proc.Find() == nil {
		var message [1024]byte
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&message[0])))
		if r != 0 {
			return fmt.Errorf("SteamAPI_InitFlat 失败（code=%d）：%s", r, steamUGCString(message[:]))
		}
		return nil
	}
	r, _, _ := steamUGCProc(steam, "SteamAPI_Init").Call()
	if r == 0 {
		return fmt.Errorf("SteamAPI_Init 返回失败（Steam 客户端未运行或未登录？）")
	}
	return nil
}

// steamUGCInterface 逐个探测 ISteamUGC 接口版本后缀（随 SDK 版本变动，由新到旧）。
func steamUGCInterface(steam *syscall.LazyDLL) (uintptr, error) {
	for _, name := range []string{
		"SteamAPI_SteamUGC_v022", "SteamAPI_SteamUGC_v021", "SteamAPI_SteamUGC_v020",
		"SteamAPI_SteamUGC_v019", "SteamAPI_SteamUGC_v018", "SteamAPI_SteamUGC_v017",
		"SteamAPI_SteamUGC_v016", "SteamAPI_SteamUGC_v015", "SteamAPI_SteamUGC_v014",
		"SteamAPI_SteamUGC_v013",
	} {
		proc := steamUGCProc(steam, name)
		if proc.Find() != nil {
			continue
		}
		if handle, _, _ := proc.Call(); handle != 0 {
			return handle, nil
		}
	}
	return 0, fmt.Errorf("DLL 里没有可用的 ISteamUGC 接口（文件损坏或版本过旧？）")
}

// steamUGCString 取 C 字符串缓冲区里的内容（到首个 NUL 为止）。
func steamUGCString(buf []byte) string {
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		return string(buf[:i])
	}
	return string(buf)
}

func (s *steamUGCSessionDLL) SubscribedItems() ([]uint64, error) {
	r, _, _ := s.numSubs.Call(s.ugc)
	count := uint32(r)
	if count == 0 {
		return nil, nil
	}
	ids := make([]uint64, count)
	got, _, _ := s.getSubs.Call(s.ugc, uintptr(unsafe.Pointer(&ids[0])), uintptr(count))
	n := uint32(got)
	if n > count {
		n = count
	}
	// 复制一份返回，不把大缓冲留给调用方（调用方通常只做成员判断）。
	out := make([]uint64, n)
	copy(out, ids[:n])
	return out, nil
}

func (s *steamUGCSessionDLL) SetSubscribed(id uint64, want bool) error {
	proc := s.unsubscribe
	verb := "退订"
	if want {
		proc = s.subscribe
		verb = "订阅"
	}
	call, _, _ := proc.Call(s.ugc, uintptr(id))
	if call == 0 {
		return fmt.Errorf("Steam 拒绝了%s请求（接口未就绪）", verb)
	}
	return s.waitSubscribedState(id, want, verb)
}

// waitSubscribedState 轮询等待目标状态生效（写验证实测 ~0.7s 内确认；25s 超时）。
func (s *steamUGCSessionDLL) waitSubscribedState(id uint64, want bool, verb string) error {
	deadline := time.Now().Add(steamUGCConfirmTimeout)
	for {
		s.runCallbacks.Call()
		if s.subscribedState(id) == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("等待 Steam 确认%s超时，请稍后在 Steam 订阅页核对", verb)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *steamUGCSessionDLL) subscribedState(id uint64) bool {
	ids, _ := s.SubscribedItems()
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func (s *steamUGCSessionDLL) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	s.shutdown.Call()
}
