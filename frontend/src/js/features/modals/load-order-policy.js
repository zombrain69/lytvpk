import { appState } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { refreshFilesKeepFilter } from "../file-list/filters.js";
import { renderFileList } from "../file-list/render.js";
import { refreshLoadOrderMap } from "../file-list/sorting.js";
import { initLoadOrderPriorityControls, syncLoadOrderPriorityPanel } from "./load-order-priority.js";
import {
  LOAD_ORDER_ENTRY_STATUS,
  classifyLoadOrderEntries,
  normalizeLoadOrderKey,
} from "./load-order-entry-status.mjs";
import { createRuleListbox } from "./load-order-rule-list.mjs";
import {
  LOAD_ORDER_PREVIEW_FALLBACK_ROW_PITCH,
  LOAD_ORDER_PREVIEW_OVERSCAN_ROWS,
  computePreviewWindow,
  previewSpacerHeights,
} from "./load-order-preview-window.mjs";
import {
  ApplyAddonListLoadOrderPolicy,
  GetAddonListLoadOrderEntries,
  GetAddonListRestoreCandidate,
  GetVPKLoadOrder,
  PreviewAddonListLoadOrderPolicy,
  RestoreAddonListBackup,
  RestoreAddonListFirstEditBackup,
  SetVPKLoadOrder,
} from "../../../../wailsjs/go/app/App";
import { confirmHtmlInApp } from "./confirm.js";
import { buildRestoreConfirmHtml, buildWriteDiffHtml, hasWriteDiffChanges } from "./load-order-write-diff.mjs";

let currentLoadOrderFile = null;
let currentOrder = -1;
let currentEntries = [];
let currentPolicy = createEmptyPolicy();
let controlsBound = false;
let currentMode = "global";
let selectedSourceKeys = new Set();
let selectedTargetKey = "";
let selectedPaths = [];
let selectedPathNames = [];
let entryFileByKey = new Map();
let entrySearchTextByKey = new Map();
// 弹窗可以在异步读取 addonlist.txt 的途中被关闭或以另一个 Mod 重新打开。
// 会话号阻止旧窗口回写新窗口；操作号进一步阻止同一窗口内旧预览覆盖新操作。
let loadOrderModalGeneration = 0;
let loadOrderOperationGeneration = 0;

const LOAD_ORDER_PRESETS = {
  recommended: {
    labelKey: "loadOrder.presetTidy",
    policy: { rootFirst: true, groupWorkshop: true, stateOrder: "keep" },
  },
  "root-first": {
    labelKey: "loadOrder.presetRootFirst",
    policy: { rootFirst: true, groupWorkshop: false, stateOrder: "keep" },
  },
  "group-workshop": {
    labelKey: "loadOrder.presetWorkshopGroup",
    policy: { rootFirst: false, groupWorkshop: true, stateOrder: "keep" },
  },
  "enabled-first": {
    labelKey: "loadOrder.presetEnabledFirst",
    policy: { rootFirst: false, groupWorkshop: false, stateOrder: "enabled-first" },
  },
  "disabled-first": {
    labelKey: "loadOrder.presetDisabledFirst",
    policy: { rootFirst: false, groupWorkshop: false, stateOrder: "disabled-first" },
  },
};

function createEmptyPolicy() {
  return { rootFirst: false, groupWorkshop: false, stateOrder: "keep", constraints: [] };
}

function beginLoadOrderOperation() {
  return {
    modalGeneration: loadOrderModalGeneration,
    operationGeneration: ++loadOrderOperationGeneration,
    filePath: currentLoadOrderFile,
  };
}

function invalidateLoadOrderOperations() {
  loadOrderOperationGeneration++;
}

function isCurrentLoadOrderOperation(operation) {
  const modal = document.getElementById("load-order-modal");
  return Boolean(
    operation &&
      operation.modalGeneration === loadOrderModalGeneration &&
      operation.operationGeneration === loadOrderOperationGeneration &&
      operation.filePath === currentLoadOrderFile &&
      modal &&
      !modal.classList.contains("hidden")
  );
}

