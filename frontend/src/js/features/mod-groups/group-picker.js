// "把选中的 Mod 加入 / 移出 / 移动到某个策略组"的选择器对话框。
//
// 入口来自单文件菜单与多选菜单（context-menu.js），也复用同一套后端方法：
//   add    -> AddModStrategyGroupMembers
//   remove -> RemoveModStrategyGroupMembers
//   move   -> MoveModStrategyGroupMembers（一次写盘完成"移出 + 加入"）
//
// 这里只负责"选哪个组 + 调后端 + 刷新组归属"；刷新文件列表由调用方的 onDone 负责，
// 以避免 mod-groups 与 file-list 之间出现循环依赖。

import { appState } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { filePriorityKeys } from "../conflicts/conflict-badge.mjs";
import {
  AddModStrategyGroupMembers,
  CreateModStrategyGroupFromKeys,
  ListModStrategyGroups,
  MoveModStrategyGroupMembers,
  RemoveModStrategyGroupMembers,
} from "../../../../wailsjs/go/app/App";
import { refreshModGroupMembershipState } from "./group-state.mjs";
import { normalizeGroupKey } from "./group-view.mjs";
import {
  GROUP_PICKER_MODES,
  buildGroupPickerRows,
  filterGroupPickerRows,
  formatGroupPickerVisibleSummary,
  formatGroupPickerResult,
  formatGroupPickerSummary,
  groupPickerRowDetail,
  groupPickerRowTags,
  groupPickerConfirmLabel,
  groupPickerTitle,
  nextSelectableRowId,
  sortGroupPickerRows,
} from "./group-picker-view.mjs";

let pickerState = null;
// 视图态：搜索词、是否只看相关组、键盘高亮的行（与 pickerState 分开，重画不会丢）。
let pickerView = { query: "", relevantOnly: false, activeId: "" };

function element(id) {
  return document.getElementById(id);
}

/** keysForPaths 把文件路径换算成 addonlist 键（去重、保序）。 */
export function keysForPaths(paths) {
  const files = appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
  const byPath = new Map(files.map((file) => [file.path, file]));
  const keys = [];
  const seen = new Set();
  for (const path of paths || []) {
    const file = byPath.get(path);
    if (!file) continue;
    const fileKeys = filePriorityKeys(file, appState.currentDirectory);
    const key = fileKeys[0];
    if (!key || seen.has(key)) continue;
    seen.add(key);
    keys.push(key);
  }
  return keys;
}

/** groupIdsForPaths 返回这些文件当前所属的策略组 ID（去重、保序）。 */
export function groupIdsForPaths(paths) {
  const index = appState.modGroupIndex;
  const ids = [];
  const seen = new Set();
  for (const key of keysForPaths(paths)) {
    for (const membership of index?.get(normalizeGroupKey(key)) || []) {
      if (!membership?.groupId || seen.has(membership.groupId)) continue;
      seen.add(membership.groupId);
      ids.push(membership.groupId);
    }
  }
  return ids;
}

async function loadGroups() {
  try {
    return (await ListModStrategyGroups()) || [];
  } catch (error) {
    console.warn("读取策略组失败:", error);
    return [];
  }
}

/**
 * openGroupPicker 打开"加入 / 移出 / 移动到策略组"对话框。
 * @param {object} options
 * @param {"add"|"remove"|"move"} options.mode
 * @param {string[]} options.keys addonlist 键
 * @param {string} [options.sourceGroupId] move 模式的源组（缺省时先选源组）
 * @param {() => void} [options.onDone] 完成后的刷新回调（例如重画文件列表）
 */
export async function openGroupPicker({ mode = GROUP_PICKER_MODES.add, keys, sourceGroupId = "", onDone } = {}) {
  const modal = element("group-picker-modal");
  if (!modal) return;
  const normalizedKeys = [...new Set((keys || []).map((key) => String(key || "").trim()).filter(Boolean))];
  if (normalizedKeys.length === 0) {
    showError("请先勾选要处理的 Mod（当前没有可用的 addonlist 键）");
    return;
  }
  const groups = await loadGroups();
  if (groups.length === 0) {
    showError("还没有策略组：先用「分组建议」或「用选中的 Mod 建组」建一个组");
    return;
  }
  pickerState = {
    mode,
    keys: normalizedKeys,
    groups,
    sourceGroupId: String(sourceGroupId || ""),
    onDone,
    stage: mode === GROUP_PICKER_MODES.move && !sourceGroupId ? "source" : "target",
  };
  // 移出 / 换组时默认只看相关组：组一多，"不在组里"的那些行只是噪音。
  pickerView = {
    query: "",
    relevantOnly: mode === GROUP_PICKER_MODES.remove,
    activeId: "",
  };
  renderGroupPicker();
  modal.classList.remove("hidden");
  // 打开就把焦点交给搜索框：直接打字即可筛组，回车确认。
  const search = element("group-picker-search");
  if (search) {
    search.value = "";
    search.focus();
  }
  syncPickerControls();
}

