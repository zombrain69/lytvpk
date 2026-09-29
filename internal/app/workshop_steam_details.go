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

// steamTag 是官方接口里的标签条目。
type steamTag struct {
	Tag string `json:"tag"`
}

// steamPublishedFileDetail 是 Steam 官方接口的返回条目（只取本项目要用的字段）。
type steamPublishedFileDetail struct {
	PublishedFileID       string     `json:"publishedfileid"`
	Result                int        `json:"result"`
	Title                 string     `json:"title"`
	Subscriptions         uint32     `json:"subscriptions"`
	Favorited             uint32     `json:"favorited"`
	LifetimeSubscriptions uint32     `json:"lifetime_subscriptions"`
	LifetimeFavorited     uint32     `json:"lifetime_favorited"`
	Views                 uint32     `json:"views"`
	Tags                  []steamTag `json:"tags"`
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
//
// 先走 Steam 官方接口；如果直连被拦/超时（例如本机需要系统代理才能访问
// api.steampowered.com），退回本项目工坊 worker 的 /detail —— 这与应用其它
// 工坊功能使用同一个可达入口，避免「只有抓取标签/统计失败」的割裂体验。
func fetchSteamPublishedFileDetails(client *http.Client, ids []string) (map[string]steamPublishedFileDetail, error) {
	if len(ids) == 0 {
		return map[string]steamPublishedFileDetail{}, nil
	}

	official, officialErr := fetchSteamPublishedFileDetailsOfficial(client, ids)
	if officialErr == nil && len(official) > 0 {
		return official, nil
	}

	fallback, fallbackErr := workshopWorkerDetailFetcher(ids)
	if len(fallback) > 0 {
		for id, detail := range official {
			fallback[id] = detail
		}
		return fallback, nil
	}
	if officialErr != nil {
		return official, officialErr
	}
	if fallbackErr != nil {
		return official, fallbackErr
	}
	return official, nil
}

// fetchSteamPublishedFileDetailsOfficial 只走 Steam 官方接口（原始实现）。
func fetchSteamPublishedFileDetailsOfficial(client *http.Client, ids []string) (map[string]steamPublishedFileDetail, error) {
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

// workshopWorkerDetailFetcher 是「退回工坊 worker 取详情」的可注入入口（测试替换用）。
var workshopWorkerDetailFetcher = fetchWorkshopWorkerDetails

// fetchWorkshopWorkerDetails 逐个工坊 ID 调用 worker 的 /detail，并映射成官方结构。
// 用 getWorkshopClient（15s 超时 + IPv4），与浏览/详情功能走同一条可达链路。
func fetchWorkshopWorkerDetails(ids []string) (map[string]steamPublishedFileDetail, error) {
	result := make(map[string]steamPublishedFileDetail, len(ids))
	client := getWorkshopClient()
	var lastErr error
	for _, id := range ids {
		var payload SteamDetailResponse
		response, err := client.R().
			SetQueryParam("id", id).
			SetResult(&payload).
			Get(WorkshopWorkerURL + "/detail")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode() != http.StatusOK {
			lastErr = fmt.Errorf("工坊接口返回 %d", response.StatusCode())
			continue
		}
		if len(payload.Response.PublishedFileDetails) == 0 {
			lastErr = fmt.Errorf("工坊接口没有返回作品 %s 的详情", id)
			continue
		}
		result[id] = workerItemToSteamDetail(payload.Response.PublishedFileDetails[0], id)
	}
	if len(result) == 0 && lastErr != nil {
		return result, lastErr
	}
	return result, nil
}

// workerItemToSteamDetail 把 worker 作品详情映射成官方结构（只映射本项目会用到的字段）。
func workerItemToSteamDetail(item WorkshopItemDetail, id string) steamPublishedFileDetail {
	detail := steamPublishedFileDetail{
		PublishedFileID: firstNonEmptyString(item.PublishedFileId, id),
		Result:          1,
		Title:           item.Title,
		Subscriptions:   interfaceToUint32(item.Subscriptions),
		Favorited:       interfaceToUint32(item.Favorited),
		Views:           interfaceToUint32(item.Views),
	}
	for _, tag := range item.Tags {
		name := strings.TrimSpace(tag.Tag)
		if name == "" {
			continue
		}
		detail.Tags = append(detail.Tags, steamTag{Tag: name})
	}
	return detail
}

// interfaceToUint32 兼容 worker 返回的数字/字符串（interface{} 字段）。
func interfaceToUint32(value any) uint32 {
	switch typed := value.(type) {
	case nil:
		return 0
	case float64:
		if typed <= 0 {
			return 0
		}
		return uint32(typed)
	case float32:
		if typed <= 0 {
			return 0
		}
		return uint32(typed)
	case int:
		if typed <= 0 {
			return 0
		}
		return uint32(typed)
	case int64:
		if typed <= 0 {
			return 0
		}
		return uint32(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed <= 0 {
			return 0
		}
		return uint32(parsed)
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 32)
		if err != nil {
			return 0
		}
		return uint32(parsed)
	default:
		return 0
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
