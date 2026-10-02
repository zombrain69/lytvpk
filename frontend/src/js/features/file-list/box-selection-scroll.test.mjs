// 框选的"拖动中滚动 → 取消框选"必须挂在真正的滚动容器上。
//
// 真机回归（2026-10-02）：监听原来挂在 .file-list-container 上，而它 CSS 是
// overflow:hidden —— 真正滚动的是 #file-list，scroll 事件又不冒泡，
// 于是这条保护从来没触发过：用户拖着框选中途滚动，选择框和已选高亮会留在屏幕上
// 但坐标已经失真。这里用静态断言钉住接线。

import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));

test("框选滚动取消要挂在真正的滚动容器 #file-list 上", () => {
  const source = readFileSync(path.resolve(here, "box-selection.js"), "utf8");
  assert.match(
    source,
    /getElementById\("file-list"\)\?\.addEventListener\("scroll", handleScroll/,
    "滚动取消必须监听 #file-list（外层容器 overflow:hidden，收不到 scroll）",
  );
  assert.match(source, /function handleScroll\(\)/, "缺少滚动取消的实现");
});
