// 命名规范化规则（路线图第 20 项 P3）的纯逻辑：词表替换输入校验、结果计数与视图归一。
// 不 import i18n 运行时、不碰 DOM —— node --test 直测，设置页与右键菜单消费。
//
// 校验口径与后端 internal/app/naming_rules_config.go 保持一致（Windows 非法文件名字符 +
// 换行 / 制表符；替换名与原词相同时后端会拒绝，这里提前拦下给即时反馈）。

const INVALID_TARGET_PATTERN = /[<>:"/\\|?*\r\n\t]/;
const INVALID_SOURCE_PATTERN = /[\r\n\t]/;

// validateAliasInput 校验词表替换输入，返回错误码（"" = 通过）：
//   "empty"        —— 原词或替换名为空
//   "same"         —— 替换名与原词相同
//   "invalidChars" —— 替换名含 Windows 非法字符，或原词含控制字符
export function validateAliasInput(from, to) {
  const source = String(from ?? "").trim();
  const target = String(to ?? "").trim();
  if (!source || !target) return "empty";
  if (source === target) return "same";
  if (INVALID_TARGET_PATTERN.test(target) || INVALID_SOURCE_PATTERN.test(source)) {
    return "invalidChars";
  }
  return "";
}

// ALIAS_ERROR_KEYS 把校验错误码映射到设置页文案 key。
export const ALIAS_ERROR_KEYS = {
  empty: "settings.addonlist.maintenance.naming.aliasInvalid",
  same: "settings.addonlist.maintenance.naming.aliasSame",
  invalidChars: "settings.addonlist.maintenance.naming.aliasInvalidChars",
};

// mutationCounts 把后端增删结果归一成计数（缺字段容错）。
export function mutationCounts(result) {
  return {
    added: Number(result?.added) || 0,
    removed: Number(result?.removed) || 0,
    skipped: Array.isArray(result?.skipped) ? result.skipped.length : 0,
  };
}

// aliasEntries 归一化词表替换视图（缺失字段容错）。
export function aliasEntries(view) {
  const list = Array.isArray(view?.subjectAliases) ? view.subjectAliases : [];
  return list
    .map((alias) => ({ from: String(alias?.from || ""), to: String(alias?.to || "") }))
    .filter((alias) => alias.from !== "" && alias.to !== "");
}

// exemptEntries 归一化例外表视图：缺失字段容错 + 按存储键排序稳定展示。
export function exemptEntries(view) {
  const list = Array.isArray(view?.exemptions) ? view.exemptions : [];
  return list
    .map((entry) => ({
      path: String(entry?.path || ""),
      name: String(entry?.name || entry?.path || ""),
      exists: Boolean(entry?.exists),
    }))
    .filter((entry) => entry.path !== "")
    .sort((a, b) => a.path.localeCompare(b.path));
}
