// "把选中的 Mod 放进哪个策略组"的纯逻辑：行数据、可点性判定与文案。
//
// 三种模式共用一套行结构：
//   add    —— 加入某个组（已经在组里的会跳过，全是重复时该行不可点）
//   remove —— 从某个组移出（一个都不在组里、或会把整组移空时不可点）
//   move   —— 从源组移到目标组（源组自己不能当目标；同样不允许把源组移空）
//
// 与 DOM 无关，node --test 覆盖。

import { formatGroupStrategy, normalizeGroupKey } from "./group-view.mjs";

export const GROUP_PICKER_MODES = {
  add: "add",
  remove: "remove",
  move: "move",
};

const MODE_VERBS = {
  add: "加入",
  remove: "移出",
  move: "移动",
};

function normalizeKeys(keys) {
  const result = [];
  const seen = new Set();
  (Array.isArray(keys) ? keys : []).forEach((key) => {
    const normalized = normalizeGroupKey(key);
    if (!normalized || seen.has(normalized)) return;
    seen.add(normalized);
    result.push(normalized);
  });
  return result;
}

function memberKeys(group) {
  return (Array.isArray(group?.members) ? group.members : [])
    .map((member) => normalizeGroupKey(member?.key ?? member))
    .filter(Boolean);
}

/** pickerRowDisabledReason 返回该行为什么不能点；可点时返回空字符串。 */
export function pickerRowDisabledReason(row, mode) {
  if (!row) return "没有可用的策略组";
  const { selectedInGroup, memberCount, wouldEmptyGroup, isSource, id } = row;
  switch (mode) {
    case GROUP_PICKER_MODES.add:
      if (selectedInGroup === 0) return "";
      if (selectedInGroup >= row.keyCount) return "选中的 Mod 都已经在这个组里";
      return "";
    case GROUP_PICKER_MODES.remove:
      if (selectedInGroup === 0) return "选中的 Mod 都不在这个组里";
      if (wouldEmptyGroup) return "会把该组成员全部移出（组至少要保留 1 个成员）";
      return "";
    case GROUP_PICKER_MODES.move:
      if (isSource) return `「${row.name}」就是当前所在的组`;
      if (selectedInGroup > 0 && wouldEmptyGroup) return "会把源组成员全部移出（组至少要保留 1 个成员）";
      return "";
    default:
      return id ? "" : "没有可用的策略组";
  }
}

/**
 * buildGroupPickerRows 把策略组列表转成选择器的行数据。
 * @param {{groups: any[], keys: string[], mode: string, sourceGroupId?: string}} options
 */
export function buildGroupPickerRows({ groups, keys, mode, sourceGroupId = "" } = {}) {
  const normalizedKeys = normalizeKeys(keys);
  const keySet = new Set(normalizedKeys);
  const list = Array.isArray(groups) ? groups : [];
  const nameById = new Map(
    list
      .map((group) => [String(group?.id || "").trim(), String(group?.name || "").trim()])
      .filter(([id]) => id),
  );
  const rows = [];
  list.forEach((group) => {
    const id = String(group?.id || "").trim();
    if (!id) return;
    const members = memberKeys(group);
    const selectedInGroup = members.filter((key) => keySet.has(key)).length;
    const row = {
      id,
      name: String(group?.name || id),
      strategy: String(group?.strategy || ""),
      // 模式写进行里：次要说明要按"加入 / 移出 / 移动"说人话。
      mode: mode || GROUP_PICKER_MODES.add,
      // 组权重 / 自动联动 / 上级分组：让用户在列表里就能分辨"这是哪一组"。
      tier: group?.tier ?? null,
      enforce: Boolean(group?.enforce),
      parentId: String(group?.parentId || ""),
      parentName: nameById.get(String(group?.parentId || "")) || "",
      memberCount: members.length,
      keyCount: normalizedKeys.length,
      selectedInGroup,
      missingSelected: Math.max(0, normalizedKeys.length - selectedInGroup),
      isSource: Boolean(sourceGroupId) && id === String(sourceGroupId),
      wouldEmptyGroup: selectedInGroup > 0 && selectedInGroup >= members.length,
      disabled: false,
      disabledReason: "",
    };
    row.disabledReason = pickerRowDisabledReason(row, mode);
    row.disabled = row.disabledReason !== "";
    rows.push(row);
  });
  return rows;
}

export function formatGroupPickerSummary({ rows, keyCount, mode, sourceName = "" } = {}) {
  const all = Array.isArray(rows) ? rows : [];
  const usable = all.filter((row) => !row.disabled).length;
  const count = Number(keyCount || 0);
  const prefix = `已选中 ${count} 个 Mod；`;
  if (mode === GROUP_PICKER_MODES.move) {
    return `${prefix}从「${sourceName || "当前组"}」移动到下面 ${usable} 个策略组之一`;
  }
  const verb = MODE_VERBS[mode] || "处理";
  return `${prefix}下面 ${all.length} 个策略组里 ${usable} 个可以${verb}`;
}

export function formatGroupPickerResult(result, { mode, keyCount = 0 } = {}) {
  if (mode === GROUP_PICKER_MODES.move) {
    const moved = (result?.moved || []).length;
    const already = (result?.alreadyInTarget || []).length;
    const target = result?.targetName || "目标组";
    if (moved === 0) {
      return `这些 Mod 本来就在「${target}」里，没有需要移动的成员`;
    }
    const total = Number(result?.targetTotal || 0);
    return (
      `已把 ${moved} 个 Mod 从「${result?.sourceName || "源组"}」移到「${target}」` +
      `（目标组现在共 ${total} 个成员` +
      (already > 0 ? `，另有 ${already} 个本来就在目标组里` : "") +
      "）"
    );
  }
  const name = result?.name || "该策略组";
  const total = (result?.members || []).length;
  const verb = MODE_VERBS[mode] || "处理";
  return `已把 ${Number(keyCount || 0)} 个 Mod ${verb}「${name}」（现在共 ${total} 个成员）`;
}

