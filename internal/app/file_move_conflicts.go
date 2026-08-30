package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	moveConflictActionReplace = "replace"
	moveConflictActionSkip    = "skip"
	moveConflictActionCancel  = "cancel"
)

var (
	errMoveSkipped   = errors.New("移动已跳过")
	errMoveCancelled = errors.New("移动已取消")
)

// FileMoveConflict 描述一个待移动文件与目标文件之间的冲突。
// Source/Target 的大小和修改时间用于在前端显示“比较文件信息”。
type FileMoveConflict struct {
	SourcePath    string `json:"sourcePath"`
	TargetPath    string `json:"targetPath"`
	FileType      string `json:"fileType"`
	SourceSize    int64  `json:"sourceSize"`
	TargetSize    int64  `json:"targetSize"`
	SourceModTime string `json:"sourceModTime"`
	TargetModTime string `json:"targetModTime"`
}

type moveFileCandidate struct {
	source   string
	target   string
	fileType string
}

func normalizeMoveConflictAction(action string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" || action == moveConflictActionReplace || action == moveConflictActionSkip || action == moveConflictActionCancel {
		return action, nil
	}
	return "", fmt.Errorf("不支持的文件冲突处理方式: %s", action)
}

func moveCandidatesForPath(srcPath, destDir string) []moveFileCandidate {
	baseName := filepath.Base(srcPath)
	candidates := []moveFileCandidate{{
		source:   srcPath,
		target:   filepath.Join(destDir, baseName),
		fileType: "主文件",
	}}

	basePath := strings.TrimSuffix(srcPath, filepath.Ext(srcPath))
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".bmp", ".gif", ".meta"} {
		sidecar := basePath + ext
		if _, err := os.Stat(sidecar); err == nil {
			candidates = append(candidates, moveFileCandidate{
				source:   sidecar,
				target:   filepath.Join(destDir, filepath.Base(sidecar)),
				fileType: "伴随文件",
			})
		}
	}
	return candidates
}

func fileMoveConflictFromCandidate(candidate moveFileCandidate) (FileMoveConflict, error) {
	sourceInfo, err := os.Stat(candidate.source)
	if err != nil {
		return FileMoveConflict{}, err
	}
	targetInfo, err := os.Stat(candidate.target)
	if err != nil {
		return FileMoveConflict{}, err
	}
	return FileMoveConflict{
		SourcePath:    candidate.source,
		TargetPath:    candidate.target,
		FileType:      candidate.fileType,
		SourceSize:    sourceInfo.Size(),
		TargetSize:    targetInfo.Size(),
		SourceModTime: sourceInfo.ModTime().Format(time.RFC3339Nano),
		TargetModTime: targetInfo.ModTime().Format(time.RFC3339Nano),
	}, nil
}

// CheckFileMoveConflicts 只检查目标目录，不执行移动或复制。
func (a *App) CheckFileMoveConflicts(filePaths []string, destDir string) ([]FileMoveConflict, error) {
	destDir = filepath.Clean(strings.TrimSpace(destDir))
	if destDir == "" || destDir == "." {
		return nil, fmt.Errorf("目标目录不能为空")
	}
	if info, err := os.Stat(destDir); err == nil && !info.IsDir() {
		return nil, fmt.Errorf("目标路径不是目录: %s", destDir)
	}

	conflicts := make([]FileMoveConflict, 0)
	for _, srcPath := range filePaths {
		if strings.TrimSpace(srcPath) == "" {
			continue
		}
		if _, err := os.Stat(srcPath); err != nil {
			return nil, fmt.Errorf("源文件不存在或无法访问: %s: %w", srcPath, err)
		}
		for _, candidate := range moveCandidatesForPath(srcPath, destDir) {
			if filepath.Clean(candidate.source) == filepath.Clean(candidate.target) {
				continue
			}
			if _, err := os.Stat(candidate.target); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("无法检查目标文件 %s: %w", candidate.target, err)
			}
			conflict, err := fileMoveConflictFromCandidate(candidate)
			if err != nil {
				return nil, fmt.Errorf("无法读取冲突文件信息: %w", err)
			}
			conflicts = append(conflicts, conflict)
		}
	}
	return conflicts, nil
}

func copyFileOverwrite(srcPath, destPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	dest, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(dest, src); err != nil {
		_ = dest.Close()
		_ = os.Remove(destPath)
		return err
	}
	if err := dest.Close(); err != nil {
		_ = os.Remove(destPath)
		return err
	}
	return nil
}