export async function openEnhancedLoadOrderModal(context = {}) {
  const normalizedContext = normalizeLoadOrderContext(context);
  const vpkFiles = appState.vpkFiles || [];
  const file = normalizedContext.filePath
    ? vpkFiles.find((item) => item.path === normalizedContext.filePath)
    : null;
  if (normalizedContext.mode === "single" && !file) return;

  currentMode = normalizedContext.mode;
  ++loadOrderModalGeneration;
  currentLoadOrderFile = file?.path || null;
  currentOrder = -1;
  currentEntries = [];
  currentPolicy = createEmptyPolicy();
  selectedSourceKeys = new Set();
  selectedTargetKey = "";
  selectedPaths = normalizedContext.selectedPaths;
  selectedPathNames = selectedPaths
    .map((path) => vpkFiles.find((item) => item.path === path)?.name)
    .filter(Boolean);
  setupLoadOrderControls();

  renderModalMode(file);
  syncPolicyControls();
  const modal = document.getElementById("load-order-modal");
  if (!modal) return;
  const currentEl = document.getElementById("load-order-current");
  const input = document.getElementById("load-order-input");
  const sourceSearch = document.getElementById("load-order-rule-sources-search");
  const targetSearch = document.getElementById("load-order-rule-target-search");
  if (currentEl) currentEl.textContent = t("loadOrder.loading");
  if (input) input.value = "";
  if (sourceSearch) sourceSearch.value = "";
  if (targetSearch) targetSearch.value = "";
  modal.classList.remove("hidden");
  const operation = beginLoadOrderOperation();

  try {
    const refreshed = await refreshLoadOrderModal(operation);
    if (!refreshed || !isCurrentLoadOrderOperation(operation)) return;
    if (currentMode === "single") {
      input?.focus();
    } else {
      sourceSearch?.focus();
    }
  } catch (err) {
    if (!isCurrentLoadOrderOperation(operation)) return;
    console.error("获取加载顺序失败:", err);
    closeEnhancedLoadOrderModal();
    if (String(err).includes("addonlist.txt 不存在")) {
      showError(t("loadOrder.notFound"));
      return;
    }
    showError(t("loadOrder.fetchFailed", { error: err }));
  }
}

export function closeEnhancedLoadOrderModal() {
  ++loadOrderModalGeneration;
  invalidateLoadOrderOperations();
  detachPreviewWindow();
  document.getElementById("load-order-modal")?.classList.add("hidden");
  currentLoadOrderFile = null;
  selectedSourceKeys = new Set();
  selectedTargetKey = "";
  selectedPaths = [];
  selectedPathNames = [];
}

export async function saveEnhancedLoadOrder() {
  if (currentMode !== "single" || !currentLoadOrderFile) return;
  if (currentOrder <= 0) {
    showError(t("loadOrder.notWritten"));
    return;
  }
  const input = document.getElementById("load-order-input");
  if (!input) {
    showError(t("loadOrder.inputUnavailable"));
    return;
  }
  const order = Number.parseInt(input.value.trim(), 10);
  if (!Number.isInteger(order)) {
    showError(t("loadOrder.invalidIndex"));
    return;
  }
  const operation = beginLoadOrderOperation();
  const filePath = operation.filePath;
  try {
    await SetVPKLoadOrder(filePath, order);
    await refreshModListAfterLoadOrderChange();
    if (!isCurrentLoadOrderOperation(operation)) return;
    const refreshed = await refreshLoadOrderModal(operation);
    if (!refreshed || !isCurrentLoadOrderOperation(operation)) return;
    showNotification(t("loadOrder.savedSingle"), "success");
  } catch (err) {
    if (!isCurrentLoadOrderOperation(operation)) return;
    console.error("保存加载顺序失败:", err);
    showError(t("loadOrder.saveFailed", { error: err }));
  }
}

function setupLoadOrderControls() {
  if (controlsBound) return;
  controlsBound = true;

  document.getElementById("move-load-order-up-btn")?.addEventListener("click", () => moveCurrentFileBy(-1));
  document.getElementById("move-load-order-down-btn")?.addEventListener("click", () => moveCurrentFileBy(1));
  document.getElementById("add-load-order-before-rule-btn")?.addEventListener("click", () => addConstraints("before"));
  document.getElementById("add-load-order-after-rule-btn")?.addEventListener("click", () => addConstraints("after"));
  document.getElementById("load-order-presets")?.addEventListener("click", (event) => {
    const presetButton = event.target.closest("[data-load-order-preset]");
    if (!presetButton) return;
    applyLoadOrderPreset(presetButton.dataset.loadOrderPreset);
  });
  document.getElementById("preview-load-order-policy-btn")?.addEventListener("click", previewLoadOrderPolicy);
  document.getElementById("apply-load-order-policy-btn")?.addEventListener("click", applyLoadOrderPolicy);
  document.getElementById("restore-addonlist-backup-btn")?.addEventListener("click", restoreLatestAddonListBackup);
  initLoadOrderPriorityControls(async () => {
    await refreshModListAfterLoadOrderChange();
    const operation = beginLoadOrderOperation();
    await refreshLoadOrderModal(operation);
    await syncLoadOrderPriorityPanel(currentLoadOrderFile);
  });
  document.getElementById("load-order-root-first")?.addEventListener("change", (event) => {
    currentPolicy.rootFirst = event.target.checked;
    invalidateLoadOrderOperations();
    renderActivePolicy();
  });
  document.getElementById("load-order-group-workshop")?.addEventListener("change", (event) => {
    currentPolicy.groupWorkshop = event.target.checked;
    invalidateLoadOrderOperations();
    renderActivePolicy();
  });
  document.getElementById("load-order-state-order")?.addEventListener("change", (event) => {
    currentPolicy.stateOrder = event.target.value;
    invalidateLoadOrderOperations();
    renderActivePolicy();
  });
  document
    .getElementById("load-order-rule-sources-search")
    ?.addEventListener("input", () => renderRuleSelectors());
  document
    .getElementById("load-order-rule-target-search")
    ?.addEventListener("input", () => renderRuleSelectors());
  // 两个规则列表改成"可搜索 + 虚拟化的 listbox"（原来是原生 <select multiple size=7>，
  // 2599 条 addonlist 时两个下拉合计 5,217 个 <option>；见 load-order-rule-list.mjs）。
  // 选择集由这里持有：搜索/刷新列表不会丢选择。
  if (!ruleSourcesList) {
    ruleSourcesList = createRuleListbox(document.getElementById("load-order-rule-sources"), {
      multiple: true,
      onSelectionChange: (selected) => {
        selectedSourceKeys = new Set(selected);
      },
    });
  }
  if (!ruleTargetList) {
    ruleTargetList = createRuleListbox(document.getElementById("load-order-rule-target"), {
      multiple: false,
      onSelectionChange: (selected) => {
        selectedTargetKey = [...selected][0] || "";
        invalidateLoadOrderOperations();
      },
    });
  }
  document.getElementById("load-order-rules")?.addEventListener("click", (event) => {
    const button = event.target.closest("[data-load-order-rule-index]");
    if (!button) return;
    const index = Number.parseInt(button.dataset.loadOrderRuleIndex, 10);
    if (!Number.isInteger(index)) return;
    currentPolicy.constraints.splice(index, 1);
    invalidateLoadOrderOperations();
    renderRules();
  });
}

