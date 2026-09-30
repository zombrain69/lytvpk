// Package ruletable 是「标签识别规则表」的**唯一事实源**（W2 迁移收尾）。
//
// 它只做三件事：把 rules.json 嵌进二进制、解析成类型、提供只读访问。
// **刻意不 import 任何项目内包**：解析器要直接读它（parser → ruletable），
// 校验器也要读它（gamedata/rules → ruletable），叶子包才能避免循环依赖。
//
// 数据来源与维护方式：
//   - `contentRules` / `weaponPathRules` / `characterRules` / `fileKinds` 就是解析器实际使用的规则；
//     改规则 = 改这个 JSON（`internal/gamedata/rules` 的校验器会用游戏本体索引检查它是否真的能命中）；
//   - 回归护栏见 `--check-tag-regression`（标签只增不减）。
package ruletable

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed rules.json
var rawTable []byte

// SchemaVersion 是规则表格式版本。
const SchemaVersion = 1

// Table 是整张规则表。
type Table struct {
	SchemaVersion   int           `json:"schemaVersion"`
	Notes           string        `json:"notes,omitempty"`
	FileKinds       []FileKind    `json:"fileKinds"`
	ContentRules    []ContentRule `json:"contentRules"`
	WeaponPathRules []MatchRule   `json:"weaponPathRules"`
	// WeaponMetadataRules 是"标题/描述"侧的武器关键词表（W4 通道 4 使用）。
	WeaponMetadataRules []MatchRule    `json:"weaponMetadataRules"`
	CharacterRules      CharacterRules `json:"characterRules"`
}

// FileKind 是「文件类别」标签（UI / 声音 / 脚本 / 模型 / 贴图 / 粒子特效…），按目录前缀判定。
type FileKind struct {
	Tag      string   `json:"tag"`
	Prefixes []string `json:"prefixes"`
}

// ContentRule 是内容/物品规则：目录前缀限定 + 关键词命中（关键词为空表示"该目录下全算"）。
type ContentRule struct {
	Tag      string   `json:"tag"`
	Prefixes []string `json:"prefixes"`
	Keywords []string `json:"keywords,omitempty"`
	Item     bool     `json:"item,omitempty"`
}

// MatchRule 是"关键词 → 标签"的简单映射（武器路径规则）。
type MatchRule struct {
	Keyword string `json:"keyword"`
	Tag     string `json:"tag"`
}

// CharacterRules 汇总角色侧的六张表。
type CharacterRules struct {
	VoiceDirSurvivor []VoiceDirRule `json:"voiceDirSurvivor"`
	VoiceDirInfected []VoiceDirRule `json:"voiceDirInfected"`
	SurvivorVariants []MatchRule    `json:"survivorVariants"`
	Survivors        []MatchRule    `json:"survivors"`
	SpecialInfected  []MatchRule    `json:"specialInfected"`
	CommonInfected   []MatchRule    `json:"commonInfected"`
}

// VoiceDirRule 是"语音目录段 → 角色名"的映射（如 namvet → Bill）。
type VoiceDirRule struct {
	Dir       string `json:"dir"`
	Character string `json:"character"`
}

// Load 解析内置规则表。
func Load() (*Table, error) {
	var table Table
	if err := json.Unmarshal(rawTable, &table); err != nil {
		return nil, fmt.Errorf("解析内置规则表失败: %w", err)
	}
	if table.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("规则表版本不匹配：文件 %d，当前支持 %d", table.SchemaVersion, SchemaVersion)
	}
	if len(table.ContentRules) == 0 || len(table.WeaponPathRules) == 0 {
		return nil, fmt.Errorf("规则表内容为空")
	}
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
