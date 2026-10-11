// Mod 体检结果的展示文案。

import { t } from "../../core/i18n-runtime.js";

// kind → 文案 key（调用时再取，模块求值期不依赖已安装目录）。
const HEALTH_ISSUE_KIND_KEYS = {
  missing_addonlist: "settings.health.kind.missingAddonlist",
  missing_file: "settings.health.kind.missingFile",
  disabled_only: "settings.health.kind.disabledOnly",
  unrecorded: "settings.health.kind.unrecorded",
  duplicate_entry: "settings.health.kind.duplicateEntry",
  invalid_vpk: "settings.health.kind.invalidVpk",
  orphan_meta: "settings.health.kind.orphanMeta",
  missing_meta: "settings.health.kind.missingMeta",
  dependency_disabled: "settings.health.kind.dependencyDisabled",
  dependency_missing: "settings.health.kind.dependencyMissing",
  dependency_cycle: "settings.health.kind.dependencyCycle",
  file_type_mismatch: "settings.health.kind.fileTypeMismatch",
  meta_id_mismatch: "settings.health.kind.metaIdMismatch",
  outside_root: "settings.health.kind.outsideRoot",
  duplicate_vpk_copy: "settings.health.kind.duplicateVpkCopy",
  duplicate_disabled_copy: "settings.health.kind.duplicateDisabledCopy",
  subfolder_vpks: "settings.health.kind.subfolderVpks",
  filename_too_long: "settings.health.kind.filenameTooLong",
  xdr_slot_collision: "settings.health.kind.xdrSlotCollision",
  xdr_missing_base: "settings.health.kind.xdrMissingBase",
  xdr_variant_pack: "settings.health.kind.xdrVariantPack",
};

export function formatHealthIssueKind(kind) {
  const key = String(kind || "").trim();
  if (!key) {
    return t("settings.health.kind.unknown");
  }
  const translationKey = HEALTH_ISSUE_KIND_KEYS[key];
  return translationKey ? t(translationKey) : key;
}

const HEALTH_SEVERITY_KEYS = {
  critical: "settings.health.severity.critical",
  warning: "settings.health.severity.warning",
  info: "settings.health.severity.info",
};

export function formatHealthSeverity(severity) {
  return t(HEALTH_SEVERITY_KEYS[String(severity || "").trim()] || "settings.health.severity.info");
}

/**
 * healthIssueAutoFixAction 返回这一条体检问题能不能"一键修复"，以及走哪个动作。
 *
 * 对齐 FireAxe 的 `Problem.CanAutomaticallyFix` / `TryAutomaticallyFix` 分级：
 * 只有**语义明确、改动可逆、且已有专用接口**的问题才进白名单 ——
 *   1. 重复条目：删掉 addonlist.txt 里重复的键（后端 RemoveDuplicateAddonListEntries）；
 *   2. 条目缺少文件：清掉指向已不存在文件的条目（后端 RemoveMissingFileAddonListEntries）；
 *   3. 依赖未开启：给这条问题的主 Mod 启用依赖（后端 EnableModDependencies）——
 *      这一条正是上游唯一声明可自动修复的问题（`AddonDependencyProblem.cs:12-26`）。
 * 其余问题（VPK 损坏、依赖缺失 / 成环、工坊信息问题…）需要人工判断，不自动改，只给建议。
 */
const HEALTH_AUTO_FIX_ACTIONS = [
  {
    id: "remove-duplicates",
    kinds: ["duplicate_entry"],
    labelKey: "settings.health.autoFix.removeDuplicates.label",
    shortLabelKey: "settings.health.autoFix.removeDuplicates.shortLabel",
    titleKey: "settings.health.autoFix.removeDuplicates.title",
    needsTarget: false,
  },
  {
    id: "remove-missing",
    kinds: ["missing_file"],
    labelKey: "settings.health.autoFix.removeMissing.label",
    shortLabelKey: "settings.health.autoFix.removeMissing.shortLabel",
    titleKey: "settings.health.autoFix.removeMissing.title",
    needsTarget: false,
  },
  {
    id: "enable-dependencies",
    kinds: ["dependency_disabled"],
    labelKey: "settings.health.autoFix.enableDependencies.label",
    shortLabelKey: "settings.health.autoFix.enableDependencies.shortLabel",
    titleKey: "settings.health.autoFix.enableDependencies.title",
    needsTarget: true,
  },
];

