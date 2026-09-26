// 策略组树形层级的纯函数层（node --test 覆盖）。
//
// 约定：扁平视图仍是默认视图。没有任何"上级分组"时，展开结果与扁平列表逐项一致，
// 只是每项多带一个 depth=1（渲染时不产生缩进）。

/**
 * flattenStrategyGroupTree 把后端 ListModStrategyGroupTree 的结果展开成
 * [{ group, depth }] 的渲染序列；输入不是树时退化为扁平列表。
 */
export function flattenStrategyGroupTree(tree, fallbackGroups = []) {
  const rows = [];
  const visit = (nodes, depth) => {
    if (!Array.isArray(nodes)) return;
    nodes.forEach((node) => {
      if (!node || !node.group) return;
      rows.push({ group: node.group, depth: Number(node.depth) > 0 ? Number(node.depth) : depth });
      visit(node.children, depth + 1);
    });
  };
  if (Array.isArray(tree) && tree.length > 0) {
    visit(tree, 1);
    return rows;
  }
  return (Array.isArray(fallbackGroups) ? fallbackGroups : [])
    .filter((group) => group && group.id)
    .map((group) => ({ group, depth: 1 }));
}

/** buildParentOptions 生成"上级分组"下拉的候选：排除自己与自己的下级，避免直接制造环。 */
export function buildParentOptions(rows, currentGroupId) {
  const blocked = new Set([currentGroupId]);
  let changed = true;
  while (changed) {
    changed = false;
    rows.forEach((row) => {
      const parentId = row.group?.parentId || "";
      if (parentId && blocked.has(parentId) && !blocked.has(row.group.id)) {
        blocked.add(row.group.id);
        changed = true;
      }
    });
  }
  return rows
    .filter((row) => row.group?.id && !blocked.has(row.group.id))
    .map((row) => ({ id: row.group.id, label: `${"— ".repeat(Math.max(row.depth - 1, 0))}${row.group.name || row.group.id}` }));
}

/** 策略组树的最大层级，与后端 CreateModStrategyGroupChild / MoveModStrategyGroup 保持一致。 */
export const STRATEGY_GROUP_MAX_DEPTH = 4;

/** 把行序列规约成 [{ id, depth }]，拖放判定与重排都基于它（避免重复处理 group 结构）。 */
function entryList(rows) {
  return (Array.isArray(rows) ? rows : [])
    .filter((row) => row?.group?.id)
    .map((row) => ({
      id: String(row.group.id),
      name: String(row.group.name || row.group.id),
      parentId: String(row.group.parentId || ""),
      depth: Number(row.depth) > 0 ? Number(row.depth) : 1,
    }));
}

/** 子树结束位置：行序是深度优先，子树一定连续。 */
function subtreeEnd(entries, startIndex) {
  const depth = entries[startIndex]?.depth || 1;
  let end = startIndex + 1;
  while (end < entries.length && (entries[end]?.depth || 1) > depth) end += 1;
  return end;
}

/**
 * resolveStrategyGroupDrop 判定一次拖放的落点是否合法。
 *
 * position：
 *   - "inside"：放进目标组里面，成为它的最后一个子组；
 *   - "before" / "after"：排在目标组的同一个上级下，位于它前 / 后（同级排序）；
 *   - "root"：移动到顶层末尾（拖到列表空白处）。
 *
 * 返回 {ok:true, parentId, position, newDepth, noop} 或 {ok:false, reason}。
 * 拒绝自己拖自己、拖进自己的子树（成环）、以及超过最大层级的拖放。
 */
