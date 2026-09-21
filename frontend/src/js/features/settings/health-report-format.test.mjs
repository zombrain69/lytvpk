import assert from "node:assert/strict";
import { test } from "node:test";

import {
  buildHealthIssueRows,
  formatHealthIssueKind,
  formatHealthReportSummary,
  formatHealthSeverity,
} from "./health-report-format.mjs";

test("buildHealthIssueRows maps issues and reports hidden rows", () => {
  const empty = buildHealthIssueRows(null);
  assert.deepEqual(empty, { rows: [], hiddenCount: 0 });

  const { rows, hiddenCount } = buildHealthIssueRows(
    {
      issues: [
        { kind: "missing_file", severity: "critical", name: "a.vpk", message: "缺少文件" },
        { kind: "duplicate_entry", severity: "warning", name: "b.vpk", message: "重复" },
        { kind: "unrecorded", severity: "info", name: "c.vpk", message: "未记录" },
      ],
    },
    2,
  );

  assert.equal(hiddenCount, 1);
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[0], {
    severity: "critical",
    severityLabel: "严重",
    title: "条目缺少文件 · a.vpk",
    message: "缺少文件",
  });
  assert.equal(rows[1].severityLabel, "警告");
});

test("buildHealthIssueRows defaults the severity to info", () => {
  const { rows } = buildHealthIssueRows({ issues: [{ name: "x.vpk" }] });
  assert.equal(rows[0].severity, "info");
  assert.equal(rows[0].severityLabel, "提示");
});

test("formatHealthSeverity maps known severities", () => {
  assert.equal(formatHealthSeverity("critical"), "严重");
  assert.equal(formatHealthSeverity("warning"), "警告");
  assert.equal(formatHealthSeverity("info"), "提示");
  assert.equal(formatHealthSeverity(""), "提示");
});

test("formatHealthIssueKind maps known kinds and falls back to the raw value", () => {
  assert.equal(formatHealthIssueKind("missing_file"), "条目缺少文件");
  assert.equal(formatHealthIssueKind("disabled_only"), "只剩 disabled 副本");
  assert.equal(formatHealthIssueKind("unrecorded"), "未写入开关记录");
  assert.equal(formatHealthIssueKind("duplicate_entry"), "重复条目");
  assert.equal(formatHealthIssueKind("invalid_vpk"), "VPK 无法解析");
  assert.equal(formatHealthIssueKind("missing_addonlist"), "缺少 addonlist.txt");
  assert.equal(formatHealthIssueKind("orphan_meta"), "孤立的工坊信息");
  assert.equal(formatHealthIssueKind("missing_meta"), "缺少工坊信息");
  assert.equal(formatHealthIssueKind("dependency_disabled"), "依赖未开启");
  assert.equal(formatHealthIssueKind("dependency_missing"), "依赖缺失");
  assert.equal(formatHealthIssueKind("something_else"), "something_else");
  assert.equal(formatHealthIssueKind(undefined), "未知问题");
});

test("formatHealthReportSummary counts severities", () => {
  assert.equal(formatHealthReportSummary(null), "还没有体检结果");
  assert.equal(
    formatHealthReportSummary({ totalIssues: 0, issues: [] }),
    "未发现问题",
  );
  assert.equal(
    formatHealthReportSummary({
      totalIssues: 3,
      issues: [
        { severity: "critical" },
        { severity: "warning" },
        { severity: "info" },
      ],
    }),
    "发现 3 个问题：严重 1、警告 1、提示 1",
  );
});

test("formatHealthReportSummary handles a partial severity list", () => {
  assert.equal(
    formatHealthReportSummary({
      totalIssues: 2,
      issues: [{ severity: "critical" }, { severity: "critical" }],
    }),
    "发现 2 个问题：严重 2",
  );
});
