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
