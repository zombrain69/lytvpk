package app

import (
	"fmt"
	"sort"
	"strings"

	"vpk-manager/internal/parser"
)

// XDR（xdReanimsBase）的优先级只看槽位，与 addonlist.txt 的加载顺序无关：
//
//   - 同角色同槽被两个 Mod 占用：游戏随机生效一个
//     （官方原文 "Combining both mods on same character slot won't work. Random one will be picked instead."）
//     这种 Mod 不能说"会播放"，只能标成"随机"。
//   - 该槽只有它一个：没有同槽竞争者，动作会按预期播放，标成"生效"。
//
// "两个 Mod 替换同一序列时低槽优先"要有序列级证据才能判（不同槽、同一序列），
// 本项目扫描拿不到序列名，所以不猜，只给出同槽这一层确定结论。

// xdrPriorityOccupant 是同槽统计用的占位记录。
type xdrPriorityOccupant struct {
	key   string // 去重键：文件名（小写）+ 大小，用来折叠根目录/工坊的同一份副本
	name  string
	title string
}

// xdrPriorityIndex 用全量列表算出每个 Mod 的 XDR 动作生效情况（键 = 文件路径）。
//
// 复杂度只和 XDR Mod 数量相关（真实库 2958 个 Mod 里 68 个），按需现算即可，不引入缓存。
func (a *App) xdrPriorityIndex() map[string]parser.XDRPriorityInfo {
	files := a.allVPKFilesSnapshot()
	if len(files) == 0 {
		return nil
	}

	bySlot := make(map[string][]xdrPriorityOccupant)
	for _, file := range files {
		if len(file.XDRSlots) == 0 || !xdrModCountsAsActive(a, file) {
			continue
		}
		occupant := xdrPriorityOccupant{
			key:   fmt.Sprintf("%s|%d", strings.ToLower(file.Name), file.Size),
			name:  file.Name,
			title: file.Title,
		}
		for _, slot := range file.XDRSlots {
			if slot.Slot <= 0 {
				continue
			}
			key := xdrSlotKey(slot.Character, slot.Slot)
			exists := false
			for _, item := range bySlot[key] {
				if item.key == occupant.key {
					exists = true
					break
				}
			}
			if !exists {
				bySlot[key] = append(bySlot[key], occupant)
			}
		}
	}

	index := make(map[string]parser.XDRPriorityInfo)
	for _, file := range files {
		if len(file.XDRSlots) == 0 || !xdrModCountsAsActive(a, file) {
			continue
		}
		selfKey := fmt.Sprintf("%s|%d", strings.ToLower(file.Name), file.Size)
		info := parser.XDRPriorityInfo{}
		for _, slot := range file.XDRSlots {
			if slot.Slot <= 0 {
				continue
			}
			status := parser.XDRSlotStatus{
				Character: slot.Character,
				Slot:      slot.Slot,
				SlotLabel: slot.SlotLabel,
				SlotName:  slot.SlotName,
				State:     "active",
			}
			for _, rival := range bySlot[xdrSlotKey(slot.Character, slot.Slot)] {
				if rival.key == selfKey {
					continue
				}
				status.Rivals = append(status.Rivals, parser.XDRSlotRival{Name: rival.name, Title: rival.title})
			}
			if len(status.Rivals) > 0 {
				status.State = "random"
				info.RandomSlots++
			} else {
				info.ActiveSlots++
			}
			info.Slots = append(info.Slots, status)
		}
		if len(info.Slots) == 0 {
			continue
		}
		sort.Slice(info.Slots, func(i, j int) bool {
			if info.Slots[i].Character != info.Slots[j].Character {
				return info.Slots[i].Character < info.Slots[j].Character
			}
			return info.Slots[i].Slot < info.Slots[j].Slot
		})
		switch {
		case info.RandomSlots == 0:
			info.State = "active"
		case info.ActiveSlots == 0:
			info.State = "random"
		default:
			info.State = "partial"
		}
		index[file.Path] = info
	}
	return index
}

// xdrSlotKey 生成"角色 + 槽位"的键（角色大小写不敏感）。
func xdrSlotKey(character string, slot int) string {
	return fmt.Sprintf("%s|%d", strings.ToLower(strings.TrimSpace(character)), slot)
}

// xdrModCountsAsActive 判断这个 Mod 会不会被游戏挂载：
// disabled 目录里的不加载；addonlist 明确记着 0（游戏内关闭）的也不加载。
// 没有任何 addonlist 记录的 VPK 游戏仍会挂载，所以算作生效候选。
func xdrModCountsAsActive(a *App, file VPKFile) bool {
	location := file.Location
	if computed := a.getLocationFromPath(file.Path); computed != "" {
		location = computed
	}
	if location == "disabled" {
		return false
	}
	if file.GameStateKnown && !file.GameEnabled {
		return false
	}
	return true
}

// attachXDRPriority 把算好的结论挂到列表上（只在有 XDR 槽位证据的文件上写）。
func attachXDRPriority(files []VPKFile, index map[string]parser.XDRPriorityInfo) []VPKFile {
	if len(index) == 0 {
		return files
	}
	for position := range files {
		if len(files[position].XDRSlots) == 0 {
			continue
		}
		if info, ok := index[files[position].Path]; ok {
			copied := info
			files[position].XDRPriority = &copied
		}
	}
	return files
}