async function refreshLoadOrderModal(operation = beginLoadOrderOperation()) {
  const filePath = operation.filePath;
  const [entries, order] = await Promise.all([
    GetAddonListLoadOrderEntries(),
    filePath ? GetVPKLoadOrder(filePath) : Promise.resolve(-1),
  ]);
  if (!isCurrentLoadOrderOperation(operation)) return false;
  currentOrder = order;
  currentEntries = entries || [];
  prepareEntryMetadata();
  preselectContextEntries();
  renderModalMode(filePath ? (appState.vpkFiles || []).find((item) => item.path === filePath) : null);
  renderCurrentOrder();
  renderRuleSelectors();
  renderRules();
  renderActivePolicy();
  renderPreview(currentEntries, t("loadOrder.previewCurrent"));
  return true;
}

function renderCurrentOrder() {
  const singleSection = document.getElementById("load-order-single-section");
  if (singleSection?.classList.contains("hidden")) return;
  const currentOrderEl = document.getElementById("load-order-current");
  const input = document.getElementById("load-order-input");
  const upButton = document.getElementById("move-load-order-up-btn");
  const downButton = document.getElementById("move-load-order-down-btn");
  if (!currentOrderEl || !input || !upButton || !downButton) return;
  if (currentOrder > 0) {
    currentOrderEl.textContent = currentOrder;
    input.placeholder = String(currentOrder);
  } else {
    currentOrderEl.textContent = t("loadOrder.notGenerated");
    input.placeholder = t("loadOrder.newIndexHint");
  }
  upButton.disabled = currentOrder <= 1;
  downButton.disabled = currentOrder <= 0 || currentOrder >= currentEntries.length;
}

async function moveCurrentFileBy(delta) {
  if (!currentLoadOrderFile || currentOrder <= 0) {
    showError(t("loadOrder.notWrittenFirst"));
    return;
  }
  const destination = Math.min(Math.max(currentOrder + delta, 1), currentEntries.length);
  if (destination === currentOrder) return;
  const operation = beginLoadOrderOperation();
  const filePath = operation.filePath;
  try {
    await SetVPKLoadOrder(filePath, destination);
    await refreshModListAfterLoadOrderChange();
    if (!isCurrentLoadOrderOperation(operation)) return;
    const refreshed = await refreshLoadOrderModal(operation);
    if (!refreshed || !isCurrentLoadOrderOperation(operation)) return;
    showNotification(delta < 0 ? t("loadOrder.movedUp") : t("loadOrder.movedDown"), "success");
  } catch (err) {
    if (!isCurrentLoadOrderOperation(operation)) return;
    showError(t("loadOrder.moveFailed", { error: err }));
  }
}

function renderRuleSelectors() {
  const sourceQuery = document.getElementById("load-order-rule-sources-search")?.value || "";
  const targetQuery = document.getElementById("load-order-rule-target-search")?.value || "";
  // 两个列表都是虚拟化的（只物化视口 ± overscan 的行），同步换数据即可；
  // 选择集在 selectedSourceKeys / selectedTargetKey 里，不随搜索丢失。
  const sourceEntries = currentEntries.filter((entry) => entryMatchesSearch(entry, sourceQuery));
  const targetEntries = currentEntries.filter((entry) => entryMatchesSearch(entry, targetQuery));
  ruleSourcesList?.setItems(sourceEntries);
  ruleSourcesList?.setSelection(selectedSourceKeys);
  ruleTargetList?.setItems(targetEntries);
  ruleTargetList?.setSelection(selectedTargetKey ? new Set([selectedTargetKey]) : new Set());
}

