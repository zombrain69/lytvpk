package app

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type PanelUser struct {
	Name     string `json:"name"`
	ID       int    `json:"id"`
	SteamID  string `json:"steamid"`
	IP       string `json:"ip"`
	Location string `json:"location"`
	Status   string `json:"status"`
	Delay    int    `json:"delay"`
	Loss     int    `json:"loss"`
	Duration string `json:"duration"`
	LinkRate int    `json:"linkrate"`
}

type PanelServerStatus struct {
	Users      []PanelUser `json:"users"`
	Players    string      `json:"players"`
	Map        string      `json:"map"`
	Hostname   string      `json:"hostname"`
	Name       string      `json:"name"`
	ServerName string      `json:"serverName"`
	Difficulty string      `json:"difficulty"`
	GameMode   string      `json:"gameMode"`
}

type PanelCampaign struct {
	Title    string         `json:"title"`
	Chapters []PanelChapter `json:"chapters"`
	VpkName  string         `json:"vpkName"`
}

type PanelMapFile struct {
	Name string `json:"name"`
	Size string `json:"size"`
}

type PanelChapter struct {
	Code  string   `json:"code"`
	Title string   `json:"title"`
	Modes []string `json:"modes"`
}

type PanelMapHotReloadStatus struct {
	UsingDefault bool `json:"using_default"`
}

type PanelMapHotReloadResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type PanelMapIssue struct {
	DictionaryMissing    int  `json:"dictionaryMissing"`
	DictionaryUnreadable bool `json:"dictionaryUnreadable"`
	GlobalScripts        int  `json:"globalScripts"`
	ScriptOverrides      int  `json:"scriptOverrides"`
}

type PanelMapIssuesResponse struct {
	Supported bool                     `json:"supported"`
	Items     map[string]PanelMapIssue `json:"items"`
}

type panelMapSummaryResponse struct {
	Items map[string]panelMapSummary `json:"items"`
}

type panelMapSummary struct {
	Error      string              `json:"error"`
	Inspection *panelMapInspection `json:"inspection"`
}

type panelMapInspection struct {
	Dictionary      panelMapDictionaryInspection `json:"dictionary"`
	GlobalScripts   panelMapGlobalScripts        `json:"global_scripts"`
	ScriptOverrides panelMapScriptOverrides      `json:"script_overrides"`
}

type panelMapDictionaryInspection struct {
	Status   string                      `json:"status"`
	Chapters []panelMapChapterInspection `json:"chapters"`
}

type panelMapChapterInspection struct {
	Status string `json:"status"`
}

type panelMapGlobalScripts struct {
	Status string   `json:"status"`
	Files  []string `json:"files"`
}

type panelMapScriptOverrides struct {
	Status string   `json:"status"`
	Files  []string `json:"files"`
}

type panelCredentials struct {
	baseURL    string
	password   string
	serverName string
}

