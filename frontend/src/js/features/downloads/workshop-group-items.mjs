// 工坊解析结果的"要渲染哪些条目"纯逻辑（对齐上游 c9e8a25）。
//
// 背景：合集解析时后端会把"合集本体"也放进 group.items（file_type=2，本身没有文件、不可下载），
// 以前前端会把它渲染成一张"不可下载"的卡片，看起来像解析失败。
// 合集标题栏已经展示合集名与 ID，正文只应该列里面的成员。
//
// 注意：**不能改变条目在 items 里的下标**——下载按钮用 data-item-index 回查
// `items[itemIndex]`，所以这里返回的是"跳过哪些下标"，而不是重新切片。

/** isCollectionGroup 判断这一组的主物品是不是合集（Steam file_type=2）。 */
export function isCollectionGroup(group) {
  return Number(group?.main?.file_type) === 2;
}

/**
 * shouldRenderWorkshopGroupItem 判断第 index 个条目要不要渲染成卡片。
 * 合集本体的那一条跳过（正文只列成员），其它条目照旧。
 */
export function shouldRenderWorkshopGroupItem(group, details, index) {
  if (!isCollectionGroup(group)) return true;
  if (index !== 0) return true;
  const mainId = String(group?.main?.publishedfileid ?? "").trim();
  const itemId = String(details?.publishedfileid ?? "").trim();
  // 任何一边拿不到 ID 就不跳过：宁可多显示一张卡片，也不要把真正的成员吞掉。
  if (mainId === "" || itemId === "") return true;
  return mainId !== itemId;
}
