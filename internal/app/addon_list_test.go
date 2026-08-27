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

func TestApplyAddonListGameStatesMatchesRootAndWorkshopKeys(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0755); err != nil {
		t.Fatal(err)
	}
	addonList := "\"AddonList\"\n{\n\t\"root.vpk\"\t\t\"1\"\n\t\"workshop\\123.vpk\"\t\t\"0\"\n}\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(addonsDir), "addonlist.txt"), []byte(addonList), 0644); err != nil {
		t.Fatal(err)
	}

	rootPath := filepath.Join(addonsDir, "root.vpk")
	workshopPath := filepath.Join(addonsDir, "workshop", "123.vpk")
	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(rootPath, &VPKFileCache{File: VPKFile{Path: rootPath, Name: "root.vpk", Location: "root"}})
	app.vpkCache.Store(workshopPath, &VPKFileCache{File: VPKFile{Path: workshopPath, Name: "123.vpk", Location: "workshop"}})

	app.applyAddonListGameStates()

	rootFile := app.mustCachedVPKFile(t, rootPath)
	if !rootFile.GameStateKnown || !rootFile.GameEnabled {
		t.Fatalf("root state = known:%t enabled:%t, want true/true", rootFile.GameStateKnown, rootFile.GameEnabled)
	}
	workshopFile := app.mustCachedVPKFile(t, workshopPath)
	if !workshopFile.GameStateKnown || workshopFile.GameEnabled {
		t.Fatalf("workshop state = known:%t enabled:%t, want true/false", workshopFile.GameStateKnown, workshopFile.GameEnabled)
	}
}

func TestSetVPKGameEnabledPreservesGBKLayoutAndWorkshopKey(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0755); err != nil {
		t.Fatal(err)
	}

	original := "\"AddonList\"\r\n{\r\n\t// 保留这条注释\r\n\t\"workshop\\123.vpk\"\t\t\"0\" // 保留尾注释\r\n}\r\n"
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(original))
	if err != nil {
		t.Fatal(err)
	}
	addonListPath := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	if err := os.WriteFile(addonListPath, encoded, 0644); err != nil {
		t.Fatal(err)
	}

	vpkPath := filepath.Join(addonsDir, "workshop", "123.vpk")
	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{Path: vpkPath, Name: "123.vpk", Location: "workshop"}})

	if err := app.SetVPKGameEnabled(vpkPath, true); err != nil {
		t.Fatalf("SetVPKGameEnabled: %v", err)
	}

	updatedBytes, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	backupBytes, err := os.ReadFile(addonListPath + ".lytvpk.bak")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backupBytes, encoded) {
		t.Fatal("backup does not contain the original GBK bytes")
	}
	if utf8Bytes := bytes.Contains(updatedBytes, []byte("保留")); utf8Bytes {
		t.Fatal("addonlist.txt was unexpectedly rewritten as UTF-8")
	}
	updated, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), updatedBytes)
	if err != nil {
		t.Fatal(err)
	}
	expected := "\"AddonList\"\r\n{\r\n\t// 保留这条注释\r\n\t\"workshop\\123.vpk\"\t\t\"1\" // 保留尾注释\r\n}\r\n"
	if string(updated) != expected {
		t.Fatalf("unexpected preserved content:\nwant: %q\n got: %q", expected, string(updated))
	}

	file := app.mustCachedVPKFile(t, vpkPath)
	if !file.GameStateKnown || !file.GameEnabled {
		t.Fatalf("cached state = known:%t enabled:%t, want true/true", file.GameStateKnown, file.GameEnabled)
	}
}

func TestSetVPKGameEnabledAddsMissingEntryAndPreservesUTF8BOM(t *testing.T) {
	root := t.TempDir()
	addonsDir := filepath.Join(root, "left4dead2", "addons")
	if err := os.MkdirAll(addonsDir, 0755); err != nil {
		t.Fatal(err)
	}

	original := append([]byte{0xEF, 0xBB, 0xBF}, []byte("\"AddonList\"\n{\n\t\"existing.vpk\"\t\t\"1\"\n}\n")...)
	addonListPath := filepath.Join(filepath.Dir(addonsDir), "addonlist.txt")
	if err := os.WriteFile(addonListPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	vpkPath := filepath.Join(addonsDir, "new.vpk")
	app := &App{rootDir: addonsDir}
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{Path: vpkPath, Name: "new.vpk", Location: "root"}})

	if err := app.SetVPKGameEnabled(vpkPath, false); err != nil {
		t.Fatalf("SetVPKGameEnabled: %v", err)
	}

	updated, err := os.ReadFile(addonListPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := append([]byte{0xEF, 0xBB, 0xBF}, []byte("\"AddonList\"\n{\n\t\"existing.vpk\"\t\t\"1\"\n\t\"new.vpk\"\t\t\"0\"\n}\n")...)
	if !bytes.Equal(updated, expected) {
		t.Fatalf("unexpected updated content:\nwant: %q\n got: %q", expected, updated)
	}

	backup, err := os.ReadFile(addonListPath + ".lytvpk.bak")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatal("backup does not contain the original UTF-8 BOM bytes")
	}
}

func TestAddonListDocumentPreservesUTF16LE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addonlist.txt")
	content := "\"AddonList\"\n{\n\t\"中文.vpk\"\t\t\"1\"\n}\n"
	encoded, _, err := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder(), []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte{0xFF, 0xFE}, encoded...)
	if got := addonListEncodingName(original); got != "UTF-16 LE" {
		t.Fatalf("UTF-16 encoding label = %q", got)
	}
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := readAddonListDocumentAtPath(path)
	if err != nil {
		t.Fatalf("read UTF-16 addonlist: %v", err)
	}
	if doc.encoding != addonListEncodingUTF16LE || doc.content != content {
		t.Fatalf("UTF-16 document = encoding %d content %q", doc.encoding, doc.content)
	}
	updated, err := encodeAddonListDocument(doc, content+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) < 2 || !bytes.Equal(updated[:2], []byte{0xFF, 0xFE}) {
		t.Fatal("UTF-16 LE BOM was not preserved")
	}
	decoded, _, err := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder(), updated)
	if err != nil || string(decoded) != content+"\n" {
		t.Fatalf("UTF-16 round trip = %q, %v", decoded, err)
	}
}

func (a *App) mustCachedVPKFile(t *testing.T, path string) VPKFile {
	t.Helper()
	cached, ok := a.vpkCache.Load(path)
	if !ok {
		t.Fatalf("cache miss: %s", path)
	}
	return cached.(*VPKFileCache).File
}