let previewRenderToken = 0;
// 两个规则列表（可搜索 + 虚拟化的 listbox），由 setupLoadOrderControls 建一次。
let ruleSourcesList = null;
let ruleTargetList = null;

function addConstraints(direction) {
  if (!ruleSourcesList || !ruleTargetList) {
    showError(t("loadOrder.ruleSelectorUnavailable"));
    return;
  }
  const entryKeys = new Set(currentEntries.map((entry) => entry.key));
  // 选择集是唯一事实来源（虚拟化列表里没物化的行也算选中）。
  const sources = [...selectedSourceKeys].filter((key) => entryKeys.has(key));
  const target = selectedTargetKey;
  if (!target || sources.length === 0) {
    showError(t("loadOrder.selectRuleFirst"));
    return;
  }

  let added = 0;
  sources.forEach((source) => {
    if (source === target) return;
    const constraint = direction === "before"
      ? { before: source, after: target, anchorMove: "before" }
      : { before: target, after: source, anchorMove: "after" };
    const exists = currentPolicy.constraints.some((item) => item.before === constraint.before && item.after === constraint.after);
    if (!exists) {
      currentPolicy.constraints.push(constraint);
      added++;
    }
  });
  if (added === 0) {
    showError(t("loadOrder.ruleDuplicate"));
    return;
  }
  invalidateLoadOrderOperations();
  renderRules();
}

function renderRules() {
  const container = document.getElementById("load-order-rules");
  if (!container) return;
  container.replaceChildren();
  if (currentPolicy.constraints.length === 0) {
    const empty = document.createElement("div");
    empty.className = "load-order-rule-empty";
    empty.textContent = t("loadOrder.rulesEmpty");
    container.appendChild(empty);
    renderActivePolicy();
    return;
  }
  currentPolicy.constraints.forEach((constraint, index) => {
    const row = document.createElement("div");
    row.className = "load-order-rule";
    const text = document.createElement("span");
    text.className = "load-order-rule-text";
    if (constraint.anchorMove === "before") {
      text.textContent = t("loadOrder.ruleMoveBefore", { source: entryDisplayName(constraint.before), target: entryDisplayName(constraint.after) });
    } else if (constraint.anchorMove === "after") {
      text.textContent = t("loadOrder.ruleMoveAfter", { source: entryDisplayName(constraint.after), target: entryDisplayName(constraint.before) });
    } else {
      text.textContent = t("loadOrder.ruleOrderBefore", { source: entryDisplayName(constraint.before), target: entryDisplayName(constraint.after) });
    }
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "load-order-rule-delete";
    remove.dataset.loadOrderRuleIndex = String(index);
    remove.textContent = t("loadOrder.ruleRemove");
    row.append(text, remove);
    container.appendChild(row);
  });
  renderActivePolicy();
}

async function previewLoadOrderPolicy() {
  const operation = beginLoadOrderOperation();
  const policy = {
    ...currentPolicy,
    constraints: currentPolicy.constraints.map((constraint) => ({ ...constraint })),
  };
  try {
    const preview = await PreviewAddonListLoadOrderPolicy(policy);
    if (!isCurrentLoadOrderOperation(operation)) return;
    renderPreview(preview.entries || [], t("loadOrder.previewNotWritten"));
    showNotification(t("loadOrder.previewDone"), "info");
  } catch (err) {
    if (!isCurrentLoadOrderOperation(operation)) return;
    console.error("预览加载顺序失败:", err);
    showError(t("loadOrder.previewFailed", { error: err }));
  }
}

