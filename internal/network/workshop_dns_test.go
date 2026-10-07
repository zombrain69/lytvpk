package network

import (
	"testing"
	"time"
)

func TestNormalizeWorkshopDNSConfig(t *testing.T) {
	cases := []struct {
		name    string
		input   WorkshopDNSConfig
		want    WorkshopDNSConfig
		wantErr bool
	}{
		{name: "空配置=系统 DNS", input: WorkshopDNSConfig{}, want: DefaultWorkshopDNSConfig()},
		{name: "system 模式", input: WorkshopDNSConfig{Mode: "SYSTEM"}, want: DefaultWorkshopDNSConfig()},
		{name: "自定义 IPv4", input: WorkshopDNSConfig{Mode: "custom", CustomAddress: " 119.29.29.29 "},
			want: WorkshopDNSConfig{Mode: WorkshopDNSModeCustom, CustomAddress: "119.29.29.29"}},
		{name: "自定义 IPv6", input: WorkshopDNSConfig{Mode: "custom", CustomAddress: "2001:4860:4860::8888"},
			want: WorkshopDNSConfig{Mode: WorkshopDNSModeCustom, CustomAddress: "2001:4860:4860::8888"}},
		{name: "自定义缺地址", input: WorkshopDNSConfig{Mode: "custom"}, wantErr: true},
		{name: "自定义填了域名", input: WorkshopDNSConfig{Mode: "custom", CustomAddress: "dns.alidns.com"}, wantErr: true},
		{name: "自定义带端口", input: WorkshopDNSConfig{Mode: "custom", CustomAddress: "119.29.29.29:53"}, wantErr: true},
		{name: "自定义通配地址", input: WorkshopDNSConfig{Mode: "custom", CustomAddress: "0.0.0.0"}, wantErr: true},
		{name: "未知模式", input: WorkshopDNSConfig{Mode: "dnspod"}, wantErr: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := NormalizeWorkshopDNSConfig(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际得到 %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("归一化失败: %v", err)
			}
			if got != test.want {
				t.Fatalf("归一化 = %#v，期望 %#v", got, test.want)
			}
		})
	}
}

func TestWorkshopResolverFor(t *testing.T) {
	resolver, err := WorkshopResolverFor(DefaultWorkshopDNSConfig())
	if err != nil {
		t.Fatalf("系统模式不该报错: %v", err)
	}
	if resolver != nil {
		t.Fatal("系统模式必须返回 nil（走默认解析器）")
	}

	resolver, err = WorkshopResolverFor(WorkshopDNSConfig{Mode: WorkshopDNSModeCustom, CustomAddress: "223.5.5.5"})
	if err != nil {
		t.Fatalf("自定义模式: %v", err)
	}
	if resolver == nil || !resolver.PreferGo || resolver.Dial == nil {
		t.Fatalf("自定义模式必须给出带 Dial 的 Go 解析器，实际 %#v", resolver)
	}
}

func TestNewWorkshopDialerUsesActiveResolver(t *testing.T) {
	t.Cleanup(func() { SetWorkshopDNSResolver(nil) })

	SetWorkshopDNSResolver(nil)
	if dialer := NewWorkshopDialer(time.Second, time.Second); dialer.Resolver != nil {
		t.Fatal("系统 DNS 模式下不应设置自定义解析器")
	}

	custom, err := WorkshopResolverFor(WorkshopDNSConfig{Mode: WorkshopDNSModeCustom, CustomAddress: "119.29.29.29"})
	if err != nil {
		t.Fatal(err)
	}
	SetWorkshopDNSResolver(custom)
	dialer := NewWorkshopDialer(10*time.Second, 30*time.Second)
	if dialer.Resolver == nil {
		t.Fatal("保存自定义 DNS 后，新 dialer 必须带上解析器（设置立即生效）")
	}
	if dialer.Timeout != 10*time.Second || dialer.KeepAlive != 30*time.Second {
		t.Fatalf("dialer 超时参数没保留：%#v", dialer)
	}

	// 自定义地址必须是 DNS 服务器地址而不是原目标地址：这里只验证解析器确实是我们给的那个。
	if dialer.Resolver != custom {
		t.Fatal("dialer 应使用当前生效的解析器")
	}

	SetWorkshopDNSResolver(nil)
	if dialer := NewWorkshopDialer(time.Second, time.Second); dialer.Resolver != nil {
		t.Fatal("清空设置后必须回到系统 DNS")
	}
}
