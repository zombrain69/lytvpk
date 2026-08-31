package app

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	addonListManagedSnapshotSuffix = ".lytvpk-managed"
	addonListBackupDirectorySuffix = ".lytvpk-backups"
	addonListGuardPollInterval     = time.Second
)

// AddonListInfo 描述当前游戏配置及 LytVPK 的保护状态。
type AddonListInfo struct {
	Path                  string `json:"path"`
	Exists                bool   `json:"exists"`
	Size                  int64  `json:"size"`
	LastModified          string `json:"lastModified"`
	Encoding              string `json:"encoding"`
	ManagedSnapshotExists bool   `json:"managedSnapshotExists"`
	GuardEnabled          bool   `json:"guardEnabled"`
	LastGuardRestore      string `json:"lastGuardRestore"`
	LastGuardError        string `json:"lastGuardError"`
}

// AddonListBackup 表示一份可恢复的 addonlist.txt 原始字节备份。
type AddonListBackup struct {
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	Size      int64  `json:"size"`
	Kind      string `json:"kind"`
}

func addonListManagedSnapshotPath(addonListPath string) string {
	return addonListPath + addonListManagedSnapshotSuffix
}

func addonListBackupDirectory(addonListPath string) string {
	return addonListPath + addonListBackupDirectorySuffix
}

func addonListEncodingName(content []byte) string {
	if len(content) >= 3 && bytes.Equal(content[:3], []byte{0xEF, 0xBB, 0xBF}) {
		return "UTF-8 BOM"
	}
	if len(content) >= 2 && bytes.Equal(content[:2], []byte{0xFF, 0xFE}) {
		return "UTF-16 LE"
	}
	if len(content) >= 2 && bytes.Equal(content[:2], []byte{0xFE, 0xFF}) {
		return "UTF-16 BE"
	}
	if isValidUTF8(content) {
		return "UTF-8"
	}
	if decoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), content); err == nil && !strings.ContainsRune(string(decoded), '\uFFFD') {
		return "GBK/ANSI"
	}
	if decoded, _, err := transform.Bytes(charmap.Windows1252.NewDecoder(), content); err == nil && !strings.ContainsRune(string(decoded), '\uFFFD') {
		return "Windows-1252/ANSI"
	}
	return "GBK/ANSI"
}

func isValidUTF8(content []byte) bool {
	if len(content) >= 3 && bytes.Equal(content[:3], []byte{0xEF, 0xBB, 0xBF}) {
		content = content[3:]
	}
	return utf8.Valid(content)
}

func (a *App) addonListGuardStatus() (enabled bool, lastRestore string, lastError string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.addonListGuardEnabled, a.addonListLastRestore, a.addonListLastError
}

func (a *App) setAddonListGuardStatus(lastRestore, lastError string) {
	a.mu.Lock()
	if lastRestore != "" {
		a.addonListLastRestore = lastRestore
	}
	a.addonListLastError = lastError
	a.mu.Unlock()
}

func (a *App) setAddonListGuardEnabled(enabled bool) {
	a.mu.Lock()
	a.addonListGuardEnabled = enabled
	if !enabled {
		a.addonListLastError = ""
	}
	a.mu.Unlock()
}

