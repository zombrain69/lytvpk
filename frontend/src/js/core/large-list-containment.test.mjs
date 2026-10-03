// 大列表的"跳过屏外渲染"必须留在 CSS 里。
//
// 真机背景（2904 个 Mod + 1020 个工坊物品）：
// - Mod 卡片有 content-visibility 时，隐藏/显示整页只要 13–25ms；
// - 工坊卡片没有时，每次切到「创意工坊」都要把 14,280 个节点重新布局，
//   实测 204–237ms（`updateActiveIndicator()` 里的 getBoundingClientRect 会强制结算）；
//   加上之后降到 78–87ms。
// 这条规则一旦被删掉，页面切换会悄悄退化回两三百毫秒，所以用测试钉住。

import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const readCss = (relative) => readFileSync(path.resolve(here, "../../css", relative), "utf8");

function ruleFor(css, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = css.match(new RegExp(`(^|\\n)${escaped}\\s*\\{[^}]*\\}`, "s"));
  return match ? match[0] : "";
}

// 注释里会写"这里原来有 backdrop-filter"之类的说明，静态断言只看声明。
const stripComments = (css) => css.replace(/\/\*[\s\S]*?\*\//g, "");

test("Mod 卡片与工坊卡片都跳过屏外渲染（保住页面切换与滚动性能）", () => {
  const modsCard = ruleFor(readCss("app/mods.css"), ".file-card");
  assert.ok(modsCard, "没找到 .file-card 规则");
  assert.match(modsCard, /content-visibility:\s*auto/, "Mod 卡片要跳过屏外渲染");
  assert.match(
    modsCard,
    /contain-intrinsic-(size|block-size):/,
    "跳过渲染时必须给出尺寸估计，否则网格尺寸会算错",
  );

  const workshopCard = ruleFor(readCss("app/workshop-browser.css"), ".workshop-card");
  assert.ok(workshopCard, "没找到 .workshop-card 规则");
  assert.match(
    workshopCard,
    /content-visibility:\s*auto/,
    "工坊卡片要跳过屏外渲染（1020 张卡时切换页面会慢 2.6 倍）",
  );
  assert.match(
    workshopCard,
    /contain-intrinsic-size:\s*auto\s+\d+(px|rem)/,
    "工坊卡片高度不固定：要用 auto 记住真实尺寸 + 一个初始估计值",
  );
});

// 加载顺序预览（2599 行）与 autoexec 帮助列表（193 条）同样是大列表：
// 让它们参与整块布局时，"开窗 / 切分栏"会被强制结算顶出 150–370ms 的长任务。
test("加载顺序预览与 autoexec 帮助列表也跳过屏外渲染", () => {
  const previewRow = ruleFor(readCss("app/mods.css"), ".load-order-preview-item");
  assert.ok(previewRow, "没找到 .load-order-preview-item 规则");
  // 预览行改成"只物化视口 ± overscan"的窗口化渲染：content-visibility 在真机上
  // 反而让滚动 23 帧 >50ms（去掉后 203 FPS / 0 长任务），所以这里反过来钉住不再依赖它。
  assert.doesNotMatch(
    stripComments(previewRow),
    /content-visibility/,
    "预览行不应再依赖 content-visibility（真机滚动反而更慢）",
  );
  const policySource = readFileSync(
    path.resolve(here, "../features/modals/load-order-policy.js"),
    "utf8",
  );
  assert.match(policySource, /computePreviewWindow\(/, "预览行要走窗口化渲染");
  assert.match(policySource, /LOAD_ORDER_PREVIEW_OVERSCAN_ROWS/, "窗口要带 overscan");

  const helpItem = ruleFor(readCss("app/settings.css"), ".autoexec-help-item");
  assert.ok(helpItem, "没找到 .autoexec-help-item 规则");
  assert.match(helpItem, /content-visibility:\s*auto/, "指令说明列表要跳过屏外渲染");
  assert.match(helpItem, /contain-intrinsic-size:\s*auto\s+\d+(px|rem)/, "要给出尺寸估计");

  const matchItem = ruleFor(readCss("app/diagnostics.css"), ".autoexec-match");
  assert.ok(matchItem, "没找到 .autoexec-match 规则");
  assert.match(matchItem, /content-visibility:\s*auto/, "指令匹配列表要跳过屏外渲染");
});

// 设置页「常用指令说明」（真机 193 条）曾经整段塞进模板 HTML：插入时一次 parse + 布局，
// 首开「游戏配置」分区要为此付 100ms 级停顿。现在改成插入后分片渲染，这里钉住这条接线。
test("设置页的常用指令说明要分片渲染，不能整段进模板", () => {
  const source = readFileSync(
    path.resolve(here, "../features/settings/settings-page.js"),
    "utf8",
  );
  assert.match(source, /AUTOEXEC_HELP_FIRST_CHUNK/, "缺少首屏批次常量");
  assert.match(source, /scheduleAutoexecHelpChunk/, "缺少分帧补齐调度");
  assert.match(
    source,
    /renderAutoexecHelpListIfNeeded\(\)/,
    "分区首次可见时要触发帮助列表填充",
  );
  assert.match(
    source,
    /requestAnimationFrame\(\(\) => requestAnimationFrame\(run\)\)/,
    "填充要延后两帧，避免搭进分区首次布局",
  );
  assert.equal(
    /renderAutoexecHelpItems\(autoexecHelp\)/.test(source),
    false,
    "不要再把整份帮助列表塞进模板 HTML（插入时会一次性 parse + 布局）",
  );
});

// 每卡片元素上的 backdrop-filter 会给每张卡建一个 backdrop root。
// 真机 A/B（2560×1440、2912 张卡全部物化、真鼠标在按钮间滑动）：
//   保留：90.8–92.2 FPS、p90 38–44ms、12 个 50–95ms 长任务
//   去掉：195.9 FPS、p90 7.2ms、0 长任务
// 单实例的大面积 blur（模态遮罩、头部、状态栏）不在此列，不动。
test("列表卡片内的元素不得使用 backdrop-filter（大列表 hover 会掉一半帧）", () => {
  const mods = stripComments(readCss("app/mods.css"));
  const badge = ruleFor(mods, ".card-badge");
  assert.ok(badge, "没找到 .card-badge 规则");
  assert.doesNotMatch(
    badge,
    /backdrop-filter/,
    "卡片徽标不能再带 backdrop-filter：23,633 个徽标在鼠标移动时会掉到 90 FPS",
  );

  const checkbox = ruleFor(mods, ".file-checkbox.card-checkbox");
  assert.ok(checkbox, "没找到 .file-checkbox.card-checkbox 规则");
  assert.doesNotMatch(
    checkbox,
    /backdrop-filter/,
    "卡片复选框也不能带 backdrop-filter：2,912 个复选框同样参与每帧重采样",
  );

  const collectionTag = ruleFor(stripComments(readCss("app/workshop-browser.css")), ".collection-card-tag");
  assert.ok(collectionTag, "没找到 .collection-card-tag 规则");
  assert.doesNotMatch(
    collectionTag,
    /backdrop-filter/,
    "工坊集合卡角标与 Mod 徽标同因，不能再带 backdrop-filter",
  );
});
