// 批量规范化命名（路线图第 20 项 P2）——弹窗接线。
//
// 流程：多选 Mod → 右键「批量规范化命名…」→ 后端只读生成计划（PlanNamingNormalization）
// → 弹窗预览（旧名 → 新名 + 判定依据 + 待复核 / 跳过分组）→ 用户勾选 → 确认执行
// （ApplyNamingNormalization：执行前自动落盘撤销清单）→ 逐项结果 + 汇总。
//
// 纯逻辑在 batch-naming-view.mjs（分组 / 请求拼装 / 剪贴板载荷），展示映射在
// naming-display.mjs；两条都有 node --test 覆盖，这里只做 DOM 与调用接线。
import { appState, showLoadingScreen, showMainScreen, updateLoadingMessage } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { ApplyNamingNormalization, PlanNamingNormalization } from "../../../../wailsjs/go/app/App";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import { confirmHtmlInApp } from "../modals/confirm.js";
import { refreshFilesKeepFilter } from "./filters.js";
import { deselectAll } from "./actions.js";
import { localizeReason, stageLabelKey } from "./naming-display.mjs";
import {
  buildApplyRequests,
  defaultCheckedPaths,
  selectablePaths,
  serializePlanPayload,
  serializeUndoPayload,
  splitPlanItems,
} from "./batch-naming-view.mjs";

let currentPlan = null;
let checkedPaths = new Set();
let lastResult = null;
let executing = false;
let progressBound = false;
const rowByPath = new Map();

function byId(id) {
  return document.getElementById(id);
}

// setStatusLine 更新页脚状态行（计划中 / 执行进度 / 结果汇总的补充信息）。
function setStatusLine(text) {
  const status = byId("batch-naming-status");
  if (!status) return;
  status.textContent = text || "";
  status.classList.toggle("hidden", !text);
}

function setApplyButtonState() {
  const applyBtn = byId("apply-batch-naming-btn");
  if (!applyBtn) return;
  const count = buildApplyRequests(currentPlan, checkedPaths).length;
  applyBtn.textContent = count > 0 ? t("modals.batchNaming.applyCount", { count }) : t("modals.batchNaming.apply");
  applyBtn.disabled = executing || count === 0 || Boolean(lastResult);
}

function setBusyControls() {
  const applyBtn = byId("apply-batch-naming-btn");
  const cancelBtn = byId("cancel-batch-naming-btn");
  const closeBtn = byId("close-batch-naming-modal-btn");
  const selectAll = byId("batch-naming-select-all");
  if (applyBtn) applyBtn.disabled = executing;
  if (cancelBtn) cancelBtn.disabled = executing;
  if (closeBtn) closeBtn.disabled = executing;
  if (selectAll) selectAll.disabled = executing;
}

function updateSelectAllState() {
  const selectAll = byId("batch-naming-select-all");
  if (!selectAll) return;
  const selectable = selectablePaths(currentPlan);
  const checkedCount = selectable.filter((path) => checkedPaths.has(path)).length;
  selectAll.checked = selectable.length > 0 && checkedCount === selectable.length;
  selectAll.indeterminate = checkedCount > 0 && checkedCount < selectable.length;
}

// createBadge 生成一个标签徽标（阶段 / 待复核 / 已截断）。
function createBadge(text, extraClass) {
  const badge = document.createElement("span");
  badge.className = `batch-naming-badge ${extraClass}`;
  badge.textContent = text;
  return badge;
}

// createRow 渲染一行计划项；selectable 为 false 时是「跳过」只读行。
function createRow(item, selectable) {
  const row = document.createElement("div");
  row.className = "batch-naming-row";
  if (!selectable) row.classList.add("is-skipped");
  row.dataset.path = item.path;
  rowByPath.set(item.path, row);

  const main = document.createElement(selectable ? "label" : "div");
  main.className = "batch-naming-row-main";

  if (selectable) {
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.className = "batch-naming-checkbox";
    checkbox.checked = checkedPaths.has(item.path);
    checkbox.addEventListener("change", () => {
      if (executing) return;
      if (checkbox.checked) checkedPaths.add(item.path);
      else checkedPaths.delete(item.path);
      updateSelectAllState();
      setApplyButtonState();
    });
    main.appendChild(checkbox);
  }

  const names = document.createElement("span");
  names.className = "batch-naming-names";

  const oldName = document.createElement("span");
  oldName.className = "batch-naming-old";
  oldName.textContent = item.oldName || "";
  names.appendChild(oldName);

  if (item.newName) {
    const arrow = document.createElement("span");
    arrow.className = "batch-naming-arrow";
    arrow.textContent = "→";
    names.appendChild(arrow);

    const newName = document.createElement("span");
    newName.className = "batch-naming-new";
    newName.textContent = item.newName;
    names.appendChild(newName);
  }
  main.appendChild(names);

  const badges = document.createElement("span");
  badges.className = "batch-naming-badges";
  const stageKey = stageLabelKey(item.stage);
  if (stageKey && item.actionable) {
    badges.appendChild(createBadge(t(stageKey), "badge-stage"));
  }
  if (item.pendingReview) {
    badges.appendChild(createBadge(t("modals.batchNaming.badgePending"), "badge-pending"));
  }
  if (item.truncated) {
    badges.appendChild(createBadge(t("modals.batchNaming.badgeTruncated"), "badge-truncated"));
  }
  if (badges.childElementCount > 0) main.appendChild(badges);
  row.appendChild(main);

  if (item.actionable) {
    const reason = localizeReason(item, t);
    if (reason) {
      const reasonLine = document.createElement("p");
      reasonLine.className = "batch-naming-reason";
      reasonLine.textContent = t("modals.batchNaming.reasonPrefix", { reason });
      row.appendChild(reasonLine);
    }
  } else {
    const skipLine = document.createElement("p");
    skipLine.className = "batch-naming-skip";
    const skipText = document.createElement("span");
    skipText.textContent = t(`modals.batchNaming.skip.${item.skipReason || "invalid"}`);
    skipLine.appendChild(skipText);
    if (item.skipDetail) {
      const detail = document.createElement("span");
      detail.className = "batch-naming-skip-detail";
      detail.textContent = ` ${item.skipDetail}`;
      skipLine.appendChild(detail);
    }
    row.appendChild(skipLine);
  }

  if (item.actionable) {
    const status = document.createElement("p");
    status.className = "batch-naming-row-status hidden";
    row.appendChild(status);
  }
  return row;
}

