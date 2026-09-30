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
//	LytVPK-Community-Fork.exe --export-tag-baseline [输出路径]
//	    导出"标签召回基线"（entryId → 标签集合，默认写到配置目录的 tag_baseline.json）。
//	LytVPK-Community-Fork.exe --check-tag-regression <基线> [--allowlist <文件>] [--out <JSON>]
//	    与基线逐 Mod 比对：**只允许新增标签**；消失的标签必须出现在 allowlist 里，
//	    否则退出码 1（用于 CI / 发版前检查"没有少标"）。
//	LytVPK-Community-Fork.exe --build-stock-index [--game-dir <路径>]
//	    重建「游戏本体文件索引」（标签识别的精确证据来源）；不传 --game-dir 时用当前配置目录。
//	LytVPK-Community-Fork.exe --stock-index-status
//	    打印当前本体索引状态（挂载点、条目数），不写任何文件。
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
	case "--export-tag-baseline":
		target := ""
		if len(args) >= 3 {
			target = strings.TrimSpace(args[2])
		}
		os.Exit(runExportTagBaseline(target))
	case "--check-tag-regression":
		if len(args) < 3 || strings.TrimSpace(args[2]) == "" {
			fmt.Fprintln(os.Stderr, "用法: --check-tag-regression <基线文件> [--allowlist <文件>] [--out <JSON>]")
			os.Exit(2)
		}
		baseline := strings.TrimSpace(args[2])
		allowlist, output := "", ""
		for i := 3; i < len(args); i++ {
			switch strings.ToLower(strings.TrimSpace(args[i])) {
			case "--allowlist":
				if i+1 < len(args) {
					i++
					allowlist = strings.TrimSpace(args[i])
				}
			case "--out":
				if i+1 < len(args) {
					i++
					output = strings.TrimSpace(args[i])
				}
			}
		}
		os.Exit(runCheckTagRegression(baseline, allowlist, output))
	case "--build-stock-index":
		gameDir := ""
		for i := 2; i < len(args); i++ {
			if strings.EqualFold(strings.TrimSpace(args[i]), "--game-dir") && i+1 < len(args) {
				i++
				gameDir = strings.TrimSpace(args[i])
			}
		}
		os.Exit(runBuildStockIndex(gameDir))
	case "--stock-index-status":
		os.Exit(runStockIndexStatus())
	case "--stock-index-query":
		if len(args) < 3 || strings.TrimSpace(args[2]) == "" {
			fmt.Fprintln(os.Stderr, "用法: --stock-index-query <glob> [--limit N]")
			os.Exit(2)
		}
		limit := 20
		for i := 3; i < len(args); i++ {
			if strings.EqualFold(strings.TrimSpace(args[i]), "--limit") && i+1 < len(args) {
				i++
				fmt.Sscanf(strings.TrimSpace(args[i]), "%d", &limit)
			}
		}
		os.Exit(runStockIndexQuery(strings.TrimSpace(args[2]), limit))
	case "--validate-tag-rules":
		output := ""
		for i := 2; i < len(args); i++ {
			if strings.EqualFold(strings.TrimSpace(args[i]), "--out") && i+1 < len(args) {
				i++
				output = strings.TrimSpace(args[i])
			}
		}
		os.Exit(runValidateTagRules(output))
	case "--generate-entity-table":
		gameDir, output := "", ""
		for i := 2; i < len(args); i++ {
			key := strings.ToLower(strings.TrimSpace(args[i]))
			if i+1 >= len(args) {
				break
			}
			switch key {
			case "--game-dir":
				i++
				gameDir = strings.TrimSpace(args[i])
			case "--out":
				i++
				output = strings.TrimSpace(args[i])
			}
		}
		os.Exit(runGenerateEntityTable(gameDir, output))
	case "--entity-table-status":
		os.Exit(runEntityTableStatus())
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
	fmt.Println("  --export-tag-baseline [输出路径]      导出标签召回基线（默认写入配置目录的 tag_baseline.json）")
	fmt.Println("  --check-tag-regression <基线> [--allowlist <文件>] [--out <JSON>]")
	fmt.Println("      与基线比对：只允许新增标签；未登记的标签消失 → 退出码 1（少标回归）")
	fmt.Println("  --build-stock-index [--game-dir <路径>]  重建游戏本体文件索引（标签识别的精确证据来源）")
	fmt.Println("  --stock-index-status                  只读打印本体索引状态（挂载点 / 条目数）")
	fmt.Println("  --stock-index-query <glob> [--limit N]  在本体索引里查路径（支持 ** / * / ?）")
	fmt.Println("  --validate-tag-rules [--out <JSON>]   用本体索引校验规则表：前缀 0 命中=错误，")
	fmt.Println("      关键词本体 0 命中=警告；退出码 0 通过 / 1 有错误 / 2 无法校验（无本体索引）")
	fmt.Println("  --generate-entity-table [--game-dir <路径>] [--out <JSON>]")
	fmt.Println("      从游戏本体脚本（scripts/weapon_*.txt、scripts/melee/*.txt）生成实体表")
	fmt.Println("  --entity-table-status                 只读打印内置实体表状态（实体数 / 锚点数）")
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