func (a *App) getAddonListInfoLocked() (AddonListInfo, error) {
	path, err := a.addonListPath()
	if err != nil {
		return AddonListInfo{}, err
	}

	enabled, lastRestore, lastError := a.addonListGuardStatus()
	info := AddonListInfo{
		Path:                  path,
		ManagedSnapshotExists: fileExists(addonListManagedSnapshotPath(path)),
		GuardEnabled:          enabled,
		LastGuardRestore:      lastRestore,
		LastGuardError:        lastError,
	}

	metadata, err := os.Stat(path)
	if os.IsNotExist(err) {
		return info, nil
	}
	if err != nil {
		return info, fmt.Errorf("无法读取 addonlist.txt 状态: %w", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return info, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}
	info.Exists = true
	info.Size = metadata.Size()
	info.LastModified = metadata.ModTime().Format(time.RFC3339)
	info.Encoding = addonListEncodingName(content)
	return info, nil
}

// GetAddonListInfo 返回 addonlist.txt 及其保护快照的状态；文件不存在并非错误。
func (a *App) GetAddonListInfo() (AddonListInfo, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()
	return a.getAddonListInfoLocked()
}

// SaveAddonListManagedSnapshot 将当前 addonlist.txt 保存为自动恢复所使用的受保护版本。
func (a *App) SaveAddonListManagedSnapshot() (AddonListInfo, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	path, err := a.addonListPath()
	if err != nil {
		return AddonListInfo{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AddonListInfo{}, fmt.Errorf("无法保存受保护版本: %w", err)
	}
	if err := writeAddonListBytesAtomically(addonListManagedSnapshotPath(path), content); err != nil {
		return AddonListInfo{}, fmt.Errorf("无法写入受保护版本: %w", err)
	}
	a.setAddonListGuardStatus("", "")
	return a.getAddonListInfoLocked()
}

func (a *App) createAddonListBackupLocked(kind string, content []byte) (AddonListBackup, error) {
	path, err := a.addonListPath()
	if err != nil {
		return AddonListBackup{}, err
	}
	backupDir := addonListBackupDirectory(path)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return AddonListBackup{}, fmt.Errorf("无法创建 addonlist.txt 备份目录: %w", err)
	}

	now := time.Now()
	base := fmt.Sprintf("%s-%s", kind, now.Format("20060102T150405.000000000Z07"))
	for index := 0; index < 100; index++ {
		name := base + ".txt"
		if index > 0 {
			name = fmt.Sprintf("%s-%02d.txt", base, index)
		}
		backupPath := filepath.Join(backupDir, name)
		file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return AddonListBackup{}, fmt.Errorf("无法创建 addonlist.txt 备份: %w", err)
		}
		if _, err := file.Write(content); err != nil {
			_ = file.Close()
			_ = os.Remove(backupPath)
			return AddonListBackup{}, fmt.Errorf("无法写入 addonlist.txt 备份: %w", err)
		}
		if err := file.Close(); err != nil {
			return AddonListBackup{}, fmt.Errorf("无法关闭 addonlist.txt 备份: %w", err)
		}
		return AddonListBackup{Name: name, CreatedAt: now.Format(time.RFC3339), Size: int64(len(content)), Kind: kind}, nil
	}
	return AddonListBackup{}, fmt.Errorf("无法生成唯一的 addonlist.txt 备份文件名")
}

// CreateAddonListBackup 创建一份当前 addonlist.txt 的历史备份。
func (a *App) CreateAddonListBackup() (AddonListBackup, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	path, err := a.addonListPath()
	if err != nil {
		return AddonListBackup{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AddonListBackup{}, fmt.Errorf("无法创建备份: %w", err)
	}
	return a.createAddonListBackupLocked("manual", content)
}

// ListAddonListBackups 返回手动和自动创建的可恢复备份，按创建时间从新到旧排列。
func (a *App) ListAddonListBackups() ([]AddonListBackup, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	path, err := a.addonListPath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(addonListBackupDirectory(path))
	if os.IsNotExist(err) {
		return []AddonListBackup{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("无法列出 addonlist.txt 备份: %w", err)
	}

	backups := make([]AddonListBackup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".txt") {
			continue
		}
		metadata, err := entry.Info()
		if err != nil {
			continue
		}
		name := entry.Name()
		base := strings.TrimSuffix(name, filepath.Ext(name))
		kind := strings.SplitN(base, "-", 2)[0]
		// Most backup kinds use one leading word. Preserve the legacy
		// game-save marker so old backups can still be identified in settings.
		if strings.HasPrefix(base, "game-save-") {
			kind = "game-save"
		}
		backups = append(backups, AddonListBackup{
			Name:      name,
			CreatedAt: metadata.ModTime().Format(time.RFC3339),
			Size:      metadata.Size(),
			Kind:      kind,
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].CreatedAt == backups[j].CreatedAt {
			return backups[i].Name > backups[j].Name
		}
		return backups[i].CreatedAt > backups[j].CreatedAt
	})
	return backups, nil
}

func validAddonListBackupName(name string) bool {
	return name != "" && name == filepath.Base(name) && filepath.Clean(name) == name && strings.HasSuffix(strings.ToLower(name), ".txt")
}

// RestoreAddonListBackup 恢复一个历史备份，并同步更新受保护版本，避免自动监控将其回滚。
func (a *App) RestoreAddonListBackup(name string) (AddonListInfo, error) {
	if !validAddonListBackupName(name) {
		return AddonListInfo{}, fmt.Errorf("无效的 addonlist.txt 备份名称")
	}

	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	path, err := a.addonListPath()
	if err != nil {
		return AddonListInfo{}, err
	}
	backupPath := filepath.Join(addonListBackupDirectory(path), name)
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return AddonListInfo{}, fmt.Errorf("无法读取 addonlist.txt 备份: %w", err)
	}
	if current, err := os.ReadFile(path); err == nil {
		if _, err := a.createAddonListBackupLocked("before-restore", current); err != nil {
			return AddonListInfo{}, err
		}
	} else if !os.IsNotExist(err) {
		return AddonListInfo{}, fmt.Errorf("无法读取当前 addonlist.txt: %w", err)
	}
	if err := writeAddonListBytesAtomically(path, content); err != nil {
		return AddonListInfo{}, fmt.Errorf("无法恢复 addonlist.txt: %w", err)
	}
	if err := writeAddonListBytesAtomically(addonListManagedSnapshotPath(path), content); err != nil {
		return AddonListInfo{}, fmt.Errorf("无法同步受保护版本: %w", err)
	}
	a.applyAddonListGameStates()
	a.setAddonListGuardStatus("", "")
	return a.getAddonListInfoLocked()
}

