package app

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 分块下载的"断点续传检查点"。
//
// 对齐 FireAxe `DownloadService`（`Pause` / `Resume` / `_downloadedBytes`）：
// 暂停时保留已下载的数据与"哪些区块已经完整拿到"的记录，继续时只补缺的区块，
// 而不是把整个文件重下一遍。
//
// 为什么必须记"区块"而不是只记字节数：分块下载是 N 个协程随机写同一文件的
// 不同偏移，字节数达到 X 并不代表 [0,X) 是连续的。只有"整块已完成"才是可信的
// 续传依据 —— 这也是本文件唯一的职责边界。

const downloadBlockCheckpointSchemaVersion = 1

// downloadBlockCheckpoint 是 `<taskID>_final.ckpt.json` 的结构。
type downloadBlockCheckpoint struct {
	SchemaVersion int    `json:"schemaVersion"`
	TaskID        string `json:"taskId"`
	TotalSize     int64  `json:"totalSize"`
	BlockSize     int64  `json:"blockSize"`
	// Completed 是"已经完整写入临时文件"的区块下标（升序、去重）。
	Completed []int  `json:"completed"`
	UpdatedAt string `json:"updatedAt"`
}

func downloadBlockCheckpointPath(finalPath string) string {
	return finalPath + ".ckpt.json"
}

// blockCountFor 计算 totalSize 在给定块大小下会切成多少块（与 BlockManager 一致）。
func blockCountFor(totalSize int64, blockSize int64) int {
	if totalSize <= 0 || blockSize <= 0 {
		return 0
	}
	return int((totalSize + blockSize - 1) / blockSize)
}

// normalizeCompletedBlocks 清洗区块下标：去负、去越界、去重、升序。
func normalizeCompletedBlocks(completed []int, totalSize int64, blockSize int64) []int {
	blocks := blockCountFor(totalSize, blockSize)
	seen := make(map[int]bool, len(completed))
	result := make([]int, 0, len(completed))
	for _, index := range completed {
		if index < 0 || index >= blocks || seen[index] {
			continue
		}
		seen[index] = true
		result = append(result, index)
	}
	sort.Ints(result)
	return result
}

// validateBlockCheckpoint 判定检查点是否可用于续传。
// 任何一项对不上（任务换了、文件被改过、块大小变了）都必须退化成"从头下"，
// 宁可多下一次，也不能把错位的数据拼进最终文件。
func validateBlockCheckpoint(checkpoint *downloadBlockCheckpoint, taskID string, totalSize int64, blockSize int64) []int {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.SchemaVersion != downloadBlockCheckpointSchemaVersion {
		return nil
	}
	if checkpoint.TaskID != "" && checkpoint.TaskID != taskID {
		return nil
	}
	if checkpoint.TotalSize != totalSize || checkpoint.BlockSize != blockSize {
		return nil
	}
	normalized := normalizeCompletedBlocks(checkpoint.Completed, totalSize, blockSize)
	if len(normalized) == 0 {
		return nil
	}
	// 全部块都"完成"但没有走完收尾流程：不可信，当作没下过。
	if len(normalized) >= blockCountFor(totalSize, blockSize) {
		return nil
	}
	return normalized
}

// loadBlockCheckpoint 读检查点；文件不存在或损坏时返回 nil（调用方按"从头下"处理）。
func loadBlockCheckpoint(path string, taskID string, totalSize int64, blockSize int64) *downloadBlockCheckpoint {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var checkpoint downloadBlockCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil
	}
	if len(validateBlockCheckpoint(&checkpoint, taskID, totalSize, blockSize)) == 0 {
		return nil
	}
	return &checkpoint
}

// saveBlockCheckpoint 原子写入检查点（先写临时文件再替换，避免半截 JSON）。
func saveBlockCheckpoint(path string, taskID string, totalSize int64, blockSize int64, completed []int) error {
	if path == "" {
		return errors.New("缺少检查点路径")
	}
	checkpoint := downloadBlockCheckpoint{
		SchemaVersion: downloadBlockCheckpointSchemaVersion,
		TaskID:        taskID,
		TotalSize:     totalSize,
		BlockSize:     blockSize,
		Completed:     normalizeCompletedBlocks(completed, totalSize, blockSize),
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomically(path, data)
}

// removeDownloadCheckpointFiles 清掉临时文件与检查点（取消下载 / 下载成功收尾时调用）。
func removeDownloadCheckpointFiles(finalPath string) {
	if finalPath == "" {
		return
	}
	_ = os.Remove(finalPath)
	_ = os.Remove(downloadBlockCheckpointPath(finalPath))
}

// reusablePartialDownload 判断"临时文件 + 检查点"是否还能续传：
// 文件必须存在且大小与本次要下的完全一致，否则整组丢弃。
func reusablePartialDownload(finalPath string, taskID string, totalSize int64, blockSize int64) *downloadBlockCheckpoint {
	stat, err := os.Stat(finalPath)
	if err != nil || stat.Size() != totalSize {
		return nil
	}
	return loadBlockCheckpoint(downloadBlockCheckpointPath(finalPath), taskID, totalSize, blockSize)
}

// staleDownloadTempMaxAge 是临时下载残留的保留时长：超过它的 `*_final` / `*_final.ckpt.json`
// 会在启动时被清掉。留 7 天是为了让"失败后过几天再点重试"仍然能续传。
const staleDownloadTempMaxAge = 7 * 24 * time.Hour

// cleanupStaleDownloadTempFiles 清理 temp 目录里过期的分块下载残留（临时文件 + 检查点）。
// 只认本模块的命名，并且只删超过 staleDownloadTempMaxAge 的，避免误删正在进行的下载。
func (a *App) cleanupStaleDownloadTempFiles() int {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return 0
	}
	tempDir := filepath.Join(rootDir, "temp")
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return 0
	}

	now := time.Now()
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, "_final") && !strings.HasSuffix(name, "_final.ckpt.json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) < staleDownloadTempMaxAge {
			continue
		}
		if err := os.Remove(filepath.Join(tempDir, name)); err == nil {
			removed++
		}
	}
	if removed > 0 {
		log.Printf("已清理过期的下载残留文件：%d 个", removed)
	}
	return removed
}
