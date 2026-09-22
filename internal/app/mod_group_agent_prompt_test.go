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
	seedCachedMod(a, filepath.Join(addonsDir, "a.vpk"), VPKFile{Name: "a.vpk", Title: "甲", Location: "root"})
	seedCachedMod(a, filepath.Join(addonsDir, "workshop", "123.vpk"), VPKFile{Name: "123.vpk", Title: "工坊条目", Location: "workshop", WorkshopID: "123"})

	workspace, err := a.PrepareGroupingWorkspace()
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if workspace.ModCount != 2 {
		t.Fatalf("Mod 数量 = %d, want 2", workspace.ModCount)
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
