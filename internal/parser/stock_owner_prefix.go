package parser

import "strings"

// 本体独占资源前缀通道（W7）：实体表里由**本体索引**推导出的"独占资源前缀"
// （sound/weapons/knife/**、materials/**/knife_t/**、materials/vgui/hud/icon_knife.* …）
// 也算证据：mod 里出现同前缀的文件 → 记该实体标签。
//
// 为什么需要它：开发文档 《mod-tag-recognition-v2》 §4.2 规划的"实体槽位"
// （worldmodel/viewmodel/script/sound/material）只落地了 model 锚点；非模型资源
// （武器音效 / 贴图目录 / HUD 图标）过去只能靠**手写关键词**，2026-10-07 的
// 「匕首」误标（裸关键词 knife 命中作者命名空间）就是这么来的。
//
// 这条通道把归属交给本体数据：前缀由 `--generate-entity-table` 用本体索引 + 脚本锚点
// 推导（只保留"全表唯一属于一个实体"的词），作者命名空间（materials/weapons/oe/knife_annihil/**）
// 不在本体索引里，因此永远不会命中。
//
// 与"本体基名 token 通道"的分工：token 通道按整段基名（rifle_ak47）匹配；
// 本通道按目录/文件 stem 前缀匹配，覆盖"目录名只是武器名一部分"的资源（音效目录最典型）。

type stockOwnerPrefixHit struct {
	Tag      string
	EntityID string
	IsItem   bool
}

var stockOwnerPrefixIndex = buildStockOwnerPrefixIndex()

func buildStockOwnerPrefixIndex() map[string]stockOwnerPrefixHit {
	index := make(map[string]stockOwnerPrefixHit)
	if stockEntities == nil {
		return index
	}
	for _, entity := range stockEntities.Entities {
		if entity.Tag == "" || len(entity.Slots) == 0 {
			continue
		}
		hit := stockOwnerPrefixHit{Tag: entity.Tag, EntityID: entity.ID, IsItem: entity.IsItem}
		for _, slot := range entity.Slots {
			prefix := strings.ToLower(strings.TrimSpace(slot.Prefix))
			if prefix == "" {
				continue
			}
			index[prefix] = hit
		}
	}
	return index
}

// stockOwnerPrefixHits 返回路径命中的独占资源前缀（按实体去重，最多两条）。
func stockOwnerPrefixHits(path string) []stockOwnerPrefixHit {
	if len(stockOwnerPrefixIndex) == 0 {
		return nil
	}
	path = strings.ToLower(strings.TrimSpace(path))
	if path == "" {
		return nil
	}
	// 与本体 token 通道同一口径：地图自带音乐 / 彩蛋目录不算武器/物品证据。
	if strings.HasPrefix(path, "sound/music") {
		return nil
	}

	hits := make([]stockOwnerPrefixHit, 0, 2)
	seen := make(map[string]struct{}, 2)
	add := func(prefix string) {
		hit, ok := stockOwnerPrefixIndex[prefix]
		if !ok {
			return
		}
		if _, dup := seen[hit.EntityID]; dup {
			return
		}
		seen[hit.EntityID] = struct{}{}
		hits = append(hits, hit)
	}
	// 目录边界逐级匹配：`sound/` → `sound/weapons/` → `sound/weapons/knife/`
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			add(path[:i+1])
		}
	}
	// 文件 stem（本体文件级前缀，例如 materials/vgui/hud/icon_knife）
	if dot := strings.LastIndex(path, "."); dot > strings.LastIndex(path, "/") {
		add(path[:dot])
	}
	return hits
}
