package gamedata

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"l4d2-manager-next/pkg/valve/vpk"
)

// writeTestMountVPK 写一个最小可读的 pak01_dir.vpk（只为覆盖"读取目录树"路径）。
func writeTestMountVPK(t *testing.T, mountDir string, entries []string) {
	t.Helper()
	if err := os.MkdirAll(mountDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archive := &vpk.Archive{Header: vpk.Header{Magic: vpk.Magic, Version: 1}}
	var offset uint32
	var payload bytes.Buffer
	for _, entry := range entries {
		dir, base, ext := splitVPKTestPath(entry)
		data := []byte(entry)
		archive.Files = append(archive.Files, vpk.File{
			Dir:  dir,
			Base: base,
			Ext:  ext,
			DirEntry: vpk.DirEntry{
				CRC:           crc32.ChecksumIEEE(data),
				DataLocation:  []vpk.DataChunk{{ArchiveIndex: 0x7fff, EntryOffset: offset, EntryLength: uint32(len(data))}},
				MetadataBytes: 0,
			},
		})
		offset += uint32(len(data))
		payload.Write(data)
	}

	var buffer bytes.Buffer
	if err := vpk.WriteDirectory(&buffer, archive); err != nil {
		t.Fatalf("write vpk directory: %v", err)
	}
	buffer.Write(payload.Bytes())
	if err := os.WriteFile(filepath.Join(mountDir, "pak01_dir.vpk"), buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func splitVPKTestPath(name string) (dir, base, ext string) {
	name = strings.ReplaceAll(name, "\\", "/")
	dir = filepath.ToSlash(filepath.Dir(name))
	if dir == "." {
		dir = " "
	}
	ext = strings.TrimPrefix(filepath.Ext(name), ".")
	base = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	if ext == "" {
		ext = " "
	}
	return dir, base, ext
}

func TestNormalizeGameRootAcceptsInstallRootMountDirAndAddonsDir(t *testing.T) {
	root := t.TempDir()
	mount := filepath.Join(root, "left4dead2")
	writeTestMountVPK(t, mount, []string{"models/props_junk/gnome.mdl"})
	addons := filepath.Join(mount, "addons")
	if err := os.MkdirAll(addons, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, input := range []string{root, mount, addons} {
		got, err := NormalizeGameRoot(input)
		if err != nil {
			t.Fatalf("NormalizeGameRoot(%q) 失败: %v", input, err)
		}
		if got != root {
			t.Fatalf("NormalizeGameRoot(%q) = %q，期望 %q", input, got, root)
		}
	}

	if _, err := NormalizeGameRoot(filepath.Join(root, "not-a-game")); err == nil {
		t.Fatalf("非游戏目录必须报错")
	}
}

func TestBuildCollectsVPKEntriesAndLooseFiles(t *testing.T) {
	root := t.TempDir()
	writeTestMountVPK(t, filepath.Join(root, "left4dead2"), []string{
		"models/w_models/weapons/w_shotgun.mdl",
		"models/w_models/weapons/w_pumpshotgun_A.mdl",
	})

	// 语音只在松散目录里（与真机一致）。
	voice := filepath.Join(root, "left4dead2", "sound", "player", "hunter", "voice", "warn01.wav")
	if err := os.MkdirAll(filepath.Dir(voice), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(voice, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 高优先级挂载点补一条 VPK 条目。
	writeTestMountVPK(t, filepath.Join(root, "update"), []string{"scripts/melee/golfclub.txt"})

	index, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"models/w_models/weapons/w_shotgun.mdl",       // 低优先级挂载点
		"models/w_models/weapons/w_pumpshotgun_a.mdl", // 条目名统一小写
		"scripts/melee/golfclub.txt",                  // 高优先级挂载点
		"sound/player/hunter/voice/warn01.wav",        // 松散文件
	} {
		if !index.Exists(want) {
			t.Fatalf("本体索引缺少 %q（共 %d 条）", want, index.PathCount)
		}
	}
	if index.Exists("models/does/not/exist.mdl") {
		t.Fatalf("不存在的路径不应命中")
	}
	if index.PathCount < 4 {
		t.Fatalf("PathCount=%d，期望 ≥4", index.PathCount)
	}
	if got := index.Category("sound/player/hunter/voice/warn01.wav"); got != "sound" {
		t.Fatalf("Category=%q，期望 sound", got)
	}

	byName := map[string]MountStat{}
	for _, mount := range index.Mounts {
		byName[mount.Name] = mount
	}
	if byName["left4dead2"].VPKEntries != 2 || byName["left4dead2"].LooseFiles != 1 {
		t.Fatalf("left4dead2 挂载点统计异常: %+v", byName["left4dead2"])
	}
	if byName["update"].VPKEntries != 1 {
		t.Fatalf("update 挂载点统计异常: %+v", byName["update"])
	}
	if !byName["left4dead2_dlc1"].Skipped {
		t.Fatalf("缺失的挂载点应标记为 Skipped: %+v", byName["left4dead2_dlc1"])
	}
	if len(index.PathsWithPrefix("models/w_models/weapons/")) != 2 {
		t.Fatalf("PathsWithPrefix 结果异常: %v", index.PathsWithPrefix("models/w_models/weapons/"))
	}
}

func TestGlobMatchesDoubleStarSingleStarAndQuestionMark(t *testing.T) {
	index := NewFromPaths(nil, []string{
		"models/w_models/weapons/w_shotgun.mdl",
		"models/props_junk/gnome.mdl",
		"models/props_junk/gascan001a.mdl",
		"particles/weapon_fx.pcf",
		"particles/xx2_c6_custom_rifle_ak47.pcf",
		"sound/player/hunter/voice/warn01.wav",
		"scripts/melee/golfclub.txt",
	})

	cases := []struct {
		pattern string
		want    int
		sample  string
	}{
		{"models/**/*.mdl", 3, "models/props_junk/gnome.mdl"},
		{"models/props_junk/*.mdl", 2, "models/props_junk/gascan001a.mdl"},
		{"particles/*.pcf", 2, "particles/weapon_fx.pcf"},
		{"particles/**ak47*.pcf", 1, "particles/xx2_c6_custom_rifle_ak47.pcf"},
		{"sound/player/*/voice/*.wav", 1, "sound/player/hunter/voice/warn01.wav"},
		{"models/**/w_shotgun.mdl", 1, "models/w_models/weapons/w_shotgun.mdl"},
		// 单个 `*` 不跨目录：`models/*/w_shotgun.mdl` 不能匹配
		// models/w_models/weapons/w_shotgun.mdl（中间有两层目录）。
		{"models/*/w_shotgun.mdl", 0, ""},
		{"scripts/melee/golfclu?.txt", 1, "scripts/melee/golfclub.txt"},
	}
	for _, tc := range cases {
		got := index.Glob(tc.pattern)
		if len(got) != tc.want {
			t.Fatalf("Glob(%q) = %v，期望 %d 条", tc.pattern, got, tc.want)
		}
		if tc.sample != "" {
			found := false
			for _, item := range got {
				if item == tc.sample {
					found = true
				}
			}
			if !found {
				t.Fatalf("Glob(%q) 缺少 %q：%v", tc.pattern, tc.sample, got)
			}
		}
	}

	// 缓存命中路径也要一致。
	if again := index.Glob("particles/*.pcf"); len(again) != 2 {
		t.Fatalf("第二次 Glob 结果不一致: %v", again)
	}
}

func TestLoadOrBuildUsesCacheAndRejectsStaleSchema(t *testing.T) {
	root := t.TempDir()
	writeTestMountVPK(t, filepath.Join(root, "left4dead2"), []string{"models/props_junk/gnome.mdl"})
	cache := filepath.Join(t.TempDir(), "stock_index.json")

	built, err := LoadOrBuild(cache, root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !built.Exists("models/props_junk/gnome.mdl") {
		t.Fatalf("首次构建缺少条目")
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("应写出缓存: %v", err)
	}

	// 缓存有效时：即使游戏目录已不可用，也必须能读到（"不会因为游戏更新/移动就丢标签"）。
	cached, err := LoadOrBuild(cache, filepath.Join(root, "gone"), 0)
	if err != nil {
		t.Fatalf("应命中缓存: %v", err)
	}
	if !cached.Exists("models/props_junk/gnome.mdl") {
		t.Fatalf("缓存内容异常")
	}

	// 版本不匹配必须报错，让调用方重建而不是误用。
	if err := os.WriteFile(cache, []byte(`{"schemaVersion":999,"paths":["a/b"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(cache); err == nil {
		t.Fatalf("版本不匹配应报错")
	}
	if _, err := LoadOrBuild(cache, root, time.Minute); err != nil {
		t.Fatalf("版本不匹配时应重建: %v", err)
	}
}

func TestBuildFailsWithoutMountsAndNilIndexDegradesSafely(t *testing.T) {
	if _, err := Build(filepath.Join(t.TempDir(), "empty")); err == nil {
		t.Fatalf("空目录必须报错（调用方据此降级）")
	}

	var nilIndex *StockIndex
	if nilIndex.Exists("models/props_junk/gnome.mdl") {
		t.Fatalf("nil 索引必须安全返回 false")
	}
	if nilIndex.Glob("models/**") != nil {
		t.Fatalf("nil 索引必须安全返回 nil")
	}
}
