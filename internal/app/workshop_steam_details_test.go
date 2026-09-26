package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"vpk-manager/internal/grouping"
	"vpk-manager/internal/parser"
)

// steamDetailsTestServer 造一个 Steam 官方接口替身：**用真实抓取的响应当夹具**
// （testdata/steam_published_file_details.json，2026-09-24 从
//  ISteamRemoteStorage/GetPublishedFileDetails 真实抓下来的两个作品），
// 这样测试断言的是真实响应结构，而不是我们自己手写的形状。
func steamDetailsTestServer(t *testing.T) (*httptest.Server, *[]string, *sync.Mutex) {
	t.Helper()
	captured, err := os.ReadFile(filepath.Join("testdata", "steam_published_file_details.json"))
	if err != nil {
		t.Fatalf("读取真实响应夹具失败: %v", err)
	}
	var fixture struct {
		Response struct {
			PublishedFileDetails []map[string]any `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if err := json.Unmarshal(captured, &fixture); err != nil {
		t.Fatalf("解析真实响应夹具失败: %v", err)
	}

	received := make([]string, 0)
	mu := &sync.Mutex{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		count := r.PostFormValue("itemcount")
		ids := make([]string, 0)
		for index := 0; ; index++ {
			value := r.PostFormValue("publishedfileids[" + itoa(index) + "]")
			if value == "" {
				break
			}
			ids = append(ids, value)
		}
		mu.Lock()
		received = append(received, ids...)
		mu.Unlock()

		details := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			found := false
			for _, item := range fixture.Response.PublishedFileDetails {
				if idValue, ok := item["publishedfileid"].(string); ok && idValue == id {
					details = append(details, item)
					found = true
					break
				}
			}
			if !found {
				// 夹具里没有的 ID：返回"未找到"，用来验证 Failed 分支。
				details = append(details, map[string]any{"publishedfileid": id, "result": 9})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{"publishedfiledetails": details},
		})
		_ = count
	}))
	t.Cleanup(server.Close)
	return server, &received, mu
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func TestApplySteamDetailToMeta(t *testing.T) {
	fetchedAt := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	meta := &WorkshopMeta{
		WorkshopID: "2302720558",
		Title:      "本地标题",
		Tags:       []string{"音效"}, // 本项目自己的标签层级，不能被官方标签覆盖
		PrimaryTag: "音效",
	}
	detail := steamPublishedFileDetail{
		PublishedFileID:       "2302720558",
		Result:                1,
		Subscriptions:         21564,
		Favorited:             6515,
		LifetimeSubscriptions: 30000,
		LifetimeFavorited:     9000,
		Views:                 53208,
		Tags: []struct {
			Tag string `json:"tag"`
		}{
			{Tag: "Survivors"},
			{Tag: " Sounds "},
			{Tag: "sounds"},
			{Tag: ""},
		},
	}

	if !applySteamDetailToMeta(meta, detail, fetchedAt) {
		t.Fatal("首次合并应判定为有变化")
	}
	if len(meta.SteamTags) != 2 || meta.SteamTags[0] != "Survivors" || meta.SteamTags[1] != "Sounds" {
		t.Fatalf("官方标签应去空白、按大小写去重并保序，实际 %v", meta.SteamTags)
	}
	if meta.PrimaryTag != "音效" || len(meta.Tags) != 1 {
		t.Fatalf("本项目自己的标签不应被改动: %+v", meta)
	}
	if meta.Subscriptions != 21564 || meta.Views != 53208 || meta.Favorited != 6515 {
		t.Fatalf("统计未写入: %+v", meta)
	}
	if meta.StatsFetchedAt == "" {
		t.Fatal("应记录抓取时间")
	}

	// 同样的数据再来一次：不应判定为变化（避免无意义写盘）。
	if applySteamDetailToMeta(meta, detail, fetchedAt) {
		t.Fatal("相同数据不应判定为变化")
	}

	// 接口抽风返回 0：不能把已有统计清零。
	zeroed := steamPublishedFileDetail{PublishedFileID: "2302720558", Result: 1}
	if applySteamDetailToMeta(meta, zeroed, fetchedAt) {
		t.Fatal("全 0 的返回不应改动已有统计")
	}
	if meta.Subscriptions != 21564 {
		t.Fatalf("统计被清零了: %+v", meta)
	}
}

func TestNormalizeWorkshopIDList(t *testing.T) {
	got := normalizeWorkshopIDList([]string{"  123 ", "123", "direct-1", "", "abc", "456"})
	if len(got) != 2 || got[0] != "123" || got[1] != "456" {
		t.Fatalf("清洗结果 = %v", got)
	}
}

// 端到端：真实 .meta 文件 → 抓取 → 落盘 → 第二次调用判定未变化。
func TestEnrichWorkshopMetadataEndToEnd(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	server, received, mu := steamDetailsTestServer(t)
	previous := steamPublishedFileDetailsURL
	steamPublishedFileDetailsURL = server.URL
	t.Cleanup(func() { steamPublishedFileDetailsURL = previous })

	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(workshopDir, "2302720558.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{"scripts/addoninfo.txt": []byte("AddonInfo\n")})
	if err := saveWorkshopMeta(vpkPath, &WorkshopMeta{
		WorkshopID: "2302720558",
		Title:      "ASMR 音效",
		Tags:       []string{"音效"},
		PrimaryTag: "音效",
	}); err != nil {
		t.Fatalf("写 .meta 失败: %v", err)
	}

	result, err := a.EnrichWorkshopMetadata([]string{"2302720558", "404040"})
	if err != nil {
		t.Fatalf("抓取失败: %v", err)
	}
	if result.Requested != 2 || result.Updated != 1 {
		t.Fatalf("结果异常: %+v", result)
	}
	if len(result.Missing) != 1 || result.Missing[0] != "404040" {
		t.Fatalf("本地没有的 ID 应报 missing: %+v", result.Missing)
	}

	meta, err := LoadWorkshopMeta(vpkPath)
	if err != nil || meta == nil {
		t.Fatalf("读取 .meta 失败: %v", err)
	}
	// 真实响应里这个作品带 4 个官方标签：Survivors / Sounds / Single Player / Other。
	if len(meta.SteamTags) != 4 || meta.SteamTags[0] != "Survivors" || meta.SteamTags[3] != "Other" {
		t.Fatalf("官方标签未落盘: %+v", meta.SteamTags)
	}
	if meta.Subscriptions != 21564 || meta.Views != 53208 || meta.Favorited != 6515 {
		t.Fatalf("统计未落盘: %+v", meta)
	}
	if meta.Tags[0] != "音效" {
		t.Fatalf("本项目标签被覆盖: %+v", meta.Tags)
	}

	mu.Lock()
	gotIDs := strings.Join(*received, ",")
	mu.Unlock()
	if !strings.Contains(gotIDs, "2302720558") {
		t.Fatalf("没有把 ID 发给官方接口: %s", gotIDs)
	}

	// 第二次：同一批数据应判定为未变化（不重复写盘）。
	again, err := a.EnrichWorkshopMetadata([]string{"2302720558"})
	if err != nil {
		t.Fatal(err)
	}
	if again.Unchanged != 1 || again.Updated != 0 {
		t.Fatalf("第二次应为未变化: %+v", again)
	}
}

// 官方接口整批失败时：返回错误摘要，但不影响本地文件。
func TestEnrichWorkshopMetadataReportsBatchFailure(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	previous := steamPublishedFileDetailsURL
	steamPublishedFileDetailsURL = server.URL
	t.Cleanup(func() { steamPublishedFileDetailsURL = previous })

	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(workshopDir, "555.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{"scripts/addoninfo.txt": []byte("AddonInfo\n")})
	before, err := os.ReadFile(GetMetaFilePath(vpkPath))
	if err == nil {
		t.Fatalf("前置条件：此用例开始前不应有 .meta：%s", before)
	}

	result, err := a.EnrichWorkshopMetadata([]string{"555"})
	if err != nil {
		t.Fatalf("批量失败不应直接返回错误: %v", err)
	}
	if len(result.Failed) != 1 || len(result.Errors) == 0 {
		t.Fatalf("应记录失败与原因: %+v", result)
	}
	if _, statErr := os.Stat(GetMetaFilePath(vpkPath)); statErr == nil {
		t.Fatal("失败时不应写出 .meta")
	}
}

// 抓到的官方标签与统计必须进入"给智能体看的材料"（catalog 的 workshop 段）。
func TestWorkshopSnapshotExposesOfficialTagsAndStats(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	workshopDir := filepath.Join(addonsDir, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(workshopDir, "888.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{"scripts/addoninfo.txt": []byte("AddonInfo\n")})
	if err := saveWorkshopMeta(vpkPath, &WorkshopMeta{
		WorkshopID:    "888",
		Title:         "官方标签测试",
		Tags:          []string{"音效"},
		PrimaryTag:    "音效",
		SteamTags:     []string{"Sounds", "Single Player"},
		Subscriptions: 1200,
		Favorited:     340,
		Views:         5600,
	}); err != nil {
		t.Fatal(err)
	}

	snapshot := a.workshopSnapshotFor(
		parser.VPKFile{Path: vpkPath, WorkshopID: "888"},
		grouping.Mod{},
		nil,
	)
	if snapshot == nil {
		t.Fatal("应生成 workshop 快照")
	}
	if len(snapshot.SteamTags) != 2 || snapshot.SteamTags[0] != "Sounds" {
		t.Fatalf("官方标签未进入材料: %+v", snapshot.SteamTags)
	}
	if snapshot.Subscriptions != 1200 || snapshot.Favorited != 340 || snapshot.Views != 5600 {
		t.Fatalf("官方统计未进入材料: %+v", snapshot)
	}
	if len(snapshot.Tags) != 1 || snapshot.Tags[0] != "音效" {
		t.Fatalf("本项目标签不应被官方标签顶掉: %+v", snapshot.Tags)
	}
}