export function healthIssueAutoFixAction(kind) {
  const key = String(kind || "").trim();
  if (!key) {
    return null;
  }
  const action = HEALTH_AUTO_FIX_ACTIONS.find((item) => item.kinds.includes(key));
  if (!action) return null;
  return {
    ...action,
    label: t(action.labelKey),
    shortLabel: t(action.shortLabelKey),
    title: t(action.titleKey),
  };
}

/** summarizeHealthAutoFix 统计体检结果里可自动修复的问题数量（按动作分组）。 */
export function summarizeHealthAutoFix(report) {
  const issues = Array.isArray(report?.issues) ? report.issues : [];
  const counts = new Map();
  issues.forEach((issue) => {
    const action = healthIssueAutoFixAction(issue?.kind);
    if (!action) return;
    counts.set(action.id, (counts.get(action.id) || 0) + 1);
  });
  // 按白名单声明顺序输出，界面上的统计条不会因为问题顺序变化而抖动。
  const actions = HEALTH_AUTO_FIX_ACTIONS.filter((item) => counts.has(item.id)).map((item) => ({
    id: item.id,
    count: counts.get(item.id),
    label: t(item.labelKey),
    shortLabel: t(item.shortLabelKey),
    title: t(item.titleKey),
  }));
  return {
    total: actions.reduce((sum, item) => sum + item.count, 0),
    actions,
  };
}

export function formatHealthReportSummary(report) {
  if (!report) {
    return t("settings.health.summary.empty");
  }
  const total = Number(report.totalIssues) || 0;
  if (total === 0) {
    return t("settings.health.summary.clean");
  }

  const counts = { critical: 0, warning: 0, info: 0 };
  for (const issue of report.issues || []) {
    const severity = String(issue?.severity || "").trim();
    if (severity in counts) {
      counts[severity] += 1;
    }
  }

  const parts = [];
  if (counts.critical > 0) parts.push(t("settings.health.summary.critical", { count: counts.critical }));
  if (counts.warning > 0) parts.push(t("settings.health.summary.warning", { count: counts.warning }));
  if (counts.info > 0) parts.push(t("settings.health.summary.info", { count: counts.info }));
  return parts.length > 0
    ? t("settings.health.summary.issues", { total, parts: parts.join("、") })
    : t("settings.health.summary.issuesPlain", { total });
}

// buildHealthIssueRows 把体检结果整理成可直接渲染的行，并给出被截断的条数。
export function buildHealthIssueRows(report, limit = 100) {
  const issues = Array.isArray(report?.issues) ? report.issues : [];
  const max = Number.isFinite(limit) && limit > 0 ? Math.floor(limit) : issues.length;
  const rows = issues.slice(0, Math.max(max, 0)).map((issue) => {
    const severity = String(issue?.severity || "info").trim() || "info";
    return {
      severity,
      severityLabel: formatHealthSeverity(severity),
      title: `${formatHealthIssueKind(issue?.kind)} · ${issue?.name || ""}`,
      message: issue?.message || "",
      kind: String(issue?.kind || ""),
      autoFix: healthIssueAutoFixAction(issue?.kind),
      // 白名单动作要操作谁：后端给的 target（addonlist 键 / 路径）优先，退回问题名。
      fixTarget: String(issue?.target || issue?.name || ""),
    };
  });
  return { rows, hiddenCount: Math.max(issues.length - rows.length, 0) };
}
