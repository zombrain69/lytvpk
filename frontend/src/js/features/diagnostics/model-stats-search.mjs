// 模型统计列表的检索：语法与 Mod 列表 / 归档面板完全一致（core/field-search.mjs 同一份实现），
// 只是字段与 `tag:` 的含义按这个面板定：
//   fields → Mod 标题 / 文件名 / 路径 / 备注 / 每个模型的 .mdl、.vtx、.vvd 路径
//   tags   → 有模型 / 无模型 / 估算（triangleStripEstimated：三角形数是按 strip 估算的）

import { describeFieldSearch, highlightSpecFor, searchByFields } from "../../core/field-search.mjs";

export const MODEL_STATS_STATE_TAGS = ["有模型", "无模型", "估算"];

/** modelStatsStateTags 返回这个 Mod 在扫描结果里的状态标签（供 `tag:` 与界面角标使用）。 */
export function modelStatsStateTags(item) {
  const models = Array.isArray(item?.models) ? item.models : [];
  const tags = [models.length > 0 ? "有模型" : "无模型"];
  if (models.some((model) => model?.triangleStripEstimated)) tags.push("估算");
  return tags;
}

/** modelStatsFields 返回参与匹配的文本字段。 */
export function modelStatsFields(item) {
  const fields = [item?.title, item?.name, item?.path, item?.message];
  for (const model of item?.models || []) {
    fields.push(model?.path, model?.vtxPath, model?.vvdPath);
  }
  return fields
    .filter((value) => value !== undefined && value !== null && String(value) !== "")
    .map(String);
}

const accessors = { fields: modelStatsFields, tags: modelStatsStateTags };

/** searchModelStatsItems 过滤扫描结果里的 Mod 列表（三个视图都基于过滤后的 Mod 再排行）。 */
export function searchModelStatsItems(items, query) {
  return searchByFields(items, query, accessors);
}

/** describeModelStatsSearch 生成计数文案。 */
export function describeModelStatsSearch({ total = 0, matched = 0, query = "", regexInvalid = "" } = {}) {
  return describeFieldSearch({ total, matched, query, regexInvalid, noun: "Mod" });
}

/** modelStatsHighlightSpec 给出高亮参数（交给 highlightMatches）。 */
export function modelStatsHighlightSpec(result) {
  return highlightSpecFor(result);
}