func copyAndDeleteOverwrite(srcPath, destPath string) error {
	if err := copyFileOverwrite(srcPath, destPath); err != nil {
		return err
	}
	return os.Remove(srcPath)
}

func moveFileWithConflictAction(srcPath, destPath, action string) error {
	if filepath.Clean(srcPath) == filepath.Clean(destPath) {
		return nil
	}
	if _, err := os.Stat(destPath); err == nil {
		switch action {
		case moveConflictActionReplace:
			return copyAndDeleteOverwrite(srcPath, destPath)
		case moveConflictActionSkip:
			return errMoveSkipped
		case moveConflictActionCancel:
			return errMoveCancelled
		default:
			return fmt.Errorf("目标文件已存在: %s", destPath)
		}
	}
	return moveFile(srcPath, destPath)
}

func copyRegularFileWithConflictAction(srcPath, destPath, action string) error {
	if filepath.Clean(srcPath) == filepath.Clean(destPath) {
		return nil
	}
	if _, err := os.Stat(destPath); err == nil {
		switch action {
		case moveConflictActionReplace:
			return copyFileOverwrite(srcPath, destPath)
		case moveConflictActionSkip:
			return errMoveSkipped
		case moveConflictActionCancel:
			return errMoveCancelled
		default:
			return fmt.Errorf("目标文件已存在: %s", destPath)
		}
	}
	return copyRegularFile(srcPath, destPath)
}

func copyWorkshopSidecarsWithConflictAction(srcPath, destPath, action string) error {
	srcBase := strings.TrimSuffix(srcPath, filepath.Ext(srcPath))
	destBase := strings.TrimSuffix(destPath, filepath.Ext(destPath))
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".bmp", ".gif", ".meta"} {
		sidecar := srcBase + ext
		if _, err := os.Stat(sidecar); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := copyRegularFileWithConflictAction(sidecar, destBase+ext, action); err != nil {
			if errors.Is(err, errMoveSkipped) {
				logMoveSidecarSkip(destBase + ext)
				continue
			}
			return err
		}
	}
	return nil
}

// MoveVpkFilesWithConflictAction 按指定策略移动文件。action 为空表示不覆盖，
// replace 覆盖，skip 跳过，cancel 立即停止后续文件。
func (a *App) MoveVpkFilesWithConflictAction(filePaths []string, destDir, action string) (MoveResult, error) {
	result := MoveResult{}
	var err error
	if action, err = normalizeMoveConflictAction(action); err != nil {
		return result, err
	}
	destDir = filepath.Clean(strings.TrimSpace(destDir))
	if destDir == "" || destDir == "." {
		return result, fmt.Errorf("目标目录不能为空")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return result, fmt.Errorf("无法创建目标目录: %w", err)
	}

	for _, srcPath := range filePaths {
		candidates := moveCandidatesForPath(srcPath, destDir)
		if len(candidates) == 0 {
			continue
		}
		if _, err := os.Stat(candidates[0].source); err != nil {
			result.FailCount++
			result.Errors = append(result.Errors, fmt.Sprintf("移动 %s 失败: %v", filepath.Base(srcPath), err))
			continue
		}

		err := moveFileWithConflictAction(candidates[0].source, candidates[0].target, action)
		if errors.Is(err, errMoveCancelled) {
			result.Cancelled = true
			return result, nil
		}
		if errors.Is(err, errMoveSkipped) {
			result.SkippedCount++
			continue
		}
		if err != nil {
			result.FailCount++
			result.Errors = append(result.Errors, fmt.Sprintf("移动 %s 失败: %v", filepath.Base(srcPath), err))
			continue
		}

		for _, candidate := range candidates[1:] {
			if err := moveFileWithConflictAction(candidate.source, candidate.target, action); err != nil {
				if errors.Is(err, errMoveCancelled) {
					result.Cancelled = true
					return result, nil
				}
				if errors.Is(err, errMoveSkipped) {
					logMoveSidecarSkip(candidate.target)
					continue
				}
				logMoveSidecarFailure(candidate.target, err)
			}
		}
		result.SuccessCount++
	}
	return result, nil
}

func logMoveSidecarSkip(path string) {
	// 保持批量移动的主文件结果可用；伴随文件的跳过信息写入日志即可。
	log.Printf("跳过已存在的伴随文件: %s", path)
}

func logMoveSidecarFailure(path string, err error) {
	// 伴随文件不是主文件移动失败，不阻断剩余 Mod。
	log.Printf("移动伴随文件失败: %s: %v", path, err)
}
