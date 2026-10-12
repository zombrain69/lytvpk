// 策略组在 Mod 管理页的纯函数层（与 DOM 无关，node --test 覆盖）。
//
// 后端 GetModGroupMembership 返回的是"成员级"记录（一个 Mod 可以属于多个组），
// 这里把它转成列表徽标与筛选需要的索引，并集中实现筛选判定与文案。

import { t } from "../../core/i18n-runtime.js";
import { createLocaleCollator } from "../../core/locale-collator.mjs";
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

// 策略名文案走目录（模块求值期不调 t()）：key 表 + 取值时解析。
const STRATEGY_LABEL_KEYS = {
  all: "strategyGroups.strategy.all",
  off: "strategyGroups.strategy.off",
  single: "strategyGroups.strategy.single",
  single_random: "strategyGroups.strategy.singleRandom",
};

export function formatGroupStrategy(strategy) {
  const key = STRATEGY_LABEL_KEYS[String(strategy || "")];
  if (key) return t(key);
  return String(strategy || t("strategyGroups.strategy.unset"));
}

/** formatGroupChip：列表徽标上的短标签。 */
export function formatGroupChip(membership) {
  const name = String(membership?.groupName || membership?.groupId || "").trim();
  return name ? t("strategyGroups.view.chip", { name }) : "";
}

export function formatGroupChipTitle(membership) {
  const name = String(membership?.groupName || membership?.groupId || "").trim();
  if (!name) return "";
  const parts = [
    t("strategyGroups.view.chipTitle.group", { name }),
    t("strategyGroups.view.chipTitle.members", { count: Number(membership?.memberCount || 0) }),
    t("strategyGroups.view.chipTitle.strategy", { strategy: formatGroupStrategy(membership?.strategy) }),
  ];
  if (membership?.tier !== null && membership?.tier !== undefined) {
    parts.push(t("strategyGroups.view.chipTitle.tier", { tier: membership.tier }));
  }
  if (membership?.enforce) parts.push(t("strategyGroups.view.chipTitle.enforce"));
  parts.push(t("strategyGroups.view.chipTitle.openHint"));
  parts.push(t("strategyGroups.view.chipTitle.menuHint"));
  return parts.join(" · ");
}

/**
 * groupEnabledVote 统计一个策略组在 addonlist 里的开关情况。
 *
 * 「整组开关」要按多数派决定方向（多数开着就整组关，反之整组开），
 * Mod 列表的分组菜单与「策略组管理」窗口的聚焦条共用这一份判定。
 */
export function groupEnabledVote(memberships, files, groupId, currentDirectory = "") {
  const keys = new Set(
    (Array.isArray(memberships) ? memberships : [])
      .filter((item) => String(item?.groupId || "") === String(groupId || ""))
      .map((item) => normalizeGroupKey(item?.key)),
  );
  let enabled = 0;
  let disabled = 0;
  (Array.isArray(files) ? files : []).forEach((file) => {
    const fileKeys = filePriorityKeys(file, currentDirectory);
    if (!fileKeys.some((key) => keys.has(key))) return;
    if (file.gameEnabled) enabled += 1;
    else if (file.gameStateKnown) disabled += 1;
  });
  return { enabled, disabled, mostlyEnabled: enabled >= disabled };
}

// ── 「从 Mod 列表点进来」的聚焦条文案 ─────────────────────────────────────
// 徽标点击 → 策略组管理窗口的一个聚焦模式：窗口顶部把"这个 Mod 属于哪些组"
// 一次列全，点其中一个就展开并跳到那一组，不用再回到列表里逐个猜。

function membershipLabel(membership) {
  return String(membership?.groupName || membership?.groupId || "").trim();
}

/** formatModGroupFocusSummary 聚焦条上的一句话：这个 Mod 一共属于几个组。 */
export function formatModGroupFocusSummary(memberships) {
  const count = (Array.isArray(memberships) ? memberships : []).filter((item) =>
    membershipLabel(item),
  ).length;
  if (count === 0) return t("strategyGroups.view.focusNone");
  return t("strategyGroups.view.focusSummary", { count });
}

