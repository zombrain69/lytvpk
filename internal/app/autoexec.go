package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type autoexecEncoding uint8

const (
	autoexecEncodingUTF8 autoexecEncoding = iota
	autoexecEncodingGBK
	autoexecEncodingUTF16LE
	autoexecEncodingUTF16BE
)

type autoexecDocument struct {
	path       string
	content    string
	encoding   autoexecEncoding
	hasUTF8BOM bool
	lineEnding string
}

// AutoexecConfig is the editable game cfg/autoexec.cfg document and its
// on-disk metadata. Content is always UTF-8 in the Wails boundary.
type AutoexecConfig struct {
	Path         string `json:"path"`
	Exists       bool   `json:"exists"`
	Content      string `json:"content"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
	Encoding     string `json:"encoding"`
	LineEnding   string `json:"lineEnding"`
}

type AutoexecCommandHelp struct {
	Command string `json:"command"`
	Summary string `json:"summary"`
	Scope   string `json:"scope"`
	Risk    string `json:"risk"`
	Source  string `json:"source"`
}

type AutoexecCommandMatch struct {
	Line    int                  `json:"line"`
	Raw     string               `json:"raw"`
	Command string               `json:"command"`
	Known   bool                 `json:"known"`
	Help    *AutoexecCommandHelp `json:"help,omitempty"`
}

var autoexecCommandToken = regexp.MustCompile(`^\s*([A-Za-z0-9_+.-]+)`)

// The built-in entries are intentionally concise and conservative. Plugin
// commands are marked as such so a user can distinguish them from Source
// engine commands before saving a config.
var autoexecCommandCatalog = []AutoexecCommandHelp{
	{Command: "bind", Summary: "将按键绑定到一个或多个控制台命令", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "alias", Summary: "创建一个可复用的命令别名", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "exec", Summary: "执行 cfg 目录中的另一个配置文件", Scope: "客户端", Risk: "中：会继续执行被引用文件", Source: "L4D2 Console commands"},
	{Command: "fps_max", Summary: "限制客户端最大帧率；0 表示不限制", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "rate", Summary: "设置客户端网络带宽速率上限", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "cl_cmdrate", Summary: "设置客户端向服务器发送数据的频率", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "cl_updaterate", Summary: "设置客户端接收服务器更新的频率", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "cl_interp", Summary: "设置客户端插值时间（通常配合 cl_interp_ratio）", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "cl_interp_ratio", Summary: "设置客户端插值比例", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "net_graph", Summary: "显示网络、帧率和性能统计信息", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "mat_monitorgamma", Summary: "调整游戏画面伽马/亮度", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "volume", Summary: "设置游戏音量", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "sensitivity", Summary: "设置鼠标灵敏度", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "thirdperson", Summary: "切换第三人称视角（部分场景或服务器可能限制）", Scope: "客户端/服务器", Risk: "中：可能需要 sv_cheats", Source: "L4D2 Console commands"},
	{Command: "noclip", Summary: "切换穿墙飞行模式", Scope: "客户端/服务器", Risk: "高：通常需要 sv_cheats", Source: "L4D2 Wiki"},
	{Command: "sv_cheats", Summary: "允许服务器上的作弊命令", Scope: "服务器", Risk: "高：影响联机规则", Source: "L4D2 Wiki"},
	{Command: "director_stop", Summary: "暂停 Director 的感染者/事件生成", Scope: "服务器", Risk: "高：改变当前战局", Source: "L4D2 Wiki"},
	{Command: "z_spawn", Summary: "生成指定感染者或物品", Scope: "服务器", Risk: "高：通常需要 sv_cheats", Source: "L4D2 Wiki"},
	{Command: "z_kill", Summary: "杀死准星指向的感染者", Scope: "服务器", Risk: "高：改变当前战局", Source: "L4D2 Wiki"},
	{Command: "showscores", Summary: "显示或隐藏计分板", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "thirdpersonshoulder", Summary: "切换肩后第三人称视角", Scope: "客户端", Risk: "中：可能受服务器设置限制", Source: "L4D2 Console commands"},
	{Command: "go_away_from_keyboard", Summary: "将玩家切换为闲置状态", Scope: "客户端/服务器", Risk: "中：可能转交控制权", Source: "L4D2 Console commands"},
	{Command: "mat_setvideomode", Summary: "设置分辨率、窗口模式并重新加载视频设置", Scope: "客户端", Risk: "中：可能触发界面重载", Source: "Source 引擎"},
	{Command: "wait", Summary: "等待指定帧数后继续执行命令串", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "echo", Summary: "在控制台输出一行文字", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "show_menu", Summary: "显示指定的 VGUI 菜单", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "sb_all_bot_game", Summary: "允许在没有真人玩家时继续进行战役", Scope: "服务器", Risk: "中：改变战局规则", Source: "L4D2 Console commands"},
	{Command: "sv_consistency", Summary: "控制服务器对客户端文件的一致性检查", Scope: "服务器", Risk: "中：影响 Mod 校验", Source: "L4D2 Console commands"},
	{Command: "sv_pausable", Summary: "控制当前服务器是否允许暂停", Scope: "服务器", Risk: "中：影响联机暂停", Source: "L4D2 Console commands"},
	{Command: "pause", Summary: "暂停或继续当前游戏（需服务器允许）", Scope: "服务器", Risk: "中：影响当前战局", Source: "L4D2 Console commands"},
	{Command: "kick", Summary: "将指定玩家从当前服务器踢出", Scope: "服务器", Risk: "高：影响当前联机玩家", Source: "L4D2 Console commands"},
	{Command: "cl_crosshair_alpha", Summary: "设置准星透明度", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "cl_crosshair_dynamic", Summary: "控制准星是否根据移动/射击动态变化", Scope: "客户端", Risk: "低", Source: "Source 引擎"},
	{Command: "c_maxyaw", Summary: "设置第三人称视角水平旋转上限", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "c_minyaw", Summary: "设置第三人称视角水平旋转下限", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "c_mindistance", Summary: "设置第三人称摄像机最小距离", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "cam_collision", Summary: "控制第三人称摄像机碰撞检测", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "cam_idealdelta", Summary: "设置第三人称摄像机跟随速度", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "cam_ideallag", Summary: "设置第三人称摄像机跟随延迟", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "cam_idealpitch", Summary: "设置第三人称摄像机俯仰角", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "cam_idealyaw", Summary: "设置第三人称摄像机水平角", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "cam_snapto", Summary: "控制第三人称摄像机是否立即对齐", Scope: "客户端", Risk: "低", Source: "L4D2 Console commands"},
	{Command: "necola_menu", Summary: "打开 Neko/L4D 插件菜单（若已安装）", Scope: "插件/Mod", Risk: "低", Source: "本机 autoexec.cfg"},
	{Command: "l4n_game_usage", Summary: "显示可用内存、datacache 和实体峰值等稳定性数据", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_open_shader_cache_dir", Summary: "打开着色器缓存目录，便于清理重编译缓存", Scope: "L4N 插件", Risk: "中：打开目录后请先退出游戏再清理", Source: "readme_l4n.txt"},
	{Command: "l4n_revert_cvar", Summary: "将指定 convar 恢复为默认值", Scope: "L4N 插件", Risk: "中：会改变指定设置", Source: "readme_l4n.txt"},
	{Command: "l4n_cvar", Summary: "读取或设置普通控制台无法直接访问的 convar", Scope: "L4N 插件", Risk: "中：可能修改隐藏设置", Source: "readme_l4n.txt"},
	{Command: "l4n_menu", Summary: "显示 L4N 主菜单", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_inspect_mode", Summary: "进入手模展示/调整模式，可用数字键和滚轮调整参数", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_addon_fix", Summary: "强制手模匹配玩家模型，修复一代图使用二代角色时的不匹配", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_patch_team_player_display", Summary: "接管 HUD 队友头像、倒地图标和 Bot 名字显示", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4nsurvivor", Summary: "启用 L4N 扩展幸存者模型功能；值 2 可不替换队友模型", Scope: "L4N 插件", Risk: "中：与传统角色 Mod 可能冲突", Source: "readme_l4n.txt"},
	{Command: "l4n_allow_lobby_cheats", Summary: "控制大厅中作弊相关行为；启用后仍需自行建立 listen server", Scope: "L4N 插件", Risk: "高：可能影响联机规则", Source: "readme_l4n.txt"},
	{Command: "l4n_allow_consistency_check", Summary: "控制 L4N 是否允许一致性检查", Scope: "L4N 插件", Risk: "中：可能影响 Mod 校验", Source: "readme_l4n.txt"},
	{Command: "l4n_commoninfected_noragdoll", Summary: "禁用普通感染者死亡时的布娃娃效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_survivor_scale", Summary: "全局修改幸存者实体大小", Scope: "L4N 插件", Risk: "中：可能影响碰撞和表现", Source: "readme_l4n.txt"},
	{Command: "l4n_force_skyname", Summary: "覆盖天空盒材质；传入空字符串可还原", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_mat_specular", Summary: "控制环境反射效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_flashlight_factor", Summary: "设置手电筒亮度倍率", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_dlight_muzzleflash", Summary: "控制第一人称枪火动态光源", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_server_filter", Summary: "启用 L4N 服务器过滤", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_buildcubemaps", Summary: "配置环境并编译 cubemaps；可附加 allow_specular", Scope: "L4N 插件", Risk: "高：修改地图/渲染缓存", Source: "readme_l4n.txt"},
	{Command: "l4n_fast_record", Summary: "开始录制并自动命名 demo", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_clear_datacache", Summary: "清理引擎 datacache", Scope: "L4N 插件", Risk: "中：可能造成短暂加载", Source: "readme_l4n.txt"},
	{Command: "l4n_reload_config", Summary: "重新加载 L4N 配置", Scope: "L4N 插件", Risk: "中：会应用配置文件中的设置", Source: "readme_l4n.txt"},
}

var autoexecCommandByName = func() map[string]AutoexecCommandHelp {
	result := make(map[string]AutoexecCommandHelp, len(autoexecCommandCatalog))
	for _, item := range autoexecCommandCatalog {
		result[strings.ToLower(item.Command)] = item
	}
	return result
}()

func autoexecPathForRoot(rootDir string) (string, error) {
	rootDir = filepath.Clean(strings.TrimSpace(rootDir))
	if rootDir == "" || rootDir == "." {
		return "", fmt.Errorf("未选择L4D2目录")
	}
	if strings.EqualFold(filepath.Base(rootDir), "addons") && strings.EqualFold(filepath.Base(filepath.Dir(rootDir)), "left4dead2") {
		rootDir = filepath.Dir(rootDir)
	}
	return filepath.Join(rootDir, "cfg", "autoexec.cfg"), nil
}

func (a *App) autoexecPath() (string, error) {
	return autoexecPathForRoot(a.rootDirectorySnapshot())
}

func autoexecEncodingName(doc autoexecDocument) string {
	switch doc.encoding {
	case autoexecEncodingGBK:
		return "GBK/ANSI"
	case autoexecEncodingUTF16LE:
		return "UTF-16 LE"
	case autoexecEncodingUTF16BE:
		return "UTF-16 BE"
	default:
		if doc.hasUTF8BOM {
			return "UTF-8 BOM"
		}
		return "UTF-8"
	}
}

func detectLineEnding(content string) string {
	if strings.Contains(content, "\r\n") {
		return "CRLF"
	}
	if strings.Contains(content, "\r") {
		return "CR"
	}
	return "LF"
}

func readAutoexecDocumentAtPath(path string) (autoexecDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return autoexecDocument{path: path, encoding: autoexecEncodingUTF8, lineEnding: "CRLF"}, nil
		}
		return autoexecDocument{path: path}, fmt.Errorf("无法读取 autoexec.cfg: %w", err)
	}
	doc := autoexecDocument{path: path, encoding: autoexecEncodingUTF8, lineEnding: "CRLF"}
	if bytes.HasPrefix(raw, []byte{0xFF, 0xFE}) {
		decoded, _, decodeErr := transform.Bytes(unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder(), raw)
		if decodeErr != nil {
			return autoexecDocument{}, fmt.Errorf("无法按 UTF-16 LE 解码 autoexec.cfg: %w", decodeErr)
		}
		doc.content, doc.encoding = string(decoded), autoexecEncodingUTF16LE
	} else if bytes.HasPrefix(raw, []byte{0xFE, 0xFF}) {
		decoded, _, decodeErr := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder(), raw)
		if decodeErr != nil {
			return autoexecDocument{}, fmt.Errorf("无法按 UTF-16 BE 解码 autoexec.cfg: %w", decodeErr)
		}
		doc.content, doc.encoding = string(decoded), autoexecEncodingUTF16BE
	} else if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		payload := raw[3:]
		if !utf8.Valid(payload) {
			return autoexecDocument{}, fmt.Errorf("UTF-8 BOM 后的 autoexec.cfg 不是有效 UTF-8")
		}
		doc.content, doc.hasUTF8BOM = string(payload), true
	} else if utf8.Valid(raw) {
		doc.content = string(raw)
	} else {
		decoded, _, decodeErr := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), raw)
		if decodeErr != nil {
			return autoexecDocument{}, fmt.Errorf("无法按 GBK/ANSI 解码 autoexec.cfg: %w", decodeErr)
		}
		doc.content, doc.encoding = string(decoded), autoexecEncodingGBK
	}
	doc.lineEnding = detectLineEnding(doc.content)
	return doc, nil
}

func normalizeAutoexecLineEndings(content, lineEnding string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if lineEnding == "CRLF" {
		return strings.ReplaceAll(content, "\n", "\r\n")
	}
	if lineEnding == "CR" {
		return strings.ReplaceAll(content, "\n", "\r")
	}
	return content
}

func encodeAutoexecDocument(doc autoexecDocument, content string) ([]byte, error) {
	content = normalizeAutoexecLineEndings(content, doc.lineEnding)
	switch doc.encoding {
	case autoexecEncodingGBK:
		encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(content))
		if err != nil {
			return nil, fmt.Errorf("autoexec.cfg 内容无法按 GBK/ANSI 保存（可能包含 GBK 不支持的字符）: %w", err)
		}
		return encoded, nil
	case autoexecEncodingUTF16LE, autoexecEncodingUTF16BE:
		endian := unicode.LittleEndian
		bom := []byte{0xFF, 0xFE}
		if doc.encoding == autoexecEncodingUTF16BE {
			endian, bom = unicode.BigEndian, []byte{0xFE, 0xFF}
		}
		encoded, _, err := transform.Bytes(unicode.UTF16(endian, unicode.IgnoreBOM).NewEncoder(), []byte(content))
		if err != nil {
			return nil, fmt.Errorf("无法按 UTF-16 编码 autoexec.cfg: %w", err)
		}
		return append(bom, encoded...), nil
	default:
		encoded := []byte(content)
		if doc.hasUTF8BOM {
			encoded = append([]byte{0xEF, 0xBB, 0xBF}, encoded...)
		}
		return encoded, nil
	}
}

func backupAutoexecDocument(doc autoexecDocument) error {
	original, err := os.ReadFile(doc.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法读取 autoexec.cfg 以创建备份: %w", err)
	}
	backupPath := doc.path + ".lytvpk.bak"
	backupFile, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法创建 autoexec.cfg 备份: %w", err)
	}
	if _, err := backupFile.Write(original); err != nil {
		_ = backupFile.Close()
		_ = os.Remove(backupPath)
		return fmt.Errorf("无法写入 autoexec.cfg 备份: %w", err)
	}
	if err := backupFile.Close(); err != nil {
		return fmt.Errorf("无法关闭 autoexec.cfg 备份: %w", err)
	}
	return nil
}

func writeAutoexecDocument(doc autoexecDocument, content string) error {
	encoded, err := encodeAutoexecDocument(doc, content)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(doc.path), 0755); err != nil {
		return fmt.Errorf("无法创建 cfg 目录: %w", err)
	}
	if err := backupAutoexecDocument(doc); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(doc.path), ".lytvpk-autoexec-*")
	if err != nil {
		return fmt.Errorf("无法创建 autoexec.cfg 临时文件: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(encoded); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("无法写入 autoexec.cfg 临时文件: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("无法关闭 autoexec.cfg 临时文件: %w", err)
	}
	if err := os.Rename(tempPath, doc.path); err != nil {
		return fmt.Errorf("无法替换 autoexec.cfg: %w", err)
	}
	return nil
}

// GetAutoexecConfig reads the game config bound to the currently selected
// addons directory. Missing cfg/autoexec.cfg is represented as an empty file.
func (a *App) GetAutoexecConfig() (AutoexecConfig, error) {
	path, err := a.autoexecPath()
	if err != nil {
		return AutoexecConfig{}, err
	}
	doc, err := readAutoexecDocumentAtPath(path)
	if err != nil {
		return AutoexecConfig{}, err
	}
	result := AutoexecConfig{Path: path, Content: doc.content, Encoding: autoexecEncodingName(doc), LineEnding: doc.lineEnding}
	info, statErr := os.Stat(path)
	if os.IsNotExist(statErr) {
		return result, nil
	}
	if statErr != nil {
		return AutoexecConfig{}, fmt.Errorf("无法读取 autoexec.cfg 状态: %w", statErr)
	}
	result.Exists, result.Size = true, info.Size()
	result.LastModified = info.ModTime().Format(time.RFC3339)
	return result, nil
}

// SaveAutoexecConfig writes UTF-8 input while preserving the existing file's
// encoding, BOM and newline convention. It never executes console commands.
func (a *App) SaveAutoexecConfig(content string) error {
	path, err := a.autoexecPath()
	if err != nil {
		return err
	}
	doc, err := readAutoexecDocumentAtPath(path)
	if err != nil {
		return err
	}
	return writeAutoexecDocument(doc, content)
}

// GetAutoexecCommandHelp returns built-in and L4N plugin command descriptions.
// Query is matched against command names and Chinese summaries.
func (a *App) GetAutoexecCommandHelp(query string) []AutoexecCommandHelp {
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]AutoexecCommandHelp, 0, len(autoexecCommandCatalog))
	for _, item := range autoexecCommandCatalog {
		if query == "" || strings.Contains(strings.ToLower(item.Command), query) || strings.Contains(strings.ToLower(item.Summary), query) {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Command < result[j].Command })
	return result
}

// AnalyzeAutoexecCommands identifies the first command token on each cfg line.
// Comments and blank lines are ignored; unknown commands remain visible so
// plugin/Mod-specific commands are not silently dropped.
func (a *App) AnalyzeAutoexecCommands(content string) []AutoexecCommandMatch {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	result := make([]AutoexecCommandMatch, 0)
	for lineNumber, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if match := autoexecCommandToken.FindStringSubmatch(trimmed); len(match) == 2 {
			command := match[1]
			key := strings.ToLower(command)
			if help, ok := autoexecCommandByName[key]; ok {
				helpCopy := help
				result = append(result, AutoexecCommandMatch{Line: lineNumber + 1, Raw: raw, Command: command, Known: true, Help: &helpCopy})
			} else {
				result = append(result, AutoexecCommandMatch{Line: lineNumber + 1, Raw: raw, Command: command})
			}
		}
	}
	return result
}
