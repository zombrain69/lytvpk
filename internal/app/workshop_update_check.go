package app

import (
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// UpdateCheckResult 更新检测结果
type UpdateCheckResult struct {
	TotalUpdates int `json:"total_updates"` // 总计需要更新的Mod数
	NewDetected  int `json:"new_detected"`  // 本次新检测到的更新数
	// SkippedStale 记录"检测期间目标被移动 / 替换"而主动放弃写入的数量
	// （对齐 FireAxe ValidTaskCreator：目标失效就不落地结果）。
	SkippedStale int `json:"skipped_stale"`
}

// workshopItemDetailFetcher 取单个工坊作品详情；测试注入假实现以不触网。
type workshopItemDetailFetcher func(id string) (WorkshopItemDetail, error)

// fetchWorkshopItemDetail 是更新检测取详情的唯一入口：
// 生产走真实接口 FetchWorkshopDetail，测试可注入 workshopItemDetailFetcher。
func (a *App) fetchWorkshopItemDetail(id string) (WorkshopItemDetail, error) {
	if a.workshopItemDetailFetcher != nil {
		return a.workshopItemDetailFetcher(id)
	}
	return a.FetchWorkshopDetail(id)
}

// modUpdateCheckRunner 让"检查更新"的扫描逻辑可注入，测试用它观察去重行为。
var modUpdateCheckRunner = func(a *App) UpdateCheckResult {
	return a.runModUpdateCheck()
}

// modUpdateCheckState 对齐 FireAxe WorkshopVpkAddon.cs:428-450 的做法：
// 把进行中的检查任务存起来，重复触发直接复用同一次任务，而不是并行跑两遍
// （并发跑会成倍消耗工坊接口配额，也容易触发限流）。
type modUpdateCheckState struct {
	mu      sync.Mutex
	running bool
	done    chan struct{}
	result  UpdateCheckResult
}

var modUpdateCheck modUpdateCheckState

func resetModUpdateCheckState() {
	modUpdateCheck.mu.Lock()
	modUpdateCheck.running = false
	modUpdateCheck.done = nil
	modUpdateCheck.result = UpdateCheckResult{}
	modUpdateCheck.mu.Unlock()
}

// beginModUpdateCheck 返回 (done, true) 表示本次调用拿到了执行权；
// 返回 (done, false) 表示已有同一次检查在跑，等 done 关闭后复用结果。
func beginModUpdateCheck() (chan struct{}, bool) {
	modUpdateCheck.mu.Lock()
	defer modUpdateCheck.mu.Unlock()
	if modUpdateCheck.running {
		return modUpdateCheck.done, false
	}
	modUpdateCheck.running = true
	modUpdateCheck.done = make(chan struct{})
	return modUpdateCheck.done, true
}

func finishModUpdateCheck(result UpdateCheckResult) {
	modUpdateCheck.mu.Lock()
	modUpdateCheck.running = false
	modUpdateCheck.result = result
	done := modUpdateCheck.done
	modUpdateCheck.mu.Unlock()
	if done != nil {
		close(done)
	}
}

func lastModUpdateCheckResult() UpdateCheckResult {
	modUpdateCheck.mu.Lock()
	defer modUpdateCheck.mu.Unlock()
	return modUpdateCheck.result
}

// CheckModUpdates 检测所有含有meta数据的mod是否有更新。
// 同一时间只跑一次；重复调用会等待并复用正在飞行中的那次结果。
func (a *App) CheckModUpdates() (result UpdateCheckResult) {
	if !a.workshopUpdateCheckEnabled || !a.workshopMetaEnabled {
		return UpdateCheckResult{}
	}

	done, started := beginModUpdateCheck()
	if !started {
		if done != nil {
			<-done
		}
		return lastModUpdateCheckResult()
	}
	// 无论正常结束还是 panic，都要释放执行权，避免后续检查全部卡在等待上。
	defer func() { finishModUpdateCheck(result) }()

	result = modUpdateCheckRunner(a)
	return result
}

// runModUpdateCheck 是真正的扫描逻辑。
func (a *App) runModUpdateCheck() UpdateCheckResult {
	if !a.workshopUpdateCheckEnabled || !a.workshopMetaEnabled {
		return UpdateCheckResult{}
	}

	log.Println("开始检测Mod更新...")

	var confirmedCount int
	type updateCheckItem struct {
		filePath     string
		workshopID   string
		downloadedAt time.Time
		target       modTaskTarget
	}
	var toCheck []updateCheckItem

	a.vpkCache.Range(func(key, value interface{}) bool {
		cache := value.(*VPKFileCache)
		vpkFile := cache.File

		if vpkFile.WorkshopID == "" || strings.HasPrefix(vpkFile.WorkshopID, "direct-") {
			return true
		}

		meta, err := LoadWorkshopMeta(vpkFile.Path)
		if meta == nil || err != nil || meta.DownloadedAt == "" {
			return true
		}

		downloadedAt, dErr := time.Parse(time.RFC3339, meta.DownloadedAt)
		if dErr != nil {
			return true
		}

		// 本地记录的更新时间已超过下载时间 → 已确认更新，跳过检查
		if meta.TimeUpdated != "" {
			timeUpdated, tErr := time.Parse(time.RFC3339, meta.TimeUpdated)
			if tErr == nil && timeUpdated.After(downloadedAt) {
				confirmedCount++
				return true
			}
		}

		// 本地更新时间未超过下载时间 → 需要调用API检查
		target, ok := captureModTaskTarget(vpkFile.Path)
		if !ok {
			return true
		}
		toCheck = append(toCheck, updateCheckItem{vpkFile.Path, vpkFile.WorkshopID, downloadedAt, target})

		return true
	})

	log.Printf("需要检测更新的Mod数量: %d（已确认有更新: %d）", len(toCheck), confirmedCount)

	var newDetected int
	var skippedStale int
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, item := range toCheck {
		wg.Add(1)
		filePath := item.filePath
		workshopID := item.workshopID
		downloadedAt := item.downloadedAt
		target := item.target
		a.submitPoolTask(func() {
			defer wg.Done()

			detail, err := a.fetchWorkshopItemDetail(workshopID)
			if err != nil {
				log.Printf("检测更新失败: (ID: %s), 错误: %v", workshopID, err)
				return
			}

			var timeUpdated time.Time
			switch v := detail.TimeUpdated.(type) {
			case float64:
				timeUpdated = time.Unix(int64(v), 0)
			case int64:
				timeUpdated = time.Unix(v, 0)
			case string:
				ts, pErr := strconv.ParseInt(v, 10, 64)
				if pErr != nil {
					return
				}
				timeUpdated = time.Unix(ts, 0)
			default:
				return
			}

			if timeUpdated.After(downloadedAt) {
				// 回写前复查目标（对齐 FireAxe ValidTaskCreator）：
				// 用户可能在检测期间把 Mod 移走 / 换掉，这条结果就已经不属于当前文件了。
				if !target.stillValid() {
					log.Printf("检测到更新，但目标已被移动或替换，跳过写入: %s", filePath)
					mu.Lock()
					skippedStale++
					mu.Unlock()
					return
				}

				mu.Lock()
				newDetected++
				mu.Unlock()

				// 写入TimeUpdated到meta文件
				timeUpdatedStr := timeUpdated.Format(time.RFC3339)
				if writeErr := UpdateWorkshopMetaTimeUpdated(filePath, timeUpdatedStr); writeErr != nil {
					log.Printf("写入TimeUpdated失败: %s, 错误: %v", filePath, writeErr)
				}
			}
		})
	}

	wg.Wait()

	totalUpdates := confirmedCount + newDetected
	log.Printf("更新检测完成，总计 %d 个Mod有更新（新检测: %d）", totalUpdates, newDetected)

	result := UpdateCheckResult{
		TotalUpdates: totalUpdates,
		NewDetected:  newDetected,
		SkippedStale: skippedStale,
	}

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "mod_update_check_complete", result)
	}

	return result
}
