package parser

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"l4d2-manager-next/pkg/valve/vpk"
)

func gbkBytes(t *testing.T, value string) []byte {
	t.Helper()
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(value))
	if err != nil {
		t.Fatalf("encode GBK fixture: %v", err)
	}
	return encoded
}

func windows1252Bytes(t *testing.T, value string) []byte {
	t.Helper()
	encoded, _, err := transform.Bytes(charmap.Windows1252.NewEncoder(), []byte(value))
	if err != nil {
		t.Fatalf("encode Windows-1252 fixture: %v", err)
	}
	return encoded
}

func TestDecodeVPKTextSupportsGBKAndUTF8BOM(t *testing.T) {
	want := "\u6d4b\u8bd5\u63cf\u8ff0"
	got, err := DecodeVPKText(gbkBytes(t, want))
	if err != nil {
		t.Fatalf("decode GBK: %v", err)
	}
	if got != want {
		t.Fatalf("GBK decoded as %q, want %q", got, want)
	}

	utf8Data := append([]byte{0xEF, 0xBB, 0xBF}, []byte(want)...)
	got, err = DecodeVPKText(utf8Data)
	if err != nil {
		t.Fatalf("decode UTF-8 BOM: %v", err)
	}
	if got != want {
		t.Fatalf("UTF-8 BOM decoded as %q, want %q", got, want)
	}
}

func TestDecodeVPKTextSupportsWindows1252ANSI(t *testing.T) {
	want := "The Footsteps – Café Ê"
	got, err := DecodeVPKText(windows1252Bytes(t, want))
	if err != nil {
		t.Fatalf("decode Windows-1252: %v", err)
	}
	if got != want {
		t.Fatalf("Windows-1252 decoded as %q, want %q", got, want)
	}
}

func TestDecodeVPKTextSupportsUTF16BOM(t *testing.T) {
	want := "UTF-16 中文"
	for _, tc := range []struct {
		name   string
		endian unicode.Endianness
		bom    []byte
	}{
		{name: "LE", endian: unicode.LittleEndian, bom: []byte{0xFF, 0xFE}},
		{name: "BE", endian: unicode.BigEndian, bom: []byte{0xFE, 0xFF}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, _, err := transform.Bytes(unicode.UTF16(tc.endian, unicode.IgnoreBOM).NewEncoder(), []byte(want))
			if err != nil {
				t.Fatalf("encode UTF-16 fixture: %v", err)
			}
			encoded = append(tc.bom, encoded...)
			got, err := DecodeVPKText(encoded)
			if err != nil || got != want {
				t.Fatalf("UTF-16 decoded as %q, %v", got, err)
			}
		})
	}
}

func TestVPKEntryNameGBKRoundTrip(t *testing.T) {
	want := "materials/\u82b1/\u9888\u73af_uv.vmt"
	encoded := EncodeVPKEntryName(want)
	if bytes.Equal([]byte(encoded), []byte(want)) {
		t.Fatal("expected Chinese VPK entry name to use GBK bytes")
	}
	got, err := DecodeVPKEntryName(encoded)
	if err != nil {
		t.Fatalf("decode VPK name: %v", err)
	}
	if got != want {
		t.Fatalf("VPK name decoded as %q, want %q", got, want)
	}
}

func TestDecodeVPKEntryNameWindows1252(t *testing.T) {
	want := "sound/café.vpk"
	encoded := windows1252Bytes(t, want)
	got, err := DecodeVPKEntryName(string(encoded))
	if err != nil {
		t.Fatalf("decode Windows-1252 VPK name: %v", err)
	}
	if got != want {
		t.Fatalf("Windows-1252 VPK name decoded as %q, want %q", got, want)
	}
}

func TestParseVPKFileMetadataDecodesGBKAddonInfo(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "gbk-addoninfo.vpk")
	content := gbkBytes(t, "\"AddonInfo\"\n{\n\t\"addontitle\" \"中文标题\"\n\t\"addonauthor\" \"作者\"\n}\n")
	archive := &vpk.Archive{
		Header: vpk.Header{Magic: vpk.Magic, Version: 1},
		Files: []vpk.File{{
			Dir: " ", Base: "addoninfo", Ext: "txt",
			DirEntry: vpk.DirEntry{
				CRC:          crc32.ChecksumIEEE(content),
				DataLocation: []vpk.DataChunk{{ArchiveIndex: 0x7fff, EntryOffset: 0, EntryLength: uint32(len(content))}},
			},
		}},
	}
	var buffer bytes.Buffer
	if err := vpk.WriteDirectory(&buffer, archive); err != nil {
		t.Fatal(err)
	}
	buffer.Write(content)
	if err := os.WriteFile(vpkPath, buffer.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	file, err := ParseVPKFileMetadata(vpkPath)
	if err != nil {
		t.Fatal(err)
	}
	if file.Title != "中文标题" || file.Author != "作者" {
		t.Fatalf("GBK addoninfo parsed as title=%q author=%q", file.Title, file.Author)
	}
}

func TestEncodeVPKEntryNameKeepsUnrepresentableUTF8(t *testing.T) {
	want := "materials/emoji_😀.vmt"
	encoded := EncodeVPKEntryName(want)
	if encoded != want {
		t.Fatalf("unrepresentable name changed to %q", encoded)
	}
	got, err := DecodeVPKEntryName(encoded)
	if err != nil || got != want {
		t.Fatalf("UTF-8 fallback round trip = %q, %v", got, err)
	}
}
