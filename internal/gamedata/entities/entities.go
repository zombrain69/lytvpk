// Package entities 持有「本体实体表」：由游戏自带脚本生成，把**本体规范路径 → 实体标签**钉死。
//
// 为什么需要它：标签识别历史上靠"命名直觉"猜实体，会在两处翻车——
//
//	猜错：`weapon_shotgun_chrome` 的世界模型叫 `models/w_models/weapons/w_pumpshotgun_A.mdl`，
//	     而 `weapon_pumpshotgun` 的才叫 `w_shotgun.mdl`（按名字猜会把两者对调）；
//	漏掉：本体确实存在、关键词表里却没有的实体（`w_golfclub.mdl`、`explosive_box001.mdl`、`w_cola.mdl`）。
//
// 本包只做**精确路径匹配**：mod 里出现与本体完全相同的路径 → 判定为"替换了该实体"。
// 它是纯叶子包（不 import 任何内部包），方便 parser 直接引用而不产生循环依赖。
package entities

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed entities.json
var rawTable []byte

// SchemaVersion 是实体表格式版本。
const SchemaVersion = 1

// Table 是整张实体表。
type Table struct {
	SchemaVersion int      `json:"schemaVersion"`
	GeneratedAt   string   `json:"generatedAt,omitempty"`
	Source        string   `json:"source,omitempty"`
	Entities      []Entity `json:"entities"`

	byAnchor map[string][]Hit
}

// Entity 是一个本体实体（一把枪、一个物品、一只特感的爪子……）。
type Entity struct {
	ID       string `json:"id"`
	Tag      string `json:"tag"`
	Category string `json:"category,omitempty"`
	Group    string `json:"group,omitempty"`
	Official bool   `json:"official,omitempty"`
	IsItem   bool   `json:"item,omitempty"`
	// Character 非空表示这是角色资产（特感爪子/幸存者手臂），值是角色名（Hunter / Coach…）。
	Character string `json:"character,omitempty"`
	// Script 是证据来源（本体脚本路径），便于回溯。
	Script string `json:"script,omitempty"`
	// Anchors 是本体规范路径（小写、"/" 分隔）。
	Anchors []string `json:"anchors"`
	// Slots 是显式"槽位"表（开发文档 §4.2）：每个 slot = 类别 + 本体前缀 + 该前缀在
	// 本体索引里的命中数。由 `--generate-entity-table` 推导产出并随表提交，人工可逐条 review；
	// `--validate-entity-slots` 会拿当前本体索引复验（0 命中 = 错误）。
	Slots []Slot `json:"slots,omitempty"`
	Note  string `json:"note,omitempty"`
}

// Slot 是一个可独立替换的资源槽位（worldmodel / viewmodel / script / sound / material / ui）。
type Slot struct {
	Kind   string `json:"kind"`
	Prefix string `json:"prefix"`
	Hits   int    `json:"hits"`
}

// Hit 是一次锚点命中的结果。
type Hit struct {
	EntityID  string
	Tag       string
	Category  string
	Group     string
	IsItem    bool
	Character string
	Anchor    string
}

// Load 解析内置实体表。
func Load() (*Table, error) {
	var table Table
	if err := json.Unmarshal(rawTable, &table); err != nil {
		return nil, fmt.Errorf("解析内置实体表失败: %w", err)
	}
	if table.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("实体表版本不匹配：文件 %d，当前支持 %d", table.SchemaVersion, SchemaVersion)
	}
	table.attach()
	return &table, nil
}

// MustLoad 供解析器与测试使用；内置资源损坏属于构建期错误。
func MustLoad() *Table {
	table, err := Load()
	if err != nil {
		panic(err)
	}
	return table
}

// NewTable 用给定实体构造表（测试与外部工具用）。
func NewTable(items []Entity) *Table {
	table := &Table{SchemaVersion: SchemaVersion, Entities: items}
	table.attach()
	return table
}

func (table *Table) attach() {
	if table.byAnchor != nil {
		return
	}
	table.byAnchor = make(map[string][]Hit, len(table.Entities))
	for _, entity := range table.Entities {
		for _, anchor := range entity.Anchors {
			key := NormalizePath(anchor)
			if key == "" {
				continue
			}
			table.byAnchor[key] = append(table.byAnchor[key], Hit{
				EntityID:  entity.ID,
				Tag:       entity.Tag,
				Category:  entity.Category,
				Group:     entity.Group,
				IsItem:    entity.IsItem,
				Character: entity.Character,
				Anchor:    key,
			})
		}
	}
}

// Lookup 返回某个归档内路径命中的全部实体（大小写不敏感，"/" 分隔）。
func (table *Table) Lookup(path string) []Hit {
	if table == nil {
		return nil
	}
	table.attach()
	return table.byAnchor[NormalizePath(path)]
}

// AnchorCount 返回锚点总数（去重后），供状态展示与测试。
func (table *Table) AnchorCount() int {
	if table == nil {
		return 0
	}
	table.attach()
	return len(table.byAnchor)
}

// NormalizePath 与 gamedata.NormalizePath 同口径（本包为叶子包，故自带一份最小实现）。
func NormalizePath(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	return strings.ToLower(strings.TrimSpace(name))
}
