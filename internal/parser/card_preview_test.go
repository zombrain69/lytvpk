package parser

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestEncodeCardPreviewReaderDownsamplesToTwoXCardBounds(t *testing.T) {
	fixture := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	fixture.Set(0, 0, color.RGBA{R: 0x4c, G: 0xb8, B: 0x6a, A: 0xff})
	var source bytes.Buffer
	if err := png.Encode(&source, fixture); err != nil {
		t.Fatal(err)
	}

	dataURL, err := encodeCardPreviewReader(bytes.NewReader(source.Bytes()))
	if err != nil {
		t.Fatalf("encode card preview: %v", err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		t.Fatalf("card preview format = %q, want PNG data URL", dataURL[:min(len(dataURL), 32)])
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, prefix))
	if err != nil {
		t.Fatalf("decode data URL: %v", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("decode thumbnail config: %v", err)
	}
	if format != "png" || config.Width != 512 || config.Height != 288 {
		t.Fatalf("thumbnail dimensions = %dx%d %s, want 512x288 PNG", config.Width, config.Height, format)
	}
}

func TestEncodeCardPreviewReaderRejectsOversizedSource(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, cardPreviewMaxSourceBytes+1)
	if _, err := encodeCardPreviewReader(bytes.NewReader(data)); err == nil {
		t.Fatal("oversized card preview source was accepted")
	}
}
