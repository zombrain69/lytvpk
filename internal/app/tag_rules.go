package app

import (
	"vpk-manager/internal/gamedata/entities"
	"vpk-manager/internal/gamedata/rules"
)

// ValidateTagRules 用本体索引校验「声明式标签规则表」：
// 前缀解析 0 命中是错误（规则永远不可能生效 → 少标根因），
// 关键词在本体里 0 命中是警告（可能只匹配作者命名空间）。
//
// 本体索引不可用时返回的 Report 会带 Notes 说明「无法校验」，调用方应据此区分
// 「规则坏了」与「没有游戏目录」。
func (a *App) ValidateTagRules() (rules.Report, error) {
	table, err := rules.Load()
	if err != nil {
		return rules.Report{}, err
	}
	return rules.Validate(table, a.loadStockIndex(), entities.MustLoad()), nil
}
