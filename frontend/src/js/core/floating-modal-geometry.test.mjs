import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { clampWindowSize } from "./window-geometry.mjs";

const source = readFileSync(new URL("./floating-modal.js", import.meta.url), "utf8");

// 窗口尺寸按窗口记忆（localStorage）。换了小屏之后必须还能把窗口拉回可视区，
// 否则标题栏会被推到屏幕外、按钮点不到 —— 这是"窗口交互灵活易用"的底线。
test("clampWindowSize 把过大的记忆尺寸压回可视区", () => {
  // 1920×1080 屏上记了 1900×1000：在小一点的窗口里要按比例压回来。
  const clamped = clampWindowSize(1900, 1000, 1280, 800);
  assert.equal(clamped.width, Math.round(1280 * 0.96));
  assert.equal(clamped.height, Math.round(800 * 0.94));
});

test("clampWindowSize 不影响正常尺寸，也不产生 0/NaN", () => {
  assert.deepEqual(clampWindowSize(900, 600, 1600, 900), { width: 900, height: 600 });
  // 没有记忆尺寸（0 / NaN）时给可视区上限，避免算出 0 宽高。
  assert.deepEqual(clampWindowSize(0, Number.NaN, 1000, 700), {
    width: Math.round(1000 * 0.96),
    height: Math.round(700 * 0.94),
  });
  // 极小的可视区也有兜底下限，不会压成不可用尺寸。
  const tiny = clampWindowSize(800, 600, 200, 150);
  assert.ok(tiny.width >= 240 && tiny.height >= 200, `兜底尺寸过小: ${JSON.stringify(tiny)}`);
});

test("pinContent 使用 clampWindowSize（位置与尺寸一起收敛）", () => {
  assert.match(source, /clampWindowSize\(/, "pinContent 要调用 clampWindowSize");
  assert.match(
    source,
    /const \{ width, height \} = clampWindowSize\(\s*Number\(position\?\.width \|\| rect\.width\),\s*Number\(position\?\.height \|\| rect\.height\),\s*window\.innerWidth,\s*window\.innerHeight,\s*\)/,
    "要把可视区尺寸传进去",
  );
});

test("双击标题栏可以重置窗口位置与大小", () => {
  assert.match(source, /addEventListener\("dblclick"/, "缺少双击标题栏处理");
  assert.match(source, /removeItem\(positionKey\)/, "重置要清掉记忆的几何");
  assert.match(source, /clearInlinePosition\(content\)/, "重置要清掉内联几何样式");
});

test("resetAllWindowGeometry：清掉全部记忆并复位当前浮动窗口", () => {
  assert.match(source, /export function resetAllWindowGeometry/, "要提供「一键重置所有窗口」入口");
  assert.match(source, /POSITION_KEY_PREFIX/, "要按窗口几何的前缀清理 localStorage");
  assert.match(source, /registry\.forEach/, "要顺带复位当前已注册的窗口");
  // 命令面板里要有对应命令（自救入口不能只藏在代码里）。
  const runtime = readFileSync(new URL("../features/app-runtime.js", import.meta.url), "utf8");
  assert.match(runtime, /"reset-window-geometry":/, "命令面板要注册重置命令");
  assert.match(runtime, /resetAllWindowGeometry\(\)/, "命令要真的调用重置");
});
