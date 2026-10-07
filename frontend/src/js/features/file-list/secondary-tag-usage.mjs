// 「常用子标签」排序。
//
// 背景（用户口径 2026-10-07）：筛选条的二级标签默认只显示一行，过去按字典序取前几个，
// 露出来的其实是偶然项（截图里的 #L4D360UI_* 之类）。后端现在同时给出每个子标签
// 命中了多少个 Mod（GetSecondaryTagCounts，key 为小写标签），这里按常用度降序排，
// 同分再按中文/字母序，保证顺序稳定、可预期。

/** 取某个标签的 Mod 命中数；缺数据时按 0 处理（排序是增强，不能因为缺计数报错）。 */
export function secondaryTagCount(counts, tag) {
  if (!counts || tag === undefined || tag === null) return 0;
  const value = counts[String(tag).toLowerCase()];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

/** 常用度优先、其次字典序。 */
export function compareSecondaryTagsByUsage(a, b, counts) {
  const countA = secondaryTagCount(counts, a);
  const countB = secondaryTagCount(counts, b);
  if (countA !== countB) return countB - countA;
  return String(a).localeCompare(String(b), "zh-CN");
}
