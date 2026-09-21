// 工坊合集实体化的文案层（与 DOM 无关，node --test 覆盖）。

export function countMissingCollectionMembers(link) {
  if (!link || !Array.isArray(link.members)) return 0;
  return link.members.filter((member) => !member?.present).length;
}

export function formatCollectionSummary(link) {
  if (!link) return "";
  const total = Array.isArray(link.members) ? link.members.length : 0;
  const missing = countMissingCollectionMembers(link);
  const truncated = link.childCollectionsTruncated ? "（子合集过多，已截断）" : "";
  if (total === 0) return `没有可下载成员${truncated}`;
  return missing === 0
    ? `${total} 个成员，已全部下载${truncated}`
    : `${total} 个成员，其中 ${missing} 个未下载${truncated}`;
}

export function formatCollectionRefreshSummary(result) {
  if (!result) return "";
  const parts = [];
  if (Number(result.addedCount || 0) > 0) {
    const titles = (result.addedTitles || []).filter(Boolean).slice(0, 3).join("、");
    parts.push(`新增 ${result.addedCount} 个${titles ? `（${titles}…）` : ""}`);
  }
  if (Number(result.removedCount || 0) > 0) {
    const titles = (result.removedTitles || []).filter(Boolean).slice(0, 3).join("、");
    parts.push(`下架 ${result.removedCount} 个${titles ? `（${titles}…）` : ""}`);
  }
  if (Number(result.missingCount || 0) > 0) {
    parts.push(`本地缺少 ${result.missingCount} 个`);
  }
  if (parts.length === 0) return "合集内容没有变化";
  return parts.join("；");
}

export function formatCollectionQueueSummary(queuedIds) {
  const ids = Array.isArray(queuedIds) ? queuedIds.filter(Boolean) : [];
  if (ids.length === 0) return "没有需要下载的成员";
  return `已加入下载队列 ${ids.length} 个成员`;
}
