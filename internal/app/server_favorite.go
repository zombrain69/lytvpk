package app

import (
	"fmt"
	"strings"

	"vpk-manager/internal/serveraddress"
)

// addFavoriteServer 把 `lytvpk://favoriteServer/...` 协议传入的服务器写进收藏列表。
//
// 对齐上游 `feat 添加新的favoriteServer外部协议`（c1b4972）：
//   - 地址统一规范化（缺端口补 27015），规范化后的地址是幂等键 —— 同一台服务器
//     重复点深链不会生成重复记录；
//   - 已存在时返回 added=false（不是错误），前端据此提示"已收藏过"。
func (a *App) addFavoriteServer(name string, address string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("服务器名称不能为空")
	}
	normalizedAddress, err := serveraddress.Normalize(address)
	if err != nil {
		return false, err
	}

	a.serverStorageMu.Lock()
	defer a.serverStorageMu.Unlock()

	storage := a.GetServerStorage()
	target := normalizeStoredAddress(normalizedAddress)
	for _, server := range storage.Servers {
		if normalizeStoredAddress(server.Address) == target {
			return false, nil
		}
	}

	storage.Servers = append(storage.Servers, SavedServer{
		ID:      newServerID(),
		Name:    name,
		Address: normalizedAddress,
		Weight:  0,
	})
	if err := a.SaveServerStorage(storage); err != nil {
		return false, fmt.Errorf("保存服务器收藏失败: %w", err)
	}
	return true, nil
}