async function applyLoadOrderPolicy(options = {}) {
  const operation = beginLoadOrderOperation();
  const policy = {
    ...currentPolicy,
    constraints: currentPolicy.constraints.map((constraint) => ({ ...constraint })),
  };
  try {
    // 写前确认：先拿带差异的预览（与写入共用同一份计划）；没有变化就明说、不写盘。
    const preview = await PreviewAddonListLoadOrderPolicy(policy);
    if (!isCurrentLoadOrderOperation(operation)) return;
    if (!hasWriteDiffChanges(preview?.diff)) {
      renderPreview(preview?.entries || [], t("loadOrder.previewNoChanges"));
      showNotification(t("loadOrder.noChanges"), "info");
      return;
    }
    const confirmed = await confirmHtmlInApp(
      t("loadOrder.writeConfirmTitle"),
      buildWriteDiffHtml(preview.diff, {
        note: t("loadOrder.writeConfirmNote"),
      }),
      "load-order-write-confirm",
    );
    if (!confirmed || !isCurrentLoadOrderOperation(operation)) return;

    const result = await ApplyAddonListLoadOrderPolicy(policy);
    await refreshModListAfterLoadOrderChange();
    if (!isCurrentLoadOrderOperation(operation)) return;
    const order = operation.filePath ? await GetVPKLoadOrder(operation.filePath) : -1;
    if (!isCurrentLoadOrderOperation(operation)) return;
    currentEntries = result.entries || [];
    prepareEntryMetadata();
    renderRuleSelectors();
    // 保存时后端会顺手清理"文件已不存在/只在 disabled 里"的失效条目（对齐上游 a50cf4f），
    // 清理前已自动留一份历史备份；这里把结果说清楚，避免用户以为条目"凭空消失"。
    const removedStale = Array.isArray(result.removedStale) ? result.removedStale : [];
    const staleNote = removedStale.length > 0
      ? t("loadOrder.staleNote", { count: removedStale.length, list: removedStale.slice(0, 3).join("、"), more: removedStale.length > 3 ? t("loadOrder.staleMore") : "" })
      : "";
    renderPreview(currentEntries, staleNote || t("loadOrder.written"));
    currentOrder = order;
    renderCurrentOrder();
    renderActivePolicy();
    showNotification(
      options.successMessage || t("loadOrder.writeSuccess", { stale: staleNote ? t("loadOrder.writeSuccessStale", { note: staleNote }) : "" }),
      "success"
    );
  } catch (err) {
    if (!isCurrentLoadOrderOperation(operation)) return;
    console.error("应用加载顺序失败:", err);
    showError(t("loadOrder.writeFailed", { error: err }));
  }
}

// 恢复上一份备份：首次编辑备份（.lytvpk.bak，游戏原始内容）与历史备份里更新的一份，谁更新恢复谁。
// 恢复会覆盖当前 addonlist.txt —— 后端会先把当前内容另存为 before-restore 备份（可再回退）。
async function restoreLatestAddonListBackup() {
  try {
    const candidate = await GetAddonListRestoreCandidate();
    if (!candidate?.found) {
      showNotification(t("loadOrder.noBackup"), "info");
      return;
    }
    const confirmed = await confirmHtmlInApp(
      t("loadOrder.restoreTitle"),
      buildRestoreConfirmHtml(candidate),
      "load-order-write-confirm",
    );
    if (!confirmed) return;
    if (candidate.source === "first-edit") {
      await RestoreAddonListFirstEditBackup();
    } else {
      await RestoreAddonListBackup(candidate.name);
    }
    await refreshModListAfterLoadOrderChange();
    const operation = beginLoadOrderOperation();
    await refreshLoadOrderModal(operation);
    showNotification(t("loadOrder.restored"), "success");
  } catch (err) {
    console.error("恢复 addonlist.txt 备份失败:", err);
    showError(t("loadOrder.restoreFailed", { error: err }));
  }
}

// 只有用户当前正在按加载顺序查看列表时才重新扫描，避免普通排序场景产生不必要的
// VPK 扫描开销；筛选条件、排序方向和选中项由 refreshFilesKeepFilter 负责保留。
async function refreshModListAfterLoadOrderChange() {
  try {
    // 顺序/分层映射总是要刷新：即使当前不是按加载顺序排序，
    // 卡片上的「优先级 #N」「（分层 T）」角标也依赖这两个映射。
    // 只有"重新排序列表"本身才需要按加载顺序排序时才做。
    await refreshLoadOrderMap();
    if (appState.sortType === "loadOrder") {
      await refreshFilesKeepFilter();
      return;
    }
    renderFileList();
  } catch (error) {
    // 写入已经成功，刷新失败不应被误报为“写入失败”；记录日志并给出可操作提示。
    console.error("加载顺序已写入，但 Mod 列表刷新失败:", error);
    showNotification(t("loadOrder.refreshFailed"), "warning");
  }
}

