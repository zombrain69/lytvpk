package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// XDR 槽位改名（"把某个 Mod 的动作移到空槽"）的条目级规划。
//
// 背景：官方对"同角色同槽冲突"的处置建议是 "unpack and move the anim file to
// another slot" —— 把 VPK 里 `<角色 token>_slot_<NNN>` 的槽位文件改成新的槽号。
// 框架按**文件路径**里的槽号注册动作，所以只需要改文件名里的数字，
// 条目内容（.mdl / .ani 等）原样保留。
// XDRSlotRename 描述一条槽位改名：某条目名（正斜杠分隔的 VPK 内路径）→ 新条目名。
type XDRSlotRename struct {
	From string
	To   string
}

// PlanXDRSlotRenames 为一份条目名清单规划「角色 character 的 fromSlot → toSlot」改名。
//
// 规则（与扫描口径 parser/xdr_parser.go 保持一致）：
//   - 只认条目 **basename** 里的槽位模式（目录名里的 "xxx_slot_NNN_anims" 这类
//     源文件工作目录不动，它们不是游戏加载的资源路径）；
//   - 角色按别名表识别（bill / namvet → Bill 等），与 evidence 同源；
//   - 槽号按数值比较（007 与 7 等价）；新槽号零填充到原数字宽度（至少 1 位）；
//   - 一条 basename 里有多个槽位模式时，取第一个"角色+槽号都命中"的。
//
// 也就是说：这份规划只负责"改哪些名、改成什么"，值不值得改（撞名、占用）由调用方判定。
func PlanXDRSlotRenames(entryNames []string, character string, fromSlot int, toSlot int) []XDRSlotRename {
	character = strings.TrimSpace(character)
	if character == "" || fromSlot < 1 || fromSlot > 48 || toSlot < 1 || toSlot > 48 || fromSlot == toSlot {
		return nil
	}
	renames := make([]XDRSlotRename, 0, 4)
	for _, name := range entryNames {
		if newName, ok := xdrSlotRenamedEntryName(name, character, fromSlot, toSlot); ok {
			renames = append(renames, XDRSlotRename{From: name, To: newName})
		}
	}
	return renames
}

// xdrSlotRenamedEntryName 判断单个条目名是否需要改名，并返回新名字。
func xdrSlotRenamedEntryName(name string, character string, fromSlot int, toSlot int) (string, bool) {
	slash := strings.LastIndex(name, "/")
	base := name[slash+1:]
	if base == "" {
		return "", false
	}
	lowerPath := strings.ToLower(name)
	matches := xdrSlotPattern.FindAllStringSubmatchIndex(base, -1)
	for _, loc := range matches {
		token := strings.ToLower(base[loc[2]:loc[3]])
		slot, err := strconv.Atoi(base[loc[4]:loc[5]])
		if err != nil || slot < 1 || slot > 48 || slot != fromSlot {
			continue
		}
		detected, _ := detectXDRCharacter(token, lowerPath)
		if detected != character {
			continue
		}
		// 数字段按原宽度零填充：007 → 044、7 → 44。
		newNumber := fmt.Sprintf("%0*d", loc[5]-loc[4], toSlot)
		return name[:slash+1] + base[:loc[4]] + newNumber + base[loc[5]:], true
	}
	return "", false
}