export function closeGroupPicker() {
  element("group-picker-modal")?.classList.add("hidden");
  pickerState = null;
  pickerView = { query: "", relevantOnly: false, activeId: "" };
}

/**
 * computeVisibleRows 统一的"行 → 作用域过滤 → 用户筛选 → 排序"管线。
 * 渲染与键盘导航都走它，避免两处规则不一致（历史上就是两套逻辑各写一遍）。
 */
function computeVisibleRows() {
  if (!pickerState) return { rows: [], scopedRows: [], visibleRows: [], relevantOnly: false };
  const choosingSource = pickerState.stage === "source";
  const rows = buildGroupPickerRows({
    groups: pickerState.groups,
    keys: pickerState.keys,
    mode: choosingSource ? GROUP_PICKER_MODES.remove : pickerState.mode,
    sourceGroupId: pickerState.sourceGroupId,
  });
  // 源组阶段本身就只列"含选中项"的组，不再叠加用户筛选。
  const scopedRows = choosingSource ? rows.filter((row) => row.selectedInGroup > 0) : rows;
  const relevantOnly = choosingSource ? false : pickerView.relevantOnly;
  const visibleRows = sortGroupPickerRows(
    filterGroupPickerRows(scopedRows, { query: pickerView.query, relevantOnly }),
  );
  return { rows, scopedRows, visibleRows, relevantOnly };
}

function renderGroupPicker() {
  if (!pickerState) return;
  const title = element("group-picker-title");
  const summary = element("group-picker-summary");
  const list = element("group-picker-list");
  const confirm = element("group-picker-confirm-btn");
  const newRow = element("group-picker-new-row");
  if (!list) return;

  const choosingSource = pickerState.stage === "source";
  const mode = pickerState.mode;
  const sourceGroup = pickerState.groups.find((group) => group.id === pickerState.sourceGroupId);
  const sourceName = sourceGroup?.name || "";

  if (title) {
    title.textContent = choosingSource ? "选择要移出的策略组" : groupPickerTitle(mode);
  }

  const { rows, scopedRows, visibleRows, relevantOnly } = computeVisibleRows();
  // 不预选任何行：避免"打开就回车"误操作；按 ↓ / 点一行才产生高亮与预选。
  if (pickerView.activeId && !visibleRows.some((row) => row.id === pickerView.activeId && !row.disabled)) {
    pickerView.activeId = "";
  }

  if (summary) {
    if (choosingSource) {
      summary.textContent =
        `已选中 ${pickerState.keys.length} 个 Mod；它们分布在 ${visibleRows.length} 个策略组里，` +
        "请选择要从哪个组移动（下一步再选目标组）";
    } else {
      summary.textContent = formatGroupPickerSummary({
        rows,
        keyCount: pickerState.keys.length,
        mode,
        sourceName,
      });
    }
  }
  const visibleLabel = element("group-picker-visible");
  if (visibleLabel) {
    visibleLabel.textContent = formatGroupPickerVisibleSummary({
      total: scopedRows.length,
      shown: visibleRows.length,
      usable: visibleRows.filter((row) => !row.disabled).length,
      hiddenByFilter: Math.max(0, scopedRows.length - visibleRows.length),
    });
  }

  list.replaceChildren();
  if (visibleRows.length === 0) {
    const empty = document.createElement("div");
    empty.className = "group-picker-empty";
    empty.textContent = pickerView.query
      ? `没有匹配「${pickerView.query}」的策略组`
      : relevantOnly
        ? "没有包含这些 Mod 的策略组（取消「只看相关」可以看到全部）"
        : "没有可用的策略组";
    list.appendChild(empty);
  }
  visibleRows.forEach((row) => {
    const item = document.createElement("label");
    item.className = "group-picker-row";
    item.dataset.rowId = row.id;
    if (row.disabled) item.classList.add("is-disabled");
    if (row.id === pickerView.activeId && !row.disabled) item.classList.add("is-active");

    const radio = document.createElement("input");
    radio.type = "radio";
    radio.name = "group-picker-target";
    radio.value = row.id;
    radio.disabled = row.disabled;
    radio.checked = row.id === pickerView.activeId && !row.disabled;
    radio.addEventListener("change", () => {
      if (confirm) confirm.disabled = false;
      pickerView.activeId = row.id;
      highlightActiveRow();
    });

    const main = document.createElement("span");
    main.className = "group-picker-row-main";
    const name = document.createElement("strong");
    name.textContent = row.name;
    main.append(name);
    const tags = groupPickerRowTags(row);
    if (tags.length > 0) {
      const tagRow = document.createElement("span");
      tagRow.className = "group-picker-row-tags";
      tags.forEach((text) => {
        const tag = document.createElement("span");
        tag.className = "group-picker-row-tag";
        tag.textContent = text;
        tagRow.appendChild(tag);
      });
      main.appendChild(tagRow);
    }
    const meta = document.createElement("span");
    meta.className = "group-picker-row-meta";
    meta.textContent = groupPickerRowDetail(row);
    main.appendChild(meta);

    item.append(radio, main);
    if (row.disabled) item.title = row.disabledReason;
    // 双击 = 选中并立即执行：组少的时候一步到位。
    item.addEventListener("dblclick", (event) => {
      if (row.disabled) return;
      event.preventDefault();
      const input = item.querySelector("input[name='group-picker-target']");
      if (input) input.checked = true;
      pickerView.activeId = row.id;
      void confirmGroupPicker();
    });
    list.appendChild(item);
  });

  if (confirm) {
    confirm.disabled = !pickerView.activeId;
    confirm.textContent = choosingSource ? "下一步" : groupPickerConfirmLabel(mode);
  }
  if (newRow) {
    // "新建并加入"只在加入模式且已经选定目标时才有意义。
    newRow.classList.toggle("hidden", choosingSource || mode !== GROUP_PICKER_MODES.add);
    const input = element("group-picker-new-name");
    if (input) input.value = "";
  }
}

