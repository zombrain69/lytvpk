package app

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// 全局变量存储下载地址，避免 DoUpdate 时再次请求 API 导致速率限制或网络错误
var pendingUpdateURL string

const (
	unconfiguredUpdateSourceMessage = "此 Community Fork 尚未配置 GitHub 发布源；不会检查或安装 LaoYutang/lytvpk 的更新。"
	developmentBuildVersion         = "0.0.0-dev"
)

var githubRepositoryPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?/[A-Za-z0-9_.-]+$`)

// UpdateInfo 返回给前端的结构体
type UpdateInfo struct {
	HasUpdate   bool   `json:"has_update"`
	LatestVer   string `json:"latest_ver"`
	CurrentVer  string `json:"current_ver"`
	ReleaseNote string `json:"release_note"`
	DownloadURL string `json:"download_url"`
	Error       string `json:"error,omitempty"`
}

// ForkInfo gives the frontend a single source of truth for the fork identity.
// The upstream URL remains visible for attribution but is never a release URL.
type ForkInfo struct {
	Name         string `json:"name"`
	AppVersion   string `json:"app_version"`
	UpstreamRepo string `json:"upstream_repo"`
	UpdateRepo   string `json:"update_repo"`
	SourceURL    string `json:"source_url"`
	IssuesURL    string `json:"issues_url"`
	Configured   bool   `json:"configured"`
	Error        string `json:"error,omitempty"`
}

// GithubRelease 简化的 GitHub Release 结构
type GithubRelease struct {
	TagName string               `json:"tag_name"`
	Body    string               `json:"body"`
	Assets  []GithubReleaseAsset `json:"assets"`
}

// GithubReleaseAsset is deliberately named so updater selection can be unit
// tested without relying on a live GitHub response.
type GithubReleaseAsset struct {
	BrowserDownloadURL string `json:"browser_download_url"`
	Name               string `json:"name"`
}

func configuredUpdateRepo() (string, error) {
	repo := strings.TrimSpace(UpdateRepo)
	if repo == "" {
		return "", fmt.Errorf(unconfiguredUpdateSourceMessage)
	}
	if !githubRepositoryPattern.MatchString(repo) {
		return "", fmt.Errorf("Fork GitHub 发布源格式无效（应为 owner/repo）")
	}
	if strings.EqualFold(repo, UpstreamGithubRepo) {
		return "", fmt.Errorf("Fork GitHub 发布源不能指向上游 %s", UpstreamGithubRepo)
	}
	return repo, nil
}

func isDevelopmentBuildVersion(version string) bool {
	return strings.EqualFold(strings.TrimSpace(version), developmentBuildVersion)
}

// GetForkInfo exposes release/source links for the About page without ever
// falling back to the upstream repository.
func (a *App) GetForkInfo() ForkInfo {
	info := ForkInfo{
		Name:         "LytVPK Community Fork",
		AppVersion:   AppVersion,
		UpstreamRepo: UpstreamGithubRepo,
	}
	repo, err := configuredUpdateRepo()
	if err != nil {
		info.Error = err.Error()
		return info
	}

	info.Configured = true
	info.UpdateRepo = repo
	info.SourceURL = "https://github.com/" + repo
	info.IssuesURL = info.SourceURL + "/issues/new/choose"
	return info
}

// MirrorList 镜像源列表 (与前端保持一致)
var MirrorList = []string{
	"https://hk.gh-proxy.com/",
	"https://gh-proxy.com/",
	"https://gh.llkk.cc/",
	"https://ghfast.top/",
}

// MirrorWithLatency 带有延迟信息的镜像源
type MirrorWithLatency struct {
	URL     string `json:"url"`
	Latency int64  `json:"latency"` // 毫秒，-1 表示超时或错误
}

// checkLatency 检测镜像源延迟
func checkLatency(url string) int64 {
	start := time.Now()
	client := http.Client{
		Timeout: 3 * time.Second,
	}

	// 尝试 HEAD 请求
	resp, err := client.Head(url)
	if err != nil || resp.StatusCode >= 400 {
		// 如果 HEAD 失败，尝试 GET
		resp, err = client.Get(url)
	}

	if err != nil {
		return -1
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return -1
	}

	return time.Since(start).Milliseconds()
}

// fetchReleases 获取最近的版本列表
func fetchReleases(repo string) ([]GithubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=10", repo)
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "vpk-manager-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %s", resp.Status)
	}

	var releases []GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	return releases, nil
}

func selectWindowsAMD64ZipAsset(release GithubRelease) string {
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), "windows_amd64.zip") {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

// CheckUpdate 检查更新
func (a *App) CheckUpdate() UpdateInfo {
	pendingUpdateURL = ""
	if isDevelopmentBuildVersion(AppVersion) {
		return UpdateInfo{
			CurrentVer: AppVersion,
			Error:      "当前为开发构建，已跳过自动更新检查；请使用带正式版本号的发布构建。",
		}
	}

	repo, err := configuredUpdateRepo()
	if err != nil {
		return UpdateInfo{CurrentVer: AppVersion, Error: err.Error()}
	}

	// 1. 解析当前版本
	vCurrent, err := semver.ParseTolerant(AppVersion)
	if err != nil {
		return UpdateInfo{CurrentVer: AppVersion, Error: "当前版本号格式错误: " + err.Error()}
	}

	// 2. 仅从此 Fork 的 GitHub Releases API 读取版本与真实资产。
	// 不从上游或镜像页面猜测资产名，避免下载错误/不可验证的 ZIP。
	releases, err := fetchReleases(repo)
	if err != nil {
		return UpdateInfo{CurrentVer: AppVersion, Error: "检查 Fork 更新失败: " + err.Error()}
	}
	if len(releases) == 0 {
		return UpdateInfo{CurrentVer: AppVersion, Error: "检查 Fork 更新失败: 未找到 Release"}
	}

	var bestRel GithubRelease
	var maxVer semver.Version
	found := false
	for _, release := range releases {
		version, parseErr := semver.ParseTolerant(release.TagName)
		if parseErr != nil {
			continue
		}
		if !found || version.GT(maxVer) {
			maxVer = version
			bestRel = release
			found = true
		}
	}
	if !found {
		return UpdateInfo{CurrentVer: AppVersion, Error: "检查 Fork 更新失败: 没有有效的语义化版本标签"}
	}

	if maxVer.GT(vCurrent) {
		url := selectWindowsAMD64ZipAsset(bestRel)
		if url == "" {
			return UpdateInfo{
				CurrentVer: AppVersion,
				LatestVer:  maxVer.String(),
				Error:      "最新 Fork Release 未提供 windows_amd64.zip 安装包",
			}
		}

		var notes strings.Builder
		for _, release := range releases {
			version, parseErr := semver.ParseTolerant(release.TagName)
			if parseErr == nil && version.GT(vCurrent) {
				notes.WriteString(fmt.Sprintf("【%s】\n%s\n\n", release.TagName, release.Body))
			}
		}

		pendingUpdateURL = url
		return UpdateInfo{
			HasUpdate:   true,
			LatestVer:   maxVer.String(),
			CurrentVer:  AppVersion,
			ReleaseNote: notes.String(),
			DownloadURL: url,
		}
	}

	return UpdateInfo{
		HasUpdate:  false,
		CurrentVer: AppVersion,
		LatestVer:  maxVer.String(),
	}
}

// GetMirrors 获取镜像列表
func (a *App) GetMirrors() []string {
	return MirrorList
}

// GetMirrorsInitial 获取镜像列表初始状态 (不含延迟)
func (a *App) GetMirrorsInitial() []MirrorWithLatency {
	var results []MirrorWithLatency

	// 1. 添加直连
	results = append(results, MirrorWithLatency{URL: "", Latency: 0}) // 0 表示未检测/检测中

	// 2. 添加镜像源
	for _, m := range MirrorList {
		results = append(results, MirrorWithLatency{URL: m, Latency: 0})
	}

	return results
}

// TestMirrorsLatency 异步测试镜像源延迟，通过事件返回结果
func (a *App) TestMirrorsLatency() {
	// 1. 直连检测
	go func() {
		target := pendingUpdateURL
		if target == "" {
			target = "https://github.com"
		}
		latency := checkLatency(target)
		wailsRuntime.EventsEmit(a.ctx, "mirror_latency_result", MirrorWithLatency{URL: "", Latency: latency})
	}()

	// 2. 镜像源检测
	for _, mirror := range MirrorList {
		go func(m string) {
			target := m
			if pendingUpdateURL != "" {
				prefix := m
				if !strings.HasSuffix(prefix, "/") {
					prefix += "/"
				}
				target = prefix + pendingUpdateURL
			}
			latency := checkLatency(target)
			wailsRuntime.EventsEmit(a.ctx, "mirror_latency_result", MirrorWithLatency{URL: m, Latency: latency})
		}(mirror)
	}
}

// GetMirrorsWithLatency 获取带有延迟信息的镜像列表 (保留向后兼容，但建议使用 TestMirrorsLatency)
func (a *App) GetMirrorsWithLatency() []MirrorWithLatency {
	var results []MirrorWithLatency
	var wg sync.WaitGroup
	var mu sync.Mutex

	// 1. 添加直连检测
	wg.Add(1)
	go func() {
		defer wg.Done()
		target := pendingUpdateURL
		if target == "" {
			// 如果没有待更新的 URL，检测 GitHub 主站作为参考
			target = "https://github.com"
		}
		latency := checkLatency(target)
		mu.Lock()
		// 直连的 URL 为空字符串，与 DoUpdate 逻辑保持一致
		results = append(results, MirrorWithLatency{URL: "", Latency: latency})
		mu.Unlock()
	}()

	// 2. 检测镜像源
	for _, mirror := range MirrorList {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			target := m
			// 如果有具体的下载地址，拼接检测更准确
			if pendingUpdateURL != "" {
				prefix := m
				if !strings.HasSuffix(prefix, "/") {
					prefix += "/"
				}
				target = prefix + pendingUpdateURL
			}

			latency := checkLatency(target)
			mu.Lock()
			results = append(results, MirrorWithLatency{URL: m, Latency: latency})
			mu.Unlock()
		}(mirror)
	}
	wg.Wait()

	// 排序：延迟低的在前，失败的(-1)在后
	sort.Slice(results, func(i, j int) bool {
		if results[i].Latency == -1 {
			return false
		}
		if results[j].Latency == -1 {
			return true
		}
		return results[i].Latency < results[j].Latency
	})

	return results
}

// DoUpdate 执行更新
func (a *App) DoUpdate(mirror string) string {
	if _, err := configuredUpdateRepo(); err != nil {
		return err.Error()
	}

	downloadURL := pendingUpdateURL

	// 如果缓存为空，尝试重新获取
	if downloadURL == "" {
		// 复用 CheckUpdate 的逻辑 (这里简化处理，直接调用 CheckUpdate)
		info := a.CheckUpdate()
		if info.Error != "" {
			return "更新检测失败: " + info.Error
		}
		if !info.HasUpdate {
			return "当前已是最新版本"
		}
		downloadURL = info.DownloadURL
	}

	if downloadURL == "" {
		return "未找到适合当前系统的更新包"
	}

	// 获取当前执行文件路径
	exe, err := os.Executable()
	if err != nil {
		return "无法获取程序路径"
	}

	// 构造最终下载地址
	targetURL := downloadURL
	if mirror != "" {
		if !strings.HasSuffix(mirror, "/") {
			mirror += "/"
		}
		targetURL = mirror + downloadURL
	}

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "update-*.zip")
	if err != nil {
		return "创建临时文件失败: " + err.Error()
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// 下载带进度
	if err := a.downloadWithProgress(targetURL, tmpFile.Name()); err != nil {
		return "下载失败: " + err.Error()
	}

	// 安装更新
	if err := installUpdate(tmpFile.Name(), exe); err != nil {
		return "安装更新失败: " + err.Error()
	}

	return "success"
}

// downloadWithProgress 下载文件并发送进度
func (a *App) downloadWithProgress(url string, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	total := resp.ContentLength
	counter := &WriteCounter{
		Total: uint64(total),
		Ctx:   a.ctx,
	}

	if _, err = io.Copy(out, io.TeeReader(resp.Body, counter)); err != nil {
		return err
	}
	return nil
}

type WriteCounter struct {
	Total   uint64
	Current uint64
	Ctx     context.Context
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Current += uint64(n)
	if wc.Total > 0 {
		percent := float64(wc.Current) / float64(wc.Total) * 100
		wailsRuntime.EventsEmit(wc.Ctx, "update_progress", int(percent))
	}
	return n, nil
}

func selectUpdateExecutable(files []*zip.File) (*zip.File, error) {
	for _, file := range files {
		if file.FileInfo().IsDir() {
			continue
		}
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if strings.EqualFold(path.Base(name), CommunityForkExecutableName) {
			return file, nil
		}
	}
	return nil, fmt.Errorf("zip 中未找到统一更新程序 %s", CommunityForkExecutableName)
}

// installUpdate 解压 zip 中的统一程序并替换当前 exe。
// 保留 currentExe 的文件名，因此早期版本仍能更新到新版本；全新解压安装
// 则统一使用 CommunityForkExecutableName。
func installUpdate(zipPath, currentExe string) error {
	// 1. 解压 zip
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	file, err := selectUpdateExecutable(r.File)
	if err != nil {
		return err
	}

	// 2. 解压出新文件
	newExePath := currentExe + ".new"
	outFile, err := os.Create(newExePath)
	if err != nil {
		return err
	}

	rc, err := file.Open()
	if err != nil {
		outFile.Close()
		return err
	}

	_, err = io.Copy(outFile, rc)
	rc.Close()
	outFile.Close()
	if err != nil {
		return err
	}

	// 3. 替换逻辑 (Windows)
	oldExePath := currentExe + ".old"

	// 如果存在旧的 .old，先删除
	os.Remove(oldExePath)

	// 重命名当前 exe -> .old
	if err := os.Rename(currentExe, oldExePath); err != nil {
		return fmt.Errorf("备份旧文件失败: %w", err)
	}

	// 重命名新 exe -> 当前 exe
	if err := os.Rename(newExePath, currentExe); err != nil {
		// 尝试回滚
		os.Rename(oldExePath, currentExe)
		return fmt.Errorf("替换文件失败: %w", err)
	}

	return nil
}