// DeleteAddonListBackup 删除一份历史备份。受保护版本和首次编辑备份不会受影响。
func (a *App) DeleteAddonListBackup(name string) error {
	if !validAddonListBackupName(name) {
		return fmt.Errorf("无效的 addonlist.txt 备份名称")
	}

	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()
	path, err := a.addonListPath()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(addonListBackupDirectory(path), name)); err != nil {
		return fmt.Errorf("无法删除 addonlist.txt 备份: %w", err)
	}
	return nil
}

// DeleteAddonList 删除当前 addonlist.txt；删除前会自动创建 before-delete 备份。
func (a *App) DeleteAddonList() error {
	a.addonListGuardMu.Lock()
	path, err := a.addonListPath()
	if err != nil {
		a.addonListGuardMu.Unlock()
		return err
	}
	if content, err := os.ReadFile(path); err == nil {
		if _, err := a.createAddonListBackupLocked("before-delete", content); err != nil {
			a.addonListGuardMu.Unlock()
			return err
		}
		if err := os.Remove(path); err != nil {
			a.addonListGuardMu.Unlock()
			return fmt.Errorf("无法删除 addonlist.txt: %w", err)
		}
	} else if !os.IsNotExist(err) {
		a.addonListGuardMu.Unlock()
		return fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}
	if err := os.Remove(addonListManagedSnapshotPath(path)); err != nil && !os.IsNotExist(err) {
		a.addonListGuardMu.Unlock()
		return fmt.Errorf("无法删除受保护版本: %w", err)
	}
	a.setAddonListGuardEnabled(false)
	a.addonListGuardMu.Unlock()

	a.stopAddonListMonitor()
	if err := a.saveAddonListGuardConfig(); err != nil {
		return err
	}
	return nil
}

// SetAddonListGuardEnabled 启用后，LytVPK 运行期间会把被游戏覆盖的 addonlist.txt 恢复为受保护版本。
func (a *App) SetAddonListGuardEnabled(enabled bool) (AddonListInfo, error) {
	a.addonListGuardMu.Lock()
	if enabled {
		path, err := a.addonListPath()
		if err != nil {
			a.addonListGuardMu.Unlock()
			return AddonListInfo{}, err
		}
		if !fileExists(addonListManagedSnapshotPath(path)) {
			content, err := os.ReadFile(path)
			if err != nil {
				a.addonListGuardMu.Unlock()
				return AddonListInfo{}, fmt.Errorf("启用监控前无法读取 addonlist.txt: %w", err)
			}
			if err := writeAddonListBytesAtomically(addonListManagedSnapshotPath(path), content); err != nil {
				a.addonListGuardMu.Unlock()
				return AddonListInfo{}, fmt.Errorf("无法创建受保护版本: %w", err)
			}
		}
	}
	a.setAddonListGuardEnabled(enabled)
	a.setAddonListGuardStatus("", "")
	info, infoErr := a.getAddonListInfoLocked()
	a.addonListGuardMu.Unlock()
	if infoErr != nil {
		return AddonListInfo{}, infoErr
	}
	if err := a.saveAddonListGuardConfig(); err != nil {
		return info, err
	}
	if enabled {
		a.restartAddonListMonitor()
	} else {
		a.stopAddonListMonitor()
	}
	return info, nil
}

func (a *App) saveAddonListGuardConfig() error {
	a.mu.RLock()
	hasConfigPath := a.configPath != "" || a.configDir != ""
	a.mu.RUnlock()
	if !hasConfigPath {
		return nil
	}
	return a.writeConfigFile(a.snapshotConfig())
}

// syncManagedAddonListSnapshotLocked must be called while addonListGuardMu is held.
func (a *App) syncManagedAddonListSnapshotLocked(path string) error {
	enabled, _, _ := a.addonListGuardStatus()
	if !enabled {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("无法同步受保护版本: %w", err)
	}
	if err := writeAddonListBytesAtomically(addonListManagedSnapshotPath(path), content); err != nil {
		return fmt.Errorf("无法同步受保护版本: %w", err)
	}
	a.setAddonListGuardStatus("", "")
	return nil
}

func writeAddonListBytesAtomically(path string, content []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".lytvpk-addonlist-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (a *App) startAddonListMonitor() {
	a.addonListMonitorMu.Lock()
	if a.addonListMonitorStop != nil {
		a.addonListMonitorMu.Unlock()
		return
	}
	stop := make(chan struct{})
	a.addonListMonitorStop = stop
	a.addonListMonitorMu.Unlock()
	go a.runAddonListMonitor(stop)
}