// runExportTagBaseline 导出标签召回基线（entryId → 标签集合）。
func runExportTagBaseline(target string) int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	written, err := a.ExportTagBaseline(target)
	if err != nil {
		writeCLIError(err)
		return 2
	}
	fmt.Println(written)
	return 0
}

// runCheckTagRegression 比对基线：只允许新增标签，未登记的消失即回归（退出码 1）。
func runCheckTagRegression(baseline, allowlist, output string) int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	written, report, err := a.CheckTagRegression(baseline, allowlist, output)
	if err != nil {
		writeCLIError(err)
		return 2
	}
	if report == nil {
		writeCLIError(fmt.Errorf("回归检查未返回结果"))
		return 2
	}

	fmt.Printf("标签回归检查：基线 %d 个 Mod / 当前 %d 个 Mod\n", report.BaselineMods, report.CurrentMods)
	fmt.Printf("新增标签 +%d，消失标签 -%d（allowlist 放行 %d）\n",
		report.AddedTags, report.RemovedTags, report.AllowlistedRemovals)
	if len(report.MissingMods) > 0 {
		fmt.Printf("基线里已不在磁盘上的 Mod：%d 个（不算回归）\n", len(report.MissingMods))
	}
	if len(report.NewMods) > 0 {
		fmt.Printf("新增 Mod：%d 个\n", len(report.NewMods))
	}
	if !report.OK {
		fmt.Println("[FAIL] 出现未登记的标签消失（少标回归）：")
		printed := 0
		for _, change := range report.Changes {
			if len(change.Unexcused) == 0 {
				continue
			}
			fmt.Printf("  %s  %s  消失=[%s]\n", change.EntryID, change.Name, strings.Join(change.Unexcused, ", "))
			printed++
			if printed >= 50 {
				fmt.Println("  …（完整清单见报告 JSON）")
				break
			}
		}
	}
	fmt.Println(written)
	if !report.OK {
		return 1
	}
	return 0
}

// runBuildStockIndex 重建本体索引并打印状态。
func runBuildStockIndex(gameDir string) int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	status, err := a.BuildStockIndex(gameDir)
	if err != nil {
		writeCLIError(err)
		return 2
	}
	encoded, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		writeCLIError(err)
		return 2
	}
	fmt.Println(string(encoded))
	return 0
}

// runStockIndexStatus 打印当前索引状态（只读）。
func runStockIndexStatus() int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	status := a.GetStockIndexStatus()
	encoded, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		writeCLIError(err)
		return 2
	}
	fmt.Println(string(encoded))
	if status.Error != "" {
		return 1
	}
	return 0
}

// runStockIndexQuery 在索引上执行一次 glob 查询（只读，供维护者核对规则）。
func runStockIndexQuery(pattern string, limit int) int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	result := a.QueryStockIndex(pattern, limit)
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		writeCLIError(err)
		return 2
	}
	fmt.Println(string(encoded))
	if result.Error != "" {
		return 1
	}
	return 0
}

// runValidateTagRules 校验声明式规则表（只读；--out 时同时写完整报告）。
func runValidateTagRules(output string) int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	report, err := a.ValidateTagRules()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		writeCLIError(err)
		return 2
	}
	if strings.TrimSpace(output) != "" {
		if writeErr := os.WriteFile(output, encoded, 0o644); writeErr != nil {
			writeCLIError(fmt.Errorf("写入报告失败: %w", writeErr))
			return 2
		}
	}
	fmt.Println(string(encoded))

	if report.IndexPaths == 0 {
		// 没有本体索引：这是"未校验"，不是"规则坏了"。
		fmt.Fprintln(os.Stderr, "无法校验：本体索引不可用（先运行 --build-stock-index）")
		return 2
	}
	if !report.OK {
		return 1
	}
	return 0
}

// runGenerateEntityTable 从游戏本体脚本生成实体表（维护者工具）。
func runGenerateEntityTable(gameDir, output string) int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	written, table, err := a.GenerateEntityTable(gameDir, output)
	if err != nil {
		writeCLIError(err)
		return 2
	}
	summary := map[string]any{
		"file":     written,
		"entities": len(table.Entities),
		"source":   table.Source,
	}
	encoded, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(encoded))
	return 0
}

// runEntityTableStatus 打印内置实体表状态（只读）。
func runEntityTableStatus() int {
	a, err := newHeadlessApp()
	if err != nil {
		writeCLIError(err)
		return 2
	}
	status := a.GetEntityTableStatus()
	encoded, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(encoded))
	if status.Error != "" {
		return 1
	}
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
