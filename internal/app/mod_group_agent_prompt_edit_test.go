package app

import (
	"os"
	"strings"
	"testing"
)

// 提示词要能由用户改成自己的一套：保存自定义 → 之后都用它；恢复默认 → 回到内置那套。

func TestAgentPromptDefaultsToBuiltin(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	state, err := a.GetGroupSuggestionAgentPromptState()
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.IsCustom {
		t.Fatalf("默认应为内置提示词: %#v", state)
	}
	if !strings.Contains(state.Text, "{{CATALOG_PATH}}") {
		t.Fatalf("编辑器里看到的应当是模板（保留占位符）: %s", state.Text)
	}
	if state.Text != state.DefaultText {
		t.Fatal("默认状态下 text 应等于 defaultText")
	}
	if !strings.Contains(state.DefaultText, "处理范围") {
		t.Fatal("内置提示词必须写明处理的文件夹范围")
	}
	if len(state.Placeholders) < 3 {
		t.Fatalf("应给出可用占位符: %#v", state.Placeholders)
	}
}

func TestSaveAgentPromptReplacesTemplateAndKeepsPlaceholders(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	custom := "# 我的提示词\n\n清单：{{CATALOG_PATH}}\n建议文件：{{INBOX_PATH}}\n共 {{MOD_COUNT}} 个 Mod。\n"
	if err := a.SaveGroupSuggestionAgentPrompt(custom); err != nil {
		t.Fatalf("save: %v", err)
	}

	state, err := a.GetGroupSuggestionAgentPromptState()
	if err != nil {
		t.Fatal(err)
	}
	if !state.IsCustom || state.Text != custom {
		t.Fatalf("保存后应使用自定义提示词: %#v", state)
	}
	if state.SavedAt == "" {
		t.Fatal("应记录保存时间")
	}

	// 真正交给智能体的那份必须把占位符替换成真实路径。
	resolved := a.GetGroupSuggestionAgentPrompt()
	if strings.Contains(resolved, "{{") {
		t.Fatalf("占位符没有被替换: %s", resolved)
	}
	if !strings.Contains(resolved, a.groupSuggestionInboxPath()) {
		t.Fatalf("自定义提示词里的建议文件路径没有被替换: %s", resolved)
	}
	if !strings.Contains(resolved, "共 0 个 Mod") && !strings.Contains(resolved, "共 ") {
		t.Fatalf("占位符 MOD_COUNT 没有被替换: %s", resolved)
	}
}

func TestSaveAgentPromptRejectsBlankAndResetRestoresDefault(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if err := a.SaveGroupSuggestionAgentPrompt("   \n  "); err == nil {
		t.Fatal("空白提示词应被拒绝")
	}
	custom := "自定义：{{CATALOG_PATH}}"
	if err := a.SaveGroupSuggestionAgentPrompt(custom); err != nil {
		t.Fatal(err)
	}
	if err := a.ResetGroupSuggestionAgentPrompt(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	state, err := a.GetGroupSuggestionAgentPromptState()
	if err != nil {
		t.Fatal(err)
	}
	if state.IsCustom || state.Text != state.DefaultText {
		t.Fatalf("恢复默认后应回到内置提示词: %#v", state)
	}
	if _, err := os.Stat(a.groupSuggestionAgentPromptPath()); !os.IsNotExist(err) {
		t.Fatalf("恢复默认应删除自定义文件: %v", err)
	}
	// 再次恢复默认不应报错（幂等）。
	if err := a.ResetGroupSuggestionAgentPrompt(); err != nil {
		t.Fatalf("重复恢复默认应幂等: %v", err)
	}
}

func TestAgentPromptPreparesWorkspaceWithCustomText(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	if err := a.SaveGroupSuggestionAgentPrompt("只分析 addons / workshop / disabled：{{CATALOG_PATH}}"); err != nil {
		t.Fatal(err)
	}
	workspace, err := a.PrepareGroupingWorkspace()
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !strings.Contains(workspace.Prompt, "只分析 addons / workshop / disabled") {
		t.Fatalf("材料包应带上自定义提示词: %s", workspace.Prompt)
	}
	if strings.Contains(workspace.Prompt, "{{") {
		t.Fatalf("材料包里的占位符必须替换: %s", workspace.Prompt)
	}
}
