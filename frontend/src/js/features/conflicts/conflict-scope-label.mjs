// 「冲突分析对比范围」的纯文案层（node --test 覆盖）。
//
// 背景（真实反馈）：只勾选 1 个条件时，文案仍写成“满足任一：游戏内开启”，
// 且弹窗里的预览行只在打开时刷新 —— 用户点了「满足全部 / 满足任一」看不到任何变化，
// 会以为组合开关坏了。这里把规则集中成：
//   - 单条件：直接写条件名（组合方式没有意义）；
//   - 多条件：写“同时满足 / 满足任一：A + B / A / B”；
// 并暴露 usesCombination 供界面决定是否禁用组合开关。

export const CONFLICT_SCOPE_DEFAULT_RULES = Object.freeze([{ type: "enabled", value: "" }]);

const RULE_LABELS = Object.freeze({
  enabled: "游戏内开启",
  not_disabled: "未禁用",
  root: "根目录",
  workshop: "创意工坊",
});

/** conflictScopeRuleLabel 生成单个条件的显示名；未知类型回落为原样输出。 */
export function conflictScopeRuleLabel(rule) {
  const type = String(rule?.type || "").trim().toLowerCase();
  if (type === "tag") {
    const value = String(rule?.value || "").trim();
    return value ? `标签：${value}` : "标签：未指定";
  }
  return RULE_LABELS[type] || String(rule?.type || "");
}

/** normalizeConflictScopeRules 复刻后端语义：空清单 = 默认“游戏内开启”。 */
export function normalizeConflictScopeRules(rules) {
  const list = Array.isArray(rules)
    ? rules.filter((rule) => rule && String(rule.type || "").trim() !== "")
    : [];
  return list.length ? list : [...CONFLICT_SCOPE_DEFAULT_RULES];
}

/**
 * describeConflictScope 计算对比范围文案。
 *
 * @returns {{ label: string, usesCombination: boolean, ruleLabels: string[], matchMode: "and"|"or" }}
 */
export function describeConflictScope(options = {}) {
  const rules = normalizeConflictScopeRules(options.baselineRules);
  const matchMode = String(options.matchMode || "").toLowerCase() === "and" ? "and" : "or";
  const ruleLabels = rules.map((rule) => conflictScopeRuleLabel(rule));
  if (ruleLabels.length === 1) {
    return { label: ruleLabels[0], usesCombination: false, ruleLabels, matchMode };
  }
  const joined =
    matchMode === "and" ? ruleLabels.join(" + ") : ruleLabels.join(" / ");
  const prefix = matchMode === "and" ? "同时满足" : "满足任一";
  return { label: `${prefix}：${joined}`, usesCombination: true, ruleLabels, matchMode };
}

/**
 * conflictScopeSummaryLabel 生成不含前缀的范围文案（含“按加载顺序判定覆盖”后缀）。
 * 例：`满足任一：游戏内开启 / 创意工坊 · 按加载顺序判定覆盖`
 */
export function conflictScopeSummaryLabel(options = {}) {
  const { label } = describeConflictScope(options);
  return options.priorityAware === true ? `${label} · 按加载顺序判定覆盖` : label;
}

/**
 * formatConflictScopeText 生成界面上一整句。
 *
 * @param {"当前"|"将按"|string} prefix
 */
export function formatConflictScopeText(options = {}, prefix = "当前") {
  return `${prefix}：${conflictScopeSummaryLabel(options)}`;
}

/**
 * serializeConflictScopeOptions 把范围选项压成可比较的字符串，
 * 供弹窗判断“预览里的选项是否已经改动、还没点应用”。
 */
export function serializeConflictScopeOptions(options = {}) {
  const rules = normalizeConflictScopeRules(options.baselineRules)
    .map((rule) => `${String(rule.type || "").trim().toLowerCase()}=${String(rule.value || "").trim()}`)
    .sort();
  const matchMode = String(options.matchMode || "").toLowerCase() === "and" ? "and" : "or";
  return `${matchMode}|${options.priorityAware === true ? "aware" : "plain"}|${rules.join(",")}`;
}