function renderPreview(entries, summary) {
  const container = document.getElementById("load-order-preview");
  const summaryEl = document.getElementById("load-order-preview-summary");
  if (!container) return;
  detachPreviewWindow();
  // 失效条目（文件不存在 / 在 disabled）与未记录 Mod 是排查加载顺序时最常用的两条线索。
  const { statuses, invalidCount, unrecorded, unrecordedEntries } = classifyLoadOrderEntries(
    entries,
    appState.allVpkFiles || appState.vpkFiles || [],
  );
  if (summaryEl) {
    const extra = [];
    if (invalidCount > 0) extra.push(t("loadOrder.staleInvalid", { count: invalidCount }));
    if (unrecorded > 0) extra.push(t("loadOrder.staleUnrecorded", { count: unrecorded }));
    summaryEl.textContent = [t("loadOrder.summaryLine", { summary, count: entries.length }), ...extra].join(" · ");
  }
  container.replaceChildren();
  if (entries.length === 0) {
    const empty = document.createElement("div");
    empty.className = "load-order-preview-empty";
    empty.textContent = t("loadOrder.entriesEmpty");
    container.appendChild(empty);
    return;
  }

  const sourceKeys = new Set();
  const targetKeys = new Set();
  currentPolicy.constraints.forEach((constraint) => {
    if (constraint.anchorMove === "before") {
      sourceKeys.add(constraint.before);
      targetKeys.add(constraint.after);
    } else if (constraint.anchorMove === "after") {
      sourceKeys.add(constraint.after);
      targetKeys.add(constraint.before);
    } else {
      sourceKeys.add(constraint.before);
      targetKeys.add(constraint.after);
    }
  });
  // 「新增」条目（磁盘上有、addonlist 里没有）追加在末尾：它们本来就还没进加载顺序，
  // 默认位置与「未记录 Mod 首次开启时放在末尾」的设置一致。
  const displayEntries = [
    ...entries,
    ...(unrecordedEntries || []).map((item) => ({
      key: item.key,
      value: "",
      order: 0,
      isWorkshop: item.location === "workshop",
      isRoot: item.location === "root",
      isUnrecorded: true,
    })),
  ];
  const token = ++previewRenderToken;
  const buildRow = (entry) => {
    const row = document.createElement("div");
    row.className = "load-order-preview-item";
    if (entry.isUnrecorded) row.classList.add("is-unrecorded");
    if (sourceKeys.has(entry.key)) row.classList.add("is-rule-source");
    if (targetKeys.has(entry.key)) row.classList.add("is-rule-target");
    const order = document.createElement("span");
    order.className = "load-order-preview-number";
    order.textContent = entry.isUnrecorded ? "—" : String(entry.order);
    const type = document.createElement("span");
    const location = entry.isWorkshop ? "workshop" : entry.isRoot ? "root" : "other";
    type.className = `load-order-preview-type ${location}`;
    type.textContent = entry.isWorkshop ? t("loadOrder.previewType.workshop") : entry.isRoot ? t("loadOrder.previewType.root") : t("loadOrder.previewType.sub");
    const key = document.createElement("span");
    key.className = "load-order-preview-key";
    key.title = entryLabel(entry);
    key.textContent = entryDisplayName(entry.key);
    const state = document.createElement("span");
    const enabled = entry.value === "1";
    state.className = entry.isUnrecorded
      ? "load-order-preview-state unrecorded"
      : `load-order-preview-state ${enabled ? "enabled" : "disabled"}`;
    state.textContent = entry.isUnrecorded ? t("mods.priority.labelUnknown") : enabled ? t("mods.category.status.gameOn") : t("mods.category.status.gameOff");
    row.append(order, type, key, state);

    if (entry.isUnrecorded) {
      const badge = document.createElement("span");
      badge.className = "load-order-preview-status is-unrecorded";
      badge.textContent = t("loadOrder.badgeNew");
      badge.title = t("loadOrder.badgeNewTitle");
      row.appendChild(badge);
    }

    const status = statuses.get(normalizeLoadOrderKey(entry.key));
    if (status && status !== LOAD_ORDER_ENTRY_STATUS.existing) {
      const badge = document.createElement("span");
      badge.className = `load-order-preview-status is-${status}`;
      badge.textContent =
        status === LOAD_ORDER_ENTRY_STATUS.disabled ? t("loadOrder.badgeDisabledFile") : t("loadOrder.badgeMissingFile");
      badge.title =
        status === LOAD_ORDER_ENTRY_STATUS.disabled
          ? t("loadOrder.badgeDisabledTitle")
          : t("loadOrder.badgeMissingTitle");
      row.appendChild(badge);
    }
    return row;
  };
  // 窗口化渲染：容器里始终是 [上占位块, 视口±overscan 的行..., 下占位块]，
  // 滚出窗口的行会被回收（真机 2607 行整表渲染时滚动 23 帧 >50ms）。
  const topSpacer = document.createElement("div");
  topSpacer.className = "load-order-preview-spacer";
  topSpacer.dataset.previewSpacer = "top";
  topSpacer.style.display = "none";
  const bottomSpacer = document.createElement("div");
  bottomSpacer.className = "load-order-preview-spacer";
  bottomSpacer.dataset.previewSpacer = "bottom";
  bottomSpacer.style.display = "none";
  container.append(topSpacer, bottomSpacer);

  const state = {
    container,
    token,
    entries: displayEntries,
    buildRow,
    nodes: new Map(),
    start: 0,
    end: 0,
    rowPitch: 0,
    topSpacer,
    bottomSpacer,
    scrollPending: false,
  };
  previewWindowState = state;
  updatePreviewWindow(state, { force: true });
  container.addEventListener("scroll", handlePreviewWindowScroll, { passive: true });
}

