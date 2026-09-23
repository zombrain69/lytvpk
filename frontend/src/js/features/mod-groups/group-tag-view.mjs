// 「给这组打标签」的纯文案层（与 DOM 无关，node --test 覆盖）。

/** formatTagApplySummary 汇总一次打标签的结果。 */
export function formatTagApplySummary(result) {
  const applied = (result?.applied || []).length;
  const skipped = (result?.skipped || []).length;
  const missing = (result?.missing || []).length;
  const failed = (result?.failed || []).length;
  const tag = String(result?.tag || "");
  if (applied === 0 && skipped === 0 && missing === 0 && failed === 0) {
    return `没有需要打标签的 Mod（${tag ? `标签「${tag}」` : "标签为空"}）`;
  }
  const parts = [`已给 ${applied} 个 Mod 打上标签「${tag}」`];
  if (skipped > 0) parts.push(`${skipped} 个已经有这个标签`);
  if (missing > 0) parts.push(`${missing} 个文件已不在列表里`);
  if (failed > 0) parts.push(`${failed} 个写入失败`);
  return parts.join("；");
}

/** formatTagOrigin 说明标签是怎么推导出来的。 */
export function formatTagOrigin(origin) {
  switch (String(origin || "")) {
    case "common-tag":
      return "来自这一组已有的共同标签（用来补齐还没打标签的成员）";
    case "subject":
      return "来自成员共同的主体识别结果";
    case "label":
      return "来自组名";
    default:
      return "";
  }
}

/** formatTagSuggestionLine 生成对话框顶部的说明。 */
export function formatTagSuggestionLine(suggested, origin) {
  const tag = String(suggested || "").trim();
  if (!tag) {
    return "没有自动推导出标签（组名太长或成员没有共同主体），可以自己填一个。";
  }
  const originText = formatTagOrigin(origin);
  return `建议标签：${tag}` + (originText ? `（${originText}）` : "") + "；可以直接改成别的名字。";
}

/**
 * buildTagApplyPlan 把一条建议（或一个组）的标签提案整理成"标签 → 成员键"的应用计划：
 *
 *   - `tag`（整组统一标签）套用到全部成员；
 *   - `memberTags`（逐成员标签，来自外部建议文件）按标签归并；
 *   - 标签名大小写不同视为同一个；同一个键不会重复出现。
 *
 * 返回顺序：先整组标签，再逐成员标签（按首次出现顺序）。
 */
export function buildTagApplyPlan(suggestion) {
  if (!suggestion) return [];
  const memberKeys = Array.isArray(suggestion.memberKeys) ? suggestion.memberKeys : [];
  const memberNames = Array.isArray(suggestion.memberNames) ? suggestion.memberNames : [];
  const nameByKey = new Map();
  memberKeys.forEach((key, index) => {
    if (key) nameByKey.set(String(key), String(memberNames[index] || key));
  });

  const plan = [];
  const byFoldedTag = new Map();
  const addTag = (rawTag, key) => {
    const tag = String(rawTag || "").trim();
    if (!tag) return;
    const folded = tag.toLowerCase();
    if (!byFoldedTag.has(folded)) {
      const entry = { tag, keys: [], names: [] };
      byFoldedTag.set(folded, entry);
      plan.push(entry);
    }
    const entry = byFoldedTag.get(folded);
    if (!key) return;
    const normalizedKey = String(key);
    if (entry.keys.includes(normalizedKey)) return;
    entry.keys.push(normalizedKey);
    if (nameByKey.has(normalizedKey)) entry.names.push(nameByKey.get(normalizedKey));
  };

  // 整组标签优先：它代表"这一组的统一分类"。
  if (String(suggestion.tag || "").trim()) {
    memberKeys.forEach((key) => addTag(suggestion.tag, key));
  }
  const memberTags = suggestion.memberTags || {};
  Object.keys(memberTags).forEach((key) => {
    const tags = Array.isArray(memberTags[key]) ? memberTags[key] : [];
    tags.forEach((tag) => addTag(tag, key));
  });
  return plan;
}
