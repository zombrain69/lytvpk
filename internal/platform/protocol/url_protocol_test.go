package protocol

import "testing"

func TestParseProtocolURLParseSupportsMultipleIDs(t *testing.T) {
	got, err := ParseProtocolURL("lytvpk://parse/123456,234567")
	if err != nil {
		t.Fatalf("ParseProtocolURL returned error: %v", err)
	}

	if got.Action != ProtocolActionParse {
		t.Fatalf("expected parse action, got %q", got.Action)
	}
	if got.WorkshopID != "123456,234567" {
		t.Fatalf("expected normalized ids, got %q", got.WorkshopID)
	}
}

func TestParseProtocolURLParseSupportsEscapedComma(t *testing.T) {
	got, err := ParseProtocolURL("lytvpk://parse/123456%2C234567")
	if err != nil {
		t.Fatalf("ParseProtocolURL returned error: %v", err)
	}

	if got.WorkshopID != "123456,234567" {
		t.Fatalf("expected decoded ids, got %q", got.WorkshopID)
	}
}

func TestParseProtocolURLWorkshopRejectsMultipleIDs(t *testing.T) {
	if _, err := ParseProtocolURL("lytvpk://workshop/123456,234567"); err == nil {
		t.Fatal("expected multi-id workshop URL to be rejected")
	}
}

func TestParseWorkshopIDList(t *testing.T) {
	got, err := ParseWorkshopIDList("123456, 234567,123456")
	if err != nil {
		t.Fatalf("ParseWorkshopIDList returned error: %v", err)
	}

	want := []string{"123456", "234567"}
	if len(got) != len(want) {
		t.Fatalf("expected %d ids, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("id %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestParseWorkshopIDListRejectsInvalidID(t *testing.T) {
	if _, err := ParseWorkshopIDList("123456,abc"); err == nil {
		t.Fatal("expected invalid id to be rejected")
	}
}

// 对齐上游 c1b4972：外部程序/网页可以把服务器推进收藏列表。
func TestParseProtocolURLFavoriteServer(t *testing.T) {
	got, err := ParseProtocolURL("lytvpk://favoriteServer/My%20Server/example.com")
	if err != nil {
		t.Fatalf("ParseProtocolURL 返回错误: %v", err)
	}
	if got.Action != ProtocolActionFavoriteServer {
		t.Fatalf("action = %q", got.Action)
	}
	if got.ServerName != "My Server" {
		t.Fatalf("名称未解码: %q", got.ServerName)
	}
	if got.ServerAddress != "example.com:27015" {
		t.Fatalf("地址未补默认端口: %q", got.ServerAddress)
	}

	withPort, err := ParseProtocolURL("lytvpk://favoriteServer/%E6%B5%8B%E8%AF%95/127.0.0.1:27016")
	if err != nil {
		t.Fatalf("带端口解析失败: %v", err)
	}
	if withPort.ServerName != "测试" || withPort.ServerAddress != "127.0.0.1:27016" {
		t.Fatalf("解析结果错误: %+v", withPort)
	}
}

func TestParseProtocolURLFavoriteServerRejectsBadInput(t *testing.T) {
	cases := []string{
		"lytvpk://favoriteServer/only-name",            // 缺地址
		"lytvpk://favoriteServer//example.com",         // 名称为空
		"lytvpk://favoriteServer/name/example.com:abc", // 端口非法
		"lytvpk://favoriteServer/name/",                // 地址为空
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseProtocolURL(raw); err == nil {
				t.Fatalf("%q 应该被拒绝", raw)
			}
		})
	}
}
