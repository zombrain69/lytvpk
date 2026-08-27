package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type autoexecEncoding uint8

const (
	autoexecEncodingUTF8 autoexecEncoding = iota
	autoexecEncodingGBK
	autoexecEncodingWindows1252
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
var autoexecAliasDefinition = regexp.MustCompile(`^\s*alias\s+"?([A-Za-z0-9_+.-]+)"?`)

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
	{Command: "l4n_allow_hud_team_player_display", Summary: "允许 HUD 显示队友状态", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_allow_draw_sprite", Summary: "允许渲染 Sprite；关闭后可能影响关卡机关提示", Scope: "L4N 插件", Risk: "中：可能影响关卡提示", Source: "readme_l4n.txt"},
	{Command: "l4nsurvivor", Summary: "启用 L4N 扩展幸存者模型功能；值 2 可不替换队友模型", Scope: "L4N 插件", Risk: "中：与传统角色 Mod 可能冲突", Source: "readme_l4n.txt"},
	{Command: "l4n_survivor", Summary: "L4N 扩展幸存者模型功能的兼容开关", Scope: "L4N 插件", Risk: "中：与传统角色 Mod 可能冲突", Source: "readme_l4n.txt"},
	{Command: "l4n_allow_lobby_cheats", Summary: "控制大厅中作弊相关行为；启用后仍需自行建立 listen server", Scope: "L4N 插件", Risk: "高：可能影响联机规则", Source: "readme_l4n.txt"},
	{Command: "l4n_allow_consistency_check", Summary: "控制 L4N 是否允许一致性检查", Scope: "L4N 插件", Risk: "中：可能影响 Mod 校验", Source: "readme_l4n.txt"},
	{Command: "l4n_commoninfected_noragdoll", Summary: "禁用普通感染者死亡时的布娃娃效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_survivor_scale", Summary: "全局修改幸存者实体大小", Scope: "L4N 插件", Risk: "中：可能影响碰撞和表现", Source: "readme_l4n.txt"},
	{Command: "l4n_force_skyname", Summary: "覆盖天空盒材质；传入空字符串可还原", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_mat_specular", Summary: "控制环境反射效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_flashlight_factor", Summary: "设置手电筒亮度倍率", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_flashlight_r", Summary: "设置手电筒颜色 R 通道倍率", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_flashlight_g", Summary: "设置手电筒颜色 G 通道倍率", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_flashlight_b", Summary: "设置手电筒颜色 B 通道倍率", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_dlight_muzzleflash", Summary: "控制第一人称枪火动态光源", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_server_filter", Summary: "启用 L4N 服务器过滤", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_buildcubemaps", Summary: "配置环境并编译 cubemaps；可附加 allow_specular", Scope: "L4N 插件", Risk: "高：修改地图/渲染缓存", Source: "readme_l4n.txt"},
	{Command: "l4n_fast_record", Summary: "开始录制并自动命名 demo", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_clear_datacache", Summary: "清理引擎 datacache", Scope: "L4N 插件", Risk: "中：可能造成短暂加载", Source: "readme_l4n.txt"},
	{Command: "l4n_reload_config", Summary: "重新加载 L4N 配置", Scope: "L4N 插件", Risk: "中：会应用配置文件中的设置", Source: "readme_l4n.txt"},
	{Command: "l4n_allow_flashlightmuzzleflash", Summary: "允许第一人称手电筒火光", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_game_hud_visible", Summary: "控制 L4N 游戏 HUD 的显示与隐藏", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_force_dummy_addoninfo", Summary: "强制使用虚拟 addoninfo，绕过部分 Mod 检查限制", Scope: "L4N 插件", Risk: "中：会改变 Mod 检查行为", Source: "readme_l4n.txt"},
	{Command: "l4n_max_background_bik", Summary: "设置大厅背景视频数量，减少重复视频占用或扩展随机范围", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_specialinfected_randommodel", Summary: "控制特殊感染者随机使用一代或二代模型", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4nsurvivor_allocation_algorithm", Summary: "设置为队友分配 l4nsurvivor 模型的算法（userid/SteamID/随机数）", Scope: "L4N 插件", Risk: "中：影响联机模型分配", Source: "readme_l4n.txt"},
	{Command: "l4nsurvivor_allow_bot", Summary: "允许 Bot 使用 l4nsurvivor 模型", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_sway", Summary: "开启或关闭第一人称手模随视角转动的滞后效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_sway_interp", Summary: "设置手模 sway 效果的恢复时间；0 可禁用滞后", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_sway_scale", Summary: "设置手模 sway 的幅度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_sway_ignore_helpinghand", Summary: "手模伸手动画期间是否禁用 sway", Scope: "L4N 插件", Risk: "中：可能影响 ADS 等动画", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_offset_x", Summary: "全局调整第一人称手模 X 轴位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_offset_y", Summary: "全局调整第一人称手模 Y 轴位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_offset_z", Summary: "全局调整第一人称手模 Z 轴位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_allow_camera_animation", Summary: "允许手模动画驱动摄像机", Scope: "L4N 插件", Risk: "中：会改变视角动画", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_pin", Summary: "固定 viewmodel 实体", Scope: "L4N 插件", Risk: "中：可能影响手模更新", Source: "readme_l4n.txt"},
	{Command: "l4n_menu_offset_x", Summary: "调整 l4n_menu 的水平位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_menu_offset_y", Summary: "调整 l4n_menu 的垂直位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_menu_font_size", Summary: "设置 l4n_menu 字体大小", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_mat_colorcorrection", Summary: "控制色彩校正/颜色滤镜", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_tonemap_scale", Summary: "调整 HDR 曝光强度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_survivor_stylized_amibentlight", Summary: "控制幸存者实体的风格化环境光", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_survivor_lighting_scale", Summary: "设置幸存者实体光照倍率", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_game_usage_pos", Summary: "调整 l4n_game_usage HUD 的位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_game_usage_padding", Summary: "调整 l4n_game_usage HUD 与屏幕边缘的距离", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_survivor_sequence_strip", Summary: "修复幸存者 Mod 的动画数量；修改后需重启游戏", Scope: "L4N 插件", Risk: "中：需要重启游戏", Source: "readme_l4n.txt"},
	{Command: "l4n_prevent_varms_stretching", Summary: "禁止第一人称手模动画部分骨骼的拉伸", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_player_list_show_steam_avatar", Summary: "让 +l4n_player_list 显示 Steam 玩家头像", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_player_list_vomitjar_icon_clip", Summary: "裁剪 +l4n_player_list 中的胆汁图标", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_engine_post_allow_local_contrast", Summary: "控制局部对比度后处理效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_engine_post_allow_vomit", Summary: "控制胆汁造成的模糊视线效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_enhanced_material_pxory", Summary: "增强部分材质代理并同步第一/第三人称效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_proxy_entity_random_seed_offset", Summary: "调整 EntityRandom 材质代理的随机种子偏移", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_disable_survivor_bandage", Summary: "禁用幸存者模型的绷带粒子，避免与模型缩放冲突", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_auto_flush_unused_models", Summary: "自动清理无用模型缓存；可设为 0、1 或 2", Scope: "L4N 插件", Risk: "中：影响缓存和加载", Source: "readme_l4n.txt"},
	{Command: "l4n_player_identity_render_color", Summary: "控制玩家相关实体染色", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_env_cubemap_redirect", Summary: "将 env_cubemap 加载重定向到对应的 HDR/PWL 资源", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_use_nekosky", Summary: "启用 L4N NekoSky 着色器渲染天空", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_proxy", Summary: "启用或禁用 L4N 材质代理", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_ambient_darkness_limit", Summary: "限制环境光的暗度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_lightmap_darkness_limit", Summary: "限制光照贴图的暗度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_charactor_model_random_scale", Summary: "启用角色模型随机缩放并设置缩放范围", Scope: "L4N 插件", Risk: "中：改变模型表现", Source: "readme_l4n.txt"},
	{Command: "l4n_to_nekotoon_allow_outline", Summary: "旧版 NekoToon 描边开关；L4N 2.42.0 起已移除，请改用 l4n_to_nekotoon_outline_type", Scope: "L4N 插件（旧版）", Risk: "高：当前版本可能无效", Source: "readme_l4n.txt"},
	{Command: "l4n_to_nekotoon_outline_type", Summary: "设置模型转换为 NekoToon 时的描边类型（L4N 2.42.0+）", Scope: "L4N 插件", Risk: "中：改变模型渲染", Source: "readme_l4n.txt"},
	{Command: "l4n_dlight_muzzleflash_brightness", Summary: "设置第一人称枪火动态光源亮度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_dlight_muzzleflash_distance", Summary: "设置第一人称枪火动态光源距离", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_dlight_muzzleflash_prevent_tonemapscale", Summary: "抑制曝光对枪火动态光源亮度的影响", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_thirdperson_fire_sound_fix", Summary: "修复第三人称开火声音", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_reshade_draw", Summary: "控制 ReShade 效果是否在 HUD 前渲染", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_scripted_hud_allow", Summary: "允许脚本化 HUD 渲染", Scope: "L4N 插件", Risk: "中：可能影响 HUD Mod", Source: "readme_l4n.txt"},
	{Command: "l4n_scripted_hud_allow_slot", Summary: "控制脚本化 HUD 指定 slot（1 至 15）的渲染", Scope: "L4N 插件", Risk: "中：可能影响 HUD Mod", Source: "readme_l4n.txt"},
	{Command: "l4n_allow_entity_dissolve", Summary: "允许实体消逝效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_decal_allow_staticprop", Summary: "允许静态道具贴花效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_decal_allow_entity", Summary: "允许实体贴花效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_decal_allow_special_infected", Summary: "允许特殊感染者贴花效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_decal_allow_survivor", Summary: "允许幸存者贴花效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_decal_allow_common_infected", Summary: "允许普通感染者贴花效果", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_hudmenu_offset_x", Summary: "调整 HUD 菜单水平位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_hudmenu_offset_y", Summary: "调整 HUD 菜单垂直位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_view_punch_scale", Summary: "调整受击视角晃动强度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_screen_shake_scale", Summary: "调整屏幕震动强度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_thirdpersion_crosshair_alpha", Summary: "设置精确第三人称准星透明度", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_thirdpersion_crosshair_scale", Summary: "设置精确第三人称准星缩放", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_thirdpersion_crosshair_dynamic", Summary: "控制精确第三人称准星动态缩放", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_hudscope_draw_override", Summary: "控制是否接管开镜 HUD 渲染", Scope: "L4N 插件", Risk: "中：关闭会影响相关 HUD 功能", Source: "readme_l4n.txt"},
	{Command: "l4n_hudscope_draw_padding_block", Summary: "控制开镜 HUD 黑边填充", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_hudscope_draw", Summary: "控制开镜 HUD 渲染", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_offset2", Summary: "按 x/dx/y/dy/z/dz 或 reset 调整当前手模位置", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_survivor_sequence_test", Summary: "检查场景中幸存者模型动画是否存在问题", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "+l4n_player_list", Summary: "显示包含更多信息的玩家列表面板", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "+l4n_lookat", Summary: "触发物品拾取或帮助倒地队友的伸手动画", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_placelight", Summary: "放置或移除动态环境光源；动态光源可能明显掉帧", Scope: "L4N 插件", Risk: "高：影响性能", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_selfillum", Summary: "调整当前手模的自发光强度，支持增量参数", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_mat_showtextures", Summary: "显示材质并支持字符串匹配过滤", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_vm_2pbr", Summary: "将当前手模材质转换为 PBR；参数 0 可还原", Scope: "L4N 插件", Risk: "中：改变当前材质", Source: "readme_l4n.txt"},
	{Command: "l4n_print_launch_options", Summary: "输出当前能被游戏识别的启动项", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_reset_player_render_color", Summary: "重置玩家实体渲染颜色", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_env_report", Summary: "输出游戏运行环境信息", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_flush_unused_models", Summary: "立即清理无用模型缓存", Scope: "L4N 插件", Risk: "中：可能造成短暂加载", Source: "readme_l4n.txt"},
	{Command: "l4n_random_player_render_color", Summary: "随机设置玩家实体渲染颜色", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_to_nekotoon", Summary: "将自己的模型或指定玩家/手模材质转换为 NekoToon", Scope: "L4N 插件", Risk: "中：改变当前材质", Source: "readme_l4n.txt"},
	{Command: "l4n_nekook_path_append", Summary: "添加高优先级资产搜索路径，优先于 VPK", Scope: "L4N 插件", Risk: "中：改变资源加载来源", Source: "readme_l4n.txt"},
	{Command: "l4n_nekook_path_remove", Summary: "移除高优先级资产搜索路径", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_refresh_entity_random", Summary: "刷新所有实体的随机数", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_is_proxy_exist", Summary: "检查指定材质代理是否存在", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_print_environment_variables", Summary: "输出游戏进程环境变量", Scope: "L4N 插件", Risk: "中：可能输出环境信息", Source: "readme_l4n.txt"},
	{Command: "l4n_reload_vgui_schemes", Summary: "重新加载 VGUI scheme", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4n_print_particles_manifest", Summary: "输出 particles_manifest 内容", Scope: "L4N 插件", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "l4nsurvivor_roll", Summary: "切换到下一个 l4nsurvivor 模型，可作用于自己或队友", Scope: "L4N 插件", Risk: "中：改变模型分配", Source: "readme_l4n.txt"},
	{Command: "l4n_custom_command_menu", Summary: "加载配置文件并显示自定义命令菜单", Scope: "L4N 插件", Risk: "中：会加载外部配置", Source: "readme_l4n.txt"},
	{Command: "l4n_reload_sequence_event_vdf", Summary: "重新加载手模序列事件 VDF 配置", Scope: "L4N 插件", Risk: "中：会应用外部配置", Source: "readme_l4n.txt"},
	{Command: "mat_tonemapping_occlusion_use_stencil", Summary: "使用 stencil 修复部分地图过暗问题", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekosky_overlay_lf", Summary: "设置 NekoSky 天空盒 left 面的叠加贴图", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_outline_thickness_scale", Summary: "设置 NekoToon 描边粗细倍率；0 可关闭", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_pbr_where", Summary: "高亮显示使用 PBR 着色器的材质", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekosky_overlay_", Summary: "设置 NekoSky 六个面的叠加纹理路径（rt/bk/lf/ft/up/dn）", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekosky_overlay_strength", Summary: "设置 NekoSky 叠加纹理强度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_neko_allow_invert_tonemap", Summary: "控制色调映射曲线对颜色的抑制；值 2 可配合 NekoBloom", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekorefract_color_invert_exponent", Summary: "设置 NekoRefract 颜色反转指数", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_allow_lightwarp", Summary: "控制 NekoToon 是否使用非平展光照渲染", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_lambert_factor", Summary: "设置 NekoToon 环境光或手电筒光照阴影强度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_lighting_scale", Summary: "设置 NekoToon 光照倍率", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_rimlight_boost", Summary: "设置 NekoToon $rimlightboost 倍率", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_rimlight_viewmodel_boost", Summary: "设置 NekoToon viewmodel 的 rimlight 倍率", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_brightness_limit", Summary: "限制 NekoToon 渲染结果亮度，避免模型过曝", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_darkness_limit", Summary: "限制 NekoToon 渲染结果暗度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_lazy_texture_load", Summary: "控制 NekoToon 是否在渲染时才加载材质贴图", Scope: "NekoShaders/材质", Risk: "中：可能改变加载时机", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_ignore_flat_normal", Summary: "控制是否禁止使用 flat_normal 以提升性能；值 2 更严格", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekotoon_normalized_lightwarp", Summary: "统一 NekoToon lightwarp 最大亮度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_neko_tonemapping_algorithm", Summary: "选择 Neko 引擎后处理 ToneMapping 曲线（需 -l4n_use_neko_engine_post）", Scope: "NekoShaders/材质", Risk: "中：改变整体曝光和颜色", Source: "readme_l4n.txt"},
	{Command: "mat_neko_tonemapping_force_linear", Summary: "强制使用 linear tonemapping，主要用于调试", Scope: "NekoShaders/材质", Risk: "中：改变整体颜色", Source: "readme_l4n.txt"},
	{Command: "mat_neko_gamma", Summary: "设置 Neko 引擎后处理 gamma", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_neko_engine_post_after", Summary: "设置相对原始画面的后处理混合比例", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekobloom_luminance_threshold", Summary: "设置 NekoBloom 激发亮度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekobloom_scale", Summary: "设置 NekoBloom 强度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekobloom_max_brightness", Summary: "限制 NekoBloom 亮度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekobloom_radius", Summary: "设置 NekoBloom 模糊半径", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekobloom_maptex_strength", Summary: "设置 NekoBloom 蒙版强度", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekobloom_maptex_weight", Summary: "设置 NekoBloom 蒙版权重", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_nekobloom_blend_mode", Summary: "设置 NekoBloom 混合模式（1 add、2 screen、3 softlight、4 replace）", Scope: "NekoShaders/材质", Risk: "低", Source: "readme_l4n.txt"},
	{Command: "mat_neko_pre_tonemapping", Summary: "在更早时机应用 tonemapping 曲线；可能与部分 ReShade 配置不兼容", Scope: "NekoShaders/材质", Risk: "中：可能影响 ReShade", Source: "readme_l4n.txt"},
	{Command: "-l4n_use_neko_engine_post", Summary: "启用 Neko 引擎后处理；这是启动项，不是 autoexec 控制台命令", Scope: "启动项", Risk: "中：需写入启动参数，不能放入 autoexec", Source: "readme_l4n.txt"},
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
	base := filepath.Base(rootDir)
	if strings.EqualFold(base, "addons") {
		// Normally rootDir is the selected left4dead2\addons directory. Keep
		// custom test/addons roots working, while recognizing the real game tree.
		parent := filepath.Dir(rootDir)
		if strings.EqualFold(filepath.Base(parent), "left4dead2") {
			rootDir = parent
		}
	} else if strings.EqualFold(base, "left4dead2") {
		// Already a game directory.
	} else {
		// If the user selected the Steam game root ("Left 4 Dead 2"), prefer
		// its real left4dead2 child when present.
		candidate := filepath.Join(rootDir, "left4dead2")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			rootDir = candidate
		}
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
	case autoexecEncodingWindows1252:
		return "Windows-1252/ANSI"
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
		decoded, _, gbkErr := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), raw)
		if gbkErr == nil && !strings.ContainsRune(string(decoded), '\uFFFD') {
			doc.content, doc.encoding = string(decoded), autoexecEncodingGBK
		} else {
			decoded, _, ansiErr := transform.Bytes(charmap.Windows1252.NewDecoder(), raw)
			if ansiErr != nil {
				return autoexecDocument{}, fmt.Errorf("无法按 GBK/ANSI 解码 autoexec.cfg（GBK: %v；Windows-1252: %w）", gbkErr, ansiErr)
			}
			if strings.ContainsRune(string(decoded), '\uFFFD') {
				return autoexecDocument{}, fmt.Errorf("无法按 GBK/ANSI 解码 autoexec.cfg：Windows-1252 结果包含替换字符（GBK: %v）", gbkErr)
			}
			doc.content, doc.encoding = string(decoded), autoexecEncodingWindows1252
		}
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
	case autoexecEncodingWindows1252:
		encoded, _, err := transform.Bytes(charmap.Windows1252.NewEncoder(), []byte(content))
		if err != nil {
			return nil, fmt.Errorf("autoexec.cfg 内容无法按 Windows-1252/ANSI 保存: %w", err)
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
	if err := replaceFile(tempPath, doc.path); err != nil {
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
	localAliases := make(map[string]struct{})
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if match := autoexecAliasDefinition.FindStringSubmatch(trimmed); len(match) == 2 {
			localAliases[strings.ToLower(match[1])] = struct{}{}
		}
	}
	result := make([]AutoexecCommandMatch, 0)
	for lineNumber, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if match := autoexecCommandToken.FindStringSubmatch(trimmed); len(match) == 2 {
			command := match[1]
			key := strings.ToLower(command)
			// The README documents l4n_scripted_hud_allow_slot[1~15] as
			// concrete convars (slot1 through slot15), not a literal command
			// containing brackets. Match only that bounded legal range.
			if strings.HasPrefix(key, "l4n_scripted_hud_allow_slot") {
				suffix := strings.TrimPrefix(key, "l4n_scripted_hud_allow_slot")
				if slot, parseErr := strconv.Atoi(suffix); parseErr == nil && slot >= 1 && slot <= 15 {
					key = "l4n_scripted_hud_allow_slot"
				}
			}
			if help, ok := autoexecCommandByName[key]; ok {
				helpCopy := help
				result = append(result, AutoexecCommandMatch{Line: lineNumber + 1, Raw: raw, Command: command, Known: true, Help: &helpCopy})
			} else if _, ok := localAliases[key]; ok {
				help := AutoexecCommandHelp{
					Command: command,
					Summary: "当前 autoexec.cfg 中定义的本地别名",
					Scope:   "本地配置",
					Risk:    "取决于别名展开内容",
					Source:  "当前 autoexec.cfg",
				}
				result = append(result, AutoexecCommandMatch{Line: lineNumber + 1, Raw: raw, Command: command, Known: true, Help: &help})
			} else {
				result = append(result, AutoexecCommandMatch{Line: lineNumber + 1, Raw: raw, Command: command})
			}
		}
	}
	return result
}