func (a *App) stopAddonListMonitor() {
	a.addonListMonitorMu.Lock()
	stop := a.addonListMonitorStop
	a.addonListMonitorStop = nil
	a.addonListMonitorMu.Unlock()
	if stop != nil {
		close(stop)
	}
}

func (a *App) restartAddonListMonitor() {
	a.stopAddonListMonitor()
	if a.addonListMonitorNeeded() {
		a.startAddonListMonitor()
	}
}

func (a *App) finishAddonListMonitor(stop chan struct{}) {
	a.addonListMonitorMu.Lock()
	if a.addonListMonitorStop == stop {
		a.addonListMonitorStop = nil
	}
	a.addonListMonitorMu.Unlock()

	// 监控结束与新的用户保护设置可能交错；释放旧通道后再次确认，避免
	// 切换保护状态时遗漏新的监控请求。
	if a.addonListMonitorNeeded() {
		a.startAddonListMonitor()
	}
}

func (a *App) runAddonListMonitor(stop chan struct{}) {
	ticker := time.NewTicker(addonListGuardPollInterval)
	defer ticker.Stop()

	lastSignature := ""
	stableMismatchCount := 0
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			mismatch, signature, err := a.addonListGuardMismatch()
			if err != nil {
				a.setAddonListGuardStatus("", err.Error())
				lastSignature = ""
				stableMismatchCount = 0
			} else if !mismatch {
				lastSignature = ""
				stableMismatchCount = 0
			} else if signature == lastSignature {
				stableMismatchCount++
			} else {
				lastSignature = signature
				stableMismatchCount = 1
			}
			if mismatch && stableMismatchCount >= 2 {
				restored, restoreErr := a.restoreManagedAddonListSnapshot(signature)
				if restoreErr != nil {
					a.setAddonListGuardStatus("", restoreErr.Error())
				}
				if restored {
					lastSignature = ""
					stableMismatchCount = 0
				}
			}

			if !a.addonListMonitorNeeded() {
				a.finishAddonListMonitor(stop)
				return
			}
		}
	}
}

// addonListMonitorNeeded reports whether the user explicitly enabled the
// full-file guard.
func (a *App) addonListMonitorNeeded() bool {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()
	return a.addonListMonitorNeededLocked(time.Now())
}

func (a *App) addonListMonitorNeededLocked(now time.Time) bool {
	enabled, _, _ := a.addonListGuardStatus()
	return enabled
}

func (a *App) addonListGuardMismatch() (bool, string, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	enabled, _, _ := a.addonListGuardStatus()
	if !enabled {
		return false, "", nil
	}
	path, err := a.addonListPath()
	if err != nil {
		return false, "", err
	}
	snapshot, err := os.ReadFile(addonListManagedSnapshotPath(path))
	if err != nil {
		return false, "", fmt.Errorf("无法读取受保护版本: %w", err)
	}
	current, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return false, "", fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}
	if exists && bytes.Equal(snapshot, current) {
		a.setAddonListGuardStatus("", "")
		return false, "", nil
	}
	snapshotHash := sha256.Sum256(snapshot)
	if !exists {
		return true, fmt.Sprintf("missing:%x", snapshotHash), nil
	}
	currentHash := sha256.Sum256(current)
	return true, fmt.Sprintf("%x:%x", snapshotHash, currentHash), nil
}

func (a *App) restoreManagedAddonListSnapshot(expectedSignature string) (bool, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	enabled, _, _ := a.addonListGuardStatus()
	if !enabled {
		return false, nil
	}
	path, err := a.addonListPath()
	if err != nil {
		return false, err
	}
	snapshot, err := os.ReadFile(addonListManagedSnapshotPath(path))
	if err != nil {
		return false, fmt.Errorf("无法读取受保护版本: %w", err)
	}
	current, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}
	if exists && bytes.Equal(snapshot, current) {
		return false, nil
	}
	snapshotHash := sha256.Sum256(snapshot)
	actualSignature := fmt.Sprintf("missing:%x", snapshotHash)
	if exists {
		currentHash := sha256.Sum256(current)
		actualSignature = fmt.Sprintf("%x:%x", snapshotHash, currentHash)
	}
	if actualSignature != expectedSignature {
		return false, nil
	}
	if exists {
		if _, err := a.createAddonListBackupLocked("external", current); err != nil {
			return false, err
		}
	}
	if err := writeAddonListBytesAtomically(path, snapshot); err != nil {
		return false, fmt.Errorf("无法恢复 addonlist.txt: %w", err)
	}
	a.applyAddonListGameStates()
	restoredAt := time.Now().Format(time.RFC3339)
	a.setAddonListGuardStatus(restoredAt, "")
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "addonlist_guard_restored", map[string]string{
			"path":       path,
			"restoredAt": restoredAt,
		})
	}
	return true, nil
}
