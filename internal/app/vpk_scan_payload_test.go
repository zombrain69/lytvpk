package app

import (
	"path/filepath"
	"testing"

	"vpk-manager/internal/parser"
)

// 列表 IPC 不带"依据类"重字段，但缓存与详情接口必须还留着。
//
// 真机背景（2904 个 Mod）：完整 payload 6.11MB / 一次 IPC 340ms，其中
// tagEvidence 2.15MB + structure* 1.23MB = 55%，而列表一个都不用 ——
// tagEvidence 只在详情弹窗显示，structure* 只有 Go 侧的分组目录导出在用。
// 这条用例同时守住两件事：列表确实剥掉了、剥的时候没有把缓存改坏。
func TestListPayloadDropsEvidenceButCacheKeepsIt(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	path := filepath.Join(addonsDir, "a.vpk")
	a.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
		Path:     path,
		Name:     "a.vpk",
		Location: "root",
		Enabled:  true,
		TagEvidence: []parser.TagEvidence{{
			Tag:    "材质",
			Rule:   "materials/vtf",
			Level:  "exact",
			Source: []string{"materials/shared.vtf"},
		}},
		StructureTopDirs:     []string{"materials (3)"},
		StructureSamplePaths: []string{"materials/shared.vtf"},
		StructureTargets:     []string{"props_shared"},
	}})

	list := a.GetVPKFiles()
	if len(list) != 1 {
		t.Fatalf("列表应返回 1 条，实际 %d", len(list))
	}
	if len(list[0].TagEvidence) != 0 {
		t.Fatalf("列表 payload 不应带 tagEvidence（占 35%% 体积）：%#v", list[0].TagEvidence)
	}
	if len(list[0].StructureTopDirs) != 0 ||
		len(list[0].StructureSamplePaths) != 0 ||
		len(list[0].StructureTargets) != 0 {
		t.Fatalf("列表 payload 不应带 structure* 字段：%#v", list[0])
	}

	// 缓存里的原始数据不能被"剥离副本"改坏。
	cached := a.mustCachedVPKFile(t, path)
	if len(cached.TagEvidence) != 1 || len(cached.StructureTopDirs) != 1 {
		t.Fatalf("剥离必须只作用于副本，缓存被改坏了：%#v", cached)
	}

	// 内部快照（问题扫描 / 随机轮换）仍然拿得到完整对象。
	snapshot := a.allVPKFilesSnapshot()
	if len(snapshot) != 1 || len(snapshot[0].TagEvidence) != 1 {
		t.Fatalf("内部快照应保留完整字段：%#v", snapshot)
	}

	// 详情弹窗按需取的接口必须给出同一份数据。
	evidence, err := a.GetModEvidence(path)
	if err != nil {
		t.Fatalf("GetModEvidence: %v", err)
	}
	if len(evidence.TagEvidence) != 1 || evidence.TagEvidence[0].Tag != "材质" {
		t.Fatalf("详情接口应返回标签依据：%#v", evidence.TagEvidence)
	}
	if len(evidence.StructureTopDirs) != 1 || len(evidence.StructureSamplePaths) != 1 ||
		len(evidence.StructureTargets) != 1 {
		t.Fatalf("详情接口应返回结构样本：%#v", evidence)
	}
}

func TestGetModEvidenceRejectsUnknownPath(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if _, err := a.GetModEvidence(filepath.Join("nope", "missing.vpk")); err == nil {
		t.Fatalf("未知路径应报错，而不是返回空对象")
	}
	if _, err := a.GetModEvidence("   "); err == nil {
		t.Fatalf("空路径应报错")
	}
}
