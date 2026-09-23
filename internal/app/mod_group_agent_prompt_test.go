package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vpk-manager/internal/grouping"
)

// 提示词必须把"给智能体的材料"说清楚：清单路径、建议文件路径、标准格式、策略与硬性要求。
func TestGroupSuggestionAgentPromptIsSelfContained(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	prompt := a.GetGroupSuggestionAgentPrompt()

	required := []string{
		"version",
		"\"suggestions\"",
		"strategy",
		"single",
		"不要修改、移动、删除任何 Mod 文件",
		"grouping_catalog.json",
		"group_suggestions.json",
		"至少 2 个",
		"key",
		"structure.samplePaths",
		"structure.topDirs",
		"structure.targets",
		"clusterHints",
		"duplicateGroups",
		"workshop.title",
		"workshop.tags",
		"addonInfo.desc",
		"management.dependencies",
		"management.collections",
		"coverage",
		"withWorkshopMeta",
		"entryId",
		"schemaRev",
		"capabilities",
		"scope",
		"ungroupedKeys",
		"themeHints",
		"preloadHints",
		"unreadableMods",
		"--validate-group-suggestions",
		"你不需要打开 VPK",
		// 处理范围要写死：只处理 addons 根目录 + workshop + disabled。
		"处理范围（硬性",
		"`addons\\workshop\\`",
		"`addons\\disabled\\`",
		"不参与扫描",
		// 标签：让智能体把标签一起产出，缺失时由规则兜底。
		"\"tag\"",
		"tagReason",
		"memberTags",
		"24 个字符",
		"降级",
		"withTags",
		"taggedMembers",
		// 清单与磁盘的时间差 + root/disabled 同键的退化副本
		"清单与磁盘的时间差",
		"generatedAt",
		"游戏不会加载里面的任何文件",
		"singleAddonListKey",
		// 套装/配套模块：共享非官方资源前缀 → all 组（本轮补的漏判规则）
		"套装 / 配套模块（很容易漏，务必专门扫一遍）",
		"airi_evilfall",
		"同一个官方目标",
		// 套件命名空间（本轮新增的内置信号 + 清单字段）
		"structure.resourceRoots",
		"套装资源目录",
		"codm/ice",
		// 本体关联的两条线索 + 禁止跨套件合并
		"套件名关键词",
		"shinano维纳斯bill.vpk",
		"绝对不要跨套件合并",
		"sikushui",
	}
	for _, fragment := range required {
		if !strings.Contains(prompt, fragment) {
			t.Fatalf("提示词缺少关键内容 %q", fragment)
		}
	}
	if strings.Contains(prompt, "{{") {
		t.Fatalf("提示词里的占位符没有被替换: %s", prompt)
	}
	// 路径必须落在本机配置目录里，智能体才能直接写文件。
	if !strings.Contains(prompt, a.groupSuggestionInboxPath()) {
		t.Fatalf("提示词缺少建议文件路径")
	}
}

func TestPrepareGroupingWorkspaceExportsCatalogAndPrompt(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	// 准备材料会先重新扫描目录（保证材料是最新的），所以这里用真实文件而不是注入缓存。
	writeTestVPK(t, filepath.Join(addonsDir, "prompt-fixture.vpk"), map[string][]byte{
		"materials/prompt-fixture.vtf": []byte("p"),
	})

	workspace, err := a.PrepareGroupingWorkspace()
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	// 数量必须与实际扫描结果一致（夹具目录里是 a/b/c + workshop/123 + 新增一个）。
	snapshot := readCatalogSnapshot(t, workspace.CatalogPath)
	if workspace.ModCount != snapshot.ModCount || workspace.ModCount < 5 {
		t.Fatalf("Mod 数量 = %d，清单里是 %d（应一致且包含新写入的文件）",
			workspace.ModCount, snapshot.ModCount)
	}
	if !catalogKeys(snapshot)["prompt-fixture.vpk"] {
		t.Fatalf("清单里应包含刚写入的真实文件: %#v", catalogKeys(snapshot))
	}
	if workspace.CatalogPath == "" || workspace.InboxPath == "" || workspace.Prompt == "" {
		t.Fatalf("材料包不完整: %#v", workspace)
	}
	if _, err := os.Stat(workspace.CatalogPath); err != nil {
		t.Fatalf("清单文件没有生成: %v", err)
	}
	if !strings.Contains(workspace.Prompt, "group_suggestions.json") {
		t.Fatalf("提示词缺少建议文件说明")
	}
}

// App 层常量与 internal/grouping 的信号目录必须一致（防止两处漂移）。
func TestAppSignalConstantsMatchGroupingCatalog(t *testing.T) {
	pairs := map[string]string{
		modGroupSignalCollection:     "collection",
		modGroupSignalFilenamePrefix: "filename-prefix",
		modGroupSignalSharedTags:     "shared-tags",
		modGroupSignalSameAuthor:     "same-author",
		modGroupSignalSubject:        "subject",
		modGroupSignalVoice:          "voice-character",
		modGroupSignalFolder:         "folder",
		modGroupSignalSameFileName:   "same-filename",
	}
	for label, id := range pairs {
		if got := grouping.SignalID(label); got != id {
			t.Fatalf("信号 %q 的 ID = %q, want %q", label, got, id)
		}
	}
	if modGroupSuggestionFolderScore != grouping.FolderScore {
		t.Fatalf("文件夹基准分漂移: %d vs %d", modGroupSuggestionFolderScore, grouping.FolderScore)
	}
	if modGroupSuggestionMaxAuthorMembers != 12 {
		t.Fatalf("作者上限漂移: %d", modGroupSuggestionMaxAuthorMembers)
	}
}