// ── 预览行的窗口化渲染 ────────────────────────────────────────────────────
// 真机（2607 行）两条路都试过：整表 + content-visibility 滚动 23 帧 >50ms；
// 去掉 content-visibility 滚动顺了但开窗/填充变成 25 个长任务（50–249ms）。
// 窗口化只建"视口 ± overscan"的几十行，开窗与滚动都保持满帧。
let previewWindowState = null;

function detachPreviewWindow() {
  const state = previewWindowState;
  if (!state) return;
  state.container?.removeEventListener?.("scroll", handlePreviewWindowScroll);
  previewWindowState = null;
}

function handlePreviewWindowScroll() {
  const state = previewWindowState;
  if (!state || state.scrollPending) return;
  state.scrollPending = true;
  const run = () => {
    state.scrollPending = false;
    if (previewWindowState !== state) return;
    updatePreviewWindow(state);
  };
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(run);
  setTimeout(run, 50);
}

function setPreviewSpacer(spacer, height) {
  if (!spacer) return;
  const safe = Math.max(0, Number(height) || 0);
  spacer.style.height = `${safe}px`;
  spacer.style.display = safe > 0 ? "" : "none";
}

function updatePreviewWindow(state, { force = false } = {}) {
  if (previewWindowState !== state || state.token !== previewRenderToken) return;
  const { container, entries, nodes } = state;
  const total = entries.length;
  if (total === 0) return;
  const { start, end } = computePreviewWindow({
    total,
    rowPitch: state.rowPitch,
    scrollTop: container.scrollTop,
    clientHeight: container.clientHeight,
    overscanRows: LOAD_ORDER_PREVIEW_OVERSCAN_ROWS,
  });
  if (!force && start === state.start && end === state.end) return;

  for (const [index, node] of [...nodes]) {
    if (index >= start && index < end) continue;
    node.remove?.();
    nodes.delete(index);
  }
  const kept = [...nodes.keys()].sort((a, b) => a - b);
  if (kept.length === 0) {
    const fragment = document.createDocumentFragment();
    for (let index = start; index < end; index += 1) {
      const node = state.buildRow(entries[index]);
      nodes.set(index, node);
      fragment.appendChild(node);
    }
    container.insertBefore(fragment, state.bottomSpacer);
  } else {
    const firstKept = kept[0];
    const lastKept = kept[kept.length - 1];
    if (start < firstKept) {
      const anchor = nodes.get(firstKept);
      for (let index = start; index < firstKept; index += 1) {
        const node = state.buildRow(entries[index]);
        nodes.set(index, node);
        container.insertBefore(node, anchor);
      }
    }
    if (end > lastKept + 1) {
      for (let index = lastKept + 1; index < end; index += 1) {
        const node = state.buildRow(entries[index]);
        nodes.set(index, node);
        container.insertBefore(node, state.bottomSpacer);
      }
    }
  }
  state.start = start;
  state.end = end;
  if (!state.rowPitch) {
    const firstRow = nodes.get(start);
    const measured = firstRow?.getBoundingClientRect?.().height || 0;
    state.rowPitch = Math.max(1, Math.round(measured) || LOAD_ORDER_PREVIEW_FALLBACK_ROW_PITCH);
  }
  const heights = previewSpacerHeights({ total, start, end, rowPitch: state.rowPitch });
  setPreviewSpacer(state.topSpacer, heights.top);
  setPreviewSpacer(state.bottomSpacer, heights.bottom);
}

function entryLabel(entry) {
  return `${entry.order}. ${entryDisplayName(entry.key)}${entry.value === "1" ? t("loadOrder.previewRowOn") : t("loadOrder.previewRowOff")}`;
}

function normalizeLoadOrderContext(context) {
  if (typeof context === "string") {
    return { mode: "single", filePath: context, selectedPaths: [] };
  }
  const selectedPaths = Array.from(new Set((context?.selectedPaths || []).filter(Boolean)));
  if (context?.mode === "single" && context.filePath) {
    return { mode: "single", filePath: context.filePath, selectedPaths: [] };
  }
  if (context?.mode === "selection" || selectedPaths.length > 0) {
    return { mode: "selection", filePath: null, selectedPaths };
  }
  return { mode: "global", filePath: null, selectedPaths: [] };
}

