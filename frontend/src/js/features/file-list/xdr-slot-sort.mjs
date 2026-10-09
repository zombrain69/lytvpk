// 动作槽位排序的比较规则（与 DOM 无关，node --test 覆盖）。
//
// 排序键：slot = 该 Mod 全部 XDR 槽位里最小的槽位号（1–48；多角色/多槽取最小）。
// 规则：
//   1. 两者都有槽位：按槽位号升序；相同按文件名（中文 collation + 数字感知，稳定）；
//   2. 只有一方有槽位：有槽位的排前面（与优先级排序"有分层的排前"同精神）；
//   3. 都没有槽位：按文件名。
// 方向取反由调用方用 applySortOrder 套一层（与其它排序一致）。

import { compareNames } from "./priority-sort.mjs";

/** minXDRSlot 返回该 Mod 的最小槽位号；没有槽位/字段异常时返回 null。 */
export function minXDRSlot(file) {
  const slots = Array.isArray(file?.xdrSlots) ? file.xdrSlots : [];
  let min = null;
  for (const slot of slots) {
    const value = Number(slot?.slot);
    if (!Number.isFinite(value) || value <= 0) continue;
    if (min === null || value < min) min = value;
  }
  return min;
}

/**
 * compareByXDRSlot 比较两个"装饰过"的条目（{ slot, name }；name 供平局兜底）。
 * 升序语义；调用方负责按 sortOrder 取反。
 */
export function compareByXDRSlot(left, right) {
  const leftSlot = Number.isInteger(left?.slot) ? left.slot : null;
  const rightSlot = Number.isInteger(right?.slot) ? right.slot : null;

  if (leftSlot !== null && rightSlot !== null) {
    if (leftSlot !== rightSlot) return leftSlot - rightSlot;
    return compareNames(left, right);
  }
  if ((leftSlot !== null) !== (rightSlot !== null)) {
    return leftSlot !== null ? -1 : 1;
  }
  return compareNames(left, right);
}
