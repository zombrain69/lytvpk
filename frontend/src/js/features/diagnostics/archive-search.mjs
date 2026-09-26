// 归档管理器的检索：**语法与 Mod 列表完全一致**（解析复用 search-syntax.mjs），
// 这样"普通词 / 引号短语 / -排除 / re: 正则 / tag:"在所有列表里含义相同，
// 用户在一处学会就能到处用 —— 这就是"明确的匹配功能的直觉交互"。
//
// 差异只在"字段"：归档里的东西没有一级标签，所以 `tag:` 落在**包状态**上：
//   密码 / 错误 / 已有 / 待导入（可用 `|` 写"或"，例如 `tag:密码|错误`）。
//
// 普通词的匹配语义与后端 SearchVPKFiles 的 fuzzyMatch 保持一致（字符按顺序出现即命中），
// 高亮交给 search-match.mjs 的 highlightMatches，避免出现"搜出来了却没有高亮"。

import { describeFieldSearch, highlightSpecFor, matchByFields, searchByFields } from "../../core/field-search.mjs";

export const ARCHIVE_STATE_TAGS = ["密码", "错误", "已有", "待导入"];

/** archivePackageStateTags 返回这个压缩包的状态标签（供 `tag:` 与界面角标使用）。 */
export function archivePackageStateTags(item) {
  const tags = [];
  if (!item) return tags;
  if (item.requiresPassword) tags.push("密码");
  if (item.error && !item.requiresPassword) tags.push("错误");
  const vpks = Array.isArray(item.vpks) ? item.vpks : [];
  if (vpks.some((vpk) => vpk?.matchState === "new")) tags.push("待导入");
  if (vpks.some((vpk) => vpk?.matchState === "existing")) tags.push("已有");
  return tags;
}

/** archivePackageFields 返回参与匹配的文本字段（压缩包名、路径、里面每个 VPK 的名字与路径）。 */
export function archivePackageFields(item) {
  const fields = [item?.name, item?.path];
  for (const vpk of item?.vpks || []) {
    fields.push(vpk?.name, vpk?.entryPath);
  }
  return fields.filter((value) => value !== undefined && value !== null && String(value) !== "").map(String);
}

// 匹配逻辑在 core/field-search.mjs（与模型统计共用同一份实现）。
// `archivePackageFields` / `archivePackageStateTags` 是函数声明，可以在模块顶层被引用。
const accessors = { fields: archivePackageFields, tags: archivePackageStateTags };

/** matchArchivePackage 判断一个压缩包是否满足解析后的搜索语法。 */
export function matchArchivePackage(item, spec) {
  return matchByFields(item, spec, accessors);
}

/**
 * searchArchivePackages 过滤压缩包列表。
 * 返回 { items, matched, total, regexInvalid, syntax }：
 *   - regexInvalid 非空时 items 为空（与 Mod 列表一致：宁可空列表也要说清正则写错了）；
 *   - matched / total 供界面显示"命中 N / 共 M"。
 */
export function searchArchivePackages(items, query) {
  return searchByFields(items, query, accessors);
}

/**
 * describeArchiveSearchResult 生成搜索框旁边的计数文案。
 * 与 Mod 列表的 describeSearchResult 同风格（包括"正则写错了就说清原因"），
 * 只是把名词换成"压缩包"。
 */
export function describeArchiveSearchResult({ total = 0, matched = 0, query = "", regexInvalid = "" } = {}) {
  return describeFieldSearch({ total, matched, query, regexInvalid, noun: "压缩包" });
}

/**
 * archiveHighlightSpec 给出"点击率最高的那几个词"的高亮参数（交给 highlightMatches）。
 * 没有查询、或正则写错时返回 null —— 与 Mod 列表一致：高亮只跟正向普通词 + 合法正则走。
 */
export function archiveHighlightSpec(result) {
  return highlightSpecFor(result);
}
