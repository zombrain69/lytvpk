package app

import (
	"regexp"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// workshopItemLinkBase 是工坊物品链接的规范形式（与前端 checkWorkshopUrl 认识的写法一致）。
const workshopItemLinkBase = "https://steamcommunity.com/sharedfiles/filedetails/?id="

var (
	// 对齐 FireAxe 的 PublishedFileUtils.s_publishedFileIdLinkRegex（sharedfiles / workshop 两条路径都认），
	// 但放宽一点：id 不必是第一个查询参数，链接前后可以夹着别的文字（用户复制的往往是一整句话）。
	workshopClipboardLinkPattern = regexp.MustCompile(`(?i)steamcommunity\.com/(?:sharedfiles|workshop)/filedetails/?\?[^\s]*?\bid=(\d+)`)
	// 纯数字 ID 直接当作作品 ID（FireAxe 的 TryParsePublishedFileId 同样先试纯数字）。
	workshopClipboardIDPattern = regexp.MustCompile(`^\d+$`)
)

// parseWorkshopClipboardLink 从任意文本里提取工坊物品 ID，返回规范链接。
// 认不出返回 ("", false)。纯函数，便于单测（真正读剪贴板的入口是 CheckClipboardWorkshopLink）。
func parseWorkshopClipboardLink(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	if workshopClipboardIDPattern.MatchString(trimmed) {
		return workshopItemLinkBase + trimmed, true
	}
	match := workshopClipboardLinkPattern.FindStringSubmatch(trimmed)
	if len(match) < 2 {
		return "", false
	}
	return workshopItemLinkBase + match[1], true
}

// CheckClipboardWorkshopLink 读取系统剪贴板；如果里面是创意工坊物品链接（或纯作品 ID），
// 返回规范化链接，否则返回空字符串。
//
// 对应 FireAxe 的 MainWindowViewModel.CheckClipboard（AppSettings.IsAutoDetectWorkshopItemLinkInClipboard）：
// 上游是 0.5s 轮询 + 弹窗确认；本项目由前端按"开关 + 窗口聚焦 + 间隔"节流调用本方法，
// 判定与去重逻辑仍在前端（core/workshop-clipboard.mjs），后端只负责"读 + 解析"。
func (a *App) CheckClipboardWorkshopLink() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return "", err
	}
	link, ok := parseWorkshopClipboardLink(text)
	if !ok {
		return "", nil
	}
	return link, nil
}
