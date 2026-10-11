// 键盘快捷键的**单一事实来源**。
//
// 以前快捷键只散落在各处绑定里（Ctrl+K / Ctrl+F / Ctrl+±/0 / ↑↓/Enter/Esc），
// 用户只能靠"别人告诉他"或误触发现；搜索语法说明书里那几行也是另一份手写副本。
// 现在：这份表驱动 ①标题栏 `?` 浮层 ②搜索框说明书里的快捷键段 ③命令面板里的「查看键盘快捷键」，
// 三处同源，改一处就够。

import { t } from "./i18n-runtime.js";

export const SHORTCUT_GROUPS = [
  {
    id: "global",
    titleKey: "app.shortcuts.group.global",
    rows: [
      { id: "command-palette", keys: "Ctrl + K", descriptionKey: "app.shortcuts.desc.commandPalette" },
      { id: "focus-search", keys: "Ctrl + F", descriptionKey: "app.shortcuts.desc.focusSearch" },
      { id: "zoom-in", keys: "Ctrl + =", descriptionKey: "app.shortcuts.desc.zoomIn" },
      { id: "zoom-out", keys: "Ctrl + -", descriptionKey: "app.shortcuts.desc.zoomOut" },
      { id: "zoom-reset", keys: "Ctrl + 0", descriptionKey: "app.shortcuts.desc.zoomReset" },
      { id: "shortcuts-help", keys: "?", descriptionKey: "app.shortcuts.desc.shortcutsHelp" },
    ],
  },
  {
    id: "search",
    titleKey: "app.shortcuts.group.search",
    rows: [
      { id: "search-cursor", keys: "↑ / ↓", descriptionKey: "app.shortcuts.desc.searchCursor" },
      { id: "search-open", keys: "Enter", descriptionKey: "app.shortcuts.desc.searchOpen" },
      { id: "search-clear", keys: "Esc", descriptionKey: "app.shortcuts.desc.searchClear" },
      // 对齐 FireAxe v0.7.0/v0.7.2 的编辑快捷键：F2 重命名、Delete 删除选中项。
      { id: "rename-mod", keys: "F2", descriptionKey: "app.shortcuts.desc.renameMod" },
      { id: "delete-mod", keys: "Delete", descriptionKey: "app.shortcuts.desc.deleteMod" },
      // 对齐 FireAxe v0.5.1 的 Ctrl+X / Ctrl+V：标记待移动 → 选目标目录移动。
      { id: "cut-mods", keys: "Ctrl + X", descriptionKey: "app.shortcuts.desc.cutMods" },
      { id: "paste-mods", keys: "Ctrl + V", descriptionKey: "app.shortcuts.desc.pasteMods" },
    ],
  },
  {
    id: "layers",
    titleKey: "app.shortcuts.group.layers",
    rows: [
      { id: "close-layer", keys: "Esc", descriptionKey: "app.shortcuts.desc.closeLayer" },
      { id: "reset-window", keysKey: "app.shortcuts.keys.resetWindow", descriptionKey: "app.shortcuts.desc.resetWindow" },
    ],
  },
];

/**
 * isTextEntryElement 判断一个元素是不是"正在输入文字"的控件。
 *
 * 为什么要单独抽出来：快捷键要不要抢键，取决于这一点。
 * 真机踩过的坑：列表行的复选框是 `<input type="checkbox">`，
 * 若把所有 `INPUT` 都当成输入框，勾选一行之后 Ctrl+X / Ctrl+V 就完全没反应。
 */
export function isTextEntryElement(element) {
  if (!element) return false;
  if (element.isContentEditable) return true;
  const tag = String(element.tagName || "").toUpperCase();
  if (tag === "TEXTAREA") return true;
  if (tag !== "INPUT") return false;
  const type = String(element.type || "text").toLowerCase();
  return ![
    "checkbox",
    "radio",
    "button",
    "submit",
    "reset",
    "file",
    "range",
    "color",
  ].includes(type);
}

function escapeHtmlText(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** allShortcutRows 按组顺序摊平所有快捷键行（供测试与检索用）。 */
export function allShortcutRows() {
  return SHORTCUT_GROUPS.flatMap((group) => group.rows.map((row) => ({ ...row, group: group.id })));
}

/**
 * shortcutRowsByIds 按给定 id 顺序取行（找不到的 id 直接忽略）。
 * 搜索语法说明书就是这么从这份表里挑它需要的几行，避免再抄一份。
 */
export function shortcutRowsByIds(ids) {
  const byId = new Map(allShortcutRows().map((row) => [row.id, row]));
  return (Array.isArray(ids) ? ids : []).map((id) => byId.get(String(id ?? ""))).filter(Boolean);
}

/** resolveShortcutGroupTitle / resolveShortcutDescription / resolveShortcutKeys 取当前语言文案
 *  （key → t()；兼容直接给字面量的行）。模块求值期不调 t()。 */
function resolveShortcutGroupTitle(group) {
  return group?.titleKey ? t(group.titleKey) : String(group?.title ?? "");
}

function resolveShortcutDescription(row) {
  return row?.descriptionKey ? t(row.descriptionKey) : String(row?.description ?? "");
}

function resolveShortcutKeys(row) {
  return row?.keysKey ? t(row.keysKey) : String(row?.keys ?? "");
}

/** buildShortcutsHtml 生成浮层内容（全部转义，复用 .search-help-* 样式）。 */
export function buildShortcutsHtml(groups = SHORTCUT_GROUPS) {
  const list = Array.isArray(groups) ? groups : SHORTCUT_GROUPS;
  return list
    .map((group) => {
      const rows = (group.rows || [])
        .map(
          (row) =>
            `<div class="search-help-row"><code>${escapeHtmlText(resolveShortcutKeys(row))}</code><span>${escapeHtmlText(
              resolveShortcutDescription(row),
            )}</span></div>`,
        )
        .join("");
      return `
    <div class="search-help-section">
      <div class="search-help-title">${escapeHtmlText(resolveShortcutGroupTitle(group))}</div>
      ${rows}
    </div>`;
    })
    .join("");
}

/** buildShortcutsTitle 生成按钮的悬停提示（一行版）。 */
export function buildShortcutsTitle() {
  const keys = shortcutRowsByIds(["command-palette", "focus-search", "zoom-reset", "shortcuts-help"])
    .map((row) => resolveShortcutKeys(row))
    .join(" / ");
  return t("app.shell.shortcuts.tooltipLine", { keys });
}
