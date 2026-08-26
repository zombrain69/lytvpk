package parser

import "l4d2-manager-next/pkg/valve/vpk"

// DetermineVPKType 确定VPK的主要类型
func DetermineVPKType(archive *vpk.Archive) string {
	return determineVPKType(buildArchivePathIndex(archive))
}

func determineVPKType(index archivePathIndex) string {
	// 地图的 .bsp 是强证据，维持既有的优先级。
	if index.hasMap {
		return "地图"
	}
	if len(index.characterFiles) > 0 {
		return "人物"
	}
	if len(index.weaponFiles) > 0 {
		return "武器"
	}

	return "其他"
}
