package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestNormalizePanelMapIssueNames(t *testing.T) {
	got := normalizePanelMapIssueNames([]string{
		" first.vpk ",
		"",
		"FIRST.VPK",
		"second.vpk",
		"   ",
	})
	want := []string{"first.vpk", "second.vpk"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizePanelMapIssueNames() = %#v, want %#v", got, want)
	}
}

func TestParsePanelMapFiles(t *testing.T) {
	got := parsePanelMapFiles("first.vpk$$1.25GB\r\nsecond part.VPK$$unknown\nlegacy.vpk\n$$5MB\n")
	want := []PanelMapFile{
		{Name: "first.vpk", Size: "1.25GB"},
		{Name: "second part.VPK", Size: "unknown"},
		{Name: "legacy.vpk", Size: "unknown"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePanelMapFiles() = %#v, want %#v", got, want)
	}
}

func TestCompactPanelMapIssue(t *testing.T) {
	tests := []struct {
		name       string
		inspection panelMapInspection
		want       PanelMapIssue
	}{
		{
			name: "healthy map",
			inspection: panelMapInspection{
				Dictionary:      panelMapDictionaryInspection{Status: "present"},
				GlobalScripts:   panelMapGlobalScripts{Status: "clean"},
				ScriptOverrides: panelMapScriptOverrides{Status: "clean"},
			},
			want: PanelMapIssue{},
		},
		{
			name: "mixed missing and unreadable chapters",
			inspection: panelMapInspection{
				Dictionary: panelMapDictionaryInspection{
					Status: "missing",
					Chapters: []panelMapChapterInspection{
						{Status: "present"},
						{Status: "missing"},
						{Status: "MISSING"},
						{Status: "unreadable"},
					},
				},
				GlobalScripts: panelMapGlobalScripts{
					Status: "detected",
					Files:  []string{"scripts/vscripts/mapspawn_addon.nut", "scripts/vscripts/scriptedmode_addon.nut"},
				},
				ScriptOverrides: panelMapScriptOverrides{
					Status: "detected",
					Files:  []string{"scripts/gamemodes.txt", "scripts/weapon_rifle.txt", "scripts/weapon_smg.txt"},
				},
			},
			want: PanelMapIssue{
				DictionaryMissing:    2,
				DictionaryUnreadable: true,
				GlobalScripts:        2,
				ScriptOverrides:      3,
			},
		},
		{
			name: "overall dictionary unreadable",
			inspection: panelMapInspection{
				Dictionary: panelMapDictionaryInspection{Status: "unreadable"},
				GlobalScripts: panelMapGlobalScripts{
					Status: "clean",
					Files:  []string{"ignored.nut"},
				},
				ScriptOverrides: panelMapScriptOverrides{
					Status: "clean",
					Files:  []string{"ignored.txt"},
				},
			},
			want: PanelMapIssue{DictionaryUnreadable: true},
		},
		{
			name: "not checked ignores inconsistent details",
			inspection: panelMapInspection{
				Dictionary: panelMapDictionaryInspection{
					Status:   "not_checked",
					Chapters: []panelMapChapterInspection{{Status: "missing"}},
				},
				GlobalScripts: panelMapGlobalScripts{
					Status: "not_checked",
					Files:  []string{"ignored.nut"},
				},
				ScriptOverrides: panelMapScriptOverrides{
					Status: "not_checked",
					Files:  []string{"ignored.txt"},
				},
			},
			want: PanelMapIssue{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactPanelMapIssue(tt.inspection); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("compactPanelMapIssue() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFetchPanelMapIssues(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI 加密仅在 Windows 上验证")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/maps/summary" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer panel-secret" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
			t.Fatalf("unexpected content type: %q", contentType)
		}

		var request struct {
			Maps []string `json:"maps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		wantMaps := []string{"mixed.vpk", "clean.vpk", "resource-only.vpk", "broken.vpk"}
		if !reflect.DeepEqual(request.Maps, wantMaps) {
			t.Fatalf("request maps = %#v, want %#v", request.Maps, wantMaps)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items": {
				"mixed.vpk": {
					"error": "",
					"inspection": {
						"dictionary": {
							"status": "missing",
							"chapters": [
								{"status": "missing"},
								{"status": "unreadable"}
							]
						},
						"global_scripts": {
							"status": "detected",
							"files": ["one.nut", "two.nut"]
						},
						"script_overrides": {
							"status": "detected",
							"files": ["scripts/gamemodes.txt", "scripts/weapon_rifle.txt", "scripts/weapon_smg.txt"]
						}
					}
				},
				"clean.vpk": {
					"error": "",
					"inspection": {
						"dictionary": {"status": "present", "chapters": []},
						"global_scripts": {"status": "clean", "files": []},
						"script_overrides": {"status": "clean", "files": []}
					}
				},
				"resource-only.vpk": {
					"error": "解析地图名称失败: VPK 中未找到 missions",
					"inspection": {
						"dictionary": {"status": "not_applicable", "chapters": []},
						"global_scripts": {"status": "clean", "files": []},
						"script_overrides": {
							"status": "detected",
							"files": ["scripts/gamemodes.txt", "scripts/weapon_rifle.txt"]
						}
					}
				},
				"broken.vpk": {"error": "地图文件不存在"}
			}
		}`))
	}))
	defer server.Close()

	app := newPanelIssueTestApp(t, server.URL+"/panel")
	result, err := app.FetchPanelMapIssues("srv_panel_issues", []string{
		" mixed.vpk ",
		"MIXED.VPK",
		"clean.vpk",
		"resource-only.vpk",
		"",
		"broken.vpk",
	})
	if err != nil {
		t.Fatalf("FetchPanelMapIssues() error = %v", err)
	}
	if !result.Supported {
		t.Fatal("expected map issues endpoint to be supported")
	}
	wantMixed := PanelMapIssue{
		DictionaryMissing:    1,
		DictionaryUnreadable: true,
		GlobalScripts:        2,
		ScriptOverrides:      3,
	}
	if got := result.Items["mixed.vpk"]; !reflect.DeepEqual(got, wantMixed) {
		t.Fatalf("mixed issue = %#v, want %#v", got, wantMixed)
	}
	if got, exists := result.Items["clean.vpk"]; !exists || got != (PanelMapIssue{}) {
		t.Fatalf("clean issue = %#v, exists = %t", got, exists)
	}
	if got := result.Items["resource-only.vpk"]; got.ScriptOverrides != 2 {
		t.Fatalf("resource-only issue = %#v, want 2 script overrides", got)
	}
	if _, exists := result.Items["broken.vpk"]; exists {
		t.Fatal("map without an inspection should not produce an issue item")
	}
}

