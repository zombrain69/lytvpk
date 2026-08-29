package parser

// ChapterInfo 章节信息用于前端显示
type ChapterInfo struct {
	Title string   `json:"title"` // 章节标题
	Modes []string `json:"modes"` // 支持的游戏模式
}

// XDRSlotInfo describes one xdReanimsBase-compatible animation model and the
// slot it occupies for a survivor or infected character.
type XDRSlotInfo struct {
	Character  string   `json:"character"`
	Model      string   `json:"model"`
	Scope      string   `json:"scope"`
	Slot       int      `json:"slot"`
	SlotLabel  string   `json:"slotLabel"`
	Actions    []string `json:"actions"`
	Evidence   []string `json:"evidence"`
	Confidence string   `json:"confidence"`
}

// VPKFile 表示一个VPK文件的信息
type VPKFile struct {
	Name              string                 `json:"name"`
	Path              string                 `json:"path"`
	Size              int64                  `json:"size"`
	PrimaryTag        string                 `json:"primaryTag"`        // 一级标签: "地图", "人物", "武器", "其他"
	SecondaryTags     []string               `json:"secondaryTags"`     // 二级标签: ["ellis", "ak47", "versus"] 等
	VoiceCharacters   []string               `json:"voiceCharacters"`   // 从标准 sound/player 语音目录识别出的替换角色
	ContentSubjects   []string               `json:"contentSubjects"`   // 基于资源路径证据识别出的实际主体
	SubjectSummary    string                 `json:"subjectSummary"`    // 面向用户的主体摘要
	SubjectConfidence string                 `json:"subjectConfidence"` // 主体证据置信度：高/中/低
	XDRSlots          []XDRSlotInfo          `json:"xdrSlots"`          // xdReanimsBase 角色/模型与 slot 证据
	XDRSummary        string                 `json:"xdrSummary"`        // 面向用户的 XDR 精确摘要
	Location          string                 `json:"location"`          // "root", "workshop", "disabled"
	Enabled           bool                   `json:"enabled"`
	GameEnabled       bool                   `json:"gameEnabled"`    // addonlist.txt 中的游戏内开关
	GameStateKnown    bool                   `json:"gameStateKnown"` // addonlist.txt 是否包含此 Mod
	ModelStatsKnown   bool                   `json:"modelStatsKnown"`
	ModelCount        int                    `json:"modelCount"`
	ModelVertices     int                    `json:"modelVertices"`
	ModelTriangles    int                    `json:"modelTriangles"`
	Campaign          string                 `json:"campaign"`
	Chapters          map[string]ChapterInfo `json:"chapters"` // key: 章节代码, value: 章节信息
	Mode              string                 `json:"mode"`
	PreviewImage      string                 `json:"previewImage"` // Base64编码的预览图
	LastModified      string                 `json:"lastModified"`
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
