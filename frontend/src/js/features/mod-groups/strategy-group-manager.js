// 「策略组管理」独立窗口：策略组的完整生命周期都集中在这里，不再塞进设置页。
//
// 覆盖：用选中的 Mod 建组 / 按策略应用 / 随机单选 / 全关 / 重命名 / 删除 /
// 上级分组（树形层级）/ 组权重 / 自动联动，以及多选后的批量管理
// （批量删除、批量开关自动联动、批量设置·清除权重）。
//
// 只写 groups.json：不改动任何 Mod 文件，也不改 addonlist.txt；
// 组权重只写本地分层，仍需在「编辑加载顺序」里点「按分层应用」才会重排。

import { appState, onFileSelectionChanged } from "../state.js";
import { escapeHtml } from "../../core/utils.js";
import { showError, showNotification } from "../../core/toast.js";
import { getConfig, saveConfig } from "../../core/config.js";
import { getFloatingModal, setupFloatingModal } from "../../core/floating-modal.js";
import {
  ApplyModStrategyGroup,
  BatchUpdateModStrategyGroups,
  CaptureModStrategyGroup,
  CheckModStrategyGroupApply,
  DeleteModStrategyGroup,
  GetModStrategyGroupMissingMembers,
  ListModStrategyGroups,
  ListModStrategyGroupTree,
  MoveModStrategyGroup,
  ReorderModStrategyGroups,
  RenameModStrategyGroup,
  RemoveModStrategyGroupMembers,
  CreateModStrategyGroupChild,
  SetModStrategyGroupEnabled,
  SetModStrategyGroupEnforcement,
  SetModStrategyGroupTier,
  SetVPKGameEnabledBatch,
  ToggleVPKFile,
} from "../../../../wailsjs/go/app/App";
import { renderFileList } from "../file-list/render.js";
import { refreshFilesKeepFilter } from "../file-list/filters.js";
import {
  confirmVPKOperationWarning,
  moveWorkshopFilesToAddons,
} from "../file-list/operations.js";
import {
  formatBatchGameStateConfirm,
  formatBatchGameStateSummary,
} from "../file-list/batch-game-state-format.mjs";
import { formatPriorityLabel, normalizePriorityTier } from "../file-list/priority-label.mjs";
import { GROUP_MANAGER_SEARCH_HELP_VARIANT, buildSearchHelpHtml, buildSearchHelpTitle } from "../file-list/search-help.mjs";
import { buildGroupMembersFromSelection } from "../settings/selection-args.mjs";
import {
  applyStrategyGroupDropOrder,
  buildParentOptions,
  flattenStrategyGroupTree,
  formatStrategyGroupDropMessage,
  resolveStrategyGroupDrop,
} from "../settings/strategy-group-tree.mjs";
import {
  formatStrategyGroupApplySummary,
  formatStrategyGroupBatchConfirm,
  formatStrategyGroupBatchResult,
  formatStrategyGroupFilterState,
  formatStrategyGroupSelectionLabel,
  formatStrategyGroupStrategy,
  summarizeStrategyGroupSelection,
} from "../settings/strategy-group-format.mjs";
import {
  buildSuggestionFileIndex,
  buildGroupIndex,
  formatModGroupFocusChip,
  formatModGroupFocusChipTitle,
  formatModGroupFocusSummary,
  formatGroupMissingNotice,
  formatGroupSubtreeMissingNotice,
  groupEnabledVote,
  groupsForFile,
  normalizeGroupKey,
} from "./group-view.mjs";
import { filePriorityKeys } from "../conflicts/conflict-badge.mjs";
import { buildModMemberRow } from "./member-row.js";
import {
  applyMemberScopeToggle,
  collectAllMemberKeys,
  collectMemberBatchTargets,
  countMemberKeysOutsideScope,
  describeMemberBatchActions,
  formatMemberBatchResult,
  formatMemberBatchSelectionLabel,
  memberKeysOfGroup,
  planMemberRemoval,
} from "./member-batch.mjs";
import { onModGroupMembershipChanged } from "./group-state.mjs";
import { showConfirmModal } from "../modals/confirm.js";
import { showPromptModal } from "../modals/prompt.js";
import {
  describeStrategyGroupSearch,
  filterStrategyGroupRows,
  formatBatchHint,
} from "./strategy-group-filter.mjs";
import { parseSearchSyntax } from "../file-list/search-syntax.mjs";

let managerState = null;

// 正在被拖动的策略组 ID。窗口里的行与底部"回到顶层"落点都要用它，
// 所以放在模块级（每行监听器随重画重建，但拖动状态只在这里维护）。
let draggingGroupId = "";
// 组列表的搜索关键字（窗口重画后保留）。
let managerQuery = "";