// createGroup 渲染一组（标题 + 若干行）。
function createGroup(list, titleKey, count, selectable) {
  if (count <= 0 || list.length === 0) return null;
  const group = document.createElement("section");
  group.className = "batch-naming-group";

  const title = document.createElement("h4");
  title.className = "batch-naming-group-title";
  title.textContent = t(titleKey, { count });
  group.appendChild(title);

  list.forEach((item) => group.appendChild(createRow(item, selectable)));
  return group;
}

function renderPlan() {
  rowByPath.clear();
  const list = byId("batch-naming-list");
  const summary = byId("batch-naming-summary");
  const empty = byId("batch-naming-empty");
  const selectAllWrap = byId("batch-naming-select-all-wrap");
  if (!list || !summary || !currentPlan) return;

  summary.textContent = t("modals.batchNaming.summary", {
    total: currentPlan.total,
    renamable: currentPlan.renamable,
    pending: currentPlan.pending,
    skipped: currentPlan.skipped,
  });
  setStatusLine(t("modals.batchNaming.hint"));

  list.replaceChildren();
  const groups = splitPlanItems(currentPlan);
  const actionableGroup = createGroup(
    groups.actionable,
    "modals.batchNaming.groupActionable",
    groups.actionable.length,
    true,
  );
  const pendingGroup = createGroup(
    groups.pending,
    "modals.batchNaming.groupPending",
    groups.pending.length,
    true,
  );
  const skippedGroup = createGroup(
    groups.skipped,
    "modals.batchNaming.groupSkipped",
    groups.skipped.length,
    false,
  );
  [actionableGroup, pendingGroup, skippedGroup].forEach((group) => {
    if (group) list.appendChild(group);
  });

  if (empty) {
    empty.classList.toggle("hidden", currentPlan.total > 0);
  }
  if (selectAllWrap) {
    const anySelectable = groups.actionable.length + groups.pending.length > 0;
    selectAllWrap.classList.toggle("hidden", !anySelectable);
  }
  updateSelectAllState();
  setApplyButtonState();
}

function resetModalState() {
  currentPlan = null;
  checkedPaths = new Set();
  lastResult = null;
  executing = false;
  setStatusLine("");
  const copyUndo = byId("copy-batch-naming-undo-btn");
  if (copyUndo) copyUndo.classList.add("hidden");
  const list = byId("batch-naming-list");
  if (list) list.replaceChildren();
}

export async function openBatchNamingModal() {
  if (appState.selectedFiles.size === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }
  const paths = Array.from(appState.selectedFiles);
  updateLoadingMessage(t("modals.batchNaming.planning"));
  showLoadingScreen();
  try {
    const plan = await PlanNamingNormalization(paths);
    resetModalState();
    currentPlan = plan;
    checkedPaths = new Set(defaultCheckedPaths(plan));
    ensureProgressBinding();
    renderPlan();
    setBusyControls();
    const modal = byId("batch-naming-modal");
    if (modal) modal.classList.remove("hidden");
  } catch (error) {
    showError(t("modals.batchNaming.planFailed", { error: String(error) }));
  } finally {
    showMainScreen();
  }
}

// copyTextToClipboard 优先浏览器剪贴板 API，失败退回 Wails 绑定（同 group-ui 口径）。
async function copyTextToClipboard(text) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (error) {
    console.warn("剪贴板 API 不可用:", error);
  }
  try {
    const binding = window.go?.app?.App?.ClipboardSetText;
    if (typeof binding === "function") {
      await binding(text);
      return true;
    }
  } catch (error) {
    console.warn("Wails 剪贴板绑定不可用:", error);
  }
  return false;
}

