package stockfiles

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseLinesNormalizesAndSkipsComments(t *testing.T) {
	content := "# 注释\r\n\r\nMaterials\\Shared.VTF\r\nmaterials/shared.vtf\r\nscripts/vscripts/\r\n// 另一个注释\r\n"
	got := ParseLines(content)
	want := []string{"materials/shared.vtf", "scripts/vscripts/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseLines = %#v, want %#v", got, want)
	}
}

func TestLoadBuiltinExposesEngineGlueBatch(t *testing.T) {
	batches, descriptions := LoadBuiltin()
	if len(descriptions) == 0 {
		t.Fatal("内置批次不应为空")
	}
	var found bool
	for _, description := range descriptions {
		if description.Error != "" {
			t.Fatalf("内置批次 %s 解析失败: %s", description.Name, description.Error)
		}
		if description.Name == "engine-glue.txt" {
			found = true
			if description.Count == 0 {
				t.Fatal("engine-glue 批次不应为空")
			}
		}
	}
	if !found {
		t.Fatalf("缺少 engine-glue 批次: %#v", descriptions)
	}
	paths := batches["engine-glue.txt"]
	lookup := map[string]bool{}
	for _, path := range paths {
		lookup[path] = true
	}
	for _, want := range []string{"addoninfo.txt", "sound/sound.cache", "scripts/vscripts/director_base_addon.nut"} {
		if !lookup[want] {
			t.Fatalf("内置批次缺少 %q: %#v", want, paths)
		}
	}
}

func TestLoadDirectorySkipsMissingDirectoryAndReportsBadFile(t *testing.T) {
	batches, descriptions, err := LoadDirectory(filepath.Join(t.TempDir(), "not-there"))
	if err != nil {
		t.Fatalf("目录缺失不应报错: %v", err)
	}
	if len(batches) != 0 || len(descriptions) != 0 {
		t.Fatalf("目录缺失应返回空结果: %#v %#v", batches, descriptions)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "models.txt"), []byte("models/a.mdl\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	batches, descriptions, err = LoadDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(descriptions) != 1 || descriptions[0].Count != 1 || descriptions[0].Error != "" {
		t.Fatalf("批次描述 = %#v", descriptions)
	}
	if !reflect.DeepEqual(batches["models.txt"], []string{"models/a.mdl"}) {
		t.Fatalf("批次内容 = %#v", batches)
	}
}

func TestCategoryNameGroupsByTopLevelDirectory(t *testing.T) {
	cases := map[string]string{
		"models/survivor/coach.mdl": "models.txt",
		"Materials\\Shared.VTF":     "materials.txt",
		"iohints.txt":               "_root.txt",
		"":                          "",
	}
	for input, want := range cases {
		if got := CategoryName(input); got != want {
			t.Fatalf("CategoryName(%q) = %q, want %q", input, got, want)
		}
	}
}
