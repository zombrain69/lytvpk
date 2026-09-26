// 冲突弹窗的检索：语法与 Mod 列表 / 归档 / 模型统计 / 策略组**同一套**
// （core/field-search.mjs 同一份实现），只是字段与 `tag:` 的含义按这个面板定：
//   fields → 冲突资源路径（归档内路径）+ 参与 Mod 的名称 / 标题 / 文件路径
//   tags   → 严重度（严重 / 警告 / 提示）+ 同层（有确定的有效分层）+ 未记录（有参与者没写进 addonlist）

import { describeFieldSearch, highlightSpecFor, searchByFields } from "../../core/field-search.mjs";

const SEVERITY_LABELS = { critical: "严重", warning: "警告", info: "提示" };

/** conflictSeverityLabel 把后端的 severity 值翻成界面上的中文。 */
export function conflictSeverityLabel(severity) {
  return SEVERITY_LABELS[String(severity || "").trim()] || "提示";
}

/** conflictGroupTags 返回这一组冲突的 `tag:` 分类值。 */
export function conflictGroupTags(group) {
  const tags = [conflictSeverityLabel(group?.severity)];
  const vpkFiles = Array.isArray(group?.vpk_files) ? group.vpk_files : [];
  if (group?.layer !== null && group?.layer !== undefined) tags.push("同层");
  if (vpkFiles.some((vpk) => Number(vpk?.order) < 0)) tags.push("未记录");
  return tags;
}

/** conflictGroupFields 返回参与匹配的文本字段。 */
export function conflictGroupFields(group) {
  const files = Array.isArray(group?.files) ? group.files : [];
  const vpkFiles = Array.isArray(group?.vpk_files) ? group.vpk_files : [];
  const fields = [...files];
  vpkFiles.forEach((vpk) => {
    fields.push(vpk?.title, vpk?.name, vpk?.path);
  });
  return fields
    .filter((value) => value !== undefined && value !== null && String(value) !== "")
    .map(String);
}

const accessors = { fields: conflictGroupFields, tags: conflictGroupTags };

/** filterConflictGroupsByQuery 按统一语法过滤冲突 / 覆盖组。 */
export function filterConflictGroupsByQuery(groups, query) {
  return searchByFields(groups, query, accessors).items;
}

/** searchConflictGroups 返回完整结果（含计数与正则错误），供界面直接用。 */
export function searchConflictGroups(groups, query) {
  return searchByFields(groups, query, accessors);
}

/** describeConflictSearch 生成计数文案。 */
export function describeConflictSearch({ total = 0, matched = 0, query = "", regexInvalid = "" } = {}) {
  return describeFieldSearch({ total, matched, query, regexInvalid, noun: "组冲突" });
}

/** conflictHighlightSpec 给出高亮参数（交给 highlightMatches）。 */
export function conflictHighlightSpec(result) {
  return highlightSpecFor(result);
}