/** formatModGroupFocusChip 聚焦条上的一个组按钮文案（带成员数）。 */
export function formatModGroupFocusChip(membership) {
  const name = membershipLabel(membership);
  if (!name) return "";
  return t("strategyGroups.view.focusChip", { name, count: Number(membership?.memberCount || 0) });
}

/** formatModGroupFocusChipTitle 聚焦条上组按钮的悬浮说明。 */
export function formatModGroupFocusChipTitle(membership, activeGroupId = "") {
  const name = membershipLabel(membership);
  if (!name) return "";
  const isActive = String(membership?.groupId || "") === String(activeGroupId || "");
  return (
    t("strategyGroups.view.focusChipTitle.base", {
      name,
      strategy: formatGroupStrategy(membership?.strategy),
    }) +
    t("strategyGroups.view.focusChipTitle.members", { count: Number(membership?.memberCount || 0) }) +
    (isActive
      ? t("strategyGroups.view.focusChipTitle.active")
      : t("strategyGroups.view.focusChipTitle.idle"))
  );
}

/**
 * buildGroupFilterOptions：按组聚合出筛选下拉需要的选项。
 *
 * 选项里带上 `tier`（组权重）、`parentId` / `parentName` / `depth`（上级分组层级），
 * 这样筛选菜单能像「策略组管理」窗口一样按权重排序并把子组显示在父组下面。
 */
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
        enforce: Boolean(membership?.enforce),
        tier:
          membership?.tier === null || membership?.tier === undefined || membership?.tier === ""
            ? null
            : Number(membership.tier),
        parentId: String(membership?.parentId || ""),
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
  // 层级信息需要"全部组都在手里"才能算，所以放在循环之后补。
  const options = [...byId.values()];
  options.forEach((option) => {
    option.parentName = option.parentId ? byId.get(option.parentId)?.name || "" : "";
    if (option.parentId && !byId.has(option.parentId)) option.parentId = "";
    option.depth = 1;
  });
  options.forEach((option) => {
    option.depth = groupOptionDepth(option, byId);
  });
  return options;
}

/** groupOptionDepth 计算缩进层级；父链成环或缺父时按顶层处理（不能无限递归）。 */
function groupOptionDepth(option, byId) {
  let depth = 1;
  let current = option;
  const seen = new Set([option.id]);
  while (current?.parentId) {
    const parent = byId.get(current.parentId);
    if (!parent || seen.has(parent.id)) return 1;
    seen.add(parent.id);
    depth += 1;
    current = parent;
  }
  return depth;
}

/** groupOptionTierSortValue 未设置权重的组排在最后。 */
function groupOptionTierSortValue(option) {
  const tier = option?.tier;
  return tier === null || tier === undefined || Number.isNaN(Number(tier))
    ? Number.POSITIVE_INFINITY
    : Number(tier);
}

// 本地化排序（第 19 项 P3）：组名/ID 跟随界面语言。
const GROUP_COLLATOR = createLocaleCollator();

function compareGroupOptions(left, right) {
  const tierDiff = groupOptionTierSortValue(left) - groupOptionTierSortValue(right);
  if (tierDiff !== 0) return tierDiff;
  const byName = GROUP_COLLATOR.compare(String(left?.name || ""), String(right?.name || ""));
  if (byName !== 0) return byName;
  return GROUP_COLLATOR.compare(String(left?.id || ""), String(right?.id || ""));
}

/**
 * sortGroupFilterOptions 生成「按分组筛选」的显示顺序：
 *   1. **按组权重升序**（未设置权重排最后，同权重按名称）—— 常用组给个更小的权重就排到最上面；
 *   2. **子组紧跟自己的上级分组**：一棵分组树整体上下移动，不会因为子组名字前缀不同
 *      （例如 `【…】` 和 `!…`）被甩到列表另一头、跟父组脱开；
 *   3. 子树的位置由**子树里最小的权重**决定：给子组设权重，它所在的整棵树会一起上浮，
 *      这样"常编辑的子组"也能一键排到前面，同时仍然待在自己的上级分组下面。
 */
