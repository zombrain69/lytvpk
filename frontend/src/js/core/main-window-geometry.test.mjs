import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  MAIN_WINDOW_MAX_HEIGHT,
  MAIN_WINDOW_MAX_WIDTH,
  MAIN_WINDOW_MIN_HEIGHT,
  MAIN_WINDOW_MIN_WIDTH,
  describeMainWindowGeometry,
  pickPrimaryScreen,
  sanitizeMainWindowGeometry,
  shouldApplyMainWindowGeometry,
} from "./main-window-geometry.mjs";

test("没记录过 / 非法尺寸一律当没记录（保持默认尺寸）", () => {
  assert.equal(sanitizeMainWindowGeometry(null), null);
  assert.equal(sanitizeMainWindowGeometry(undefined), null);
  assert.equal(sanitizeMainWindowGeometry({}), null);
  assert.equal(sanitizeMainWindowGeometry({ width: 0, height: 800 }), null);
  assert.equal(sanitizeMainWindowGeometry({ width: 1280, height: -5 }), null);
  assert.equal(sanitizeMainWindowGeometry({ width: "宽", height: 800 }), null);
  assert.equal(shouldApplyMainWindowGeometry(null), false);
});

test("正常尺寸原样保留；过小的尺寸被抬到可用下限", () => {
  assert.deepEqual(sanitizeMainWindowGeometry({ width: 1440, height: 900 }), {
    width: 1440,
    height: 900,
    maximised: false,
  });
  // 手改配置写出 300×200 这种"会挤掉工具栏"的值 → 抬到下限
  assert.deepEqual(sanitizeMainWindowGeometry({ width: 300, height: 200 }), {
    width: MAIN_WINDOW_MIN_WIDTH,
    height: MAIN_WINDOW_MIN_HEIGHT,
    maximised: false,
  });
  // 超过硬上限 → 压回上限
  assert.deepEqual(sanitizeMainWindowGeometry({ width: 99999, height: 99999 }), {
    width: MAIN_WINDOW_MAX_WIDTH,
    height: MAIN_WINDOW_MAX_HEIGHT,
    maximised: false,
  });
});

test("换了小屏之后尺寸被钳制在屏幕内（按钮不会被推到屏幕外）", () => {
  const smallScreen = { width: 1366, height: 768 };
  assert.deepEqual(sanitizeMainWindowGeometry({ width: 1920, height: 1080 }, smallScreen), {
    width: 1366,
    height: 768,
    maximised: false,
  });
  // 屏幕比下限还窄时以屏幕为准
  assert.deepEqual(sanitizeMainWindowGeometry({ width: 1920, height: 1080 }, { width: 800, height: 500 }), {
    width: 800,
    height: 500,
    maximised: false,
  });
  // 不传屏幕信息时只按硬上限
  assert.equal(sanitizeMainWindowGeometry({ width: 1920, height: 1080 }).width, 1920);
});

test("最大化状态只在显式 true 时为真，并且会写进说明文案", () => {
  assert.equal(sanitizeMainWindowGeometry({ width: 1280, height: 800, maximised: true }).maximised, true);
  assert.equal(sanitizeMainWindowGeometry({ width: 1280, height: 800, maximised: "yes" }).maximised, false);

  assert.match(describeMainWindowGeometry({ width: 1280, height: 800, maximised: true }), /最大化（还原尺寸 1280×800）/);
  assert.match(describeMainWindowGeometry({ width: 1280, height: 800 }), /1280×800/);
  assert.match(describeMainWindowGeometry(null), /还没记录过/);
});

test("pickPrimaryScreen 取主屏幕，取不到时返回 null", () => {
  assert.deepEqual(
    pickPrimaryScreen([
      { width: 1920, height: 1080, isPrimary: false },
      { width: 1366, height: 768, isPrimary: true },
    ]),
    { width: 1366, height: 768 },
  );
  // 没有 isPrimary 标记时用第一个
  assert.deepEqual(pickPrimaryScreen([{ width: 2560, height: 1440 }]), { width: 2560, height: 1440 });
  assert.equal(pickPrimaryScreen([]), null);
  assert.equal(pickPrimaryScreen(null), null);
  assert.equal(pickPrimaryScreen([{ width: 0, height: 0, isPrimary: true }]), null);
});

test("window-geometry 模块真的被接线到启动流程（恢复 + 保存）", () => {
  const source = readFileSync(new URL("../features/app-runtime.js", import.meta.url), "utf8");
  assert.match(source, /restoreMainWindowGeometry/, "启动时要恢复主窗口几何");
  assert.match(source, /trackMainWindowGeometry/, "要跟踪并保存主窗口几何变化");
  assert.match(source, /WindowGetSize|WindowIsMaximised/, "要用 Wails 运行时读窗口状态");
});
