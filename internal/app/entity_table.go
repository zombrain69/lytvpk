package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vpk-manager/internal/gamedata"
	"vpk-manager/internal/gamedata/entities"
	"vpk-manager/internal/gamedata/entitygen"
)

// 实体表生成（维护者工具）。
//
// 生成结果写进仓库（internal/gamedata/entities/entities.json）后由解析器嵌入使用；
// 生成过程只读游戏目录。默认输出到配置目录，便于先比对再入库。

// GenerateEntityTable 从游戏本体脚本生成实体表。
func (a *App) GenerateEntityTable(gameDirOverride, target string) (string, *entities.Table, error) {
	gameRoot := ""
	if strings.TrimSpace(gameDirOverride) != "" {
		normalized, err := gamedata.NormalizeGameRoot(gameDirOverride)
		if err != nil {
			return "", nil, err
		}
		gameRoot = normalized
	} else {
		resolved, err := a.gameRootForStockIndex()
		if err != nil {
			return "", nil, err
		}
		gameRoot = resolved
	}

	table, err := entitygen.Generate(gameRoot)
	if err != nil {
		return "", nil, err
	}

	out := strings.TrimSpace(target)
	if out == "" {
		a.ensureConfigPaths()
		if a.configDir == "" {
			return "", nil, fmt.Errorf("未配置配置目录，请用 --out 指定输出路径")
		}
		out = filepath.Join(a.configDir, "entities.json")
	}
	data, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		return "", nil, err
	}
	return out, table, nil
}

// EntityTableStatus 是实体表的简要状态（给 CLI / 设置页展示）。
type EntityTableStatus struct {
	Entities   int    `json:"entities"`
	Anchors    int    `json:"anchors"`
	Characters int    `json:"characters"`
	Items      int    `json:"items"`
	File       string `json:"file,omitempty"`
	Error      string `json:"error,omitempty"`
}

// GetEntityTableStatus 读取内置实体表的状态（不写任何文件）。
func (a *App) GetEntityTableStatus() EntityTableStatus {
	table, err := entities.Load()
	if err != nil {
		return EntityTableStatus{Error: err.Error()}
	}
	status := EntityTableStatus{Entities: len(table.Entities), Anchors: table.AnchorCount()}
	for _, entity := range table.Entities {
		if entity.Character != "" {
			status.Characters++
		}
		if entity.IsItem {
			status.Items++
		}
	}
	return status
}
