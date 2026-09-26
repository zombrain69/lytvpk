// 「同一套搜索语法，应用到任意列表」的通用匹配层。
//
// 语法解析只有一份（features/file-list/search-syntax.mjs），普通词的语义与后端
// SearchVPKFiles 的 fuzzyMatch 一致（字符按顺序出现）。各面板只需要回答两件事：
//   fields(item) → 参与匹配的文本字段
//   tags(item)   → `tag:` 用的分类值
// 归档管理器与模型统计都走这里，避免每个面板再写一遍匹配逻辑（写三份必然漂移）。

import { compiledRegex, negatedTerms, parseSearchSyntax, positiveTerms } from "../features/file-list/search-syntax.mjs";
import { subsequenceMatch } from "../features/file-list/search-match.mjs";

const toTextFields = (values) =>
  (Array.isArray(values) ? values : [])
    .filter((value) => value !== undefined && value !== null && String(value) !== "")
    .map(String);

function matchesAnyField(fields, predicate) {
  return fields.some((field) => predicate(field));
}

/**
 * containsText 子串匹配（大小写不敏感），专供 `-排除` 使用。
 * 排除必须精确：字段里有完整路径，模糊匹配会让 `-old`、`-hd` 这类短词
 * 在长路径里"凑字母"命中，把不相干的记录一起排掉（Go 侧行为一致）。
 */
function containsText(text, needle) {
  const value = String(text ?? "").toLowerCase();
  const target = String(needle ?? "").trim().toLowerCase();
  if (!target) return false;
  return value.includes(target);
}

/** matchByFields 判断一条记录是否满足解析后的搜索语法。 */
export function matchByFields(item, spec, { fields, tags } = {}) {
  if (!spec) return true;
  const values = toTextFields(fields ? fields(item) : []);
  const tagValues = (tags ? tags(item) : []).map((tag) => String(tag).toLowerCase());

  // 正向普通词：每个词都要在某个字段里按顺序命中（与 Mod 列表一致）。
  for (const term of positiveTerms(spec)) {
    if (!matchesAnyField(values, (field) => subsequenceMatch(field, term))) return false;
  }
  // 排除词：任一字段**包含**它就整条排除（子串，不做模糊——见 containsText 的说明）。
  for (const term of negatedTerms(spec)) {
    if (matchesAnyField(values, (field) => containsText(field, term))) return false;
  }
  // 正则：任选一个字段命中即可。
  const regex = compiledRegex(spec);
  if (regex && !matchesAnyField(values, (field) => regex.test(field))) return false;
  // tag: 组之间是"且"，组内 `|` 是"或"。
  for (const alternatives of spec.includeTags || []) {
    const wanted = alternatives.map((value) => String(value).toLowerCase());
    if (!wanted.some((name) => tagValues.includes(name))) return false;
  }
  for (const excluded of spec.excludeTags || []) {
    if (tagValues.includes(String(excluded).toLowerCase())) return false;
  }
  return true;
}

/**
 * searchByFields 过滤列表。
 * 返回 { items, matched, total, regexInvalid, syntax }；正则写错时 items 为空（宁可空列表也要说清原因）。
 */
export function searchByFields(items, query, accessors = {}) {
  const list = Array.isArray(items) ? items : [];
  const syntax = parseSearchSyntax(query);
  if (syntax.regexInvalid) {
    return { items: [], matched: 0, total: list.length, regexInvalid: syntax.regexInvalid, syntax };
  }
  const filtered = list.filter((item) => matchByFields(item, syntax, accessors));
  return { items: filtered, matched: filtered.length, total: list.length, regexInvalid: "", syntax };
}

/** describeFieldSearch 生成计数文案（名词由调用方给，例如"压缩包" / "Mod"）。 */
export function describeFieldSearch({ total = 0, matched = 0, query = "", regexInvalid = "", noun = "条目" } = {}) {
  const keyword = String(query ?? "").trim();
  if (!keyword) return "";
  if (regexInvalid) return `正则表达式无效：${regexInvalid}`;
  // 拉丁字母名词（Mod / VPK）与中文之间留一个空格，中文名词不加。
  const separator = /^[A-Za-z]/.test(noun) ? " " : "";
  const label = `${separator}${noun}`;
  if (Number(matched) === 0) return `没有匹配「${keyword}」的${label}（共 ${Number(total) || 0} 个）`;
  return `匹配 ${Number(matched) || 0} / ${Number(total) || 0} 个${label}`;
}

/** highlightSpecFor 给出高亮参数（没有查询或正则写错时返回 null）。 */
export function highlightSpecFor(result) {
  const syntax = result?.syntax;
  if (!syntax || !syntax.raw || result?.regexInvalid) return null;
  return { terms: positiveTerms(syntax), regex: compiledRegex(syntax) };
}