// 组名/组 ID 会被写进 HTML 属性，这里比 escapeHtml 多转义引号。
function escapeAttr(value) {
  return escapeHtml(value).replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

function element(id) {
  return document.getElementById(id);
}

function setStatus(message) {
  const status = element("strategy-group-status");
  if (status) status.textContent = message;
}

// ── 「上级分组」下拉：惰性填充 ──────────────────────────────────────────────
//
// 每一行的上级分组下拉原本内联了所有可选的组（除自己与自己的下级）。
// 真机上 212 个组 = 44,779 个 <option>、4.9 万节点、3.7MB HTML，
// 打开窗口要 1.6 秒（长任务 158ms）。这些选项绝大多数永远用不到，
// 所以先只画"当前值"那一个选项，等用户真的要改（pointerdown / 聚焦 / 键盘）
// 再一次性把候选填进去 —— 交互完全不变，DOM 少 95%。

/** parentOptionLabel 单个组的显示名（与 buildParentOptions 的缩进规则一致）。 */
function parentOptionLabel(groupId) {
  const row = (managerState?.rows || []).find((item) => String(item.group?.id) === String(groupId));
  if (!row) return "";
  return `${"— ".repeat(Math.max((row.depth || 1) - 1, 0))}${row.group.name || row.group.id}`;
}

/** parentOptionSummaryHtml 只渲染"当前值"，完整候选留到用户真的要用时再填。 */
function parentOptionSummaryHtml(group) {
  const parentId = String(group?.parentId || "");
  if (!parentId) {
    return `<option value="">（顶层）</option>`;
  }
  // 上级可能已经被删掉（脏数据）：这时退回"顶层"，与后端迁移行为一致。
  const label = parentOptionLabel(parentId);
  if (!label) {
    return `<option value="">（顶层）</option>`;
  }
  return `<option value="${escapeAttr(parentId)}" selected>${escapeHtml(label)}</option>`;
}

/** fillParentOptions 在用户第一次操作某个下拉时补上完整候选（同一个只填一次）。 */
function fillParentOptions(select) {
  if (!select || select.dataset.parentOptions === "filled") return;
  const keep = String(select.value || "");
  const options = buildParentOptions(managerState?.rows || [], select.dataset.groupId || "")
    .map(
      (option) =>
        `<option value="${escapeAttr(option.id)}" ${option.id === keep ? "selected" : ""}>${escapeHtml(option.label)}</option>`,
    )
    .join("");
  select.innerHTML = `<option value="">（顶层）</option>${options}`;
  select.dataset.parentOptions = "filled";
  select.value = keep;
}
// refreshAfterChange 把"组/开关变了"反映到主列表与筛选结果上。
async function refreshAfterChange() {
  try {
    renderFileList();
  } catch (error) {
    console.warn("刷新文件列表失败:", error);
  }
  try {
    await refreshFilesKeepFilter();
  } catch (error) {
    console.warn("刷新筛选结果失败:", error);
  }
}

/** 读取策略组、树形层级与缺失成员，供渲染使用。 */
async function loadManagerData() {
  // 三个调用互不依赖：并行发出。串行时窗口首屏要等三次 IPC 相加
  // （真机 17 + 18 + 10 ms，改造前其中一次是 1593ms）。
  const [groupsResult, treeResult, missingResult] = await Promise.allSettled([
    ListModStrategyGroups(),
    ListModStrategyGroupTree(),
    GetModStrategyGroupMissingMembers(),
  ]);
  let error = "";
  let groups = [];
  if (groupsResult.status === "fulfilled") {
    groups = groupsResult.value || [];
  } else {
    error = String(groupsResult.reason?.message || groupsResult.reason || "无法读取策略组");
  }
  const tree = treeResult.status === "fulfilled" ? treeResult.value || null : null;
  const missingByName = new Map();
  // missingSummary 保留完整条目（含"子树缺失"汇总），供父组行显示 AddonChildrenProblem 式的提示。
  const missingSummary = new Map();
  if (missingResult.status === "fulfilled") {
    const missing = missingResult.value || [];
    missing.forEach((item) => {
      missingByName.set(String(item.groupId), item.missingNames || []);
      missingSummary.set(String(item.groupId), item);
    });
  } else {
    console.warn("读取缺失成员失败:", missingResult.reason);
  }
  return {
    groups,
    rows: flattenStrategyGroupTree(tree, groups),
    missingByName,
    missingSummary,
    error,
  };
}

function selectionSet() {
  const state = appState;
  if (!(state.strategyGroupSelection instanceof Set)) state.strategyGroupSelection = new Set();
  return state.strategyGroupSelection;
}

function currentSummary() {
  const selection = selectionSet();
  const missingByGroupId = new Map();
  managerState?.rows?.forEach(({ group }) => {
    missingByGroupId.set(String(group.id), (managerState.missingByName.get(String(group.id)) || []).length);
  });
  return summarizeStrategyGroupSelection([...selection], managerState?.groups || [], missingByGroupId);
}

function pruneSelection() {
  const selection = selectionSet();
  const valid = new Set((managerState?.groups || []).map((group) => String(group.id)));
  [...selection].forEach((id) => {
    if (!valid.has(String(id))) selection.delete(id);
  });
}

/** expandedSet 「展开成员明细」的组集合（存在 appState，窗口重画后保持展开状态）。 */
function expandedSet() {
  const state = appState;
  if (!(state.strategyGroupExpanded instanceof Set)) state.strategyGroupExpanded = new Set();
  return state.strategyGroupExpanded;
}

/** activeFilterSet 主界面「按分组筛选」当前勾选的组（管理窗口里可以直接改）。 */
function activeFilterSet() {
  if (!(appState.activeGroupFilter instanceof Set)) appState.activeGroupFilter = new Set();
  return appState.activeGroupFilter;
}

/**
 * applyGroupFilterIds 把"选中的组"变成主界面的分组筛选：
 * 只改视图（appState.activeGroupFilter + 重新筛选列表），不写任何文件。
 */
async function applyGroupFilterIds(ids) {
  appState.activeGroupFilter = new Set((ids || []).map((id) => String(id)));
  renderManager();
  try {
    await refreshFilesKeepFilter();
  } catch (error) {
    setStatus("筛选失败: " + String(error?.message || error));
  }
}

function pruneExpanded() {
  const expanded = expandedSet();
  const valid = new Set((managerState?.groups || []).map((group) => String(group.id)));
  [...expanded].forEach((id) => {
    if (!valid.has(String(id))) expanded.delete(String(id));
  });
}

/** syncSelectAll 让「全选」勾选框跟随当前选中集合（部分选中时显示半选态）。 */
function syncSelectAll() {
  const selectAll = element("strategy-group-select-all");
  if (!selectAll) return;
  // 「全选」只看当前搜索结果：搜出 3 个组时全选 = 勾这 3 个。
  const visibleIds = visibleGroupIds();
  const selection = selectionSet();
  const selectedVisible = visibleIds.filter((id) => selection.has(id)).length;
  selectAll.checked = visibleIds.length > 0 && selectedVisible >= visibleIds.length;
  selectAll.indeterminate = selectedVisible > 0 && selectedVisible < visibleIds.length;
  selectAll.title =
    managerQuery && visibleIds.length !== (managerState?.groups || []).length
      ? `全选当前搜索出的 ${visibleIds.length} 个组`
      : "全选 / 取消全选下面列出的策略组";
}

/** visibleGroupIds 当前搜索条件下显示的组 ID（批量操作与"全选"都以此为准）。 */
function visibleGroupIds() {
  return filterStrategyGroupRows(managerState?.rows || [], managerQuery).map((row) =>
    String(row.group.id),
  );
}

// ── 成员级批量 ─────────────────────────────────────────────────────────────
// 组级批量（上面那条工具条）管的是"组"；这里管的是"展开出来的组成员"。
// 选择状态按 addonlist 键归一化保存，窗口重画后仍然保留。
let memberSelection = new Set();

function groupMemberKeys(group) {
  return memberKeysOfGroup(group);
}

/** expandedMemberKeys 当前展开的所有组成员键（"全选成员"的作用范围）。 */
function expandedMemberKeys() {
  const expanded = expandedSet();
  const keys = [];
  (managerState?.groups || []).forEach((group) => {
    if (!expanded.has(String(group.id))) return;
    groupMemberKeys(group).forEach((key) => keys.push(normalizeGroupKey(key)));
  });
  return [...new Set(keys)];
}

/** allMemberKeys 所有策略组的成员键（"全选所有组"的作用范围）。 */
function allMemberKeys() {
  return collectAllMemberKeys(managerState?.groups || []).map((key) => normalizeGroupKey(key));
}

function pruneMemberSelection() {
  if (memberSelection.size === 0) return;
  const valid = new Set();
  (managerState?.groups || []).forEach((group) => {
    groupMemberKeys(group).forEach((key) => valid.add(normalizeGroupKey(key)));
  });
  [...memberSelection].forEach((key) => {
    if (!valid.has(normalizeGroupKey(key))) memberSelection.delete(key);
  });
}

function currentMemberTargets() {
  const files = appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
  const fileIndex = buildSuggestionFileIndex(files, appState.currentDirectory);
  return collectMemberBatchTargets([...memberSelection], fileIndex);
}

// 成员批量按钮的 id：与 member-batch.mjs 里的动作 id 一一对应。
const MEMBER_BATCH_BUTTON_IDS = {
  "game-on": "strategy-group-member-batch-game-on",
  "game-off": "strategy-group-member-batch-game-off",
  enable: "strategy-group-member-batch-enable",
  disable: "strategy-group-member-batch-disable",
  transfer: "strategy-group-member-batch-transfer",
  remove: "strategy-group-member-batch-remove",
};

/** updateMemberBatchBar 刷新成员批量条的计数、按钮可用性与"为什么不能点"。 */
function updateMemberBatchBar() {
  const bar = element("strategy-group-member-batch");
  if (!bar) return;
  pruneMemberSelection();
  bar.classList.toggle("hidden", expandedSet().size === 0);

  const expandedKeys = expandedMemberKeys();
  const selectionLabel = element("strategy-group-member-selection");
  if (selectionLabel) {
    // 跨组勾选是刻意保留的能力，但折叠组里的勾选在界面上看不见——
    // 不把数量说出来，用户点完「全选展开的成员」会看到计数比眼前多，却不知道多在哪。
    selectionLabel.textContent = formatMemberBatchSelectionLabel(
      memberSelection.size,
      countMemberKeysOutsideScope(memberSelection, expandedKeys),
    );
  }
  const hint = element("strategy-group-member-batch-hint");
  if (hint) {
    hint.textContent =
      memberSelection.size > 0
        ? "批量按钮一次作用于勾选的成员；每一行右侧的按钮仍然只作用那一行"
        : "勾选成员行最左边的方框后，这一排按钮会一次作用于全部选中（和每行右侧的单行按钮同义）";
    hint.classList.toggle("is-ready", memberSelection.size > 0);
  }

  describeMemberBatchActions(currentMemberTargets()).forEach((action) => {
    const button = element(MEMBER_BATCH_BUTTON_IDS[action.id]);
    if (!button) return;
    button.disabled = action.disabled;
    button.title = action.title;
  });

  const selectAll = element("strategy-group-member-select-all");
  if (selectAll) {
    const selected = expandedKeys.filter((key) => memberSelection.has(key)).length;
    selectAll.checked = expandedKeys.length > 0 && selected >= expandedKeys.length;
    selectAll.indeterminate = selected > 0 && selected < expandedKeys.length;
    selectAll.disabled = expandedKeys.length === 0;
    selectAll.title =
      expandedKeys.length > 0
        ? `全选 / 取消全选当前展开的 ${expandedSet().size} 个组（共 ${expandedKeys.length} 个成员）；不会动别的组已经勾上的`
        : "先展开至少一个组（点组名左边的展开箭头），这里才能全选成员";
  }

  // 每个组自己的「本组全选」：三态按本组勾选情况同步。
  const allKeys = allMemberKeys();
  document
    .querySelectorAll("#strategy-group-list input[data-group-member-pick-all]")
    .forEach((input) => {
      const group = groupById(input.dataset.groupMemberPickAll);
      if (!group) return;
      const keys = groupMemberKeys(group).map((key) => normalizeGroupKey(key));
      const selected = keys.filter((key) => memberSelection.has(key)).length;
      input.checked = keys.length > 0 && selected >= keys.length;
      input.indeterminate = selected > 0 && selected < keys.length;
      input.disabled = keys.length === 0;
    });

  const selectAllGroups = element("strategy-group-member-select-all-groups");
  if (selectAllGroups) {
    const selected = allKeys.filter((key) => memberSelection.has(key)).length;
    const allSelected = allKeys.length > 0 && selected >= allKeys.length;
    selectAllGroups.disabled = allKeys.length === 0;
    selectAllGroups.classList.toggle("is-on", allSelected);
    selectAllGroups.setAttribute("aria-pressed", String(allSelected));
    selectAllGroups.title = `勾选当前 ${(managerState?.groups || []).length} 个策略组的全部成员（共 ${allKeys.length} 个，多个组共用的 Mod 只算一次）`;
  }
  const clearAll = element("strategy-group-member-clear-all");
  if (clearAll) {
    clearAll.disabled = memberSelection.size === 0;
    clearAll.title =
      memberSelection.size > 0
        ? `取消全部勾选（包括折叠起来的组里的 ${countMemberKeysOutsideScope(memberSelection, expandedKeys)} 个）`
        : "当前没有勾选任何成员";
  }
}

/** syncMemberCheckboxes 只同步勾选框状态，不整表重画（避免滚动位置跳动）。 */
function syncMemberCheckboxes() {
  document
    .querySelectorAll("#strategy-group-list .mod-member-pick input[data-member-key]")
    .forEach((input) => {
      input.checked = memberSelection.has(normalizeGroupKey(input.dataset.memberKey));
    });
}

// ── 「从 Mod 列表点进来」的聚焦模式 ────────────────────────────────────────
// Mod 列表上的「组：xxx」徽标单击 → 打开这个窗口并定位到那一组：
// 顶部把这个 Mod 所属的**全部**组一次列全（点一下切组），那一组自动展开并高亮，
// 这个 Mod 的成员行也加上高亮。这里只做"看和跳"，所有动作仍走窗口里原有的按钮。

let focusState = null;

function currentFiles() {
  return appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
}

function fileByPath(filePath) {
  const target = String(filePath || "");
  if (!target) return null;
  return currentFiles().find((file) => String(file?.path || "") === target) || null;
}

/** focusedFileKeys 聚焦 Mod 的 addonlist 键（用来给成员行加高亮）。 */
function focusedFileKeys() {
  const file = fileByPath(focusState?.filePath);
  if (!file) return new Set();
  return new Set(filePriorityKeys(file, appState.currentDirectory).map(normalizeGroupKey));
}

/** focusMemberships 聚焦 Mod 所属的全部策略组（与列表徽标同一份索引）。 */
function focusMemberships() {
  const file = fileByPath(focusState?.filePath);
  if (!file) return [];
  const index =
    appState.modGroupIndex instanceof Map && appState.modGroupIndex.size > 0
      ? appState.modGroupIndex
      : buildGroupIndex(appState.modGroupMemberships || []);
  return groupsForFile(file, index, appState.currentDirectory);
}

function focusFileName() {
  const file = fileByPath(focusState?.filePath);
  const raw = String(file?.name || focusState?.filePath || "");
  return raw.split(/[\\/]/).pop() || raw;
}

/** renderFocusBar 画窗口顶部的聚焦条（没有聚焦目标时隐藏）。 */
function renderFocusBar() {
  const host = element("strategy-group-focus");
  if (!host) return;
  if (!focusState) {
    host.classList.add("hidden");
    host.replaceChildren();
    return;
  }
  const memberships = focusMemberships();
  const activeId = String(focusState.groupId || "");
  const chips = memberships
    .map((membership) => {
      const isActive = String(membership?.groupId || "") === activeId;
      const aria = isActive ? ' aria-current="true"' : "";
      const title = escapeAttr(formatModGroupFocusChipTitle(membership, activeId));
      const label = escapeHtml(formatModGroupFocusChip(membership));
      return (
        `<button type="button" class="strategy-group-focus-chip${isActive ? " is-active" : ""}"` +
        ` data-focus-group-id="${escapeAttr(membership?.groupId)}"${aria} title="${title}">${label}</button>`
      );
    })
    .join("");
  const title = escapeHtml(focusFileName());
  const pathTitle = escapeAttr(focusState.filePath);
  const summary = escapeHtml(formatModGroupFocusSummary(memberships));
  // 定位条要尽量矮。真机量过：三行（标题 / 组按钮 / 说明）＝144px，而这个窗口的
  // 滚动区一共只有 271px，再叠上底部那条粘性「回到顶层」落点（35px），
  // 刚跳过去的那一组只剩 60px 无遮挡视野 —— 它右侧的组级按钮（标题下第二行）
  // 整排被落点盖住，elementFromPoint 命中测试 10/10 都点不到。
  // 说明改挂 title：鼠标悬停照样看得全，条上只留一行标题 + 一行组按钮。
  host.title =
    "点下面的组名＝展开它并跳到那一行；那一行右侧的「按策略应用 / 随机单选 / 全关 / 重命名 / 删除」就是组级管理；" +
    "「展开成员」后还能勾选多行、用窗口底部的批量条一次处理。";
  host.classList.remove("hidden");
  host.innerHTML =
    `<div class="strategy-group-focus-main">` +
    `<span class="strategy-group-focus-title">定位：</span>` +
    `<span class="strategy-group-focus-file" title="${pathTitle}">${title}</span>` +
    `<span class="strategy-group-focus-count">${summary}</span>` +
    `<button type="button" class="settings-strategy-batch-btn" data-focus-action="toggle-group" title="把当前定位的这一组全部成员一起启用或关闭（只改 addonlist.txt 的 0/1）">整组开关</button>` +
    `<button type="button" class="settings-strategy-batch-btn" data-focus-action="clear" title="结束定位，回到普通的策略组列表（不会取消任何勾选）">清除定位</button>` +
    `</div>` +
    (chips ? `<div class="strategy-group-focus-chips">${chips}</div>` : "");
}

/** setFocusRequest 记录"从 Mod 列表点进来"的定位目标（空参数＝退出定位）。 */
function setFocusRequest(options = {}) {
  const groupId = String(options?.focusGroupId || "").trim();
  const filePath = String(options?.focusFilePath || "").trim();
  if (!groupId && !filePath) {
    focusState = null;
    return;
  }
  focusState = { groupId, filePath };
  if (!groupId) return;
  expandedSet().add(groupId);
  clearSearchIfGroupHidden(groupId);
}

/** clearSearchIfGroupHidden 搜索关键字会把定位的组筛掉时清空它（否则"定位"落到空白处）。 */
function clearSearchIfGroupHidden(groupId) {
  if (!managerQuery || visibleGroupIds().includes(String(groupId || ""))) return;
  managerQuery = "";
  const search = element("strategy-group-search");
  if (search) search.value = "";
}

/** focusOnGroup 在聚焦条里切到另一个组：展开它、跳过挡住它的搜索、滚过去并高亮。 */
function focusOnGroup(groupId) {
  if (!focusState) return;
  const nextId = String(groupId || "");
  focusState = { ...focusState, groupId: nextId };
  if (nextId) {
    expandedSet().add(nextId);
    clearSearchIfGroupHidden(nextId);
  }
  renderManager();
  revealFocusedGroup();
}

/**
 * applyFocusHighlight 给定位的组行与这个 Mod 的成员行补上高亮（只改 class，不滚动）。
 *
 * 必须由 renderManager 在每次重画后调用：列表是整片 innerHTML 重建的，
 * 只要别处再来一次重画（例如启动期组成员刷新触发的 reload），
 * 只加一次的 class 就会被抹掉 —— 真机上就是这么丢的（marked=1 → 0）。
 */
function applyFocusHighlight() {
  const list = element("strategy-group-list");
  if (!list || !focusState) return null;
  list
    .querySelectorAll(".mod-member-row.is-focus-member")
    .forEach((row) => row.classList.remove("is-focus-member"));
  const keys = focusedFileKeys();
  if (keys.size > 0) {
    list.querySelectorAll(".mod-member-row[data-member-key]").forEach((row) => {
      if (keys.has(normalizeGroupKey(row.dataset.memberKey))) row.classList.add("is-focus-member");
    });
  }
  const groupId = String(focusState.groupId || "");
  if (!groupId) return null;
  list
    .querySelectorAll("[data-group-row].is-focus-target")
    .forEach((row) => row.classList.remove("is-focus-target"));
  const row = list.querySelector(`[data-group-row="${CSS.escape(groupId)}"]`);
  if (!row) return null;
  row.classList.add("is-focus-target");
  return row;
}

/** revealFocusedGroup 重新补高亮，并把定位行滚到定位条下面。 */
function revealFocusedGroup() {
  const row = applyFocusHighlight();
  if (row) scrollRowBelowFocusBar(row);
}

/**
 * scrollRowBelowFocusBar 把定位的组行滚到"定位条下面"。
 *
 * 不能用 block:"center"：展开过的组行（25 个成员）可能比可视区还高，
 * 居中等于把组名顶出视野、只剩成员列表 —— 真机上量到过 rowTopVsBody = -1015。
 * 先按 block:"start" 贴到滚动区顶部，再把被粘性定位条盖住的那一段补回来。
 *
 * 注意方向：**加大** scrollTop 是把内容往上推，所以要把被盖住的部分"顶下来"，
 * 得**减** scrollTop（第一版写成 += ，真机量到行仍停在 -167，等于没修）。
 */
function scrollRowBelowFocusBar(row) {
  const body = row.closest(".strategy-group-body");
  if (!body) return;
  row.scrollIntoView({ block: "start" });
  const bar = element("strategy-group-focus");
  if (!bar || bar.classList.contains("hidden")) return;
  const overlap = bar.getBoundingClientRect().bottom + 8 - row.getBoundingClientRect().top;
  if (overlap > 0) body.scrollTop = Math.max(0, body.scrollTop - overlap);
}

/** toggleFocusedGroupEnabled 聚焦条上的「整组开关」：语义与列表「分组」菜单完全一致。 */
async function toggleFocusedGroupEnabled() {
  const groupId = String(focusState?.groupId || "");
  const group = groupById(groupId);
  if (!group) {
    setStatus("先在上面点一个组，再来整组开关");
    return;
  }
  const vote = groupEnabledVote(
    appState.modGroupMemberships || [],
    currentFiles(),
    groupId,
    appState.currentDirectory,
  );
  const nextEnabled = !vote.mostlyEnabled;
  // 与 Mod 列表的「整组开关」同一句确认文案：只说会改 addonlist 的 0/1。
  const action = nextEnabled ? "启用" : "关闭";
  const memberCount = (group.members || []).length;
  showConfirmModal(
    "整组开关",
    `把「${group.name}」的 ${memberCount} 个成员一起${action}。\n` +
      `（当前统计：启用 ${vote.enabled} 个、关闭 ${vote.disabled} 个）\n` +
      "只会修改 addonlist.txt 里的 0/1，不会重排顺序，也不会改动 Mod 文件。是否继续？",
    async () => {
      try {
        const result = await SetModStrategyGroupEnabled(groupId, nextEnabled);
        const skipped = Array.isArray(result?.skipped) ? result.skipped : [];
        const done = (result?.enabled?.length || 0) + (result?.disabled?.length || 0);
        showNotification(
          `「${group.name}」已整组${action} ${done} 个成员` +
            (skipped.length > 0 ? `，${skipped.length} 个成员的文件已不在列表里，已跳过` : ""),
          skipped.length > 0 ? "info" : "success",
        );
        await reload();
        await refreshAfterChange();
        revealFocusedGroup();
      } catch (error) {
        setStatus("整组开关失败: " + String(error?.message || error));
      }
    },
  );
}

/** 渲染批量工具条与组列表（整体重画，动作完成后调用）。 */
function renderManager() {
  if (!managerState) return;
  pruneSelection();
  pruneExpanded();
  pruneMemberSelection();
  syncSelectAll();
  renderFocusBar();
  // 定位模式标在滚动区上：底部那条「回到顶层」落点是 sticky 浮层，会把刚跳过去的
  // 组行盖掉一截（见 mods.css 的 .is-focus-mode 规则）。定位模式下它回到列表末尾。
  element("strategy-group-list")
    ?.closest(".strategy-group-body")
    ?.classList.toggle("is-focus-mode", Boolean(focusState));
  const list = element("strategy-group-list");
  const batch = element("strategy-group-batch");
  const status = element("strategy-group-status");
  if (status && managerState.error) status.textContent = managerState.error;
  const allRows = managerState.rows || [];
  // 搜索：只影响这一份列表；「全选 / 批量操作」都以"当前显示的组"为准。
  const rows = filterStrategyGroupRows(allRows, managerQuery);
  const visibleLabel = element("strategy-group-visible");
  if (visibleLabel) {
    visibleLabel.textContent = describeStrategyGroupSearch({
      total: allRows.length,
      shown: rows.length,
      query: managerQuery,
    });
  }
  const search = element("strategy-group-search");
  if (search && search.value !== managerQuery) search.value = managerQuery;
  // 悬停提示与 `?` 浮层都和 Mod 列表同源（search-help.mjs），只是字段与 tag: 含义不同。
  const searchHelpBtn = element("strategy-group-search-help-btn");
  const searchHelpPopover = element("strategy-group-search-help-popover");
  if (search) {
    search.title = buildSearchHelpTitle(GROUP_MANAGER_SEARCH_HELP_VARIANT);
    if (searchHelpPopover && searchHelpPopover.dataset.filled !== "1") {
      searchHelpPopover.innerHTML = buildSearchHelpHtml(GROUP_MANAGER_SEARCH_HELP_VARIANT);
      searchHelpPopover.dataset.filled = "1";
    }
    if (searchHelpBtn && searchHelpBtn.dataset.bound !== "1") {
      searchHelpBtn.dataset.bound = "1";
      const setHelpOpen = (open) => {
        searchHelpPopover?.classList.toggle("hidden", !open);
        searchHelpBtn.setAttribute("aria-expanded", String(open));
        searchHelpBtn.classList.toggle("is-active", open);
      };
      searchHelpBtn.addEventListener("click", (event) => {
        event.stopPropagation();
        setHelpOpen(searchHelpPopover?.classList.contains("hidden") ?? false);
      });
      document.addEventListener("click", (event) => {
        if (!searchHelpPopover || searchHelpPopover.classList.contains("hidden")) return;
        if (searchHelpPopover.contains(event.target) || searchHelpBtn.contains(event.target)) return;
        setHelpOpen(false);
      });
      document.addEventListener("keydown", (event) => {
        if (event.key !== "Escape" || !searchHelpPopover || searchHelpPopover.classList.contains("hidden")) return;
        event.stopPropagation();
        setHelpOpen(false);
      });
    }
  }

  if (batch) {
    batch.classList.toggle("hidden", allRows.length === 0);
    const selectionLabel = element("strategy-group-selection");
    if (selectionLabel) {
      // 和成员批量条同一类问题：搜索筛掉的那些组如果还勾着，界面上看不见，
      // 但批量删除 / 开关自动联动照样会作用到它们，所以把数量写在计数里。
      const visible = new Set(visibleGroupIds());
      const hiddenFromFilter = [...selectionSet()].filter((id) => !visible.has(String(id))).length;
      selectionLabel.textContent = formatStrategyGroupSelectionLabel(
        currentSummary(),
        hiddenFromFilter,
      );
    }
    const hasSelection = currentSummary().count > 0;
    // 「禁用」不是坏了：这里把原因显式说出来，按钮 tooltip 也补上说明。
    const hint = element("strategy-group-batch-hint");
    if (hint) {
      hint.textContent = hasSelection
        ? formatBatchHint(currentSummary().count)
        : formatBatchHint(0);
      hint.classList.toggle("is-ready", hasSelection);
    }
    [
      "strategy-group-batch-delete",
      "strategy-group-batch-enforce-on",
      "strategy-group-batch-enforce-off",
      "strategy-group-batch-tier-save",
      "strategy-group-batch-tier-clear",
      "strategy-group-batch-filter",
    ].forEach((id) => {
      const button = element(id);
      if (!button) return;
      if (button.dataset.defaultTitle === undefined) {
        button.dataset.defaultTitle = button.title || "";
      }
      button.disabled = !hasSelection;
      button.title = hasSelection
        ? button.dataset.defaultTitle
        : "先勾选要批量操作的策略组（每行最左边的方框，或直接点组名）";
    });
    const clearAll = element("strategy-group-clear-all");
    if (clearAll) {
      clearAll.disabled = !hasSelection;
      clearAll.title = hasSelection
        ? `取消全部勾选（当前共选中 ${currentSummary().count} 个组）`
        : "当前没有勾选任何策略组";
    }
  }
  if (!list) return;
  if (allRows.length === 0) {
    list.innerHTML =
      `<div class="setting-row-desc">还没有策略组。在 Mod 管理页勾选几个 Mod，再用上面的「用选中的 N 个 Mod 建组」创建。</div>`;
    return;
  }
  if (rows.length === 0) {
    const invalidRegex = parseSearchSyntax(managerQuery).regexInvalid;
    list.innerHTML = invalidRegex
      ? `<div class="setting-row-desc">正则表达式无效：${escapeHtml(invalidRegex)}；改好写法或清空搜索框即可看到全部 ${allRows.length} 个组。</div>`
      : `<div class="setting-row-desc">没有匹配「${escapeHtml(managerQuery)}」的策略组；清空搜索框即可看到全部 ${allRows.length} 个组。</div>`;
    return;
  }

  const selection = selectionSet();
  const expanded = expandedSet();
  const activeFilter = activeFilterSet();
  const filterState = element("strategy-group-filter-state");
  if (filterState) {
    filterState.textContent = formatStrategyGroupFilterState([...activeFilter], managerState.groups || []);
  }
  const clearFilterButton = element("strategy-group-batch-filter-clear");
  if (clearFilterButton) clearFilterButton.disabled = activeFilter.size === 0;
  list.innerHTML = rows
    .map(({ group, depth }) => {
      const missingNames = managerState.missingByName.get(String(group.id)) || [];
      const notice = formatGroupMissingNotice(missingNames);
      const missingEntry = managerState.missingSummary?.get(String(group.id));
      const subtreeNotice = missingEntry
        ? formatGroupSubtreeMissingNotice(
            missingEntry.subtreeMissingCount,
            missingEntry.affectedChildCount,
          )
        : "";
      const isExpanded = expanded.has(String(group.id));
      const memberCount = (group.members || []).length;
      const isFiltered = activeFilter.has(String(group.id));
      return `
        <div class="settings-profile-item${isFiltered ? " is-filtered" : ""}" data-group-row="${escapeAttr(group.id)}" data-group-name="${escapeAttr(group.name)}" data-group-depth="${Math.max(depth, 1)}" draggable="true" title="拖动这一行：放到别的组中间 = 变成它的子组；放到行的上/下边缘 = 排到它前/后；拖到列表空白处 = 回到顶层" style="margin-left: ${Math.max(depth - 1, 0) * 1.25}rem">
          <div class="settings-profile-main">
            <label class="settings-strategy-pick" title="选中这个策略组（用于批量管理）">
              <input type="checkbox" class="settings-strategy-pick-input" data-group-id="${escapeAttr(group.id)}" ${selection.has(String(group.id)) ? "checked" : ""}>
              <strong>${depth > 1 ? "└ " : ""}${escapeHtml(group.name)}</strong>
            </label>
            <span>${escapeHtml(formatStrategyGroupStrategy(group.strategy))} · ${(group.members || []).length} 个成员${group.enforce ? " · 自动联动" : ""}</span>
            ${
              notice
                ? `<span class="settings-strategy-missing" title="组成员不会因为文件被删除而移除；放在 addons / workshop / disabled 的同名文件会自动回到组里">⚠️ ${escapeHtml(notice)}</span>`
                : ""
            }
            ${
              subtreeNotice
                ? `<span class="settings-strategy-missing is-subtree" title="这个组自己的成员都在，但它的子孙组里有缺失文件；展开子组即可看到具体是哪些">⚠️ ${escapeHtml(subtreeNotice)}</span>`
                : ""
            }
          </div>
          <div class="settings-profile-actions">
            <button type="button" class="settings-strategy-expand" data-group-id="${escapeAttr(group.id)}" title="展开成员后可以直接查看详情、改游戏开关、启用/禁用、复制到 addons、把单个 Mod 移出本组">${isExpanded ? "收起成员" : `展开成员（${memberCount}）`}</button>
            <button type="button" class="settings-strategy-add-child" data-group-id="${escapeAttr(group.id)}" data-group-name="${escapeAttr(group.name)}" title="在这个组下面新建一个子组（子组会累加它和全部上级分组的权重；最多 4 层）">＋ 子组</button>
            <button type="button" class="settings-strategy-filter${isFiltered ? " is-active" : ""}" data-group-id="${escapeAttr(group.id)}" title="只让主界面显示属于这个组的 Mod（等同于「按分组筛选」勾上这个组；再点一次取消）">${isFiltered ? "取消筛选" : "筛选这组"}</button>
            <button type="button" class="settings-strategy-apply" data-group-id="${escapeAttr(group.id)}">按策略应用</button>
            <button type="button" class="settings-strategy-random" data-group-id="${escapeAttr(group.id)}">随机单选</button>
            <button type="button" class="settings-strategy-off" data-group-id="${escapeAttr(group.id)}" title="把这一组全部成员在 addonlist.txt 里设为关闭（组级动作，不删文件）。只想处理勾选的成员，请用窗口底部的成员批量条。">全关</button>
            <button type="button" class="settings-strategy-rename" data-group-id="${escapeAttr(group.id)}" title="修改组名与描述（成员、权重、层级都不受影响）">重命名</button>
            <button type="button" class="settings-strategy-delete" data-group-id="${escapeAttr(group.id)}" title="删除这个策略组（只删 groups.json 里的记录）">删除</button>
            <span class="settings-strategy-parent" title="上级分组只影响这里的展示层级，不会改变优先级（优先级由组权重与 Mod 分层决定）">
              <select class="settings-strategy-parent-select" data-group-id="${escapeAttr(group.id)}" aria-label="上级分组">
                ${parentOptionSummaryHtml(group)}
              </select>
            </span>
            <span class="settings-strategy-tier" title="组权重：叠加到组内成员的有效分层；数值越小越先加载。留空表示未设置，多个组的权重取最小值。">
              <input
                type="number"
                class="settings-strategy-tier-input"
                data-group-id="${escapeAttr(group.id)}"
                value="${group.tier === null || group.tier === undefined ? "" : escapeAttr(String(group.tier))}"
                placeholder="权重"
                aria-label="策略组权重"
              >
              <button type="button" class="settings-strategy-tier-save" data-group-id="${escapeAttr(group.id)}">保存权重</button>
            </span>
            <label class="settings-strategy-enforce" title="开启后，手动开关组内成员时会按策略自动联动其它成员">
              <input type="checkbox" class="settings-strategy-enforce-toggle" data-group-id="${escapeAttr(group.id)}" ${group.enforce ? "checked" : ""}>
              <span>自动联动</span>
            </label>
          </div>
          <div class="strategy-group-members" data-members-for="${escapeAttr(group.id)}"${isExpanded ? "" : " hidden"}></div>
        </div>
      `;
    })
    .join("");
  renderExpandedMembers();
  bindRowActions();
  // 重画后补回定位高亮（不滚动：用户可能已经自己滚到别处去了）。
  applyFocusHighlight();
  // 成员批量条：只在有组展开时出现；计数与按钮可用性跟着勾选走。
  updateMemberBatchBar();
}

/**
 * renderExpandedMembers 给"展开成员"的组填充成员行。
 *
 * 成员行与「分组建议」卡片共用 mod-groups/member-row.js：详情 / 游戏开关 /
 * 启用·禁用 / 复制到 addons 的语义与禁用条件完全一致，另加一个「移出本组」。
 */
function renderExpandedMembers() {
  const list = element("strategy-group-list");
  if (!list || !managerState) return;
  const expanded = expandedSet();
  if (expanded.size === 0) return;
  const files = appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
  const fileIndex = buildSuggestionFileIndex(files, appState.currentDirectory);

  expanded.forEach((groupId) => {
    const host = list.querySelector(
      `.strategy-group-members[data-members-for="${CSS.escape(String(groupId))}"]`,
    );
    const group = groupById(groupId);
    if (!host || !group) return;
    host.replaceChildren();
    const members = Array.isArray(group.members) ? group.members : [];
    if (members.length === 0) {
      const empty = document.createElement("div");
      empty.className = "setting-row-desc";
      empty.textContent = "这个组还没有成员：先在 Mod 管理页勾选 Mod，再用「分组 → ＋ 选中的 N 个」加入。";
      host.appendChild(empty);
      return;
    }

    // 「本组全选」：把"范围"做成每组一个控件。
    // 有了它，「全选成员」（作用范围 = 所有展开的组）就不会再被误当成"只选这一组"，
    // 用户想只处理某一组时直接点它，范围写在按钮上。
    const header = document.createElement("div");
    header.className = "strategy-group-members-header";
    const pickAllLabel = document.createElement("label");
    pickAllLabel.className = "strategy-group-members-pick-all";
    pickAllLabel.title = `只勾选/取消「${String(group.name || "")}」这一组的 ${members.length} 个成员，不影响别的组`;
    const pickAllInput = document.createElement("input");
    pickAllInput.type = "checkbox";
    pickAllInput.dataset.groupMemberPickAll = String(group.id);
    pickAllInput.addEventListener("change", (event) => {
      const keys = groupMemberKeys(group).map((key) => normalizeGroupKey(key));
      memberSelection = applyMemberScopeToggle(memberSelection, keys, Boolean(event.target.checked));
      syncMemberCheckboxes();
      updateMemberBatchBar();
    });
    const pickAllText = document.createElement("span");
    pickAllText.textContent = "本组全选";
    pickAllLabel.append(pickAllInput, pickAllText);
    const countText = document.createElement("span");
    countText.className = "strategy-group-members-count";
    countText.textContent = `本组 ${members.length} 个成员`;
    header.append(pickAllLabel, countText);
    host.appendChild(header);

    members.forEach((member) => {
      const key = String(member?.key ?? member ?? "").trim();
      if (!key) return;
      const normalizedKey = normalizeGroupKey(key);
      const file = fileIndex.get(normalizedKey) || null;
      const planEntry = appState.priorityPlanMap?.get(normalizedKey) || null;
      host.appendChild(
        buildModMemberRow({
          row: { key, name: String(member?.name || key) },
          file,
          priorityLabel: planEntry ? formatPriorityLabel(planEntry) : "",
          // 成员勾选框：勾上后用上面的批量条一次处理多个成员。
          // 与「分组建议」共用同一行实现，视觉与语义保持一致。
          pick: {
            checked: memberSelection.has(normalizedKey),
          title: "勾选这个 Mod，用窗口底部的成员批量按钮一次处理多个（游戏开关 / 启用·禁用 / 复制到 addons / 移出本组）",
            onChange: (checked) => {
              if (checked) memberSelection.add(normalizedKey);
              else memberSelection.delete(normalizedKey);
              updateMemberBatchBar();
            },
          },
          extraActions: [
            {
              text: "移出本组",
              className: "settings-strategy-member-remove",
              title: "把这个 Mod 移出当前策略组（只写 groups.json；组至少要保留 1 个成员）",
              onClick: () => RemoveModStrategyGroupMembers(group.id, [key]),
            },
          ],
          extraClass: "strategy-group-member-row",
          onActionDone: async () => {
            await reload();
            await refreshAfterChange();
          },
          onError: (message) => setStatus(message),
        }),
      );
    });
  });
}

function groupById(id) {
  return (managerState?.groups || []).find((group) => String(group.id) === String(id));
}

async function reload({ keepStatus = false } = {}) {
  managerState = await loadManagerData();
  renderManager();
  if (!keepStatus && !managerState.error) setStatus("");
}

async function applyGroup(button, id, options) {
  if (!id) return;
  button.disabled = true;
  try {
    // 应用前预检（对齐 FireAxe 的 CheckEnableStrategy）：先问后端"这个策略现在能不能满足"。
    let check = null;
    try {
      check = await CheckModStrategyGroupApply(id, {
        strategy: options?.strategy || "",
        pickKey: options?.pickKey || "",
      });
    } catch (error) {
      console.warn("策略预检失败，按原流程继续:", error);
    }
    if (check && check.applicable === false) {
      setStatus(check.reason || "这个策略现在无法执行");
      showNotification(check.reason || "这个策略现在无法执行", "error");
      button.disabled = false;
      return;
    }
    const warnings = check?.warnings || [];
    const run = async () => {
      try {
        const result = await ApplyModStrategyGroup(id, options || {});
        showNotification(formatStrategyGroupApplySummary(result), "success");
        await reload();
        await refreshAfterChange();
      } catch (error) {
        setStatus("应用策略组失败: " + String(error?.message || error));
        button.disabled = false;
      }
    };
    if (warnings.length > 0) {
      // 能执行但有需要注意的成员（缺失 / disabled 目录 / 单选没有空间）→ 让用户确认。
      showConfirmModal(
        "应用前检查",
        `策略组「${check.groupName}」现在可以执行，但有几点要确认：\n· ` +
          warnings.join("\n· ") +
          "\n\n是否继续应用？",
        () => void run(),
        false,
        "",
        () => {
          button.disabled = false;
        },
      );
      return;
    }
    await run();
  } catch (error) {
    setStatus("应用策略组失败: " + String(error?.message || error));
    button.disabled = false;
  }
}

async function runBatchAction(action, tier) {
  const summary = currentSummary();
  if (summary.count === 0) {
    setStatus("请先勾选要批量操作的策略组");
    return;
  }
  showConfirmModal("批量管理策略组", formatStrategyGroupBatchConfirm(action, summary), async () => {
    try {
      const result = await BatchUpdateModStrategyGroups([...selectionSet()], action, tier ?? null);
      if (action === "delete") selectionSet().clear();
      showNotification(formatStrategyGroupBatchResult(action, result), "success");
      await reload();
      await refreshAfterChange();
    } catch (error) {
      setStatus("批量操作失败: " + String(error?.message || error));
    }
  });
}

// ── 成员级批量动作 ─────────────────────────────────────────────────────────
// 语义与成员行右侧的单行按钮完全一致，只是把手动逐个点变成一次处理全部勾选：
//   游戏内开启/关闭 → addonlist.txt 的 0/1；
//   批量启用/禁用   → 在 addons 与 disabled 之间搬文件；
//   复制到 addons   → 工坊成员转正（原件保留并关闭）；
//   移出本组        → 只写 groups.json。

async function runMemberBatchGameState(enabled) {
  const targets = currentMemberTargets();
  if (targets.game.length === 0) {
    showNotification("勾选的成员都不能改游戏开关（都在 disabled 目录里）", "info");
    return;
  }
  showConfirmModal(
    enabled ? "批量游戏内启用" : "批量游戏内关闭",
    formatBatchGameStateConfirm(
      {
        total: targets.game.length,
        skipped: targets.gameSkipped.length,
        unrecorded: targets.gameUnrecorded.length,
      },
      enabled,
    ),
    async () => {
      try {
        const result = await SetVPKGameEnabledBatch(targets.game, enabled);
        await reload();
        await refreshAfterChange();
        const updated = (result?.updated || []).length;
        showNotification(
          formatBatchGameStateSummary(result, enabled),
          updated > 0 ? "success" : "info",
        );
      } catch (error) {
        showError("批量设置游戏内开关失败: " + String(error?.message || error));
      }
    },
  );
}

async function runMemberBatchFileToggle(kind) {
  const targets = currentMemberTargets();
  const paths = kind === "enable" ? targets.enable : targets.disable;
  const label = kind === "enable" ? "启用" : "禁用";
  if (paths.length === 0) {
    showNotification(
      kind === "enable"
        ? "没有可启用的成员：只有 disabled 目录里的 Mod 需要搬回 addons"
        : "没有可禁用的成员：只有 addons 根目录里的 Mod 能搬进 disabled",
      "info",
    );
    return;
  }
  // 与主列表的批量启用 / 禁用同一条风险提示（VPK 完整性问题不阻断，但要先说清）。
  if (!(await confirmVPKOperationWarning(paths, `批量${label}策略组成员`))) return;
  const failures = [];
  const results = await Promise.all(
    paths.map(async (path) => {
      try {
        await ToggleVPKFile(path);
        return true;
      } catch (error) {
        failures.push(String(error?.message || error));
        console.error(`批量${label}失败:`, path, error);
        return false;
      }
    }),
  );
  const succeeded = results.filter(Boolean).length;
  await reload();
  await refreshAfterChange();
  showNotification(
    formatMemberBatchResult({
      label,
      succeeded,
      failed: paths.length - succeeded,
      firstError: failures[0] || "",
    }),
    succeeded > 0 ? "success" : "error",
  );
}

async function runMemberBatchTransfer() {
  const targets = currentMemberTargets();
  if (targets.transfer.length === 0) {
    showNotification("没有可复制的成员：只有创意工坊里的 Mod 需要复制到 addons", "info");
    return;
  }
  // 复用主列表的批量转移（一次确认 + 进度事件 + 冲突处理），避免这里再写一套。
  await moveWorkshopFilesToAddons(targets.transfer);
  await reload();
  await refreshAfterChange();
}

async function runMemberBatchRemove() {
  const plan = planMemberRemoval([...memberSelection], managerState?.groups || []);
  if (plan.length === 0) {
    showNotification("请先勾选要移出的成员", "info");
    return;
  }
  const totalKeys = plan.reduce((sum, item) => sum + item.keys.length, 0);
  const emptying = plan.filter((item) => item.wouldEmpty);
  const lines = [
    `把勾选的 ${totalKeys} 个成员从所属策略组移出？`,
    "· 只写 groups.json，不移动、不删除任何 Mod 文件",
  ];
  if (emptying.length > 0) {
    lines.push(
      `· 「${emptying.map((item) => item.groupName).join("」「")}」会被清空，后端会拒绝（每个组至少要保留 1 个成员）`,
    );
  }
  showConfirmModal("批量移出成员", lines.join("\n"), async () => {
    let succeeded = 0;
    const failures = [];
    for (const item of plan) {
      try {
        await RemoveModStrategyGroupMembers(item.groupId, item.keys);
        succeeded += item.keys.length;
      } catch (error) {
        failures.push(`${item.groupName}: ${String(error?.message || error)}`);
      }
    }
    memberSelection.clear();
    await reload();
    await refreshAfterChange();
    if (failures.length > 0) {
      showError(`有 ${failures.length} 个组没有移出成功：${failures[0]}`);
    }
    if (succeeded > 0) showNotification(`已从策略组移出 ${succeeded} 个成员`, "success");
  });
}

/** 重新绑定当前 DOM 上的动作按钮（每次重画后调用）。 */
function bindRowActions() {
  const root = element("strategy-group-list");
  if (!root) return;

  root.querySelectorAll(".settings-strategy-pick-input").forEach((input) => {
    input.addEventListener("change", () => {
      const id = String(input.dataset.groupId || "");
      if (!id) return;
      if (input.checked) selectionSet().add(id);
      else selectionSet().delete(id);
      syncSelectAll();
      const selectionLabel = element("strategy-group-selection");
      if (selectionLabel) selectionLabel.textContent = formatStrategyGroupSelectionLabel(currentSummary());
      const hasSelection = currentSummary().count > 0;
      [
        "strategy-group-batch-delete",
        "strategy-group-batch-enforce-on",
        "strategy-group-batch-enforce-off",
        "strategy-group-batch-tier-save",
        "strategy-group-batch-tier-clear",
        "strategy-group-batch-filter",
      ].forEach((id2) => {
        const button = element(id2);
        if (button) button.disabled = !hasSelection;
      });
    });
  });

  root.querySelectorAll(".settings-strategy-apply").forEach((button) => {
    button.addEventListener("click", () => void applyGroup(button, button.dataset.groupId, {}));
  });
  // 展开 / 收起成员明细：展开后可以直接操作单个 Mod（详情 / 游戏开关 / 启用禁用 / 移出本组）。
  root.querySelectorAll(".settings-strategy-expand").forEach((button) => {
    button.addEventListener("click", () => {
      const id = String(button.dataset.groupId || "");
      if (!id) return;
      const expanded = expandedSet();
      if (expanded.has(id)) expanded.delete(id);
      else expanded.add(id);
      renderManager();
    });
  });
  // 「筛选这组」：把这一组变成主界面的分组筛选（再点一次取消这组）。
  root.querySelectorAll(".settings-strategy-filter").forEach((button) => {
    button.addEventListener("click", () => {
      const id = String(button.dataset.groupId || "");
      if (!id) return;
      const next = new Set(activeFilterSet());
      if (next.has(id)) next.delete(id);
      else next.add(id);
      void applyGroupFilterIds([...next]);
    });
  });
  // 「＋ 子组」：在这个组下面直接建子组（Mod 管理页勾选的 Mod 会一起放进去）。
  root.querySelectorAll(".settings-strategy-add-child").forEach((button) => {
    button.addEventListener("click", () => {
      const parentId = String(button.dataset.groupId || "");
      if (!parentId) return;
      const parentName = String(button.dataset.groupName || "");
      const selected = [...(appState.selectedFiles || [])];
      showPromptModal(
        "新建子组",
        `在「${parentName}」下面新建一个子组。\n` +
          (selected.length > 0
            ? `· 当前在 Mod 管理页勾选的 ${selected.length} 个 Mod 会一起放进这个子组\n`
            : "· 现在没有勾选 Mod，会先建一个空子组（之后可用「加入策略组…」把 Mod 放进去）\n") +
          "· 子组的有效分层会累加它自己和全部上级分组的权重（最多 4 层）",
        {
          defaultValue: "",
          placeholder: "子组名称，例如 上衣关 / 材质包",
          confirmText: "创建子组",
          onConfirm: async (value) => {
            const name = String(value || "").trim();
            if (!name) {
              setStatus("子组名称不能为空");
              return;
            }
            try {
              const created = await CreateModStrategyGroupChild(parentId, name, "single", selected);
              // 后端只会收下能解析成 addonlist 键的成员：选择里混进失效路径时会被静默丢掉。
              // 所以这里必须报"实际入组数"，否则提示会拿勾选数骗人（真机联调时踩到过）。
              const added = Array.isArray(created?.members) ? created.members.length : selected.length;
              const dropped = Math.max(0, selected.length - added);
              const groupLabel = `已在「${parentName}」下创建子组「${created?.name || name}」`;
              showNotification(
                dropped > 0
                  ? `${groupLabel}（加入 ${added} 个 Mod，另有 ${dropped} 个无法加入）`
                  : added > 0
                    ? `${groupLabel}（含 ${added} 个 Mod）`
                    : groupLabel,
                "success",
              );
              await reload();
              await refreshAfterChange();
            } catch (error) {
              setStatus("新建子组失败: " + String(error?.message || error));
            }
          },
        },
      );
    });
  });
  root.querySelectorAll(".settings-strategy-random").forEach((button) => {
    button.addEventListener("click", () =>
      void applyGroup(button, button.dataset.groupId, { strategy: "single_random" }),
    );
  });
  root.querySelectorAll(".settings-strategy-off").forEach((button) => {
    button.addEventListener("click", () => void applyGroup(button, button.dataset.groupId, { strategy: "off" }));
  });

  root.querySelectorAll(".settings-strategy-rename").forEach((button) => {
    button.addEventListener("click", () => {
      const id = button.dataset.groupId;
      const group = groupById(id);
      if (!id) return;
      showPromptModal(
        "重命名策略组",
        `修改「${group?.name || id}」的名称；描述会一起保存。只写 groups.json 里的这一条记录。`,
        {
          defaultValue: group?.name || "",
          placeholder: "策略组名称",
          confirmText: "保存",
          onConfirm: async (value) => {
            const name = String(value || "").trim();
            if (!name) {
              setStatus("策略组名称不能为空");
              return;
            }
            try {
              await RenameModStrategyGroup(id, name, group?.description || "");
              showNotification(`已重命名为「${name}」`, "success");
              await reload();
              // 组名会出现在「按分组筛选」菜单里，改完要一起刷新。
              await refreshAfterChange();
            } catch (error) {
              setStatus("重命名策略组失败: " + String(error?.message || error));
            }
          },
        },
      );
    });
  });

  root.querySelectorAll(".settings-strategy-delete").forEach((button) => {
    button.addEventListener("click", () => {
      const id = button.dataset.groupId;
      if (!id) return;
      const groups = managerState?.groups || [];
      const group = groups.find((item) => String(item.id) === String(id));
      const childCount = groups.filter((item) => String(item.parentId || "") === String(id)).length;
      showConfirmModal(
        "删除策略组",
        `删除「${group?.name || id}」：\n` +
          `· 只删除 groups.json 里的这一条记录（${(group?.members || []).length} 个成员）\n` +
          "· 不会改动 addonlist.txt，也不会删除任何 Mod 文件\n" +
          "· 组成员已经写入的 Mod 分层（priority.json）会保留\n" +
          (childCount > 0 ? `· 它的 ${childCount} 个下级分组会回到顶层，不会被一起删除\n` : "") +
          "是否继续？",
        async () => {
          button.disabled = true;
          try {
            await DeleteModStrategyGroup(id);
            selectionSet().delete(String(id));
            showNotification("策略组已删除", "success");
            await reload();
            await refreshAfterChange();
          } catch (error) {
            setStatus("删除策略组失败: " + String(error?.message || error));
            button.disabled = false;
          }
        },
      );
    });
  });

  root.querySelectorAll(".settings-strategy-enforce-toggle").forEach((input) => {
    input.addEventListener("change", async () => {
      const id = input.dataset.groupId;
      if (!id) return;
      input.disabled = true;
      try {
        await SetModStrategyGroupEnforcement(id, input.checked);
        showNotification(
          input.checked ? "已开启策略组自动联动" : "已关闭策略组自动联动",
          "success",
        );
        await reload();
        await refreshAfterChange();
      } catch (error) {
        input.checked = !input.checked;
        setStatus("保存自动联动设置失败: " + String(error?.message || error));
      } finally {
        input.disabled = false;
      }
    });
  });

  root.querySelectorAll(".settings-strategy-tier-save").forEach((button) => {
    button.addEventListener("click", async () => {
      const id = button.dataset.groupId;
      if (!id) return;
      const input = root.querySelector(
        `.settings-strategy-tier-input[data-group-id="${CSS.escape(String(id))}"]`,
      );
      const raw = String(input?.value ?? "").trim();
      const tier = normalizePriorityTier(raw);
      if (raw !== "" && tier === null) {
        setStatus("组权重必须是整数（可为负数；数值越小越先加载）");
        return;
      }
      button.disabled = true;
      try {
        await SetModStrategyGroupTier(id, tier);
        showNotification(
          tier === null
            ? "已清除策略组权重（不会自动还原原顺序；要还原请用「历史备份」里的 addonlist.txt 备份）"
            : `已保存策略组权重 ${tier}（需按分层应用才会重排）`,
          "success",
        );
        await reload();
        // 组权重直接决定「按分组筛选」的顺序与列表里的优先级角标，必须一起刷新
        // （以前这里只 reload 了本窗口：外面菜单要手动点"刷新"才更新）。
        await refreshAfterChange();
      } catch (error) {
        setStatus("保存策略组权重失败: " + String(error?.message || error));
        button.disabled = false;
      }
    });
  });

  root.querySelectorAll(".settings-strategy-parent-select").forEach((select) => {
    select.addEventListener("change", async () => {
      const id = select.dataset.groupId;
      if (!id) return;
      select.disabled = true;
      try {
        await MoveModStrategyGroup(id, select.value || "");
        showNotification(select.value ? "已设置上级分组" : "已移动到顶层", "success");
        await reload();
        // 层级会改变筛选菜单里的分组归属与缩进，改完要一起刷新。
        await refreshAfterChange();
      } catch (error) {
        setStatus("设置上级分组失败: " + String(error?.message || error));
        select.disabled = false;
        await reload({ keepStatus: true });
      }
    });
  });

  bindGroupDragRows(root);
}

// bindGroupDropRoot 给窗口底部的"变成顶层组"落点绑定一次事件（静态元素，不能随重画重复绑定）。
function bindGroupDropRoot() {
  const dropRoot = element("strategy-group-drop-root");
  if (!dropRoot || dropRoot.dataset.bound === "1") return;
  dropRoot.dataset.bound = "1";

  dropRoot.addEventListener("dragover", (event) => {
    if (!draggingGroupId) return;
    if (!resolveStrategyGroupDrop(managerState?.rows || [], draggingGroupId, "", "root").ok) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
    dropRoot.classList.add("is-drop-active");
  });

  dropRoot.addEventListener("dragleave", () => dropRoot.classList.remove("is-drop-active"));

  dropRoot.addEventListener("drop", async (event) => {
    const movingId = draggingGroupId;
    if (!movingId) return;
    event.preventDefault();
    dropRoot.classList.remove("is-drop-active");
    await applyStrategyGroupDrop(
      movingId,
      "",
      "root",
      resolveStrategyGroupDrop(managerState?.rows || [], movingId, "", "root"),
    );
  });
}

// 拖动策略组的落点判定全部交给 strategy-group-tree.mjs 的纯函数（node --test 覆盖）：
//   - 拖到某一行中间 → 变成它的子组（换上级）
//   - 拖到某一行的上 / 下边缘 → 排到它前 / 后（同级排序）
//   - 拖到列表空白处 → 回到顶层
function bindGroupDragRows(root) {
  const rowsForDrag = () => managerState?.rows || [];
  const dropClasses = ["is-drop-inside", "is-drop-before", "is-drop-after"];

  const clearDropMarks = () => {
    root.querySelectorAll(`.${dropClasses.join(", .")}`).forEach((row) => {
      dropClasses.forEach((name) => row.classList.remove(name));
    });
  };

  const positionFor = (row, event) => {
    const rect = row.getBoundingClientRect();
    const offset = rect.height > 0 ? (event.clientY - rect.top) / rect.height : 0.5;
    if (offset < 0.28) return "before";
    if (offset > 0.72) return "after";
    return "inside";
  };

  root.querySelectorAll("[data-group-row]").forEach((row) => {
    row.addEventListener("dragstart", (event) => {
      draggingGroupId = String(row.dataset.groupRow || "");
      if (!draggingGroupId) {
        event.preventDefault();
        return;
      }
      if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = "move";
        event.dataTransfer.setData("text/plain", draggingGroupId);
      }
      row.classList.add("is-dragging");
    });

    row.addEventListener("dragend", () => {
      row.classList.remove("is-dragging");
      clearDropMarks();
      root.classList.remove("is-drop-root");
      draggingGroupId = "";
    });

    row.addEventListener("dragover", (event) => {
      if (!draggingGroupId) return;
      const position = positionFor(row, event);
      const plan = resolveStrategyGroupDrop(
        rowsForDrag(),
        draggingGroupId,
        row.dataset.groupRow,
        position,
      );
      if (!plan.ok) {
        if (event.dataTransfer) event.dataTransfer.dropEffect = "none";
        return;
      }
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
      clearDropMarks();
      row.classList.add(`is-drop-${position}`);
    });

    row.addEventListener("dragleave", () => {
      dropClasses.forEach((name) => row.classList.remove(name));
    });

    row.addEventListener("drop", async (event) => {
      const movingId = draggingGroupId;
      if (!movingId) return;
      event.preventDefault();
      event.stopPropagation();
      const position = positionFor(row, event);
      const targetId = String(row.dataset.groupRow || "");
      clearDropMarks();
      await applyStrategyGroupDrop(
        movingId,
        targetId,
        position,
        resolveStrategyGroupDrop(rowsForDrag(), movingId, targetId, position),
      );
    });
  });

  // 列表空白处（或最后一行下方）＝ 回到顶层。
  root.addEventListener("dragover", (event) => {
    if (!draggingGroupId || event.target.closest?.("[data-group-row]")) return;
    if (!resolveStrategyGroupDrop(rowsForDrag(), draggingGroupId, "", "root").ok) return;
    event.preventDefault();
    root.classList.add("is-drop-root");
  });

  root.addEventListener("dragleave", (event) => {
    if (event.target === root) root.classList.remove("is-drop-root");
  });

  root.addEventListener("drop", async (event) => {
    const movingId = draggingGroupId;
    if (!movingId || event.target.closest?.("[data-group-row]")) return;
    event.preventDefault();
    root.classList.remove("is-drop-root");
    await applyStrategyGroupDrop(
      movingId,
      "",
      "root",
      resolveStrategyGroupDrop(rowsForDrag(), movingId, "", "root"),
    );
  });
}

