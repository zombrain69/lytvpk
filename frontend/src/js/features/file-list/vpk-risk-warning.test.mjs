import assert from "node:assert/strict";
import test from "node:test";

import {
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
