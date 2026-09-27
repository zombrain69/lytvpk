package serveraddress

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		expects string
	}{
		{name: "IPv4 默认端口", input: "127.0.0.1", expects: "127.0.0.1:27015"},
		{name: "域名默认端口", input: "example.com", expects: "example.com:27015"},
		{name: "空端口用默认", input: "example.com:", expects: "example.com:27015"},
		{name: "显式端口", input: "example.com:27016", expects: "example.com:27016"},
		{name: "裸 IPv6 默认端口", input: "2001:db8::1", expects: "[2001:db8::1]:27015"},
		{name: "带方括号 IPv6", input: "[2001:db8::1]", expects: "[2001:db8::1]:27015"},
		{name: "带方括号与端口", input: "[2001:db8::1]:27016", expects: "[2001:db8::1]:27016"},
		{name: "前后空白", input: "  example.com:27015  ", expects: "example.com:27015"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Normalize(test.input)
			if err != nil {
				t.Fatalf("Normalize 返回错误: %v", err)
			}
			if got != test.expects {
				t.Fatalf("期望 %q，实际 %q", test.expects, got)
			}
		})
	}
}

func TestNormalizeRejectsInvalidAddress(t *testing.T) {
	inputs := []string{
		"",
		":27015",
		"example.com:not-a-port",
		"example.com:0",
		"example.com:65536",
		"steam://connect/example.com",
		"[not-ipv6]",
		"2001:db8::invalid",
		"example.com/evil",
		"example.com?x=1",
		"example .com",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			if _, err := Normalize(input); err == nil {
				t.Fatalf("%q 应该被拒绝", input)
			}
		})
	}
}
