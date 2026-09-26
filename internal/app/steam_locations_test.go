package app

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

// 对齐 FireAxe `GamePathUtils.TryFind`（GamePathUtils.cs:33-81）：
// 先读 Steam 注册表拿到安装目录，再沿库目录找游戏；只有找到真正的游戏目录才算命中
// （上游的 `CheckValidity` 要求存在 `left4dead2` 子目录）。
// 本项目额外解析 `libraryfolders.vdf`，因为把游戏装在第二个 Steam 库（例如 `E:\SteamLibrary`）
// 是国内玩家的常态，而"盘符 + 路径猜测"猜不到自定义库目录。
func TestParseSteamLibraryFoldersHandlesBothFormats(t *testing.T) {
	content := strings.Join([]string{
		`"libraryfolders"`,
		`{`,
		`	"0"`,
		`	{`,
		`		"path"		"C:\\Program Files (x86)\\Steam"`,
		`		"label"		""`,
		`		"apps"`,
		`		{`,
		`			"550"		"1234567890"`,
		`		}`,
		`	}`,
		`	"1"`,
		`	{`,
		`		"path"		"E:\\SteamLibrary"`,
		`	}`,
		`	"2"		"D:\\Games\\SteamLibrary"`,
		`	"3"`,
		`	{`,
		`		"path"		"e:\\steamlibrary"`,
		`	}`,
		`}`,
	}, "\n")

	paths := parseSteamLibraryFolders(content)
	want := []string{`C:\Program Files (x86)\Steam`, `E:\SteamLibrary`, `D:\Games\SteamLibrary`}
	if len(paths) != len(want) {
		t.Fatalf("expected %d libraries, got %#v", len(want), paths)
	}
	for index, expected := range want {
		if paths[index] != expected {
			t.Fatalf("library %d: expected %q, got %q", index, expected, paths[index])
		}
	}
	// 重复库（大小写不同）只保留一条。
	if len(parseSteamLibraryFolders(`"libraryfolders" { "1" { "path" "E:\\SteamLibrary" } "2" { "path" "E:\\steamlibrary" } }`)) != 1 {
		t.Fatal("大小写不同的同一个库应当去重")
	}
	if len(parseSteamLibraryFolders("不是 VDF 的内容")) != 0 {
		t.Fatal("解析不了时应当返回空，而不是猜一个路径")
	}
}

func TestFindAddonsInSteamLibrariesUsesRegistryAndLibraryFolders(t *testing.T) {
	libraries := map[string]bool{}
	addGame := func(base string) {
		libraries[base+`\left4dead2`] = true
		libraries[base+`\left4dead2\addons`] = true
	}
	addGame(`C:\Steam\steamapps\common\Left 4 Dead 2`)
	addGame(`E:\SteamLibrary\steamapps\common\Left 4 Dead 2`)
	dirExists := func(path string) bool { return libraries[path] }
	readFile := func(path string) ([]byte, error) {
		if !strings.EqualFold(path, `C:\Steam\steamapps\libraryfolders.vdf`) {
			return nil, fs.ErrNotExist
		}
		return []byte(`"libraryfolders" { "1" { "path" "E:\SteamLibrary" } }`), nil
	}

	// 注册表指向 C:\Steam，第二个库登记在 libraryfolders.vdf 里；两边都装了游戏时取第一个命中的。
	got := findAddonsInSteamLibraries(`C:\Steam`, readFile, dirExists)
	if got != `C:\Steam\steamapps\common\Left 4 Dead 2\left4dead2\addons` {
		t.Fatalf("expected the install directory to win, got %q", got)
	}

	// 安装目录里没有游戏时，应当继续看 libraryfolders.vdf 登记的自定义库。
	delete(libraries, `C:\Steam\steamapps\common\Left 4 Dead 2\left4dead2`)
	delete(libraries, `C:\Steam\steamapps\common\Left 4 Dead 2\left4dead2\addons`)
	got = findAddonsInSteamLibraries(`C:\Steam`, readFile, dirExists)
	if got != `E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2\addons` {
		t.Fatalf("expected the extra Steam library to be used, got %q", got)
	}

	// 注册表读不到（绿色版 / 非 Steam 安装）时返回空，交给盘符扫描兜底。
	if got := findAddonsInSteamLibraries("", readFile, dirExists); got != "" {
		t.Fatalf("没有注册表路径时不该猜，got %q", got)
	}

	// 库目录里只有游戏目录、没有 addons 时不返回（SetRootDirectory 要求目录真实存在）。
	fakeLibraries := map[string]bool{`D:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2`: true}
	if got := findAddonsInSteamLibraries(`D:\SteamLibrary`, func(string) ([]byte, error) { return nil, fs.ErrNotExist },
		func(path string) bool { return fakeLibraries[path] }); got != "" {
		t.Fatalf("没有 addons 目录时不该返回，got %q", got)
	}
}

func TestSteamLibraryCandidatesIncludeConfigFolder(t *testing.T) {
	candidates := steamLibraryCandidates(`C:\Steam`)
	joined := strings.Join(candidates, "|")
	for _, expected := range []string{`C:\Steam`, `C:\Steam\config`, `C:\Steam\steamapps`} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("候选里应包含 %s：%s", expected, joined)
		}
	}
	if len(steamLibraryCandidates("")) != 0 {
		t.Fatal("没有安装目录时不应有候选")
	}
}

func TestAddonsPathIfGameRequiresLeft4Dead2Directory(t *testing.T) {
	base := `E:\SteamLibrary\steamapps\common\Left 4 Dead 2`
	game := base + `\left4dead2`
	cases := []struct {
		name  string
		dirs  []string
		want  string
	}{
		{name: "游戏与 addons 都在", dirs: []string{game, game + `\addons`}, want: game + `\addons`},
		{name: "只有游戏目录", dirs: []string{game}, want: ""},
		{name: "完全没有", dirs: []string{`E:\SteamLibrary\steamapps\common\别的游戏`}, want: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			set := map[string]bool{}
			for _, dir := range testCase.dirs {
				set[dir] = true
			}
			got := addonsPathIfGame(`E:\SteamLibrary`, func(path string) bool { return set[path] })
			if got != testCase.want {
				t.Fatalf("expected %q, got %q", testCase.want, got)
			}
		})
	}
}

func TestParseSteamLibraryFoldersIgnoresUnrelatedPaths(t *testing.T) {
	content := fmt.Sprintf(`"libraryfolders" { "1" { "path" "%s" "contentid" "550" } }`, "E:\\Lib")
	paths := parseSteamLibraryFolders(content)
	if len(paths) != 1 || paths[0] != `E:\Lib` {
		t.Fatalf("expected only the path entry, got %#v", paths)
	}
}
