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
	template := a.groupSuggestionAgentPromptTemplate()
	catalogPath := filepath.Join(a.groupingCatalogDir(), "grouping_catalog.json")
	inboxPath := a.groupSuggestionInboxPath()
	prompt := template
	prompt = strings.ReplaceAll(prompt, "{{CATALOG_PATH}}", catalogPath)
	prompt = strings.ReplaceAll(prompt, "{{INBOX_PATH}}", inboxPath)
	prompt = strings.ReplaceAll(prompt, "{{MOD_COUNT}}", fmt.Sprintf("%d", a.countCachedMods()))
	return prompt
}

// 提示词的占位符：界面上会列出来，用户点一下就能插入。
var groupSuggestionPromptPlaceholders = []string{
	"{{CATALOG_PATH}}",
	"{{INBOX_PATH}}",
	"{{MOD_COUNT}}",
}

// ModGroupPromptState 描述"编辑器要显示什么"。
type ModGroupPromptState struct {
	// Text 是当前模板原文（保留占位符）；自定义时为用户保存的内容。
	Text string `json:"text"`
	// DefaultText 是 LytVPK 内置的那一套，用于"恢复默认"。
	DefaultText string `json:"defaultText"`
	IsCustom    bool   `json:"isCustom"`
	// Path 是自定义提示词的落盘位置（保存后才有文件）。
	Path         string   `json:"path"`
	SavedAt      string   `json:"savedAt,omitempty"`
	Placeholders []string `json:"placeholders"`
}

// groupSuggestionAgentPromptPath 是自定义提示词的保存位置。
func (a *App) groupSuggestionAgentPromptPath() string {
	if dir := a.groupSuggestionInboxPath(); dir != "" {
		return filepath.Join(filepath.Dir(dir), "group_suggestion_agent_prompt.md")
	}
	return ""
}

// groupSuggestionAgentPromptTemplate 返回当前生效的模板原文（自定义优先，失败回退内置）。
func (a *App) groupSuggestionAgentPromptTemplate() string {
	path := a.groupSuggestionAgentPromptPath()
	if path == "" {
		return groupSuggestionAgentPromptTemplate
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return groupSuggestionAgentPromptTemplate
	}
	text := string(data)
	if strings.TrimSpace(text) == "" {
		return groupSuggestionAgentPromptTemplate
	}
	return text
}

// GetGroupSuggestionAgentPromptState 返回编辑器状态：当前模板、内置默认、是否自定义、可用占位符。
func (a *App) GetGroupSuggestionAgentPromptState() (ModGroupPromptState, error) {
	path := a.groupSuggestionAgentPromptPath()
	template := a.groupSuggestionAgentPromptTemplate()
	state := ModGroupPromptState{
		Text:         template,
		DefaultText:  groupSuggestionAgentPromptTemplate,
		IsCustom:     template != groupSuggestionAgentPromptTemplate,
		Path:         path,
		Placeholders: append([]string(nil), groupSuggestionPromptPlaceholders...),
	}
	if state.IsCustom && path != "" {
		if info, err := os.Stat(path); err == nil {
			state.SavedAt = info.ModTime().Format(time.RFC3339)
		}
	}
	return state, nil
}

// SaveGroupSuggestionAgentPrompt 保存用户自己的一套提示词模板（保留 {{...}} 占位符即可自动填路径）。
func (a *App) SaveGroupSuggestionAgentPrompt(text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("提示词不能为空（如需回到内置版本请点「恢复默认」）")
	}
	path := a.groupSuggestionAgentPromptPath()
	if path == "" {
		return fmt.Errorf("配置目录不可用，无法保存提示词")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// 统一换行，避免编辑器把 CRLF 与 LF 混在一起导致每次保存都"看起来变了"。
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	return os.WriteFile(path, []byte(normalized), 0o644)
}

// ResetGroupSuggestionAgentPrompt 删除自定义提示词，回到 LytVPK 内置的那一套（幂等）。
func (a *App) ResetGroupSuggestionAgentPrompt() error {
	path := a.groupSuggestionAgentPromptPath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
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
	// 先重新扫描再导出：材料必须反映最新状态（用户可能刚加/删过 Mod 而没点刷新）。
	catalog, err := a.exportGroupingCatalogFresh("")
	if err != nil {
		return GroupingWorkspace{}, err
	}
	return GroupingWorkspace{
		CatalogPath: catalog,
		InboxPath:   a.groupSuggestionInboxPath(),
		Prompt:      a.GetGroupSuggestionAgentPrompt(),
		ModCount:    a.countCachedMods(),
		PreparedAt:  time.Now().Format(time.RFC3339),
		Notice: "这份材料是刚重新扫描过的最新状态。建议在智能体写完建议文件、你完成导入之前" +
			"不要再改动 mod 目录：期间加/删/移动过的 Mod 会被校验判为「未匹配」。" +
			"确实改过就重新点一次「准备给智能体的材料」，让智能体按新清单更新成员引用。",
	}, nil
}

// GroupingWorkspace 是"交给智能体的材料包"。
type GroupingWorkspace struct {
	CatalogPath string `json:"catalogPath"`
	InboxPath   string `json:"inboxPath"`
	Prompt      string `json:"prompt"`
	ModCount    int    `json:"modCount"`
	PreparedAt  string `json:"preparedAt"`
	// Notice 是给用户的"使用提示"（材料时效性），界面会原样展示。
	Notice string `json:"notice"`
}
