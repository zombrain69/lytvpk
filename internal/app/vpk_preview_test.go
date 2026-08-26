package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestScanVPKFilesDefersPreviewImageUntilRequested(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "addons")
	sourceDir := filepath.Join(tempDir, "preview_fixture")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "addon.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	var imageData bytes.Buffer
	// Ensure the fixture has a visible non-transparent pixel while keeping it tiny.
	fixtureImage := image.NewRGBA(image.Rect(0, 0, 1, 1))
	fixtureImage.Set(0, 0, color.RGBA{R: 0x4c, G: 0xb8, B: 0x6a, A: 0xff})
	if err := png.Encode(&imageData, fixtureImage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "addonimage.png"), imageData.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	packer := &App{}
	if _, err := packer.PackVPKDirectory(sourceDir, rootDir, false); err != nil {
		t.Fatalf("pack preview VPK: %v", err)
	}

	vpkPath := filepath.Join(rootDir, "preview_fixture.vpk")
	app := &App{rootDir: rootDir} // nil pool exercises the synchronous safe fallback.
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("scan VPK files: %v", err)
	}

	cached, ok := app.vpkCache.Load(vpkPath)
	if !ok {
		t.Fatalf("scan did not cache %s", vpkPath)
	}
	if preview := cached.(*VPKFileCache).File.PreviewImage; preview != "" {
		t.Fatalf("normal scan retained preview image (%d bytes)", len(preview))
	}

	preview := app.GetVPKPreviewImage(vpkPath)
	if preview == "" {
		t.Fatal("on-demand preview image was empty")
	}
	if cached.(*VPKFileCache).File.PreviewImage != "" {
		t.Fatal("on-demand preview must not make the full scan cache retain Base64 data")
	}
}
