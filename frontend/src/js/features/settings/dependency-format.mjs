// Mod 依赖相关操作的提示文案。

import { t } from "../../core/i18n-runtime.js";
import { commaSeparator, labelSeparator } from "../../core/format-list.mjs";

function bucketCount(value) {
  return Array.isArray(value) ? value.length : 0;
}

export function formatDependencyEnableSummary(name, result) {
  const label = String(name || "").trim();
  const parts = [];
  const enabled = bucketCount(result?.enabled);
  const alreadyEnabled = bucketCount(result?.alreadyEnabled);
  const missing = bucketCount(result?.missing);

  if (enabled > 0) parts.push(t("settings.dependencies.summary.enabled", { count: enabled }));
  if (alreadyEnabled > 0) parts.push(t("settings.dependencies.summary.alreadyEnabled", { count: alreadyEnabled }));
  if (missing > 0) parts.push(t("settings.dependencies.summary.missing", { count: missing }));

  const body = parts.join(commaSeparator()) || t("settings.dependencies.summary.none");
  return label ? `${label}${labelSeparator()}${body}` : body;
}

export function formatDependencyBatchEnableSummary(result) {
  const enabled = bucketCount(result?.enabled);
  const masters = Number(result?.masterCount) || 0;
  if (enabled === 0 || masters === 0) {
    return t("settings.dependencies.batch.none");
  }
  return t("settings.dependencies.batch.done", { masters, enabled });
}