async function copyText(text) {
  const copied = await copyTextToClipboard(text);
  if (copied) {
    showNotification(t("modals.batchNaming.copied"), "success");
  } else {
    showError(t("modals.batchNaming.copyFailed", { error: "clipboard unavailable" }));
  }
}

// ensureProgressBinding 订阅后端批量进度事件（只绑一次）。
function ensureProgressBinding() {
  if (progressBound) return;
  progressBound = true;
  EventsOn("naming_batch_progress", (payload) => {
    if (!payload) return;
    setStatusLine(t("modals.batchNaming.executing", { done: payload.done, total: payload.total }));
  });
}

// renderResult 把执行结果逐项写回行内状态，并显示汇总 / 撤销清单路径。
function renderResult(result) {
  lastResult = result;
  const items = Array.isArray(result.items) ? result.items : [];
  items.forEach((item) => {
    const row = rowByPath.get(item.path);
    if (!row) return;
    row.classList.add(item.ok ? "is-ok" : "is-fail");
    const status = row.querySelector(".batch-naming-row-status");
    if (status) {
      status.textContent = item.ok
        ? t("modals.batchNaming.rowOk")
        : t("modals.batchNaming.rowFailed", { error: item.error || "" });
      status.classList.remove("hidden");
    }
  });
  setStatusLine(t("modals.batchNaming.resultSummary", { success: result.success, failed: result.failed }));
  const backup = byId("batch-naming-backup-note");
  if (backup && result.backupPath) {
    backup.textContent = t("modals.batchNaming.backupNote", { path: result.backupPath });
    backup.classList.remove("hidden");
  }
  const copyUndo = byId("copy-batch-naming-undo-btn");
  if (copyUndo && items.some((item) => item.ok)) {
    copyUndo.classList.remove("hidden");
  }
}

async function executePlan() {
  const requests = buildApplyRequests(currentPlan, checkedPaths);
  if (requests.length === 0 || executing || lastResult) return;

  const pendingCount = currentPlan.items.filter(
    (item) => item.actionable && item.pendingReview && checkedPaths.has(item.path),
  ).length;
  if (pendingCount > 0) {
    const confirmed = await confirmHtmlInApp(
      t("modals.batchNaming.pendingConfirmTitle"),
      `<p>${t("modals.batchNaming.pendingConfirmBody", { count: pendingCount })}</p>`,
    );
    if (!confirmed) return;
  }

  executing = true;
  setBusyControls();
  setApplyButtonState();
  setStatusLine(t("modals.batchNaming.executing", { done: 0, total: requests.length }));
  try {
    const result = await ApplyNamingNormalization(requests);
    renderResult(result);
    if (result.failed > 0) {
      showNotification(
        t("modals.batchNaming.resultSummary", { success: result.success, failed: result.failed }),
        "warning",
      );
    } else {
      showNotification(
        t("modals.batchNaming.resultSummary", { success: result.success, failed: result.failed }),
        "success",
      );
    }
    await refreshFilesKeepFilter();
    deselectAll();
  } catch (error) {
    showError(t("modals.batchNaming.executeFailed", { error: String(error) }));
  } finally {
    executing = false;
    setBusyControls();
    setApplyButtonState();
  }
}

function closeModal() {
  if (executing) return;
  const modal = byId("batch-naming-modal");
  if (modal) modal.classList.add("hidden");
}

export function setupBatchNamingListeners() {
  const closeBtn = byId("close-batch-naming-modal-btn");
  if (closeBtn) closeBtn.addEventListener("click", closeModal);
  const cancelBtn = byId("cancel-batch-naming-btn");
  if (cancelBtn) cancelBtn.addEventListener("click", closeModal);

  const selectAll = byId("batch-naming-select-all");
  if (selectAll) {
    selectAll.addEventListener("change", () => {
      if (executing || !currentPlan) return;
      const selectable = selectablePaths(currentPlan);
      if (selectAll.checked) selectable.forEach((path) => checkedPaths.add(path));
      else selectable.forEach((path) => checkedPaths.delete(path));
      rowByPath.forEach((row, path) => {
        const checkbox = row.querySelector(".batch-naming-checkbox");
        if (checkbox) checkbox.checked = checkedPaths.has(path);
      });
      updateSelectAllState();
      setApplyButtonState();
    });
  }

  const applyBtn = byId("apply-batch-naming-btn");
  if (applyBtn) {
    applyBtn.addEventListener("click", () => {
      void executePlan();
    });
  }

  const copyPlanBtn = byId("copy-batch-naming-plan-btn");
  if (copyPlanBtn) {
    copyPlanBtn.addEventListener("click", () => {
      if (!currentPlan) return;
      void copyText(serializePlanPayload(currentPlan, new Date().toISOString()));
    });
  }

  const copyUndoBtn = byId("copy-batch-naming-undo-btn");
  if (copyUndoBtn) {
    copyUndoBtn.addEventListener("click", () => {
      if (!lastResult) return;
      void copyText(serializeUndoPayload(lastResult, new Date().toISOString()));
    });
  }
}