export function groupPickerConfirmLabel(mode) {
  switch (mode) {
    case GROUP_PICKER_MODES.remove:
      return "移出这个组";
    case GROUP_PICKER_MODES.move:
      return "移动到这个组";
    default:
      return "加入这个组";
  }
}

export function groupPickerTitle(mode) {
  switch (mode) {
    case GROUP_PICKER_MODES.remove:
      return "从策略组移出";
    case GROUP_PICKER_MODES.move:
      return "移动到其它策略组";
    default:
      return "加入策略组";
  }
}

/**
 * groupPickerRowSearchText 把一行拼成搜索用的文本（组名 / 上级分组 / 策略名）。
 * 搜索是"够便捷"的核心：组多起来以后，找组只能靠滚动会很难用。
 */
export function groupPickerRowSearchText(row) {
  if (!row) return "";
  return [row.name, row.parentName, formatGroupStrategy(row.strategy)]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
}

/** groupPickerRowTags 生成行上的小标签：策略之外的可辨识信息（权重 / 自动联动 / 上级分组）。 */
export function groupPickerRowTags(row) {
  if (!row) return [];
  const tags = [formatGroupStrategy(row.strategy)];
  if (row.tier !== null && row.tier !== undefined && row.tier !== "") {
    tags.push(`权重 ${row.tier}`);
  }
  if (row.enforce) tags.push("自动联动");
  if (row.parentName) tags.push(`上级：${row.parentName}`);
  return tags.filter(Boolean);
}

/** groupPickerRowDetail 生成次要说明：成员数 + 选中里已有多少 + 禁用原因。 */
export function groupPickerRowDetail(row) {
  if (!row) return "";
  const parts = [`${row.memberCount} 个成员`];
  // 只补"部分命中"的信息：全部命中 / 一个都不在的情况由 disabledReason 说明，
  // 否则同一件事会在同一行里出现两遍（真实界面里出现过）。
  if (row.selectedInGroup > 0 && row.selectedInGroup < row.keyCount) {
    parts.push(`选中里已有 ${row.selectedInGroup} 个`);
  }
  if (row.disabled) parts.push(row.disabledReason);
  return parts.join(" · ");
}

/**
 * sortGroupPickerRows 让最可能被点的组排在最前：
 * 可点优先 → 含选中项的优先 → 名称（中文按拼音）→ 原顺序兜底。
 */
export function sortGroupPickerRows(rows = []) {
  const list = Array.isArray(rows) ? rows : [];
  return list
    .map((row, index) => ({ row, index }))
    .sort((left, right) => {
      if (left.row.disabled !== right.row.disabled) return left.row.disabled ? 1 : -1;
      const leftHit = left.row.selectedInGroup > 0 ? 1 : 0;
      const rightHit = right.row.selectedInGroup > 0 ? 1 : 0;
      if (leftHit !== rightHit) return rightHit - leftHit;
      const byName = String(left.row.name || "").localeCompare(String(right.row.name || ""), "zh-Hans-CN");
      if (byName !== 0) return byName;
      return left.index - right.index;
    })
    .map((entry) => entry.row);
}

/**
 * filterGroupPickerRows 过滤搜索结果。
 * relevantOnly 只保留"含选中 Mod"的组 —— 移出 / 换组时通常只关心这些组。
 */
export function filterGroupPickerRows(rows = [], { query = "", relevantOnly = false } = {}) {
  const list = Array.isArray(rows) ? rows : [];
  const keyword = String(query || "").trim().toLowerCase();
  return list.filter((row) => {
    if (relevantOnly && !(row?.selectedInGroup > 0)) return false;
    if (!keyword) return true;
    return groupPickerRowSearchText(row).includes(keyword);
  });
}

/** formatGroupPickerVisibleSummary 说明"当前显示了几个 / 一共几个 / 几个能点"。 */
export function formatGroupPickerVisibleSummary({ total = 0, shown = 0, usable = 0, hiddenByFilter = 0 } = {}) {
  const parts = [`显示 ${shown} / ${total} 个组`, `${usable} 个可操作`];
  if (hiddenByFilter > 0) parts.push(`已按筛选隐藏 ${hiddenByFilter} 个`);
  return parts.join(" · ");
}

/**
 * nextSelectableRowId 键盘上下键的落点：只在可点的行之间移动，循环滚动。
 * currentId 为空时返回第一个可点的行。
 */
export function nextSelectableRowId(rows = [], currentId = "", delta = 1) {
  const selectable = (Array.isArray(rows) ? rows : []).filter((row) => row && !row.disabled);
  if (selectable.length === 0) return "";
  const currentIndex = selectable.findIndex((row) => String(row.id) === String(currentId || ""));
  // 还没有高亮行时：↓ 落到第一个，↑ 落到最后一个（符合列表的直觉）。
  if (currentIndex < 0) return delta >= 0 ? selectable[0].id : selectable[selectable.length - 1].id;
  const step = delta >= 0 ? 1 : -1;
  const nextIndex = (currentIndex + step + selectable.length) % selectable.length;
  return selectable[nextIndex].id;
}