func TestFetchPanelMapIssuesEndpointCompatibility(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI 加密仅在 Windows 上验证")
	}

	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(status), status)
			}))
			defer server.Close()

			app := newPanelIssueTestApp(t, server.URL)
			result, err := app.FetchPanelMapIssues("srv_panel_issues", []string{"map.vpk"})
			if status == http.StatusInternalServerError {
				if err == nil {
					t.Fatal("expected server error")
				}
				if result != nil {
					t.Fatalf("result = %#v, want nil", result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unsupported endpoint returned error: %v", err)
			}
			if result == nil || result.Supported {
				t.Fatalf("result = %#v, want unsupported response", result)
			}
		})
	}
}

func TestFetchAndDeletePanelMapFiles(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI 加密仅在 Windows 上验证")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer panel-secret" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}

		switch r.URL.Path {
		case "/panel/list":
			_, _ = w.Write([]byte("campaign.vpk$$512.00MB\nresources.vpk$$128.00MB\n"))
		case "/panel/remove":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse remove form: %v", err)
			}
			if got := r.Form.Get("map"); got != "resources.vpk" {
				t.Fatalf("remove map = %q, want resources.vpk", got)
			}
			_, _ = w.Write([]byte("删除成功！"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := newPanelIssueTestApp(t, server.URL+"/panel")
	files, err := app.FetchPanelMapFiles("srv_panel_issues")
	if err != nil {
		t.Fatalf("FetchPanelMapFiles() error = %v", err)
	}
	wantFiles := []PanelMapFile{
		{Name: "campaign.vpk", Size: "512.00MB"},
		{Name: "resources.vpk", Size: "128.00MB"},
	}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("FetchPanelMapFiles() = %#v, want %#v", files, wantFiles)
	}

	message, err := app.DeletePanelMapFile("srv_panel_issues", " resources.vpk ")
	if err != nil {
		t.Fatalf("DeletePanelMapFile() error = %v", err)
	}
	if message != "删除成功！" {
		t.Fatalf("DeletePanelMapFile() = %q, want 删除成功！", message)
	}
}

func TestDeletePanelMapFileRejectsEmptyName(t *testing.T) {
	if _, err := (&App{}).DeletePanelMapFile("server", "  "); err == nil {
		t.Fatal("DeletePanelMapFile() expected empty name error")
	}
}

func newPanelIssueTestApp(t *testing.T, panelURL string) *App {
	t.Helper()
	app := newConfigTestApp(t)
	if err := app.SaveServerStorage(ServerStorage{
		Servers: []SavedServer{{
			ID:            "srv_panel_issues",
			Name:          "Panel Issues",
			Address:       "127.0.0.1:27015",
			PanelURL:      panelURL,
			PanelPassword: "panel-secret",
		}},
	}); err != nil {
		t.Fatalf("save panel config: %v", err)
	}
	return app
}
