package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// HandleCommandLine 处理"不启动界面"的命令行子命令，供外部智能体/脚本使用：
//
//	LytVPK-Community-Fork.exe --validate-group-suggestions <文件>
//	    只读校验建议文件，打印 JSON（逐条建议 + 逐成员解析结果 + 全部警告）；
//	    全部有效退出码 0，存在无效建议退出码 1。
//	    GUI 子系统下 stdout 可能不可见，因此同时把 JSON 写到 <文件>.validation.json
//	    （可用 --out <路径> 指定），并把该路径打印出来。
//	LytVPK-Community-Fork.exe --export-grouping-catalog [输出路径]
//	    按当前配置扫描并导出清单（默认写到配置目录的 grouping_catalog.json）。
//
// 返回 true 表示这条命令行已被处理（调用方应直接退出，不再启动界面）。
func HandleCommandLine(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(args[1])) {
	case "--validate-group-suggestions", "--validate-suggestions":
		if len(args) < 3 || strings.TrimSpace(args[2]) == "" {
			fmt.Fprintln(os.Stderr, "用法: --validate-group-suggestions <建议文件路径>")
			os.Exit(2)
		}
		output := ""
		if len(args) >= 5 && strings.EqualFold(strings.TrimSpace(args[3]), "--out") {
			output = strings.TrimSpace(args[4])
		}
		os.Exit(runValidateGroupSuggestions(args[2], output))
	case "--export-grouping-catalog":
		target := ""
		if len(args) >= 3 {
			target = strings.TrimSpace(args[2])
		}
		os.Exit(runExportGroupingCatalog(target))
	case "--help", "-h", "/?":
		printCommandLineHelp()
		os.Exit(0)
	}
	return false
}

func printCommandLineHelp() {
	fmt.Println("LytVPK 命令行工具：")
	fmt.Println("  --validate-group-suggestions <文件> [--out <JSON>]   只读校验组建议文件（不启动界面），")
	fmt.Println("      结果写入 <文件>.validation.json 或 --out 指定路径；全部有效退出码 0，否则 1")
	fmt.Println("  --export-grouping-catalog [输出路径]  导出分组用 Mod 清单（默认写入配置目录）")
}

// newHeadlessApp 构造一个"只读"的 App：加载配置 + 扫描当前目录，供 CLI 使用。
func newHeadlessApp() (*App, error) {
	a := NewApp()
	a.ensureConfigPaths()
	a.loadConfig()
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		rootDir = a.lastActiveDirectory
	}
	if strings.TrimSpace(rootDir) == "" {
		return a, fmt.Errorf("配置里没有可用目录，请先在 LytVPK 里选择 addons 目录")
	}
	a.mu.Lock()
	a.rootDir = rootDir
	a.mu.Unlock()
	if err := a.ScanVPKFiles(); err != nil {
		return a, err
	}
	return a, nil
}

func runValidateGroupSuggestions(path string, output string) int {
	absolute := path
	if resolved, err := filepath.Abs(path); err == nil {
		absolute = resolved
	}
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	validation, err := a.ValidateGroupSuggestionsFile(absolute)
	if err != nil {
		writeCLIError(err)
		return 2
	}
	encoded, err := json.MarshalIndent(validation, "", "  ")
	if err != nil {
		writeCLIError(err)
		return 2
	}
	fmt.Println(string(encoded))
	// GUI 子系统下看不到 stdout，因此同时落盘一份，便于智能体读取。
	target := strings.TrimSpace(output)
	if target == "" {
		target = absolute + ".validation.json"
	}
	if writeErr := os.WriteFile(target, encoded, 0o644); writeErr != nil {
		writeCLIError(fmt.Errorf("写入校验结果失败: %w", writeErr))
	} else {
		fmt.Println(target)
	}
	if validation.Invalid > 0 {
		return 1
	}
	return 0
}

func runExportGroupingCatalog(target string) int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	// newHeadlessApp 启动时已经扫过一次，这里直接用低层导出，避免重复扫描。
	written, err := a.ExportGroupingCatalog(target)
	if err != nil {
		writeCLIError(err)
		return 2
	}
	fmt.Println(written)
	return 0
}

func writeCLIError(err error) {
	payload, marshalErr := json.MarshalIndent(map[string]string{"error": err.Error()}, "", "  ")
	if marshalErr != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return
	}
	fmt.Fprintln(os.Stderr, string(payload))
}
