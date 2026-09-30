import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// 「窗口拉宽了，文字还是缩着」的防线：
// 标题/文件名一开始用 max-width: 20ch / 15ch 之类的固定字数上限（还有 24/26/34/28ch），
// 于是模态窗口被拉宽、容器变宽之后，文字仍然停在原地被省略号截断。
// 这里把"不许再出现固定 ch 上限"和"这几处必须随容器伸缩"钉住。

const here = path.dirname(fileURLToPath(import.meta.url));
const cssRoot = path.resolve(here, "../../css");

function listCSSFiles(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) result.push(...listCSSFiles(full));
    else if (entry.name.endsWith(".css")) result.push(full);
  }
  return result;
}

const cssFiles = listCSSFiles(cssRoot);
const read = (name) => readFileSync(path.join(cssRoot, name), "utf8");

test("CSS 里不再有 max-width: Nch 这类固定字数上限", () => {
  const offenders = [];
  for (const file of cssFiles) {
    const source = readFileSync(file, "utf8");
    for (const match of source.matchAll(/max-width:\s*[0-9.]+ch/g)) {
      offenders.push(`${path.relative(cssRoot, file)}: ${match[0]}`);
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `固定 ch 上限会让"窗口拉宽但文字不展开"，请改成 max-width: 100%：\n${offenders.join("\n")}`,
  );
});

test("Mod 标题与文件名改成跟随可用宽度伸缩", () => {
  const global = read("global.css");
  const title = global.match(/\.conflict-vpk-title\s*\{[^}]*\}/s)?.[0] || "";
  const filename = global.match(/\.conflict-vpk-filename\s*\{[^}]*\}/s)?.[0] || "";
  for (const [label, block] of [["标题", title], ["文件名", filename]]) {
    assert.match(block, /min-width:\s*0/, `${label}要允许收缩（min-width: 0）`);
    assert.match(block, /max-width:\s*100%/, `${label}最多占满容器（max-width: 100%）`);
    assert.match(block, /text-overflow:\s*ellipsis/, `${label}放不下时仍然用省略号`);
  }
});

