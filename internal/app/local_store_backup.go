package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hymkor/trash-go"
)

// 本地记录备份轮转（对齐 FireAxe `AddonRoot.BackUpIfNeed` 的四个约束）
//
//  1. 最短间隔：距离上一份备份不足间隔时不重复备份；
//  2. 内容去重：当前内容与最近一份备份逐字节相同则跳过；
//  3. 数量上限：保留最近 N 份；
//  4. 溢出处理：超出上限的旧备份进回收站，而不是直接删除。
//
// 作用对象：profiles.json / groups.json / dependencies.json / ignore.json / priority.json。
// 备份写在配置目录的 `backups/` 下，命名 `<记录名>.backup_YYYY-MM-DD_HH-mm.json`。

const (
	defaultLocalStoreBackupInterval = 15 * time.Minute
	defaultLocalStoreBackupMaxFiles = 10
	localStoreBackupDirName         = "backups"
	localStoreBackupTimeLayout      = "2006-01-02_15-04"
)

// localStoreBackupFileName 返回某个记录在给定时刻的备份文件名。
func localStoreBackupFileName(storeName string, moment time.Time) string {
	return fmt.Sprintf("%s.backup_%s.json", storeName, moment.Format(localStoreBackupTimeLayout))
}

func localStoreBackupNamePattern(storeName string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(storeName) + `\.backup_(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})\.json$`)
}

// parseLocalStoreBackupFileName 解析备份文件名中的时间；无法解析时返回 false。
func parseLocalStoreBackupFileName(storeName string, fileName string) (time.Time, bool) {
	match := localStoreBackupNamePattern(storeName).FindStringSubmatch(fileName)
	if len(match) != 6 {
		return time.Time{}, false
	}
	values := make([]int, 0, 5)
	for _, raw := range match[1:] {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return time.Time{}, false
		}
		values = append(values, value)
	}
	moment := time.Date(values[0], time.Month(values[1]), values[2], values[3], values[4], 0, 0, time.Local)
	return moment, true
}

// localStoreBackupEntry 是磁盘上的一份历史备份。
type localStoreBackupEntry struct {
	Path     string
	Moment   time.Time
	FileName string
}

// rotateLocalStoreBackup 是纯文件系统逻辑：按 FireAxe 的四条约束决定是否创建新备份，
// 并返回需要移入回收站的旧备份列表。调用方负责真正删除（见 moveLocalStoreBackupsToTrash）。
func rotateLocalStoreBackup(
	backupDir string,
	storeName string,
	content []byte,
	now time.Time,
	interval time.Duration,
	maxFiles int,
) (created bool, backupPath string, overflow []localStoreBackupEntry, err error) {
	if strings.TrimSpace(storeName) == "" {
		return false, "", nil, fmt.Errorf("缺少备份记录名")
	}
	if maxFiles < 1 {
		maxFiles = defaultLocalStoreBackupMaxFiles
	}
	if interval < 0 {
		interval = 0
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return false, "", nil, err
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return false, "", nil, err
	}
	existing := make([]localStoreBackupEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		moment, ok := parseLocalStoreBackupFileName(storeName, entry.Name())
		if !ok {
			continue
		}
		existing = append(existing, localStoreBackupEntry{
			Path:     filepath.Join(backupDir, entry.Name()),
			Moment:   moment,
			FileName: entry.Name(),
		})
	}
	sort.SliceStable(existing, func(i, j int) bool { return existing[i].Moment.Before(existing[j].Moment) })

	// 忽略"未来时间"的备份（用户改过系统时间或手动放进来），避免间隔判断被带偏。
	for len(existing) > 0 && existing[len(existing)-1].Moment.After(now) {
		existing = existing[:len(existing)-1]
	}

	if len(existing) > 0 {
		latest := existing[len(existing)-1]
		if interval > 0 && now.Sub(latest.Moment) < interval {
			return false, "", nil, nil
		}
		// 与最近一份内容相同则跳过，避免把"没有实质变化"的写入变成噪音。
		if latestContent, readErr := os.ReadFile(latest.Path); readErr == nil && string(latestContent) == string(content) {
			return false, "", nil, nil
		}
	}

	overflowCount := len(existing) + 1 - maxFiles
	if overflowCount > 0 {
		overflow = append(overflow, existing[:overflowCount]...)
	}

	backupPath = filepath.Join(backupDir, localStoreBackupFileName(storeName, now))
	if err := os.WriteFile(backupPath, content, 0o644); err != nil {
		return false, "", nil, err
	}
	return true, backupPath, overflow, nil
}

