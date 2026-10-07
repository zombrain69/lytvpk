package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hymkor/trash-go"
)

// Mod 快照（对齐上游 ba20411 的最小闭环）。
//
// 目标：在"批量启用/禁用、整理文件、更新或删除 Mod 之前"留一个可整体回滚的状态。
//
// 两种类型：
//   - names：只记录 addonlist.txt 的条目 + Mod 清单（名称/大小/位置），创建快、占用小；
//   - full ：额外把 addons 根目录里的 VPK 与同名图片/.meta 复制进快照目录，可恢复文件内容。
//
// 边界（与上游一致）：
//   - workshop 目录里的 Mod 只记录、不备份文件（它们由 Steam 管，恢复时靠 addonlist 决定启停）；
//   - 恢复 = 把 Mod 调整回快照时的状态：快照之后新增的 Mod 会被移到 disabled；
//   - 执行恢复前自动留一份 addonlist 备份，并且遵守沙箱只读闸门；
//   - 文件大小相同即视为"内容一致"（不做全量哈希：真机库里有 400MB 级 VPK，哈希代价不可接受）。

const (
	modSnapshotVersion  = 1
	modSnapshotModeName = "names"
	modSnapshotModeFull = "full"
)

// ModSnapshotMeta 是快照列表里的一条记录（不含明细）。
type ModSnapshotMeta struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	CreatedAt        string `json:"createdAt"`
	ModCount         int    `json:"modCount"`
	TotalSize        int64  `json:"totalSize"`
	BackupSize       int64  `json:"backupSize"`
	AddonListEntries int    `json:"addonListEntries"`
	AddonsRoot       string `json:"addonsRoot,omitempty"`
}

type modSnapshotSidecar struct {
	Name   string `json:"name"`
	Backup string `json:"backup,omitempty"`
	Size   int64  `json:"size"`
}

type modSnapshotFile struct {
	Key      string               `json:"key"`
	Location string               `json:"location"`
	Size     int64                `json:"size"`
	Backup   string               `json:"backup,omitempty"`
	Sidecars []modSnapshotSidecar `json:"sidecars,omitempty"`
}

