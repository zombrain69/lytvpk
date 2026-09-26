// 命令面板（Ctrl+K）的纯逻辑层：命令表 + 模糊检索 + 排序 + 文案。
//
// 为什么需要：功能一多，"某项功能在哪一页哪一层"就变成记忆负担。
// 命令面板把"页面跳转 + 常用动作"变成一句话检索（`冲突`、`体检`、`工坊`…），
// 与 Mod 列表的搜索语法形成互补：一个找 Mod，一个找功能。

/**
 * COMMANDS 是命令清单（不含具体实现）。id 由 app-runtime.js 映射到实际动作，
 * 这里只负责"叫什么、怎么被搜到、提示什么"。
 */
export const COMMANDS = [
  { id: "focus-mod-search", title: "搜索 Mod（跳到搜索框）", keywords: "查找 查询 过滤 搜索 mod search", hint: "Ctrl+F" },
  { id: "page-mods", title: "打开：MOD 管理", keywords: "主页 列表 mods 首页", hint: "页面" },
  { id: "page-workshop", title: "打开：创意工坊", keywords: "workshop 工坊 浏览 下载mod", hint: "页面" },
  { id: "page-downloads", title: "打开：下载与解析", keywords: "downloads 链接 解析 下载队列", hint: "页面" },
  { id: "page-servers", title: "打开：收藏服务器", keywords: "servers 服务器 收藏", hint: "页面" },
  { id: "page-diagnostics", title: "打开：工具箱", keywords: "diagnostics 工具 打包 解包 mdmp 模型 喷漆", hint: "页面" },
  { id: "page-settings", title: "打开：设置", keywords: "settings 配置 偏好", hint: "页面" },
  { id: "page-about", title: "打开：关于", keywords: "about 版本 更新 许可", hint: "页面" },
  { id: "interface-settings", title: "界面设置（文字大小 / 阅读舒适度）", keywords: "界面 字号 文字大小 对比度 行距 舒适 缩放", hint: "设置 → 界面设置" },
  { id: "strategy-group-manager", title: "策略组管理（应用 / 权重 / 层级 / 拖放）", keywords: "策略组 分组 权重 层级 group", hint: "窗口" },
  { id: "group-suggest", title: "分组建议（推导可能同组的 Mod）", keywords: "建议 推导 同组 识别 suggest", hint: "窗口" },
  { id: "load-order", title: "加载顺序优化", keywords: "加载顺序 优先级 覆盖 load order", hint: "窗口" },
  { id: "conflict-analysis", title: "冲突分析（开关）", keywords: "冲突 覆盖 conflict 分析", hint: "工具栏" },
  { id: "health-check", title: "开始体检", keywords: "体检 检查 健康 health 修复", hint: "设置 → 游戏配置" },
  { id: "workshop-enrich", title: "抓取工坊官方标签与统计", keywords: "工坊 标签 统计 steam tags", hint: "设置 → 工坊设置" },
  { id: "toggle-theme", title: "切换主题（浅色 / 深色）", keywords: "主题 夜间 深色 浅色 theme", hint: "标题栏" },
  { id: "show-shortcuts", title: "查看键盘快捷键", keywords: "快捷键 键位 键盘 shortcut help 帮助 问号", hint: "按 ? 也能打开" },
  { id: "reset-window-geometry", title: "重置所有窗口的位置与大小", keywords: "窗口 浮动 位置 大小 复位 重置 跑到屏幕外", hint: "浮动窗口自救" },
];

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
  const list = Array.isArray(commands) ? commands : [];
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
    const recent = Number(recentCount) > 0 ? `最近用过 ${Number(recentCount)} 条在最上面 · ` : "";
    return `${recent}共 ${Number(total) || 0} 条命令 · ↑↓ 选择，Enter 执行，Esc 关闭 · 快捷键：Ctrl+K 命令面板 / Ctrl+F 搜索 / Ctrl+±/0 缩放`;
  }
  if (!shown) return `没有匹配「${keyword}」的命令`;
  return `匹配 ${Number(shown) || 0} / ${Number(total) || 0} 条命令 · Enter 执行`;
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
