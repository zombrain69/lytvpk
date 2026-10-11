package app

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vtf"
)

// 造一个包含 文本 / PNG / 二进制 三种条目的 VPK，验证列表与预览的分支。
func TestVPKPreviewReadsTextAndImage(t *testing.T) {
	app := &App{}
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "materials"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "addoninfo.txt"), []byte("\"AddonInfo\"\n{\n\taddontitle \"预览测试\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var pngBuffer bytes.Buffer
	if err := png.Encode(&pngBuffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "materials", "icon.png"), pngBuffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	// .phy 属于"没有预览通道"的二进制类型（.mdl/.bsp 现在各自有信息卡）。
	if err := os.WriteFile(filepath.Join(srcDir, "materials", "model.phy"), []byte{0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}
	// 假模型：不是 IDST 头 → 走"信息卡 + 可读说明"分支，而不是 binary。
	if err := os.WriteFile(filepath.Join(srcDir, "materials", "fake.mdl"), []byte{0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	packed, err := app.packVPKDirectoryWithOptions(srcDir, outDir, false, "preview-fixture", nil)
	if err != nil {
		t.Fatalf("打包夹具失败: %v", err)
	}

	list, err := app.ListVPKEntries(packed.OutputPath)
	if err != nil {
		t.Fatalf("列条目失败: %v", err)
	}
	if list.TotalCount != 4 {
		t.Fatalf("条目数 = %d，期望 4（%#v）", list.TotalCount, list.Entries)
	}
	if list.Entries[0].Path > list.Entries[1].Path {
		t.Fatalf("条目应按路径排序: %#v", list.Entries)
	}

	textPreview, err := app.PreviewVPKEntry(packed.OutputPath, "addoninfo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if textPreview.Kind != "text" || !strings.Contains(textPreview.Text, "预览测试") {
		t.Fatalf("文本预览不对: %#v", textPreview)
	}

	imagePreview, err := app.PreviewVPKEntry(packed.OutputPath, "materials/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	if imagePreview.Kind != "image" || !strings.HasPrefix(imagePreview.DataURL, "data:image/png;base64,") {
		t.Fatalf("图片预览不对: kind=%s prefix=%s", imagePreview.Kind, imagePreview.DataURL[:min(24, len(imagePreview.DataURL))])
	}

	binaryPreview, err := app.PreviewVPKEntry(packed.OutputPath, "materials/model.phy")
	if err != nil {
		t.Fatal(err)
	}
	if binaryPreview.Kind != "binary" || binaryPreview.Note == "" {
		t.Fatalf("二进制条目应只给说明: %#v", binaryPreview)
	}

	// 模型条目：即使不是合法 IDST 头，也要给"模型信息卡 + 说明"，不能是 binary。
	modelPreview, err := app.PreviewVPKEntry(packed.OutputPath, "materials/fake.mdl")
	if err != nil {
		t.Fatal(err)
	}
	if modelPreview.Kind != "model" || !strings.Contains(modelPreview.Text, "无法解析模型头部") {
		t.Fatalf("模型条目应给信息卡与说明: %#v", modelPreview)
	}

	if _, err := app.PreviewVPKEntry(packed.OutputPath, "materials/missing.txt"); err == nil {
		t.Fatal("不存在的条目必须报错")
	}
}

// buildTinyWAV 造一个最小可播放的 PCM WAV（8kHz 单声道 16bit，8 个静音采样）。
func buildTinyWAV() []byte {
	data := make([]byte, 8*2)
	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(data)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint32(8000))
	binary.Write(&buf, binary.LittleEndian, uint32(8000*2))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)
	return buf.Bytes()
}

// 音频预览：.wav / .mp3 走 data URL（浏览器原生播放器），base64 解码后必须与源字节一致；
// 不在支持列表里的音频扩展名仍按"其它类型"只给说明。
func TestVPKPreviewReadsAudio(t *testing.T) {
	app := &App{}
	srcDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(srcDir, "sound"), 0o755); err != nil {
		t.Fatal(err)
	}
	wavBytes := buildTinyWAV()
	if err := os.WriteFile(filepath.Join(srcDir, "sound", "alert.wav"), wavBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sound", "theme.mp3"), []byte("ID3\x03\x00\x00\x00fake-mp3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sound", "extra.ogg"), []byte("OggSxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	packed, err := app.packVPKDirectoryWithOptions(srcDir, outDir, false, "audio-fixture", nil)
	if err != nil {
		t.Fatalf("打包夹具失败: %v", err)
	}

	wavPreview, err := app.PreviewVPKEntry(packed.OutputPath, "sound/alert.wav")
	if err != nil {
		t.Fatal(err)
	}
	if wavPreview.Kind != "audio" {
		t.Fatalf("wav 应走音频分支: %#v", wavPreview)
	}
	const wavPrefix = "data:audio/wav;base64,"
	if !strings.HasPrefix(wavPreview.DataURL, wavPrefix) {
		t.Fatalf("wav data URL 前缀不对: %s", wavPreview.DataURL[:min(32, len(wavPreview.DataURL))])
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(wavPreview.DataURL, wavPrefix))
	if err != nil {
		t.Fatalf("解码 base64 失败: %v", err)
	}
	if !bytes.Equal(decoded, wavBytes) {
		t.Fatalf("音频字节 round-trip 不一致（解码 %d 字节 / 源 %d 字节）", len(decoded), len(wavBytes))
	}

	mp3Preview, err := app.PreviewVPKEntry(packed.OutputPath, "sound/theme.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if mp3Preview.Kind != "audio" || !strings.HasPrefix(mp3Preview.DataURL, "data:audio/mpeg;base64,") {
		t.Fatalf("mp3 预览不对: %#v", mp3Preview)
	}

	oggPreview, err := app.PreviewVPKEntry(packed.OutputPath, "sound/extra.ogg")
	if err != nil {
		t.Fatal(err)
	}
	if oggPreview.Kind != "binary" || oggPreview.Note == "" {
		t.Fatalf(".ogg 不在支持列表里，应只给说明: %#v", oggPreview)
	}
}

// DXT1 解码：整块用 color0（红色 0xF800）时应铺满纯红、且不透明。
func TestDecodeDXTSolidBlock(t *testing.T) {
	bitmap := &vtf.Bitmap{
		Width:  4,
		Height: 4,
		Stride: 8,
		Format: vtf.ImageFormatDXT1,
		Pix:    []byte{0x00, 0xF8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	decodeDXT(img, bitmap)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			pixel := img.RGBAAt(x, y)
			if pixel.R != 255 || pixel.G != 0 || pixel.B != 0 || pixel.A != 255 {
				t.Fatalf("(%d,%d) = %#v，期望纯红不透明", x, y, pixel)
			}
		}
	}
}

// VTF 预览：不支持的格式要给出"可读说明"，而不是抛错或返回空白。
func TestPreviewVTFFromBytesRejectsUnknownFormat(t *testing.T) {
	if _, _, err := previewVTFFromBytes([]byte("not a vtf")); err == nil {
		t.Fatal("非 VTF 数据必须报错")
	}
}
