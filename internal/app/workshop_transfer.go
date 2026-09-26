package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

)

func (a *App) processChunkedDownload(ctx context.Context, task *DownloadTask, downloadUrl string, bestIP string, totalSize int64, workerCount int, tempDir string) (string, error) {
	const blockSize = int64(5 * 1024 * 1024)

	finalPath := filepath.Join(tempDir, task.ID+"_final")
	checkpointPath := downloadBlockCheckpointPath(finalPath)

	// 1. 能续传就续传（临时文件大小必须完全一致 + 检查点有效），否则重新预分配。
	var resumed []int
	var file *os.File
	var err error
	if checkpoint := reusablePartialDownload(finalPath, task.ID, totalSize, blockSize); checkpoint != nil {
		resumed = validateBlockCheckpoint(checkpoint, task.ID, totalSize, blockSize)
		file, err = os.OpenFile(finalPath, os.O_RDWR, 0o644)
		if err != nil {
			// 打不开就退回重下，不把这次失败变成"下不了"。
			resumed = nil
			removeDownloadCheckpointFiles(finalPath)
			file, err = createPreallocatedFile(finalPath, totalSize)
		}
	} else {
		// 有残留但不可用（换了任务 / 文件被截断 / 检查点损坏）：清干净再下。
		removeDownloadCheckpointFiles(finalPath)
		file, err = createPreallocatedFile(finalPath, totalSize)
	}
	if err != nil {
		return "", err
	}

	// 2. Create BlockManager（带续传区块）
	bm := newBlockManagerWithResume(totalSize, workerCount, blockSize, resumed)

	// 2.1 检查点：每完成一块写一次，但最多每秒一次；暂停 / 失败 / 收尾时强制补写。
	var checkpointMu sync.Mutex
	lastCheckpoint := time.Time{}
	writeCheckpoint := func(force bool) {
		checkpointMu.Lock()
		defer checkpointMu.Unlock()
		if !force && !lastCheckpoint.IsZero() && time.Since(lastCheckpoint) < time.Second {
			return
		}
		lastCheckpoint = time.Now()
		if err := saveBlockCheckpoint(checkpointPath, task.ID, totalSize, blockSize, bm.CompletedIndices()); err != nil {
			fmt.Printf("[ChunkedDownload] 写检查点失败（不影响本次下载）: %v\n", err)
		}
	}
	bm.onBlockCompleted = func() { writeCheckpoint(false) }

	// 3. Link external context cancellation to BlockManager
	go func() {
		<-ctx.Done()
		bm.cancel()
	}()

	fmt.Printf("[ChunkedDownload] Starting dynamic %d-worker download for %s (Size: %.2f MB, Blocks: %d, Resumed: %d)\n",
		workerCount, task.Filename, float64(totalSize)/1024/1024, len(bm.blocks), len(resumed))

	// 4. Start progress reporter
	stopReporter := make(chan struct{})
	go a.progressReporter(bm, task, stopReporter)

	// 5. Start workers
	var wg sync.WaitGroup
	for i := range workerCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			a.downloadWorker(id, bm, file, downloadUrl, bestIP)
		}(i)
	}

	// 6. Wait for all workers to finish
	wg.Wait()
	close(stopReporter)

	// 7. Close file
	file.Close()

	// 8. Check for cancellation：区分"用户暂停"与"取消/其它错误"。
	if ctx.Err() != nil {
		if downloadTaskIsPaused(task.ID) {
			// 暂停：保留临时文件 + 检查点，下次继续只补缺的区块。
			writeCheckpoint(true)
			return "", errDownloadPaused
		}
		removeDownloadCheckpointFiles(finalPath)
		return "", ctx.Err()
	}

	// 9. Check for fatal errors：留下已完成区块，失败后的「重试」直接从断点继续。
	if fatalErr := bm.HasFatalError(); fatalErr != nil {
		writeCheckpoint(true)
		return "", fatalErr
	}

	// 10. Verify final file size
	stat, err := os.Stat(finalPath)
	if err != nil || stat.Size() != totalSize {
		removeDownloadCheckpointFiles(finalPath)
		return "", fmt.Errorf("final size mismatch: expected %d, got %d", totalSize, stat.Size())
	}

	// 11. 收尾：文件已完整，检查点不再需要（临时文件由调用方改名）。
	_ = os.Remove(checkpointPath)

	// 12. Emit final progress
	taskManager.mu.Lock()
	task.DownloadedSize = totalSize
	task.Progress = 100
	taskManager.mu.Unlock()
	a.emitTaskProgress(task)

	fmt.Printf("[ChunkedDownload] Successfully downloaded %s with dynamic workers\n", task.Filename)

	return finalPath, nil
}

// getFileSize gets file size via HEAD request
func (a *App) getFileSize(ctx context.Context, downloadUrl string, bestIP string) int64 {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if bestIP != "" {
		dialer := &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, _ := net.SplitHostPort(addr)
			u, parseErr := url.Parse(downloadUrl)
			if parseErr == nil && u.Hostname() == host {
				return dialer.DialContext(ctx, network, net.JoinHostPort(bestIP, port))
			}
			return dialer.DialContext(ctx, network, addr)
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", downloadUrl, nil)
	if err != nil {
		return 0
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://steamcommunity.com/")

	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		return resp.ContentLength
	}

	return 0
}

// downloadPreviewImage downloads preview image for the task
func (a *App) downloadPreviewImage(task *DownloadTask, targetPath string) {
	if task.PreviewUrl == "" {
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(task.PreviewUrl)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	// 根据实际内容格式确定扩展名
	imgExt := ".jpg"
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		switch format {
		case "png":
			imgExt = ".png"
		case "gif":
			imgExt = ".gif"
		case "jpeg":
			imgExt = ".jpg"
		}
	}

	vpkExt := filepath.Ext(targetPath)
	imgPath := strings.TrimSuffix(targetPath, vpkExt) + imgExt

	out, err := os.Create(imgPath)
	if err != nil {
		return
	}
	defer out.Close()

	out.Write(data)
}

// handleArchiveExtraction handles auto extraction for archive files
func (a *App) handleArchiveExtraction(task *DownloadTask, targetPath string, updateStatus func(string, string)) {
	ext := strings.ToLower(filepath.Ext(targetPath))
	if strings.HasPrefix(task.WorkshopID, "direct-") && (ext == ".zip" || ext == ".rar" || ext == ".7z") {
		rootDir := a.rootDirectorySnapshot()
		if rootDir == "" {
			return
		}
		updateStatus("downloading", "正在解压...")
		err := a.ExtractVPKFromArchive(targetPath, rootDir)
		if err != nil {
			fmt.Printf("解压压缩包失败: %v\n", err)
		} else {
			if err := os.Remove(targetPath); err != nil {
				fmt.Printf("删除压缩文件失败: %v\n", err)
			} else {
				fmt.Printf("已删除压缩文件: %s\n", targetPath)
			}
		}
	}
}
