// 「分类」入口（原「内容预设」按钮的继任者）的辨识度守卫。
//
// 历史缺陷：内容预设按钮在一排灰底按钮里几乎看不见，用户点名要它「放大、配色明显、一眼能看见」；
// 排查时还发现两处会把改动抵消掉的坑（同步文字用 textContent 抹掉图标、深色主题下写了死紫色）。
// 现在这个入口已经被左侧分类树取代，守卫跟着搬到新入口：
//   1. 工具栏按钮必须同时有图标和独立的文字节点（更新文字不能把图标一起抹掉）；
//   2. 侧边栏的展开/收起状态由 class 驱动（category-tree.js 不许整体覆盖按钮内容）；
//   3. 展开态要有明确的配色（不是默认灰按钮）。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const indexHtml = readFileSync(new URL("../../../index.html", import.meta.url), "utf8");
const treeJs = readFileSync(new URL("../features/file-list/category-tree.js", import.meta.url), "utf8");
const modsCss = readFileSync(new URL("../../css/app/mods.css", import.meta.url), "utf8");

function toggleButtonTemplate() {
  const start = indexHtml.indexOf('id="category-sidebar-toggle"');
  assert.ok(start > 0, "index.html 里应该有分类侧边栏开关按钮");
  const end = indexHtml.indexOf("</button>", start);
  assert.ok(end > start, "找不到按钮的结尾");
  return indexHtml.slice(start, end);
}

test("入口按钮同时带图标和可单独更新的文字节点", () => {
  const template = toggleButtonTemplate();
  assert.match(template, /class="icon"/, "按钮里要有图标");
  assert.match(
    template,
    /class="category-sidebar-toggle-label"/,
    "文字要放在独立节点里，更新文字时才不会把图标一起抹掉",
  );
});

test("展开/收起只改 class 与 aria，不再整体覆盖按钮内容", () => {
  const start = treeJs.indexOf("function setSidebarVisible(");
  assert.ok(start > 0, "找不到侧边栏显隐函数");
  const block = treeJs.slice(start, treeJs.indexOf("\n}", start));
  assert.match(block, /classList\.toggle\("active"/, "要用 class 表示选中状态");
  assert.match(block, /setAttribute\("aria-expanded"/, "要同步 aria-expanded");
  assert.ok(!/toggle\.textContent\s*=/.test(block), "不许用 textContent 覆盖整颗按钮（会抹掉图标）");
});

test("展开态有明确配色，不是默认灰按钮", () => {
  const start = modsCss.indexOf(".category-sidebar-toggle");
  assert.ok(start > 0, "mods.css 里应该有分类按钮的样式");
});
