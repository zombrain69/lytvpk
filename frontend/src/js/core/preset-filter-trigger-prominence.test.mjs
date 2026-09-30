// 「内容预设」按钮的辨识度守卫。
//
// 真实缺陷：这个按钮在一排灰底按钮里几乎看不见，用户直接点名要它「放大、配色明显、
// 一眼能看见」。排查时又发现两个把改动抵消掉的坑，所以这里逐条钉住：
//   1. syncSecondaryTagFilterUI() 用 trigger.textContent 写文字，会把按钮里的图标一起抹掉；
//   2. mod-management.css 里另有一份写死 #6d28d9 的覆盖，深色主题下深紫字压在深底上读不出来。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const filters = readFileSync(
  new URL("../features/file-list/filters.js", import.meta.url),
  "utf8"
);
const modsCss = readFileSync(new URL("../../css/app/mods.css", import.meta.url), "utf8");
const modManagementCss = readFileSync(new URL("../../css/mod-management.css", import.meta.url), "utf8");
const darkModeCss = readFileSync(new URL("../../css/dark-mode.css", import.meta.url), "utf8");

function declarationsFor(css, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = new RegExp("^[ \\t]*" + escaped + "[ \\t]*(?:,|\\{)", "m").exec(css);
  assert.ok(match, `找不到 ${selector} 的规则`);
  const open = css.indexOf("{", match.index + match[0].length - 1);
  const close = css.indexOf("}", open);
  return css.slice(open + 1, close);
}

// 只看「内容预设」自己那个 forEach：
// 子标签按钮（.multi-select-trigger）没有图标，它写 textContent 是正常的。
function presetSyncBlock() {
  const start = filters.indexOf(".secondary-preset-dropdown .preset-filter-trigger");
  assert.ok(start > 0, "找不到「内容预设」按钮的同步代码");
  const end = filters.indexOf("\n  });", start);
  assert.ok(end > start, "找不到「内容预设」同步代码的结尾");
  return filters.slice(start, end);
}

test("按钮模板同时带图标和可单独更新的文字节点", () => {
  const start = filters.indexOf("dropdown.innerHTML = `");
  assert.ok(start > 0, "找不到「内容预设」按钮的模板");
  const template = filters.slice(start, filters.indexOf("`;", start));
  assert.match(template, /class="icon-svg"/, "按钮模板里要有图标");
  assert.match(
    template,
    /class="preset-filter-trigger-label"/,
    "文字要包在 .preset-filter-trigger-label 里，否则同步文字时会连图标一起抹掉"
  );
});

test("同步文字只改 label，不再整体覆盖按钮内容", () => {
  const block = presetSyncBlock();
  // 允许保留「找不到 label 节点时退回写按钮」的兜底分支，但不能有无条件覆盖。
  const withoutFallback = block.replace(/\}\s*else\s*\{[^}]*\}/g, "}");
  assert.doesNotMatch(
    withoutFallback,
    /trigger\.textContent\s*=/,
    "trigger.textContent = … 会删掉按钮里的图标（这正是图标消失的原因）"
  );
  assert.match(block, /preset-filter-trigger-label/, "应当更新 .preset-filter-trigger-label 的文字");
});

test("按钮配色只由 .preset-filter-trigger 一处决定，且跟着主题走", () => {
  const base = declarationsFor(modsCss, ".preset-filter-trigger");
  assert.match(base, /background:/, "缺少强调底色");
  assert.match(base, /color:\s*var\(--preset-trigger-ink\)/, "文字色要走主题变量，不能写死");
  assert.match(base, /font-weight:\s*(700|800)/, "要加粗才显眼");
  assert.match(base, /min-width:/, "要比一排灰按钮更宽");
  assert.match(base, /height:/, "要比一排灰按钮略高");

  assert.doesNotMatch(
    modManagementCss,
    /\.preset-filter-trigger\s*\{[^}]*color:\s*#/s,
    "mod-management.css 里不能再有写死颜色的覆盖（深色主题下会读不出来）"
  );
  assert.doesNotMatch(
    modManagementCss,
    /\.preset-filter-trigger\s*\{[^}]*height:\s*2\.1rem/s,
    "mod-management.css 里不能再把按钮压回 2.1rem"
  );

  const dark = declarationsFor(darkModeCss, "html.dark-mode .preset-filter-trigger");
  assert.match(
    dark,
    /--preset-trigger-ink:\s*#c7d2fe/,
    "深色主题要换成浅色字（深紫字压在深底上读不出来就是这次的根因）"
  );
});