func (a *App) FetchPanelServerStatus(serverID string) (*PanelServerStatus, error) {
	var status PanelServerStatus
	if _, err := a.panelPost(serverID, "/rcon/getstatus", nil, &status); err != nil {
		return nil, err
	}
	status.Hostname = firstNonEmpty(status.Hostname, status.Name, status.ServerName)
	return &status, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (a *App) RestartPanelServer(serverID string) (string, error) {
	return a.panelPost(serverID, "/restart", nil, nil)
}

func (a *App) FetchPanelMapList(serverID string) ([]PanelCampaign, error) {
	var maps []PanelCampaign
	if _, err := a.panelPost(serverID, "/rcon/maplist", nil, &maps); err != nil {
		return nil, err
	}
	if maps == nil {
		return []PanelCampaign{}, nil
	}
	return maps, nil
}

func (a *App) FetchPanelMapFiles(serverID string) ([]PanelMapFile, error) {
	raw, err := a.panelPost(serverID, "/list", nil, nil)
	if err != nil {
		return nil, err
	}
	return parsePanelMapFiles(raw), nil
}

func parsePanelMapFiles(raw string) []PanelMapFile {
	files := make([]PanelMapFile, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		name, size, found := strings.Cut(line, "$$")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		size = strings.TrimSpace(size)
		if !found || size == "" {
			size = "unknown"
		}
		files = append(files, PanelMapFile{Name: name, Size: size})
	}
	return files
}

func (a *App) FetchPanelMapIssues(serverID string, vpkNames []string) (*PanelMapIssuesResponse, error) {
	names := normalizePanelMapIssueNames(vpkNames)
	result := &PanelMapIssuesResponse{
		Supported: true,
		Items:     make(map[string]PanelMapIssue),
	}
	if len(names) == 0 {
		return result, nil
	}

	var summaries panelMapSummaryResponse
	response, err := a.panelPostJSON(serverID, "/maps/summary", map[string]interface{}{
		"maps": names,
	}, &summaries)
	if response != nil && (response.StatusCode() == 404 || response.StatusCode() == 405) {
		result.Supported = false
		return result, nil
	}
	if err != nil {
		return nil, err
	}

	for mapName, summary := range summaries.Items {
		if summary.Inspection == nil {
			continue
		}
		result.Items[mapName] = compactPanelMapIssue(*summary.Inspection)
	}
	return result, nil
}

func normalizePanelMapIssueNames(vpkNames []string) []string {
	result := make([]string, 0, len(vpkNames))
	seen := make(map[string]struct{}, len(vpkNames))
	for _, vpkName := range vpkNames {
		vpkName = strings.TrimSpace(vpkName)
		if vpkName == "" {
			continue
		}
		key := strings.ToLower(vpkName)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, vpkName)
	}
	return result
}

func compactPanelMapIssue(inspection panelMapInspection) PanelMapIssue {
	dictionaryStatus := strings.ToLower(strings.TrimSpace(inspection.Dictionary.Status))
	issue := PanelMapIssue{
		DictionaryUnreadable: dictionaryStatus == "unreadable",
	}
	if dictionaryStatus == "missing" || dictionaryStatus == "unreadable" {
		for _, chapter := range inspection.Dictionary.Chapters {
			switch {
			case strings.EqualFold(chapter.Status, "missing"):
				issue.DictionaryMissing++
			case strings.EqualFold(chapter.Status, "unreadable"):
				issue.DictionaryUnreadable = true
			}
		}
	}
	if strings.EqualFold(inspection.GlobalScripts.Status, "detected") {
		issue.GlobalScripts = len(inspection.GlobalScripts.Files)
	}
	if strings.EqualFold(inspection.ScriptOverrides.Status, "detected") {
		issue.ScriptOverrides = len(inspection.ScriptOverrides.Files)
	}
	return issue
}

func (a *App) ClearPanelMaps(serverID string) (string, error) {
	return a.panelPost(serverID, "/clear", nil, nil)
}

func (a *App) DeletePanelMapFile(serverID string, mapName string) (string, error) {
	mapName = strings.TrimSpace(mapName)
	if mapName == "" {
		return "", fmt.Errorf("地图文件名不能为空")
	}
	return a.panelPost(serverID, "/remove", map[string]string{"map": mapName}, nil)
}

func (a *App) ChangePanelMap(serverID string, mapName string) (string, error) {
	mapName = strings.TrimSpace(mapName)
	if mapName == "" {
		return "", fmt.Errorf("地图名称不能为空")
	}
	return a.panelPost(serverID, "/rcon/changemap", map[string]string{"mapName": mapName}, nil)
}

