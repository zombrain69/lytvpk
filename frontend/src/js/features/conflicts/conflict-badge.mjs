// 冲突角标的纯文案层（node --test 覆盖）。
//
// 后端 GetConflictBadges 已按路径给出每个 Mod 的冲突/覆盖文件数与严重度，
// 这里只负责把它变成列表上的一行短标签，以及决定是否需要提示。

export function formatConflictBadgeLabel(badge) {
  if (!badge) return "";
  const conflicts = Number(badge.conflictFiles || 0);
  const overrides = Number(badge.overrideFiles || 0);
  const parts = [];
  if (conflicts > 0) parts.push(`${conflicts} 处冲突`);
  if (overrides > 0) parts.push(`${overrides} 处覆盖`);
  return parts.join(" · ");
}

export function conflictBadgeLevel(badge) {
  if (!badge) return "none";
  if (Number(badge.conflictFiles || 0) > 0) {
    return badge.severity === "critical"
      ? "critical"
      : badge.severity === "warning"
        ? "warning"
        : "info";
  }
  if (Number(badge.overrideFiles || 0) > 0) return "override";
  return "none";
}

export function shouldShowConflictBadge(badge) {
  return conflictBadgeLevel(badge) !== "none";
}

/**
 * buildConflictBadgeMap 把 GetConflictBadges 的结果转成两张索引：
 * byPath（完整路径）与 byKey（addonlist 归一化键，小写、"\" 分隔）。
 */
export function buildConflictBadgeMap(badges) {
  const byPath = new Map();
  const byKey = new Map();
  if (!Array.isArray(badges)) return { byPath, byKey };
  badges.forEach((badge) => {
    if (!badge) return;
    if (badge.path) byPath.set(String(badge.path), badge);
    const key = String(badge.key || "")
      .trim()
      .replaceAll("/", "\\")
      .toLowerCase();
    if (key) byKey.set(key, badge);
  });
  return { byPath, byKey };
}

/** filePriorityKeys 复刻 Go 侧 addonlist 键推导顺序，便于按 key 命中角标。 */
export function filePriorityKeys(file, currentDirectory = "") {
  const normalize = (value) =>
    String(value || "")
      .trim()
      .replaceAll("/", "\\")
      .replace(/^\.\\/, "")
      .toLowerCase();
  const name = normalize(file?.name);
  const path = normalize(file?.path);
  const root = normalize(currentDirectory);
  const keys = [];
  if (root && path.startsWith(`${root}\\`)) keys.push(path.slice(root.length + 1));
  if (file?.location === "workshop" && name) keys.push(`workshop\\${name}`);
  if (file?.location === "disabled" && name) keys.push(name);
  if (name) keys.push(name);
  return [...new Set(keys.filter(Boolean))];
}