export function sortGroupFilterOptions(options) {
  const list = Array.isArray(options) ? options : [];
  const byId = new Map(list.map((option) => [String(option?.id || ""), option]));
  const childrenOf = new Map();
  const roots = [];
  list.forEach((option) => {
    const id = String(option?.id || "");
    const parentId = String(option?.parentId || "");
    const parent = parentId && parentId !== id ? byId.get(parentId) : null;
    // 父组不在列表里 / 父链成环 → 当顶层处理，保证每组都会出现且只出现一次。
    if (parent && !hasAncestorCycle(option, byId)) {
      if (!childrenOf.has(parentId)) childrenOf.set(parentId, []);
      childrenOf.get(parentId).push(option);
      return;
    }
    roots.push(option);
  });

  const subtreeKeys = new Map();
  const subtreeKey = (option, guard) => {
    const id = String(option?.id || "");
    const cached = subtreeKeys.get(id);
    if (cached !== undefined) return cached;
    if (guard.has(id)) return groupOptionTierSortValue(option);
    guard.add(id);
    let key = groupOptionTierSortValue(option);
    (childrenOf.get(id) || []).forEach((child) => {
      key = Math.min(key, subtreeKey(child, guard));
    });
    guard.delete(id);
    subtreeKeys.set(id, key);
    return key;
  };

  const compareSubtree = (left, right) => {
    const diff = subtreeKey(left, new Set()) - subtreeKey(right, new Set());
    if (diff !== 0) return diff;
    return compareGroupOptions(left, right);
  };

  const flat = [];
  const visited = new Set();
  const push = (option) => {
    const id = String(option?.id || "");
    if (visited.has(id)) return;
    visited.add(id);
    flat.push(option);
    (childrenOf.get(id) || []).slice().sort(compareSubtree).forEach(push);
  };
  roots.slice().sort(compareSubtree).forEach(push);
  // 兜底：成环等异常情况下没被走过的组，按原顺序补在最后，绝不丢组。
  list.forEach((option) => push(option));
  return flat;
}

/** hasAncestorCycle 判断这个组的父链是否会绕回自己。 */
function hasAncestorCycle(option, byId) {
  const seen = new Set([String(option?.id || "")]);
  let current = byId.get(String(option?.parentId || ""));
  while (current) {
    const id = String(current.id || "");
    if (seen.has(id)) return true;
    seen.add(id);
    current = current.parentId ? byId.get(String(current.parentId)) : null;
  }
  return false;
}

/** formatGroupOptionIndent 生成筛选菜单里的层级缩进（子组显示在父组下面）。 */
export function formatGroupOptionIndent(option) {
  const depth = Math.max(1, Number(option?.depth || 1));
  if (depth <= 1) return "";
  return `${"　".repeat(depth - 2)}└ `;
}

/**
 * formatGroupOptionLabel 生成分组筛选菜单里的组标题：
 * 「组名（成员数） · 权重 N」；有缺失成员时补上「含 N 个缺失」。
 */
export function formatGroupOptionLabel(option) {
  if (!option) return "";
  const memberCount = Number(option.memberCount || 0);
  const missingCount = Number(option.missingCount || 0);
  const tier = groupOptionTierSortValue(option);
  const parts = [t("strategyGroups.view.optionName", { name: option.name || option.id, count: memberCount })];
  if (Number.isFinite(tier)) parts.push(t("strategyGroups.common.tierTag", { tier }));
  if (missingCount > 0) parts.push(t("strategyGroups.view.optionMissing", { count: missingCount }));
  return parts.join(" · ");
}

/**
 * formatGroupMissingNotice 生成"缺失成员"提示文案（策略组管理窗口/悬浮说明用）。
 * 示例：文件缺失 2 个：a.vpk、b.vpk（放回同名文件会自动回到组里）
 */
export function formatGroupMissingNotice(missingNames, limit = 4) {
  const names = (Array.isArray(missingNames) ? missingNames : [])
    .map((name) => String(name || "").trim())
    .filter(Boolean);
  if (names.length === 0) return "";
  const shown = names.slice(0, limit).join("、");
  const extra = names.length > limit ? t("strategyGroups.view.missingMore", { count: names.length }) : "";
  return t("strategyGroups.view.missingNotice", { count: names.length, shown, extra });
}

/**
 * formatGroupSubtreeMissingNotice 生成"子组里有缺失"的汇总提示。
 *
 * 为什么需要（对齐 FireAxe 的 AddonChildrenProblem）：父组自己的成员可能一个不缺，
 * 问题藏在展开后的子组里 —— 父组行如果什么都不显示，用户会以为整棵树都是健康的。
 */