func (a *App) localStoreBackupClockNow() time.Time {
	if a.localStoreBackupClock != nil {
		return a.localStoreBackupClock()
	}
	return time.Now()
}

func (a *App) localStoreBackupIntervalValue() time.Duration {
	if a.localStoreBackupInterval > 0 {
		return a.localStoreBackupInterval
	}
	return defaultLocalStoreBackupInterval
}

func (a *App) localStoreBackupMaxValues() int {
	if a.localStoreBackupMaxFiles > 0 {
		return a.localStoreBackupMaxFiles
	}
	return defaultLocalStoreBackupMaxFiles
}

// localStoreBackupDirectory 返回某个记录文件的备份目录。
func (a *App) localStoreBackupDirectory() string {
	a.ensureConfigPaths()
	if a.configDir == "" {
		return ""
	}
	return filepath.Join(a.configDir, localStoreBackupDirName)
}

// backupLocalStoreFileIfNeeded 在写入本地记录前按轮转策略建立备份。
// 备份失败只记录日志，不阻断用户保存设置：备份是保护措施，不是写入前提。
func (a *App) backupLocalStoreFileIfNeeded(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	backupDir := a.localStoreBackupDirectory()
	if backupDir == "" {
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		// 文件还不存在（首次写入）时无需备份。
		return
	}
	storeName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if storeName == "" {
		return
	}

	created, _, overflow, err := rotateLocalStoreBackup(
		backupDir,
		storeName,
		content,
		a.localStoreBackupClockNow(),
		a.localStoreBackupIntervalValue(),
		a.localStoreBackupMaxValues(),
	)
	if err != nil {
		log.Printf("本地记录备份失败（不影响本次写入）: %s: %v", path, err)
		return
	}
	if !created {
		return
	}
	a.moveLocalStoreBackupsToTrash(overflow)
}

// moveLocalStoreBackupsToTrash 把溢出的旧备份移入回收站。
func (a *App) moveLocalStoreBackupsToTrash(entries []localStoreBackupEntry) {
	for _, entry := range entries {
		if a.localStoreBackupRemove != nil {
			if err := a.localStoreBackupRemove(entry.Path); err != nil {
				log.Printf("旧备份移入回收站失败: %s: %v", entry.Path, err)
			}
			continue
		}
		if err := trash.Throw(entry.Path); err != nil {
			log.Printf("旧备份移入回收站失败: %s: %v", entry.Path, err)
		}
	}
}

// ListLocalStoreBackups 返回某个本地记录的历史备份（按时间升序），供前端与脚本查看。
func (a *App) ListLocalStoreBackups(storeName string) ([]string, error) {
	storeName = strings.TrimSpace(storeName)
	if storeName == "" {
		return nil, fmt.Errorf("缺少记录名，例如 profiles")
	}
	storeName = strings.TrimSuffix(filepath.Base(storeName), filepath.Ext(storeName))
	dir := a.localStoreBackupDirectory()
	if dir == "" {
		return nil, fmt.Errorf("未配置配置目录，无法列出本地记录备份")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	backups := make([]localStoreBackupEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		moment, ok := parseLocalStoreBackupFileName(storeName, entry.Name())
		if !ok {
			continue
		}
		backups = append(backups, localStoreBackupEntry{
			Path:   filepath.Join(dir, entry.Name()),
			Moment: moment,
		})
	}
	sort.SliceStable(backups, func(i, j int) bool { return backups[i].Moment.Before(backups[j].Moment) })
	result := make([]string, 0, len(backups))
	for _, backup := range backups {
		result = append(result, backup.Path)
	}
	return result, nil
}
