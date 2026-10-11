// 启用方案应用结果的提示文案。

import { t } from "../../core/i18n-runtime.js";

export function formatProfileApplySummary(result) {
  const name = String(result?.profileName || "").trim();
  const applied = Number(result?.appliedCount) || 0;
  const added = Number(result?.addedCount) || 0;
  const kept = Number(result?.keptCount) || 0;
  const groups = Number(result?.restoredGroups) || 0;
  const dependencies = Number(result?.restoredDependencies) || 0;

  const parts = [
    t("settings.profiles.apply.applied", { count: applied }),
    t("settings.profiles.apply.added", { count: added }),
    t("settings.profiles.apply.kept", { count: kept }),
  ];
  if (groups > 0) parts.push(t("settings.profiles.apply.groups", { count: groups }));
  if (dependencies > 0) parts.push(t("settings.profiles.apply.dependencies", { count: dependencies }));

  const head = name ? t("settings.profiles.apply.titled", { name }) : t("settings.profiles.apply.untitled");
  return `${head}：${parts.join("，")}`;
}