// applyStrategyGroupDrop 把一次合法的拖放落到后端：先改上级（必要时），再写同级顺序。
// 排序与上级是两个独立接口，两步都写完再刷新一次界面。
async function applyStrategyGroupDrop(movingId, targetId, position, plan) {
  if (!plan || !plan.ok) {
    if (plan?.reason) showNotification(plan.reason, "error");
    return;
  }
  if (plan.noop) return;

  const rows = managerState?.rows || [];
  const order = applyStrategyGroupDropOrder(rows, movingId, targetId, position);
  try {
    await MoveModStrategyGroup(movingId, plan.parentId || "");
    if (order && order.length > 0) {
      await ReorderModStrategyGroups(order);
    }
    showNotification(formatStrategyGroupDropMessage(rows, movingId, targetId, position), "success");
    await reload();
    await refreshAfterChange();
  } catch (error) {
    setStatus("拖动策略组失败: " + String(error?.message || error));
    await reload({ keepStatus: true });
  }
}

async function captureGroupFromSelection() {
  const name = String(element("strategy-group-name")?.value || "").trim();
  if (!name) {
    setStatus("请先填写策略组名称");
    element("strategy-group-name")?.focus();
    return;
  }
  const members = buildGroupMembersFromSelection([...(appState.selectedFiles || [])]);
  if (members.length === 0) {
    setStatus("请先在 Mod 管理页选中要归入策略组的 Mod");
    return;
  }
  const button = element("strategy-group-capture");
  if (button) button.disabled = true;
  try {
    await CaptureModStrategyGroup(
      name,
      "",
      element("strategy-group-strategy")?.value || "single",
      members,
    );
    const input = element("strategy-group-name");
    if (input) input.value = "";
    showNotification(`已保存策略组「${name}」`, "success");
    await reload();
    await refreshAfterChange();
  } catch (error) {
    setStatus("保存策略组失败: " + String(error?.message || error));
  } finally {
    if (button) button.disabled = false;
  }
}

