// 「搜索框被旁边的长计数文案挤扁」的守护测试。
//
// 真实缺陷（用户截图 + 真机复现，2861 个 Mod）：
//   .search-box 在 mod-management.css 里有 max-width: 21rem（336px），
//   而命中计数 #search-hit-count 就挂在这个框**里面**。
//   计数文案是 nowrap，0 命中时最长（「没有匹配「xxx」的 Mod（共 2861 个）」）；
//   .search-input 又没有 min-width 下限，于是 flex 只能从输入框身上扣宽度：
//
//     查询       计数文案宽   输入框宽
//     (空)       0px          292px
//     ak         141px        151px
//     马格南      127px        166px
//     zzzzzz     224px         69px
//     搜狗词典     236px         58px   ← 只剩左 40 + 右 16 + 边框 2，成了个只有放大镜的小方块
//
// 所以这里守两件事：
//   1. 命中计数待在 .search-box **外面**（它属于"列表状态"，不是搜索控件的一部分）；
//   2. 同一形状的「输入框 + nowrap 计数」行——冲突检测 / 策略组 / 模型统计——
//      输入框要有 min-width 下限，计数文案要允许自己被压缩（flex: 0 1 auto; min-width: 0）。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const html = readFileSync(new URL("../../../index.html", import.meta.url), "utf8");
const read = (relative) => readFileSync(new URL(relative, import.meta.url), "utf8");

const modsCss = read("../../css/app/mods.css");
const globalCss = read("../../css/global.css");
const modelStatsCss = read("../../css/app/model-stats-scan.css");
const readingComfortCss = read("../../css/app/reading-comfort.css");

function escapeRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function declarationsFor(css, selector) {
  const pattern = new RegExp("^[ \\t]*" + escapeRegExp(selector) + "[ \\t]*(?:,|\\{)", "m");
  const match = pattern.exec(css);
  assert.ok(match, `找不到 ${selector} 的规则`);
  const open = css.indexOf("{", match.index + match[0].length - 1);
  const close = css.indexOf("}", open);
  return css.slice(open + 1, close);
}

const NON_ZERO = /min-width:\s*(?!0(?:\s*(?:px|rem|em))?\s*;)(\d|\.\d)/;

function assertInputHasFloor(css, selector, note) {
  const body = declarationsFor(css, selector);
  assert.match(body, NON_ZERO, `${selector} 需要一个非 0 的 min-width，否则会被同行文案挤到只剩内边距（${note}）`);
}

function assertLabelCanShrink(css, selector, note) {
  const body = declarationsFor(css, selector);
  assert.match(body, /flex:\s*0 1 auto/, `${selector} 要允许自己被压缩（flex: 0 1 auto），否则被压的只能是输入框（${note}）`);
  assert.match(body, /min-width:\s*0/, `${selector} 需要 min-width: 0 才能真正让出宽度（${note}）`);
}

// 主搜索框
test("命中计数必须在 .search-box 外面", () => {
  const openTag = '<div class="search-box">';
  const open = html.indexOf(openTag);
  assert.ok(open > 0, "找不到 .search-box");
  const count = html.indexOf('id="search-hit-count"');
  assert.ok(count > open, "找不到 #search-hit-count");

  const inner = html.slice(open + openTag.length, count);
  const depth = (inner.match(/<div\b/g) || []).length - (inner.match(/<\/div>/g) || []).length;
  assert.ok(
    depth < 0,
    "#search-hit-count 又回到 .search-box 里了：那个框有 max-width: 21rem，长文案会把输入框压扁"
  );
});

test("主搜索框的输入框有宽度下限，计数文案自己让步", () => {
  assertInputHasFloor(modsCss, ".search-input", "Mod 列表搜索框");
  assertLabelCanShrink(readingComfortCss, ".search-hit-count", "命中计数");
});

// 同一形状的其它行：输入框 + nowrap 计数文案
const SAME_SHAPE = [
  [globalCss, ".conflict-search", ".conflict-search-count", "冲突检测"],
  [modsCss, ".strategy-group-search", ".strategy-group-visible", "策略组"],
  [modelStatsCss, ".model-stats-search", ".model-stats-search-count", "模型统计"],
];

test("同一形状的搜索行都不能被计数文案压扁", () => {
  for (const [css, input, label, note] of SAME_SHAPE) {
    assertInputHasFloor(css, input, note);
    assertLabelCanShrink(css, label, note);
  }
});
