import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 分组建议弹窗的"阅览"能力：可缩放、可搜索、可按置信度/来源筛选、默认折叠成卡片、
// 以及分页渲染（234 条建议时不能一次性塞满 DOM）。
// 这里锁住这些约束，避免后续改回"一屏两条、只能滚到底"的形态。

const here = path.dirname(fileURLToPath(import.meta.url));
const frontendRoot = path.resolve(here, "../../../..");
const indexHtml = readFileSync(path.join(frontendRoot, "index.html"), "utf8");
const cssSource = readFileSync(
  path.resolve(frontendRoot, "src/css/app/mods.css"),
  "utf8",
).replace(/\/\*[\s\S]*?\*\//g, "");
const groupUi = readFileSync(path.join(here, "group-ui.js"), "utf8");

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

test("分组建议弹窗提供搜索、置信度、来源与排序控件", () => {
  ["search", "confidence", "source", "sort"].forEach((name) => {
    assert.match(
      indexHtml,
      new RegExp(`id="mod-group-suggest-${name}"`),
      `index.html 缺少 id="mod-group-suggest-${name}"`,
    );
  });
  assert.match(indexHtml, /id="mod-group-suggest-filter-summary"/, "缺少筛选结果计数");
  assert.match(indexHtml, /id="mod-group-suggest-expand-all-btn"/, "缺少「全部展开」按钮");
  assert.match(indexHtml, /id="mod-group-suggest-collapse-all-btn"/, "缺少「全部折叠」按钮");
  assert.match(indexHtml, /id="mod-group-suggest-more-btn"/, "缺少「显示更多建议」按钮");
});

test("弹窗可以拖动缩放，并限制最小尺寸", () => {
  const body = lastRuleBody(cssSource, "#mod-group-suggest-modal .mod-group-suggest-content");
  assert.ok(body, "缺少 #mod-group-suggest-modal .mod-group-suggest-content 规则");
  assert.equal(declaration(body, "resize"), "both", "弹窗必须可以双向拖动缩放");
  assert.ok(declaration(body, "min-width"), "弹窗需要最小宽度，避免被拖成一条缝");
  assert.ok(declaration(body, "min-height"), "弹窗需要最小高度");
  assert.match(groupUi, /ResizeObserver/, "需要监听缩放后的尺寸，才能记住用户拖出来的大小");
  assert.match(groupUi, /lytvpk\.modGroupSuggestModalSize/, "弹窗尺寸需要持久化到 localStorage");
});

test("建议默认折叠成摘要卡片，展开后才渲染成员明细", () => {
  assert.match(groupUi, /is-collapsed/, "缺少折叠态样式类");
  assert.match(groupUi, /mod-group-suggest-preview/, "折叠卡片需要成员预览");
  assert.match(groupUi, /suggestionExpanded/, "需要记录哪些卡片被展开");
  // 折叠态用"选择器组"同时隐藏成员明细与操作区，所以按整块规则匹配。
  assert.match(
    cssSource,
    /\.mod-group-suggest-card\.is-collapsed[^{]*\{[^}]*display:\s*none/,
    "折叠态必须隐藏成员明细与操作区",
  );
});

test("建议列表分页渲染而不是一次性铺开", () => {
  assert.match(groupUi, /from\s+"\.\/suggestion-filter\.mjs"/, "需要复用纯函数筛选/分页模块");
  assert.match(groupUi, /paginateSuggestions\(/, "缺少分页渲染");
  assert.match(groupUi, /suggestionVisibleCount = SUGGESTION_PAGE_SIZE/, "重新推导后应回到第一页");
  assert.match(groupUi, /suggestionVisibleCount \+= SUGGESTION_PAGE_SIZE/, "「显示更多」应逐页增加");
});

test("筛选变化只重画列表，不重新推导（避免每次都解析 VPK）", () => {
  assert.match(groupUi, /function rerenderSuggestionView/, "缺少只重画列表的入口");
  assert.match(groupUi, /applySuggestionView\(appState\.groupSuggestions \|\| \[\], suggestionFilter\)/);
});

test("策略组在 Mod 管理页就能重命名与删除", () => {
  assert.match(groupUi, /RenameModStrategyGroup/, "分组菜单缺少重命名");
  assert.match(groupUi, /confirmDeleteStrategyGroup/, "分组菜单缺少删除入口");
  assert.match(groupUi, /AddModStrategyGroupMembers/, "分组菜单缺少「把选中的 Mod 加入组」");
  assert.match(groupUi, /RemoveModStrategyGroupMembers/, "分组菜单缺少「把选中的 Mod 移出组」");
});

test("标签与分组建议互相利用：可隐藏标签已覆盖的建议，也能用标签筛选", () => {
  // 筛选栏里必须有"标签已覆盖"的开关（默认隐藏这类建议）。
  assert.match(indexHtml, /id="mod-group-suggest-tag-covered"/);
  assert.match(groupUi, /isSuggestionTagCovered/, "卡片需要判断标签是否已覆盖");
  assert.match(groupUi, /用标签筛选/, "标签已覆盖的建议要能一键改用标签筛选");
  assert.match(groupUi, /applyTagFilterFromSuggestion/, "缺少「用标签筛选」的实现");
  assert.match(groupUi, /appState\.selectedSecondaryTags = \[tag\]/, "用标签筛选需要写入二级标签筛选");
});

test("分组菜单提供「打标签…」，并接到标签对话框", () => {
  assert.match(groupUi, /"打标签…"/);
  assert.match(groupUi, /openGroupTagDialog\(/);
  assert.match(indexHtml, /id="group-tag-modal"/);
  assert.match(indexHtml, /id="group-tag-input"/);
  assert.match(indexHtml, /id="group-tag-apply-btn"/);
});

test("建议文件里带的标签可以直接应用（导入标签 → 落盘）", () => {
  assert.match(groupUi, /buildTagApplyPlan\(suggestion\)/, "需要把 tag / memberTags 整理成应用计划");
  assert.match(groupUi, /ApplyTagToModKeys/, "缺少应用标签的绑定调用");
  assert.match(groupUi, /applyImportedTags\(/, "缺少「应用标签」的实现");
  assert.match(groupUi, /导入标签：/, "卡片需要显示导入标签徽标");
});

test("准备材料 / 导出清单会先重新扫描，并给出进行中反馈", () => {
  // 两个按钮的 tooltip 都要说明"先重新扫描"
  assert.match(indexHtml, /先重新扫描一遍 mod 目录/, "按钮提示需要写明会先扫描");
  // 导出期间要有进行中状态，结束后恢复
  assert.match(groupUi, /正在扫描并导出…/, "需要「正在扫描并导出…」的进行中反馈");
  assert.match(groupUi, /已按最新扫描导出 Mod 清单/, "成功提示要说明是按最新扫描导出的");
  const prepareBlock = groupUi.slice(
    groupUi.indexOf("async function prepareAgentWorkspace"),
    groupUi.indexOf("async function copyAgentPrompt"),
  );
  assert.match(prepareBlock, /prepareButton\.disabled = true/, "准备材料期间按钮应禁用");
  assert.match(prepareBlock, /finally\s*\{/, "准备材料必须恢复按钮状态（finally）");
  const exportBlock = groupUi.slice(
    groupUi.indexOf("async function exportGroupingCatalog"),
    groupUi.indexOf("async function clearExternalGroupSuggestions"),
  );
  assert.match(exportBlock, /exportButton\.disabled = true/, "导出清单期间按钮应禁用");
  assert.match(exportBlock, /finally\s*\{/, "导出清单必须恢复按钮状态（finally）");
  // 材料时效性提示：材料就绪弹窗要展示后端给的 notice
  assert.match(groupUi, /workspace\.notice/, "材料就绪弹窗要显示时效性提示（notice）");
});

test("提示词可编辑：编辑器结构、入口与三个后端方法都接好了", () => {
  assert.match(indexHtml, /id="mod-group-suggest-prompt-btn"/, "缺少「编辑提示词…」入口");
  assert.match(indexHtml, /id="agent-prompt-modal"/);
  assert.match(indexHtml, /id="agent-prompt-text"/);
  assert.match(indexHtml, /id="agent-prompt-placeholder-list"/);
  assert.match(indexHtml, /id="agent-prompt-save-btn"/);
  assert.match(indexHtml, /id="agent-prompt-reset-btn"/);

  const editorSource = readFileSync(path.join(here, "agent-prompt.js"), "utf8");
  assert.match(editorSource, /GetGroupSuggestionAgentPromptState/);
  assert.match(editorSource, /SaveGroupSuggestionAgentPrompt/);
  assert.match(editorSource, /ResetGroupSuggestionAgentPrompt/);
  assert.match(editorSource, /insertPromptPlaceholder/, "占位符要能一键插入");
  assert.match(groupUi, /openAgentPromptEditor/);
  // 入口在「分组建议」弹窗里，必须比普通模态框更高一层，否则会被盖住看不见。
  assert.match(
    cssSource,
    /#agent-prompt-modal\s*\{[^}]*z-index:\s*var\(--z-popover\)/,
    "编辑提示词弹窗需要更高的 z-index（它的入口在分组建议弹窗内部）",
  );
});
