package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ImageProxyServer 提供本地图片代理服务
type ImageProxyServer struct {
	server   *http.Server
	port     int
	selector *IPSelector
}

func NewImageProxyServer(selector *IPSelector) *ImageProxyServer {
	return &ImageProxyServer{
		selector: selector,
	}
}

// Start 启动代理服务器
func (s *ImageProxyServer) Start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	s.port = listener.Addr().(*net.TCPAddr).Port
	fmt.Printf("[Proxy] Started image proxy on port %d\n", s.port)

	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", s.handleProxy)

	s.server = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[Proxy] Server error: %v\n", err)
		}
	}()

	return nil
}

// GetProxyUrl 将原始URL转换为代理URL
func (s *ImageProxyServer) GetProxyUrl(originalUrl string) string {
	if s.port == 0 {
		return originalUrl
	}
	return fmt.Sprintf("http://127.0.0.1:%d/proxy?url=%s", s.port, url.QueryEscape(originalUrl))
}

// Close 停止图片代理并释放端口。
//
// 这个代理是"随窗口存在"的后台资源：窗口关掉以后不允许还留着监听端口/线程。
// 可以重复调用（第二次起直接返回），因为关闭窗口与"重启应用"都会走一遍。
func (s *ImageProxyServer) Close() {
	if s == nil || s.server == nil {
		return
	}
	port := s.port
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		_ = s.server.Close()
	}
	s.server = nil
	s.port = 0
	fmt.Printf("[Proxy] Stopped image proxy on port %d\n", port)
}

func (s *ImageProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	targetUrlStr := r.URL.Query().Get("url")
	if targetUrlStr == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	targetUrl, err := url.Parse(targetUrlStr)
	if err != nil {
		http.Error(w, "Invalid url", http.StatusBadRequest)
		return
	}

	// 获取优选IP
	// 注意：这里我们使用 IPSelector 的缓存结果
	// 如果没有优选IP，就直接连接
	bestIP := s.selector.GetCachedBestIP()

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// 如果有优选IP，强制使用该IP连接
	if bestIP != "" {
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, _ := net.SplitHostPort(addr)
			// 只对目标主机使用优选IP
			if host == targetUrl.Hostname() {
				return dialer.DialContext(ctx, network, net.JoinHostPort(bestIP, port))
			}
			return dialer.DialContext(ctx, network, addr)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	req, err := http.NewRequest("GET", targetUrlStr, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 复制请求头
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://steamcommunity.com/")

	// Log the request
	startTime := time.Now()
	fmt.Printf("[Proxy] %s %s -> ", req.Method, targetUrlStr)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("%d (%v)\n", resp.StatusCode, time.Since(startTime))

	// 复制响应头
	for k, v := range resp.Header {
		// 跳过一些可能引起问题的头
		if strings.EqualFold(k, "Content-Encoding") {
			continue
		}
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
	w.WriteHeader(resp.StatusCode)

	io.Copy(w, resp.Body)
}
