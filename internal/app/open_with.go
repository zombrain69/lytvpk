package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// 「自定义外部打开程序」——对齐 FireAxe v0.7.2 的 process file customization
// （AppSettings.OpenDirectoryCustomProcessFileName / ShowFileCustomProcessFileName，
// 消费点 Utils.ShowInFileExplorer，用 string.Format(args, path) 把路径塞进参数）。
//
// 本项目的取向：
//   - **两个字段都留空 = 没配置**，此时 OpenFileLocation 完全保持原来的系统默认行为
//     （Windows: explorer /select；macOS: open -R；Linux: xdg-open），一个字节都不变；
//   - 配了程序才替换。参数模板支持 {path} / {dir} / {name}（另兼容 FireAxe 的 {0} = 完整路径）；
//     模板里一个占位符都没有时，自动把完整路径追加到参数末尾；
//   - 不经过 shell，程序与参数直接交给 exec.Command，避免命令行注入面。

// OpenWithSettings 是「打开文件方式」的两个字段。
type OpenWithSettings struct {
	Program   string `json:"program"`
	Arguments string `json:"arguments"`
}

// openWithCommand 是最终要执行的命令。
type openWithCommand struct {
	Program string
	Args    []string
}

// splitOpenWithArguments 把参数模板切成参数数组。
// 规则：空白分隔；双引号 / 单引号内保留空白；反斜杠**只在引号前**有特殊含义
// （`\"` 得到一个字面量引号），其它位置一律按字面量保留 —— 这是 Windows 命令行约定，
// 也是必须的：否则 `C:\Tools\tool.exe` 里的反斜杠会被当成转义符吃掉。
func splitOpenWithArguments(template string) []string {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return nil
	}

	var args []string
	var current strings.Builder
	var quote rune
	started := false

	flush := func() {
		args = append(args, current.String())
		current.Reset()
		started = false
	}

	runes := []rune(trimmed)
	for index := 0; index < len(runes); index++ {
		r := runes[index]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			started = true
		case r == '"' || r == '\'':
			quote = r
			started = true
		case r == '\\':
			if index+1 < len(runes) && (runes[index+1] == '"' || runes[index+1] == '\'') {
				current.WriteRune(runes[index+1])
				index++
			} else {
				current.WriteRune(r)
			}
			started = true
		case r == ' ' || r == '\t':
			if started {
				flush()
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if started {
		flush()
	}
	return args
}

// buildOpenWithCommand 组装最终命令。
// 返回 ok=false 表示"没配置外部程序"，调用方应继续走系统默认。
func buildOpenWithCommand(program, arguments, filePath string) (openWithCommand, bool, error) {
	program = strings.TrimSpace(program)
	if program == "" {
		return openWithCommand{}, false, nil
	}

	cleanPath := filepath.Clean(filePath)
	dir := filepath.Dir(cleanPath)
	name := filepath.Base(cleanPath)

	args := splitOpenWithArguments(arguments)
	if len(args) == 0 {
		return openWithCommand{Program: program, Args: []string{cleanPath}}, true, nil
	}

	replacer := strings.NewReplacer(
		"{path}", cleanPath,
		"{dir}", dir,
		"{name}", name,
		"{0}", cleanPath,
	)
	hasPlaceholder := false
	resolved := make([]string, len(args))
	for index, arg := range args {
		if strings.Contains(arg, "{path}") ||
			strings.Contains(arg, "{dir}") ||
			strings.Contains(arg, "{name}") ||
			strings.Contains(arg, "{0}") {
			hasPlaceholder = true
		}
		resolved[index] = replacer.Replace(arg)
	}
	if !hasPlaceholder {
		resolved = append(resolved, cleanPath)
	}

	return openWithCommand{Program: program, Args: resolved}, true, nil
}

// validateOpenWithProgram 保存前做静态校验：
// 纯命令名（如 code）交给 PATH 解析，放行；带路径分隔符时必须真的存在，避免填错还不自知。
func validateOpenWithProgram(program string) error {
	trimmed := strings.TrimSpace(program)
	if trimmed == "" {
		return nil
	}
	if strings.ContainsAny(trimmed, `\/`) {
		if _, err := os.Stat(trimmed); err != nil {
			return fmt.Errorf("找不到程序: %s（请填完整路径，或只填命令名交给 PATH 查找）", trimmed)
		}
	}
	return nil
}

// openWithCommandForFile 读取当前配置并组装命令；没配置时返回 ok=false。
func (a *App) openWithCommandForFile(filePath string) (openWithCommand, bool, error) {
	a.mu.RLock()
	program := a.openWithProgram
	arguments := a.openWithArguments
	a.mu.RUnlock()
	return buildOpenWithCommand(program, arguments, filePath)
}

// GetOpenWithSettings 读取当前配置（空字符串 = 未配置 = 系统默认）。
func (a *App) GetOpenWithSettings() OpenWithSettings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return OpenWithSettings{Program: a.openWithProgram, Arguments: a.openWithArguments}
}

// SetOpenWithSettings 保存配置。传空字符串即恢复"系统默认"。
func (a *App) SetOpenWithSettings(program string, arguments string) (OpenWithSettings, error) {
	trimmedProgram := strings.TrimSpace(program)
	if err := validateOpenWithProgram(trimmedProgram); err != nil {
		return a.GetOpenWithSettings(), err
	}
	trimmedArguments := strings.TrimSpace(arguments)
	if trimmedProgram == "" {
		// 没填程序时参数没有意义，一起清掉，避免留下"看似生效"的残留。
		trimmedArguments = ""
	}

	a.mu.Lock()
	a.openWithProgram = trimmedProgram
	a.openWithArguments = trimmedArguments
	a.mu.Unlock()

	a.saveConfig()
	return OpenWithSettings{Program: trimmedProgram, Arguments: trimmedArguments}, nil
}

// startOpenWithCommand 启动外部程序。刻意不用 shell，参数直接传递。
func startOpenWithCommand(cmd openWithCommand) error {
	if err := exec.Command(cmd.Program, cmd.Args...).Start(); err != nil {
		return fmt.Errorf("外部程序启动失败: %v（检查 设置 → 界面设置 → 打开文件方式）", err)
	}
	return nil
}
