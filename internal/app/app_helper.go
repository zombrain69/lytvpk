package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hymkor/trash-go"
)

func copyRegularFile(srcPath, destPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	dest, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(dest, src); err != nil {
		dest.Close()
		_ = os.Remove(destPath)
		return err
	}
	if err := dest.Close(); err != nil {
		_ = os.Remove(destPath)
		return err
	}
	return nil
}

func copySidecarFiles(srcPath, destPath string) error {
	srcExt := filepath.Ext(srcPath)
	srcBase := strings.TrimSuffix(srcPath, srcExt)
	destExt := filepath.Ext(destPath)
	destBase := strings.TrimSuffix(destPath, destExt)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".meta"} {
		srcSidecar := srcBase + ext
		if _, err := os.Stat(srcSidecar); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := copyRegularFile(srcSidecar, destBase+ext); err != nil {
			return err
		}
	}
	return nil
}

// handleSidecarFile 处理伴随文件（如同名图片）的移动/重命名/删除
// op: "move", "delete" (rename is essentially move)
// srcPath: 源文件路径 (VPK路径)
// destPath: 目标文件路径 (VPK路径，delete操作可为空)
func (a *App) handleSidecarFile(srcPath, destPath string, op string) {
	srcExt := filepath.Ext(srcPath)
	srcBase := strings.TrimSuffix(srcPath, srcExt)

	// 可能的图片扩展名和meta扩展名
	exts := []string{".jpg", ".jpeg", ".png", ".gif", ".meta"}

	for _, ext := range exts {
		srcImg := srcBase + ext
		if _, err := os.Stat(srcImg); err == nil {
			// Found sidecar file
			if op == "delete" {
				// 使用 trash 库删除文件到回收站，与 VPK 保持一致
				trash.Throw(srcImg)
				continue
			}

			if destPath == "" {
				continue
			}

			destExt := filepath.Ext(destPath)
			destBase := strings.TrimSuffix(destPath, destExt)
			destImg := destBase + ext

			// 确保目标目录存在（虽然处理 VPK 时应该已经创建了）
			os.MkdirAll(filepath.Dir(destImg), 0755)

			// 移动/重命名
			os.Rename(srcImg, destImg)
		}
	}
}