export function formatGroupSubtreeMissingNotice(subtreeMissingCount, affectedChildCount) {
  const missing = Number(subtreeMissingCount) || 0;
  if (missing <= 0) return "";
  const children = Number(affectedChildCount) || 0;
  const scope = children > 0 ? t("strategyGroups.view.subtreeScope", { count: children }) : "";
  return t("strategyGroups.view.subtreeMissing", { count: missing, scope });
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
  return t("strategyGroups.view.deleteImpact", { count: names.length, names: names.join("、") });
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
  if (count === 0) return t("strategyGroups.view.filterAll");
  if (count === 1) {
    const id = [...activeGroupIds][0];
    const option = (options || []).find((item) => item.id === id);
    return option ? option.name : t("strategyGroups.view.filterOne");
  }
  return t("strategyGroups.view.filterCount", { count });
}

const CONFIDENCE_LABEL_KEYS = {
  high: "strategyGroups.view.confidence.high",
  medium: "strategyGroups.view.confidence.medium",
  low: "strategyGroups.view.confidence.low",
};

export function formatSuggestionConfidence(confidence) {
  return t(CONFIDENCE_LABEL_KEYS[String(confidence || "")] || "strategyGroups.view.confidence.fallback");
}

/** formatSuggestionSummary：建议卡片副标题。 */
export function formatSuggestionSummary(suggestion) {
  if (!suggestion) return "";
  const memberCount = Array.isArray(suggestion.memberKeys) ? suggestion.memberKeys.length : 0;
  const parts = [
    t("strategyGroups.view.modCount", { count: memberCount }),
    formatSuggestionConfidence(suggestion.confidence),
    String(suggestion.reason || "").trim(),
  ].filter(Boolean);
  return parts.join(" · ");
}

export function formatSuggestionSignals(suggestion) {
  return Array.isArray(suggestion?.signals) ? suggestion.signals.filter(Boolean) : [];
}

const SUGGESTION_MEMBER_LOCATION_KEYS = {
  root: "strategyGroups.view.location.root",
  workshop: "strategyGroups.view.location.workshop",
  disabled: "strategyGroups.view.location.disabled",
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
      location: t("strategyGroups.view.location.unknown"),
      gameState: t("strategyGroups.view.gameState.unrecorded"),
      gameStateButton: t("strategyGroups.view.gameStateButton.unrecorded"),
      priority: priorityLabel || t("mods.priority.labelUnknown"),
      fileAction: "",
      fileActionKind: "unknown",
      canToggleFile: false,
      canEditGameState: false,
    };
  }

  const locationKey = String(file.location || "root");
  const location = SUGGESTION_MEMBER_LOCATION_KEYS[locationKey]
    ? t(SUGGESTION_MEMBER_LOCATION_KEYS[locationKey])
    : locationKey;
  const gameStateKnown = Boolean(file.gameStateKnown);
  let gameState = t("strategyGroups.view.gameState.unrecorded");
  let gameStateButton = t("strategyGroups.view.gameStateButton.unrecorded");
  if (gameStateKnown) {
    gameState = file.gameEnabled
      ? t("strategyGroups.view.gameState.on")
      : t("strategyGroups.view.gameState.off");
    gameStateButton = file.gameEnabled
      ? t("strategyGroups.view.gameStateButton.on")
      : t("strategyGroups.view.gameStateButton.off");
  }

  let fileAction = t("strategyGroups.view.fileAction.disable");
  let fileActionKind = "disable";
  if (locationKey === "workshop") {
    fileAction = t("strategyGroups.view.fileAction.transfer");
    fileActionKind = "transfer";
  } else if (locationKey === "disabled") {
    fileAction = t("strategyGroups.view.fileAction.enable");
    fileActionKind = "enable";
  }

  return {
    found: true,
    location,
    gameState,
    gameStateButton,
    priority: priorityLabel || t("mods.priority.labelUnknown"),
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
  if (selected === 0) return t("strategyGroups.view.createNone");
  if (selected === total) return t("strategyGroups.view.createAll", { count: total });
  return t("strategyGroups.view.createPartial", { selected, total });
}
