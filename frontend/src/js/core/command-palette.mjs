// 命令面板（Ctrl+K）的纯逻辑层：命令表 + 模糊检索 + 排序 + 文案。
//
// 为什么需要：功能一多，"某项功能在哪一页哪一层"就变成记忆负担。
// 命令面板把"页面跳转 + 常用动作"变成一句话检索（`冲突`、`体检`、`工坊`…），
// 与 Mod 列表的搜索语法形成互补：一个找 Mod，一个找功能。

import { t } from "./i18n-runtime.js";

/**
 * COMMANDS 是命令清单（不含具体实现）。id 由 app-runtime.js 映射到实际动作，
 * 这里只负责"叫什么、怎么被搜到、提示什么"。
 */
export const COMMANDS = [
  { id: "focus-mod-search", titleKey: "app.commandPalette.cmd.focusModSearch.title", keywordsKey: "app.commandPalette.cmd.focusModSearch.keywords", hintKey: "app.commandPalette.hint.ctrlF" },
  { id: "page-mods", titleKey: "app.commandPalette.openPage", pageKey: "app.shell.nav.mods", keywordsKey: "app.commandPalette.cmd.pageMods.keywords", hintKey: "app.commandPalette.hint.page" },
  { id: "page-workshop", titleKey: "app.commandPalette.openPage", pageKey: "app.shell.nav.workshop", keywordsKey: "app.commandPalette.cmd.pageWorkshop.keywords", hintKey: "app.commandPalette.hint.page" },
  { id: "page-downloads", titleKey: "app.commandPalette.openPage", pageKey: "app.shell.nav.downloads", keywordsKey: "app.commandPalette.cmd.pageDownloads.keywords", hintKey: "app.commandPalette.hint.page" },
  { id: "page-servers", titleKey: "app.commandPalette.openPage", pageKey: "app.shell.nav.servers", keywordsKey: "app.commandPalette.cmd.pageServers.keywords", hintKey: "app.commandPalette.hint.page" },
  { id: "page-diagnostics", titleKey: "app.commandPalette.openPage", pageKey: "app.shell.nav.diagnostics", keywordsKey: "app.commandPalette.cmd.pageDiagnostics.keywords", hintKey: "app.commandPalette.hint.page" },
  { id: "page-settings", titleKey: "app.commandPalette.openPage", pageKey: "app.shell.nav.settings", keywordsKey: "app.commandPalette.cmd.pageSettings.keywords", hintKey: "app.commandPalette.hint.page" },
  { id: "page-about", titleKey: "app.commandPalette.openPage", pageKey: "app.shell.nav.about", keywordsKey: "app.commandPalette.cmd.pageAbout.keywords", hintKey: "app.commandPalette.hint.page" },
  { id: "interface-settings", titleKey: "app.commandPalette.cmd.interfaceSettings.title", keywordsKey: "app.commandPalette.cmd.interfaceSettings.keywords", hintKey: "app.commandPalette.hint.interfaceSettings" },
  { id: "strategy-group-manager", titleKey: "app.commandPalette.cmd.strategyGroupManager.title", keywordsKey: "app.commandPalette.cmd.strategyGroupManager.keywords", hintKey: "app.commandPalette.hint.window" },
  { id: "group-suggest", titleKey: "app.commandPalette.cmd.groupSuggest.title", keywordsKey: "app.commandPalette.cmd.groupSuggest.keywords", hintKey: "app.commandPalette.hint.window" },
  { id: "load-order", titleKey: "app.commandPalette.cmd.loadOrder.title", keywordsKey: "app.commandPalette.cmd.loadOrder.keywords", hintKey: "app.commandPalette.hint.window" },
  { id: "conflict-analysis", titleKey: "app.commandPalette.cmd.conflictAnalysis.title", keywordsKey: "app.commandPalette.cmd.conflictAnalysis.keywords", hintKey: "app.commandPalette.hint.toolbar" },
  { id: "health-check", titleKey: "app.commandPalette.cmd.healthCheck.title", keywordsKey: "app.commandPalette.cmd.healthCheck.keywords", hintKey: "app.commandPalette.hint.healthCheck" },
  { id: "workshop-enrich", titleKey: "app.commandPalette.cmd.workshopEnrich.title", keywordsKey: "app.commandPalette.cmd.workshopEnrich.keywords", hintKey: "app.commandPalette.hint.workshopSettings" },
  { id: "toggle-theme", titleKey: "app.commandPalette.cmd.toggleTheme.title", keywordsKey: "app.commandPalette.cmd.toggleTheme.keywords", hintKey: "app.commandPalette.hint.titleBar" },
  { id: "show-shortcuts", titleKey: "app.commandPalette.cmd.showShortcuts.title", keywordsKey: "app.commandPalette.cmd.showShortcuts.keywords", hintKey: "app.commandPalette.hint.shortcuts" },
  { id: "reset-window-geometry", titleKey: "app.commandPalette.cmd.resetWindowGeometry.title", keywordsKey: "app.commandPalette.cmd.resetWindowGeometry.keywords", hintKey: "app.commandPalette.hint.windowRescue" },
];

/**
 * resolveCommand 把命令表的 key 解析成当前语言的文案（模块求值期不调 t()，
 * 与 category-tree / 服务器数据表同一套「key 表 + 延迟取文案」做法）。
 */
export function resolveCommand(command) {
  const source = command || {};
  const page = source.pageKey ? t(source.pageKey) : "";
  return {
    id: source.id,
    title: t(source.titleKey, page ? { page } : undefined),
    keywords: t(source.keywordsKey),
    hint: t(source.hintKey),
  };
}

