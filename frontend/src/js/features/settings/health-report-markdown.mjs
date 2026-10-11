// 把体检结果渲染成可直接粘贴到社区答疑的 Markdown 报告。

import {
  formatHealthIssueKind,
  formatHealthSeverity,
} from "./health-report-format.mjs";
import { t } from "../../core/i18n-runtime.js";

const SEVERITY_ORDER = ["critical", "warning", "info"];

// Markdown 表格单元格里需要转义竖线，并把换行压成空格。
function toTableCell(value) {
  return String(value ?? "")
    .replace(/\|/g, "\\|")
    .replace(/\r?\n/g, " ")
    .trim();
}

export function buildHealthReportMarkdown(report, options = {}) {
  const generatedAt = String(options?.generatedAt || "").trim() || t("settings.health.markdown.unknownTime");
  const issues = Array.isArray(report?.issues) ? report.issues : [];
  const deepScanned = report?.deepScanned === true;

  const counts = { critical: 0, warning: 0, info: 0 };
  let unknownSeverityCount = 0;
  for (const issue of issues) {
    const severity = String(issue?.severity || "info").trim() || "info";
    if (severity in counts) {
      counts[severity] += 1;
    } else {
      unknownSeverityCount += 1;
    }
  }

  const countParts = SEVERITY_ORDER.filter((severity) => counts[severity] > 0).map(
    (severity) => `${formatHealthSeverity(severity)} ${counts[severity]}`,
  );
  const countText = countParts.length > 0 ? `（${countParts.join("、")}）` : "";

  const lines = [
    t("settings.health.markdown.title"),
    "",
    t("settings.health.markdown.generatedAt", { time: generatedAt }),
    t("settings.health.markdown.total", { count: issues.length, counts: countText }),
    t("settings.health.markdown.deepScan", {
      value: t(deepScanned ? "settings.health.markdown.yes" : "settings.health.markdown.no"),
    }),
    "",
  ];

  if (issues.length === 0) {
    lines.push(t("settings.health.markdown.clean"), "");
    return lines.join("\n");
  }

  for (const severity of SEVERITY_ORDER) {
    const group = issues.filter((issue) => {
      const value = String(issue?.severity || "info").trim() || "info";
      return value === severity;
    });
    if (group.length === 0) {
      continue;
    }
    lines.push(t("settings.health.markdown.section", { severity: formatHealthSeverity(severity), count: group.length }), "");
    lines.push(t("settings.health.markdown.tableHeader"), "| --- | --- | --- |");
    for (const issue of group) {
      lines.push(
        `| ${toTableCell(formatHealthIssueKind(issue?.kind))} | ${toTableCell(issue?.name)} | ${toTableCell(issue?.message)} |`,
      );
    }
    lines.push("");
  }

  if (unknownSeverityCount > 0) {
    lines.push(t("settings.health.markdown.unknownSeverity", { count: unknownSeverityCount }), "");
  }
  lines.push("---", "", t("settings.health.markdown.footer"), "");
  return lines.join("\n");
}
