import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 真实缺陷：分组筛选菜单只有 161px 宽，里面的按钮与空状态文案被挤成竖条。
// 原因是 `.dropdown-content { min-width: 120px }` 与 `.mod-group-filter-menu`
// 同为单类选择器且排在后面，覆盖了菜单自己的 min-width。
// 这里要求菜单规则必须是“组合选择器 + 足够宽度”，避免再次被通用下拉样式压掉。

const here = path.dirname(fileURLToPath(import.meta.url));
const modsCss = readFileSync(path.resolve(here, "../../../css/app/mods.css"), "utf8").replace(
  /\/\*[\s\S]*?\*\//g,
  "",
);
const groupUiSource = readFileSync(path.join(here, "group-ui.js"), "utf8");
const groupStateSource = readFileSync(path.join(here, "group-state.mjs"), "utf8");

function rulesFor(css, className) {
  const pattern = new RegExp(`([^{}]*\\.${className}[^{}]*)\\{([^}]*)\\}`, "g");
  const rules = [];
  let match;
  while ((match = pattern.exec(css)) !== null) {
    rules.push({ selector: match[1].trim().replace(/\s+/g, " "), body: match[2] });
  }
  return rules;
}

function minWidthValue(body) {
  const match = body.match(/min-width\s*:\s*([^;]+)/i);
  return match ? match[1].trim() : null;
}

test("分组筛选菜单声明了足够宽的最小宽度", () => {
  const rules = rulesFor(modsCss, "mod-group-filter-menu");
  assert.ok(rules.length > 0, "缺少 .mod-group-filter-menu 规则");

  const widthRule = rules
    .map((rule) => ({ ...rule, minWidth: minWidthValue(rule.body) }))
    .find((rule) => rule.minWidth && /\d/.test(rule.minWidth));
  assert.ok(widthRule, "菜单必须显式声明 min-width（否则会被 .dropdown-content 压窄）");

  const numeric = Number((widthRule.minWidth.match(/(\d+(?:\.\d+)?)px/) || [])[1] || 0);
  assert.ok(numeric >= 320, `菜单 min-width 至少 320px，当前为 ${widthRule.minWidth}`);
});

test("分组筛选菜单选择器带上下文，能压过通用 .dropdown-content", () => {
  const rules = rulesFor(modsCss, "mod-group-filter-menu");
  const hasScopedRule = rules.some(
    (rule) => rule.selector.split(/\s+/).filter((part) => part.includes(".")).length >= 2,
  );
  assert.ok(
    hasScopedRule,
    "菜单规则需要组合选择器（如 .dropdown-host .mod-group-filter-menu）才能覆盖 .dropdown-content 的 min-width",
  );
});

test("分组筛选菜单向左对齐展开，避免被 .page-view 的 overflow: hidden 裁掉", () => {
  const rules = rulesFor(modsCss, "mod-group-filter-menu");
  const aligned = rules.some(
    (rule) => /left\s*:\s*0/.test(rule.body) && /right\s*:\s*auto/.test(rule.body),
  );
  assert.ok(
    aligned,
    "较宽的菜单必须 left: 0 / right: auto，否则会溢到 Mod 管理页外被裁剪（点击会落到下层元素）",
  );
});

test("分组筛选菜单的顺序由组权重决定，并显示层级缩进与顺序说明", () => {
  // 顺序在 group-state 里统一算：完全按权重升序（未设置排最后）。
  assert.match(groupStateSource, /sortGroupFilterOptions\(buildGroupFilterOptions\(memberships\)\)/);
  // 菜单渲染：子组缩进 + 顺序说明 + 悬停解释"权重在哪设置、层级不影响优先级"。
  assert.match(groupUiSource, /formatGroupOptionIndent\(option\) \+ formatGroupOptionLabel\(option\)/);
  assert.match(groupUiSource, /mod-group-filter-order-hint/);
  assert.match(groupUiSource, /顺序＝组权重（未设置排最后）· 子组紧跟上级分组/);
  assert.match(groupUiSource, /「上级分组」只决定层级归属与缩进/);
  const rules = rulesFor(modsCss, "mod-group-filter-order-hint");
  assert.ok(rules.length > 0, "顺序说明需要自己的样式");
});

test("浮动窗口的样式是通用的（所有管理窗口共用一份）", () => {
  // 通用规则在 app/modals-details.css 的 .modal.is-floating 里；详细断言见 core/floating-modal.test.mjs。
  const detailsCss = readFileSync(path.resolve(here, "../../../css/app/modals-details.css"), "utf8");
  assert.match(detailsCss, /\.modal\.is-floating\s*\{/, "缺少通用浮动窗口样式");
  assert.equal(
    /is-strategy-group-dragging/.test(modsCss),
    false,
    "策略组窗口不应再自带一套拖动样式（已收敛到通用模块）",
  );
});
