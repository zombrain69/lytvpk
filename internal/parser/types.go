package parser

// ChapterInfo 章节信息用于前端显示
type ChapterInfo struct {
	Title string   `json:"title"` // 章节标题
	Modes []string `json:"modes"` // 支持的游戏模式
}

// XDRSlotInfo describes one xdReanimsBase-compatible animation model and the
// slot it occupies for a survivor or infected character.
type XDRSlotInfo struct {
	Character string `json:"character"`
	Model     string `json:"model"`
	Scope     string `json:"scope"`
	Slot      int    `json:"slot"`
	SlotLabel string `json:"slotLabel"`
	// SlotName / SlotGroup 是官方建议用途（见 xdr_slots.go）。
	// 作者可能自选其它槽位，所以只能当参考，不能当事实。
	SlotName   string   `json:"slotName,omitempty"`
	SlotGroup  string   `json:"slotGroup,omitempty"`
	Actions    []string `json:"actions"`
	Evidence   []string `json:"evidence"`
	Confidence string   `json:"confidence"`
}

// XDRSlotRival 是"同一个角色同一个槽"上的另一个 Mod。
//
// 官方规则（xdReanimsBase 作者）：同角色同槽只会**随机生效一个**，不是按加载顺序。
// 所以同槽的 Mod 都要被标出来，用户才知道自己看到的动作可能是哪一个。
type XDRSlotRival struct {
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
}

// XDRSlotStatus 描述"这个 Mod 的某个角色/槽位会不会真的播出来"。
// State 取值：active（该槽只有它，会按预期播放）/ random（同槽有别人，游戏随机选一个）。
type XDRSlotStatus struct {
	Character string         `json:"character"`
	Slot      int            `json:"slot"`
	SlotLabel string         `json:"slotLabel"`
	SlotName  string         `json:"slotName,omitempty"`
	State     string         `json:"state"`
	Rivals    []XDRSlotRival `json:"rivals,omitempty"`
}

// XDRPriorityInfo 汇总一个 Mod 的 XDR 动作生效情况。
// State 取值：active（全部槽位唯一）/ random（全部槽位都被同槽占用）/ partial（部分唯一）。
type XDRPriorityInfo struct {
	State       string          `json:"state"`
	ActiveSlots int             `json:"activeSlots"`
	RandomSlots int             `json:"randomSlots"`
	Slots       []XDRSlotStatus `json:"slots,omitempty"`
}

// VPKFile 表示一个VPK文件的信息
type VPKFile struct {
	Name              string        `json:"name"`
	Path              string        `json:"path"`
	Size              int64         `json:"size"`
	PrimaryTag        string        `json:"primaryTag"`        // 一级标签: "地图", "人物", "武器", "其他"
	SecondaryTags     []string      `json:"secondaryTags"`     // 二级标签: ["ellis", "ak47", "versus"] 等
	TagEvidence       []TagEvidence `json:"tagEvidence"`       // 每个标签的证据来源（W6：只解释，不参与取舍）
	VoiceCharacters   []string      `json:"voiceCharacters"`   // 从标准 sound/player 语音目录识别出的替换角色
	ContentSubjects   []string      `json:"contentSubjects"`   // 基于资源路径证据识别出的实际主体
	SubjectSummary    string        `json:"subjectSummary"`    // 面向用户的主体摘要
	SubjectConfidence string        `json:"subjectConfidence"` // 主体证据置信度：高/中/低
	XDRSlots          []XDRSlotInfo `json:"xdrSlots"`          // xdReanimsBase 角色/模型与 slot 证据
	XDRSummary        string        `json:"xdrSummary"`        // 面向用户的 XDR 精确摘要
	// XDRPriority 是"这些动作到底会不会播"的结论（跨 Mod 计算，扫描后按需附加）。
	XDRPriority *XDRPriorityInfo `json:"xdrPriority,omitempty"`
	// VPK 内部结构摘要（扫描时顺带统计，供分组推导与外部智能体分析使用）。
	StructureTopDirs     []string `json:"structureTopDirs"`     // 顶层目录及条目数（按名称排序）
	StructureFileCount   int      `json:"structureFileCount"`   // 条目总数
	StructureTotalSize   int64    `json:"structureTotalSize"`   // 条目大小合计（压缩前）
	StructureSamplePaths []string `json:"structureSamplePaths"` // 有代表性的资源路径（截断）
	StructureTargets     []string `json:"structureTargets"`     // 压缩后的替换目标（如 props_interiors/medicalcabinet02）
	// StructureResourceRoots 是"作者/套件命名空间"（如 913limod/airi_evilfall、codm/ice）：
	// 同一个套件常拆成很多 VPK（本体 + 配件 / 贴图包 / 参数包），它们共享这个目录，
	// 是判断"需要一起启用"的结构性证据。只收非官方根，最多 6 条。
	StructureResourceRoots []string `json:"structureResourceRoots"`
	Location               string   `json:"location"` // "root", "workshop", "disabled"
	Enabled                bool     `json:"enabled"`
	// ArchivePack 非空表示"扩展名是 .vpk、实际是压缩包"：工坊作者特意做的插件/工具/教程包。
	// 游戏不会加载它，但它常常已经占着 addonlist.txt 的一行（也就占了优先级位置），
	// 所以照常出现在列表里、照常可以开/关，只是界面上要带一个显眼的"非 VPK 压缩包"标记。
	ArchivePack     *ArchivePackInfo       `json:"archivePack,omitempty"`
	GameEnabled     bool                   `json:"gameEnabled"`    // addonlist.txt 中的游戏内开关
	GameStateKnown  bool                   `json:"gameStateKnown"` // addonlist.txt 是否包含此 Mod
	ModelStatsKnown bool                   `json:"modelStatsKnown"`
	ModelCount      int                    `json:"modelCount"`
	ModelVertices   int                    `json:"modelVertices"`
	ModelTriangles  int                    `json:"modelTriangles"`
	Campaign        string                 `json:"campaign"`
	Chapters        map[string]ChapterInfo `json:"chapters"` // key: 章节代码, value: 章节信息
	Mode            string                 `json:"mode"`
	PreviewImage    string                 `json:"previewImage"`    // Base64编码的预览图
	PreviewRevision string                 `json:"previewRevision"` // 预览源签名；用于前端跨刷新/移动复用已解码图片
	LastModified    string                 `json:"lastModified"`
	// addoninfo.txt 相关信息
	Title      string `json:"title"`      // addontitle (必有)
	Author     string `json:"author"`     // addonauthor (若有)
	Version    string `json:"version"`    // addonversion (若有)
	Desc       string `json:"desc"`       // addonDescription (若有)
	AddonURL0  string `json:"addonURL0"`  // addonURL0 (若有)
	WorkshopID string `json:"workshopId"` // 工坊ID (从meta文件读取)
	HasUpdate  bool   `json:"hasUpdate"`  // 远端更新时间 > 下载时间且开启了更新检测
}

// Campaign 战役信息
type Campaign struct {
	Title    string
	Chapters []*Chapter
}

// Chapter 章节信息
type Chapter struct {
	Code  string   // 章节代码 (如 c1m1_hotel)
	Title string   // 章节显示名 (如 "The Hotel")
	Modes []string // 支持的游戏模式
}
