// 阅读舒适度「高对比」档的两条不变量（CSS 守卫，和 font-size-units.test.mjs 同一路数）：
//   1. 三档文字色必须保持层级：正文最醒目，次要其次，装饰性文字（计数 / 提示 / chip）最弱；
//   2. 高对比**只提正文与次要文字**，装饰文字不比标准档更抢眼 ——
//      否则满屏提示一起变黑/变亮，读起来更累，也就失去了"舒适度"档位的意义。
//
// 用相对亮度（WCAG 公式）比较，避免"看起来差不多"的模糊判断。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const comfortCss = readFileSync(new URL("../../css/app/reading-comfort.css", import.meta.url), "utf8");
const baseCss = readFileSync(new URL("../../css/app/base.css", import.meta.url), "utf8");
const darkCss = readFileSync(new URL("../../css/dark-mode.css", import.meta.url), "utf8");

function blockFor(source, selector) {
  const start = source.indexOf(`${selector} {`);
  assert.ok(start >= 0, `找不到 ${selector} 区块`);
  const open = source.indexOf("{", start);
  const end = source.indexOf("}", open);
  return source.slice(open, end);
}

function readTextVars(block) {
  const pick = (name) => {
    const match = block.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{6})`));
    return match ? match[1].toLowerCase() : "";
  };
  return { primary: pick("text-primary"), secondary: pick("text-secondary"), tertiary: pick("text-tertiary") };
}

function relativeLuminance(hex) {
  const channels = [1, 3, 5].map((index) => parseInt(hex.slice(index, index + 2), 16) / 255);
  const linear = channels.map((value) => (value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4));
  return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2];
}

const lightContrast = readTextVars(blockFor(comfortCss, 'html[data-reading-comfort="contrast"]'));
const darkContrast = readTextVars(blockFor(comfortCss, 'html.dark-mode[data-reading-comfort="contrast"]'));
const lightStandard = readTextVars(blockFor(baseCss, ":root"));
const darkStandard = readTextVars(blockFor(darkCss, "html.dark-mode"));

test("高对比档三档文字色齐全，且保持「正文 > 次要 > 装饰」的层级", () => {
  for (const [name, vars] of [["明亮", lightContrast], ["暗色", darkContrast]]) {
    for (const key of ["primary", "secondary", "tertiary"]) {
      assert.ok(vars[key], `${name}主题的高对比档缺少 --text-${key}`);
    }
  }

  // 明亮主题：文字越深越醒目
  const lightPrimary = relativeLuminance(lightContrast.primary);
  const lightSecondary = relativeLuminance(lightContrast.secondary);
  const lightTertiary = relativeLuminance(lightContrast.tertiary);
  assert.ok(lightPrimary < lightSecondary, `正文应比次要文字更醒目：${lightContrast.primary} vs ${lightContrast.secondary}`);
  assert.ok(lightSecondary < lightTertiary, `次要文字应比装饰文字更醒目：${lightContrast.secondary} vs ${lightContrast.tertiary}`);

  // 暗色主题：文字越亮越醒目
  const darkPrimary = relativeLuminance(darkContrast.primary);
  const darkSecondary = relativeLuminance(darkContrast.secondary);
  const darkTertiary = relativeLuminance(darkContrast.tertiary);
  assert.ok(darkPrimary > darkSecondary, `正文应比次要文字更醒目：${darkContrast.primary} vs ${darkContrast.secondary}`);
  assert.ok(darkSecondary > darkTertiary, `次要文字应比装饰文字更醒目：${darkContrast.secondary} vs ${darkContrast.tertiary}`);
});

test("高对比档只提正文与次要文字，装饰文字不比标准档更抢眼", () => {
  // 明亮主题：正文/次要都要比标准档更深
  assert.ok(
    relativeLuminance(lightContrast.primary) <= relativeLuminance(lightStandard.primary),
    `高对比的正文应当不浅于标准档：${lightContrast.primary} vs ${lightStandard.primary}`,
  );
  assert.ok(
    relativeLuminance(lightContrast.secondary) <= relativeLuminance(lightStandard.secondary),
    `高对比的次要文字应当不浅于标准档：${lightContrast.secondary} vs ${lightStandard.secondary}`,
  );
  // 但装饰文字不许跟着一起加深（那会变成"整屏都是重点"）
  assert.ok(
    relativeLuminance(lightContrast.tertiary) >= relativeLuminance(lightStandard.tertiary) * 0.98,
    `高对比的装饰文字应保持弱化（不深于标准档）：${lightContrast.tertiary} vs ${lightStandard.tertiary}`,
  );

  // 暗色主题：正文/次要更亮，装饰不跟着更亮
  assert.ok(
    relativeLuminance(darkContrast.primary) >= relativeLuminance(darkStandard.primary),
    `高对比的正文应当不暗于标准档：${darkContrast.primary} vs ${darkStandard.primary}`,
  );
  assert.ok(
    relativeLuminance(darkContrast.secondary) >= relativeLuminance(darkStandard.secondary),
    `高对比的次要文字应当不暗于标准档：${darkContrast.secondary} vs ${darkStandard.secondary}`,
  );
  assert.ok(
    relativeLuminance(darkContrast.tertiary) <= relativeLuminance(darkStandard.tertiary) * 1.02,
    `高对比的装饰文字应保持弱化（不亮于标准档）：${darkContrast.tertiary} vs ${darkStandard.tertiary}`,
  );
});

// 命中高亮（mark.search-hit）也是"文字颜色"的一部分：
// 高对比档下正文被提亮/加深了，底色如果还按标准档的比例混，反而会吃掉文字对比度。
function markBackgroundMix(selector) {
  const block = blockFor(comfortCss, selector);
  const match = block.match(/background:\s*color-mix\(in srgb,\s*var\(--primary\)\s*(\d+)%/);
  assert.ok(match, `${selector} 应当用 --primary 的百分比混出底色`);
  return Number(match[1]);
}

test("高对比档下命中底色比标准档更克制（正文对比度不被底色吃掉）", () => {
  const lightStandardMix = markBackgroundMix("mark.search-hit");
  const lightContrastMix = markBackgroundMix('html[data-reading-comfort="contrast"] mark.search-hit');
  const darkStandardMix = markBackgroundMix("html.dark-mode mark.search-hit");
  const darkContrastMix = markBackgroundMix('html.dark-mode[data-reading-comfort="contrast"] mark.search-hit');

  assert.ok(
    lightContrastMix < lightStandardMix,
    `明亮主题：高对比的命中底色应更淡（${lightContrastMix}% vs ${lightStandardMix}%）`,
  );
  assert.ok(
    darkContrastMix < darkStandardMix,
    `暗色主题：高对比的命中底色应更淡（${darkContrastMix}% vs ${darkStandardMix}%）`,
  );
  // 底色变淡之后，仍然要看得出来是"命中"：用更粗的下划线补足
  const contrastBlock = blockFor(comfortCss, 'html[data-reading-comfort="contrast"] mark.search-hit');
  assert.match(contrastBlock, /inset 0 -2px 0/, "要靠更明显的下划线保持可辨识度");
  // 文字颜色仍用最强的那个
  assert.match(contrastBlock, /color:\s*var\(--text-primary\)/, "明亮主题的命中文字应为正文色");
  const darkContrastBlock = blockFor(comfortCss, 'html.dark-mode[data-reading-comfort="contrast"] mark.search-hit');
  assert.match(darkContrastBlock, /color:\s*#ffffff/, "暗色主题的命中文字应为纯白");
});
