package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AddonListMergeConflict 表示两个配置中同一 Mod 的游戏内开关不一致。
type AddonListMergeConflict struct {
	Key            string `json:"key"`
	CurrentValue   string `json:"currentValue"`
	SourceValue    string `json:"sourceValue"`
	CurrentEnabled bool   `json:"currentEnabled"`
	SourceEnabled  bool   `json:"sourceEnabled"`
}

// AddonListMergePreview 是用户确认融合前展示给前端的差异。
type AddonListMergePreview struct {
	SourcePath string                   `json:"sourcePath"`
	TargetPath string                   `json:"targetPath"`
	Added      []AddonListItem          `json:"added"`
	Conflicts  []AddonListMergeConflict `json:"conflicts"`
}

// SelectAddonListMergeSource 选择一个要融合的 addonlist.txt 文件。
func (a *App) SelectAddonListMergeSource() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择要融合的 addonlist.txt",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "addonlist.txt",
				Pattern:     "addonlist.txt",
			},
			{
				DisplayName: "文本文件 (*.txt)",
				Pattern:     "*.txt",
			},
		},
	})
	if err != nil || path == "" {
		return path, err
	}
	if !strings.EqualFold(filepath.Base(path), "addonlist.txt") {
		return "", fmt.Errorf("请选择名为 addonlist.txt 的文件")
	}
	return path, nil
}

// PreviewAddonListMerge 对比当前目标配置与来源配置。相同项的当前开关默认优先保留。
func (a *App) PreviewAddonListMerge(sourcePath string) (AddonListMergePreview, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()
	return a.previewAddonListMergeLocked(sourcePath)
}

// ApplyAddonListMerge 融合来源配置。所有新增项都会加入；仅 sourceWinsKeys 中的冲突项采用来源开关。
func (a *App) ApplyAddonListMerge(sourcePath string, sourceWinsKeys []string) error {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	preview, err := a.previewAddonListMergeLocked(sourcePath)
	if err != nil {
		return err
	}

	doc, err := a.readAddonListDocument()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		doc = addonListDocument{
			path:     preview.TargetPath,
			content:  "\"AddonList\"\n{\n}\n",
			encoding: addonListEncodingUTF8,
		}
	}

	sourceDoc, err := readAddonListDocumentAtPath(preview.SourcePath)
	if err != nil {
		return fmt.Errorf("无法读取来源 addonlist.txt: %w", err)
	}
	currentItems := parseAddonListItems(doc.content)
	sourceItems := parseAddonListItems(sourceDoc.content)
	currentByKey := addonListItemsByKey(currentItems)
	sourceByKey, sourceOrder := addonListItemsByKeyInOrder(sourceItems)
	sourceWins := make(map[string]bool, len(sourceWinsKeys))
	for _, key := range sourceWinsKeys {
		sourceWins[normalizeAddonListKey(key)] = true
	}

	updatedContent := doc.content
	changed := false
	for _, key := range sourceOrder {
		sourceItem := sourceByKey[key]
		currentItem, exists := currentByKey[key]
		if exists && (currentItem.Value == sourceItem.Value || !sourceWins[key]) {
			continue
		}

		var replaced bool
		updatedContent, replaced, err = replaceAddonListValue(updatedContent, key, sourceItem.Value)
		if err != nil {
			return err
		}
		if replaced {
			changed = true
			currentByKey[key] = sourceItem
		}
	}

	if !changed {
		return nil
	}
	if err := a.writeAddonListDocument(doc, updatedContent); err != nil {
		return err
	}
	if err := a.syncManagedAddonListSnapshotLocked(doc.path); err != nil {
		return err
	}
	a.applyAddonListGameStates()
	return nil
}

func (a *App) previewAddonListMergeLocked(sourcePath string) (AddonListMergePreview, error) {
	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	if sourcePath == "." || sourcePath == "" {
		return AddonListMergePreview{}, fmt.Errorf("请选择要融合的 addonlist.txt")
	}
	if !strings.EqualFold(filepath.Base(sourcePath), "addonlist.txt") {
		return AddonListMergePreview{}, fmt.Errorf("来源文件必须命名为 addonlist.txt")
	}
	if info, err := os.Stat(sourcePath); err != nil {
		return AddonListMergePreview{}, fmt.Errorf("无法读取来源 addonlist.txt: %w", err)
	} else if info.IsDir() {
		return AddonListMergePreview{}, fmt.Errorf("来源 addonlist.txt 不能是目录")
	}

	targetPath, err := a.addonListPath()
	if err != nil {
		return AddonListMergePreview{}, err
	}
	if strings.EqualFold(filepath.Clean(sourcePath), filepath.Clean(targetPath)) {
		return AddonListMergePreview{}, fmt.Errorf("来源 addonlist.txt 就是当前目标文件，无需融合")
	}

	sourceDoc, err := readAddonListDocumentAtPath(sourcePath)
	if err != nil {
		return AddonListMergePreview{}, fmt.Errorf("无法读取来源 addonlist.txt: %w", err)
	}

	currentItems := make([]AddonListItem, 0)
	if currentDoc, currentErr := a.readAddonListDocument(); currentErr == nil {
		currentItems = parseAddonListItems(currentDoc.content)
	} else if !errors.Is(currentErr, os.ErrNotExist) {
		return AddonListMergePreview{}, currentErr
	}

	currentByKey := addonListItemsByKey(currentItems)
	sourceByKey, sourceOrder := addonListItemsByKeyInOrder(parseAddonListItems(sourceDoc.content))
	preview := AddonListMergePreview{
		SourcePath: sourcePath,
		TargetPath: targetPath,
		Added:      make([]AddonListItem, 0),
		Conflicts:  make([]AddonListMergeConflict, 0),
	}
	for _, key := range sourceOrder {
		sourceItem := sourceByKey[key]
		currentItem, exists := currentByKey[key]
		if !exists {
			preview.Added = append(preview.Added, sourceItem)
			continue
		}
		if currentItem.Value != sourceItem.Value {
			preview.Conflicts = append(preview.Conflicts, AddonListMergeConflict{
				Key:            sourceItem.Name,
				CurrentValue:   currentItem.Value,
				SourceValue:    sourceItem.Value,
				CurrentEnabled: currentItem.Value == "1",
				SourceEnabled:  sourceItem.Value == "1",
			})
		}
	}
	return preview, nil
}

func addonListItemsByKey(items []AddonListItem) map[string]AddonListItem {
	byKey := make(map[string]AddonListItem, len(items))
	for _, item := range items {
		byKey[normalizeAddonListKey(item.Name)] = item
	}
	return byKey
}

func addonListItemsByKeyInOrder(items []AddonListItem) (map[string]AddonListItem, []string) {
	byKey := make(map[string]AddonListItem, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		key := normalizeAddonListKey(item.Name)
		if _, exists := byKey[key]; !exists {
			order = append(order, key)
		}
		byKey[key] = item
	}
	return byKey, order
}
