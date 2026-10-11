// 工坊合集实体化的文案层（与 DOM 无关，node --test 覆盖）。

import { t } from "../../core/i18n-runtime.js";

export function countMissingCollectionMembers(link) {
  if (!link || !Array.isArray(link.members)) return 0;
  return link.members.filter((member) => !member?.present).length;
}

export function formatCollectionSummary(link) {
  if (!link) return "";
  const total = Array.isArray(link.members) ? link.members.length : 0;
  const missing = countMissingCollectionMembers(link);
  const truncated = link.childCollectionsTruncated ? t("settings.collections.summary.truncated") : "";
  if (total === 0) return t("settings.collections.summary.empty", { truncated });
  return missing === 0
    ? t("settings.collections.summary.complete", { total, truncated })
    : t("settings.collections.summary.missing", { total, missing, truncated });
}

export function formatCollectionRefreshSummary(result) {
  if (!result) return "";
  const parts = [];
  if (Number(result.addedCount || 0) > 0) {
    const titles = (result.addedTitles || []).filter(Boolean).slice(0, 3).join("、");
    const titlesPart = titles ? t("settings.collections.refresh.titlesPart", { titles }) : "";
    parts.push(t("settings.collections.refresh.added", { count: result.addedCount, titles: titlesPart }));
  }
  if (Number(result.removedCount || 0) > 0) {
    const titles = (result.removedTitles || []).filter(Boolean).slice(0, 3).join("、");
    const titlesPart = titles ? t("settings.collections.refresh.titlesPart", { titles }) : "";
    parts.push(t("settings.collections.refresh.removed", { count: result.removedCount, titles: titlesPart }));
  }
  if (Number(result.missingCount || 0) > 0) {
    parts.push(t("settings.collections.refresh.missing", { count: result.missingCount }));
  }
  if (parts.length === 0) return t("settings.collections.refresh.noChange");
  return parts.join("；");
}

export function formatCollectionQueueSummary(queuedIds) {
  const ids = Array.isArray(queuedIds) ? queuedIds.filter(Boolean) : [];
  if (ids.length === 0) return t("settings.collections.queue.empty");
  return t("settings.collections.queue.queued", { count: ids.length });
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
  if (list.length === 0) return t("settings.collections.checkAll.none");

  const failed = list.filter((item) => String(item?.error || "").trim());
  const ok = list.filter((item) => !String(item?.error || "").trim());
  const changed = ok.filter(
    (item) => Number(item.addedCount || 0) + Number(item.removedCount || 0) > 0,
  );

  const parts = [];
  if (ok.length === 0) {
    parts.push(t("settings.collections.checkAll.allFailed", { total: list.length }));
  } else if (changed.length === 0) {
    parts.push(
      ok.length === list.length
        ? t("settings.collections.checkAll.noChangeAll")
        : t("settings.collections.checkAll.noChangeSome", { count: ok.length }),
    );
  } else {
    parts.push(
      changed.map((item) => `${item.title || item.collectionId}：${formatCollectionRefreshSummary(item)}`).join("；"),
    );
  }
  if (failed.length > 0) {
    const first = failed[0];
    parts.push(
      t("settings.collections.checkAll.failed", {
        count: failed.length,
        title: first.title || first.collectionId,
        error: String(first.error).slice(0, 60),
      }),
    );
  }
  return parts.join("；");
}
