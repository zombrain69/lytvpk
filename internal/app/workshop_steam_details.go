package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 工坊官方标签与统计采集。
//
// 对齐 FireAxe `PublishedFileUtils.GetPublishedFileDetailsAsync` +
// `PublishedFileDetails.cs`（tags / subscriptions / favorited / lifetime_* / views）：
// FireAxe 直接问 Steam 官方接口要这些字段，而不是依赖第三方解析服务。
//
// 为什么本项目需要补这一块（实测结论，2026-09-24）：
//   - 现有的镜像接口 l4d2-workshop-parse.laoyutang.cn 只回
//     result/publishedfileid/file_type/filename/file_size/file_url/preview_url/title/file_description/children，
//     **没有 tags**；
//   - Steam 官方 ISteamRemoteStorage/GetPublishedFileDetails 对同一个作品返回
//     tags（如 Survivors / Sounds / Single Player / Other）以及订阅数、收藏数、浏览量。
//
// 采集结果是"增补"：拿不到就保持原样，绝不覆盖本项目的标签层级（Tags）。

const (
	// steamPublishedFileDetailsBatches 是官方接口单次请求的条目上限。
	steamPublishedFileDetailsBatch = 50
	// steamPublishedFileDetailsTimeout 是单批请求的超时。
	steamPublishedFileDetailsTimeout = 25 * time.Second
)

// steamPublishedFileDetailsURL 可注入：测试用 httptest 替身，生产用官方端点。
var steamPublishedFileDetailsURL = "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"

// steamPublishedFileDetail 是 Steam 官方接口的返回条目（只取本项目要用的字段）。
type steamPublishedFileDetail struct {
	PublishedFileID       string `json:"publishedfileid"`
	Result                int    `json:"result"`
	Title                 string `json:"title"`
	Subscriptions         uint32 `json:"subscriptions"`
	Favorited             uint32 `json:"favorited"`
	LifetimeSubscriptions uint32 `json:"lifetime_subscriptions"`
	LifetimeFavorited     uint32 `json:"lifetime_favorited"`
	Views                 uint32 `json:"views"`
	Tags                  []struct {
		Tag string `json:"tag"`
	} `json:"tags"`
}

type steamPublishedFileDetailsResponse struct {
	Response struct {
		PublishedFileDetails []steamPublishedFileDetail `json:"publishedfiledetails"`
	} `json:"response"`
}

// steamTagNames 提取并清洗官方标签：去空白、去重、保持原顺序。
func steamTagNames(detail steamPublishedFileDetail) []string {
	names := make([]string, 0, len(detail.Tags))
	seen := make(map[string]bool, len(detail.Tags))
	for _, tag := range detail.Tags {
		name := strings.TrimSpace(tag.Tag)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		names = append(names, name)
	}
	return names
}

// fetchSteamPublishedFileDetails 拉取一批作品的官方详情。
func fetchSteamPublishedFileDetails(client *http.Client, ids []string) (map[string]steamPublishedFileDetail, error) {
	result := make(map[string]steamPublishedFileDetail, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	if client == nil {
		client = &http.Client{Timeout: steamPublishedFileDetailsTimeout}
	}

	form := url.Values{}
	form.Set("itemcount", strconv.Itoa(len(ids)))
	for index, id := range ids {
		form.Set(fmt.Sprintf("publishedfileids[%d]", index), id)
	}

	response, err := client.PostForm(steamPublishedFileDetailsURL, form)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return result, err
	}
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("Steam 官方接口返回 %d", response.StatusCode)
	}

	var payload steamPublishedFileDetailsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return result, fmt.Errorf("解析 Steam 官方接口失败: %w", err)
	}
	for _, detail := range payload.Response.PublishedFileDetails {
		id := strings.TrimSpace(detail.PublishedFileID)
		if id == "" || detail.Result != 1 {
			continue
		}
		result[id] = detail
	}
	return result, nil
}

// applySteamDetailToMeta 把官方详情合并进 .meta；返回是否有变化。
//
// 纯函数（不落盘），规则：
//   - 官方标签整体替换（以最新抓取为准），但**不动**本项目的 Tags / PrimaryTag / SecondaryTags；
//   - 统计字段只在新值非 0 时覆盖，避免接口抽风把已有数据清零；
//   - 完全没变化时返回 false，调用方就不写盘（减少磁盘写入与备份轮转）。
func applySteamDetailToMeta(meta *WorkshopMeta, detail steamPublishedFileDetail, fetchedAt time.Time) bool {
	if meta == nil {
		return false
	}
	changed := false

	tags := steamTagNames(detail)
	if len(tags) > 0 && !stringSlicesEqual(meta.SteamTags, tags) {
		meta.SteamTags = tags
		changed = true
	}

	if detail.Subscriptions > 0 && meta.Subscriptions != detail.Subscriptions {
		meta.Subscriptions = detail.Subscriptions
		changed = true
	}
	if detail.Favorited > 0 && meta.Favorited != detail.Favorited {
		meta.Favorited = detail.Favorited
		changed = true
	}
	if detail.LifetimeSubscriptions > 0 && meta.LifetimeSubscriptions != detail.LifetimeSubscriptions {
		meta.LifetimeSubscriptions = detail.LifetimeSubscriptions
		changed = true
	}
	if detail.LifetimeFavorited > 0 && meta.LifetimeFavorited != detail.LifetimeFavorited {
		meta.LifetimeFavorited = detail.LifetimeFavorited
		changed = true
	}
	if detail.Views > 0 && meta.Views != detail.Views {
		meta.Views = detail.Views
		changed = true
	}

	if changed {
		meta.StatsFetchedAt = fetchedAt.Format(time.RFC3339)
	}
	return changed
}

