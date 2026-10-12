package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vpk-manager/internal/parser"
)

// 批量规范化命名（路线图第 20 项 P2）：
//
//   - PlanNamingNormalization —— 只读计划器：对所选文件逐个推演建议（复用 P1 的 S1–S5 引擎
//     与 63B 截断器），组装「旧名 → 新名」对照与跳过明细（工坊 / 未变化 / 无法压缩 /
//     目标冲突 / 超长 / 读取失败）；幂等：规范库重跑全部落在「未变化」。
//   - ApplyNamingNormalization —— 执行器：执行前先把「撤销清单」写入配置目录
//     （写不出去就整体中止、不碰任何文件），再逐项走 RenameVPKFile 全链路
//     （标签保留 / 侧车联动 / addonlist 键迁移 / 缓存更新 / 冲突复检失效），
//     逐项汇报结果并推送 naming_batch_progress 进度事件。
//
// 口径纪律（对齐 addonlist「写前差异确认」）：预览与写入使用同一份后端结果 ——
// 前端把预览里逐字展示的 newName 原样回传执行，执行阶段不再重新推演建议，
// 保证「看到什么就写什么」；执行前的二次校验只做拒绝（文件已变化 / 目标已存在），
// 不会替换成另一份命名。
//
// 账号与红线无关；本功能只操作本地受管 Mod 目录里非工坊位置的 .vpk 文件。