/**
 * openStrategyGroupManager 打开独立的策略组管理窗口。
 *
 * @param {{focusGroupId?: string, focusFilePath?: string}} [options]
 *        从 Mod 列表的「组：xxx」徽标点进来时传：窗口顶部会列出这个 Mod 所属的
 *        全部策略组，并展开 / 高亮 focusGroupId 这一组。不传就是普通打开。
 */
export async function openStrategyGroupManager(options = {}) {
  const modal = element("strategy-group-modal");
  if (!modal) return;
  modal.classList.remove("hidden");
  // 每次打开都按当前偏好重新落位（偏好由 core/floating-modal.js 统一管理）。
  getFloatingModal("strategy-group-modal")?.refresh();
  // 成员批量选择不跨窗口生命周期保留：重新打开就是一次新的批量操作。
  memberSelection = new Set();
  managerState = await loadManagerData();
  setFocusRequest(options);
  renderManager();
  revealFocusedGroup();
  updateCaptureButton();
}

function closeStrategyGroupManager() {
  // 定位只在这一次打开里有效：下次（比如从设置页）打开是普通的组列表。
  focusState = null;
  element("strategy-group-modal")?.classList.add("hidden");
}

/** updateCaptureButton 让"用选中的 N 个 Mod 建组"跟随当前选择。 */
export function updateCaptureButton() {
  const button = element("strategy-group-capture");
  if (!button) return;
  const count = appState.selectedFiles?.size || 0;
  button.disabled = count === 0;
  // 禁用时把"为什么不能点"写在按钮上，避免看起来像坏掉的按钮。
  button.textContent = count > 0 ? `用选中的 ${count} 个 Mod 建组` : "先在 Mod 管理页勾选 Mod";
  button.title =
    count > 0
      ? `把 Mod 管理页当前勾选的 ${count} 个 Mod 建成一个策略组（名称见左边的输入框）`
      : "还没勾选 Mod：先回到 Mod 管理页勾选要归入同一组的 Mod，这个按钮就会亮起来";
}

