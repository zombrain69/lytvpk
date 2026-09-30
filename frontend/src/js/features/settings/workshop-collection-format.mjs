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

/**
 * 「检查全部更新」的汇总文案。
 *
 * 以前 Go 侧遇到第一条解析失败就直接返回错误，界面只显示「检查合集更新失败: …」，
 * 其余合集的结果一起丢掉。现在后端逐条检查、把失败原因装进 result.error，
 * 这里负责汇总成「变化 + N 个检查失败（原因）」。
 */
export function formatCollectionCheckAllSummary(results) {
  const list = Array.isArray(results) ? results : [];
  if (list.length === 0) return "还没有保存过工坊合集";

  const failed = list.filter((item) => String(item?.error || "").trim());
  const ok = list.filter((item) => !String(item?.error || "").trim());
  const changed = ok.filter(
    (item) => Number(item.addedCount || 0) + Number(item.removedCount || 0) > 0,
  );

  const parts = [];
  if (ok.length === 0) {
    parts.push(`全部 ${list.length} 个合集都检查失败`);
  } else if (changed.length === 0) {
    parts.push(ok.length === list.length ? "所有合集都没有成员变化" : `${ok.length} 个合集没有成员变化`);
  } else {
    parts.push(
      changed.map((item) => `${item.title || item.collectionId}：${formatCollectionRefreshSummary(item)}`).join("；"),
    );
  }
  if (failed.length > 0) {
    const first = failed[0];
    parts.push(
      `${failed.length} 个检查失败：${first.title || first.collectionId}（${String(first.error).slice(0, 60)}）`,
    );
  }
  return parts.join("；");
}
