package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/charmap"
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

func encodeWindows1252AutoexecFixture(t *testing.T, content string) []byte {
	t.Helper()
	encoded, _, err := transform.Bytes(charmap.Windows1252.NewEncoder(), []byte(content))
	if err != nil {
		t.Fatalf("encode Windows-1252 fixture: %v", err)
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

func TestAutoexecPathAcceptsLeft4Dead2OrSteamGameRoot(t *testing.T) {
	base := t.TempDir()
	gameRoot := filepath.Join(base, "Left 4 Dead 2")
	gameDir := filepath.Join(gameRoot, "left4dead2")
	if err := os.MkdirAll(filepath.Join(gameDir, "addons"), 0755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(gameDir, "cfg", "autoexec.cfg")
	for _, input := range []string{gameDir, gameRoot} {
		got, err := autoexecPathForRoot(input)
		if err != nil {
			t.Fatalf("autoexec path for %q: %v", input, err)
		}
		if got != want {
			t.Fatalf("autoexec path for %q = %q, want %q", input, got, want)
		}
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

func TestAutoexecWindows1252RoundTripPreservesEncoding(t *testing.T) {
	app, path := newAutoexecTestApp(t)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := "// Café – Ê\r\nfps_max 144\r\n"
	if err := os.WriteFile(path, encodeWindows1252AutoexecFixture(t, original), 0644); err != nil {
		t.Fatal(err)
	}
	config, err := app.GetAutoexecConfig()
	if err != nil || config.Encoding != "Windows-1252/ANSI" || config.Content != original {
		t.Fatalf("read Windows-1252 autoexec = %#v, err=%v", config, err)
	}
	if err := app.SaveAutoexecConfig("// Révision\nfps_max 240\n"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := encodeWindows1252AutoexecFixture(t, "// Révision\r\nfps_max 240\r\n")
	if !bytes.Equal(raw, want) {
		t.Fatalf("Windows-1252 bytes changed unexpectedly: %x, want %x", raw, want)
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
	matches := app.AnalyzeAutoexecCommands("// comment\n; comment\nalias local_toggle fps_max\nbind F1 \\\"l4n_menu\\\"\nlocal_toggle 144\nmy_plugin_command 1\n")
	if len(matches) != 4 {
		t.Fatalf("matches = %#v", matches)
	}
	if !matches[0].Known || matches[0].Command != "alias" || matches[0].Line != 3 {
		t.Fatalf("alias match = %#v", matches[0])
	}
	if !matches[1].Known || matches[1].Command != "bind" || matches[1].Line != 4 {
		t.Fatalf("known match = %#v", matches[1])
	}
	if !matches[2].Known || matches[2].Command != "local_toggle" || matches[2].Help == nil || matches[2].Help.Scope != "本地配置" {
		t.Fatalf("local alias match = %#v", matches[2])
	}
	if matches[3].Known || matches[3].Command != "my_plugin_command" || matches[3].Line != 6 {
		t.Fatalf("unknown match = %#v", matches[3])
	}
}

func TestAnalyzeAutoexecCommandsRecognizesSlotsAndLaunchOptions(t *testing.T) {
	app := &App{}
	matches := app.AnalyzeAutoexecCommands("l4n_scripted_hud_allow_slot1 1\nl4n_scripted_hud_allow_slot15 0\nl4n_scripted_hud_allow_slot16 1\n-l4n_use_neko_engine_post\n")
	if len(matches) != 4 {
		t.Fatalf("matches = %#v", matches)
	}
	if !matches[0].Known || matches[0].Help == nil || matches[0].Help.Command != "l4n_scripted_hud_allow_slot" {
		t.Fatalf("slot1 match = %#v", matches[0])
	}
	if !matches[1].Known || matches[1].Help == nil {
		t.Fatalf("slot15 match = %#v", matches[1])
	}
	if matches[2].Known {
		t.Fatalf("slot16 should remain unknown: %#v", matches[2])
	}
	if !matches[3].Known || matches[3].Help == nil || matches[3].Help.Scope != "启动项" {
		t.Fatalf("launch option match = %#v", matches[3])
	}
}

func TestAutoexecCommandHelpFiltersL4N(t *testing.T) {
	app := &App{}
	items := app.GetAutoexecCommandHelp("l4n_game_usage")
	var exact *AutoexecCommandHelp
	for i := range items {
		if items[i].Command == "l4n_game_usage" {
			exact = &items[i]
			break
		}
	}
	if exact == nil || exact.Source != "readme_l4n.txt" {
		t.Fatalf("L4N help = %#v", items)
	}
}

func TestAutoexecCommandHelpCoversAdvancedReadmeEntries(t *testing.T) {
	app := &App{}
	all := app.GetAutoexecCommandHelp("")
	if len(all) < 90 {
		t.Fatalf("expected comprehensive command catalog, got %d entries", len(all))
	}
	seen := make(map[string]AutoexecCommandHelp, len(all))
	for _, item := range all {
		seen[item.Command] = item
	}
	for _, command := range []string{
		"l4n_vm_sway", "l4n_placelight", "+l4n_lookat", "l4n_print_launch_options", "mat_nekosky_overlay_lf",
		"l4n_allow_draw_sprite", "l4n_allow_hud_team_player_display", "l4n_flashlight_r", "l4n_flashlight_g", "l4n_flashlight_b",
		"l4n_force_dummy_addoninfo", "l4n_max_background_bik", "l4n_survivor", "l4n_thirdpersion_crosshair_alpha",
		"l4n_thirdpersion_crosshair_dynamic", "l4n_thirdpersion_crosshair_scale", "l4n_vm_allow_camera_animation", "l4n_vm_pin",
		"mat_nekosky_overlay_strength", "mat_neko_allow_invert_tonemap", "mat_nekorefract_color_invert_exponent",
		"mat_nekotoon_allow_lightwarp", "mat_nekotoon_lambert_factor", "mat_nekotoon_lighting_scale",
		"mat_nekotoon_rimlight_boost", "mat_nekotoon_rimlight_viewmodel_boost", "mat_nekotoon_brightness_limit",
		"mat_nekotoon_darkness_limit", "mat_nekotoon_lazy_texture_load", "mat_nekotoon_ignore_flat_normal",
		"mat_nekotoon_normalized_lightwarp", "mat_neko_tonemapping_algorithm", "mat_neko_tonemapping_force_linear",
		"mat_neko_gamma", "mat_neko_engine_post_after", "mat_nekobloom_luminance_threshold", "mat_nekobloom_scale",
		"mat_nekobloom_max_brightness", "mat_nekobloom_radius", "mat_nekobloom_maptex_strength", "mat_nekobloom_maptex_weight",
		"mat_nekobloom_blend_mode", "mat_neko_pre_tonemapping", "l4n_to_nekotoon_outline_type", "-l4n_use_neko_engine_post",
	} {
		item, ok := seen[command]
		if !ok || item.Source != "readme_l4n.txt" {
			t.Fatalf("missing README command %q: %#v", command, item)
		}
	}
}
