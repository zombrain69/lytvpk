package app

import (
	"os"
	"path/filepath"
	rt "runtime"
	"strings"
	"testing"
	"time"
)

// 对齐 FireAxe v0.7.2 的 process file customization：让用户指定"用哪个程序打开文件/目录"。
// 关键约定：**没配置就完全走原来的系统默认行为**，配置了才替换。

func TestSplitOpenWithArguments(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     []string
	}{
		{name: "空模板", template: "", want: nil},
		{name: "纯空白", template: "   ", want: nil},
		{name: "单个参数", template: "/O", want: []string{"/O"}},
		{name: "多个参数按空白切", template: "/O /T /L:{dir}", want: []string{"/O", "/T", "/L:{dir}"}},
		{name: "双引号分组", template: `--arg "值 里有空格"`, want: []string{"--arg", "值 里有空格"}},
		{name: "引号包住整个参数", template: `"/L={dir}"`, want: []string{"/L={dir}"}},
		{name: "转义引号", template: `--say \"hi\"`, want: []string{"--say", `"hi"`}},
		{name: "多余空白压缩", template: "  a   b  ", want: []string{"a", "b"}},
		{name: "单引号也可分组", template: `-c 'a b'`, want: []string{"-c", "a b"}},
		{
			// 真机复现过的 bug：反斜杠被当成转义符，Windows 路径被吃成 C:Toolstool.exe。
			name:     "Windows 路径里的反斜杠要原样保留",
			template: `-L "C:\Users\Administrator\Desktop" -file "D:\Mods\武器包"`,
			want:     []string{"-L", `C:\Users\Administrator\Desktop`, "-file", `D:\Mods\武器包`},
		},
		{
			name:     "未加引号的 Windows 路径同样保留反斜杠",
			template: `/O /T /L:C:\Tools\work`,
			want:     []string{"/O", "/T", `/L:C:\Tools\work`},
		},
		{name: "反斜杠转义引号仍然是引号", template: `--title \"名字\"`, want: []string{"--title", `"名字"`}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitOpenWithArguments(tc.template)
			if len(got) != len(tc.want) {
				t.Fatalf("参数个数 = %d (%v), want %d (%v)", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("第 %d 个参数 = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestBuildOpenWithCommand(t *testing.T) {
	filePath := filepath.Join("C:", "Mods", "addons", "ak47.vpk")
	dir := filepath.Join("C:", "Mods", "addons")

	t.Run("没配置程序时明确走系统默认", func(t *testing.T) {
		cmd, ok, err := buildOpenWithCommand("  ", "", filePath)
		if err != nil {
			t.Fatalf("不该报错: %v", err)
		}
		if ok {
			t.Fatalf("没配置程序时 ok 应为 false（调用方继续走 explorer），实际 %+v", cmd)
		}
	})

	t.Run("参数为空时自动把文件路径作为唯一参数", func(t *testing.T) {
		cmd, ok, err := buildOpenWithCommand(`C:\Tools\tc.exe`, "", filePath)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if cmd.Program != `C:\Tools\tc.exe` {
			t.Fatalf("程序 = %q", cmd.Program)
		}
		if len(cmd.Args) != 1 || cmd.Args[0] != filePath {
			t.Fatalf("参数 = %v, want [%s]", cmd.Args, filePath)
		}
	})

	t.Run("path / dir / name 三个占位符", func(t *testing.T) {
		cmd, ok, err := buildOpenWithCommand(
			"tool.exe",
			`/O "{dir}" --name "{name}" --file "{path}"`,
			filePath,
		)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		want := []string{"/O", dir, "--name", "ak47.vpk", "--file", filePath}
		if len(cmd.Args) != len(want) {
			t.Fatalf("参数 = %v, want %v", cmd.Args, want)
		}
		for i := range want {
			if cmd.Args[i] != want[i] {
				t.Fatalf("第 %d 个参数 = %q, want %q", i, cmd.Args[i], want[i])
			}
		}
	})

	t.Run("{0} 兼容 FireAxe 的写法（等价于 {path}）", func(t *testing.T) {
		cmd, ok, err := buildOpenWithCommand("tool.exe", "/select,{0}", filePath)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if len(cmd.Args) != 1 || cmd.Args[0] != "/select,"+filePath {
			t.Fatalf("参数 = %v", cmd.Args)
		}
	})

	t.Run("模板里没有占位符时追加文件路径", func(t *testing.T) {
		cmd, ok, err := buildOpenWithCommand("tool.exe", "/O /T", filePath)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if len(cmd.Args) != 3 || cmd.Args[2] != filePath {
			t.Fatalf("参数 = %v（应把文件路径追加到末尾）", cmd.Args)
		}
	})
}

func TestValidateOpenWithProgram(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "tool.exe")
	if err := os.WriteFile(existing, []byte("stub"), 0o644); err != nil {
		t.Fatalf("准备夹具失败: %v", err)
	}

	if err := validateOpenWithProgram(""); err != nil {
		t.Fatalf("空程序名表示未配置，不该报错: %v", err)
	}
	if err := validateOpenWithProgram("code"); err != nil {
		t.Fatalf("纯命令名（交给 PATH 解析）应放行: %v", err)
	}
	if err := validateOpenWithProgram(existing); err != nil {
		t.Fatalf("真实存在的程序应通过: %v", err)
	}
	err := validateOpenWithProgram(filepath.Join(dir, "missing.exe"))
	if err == nil {
		t.Fatal("带路径但文件不存在时应报错，避免用户填错还不自知")
	}
	if !strings.Contains(err.Error(), "找不到") {
		t.Fatalf("错误信息应说明找不到程序，实际: %v", err)
	}
}

// TestOpenWithSettingsRoundTrip：保存后要落进 config.json，读回来要一致；
// 程序留空时参数一并清掉（避免"看着生效其实没生效"）。
func TestOpenWithSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	app := &App{configDir: dir, configPath: filepath.Join(dir, "config.json")}
	// 用一个真实存在的程序路径，避免触发"路径写错"的校验。
	existingProgram := filepath.Join(dir, "tool.exe")
	if err := os.WriteFile(existingProgram, []byte("stub"), 0o644); err != nil {
		t.Fatalf("准备夹具失败: %v", err)
	}

	saved, err := app.SetOpenWithSettings("  "+existingProgram+"  ", `  /O /T /L="{dir}"  `)
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if saved.Program != existingProgram || saved.Arguments != `/O /T /L="{dir}"` {
		t.Fatalf("保存结果未去空白: %+v", saved)
	}

	data, err := os.ReadFile(app.configPath)
	if err != nil {
		t.Fatalf("读回 config.json 失败: %v", err)
	}
	if !strings.Contains(string(data), "openWithProgram") {
		t.Fatalf("config.json 里应写入 openWithProgram，实际内容: %s", data)
	}

	// 清空程序名：参数要一起清掉。
	cleared, err := app.SetOpenWithSettings("", "/O")
	if err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if cleared.Program != "" || cleared.Arguments != "" {
		t.Fatalf("程序留空时参数应被清掉，实际 %+v", cleared)
	}

	// 保存一个不存在、但带路径的程序要直接报错（避免用户填错还不自知）。
	if _, err := app.SetOpenWithSettings(filepath.Join(dir, "missing.exe"), ""); err == nil {
		t.Fatal("带路径但不存在时应报错")
	}
}

// TestOpenFileLocationUsesConfiguredProgram：配置了外部程序后，点击"打开所在位置"
// 真的会启动那个程序，并且真的把文件路径传了过去。
// 证据用系统自带的 xcopy.exe：让它把夹具文件复制一份，能复制成就说明"程序启动了 + 路径传对了"。
func TestOpenFileLocationUsesConfiguredProgram(t *testing.T) {
	if rt.GOOS != "windows" {
		t.Skipf("该端到端断言只在 Windows 上跑（当前 %s）", rt.GOOS)
	}
	xcopyPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "xcopy.exe")
	if _, err := os.Stat(xcopyPath); err != nil {
		t.Skipf("找不到 xcopy.exe（%s），跳过", xcopyPath)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "ak47.vpk")
	if err := os.WriteFile(target, []byte("vpk-stub"), 0o644); err != nil {
		t.Fatalf("准备夹具失败: %v", err)
	}
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("准备目标目录失败: %v", err)
	}
	copiedFile := filepath.Join(outDir, "ak47.vpk")

	app := &App{configDir: dir, configPath: filepath.Join(dir, "config.json")}
	if _, err := app.SetOpenWithSettings(xcopyPath, `/Y {path} `+outDir); err != nil {
		t.Fatalf("保存外部程序失败: %v", err)
	}
	if resolved, ok, err := app.openWithCommandForFile(target); err != nil || !ok {
		t.Fatalf("组装命令失败: ok=%v err=%v", ok, err)
	} else {
		t.Logf("将执行: %s %v", resolved.Program, resolved.Args)
	}
	if err := app.OpenFileLocation(target); err != nil {
		t.Fatalf("OpenFileLocation 应走外部程序且不报错: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if data, err := os.ReadFile(copiedFile); err == nil {
			if string(data) != "vpk-stub" {
				t.Fatalf("外部程序复制出来的内容不对: %q", data)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("等了 3 秒也没等到外部程序执行完（说明没真正启动或没拿到路径）")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 反向确认：清空配置后不该再走外部程序（而是回到系统默认）。
	if _, err := app.SetOpenWithSettings("", ""); err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if cmd, ok, err := app.openWithCommandForFile(target); err != nil || ok {
		t.Fatalf("清空后应回到系统默认，实际 ok=%v cmd=%+v err=%v", ok, cmd, err)
	}
}
