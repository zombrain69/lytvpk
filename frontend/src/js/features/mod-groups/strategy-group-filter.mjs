// 「策略组管理」窗口的组搜索：纯函数，node --test 覆盖。
//
// 为什么需要：窗口里组一多（实测 19 组、几十个成员名）就只能靠滚；
// 而窗口顶部那个输入框其实是"新建组名称"，不是搜索框 —— 用户会误以为它能搜。
// 这里提供真正的按组名 / 成员名 / 策略名搜索，并保留树形缩进可读性。
//
// 语法与 Mod 列表 / 归档 / 模型统计**同一套**（core/field-search.mjs 同一份实现）：
// 普通词（多个词要同时命中）/ `"引号短语"` / `-排除` / `re:` 正则 / `tag:` 组状态。

import { describeFieldSearch, matchByFields } from "../../core/field-search.mjs";
import { parseSearchSyntax } from "../file-list/search-syntax.mjs";

const STRATEGY_TAGS = {
  all: "全开",
  off: "全关",
  single: "单选",
  single_random: "随机单选",
};

/** groupTags 返回这个组在 `tag:` 里的分类值：策略 / 层级 / 是否常开联动。 */
export function groupTags(group) {
  const tags = [];
  const strategy = String(group?.strategy || "").trim();
  if (STRATEGY_TAGS[strategy]) tags.push(STRATEGY_TAGS[strategy]);
  tags.push(group?.parentId ? "子组" : "顶层");
  if (group?.enforce) tags.push("自动联动");
  return tags;
}

/** groupFields 返回参与匹配的文本：组名 / 描述 / 成员名（含文件键）。 */
export function groupFields(group) {
  const members = Array.isArray(group?.members) ? group.members : [];
  return [
    String(group?.name || ""),
    String(group?.description || ""),
    ...members.map((member) => String(member?.name || member?.key || "")),
  ].filter((value) => value.trim() !== "");
}

const groupAccessors = { fields: groupFields, tags: groupTags };

/**
 * filterStrategyGroupRows 按关键字过滤 [{group, depth}] 行。
 *
 * 命中规则：与其它列表同一套搜索语法（组名 / 描述 / 成员名都可命中）。
 * 树形可读性：子组命中时把它的所有上级一起带上；父组命中时它的子组也一起保留
 * （否则缩进的 `└` 会指向一个看不见的父组）。
 */
export function filterStrategyGroupRows(rows, query) {
  const list = Array.isArray(rows) ? rows : [];
  const raw = String(query || "").trim();
  if (!raw) return list.slice();
  const spec = parseSearchSyntax(raw);
  if (spec.regexInvalid) return [];

  const byId = new Map(list.map((row) => [String(row?.group?.id || ""), row]));
  const matched = new Set();
  list.forEach((row) => {
    if (matchByFields(row?.group, spec, groupAccessors)) {
      matched.add(String(row?.group?.id || ""));
    }
  });
  if (matched.size === 0) return [];

  const kept = new Set(matched);
  // 子组命中 → 带上所有上级
  matched.forEach((id) => {
    let current = byId.get(id);
    const guard = new Set([id]);
    while (current?.group?.parentId) {
      const parentId = String(current.group.parentId);
      if (guard.has(parentId) || !byId.has(parentId)) break;
      guard.add(parentId);
      kept.add(parentId);
      current = byId.get(parentId);
    }
  });
  // 父组命中 → 带上所有下级
  list.forEach((row) => {
    const parentId = String(row?.group?.parentId || "");
    const id = String(row?.group?.id || "");
    // 但被 `-排除` 明确否掉的组不再补回来，否则 `tag:顶层 -tag:子组` 会被树形补全悄悄破坏。
    if (!parentId || !matched.has(parentId) || rejectedByNegatives(row?.group, spec)) return;
    kept.add(id);
  });
  return list.filter((row) => kept.has(String(row?.group?.id || "")));
}

/**
 * rejectedByNegatives 判断一个组是不是**命中了任一排除条件**。
 * 树形补全（父组命中就带上子组）会绕开"这行自己判失败"的事实，
 * 所以这里单独把否定条件拎出来查一遍：排除的语义必须硬。
 */
function rejectedByNegatives(group, spec) {
  const negativeTerms = (spec.terms || [])
    .filter((term) => String(term).startsWith("-") && String(term).length > 1)
    .map((term) => String(term).slice(1));
  for (const term of negativeTerms) {
    const onlyThisTerm = { terms: [term], includeTags: [], excludeTags: [], regex: "", regexInvalid: "" };
    if (matchByFields(group, onlyThisTerm, groupAccessors)) return true;
  }
  const excludedTags = (spec.excludeTags || []).map((tag) => String(tag).toLowerCase());
  if (excludedTags.length > 0) {
    const ownTags = groupTags(group).map((tag) => tag.toLowerCase());
    if (excludedTags.some((tag) => ownTags.includes(tag))) return true;
  }
  return false;
}

/** describeStrategyGroupSearch 生成搜索框旁边的计数文案。 */
export function describeStrategyGroupSearch({ total = 0, shown = 0, query = "" } = {}) {
  const all = Number(total || 0);
  const visible = Number(shown || 0);
  if (!String(query || "").trim()) return `共 ${all} 个组`;
  // 正则写错时直说原因（与其它列表一致），不再含糊地报"没有匹配的组"。
  const invalid = parseSearchSyntax(query).regexInvalid;
  if (invalid) {
    return describeFieldSearch({ total: all, matched: 0, query, regexInvalid: invalid, noun: "个组" });
  }
  if (visible === 0) return `没有匹配的组（共 ${all} 个）`;
  return `显示 ${visible} / ${all} 个组`;
}

/** formatBatchHint 生成批量工具条的提示：没勾选时告诉用户先勾，勾了之后给数量。 */
export function formatBatchHint(selectionCount) {
  const count = Number(selectionCount || 0);
  if (count <= 0) {
    return "先勾选组（每行最左边的方框，或点组名）才能批量删除 / 开关自动联动 / 设置权重 / 用这些组筛选";
  }
  return `已勾选 ${count} 个组，下面的批量按钮已可用`;
}
