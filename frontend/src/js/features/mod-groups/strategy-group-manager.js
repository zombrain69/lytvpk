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
import { showNotification } from "../../core/toast.js";
import { getConfig, saveConfig } from "../../core/config.js";
import { getFloatingModal, setupFloatingModal } from "../../core/floating-modal.js";
import {
  ApplyModStrategyGroup,
  BatchUpdateModStrategyGroups,
  CaptureModStrategyGroup,
  DeleteModStrategyGroup,
  GetModStrategyGroupMissingMembers,
  ListModStrategyGroups,
  ListModStrategyGroupTree,
  MoveModStrategyGroup,
  RenameModStrategyGroup,
  RemoveModStrategyGroupMembers,
  SetModStrategyGroupEnforcement,
  SetModStrategyGroupTier,
} from "../../../../wailsjs/go/app/App";
import { renderFileList } from "../file-list/render.js";
import { refreshFilesKeepFilter } from "../file-list/filters.js";
import { formatPriorityLabel, normalizePriorityTier } from "../file-list/priority-label.mjs";
import { buildGroupMembersFromSelection } from "../settings/selection-args.mjs";
import { buildParentOptions, flattenStrategyGroupTree } from "../settings/strategy-group-tree.mjs";
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
  formatGroupMissingNotice,
  normalizeGroupKey,
} from "./group-view.mjs";
import { buildModMemberRow } from "./member-row.js";
import { onModGroupMembershipChanged } from "./group-state.mjs";
import { showConfirmModal } from "../modals/confirm.js";
import { showPromptModal } from "../modals/prompt.js";

let managerState = null;

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
  let groups = [];
  let error = "";
  try {
    groups = (await ListModStrategyGroups()) || [];
  } catch (err) {
    error = String(err?.message || err || "无法读取策略组");
  }
  let tree = null;
  try {
    tree = (await ListModStrategyGroupTree()) || null;
  } catch (err) {
    tree = null;
  }
  const missingByName = new Map();
  try {
    const missing = (await GetModStrategyGroupMissingMembers()) || [];
    missing.forEach((item) => {
      missingByName.set(String(item.groupId), item.missingNames || []);
    });
  } catch (err) {
    console.warn("读取缺失成员失败:", err);
  }
  return {
    groups,
    rows: flattenStrategyGroupTree(tree, groups),
    missingByName,
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
  const total = (managerState?.groups || []).length;
  const selected = selectionSet().size;
  selectAll.checked = total > 0 && selected >= total;
  selectAll.indeterminate = selected > 0 && selected < total;
}

/** 渲染批量工具条与组列表（整体重画，动作完成后调用）。 */
function renderManager() {
  if (!managerState) return;
  pruneSelection();
  pruneExpanded();
  syncSelectAll();
  const list = element("strategy-group-list");
  const batch = element("strategy-group-batch");
  const status = element("strategy-group-status");
  if (status && managerState.error) status.textContent = managerState.error;
  const rows = managerState.rows || [];

  if (batch) {
    batch.classList.toggle("hidden", rows.length === 0);
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
    ].forEach((id) => {
      const button = element(id);
      if (button) button.disabled = !hasSelection;
    });
  }
  if (!list) return;
  if (rows.length === 0) {
    list.innerHTML =
      `<div class="setting-row-desc">还没有策略组。在 Mod 管理页勾选几个 Mod，再用上面的「用选中的 N 个 Mod 建组」创建。</div>`;
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
      const isExpanded = expanded.has(String(group.id));
      const memberCount = (group.members || []).length;
      const isFiltered = activeFilter.has(String(group.id));
      return `
        <div class="settings-profile-item${isFiltered ? " is-filtered" : ""}" data-group-row="${escapeAttr(group.id)}" style="margin-left: ${Math.max(depth - 1, 0) * 1.25}rem">
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
          </div>
          <div class="settings-profile-actions">
            <button type="button" class="settings-strategy-expand" data-group-id="${escapeAttr(group.id)}" title="展开成员后可以直接查看详情、改游戏开关、启用/禁用、复制到 addons、把单个 Mod 移出本组">${isExpanded ? "收起成员" : `展开成员（${memberCount}）`}</button>
            <button type="button" class="settings-strategy-filter${isFiltered ? " is-active" : ""}" data-group-id="${escapeAttr(group.id)}" title="只让主界面显示属于这个组的 Mod（等同于「按分组筛选」勾上这个组；再点一次取消）">${isFiltered ? "取消筛选" : "筛选这组"}</button>
            <button type="button" class="settings-strategy-apply" data-group-id="${escapeAttr(group.id)}">按策略应用</button>
            <button type="button" class="settings-strategy-random" data-group-id="${escapeAttr(group.id)}">随机单选</button>
            <button type="button" class="settings-strategy-off" data-group-id="${escapeAttr(group.id)}">全关</button>
            <button type="button" class="settings-strategy-rename" data-group-id="${escapeAttr(group.id)}" title="修改组名与描述（成员、权重、层级都不受影响）">重命名</button>
            <button type="button" class="settings-strategy-delete" data-group-id="${escapeAttr(group.id)}" title="删除这个策略组（只删 groups.json 里的记录）">删除</button>
            <span class="settings-strategy-parent" title="上级分组只影响这里的展示层级，不会改变优先级（优先级由组权重与 Mod 分层决定）">
              <select class="settings-strategy-parent-select" data-group-id="${escapeAttr(group.id)}" aria-label="上级分组">
                <option value="">（顶层）</option>
                ${buildParentOptions(managerState.rows, group.id)
                  .map(
                    (option) =>
                      `<option value="${escapeAttr(option.id)}" ${option.id === group.parentId ? "selected" : ""}>${escapeHtml(option.label)}</option>`,
                  )
                  .join("")}
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
    const result = await ApplyModStrategyGroup(id, options || {});
    showNotification(formatStrategyGroupApplySummary(result), "success");
    await reload();
    await refreshAfterChange();
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
          tier === null ? "已清除策略组权重" : `已保存策略组权重 ${tier}（需按分层应用才会重排）`,
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

/** openStrategyGroupManager 打开独立的策略组管理窗口。 */
export async function openStrategyGroupManager() {
  const modal = element("strategy-group-modal");
  if (!modal) return;
  modal.classList.remove("hidden");
  // 每次打开都按当前偏好重新落位（偏好由 core/floating-modal.js 统一管理）。
  getFloatingModal("strategy-group-modal")?.refresh();
  managerState = await loadManagerData();
  renderManager();
  updateCaptureButton();
}

function closeStrategyGroupManager() {
  element("strategy-group-modal")?.classList.add("hidden");
}

/** updateCaptureButton 让"用选中的 N 个 Mod 建组"跟随当前选择。 */
export function updateCaptureButton() {
  const button = element("strategy-group-capture");
  if (!button) return;
  const count = appState.selectedFiles?.size || 0;
  button.disabled = count === 0;
  button.textContent = count > 0 ? `用选中的 ${count} 个 Mod 建组` : "用选中的 Mod 建组";
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
    if (event.target.checked) (managerState?.groups || []).forEach((group) => selection.add(String(group.id)));
    else selection.clear();
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
