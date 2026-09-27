package app

import "testing"

// 对齐上游 c1b4972 的语义：外部协议添加收藏服务器，按规范化地址幂等。
func TestAddFavoriteServerNormalizesAndPersists(t *testing.T) {
	app := &App{configDir: t.TempDir()}

	added, err := app.addFavoriteServer("  我的服  ", "example.com")
	if err != nil {
		t.Fatalf("添加失败: %v", err)
	}
	if !added {
		t.Fatal("首次添加应返回 added=true")
	}

	storage := app.GetServerStorage()
	if len(storage.Servers) != 1 {
		t.Fatalf("收藏数量 = %d, 期望 1", len(storage.Servers))
	}
	got := storage.Servers[0]
	if got.Name != "我的服" {
		t.Fatalf("名称未去空白: %q", got.Name)
	}
	if got.Address != "example.com:27015" {
		t.Fatalf("地址未补默认端口: %q", got.Address)
	}
	if got.ID == "" {
		t.Fatal("应分配服务器 ID")
	}
}

func TestAddFavoriteServerIsIdempotentByAddress(t *testing.T) {
	app := &App{configDir: t.TempDir()}

	if _, err := app.addFavoriteServer("甲", "example.com:27015"); err != nil {
		t.Fatalf("添加失败: %v", err)
	}

	added, err := app.addFavoriteServer("乙（同名服务器）", "example.com")
	if err != nil {
		t.Fatalf("重复添加不应报错: %v", err)
	}
	if added {
		t.Fatal("同一地址（只差默认端口）不应重复收藏")
	}

	storage := app.GetServerStorage()
	if len(storage.Servers) != 1 {
		t.Fatalf("收藏数量 = %d, 期望 1（幂等）", len(storage.Servers))
	}
	if storage.Servers[0].Name != "甲" {
		t.Fatalf("已存在的记录不应被改写: %q", storage.Servers[0].Name)
	}
}

func TestAddFavoriteServerKeepsExistingEntries(t *testing.T) {
	app := &App{configDir: t.TempDir()}

	if err := app.SaveServerStorage(ServerStorage{
		Servers:       []SavedServer{{ID: "srv_existing", Name: "已有的服", Address: "old.example.com:27015"}},
		RecentServers: []RecentServer{},
	}); err != nil {
		t.Fatalf("准备夹具失败: %v", err)
	}

	if _, err := app.addFavoriteServer("新服", "127.0.0.1:27016"); err != nil {
		t.Fatalf("添加失败: %v", err)
	}

	storage := app.GetServerStorage()
	if len(storage.Servers) != 2 {
		t.Fatalf("收藏数量 = %d, 期望 2", len(storage.Servers))
	}
	addresses := map[string]bool{}
	for _, server := range storage.Servers {
		addresses[server.Address] = true
	}
	if !addresses["old.example.com:27015"] || !addresses["127.0.0.1:27016"] {
		t.Fatalf("地址集合异常: %v", addresses)
	}
}

func TestAddFavoriteServerRejectsInvalidInput(t *testing.T) {
	app := &App{configDir: t.TempDir()}

	if _, err := app.addFavoriteServer("   ", "example.com"); err == nil {
		t.Fatal("空名称应被拒绝")
	}
	if _, err := app.addFavoriteServer("服", "example.com:abc"); err == nil {
		t.Fatal("非法端口应被拒绝")
	}
	if _, err := app.addFavoriteServer("服", "steam://connect/example.com"); err == nil {
		t.Fatal("非服务器地址应被拒绝")
	}

	if storage := app.GetServerStorage(); len(storage.Servers) != 0 {
		t.Fatalf("失败时不应写入任何记录: %+v", storage.Servers)
	}
}