function renderModalMode(file) {
  const isSingle = currentMode === "single";
  const isSelection = currentMode === "selection";
  const title = document.getElementById("load-order-title");
  const filename = document.getElementById("load-order-filename");
  const contextNote = document.getElementById("load-order-context-note");
  const singleSection = document.getElementById("load-order-single-section");
  const confirmButton = document.getElementById("confirm-load-order-btn");
  const priorityPanel = document.getElementById("load-order-priority-panel");
  if (title) title.textContent = isSingle ? t("loadOrder.titleSingle") : isSelection ? t("loadOrder.titleSelection") : t("loadOrder.title");
  singleSection?.classList.toggle("hidden", !isSingle);
  confirmButton?.classList.toggle("hidden", !isSingle);
  if (!isSingle) priorityPanel?.classList.add("hidden");

  if (isSingle) {
    if (filename) filename.textContent = file?.name || t("loadOrder.currentMod");
    if (contextNote) contextNote.textContent = t("loadOrder.modeSingleNote");
    void syncLoadOrderPriorityPanel(file?.path || null);
    return;
  }
  if (isSelection) {
    if (filename) filename.textContent = t("loadOrder.selectionCount", { count: selectedPathNames.length });
    if (contextNote) contextNote.textContent = t("loadOrder.modeSelectionNote");
    return;
  }
  if (filename) filename.textContent = t("loadOrder.modeGlobalFileName");
  if (contextNote) contextNote.textContent = t("loadOrder.modeGlobalNote");
}

function preselectContextEntries() {
  if (currentMode !== "selection" || selectedSourceKeys.size > 0) return;
  const entryKeys = new Set(currentEntries.map((entry) => entry.key));
  const unmatchedNames = [];
  selectedPaths
    .map((path) => (appState.vpkFiles || []).find((file) => file.path === path))
    .filter(Boolean)
    .forEach((file) => {
      const key = addonListKeyForFile(file);
      if (key && entryKeys.has(key)) {
        selectedSourceKeys.add(key);
      } else {
        unmatchedNames.push(file.name);
      }
    });
  if (unmatchedNames.length > 0) {
    const contextNote = document.getElementById("load-order-context-note");
    if (contextNote) contextNote.textContent += t("loadOrder.unmatchedSuffix", { names: unmatchedNames.join("、") });
  }
}

function applyLoadOrderPreset(presetName) {
  const preset = LOAD_ORDER_PRESETS[presetName];
  if (!preset) return;
  currentPolicy = { ...createEmptyPolicy(), ...preset.policy };
  syncPolicyControls();
  renderRules();
  applyLoadOrderPolicy({ successMessage: t("loadOrder.presetApplied", { preset: t(preset.labelKey) }) });
}

function syncPolicyControls() {
  const rootFirst = document.getElementById("load-order-root-first");
  const groupWorkshop = document.getElementById("load-order-group-workshop");
  const stateOrder = document.getElementById("load-order-state-order");
  if (rootFirst) rootFirst.checked = currentPolicy.rootFirst;
  if (groupWorkshop) groupWorkshop.checked = currentPolicy.groupWorkshop;
  if (stateOrder) stateOrder.value = currentPolicy.stateOrder || "keep";
  renderActivePolicy();
}

function renderActivePolicy() {
  const summary = document.getElementById("load-order-active-policy");
  if (!summary) return;
  const labels = [];
  if (currentPolicy.rootFirst) labels.push(t("loadOrder.presetRootFirst"));
  if (currentPolicy.groupWorkshop) labels.push(t("loadOrder.presetWorkshopGroup"));
  if (currentPolicy.stateOrder === "enabled-first") labels.push(t("loadOrder.presetEnabledFirst"));
  if (currentPolicy.stateOrder === "disabled-first") labels.push(t("loadOrder.presetDisabledFirst"));
  if (currentPolicy.constraints.length > 0) labels.push(t("loadOrder.anchorRuleCount", { count: currentPolicy.constraints.length }));
  summary.textContent = labels.length > 0 ? t("loadOrder.activePolicySummary", { labels: labels.join(" · ") }) : t("loadOrder.activePolicyNone");
}

function addonListKeyForFile(file) {
  if (!file?.name) return "";
  const name = String(file.name).replaceAll("/", "\\").toLowerCase();
  return file.location === "workshop" ? `workshop\\${name}` : name;
}

function entryMatchesSearch(entry, query) {
  const needle = String(query || "").trim().toLocaleLowerCase();
  if (!needle) return true;
  return entrySearchText(entry).includes(needle);
}

function entrySearchText(entry) {
  const key = String(entry.key || "").toLocaleLowerCase();
  return entrySearchTextByKey.get(key) || key;
}

function entryDisplayName(key) {
  const normalizedKey = String(key || "").toLocaleLowerCase();
  const file = entryFileByKey.get(normalizedKey);
  if (!file) return key;
  const title = String(file.title || "").trim();
  return title && title !== file.name ? `${title} · ${file.name}` : file.name;
}

function prepareEntryMetadata() {
  entryFileByKey = new Map();
  entrySearchTextByKey = new Map();
  (appState.vpkFiles || []).forEach((file) => {
    const key = addonListKeyForFile(file);
    if (key && !entryFileByKey.has(key)) {
      entryFileByKey.set(key, file);
    }
  });
  currentEntries.forEach((entry) => {
    const key = String(entry.key || "").toLocaleLowerCase();
    const file = entryFileByKey.get(key);
    entrySearchTextByKey.set(
      key,
      `${key} ${file?.name || ""} ${file?.title || ""}`.toLocaleLowerCase()
    );
  });
}
