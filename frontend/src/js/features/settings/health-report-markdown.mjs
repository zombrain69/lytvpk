// 把体检结果渲染成可直接粘贴到社区答疑的 Markdown 报告。

import {
  formatHealthIssueKind,
  formatHealthSeverity,
} from "./health-report-format.mjs";

const SEVERITY_ORDER = ["critical", "warning", "info"];

// Markdown 表格单元格里需要转义竖线，并把换行压成空格。
function toTableCell(value) {
  return String(value ?? "")
    .replace(/\|/g, "\\|")
    .replace(/\r?\n/g, " ")
    .trim();
}

export function buildHealthReportMarkdown(report, options = {}) {
  const generatedAt = String(options?.generatedAt || "").trim() || "未知";
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
    "# LytVPK Mod 体检报告",
    "",
    `- 生成时间：${generatedAt}`,
    `- 问题总数：${issues.length}${countText}`,
    `- 深度扫描：${deepScanned ? "是" : "否"}`,
    "",
  ];

  if (issues.length === 0) {
    lines.push("未发现问题。", "");
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
    lines.push(`## ${formatHealthSeverity(severity)}（${group.length}）`, "");
    lines.push("| 类型 | 对象 | 说明 |", "| --- | --- | --- |");
    for (const issue of group) {
      lines.push(
        `| ${toTableCell(formatHealthIssueKind(issue?.kind))} | ${toTableCell(issue?.name)} | ${toTableCell(issue?.message)} |`,
      );
    }
    lines.push("");
  }

  if (unknownSeverityCount > 0) {
    lines.push(`> 另有 ${unknownSeverityCount} 条结果严重度未知，未分节显示。`, "");
  }
  lines.push("---", "", "由 LytVPK 生成。", "");
  return lines.join("\n");
}
