// 优先级排序的比较规则（与 DOM 无关，node --test 覆盖）。
//
// 排序键：
//   layer = 有效分层（priority.json 的自身分层 或 策略组权重，越小越先加载；来自 GetModPriorityPlan）
//   order = addonlist.txt 中的 0 基顺序号（游戏侧的真实顺序）
//   name  = 文件名（兜底）
//
// 规则：
//   1. 两者都有 layer：按 layer 升序（越小越靠前，即越先加载）；
//   2. layer 相同且都有 order：按 order 升序（同层内保持游戏的真实相对顺序）；
//   3. 只有一方有 layer：有 layer 的排在前面（分层是显式意图；未写入 addonlist 的 Mod 排末尾）；
//   4. 都没有 layer：按文件名排序（中文按 zh-CN + 数字感知）；
//   5. 完全相等返回 0，交由调用方的稳定排序保持原有相对顺序。

function hasLayer(entry) {
  return Number.isInteger(entry?.layer);
}

function compareNames(left, right) {
  return String(left?.name || "")
    .toLowerCase()
    .localeCompare(String(right?.name || "").toLowerCase(), "zh-CN", {
      numeric: true,
      sensitivity: "accent",
    });
}

export function compareByPriority(left, right) {
  const leftHasLayer = hasLayer(left);
  const rightHasLayer = hasLayer(right);

  if (leftHasLayer && rightHasLayer) {
    if (left.layer !== right.layer) return left.layer - right.layer;
    const leftOrder = Number.isInteger(left.order) ? left.order : null;
    const rightOrder = Number.isInteger(right.order) ? right.order : null;
    if (leftOrder !== null && rightOrder !== null && leftOrder !== rightOrder) {
      return leftOrder - rightOrder;
    }
    return compareNames(left, right);
  }
  if (leftHasLayer !== rightHasLayer) {
    return leftHasLayer ? -1 : 1;
  }
  return compareNames(left, right);
}

/** 把列表按优先级稳定排序，返回新数组（不修改入参）。 */
export function sortByPriority(entries) {
  const list = Array.isArray(entries) ? [...entries] : [];
  return list.sort(compareByPriority);
}
