// 策略组在 Mod 管理页的纯函数层（与 DOM 无关，node --test 覆盖）。
//
// 后端 GetModGroupMembership 返回的是"成员级"记录（一个 Mod 可以属于多个组），
// 这里把它转成列表徽标与筛选需要的索引，并集中实现筛选判定与文案。

import { filePriorityKeys } from "../conflicts/conflict-badge.mjs";

export function normalizeGroupKey(key) {
  return String(key ?? "")
    .trim()
    .replaceAll("/", "\\")
    .toLowerCase();
}

/** buildGroupIndex：addonlist 键 -> 该键所属的组成员记录（可能多个组）。 */
export function buildGroupIndex(memberships) {
  const index = new Map();
  (Array.isArray(memberships) ? memberships : []).forEach((membership) => {
    const key = normalizeGroupKey(membership?.key);
    if (!key || !membership?.groupId) return;
    if (!index.has(key)) index.set(key, []);
    index.get(key).push(membership);
  });
  return index;
}

/** groupsForFile：某个 Mod 文件当前属于哪些组（按 addonlist 键匹配）。 */
export function groupsForFile(file, index, currentDirectory = "") {
  if (!index?.size) return [];
  const result = [];
  const seen = new Set();
  for (const key of filePriorityKeys(file, currentDirectory)) {
    for (const membership of index.get(key) || []) {
      if (seen.has(membership.groupId)) continue;
      seen.add(membership.groupId);
      result.push(membership);
    }
  }
  return result;
}

const STRATEGY_LABELS = {
  all: "全部开启",
  off: "全部关闭",
  single: "互斥单选",
  single_random: "随机单选",
};

export function formatGroupStrategy(strategy) {
  return STRATEGY_LABELS[String(strategy || "")] || String(strategy || "未设置");
}

/** formatGroupChip：列表徽标上的短标签。 */
export function formatGroupChip(membership) {
  const name = String(membership?.groupName || membership?.groupId || "").trim();
  return name ? `组：${name}` : "";
}

export function formatGroupChipTitle(membership) {
  const name = String(membership?.groupName || membership?.groupId || "").trim();
  if (!name) return "";
  const parts = [
    `策略组「${name}」`,
    `共 ${Number(membership?.memberCount || 0)} 个成员`,
    `策略：${formatGroupStrategy(membership?.strategy)}`,
  ];
  if (membership?.tier !== null && membership?.tier !== undefined) {
    parts.push(`组权重 ${membership.tier}`);
  }
  if (membership?.enforce) parts.push("已开启自动联动");
  parts.push("单击整组开关；整组优先级前移/后移在工具栏「分组」菜单里");
  return parts.join(" · ");
}

/** buildGroupFilterOptions：按组聚合出筛选下拉需要的选项。 */
export function buildGroupFilterOptions(memberships) {
  const byId = new Map();
  (Array.isArray(memberships) ? memberships : []).forEach((membership) => {
    const id = String(membership?.groupId || "").trim();
    if (!id) return;
    if (!byId.has(id)) {
      byId.set(id, {
        id,
        name: String(membership?.groupName || id),
        strategy: String(membership?.strategy || ""),
        memberCount: Number(membership?.memberCount || 0),
        keys: new Set(),
        missingCount: 0,
        missingNames: [],
      });
    }
    if (membership?.missing) {
      const option = byId.get(id);
      option.missingCount += 1;
      const name = String(membership?.name || membership?.key || "").trim();
      if (name) option.missingNames.push(name);
    }
    const key = normalizeGroupKey(membership?.key);
    if (key) byId.get(id).keys.add(key);
  });
  return [...byId.values()].sort((left, right) =>
    left.name.localeCompare(right.name, "zh-CN"),
  );
}

/**
 * formatGroupOptionLabel 生成分组筛选菜单里的组标题：
 * 「组名（成员数）」；有缺失成员时补上「含 N 个缺失」。
 */
export function formatGroupOptionLabel(option) {
  if (!option) return "";
  const memberCount = Number(option.memberCount || 0);
  const missingCount = Number(option.missingCount || 0);
  const base = `${option.name || option.id}（${memberCount}）`;
  return missingCount > 0 ? `${base} · 含 ${missingCount} 个缺失` : base;
}

/**
 * formatGroupMissingNotice 生成"缺失成员"提示文案（设置页/悬浮说明用）。
 * 示例：文件缺失 2 个：a.vpk、b.vpk（放回同名文件会自动回到组里）
 */
export function formatGroupMissingNotice(missingNames, limit = 4) {
  const names = (Array.isArray(missingNames) ? missingNames : [])
    .map((name) => String(name || "").trim())
    .filter(Boolean);
  if (names.length === 0) return "";
  const shown = names.slice(0, limit).join("、");
  const extra = names.length > limit ? ` 等 ${names.length} 个` : "";
  return `文件缺失 ${names.length} 个：${shown}${extra}（放回同名文件会自动回到组里）`;
}

/**
 * formatFileGroupImpact 生成"这个文件属于哪些组"的提示文案。
 * 删除前用来说明"组成员会保留，只是变成缺失成员"。
 */
export function formatFileGroupImpact(groupNames) {
  const names = (Array.isArray(groupNames) ? groupNames : [])
    .map((name) => String(name || "").trim())
    .filter(Boolean);
  if (names.length === 0) return "";
  return `该 Mod 还在 ${names.length} 个策略组里：${names.join("、")}。\n` +
    "删除后组成员会保留（显示为缺失成员），把同名文件放回来就会自动重新回到组里。";
}