/** subsequenceMatch 与 Mod 搜索同一套语义：字符按顺序出现即命中（大小写不敏感）。 */
export function subsequenceMatch(text, query) {
  const target = String(text ?? "").toLowerCase();
  const needle = String(query ?? "").toLowerCase();
  if (!needle) return true;
  let index = 0;
  for (let cursor = 0; cursor < target.length && index < needle.length; cursor += 1) {
    if (target[cursor] === needle[index]) index += 1;
  }
  return index === needle.length;
}

function matchScore(command, query) {
  const needle = String(query ?? "").trim().toLowerCase();
  if (!needle) return 0;
  const title = String(command.title || "").toLowerCase();
  const keywords = String(command.keywords || "").toLowerCase();

  // 标题前缀 > 标题包含 > 标题按顺序命中 > 关键字命中；同级里"命中越靠前"越优先。
  if (title.startsWith(needle)) return 1000 - title.length;
  const titleIndex = title.indexOf(needle);
  if (titleIndex >= 0) return 800 - titleIndex - title.length * 0.1;
  if (subsequenceMatch(title, needle)) return 600 - title.length * 0.1;
  if (keywords.includes(needle)) return 400 - keywords.indexOf(needle) * 0.1;
  if (subsequenceMatch(keywords, needle)) return 200;
  return -1;
}

/**
 * searchCommands 返回按相关度排序的命令（最多 limit 条）。
 * 空查询时保持命令表原顺序，但如果传了 recentIds，就把"最近用过的"提前
 * （用户按 Ctrl+K 往往是重复上一件事，先把常用的放到手边）。
 */
export function searchCommands(commands, query, limit = 10, recentIds = []) {
  const list = (Array.isArray(commands) ? commands : []).map(resolveCommand);
  const needle = String(query ?? "").trim();
  const max = Number(limit) > 0 ? Number(limit) : list.length;
  if (!needle) return orderCommandsByRecents(list, recentIds).slice(0, max);

  return list
    .map((command) => ({ command, score: matchScore(command, needle) }))
    .filter((entry) => entry.score >= 0)
    .sort((left, right) => right.score - left.score)
    .slice(0, max)
    .map((entry) => entry.command);
}

/** 最近使用记录的上限：太多就失去"最近"的意义，也让面板第一屏变长。 */
export const RECENT_COMMAND_LIMIT = 5;

/**
 * pushRecentCommand 记录一次命令使用：去重、最近在前、超上限截断。
 * 返回新数组，不改原数组（调用方负责持久化）。
 */
export function pushRecentCommand(recentIds, id, limit = RECENT_COMMAND_LIMIT) {
  const target = String(id ?? "").trim();
  const max = Number(limit) > 0 ? Math.floor(Number(limit)) : RECENT_COMMAND_LIMIT;
  if (!target) return (Array.isArray(recentIds) ? recentIds : []).slice(0, max);

  const next = [target];
  for (const raw of Array.isArray(recentIds) ? recentIds : []) {
    const value = String(raw ?? "").trim();
    if (!value || next.includes(value)) continue;
    next.push(value);
    if (next.length >= max) break;
  }
  return next.slice(0, max);
}

/**
 * orderCommandsByRecents 把最近用过的命令提到前面，其余保持命令表原顺序。
 * 用稳定排序实现：没进过"最近列表"的命令之间相对顺序不变。
 */
export function orderCommandsByRecents(commands, recentIds) {
  const list = Array.isArray(commands) ? commands : [];
  const rank = new Map();
  (Array.isArray(recentIds) ? recentIds : []).forEach((id, index) => {
    const key = String(id ?? "").trim();
    if (key && !rank.has(key)) rank.set(key, index);
  });
  if (rank.size === 0) return list.slice();
  return list.slice().sort((left, right) => {
    const leftRank = rank.has(left?.id) ? rank.get(left.id) : Number.POSITIVE_INFINITY;
    const rightRank = rank.has(right?.id) ? rank.get(right.id) : Number.POSITIVE_INFINITY;
    if (leftRank === rightRank) return 0;
    return leftRank - rightRank;
  });
}

/** describeCommandPaletteState 生成面板底部的状态文案。 */
export function describeCommandPaletteState({ total = 0, shown = 0, query = "", recentCount = 0 } = {}) {
  const keyword = String(query ?? "").trim();
  if (!keyword) {
    // 空查询时顺带把全局快捷键列出来：用户按 Ctrl+K 往往就是想找这些。
    const recent = Number(recentCount) > 0 ? t("app.commandPalette.state.recent", { count: Number(recentCount) }) : "";
    return t("app.commandPalette.state.idle", { recent, total: Number(total) || 0 });
  }
  if (!shown) return t("app.commandPalette.state.noMatch", { keyword });
  return t("app.commandPalette.state.matching", { shown: Number(shown) || 0, total: Number(total) || 0 });
}

/** nextCommandId 在命令 id 列表里移动光标（环绕；没有光标时从两端进入）。 */
export function nextCommandId(ids, currentId, delta) {
  const list = (Array.isArray(ids) ? ids : []).filter((id) => String(id || "") !== "");
  if (list.length === 0) return "";
  const rawDelta = Number(delta);
  const step = Number.isFinite(rawDelta) && rawDelta < 0 ? -1 : 1;
  const index = list.indexOf(String(currentId || ""));
  if (index < 0) return step > 0 ? list[0] : list[list.length - 1];
  return list[(index + step + list.length) % list.length];
}
