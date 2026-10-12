// 搜索语法的"说明书"：搜索框旁 `?` 按钮的浮层内容与悬停提示都从这里生成，
// 保证界面提示、文档、测试三处不会各写一份互相对不上的说明。
//
// 文案全部走目录（`search.*` 子树）：静态表只存 key，渲染时经 t() 取当前语言
// 的文案（模块求值期不调 t()）。

import { shortcutRowsByIds } from "../../core/shortcuts.mjs";
import { t } from "../../core/i18n-runtime.js";
import { listSeparator } from "../../core/format-list.mjs";

function escapeHtmlText(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** 语法行：写法 → 含义（只存 key；渲染时经 t() 取当前语言文案）。 */
export const SEARCH_SYNTAX_ROWS = [
  { syntaxKey: "search.example.mods.plain.syntax", descriptionKey: "search.example.mods.plain.desc" },
  { syntaxKey: "search.example.mods.quoted.syntax", descriptionKey: "search.example.mods.quoted.desc" },
  { syntaxKey: "search.example.mods.tagSingle.syntax", descriptionKey: "search.example.mods.tagSingle.desc" },
  { syntaxKey: "search.example.mods.tagOr.syntax", descriptionKey: "search.example.mods.tagOr.desc" },
  { syntaxKey: "search.example.mods.tagAnd.syntax", descriptionKey: "search.example.mods.tagAnd.desc" },
  { syntaxKey: "search.example.mods.tagExclude.syntax", descriptionKey: "search.example.mods.tagExclude.desc" },
  { syntaxKey: "search.example.mods.slot.syntax", descriptionKey: "search.example.mods.slot.desc" },
  { syntaxKey: "search.example.mods.slotExclude.syntax", descriptionKey: "search.example.mods.slotExclude.desc" },
  { syntaxKey: "search.example.mods.exclude.syntax", descriptionKey: "search.example.mods.exclude.desc" },
  { syntaxKey: "search.example.mods.regex.syntax", descriptionKey: "search.example.mods.regex.desc" },
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
]).map((row) => ({ syntaxKey: row.keysKey, syntax: row.keys, descriptionKey: row.descriptionKey }));

/** 参与搜索的字段（顺序 = 展示顺序）；key 指向目录，渲染时取文案。 */
export const SEARCH_FIELD_LABEL_KEYS = [
  "search.fields.mods.title",
  "search.fields.mods.fileName",
  "search.fields.mods.primaryTag",
  "search.fields.mods.secondaryTag",
  "search.fields.mods.subject",
  "search.fields.mods.voice",
  "search.fields.mods.slot",
];

/**
 * 归档管理器的说明书变体。
 * 语法是同一套（同一个解析器），差别只在"能匹配哪些字段"与 `tag:` 的含义：
 * 归档里没有一级标签，`tag:` 落在**包状态**上。
 */
export const ARCHIVE_SEARCH_HELP_VARIANT = {
  fieldKeys: [
    "search.fields.archive.packageName",
    "search.fields.archive.path",
    "search.fields.archive.innerVpkName",
    "search.fields.archive.innerVpkPath",
    "search.fields.archive.packageState",
  ],
  fieldNoteKey: "search.note.archive",
  syntaxRows: [
    { syntaxKey: "search.example.archive.plain.syntax", descriptionKey: "search.example.archive.plain.desc" },
    { syntaxKey: "search.example.archive.quoted.syntax", descriptionKey: "search.example.archive.quoted.desc" },
    { syntaxKey: "search.example.archive.exclude.syntax", descriptionKey: "search.example.archive.exclude.desc" },
    { syntaxKey: "search.example.archive.tagState.syntax", descriptionKey: "search.example.archive.tagState.desc" },
    { syntaxKey: "search.example.archive.tagStateOr.syntax", descriptionKey: "search.example.archive.tagStateOr.desc" },
    { syntaxKey: "search.example.archive.tagStateAnd.syntax", descriptionKey: "search.example.archive.tagStateAnd.desc" },
    { syntaxKey: "search.example.archive.regex.syntax", descriptionKey: "search.example.archive.regex.desc" },
  ],
};

