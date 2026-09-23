// 分组建议弹窗的"阅览层"纯函数：筛选、排序、折叠卡片预览与分页。
//
// 与 DOM 无关，node --test 覆盖；弹窗（group-ui.js）只负责把结果渲染出来。
// 设计目标：234 条建议时也能"先扫一遍、再决定看哪条"，而不是一屏只能看两条。

/** 折叠后每页渲染的建议条数（点「显示更多」继续加载）。 */
export const SUGGESTION_PAGE_SIZE = 24;

/** 折叠卡片里预览多少个成员名。 */
export const SUGGESTION_PREVIEW_MEMBERS = 3;

export const DEFAULT_SUGGESTION_FILTER = {
  query: "",
  confidence: "",
  source: "",
  // 标签能精确筛出这一批 Mod 的建议默认隐藏：直接用标签筛选即可，不必再建组
  // （这类建议仍然保留，可切换成"显示全部"或"只看标签已覆盖"）。
  tagCovered: "hide",
  sort: "recommended",
};

export const SUGGESTION_CONFIDENCE_FILTERS = [
  { value: "", label: "全部置信度" },
  { value: "high", label: "仅高置信度" },
  { value: "medium", label: "仅中置信度" },
  { value: "low", label: "仅低置信度" },
];

export const SUGGESTION_SOURCE_FILTERS = [
  { value: "", label: "全部来源" },
  { value: "external", label: "仅导入建议" },
  { value: "builtin", label: "仅内置推导" },
];

export const SUGGESTION_SORT_OPTIONS = [
  { value: "recommended", label: "推荐顺序" },
  { value: "confidence", label: "置信度优先" },
  { value: "members", label: "成员多优先" },
  { value: "name", label: "名称顺序" },
];

export const SUGGESTION_TAG_COVERAGE_FILTERS = [
  { value: "hide", label: "隐藏标签已覆盖" },
  { value: "show", label: "显示全部" },
  { value: "only", label: "只看标签已覆盖" },
];

/** isSuggestionTagCovered：标签恰好只能筛出这一批 Mod。 */
export function isSuggestionTagCovered(suggestion) {
  return String(suggestion?.tagScope || "") === "exact" && Boolean(String(suggestion?.tagKey || "").trim());
}

export function suggestionMatchesTagCoverage(suggestion, mode = "hide") {
  const wanted = String(mode || "hide");
  if (!wanted || wanted === "show") return true;
  const covered = isSuggestionTagCovered(suggestion);
  return wanted === "only" ? covered : !covered;
}

export function countTagCoveredSuggestions(suggestions) {
  return (Array.isArray(suggestions) ? suggestions : []).filter(isSuggestionTagCovered).length;
}

// 数值越小越可信；未知置信度排到最后。
const CONFIDENCE_ORDER = { high: 0, medium: 1, low: 2 };

export function confidenceRank(confidence) {
  const key = String(confidence || "").trim().toLowerCase();
  return key in CONFIDENCE_ORDER ? CONFIDENCE_ORDER[key] : 3;
}

export function normalizeSuggestionQuery(query) {
  return String(query ?? "").trim().toLowerCase();
}

function countMembers(suggestion) {
  return Array.isArray(suggestion?.memberKeys) ? suggestion.memberKeys.length : 0;
}

/**
 * suggestionMatchesQuery 在"标题 / 理由 / 信号 / 成员名 / 成员键"里搜索，
 * 这样用户既能按组标题找，也能按记得的某个 Mod 名字反查。
 */
export function suggestionMatchesQuery(suggestion, query) {
  const needle = normalizeSuggestionQuery(query);
  if (!needle) return true;
  const haystacks = [
    suggestion?.label,
    suggestion?.reason,
    ...(Array.isArray(suggestion?.signals) ? suggestion.signals : []),
    ...(Array.isArray(suggestion?.memberNames) ? suggestion.memberNames : []),
    ...(Array.isArray(suggestion?.memberKeys) ? suggestion.memberKeys : []),
  ];
  return haystacks.some((value) => String(value ?? "").toLowerCase().includes(needle));
}

function suggestionMatchesSource(suggestion, source) {
  const wanted = String(source || "").trim();
  if (!wanted) return true;
  const isExternal = String(suggestion?.source || "") === "external";
  return wanted === "external" ? isExternal : !isExternal;
}

/** filterSuggestions 按置信度 / 来源 / 关键词过滤，保持传入顺序。 */
export function filterSuggestions(suggestions, filters = {}) {
  const list = Array.isArray(suggestions) ? suggestions : [];
  const confidence = String(filters.confidence || "").trim().toLowerCase();
  return list.filter((suggestion) => {
    if (confidence && String(suggestion?.confidence || "").toLowerCase() !== confidence) {
      return false;
    }
    if (!suggestionMatchesSource(suggestion, filters.source)) return false;
    if (!suggestionMatchesTagCoverage(suggestion, filters.tagCovered ?? "hide")) return false;
    return suggestionMatchesQuery(suggestion, filters.query);
  });
}

