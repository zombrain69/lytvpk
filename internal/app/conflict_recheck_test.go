package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConflictRecheckLazilyRecomputesOnce 覆盖"脏标记 + 按需重算一次"。
func TestConflictRecheckLazilyRecomputesOnce(t *testing.T) {
	a, _ := newPriorityTestApp(t)

	initial := a.GetConflictRecheckStatus()
	if !initial.Dirty {
		t.Fatal("新建的 App 应先处于待复检状态")
	}

	badges, err := a.GetConflictBadges()
	if err != nil {
		t.Fatalf("get conflict badges: %v", err)
	}
	if len(badges) == 0 {
		t.Fatal("示例夹具应至少产生一个冲突角标")
	}

	after := a.GetConflictRecheckStatus()
	if after.Dirty {
		t.Fatal("重算后应清除脏标记")
	}
	if after.RecomputeCount != initial.RecomputeCount+1 {
		t.Fatalf("重算次数 = %d, want %d", after.RecomputeCount, initial.RecomputeCount+1)
	}
	if after.LastRun == "" {
		t.Fatal("重算后应记录时间")
	}

	// 缓存有效时不应重复扫描。
	if _, err := a.GetConflictBadges(); err != nil {
		t.Fatal(err)
	}
	again := a.GetConflictRecheckStatus()
	if again.RecomputeCount != after.RecomputeCount {
		t.Fatalf("缓存有效时不应重复扫描: %d → %d", after.RecomputeCount, again.RecomputeCount)
	}
}

// TestConflictRecheckInvalidatedByMutations 覆盖任务要求的各个触发点。
func TestConflictRecheckInvalidatedByMutations(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if _, err := a.GetConflictBadges(); err != nil {
		t.Fatal(err)
	}
	if status := a.GetConflictRecheckStatus(); status.Dirty {
		t.Fatal("前置条件：重算后应为干净状态")
	}

	// 1. 加载顺序写入（写 addonlist.txt）
	if err := a.SetVPKLoadOrder(filepath.Join(addonsDir, "a.vpk"), 2); err != nil {
		t.Fatalf("set game enabled: %v", err)
	}
	if status := a.GetConflictRecheckStatus(); !status.Dirty {
		t.Fatal("游戏内开关写入后应标记为待复检")
	}
	if !strings.Contains(a.GetConflictRecheckStatus().Reason, "addonlist") {
		t.Fatalf("脏标记原因 = %q", a.GetConflictRecheckStatus().Reason)
	}
	if _, err := a.GetConflictBadges(); err != nil {
		t.Fatal(err)
	}

	// 2. 优先级分层（只写 priority.json，不影响游戏侧状态）不应触发复检。
	if _, err := a.SetModPriority("a.vpk", "a.vpk", -1); err != nil {
		t.Fatal(err)
	}
	if status := a.GetConflictRecheckStatus(); status.Dirty {
		t.Fatal("仅保存分层不应让游戏侧复检失效（判定结果在读取时会重新计算）")
	}

	// 3. 文件重命名会改变 addonlist 键与资源归属。
	renamed, err := a.RenameVPKFile(filepath.Join(addonsDir, "b.vpk"), "b-renamed.vpk")
	if err != nil {
		t.Fatalf("rename vpk: %v", err)
	}
	if status := a.GetConflictRecheckStatus(); !status.Dirty {
		t.Fatal("文件重命名后应标记为待复检")
	}

	// 4. 文件删除同样需要失效。
	if _, err := a.GetConflictBadges(); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteVPKFile(renamed); err != nil {
		t.Fatalf("delete vpk: %v", err)
	}
	if status := a.GetConflictRecheckStatus(); !status.Dirty {
		t.Fatal("文件删除后应标记为待复检")
	}
}

// TestConflictFixSuggestionsPreferUserConfirmation 覆盖修复建议的生成规则。
func TestConflictFixSuggestionsPreferUserConfirmation(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if err := os.Remove(filepath.Join(addonsDir, "c.vpk")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a.vpk", "b.vpk"} {
		if _, err := a.SetModPriority(key, key, 4); err != nil {
			t.Fatal(err)
		}
	}

	suggestions, err := a.GetConflictFixSuggestions()
	if err != nil {
		t.Fatalf("get suggestions: %v", err)
	}
	var sameLayer *ConflictFixSuggestion
	for index := range suggestions {
		if suggestions[index].Kind == "conflict" {
			sameLayer = &suggestions[index]
			break
		}
	}
	if sameLayer == nil {
		t.Fatalf("同层冲突应产生 set-tier 建议: %#v", suggestions)
	}
	if sameLayer.Action != "set-tier" || sameLayer.SuggestedTier == nil || *sameLayer.SuggestedTier != 3 {
		t.Fatalf("同层冲突建议 = %#v", *sameLayer)
	}
	if sameLayer.TargetKey == "" || sameLayer.FileCount == 0 {
		t.Fatalf("建议缺少目标或文件数: %#v", *sameLayer)
	}

	// 覆盖关系给出"想反过来该怎么做"的建议。
	var override *ConflictFixSuggestion
	for index := range suggestions {
		if suggestions[index].Kind == "override" {
			override = &suggestions[index]
			break
		}
	}
	if override == nil {
		t.Fatalf("覆盖关系应给出可选的调整建议: %#v", suggestions)
	}
	if override.Action != "set-tier" || override.SuggestedTier == nil {
		t.Fatalf("覆盖建议 = %#v", *override)
	}
	if !strings.Contains(override.Summary, "覆盖") {
		t.Fatalf("覆盖建议文案 = %q", override.Summary)
	}

	// 关键约束：生成建议不会改动 addonlist.txt。
	before := readAddonListBytes(t, filepath.Join(filepath.Dir(a.rootDir), "addonlist.txt"))
	if _, err := a.GetConflictFixSuggestions(); err != nil {
		t.Fatal(err)
	}
	after := readAddonListBytes(t, filepath.Join(filepath.Dir(a.rootDir), "addonlist.txt"))
	if string(before) != string(after) {
		t.Fatal("生成修复建议不应改写 addonlist.txt")
	}
}

// TestConflictFixSuggestionsFlagUnregisteredMod 覆盖"未写入 addonlist"的建议分支。
func TestConflictFixSuggestionsFlagUnregisteredMod(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	// 覆盖组里 a.vpk 与工坊条目都在 addonlist 中，因此构造未记录参与者的冲突。
	suggestions, err := a.GetConflictFixSuggestions()
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) == 0 {
		t.Fatal("未记录参与者应给出至少一条建议")
	}
	found := false
	for _, suggestion := range suggestions {
		if suggestion.Kind == "unknown-order" && suggestion.Action == "register" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应提示把未记录的 Mod 先写入 addonlist: %#v", suggestions)
	}
}
