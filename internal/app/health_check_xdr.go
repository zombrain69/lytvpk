package app

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"vpk-manager/internal/parser"
)

const (
	// modHealthKindXDRSlotCollision：同一角色、同一 slot 被两个以上 Mod 占用。
	//
	// 依据（基础包作者 xdshot 的原文）：
	//   "Combining both mods on same character slot won't work. Random one will be picked instead."
	// 也就是说同槽不是"谁优先"，而是**随机**生效一个 —— 必须调整槽位或只留一个。
	modHealthKindXDRSlotCollision = "xdr_slot_collision"
	// modHealthKindXDRMissingBase：装了 XDR 动作 Mod，但没有（或在游戏内关闭了）基础包 xdReanimsBase。
	//
	// 依据：基础包是"3-rd person re-animations mods for survivors and infected"的 Base dependency，
	// 没有它，models/xdreanims/*.mdl 不会被任何模型引用，动作不会生效。
	modHealthKindXDRMissingBase = "xdr_missing_base"
	// modHealthKindXDRVariantPack：动作包 —— 包里带 loader 模型 + 一堆变体模型，
	// 实际生效的是编译进 loader 的那一版，换版本需要解包重命名/重新打包。
	modHealthKindXDRVariantPack = "xdr_variant_pack"
)

// xdReanimsBaseWorkshopID 是基础包的工坊 ID（名字叫 xdReanimsBase，作者 xdshot）。
// 工坊作品被重新打包/改名时按标题兜底，所以两个判据都要看。
const xdReanimsBaseWorkshopID = "2121557118"

// xdrHealthSlot 是"某角色的某个 slot 归谁"的一条记录，用来判定同槽冲突。
type xdrHealthSlot struct {
	Character string
	Slot      int
	Name      string
	Title     string
	Path      string
	Location  string
	Size      int64
}

// checkXDRHealth 基于已解析的 Mod 列表做 XDR（动作）体检。
//
// 为什么用已解析列表而不是再扫盘：slot 证据在 VPK 内部（models/xdreanims/<模型>_slot_NNN.mdl），
// 只有解析过的文件才有；扫描缓存里已经存好了这些信息，直接复用即可，零额外 I/O。
func (a *App) checkXDRHealth(files []VPKFile, report *ModHealthReport) {
	if report == nil {
		return
	}
	slots := make(map[string][]xdrHealthSlot)
	seen := make(map[string]bool)
	slotModCount := 0
	var baseFile *VPKFile

	for index := range files {
		file := files[index]
		location := a.getLocationFromPath(file.Path)
		if location == "" {
			location = file.Location
		}
		if location == "disabled" {
			continue
		}
		if isXDReanimsBaseFile(file) {
			candidate := file
			baseFile = &candidate
		}
		if len(file.XDRSlots) == 0 {
			continue
		}
		slotModCount++
		for _, slot := range file.XDRSlots {
			if slot.Slot <= 0 {
				continue
			}
			// 同一个文件在根目录与 workshop 各一份时（真实库里很常见）只算一次。
			key := fmt.Sprintf("%s|%d|%s|%d", strings.ToLower(slot.Character), slot.Slot,
				strings.ToLower(file.Name), file.Size)
			if seen[key] {
				continue
			}
			seen[key] = true
			groupKey := fmt.Sprintf("%s|%d", strings.ToLower(slot.Character), slot.Slot)
			slots[groupKey] = append(slots[groupKey], xdrHealthSlot{
				Character: slot.Character,
				Slot:      slot.Slot,
				Name:      file.Name,
				Title:     file.Title,
				Path:      file.Path,
				Location:  location,
				Size:      file.Size,
			})
		}
	}

	// 1) 同角色同槽：官方规则是随机生效一个，必须让用户处理。
	//
	// 聚合口径：按"槽位 + 冲突的 Mod 组合"合并 —— 一个"8 个角色通用"的动作包和另一个同样的包撞车时，
	// 8 个角色是同一件事，报 8 行只会把体检列表刷爆，所以合成一行并列出受影响角色。
	type collisionGroup struct {
		slot       int
		characters []string
		mods       []xdrHealthSlot
	}
	groups := make(map[string]*collisionGroup)
	characterKeys := make([]string, 0, len(slots))
	for key := range slots {
		characterKeys = append(characterKeys, key)
	}
	sort.Strings(characterKeys)
	for _, key := range characterKeys {
		group := slots[key]
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].Name < group[j].Name })
		modKeys := make([]string, 0, len(group))
		for _, item := range group {
			modKeys = append(modKeys, item.Name+"#"+strconv.FormatInt(item.Size, 10))
		}
		aggregateKey := fmt.Sprintf("%03d|%s", group[0].Slot, strings.Join(modKeys, "\x00"))
		existing := groups[aggregateKey]
		if existing == nil {
			existing = &collisionGroup{slot: group[0].Slot, mods: group}
			groups[aggregateKey] = existing
		}
		existing.characters = append(existing.characters, group[0].Character)
	}

	aggregateKeys := make([]string, 0, len(groups))
	for key := range groups {
		aggregateKeys = append(aggregateKeys, key)
	}
	sort.Strings(aggregateKeys)
	for _, key := range aggregateKeys {
		group := groups[key]
		characterText := strings.Join(group.characters, "、")
		if len(group.characters) > 4 {
			characterText = strings.Join(group.characters[:4], "、") +
				fmt.Sprintf(" 等 %d 个角色", len(group.characters))
		}
		names := make([]string, 0, len(group.mods))
		for _, item := range group.mods {
			label := item.Name
			if title := strings.TrimSpace(item.Title); title != "" {
				label += "（" + title + "）"
			}
			names = append(names, label)
		}
		designation := parser.XDRSlotDesignationFor(group.slot)
		hint := ""
		if designation.Name != "" {
			hint = fmt.Sprintf("；官方对该槽的建议用途是 %s", designation.Name)
		}
		report.addIssueWithTarget(group.mods[0].Path, modHealthKindXDRSlotCollision, "warning",
			fmt.Sprintf("slot %03d · %s", group.slot, characterText),
			group.mods[0].Path, group.mods[0].Location,
			fmt.Sprintf("XDR 同槽冲突：%d 个已启用的 Mod 同时占用 slot %03d（影响 %s）—— %s。官方规则是同角色同槽只会随机生效一个（不是按加载顺序，也不是按谁更新），建议把其中一个改到别的 slot 或只保留一个%s",
				len(group.mods), group.slot, characterText, strings.Join(names, "、"), hint))
	}

	// 2) 基础包依赖。
	if slotModCount > 0 {
		switch {
		case baseFile == nil:
			report.addIssue(modHealthKindXDRMissingBase, "warning", "xdReanimsBase", "", "",
				fmt.Sprintf("检测到 %d 个 XDR 动作 Mod，但没有找到基础包 xdReanimsBase：动作不会被游戏加载。请先安装并启用基础包（工坊 %s），再装各个槽位动作", slotModCount, xdReanimsBaseWorkshopID))
		case baseFile.GameStateKnown && !baseFile.GameEnabled:
			report.addIssue(modHealthKindXDRMissingBase, "warning", baseFile.Name, baseFile.Path, baseFile.Location,
				fmt.Sprintf("基础包 xdReanimsBase 在游戏内被关闭了：那 %d 个 XDR 动作 Mod 都不会生效，请在游戏内把它打开", slotModCount))
		}
	}
}

