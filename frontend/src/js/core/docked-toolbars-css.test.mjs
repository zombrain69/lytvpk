// 「动作行不能被滚走」的守护测试。
//
// 真实缺陷（用户截图）：动作条在可滚动容器内部，列表一长、用户滚到下半部分，
// 按钮就跑到视野之外。用户的原话是「别出现那种翻到下面之后，按钮都跑到不可见的
// 视野里的情况了」——所以这条测试守的是结构与声明，不是像素。
//
// 三种修法（见 layout.css 的「动作行停靠」区块）：
//   1. 单行工具行吸顶（position: sticky; top: 0）；
//   2. 多行组合整块吸顶（.mod-group-suggest-dock、.workshop-input-dock）；
//   3. 段落收尾动作吸底（position: sticky; bottom: 0）。
// 少一条就说明有人把停靠去掉了，会重演「按钮滚丢」。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const layoutCss = readFileSync(new URL("../../css/layout.css", import.meta.url), "utf8");
const modsCss = readFileSync(new URL("../../css/app/mods.css", import.meta.url), "utf8");
const html = readFileSync(new URL("../../../index.html", import.meta.url), "utf8");

function escapeRegExp(text) {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// 取「这个选择器自己那一块」的声明体。要求选择器顶到行首，这样注释里顺口提到
// 选择器（注释里不会顶行首）不会被误当成规则。
function declarationsFor(css, selector) {
  const pattern = new RegExp("^[ \\t]*" + escapeRegExp(selector) + "[ \\t]*(?:,|\\{)", "m");
  const match = pattern.exec(css);
  if (!match) return null;
  const open = css.indexOf("{", match.index + match[0].length - 1);
  if (open < 0) return null;
  const close = css.indexOf("}", open);
  if (close < 0) return null;
  return css.slice(open + 1, close);
}

function assertDocked(css, selector, anchor, note) {
  const body = declarationsFor(css, selector);
  assert.ok(body, `找不到 ${selector} 的规则（${note}）`);
  assert.match(body, /position:\s*sticky/, `${selector} 必须是 sticky，否则滚动时仍会被带走（${note}）`);
  assert.match(body, new RegExp(`${anchor}:\\s*0`), `${selector} 缺少 ${anchor}: 0（${note}）`);
  assert.match(body, /background:/, `${selector} 停靠后需要不透明底色，否则内容会从底下透出来（${note}）`);
}

const STICKY_TOP = [
  [".server-toolbar", "收藏服务器：添加服务器 / IP 直连 / 刷新"],
  [".load-order-priority-panel", "加载顺序：分层 ±1 / 保存分层 / 清除分层"],
  [".panel-action-bar", "服务器面板：刷新 / 重启 / 换图 / RCON…"],
  [".panel-map-toolbar", "面板换图：隐藏官方 / 刷新 / 热重载地图"],
  [".panel-upload-toolbar", "面板上传：上传地图 / 清空地图 / 清理已完成"],
  [".mod-group-suggest-dock", "分组建议：工具行 + 筛选栏整块吸顶"],
  [".task-list-header", "下载任务队列：标题 + 清理已完成"],
];

const STICKY_BOTTOM = [
  [".load-order-policy-actions", "加载顺序：预览优化结果 / 应用优化并写入"],
  [".detail-ignore-actions", "Mod 详情：保存忽略清单 / 清空"],
  [".server-data-actions", "收藏服务器：复制配置 / 导出文件 / 粘贴导入 / 文件导入"],
];

test("单行工具行与多行组合都吸顶", () => {
  for (const [selector, note] of STICKY_TOP) {
    assertDocked(layoutCss, selector, "top", note);
  }
});

test("段落末尾的收尾动作吸底", () => {
  for (const [selector, note] of STICKY_BOTTOM) {
    assertDocked(layoutCss, selector, "bottom", note);
  }
});

test("下载与解析页：输入行常驻，只有结果与任务队列滚动", () => {
  const body = declarationsFor(layoutCss, ".workshop-modal-content > .modal-body");
  assert.ok(body, "缺少 .workshop-modal-content > .modal-body 的规则");
  assert.match(body, /display:\s*flex/, "滚动区要变成纵向 flex 才能把输入行留在外面");
  assert.match(body, /overflow:\s*hidden/, "modal-body 自己不能再滚，否则输入行还是会跟着走");

  const slack = declarationsFor(layoutCss, ".workshop-scroll-area");
  assert.ok(slack, "缺少 .workshop-scroll-area 的规则");
  assert.match(slack, /overflow-y:\s*auto/, "结果 + 任务队列需要自己滚");
  assert.match(slack, /min-height:\s*0/, "没有 min-height: 0 时 flex 子项撑不出滚动条");

  const dock = declarationsFor(layoutCss, ".workshop-input-dock");
  assert.ok(dock, "缺少 .workshop-input-dock 的规则");
  assert.match(dock, /flex:\s*0 0 auto/, "输入行不能被压扁");
});

test("分组建议：筛选栏不再自己吸顶（吸顶收归 .mod-group-suggest-dock，避免两行互相盖住）", () => {
  const filters = declarationsFor(modsCss, ".mod-group-suggest-filters");
  assert.ok(filters, "找不到 .mod-group-suggest-filters 的规则");
  assert.doesNotMatch(
    filters,
    /position:\s*sticky/,
    "筛选栏自己再吸顶会盖住工具行——两条都 top: 0 时后一条压前一条"
  );
});

// ── HTML 结构守卫 ────────────────────────────────────────────────────────────
function divDepth(fragment) {
  return (fragment.match(/<div\b/g) || []).length - (fragment.match(/<\/div>/g) || []).length;
}

function indexOfTag(tag, from = 0) {
  const index = html.indexOf(tag, from);
  assert.ok(index > 0, `index.html 里找不到 ${tag}`);
  return index;
}

// 目标相对「容器开标签之后」的 div 深度：0 = 容器的直接子元素。
// 深度为负数说明容器已经闭合（目标跑到了容器外面）。
function depthInside(containerTag, targetTag, occurrence = 0) {
  const container = indexOfTag(containerTag);
  const start = container + containerTag.length;
  let target = indexOfTag(targetTag, start);
  for (let index = 0; index < occurrence; index += 1) {
    target = indexOfTag(targetTag, target + 1);
  }
  const depth = divDepth(html.slice(start, target));
  assert.ok(depth >= 0, `${targetTag} 不在 ${containerTag} 内部（深度 ${depth}）`);
  return depth;
}

test("下载与解析页的两层容器各自收好了成员", () => {
  const dock = '<div class="workshop-input-dock">';
  const scrollArea = '<div class="workshop-scroll-area">';

  assert.equal(depthInside(dock, '<div class="input-group">'), 0, "输入行应是 dock 的直接子元素");
  // 两条输入行必须同级（都在 .input-group 里）；只包住一条等于没修。
  assert.equal(depthInside(dock, '<div class="url-input-container">', 0), 1, "第一条输入行");
  assert.equal(depthInside(dock, '<div class="url-input-container">', 1), 1, "第二条输入行");

  assert.equal(depthInside(scrollArea, '<div id="workshop-result"'), 0, "解析结果应在滚动区里");
  assert.equal(depthInside(scrollArea, '<div class="task-list-header">'), 1, "任务队列标题行应在滚动区里");

  // 两条输入行必须在同一个 dock 内 —— 否则会退化成「各自 sticky 互相盖住」。
  assert.ok(
    indexOfTag('<div class="url-input-container">', indexOfTag(dock) + dock.length) < indexOfTag(scrollArea),
    "两条输入行都应排在滚动区之前"
  );
  assert.ok(
    indexOfTag('<div class="url-input-container">', indexOfTag('<div class="url-input-container">') + 1) <
      indexOfTag(scrollArea),
    "第二条输入行也应在滚动区之前"
  );
});

test("分组建议：工具行与筛选栏在同一个停靠容器里", () => {
  const dock = '<div class="mod-group-suggest-dock">';
  assert.equal(depthInside(dock, '<div class="mod-group-suggest-toolbar">'), 0, "工具行");
  assert.equal(depthInside(dock, '<div class="mod-group-suggest-filters">'), 0, "筛选栏");
});

test("策略组窗口的两条批量条仍在滚动区之外（HTML 结构守卫）", () => {
  const bodyOpen = indexOfTag('class="strategy-group-body"');
  for (const id of ["strategy-group-batch", "strategy-group-member-batch"]) {
    const barIndex = html.indexOf(`id="${id}"`);
    assert.ok(barIndex > bodyOpen, `${id} 应位于滚动区之后`);
    assert.equal(divDepth(html.slice(bodyOpen, barIndex)), 0, `${id} 不能在滚动区内部`);
  }
});
