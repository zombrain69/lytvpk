package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	netcfg "vpk-manager/internal/network"
)

// 工坊网络设置（当前只有 DNS 一项，对齐上游 5ce7ef5 的最小版本）。
//
// 口径：只影响"工坊数据访问"——工坊列表、详情、链接解析、依赖/合集解析这些域名请求；
// Mod 文件下载与工坊图片仍然走"网络加速（优选 IP）"那条链路。
// 目的很具体：国内网络下工坊打不开，很多时候是域名解析问题，优选 IP 解决不了。

// GetWorkshopDNSConfig 返回当前工坊 DNS 设置（只读）。
func (a *App) GetWorkshopDNSConfig() netcfg.WorkshopDNSConfig {
	a.mu.RLock()
	config := a.workshopDNSConfig
	a.mu.RUnlock()
	if config.Mode == "" {
		config = netcfg.DefaultWorkshopDNSConfig()
	}
	return config
}

// SetWorkshopDNSConfig 校验并保存工坊 DNS 设置，立即对新请求生效（不需要重启）。
func (a *App) SetWorkshopDNSConfig(config netcfg.WorkshopDNSConfig) (netcfg.WorkshopDNSConfig, error) {
	normalized, err := netcfg.NormalizeWorkshopDNSConfig(config)
	if err != nil {
		return netcfg.WorkshopDNSConfig{}, err
	}
	a.mu.Lock()
	a.workshopDNSConfig = normalized
	a.mu.Unlock()

	a.applyWorkshopDNSResolver()
	a.saveConfig()
	return normalized, nil
}

// applyWorkshopDNSResolver 把当前设置应用到 network 包（nil = 系统 DNS）。
// 设置里的地址非法时退回系统 DNS 并记日志：宁可退化成默认行为，也不要让工坊彻底不可用。
func (a *App) applyWorkshopDNSResolver() {
	config := a.GetWorkshopDNSConfig()
	resolver, err := netcfg.WorkshopResolverFor(config)
	if err != nil {
		log.Printf("工坊 DNS 设置无效（%v），已退回系统 DNS", err)
		netcfg.SetWorkshopDNSResolver(nil)
		return
	}
	netcfg.SetWorkshopDNSResolver(resolver)
}

// DescribeWorkshopDNS 生成给界面/日志用的一句话说明。
func DescribeWorkshopDNS(config netcfg.WorkshopDNSConfig) string {
	normalized, err := netcfg.NormalizeWorkshopDNSConfig(config)
	if err != nil {
		return fmt.Sprintf("配置无效：%v", err)
	}
	if normalized.Mode == netcfg.WorkshopDNSModeCustom {
		return fmt.Sprintf("自定义 DNS %s", normalized.CustomAddress)
	}
	return "系统 DNS"
}

// newWorkshopDataClient 返回走"工坊 DNS 设置"的 HTTP 客户端（工坊列表/详情/依赖解析等）。
//
// 为什么不用 http.DefaultClient：默认客户端用的是系统解析器，自定义 DNS 就落不了地；
// 这里和工坊 worker 客户端共用同一套 dialer，保证"改一次设置，所有工坊数据请求都生效"。
func newWorkshopDataClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		// 闭包参数不叫 network，避免遮蔽 netcfg 包名。
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return netcfg.NewWorkshopDialer(30*time.Second, 30*time.Second).DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}
