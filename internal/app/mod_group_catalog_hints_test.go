package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v3 说明里给开发侧的三条观察：
//  1. preloadHints 缺 entryId —— 与其它 hint 对齐；
//  2. root / disabled 同名共用 key 的"副本对"在应用里会合并成 1 个成员，无法成组 —— 明确标注；
//  3. 导出后若目录又被改动，校验会把成员判为未匹配 —— 导出产物与校验结果都要给提示，
//     并且"导入建议文件"前也先重新扫描一遍。

type catalogHintsSnapshot struct {
	PreloadHints []struct {
		Key            string   `json:"key"`
		Title          string   `json:"title"`
		Reason         string   `json:"reason"`
		EntryID        string   `json:"entryId"`
		EntryIDs       []string `json:"entryIds"`
		MemberEntryIDs []string `json:"memberEntryIds"`
	} `json:"preloadHints"`
	DuplicateGroups []struct {
		Reason             string   `json:"reason"`
		EntryIDs           []string `json:"entryIds"`
		Keys               []string `json:"keys"`
		SingleAddonListKey bool     `json:"singleAddonListKey"`
		Note               string   `json:"note"`
	} `json:"duplicateGroups"`
}

// exportCatalogHints 用**低层导出**（只读当前缓存）：测试可以自由选择
// "注入缓存"（验证 hint 结构）或"写真实文件 + 扫描"（验证判定逻辑）。
func exportCatalogHints(t *testing.T, a *App, path string, scan bool) catalogHintsSnapshot {
	t.Helper()
	if scan {
		if err := a.ScanVPKFiles(); err != nil {
			t.Fatalf("扫描: %v", err)
		}
	}
	if _, err := a.ExportGroupingCatalog(path); err != nil {
		t.Fatalf("导出: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot catalogHintsSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestPreloadHintsCarryEntryIDs(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "xdReanimsBase.vpk"), VPKFile{
		Name: "xdReanimsBase.vpk", Title: "xdReanimsBase 前置库", Location: "root",
	})
	seedCachedMod(a, filepath.Join(addonsDir, "disabled", "l4n 音频库.vpk"), VPKFile{
		Name: "l4n 音频库.vpk", Title: "l4n 音频库", Location: "disabled",
	})

	snapshot := exportCatalogHints(t, a, filepath.Join(t.TempDir(), "catalog.json"), false)
	if len(snapshot.PreloadHints) != 2 {
		t.Fatalf("应有 2 条前置库提示: %#v", snapshot.PreloadHints)
	}
	for _, hint := range snapshot.PreloadHints {
		if hint.EntryID == "" {
			t.Fatalf("preloadHints 必须带 entryId（与其它 hint 对齐）: %#v", hint)
		}
		if len(hint.MemberEntryIDs) != 1 || hint.MemberEntryIDs[0] != hint.EntryID {
			t.Fatalf("preloadHints 必须带 memberEntryIds 且与 entryId 一致: %#v", hint)
		}
		// 与 duplicateGroups[].entryIds 命名一致，外部推导方可以用同一套字段名处理所有 hint。
		if len(hint.EntryIDs) != 1 || hint.EntryIDs[0] != hint.EntryID {
			t.Fatalf("preloadHints 必须带 entryIds（数组形式，与其它 hint 命名对齐）: %#v", hint)
		}
	}
}

func TestDuplicateGroupsMarkSingleAddonListKeyPairs(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// 同名同体积的两个文件：一个在根目录、一个在 disabled。
	// 它们解析成同一个 addonlist 键（app 会把它们视为同一个成员），无法构成 ≥2 成员的组。
	writeTestVPK(t, filepath.Join(addonsDir, "同键双位置.vpk"), map[string][]byte{
		"materials/same.vtf": []byte("same"),
	})
	if err := os.MkdirAll(filepath.Join(addonsDir, "disabled"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestVPK(t, filepath.Join(addonsDir, "disabled", "同键双位置.vpk"), map[string][]byte{
		"materials/same.vtf": []byte("same"),
	})
	// 另一对是真正的"不同键"副本（root + workshop 同名），应当不被标注。
	writeTestVPK(t, filepath.Join(addonsDir, "正常副本 A.vpk"), map[string][]byte{
		"materials/copy.vtf": []byte("copy"),
	})
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestVPK(t, filepath.Join(addonsDir, "workshop", "正常副本 A.vpk"), map[string][]byte{
		"materials/copy.vtf": []byte("copy"),
	})

	snapshot := exportCatalogHints(t, a, filepath.Join(t.TempDir(), "catalog.json"), true)
	var marked, normal int
	for _, group := range snapshot.DuplicateGroups {
		joined := strings.ToLower(strings.Join(group.Keys, " "))
		switch {
		case strings.Contains(joined, "同键双位置"):
			marked++
			if !group.SingleAddonListKey {
				t.Fatalf("同键双位置副本必须标注 singleAddonListKey: %#v", group)
			}
			if !strings.Contains(group.Note, "同一个 addonlist 键") {
				t.Fatalf("标注要说明原因: %#v", group)
			}
		case strings.Contains(joined, "正常副本"):
			normal++
			if group.SingleAddonListKey {
				t.Fatalf("不同键的副本不应被标注: %#v", group)
			}
		}
	}
	if marked == 0 || normal == 0 {
		t.Fatalf("测试前置不满足：marked=%d normal=%d groups=%#v",
			marked, normal, snapshot.DuplicateGroups)
	}
}

func TestImportGroupSuggestionsRefreshesScanFirst(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	// 之后往目录里放一个文件但**不刷新**，再导入引用它的建议。
	writeTestVPK(t, filepath.Join(addonsDir, "late.vpk"), map[string][]byte{
		"materials/late.vtf": []byte("l"),
	})
	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":   "迟到文件组",
		"members": []string{"late.vpk", "a.vpk"},
	}))
	// 用界面入口那条路径（先重新扫描再导入）。
	result, err := a.importGroupSuggestionsFresh(source)
	if err != nil {
		t.Fatalf("导入: %v", err)
	}
	if result.Imported != 1 || result.Skipped != 0 {
		t.Fatalf("导入应成功（导入前会自动重新扫描）: %#v", result)
	}
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "找不到") || strings.Contains(warning, "未在当前列表") {
			t.Fatalf("刚放进目录的成员不应被判为未匹配: %v", warning)
		}
	}
}

func TestValidationWarnsAboutFolderChangesAfterExport(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "b.vpk"), VPKFile{Name: "b.vpk", Location: "root"})
	source := writeSuggestionFile(t, t.TempDir(), suggestionsPayload(map[string]any{
		"label":   "引用了不存在的成员",
		"members": []string{"a.vpk", "已经不在列表里的.vpk"},
	}))
	validation, err := a.ValidateGroupSuggestionsFile(source)
	if err != nil {
		t.Fatalf("校验: %v", err)
	}
	joined := strings.Join(validation.Warnings, "\n")
	if !strings.Contains(joined, "重新导出") {
		t.Fatalf("出现未匹配成员时应提示「重新导出材料」: %s", joined)
	}
}
