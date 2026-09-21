// 策略组在设置页里的展示文案。

const STRATEGY_LABELS = {
  single: "互斥单选",
  single_random: "随机单选",
  all: "全部开启",
  off: "全部关闭",
};

export function formatStrategyGroupStrategy(value) {
  return STRATEGY_LABELS[String(value || "").trim()] || "未设置";
}

export function formatStrategyGroupApplySummary(result) {
  const name = String(result?.groupName || "").trim();
  const strategy = formatStrategyGroupStrategy(result?.strategy);
  const enabled = Array.isArray(result?.enabled) ? result.enabled.length : 0;
  const disabled = Array.isArray(result?.disabled) ? result.disabled.length : 0;
  const picked = String(result?.pickedName || "").trim();

  const head = name ? `已应用策略组“${name}”` : "已应用策略组";
  const pickedText = picked ? `，保留 ${picked}` : "";
  return `${head}（${strategy}）：开启 ${enabled} 项，关闭 ${disabled} 项${pickedText}`;
}

// formatStrategyGroupEnforcementNotice 描述一次手动开关触发的自动联动。
// 返回空字符串表示这次操作没有联动其它成员。
export function formatStrategyGroupEnforcementNotice(changedCount) {
  const count = Number(changedCount) || 0;
  if (count <= 0) {
    return "";
  }
  return `已按策略组自动联动另外 ${count} 个 Mod`;
}