func (a *App) FetchPanelMapHotReloadStatus(serverID string) (*PanelMapHotReloadStatus, error) {
	var status PanelMapHotReloadStatus
	if _, err := a.panelPost(serverID, "/maps/hot-reload/status", nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (a *App) HotReloadPanelMaps(serverID string) (*PanelMapHotReloadResult, error) {
	var result PanelMapHotReloadResult
	if _, err := a.panelPost(serverID, "/maps/hot-reload", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) ChangePanelDifficulty(serverID string, difficulty string) (string, error) {
	difficulty = strings.TrimSpace(difficulty)
	switch difficulty {
	case "简单", "普通", "高级", "专家":
	default:
		return "", fmt.Errorf("难度无效")
	}
	return a.panelPost(serverID, "/rcon/changedifficulty", map[string]string{"difficulty": difficulty}, nil)
}

func (a *App) SendPanelRconCommand(serverID string, cmd string) (string, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", fmt.Errorf("RCON 指令不能为空")
	}
	return a.panelPost(serverID, "/rcon", map[string]string{"cmd": cmd}, nil)
}

func (a *App) getPanelCredentials(serverID string) (*panelCredentials, error) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return nil, fmt.Errorf("服务器 ID 不能为空")
	}

	a.ensureConfigPaths()
	var storage ServerStorage
	if err := readJSONFile(a.serversPath, &storage); err != nil {
		return nil, fmt.Errorf("读取服务器配置失败: %w", err)
	}

	for _, server := range storage.Servers {
		if server.ID != serverID {
			continue
		}
		baseURL, err := normalizePanelBaseURL(server.PanelURL)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(server.PanelPasswordEncrypted) == "" {
			return nil, fmt.Errorf("该服务器未保存面板密码")
		}
		password, err := unprotectSecret(server.PanelPasswordEncrypted)
		if err != nil {
			return nil, err
		}
		return &panelCredentials{
			baseURL:    baseURL,
			password:   password,
			serverName: firstNonEmpty(server.Name, server.Address, server.ID),
		}, nil
	}

	return nil, fmt.Errorf("未找到面板配置对应的服务器")
}

func (a *App) panelPost(serverID string, endpoint string, formData map[string]string, result interface{}) (string, error) {
	response, err := a.panelPostRequest(serverID, endpoint, func(request *resty.Request) {
		if formData != nil {
			request.SetFormData(formData)
		}
	}, result)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(response.String()), nil
}

func (a *App) panelPostJSON(serverID string, endpoint string, body interface{}, result interface{}) (*resty.Response, error) {
	return a.panelPostRequest(serverID, endpoint, func(request *resty.Request) {
		request.SetHeader("Content-Type", "application/json")
		request.SetBody(body)
	}, result)
}

func (a *App) panelPostRequest(serverID string, endpoint string, configure func(*resty.Request), result interface{}) (*resty.Response, error) {
	credentials, err := a.getPanelCredentials(serverID)
	if err != nil {
		return nil, err
	}

	requestURL, err := joinPanelEndpoint(credentials.baseURL, endpoint)
	if err != nil {
		return nil, err
	}

	client := resty.New().SetTimeout(8 * time.Second)
	request := client.R().SetHeader("Authorization", "Bearer "+credentials.password)
	if configure != nil {
		configure(request)
	}
	if result != nil {
		request.SetResult(result)
	}

	response, err := request.Post(requestURL)
	if err != nil {
		return response, fmt.Errorf("连接面板失败: %w", err)
	}
	if response.StatusCode() == 401 || response.StatusCode() == 429 {
		return response, fmt.Errorf("面板认证失败，请检查密码或稍后重试")
	}
	if response.StatusCode() == 403 {
		return response, fmt.Errorf("没有权限执行该面板操作")
	}
	if !response.IsSuccess() {
		body := strings.TrimSpace(response.String())
		if body == "" {
			body = response.Status()
		}
		return response, fmt.Errorf("面板请求失败(%d): %s", response.StatusCode(), body)
	}

	return response, nil
}

func normalizePanelBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("面板地址不能为空")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("面板地址格式无效: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("面板地址仅支持 http 或 https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("面板地址缺少主机")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func joinPanelEndpoint(base string, endpoint string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + endpoint
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
