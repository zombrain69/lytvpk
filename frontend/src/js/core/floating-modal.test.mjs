import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 用户反馈：打开管理窗口后主界面被遮罩 + backdrop-filter 虚化，"我看怎么看清主界面"。
// 这里锁住三件事：① 浮动时去掉虚化与遮罩；② 主界面仍然可点；③ 复杂管理窗口都接上这套能力。

const here = path.dirname(fileURLToPath(import.meta.url));
const moduleSource = readFileSync(path.join(here, "floating-modal.js"), "utf8");
const detailsCss = readFileSync(path.resolve(here, "../../css/app/modals-details.css"), "utf8").replace(
  /\/\*[\s\S]*?\*\//g,
  "",
);
const runtimeSource = readFileSync(
  path.resolve(here, "../features/app-runtime.js"),
  "utf8",
);
const managerSource = readFileSync(
  path.resolve(here, "../features/mod-groups/strategy-group-manager.js"),
  "utf8",
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

test("浮动窗口去掉遮罩与背景虚化（用户反馈：主界面变模糊看不见）", () => {
  const rules = rulesFor(detailsCss, "is-floating");
  assert.ok(rules.length > 0, "缺少 .modal.is-floating 规则");
  const base = rules.find((rule) => !rule.selector.includes(".modal-content") && !rule.selector.includes(".modal-header"));
  assert.ok(base, "缺少容器级浮动规则");
  assert.match(base.body, /backdrop-filter\s*:\s*none/, "必须关掉 backdrop-filter，否则主界面还是糊的");
  assert.match(base.body, /background\s*:\s*transparent/, "必须去掉遮罩底色");
  // -webkit- 前缀也要一起关，WebView2 / Safari 内核才生效。
  assert.match(base.body, /-webkit-backdrop-filter\s*:\s*none/);
  assert.ok(
    rules.some((rule) => rule.selector.includes(".modal-content") && /pointer-events\s*:\s*auto/.test(rule.body)),
    "窗口本体要 pointer-events: auto，否则窗口自身点不动",
  );
  assert.match(base.body, /pointer-events\s*:\s*none/, "容器要放行鼠标事件，主界面才能点");
});

test("浮动模块提供 安装 / 切换 / 拖动 / 位置记忆 四件事", () => {
  assert.match(moduleSource, /export function setupFloatingModal/);
  assert.match(moduleSource, /export function applyFloatingModal/);
  assert.match(moduleSource, /export function startModalDrag/);
  assert.match(moduleSource, /export function isFloatingModal/);
  // 位置与偏好都记住（localStorage），窗口重开时恢复。
  assert.match(moduleSource, /lytvpk\.floatingPos\./);
  assert.match(moduleSource, /lytvpk\.floating\./);
  // 打开弹窗（去掉 hidden）时自动重新应用浮动状态。
  assert.match(moduleSource, /new MutationObserver/);
  // 已有的勾选框可以直接当开关（策略组管理窗口），其它窗口自动插一个按钮。
  assert.match(moduleSource, /modal-float-toggle/);
  // 标题栏不只是 .modal-header：「加载顺序优化」用的是 .load-order-header，
  // 没有兜底就拿不到拖动把手、也插不进浮动开关。
  assert.match(moduleSource, /\.load-order-header/, "标题栏选择器要覆盖 load-order-header");
  // 缩放不重复实现：交给 modal-resizer。
  assert.match(moduleSource, /modal-resizer/);
});

test("停靠必须清掉内联 max-width / max-height（真实缺陷：开关被挤出视口）", () => {
  // CSSStyleDeclaration.removeProperty 只认连字符写法：写 "maxHeight" 会静默失败，
  // 于是 max-height: none 残留 —— 「加载顺序优化」停靠后内容撑到 1901px、标题栏被顶到 y=-471，
  // 用户再也点不到「浮动窗口」按钮。
  const clearBlock = moduleSource.slice(
    moduleSource.indexOf("const INLINE_POSITION_PROPS"),
    moduleSource.indexOf("function pinContent"),
  );
  assert.match(clearBlock, /"max-width"/, "要用连字符写法清 max-width");
  assert.match(clearBlock, /"max-height"/, "要用连字符写法清 max-height");
  assert.equal(/"maxWidth"/.test(clearBlock), false, "驼峰写法在 removeProperty 里无效");
  assert.equal(/"maxHeight"/.test(clearBlock), false, "驼峰写法在 removeProperty 里无效");
  // 兜底：如果内容仍然高于视口，改成顶部对齐，保证标题栏（含开关）始终可见。
  assert.match(moduleSource, /alignItems = "flex-start"/);
});

test("复杂管理窗口都装了浮动能力（默认浮动，主界面可见可点）", () => {
  [
    "mod-group-suggest-modal",
    "load-order-modal",
    "conflict-modal",
    "file-conflict-modal",
    "problem-scan-modal",
    "model-stats-modal",
  ].forEach((id) => {
    assert.match(runtimeSource, new RegExp(`"${id}"`), `app-runtime 没给 ${id} 装浮动`);
  });
  assert.match(runtimeSource, /setupFloatingModal\(modalId, \{ defaultFloating: true \}\)/);
  // 策略组管理窗口：也用通用开关（标题栏按钮），只是偏好写回 config.json。
  assert.match(managerSource, /setupFloatingModal\(element\("strategy-group-modal"\)/);
  assert.match(managerSource, /defaultFloating: getConfig\(\)\.strategyGroupFloating !== false/);
  assert.match(managerSource, /getFloatingModal\("strategy-group-modal"\)\?\.refresh\(\)/);
  assert.match(managerSource, /write: \(enabled\) => \{[\s\S]{0,120}config\.strategyGroupFloating = Boolean\(enabled\)/);
  assert.equal(
    /strategy-group-floating/.test(managerSource),
    false,
    "策略组窗口不应再有自己的浮动勾选框（已统一成通用按钮）",
  );
});

test("停靠状态点窗口外要能关闭（统一处理，会改 Mod 状态的窗口除外）", () => {
  // 以前"点外面关闭"是每个窗口在 app-runtime 里各写一份，新接入的窗口就漏了。
  assert.match(moduleSource, /closeOnBackdrop/, "要支持按窗口开关这个行为");
  assert.match(moduleSource, /querySelector\("\.close-btn"\)/, "点外面要复用窗口自己的关闭按钮");
  // 只在停靠（模态）状态处理：浮动态容器 pointer-events: none，事件本来就到不了。
  assert.match(moduleSource, /if \(element\.classList\.contains\(FLOATING_CLASS\)\) return;/);
  // 「问题 Mod 查找」会改 Mod 开关，必须保持打开：显式关掉这个行为。
  assert.match(runtimeSource, /setupFloatingModal\("problem-scan-modal", \{ defaultFloating: true, closeOnBackdrop: false \}\)/);
  // 策略组管理窗口不再自己写一份（收敛到通用模块）。
  assert.equal(
    /if \(event\.target === modal\) closeStrategyGroupManager\(\)/.test(managerSource),
    false,
    "策略组窗口不应再自带一套点外关闭",
  );
});