/** sortSuggestions 稳定排序；不认识的模式按"推荐顺序"原样返回。 */
export function sortSuggestions(suggestions, mode = "recommended") {
  const list = Array.isArray(suggestions) ? [...suggestions] : [];
  const normalized = String(mode || "recommended");
  const byScoreDesc = (left, right) => Number(right?.score || 0) - Number(left?.score || 0);

  if (normalized === "confidence") {
    return list
      .map((suggestion, index) => ({ suggestion, index }))
      .sort((left, right) => {
        const rankDiff =
          confidenceRank(left.suggestion?.confidence) - confidenceRank(right.suggestion?.confidence);
        if (rankDiff !== 0) return rankDiff;
        const membersDiff = countMembers(right.suggestion) - countMembers(left.suggestion);
        if (membersDiff !== 0) return membersDiff;
        const scoreDiff = byScoreDesc(left.suggestion, right.suggestion);
        if (scoreDiff !== 0) return scoreDiff;
        return left.index - right.index;
      })
      .map((entry) => entry.suggestion);
  }

  if (normalized === "members") {
    return list
      .map((suggestion, index) => ({ suggestion, index }))
      .sort((left, right) => {
        const membersDiff = countMembers(right.suggestion) - countMembers(left.suggestion);
        if (membersDiff !== 0) return membersDiff;
        const rankDiff =
          confidenceRank(left.suggestion?.confidence) - confidenceRank(right.suggestion?.confidence);
        if (rankDiff !== 0) return rankDiff;
        return left.index - right.index;
      })
      .map((entry) => entry.suggestion);
  }

  if (normalized === "name") {
    return list.sort((left, right) =>
      String(left?.label || "").localeCompare(String(right?.label || ""), "zh-CN"),
    );
  }

  return list;
}

/** applySuggestionView 是"先过滤、再排序"的组合入口，永远返回新数组。 */
export function applySuggestionView(suggestions, filters = {}) {
  const merged = { ...DEFAULT_SUGGESTION_FILTER, ...filters };
  return sortSuggestions(filterSuggestions(suggestions, merged), merged.sort);
}

/** suggestionPreviewMembers 生成折叠卡片的成员预览（成员名优先，回退到键）。 */
export function suggestionPreviewMembers(suggestion, limit = SUGGESTION_PREVIEW_MEMBERS) {
  const keys = Array.isArray(suggestion?.memberKeys) ? suggestion.memberKeys : [];
  const names = Array.isArray(suggestion?.memberNames) ? suggestion.memberNames : [];
  const all = keys.map((key, index) => String(names[index] || key || "").trim()).filter(Boolean);
  const size = Math.max(0, Number(limit) || 0);
  return {
    names: all.slice(0, size),
    more: Math.max(0, all.length - size),
    total: all.length,
  };
}

/** paginateSuggestions 只渲染前 visibleCount 条，避免上千个 DOM 节点卡住弹窗。 */
export function paginateSuggestions(suggestions, visibleCount = SUGGESTION_PAGE_SIZE) {
  const list = Array.isArray(suggestions) ? suggestions : [];
  const limit = Math.max(0, Number(visibleCount) || 0);
  return {
    items: list.slice(0, limit),
    hiddenCount: Math.max(0, list.length - limit),
    hasMore: list.length > limit,
    total: list.length,
  };
}

/** formatSuggestionViewSummary 给筛选栏右侧的计数文案。 */
export function formatSuggestionViewSummary({ total = 0, shown = 0, hidden = 0 } = {}) {
  if (Number(total) <= 0) return "没有符合筛选条件的建议";
  const base = `显示 ${shown} / ${total} 条建议`;
  return Number(hidden) > 0 ? `${base}（还有 ${hidden} 条未显示）` : base;
}

/** parseStoredSuggestionModalSize 解析 localStorage 里记住的弹窗尺寸。 */
export function parseStoredSuggestionModalSize(raw) {
  if (raw === null || raw === undefined || raw === "") return null;
  let parsed = raw;
  if (typeof raw === "string") {
    try {
      parsed = JSON.parse(raw);
    } catch {
      return null;
    }
  }
  const width = Number(parsed?.width);
  const height = Number(parsed?.height);
  if (!Number.isFinite(width) || !Number.isFinite(height)) return null;
  if (width <= 0 || height <= 0) return null;
  return { width: Math.round(width), height: Math.round(height) };
}
