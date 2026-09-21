// Mod 体检结果的展示文案。

const HEALTH_ISSUE_KIND_LABELS = {
  missing_addonlist: "缺少 addonlist.txt",
  missing_file: "条目缺少文件",
  disabled_only: "只剩 disabled 副本",
  unrecorded: "未写入开关记录",
  duplicate_entry: "重复条目",
  invalid_vpk: "VPK 无法解析",
  orphan_meta: "孤立的工坊信息",
  missing_meta: "缺少工坊信息",
  dependency_disabled: "依赖未开启",
  dependency_missing: "依赖缺失",
};

export function formatHealthIssueKind(kind) {
  const key = String(kind || "").trim();
  if (!key) {
    return "未知问题";
  }
  return HEALTH_ISSUE_KIND_LABELS[key] || key;
}

const HEALTH_SEVERITY_LABELS = {
  critical: "严重",
  warning: "警告",
  info: "提示",
};

export function formatHealthSeverity(severity) {
  return HEALTH_SEVERITY_LABELS[String(severity || "").trim()] || "提示";
}

export function formatHealthReportSummary(report) {
  if (!report) {
    return "还没有体检结果";
  }
  const total = Number(report.totalIssues) || 0;
  if (total === 0) {
    return "未发现问题";
  }

  const counts = { critical: 0, warning: 0, info: 0 };
  for (const issue of report.issues || []) {
    const severity = String(issue?.severity || "").trim();
    if (severity in counts) {
      counts[severity] += 1;
    }
  }

  const parts = [];
  if (counts.critical > 0) parts.push(`严重 ${counts.critical}`);
  if (counts.warning > 0) parts.push(`警告 ${counts.warning}`);
  if (counts.info > 0) parts.push(`提示 ${counts.info}`);
  return parts.length > 0 ? `发现 ${total} 个问题：${parts.join("、")}` : `发现 ${total} 个问题`;
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
    };
  });
  return { rows, hiddenCount: Math.max(issues.length - rows.length, 0) };
}