// stringSlicesEqual 比较两个字符串切片（顺序敏感：官方标签顺序本身有意义）。
func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// WorkshopEnrichResult 描述一次"抓取工坊官方标签与统计"的结果。
type WorkshopEnrichResult struct {
	Requested int      `json:"requested"`
	Updated   int      `json:"updated"`
	Unchanged int      `json:"unchanged"`
	Missing   []string `json:"missing,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

// normalizeWorkshopIDList 清洗工坊 ID 列表：去空白、去重、剔除非工坊条目。
func normalizeWorkshopIDList(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || strings.HasPrefix(strings.ToLower(id), "direct-") || seen[id] {
			continue
		}
		if _, err := strconv.ParseUint(id, 10, 64); err != nil {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

// workshopMetaPathForID 返回某个工坊 ID 对应的 .meta 路径（workshop 子目录）。
func (a *App) workshopMetaPathForID(id string) (string, string) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return "", ""
	}
	vpkPath := filepath.Join(rootDir, "workshop", id+".vpk")
	if _, err := os.Stat(vpkPath); err != nil {
		return "", ""
	}
	return vpkPath, GetMetaFilePath(vpkPath)
}

// EnrichWorkshopMetadata 按工坊 ID 抓取官方标签与统计，写进同名 .meta。
// 纯增量、best-effort：单个失败不影响其它，返回统计结果供界面提示。
func (a *App) EnrichWorkshopMetadata(workshopIDs []string) (WorkshopEnrichResult, error) {
	ids := normalizeWorkshopIDList(workshopIDs)
	result := WorkshopEnrichResult{Requested: len(ids)}
	if len(ids) == 0 {
		return result, fmt.Errorf("没有可抓取的工坊 ID")
	}

	type target struct {
		id       string
		vpkPath  string
		metaPath string
	}
	targets := make([]target, 0, len(ids))
	for _, id := range ids {
		vpkPath, metaPath := a.workshopMetaPathForID(id)
		if vpkPath == "" {
			result.Missing = append(result.Missing, id)
			continue
		}
		targets = append(targets, target{id: id, vpkPath: vpkPath, metaPath: metaPath})
	}
	if len(targets) == 0 {
		return result, nil
	}

	client := &http.Client{Timeout: steamPublishedFileDetailsTimeout}
	details := make(map[string]steamPublishedFileDetail, len(targets))
	for start := 0; start < len(targets); start += steamPublishedFileDetailsBatch {
		end := min(start+steamPublishedFileDetailsBatch, len(targets))
		batch := make([]string, 0, end-start)
		for _, item := range targets[start:end] {
			batch = append(batch, item.id)
		}
		fetched, err := fetchSteamPublishedFileDetails(client, batch)
		if err != nil {
			// 整批失败：记下来继续下一批，不让一次网络抖动毁掉整次采集。
			result.Errors = append(result.Errors, err.Error())
			log.Printf("抓取工坊官方详情失败（%d 个）：%v", len(batch), err)
			for _, id := range batch {
				result.Failed = append(result.Failed, id)
			}
			continue
		}
		for id, detail := range fetched {
			details[id] = detail
		}
		for _, id := range batch {
			if _, ok := fetched[id]; !ok {
				result.Failed = append(result.Failed, id)
			}
		}
	}

	fetchedAt := time.Now()
	for _, item := range targets {
		detail, ok := details[item.id]
		if !ok {
			continue
		}
		meta, err := LoadWorkshopMeta(item.vpkPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.id, err))
			continue
		}
		if meta == nil {
			// 没有 .meta 的（例如手动塞进 workshop 目录的文件）先建一份最小记录。
			meta = &WorkshopMeta{WorkshopID: item.id}
		}
		if applySteamDetailToMeta(meta, detail, fetchedAt) {
			if err := saveWorkshopMeta(item.vpkPath, meta); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.id, err))
				continue
			}
			result.Updated++
			continue
		}
		result.Unchanged++
	}

	sort.Strings(result.Missing)
	sort.Strings(result.Failed)
	return result, nil
}

// EnrichAllWorkshopMetadata 抓取 workshop 目录下所有工坊 Mod 的官方标签与统计。
func (a *App) EnrichAllWorkshopMetadata() (WorkshopEnrichResult, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return WorkshopEnrichResult{}, fmt.Errorf("未选择L4D2目录")
	}
	workshopDir := filepath.Join(rootDir, "workshop")
	entries, err := os.ReadDir(workshopDir)
	if err != nil {
		return WorkshopEnrichResult{}, fmt.Errorf("无法读取 workshop 目录: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".vpk") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
	}
	if len(ids) == 0 {
		return WorkshopEnrichResult{}, fmt.Errorf("workshop 目录下没有 VPK 文件")
	}
	return a.EnrichWorkshopMetadata(ids)
}