/** syncPickerControls 把视图态同步到工具条控件（搜索框 / 只看相关 / 新建行显隐）。 */
function syncPickerControls() {
  const search = element("group-picker-search");
  if (search && search.value !== pickerView.query) search.value = pickerView.query;
  const relevant = element("group-picker-relevant-only");
  if (relevant) relevant.checked = pickerView.relevantOnly;
  const newRow = element("group-picker-new-row");
  if (newRow && pickerState) {
    const visible = pickerState.stage !== "source" && pickerState.mode === GROUP_PICKER_MODES.add;
    newRow.classList.toggle("hidden", !visible);
  }
}

/** highlightActiveRow 只切换高亮类，避免为了移动键盘焦点整表重画。 */
function highlightActiveRow() {
  const list = element("group-picker-list");
  if (!list) return;
  list.querySelectorAll(".group-picker-row").forEach((item) => {
    const isActive = item.dataset.rowId === pickerView.activeId && !item.classList.contains("is-disabled");
    item.classList.toggle("is-active", isActive);
    const radio = item.querySelector("input[name='group-picker-target']");
    if (radio && !radio.disabled) radio.checked = isActive;
  });
  const confirm = element("group-picker-confirm-btn");
  if (confirm) confirm.disabled = !pickerView.activeId;
}

/** selectRowById 把键盘高亮落到指定行，并滚动到可见区域。 */
function selectRowById(id) {
  pickerView.activeId = String(id || "");
  highlightActiveRow();
  if (!pickerView.activeId) return;
  const item = document.querySelector(
    `#group-picker-list .group-picker-row[data-row-id="${CSS.escape(pickerView.activeId)}"]`,
  );
  item?.scrollIntoView({ block: "nearest" });
}

function selectedRowId() {
  const checked = document.querySelector(
    "#group-picker-list input[name='group-picker-target']:checked",
  );
  return checked?.value || "";
}

