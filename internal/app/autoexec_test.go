package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func encodeGBKAutoexecFixture(t *testing.T, content string) []byte {
	t.Helper()
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(content))
	if err != nil {
		t.Fatalf("encode GBK fixture: %v", err)
	}
	return encoded
}

func newAutoexecTestApp(t *testing.T) (*App, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "left4dead2", "addons")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	app.rootDir = root
	return app, filepath.Join(filepath.Dir(root), "cfg", "autoexec.cfg")
}

func TestAutoexecPathUsesSelectedGameConfig(t *testing.T) {
	app, want := newAutoexecTestApp(t)
	got, err := app.autoexecPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("autoexec path = %q, want %q", got, want)
	}
}

func TestAutoexecGBKRoundTripPreservesEncodingAndCRLF(t *testing.T) {
	app, path := newAutoexecTestApp(t)
	original := "// 测试\r\nbind F1 \"l4n_menu\"\r\n"
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encodeGBKAutoexecFixture(t, original), 0644); err != nil {
		t.Fatal(err)
	}
	config, err := app.GetAutoexecConfig()
	if err != nil || config.Encoding != "GBK/ANSI" || config.LineEnding != "CRLF" || config.Content != original {
		t.Fatalf("read GBK autoexec = %#v, err=%v", config, err)
	}
	updated := "// 更新 中文\nbind F2 \"l4n_game_usage 1\"\n"
	if err := app.SaveAutoexecConfig(updated); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantRaw := encodeGBKAutoexecFixture(t, "// 更新 中文\r\nbind F2 \"l4n_game_usage 1\"\r\n")
	if !bytes.Equal(raw, wantRaw) {
		t.Fatalf("GBK bytes changed unexpectedly: %x, want %x", raw, wantRaw)
	}
	if _, err := os.Stat(path + ".lytvpk.bak"); err != nil {
		t.Fatalf("expected autoexec backup: %v", err)
	}
}

func TestAutoexecUTF8NoBOMPreservesCRLF(t *testing.T) {
	app, path := newAutoexecTestApp(t)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := []byte("fps_max 144\r\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := app.SaveAutoexecConfig("fps_max 240\n"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, []byte("fps_max 240\r\n")) {
		t.Fatalf("UTF-8 autoexec bytes = %q", raw)
	}
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("UTF-8 autoexec unexpectedly gained BOM")
	}
}

func TestAutoexecUTF16BOMRoundTrip(t *testing.T) {
	app, path := newAutoexecTestApp(t)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	content := "// 中文\nrate 100000"
	encoded, _, err := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder(), []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	encoded = append([]byte{0xFF, 0xFE}, encoded...)
	if err := os.WriteFile(path, encoded, 0644); err != nil {
		t.Fatal(err)
	}
	config, err := app.GetAutoexecConfig()
	if err != nil || config.Encoding != "UTF-16 LE" || config.Content != content {
		t.Fatalf("read UTF-16 autoexec = %#v, err=%v", config, err)
	}
	if err := app.SaveAutoexecConfig("// 更新\nrate 200000"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte{0xFF, 0xFE}) {
		t.Fatal("UTF-16 BOM was not preserved")
	}
}

func TestAnalyzeAutoexecCommandsIncludesUnknownAndSkipsComments(t *testing.T) {
	app := &App{}
	matches := app.AnalyzeAutoexecCommands("// comment\n; comment\nbind F1 \\\"l4n_menu\\\"\nmy_plugin_command 1\n")
	if len(matches) != 2 {
		t.Fatalf("matches = %#v", matches)
	}
	if !matches[0].Known || matches[0].Command != "bind" || matches[0].Line != 3 {
		t.Fatalf("known match = %#v", matches[0])
	}
	if matches[1].Known || matches[1].Command != "my_plugin_command" || matches[1].Line != 4 {
		t.Fatalf("unknown match = %#v", matches[1])
	}
}

func TestAutoexecCommandHelpFiltersL4N(t *testing.T) {
	app := &App{}
	items := app.GetAutoexecCommandHelp("l4n_game_usage")
	if len(items) != 1 || items[0].Source != "readme_l4n.txt" {
		t.Fatalf("L4N help = %#v", items)
	}
}