/** 默认（Mod 列表）说明书变体。 */
export const MOD_SEARCH_HELP_VARIANT = {
  fieldKeys: SEARCH_FIELD_LABEL_KEYS,
  fieldNoteKey: "search.note.mods",
  syntaxRows: SEARCH_SYNTAX_ROWS,
};

/**
 * 模型统计的说明书变体：语法同一套，`tag:` 落在"扫描状态"上
 * （有模型 / 无模型 / 估算 —— 估算指三角形数是按 strip 估出来的）。
 */
export const MODEL_STATS_SEARCH_HELP_VARIANT = {
  fieldKeys: [
    "search.fields.modelStats.title",
    "search.fields.modelStats.fileName",
    "search.fields.modelStats.path",
    "search.fields.modelStats.modelPath",
    "search.fields.modelStats.scanNote",
    "search.fields.modelStats.scanState",
  ],
  fieldNoteKey: "search.note.modelStats",
  syntaxRows: [
    { syntaxKey: "search.example.modelStats.plain.syntax", descriptionKey: "search.example.modelStats.plain.desc" },
    { syntaxKey: "search.example.modelStats.multiField.syntax", descriptionKey: "search.example.modelStats.multiField.desc" },
    { syntaxKey: "search.example.modelStats.exclude.syntax", descriptionKey: "search.example.modelStats.exclude.desc" },
    { syntaxKey: "search.example.modelStats.tagState.syntax", descriptionKey: "search.example.modelStats.tagState.desc" },
    { syntaxKey: "search.example.modelStats.tagStateAnd.syntax", descriptionKey: "search.example.modelStats.tagStateAnd.desc" },
    { syntaxKey: "search.example.modelStats.regex.syntax", descriptionKey: "search.example.modelStats.regex.desc" },
  ],
};

/**
 * 策略组管理的说明书变体：同一套语法，`tag:` 落在**组状态**上
 * （策略 全开/全关/单选/随机单选 · 层级 顶层/子组 · 自动联动）。
 */
export const GROUP_MANAGER_SEARCH_HELP_VARIANT = {
  fieldKeys: [
    "search.fields.groupManager.name",
    "search.fields.groupManager.description",
    "search.fields.groupManager.member",
    "search.fields.groupManager.state",
  ],
  fieldNoteKey: "search.note.groupManager",
  syntaxRows: [
    { syntaxKey: "search.example.groupManager.plain.syntax", descriptionKey: "search.example.groupManager.plain.desc" },
    { syntaxKey: "search.example.groupManager.multiField.syntax", descriptionKey: "search.example.groupManager.multiField.desc" },
    { syntaxKey: "search.example.groupManager.exclude.syntax", descriptionKey: "search.example.groupManager.exclude.desc" },
    { syntaxKey: "search.example.groupManager.tagPolicy.syntax", descriptionKey: "search.example.groupManager.tagPolicy.desc" },
    { syntaxKey: "search.example.groupManager.tagTopLevel.syntax", descriptionKey: "search.example.groupManager.tagTopLevel.desc" },
    { syntaxKey: "search.example.groupManager.regex.syntax", descriptionKey: "search.example.groupManager.regex.desc" },
  ],
};

/**
 * 冲突弹窗的说明书变体：同一套语法。
 * 字段是"冲突资源路径 + 参与 Mod 名"；`tag:` 落在严重度与判定状态上。
 */
export const CONFLICT_SEARCH_HELP_VARIANT = {
  fieldKeys: [
    "search.fields.conflicts.path",
    "search.fields.conflicts.participants",
    "search.fields.conflicts.severityState",
  ],
  fieldNoteKey: "search.note.conflicts",
  syntaxRows: [
    { syntaxKey: "search.example.conflicts.plain.syntax", descriptionKey: "search.example.conflicts.plain.desc" },
    { syntaxKey: "search.example.conflicts.multiField.syntax", descriptionKey: "search.example.conflicts.multiField.desc" },
    { syntaxKey: "search.example.conflicts.exclude.syntax", descriptionKey: "search.example.conflicts.exclude.desc" },
    { syntaxKey: "search.example.conflicts.tagSeverity.syntax", descriptionKey: "search.example.conflicts.tagSeverity.desc" },
    { syntaxKey: "search.example.conflicts.tagSameLayer.syntax", descriptionKey: "search.example.conflicts.tagSameLayer.desc" },
    { syntaxKey: "search.example.conflicts.tagAndExclude.syntax", descriptionKey: "search.example.conflicts.tagAndExclude.desc" },
    { syntaxKey: "search.example.conflicts.regex.syntax", descriptionKey: "search.example.conflicts.regex.desc" },
  ],
};

