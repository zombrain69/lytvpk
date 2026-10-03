import assert from "node:assert/strict";
import test from "node:test";

import {
  batchGameStateActionLabel,
  formatBatchGameStateConfirm,
  formatBatchGameStateSummary,
  canApplyBatchGameStateLocally,
} from "./batch-game-state-format.mjs";

test("batchGameStateActionLabel 区分游戏内启用/关闭", () => {
  assert.equal(batchGameStateActionLabel(true), "游戏内启用");
  assert.equal(batchGameStateActionLabel(false), "游戏内关闭");
});

// 批量开关的"能不能本地应用"判据：组联动/未记录才必须整表重扫，
// 其余本地改 appState（真机实测整表 refreshFilesKeepFilter 是 165–175ms 长帧）。
test("canApplyBatchGameStateLocally：组联动/未记录才必须重扫", () => {
  const recorded = [{ gameStateKnown: true }, { gameStateKnown: true }];
  assert.equal(
    canApplyBatchGameStateLocally({ targets: recorded, enforced: 0 }),
    true,
    "已在 addonlist 里的普通批量只翻 0/1，可以本地应用",
  );
  assert.equal(
    canApplyBatchGameStateLocally({ targets: recorded, enforced: 2 }),
    false,
    "策略组联动改了别的成员，必须重扫",
  );
  assert.equal(
    canApplyBatchGameStateLocally({
      targets: [{ gameStateKnown: true }, { gameStateKnown: false }],
    }),
    false,
    "未记录条目会被写进 addonlist，顺序号/优先级会变，必须重扫",
  );
  assert.equal(
    canApplyBatchGameStateLocally({ targets: null, enforced: 0 }),
    true,
    "空目标没有什么要改的，按本地处理即可",
  );
});

test("formatBatchGameStateConfirm 说明只改 addonlist、未记录与 disabled 的情况", () => {
  const text = formatBatchGameStateConfirm({ total: 5, skipped: 2, unrecorded: 1 }, true);
  assert.match(text, /把选中的 5 个 Mod 设为游戏内启用？/);
  assert.match(text, /只改 addonlist\.txt 的 0\/1，不移动、不删除任何文件/);
  assert.match(text, /其中 1 个还没记录在 addonlist\.txt/);
  assert.match(text, /2 个在 disabled 目录里，会被跳过/);
  assert.match(text, /是否继续？/);
  // 没有异常项时不出现多余提示
  const plain = formatBatchGameStateConfirm({ total: 3 }, false);
  assert.equal(/disabled/.test(plain), false);
  assert.match(plain, /设为游戏内关闭/);
});

test("formatBatchGameStateSummary 汇总更新/保持/跳过/联动", () => {
  assert.equal(
    formatBatchGameStateSummary({ updated: ["a", "b"], unchanged: ["c"], skipped: ["d"], enforced: 2 }, true),
    "已把 2 个 Mod 设为游戏内启用；1 个本来就是这个状态；1 个跳过（disabled 目录或不在列表里）；并按策略组自动联动另外 2 个",
  );
  assert.equal(formatBatchGameStateSummary({ updated: ["a"] }, false), "已把 1 个 Mod 设为游戏内关闭");
  assert.equal(formatBatchGameStateSummary({}, true), "没有需要处理的 Mod");
});
