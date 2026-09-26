package app

import (
	"os"
	"path/filepath"
	"testing"
)

// 对齐 FireAxe `AddonChildrenProblem`：子组有问题时，父组行也要看得到。
func TestGroupMissingMembersAggregateToParents(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)

	// 结构：父组（成员都在）→ 子组（成员 b.vpk）→ 孙组（成员 c.vpk）。
	parent, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "a.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := a.CreateModStrategyGroupChild(parent.ID, "子组", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "b.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := a.CreateModStrategyGroupChild(child.ID, "孙组", modStrategyGroupAll, []string{
		filepath.Join(addonsDir, "c.vpk"),
	})
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]ModStrategyGroupMissingMembers{}
	collect := func() {
		t.Helper()
		summary, err := a.GetModStrategyGroupMissingMembers()
		if err != nil {
			t.Fatal(err)
		}
		byID = map[string]ModStrategyGroupMissingMembers{}
		for _, item := range summary {
			byID[item.GroupID] = item
		}
	}

	// 一开始全都在：一条都不该报。
	collect()
	if len(byID) != 0 {
		t.Fatalf("文件都在时不应报缺失: %+v", byID)
	}

	// 删掉孙组的成员：孙组自己报，子组与父组要带上"子树缺失"汇总。
	if err := os.Remove(filepath.Join(addonsDir, "c.vpk")); err != nil {
		t.Fatal(err)
	}
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	collect()

	grandchildEntry, ok := byID[grandchild.ID]
	if !ok {
		t.Fatalf("孙组应报自己的缺失: %+v", byID)
	}
	if grandchildEntry.MissingCount != 1 || grandchildEntry.SubtreeMissingCount != 0 {
		t.Fatalf("孙组应只报自己缺 1 个: %+v", grandchildEntry)
	}

	childEntry, ok := byID[child.ID]
	if !ok {
		t.Fatalf("子组应汇总子孙的缺失: %+v", byID)
	}
	if childEntry.MissingCount != 0 || childEntry.SubtreeMissingCount != 1 || childEntry.AffectedChildCount != 1 {
		t.Fatalf("子组应报「子树缺 1 个、影响 1 个子组」: %+v", childEntry)
	}

	parentEntry, ok := byID[parent.ID]
	if !ok {
		t.Fatalf("父组应汇总整棵子树的缺失: %+v", byID)
	}
	if parentEntry.MissingCount != 0 || parentEntry.SubtreeMissingCount != 1 || parentEntry.AffectedChildCount != 1 {
		t.Fatalf("父组应报子树缺 1 个、影响 1 个后代组: %+v", parentEntry)
	}
	if parentEntry.ParentID != "" {
		t.Fatalf("父组没有上级，ParentID 应为空: %+v", parentEntry)
	}
	if childEntry.ParentID != parent.ID {
		t.Fatalf("子组应带上 ParentID 便于界面分组: %+v", childEntry)
	}

	// 放回文件：全部恢复。
	writeTestVPK(t, filepath.Join(addonsDir, "c.vpk"), map[string][]byte{"materials/back.vtf": []byte("x")})
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatal(err)
	}
	collect()
	if len(byID) != 0 {
		t.Fatalf("文件放回后不应再报缺失: %+v", byID)
	}
}
