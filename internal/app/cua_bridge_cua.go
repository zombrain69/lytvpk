//go:build cua

package app

// CUA 调试桥（**只有 `-tags cua` 构建才存在**；默认构建与发布构建里是空实现，见 cua_bridge_stub.go）。
//
// 用途：在打包 EXE 的真实 WebView 里执行 JS 并取回结果，做 UI 实机断言与目视；
// 方法说明见 docs/development/manual-verification.md（"临时 JS 调试桥"一节）。
//
// 启动条件：环境变量 LYTVPK_CUA_BRIDGE=1；只监听 127.0.0.1。
// 端口：优先 LYTVPK_CUA_PORT，未设置则随机；实际端口写进
// %TEMP%\lytvpk-cua-bridge.log（"listening 127.0.0.1:<port>"），供外部脚本读取。
//
// 接口：
//
//	GET  /ping                                   → {"ok":true}
//	POST /eval        {"id":"x","js":"..."}      → 立即返回，结果走 /result
//	GET  /result?id=x&timeout_ms=30000           → {"ok":true,"value":...} / 504
//	POST /result?id=x {"ok":true,"value":...}    → 页面回传（带 CORS / PNA 头）
//	POST /eval-sync   {"js":"...","timeoutMs":N} → 一次性求值并返回结果
//
// 两个已知坑（都踩过，别再踩）：
//  1. 页面侧回传必须用**绝对地址** `http://127.0.0.1:<port>/result?...`；
//     用相对路径会打到页面自身，表现为"求值超时"。
//  2. 自定义 scheme 页面访问回环地址会触发 Private Network Access 预检，
//     必须回 `Access-Control-Allow-Private-Network: true`。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type cuaBridgeResult struct {
	OK    bool `json:"ok"`
	Value any  `json:"value,omitempty"`
	Error any  `json:"error,omitempty"`
}

type cuaBridge struct {
	ctx     context.Context
	baseURL string
	mu      sync.Mutex
	waits   map[string]chan cuaBridgeResult
}

// maybeStartCuaBridge 只在 LYTVPK_CUA_BRIDGE=1 时启动（且只在 `-tags cua` 构建里存在真实现）。
func maybeStartCuaBridge(ctx context.Context) {
	if strings.TrimSpace(os.Getenv("LYTVPK_CUA_BRIDGE")) != "1" {
		return
	}

	bridge := &cuaBridge{ctx: ctx, waits: make(map[string]chan cuaBridgeResult)}
	address := "127.0.0.1:0"
	if port := strings.TrimSpace(os.Getenv("LYTVPK_CUA_PORT")); port != "" {
		address = "127.0.0.1:" + port
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Printf("cua bridge: 监听失败: %v", err)
		return
	}
	actual := listener.Addr().(*net.TCPAddr).Port
	bridge.baseURL = fmt.Sprintf("http://127.0.0.1:%d", actual)
	log.Printf("cua bridge listening 127.0.0.1:%d", actual)
	if logPath := filepath.Join(os.TempDir(), "lytvpk-cua-bridge.log"); logPath != "" {
		if handle, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); openErr == nil {
			fmt.Fprintf(handle, "%s listening 127.0.0.1:%d\n", time.Now().Format(time.RFC3339), actual)
			handle.Close()
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeCUAJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/eval", bridge.handleEval)
	mux.HandleFunc("/eval-sync", bridge.handleEvalSync)
	mux.HandleFunc("/result", bridge.handleResult)
	go func() {
		if serveErr := http.Serve(listener, withCUACORS(mux)); serveErr != nil {
			log.Printf("cua bridge: 服务结束: %v", serveErr)
		}
	}()
}

func (bridge *cuaBridge) handleEval(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ID string `json:"id"`
		JS string `json:"js"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if payload.ID == "" {
		payload.ID = fmt.Sprintf("auto%d", time.Now().UnixNano())
	}
	bridge.mu.Lock()
	bridge.waits[payload.ID] = make(chan cuaBridgeResult, 1)
	bridge.mu.Unlock()
	bridge.exec(payload.ID, payload.JS)
	writeCUAJSON(w, map[string]any{"ok": true, "id": payload.ID})
}

func (bridge *cuaBridge) handleEvalSync(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		JS        string `json:"js"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if payload.TimeoutMs <= 0 {
		payload.TimeoutMs = 30000
	}
	id := fmt.Sprintf("sync%d", time.Now().UnixNano())
	ch := make(chan cuaBridgeResult, 1)
	bridge.mu.Lock()
	bridge.waits[id] = ch
	bridge.mu.Unlock()
	bridge.exec(id, payload.JS)
	select {
	case result := <-ch:
		writeCUAJSON(w, result)
	case <-time.After(time.Duration(payload.TimeoutMs) * time.Millisecond):
		writeCUAJSON(w, cuaBridgeResult{OK: false, Error: "timeout"})
	}
}

func (bridge *cuaBridge) handleResult(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if r.Method == http.MethodPost {
		var result cuaBridgeResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		bridge.deliver(id, result)
		writeCUAJSON(w, map[string]any{"ok": true})
		return
	}
	timeout := 30 * time.Second
	if raw := r.URL.Query().Get("timeout_ms"); raw != "" {
		if parsed, err := time.ParseDuration(raw + "ms"); err == nil {
			timeout = parsed
		}
	}
	bridge.mu.Lock()
	ch, exists := bridge.waits[id]
	bridge.mu.Unlock()
	if !exists {
		http.Error(w, "unknown id", http.StatusNotFound)
		return
	}
	select {
	case result := <-ch:
		bridge.mu.Lock()
		delete(bridge.waits, id)
		bridge.mu.Unlock()
		writeCUAJSON(w, result)
	case <-time.After(timeout):
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	}
}

func (bridge *cuaBridge) deliver(id string, result cuaBridgeResult) {
	bridge.mu.Lock()
	ch, exists := bridge.waits[id]
	bridge.mu.Unlock()
	if !exists {
		return
	}
	select {
	case ch <- result:
	default:
	}
}

// exec 把表达式包成 async IIFE，在页面里求值并让它把结果 POST 回 bridge（绝对地址 + CORS）。
func (bridge *cuaBridge) exec(id, expression string) {
	code := fmt.Sprintf(`(async () => {
  const __id = %q;
  const __url = %q + '/result?id=' + encodeURIComponent(__id);
  const __post = (payload) => fetch(__url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    mode: 'cors',
  }).catch(() => {});
  try {
    const __value = await (%s);
    await __post({ ok: true, value: JSON.parse(JSON.stringify(__value === undefined ? null : __value)) });
  } catch (error) {
    await __post({ ok: false, error: String((error && error.stack) || error) });
  }
})();`, id, bridge.baseURL, expression)
	runtime.WindowExecJS(bridge.ctx, code)
}

func withCUACORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeCUAJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}
