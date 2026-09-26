package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 仓库级一致性审计：把"文档 / 代码 / 脚本三层对得上"变成可重复执行的测试，
// 避免以后加一个体检类型、改一处命令，只在代码里生效、文档悄悄过期。
// 这些检查只读文件，不触碰用户目录。

const repoRootForDocs = "../.."

func readRepoFile(t *testing.T, relative string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRootForDocs, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", relative, err)
	}
	return string(data)
}

// TestFireaxeParityDocReferencesExist：对照表里用反引号引用的仓库内路径必须真实存在。
// （§5.9 起就开始人工核对，这里固化成测试。）
func TestFireaxeParityDocReferencesExist(t *testing.T) {
	doc := readRepoFile(t, "docs/development/fireaxe-parity.md")
	pattern := regexp.MustCompile("`((?:internal|frontend|docs)/[^`]+?)`")
	seen := map[string]bool{}
	var missing []string
	for _, match := range pattern.FindAllStringSubmatch(doc, -1) {
		path := match[1]
		if seen[path] || strings.Contains(path, "...") {
			// "frontend/.../x" 这种缩略路径是文档里特意举的反例，跳过。
			continue
		}
		seen[path] = true
		if _, err := os.Stat(filepath.Join(repoRootForDocs, filepath.FromSlash(path))); err != nil {
			missing = append(missing, path)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("对照表引用了不存在的文件：%v", missing)
	}
	if len(seen) < 50 {
		t.Fatalf("对照表引用文件数异常偏少（%d），可能正则或文档结构被破坏", len(seen))
	}
}

// TestHealthIssueKindsHaveLabelAndDocRow：体检类型三层一致 ——
// Go 常量（后端会产生哪些 kind）→ 前端中文标签 → 用户文档表格。
func TestHealthIssueKindsHaveLabelAndDocRow(t *testing.T) {
	goFiles := []string{"internal/app/health_check.go", "internal/app/dependencies.go"}
	constPattern := regexp.MustCompile(`modHealthKind[A-Za-z]+\s*=\s*"([a-z_]+)"`)
	kinds := map[string]bool{}
	for _, file := range goFiles {
		for _, match := range constPattern.FindAllStringSubmatch(readRepoFile(t, file), -1) {
			kinds[match[1]] = true
		}
	}
	if len(kinds) < 10 {
		t.Fatalf("提取到的体检类型只有 %d 个，正则可能失效", len(kinds))
	}

	labelSource := readRepoFile(t, "frontend/src/js/features/settings/health-report-format.mjs")
	labelPattern := regexp.MustCompile(`([a-z_]+):\s*"([^"]+)"`)
	labels := map[string]string{}
	for _, match := range labelPattern.FindAllStringSubmatch(labelSource, -1) {
		labels[match[1]] = match[2]
	}

	doc := readRepoFile(t, "docs/toolbox/mod-health-check.md")
	for kind := range kinds {
		label, ok := labels[kind]
		if !ok {
			t.Errorf("体检类型 %s 缺少前端中文标签（health-report-format.mjs）", kind)
			continue
		}
		if !strings.Contains(doc, label) {
			t.Errorf("体检类型 %s 的标签「%s」没有出现在 docs/toolbox/mod-health-check.md 的结果表里", kind, label)
		}
	}
	// 反向：前端不该留已经删掉的类型标签
	for kind := range labels {
		if !strings.Contains(kind, "_") {
			continue
		}
		if !kinds[kind] {
			t.Errorf("前端标签表里的 %s 在后端已经没有对应常量了", kind)
		}
	}
}

// TestVerifiedCommandsMatchProjectScripts：文档里写的验证命令必须和工程脚本一致 ——
// 命令改名后，文档不能继续写旧的。
func TestVerifiedCommandsMatchProjectScripts(t *testing.T) {
	type packageJSON struct {
		Scripts map[string]string `json:"scripts"`
	}
	type wailsJSON struct {
		FrontendBuild string `json:"frontend:build"`
	}

	var frontendPkg packageJSON
	if err := json.Unmarshal([]byte(readRepoFile(t, "frontend/package.json")), &frontendPkg); err != nil {
		t.Fatalf("解析 frontend/package.json 失败: %v", err)
	}
	if _, ok := frontendPkg.Scripts["build"]; !ok {
		t.Fatal("frontend/package.json 缺少 build 脚本（文档里的 `npm run build` 会失效）")
	}
	if _, ok := frontendPkg.Scripts["test"]; ok {
		// 用 node --test 直接跑，不需要脚本；这里只确认 build 存在。
		_ = ok
	}

	var docsPkg packageJSON
	if err := json.Unmarshal([]byte(readRepoFile(t, "docs/package.json")), &docsPkg); err != nil {
		t.Fatalf("解析 docs/package.json 失败: %v", err)
	}
	if _, ok := docsPkg.Scripts["docs:build"]; !ok {
		t.Fatal("docs/package.json 缺少 docs:build 脚本（文档里的 `npm run docs:build` 会失效）")
	}

	var wails wailsJSON
	if err := json.Unmarshal([]byte(readRepoFile(t, "wails.json")), &wails); err != nil {
		t.Fatalf("解析 wails.json 失败: %v", err)
	}
	if wails.FrontendBuild != "npm run build" {
		t.Fatalf("wails.json 的 frontend:build = %q，与验证流程里假设的 `npm run build` 不一致", wails.FrontendBuild)
	}

	// 对照表的验证记录里要能查到这几条命令（否则"验证过什么"就无从复核）。
	parity := readRepoFile(t, "docs/development/fireaxe-parity.md")
	for _, command := range []string{"go test ./...", "node --test", "npm run build", "wails build", "npm run docs:build"} {
		if !strings.Contains(parity, command) {
			t.Errorf("对照表的验证记录里缺少命令 %q", command)
		}
	}
}
