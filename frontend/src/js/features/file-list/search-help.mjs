// 搜索语法的"说明书"：搜索框旁 `?` 按钮的浮层内容与悬停提示都从这里生成，
// 保证界面提示、文档、测试三处不会各写一份互相对不上的说明。

import { shortcutRowsByIds } from "../../core/shortcuts.mjs";

function escapeHtmlText(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** 语法行：写法 → 含义。 */
export const SEARCH_SYNTAX_ROWS = [
  { syntax: "ak47 武器", description: "普通词（可多个）：都要命中，字符按顺序出现即可" },
  { syntax: '"ak 47"', description: "引号里的空格算一个词" },
  { syntax: "tag:步枪", description: "必须有这个标签（一级或子标签）" },
  { syntax: "tag:步枪|狙击", description: "任一标签命中即可（或）" },
  { syntax: "tag:武器 tag:步枪", description: "两个标签都要（且）" },
  { syntax: "-tag:材质", description: "排除含这个标签的 Mod" },
  { syntax: "-材质", description: "排除「包含」这个词的 Mod（子串匹配；不做模糊，长路径不会误伤）" },
  { syntax: "re:^ak\\d+$", description: "正则匹配；写法有误会直接提示「正则表达式无效」" },
];

/**
 * 快捷键行：**从 core/shortcuts.mjs 派生**，不再手写第二份。
 * 这里只挑与"搜索"最相关的几条；要改键位说明请改那份总表。
 */
export const SEARCH_SHORTCUT_ROWS = shortcutRowsByIds([
  "focus-search",
  "command-palette",
  "search-cursor",
  "search-open",
  "search-clear",
]).map((row) => ({ syntax: row.keys, description: row.description }));

/** 参与搜索的字段（顺序 = 展示顺序）。 */
export const SEARCH_FIELD_LABELS = [
  "标题",
  "文件名",
  "一级标签",
  "子标签",
  "结构化主体",
  "发音角色",
  "动作槽",
];

/**
 * 归档管理器的说明书变体。
 * 语法是同一套（同一个解析器），差别只在"能匹配哪些字段"与 `tag:` 的含义：
 * 归档里没有一级标签，`tag:` 落在**包状态**上。
 */
export const ARCHIVE_SEARCH_HELP_VARIANT = {
  fields: ["压缩包名", "路径", "包内 VPK 名", "包内 VPK 路径", "包状态"],
  fieldNote: "大小写不敏感；命中的片段会在压缩包名里高亮，并标出是哪个包状态命中的。",
  syntaxRows: [
    { syntax: "ak47 武器", description: "普通词（可多个）：都要命中，字符按顺序出现即可" },
    { syntax: '"ak 47"', description: "引号里的空格算一个词" },
    { syntax: "-old", description: "排除「包含」old 的压缩包（子串匹配，长路径不会误伤）" },
    { syntax: "tag:密码", description: "按包状态筛：密码 / 错误 / 已有 / 待导入" },
    { syntax: "tag:密码|错误", description: "任一状态命中即可（或）" },
    { syntax: "tag:错误 -tag:密码", description: "两个状态条件同时生效（且 / 排除）" },
    { syntax: "re:^mods-", description: "正则匹配；写法有误会直接提示「正则表达式无效」" },
  ],
};

/** 默认（Mod 列表）说明书变体。 */
export const MOD_SEARCH_HELP_VARIANT = {
  fields: SEARCH_FIELD_LABELS,
  fieldNote: "大小写不敏感；命中的片段会在标题/文件名里高亮，并标注是哪个字段命中的。",
  syntaxRows: SEARCH_SYNTAX_ROWS,
};

/**
 * 模型统计的说明书变体：语法同一套，`tag:` 落在"扫描状态"上
 * （有模型 / 无模型 / 估算 —— 估算指三角形数是按 strip 估出来的）。
 */
export const MODEL_STATS_SEARCH_HELP_VARIANT = {
  fields: ["Mod 标题", "文件名", "路径", "模型 .mdl / .vtx / .vvd 路径", "扫描备注", "扫描状态"],
  fieldNote: "大小写不敏感；命中的片段会在 Mod 名或模型路径里高亮，并标出是哪个扫描状态命中的。",
  syntaxRows: [
    { syntax: "ak47", description: "普通词（可多个）：都要命中，字符按顺序出现即可" },
    { syntax: "models weapons", description: "两个词可以分别命中不同字段（模型路径也算）" },
    { syntax: "-katana", description: "排除「包含」katana 的 Mod（子串匹配，长路径不会误伤）" },
    { syntax: "tag:无模型", description: "只看没检测到模型的条目（有模型 / 无模型 / 估算）" },
    { syntax: "tag:有模型 -tag:估算", description: "有模型、且三角形数不是估算值" },
    { syntax: "re:\\.mdl$", description: "正则匹配；写法有误会直接提示「正则表达式无效」" },
  ],
};

/**
 * 策略组管理的说明书变体：同一套语法，`tag:` 落在**组状态**上
 * （策略 全开/全关/单选/随机单选 · 层级 顶层/子组 · 自动联动）。
 */
export const GROUP_MANAGER_SEARCH_HELP_VARIANT = {
  fields: ["组名", "组描述", "成员名（含文件键）", "组状态"],
  fieldNote: "大小写不敏感；命中的组会连它的上级与下级一起显示，缩进不会指向看不见的组。",
  syntaxRows: [
    { syntax: "医疗箱", description: "普通词（可多个）：都要命中；组名与成员名都算" },
    { syntax: "ak47 medkit", description: "两个词可以分别命中组名与成员名" },
    { syntax: "-备用", description: "排除「包含」备用的组（子串匹配；被排除的组不会因为父组命中又被带出来）" },
    { syntax: "tag:单选", description: "按策略筛：全开 / 全关 / 单选 / 随机单选" },
    { syntax: "tag:顶层 -tag:子组", description: "只看顶层组；层级标签还有「子组」，「自动联动」对应常开联动" },
    { syntax: "re:^AK\\d+", description: "正则匹配；写法有误会直接提示「正则表达式无效」" },
  ],
};

/**
 * 冲突弹窗的说明书变体：同一套语法。
 * 字段是"冲突资源路径 + 参与 Mod 名"；`tag:` 落在严重度与判定状态上。
 */
export const CONFLICT_SEARCH_HELP_VARIANT = {
  fields: ["冲突资源路径", "参与 Mod 的名称 / 标题 / 文件路径", "严重度与判定状态"],
  fieldNote: "大小写不敏感；命中的片段会在 Mod 名与资源路径里高亮。搜索与上面的严重度筛选是「与」的关系。",
  syntaxRows: [
    { syntax: "ak47", description: "普通词（可多个）：都要命中；资源路径与 Mod 名都算" },
    { syntax: "models weapons", description: "两个词可以分别命中不同字段" },
    { syntax: "-katana", description: "排除「包含」katana 的冲突组（子串匹配，长路径不会误伤）" },
    { syntax: "tag:严重", description: "按严重度筛：严重 / 警告 / 提示" },
    { syntax: "tag:同层", description: "只看「参与者共用同一个有效分层」的真冲突；另有「未记录」（有 Mod 没写进 addonlist）" },
    { syntax: "tag:同层 -tag:严重", description: "条件可以组合（且 / 排除）" },
    { syntax: "re:\\.nut$", description: "正则匹配；写法有误会直接提示「正则表达式无效」" },
  ],
};

/**
 * 体检结果的说明书变体：同一套语法。
 * 字段是"问题对象名 / 路径 / 可操作目标 / 提示文案"；`tag:` 落在严重度与问题类型上。
 */
export const HEALTH_SEARCH_HELP_VARIANT = {
  fields: ["问题对象名", "文件路径", "可操作目标", "提示文案", "严重度与问题类型"],
  fieldNote: "大小写不敏感；命中的片段会在问题标题与说明里高亮。搜索只影响显示，不影响体检结果本身。",
  syntaxRows: [
    { syntax: "ak47", description: "普通词（可多个）：对象名、路径与提示文案都算" },
    { syntax: "addonlist 依赖", description: "两个词要同时命中同一条问题" },
    { syntax: "-unrecorded", description: "排除「包含」这个词的问题（子串匹配，长路径不会误伤）" },
    { syntax: "tag:严重", description: "按严重度筛：严重 / 警告 / 提示" },
    { syntax: "tag:条目缺少文件", description: "按问题类型筛（类型名与列表里显示的一致，例如「同名路径是文件夹」）" },
    { syntax: "tag:警告 依赖", description: "条件可以组合（且 / 排除）" },
    { syntax: "re:\\.vpk$", description: "正则匹配；写法有误会直接提示「正则表达式无效」" },
  ],
};

/** buildSearchHelpHtml 生成浮层内容（全部转义）。 */
export function buildSearchHelpHtml(variant = MOD_SEARCH_HELP_VARIANT) {
  const fields = variant?.fields || SEARCH_FIELD_LABELS;
  const syntaxRows = variant?.syntaxRows || SEARCH_SYNTAX_ROWS;
  const fieldNote = variant?.fieldNote || "大小写不敏感；命中的片段会在标题/文件名里高亮，并标注是哪个字段命中的。";
  const renderRows = (rows) =>
    rows
      .map(
        (row) =>
          `<div class="search-help-row"><code>${escapeHtmlText(row.syntax)}</code><span>${escapeHtmlText(
            row.description,
          )}</span></div>`,
      )
      .join("");
  return `
    <div class="search-help-section">
      <div class="search-help-title">匹配范围</div>
      <div class="search-help-fields">${escapeHtmlText(fields.join(" · "))}</div>
      <div class="search-help-note">${escapeHtmlText(fieldNote)}</div>
    </div>
    <div class="search-help-section">
      <div class="search-help-title">语法</div>
      ${renderRows(syntaxRows)}
    </div>
    <div class="search-help-section">
      <div class="search-help-title">快捷键</div>
      ${renderRows(SEARCH_SHORTCUT_ROWS)}
    </div>
  `;
}

/** buildSearchHelpTitle 生成搜索框的悬停提示（一行版）。 */
export function buildSearchHelpTitle(variant = MOD_SEARCH_HELP_VARIANT) {
  const fields = variant?.fields || SEARCH_FIELD_LABELS;
  const syntaxRows = variant?.syntaxRows || SEARCH_SYNTAX_ROWS;
  const syntax = syntaxRows.map((row) => row.syntax).join(" / ");
  return `匹配：${fields.join("、")}\n语法：${syntax}\n快捷键：Ctrl+F 聚焦，↑↓ 选结果，Enter 打开详情，Esc 清空`;
}