export function resolveStrategyGroupDrop(rows, draggedId, targetId, position = "inside") {
  const entries = entryList(rows);
  const dragged = String(draggedId || "");
  const index = entries.findIndex((entry) => entry.id === dragged);
  if (index < 0) return { ok: false, reason: "找不到要移动的策略组" };

  const target = targetId === null || targetId === undefined ? "" : String(targetId);
  const mode = position === "root" || !target ? "root" : position;
  const end = subtreeEnd(entries, index);
  const depths = entries.slice(index, end).map((entry) => entry.depth);
  const height = Math.max(...depths) - entries[index].depth;

  if (mode === "root") {
    return {
      ok: true,
      parentId: "",
      position: "root",
      newDepth: 1,
      noop: entries[index].parentId === "" && end === entries.length,
    };
  }

  const targetIndex = entries.findIndex((entry) => entry.id === target);
  if (targetIndex < 0) return { ok: false, reason: "找不到拖放目标策略组" };
  if (targetIndex === index) return { ok: false, reason: "不能把组拖到它自己身上" };
  if (targetIndex > index && targetIndex < end) {
    return { ok: false, reason: "不能把组拖进它自己的子组（会形成循环层级）" };
  }

  const targetEntry = entries[targetIndex];
  const targetEnd = subtreeEnd(entries, targetIndex);
  const parentId = mode === "inside" ? targetEntry.id : targetEntry.parentId;
  const newDepth = mode === "inside" ? targetEntry.depth + 1 : targetEntry.depth;

  if (newDepth + height > STRATEGY_GROUP_MAX_DEPTH) {
    return {
      ok: false,
      reason: `最多支持 ${STRATEGY_GROUP_MAX_DEPTH} 层分组，这次拖放会把「${entries[index].name}」放到第 ${newDepth + height} 层`,
    };
  }

  const alreadyChild = entries[index].parentId === parentId;
  const noop =
    (mode === "inside" && alreadyChild && end === targetEnd) ||
    (mode === "before" && alreadyChild && end === targetIndex) ||
    (mode === "after" && alreadyChild && index === targetEnd);

  return { ok: true, parentId, position: mode, newDepth, noop };
}

/**
 * applyStrategyGroupDropOrder 按落点算出新的完整组顺序（拖放排序最终写盘的就是这个顺序）。
 * 拖动的是整棵子树，因此被拖的组连同它的下级会一起移动。
 */
export function applyStrategyGroupDropOrder(rows, draggedId, targetId, position = "inside") {
  const entries = entryList(rows).map((entry) => ({ ...entry }));
  const dragged = String(draggedId || "");
  const index = entries.findIndex((entry) => entry.id === dragged);
  if (index < 0) return null;

  const target = targetId === null || targetId === undefined ? "" : String(targetId);
  const mode = position === "root" || !target ? "root" : position;
  const end = subtreeEnd(entries, index);
  const block = entries.slice(index, end);
  const rest = entries.slice(0, index).concat(entries.slice(end));

  let insertAt = rest.length;
  if (mode !== "root") {
    const targetIndex = rest.findIndex((entry) => entry.id === target);
    if (targetIndex < 0) return null;
    const targetEnd = subtreeEnd(rest, targetIndex);
    if (mode === "before") insertAt = targetIndex;
    else if (mode === "after") insertAt = targetEnd;
    else insertAt = targetEnd;
  }

  return rest.slice(0, insertAt).concat(block, rest.slice(insertAt)).map((entry) => entry.id);
}

/** formatStrategyGroupDropMessage 生成拖放完成后的提示文案。 */
export function formatStrategyGroupDropMessage(rows, draggedId, targetId, position = "inside") {
  const entries = entryList(rows);
  const dragged = entries.find((entry) => entry.id === String(draggedId || ""));
  const target = entries.find((entry) => entry.id === String(targetId || ""));
  const name = dragged?.name || String(draggedId || "");
  const mode = position === "root" || !targetId ? "root" : position;
  if (mode === "root") return `已把「${name}」移动到顶层`;
  const targetName = target?.name || String(targetId || "");
  if (mode === "before") return `已把「${name}」排到「${targetName}」前面`;
  if (mode === "after") return `已把「${name}」排到「${targetName}」后面`;
  return `已把「${name}」移动到「${targetName}」下面`;
}
