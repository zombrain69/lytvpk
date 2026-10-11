package parser

import (
	"strings"
)

// 多分片地图配套包（路线图第 8 项①，2026-10-12 落地）。
//
// 背景：部分自定义战役的作者把同一张地图拆成多个工坊作品分开发布 ——
// 主包（含 maps/*.bsp 与 missions 任务文件）加上若干“配套包”（材质 / 模型 /
// 语音 / 贴图等资产，没有 .bsp）。真机样本（2026-10-12 只读全库扫描）：
//
//   - Dead Before Dawn (Extended)：主包 2083812352（地图）+ Part 2（2083814432）/
//     Part 3（2083817728）/ Part 4（2852920241），三个分片都没有 BSP；
//   - 连接 Connection：part 1（2717267221，地图）+ part 2（2951512228，材质与模型）；
//   - 宜昌2：主图 3578078027 + 贴图包 3577759004；
//   - 3583381403：材质 + missions/gx_ls_lfswj.txt（同系列地图的配套素材包）。
//
// 这些分片包里带着**与主包同源的自定义战役任务文件**（missions/*.txt，文件名
// 不是原版风格的 campaign<数字>.txt）——这正是“它属于某张地图”的可判定证据。
// 过去它们会按资产内容落到「人物 / 武器 / 其他」，在「地图」分类下不可见
// （对照 fdklgbh/L4D2 Mod Manager 的已知问题：“多分片地图，某个 part 没有
// maps/ 或 missions/，会被分错类”）。
//
// 本文件只做一件事：把「无 BSP + 带自定义战役任务文件」的包升级为主分类
// 「地图」，并打上「地图分片」标记；战役 / 模式证据复用 collectMissionEvidence。
// 注意：分片包不执行 dropCoincidenceOnlyTags（那条规则针对“完整地图自带的
// 场景道具”，分片包不是完整地图；跳过同时守住“标签只增不减”的硬约束）。

const (
	// mapCompanionTag 标记“这是多分片地图的配套包，需与地图本体一起启用”。
	mapCompanionTag = "地图分片"
	// mapConfigTag 是 D9 引入的 missions 兜底标签；分片包同样保留它，
	// 保证原本是「其他」主类型的分片包不因升级而丢标签。
	mapConfigTag = "地图配置"
)

// isMultipartMapCompanion 判定「多分片地图配套包」：本体没有 .bsp（不是完整地图），
// 且至少带一个自定义战役任务文件。
func isMultipartMapCompanion(index archivePathIndex) bool {
	if index.hasMap || len(index.missionFiles) == 0 {
		return false
	}
	for _, file := range index.missionFiles {
		if isCustomCampaignMissionPath(normalizeArchivePath(file.Name())) {
			return true
		}
	}
	return false
}

// isCustomCampaignMissionPath 判定任务文件是否属于“自定义战役”：
// 位于 missions/ 目录下、扩展名为 .txt，且文件名不是原版风格的 campaign<数字>。
// 原版任务文件（campaign1..campaign14）只说明包在覆盖本体战役（D9 的 missions-only
// 包，例如 2239816060.vpk），不能作为地图分片证据。
func isCustomCampaignMissionPath(name string) bool {
	if !strings.HasSuffix(name, ".txt") || !strings.HasPrefix(name, "missions/") {
		return false
	}
	stem := missionFileStem(name)
	if stem == "" {
		return false
	}
	return !isVanillaCampaignStem(stem)
}

// isVanillaCampaignStem 判定任务文件名去掉扩展名后是否为原版 campaign<数字> 风格。
func isVanillaCampaignStem(stem string) bool {
	lower := strings.ToLower(strings.TrimSpace(stem))
	if !strings.HasPrefix(lower, "campaign") {
		return false
	}
	digits := strings.TrimPrefix(lower, "campaign")
	if digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
