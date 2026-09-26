// 键盘快捷键的**单一事实来源**。
//
// 以前快捷键只散落在各处绑定里（Ctrl+K / Ctrl+F / Ctrl+±/0 / ↑↓/Enter/Esc），
// 用户只能靠"别人告诉他"或误触发现；搜索语法说明书里那几行也是另一份手写副本。
// 现在：这份表驱动 ①标题栏 `?` 浮层 ②搜索框说明书里的快捷键段 ③命令面板里的「查看键盘快捷键」，
// 三处同源，改一处就够。

export const SHORTCUT_GROUPS = [
  {
    id: "global",
    title: "全局",
    rows: [
      { id: "command-palette", keys: "Ctrl + K", description: "打开命令面板（找页面 / 跑常用动作）" },
      { id: "focus-search", keys: "Ctrl + F", description: "聚焦 Mod 搜索框" },
      { id: "zoom-in", keys: "Ctrl + =", description: "界面放大（Ctrl + 加号同理）" },
      { id: "zoom-out", keys: "Ctrl + -", description: "界面缩小" },
      { id: "zoom-reset", keys: "Ctrl + 0", description: "界面缩放回到 100%" },
      { id: "shortcuts-help", keys: "?", description: "打开这份快捷键总览（光标在输入框里时不触发）" },
    ],
  },
  {
    id: "search",
    title: "搜索框与列表",
    rows: [
      { id: "search-cursor", keys: "↑ / ↓", description: "在结果里移动光标（焦点仍在搜索框）" },
      { id: "search-open", keys: "Enter", description: "打开光标所在 Mod 的详情" },
      { id: "search-clear", keys: "Esc", description: "清空搜索并回到全部" },
      // 对齐 FireAxe v0.7.0/v0.7.2 的编辑快捷键：F2 重命名、Delete 删除选中项。
      { id: "rename-mod", keys: "F2", description: "重命名光标所在（或唯一选中的）Mod" },
      { id: "delete-mod", keys: "Delete", description: "删除光标所在（或已选中的）Mod（先确认，进回收站）" },
      // 对齐 FireAxe v0.5.1 的 Ctrl+X / Ctrl+V：标记待移动 → 选目标目录移动。
      { id: "cut-mods", keys: "Ctrl + X", description: "标记选中的 Mod 为待移动（可继续改选，标记不变）" },
      { id: "paste-mods", keys: "Ctrl + V", description: "把待移动的 Mod 移到选定目录（Esc 取消标记）" },
    ],
  },
  {
    id: "layers",
    title: "窗口与浮层",
    rows: [
      { id: "close-layer", keys: "Esc", description: "关闭浮层 / 浮动窗口（点窗口外同样可以）" },
      { id: "reset-window", keys: "双击标题栏", description: "把该窗口的位置与大小复位" },
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

/** buildShortcutsHtml 生成浮层内容（全部转义，复用 .search-help-* 样式）。 */
export function buildShortcutsHtml(groups = SHORTCUT_GROUPS) {
  const list = Array.isArray(groups) ? groups : SHORTCUT_GROUPS;
  return list
    .map((group) => {
      const rows = (group.rows || [])
        .map(
          (row) =>
            `<div class="search-help-row"><code>${escapeHtmlText(row.keys)}</code><span>${escapeHtmlText(
              row.description,
            )}</span></div>`,
        )
        .join("");
      return `
    <div class="search-help-section">
      <div class="search-help-title">${escapeHtmlText(group.title)}</div>
      ${rows}
    </div>`;
    })
    .join("");
}

/** buildShortcutsTitle 生成按钮的悬停提示（一行版）。 */
export function buildShortcutsTitle() {
  const keys = shortcutRowsByIds(["command-palette", "focus-search", "zoom-reset", "shortcuts-help"])
    .map((row) => row.keys)
    .join(" / ");
  return `快捷键总览：${keys}（点开看全部）`;
}
