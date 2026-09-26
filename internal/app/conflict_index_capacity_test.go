package app

import (
	"path/filepath"
	"testing"
)

// 真机实测（2409 个 Mod）：固定容量 1024 时热重算 ≈558ms，其中 435ms 是重复解析 ——
// 容量小于库规模，缓存每轮都在抖动。这里锁定"容量跟随候选数"的规则。
func TestConflictIndexCapacityFollowsCandidateCount(t *testing.T) {
	cases := []struct {
		candidates int
		want       int
	}{
		{0, conflictIndexCacheMin},
		{10, conflictIndexCacheMin},
		{conflictIndexCacheMin, conflictIndexCacheMin},
		{conflictIndexCacheMin + 1, conflictIndexCacheMin + 1},
		{2409, 2409},
		{conflictIndexCacheHardMax, conflictIndexCacheHardMax},
		{conflictIndexCacheHardMax + 1, conflictIndexCacheHardMax},
	}
	for _, testCase := range cases {
		if got := conflictIndexCapacityFor(testCase.candidates); got != testCase.want {
			t.Fatalf("候选 %d → 容量 %d，期望 %d", testCase.candidates, got, testCase.want)
		}
	}
}

// 容量设置会真的影响淘汰：容量 2 时只留 2 个条目，容量 5 时三个都在。
func TestConflictIndexCacheHonoursConfiguredCapacity(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 0, 3)
	for _, name := range []string{"a.vpk", "b.vpk", "c.vpk"} {
		path := filepath.Join(root, name)
		writeTestVPK(t, path, map[string][]byte{"materials/" + name + ".vtf": {1}})
		paths = append(paths, path)
	}

	a := &App{rootDir: root, configDir: t.TempDir()}

	// 容量 2：第三个条目会把最久未使用的挤出去
	a.setConflictIndexCacheLimit(2)
	for _, path := range paths {
		if _, err := a.getConflictFileList(path); err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
	}
	a.conflictIndexMu.Lock()
	smallCount := len(a.conflictIndexCache)
	a.conflictIndexMu.Unlock()
	if smallCount > 2 {
		t.Fatalf("容量为 2 时缓存条目不该超过 2，实际 %d", smallCount)
	}

	// 容量 5：全部保留（这正是大库修复后的行为）
	a.setConflictIndexCacheLimit(5)
	for _, path := range paths {
		if _, err := a.getConflictFileList(path); err != nil {
			t.Fatalf("再次解析 %s 失败: %v", path, err)
		}
	}
	a.conflictIndexMu.Lock()
	fullCount := len(a.conflictIndexCache)
	a.conflictIndexMu.Unlock()
	if fullCount != 3 {
		t.Fatalf("容量够大时应保留全部 3 个条目，实际 %d", fullCount)
	}

	// 未显式设置过容量时用最小值（保证老调用点行为不变）
	fresh := &App{rootDir: root, configDir: t.TempDir()}
	if got := fresh.conflictIndexCacheLimitSnapshot(); got != conflictIndexCacheMin {
		t.Fatalf("默认容量应为 %d，实际 %d", conflictIndexCacheMin, got)
	}
}

// 端到端：跑一次冲突检测后，容量会按候选数写入（夹具小 → 至少是最小值），
// 且所有候选都留在缓存里（没有抖动淘汰）。
func TestCheckConflictsSetsIndexCapacity(t *testing.T) {
	root := t.TempDir()
	writeHealthCheckFixture(t, root, "a.vpk", "b.vpk", "c.vpk")
	writeConflictTestAddonList(t, root, []conflictTestAddonListEntry{
		{Name: "a.vpk", Value: "1"},
		{Name: "b.vpk", Value: "1"},
		{Name: "c.vpk", Value: "1"},
	})
	a := newProfileTestApp(t, root)

	if _, err := a.CheckConflicts(); err != nil {
		t.Fatalf("冲突检测失败: %v", err)
	}

	candidates := len(collectConflictFilesystemPaths(root))
	if got := a.conflictIndexCacheLimitSnapshot(); got < conflictIndexCapacityFor(0) {
		t.Fatalf("容量不该低于最小值，实际 %d", got)
	}
	a.conflictIndexMu.Lock()
	cached := len(a.conflictIndexCache)
	a.conflictIndexMu.Unlock()
	if candidates > 0 && cached == 0 {
		t.Fatal("冲突检测后索引缓存不该为空")
	}
	if cached > conflictIndexCapacityFor(candidates) {
		t.Fatalf("缓存条目 %d 超过了本轮容量 %d", cached, conflictIndexCapacityFor(candidates))
	}
}