/** fileMatchesGroupFilter：没有勾选任何组时视为不过滤。 */
export function fileMatchesGroupFilter(file, options, activeGroupIds, currentDirectory = "") {
  if (!activeGroupIds || activeGroupIds.size === 0) return true;
  const keys = filePriorityKeys(file, currentDirectory);
  for (const option of options || []) {
    if (!activeGroupIds.has(option.id)) continue;
    if (keys.some((key) => option.keys.has(key))) return true;
  }
  return false;
}

export function formatGroupFilterLabel(activeGroupIds, options) {
  const count = activeGroupIds?.size || 0;
  if (count === 0) return "全部分组";
  if (count === 1) {
    const id = [...activeGroupIds][0];
    const option = (options || []).find((item) => item.id === id);
    return option ? option.name : "1 个分组";
  }
  return `${count} 个分组`;
}

const CONFIDENCE_LABELS = { high: "高置信度", medium: "中置信度", low: "低置信度" };

export function formatSuggestionConfidence(confidence) {
  return CONFIDENCE_LABELS[String(confidence || "")] || "待确认";
}

/** formatSuggestionSummary：建议卡片副标题。 */
export function formatSuggestionSummary(suggestion) {
  if (!suggestion) return "";
  const memberCount = Array.isArray(suggestion.memberKeys) ? suggestion.memberKeys.length : 0;
  const parts = [
    `${memberCount} 个 Mod`,
    formatSuggestionConfidence(suggestion.confidence),
    String(suggestion.reason || "").trim(),
  ].filter(Boolean);
  return parts.join(" · ");
}

export function formatSuggestionSignals(suggestion) {
  return Array.isArray(suggestion?.signals) ? suggestion.signals.filter(Boolean) : [];
}

const SUGGESTION_MEMBER_LOCATION_LABELS = {
  root: "根目录",
  workshop: "创意工坊",
  disabled: "已禁用",
};

/**
 * buildSuggestionFileIndex 按 addonlist 键给当前文件列表建索引，
 * 让"分组建议"卡片能显示每个成员的详情（位置、游戏开关、优先级）与操作按钮。
 */
export function buildSuggestionFileIndex(files, currentDirectory = "") {
  const index = new Map();
  (Array.isArray(files) ? files : []).forEach((file) => {
    filePriorityKeys(file, currentDirectory).forEach((key) => {
      if (key && !index.has(key)) {
        index.set(key, file);
      }
    });
  });
  return index;
}

/**
 * describeSuggestionMember 生成建议卡片里单个成员的展示文案与可用操作
 * （与冲突检测界面保持一致的语义）：
 *   - workshop：只能"复制到 addons"；
 *   - disabled：可以"启用"，但不能直接改游戏开关；
 *   - root：可以"禁用"，也可以改游戏开关。
 */
export function describeSuggestionMember(file, options = {}) {
  const priorityLabel = String(options.priorityLabel || "").trim();
  if (!file) {
    return {
      found: false,
      location: "未知位置",
      gameState: "游戏开关：未记录",
      priority: priorityLabel || "未写入 addonlist",
      fileAction: "",
      fileActionKind: "unknown",
      canToggleFile: false,
      canEditGameState: false,
    };
  }

  const locationKey = String(file.location || "root");
  const location = SUGGESTION_MEMBER_LOCATION_LABELS[locationKey] || locationKey;
  const gameStateKnown = Boolean(file.gameStateKnown);
  const gameState = gameStateKnown
    ? file.gameEnabled
      ? "游戏开关：开"
      : "游戏开关：关"
    : "游戏开关：未记录";

  let fileAction = "禁用";
  let fileActionKind = "disable";
  if (locationKey === "workshop") {
    fileAction = "复制到 addons";
    fileActionKind = "transfer";
  } else if (locationKey === "disabled") {
    fileAction = "启用";
    fileActionKind = "enable";
  }

  return {
    found: true,
    location,
    gameState,
    priority: priorityLabel || "未写入 addonlist",
    fileAction,
    fileActionKind,
    canToggleFile: locationKey !== "workshop",
    canEditGameState: locationKey !== "disabled",
  };
}

/** suggestionMemberRows：建议弹窗里的逐行成员（带是否已勾选）。 */
export function suggestionMemberRows(suggestion, selectedKeys) {
  const keys = Array.isArray(suggestion?.memberKeys) ? suggestion.memberKeys : [];
  const names = Array.isArray(suggestion?.memberNames) ? suggestion.memberNames : [];
  return keys.map((key, index) => ({
    key,
    name: String(names[index] || key),
    selected: selectedKeys ? selectedKeys.has(key) : true,
  }));
}

export function defaultSuggestionSelection(suggestion) {
  return new Set(Array.isArray(suggestion?.memberKeys) ? suggestion.memberKeys : []);
}

export function formatSuggestionCreateSummary(suggestion, selectedKeys) {
  const total = Array.isArray(suggestion?.memberKeys) ? suggestion.memberKeys.length : 0;
  const selected = selectedKeys?.size || 0;
  if (selected === 0) return "请至少勾选一个 Mod";
  if (selected === total) return `将创建包含全部 ${total} 个 Mod 的策略组`;
  return `将创建包含 ${selected} / ${total} 个 Mod 的策略组`;
}
