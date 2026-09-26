// 体检结果的检索：语法与 Mod 列表 / 归档 / 模型统计 / 策略组 / 冲突**同一套**
// （core/field-search.mjs 同一份实现），只是字段与 `tag:` 的含义按这个面板定：
//   fields → 问题对象名 / 文件路径 / 提示文案 / 位置
//   tags   → 严重度（严重 / 警告 / 提示）+ 问题类型（复用已有中文名，例如「条目缺少文件」）

import { describeFieldSearch, highlightSpecFor, searchByFields } from "../../core/field-search.mjs";
import { formatHealthIssueKind, formatHealthSeverity } from "./health-report-format.mjs";

/** healthIssueFields 返回参与匹配的文本字段。 */
export function healthIssueFields(issue) {
  // target 是"这条问题可以直接操作谁"（addonlist 键或路径），搜它等于按目标定位。
  return [issue?.name, issue?.path, issue?.target, issue?.message, issue?.location]
    .filter((value) => value !== undefined && value !== null && String(value) !== "")
    .map(String);
}

/** healthIssueTags 返回 `tag:` 用的分类值：严重度 + 问题类型中文名。 */
export function healthIssueTags(issue) {
  return [formatHealthSeverity(issue?.severity), formatHealthIssueKind(issue?.kind)];
}

const accessors = { fields: healthIssueFields, tags: healthIssueTags };

/** searchHealthIssues 过滤体检问题列表。 */
export function searchHealthIssues(issues, query) {
  return searchByFields(issues, query, accessors);
}

/** describeHealthSearch 生成计数文案。 */
export function describeHealthSearch({ total = 0, matched = 0, query = "", regexInvalid = "" } = {}) {
  return describeFieldSearch({ total, matched, query, regexInvalid, noun: "问题" });
}

/** healthHighlightSpec 给出高亮参数（交给 highlightMatches）。 */
export function healthHighlightSpec(result) {
  return highlightSpecFor(result);
}
