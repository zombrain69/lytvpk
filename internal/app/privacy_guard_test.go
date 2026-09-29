package app

// 仓库隐私守卫：防止再把“本机路径 / 凭据”提交进公开仓库。
//
// 背景：2026-09-30 的体检发现历史文档里带过本机 Steam 库路径与 Windows 用户名，
// 已全部替换为通用占位（H:\SteamLibrary、C:\Users\Example 等）。这个测试把
// “真正属于某台机器”的写法和密钥形状钉死，避免以后再被顺手写回文档/测试里。

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// privacyGuardForbiddenPatterns 是“绝不允许出现在跟踪文件里”的写法。
// 注意：通用示例（H:\SteamLibrary、C:\Users\Example、C:\Program Files (x86)\Steam）不算。
var privacyGuardForbiddenPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"Windows 用户名目录", regexp.MustCompile(`C:\\Users\\(Administrator|PC)(\\|\b)`)},
	{"本机 Steam 库", regexp.MustCompile(`[CDEFG]:\\SteamLibrary`)},
	{"本机 Steam 安装目录", regexp.MustCompile(`C:\\steam\b`)},
	{"GitHub token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`)},
	{"GitHub PAT", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{40,}`)},
	{"私钥块", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"OpenAI 风格密钥", regexp.MustCompile(`sk-(proj-)?[A-Za-z0-9]{40,}`)},
}

// privacyGuardForbiddenFileNames 是“不允许被跟踪”的文件名（凭据/本地环境）。
var privacyGuardForbiddenFileNames = []string{
	".env", "secrets.json", "credentials.json",
}

var privacyGuardForbiddenSuffixes = []string{".pem", ".key", ".p12", ".pfx"}

func TestRepositoryHasNoLocalPathsOrSecrets(t *testing.T) {
	root := privacyGuardRepoRoot(t)
	files := privacyGuardTrackedFiles(t, root)
	if len(files) < 100 {
		t.Fatalf("git ls-files 只返回 %d 个文件，明显不对，守卫中止", len(files))
	}

	selfPath := filepath.ToSlash(filepath.Join("internal", "app", "privacy_guard_test.go"))
	for _, rel := range files {
		normalized := filepath.ToSlash(rel)
		base := strings.ToLower(filepath.Base(normalized))
		for _, forbidden := range privacyGuardForbiddenFileNames {
			if base == forbidden {
				t.Errorf("跟踪文件 %s 属于本地凭据文件，不应入库", normalized)
			}
		}
		for _, suffix := range privacyGuardForbiddenSuffixes {
			if strings.HasSuffix(base, suffix) {
				t.Errorf("跟踪文件 %s 看起来是密钥/证书，不应入库", normalized)
			}
		}
		// 守卫自己就写着这些模式，跳过自身。
		if normalized == selfPath {
			continue
		}
		if strings.HasSuffix(base, ".png") || strings.HasSuffix(base, ".jpg") ||
			strings.HasSuffix(base, ".woff2") || strings.HasSuffix(base, ".ico") ||
			strings.HasSuffix(base, ".exe") || strings.HasSuffix(base, ".vpk") {
			continue
		}
		content, err := privacyGuardReadFile(filepath.Join(root, filepath.FromSlash(normalized)))
		if err != nil || content == "" {
			continue
		}
		for _, pattern := range privacyGuardForbiddenPatterns {
			if match := pattern.re.FindString(content); match != "" {
				t.Errorf("%s 命中“%s”：%s（请改成通用占位，别把本机信息写进公开仓库）",
					normalized, pattern.name, privacyGuardMask(match))
			}
		}
	}
}

func privacyGuardMask(value string) string {
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + strings.Repeat("*", 6)
}

func privacyGuardRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("拿不到仓库根目录（可能不在 git 工作树里）: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func privacyGuardTrackedFiles(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git ls-files 失败: %v", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files
}

func privacyGuardReadFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > 512*1024 {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if strings.Contains(string(data[:min(len(data), 2048)]), "\x00") {
		return "", nil
	}
	return string(data), nil
}
