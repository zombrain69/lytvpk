// 加载顺序预览的条目状态（纯函数，可单测）。
//
// - existing：addonlist 条目对应的文件在 addons / workshop 目录里
// - disabled：条目对应的文件被放进了 disabled 目录（不会参与加载）
// - missing：条目还在 addonlist.txt 里，但磁盘上已经找不到文件
//
// 另外统计「扫描到、但还没写进 addonlist.txt」的 Mod 数量（未记录）。

export const LOAD_ORDER_ENTRY_STATUS = {
  existing: "existing",
  disabled: "disabled",
  missing: "missing",
};

/** normalizeLoadOrderKey 与 addonlist.txt 的键保持同一套写法（小写、反斜杠）。 */
export function normalizeLoadOrderKey(value) {
  return String(value || "")
    .replaceAll("/", "\\")
    .toLowerCase();
}

function addonListKeyForFile(file) {
  if (!file?.name) return "";
  const name = normalizeLoadOrderKey(file.name);
  if (!name) return "";
  return file.location === "workshop" ? `workshop\\${name}` : name;
}

/**
 * classifyLoadOrderEntries 返回每个条目的状态与统计。
 * @param {Array<{key: string}>} entries addonlist 条目（顺序表）
 * @param {Array<{name: string, location: string}>} files 当前扫描到的 VPK
 */
export function classifyLoadOrderEntries(entries, files) {
  const activeKeys = new Map();
  const disabledKeys = new Map();

  for (const file of files || []) {
    const key = addonListKeyForFile(file);
    if (!key) continue;
    if (file.location === "disabled") {
      if (!disabledKeys.has(key)) disabledKeys.set(key, file);
      continue;
    }
    if (file.location === "root" || file.location === "workshop") {
      if (!activeKeys.has(key)) activeKeys.set(key, file);
    }
  }

  const statuses = new Map();
  for (const entry of entries || []) {
    const key = normalizeLoadOrderKey(entry?.key);
    if (!key || statuses.has(key)) continue;
    statuses.set(
      key,
      activeKeys.has(key)
        ? LOAD_ORDER_ENTRY_STATUS.existing
        : disabledKeys.has(key)
          ? LOAD_ORDER_ENTRY_STATUS.disabled
          : LOAD_ORDER_ENTRY_STATUS.missing,
    );
  }

  let invalidCount = 0;
  statuses.forEach((status) => {
    if (status !== LOAD_ORDER_ENTRY_STATUS.existing) invalidCount += 1;
  });

  const unrecorded = [...activeKeys.keys()].filter((key) => !statuses.has(key)).length;

  return { statuses, invalidCount, unrecorded };
}