/**
 * 体检结果的说明书变体：同一套语法。
 * 字段是"问题对象名 / 路径 / 可操作目标 / 提示文案"；`tag:` 落在严重度与问题类型上。
 */
export const HEALTH_SEARCH_HELP_VARIANT = {
  fieldKeys: [
    "search.fields.health.subject",
    "search.fields.health.path",
    "search.fields.health.target",
    "search.fields.health.hint",
    "search.fields.health.severityType",
  ],
  fieldNoteKey: "search.note.health",
  syntaxRows: [
    { syntaxKey: "search.example.health.plain.syntax", descriptionKey: "search.example.health.plain.desc" },
    { syntaxKey: "search.example.health.twoWords.syntax", descriptionKey: "search.example.health.twoWords.desc" },
    { syntaxKey: "search.example.health.exclude.syntax", descriptionKey: "search.example.health.exclude.desc" },
    { syntaxKey: "search.example.health.tagSeverity.syntax", descriptionKey: "search.example.health.tagSeverity.desc" },
    { syntaxKey: "search.example.health.tagType.syntax", descriptionKey: "search.example.health.tagType.desc" },
    { syntaxKey: "search.example.health.tagAndWord.syntax", descriptionKey: "search.example.health.tagAndWord.desc" },
    { syntaxKey: "search.example.health.regex.syntax", descriptionKey: "search.example.health.regex.desc" },
  ],
};

/** resolveSyntaxRow 取一条语法行的当前文案（key → t()；兼容直接给字面量的行）。 */
function resolveSyntaxRow(row) {
  return {
    syntax: row.syntaxKey ? t(row.syntaxKey) : String(row.syntax ?? ""),
    description: row.descriptionKey ? t(row.descriptionKey) : String(row.description ?? ""),
  };
}

/** resolveVariantFields 取变体的"匹配范围"字段文案。 */
function resolveVariantFields(variant) {
  return (variant?.fieldKeys || SEARCH_FIELD_LABEL_KEYS).map((key) => t(key));
}

/** buildSearchHelpHtml 生成浮层内容（全部转义）。 */
export function buildSearchHelpHtml(variant = MOD_SEARCH_HELP_VARIANT) {
  const fields = resolveVariantFields(variant);
  const syntaxRows = (variant?.syntaxRows || SEARCH_SYNTAX_ROWS).map(resolveSyntaxRow);
  const fieldNote = t(variant?.fieldNoteKey || "search.note.mods");
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
      <div class="search-help-title">${escapeHtmlText(t("search.section.matches"))}</div>
      <div class="search-help-fields">${escapeHtmlText(fields.join(" · "))}</div>
      <div class="search-help-note">${escapeHtmlText(fieldNote)}</div>
    </div>
    <div class="search-help-section">
      <div class="search-help-title">${escapeHtmlText(t("search.section.syntax"))}</div>
      ${renderRows(syntaxRows)}
    </div>
    <div class="search-help-section">
      <div class="search-help-title">${escapeHtmlText(t("search.section.shortcuts"))}</div>
      ${renderRows(SEARCH_SHORTCUT_ROWS.map(resolveSyntaxRow))}
    </div>
  `;
}

/** buildSearchHelpTitle 生成搜索框的悬停提示（一行版）。 */
export function buildSearchHelpTitle(variant = MOD_SEARCH_HELP_VARIANT) {
  const fields = resolveVariantFields(variant);
  const syntaxRows = (variant?.syntaxRows || SEARCH_SYNTAX_ROWS).map(resolveSyntaxRow);
  const syntax = syntaxRows.map((row) => row.syntax).join(" / ");
  return t("search.titleTooltip", { fields: fields.join(listSeparator()), syntax });
}
