package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkshopMeta 存储工坊文件的元数据
type WorkshopMeta struct {
	WorkshopID   string `json:"workshop_id"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	Description  string `json:"description"`
	PreviewURL   string `json:"preview_url"`
	FileURL      string `json:"file_url"`
	DownloadedAt string `json:"downloaded_at"`
	TimeUpdated  string `json:"time_updated"` // 远端最后更新时间（RFC3339）
	// Tags 是完整标签列表，保留用于兼容和外部查看；PrimaryTag/SecondaryTags 保留前端的标签层级。
	Tags          []string `json:"tags,omitempty"`
	PrimaryTag    string   `json:"primary_tag,omitempty"`
	SecondaryTags []string `json:"secondary_tags,omitempty"`
}

// GetMetaFilePath 根据VPK路径计算对应的.meta文件路径
func GetMetaFilePath(filePath string) string {
	ext := filepath.Ext(filePath)
	return strings.TrimSuffix(filePath, ext) + ".meta"
}

// SaveWorkshopMeta 将工坊详情保存为.meta文件
func SaveWorkshopMeta(filePath string, details WorkshopFileDetails) error {
	meta := WorkshopMeta{
		WorkshopID:   details.PublishedFileId,
		Title:        details.Title,
		Author:       details.Creator,
		Description:  details.Description,
		PreviewURL:   details.PreviewUrl,
		FileURL:      details.FileUrl,
		DownloadedAt: time.Now().Format(time.RFC3339),
	}
	if existing, err := LoadWorkshopMeta(filePath); err != nil {
		return err
	} else if existing != nil {
		meta.Tags = append([]string(nil), existing.Tags...)
		meta.PrimaryTag = existing.PrimaryTag
		meta.SecondaryTags = append([]string(nil), existing.SecondaryTags...)
	}
	return saveWorkshopMeta(filePath, &meta)
}

// LoadWorkshopMeta 读取.meta文件，不存在时返回nil
func LoadWorkshopMeta(filePath string) (*WorkshopMeta, error) {
	metaPath := GetMetaFilePath(filePath)

	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var meta WorkshopMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// UpdateWorkshopMetaTimeUpdated 更新meta文件中的TimeUpdated字段（保留其他字段）
func UpdateWorkshopMetaTimeUpdated(filePath string, timeUpdated string) error {
	meta, err := LoadWorkshopMeta(filePath)
	if meta == nil || err != nil {
		return err
	}
	meta.TimeUpdated = timeUpdated
	return saveWorkshopMeta(filePath, meta)
}

func saveWorkshopMeta(filePath string, meta *WorkshopMeta) error {
	if meta == nil {
		return nil
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(GetMetaFilePath(filePath), data, 0644)
}
