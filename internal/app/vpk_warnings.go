package app

import (
	"fmt"
	"strings"

	"vpk-manager/internal/parser"
)

// VPKOperationWarning describes a non-blocking issue that may affect how the
// game records or loads a VPK. The operation itself remains available.
type VPKOperationWarning struct {
	HasWarning bool   `json:"hasWarning"`
	Summary    string `json:"summary"`
	Detail     string `json:"detail"`
	Repairable bool   `json:"repairable"`
}

// GetVPKOperationWarning checks risks relevant to state-changing operations.
// It deliberately returns validation failures as warning data instead of an
// error so callers can let the user continue when the game may still mount the
// archive successfully.
func (a *App) GetVPKOperationWarning(filePath string) (VPKOperationWarning, error) {
	cached, ok := a.vpkCache.Load(filePath)
	if !ok {
		return VPKOperationWarning{}, fmt.Errorf("文件未找到: %s", filePath)
	}
	file := cached.(*VPKFileCache).File
	report, inspectErr := parser.InspectVPKIntegrity(file.Path)
	if inspectErr != nil {
		return VPKOperationWarning{
			HasWarning: true,
			Summary:    "无法完成 VPK 完整性检测",
			Detail:     inspectErr.Error(),
		}, nil
	}
	if report.Valid {
		return VPKOperationWarning{}, nil
	}

	details := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		message := strings.TrimSpace(issue.Message)
		if message == "" {
			continue
		}
		if issue.Path != "" {
			message = issue.Path + ": " + message
		}
		details = append(details, message)
	}
	if len(details) == 0 {
		details = append(details, "完整性检测未通过，但未提供具体问题")
	}

	return VPKOperationWarning{
		HasWarning: true,
		Summary:    "此 Mod 存在 VPK 完整性风险",
		Detail:     strings.Join(details, "；"),
		Repairable: report.Repairable,
	}, nil
}