type modSnapshotManifest struct {
	Version    int               `json:"version"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Mode       string            `json:"mode"`
	CreatedAt  string            `json:"createdAt"`
	AddonsRoot string            `json:"addonsRoot,omitempty"`
	Mods       []modSnapshotFile `json:"mods"`
	AddonList  []AddonListItem   `json:"addonList"`
}

// ModSnapshotRestoreItem 是恢复计划里的一条。
// Action ∈ enable（从 disabled 移回根目录）| disable（移到 disabled）| restore（从快照补回文件）
// | overwrite（用快照文件覆盖当前文件）| skip（已经一致）| missing（快照有记录但找不到文件）。
type ModSnapshotRestoreItem struct {
	Action string `json:"action"`
	Key    string `json:"key"`
	Detail string `json:"detail,omitempty"`
}

type ModSnapshotRestorePlan struct {
	Snapshot       ModSnapshotMeta          `json:"snapshot"`
	Items          []ModSnapshotRestoreItem `json:"items"`
	AddonListItems int                      `json:"addonListItems"`
	Blocked        string                   `json:"blocked,omitempty"`
	Counts         map[string]int           `json:"counts"`
}

type ModSnapshotRestoreResult struct {
	Enabled         int  `json:"enabled"`
	Disabled        int  `json:"disabled"`
	Restored        int  `json:"restored"`
	Overwritten     int  `json:"overwritten"`
	Skipped         int  `json:"skipped"`
	Missing         int  `json:"missing"`
	AddonListWrote  bool `json:"addonListWrote"`
	AddonListBefore int  `json:"addonListBefore"`
}

func (a *App) modSnapshotsRoot() string {
	a.mu.RLock()
	dir := a.configDir
	a.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	return filepath.Join(dir, "snapshots")
}

// ListModSnapshots 返回全部快照（按创建时间从新到旧）。
func (a *App) ListModSnapshots() ([]ModSnapshotMeta, error) {
	root := a.modSnapshotsRoot()
	if root == "" {
		return nil, fmt.Errorf("配置目录不可用，无法读取 Mod 快照")
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []ModSnapshotMeta{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取快照目录失败: %w", err)
	}
	result := make([]ModSnapshotMeta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := a.readModSnapshotManifest(entry.Name())
		if err != nil {
			log.Printf("跳过损坏的快照 %s: %v", entry.Name(), err)
			continue
		}
		meta := ModSnapshotMeta{
			ID:               manifest.ID,
			Name:             manifest.Name,
			Mode:             manifest.Mode,
			CreatedAt:        manifest.CreatedAt,
			ModCount:         len(manifest.Mods),
			AddonListEntries: len(manifest.AddonList),
			AddonsRoot:       manifest.AddonsRoot,
		}
		for _, mod := range manifest.Mods {
			meta.TotalSize += mod.Size
			if mod.Backup != "" {
				meta.BackupSize += mod.Size
			}
		}
		result = append(result, meta)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	return result, nil
}

func (a *App) readModSnapshotManifest(id string) (modSnapshotManifest, error) {
	var manifest modSnapshotManifest
	root := a.modSnapshotsRoot()
	if root == "" {
		return manifest, fmt.Errorf("配置目录不可用")
	}
	path := filepath.Join(root, filepath.Base(strings.TrimSpace(id)), "snapshot.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Version != modSnapshotVersion {
		return manifest, fmt.Errorf("快照版本不支持: %d", manifest.Version)
	}
	return manifest, nil
}

// CreateModSnapshot 创建快照。mode 只接受 names / full。
func (a *App) CreateModSnapshot(name, mode string) (ModSnapshotMeta, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != modSnapshotModeName && mode != modSnapshotModeFull {
		return ModSnapshotMeta{}, fmt.Errorf("未知的快照类型 %q（只支持 names / full）", mode)
	}
	root := a.addonsRootForSnapshot()
	if root == "" {
		return ModSnapshotMeta{}, fmt.Errorf("请先选择 Left 4 Dead 2 的 addons 目录")
	}
	snapshotsRoot := a.modSnapshotsRoot()
	if snapshotsRoot == "" {
		return ModSnapshotMeta{}, fmt.Errorf("配置目录不可用，无法创建快照")
	}

	now := time.Now()
	id := now.Format("20060102-150405")
	if slug := snapshotSlug(name); slug != "" {
		id += "-" + slug
	}
	if _, err := os.Stat(filepath.Join(snapshotsRoot, id)); err == nil {
		id += "-" + fmt.Sprintf("%03d", now.Nanosecond()/1_000_000)
	}
	dir := filepath.Join(snapshotsRoot, id)
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		return ModSnapshotMeta{}, fmt.Errorf("创建快照目录失败: %w", err)
	}

	addonList, _, _ := a.readAddonList()
	manifest := modSnapshotManifest{
		Version:    modSnapshotVersion,
		ID:         id,
		Name:       strings.TrimSpace(name),
		Mode:       mode,
		CreatedAt:  now.Format(time.RFC3339),
		AddonsRoot: root,
		AddonList:  addonList,
	}
	if manifest.Name == "" {
		manifest.Name = now.Format("2006-01-02 15:04")
	}
	manifest.Mods = a.collectModSnapshotFiles(root, id, mode)

	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ModSnapshotMeta{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), encoded, 0o644); err != nil {
		return ModSnapshotMeta{}, fmt.Errorf("写入快照清单失败: %w", err)
	}
	result := ModSnapshotMeta{
		ID:               manifest.ID,
		Name:             manifest.Name,
		Mode:             manifest.Mode,
		CreatedAt:        manifest.CreatedAt,
		ModCount:         len(manifest.Mods),
		AddonListEntries: len(manifest.AddonList),
		AddonsRoot:       manifest.AddonsRoot,
	}
	for _, mod := range manifest.Mods {
		result.TotalSize += mod.Size
		if mod.Backup != "" {
			result.BackupSize += mod.Size
		}
	}
	return result, nil
}

func (a *App) addonsRootForSnapshot() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return strings.TrimSpace(a.rootDir)
}

// collectModSnapshotFiles 枚举 addons 根目录 / disabled / workshop 里的 VPK。
// full 模式会顺便把"根目录与 disabled"里的文件（含同名图片/.meta）复制进快照。
func (a *App) collectModSnapshotFiles(root, snapshotID string, mode string) []modSnapshotFile {
	locations := []struct {
		location string
		dir      string
	}{
		{"root", root},
		{"disabled", filepath.Join(root, "disabled")},
		{"workshop", filepath.Join(root, "workshop")},
	}
	result := make([]modSnapshotFile, 0, 256)
	for _, item := range locations {
		entries, err := os.ReadDir(item.dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			mod := modSnapshotFile{
				Key:      snapshotAddonListKey(item.location, entry.Name()),
				Location: item.location,
				Size:     info.Size(),
			}
			if mode == modSnapshotModeFull && item.location != "workshop" {
				backup, copyErr := a.copyModSnapshotFile(root, snapshotID, filepath.Join(item.dir, entry.Name()))
				if copyErr != nil {
					log.Printf("快照复制失败（%s）：%v", entry.Name(), copyErr)
				} else {
					mod.Backup = backup
				}
				mod.Sidecars = a.copyModSnapshotSidecars(root, snapshotID, item.dir, entry.Name())
			}
			result = append(result, mod)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func snapshotAddonListKey(location, name string) string {
	if location == "workshop" {
		return "workshop\\" + name
	}
	// disabled 里的文件在 addonlist 里仍是裸文件名（游戏不会加载）；快照按裸键记录，
	// 位置单独用 Location 表达，恢复时才能知道"当时它是被禁用的"。
	return name
}

func (a *App) copyModSnapshotFile(root, snapshotID, sourcePath string) (string, error) {
	relative, err := filepath.Rel(root, sourcePath)
	if err != nil {
		return "", err
	}
	target := filepath.Join(a.modSnapshotsRoot(), snapshotID, "files", relative)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("files", relative)), copyFileContents(sourcePath, target)
}

func (a *App) copyModSnapshotSidecars(root, snapshotID, dir, vpkName string) []modSnapshotSidecar {
	base := strings.TrimSuffix(vpkName, filepath.Ext(vpkName))
	sidecars := make([]modSnapshotSidecar, 0, 3)
	for _, ext := range []string{".jpg", ".png", ".jpeg", ".gif", ".meta"} {
		source := filepath.Join(dir, base+ext)
		info, err := os.Stat(source)
		if err != nil || info.IsDir() {
			continue
		}
		backup, copyErr := a.copyModSnapshotFile(root, snapshotID, source)
		if copyErr != nil {
			log.Printf("快照复制侧车文件失败（%s）：%v", source, copyErr)
			continue
		}
		sidecars = append(sidecars, modSnapshotSidecar{Name: base + ext, Backup: backup, Size: info.Size()})
	}
	return sidecars
}

func copyFileContents(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// DeleteModSnapshot 把快照目录移入回收站（删除的快照不影响 Mod 文件）。
func (a *App) DeleteModSnapshot(id string) error {
	root := a.modSnapshotsRoot()
	if root == "" {
		return fmt.Errorf("配置目录不可用")
	}
	dir := filepath.Join(root, filepath.Base(strings.TrimSpace(id)))
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("找不到这个快照: %w", err)
	}
	if err := trash.Throw(dir); err == nil {
		return nil
	} else if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		// 个别实现的回收站操作会"做成了但返回错误"（例如激活上下文缺失时的收尾步骤），
		// 目录已经不在就按成功处理，避免误报 + 二次移动。
		return nil
	}
	// 系统回收站不可用（CI/无桌面会话等）时退回"移进 .removed 子目录"：
	// 仍然是可恢复的删除，绝不做不可回滚的永久删除。
	removedDir := filepath.Join(root, ".removed")
	if err := os.MkdirAll(removedDir, 0o755); err != nil {
		return fmt.Errorf("删除快照失败: %w", err)
	}
	target := filepath.Join(removedDir, filepath.Base(dir)+"-"+time.Now().Format("20060102-150405"))
	if err := os.Rename(dir, target); err != nil {
		return fmt.Errorf("删除快照失败: %w", err)
	}
	log.Printf("系统回收站不可用，快照已移入 %s", target)
	return nil
}

// PreviewModSnapshotRestore 生成恢复计划（只读，不改任何文件）。
func (a *App) PreviewModSnapshotRestore(id string) (ModSnapshotRestorePlan, error) {
	manifest, err := a.readModSnapshotManifest(id)
	if err != nil {
		return ModSnapshotRestorePlan{}, fmt.Errorf("读取快照失败: %w", err)
	}
	root := a.addonsRootForSnapshot()
	if root == "" {
		return ModSnapshotRestorePlan{}, fmt.Errorf("请先选择 Left 4 Dead 2 的 addons 目录")
	}
	return a.buildModSnapshotRestorePlan(root, manifest), nil
}

func (a *App) buildModSnapshotRestorePlan(root string, manifest modSnapshotManifest) ModSnapshotRestorePlan {
	plan := ModSnapshotRestorePlan{
		Snapshot: ModSnapshotMeta{
			ID: manifest.ID, Name: manifest.Name, Mode: manifest.Mode, CreatedAt: manifest.CreatedAt,
			ModCount: len(manifest.Mods), AddonListEntries: len(manifest.AddonList), AddonsRoot: manifest.AddonsRoot,
		},
		AddonListItems: len(manifest.AddonList),
		Counts:         map[string]int{},
	}
	known := make(map[string]struct{}, len(manifest.Mods))
	for _, mod := range manifest.Mods {
		known[strings.ToLower(mod.Key)] = struct{}{}
		items := a.planModSnapshotFile(root, manifest, mod)
		plan.Items = append(plan.Items, items...)
	}
	// 快照之后新增的根目录 Mod：恢复到快照状态 = 移到 disabled。
	added, _ := os.ReadDir(root)
	for _, entry := range added {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
			continue
		}
		if _, ok := known[strings.ToLower(entry.Name())]; ok {
			continue
		}
		plan.Items = append(plan.Items, ModSnapshotRestoreItem{
			Action: "disable",
			Key:    entry.Name(),
			Detail: "快照之后新增的 Mod，恢复到禁用",
		})
	}
	for _, item := range plan.Items {
		plan.Counts[item.Action]++
	}
	return plan
}

func (a *App) planModSnapshotFile(root string, manifest modSnapshotManifest, mod modSnapshotFile) []ModSnapshotRestoreItem {
	rootPath := filepath.Join(root, mod.Key)
	if mod.Location == "workshop" {
		workshopPath := filepath.Join(root, "workshop", strings.TrimPrefix(mod.Key, "workshop\\"))
		if _, err := os.Stat(workshopPath); err == nil {
			return []ModSnapshotRestoreItem{{Action: "skip", Key: mod.Key, Detail: "工坊 Mod 由 Steam 管理，只恢复启停状态"}}
		}
		return []ModSnapshotRestoreItem{{Action: "missing", Key: mod.Key, Detail: "工坊文件已不在 workshop 目录"}}
	}
	disabledPath := filepath.Join(root, "disabled", filepath.Base(mod.Key))

	if mod.Location == "disabled" {
		if info, err := os.Stat(disabledPath); err == nil && !info.IsDir() {
			return []ModSnapshotRestoreItem{{Action: "skip", Key: mod.Key}}
		}
		if info, err := os.Stat(rootPath); err == nil && !info.IsDir() {
			return []ModSnapshotRestoreItem{{Action: "disable", Key: mod.Key, Detail: "快照时是禁用状态"}}
		}
		if backupPath := a.modSnapshotBackupPath(manifest, mod); backupPath != "" {
			return []ModSnapshotRestoreItem{{Action: "restore", Key: mod.Key, Detail: "从快照补回（禁用位置）"}}
		}
		return []ModSnapshotRestoreItem{{Action: "missing", Key: mod.Key, Detail: "快照后文件已删除"}}
	}

	if info, err := os.Stat(rootPath); err == nil && !info.IsDir() {
		if mod.Backup != "" && info.Size() != mod.Size {
			return []ModSnapshotRestoreItem{{Action: "overwrite", Key: mod.Key, Detail: fmt.Sprintf("大小 %d → %d，用快照覆盖", info.Size(), mod.Size)}}
		}
		return []ModSnapshotRestoreItem{{Action: "skip", Key: mod.Key}}
	}
	if info, err := os.Stat(disabledPath); err == nil && !info.IsDir() {
		_ = info
		return []ModSnapshotRestoreItem{{Action: "enable", Key: mod.Key, Detail: "从 disabled 移回根目录"}}
	}
	if backupPath := a.modSnapshotBackupPath(manifest, mod); backupPath != "" {
		return []ModSnapshotRestoreItem{{Action: "restore", Key: mod.Key, Detail: "从快照补回"}}
	}
	return []ModSnapshotRestoreItem{{Action: "missing", Key: mod.Key, Detail: "快照里只有文件名记录，文件已找不到"}}
}

func (a *App) modSnapshotBackupPath(manifest modSnapshotManifest, mod modSnapshotFile) string {
	if manifest.Mode != modSnapshotModeFull || mod.Backup == "" {
		return ""
	}
	root := a.modSnapshotsRoot()
	if root == "" {
		return ""
	}
	return filepath.Join(root, manifest.ID, filepath.FromSlash(mod.Backup))
}

// RestoreModSnapshot 执行恢复。执行前会备份当前 addonlist.txt。
func (a *App) RestoreModSnapshot(id string) (ModSnapshotRestoreResult, error) {
	if err := rejectReadonlyLibraryWrite("恢复 Mod 快照"); err != nil {
		return ModSnapshotRestoreResult{}, err
	}
	manifest, err := a.readModSnapshotManifest(id)
	if err != nil {
		return ModSnapshotRestoreResult{}, fmt.Errorf("读取快照失败: %w", err)
	}
	root := a.addonsRootForSnapshot()
	if root == "" {
		return ModSnapshotRestoreResult{}, fmt.Errorf("请先选择 Left 4 Dead 2 的 addons 目录")
	}
	plan := a.buildModSnapshotRestorePlan(root, manifest)
	result := ModSnapshotRestoreResult{AddonListBefore: len(manifest.AddonList)}

	// 执行前先备份当前 addonlist.txt（与"任何删除都要可恢复"的既有口径一致）。
	a.addonListGuardMu.Lock()
	if addonListPath, pathErr := a.addonListPath(); pathErr == nil {
		if raw, readErr := os.ReadFile(addonListPath); readErr == nil {
			if _, backupErr := a.createAddonListBackupLocked("before-snapshot-restore", raw); backupErr != nil {
				log.Printf("恢复快照前的 addonlist 备份失败: %v", backupErr)
			}
		}
	}
	a.addonListGuardMu.Unlock()

	disabledDir := filepath.Join(root, "disabled")
	_ = os.MkdirAll(disabledDir, 0o755)
	for _, item := range plan.Items {
		switch item.Action {
		case "disable":
			source := filepath.Join(root, filepath.Base(item.Key))
			target := filepath.Join(disabledDir, filepath.Base(item.Key))
			if err := os.Rename(source, target); err != nil {
				log.Printf("快照恢复：禁用 %s 失败: %v", item.Key, err)
				continue
			}
			a.handleSidecarFile(source, target, "move")
			result.Disabled++
		case "enable":
			source := filepath.Join(disabledDir, filepath.Base(item.Key))
			target := filepath.Join(root, filepath.Base(item.Key))
			if err := os.Rename(source, target); err != nil {
				log.Printf("快照恢复：启用 %s 失败: %v", item.Key, err)
				continue
			}
			a.handleSidecarFile(source, target, "move")
			result.Enabled++
		case "restore", "overwrite":
			mod, ok := findModSnapshotFile(manifest, item.Key)
			if !ok {
				result.Missing++
				continue
			}
			backupPath := a.modSnapshotBackupPath(manifest, mod)
			if backupPath == "" {
				result.Missing++
				continue
			}
			targetDir := root
			if mod.Location == "disabled" {
				targetDir = disabledDir
			}
			target := filepath.Join(targetDir, filepath.Base(mod.Key))
			if err := copyFileContents(backupPath, target); err != nil {
				log.Printf("快照恢复：写回 %s 失败: %v", mod.Key, err)
				result.Missing++
				continue
			}
			a.restoreModSnapshotSidecars(manifest, mod, targetDir)
			if item.Action == "overwrite" {
				result.Overwritten++
			} else {
				result.Restored++
			}
		case "skip":
			result.Skipped++
		case "missing":
			result.Missing++
		}
	}

	// 恢复 addonlist（保持"游戏内开关 + 加载顺序"都回到快照时刻）。
	if len(manifest.AddonList) > 0 {
		if path, pathErr := a.addonListPath(); pathErr == nil {
			a.addonListGuardMu.Lock()
			commitErr := a.commitAddonListItemsLocked(path, manifest.AddonList, nil)
			a.addonListGuardMu.Unlock()
			if commitErr != nil {
				return result, fmt.Errorf("Mod 文件已恢复，但写回 addonlist.txt 失败: %w", commitErr)
			}
			result.AddonListWrote = true
		}
	}
	return result, nil
}

func (a *App) restoreModSnapshotSidecars(manifest modSnapshotManifest, mod modSnapshotFile, targetDir string) {
	for _, sidecar := range mod.Sidecars {
		if sidecar.Backup == "" {
			continue
		}
		source := filepath.Join(a.modSnapshotsRoot(), manifest.ID, filepath.FromSlash(sidecar.Backup))
		target := filepath.Join(targetDir, filepath.Base(sidecar.Name))
		if err := copyFileContents(source, target); err != nil {
			log.Printf("快照恢复：写回侧车文件 %s 失败: %v", sidecar.Name, err)
		}
	}
}

func findModSnapshotFile(manifest modSnapshotManifest, key string) (modSnapshotFile, bool) {
	for _, mod := range manifest.Mods {
		if strings.EqualFold(mod.Key, key) {
			return mod, true
		}
	}
	return modSnapshotFile{}, false
}

// OpenModSnapshotsFolder 打开快照存放目录（不存在时先创建）。
func (a *App) OpenModSnapshotsFolder() error {
	root := a.modSnapshotsRoot()
	if root == "" {
		return fmt.Errorf("配置目录不可用")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return a.OpenFileLocation(root)
}

// snapshotSlug 把快照名转成安全的目录名片段（只保留字母数字和短横线）。
func snapshotSlug(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r - 'A' + 'a')
		case r == '-' || r == '_':
			builder.WriteRune('-')
		case r > 127:
			// 中文等非 ASCII 字符直接跳过：目录名保持 ASCII，避免跨工具链的编码问题。
		}
		if builder.Len() >= 24 {
			break
		}
	}
	return strings.Trim(builder.String(), "-")
}
