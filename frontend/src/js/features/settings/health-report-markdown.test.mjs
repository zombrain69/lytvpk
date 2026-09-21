import assert from "node:assert/strict";
import { test } from "node:test";

import { buildHealthReportMarkdown } from "./health-report-markdown.mjs";

test("buildHealthReportMarkdown reports an empty result without tables", () => {
  const markdown = buildHealthReportMarkdown(
    { totalIssues: 0, issues: [], deepScanned: false },
    { generatedAt: "2026-09-22 10:30" },
  );

  assert.match(markdown, /^# LytVPK Mod 体检报告/);
  assert.match(markdown, /问题总数：0/);
  assert.match(markdown, /未发现问题/);
  assert.ok(!markdown.includes("| --- |"));
});

test("buildHealthReportMarkdown groups issues by severity and escapes table cells", () => {
  const markdown = buildHealthReportMarkdown(
    {
      totalIssues: 3,
      deepScanned: true,
      issues: [
        { kind: "unrecorded", severity: "info", name: "c.vpk", message: "未记录" },
        { kind: "dependency_disabled", severity: "warning", name: "b.vpk", message: "依赖 a|b 未开启" },
        { kind: "missing_file", severity: "critical", name: "a.vpk", message: "缺少文件\n第二行" },
      ],
    },
    { generatedAt: "2026-09-22 10:30" },
  );

  assert.match(markdown, /问题总数：3（严重 1、警告 1、提示 1）/);
  assert.match(markdown, /深度扫描：是/);

  const criticalIndex = markdown.indexOf("## 严重（1）");
  const warningIndex = markdown.indexOf("## 警告（1）");
  const infoIndex = markdown.indexOf("## 提示（1）");
  assert.ok(criticalIndex > 0 && warningIndex > criticalIndex && infoIndex > warningIndex);

  assert.match(markdown, /条目缺少文件 \| a\.vpk \| 缺少文件 第二行/);
  assert.match(markdown, /依赖未开启 \| b\.vpk \| 依赖 a\\\|b 未开启/);
});

test("buildHealthReportMarkdown tolerates missing fields", () => {
  const markdown = buildHealthReportMarkdown(null, {});
  assert.match(markdown, /问题总数：0/);
  assert.match(markdown, /生成时间：未知/);
});
