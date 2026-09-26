import assert from "node:assert/strict";
import { test } from "node:test";

import {
  healthIssueAutoFixAction,
  summarizeHealthAutoFix,
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
    kind: "missing_file",
    // 这一行属于白名单里的"缺失条目"，界面会给「一键修复」入口。
    autoFix: healthIssueAutoFixAction("missing_file"),
    // 白名单动作需要的"可操作目标"：优先用后端给的 target，退回问题名。
    fixTarget: "a.vpk",
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
  assert.equal(formatHealthIssueKind("dependency_cycle"), "依赖成环");
  assert.equal(formatHealthIssueKind("file_type_mismatch"), "同名路径是文件夹");
  assert.equal(formatHealthIssueKind("meta_id_mismatch"), "工坊信息对不上作品");
  assert.equal(formatHealthIssueKind("outside_root"), "条目指向受管目录之外");
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

// FireAxe 的 Problem.CanAutomaticallyFix / TryAutomaticallyFix 分级：
// 上游唯一声明"可自动修复"的是 AddonDependencyProblem（依赖未开启），
// 本项目再加上自己有专用接口的"重复条目 / 缺失条目"；其余一律只给建议。
test("healthIssueAutoFixAction 只白名单三类可自动修复的问题", () => {
  assert.equal(healthIssueAutoFixAction("duplicate_entry").id, "remove-duplicates");
  assert.match(healthIssueAutoFixAction("duplicate_entry").title, /先备份/);
  assert.equal(healthIssueAutoFixAction("missing_file").id, "remove-missing");
  assert.equal(healthIssueAutoFixAction("dependency_disabled").id, "enable-dependencies");
  // 依赖问题必须带上目标才能修（否则界面不知道该给谁启用依赖）。
  assert.equal(healthIssueAutoFixAction("dependency_disabled").needsTarget, true);
  [
    "invalid_vpk",
    "dependency_missing",
    "dependency_cycle",
    "orphan_meta",
    "missing_meta",
    "file_type_mismatch",
    "meta_id_mismatch",
    "outside_root",
    "",
  ].forEach((kind) => {
    assert.equal(healthIssueAutoFixAction(kind), null, `${kind} 不应自动修复`);
  });
});

test("summarizeHealthAutoFix 统计可修复数量与动作", () => {
  const summary = summarizeHealthAutoFix({
    issues: [
      { kind: "duplicate_entry" },
      { kind: "duplicate_entry" },
      { kind: "missing_file" },
      { kind: "dependency_disabled", target: "master.vpk", name: "master.vpk" },
      { kind: "invalid_vpk" },
    ],
  });
  assert.equal(summary.total, 4);
  assert.deepEqual(
    summary.actions.map((item) => [item.id, item.count]),
    [
      ["remove-duplicates", 2],
      ["remove-missing", 1],
      ["enable-dependencies", 1],
    ],
  );
  assert.equal(summarizeHealthAutoFix({ issues: [] }).total, 0);
});

test("buildHealthIssueRows 给每一行带上 autoFix 元数据", () => {
  const { rows } = buildHealthIssueRows(
    {
      issues: [
        { kind: "duplicate_entry", severity: "warning", name: "a.vpk", message: "重复" },
        { kind: "invalid_vpk", severity: "critical", name: "b.vpk", message: "损坏" },
      ],
    },
    10,
  );
  assert.equal(rows[0].autoFix?.id, "remove-duplicates");
  assert.equal(rows[1].autoFix, null);
});

test("依赖问题的修复目标来自后端给的 target", () => {
  const { rows } = buildHealthIssueRows({
    issues: [
      { kind: "dependency_disabled", severity: "warning", name: "主 Mod.vpk", target: "workshop\\555.vpk", message: "依赖没开" },
      { kind: "dependency_disabled", severity: "warning", name: "只有名字的.vpk", message: "依赖没开" },
    ],
  });
  assert.equal(rows[0].autoFix?.id, "enable-dependencies");
  assert.equal(rows[0].fixTarget, "workshop\\555.vpk");
  // 后端没给 target 时退回问题名，界面仍然可用。
  assert.equal(rows[1].fixTarget, "只有名字的.vpk");
});
