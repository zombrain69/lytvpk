package app

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestModIgnoreFilesPersistNormalizeAndDelete 覆盖存储层行为。
func TestModIgnoreFilesPersistNormalizeAndDelete(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	record, err := a.SetModIgnoreFiles("A.VPK", "a.vpk", []string{
		" Materials/Shared.VTF ",
		"materials/shared.vtf",
		"# 注释行",
		"",
		"models/",
	})
	if err != nil {
		t.Fatalf("set mod ignore files: %v", err)
	}
	if record.Key != "a.vpk" {
		t.Fatalf("归一化键 = %q, want a.vpk", record.Key)
	}
	want := []string{"materials/shared.vtf", "models/"}
	if !reflect.DeepEqual(record.Files, want) {
		t.Fatalf("归一化清单 = %#v, want %#v", record.Files, want)
	}

	reloaded, err := a.GetModIgnoreFiles("a.vpk")
	if err != nil {
		t.Fatalf("get mod ignore files: %v", err)
	}
	if !reflect.DeepEqual(reloaded, want) {
		t.Fatalf("重新读取的清单 = %#v, want %#v", reloaded, want)
	}

	// 其它 Mod 读取不到别人清单。
	other, err := a.GetModIgnoreFiles("b.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("b.vpk 不应读到 a.vpk 的清单: %#v", other)
	}

	records, err := a.ListModIgnoreRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Key != "a.vpk" {
		t.Fatalf("记录列表 = %#v", records)
	}

	// 空清单等价于删除该记录。
	if _, err := a.SetModIgnoreFiles("a.vpk", "a.vpk", nil); err != nil {
		t.Fatal(err)
	}
	records, err = a.ListModIgnoreRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("空清单应删除记录: %#v", records)
	}
	cleared, err := a.GetModIgnoreFiles("a.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("删除后清单应为空: %#v", cleared)
	}
}

// TestModIgnoreFilesOnlyAffectThatMod 是本特性的核心语义：
// 单 Mod 清单只让该 Mod 退出它参与的重叠判定，不影响其它 Mod 之间的判定。
func TestModIgnoreFilesOnlyAffectThatMod(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	// a.vpk 声明自己不提供 materials/shared.vtf；该路径仍由 b.vpk / c.vpk / 工坊条目提供。
	if _, err := a.SetModIgnoreFiles("a.vpk", "a.vpk", []string{"materials/shared.vtf"}); err != nil {
		t.Fatal(err)
	}

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatalf("check conflicts: %v", err)
	}

	for _, group := range result.ConflictGroups {
		for _, file := range group.VpkFiles {
			if file.Name == "a.vpk" {
				t.Fatalf("a.vpk 已声明忽略 materials/shared.vtf，不应再出现在该重叠里: %#v", group.VpkFiles)
			}
		}
	}
	// b.vpk / c.vpk / workshop\123.vpk 之间的重叠仍然存在（c 未记录 → 冲突）。
	if len(result.ConflictGroups) != 1 {
		t.Fatalf("其它 Mod 之间的重叠不应被影响: %#v", result.ConflictGroups)
	}
	if got := len(result.ConflictGroups[0].VpkFiles); got != 3 {
		t.Fatalf("剩余参与者应为 3 个，实际 %d: %#v", got, result.ConflictGroups[0].VpkFiles)
	}
	// materials/decided.vtf 只有 a 与工坊条目提供，且 a 的清单没有忽略它：
	// 该覆盖关系必须保持不变（证明单 Mod 清单不会外溢到其它路径）。
	if result.TotalOverrides != 1 || result.OverrideGroups[0].Winner.Name != "123.vpk" {
		t.Fatalf("未被忽略的覆盖关系不应受影响: %#v", result.OverrideGroups)
	}
	if files := result.OverrideGroups[0].Files; !reflect.DeepEqual(files, []string{"materials/decided.vtf"}) {
		t.Fatalf("覆盖文件 = %#v", files)
	}
}

// TestModIgnoreAnnotationsExplainSkippedOverlaps 覆盖"标注被自身规则忽略的项"。
func TestModIgnoreAnnotationsExplainSkippedOverlaps(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if _, err := a.SetModIgnoreFiles(`workshop\123.vpk`, `workshop\123.vpk`, []string{"materials/decided.vtf"}); err != nil {
		t.Fatal(err)
	}

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatalf("check conflicts: %v", err)
	}
	if result.TotalModIgnoreAnnotations != 1 {
		t.Fatalf("应有 1 条被自身规则跳过的标注: %#v", result.ModIgnoreAnnotations)
	}
	annotation := result.ModIgnoreAnnotations[0]
	if annotation.File != "materials/decided.vtf" {
		t.Fatalf("标注路径 = %q", annotation.File)
	}
	if len(annotation.VpkFiles) != 1 || annotation.VpkFiles[0].Name != "123.vpk" {
		t.Fatalf("标注的 Mod = %#v", annotation.VpkFiles)
	}
	if result.TotalOverrides != 0 {
		t.Fatalf("被自身规则跳过的重叠不应计入覆盖关系: %#v", result.OverrideGroups)
	}
}

// TestGlobalIgnoreListStillAppliesWithModIgnores 保证内置规则与全局清单仍然是并集的一部分。
func TestGlobalIgnoreListStillAppliesWithModIgnores(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	writeTestVPK(t, filepath.Join(addonsDir, "d.vpk"), map[string][]byte{
		"materials/shared.vtf":    []byte("d"),
		"materials/global.vtf":    []byte("d"),
		"materials/permod.vtf":    []byte("d"),
		"materials/untouched.vtf": []byte("d"),
	})
	writeTestVPK(t, filepath.Join(addonsDir, "e.vpk"), map[string][]byte{
		"materials/global.vtf":    []byte("e"),
		"materials/permod.vtf":    []byte("e"),
		"materials/untouched.vtf": []byte("e"),
	})
	if _, err := a.SetModIgnoreFiles("d.vpk", "d.vpk", []string{"materials/permod.vtf"}); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.conflictIgnoreFiles = []string{"materials/global.vtf"}
	a.mu.Unlock()

	result, err := a.CheckConflictsWithOptions(ConflictAnalysisOptions{FullScan: true, PriorityAware: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, group := range result.ConflictGroups {
		for _, file := range group.Files {
			seen[file] = true
		}
	}
	for _, ignored := range []string{"materials/global.vtf", "materials/permod.vtf"} {
		if seen[ignored] {
			t.Fatalf("全局清单与单 Mod 清单都应与内置规则取并集，%s 不应出现: %#v", ignored, seen)
		}
	}
	if !seen["materials/untouched.vtf"] {
		t.Fatalf("未被任何清单覆盖的重叠应保留: %#v", seen)
	}
}
