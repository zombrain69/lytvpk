package app

import (
	"fmt"
	"strings"

	"vpk-manager/internal/parser"
)

// ModEvidence 是"只在详情弹窗里看"的依据数据。
//
// 为什么单独开一个接口：这些字段（尤其是标签依据）合计占了列表 payload 的 55%
// （真机 2904 个 Mod：tagEvidence 2.15MB + structure* 1.23MB，一次 IPC 340ms），
// 而列表一个都不用。列表只带轻字段，用户真的点开某个 Mod 的详情时再取这一份。
type ModEvidence struct {
	TagEvidence          []parser.TagEvidence `json:"tagEvidence"`
	StructureTopDirs     []string             `json:"structureTopDirs"`
	StructureSamplePaths []string             `json:"structureSamplePaths"`
	StructureTargets     []string             `json:"structureTargets"`
}

// GetModEvidence 返回单个 Mod 的标签依据与结构样本。
func (a *App) GetModEvidence(filePath string) (ModEvidence, error) {
	target := strings.TrimSpace(filePath)
	if target == "" {
		return ModEvidence{}, fmt.Errorf("缺少 Mod 路径")
	}
	cached, ok := a.vpkCache.Load(target)
	if !ok {
		return ModEvidence{}, fmt.Errorf("列表里找不到这个 Mod（可能已被移动或删除）")
	}
	cache, ok := cached.(*VPKFileCache)
	if !ok || cache == nil {
		return ModEvidence{}, fmt.Errorf("读取 Mod 依据失败")
	}
	file := cache.File
	return ModEvidence{
		TagEvidence:          append([]parser.TagEvidence(nil), file.TagEvidence...),
		StructureTopDirs:     append([]string(nil), file.StructureTopDirs...),
		StructureSamplePaths: append([]string(nil), file.StructureSamplePaths...),
		StructureTargets:     append([]string(nil), file.StructureTargets...),
	}, nil
}
