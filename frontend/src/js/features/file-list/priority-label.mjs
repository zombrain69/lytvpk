// 统一优先级模型的文案层（与 DOM 无关，便于 node --test 覆盖）。
//
// 约定：addonlist.txt 的顺序号仍是游戏侧唯一权威；"分层"只是用户可以编辑的
// 意图表达。有效分层 effective 由后端算好，这里只负责把结果解释成中文。

const SOURCE_LABELS = {
  tier: "显式分层",
  group: "策略组权重",
  order: "addonlist 顺序号",
};

function toFiniteInteger(value) {
  if (typeof value === "number") {
    return Number.isInteger(value) ? value : null;
  }
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (trimmed === "" || !/^-?\d+$/.test(trimmed)) return null;
    const parsed = Number.parseInt(trimmed, 10);
    return Number.isInteger(parsed) ? parsed : null;
  }
  return null;
}

/** normalizePriorityTier 把输入框内容解析为分层整数；非法输入返回 null。 */
export function normalizePriorityTier(value) {
  return toFiniteInteger(value);
}

export function describePrioritySource(source) {
  return SOURCE_LABELS[source] || "未设置分层";
}

function normalizeKey(key) {
  return String(key ?? "")
    .trim()
    .replace(/\//g, "\\")
    .toLowerCase();
}

/** buildPriorityPlanMap 把 GetModPriorityPlan 的结果转成按归一化键索引的 Map。 */
export function buildPriorityPlanMap(plan) {
  const map = new Map();
  if (!Array.isArray(plan)) return map;
  plan.forEach((entry) => {
    if (!entry) return;
    const key = normalizeKey(entry.key);
    if (!key) return;
    map.set(key, entry);
  });
  return map;
}

/**
 * formatPriorityLabel 生成列表/冲突卡片上的紧凑标签：
 * "优先级 #3（分层 2）"；未设置分层时退化为 "优先级 #3"。
 */
export function formatPriorityLabel(entry) {
  if (!entry) return "";
  const order = toFiniteInteger(entry.order);
  const tier = toFiniteInteger(entry.tier);
  const known = entry.known !== false && order !== null && order > 0;
  if (!known) {
    return tier === null ? "未写入 addonlist" : `未写入 addonlist（分层 ${tier}）`;
  }
  return tier === null ? `优先级 #${order}` : `优先级 #${order}（分层 ${tier}）`;
}

/**
 * formatEffectiveLayer 只在有效分层与顺序号不同（即分层/组权重真的起作用）时返回解释文案，
 * 避免在未分层时给界面增加噪音。
 */
export function formatEffectiveLayer(entry) {
  if (!entry) return "";
  const order = toFiniteInteger(entry.order);
  const effective = toFiniteInteger(entry.effective);
  const source = String(entry.source || "");
  if (order === null || effective === null || order <= 0) return "";
  if (source === "order" || effective === order) return "";
  if (source === "group") {
    return `有效分层 ${effective}（来自策略组权重，顺序号 #${order}）`;
  }
  return `有效分层 ${effective}（来自显式分层）`;
}