test("分组建议窗口的成员列表不再用固定像素高度卡住", () => {
  const mods = read("app/mods.css");
  const block =
    mods.match(/#mod-group-suggest-modal\s+\.mod-group-suggest-members\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(block, "没找到 #mod-group-suggest-modal .mod-group-suggest-members 规则");
  assert.doesNotMatch(block, /max-height:\s*[0-9]+px/, "固定 px 高度会让窗口拉高后列表不变高");
  assert.match(block, /max-height:\s*none/, "应交给外层滚动，跟随窗口高度");
});

test("策略组成员行在加了勾选框之后仍然让中间那列自己撑开", () => {
  const mods = read("app/mods.css");
  const row = mods.match(/#strategy-group-modal\s+\.mod-member-row\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(row, "没找到 #strategy-group-modal .mod-member-row 规则");
  assert.match(
    row,
    /grid-template-columns:\s*auto\s+minmax\(0,\s*1fr\)\s+auto/,
    "勾选框 + 成员信息 + 行内按钮三列：中间列必须用 minmax(0,1fr) 才能随窗口展开",
  );
});

// 第二类"缩着"：窗口里的清单/说明被固定 px 高度或宽度卡住，窗口拉大也不动。
// 统一改成"跟随窗口高度"（flex: 1 1 auto; max-height: none）或 max-width: 100%。
const WINDOW_CONTENT_RULES = [
  ["app/diagnostics.css", ".archive-manager-tree"],
  ["app/diagnostics.css", ".archive-manager-conflict-choice ul"],
  ["app/diagnostics.css", ".vpk-integrity-issues"],
  ["app/diagnostics.css", ".autoexec-matches"],
  ["app/problem-scan.css", ".problem-scan-candidate-list"],
  ["app/servers.css", ".panel-player-list-container"],
  ["app/servers.css", ".panel-rcon-output"],
  ["app/forms-tags.css", ".player-list-container"],
  ["app/mdmp-report.css", ".mdmp-report-two-col"],
  ["app/settings.css", ".addonlist-merge-conflicts"],
];

test("窗口内的清单跟随窗口高度，不再用固定 px 高度卡住", () => {
  const offenders = [];
  for (const [file, selector] of WINDOW_CONTENT_RULES) {
    const source = read(file);
    const block =
      source.match(new RegExp(`\\${selector}\\s*\\{[^}]*\\}`, "s"))?.[0] || "";
    if (!block) {
      offenders.push(`${file} ${selector}: 规则不存在`);
      continue;
    }
    if (/max-height:\s*[0-9.]+(px|rem)/.test(block)) {
      offenders.push(`${file} ${selector}: 仍然写死高度`);
    }
    if (!/max-height:\s*none/.test(block)) {
      offenders.push(`${file} ${selector}: 缺少 max-height: none`);
    }
    if (!/flex:\s*1 1 auto/.test(block)) {
      offenders.push(`${file} ${selector}: 缺少 flex: 1 1 auto（不跟随窗口高度）`);
    }
  }
  assert.deepEqual(offenders, [], `这些窗口内容不会随窗口变大：\n${offenders.join("\n")}`);
});

// 第三类"缩着"：页面/弹窗里的文字区被固定 px 高度卡住（窗口拉高也不展开）。
// 这些区域外层本来就能滚动，直接放开高度上限即可。
const WINDOW_TEXT_RULES = [
  ["app/forms-tags.css", ".download-tasks-list", "none"],
  ["app/settings.css", ".autoexec-help-column", "viewport"],
  ["app/updates.css", ".notes-content", "none"],
  ["app/drop-import.css", ".drop-import-results", "none"],
];

// 同一个选择器可能出现多次（例如基础布局 + 具体尺寸），这里把所有匹配块合并判断。
function selectorBlocks(source, selector) {
  return [...source.matchAll(new RegExp(`\\${selector}\\s*\\{[^}]*\\}`, "gs"))].map(
    (match) => match[0],
  );
}

test("文字/任务区跟随窗口高度，不再固定高度", () => {
  const offenders = [];
  for (const [file, selector, mode] of WINDOW_TEXT_RULES) {
    const source = read(file);
    const blocks = selectorBlocks(source, selector);
    if (blocks.length === 0) {
      offenders.push(`${file} ${selector}: 规则不存在`);
      continue;
    }
    if (blocks.some((block) => /max-height:\s*[0-9.]+(px|rem)/.test(block))) {
      offenders.push(`${file} ${selector}: 仍然写死高度`);
    }
    if (mode === "none" && !blocks.some((block) => /max-height:\s*none/.test(block))) {
      offenders.push(`${file} ${selector}: 缺少 max-height: none`);
    }
    if (mode === "viewport" && !blocks.some((block) => /max-height:\s*calc\(100vh/.test(block))) {
      offenders.push(`${file} ${selector}: 缺少随窗口高度变化的 max-height: calc(100vh …)`);
    }
  }
  assert.deepEqual(offenders, [], `这些文字区不会随窗口变大：\n${offenders.join("\n")}`);
});

// 第四类"缩着"：中文可以在任意两个字之间断行，flex 项的自动最小宽度会缩到 1 个字宽，
// 于是窗口不够宽时，一排统计项会被压成"一列一个字"（实测：每项 17px 宽、139px 高）。
// 修法是「条目内部不许换行 + 放不下整条换行」。
test("底部状态栏的统计项不会被压成一列一个字", () => {
  const mods = read("app/mods.css");
  const info = mods.match(/\.status-info\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(info, "没找到 .status-info 规则");
  assert.match(info, /flex-wrap:\s*wrap/, "统计区要允许整条换行，否则只能挤压每一条");
  const span = mods.match(/\.status-info span\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(span, "没找到 .status-info span 规则");
  assert.match(span, /white-space:\s*nowrap/, "统计项内部不许断行，否则会一字一行");
});

// 第五类"缩着"：卡片徽标（主体/优先级/分组/冲突角标…）原来写死 max-width，
// 卡片变宽、窗口变宽都不会展开，只能用省略号收尾。
const CARD_BADGE_RULES = [
  [".card-badge", /max-width:\s*100%/],
  [".card-badge.subject-badge", /max-width:\s*100%/],
  [".card-badge.secondary-tag-badge.is-long", /max-width:\s*100%/],
  [".card-badge.xdr-badge", /max-width:\s*100%/],
  [".mod-conflict-badge", /max-width:\s*100%/],
];

test("卡片徽标不再写死宽度，能跟着卡片展开", () => {
  const mods = read("app/mods.css");
  const offenders = [];
  for (const [selector, expected] of CARD_BADGE_RULES) {
    const blocks = selectorBlocks(mods, selector);
    if (blocks.length === 0) {
      offenders.push(`${selector}: 规则不存在`);
      continue;
    }
    const merged = blocks.join("\n");
    if (/max-width:\s*[0-9.]+(rem|px)/.test(merged)) {
      offenders.push(`${selector}: 仍然写死 max-width（窗口/卡片变宽也不会展开）`);
      continue;
    }
    if (!expected.test(merged)) {
      offenders.push(`${selector}: 缺少 max-width: 100%`);
    }
  }
  assert.deepEqual(offenders, [], `这些徽标不会随卡片展开：\n${offenders.join("\n")}`);
});

// 第六类"缩着"：两条同名规则互相顶掉。
// Mod 列表卡片的 .card-title 是单行 nowrap，创意工坊卡片的 .card-title 是两行截断；
// 前者不限定作用域时会跟着创意工坊一起生效，把工坊卡片标题压成一行。
test("Mod 卡片的单行标题不会顶掉创意工坊卡片的两行标题", () => {
  const mods = read("app/mods.css");
  const workshop = read("app/workshop-browser.css");

  const modTitle = mods.match(/(^|\n)([^\n{}]*\.card-title)\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(modTitle, "没找到 mods.css 里的 .card-title 规则");
  assert.match(
    modTitle,
    /\.file-card\s+\.card-title\s*\{/,
    "Mod 卡片的标题规则要限定在 .file-card 内，否则会波及创意工坊卡片",
  );

  const workshopTitle = workshop.match(/\.card-title\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(workshopTitle, "没找到 workshop-browser.css 里的 .card-title 规则");
  assert.match(workshopTitle, /-webkit-line-clamp:\s*2/, "创意工坊卡片标题要保留两行截断");
});
