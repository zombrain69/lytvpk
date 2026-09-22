package app

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 给智能体（agent）用的提示词模板：随 EXE 一起发布，界面上可以一键复制或另存。
//
//go:embed assets/group_suggestion_agent_prompt.md
var groupSuggestionAgentPromptTemplate string

// GetGroupSuggestionAgentPrompt 返回填好本机路径的提示词，可直接交给智能体。
func (a *App) GetGroupSuggestionAgentPrompt() string {
	catalogPath := filepath.Join(a.groupingCatalogDir(), "grouping_catalog.json")
	inboxPath := a.groupSuggestionInboxPath()
	prompt := groupSuggestionAgentPromptTemplate
	prompt = strings.ReplaceAll(prompt, "{{CATALOG_PATH}}", catalogPath)
	prompt = strings.ReplaceAll(prompt, "{{INBOX_PATH}}", inboxPath)
	prompt = strings.ReplaceAll(prompt, "{{MOD_COUNT}}", fmt.Sprintf("%d", a.countCachedMods()))
	return prompt
}

// countCachedMods 统计当前扫描到的 Mod 数量（用于提示词里的上下文）。
func (a *App) countCachedMods() int {
	total := 0
	a.vpkCache.Range(func(_ any, value any) bool {
		if cache, ok := value.(*VPKFileCache); ok && cache != nil {
			total++
		}
		return true
	})
	return total
}

// groupingCatalogDir 是导出清单与建议文件的默认目录。
func (a *App) groupingCatalogDir() string {
	if dir := a.groupSuggestionInboxPath(); dir != "" {
		return filepath.Dir(dir)
	}
	return os.TempDir()
}

// SaveGroupSuggestionAgentPromptDialog 把提示词另存为文件（用户取消时返回空串）。
func (a *App) SaveGroupSuggestionAgentPromptDialog() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("应用尚未就绪，无法打开保存对话框")
	}
	target, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存智能体提示词",
		DefaultFilename: "group-suggestion-agent-prompt.md",
		Filters: []runtime.FileFilter{
			{DisplayName: "Markdown (*.md)", Pattern: "*.md"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(target) == "" {
		return "", nil
	}
	if err := os.WriteFile(target, []byte(a.GetGroupSuggestionAgentPrompt()), 0o644); err != nil {
		return "", err
	}
	return target, nil
}

// PrepareGroupingWorkspace 一步到位：导出 Mod 清单并返回提示词与两个路径，
// 让界面可以"一次点击拿到交给智能体的全部材料"。
func (a *App) PrepareGroupingWorkspace() (GroupingWorkspace, error) {
	catalog, err := a.ExportGroupingCatalog("")
	if err != nil {
		return GroupingWorkspace{}, err
	}
	return GroupingWorkspace{
		CatalogPath: catalog,
		InboxPath:   a.groupSuggestionInboxPath(),
		Prompt:      a.GetGroupSuggestionAgentPrompt(),
		ModCount:    a.countCachedMods(),
		PreparedAt:  time.Now().Format(time.RFC3339),
	}, nil
}

// GroupingWorkspace 是"交给智能体的材料包"。
type GroupingWorkspace struct {
	CatalogPath string `json:"catalogPath"`
	InboxPath   string `json:"inboxPath"`
	Prompt      string `json:"prompt"`
	ModCount    int    `json:"modCount"`
	PreparedAt  string `json:"preparedAt"`
}
