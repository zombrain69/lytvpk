import assert from "node:assert/strict";
import test from "node:test";
import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";

import {
  formatVPKRiskWarning,
  summarizeVPKIntegrityResults,
  shouldAllowAfterInspectionFailure,
} from "./vpk-risk-warning.mjs";

test("汇总单个问题 VPK 时保留文件名、问题和可修复标记", () => {
  const summary = summarizeVPKIntegrityResults([
    {
      path: "C:\\addons\\broken.vpk",
      report: {
        valid: false,
        repairable: true,
        issues: [
          {
            path: "addoninfo.txt",
            message: "缺少根目录 addoninfo.txt；游戏可能不会记录此 Mod",
            severity: "error",
            repairable: true,
          },
        ],
      },
    },
  ]);

  assert.equal(summary.problemCount, 1);
  assert.equal(summary.items[0].name, "broken.vpk");
  assert.equal(summary.items[0].repairable, true);
  assert.match(summary.items[0].issues[0], /addoninfo\.txt/);
});

test("检测失败不应把操作升级为硬阻止", () => {
  assert.equal(shouldAllowAfterInspectionFailure(new Error("backend unavailable")), true);
});

// 第 13 项第 10 批：文案进目录后，这里补一条「渲染成原句」的插值实测
// （纯模块测试经 installTestCatalog() 断言真实目录文案）。
test("formatVPKRiskWarning 渲染成原句（目录插值实测）", () => {
  installTestCatalog();
  const text = formatVPKRiskWarning("删除 Mod", [
    { name: "broken.vpk", issues: ["缺少 addoninfo.txt"], repairable: true },
  ]);
  assert.equal(
    text,
    [
      "⚠️ 风险提示：删除 Mod",
      "",
      "检测到以下 Mod 可能存在 VPK 完整性问题：",
      "• broken.vpk：缺少 addoninfo.txt",
      "",
      "游戏可能不会记录或加载这些 Mod，但当前操作仍可继续。",
      "",
      "部分问题可通过“VPK 完整性检测”修复并另存为。",
      "",
      "仍要继续吗？",
    ].join("\n"),
  );
});

test("不给操作名时用「继续操作」兜底", () => {
  installTestCatalog();
  const text = formatVPKRiskWarning(null, []);
  assert.match(text, /⚠️ 风险提示：继续操作\n\n/);
});
