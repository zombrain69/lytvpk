package app

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// 本文件对齐 FireAxe `GamePathUtils`（`GamePathUtils.cs:9-121`）：
// 先读 Steam 注册表拿安装目录，再逐个 Steam 库去找游戏，最后用与上游 `CheckValidity`
// 同义的判断确认"这里真的装了 L4D2"（必须存在 `left4dead2` 子目录）。
//
// 比上游多一步：解析 `libraryfolders.vdf`。上游只扫"盘符 + 固定相对路径"，
// 而把游戏装在第二个库（例如 `E:\SteamLibrary`）或自定义目录是常态，
// 固定路径猜不到，读库清单才稳。

// vdfStringPattern 匹配 KeyValues 里的一个带引号字符串（支持 `\"` 转义）。
var vdfStringPattern = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// parseSteamLibraryFolders 从 libraryfolders.vdf 内容里取出全部库路径。
//
// 同时兼容两种写法（Steam 在不同版本里都写过）：
//
//	新版："1" { "path" "E:\\SteamLibrary" ... }
//	旧版："1" "E:\\SteamLibrary"
//
// 只认像路径的值，`label` / `apps` / `contentid` / `totalsize` 之类自然被跳过。
func parseSteamLibraryFolders(content string) []string {
	tokens := make([]string, 0, 32)
	for _, match := range vdfStringPattern.FindAllStringSubmatch(content, -1) {
		tokens = append(tokens, unescapeVDFString(match[1]))
	}

	paths := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			return
		}
		seen[key] = true
		paths = append(paths, path)
	}

	for index := 0; index < len(tokens)-1; index++ {
		key := strings.TrimSpace(tokens[index])
		value := tokens[index+1]
		if strings.EqualFold(key, "path") {
			add(value)
			index++ // 跳过刚用掉的值，避免把路径当成下一轮的键
			continue
		}
		// 旧格式：键是库序号，值是路径。
		if _, err := strconv.Atoi(key); err == nil && looksLikeAbsolutePath(value) {
			add(value)
			index++
		}
	}
	return paths
}

func unescapeVDFString(value string) string {
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\t`, "\t").Replace(value)
}

func looksLikeAbsolutePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, "/") {
		return true
	}
	// Windows 盘符：`E:\...` 或 `E:/...`
	return len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

// steamLibraryCandidates 返回"值得看一眼"的 Steam 目录：
// 安装目录本身，外加它下面可能放 libraryfolders.vdf 的 config 与 steamapps。
func steamLibraryCandidates(installPath string) []string {
	installPath = strings.TrimSpace(installPath)
	if installPath == "" {
		return nil
	}
	return []string{
		installPath,
		filepath.Join(installPath, "config"),
		filepath.Join(installPath, "steamapps"),
	}
}

// readSteamLibraryFolders 依次尝试安装目录、config、steamapps 下的 libraryfolders.vdf，
// 读到第一份就返回（Steam 不同版本把它放在不同位置）。
func readSteamLibraryFolders(installPath string, readFile func(string) ([]byte, error)) []string {
	for _, dir := range steamLibraryCandidates(installPath) {
		content, err := readFile(filepath.Join(dir, "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		return parseSteamLibraryFolders(string(content))
	}
	return nil
}

// addonsPathIfGame 判断某个 Steam 库目录下是不是装了 L4D2（与上游 `CheckValidity` 同义，
// 但额外要求 `addons` 目录存在 —— 结果最终要交给 SetRootDirectory，目录必须真实存在）。
func addonsPathIfGame(library string, dirExists func(string) bool) string {
	library = strings.TrimSpace(library)
	if library == "" {
		return ""
	}
	gameDir := filepath.Join(library, "steamapps", "common", "Left 4 Dead 2")
	if !dirExists(filepath.Join(gameDir, "left4dead2")) {
		return ""
	}
	addonsDir := filepath.Join(gameDir, "left4dead2", "addons")
	if !dirExists(addonsDir) {
		return ""
	}
	return addonsDir
}

// findAddonsInSteamLibraries 按"安装目录 → 库清单里登记的其它库"的顺序找 addons 目录。
// readFile / dirExists 都是注入点，测试不碰真实磁盘与注册表。
func findAddonsInSteamLibraries(installPath string, readFile func(string) ([]byte, error), dirExists func(string) bool) string {
	installPath = strings.TrimSpace(installPath)
	if installPath == "" {
		return ""
	}
	libraries := append([]string{installPath}, readSteamLibraryFolders(installPath, readFile)...)

	seen := make(map[string]bool, len(libraries))
	for _, library := range libraries {
		library = strings.TrimSpace(library)
		if library == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(library))
		if seen[key] {
			continue
		}
		seen[key] = true
		if addons := addonsPathIfGame(library, dirExists); addons != "" {
			return addons
		}
	}
	return ""
}
