import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// Mod 管理工具栏曾在 1400x900 窗口下出现“按钮互相压住”的真实缺陷：
// 目录选择器被压缩成 0 宽度后内容溢出，压在右侧动作按钮上。
// 这里锁住修复后的布局约束，避免后续再退回不可换行的单行工具栏。

const here = path.dirname(fileURLToPath(import.meta.url));
const cssRoot = path.resolve(here, "../../../css");

function readCss(relativePath) {
  // 去掉注释，避免声明解析被 /* ... */ 打断。
  return readFileSync(path.join(cssRoot, relativePath), "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
}

// 返回某个选择器最后一次出现的声明块内容，便于按层叠顺序判断最终生效值。
function lastRuleBody(css, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const pattern = new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, "g");
  let match;
  let body = null;
  while ((match = pattern.exec(css)) !== null) {
    body = match[1];
  }
  return body;
}

function declaration(body, property) {
  if (!body) return null;
  const pattern = new RegExp(`(?:^|;)\\s*${property}\\s*:\\s*([^;]+)`, "i");
  const match = body.match(pattern);
  return match ? match[1].trim() : null;
}

const modManagementCss = readCss("mod-management.css");

test("工具栏行允许换行，避免动作按钮被挤出容器后互相覆盖", () => {
  const body = lastRuleBody(modManagementCss, ".mod-management-page .filter-row-tools");
  assert.ok(body, "缺少 .mod-management-page .filter-row-tools 规则");
  assert.equal(
    declaration(body, "flex-wrap"),
    "wrap",
    "工具栏行必须允许换行（flex-wrap: wrap），否则窗口变窄时会溢出重叠",
  );
});

test("目录选择器不会被压缩到 0 宽度后溢出压住右侧按钮", () => {
  const body =
    lastRuleBody(modManagementCss, ".mod-management-page .toolbar-directory") ??
    lastRuleBody(modManagementCss, ".mod-management-page .directory-selector");
  assert.ok(body, "缺少工具栏目录选择器的宽度约束规则");
  const minWidth = declaration(body, "min-width");
  assert.ok(minWidth, "目录选择器必须声明 min-width");
  assert.notEqual(minWidth, "0", "目录选择器 min-width 不能为 0，否则内容会溢出重叠");
});

test("动作区允许换行且不会撑破工具栏行", () => {
  const body = lastRuleBody(modManagementCss, ".mod-management-page .filter-actions");
  assert.ok(body, "缺少 .mod-management-page .filter-actions 规则");
  assert.equal(
    declaration(body, "flex-wrap"),
    "wrap",
    "动作区必须允许换行，否则会超出工具栏可用宽度",
  );
  assert.equal(
    declaration(body, "min-width"),
    "0",
    "动作区需要 min-width: 0，避免以内容宽度撑破容器",
  );
});
