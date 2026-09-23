import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 真实缺陷（用户报告"点 mod 选项的按钮有偏移"）：
// base.css 给所有 `.btn` 加了装饰性"扫光" `.btn::before`（position:absolute; left:-100%;
// width/height:100%）。它停在按钮**左侧**、与按钮同宽，却没有 pointer-events:none ——
// 于是它会吃掉左边那个按钮的点击：点「详情」实际命中的是右边的「游戏开关」。
//
// 这里守住两件事：① .btn 的装饰伪元素必须 pointer-events:none；
// ② 全仓的装饰性 ::before/::after（content:""/none + position:absolute）都要 pointer-events:none，
//    避免同类问题在其它组件上重现。

const here = path.dirname(fileURLToPath(import.meta.url));
const cssRoot = path.resolve(here, "../../css");

function listCssFiles(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      result.push(...listCssFiles(full));
      continue;
    }
    if (entry.name.endsWith(".css")) result.push(full);
  }
  return result;
}

function stripComments(css) {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

function rulesFor(css, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const pattern = new RegExp(`(?:^|[},])\\s*${escaped}\\s*\\{([^}]*)\\}`, "g");
  const bodies = [];
  let match;
  while ((match = pattern.exec(css)) !== null) {
    bodies.push(match[1]);
  }
  return bodies;
}

const files = listCssFiles(cssRoot);
const sources = new Map(files.map((file) => [file, stripComments(readFileSync(file, "utf8"))]));

test("`.btn::before` 扫光效果不拦截点击", () => {
  const base = [...sources.entries()].find(([file]) => file.endsWith(path.join("css", "app", "base.css")));
  assert.ok(base, "找不到 app/base.css");
  const bodies = rulesFor(base[1], ".btn::before");
  assert.equal(bodies.length, 1, "应恰好有一条 .btn::before 规则");
  assert.match(bodies[0], /pointer-events:\s*none/, "装饰性扫光必须 pointer-events: none");
});

test("全仓装饰性绝对定位伪元素都设置了 pointer-events: none", () => {
  const problems = [];
  for (const [file, css] of sources) {
    const rulePattern = /([^{}]+)\{([^}]*)\}/g;
    let match;
    while ((match = rulePattern.exec(css)) !== null) {
      const selector = match[1].trim();
      const body = match[2];
      if (!/::(before|after)/.test(selector)) continue;
      if (!/position:\s*absolute/.test(body)) continue;
      if (/pointer-events:\s*none/.test(body)) continue;
      // 纯装饰（content 为空）才要求不吃点击；有文字内容的箭头/勾选等也一并要求，
      // 因为它们同样只是视觉标记。
      problems.push(`${path.relative(cssRoot, file)}: ${selector.split("\n").pop().trim()}`);
    }
  }
  assert.deepEqual(problems, [], `这些装饰伪元素会拦截点击，请加 pointer-events: none：\n${problems.join("\n")}`);
});
