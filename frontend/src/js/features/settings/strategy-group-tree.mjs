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