/** refreshStrategyGroupManagerIfOpen 组归属变化时，窗口开着就同步刷新。 */
export async function refreshStrategyGroupManagerIfOpen() {
  const modal = element("strategy-group-modal");
  if (!modal || modal.classList.contains("hidden")) return;
  await reload();
}

let managerBound = false;

/** initStrategyGroupManager 绑定窗口按钮（幂等）。 */
export function initStrategyGroupManager() {
  if (managerBound) return;
  managerBound = true;
  // 勾选变化：窗口里的「用选中的 N 个 Mod 建组」要实时跟上（清空 / 框选 / 刷新筛选都会变）。
  onFileSelectionChanged(() => updateCaptureButton());
  // 组归属变化（别的入口增删成员 / 改名 / 删除）：窗口开着就同步重画，避免看到过期列表。
  onModGroupMembershipChanged(() => void refreshStrategyGroupManagerIfOpen());
  element("strategy-group-capture")?.addEventListener("click", () => void captureGroupFromSelection());
  element("strategy-group-close-btn")?.addEventListener("click", closeStrategyGroupManager);
  element("strategy-group-close-footer-btn")?.addEventListener("click", closeStrategyGroupManager);
  // 聚焦条（从 Mod 列表徽标点进来时出现）：条本身是静态元素，重画只换里面的内容，
  // 所以这里绑一次委托。点组名＝展开并跳过去；「整组开关」与列表「分组」菜单同语义。
  element("strategy-group-focus")?.addEventListener("click", (event) => {
    const chip = event.target.closest?.("[data-focus-group-id]");
    if (chip) {
      event.preventDefault();
      focusOnGroup(chip.dataset.focusGroupId);
      return;
    }
    const action = event.target.closest?.("[data-focus-action]")?.dataset?.focusAction;
    if (action === "toggle-group") {
      event.preventDefault();
      void toggleFocusedGroupEnabled();
      return;
    }
    if (action === "clear") {
      event.preventDefault();
      focusState = null;
      renderManager();
    }
  });
  // 「上级分组」下拉的候选惰性填充：鼠标点开、键盘 Tab 进来、键盘直接操作，
  // 三种路径都先补候选再让浏览器打开原生下拉（见 fillParentOptions 的说明）。
  const groupListHost = element("strategy-group-list");
  groupListHost?.addEventListener("pointerdown", (event) => {
    const select = event.target?.closest?.(".settings-strategy-parent-select");
    if (select) fillParentOptions(select);
  });
  groupListHost?.addEventListener("focusin", (event) => {
    const select = event.target?.closest?.(".settings-strategy-parent-select");
    if (select) fillParentOptions(select);
  });
  groupListHost?.addEventListener("keydown", (event) => {
    const select = event.target?.closest?.(".settings-strategy-parent-select");
    if (select) fillParentOptions(select);
  });
  // 成员批量条：元素是静态的（index.html），这里绑定一次；
  // 计数与可用性由 updateMemberBatchBar() 在每次重画/勾选后刷新。
  element("strategy-group-member-select-all")?.addEventListener("change", (event) => {
    // 作用范围 = 当前展开的所有组。范围外的勾选原样保留（可以跨组批量），
    // 但条上的计数会把"未展开组里还有多少个"说出来，不再是看不见的勾选。
    memberSelection = applyMemberScopeToggle(
      memberSelection,
      expandedMemberKeys(),
      Boolean(event.target?.checked),
    );
    syncMemberCheckboxes();
    updateMemberBatchBar();
  });
  // 「全选所有组」= 一次勾上全部策略组的成员（切换式按钮：再点一次取消）。
  element("strategy-group-member-select-all-groups")?.addEventListener("click", () => {
    const keys = allMemberKeys();
    const allSelected = keys.length > 0 && keys.every((key) => memberSelection.has(key));
    memberSelection = applyMemberScopeToggle(memberSelection, keys, !allSelected);
    syncMemberCheckboxes();
    updateMemberBatchBar();
  });
  // 「取消全部勾选」= 一次清空，包括折叠组里那些看不见的。
  element("strategy-group-member-clear-all")?.addEventListener("click", () => {
    memberSelection = new Set();
    syncMemberCheckboxes();
    updateMemberBatchBar();
  });
  element("strategy-group-member-batch-game-on")?.addEventListener(
    "click",
    () => void runMemberBatchGameState(true),
  );
  element("strategy-group-member-batch-game-off")?.addEventListener(
    "click",
    () => void runMemberBatchGameState(false),
  );
  element("strategy-group-member-batch-enable")?.addEventListener(
    "click",
    () => void runMemberBatchFileToggle("enable"),
  );
  element("strategy-group-member-batch-disable")?.addEventListener(
    "click",
    () => void runMemberBatchFileToggle("disable"),
  );
  element("strategy-group-member-batch-transfer")?.addEventListener(
    "click",
    () => void runMemberBatchTransfer(),
  );
  element("strategy-group-member-batch-remove")?.addEventListener(
    "click",
    () => void runMemberBatchRemove(),
  );
  // 拖放排序：底部"变成顶层组"是静态元素，只在这里绑一次（列表内的行随重画绑定）。
  bindGroupDropRoot();
  // ESC 关闭窗口（只在窗口打开时拦截）；点击窗口外的遮罩同样关闭。
  document.addEventListener("keydown", (event) => {
    const modal = element("strategy-group-modal");
    if (!modal || modal.classList.contains("hidden")) return;
    if (event.key !== "Escape") return;
    event.stopPropagation();
    closeStrategyGroupManager();
  });
  // 浮动 / 拖动 / 位置记忆统一交给 core/floating-modal.js：
  // 开关是通用模块自动插到标题栏的「浮动窗口 / 停靠窗口」按钮（与其它管理窗口完全一致），
  // 偏好仍然写回 config.json 的 strategyGroupFloating。
  // 「点窗口外关闭」也由它统一提供（停靠状态走这个窗口的关闭按钮）。
  setupFloatingModal(element("strategy-group-modal"), {
    key: "strategy-group-modal",
    defaultFloating: getConfig().strategyGroupFloating !== false,
    read: () => {
      const stored = getConfig().strategyGroupFloating;
      return stored === undefined ? null : Boolean(stored);
    },
    write: (enabled) => {
      const config = getConfig();
      config.strategyGroupFloating = Boolean(enabled);
      saveConfig(config);
    },
    // 默认停靠在"工具栏下方 + 分组菜单右侧"：既不挡「分组 / 按分组筛选」，
    // 也不会压住第一行筛选菜单（菜单宽度约 420px、从 x≈230 开始）。
    defaultPosition: () => {
      const left = Math.max(660, Math.round(window.innerWidth * 0.47));
      const top = 172;
      return {
        left,
        top,
        width: Math.max(420, Math.min(1100, window.innerWidth - left - 24)),
        height: Math.max(320, Math.min(680, window.innerHeight - top - 24)),
      };
    },
  });
  element("strategy-group-select-all")?.addEventListener("change", (event) => {
    const selection = selectionSet();
    // 只作用于当前搜索出来的组：清空搜索时才等价于"全选所有组"。
    const ids = visibleGroupIds();
    if (event.target.checked) ids.forEach((id) => selection.add(id));
    else ids.forEach((id) => selection.delete(id));
    renderManager();
  });
  // 「取消全部勾选」：组勾选也补一个一次清空，包括被搜索筛掉、界面上看不见的那些。
  element("strategy-group-clear-all")?.addEventListener("click", () => {
    selectionSet().clear();
    renderManager();
  });
  // 搜索框：边打边筛（组数量是几十级，不需要防抖）。
  element("strategy-group-search")?.addEventListener("input", (event) => {
    managerQuery = String(event.target.value || "");
    renderManager();
  });
  element("strategy-group-search")?.addEventListener("keydown", (event) => {
    if (event.key !== "Escape" || !managerQuery) return;
    event.stopPropagation();
    managerQuery = "";
    renderManager();
  });
  element("strategy-group-batch-delete")?.addEventListener("click", () => void runBatchAction("delete"));
  element("strategy-group-batch-enforce-on")?.addEventListener("click", () => void runBatchAction("enforce_on"));
  element("strategy-group-batch-enforce-off")?.addEventListener("click", () => void runBatchAction("enforce_off"));
  element("strategy-group-batch-tier-save")?.addEventListener("click", () => {
    const raw = String(element("strategy-group-batch-tier-input")?.value ?? "").trim();
    const tier = normalizePriorityTier(raw);
    if (raw === "" || tier === null) {
      setStatus("批量设置权重需要一个整数（可为负数；越小越先加载）");
      return;
    }
    void runBatchAction("set_tier", tier);
  });
  element("strategy-group-batch-tier-clear")?.addEventListener("click", () => void runBatchAction("clear_tier"));
  // 批量筛选：把勾选的组一次变成主界面的分组筛选（只改视图，不写文件）。
  element("strategy-group-batch-filter")?.addEventListener("click", () => {
    const ids = [...selectionSet()];
    if (ids.length === 0) {
      setStatus("请先勾选要用作筛选的策略组");
      return;
    }
    void applyGroupFilterIds(ids);
  });
  element("strategy-group-batch-filter-clear")?.addEventListener("click", () => {
    void applyGroupFilterIds([]);
  });
}