// isXDReanimsBaseFile 判断某个 Mod 是不是 XDR 基础包（名称/标题含 xdReanimsBase，或就是那个工坊 ID）。
func isXDReanimsBaseFile(file VPKFile) bool {
	haystack := strings.ToLower(file.Name + " " + file.Title)
	return strings.Contains(haystack, "xdreanimsbase") ||
		strings.Contains(haystack, xdReanimsBaseWorkshopID)
}

// checkXDRVariantPack 在深度扫描时识别"动作包"：包里是 models/xdreanims/ 下的一堆模型，
// 但没有 slot 命名 —— 这类包靠 loader（如 survivor_incap.mdl / survivor_die.mdl）挑选变体，
// 实际生效的只有编译进去的那一版，管理器不能替用户切换。
//
// seen 用来去重：同一个包在根目录与 workshop 各一份时只报一次（键 = 文件名+大小）。
func (a *App) checkXDRVariantPack(displayName string, filePath string, location string, size int64, entries []string, seen map[string]bool, report *ModHealthReport) {
	if report == nil || len(entries) == 0 {
		return
	}
	modelCount := 0
	hasSlotFile := false
	for _, entry := range entries {
		normalized := strings.ToLower(strings.ReplaceAll(entry, "\\", "/"))
		if !strings.HasPrefix(normalized, "models/xdreanims/") {
			continue
		}
		if strings.HasSuffix(normalized, ".mdl") {
			modelCount++
		}
		if strings.Contains(path.Base(normalized), "_slot_") {
			hasSlotFile = true
		}
	}
	if hasSlotFile || modelCount < 4 {
		return
	}
	if seen != nil {
		key := strings.ToLower(displayName) + "|" + strconv.FormatInt(size, 10)
		if seen[key] {
			return
		}
		seen[key] = true
	}
	report.addIssue(modHealthKindXDRVariantPack, "info", displayName, filePath, location,
		fmt.Sprintf("动作包：models/xdreanims 下有 %d 个模型但没有 slot 命名 —— 这类包靠自己的 loader 挑选变体，实际生效的只有编译进去的那一版；想换版本需要解包重命名/重新打包，管理器无法切换", modelCount))
}
