package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 本地记录 schema 版本化（对齐 FireAxe 的存档版本 + 迁移）：
// 旧文件（没有 schemaVersion）要能读、能被迁移，并在下次写盘时盖上版本号；
// 比本程序更新的存档不做降级。
func TestLocalStoreSchemaMigratesLegacyGroupsFile(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	bPath := filepath.Join(addonsDir, "b.vpk")
	group, err := a.CaptureModStrategyGroup("父组", "", modStrategyGroupAll, []string{bPath})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}

	// 手写一份"旧版"存档：没有 schemaVersion、parentId 指向不存在的组、成员键重复。
	legacy := `{
  "groups": [
    {
      "id": "` + group.ID + `",
      "name": "父组",
      "strategy": "all",
      "enforce": false,
      "parentId": "已经不存在的组",
      "members": [
        {"key": "b.vpk", "name": "b.vpk"},
        {"key": "b.vpk", "name": "b.vpk"}
      ]
    }
  ]
}`
	path := a.ensureGroupsPath()
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := a.readModStrategyGroupStore()
	if err != nil {
		t.Fatalf("read legacy store: %v", err)
	}
	if len(store.Groups) != 1 {
		t.Fatalf("应有 1 个组: %#v", store.Groups)
	}
	migrated := store.Groups[0]
	if migrated.ParentID != "" {
		t.Fatalf("悬空 parentId 应被清空: %#v", migrated.ParentID)
	}
	if len(migrated.Members) != 1 {
		t.Fatalf("重复成员键应去重: %#v", migrated.Members)
	}
	if migrated.UpdatedAt == "" || migrated.CreatedAt == "" {
		t.Fatalf("时间戳应被补齐: %#v", migrated)
	}

	// 写回后文件里应带上当前 schema 版本。
	if err := a.writeModStrategyGroupStore(store); err != nil {
		t.Fatalf("write migrated store: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if version, _ := decoded["schemaVersion"].(float64); int(version) != localStoreSchemaVersion {
		t.Fatalf("写盘应盖上 schemaVersion=%d: %s", localStoreSchemaVersion, raw)
	}
}

func TestLocalStoreSchemaKeepsNewerVersionAndMigratesPriority(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	newer := localStoreSchemaVersion + 5
	if got := localStoreWriteVersion(newer); got != newer {
		t.Fatalf("更新的存档版本不应被降级: %d", got)
	}
	if got := localStoreWriteVersion(0); got != localStoreSchemaVersion {
		t.Fatalf("旧存档写回时应升到当前版本: %d", got)
	}

	// priority.json：空键丢弃、同键保留最后一条、补 UpdatedAt。
	priorityPath := a.ensurePriorityPath()
	if err := os.MkdirAll(filepath.Dir(priorityPath), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "entries": [
    {"key": "", "tier": 1},
    {"key": "b.vpk", "tier": 5},
    {"key": "b.vpk", "tier": -3}
  ]
}`
	if err := os.WriteFile(priorityPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := a.readModPriorityStore()
	if err != nil {
		t.Fatalf("read legacy priority: %v", err)
	}
	if len(store.Entries) != 1 {
		t.Fatalf("空键应丢弃、同键应合并: %#v", store.Entries)
	}
	if store.Entries[0].Key != "b.vpk" || store.Entries[0].Tier != -3 {
		t.Fatalf("同键应保留最后一条（-3）: %#v", store.Entries[0])
	}
	if store.Entries[0].UpdatedAt == "" {
		t.Fatalf("应补齐 UpdatedAt: %#v", store.Entries[0])
	}
}
