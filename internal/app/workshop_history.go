package app

import (
	"errors"
	"log"
	"os"
	"strings"
	"time"
)

// 「工坊解析历史」——对齐上游 `feat 添加解析历史功能`（b635ea3），
// 按本项目约定补上 schemaVersion：本地记录统一带版本号，方便以后迁移。
//
// 语义：最近解析过的工坊物品 / 合集结果整份快照存下来（含 group），
// 这样重新打开时不用再请求一次接口，直接就能回到上次那批结果。

const (
	// workshopHistoryLimit 解析历史最多保留的条数（上游同为 10）。
	workshopHistoryLimit = 10
	// workshopHistorySchemaVersion 是本文件的存档版本号。
	workshopHistorySchemaVersion = 1
	// workshopHistoryFileName 是配置目录下的文件名。
	workshopHistoryFileName = "workshop_history.json"
)

// WorkshopHistoryStorage 是解析历史的存储结构。
type WorkshopHistoryStorage struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Items         []WorkshopHistoryItem `json:"items"`
}

// WorkshopHistoryItem 是单条解析记录（保存解析时的完整结果快照）。
type WorkshopHistoryItem struct {
	RootID   string               `json:"rootId"`
	Title    string               `json:"title"`
	FileType int                  `json:"fileType"` // 2 = 合集
	ParsedAt int64                `json:"parsedAt"` // Unix 毫秒
	Group    WorkshopDetailsGroup `json:"group"`
}

// GetWorkshopHistory 返回最近解析记录（最新在前）。文件损坏或不存在时返回空列表，不报错。
func (a *App) GetWorkshopHistory() WorkshopHistoryStorage {
	a.workshopHistoryMu.Lock()
	defer a.workshopHistoryMu.Unlock()
	return a.loadWorkshopHistory()
}

func (a *App) loadWorkshopHistory() WorkshopHistoryStorage {
	a.ensureConfigPaths()
	var storage WorkshopHistoryStorage
	if err := readJSONFile(a.workshopHistoryPath, &storage); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("读取解析历史失败，已使用空记录: %v", err)
		}
		return WorkshopHistoryStorage{SchemaVersion: workshopHistorySchemaVersion, Items: []WorkshopHistoryItem{}}
	}
	storage.SchemaVersion = workshopHistorySchemaVersion
	storage.Items = cloneWorkshopHistoryItems(storage.Items)
	return storage
}

// AddWorkshopHistoryEntries 把本次解析的条目置顶写入历史，返回写入后的完整列表。
// 同一 rootId 只保留最新那条（重复解析会刷新标题、时间与快照）。
func (a *App) AddWorkshopHistoryEntries(items []WorkshopHistoryItem) (WorkshopHistoryStorage, error) {
	a.workshopHistoryMu.Lock()
	defer a.workshopHistoryMu.Unlock()

	existing := a.loadWorkshopHistory()
	now := time.Now().UnixMilli()

	merged := make([]WorkshopHistoryItem, 0, len(items)+len(existing.Items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		item.RootID = strings.TrimSpace(item.RootID)
		if item.RootID == "" || seen[item.RootID] {
			continue
		}
		seen[item.RootID] = true
		if item.ParsedAt == 0 {
			item.ParsedAt = now
		}
		merged = append(merged, item)
	}
	for _, item := range existing.Items {
		if item.RootID == "" || seen[item.RootID] {
			continue
		}
		seen[item.RootID] = true
		merged = append(merged, item)
	}

	if len(merged) > workshopHistoryLimit {
		merged = merged[:workshopHistoryLimit]
	}

	storage := WorkshopHistoryStorage{
		SchemaVersion: workshopHistorySchemaVersion,
		Items:         cloneWorkshopHistoryItems(merged),
	}
	if err := writeJSONFile(a.configDir, a.workshopHistoryPath, storage); err != nil {
		return WorkshopHistoryStorage{SchemaVersion: workshopHistorySchemaVersion, Items: []WorkshopHistoryItem{}}, err
	}
	return storage, nil
}

// ClearWorkshopHistory 清空解析历史。
func (a *App) ClearWorkshopHistory() error {
	a.workshopHistoryMu.Lock()
	defer a.workshopHistoryMu.Unlock()

	a.ensureConfigPaths()
	return writeJSONFile(a.configDir, a.workshopHistoryPath, WorkshopHistoryStorage{
		SchemaVersion: workshopHistorySchemaVersion,
		Items:         []WorkshopHistoryItem{},
	})
}

func cloneWorkshopHistoryItems(items []WorkshopHistoryItem) []WorkshopHistoryItem {
	if items == nil {
		return []WorkshopHistoryItem{}
	}
	next := make([]WorkshopHistoryItem, len(items))
	copy(next, items)
	return next
}
