package network

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

// 工坊 DNS 设置（对齐上游 5ce7ef5 的最小版本：系统 DNS / 自定义 DNS 两档）。
//
// 为什么需要：工坊列表、详情、链接解析走的是域名（api.steampowered.com、工坊 worker 等），
// 国内网络下经常是"域名解析不通"而不是"线路慢"——这时候优选 IP 帮不上忙。
// 这一项只影响本应用发起的**工坊数据请求**，不改系统 DNS，也不影响 Mod 文件下载与图片加速
// （那两条链路仍然按"网络加速（优选 IP）"的规则走）。
type WorkshopDNSConfig struct {
	// Mode：system = 跟随系统 DNS（默认）；custom = 用下面的地址解析。
	Mode string `json:"mode"`
	// CustomAddress：自定义 DNS 服务器地址（IPv4 或 IPv6，不带端口）。
	CustomAddress string `json:"customAddress,omitempty"`
}

const (
	WorkshopDNSModeSystem = "system"
	WorkshopDNSModeCustom = "custom"
)

// DefaultWorkshopDNSConfig 默认跟随系统 DNS：不改变任何现有行为。
func DefaultWorkshopDNSConfig() WorkshopDNSConfig {
	return WorkshopDNSConfig{Mode: WorkshopDNSModeSystem}
}

// NormalizeWorkshopDNSConfig 归一化并校验工坊 DNS 配置。
// 自定义模式必须给出合法 DNS 地址，否则返回错误（宁可不保存，也不要写一个"看起来像"的坏地址）。
func NormalizeWorkshopDNSConfig(config WorkshopDNSConfig) (WorkshopDNSConfig, error) {
	mode := strings.ToLower(strings.TrimSpace(config.Mode))
	switch mode {
	case "", WorkshopDNSModeSystem:
		return DefaultWorkshopDNSConfig(), nil
	case WorkshopDNSModeCustom:
		address := strings.TrimSpace(config.CustomAddress)
		if !IsValidWorkshopDNSAddress(address) {
			return WorkshopDNSConfig{}, fmt.Errorf("自定义 DNS 地址不合法：%q（只填一个 IPv4 或 IPv6 地址，不要带端口/域名）", config.CustomAddress)
		}
		return WorkshopDNSConfig{Mode: WorkshopDNSModeCustom, CustomAddress: address}, nil
	default:
		return WorkshopDNSConfig{}, fmt.Errorf("未知的工坊 DNS 模式 %q", config.Mode)
	}
}

// IsValidWorkshopDNSAddress 判断字符串是不是一个可用的 DNS 服务器地址（不含端口）。
func IsValidWorkshopDNSAddress(address string) bool {
	value := strings.TrimSpace(address)
	if value == "" {
		return false
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return false
	}
	// 通配/组播/未指定地址不能当 DNS 服务器用。
	if ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	return true
}

// activeWorkshopResolver 保存当前生效的自定义解析器；nil = 用系统 DNS。
// 用原子指针而不是普通全局变量：设置来自设置页，读取来自多个网络请求 goroutine。
var activeWorkshopResolver atomic.Pointer[net.Resolver]

// SetWorkshopDNSResolver 应用 DNS 设置（由 app 层在配置载入/保存时调用）。
// 传 nil 表示恢复系统 DNS。设置立即对新请求生效，不需要重启。
func SetWorkshopDNSResolver(resolver *net.Resolver) {
	if resolver == nil {
		activeWorkshopResolver.Store(nil)
		return
	}
	activeWorkshopResolver.Store(resolver)
}

// WorkshopResolverFor 把工坊 DNS 配置转成解析器；系统模式返回 nil。
func WorkshopResolverFor(config WorkshopDNSConfig) (*net.Resolver, error) {
	normalized, err := NormalizeWorkshopDNSConfig(config)
	if err != nil {
		return nil, err
	}
	if normalized.Mode != WorkshopDNSModeCustom {
		return nil, nil
	}
	address := net.JoinHostPort(normalized.CustomAddress, "53")
	return &net.Resolver{
		// PreferGo：必须用 Go 自带解析器，才能把查询真正发到指定的 DNS 服务器；
		// 系统 cgo 解析器会忽略我们的 Dial。
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			// 先试 UDP，失败再交给 Go 解析器重试 TCP（network 参数由标准库给出）。
			return dialer.DialContext(ctx, network, address)
		},
	}, nil
}

// NewWorkshopDialer 返回"工坊数据请求"统一使用的 dialer：带当前 DNS 覆盖（如果有）。
// 每次调用都取一次当前设置，保证设置页保存后立刻对新请求生效。
func NewWorkshopDialer(timeout, keepAlive time.Duration) *net.Dialer {
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: keepAlive}
	if resolver := activeWorkshopResolver.Load(); resolver != nil {
		dialer.Resolver = resolver
	}
	return dialer
}

// WorkshopDNSEffectiveAddress 返回当前生效的 DNS 地址（系统模式返回空串），用于界面回显与状态提示。
func WorkshopDNSEffectiveAddress(config WorkshopDNSConfig) string {
	normalized, err := NormalizeWorkshopDNSConfig(config)
	if err != nil || normalized.Mode != WorkshopDNSModeCustom {
		return ""
	}
	return normalized.CustomAddress
}
