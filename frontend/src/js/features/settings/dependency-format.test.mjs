import assert from "node:assert/strict";
import { test } from "node:test";

import {
  formatDependencyBatchEnableSummary,
  formatDependencyEnableSummary,
} from "./dependency-format.mjs";

test("formatDependencyEnableSummary reports each outcome bucket", () => {
  assert.equal(
    formatDependencyEnableSummary("角色包", {
      enabled: ["a.vpk"],
      alreadyEnabled: ["b.vpk"],
      missing: ["c.vpk"],
    }),
    "角色包：已启用 1 个，1 个本就开启，1 个文件缺失",
  );
});

test("formatDependencyEnableSummary handles a single bucket and empty results", () => {
  assert.equal(
    formatDependencyEnableSummary("角色包", { enabled: ["a.vpk", "b.vpk"] }),
    "角色包：已启用 2 个",
  );
  assert.equal(formatDependencyEnableSummary("角色包", {}), "角色包：没有需要处理的依赖");
  assert.equal(formatDependencyEnableSummary("", null), "没有需要处理的依赖");
});

test("formatDependencyBatchEnableSummary summarises a batch run", () => {
  assert.equal(
    formatDependencyBatchEnableSummary({ masterCount: 2, enabled: ["a.vpk", "b.vpk"] }),
    "已为 2 个 Mod 启用 2 个依赖",
  );
  assert.equal(formatDependencyBatchEnableSummary({ masterCount: 0, enabled: [] }), "没有需要启用的依赖");
});
