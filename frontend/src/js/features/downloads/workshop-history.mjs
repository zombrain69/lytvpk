// 「工坊解析历史」的纯逻辑（对齐上游 b635ea3，按本项目习惯拆出可单测的部分）。
// 后端存的是整份结果快照，这里只负责：把后端数据规整成界面能用的形状、
// 把一次解析结果转成历史条目、以及生成列表标题。

/** normalizeHistoryItems 过滤掉没有 rootId 的脏数据，并把字段类型收敛。 */
export function normalizeHistoryItems(items) {
  if (!Array.isArray(items)) return [];
  return items
    .filter((item) => item && typeof item.rootId === "string" && item.rootId.trim() !== "")
    .map((item) => ({
      rootId: item.rootId.trim(),
      title: typeof item.title === "string" ? item.title : "",
      fileType: Number(item.fileType) || 0,
      parsedAt: Number(item.parsedAt) || 0,
      group: item.group || null,
    }));
}

/**
 * buildHistoryEntries 把一次解析的结果（groups 数组）转成要写入历史的条目。
 * 我们的 `WorkshopFileDetails` 没有 `file_type` 字段，所以这里用"有没有子项"推断：
 * 带子项的按合集（2）记录，否则按单件（0）。
 *
 * 注意后端的真实形状：`buildWorkshopDetailsGroup` 返回的 `items` 里**第一条就是主物品本身**
 * （`items = [main, ...children]`）。真机复现：直接按 `items.length > 0` 判断，
 * 单件 Mod 也会被记成合集（fileType=2），整个历史全是"合集"。
 * 所以要先按主物品 ID 把主物品本身排除掉。
 */
export function buildHistoryEntries(groups) {
  if (!Array.isArray(groups)) return [];
  return groups
    .map((group) => {
      const rootId = String(group?.root_id || group?.main?.publishedfileid || "").trim();
      const items = Array.isArray(group?.items) ? group.items : [];
      const mainId = String(group?.main?.publishedfileid || rootId).trim();
      // 没带 id 的条目按子项算（宁可选成合集，也不要漏判）——老快照里可能没有 id。
      const hasChildren = items.some((child) => {
        const childId = String(child?.publishedfileid || "").trim();
        return childId === "" || childId !== mainId;
      });
      return {
        rootId,
        title: typeof group?.main?.title === "string" ? group.main.title : "",
        fileType: hasChildren ? 2 : 0,
        parsedAt: 0,
        group,
      };
    })
    .filter((item) => item.rootId !== "");
}

/** formatHistoryLabel 生成列表里显示的名字：优先标题，退化成 `工坊 #ID`。 */
export function formatHistoryLabel(item) {
  const title = String(item?.title || "").trim();
  if (title) return title;
  return `工坊 #${String(item?.rootId || "").trim()}`;
}

/** formatHistoryTime 把毫秒时间戳格式化成 `MM-DD HH:mm`（无有效时间返回空串）。 */
export function formatHistoryTime(parsedAt, now = Date.now()) {
  const ms = Number(parsedAt) || 0;
  if (ms <= 0) return "";
  const date = new Date(ms);
  if (Number.isNaN(date.getTime())) return "";
  // 未来时间当作异常数据，不显示
  if (ms > Number(now) + 60_000) return "";
  const pad = (value) => String(value).padStart(2, "0");
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
