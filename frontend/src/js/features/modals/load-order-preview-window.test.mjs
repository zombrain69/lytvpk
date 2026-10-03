// 加载顺序预览的窗口换算：
// 真机 2607 行整表渲染时滚动 23 帧 >50ms；去掉 content-visibility 又让开窗变成
// 25 个长任务（50–249ms）。窗口化只物化"视口 ± overscan"，这里钉住换算与占位块守恒。

import assert from "node:assert/strict";
import test from "node:test";
import { computePreviewWindow, previewSpacerHeights } from "./load-order-preview-window.mjs";

test("窗口覆盖可见区 + overscan", () => {
  const { start, end } = computePreviewWindow({
    total: 1000,
    rowPitch: 40,
    scrollTop: 4000,
    clientHeight: 520,
    overscanRows: 4,
  });
  // firstVisible = 100；visible = ceil(520/40)+1 = 14；start = 96；end = 118
  assert.equal(start, 96);
  assert.equal(end, 118);
});

test("顶部/底部夹紧，总数为 0 时返回空窗口", () => {
  assert.deepEqual(
    computePreviewWindow({ total: 10, rowPitch: 40, scrollTop: 0, clientHeight: 240 }),
    { start: 0, end: 10 },
  );
  const bottom = computePreviewWindow({ total: 100, rowPitch: 40, scrollTop: 100000, clientHeight: 240 });
  assert.equal(bottom.end, 100, "滚到底时窗口要贴住最后一行");
  assert.ok(bottom.start < 100);
  assert.deepEqual(computePreviewWindow({ total: 0 }), { start: 0, end: 0 });
});

test("量不到行高/视口时用兜底值，不能只物化一行", () => {
  const windowed = computePreviewWindow({ total: 100, rowPitch: 0, scrollTop: 0, clientHeight: 0 });
  assert.ok(
    windowed.end - windowed.start >= 5,
    `兜底窗口太小：${JSON.stringify(windowed)}`,
  );
});

test("占位块高度：上 + 窗口行数×行高 + 下 == 真实总高度", () => {
  const total = 2607;
  const rowPitch = 40;
  const start = 96;
  const end = 118;
  const { top, bottom } = previewSpacerHeights({ total, start, end, rowPitch });
  assert.equal(top, start * rowPitch);
  assert.equal(bottom, (total - end) * rowPitch);
  assert.equal(top + (end - start) * rowPitch + bottom, total * rowPitch);
});
