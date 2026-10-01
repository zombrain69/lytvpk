package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 批量存在性判定（vault key index）必须与逐键判定逐字一致。
//
// 背景（真机量化）：modKeyExistsInVault 原先每个键都要遍历整个 vpkCache ——
// 212 个策略组 / 1964 条成员 × 2904 个已缓存文件 ≈ 570 万次路径推导，
// 单次调用实测 1.6 秒。GetModGroupMembership（每次搜索都会在后台刷一次归属）
// 与 GetModStrategyGroupMissingMembers（每次打开「策略组管理」都会调）
// 都被它拖慢，用户侧表现为"点一下要等一两秒"。
func TestVaultKeyIndexMatchesSingleKeyLookup(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "123.vpk"), VPKFile{
		Name:     "123.vpk",
		Location: "workshop",
	})
	// disabled 目录里的文件在 addonlist 里仍记成裸文件名（见
	// addonListKeyForManagedVPKPathFromRoot 的 TrimPrefix("disabled\\")）。
	seedCachedMod(a, filepath.Join(addonsDir, "disabled", "old.vpk"), VPKFile{
		Name:     "old.vpk",
		Location: "disabled",
	})

	index := a.buildVaultKeyIndex(a.rootDirectorySnapshot())
	cases := []struct {
		key  string
		want bool
		why  string
	}{
		{"a.vpk", true, "根目录文件已缓存"},
		{"A.VPK", true, "大小写归一化后命中"},
		{`workshop\123.vpk`, true, "工坊条目的 addonlist 键"},
		{`WORKSHOP/123.VPK`, true, "斜杠与大小写都归一化"},
		{"old.vpk", true, "disabled 目录里的文件按裸名记账"},
		{"c.vpk", true, "缓存没扫到，但物理文件存在（兜底 stat）"},
		{"zzz.vpk", false, "缓存与磁盘都没有"},
		{"", false, "空键"},
	}
	for _, tc := range cases {
		single := a.modKeyExistsInVault(tc.key)
		batch := a.modKeyExistsInVaultIndexed(index, tc.key)
		if single != batch {
			t.Fatalf("键 %q 批量判定与逐键判定不一致: single=%v batch=%v", tc.key, single, batch)
		}
		if single != tc.want {
			t.Fatalf("键 %q 判定应为 %v（%s），实际 %v", tc.key, tc.want, tc.why, single)
		}
	}
}

// 大批量归属查询必须仍然判得准：缓存的算存在，缓存和磁盘都没有的算缺失。
//
// 这条用例同时是"批量索引没把语义改坏"的守门人：真机上 1964 条成员里有
// 相当一部分是"文件已经删了但组里还留着"的悬空成员，判错了界面就会
// 少一个/多一个 ⚠️ 缺失提示。
func TestGetModGroupMembershipBatchIndexKeepsMissingSemantics(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	const cachedCount = 600
	const memberCount = 300
	members := make([]ModStrategyGroupMember, 0, memberCount)
	for i := 0; i < cachedCount; i++ {
		name := fmt.Sprintf("bulk-%03d.vpk", i)
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{Name: name, Location: "root"})
		if i < memberCount {
			members = append(members, ModStrategyGroupMember{Key: name, Name: name})
		}
	}
	// 一个既不在缓存、也不在磁盘上的成员：必须仍然被判为缺失。
	ghostKey := "ghost-does-not-exist.vpk"
	members = append(members, ModStrategyGroupMember{Key: ghostKey, Name: ghostKey})
	store := modStrategyGroupStore{Groups: []ModStrategyGroup{{
		ID:       "batch-group",
		Name:     "批量组",
		Strategy: modStrategyGroupAll,
		Members:  members,
	}}}
	if err := a.writeModStrategyGroupStore(store); err != nil {
		t.Fatalf("写入测试用 groups.json 失败: %v", err)
	}

	membership, err := a.GetModGroupMembership()
	if err != nil {
		t.Fatalf("GetModGroupMembership: %v", err)
	}
	if len(membership) != memberCount+1 {
		t.Fatalf("应有 %d 条归属记录，实际 %d", memberCount+1, len(membership))
	}
	for _, item := range membership {
		if item.Key == ghostKey {
			if !item.Missing {
				t.Fatalf("幽灵成员 %q 应被判为缺失", ghostKey)
			}
			continue
		}
		if item.Missing {
			t.Fatalf("成员 %q 已缓存，不应被判为缺失", item.Key)
		}
	}
}

// 结构守卫：三个"按成员逐条问文件在不在"的热路径必须走批量索引。
//
// 为什么不用耗时断言：本机 time.Now() 的粒度约 0.5ms，小 fixture 的耗时比值
// 量不出来（实测扫描 600 条缓存读数是 0ns）；而 O(成员数 × 缓存数) 与
// O(缓存数 + 成员数) 的差别是源码结构上的，直接锁结构最稳（前端也用同一套做法，
// 见 features/mod-groups/strategy-group-batch.test.mjs 的接线断言）。
func TestGroupBulkLookupStaysBatchIndexed(t *testing.T) {
	insights := readAppSource(t, "mod_group_insights.go")
	if !strings.Contains(insights, "a.buildVaultKeyIndex(a.rootDirectorySnapshot())") {
		t.Fatalf("GetModGroupMembership / GetModStrategyGroupMissingMembers 必须先用 buildVaultKeyIndex 建索引")
	}
	if strings.Contains(insights, "!a.modKeyExistsInVault(key)") {
		t.Fatalf("热路径退回了逐键调用 modKeyExistsInVault（O(成员数 × 缓存数)，真机 1.6 秒）")
	}
	members := readAppSource(t, "mod_groups.go")
	if !strings.Contains(members, "a.modKeyExistsInVaultIndexed(vaultKeys, member.Key)") {
		t.Fatalf("ApplyModStrategyGroup 过滤存活成员时必须走批量索引")
	}
}

func readAppSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", name, err)
	}
	return string(raw)
}

// BenchmarkGetModGroupMembershipVaultLookup 给维护者一个可复现的对比入口：
//
//	go test ./internal/app -run '^$' -bench VaultLookup -benchmem
func BenchmarkGetModGroupMembershipVaultLookup(b *testing.B) {
	a, addonsDir := newPriorityTestApp(&testing.T{})
	const cachedCount = 2000
	const memberCount = 1000
	members := make([]ModStrategyGroupMember, 0, memberCount)
	for i := 0; i < cachedCount; i++ {
		name := fmt.Sprintf("bench-%04d.vpk", i)
		seedCachedMod(a, filepath.Join(addonsDir, name), VPKFile{Name: name, Location: "root"})
		if i < memberCount {
			members = append(members, ModStrategyGroupMember{Key: name, Name: name})
		}
	}
	store := modStrategyGroupStore{Groups: []ModStrategyGroup{{
		ID:       "bench-group",
		Name:     "压测组",
		Strategy: modStrategyGroupAll,
		Members:  members,
	}}}
	if err := a.writeModStrategyGroupStore(store); err != nil {
		b.Fatalf("写入压测 groups.json 失败: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := a.GetModGroupMembership(); err != nil {
			b.Fatalf("GetModGroupMembership: %v", err)
		}
	}
}