// NamingBatchItem 是批量规范化计划里的单项。
type NamingBatchItem struct {
	Path      string `json:"path"`
	OldName   string `json:"oldName"`
	NewName   string `json:"newName,omitempty"` // 完整新文件名（含 [标签]/_/.vpk；预览与执行同口径）
	Stage     string `json:"stage,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Subject   string `json:"subject,omitempty"`
	ReasonKey string `json:"reasonKey,omitempty"`
	// ReasonArgs 是依据模板参数：数值已格式化；domain / kind 为原文词，由界面本地化。
	ReasonArgs        map[string]string `json:"reasonArgs,omitempty"`
	AlreadyNormalized bool              `json:"alreadyNormalized,omitempty"`
	PendingReview     bool              `json:"pendingReview,omitempty"`
	Truncated         bool              `json:"truncated,omitempty"`
	Fits              bool              `json:"fits"`
	Changed           bool              `json:"changed"`
	Actionable        bool              `json:"actionable"` // 可被勾选执行（含待复核项；低置信项默认不勾选）
	Location          string            `json:"location,omitempty"`
	SkipReason        string            `json:"skipReason,omitempty"` // 机读代码：workshop / unchanged / unfittable / noStem / collision / tooLong / invalid / readError
	SkipDetail        string            `json:"skipDetail,omitempty"` // 原始细节（错误文本等；界面作为补充展示）
}

// NamingBatchPlan 是批量规范化计划（只读生成，供界面预览与勾选）。
type NamingBatchPlan struct {
	Items     []NamingBatchItem `json:"items"`
	Total     int               `json:"total"`
	Renamable int               `json:"renamable"` // 可改名且非低置信（默认勾选）
	Pending   int               `json:"pending"`   // 可改名但低置信（待复核，默认不勾选）
	Skipped   int               `json:"skipped"`
}

// NamingBatchRenameRequest 是执行请求的单项（前端回传预览中的 path / oldName / newName）。
type NamingBatchRenameRequest struct {
	Path    string `json:"path"`
	OldName string `json:"oldName,omitempty"`
	NewName string `json:"newName"`
}

// NamingBatchApplyItem 是执行结果的单项。
type NamingBatchApplyItem struct {
	Path    string `json:"path"`
	OldName string `json:"oldName,omitempty"`
	NewName string `json:"newName"`
	NewPath string `json:"newPath,omitempty"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// NamingBatchApplyResult 是批量执行结果（含撤销清单落盘路径）。
type NamingBatchApplyResult struct {
	BackupPath string                 `json:"backupPath,omitempty"`
	Total      int                    `json:"total"`
	Success    int                    `json:"success"`
	Failed     int                    `json:"failed"`
	Items      []NamingBatchApplyItem `json:"items"`
}

// NamingBatchProgress 是执行过程中的进度事件负载（事件名 naming_batch_progress）。
type NamingBatchProgress struct {
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Path  string `json:"path,omitempty"`
	OK    bool   `json:"ok"`
}

// PlanNamingNormalization 为所选文件生成批量规范化计划（只读：只解目录索引，不写盘）。
//
// 跳过规则（逐项返回原因，不静默丢弃）：工坊位置（Steam 需要原始文件名）、
// 未变化（已符合规范）、前缀放不进 63B、无法生成名字、目标名已存在或与同批项冲突、
// 超长路径、非受管文件 / 读取失败。
func (a *App) PlanNamingNormalization(paths []string) (NamingBatchPlan, error) {
	plan := NamingBatchPlan{Items: make([]NamingBatchItem, 0, len(paths))}
	seen := make(map[string]bool, len(paths))
	for _, raw := range paths {
		path := filepath.Clean(strings.TrimSpace(raw))
		if path == "" || path == "." || path == string(filepath.Separator) {
			continue
		}
		key := strings.ToLower(path)
		if seen[key] {
			continue
		}
		seen[key] = true
		plan.Items = append(plan.Items, a.planNamingItem(path))
	}
	if len(plan.Items) == 0 {
		return NamingBatchPlan{}, errors.New("未选择任何文件")
	}

	// 目标冲突检测（Windows 文件系统大小写不敏感）：
	//   - 目标文件已存在 → 跳过（不让执行阶段才失败）；
	//   - 两个项目的目标相同 → 两个都跳过（保守，避免先到先得)。
	targetOwner := make(map[string]int, len(plan.Items))
	for index := range plan.Items {
		item := &plan.Items[index]
		if !item.Actionable {
			continue
		}
		newPath := filepath.Join(filepath.Dir(item.Path), item.NewName)
		key := strings.ToLower(newPath)
		if _, err := os.Stat(newPath); err == nil {
			item.Actionable = false
			item.SkipReason = "collision"
			item.SkipDetail = item.NewName
			continue
		}
		if owner, ok := targetOwner[key]; ok {
			plan.Items[owner].Actionable = false
			plan.Items[owner].SkipReason = "collision"
			plan.Items[owner].SkipDetail = plan.Items[owner].NewName
			item.Actionable = false
			item.SkipReason = "collision"
			item.SkipDetail = item.NewName
			continue
		}
		targetOwner[key] = index
	}

	for index := range plan.Items {
		item := &plan.Items[index]
		switch {
		case item.Actionable && item.PendingReview:
			plan.Pending++
		case item.Actionable:
			plan.Renamable++
		default:
			plan.Skipped++
		}
	}
	plan.Total = len(plan.Items)
	return plan, nil
}

// planNamingItem 为单个文件生成计划项（复核校验 → 推演建议 → 组装新名）。
func (a *App) planNamingItem(path string) NamingBatchItem {
	item := NamingBatchItem{Path: path, OldName: filepath.Base(path), Fits: true}
	if err := a.validateNamingRenameTarget(path); err != nil {
		item.SkipReason = "invalid"
		item.SkipDetail = err.Error()
		return item
	}
	item.Location = a.getLocationFromPath(path)
	if item.Location == "workshop" {
		item.SkipReason = "workshop"
		return item
	}
	suggestion, err := parser.SuggestNameForFile(path)
	if err != nil {
		item.SkipReason = "readError"
		item.SkipDetail = err.Error()
		return item
	}
	item.Stage = suggestion.Stage
	item.Prefix = suggestion.Prefix
	item.Subject = suggestion.Subject
	item.ReasonKey = suggestion.ReasonKey
	item.ReasonArgs = suggestion.ReasonArgs
	item.AlreadyNormalized = suggestion.AlreadyNormalized
	item.PendingReview = suggestion.PendingReview
	item.Truncated = suggestion.Truncated
	item.Fits = suggestion.Fits
	item.Changed = suggestion.Changed
	if !suggestion.Changed {
		item.SkipReason = "unchanged"
		return item
	}
	if !suggestion.Fits {
		item.SkipReason = "unfittable"
		return item
	}
	if strings.TrimSpace(suggestion.Stem) == "" {
		item.SkipReason = "noStem"
		return item
	}
	newName := parser.ComposeNormalizedFilename(item.OldName, suggestion.Stem)
	if newName == item.OldName {
		item.SkipReason = "unchanged"
		return item
	}
	// Windows 路径长度上限（对齐 RenameVPKFile 的校验口径；路径过长在执行阶段必失败）。
	newPath := filepath.Join(filepath.Dir(path), newName)
	if windowsPathLength(newPath) > windowsMaxPathLength || windowsPathLength(newName) > windowsMaxFilenameLength {
		item.SkipReason = "tooLong"
		return item
	}
	item.NewName = newName
	item.Actionable = true
	return item
}

// validateNamingRenameTarget 校验路径是受管 Mod 目录里可改名的 .vpk 文件。
func (a *App) validateNamingRenameTarget(filePath string) error {
	root := strings.TrimSpace(a.rootDirectorySnapshot())
	if root == "" {
		return errors.New("Mod 目录未设置")
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("文件不在受管 Mod 目录内")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("找不到文件：%v", err)
	}
	if info.IsDir() {
		return errors.New("目标不是文件")
	}
	if !strings.EqualFold(filepath.Ext(filePath), ".vpk") {
		return errors.New("不是 .vpk 文件")
	}
	return nil
}

// ApplyNamingNormalization 执行批量规范化：先写撤销清单（失败即整体中止），再逐项重命名。
//
// 执行前对每一项做二次校验（文件仍存在 / 当前名与计划一致 / 目标不存在），
// 校验失败只拒绝该项，不替换命名口径；成功与失败都在结果里逐项汇报。
func (a *App) ApplyNamingNormalization(requests []NamingBatchRenameRequest) (NamingBatchApplyResult, error) {
	result := NamingBatchApplyResult{Items: make([]NamingBatchApplyItem, len(requests))}
	// 沙箱只读闸门：批量改名会改真实库里的文件名。
	if err := rejectReadonlyLibraryWrite("批量规范化命名"); err != nil {
		return result, err
	}
	if len(requests) == 0 {
		return result, errors.New("没有要执行的项目")
	}
	if !a.namingBatchMu.TryLock() {
		return result, errors.New("已有批量命名任务在执行，请稍后再试")
	}
	defer a.namingBatchMu.Unlock()

	// 第一阶段：全量二次校验（失败项记录错误、不进写清单）。
	// 重复路径先整组标记：同一路径出现多次时全部拒绝，避免「先到的执行、后到的报错」这种半执行。
	pathCount := make(map[string]int, len(requests))
	for _, raw := range requests {
		key := strings.ToLower(strings.TrimSpace(raw.Path))
		if key != "" {
			pathCount[key]++
		}
	}
	validIndexes := make([]int, 0, len(requests))
	validRequests := make([]NamingBatchRenameRequest, 0, len(requests))
	for index, raw := range requests {
		item := NamingBatchApplyItem{
			Path:    strings.TrimSpace(raw.Path),
			OldName: strings.TrimSpace(raw.OldName),
			NewName: strings.TrimSpace(raw.NewName),
		}
		switch {
		case item.Path == "" || item.NewName == "":
			item.Error = "存在无效的重命名项目（路径或新名字为空）"
		case pathCount[strings.ToLower(item.Path)] > 1:
			item.Error = "重复的项目（同一路径出现多次）"
		default:
			if err := a.validateNamingRenameTarget(item.Path); err != nil {
				item.Error = err.Error()
				break
			}
			if a.getLocationFromPath(item.Path) == "workshop" {
				item.Error = "工坊文件不改名（Steam 需要原始文件名）"
				break
			}
			if item.OldName != "" && !strings.EqualFold(filepath.Base(item.Path), item.OldName) {
				item.Error = fmt.Sprintf("文件已变化（当前为 %s），请重新生成计划", filepath.Base(item.Path))
				break
			}
			if strings.EqualFold(filepath.Base(item.Path), item.NewName) {
				item.Error = "新名与原文件名相同"
				break
			}
			newPath := filepath.Join(filepath.Dir(item.Path), item.NewName)
			if _, statErr := os.Stat(newPath); statErr == nil {
				item.Error = fmt.Sprintf("目标文件已存在: %s", item.NewName)
				break
			}
			validIndexes = append(validIndexes, index)
			validRequests = append(validRequests, NamingBatchRenameRequest{
				Path:    item.Path,
				OldName: item.OldName,
				NewName: item.NewName,
			})
			continue
		}
		result.Items[index] = item
	}

	if len(validRequests) == 0 {
		a.finalizeNamingBatchResult(&result)
		return result, nil
	}

	// 第二阶段：撤销清单先落盘（写不出去就不碰任何文件）。
	backupPath, err := a.writeNamingRenameBackup(validRequests)
	if err != nil {
		return NamingBatchApplyResult{}, fmt.Errorf("写入撤销清单失败，已中止批量命名：%w", err)
	}
	result.BackupPath = backupPath

	// 第三阶段：逐项执行（复用重命名全链路）并推送进度。
	total := len(validRequests)
	for done, index := range validIndexes {
		request := validRequests[done]
		item := result.Items[index]
		newPath, renameErr := a.RenameVPKFile(request.Path, request.NewName)
		if renameErr != nil {
			item.Error = renameErr.Error()
			item.OK = false
		} else {
			item.NewPath = newPath
			item.OK = true
		}
		result.Items[index] = item
		a.emitEvent("naming_batch_progress", NamingBatchProgress{
			Done:  done + 1,
			Total: total,
			Path:  request.Path,
			OK:    item.OK,
		})
	}

	a.finalizeNamingBatchResult(&result)
	return result, nil
}

// finalizeNamingBatchResult 汇总成功 / 失败计数。
func (a *App) finalizeNamingBatchResult(result *NamingBatchApplyResult) {
	result.Total = len(result.Items)
	result.Success = 0
	result.Failed = 0
	for _, item := range result.Items {
		if item.OK {
			result.Success++
		} else {
			result.Failed++
		}
	}
}

// namingRenameBackupPayload 是批量改名的撤销清单文件结构（写入配置目录，不属于仓库数据）。
type namingRenameBackupPayload struct {
	Version   int                      `json:"version"`
	Kind      string                   `json:"kind"`
	CreatedAt string                   `json:"createdAt"`
	Count     int                      `json:"count"`
	Items     []namingRenameBackupItem `json:"items"`
}

type namingRenameBackupItem struct {
	Path    string `json:"path"`
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

// writeNamingRenameBackup 在执行批量改名之前导出撤销清单（旧路径 ↔ 新名，便于对照回滚）。
func (a *App) writeNamingRenameBackup(requests []NamingBatchRenameRequest) (string, error) {
	a.ensureConfigPaths()
	configDir := strings.TrimSpace(a.configDir)
	if configDir == "" {
		return "", errors.New("配置目录不可用")
	}

	payload := namingRenameBackupPayload{
		Version:   1,
		Kind:      "lytvpk-naming-rename-backup",
		CreatedAt: time.Now().Format(time.RFC3339),
		Count:     len(requests),
		Items:     make([]namingRenameBackupItem, 0, len(requests)),
	}
	for _, request := range requests {
		payload.Items = append(payload.Items, namingRenameBackupItem{
			Path:    request.Path,
			OldName: request.OldName,
			NewName: request.NewName,
		})
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')

	fileName := "naming_rename_backup_" + time.Now().Format("20060102_150405") + ".json"
	path := filepath.Join(configDir, fileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