async function confirmGroupPicker() {
  if (!pickerState) return;
  const id = selectedRowId();
  if (!id) return;

  if (pickerState.mode === GROUP_PICKER_MODES.move && pickerState.stage === "source") {
    pickerState.sourceGroupId = id;
    pickerState.stage = "target";
    pickerView.activeId = "";
    renderGroupPicker();
    return;
  }

  const { mode, keys, sourceGroupId, onDone } = pickerState;
  const group = pickerState.groups.find((item) => item.id === id);
  const before = (group?.members || []).length;
  const confirmButton = element("group-picker-confirm-btn");
  if (confirmButton) confirmButton.disabled = true;
  try {
    if (mode === GROUP_PICKER_MODES.add) {
      const updated = await AddModStrategyGroupMembers(id, keys);
      const added = Math.max(0, (updated?.members || []).length - before);
      if (added === 0) {
        showNotification(`选中的 Mod 已经都在「${updated?.name || group?.name}」里了`, "info");
      } else {
        showNotification(formatGroupPickerResult(updated, { mode, keyCount: added }), "success");
      }
    } else if (mode === GROUP_PICKER_MODES.remove) {
      const updated = await RemoveModStrategyGroupMembers(id, keys);
      const removed = Math.max(0, before - (updated?.members || []).length);
      showNotification(formatGroupPickerResult(updated, { mode, keyCount: removed }), "success");
    } else {
      const result = await MoveModStrategyGroupMembers(sourceGroupId, id, keys);
      showNotification(formatGroupPickerResult(result, { mode }), (result?.moved || []).length > 0 ? "success" : "info");
    }
    await refreshModGroupMembershipState();
    if (typeof onDone === "function") onDone();
    closeGroupPicker();
  } catch (error) {
    showError(
      (mode === GROUP_PICKER_MODES.add ? "加入策略组失败: " : mode === GROUP_PICKER_MODES.remove ? "移出策略组失败: " : "移动策略组失败: ") +
        String(error?.message || error),
    );
    if (confirmButton) confirmButton.disabled = false;
  }
}

async function createGroupFromPicker() {
  if (!pickerState || pickerState.mode !== GROUP_PICKER_MODES.add) return;
  const input = element("group-picker-new-name");
  const name = String(input?.value || "").trim();
  if (!name) {
    showError("请先填写新策略组的名称");
    input?.focus();
    return;
  }
  const { keys, onDone } = pickerState;
  try {
    const group = await CreateModStrategyGroupFromKeys(name, "从 Mod 管理页勾选并加入", "single", keys);
    showNotification(`已创建策略组「${group.name}」（${keys.length} 个成员）`, "success");
    await refreshModGroupMembershipState();
    if (typeof onDone === "function") onDone();
    closeGroupPicker();
  } catch (error) {
    showError("新建策略组失败: " + String(error?.message || error));
  }
}

let pickerBound = false;

/** initGroupPicker 绑定对话框按钮（幂等）。 */
export function initGroupPicker() {
  if (pickerBound) return;
  pickerBound = true;
  element("group-picker-confirm-btn")?.addEventListener("click", () => void confirmGroupPicker());
  element("group-picker-cancel-btn")?.addEventListener("click", closeGroupPicker);
  element("group-picker-close-btn")?.addEventListener("click", closeGroupPicker);
  element("group-picker-new-btn")?.addEventListener("click", () => void createGroupFromPicker());

  // 搜索：边打边筛（组数量是几十级别，不需要防抖），回车直接确认高亮行。
  element("group-picker-search")?.addEventListener("input", (event) => {
    pickerView.query = String(event.target.value || "");
    renderGroupPicker();
  });
  element("group-picker-relevant-only")?.addEventListener("change", (event) => {
    pickerView.relevantOnly = Boolean(event.target.checked);
    pickerView.activeId = "";
    renderGroupPicker();
  });
  // 「新建并加入」输入框里回车 = 点「新建并加入」。
  element("group-picker-new-name")?.addEventListener("keydown", (event) => {
    if (event.key !== "Enter") return;
    event.preventDefault();
    void createGroupFromPicker();
  });
  // 键盘：↑↓ 选行、Enter 确认、Esc 关闭。焦点在搜索框里也能用。
  element("group-picker-modal")?.addEventListener("keydown", (event) => {
    if (!pickerState) return;
    if (event.key === "Escape") {
      event.stopPropagation();
      closeGroupPicker();
      return;
    }
    if (event.key === "Enter") {
      // 在"新建并加入"输入框里回车由它自己的处理函数负责。
      if (event.target?.id === "group-picker-new-name") return;
      if (!pickerView.activeId) return;
      event.preventDefault();
      void confirmGroupPicker();
      return;
    }
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    event.preventDefault();
    const { visibleRows } = computeVisibleRows();
    const delta = event.key === "ArrowDown" ? 1 : -1;
    const nextId = nextSelectableRowId(visibleRows, pickerView.activeId, delta);
    if (nextId) selectRowById(nextId);
  });
}
