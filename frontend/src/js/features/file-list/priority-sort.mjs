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

// 复用一个 Intl.Collator：String.prototype.localeCompare 带 options 每次调用都会
// 新建一个 collator，真机 2298 条排序 ≈ 25k 次比较 → 84ms 长任务；复用后只做比较本身。
const NAME_COLLATOR = new Intl.Collator("zh-CN", {
  numeric: true,
  sensitivity: "accent",
});

/** compareNames 按文件显示名比较（中文 collation + 数字感知），大小写不敏感。 */
export function compareNames(left, right) {
  const leftName = left?.name ? String(left.name).toLowerCase() : "";
  const rightName = right?.name ? String(right.name).toLowerCase() : "";
  return NAME_COLLATOR.compare(leftName, rightName);
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

/**
 * nextSortState 决定"点某个排序项"之后的 (type, order)。
 *
 * 规则（与工具栏/菜单文案一一对应）：
 *   · 点当前正在用的那一项 → 在 顺序 / 逆序 之间切换；
 *   · 点别的项 → 用该项的默认方向：日期/大小/模型复杂度是"从大到小/最新/高到低"（desc），
 *     文件名/优先级是"顺序"（asc）。
 * 抽成纯函数是为了让"优先级排序点第二下没反应"这类回归能被单测钉住。
 */
export function nextSortState(currentType, currentOrder, type) {
  if (currentType === type) {
    return { type, order: currentOrder === "asc" ? "desc" : "asc" };
  }
  const defaultDesc = type === "date" || type === "size" || type === "modelComplexity";
  return { type, order: defaultDesc ? "desc" : "asc" };
}

/** applySortOrder 把"升序比较结果"换算成当前方向的比较结果（降序取反，平局仍是 0）。 */
export function applySortOrder(result, order) {
  if (order !== "desc") return result;
  return result === 0 ? 0 : -result;
}
