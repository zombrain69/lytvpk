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
  assert.match(previewRow, /content-visibility:\s*auto/, "加载顺序预览行要跳过屏外渲染");
  assert.match(previewRow, /contain-intrinsic-size:\s*auto\s+\d+(px|rem)/, "要给出定高估计（实测 40px）");

  const helpItem = ruleFor(readCss("app/settings.css"), ".autoexec-help-item");
  assert.ok(helpItem, "没找到 .autoexec-help-item 规则");
  assert.match(helpItem, /content-visibility:\s*auto/, "指令说明列表要跳过屏外渲染");
  assert.match(helpItem, /contain-intrinsic-size:\s*auto\s+\d+(px|rem)/, "要给出尺寸估计");

  const matchItem = ruleFor(readCss("app/diagnostics.css"), ".autoexec-match");
  assert.ok(matchItem, "没找到 .autoexec-match 规则");
  assert.match(matchItem, /content-visibility:\s*auto/, "指令匹配列表要跳过屏外渲染");
});
