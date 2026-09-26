package app

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	rt "runtime"
	"sync"
	"time"

	"vpk-manager/internal/network"
	"vpk-manager/internal/parser"

	"github.com/go-resty/resty/v2"
	"github.com/panjf2000/ants/v2"
)

// VPKFile 类型别名,用于Wails绑定
type VPKFile = parser.VPKFile

// IPOption 类型别名,用于Wails绑定
type IPOption = network.IPOption

// ServerInfo 服务器信息
type ServerInfo struct {
	Name       string `json:"name"`
	Map        string `json:"map"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
	GameDir    string `json:"gamedir"`
	Mode       string `json:"mode"`
}

// ProgressInfo 加载进度信息
type ProgressInfo struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Message string `json:"message"`
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	File    string `json:"file"`
}

// VPKFileCache 缓存的VPK文件信息
type VPKFileCache struct {
	File         VPKFile
	ModTime      time.Time
	Size         int64
	ImageModTime time.Time // 外部图片修改时间
	MetaModTime  time.Time // meta文件修改时间
	CachedAt     time.Time
}

// VPKPreviewCache 缓存按需读取的预览图。普通目录扫描不填充 VPKFile.PreviewImage，
// 只有用户看到卡片或打开详情时才会写入这里，并通过文件签名自动失效。
type VPKPreviewCache struct {
	Data         string
	ModTime      time.Time
	Size         int64
	ImageModTime time.Time
	CachedAt     time.Time
}

// conflictIndexCacheEntry caches the directory entries used by conflict
// analysis.  VPK archives are immutable for the duration of a scan in normal
// use, so a size/modtime signature lets repeated filter changes reuse the
// expensive archive directory read without retaining preview or file content.
type conflictIndexCacheEntry struct {
	ModTime  time.Time
	Size     int64
	Files    []string
	LastUsed time.Time
}

const (
	maxVPKPreviewCacheEntries     = 24
	maxVPKPreviewCacheBytes       = 24 * 1024 * 1024
	maxVPKCardPreviewCacheEntries = 96
	maxVPKCardPreviewCacheBytes   = 8 * 1024 * 1024
)

// submitPoolTask uses the shared pool when it is available. A released or
// unavailable pool must not strand WaitGroup-based callers: synchronous
// fallback keeps scans and on-demand analysis correct during shutdown/tests.
func (a *App) submitPoolTask(task func()) {
	// 后台任务（扫描、下载、冲突检测）里的 panic 不再直接终止进程：
	// 统一走崩溃上报，记录来源、堆栈与日志尾部。
	guarded := func() {
		a.runGuarded("协程池任务", task)
	}
	if a.goroutinePool == nil {
		guarded()
		return
	}
	if err := a.goroutinePool.Submit(guarded); err != nil {
		log.Printf("协程池无法接收任务，回退为同步执行: %v", err)
		guarded()
	}
}

// App struct
type App struct {
	ctx                     context.Context
	vpkCache                sync.Map // map[string]*VPKFileCache, key是文件路径
	previewCache            sync.Map // map[string]*VPKPreviewCache, key是文件路径
	cardPreviewCache        sync.Map // map[string]*VPKPreviewCache, key是文件路径
	previewCacheMu          sync.Mutex
	cardPreviewCacheMu      sync.Mutex
	archiveScanCacheMu      sync.Mutex
	archiveScanCache        *archiveScanCache
	archiveExistingIndexMu  sync.RWMutex
	archiveExistingIndexSig archiveExistingIndexSignature
	archiveExistingIndex    archiveExistingVPKIndex
	archiveExistingIndexSet bool
	conflictIndexMu         sync.Mutex
	conflictIndexCache      map[string]conflictIndexCacheEntry
	// conflictIndexCacheLimit 是本轮允许保留的索引条目数（见 conflictIndexCapacityFor）：
	// 跟随扫描到的候选文件数，避免大 Mod 库每轮重算都抖动重解析。
	conflictIndexCacheLimit int
	mu                      sync.RWMutex
	rootDir                 string
	goroutinePool           *ants.Pool
	conflictCheckMu         sync.Mutex
	modelStatsScanMu        sync.Mutex
	modelStatsScanRunning   bool
	modelStatsScanID        string
	modelStatsScanRoot      string
	modelStatsScanProgress  ProgressInfo
	addonListGuardMu        sync.Mutex
	addonListMonitorMu      sync.Mutex
	addonListMonitorStop    chan struct{}
	configWriteMu           sync.Mutex
	// externalCache 缓存"组建议收件箱"的解析结果，避免每次推导都重新解析文件与建索引。
	externalCache externalSuggestionCache
	// unreadableMods 记录"磁盘上存在但解析失败"的 VPK（例如扩展名是 .vpk 实为 ZIP），
	// 供导出清单时说明"扫描范围里少了哪些文件"。
	unreadableMods        sync.Map // map[string]string，key 是文件路径，value 是原因
	addonListGuardEnabled bool
	addonListLastRestore  string
	addonListLastError    string
	forceClose            bool
	restyClient           *resty.Client
	proxyServer           *network.ImageProxyServer
	singletonMgr          *SingletonManager // 单例管理器

	// 配置项
	modRotationConfig              RotationConfig
	workshopPreferredIP            bool
	workshopFixedIP                string
	workshopMetaEnabled            bool
	workshopUpdateCheckEnabled     bool
	workshopAutoRedownload         bool
	workshopBrowserTarget          string
	workshopTranslateProvider      string
	workshopTranslateCustomBaseURL string
	workshopTranslateCustomAPIKey  string
	workshopTranslateCustomModelId string
	migrationVersion               int
	defaultDirectory               string
	savedDirectories               []SavedDirectory
	lastActiveDirectory            string
	displayMode                    string
	filterLayoutMode               string
	boxSelectionEnabled            bool
	ctrlClickSelectionEnabled      bool
	uiScale                        float64
	// textSize / readingComfort 是「阅读舒适度」的两个档位（前端 core/reading-comfort.mjs 定义取值）。
	textSize                        string
	readingComfort                  string
	unrecordedModLoadOrderPlacement string
	conflictPriorityAware           bool
	conflictIgnoreFiles             []string
	// strategyGroupFloating 记录「策略组管理」窗口是否以浮动模式打开（nil 表示从未设置过）。
	strategyGroupFloating *bool
	// autoDetectWorkshopLink 是"剪贴板里的工坊链接自动识别"开关（nil 表示从未设置过 → 默认开启）。
	autoDetectWorkshopLink *bool
	// openWithProgram / openWithArguments 是「打开文件方式」的自定义外部程序（空 = 系统默认）。
	openWithProgram   string
	openWithArguments string
	// mainWindowWidth / mainWindowHeight / mainWindowMaximised 记住主窗口几何（nil 表示没记录过）。
	mainWindowWidth     *int
	mainWindowHeight    *int
	mainWindowMaximised *bool
	theme                  string
	ignoredVersion         string
	lastUpdateCheckTime    string
	configDir              string
	configPath             string
	serversPath            string
	workshopWatchLaterPath string
	problemScanPath        string
	profilesPath           string
	profilesMu             sync.Mutex
	groupsPath             string
	groupsMu               sync.Mutex
	dependenciesPath       string
	dependenciesMu         sync.Mutex
	priorityPath           string
	priorityMu             sync.Mutex
	ignorePath             string
	ignoreMu               sync.Mutex
	// localStoreBackup* 是本地记录备份轮转的可注入参数（测试用于控制时钟、间隔、上限与删除方式）。
	localStoreBackupClock    func() time.Time
	localStoreBackupInterval time.Duration
	localStoreBackupMaxFiles int
	localStoreBackupRemove   func(string) error
	// conflictRecheck 保存"变更驱动自动复检"的脏标记与结果缓存。
	conflictRecheck conflictRecheckState
	// stockWhitelist 缓存游戏原版文件白名单（内置批次 + 用户增量批次）。
	stockWhitelist stockWhitelistCache
	// collectionsPath / collectionsMu 管理"工坊合集实体化"记录。
	collectionsPath string
	// downloadTasksPath 是下载任务快照（download_tasks.json）的路径。
	downloadTasksPath string
	// fileOpsMu 是文件操作（移动 / 删除 / 打包）的互斥闸门，见 file_op_gate.go。
	fileOpsMu sync.Mutex
	collectionsMu     sync.Mutex
	// workshopDetailsFetcher 允许测试注入工坊详情获取器（默认走真实接口）。
	workshopDetailsFetcher workshopDetailFetcher
	// workshopItemDetailFetcher 允许测试注入"单个作品详情"获取器（更新检测用，默认走真实接口）。
	workshopItemDetailFetcher workshopItemDetailFetcher
	// crashReporter 保存崩溃上报的状态（日志环形缓冲、报告目录、幂等安装）。
	crashReporter crashReporter
}

// rootDirectorySnapshot returns a consistent directory value for background
// work. Directory selection can happen while scans/downloads are running, so
// callers should never read rootDir directly unless they already hold a.mu.
func (a *App) rootDirectorySnapshot() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.rootDir
}

// workshopOptionsSnapshot captures the two workshop flags atomically. A scan
// must use one coherent pair instead of observing a mid-update combination.
func (a *App) workshopOptionsSnapshot() (metaEnabled, updateCheckEnabled bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workshopMetaEnabled, a.workshopUpdateCheckEnabled
}

// ConfigFile 定义配置文件结构
type ConfigFile struct {
	ModRotationConfig              RotationConfig   `json:"modRotationConfig"`
	WorkshopPreferredIP            *bool            `json:"workshopPreferredIP,omitempty"`
	WorkshopFixedIP                *string          `json:"workshopFixedIP,omitempty"`
	WorkshopMetaEnabled            *bool            `json:"workshopMetaEnabled,omitempty"`
	WorkshopUpdateCheckEnabled     *bool            `json:"workshopUpdateCheckEnabled,omitempty"`
	WorkshopAutoRedownload         *bool            `json:"workshopAutoRedownload,omitempty"`
	WorkshopBrowserTarget          *string          `json:"workshopBrowserTarget,omitempty"`
	WorkshopTranslateProvider      *string          `json:"workshopTranslateProvider,omitempty"`
	WorkshopTranslateCustomBaseURL string           `json:"workshopTranslateCustomBaseURL,omitempty"`
	WorkshopTranslateCustomAPIKey  string           `json:"workshopTranslateCustomAPIKey,omitempty"`
	WorkshopTranslateCustomModelId string           `json:"workshopTranslateCustomModelId,omitempty"`
	DefaultDirectory               string           `json:"defaultDirectory"`
	SavedDirectories               []SavedDirectory `json:"savedDirectories"`
	LastActiveDirectory            string           `json:"lastActiveDirectory"`
	DisplayMode                    string           `json:"displayMode"`
	FilterLayoutMode               string           `json:"filterLayoutMode"`
	BoxSelectionEnabled            *bool            `json:"boxSelectionEnabled,omitempty"`
	CtrlClickSelectionEnabled      *bool            `json:"ctrlClickSelectionEnabled,omitempty"`
	UIScale                        float64          `json:"uiScale,omitempty"`
	// TextSize：文字大小档位（compact / standard / large / xlarge），与整体缩放相乘。
	TextSize string `json:"textSize,omitempty"`
	// ReadingComfort：阅读舒适度（soft / standard / contrast），控制行距与文字对比度。
	ReadingComfort                  string  `json:"readingComfort,omitempty"`
	AddonListGuardEnabled           *bool   `json:"addonListGuardEnabled,omitempty"`
	UnrecordedModLoadOrderPlacement *string `json:"unrecordedModLoadOrderPlacement,omitempty"`
	Theme                           string  `json:"theme"`
	IgnoredVersion                  string  `json:"ignoredVersion"`
	LastUpdateCheckTime             string  `json:"lastUpdateCheckTime"`
	// migrationVersion=2 表示前端 localStorage 配置已迁移到配置目录。
	MigrationVersion      int      `json:"migrationVersion"`
	ConflictPriorityAware *bool    `json:"conflictPriorityAware,omitempty"`
	ConflictIgnoreFiles   []string `json:"conflictIgnoreFiles,omitempty"`
	// StrategyGroupFloating 是「策略组管理」窗口的浮动偏好（前端窗口状态的一部分）。
	// 必须是 *bool：nil 表示"没设置过"，此时窗口按默认（浮动）打开。
	StrategyGroupFloating *bool `json:"strategyGroupFloating,omitempty"`
	// AutoDetectWorkshopLink 控制"复制了工坊链接后切回应用自动提示"（对齐 FireAxe 的
	// AppSettings.IsAutoDetectWorkshopItemLinkInClipboard）。nil 同样表示"没设置过"，默认开启。
	AutoDetectWorkshopLink *bool `json:"autoDetectWorkshopLink,omitempty"`
	// OpenWithProgram / OpenWithArguments 是自定义外部打开程序（对齐 FireAxe v0.7.2 的
	// process file customization）。两个都为空表示用系统默认（explorer /select 等）。
	OpenWithProgram   string `json:"openWithProgram,omitempty"`
	OpenWithArguments string `json:"openWithArguments,omitempty"`
	// MainWindowWidth / MainWindowHeight / MainWindowMaximised 记住主窗口的几何状态
	// （对齐 FireAxe v0.7.3 的"保存主窗口宽高与最大化状态"）。
	// 都用指针：nil 表示"没记录过"，此时保持 Wails 配置里的默认尺寸。
	MainWindowWidth     *int  `json:"mainWindowWidth,omitempty"`
	MainWindowHeight    *int  `json:"mainWindowHeight,omitempty"`
	MainWindowMaximised *bool `json:"mainWindowMaximised,omitempty"`
}

// RotationConfig Mod轮换配置
type RotationConfig struct {
	EnableCharacters bool `json:"enableCharacters"`
	EnableWeapons    bool `json:"enableWeapons"`
}

type SavedDirectory struct {
	Path     string `json:"path"`
	LastUsed string `json:"lastUsed"`
}

type ServerStorage struct {
	Servers       []SavedServer  `json:"servers"`
	RecentServers []RecentServer `json:"recentServers"`
}

type SavedServer struct {
	ID                     string `json:"id,omitempty"`
	Name                   string `json:"name"`
	Address                string `json:"address"`
	Weight                 int    `json:"weight"`
	PanelURL               string `json:"panelUrl,omitempty"`
	PanelPassword          string `json:"panelPassword,omitempty"`
	PanelPasswordEncrypted string `json:"panelPasswordEncrypted,omitempty"`
	PanelPasswordSet       bool   `json:"panelPasswordSet,omitempty"`
	ClearPanelPassword     bool   `json:"clearPanelPassword,omitempty"`
}

type RecentServer struct {
	Name            string `json:"name"`
	Address         string `json:"address"`
	LastConnectedAt int64  `json:"lastConnectedAt"`
}

type WorkshopWatchLaterStorage struct {
	Items []WorkshopWatchLaterItem `json:"items"`
}

type WorkshopWatchLaterItem struct {
	PublishedFileID string `json:"publishedfileid"`
	Title           string `json:"title"`
	PreviewURL      string `json:"preview_url"`
	Views           int    `json:"views"`
	Subscriptions   int    `json:"subscriptions"`
	Favorited       int    `json:"favorited"`
	FileType        int    `json:"file_type"`
	AddedAt         string `json:"addedAt"`
}

type LocalStorageMigrationPayload struct {
	Config              string `json:"config"`
	Theme               string `json:"theme"`
	LastUpdateCheckTime string `json:"lastUpdateCheckTime"`
	Servers             string `json:"servers"`
	RecentServers       string `json:"recentServers"`
	WatchLaterItems     string `json:"watchLaterItems"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	cores := rt.GOMAXPROCS(0)
	// 确保至少有 4 个并发，提升体验
	if cores < 4 {
		cores = 4
	}
	log.Printf("应用启动，CPU核心数: %d, 协程池大小: %d", rt.GOMAXPROCS(0), cores)

	pool, _ := ants.NewPool(cores) // 创建协程池

	// 初始化 Resty 客户端（强制 IPv4，避免伪 IPv6 导致连接失败）
	ipv4Dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	ipv4Transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return ipv4Dialer.DialContext(ctx, "tcp4", addr)
		},
	}
	client := resty.New()
	client.SetTimeout(2 * time.Second)
	client.SetTransport(ipv4Transport)

	// 启动本地图片代理
	proxy := network.NewImageProxyServer(network.GlobalIPSelector)
	proxy.Start()

	// 确定配置文件路径
	configDir, _ := os.UserConfigDir()
	appConfigDir := filepath.Join(configDir, "LytVPK")
	os.MkdirAll(appConfigDir, 0755)
	configPath := filepath.Join(appConfigDir, "config.json")
	serversPath := filepath.Join(appConfigDir, "servers.json")
	workshopWatchLaterPath := filepath.Join(appConfigDir, "workshop_watch_later.json")
	problemScanPath := filepath.Join(appConfigDir, "problem_mod_scan.json")
	profilesPath := filepath.Join(appConfigDir, "profiles.json")
	groupsPath := filepath.Join(appConfigDir, "groups.json")
	dependenciesPath := filepath.Join(appConfigDir, "dependencies.json")
	priorityPath := filepath.Join(appConfigDir, "priority.json")
	ignorePath := filepath.Join(appConfigDir, "ignore.json")
	collectionsPath := filepath.Join(appConfigDir, "collections.json")
	downloadTasksPath := filepath.Join(appConfigDir, downloadTaskSnapshotFileName)

	app := &App{
		goroutinePool:                   pool,
		restyClient:                     client,
		proxyServer:                     proxy,
		configDir:                       appConfigDir,
		configPath:                      configPath,
		serversPath:                     serversPath,
		workshopWatchLaterPath:          workshopWatchLaterPath,
		problemScanPath:                 problemScanPath,
		profilesPath:                    profilesPath,
		groupsPath:                      groupsPath,
		dependenciesPath:                dependenciesPath,
		priorityPath:                    priorityPath,
		ignorePath:                      ignorePath,
		collectionsPath:                 collectionsPath,
		downloadTasksPath:               downloadTasksPath,
		workshopPreferredIP:             true,     // 默认开启优选IP
		workshopMetaEnabled:             true,     // 默认开启工坊meta信息存储
		workshopBrowserTarget:           "mirror", // 默认使用镜像站
		workshopTranslateProvider:       workshopTranslateProviderMicrosoft,
		displayMode:                     "list",
		filterLayoutMode:                "compact",
		boxSelectionEnabled:             true,
		ctrlClickSelectionEnabled:       true,
		uiScale:                         defaultUIScale,
		unrecordedModLoadOrderPlacement: addonListUnrecordedPlacementEnd,
		savedDirectories:                []SavedDirectory{},
	}

	// 加载配置
	app.loadConfig()

	return app
}
