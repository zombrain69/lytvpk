import { appState } from "../state.js";
import {
  GetAddonListLoadOrderEntries,
  SetVPKLoadOrder,
} from "../../../../wailsjs/go/app/App";

let EventsOn;
let showError;
let CheckConflicts;
let CheckConflictsForPaths;
let CheckConflictsWithOptions;
let toggleFile;
let moveFileToAddons;
let toggleGameEnabled;
let showFileDetail;
let renderFileList;
let showNotification;
let conflictProgressRegistered = false;
let isConflictChecking = false;
let isConflictModalVisible = false;
// 模态框全量扫描与列表级 scoped 扫描生命周期彼此独立。
// 关闭模态框不应让列表扫描变成“过期”状态，否则其 loading 标记可能永远不清理。
let conflictCheckRunId = 0;
let scopedConflictRunId = 0;
let scopedConflictTimer = null;
let scopedConflictInFlight = null;
let scopedConflictQueued = false;
let scopedConflictQueuedSilent = true;
let conflictOrderByPath = new Map();
let conflictOrderByKey = new Map();
let conflictOrderEntryCount = 0;

function normalizeConflictOrderKey(value) {
  return String(value || "")
    .trim()
    .replaceAll("/", "\\")
    .replace(/^\.\\/, "")
    .toLowerCase();
}

function conflictOrderKeys(vpk, file) {
  const keys = [];
  const name = normalizeConflictOrderKey(file?.name || vpk?.name);
  const path = normalizeConflictOrderKey(file?.path || vpk?.path);
  const root = normalizeConflictOrderKey(appState.currentDirectory);
  if (root && path.startsWith(`${root}\\`)) keys.push(path.slice(root.length + 1));
  const location = file?.location || vpk?.location;
  if (location === "workshop" && name) keys.push(`workshop\\${name}`);
  if (location === "disabled" && name) keys.push(`disabled\\${name}`);
  if (name) keys.push(name);
  return [...new Set(keys.filter(Boolean))];
}

async function refreshConflictOrderMap() {
  try {
    const entries = await GetAddonListLoadOrderEntries();
    const byKey = new Map((entries || []).map((entry) => [normalizeConflictOrderKey(entry.key), Number(entry.order)]));
    conflictOrderByKey = byKey;
    appState.loadOrderMap.clear();
    byKey.forEach((order, key) => {
      if (Number.isInteger(order) && order > 0) appState.loadOrderMap.set(key, order - 1);
    });
    conflictOrderEntryCount = entries?.length || 0;
    conflictOrderByPath = new Map();
    for (const vpk of appState.allVpkFiles || []) {
      const order = conflictOrderKeys(vpk, vpk).map((key) => byKey.get(key)).find((value) => Number.isInteger(value));
      if (order) conflictOrderByPath.set(vpk.path, order);
    }
    return conflictOrderByPath;
  } catch (error) {
    console.warn("读取冲突 Mod 加载顺序失败:", error);
    conflictOrderEntryCount = 0;
    conflictOrderByPath = new Map();
    conflictOrderByKey = new Map();
    return conflictOrderByPath;
  }
}

function getConflictPriority(vpk) {
  const file = (appState.allVpkFiles || []).find((item) => item.path === vpk.path);
  if (conflictOrderByPath.has(vpk.path)) return conflictOrderByPath.get(vpk.path);
  return conflictOrderKeys(vpk, file).map((key) => conflictOrderByKey.get(key)).find((value) => Number.isInteger(value)) || null;
}

function sortConflictVPKs(vpkFiles) {
  return [...vpkFiles].sort((a, b) => {
    const orderA = getConflictPriority(a);
    const orderB = getConflictPriority(b);
    if (orderA !== null && orderB !== null && orderA !== orderB) return orderA - orderB;
    if (orderA !== null && orderB === null) return -1;
    if (orderA === null && orderB !== null) return 1;
    return String(a.title || a.name || "").localeCompare(String(b.title || b.name || ""), "zh-CN", { numeric: true });
  });
}

function getConflictRelativeTarget(orderedVpkFiles, index, direction) {
  const targetIndex = direction === "before" ? index - 1 : index + 1;
  if (targetIndex < 0 || targetIndex >= orderedVpkFiles.length) return null;
  return orderedVpkFiles[targetIndex] || null;
}

function getConflictRelativeDestination(currentPath, targetVpk, direction) {
  const targetPriority = getConflictPriority(targetVpk);
  if (!Number.isInteger(targetPriority)) return null;

  const currentPriority = getConflictPriority({ path: currentPath });
  const currentIsBeforeTarget = Number.isInteger(currentPriority) && currentPriority < targetPriority;
  let destination = direction === "before" ? targetPriority : targetPriority + 1;

  // SetVPKLoadOrder 会先移除当前条目，再按 1-based 序号插入。
  // 当前条目原本位于目标之前时，目标会在移除后前移一位。
  if (currentIsBeforeTarget) destination -= 1;

  const maxOrder = conflictOrderEntryCount + (Number.isInteger(currentPriority) ? 0 : 1);
  return Math.max(1, Math.min(maxOrder, destination));
}

export function configureConflicts(deps) {
  ({
    EventsOn,
    showError,
    CheckConflicts,
    CheckConflictsForPaths,
    CheckConflictsWithOptions,
    toggleFile,
    moveFileToAddons,
    toggleGameEnabled,
    showFileDetail,
    renderFileList,
    showNotification,
  } = deps);
  registerConflictProgressEvents();
}

let currentConflictResult = null;
let currentSeverityFilter = "critical"; // 默认只显示严重
let currentConflictPage = 1;

const CONFLICT_PAGE_SIZE = 20;

const CONFLICT_SCOPE_RULES = [
  { type: "enabled", label: "游戏内开启", description: "addonlist.txt 为 1 且不在 disabled" },
  { type: "not_disabled", label: "未禁用", description: "文件不在 disabled 目录" },
  { type: "root", label: "根目录", description: "当前 addons 目录下的 Mod" },
  { type: "workshop", label: "创意工坊", description: "workshop 子目录下的 Mod" },
];

function getConflictScopeOptions() {
  const configured = appState.conflictAnalysisOptions || {};
  const rules = Array.isArray(configured.baselineRules) && configured.baselineRules.length
    ? configured.baselineRules
    : [{ type: "enabled" }];
  return {
    matchMode: configured.matchMode === "and" ? "and" : "or",
    baselineRules: rules.map((rule) => ({ type: rule.type, value: rule.value || "" })),
  };
}

function getConflictScopeLabel(options = getConflictScopeOptions()) {
  const labels = options.baselineRules.map((rule) => {
    if (rule.type === "tag") return `标签：${rule.value || "未指定"}`;
    return CONFLICT_SCOPE_RULES.find((item) => item.type === rule.type)?.label || rule.type;
  });
  if (!labels.length) return "游戏内开启";
  return `${options.matchMode === "and" ? "同时满足" : "满足任一"}：${labels.join(options.matchMode === "and" ? " + " : " / ")}`;
}

function getAvailableConflictTags() {
  const tags = new Set();
  for (const file of appState.allVpkFiles || []) {
    if (file.primaryTag) tags.add(String(file.primaryTag));
    for (const tag of file.secondaryTags || []) {
      if (tag) tags.add(String(tag));
    }
  }
  return [...tags].sort((a, b) => a.localeCompare(b, "zh-CN"));
}

function syncConflictScopeDialog() {
  const options = getConflictScopeOptions();
  document.querySelectorAll("[data-conflict-scope-rule]").forEach((input) => {
    const type = input.dataset.conflictScopeRule;
    input.checked = options.baselineRules.some((rule) => rule.type === type);
  });
  const tagRule = options.baselineRules.find((rule) => rule.type === "tag");
  const tagInput = document.getElementById("conflict-scope-tag");
  const tagCheck = document.querySelector('[data-conflict-scope-rule="tag"]');
  if (tagInput) {
    const placeholder = document.createElement("option");
    placeholder.value = "";
    placeholder.textContent = "选择标签…";
    tagInput.replaceChildren(placeholder);
    getAvailableConflictTags().forEach((tag) => {
      const option = document.createElement("option");
      option.value = tag;
      option.textContent = tag;
      tagInput.appendChild(option);
    });
    tagInput.value = tagRule?.value || "";
    tagInput.disabled = !tagCheck?.checked;
  }
  document.querySelectorAll("[data-conflict-match-mode]").forEach((button) => {
    button.classList.toggle("active", button.dataset.conflictMatchMode === options.matchMode);
    button.setAttribute("aria-pressed", button.dataset.conflictMatchMode === options.matchMode ? "true" : "false");
  });
  const targetCount = document.getElementById("conflict-scope-target-count");
  if (targetCount) targetCount.textContent = `${(appState.vpkFiles || []).length} 个当前筛选目标`;
  const preview = document.getElementById("conflict-scope-preview");
  if (preview) preview.textContent = `当前：${getConflictScopeLabel(options)}`;
}

function readConflictScopeDialog() {
  const rules = [];
  document.querySelectorAll("[data-conflict-scope-rule]:checked").forEach((input) => {
    const type = input.dataset.conflictScopeRule;
    if (type === "tag") {
      const value = document.getElementById("conflict-scope-tag")?.value || "";
      if (value) rules.push({ type, value });
      return;
    }
    rules.push({ type });
  });
  return {
    matchMode: document.querySelector("[data-conflict-match-mode].active")?.dataset.conflictMatchMode || "or",
    baselineRules: rules.length ? rules : [{ type: "enabled" }],
  };
}

export function openConflictScopeModal() {
  const modal = document.getElementById("conflict-scope-modal");
  if (!modal) return;
  syncConflictScopeDialog();
  modal.classList.remove("hidden");
}

export function closeConflictScopeModal() {
  document.getElementById("conflict-scope-modal")?.classList.add("hidden");
}

export async function applyConflictScopeOptions() {
  const options = readConflictScopeDialog();
  appState.conflictAnalysisOptions = options;
  appState.conflictAnalysisScopeLabel = getConflictScopeLabel(options);
  closeConflictScopeModal();
  if (appState.conflictAnalysisEnabled) {
    await runScopedConflictAnalysis();
  } else {
    await toggleScopedConflictAnalysis(true);
  }
}

export function showConflictModal(options = {}) {
  const modal = document.getElementById("conflict-modal");
  if (!modal) return;
  isConflictModalVisible = true;
  modal.classList.remove("hidden");
  if (isConflictChecking) {
    document.getElementById("conflict-progress-container")?.classList.remove("hidden");
    return;
  }
  resetConflictModal();
  const titleEl = document.querySelector("#conflict-modal .modal-header h2");
  if (titleEl) titleEl.textContent = options.title || "Mod冲突检测";
  if (options.result) {
    currentSeverityFilter = options.severityFilter || "all";
    updateFilterButtons();
    currentConflictResult = options.result;
    renderConflictResults(options.result);
    // 先用已有缓存立即显示，再异步补齐真实 addonlist.txt 优先级并按优先级重排。
    void refreshConflictOrderMap().then(() => {
      if (currentConflictResult === options.result && isConflictModalVisible) {
        renderConflictResults(options.result);
      }
    });
    return;
  }
  // 自动开始检测
  startConflictCheck();
}

export function hideConflictModal() {
  isConflictModalVisible = false;
  // 使尚未返回的异步扫描结果失效，避免关闭后立即重开时显示旧结果。
  conflictCheckRunId += 1;
  document.getElementById("conflict-modal")?.classList.add("hidden");
  currentConflictResult = null;
  const list = document.getElementById("conflict-list");
  if (list) list.replaceChildren();
  const titleEl = document.querySelector("#conflict-modal .modal-header h2");
  if (titleEl) titleEl.textContent = "Mod冲突检测";
}

function resetConflictModal() {
  document.getElementById("conflict-progress-container")?.classList.add("hidden");
  document.getElementById("conflict-results")?.classList.add("hidden");
  document.getElementById("conflict-empty")?.classList.add("hidden");
  // 隐藏开始按钮，因为自动开始
  const startButton = document.getElementById("start-conflict-check-btn");
  if (startButton) startButton.style.display = "none";
  const list = document.getElementById("conflict-list");
  if (list) list.replaceChildren();
  const progressBar = document.getElementById("conflict-progress-bar");
  if (progressBar) progressBar.style.width = "0%";
  const progressText = document.getElementById("conflict-progress-text");
  if (progressText) progressText.textContent = "准备开始...";
  currentConflictResult = null;

  // 重置筛选状态
  currentSeverityFilter = "critical";
  currentConflictPage = 1;
  updateFilterButtons();
}

function setConflictChecking(checking) {
  isConflictChecking = checking;

  const startButton = document.getElementById("start-conflict-check-btn");
  if (startButton) {
    startButton.disabled = checking;
    startButton.textContent = checking ? "检测中..." : "开始检测";
  }
}

// 与后端保持一致：放宽大筛选范围，同时保留硬上限避免误触发超大扫描。
const SCOPED_CONFLICT_MAX_VPKS = 5000;

function conflictSeverityRank(severity) {
  if (severity === "critical") return 3;
  if (severity === "warning") return 2;
  return 1;
}

function buildScopedConflictSummary(result) {
  const byPath = new Map();
  for (const group of result?.conflict_groups || []) {
    const severity = group.severity || "info";
    for (const vpk of group.vpk_files || []) {
      if (!vpk?.path) continue;
      const previous = byPath.get(vpk.path) || {
        severity: "info",
        groups: 0,
        files: 0,
      };
      previous.groups += 1;
      previous.files += Number(group.file_count || 0);
      if (conflictSeverityRank(severity) > conflictSeverityRank(previous.severity)) {
        previous.severity = severity;
      }
      byPath.set(vpk.path, previous);
    }
  }
  return byPath;
}

function updateScopedConflictControl() {
  const checkbox = document.getElementById("conflict-analysis-checkbox");
  const status = document.getElementById("conflict-analysis-status");
  if (checkbox) {
    checkbox.checked = Boolean(appState.conflictAnalysisEnabled);
    checkbox.disabled = Boolean(appState.conflictAnalysisLoading);
  }
  if (status) {
    if (appState.conflictAnalysisLoading) {
      status.textContent = "分析中…";
      status.className = "conflict-analysis-status loading";
    } else if (appState.conflictAnalysisEnabled) {
      const count = (appState.vpkFiles || []).length;
      const groups = appState.conflictAnalysisResult?.total_conflicts || 0;
      status.textContent = `已分析 ${count} 个筛选目标 · 对比：${appState.conflictAnalysisScopeLabel || "游戏内开启"} · ${groups} 组冲突`;
      status.className = "conflict-analysis-status active";
    } else {
      status.textContent = "默认关闭，按当前筛选分析";
      status.className = "conflict-analysis-status";
    }
  }
}

function updateConflictScopeSummary() {
  const scopeSummary = document.getElementById("conflict-scope-summary");
  if (!scopeSummary) return;
  if (!appState.conflictAnalysisEnabled) {
    scopeSummary.textContent = "全量诊断：当前 addons 与 workshop 中的全部 Mod";
    scopeSummary.title = "全量诊断会扫描当前 addons 与 workshop 中的全部 VPK 文件";
    return;
  }
  const scopeLabel = appState.conflictAnalysisScopeLabel || "游戏内开启";
  const targetCount = (appState.vpkFiles || []).length;
  scopeSummary.textContent = `目标：${targetCount} · 对比：${scopeLabel}`;
  scopeSummary.title = `当前冲突分析范围：${scopeLabel}`;
}

export async function toggleScopedConflictAnalysis(enabled) {
  if (!enabled) {
    if (scopedConflictTimer) {
      clearTimeout(scopedConflictTimer);
      scopedConflictTimer = null;
    }
    scopedConflictQueued = false;
    scopedConflictQueuedSilent = true;
    scopedConflictRunId += 1;
    appState.conflictAnalysisEnabled = false;
    appState.conflictAnalysisLoading = false;
    appState.conflictAnalysisResult = null;
    appState.conflictByPath = new Map();
    updateScopedConflictControl();
    renderFileList?.();
    return;
  }

  return runScopedConflictAnalysis();
}

export function scheduleScopedConflictAnalysis() {
  if (!appState.conflictAnalysisEnabled) return;
  if (scopedConflictTimer) clearTimeout(scopedConflictTimer);

  // 筛选结果已经变化：立即废弃旧结果，避免防抖期间仍显示上一次
  // 筛选范围的冲突标签。递增 run id 也会忽略尚未返回的旧请求。
  scopedConflictRunId += 1;
  appState.conflictAnalysisLoading = true;
  appState.conflictAnalysisResult = null;
  appState.conflictByPath = new Map();
  updateScopedConflictControl();
  renderFileList?.();

  scopedConflictTimer = setTimeout(() => {
    scopedConflictTimer = null;
    runScopedConflictAnalysis({ silent: true });
  }, 450);
}

async function executeScopedConflictAnalysis({ silent = false } = {}) {
  const runId = ++scopedConflictRunId;

  if (typeof CheckConflictsWithOptions !== "function" && typeof CheckConflictsForPaths !== "function") {
    appState.conflictAnalysisLoading = false;
    updateScopedConflictControl();
    renderFileList?.();
    if (!silent) showError?.("当前后端不支持按筛选结果分析冲突，请重新构建应用");
    return;
  }

  const files = [...(appState.vpkFiles || [])];
  if (files.length > SCOPED_CONFLICT_MAX_VPKS) {
    appState.conflictAnalysisEnabled = false;
    appState.conflictAnalysisLoading = false;
    appState.conflictAnalysisResult = null;
    appState.conflictByPath = new Map();
    updateScopedConflictControl();
    renderFileList?.();
    showError?.(`当前筛选包含 ${files.length} 个 Mod；当前最多支持 ${SCOPED_CONFLICT_MAX_VPKS} 个目标，请缩小筛选范围后再分析`);
    return;
  }

  appState.conflictAnalysisEnabled = true;
  appState.conflictAnalysisLoading = true;
  appState.conflictAnalysisResult = null;
  appState.conflictByPath = new Map();
  updateScopedConflictControl();
  renderFileList?.();

  try {
    const configured = getConflictScopeOptions();
    appState.conflictAnalysisScopeLabel = getConflictScopeLabel(configured);
    const result = typeof CheckConflictsWithOptions === "function"
      ? await CheckConflictsWithOptions({
          targetPaths: files.map((file) => file.path),
          baselineRules: configured.baselineRules,
          matchMode: configured.matchMode,
        })
      : await CheckConflictsForPaths(files.map((file) => file.path));
    if (runId !== scopedConflictRunId || !appState.conflictAnalysisEnabled) return;
    appState.conflictAnalysisResult = result || { total_conflicts: 0, conflict_groups: [] };
    appState.conflictByPath = buildScopedConflictSummary(appState.conflictAnalysisResult);
    appState.conflictAnalysisLoading = false;
    updateScopedConflictControl();
    renderFileList?.();
    if (!silent) {
      showNotification?.(
        appState.conflictAnalysisResult.total_conflicts > 0
          ? `已将当前筛选的 ${files.length} 个 Mod 与“${appState.conflictAnalysisScopeLabel}”对比，发现 ${appState.conflictAnalysisResult.total_conflicts} 组潜在冲突`
          : `已将当前筛选的 ${files.length} 个 Mod 与“${appState.conflictAnalysisScopeLabel}”对比，未发现文件冲突`,
        appState.conflictAnalysisResult.total_conflicts > 0 ? "warning" : "success",
      );
    }
  } catch (error) {
    if (runId !== scopedConflictRunId) return;
    const errorMessage = String(error?.message || error || "");
    if (errorMessage.includes("冲突检测正在进行中")) {
      appState.conflictAnalysisLoading = false;
      updateScopedConflictControl();
      renderFileList?.();
      if (!silent) showError?.("当前有其他冲突检测正在进行，请稍候重试");
      return;
    }
    appState.conflictAnalysisEnabled = false;
    appState.conflictAnalysisLoading = false;
    appState.conflictAnalysisResult = null;
    appState.conflictByPath = new Map();
    updateScopedConflictControl();
    renderFileList?.();
    showError?.("分析当前筛选目标与所选对比范围的冲突失败: " + error);
  } finally {
    // 任何异常、过期或组件卸载路径都不能把列表永久留在 pending 状态。
    // 只有当前代次负责清理；更新后的代次由自己的分析或防抖定时器负责。
    if (runId === scopedConflictRunId && appState.conflictAnalysisLoading) {
      appState.conflictAnalysisLoading = false;
      updateScopedConflictControl();
      renderFileList?.();
    }
  }
}

// 将筛选变化触发的分析串行化：后端使用互斥锁，同一时间只允许一轮
// 分析。若分析期间筛选再次变化，只保留最新一轮，避免 TryLock 竞态把
// 正常的“正在等待下一轮”误判成分析失败并关闭开关。
export function runScopedConflictAnalysis(options = {}) {
  if (scopedConflictInFlight) {
    scopedConflictQueued = true;
    scopedConflictQueuedSilent =
      scopedConflictQueuedSilent && Boolean(options.silent);
    return scopedConflictInFlight;
  }

  scopedConflictInFlight = executeScopedConflictAnalysis(options).finally(() => {
    scopedConflictInFlight = null;
    if (scopedConflictQueued && appState.conflictAnalysisEnabled) {
      const silent = scopedConflictQueuedSilent;
      scopedConflictQueued = false;
      scopedConflictQueuedSilent = true;
      queueMicrotask(() => {
        void runScopedConflictAnalysis({ silent });
      });
      return;
    }
    scopedConflictQueued = false;
    scopedConflictQueuedSilent = true;
  });

  return scopedConflictInFlight;
}

export async function showConflictDetailsForFile(filePath) {
  const result = appState.conflictAnalysisResult;
  if (!appState.conflictAnalysisEnabled || appState.conflictAnalysisLoading || !result) {
    showNotification?.(`请先开启并等待当前筛选目标与“${appState.conflictAnalysisScopeLabel || "游戏内开启"}”的冲突分析完成`, "info");
    return;
  }

  const groups = (result.conflict_groups || []).filter((group) =>
    (group.vpk_files || []).some((vpk) => vpk.path === filePath),
  );
  if (groups.length === 0) {
    showNotification?.(`这个 Mod 与“${appState.conflictAnalysisScopeLabel || "游戏内开启"}”没有发现文件冲突`, "success");
    return;
  }

  await refreshConflictOrderMap();
  showConflictModal({
    title: "Mod 冲突详情",
    severityFilter: "all",
    result: { total_conflicts: groups.length, conflict_groups: groups },
  });
}

// 筛选说明文本
const filterDescriptions = {
  critical: "大概率导致客户端崩溃，建议立即处理",
  warning: "可能导致功能异常或显示错误",
  info: "一般性冲突，通常不影响游戏体验",
  all: "显示所有冲突分组",
};

// 更新筛选按钮状态和说明
function updateFilterButtons() {
  document.querySelectorAll(".filter-btn").forEach((btn) => {
    if (btn.dataset.filter === currentSeverityFilter) {
      btn.classList.add("active");
    } else {
      btn.classList.remove("active");
    }
  });
  // 更新说明文本
  const descEl = document.getElementById("conflict-filter-desc");
  if (descEl) {
    descEl.textContent = filterDescriptions[currentSeverityFilter] || "";
  }
}

// 初始化筛选按钮事件
document.addEventListener("DOMContentLoaded", () => {
  document.querySelectorAll(".filter-btn").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      currentSeverityFilter = e.target.dataset.filter;
      currentConflictPage = 1;
      updateFilterButtons();
      if (currentConflictResult) {
        renderConflictResults(currentConflictResult);
      }
    });
  });
});

export async function startConflictCheck() {
  if (isConflictChecking) return;

  const runId = ++conflictCheckRunId;
  setConflictChecking(true);

  document.getElementById("conflict-progress-container")?.classList.remove("hidden");
  document.getElementById("conflict-results")?.classList.add("hidden");
  document.getElementById("conflict-empty")?.classList.add("hidden");
  const list = document.getElementById("conflict-list");
  if (list) list.replaceChildren();
  currentConflictResult = null;

  try {
    const result = await CheckConflicts();
    if (runId !== conflictCheckRunId || !isConflictModalVisible) {
      currentConflictResult = null;
      return;
    }

    currentConflictResult = result;
    currentConflictPage = 1;
    await refreshConflictOrderMap();
    renderConflictResults(result);
  } catch (err) {
    if (runId === conflictCheckRunId && isConflictModalVisible) {
      showError("冲突检测失败: " + err);
      document.getElementById("conflict-progress-container")?.classList.add("hidden");
      const startButton = document.getElementById("start-conflict-check-btn");
      if (startButton) startButton.style.display = "";
    }
  } finally {
    if (runId === conflictCheckRunId) {
      setConflictChecking(false);
    }
  }
}

function renderConflictResults(result) {
  document.getElementById("conflict-progress-container")?.classList.add("hidden");

  updateConflictScopeSummary();

  if (!result || result.total_conflicts === 0) {
    document.getElementById("conflict-results")?.classList.add("hidden");
    document.getElementById("conflict-empty")?.classList.remove("hidden");
    return;
  }

  document.getElementById("conflict-empty")?.classList.add("hidden");
  document.getElementById("conflict-results")?.classList.remove("hidden");


  const list = document.getElementById("conflict-list");
  if (!list) return;
  list.replaceChildren();

  const groups = getFilteredConflictGroups(result);
  const countEl = document.getElementById("conflict-count");
  if (countEl) countEl.textContent = groups.length;
  renderConflictPagination(groups.length);

  if (groups.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    const message = document.createElement("p");
    message.textContent = "当前筛选条件下无冲突";
    empty.appendChild(message);
    list.appendChild(empty);
    return;
  }

  const pageCount = Math.ceil(groups.length / CONFLICT_PAGE_SIZE);
  currentConflictPage = Math.min(Math.max(currentConflictPage, 1), pageCount);

  const start = (currentConflictPage - 1) * CONFLICT_PAGE_SIZE;
  const pageGroups = groups.slice(start, start + CONFLICT_PAGE_SIZE);

  pageGroups.forEach((group) => {
    const groupEl = createConflictGroupElement(group);
    list.appendChild(groupEl);
  });
}

function getFilteredConflictGroups(result) {
  const groups = (result.conflict_groups || []).filter((group) => {
    const severity = group.severity || "info";
    return currentSeverityFilter === "all" || severity === currentSeverityFilter;
  });
  return groups.sort((a, b) => {
    const priorityA = Math.min(...(a.vpk_files || []).map(getConflictPriority).filter(Number.isInteger));
    const priorityB = Math.min(...(b.vpk_files || []).map(getConflictPriority).filter(Number.isInteger));
    const knownA = Number.isFinite(priorityA);
    const knownB = Number.isFinite(priorityB);
    if (knownA && knownB && priorityA !== priorityB) return priorityA - priorityB;
    if (knownA !== knownB) return knownA ? -1 : 1;
    return Number(b.file_count || 0) - Number(a.file_count || 0);
  });
}

function createConflictGroupElement(group) {
  const severity = group.severity || "info";
  const files = group.files || [];
  const fileCount = Number(group.file_count ?? files.length);
  const groupEl = document.createElement("div");
  groupEl.className = `conflict-group ${severity}`;

  const orderedVpkFiles = sortConflictVPKs(group.vpk_files || []);
  const vpkListHtml = orderedVpkFiles
    .map((vpk, index) => {
      const displayName = truncateText(vpk.title || vpk.name);
      const fileName = truncateText(vpk.name);
      const isWorkshop = vpk.location === "workshop";
      const file = (appState.allVpkFiles || []).find((item) => item.path === vpk.path);
      const isDisabled = vpk.location === "disabled";
      const priority = getConflictPriority(vpk);
      const hasPriority = Number.isInteger(priority);
      const previousConflict = getConflictRelativeTarget(orderedVpkFiles, index, "before");
      const nextConflict = getConflictRelativeTarget(orderedVpkFiles, index, "after");
      const canMoveUp = !isDisabled && Boolean(previousConflict && Number.isInteger(getConflictPriority(previousConflict)));
      const canMoveDown = !isDisabled && Boolean(nextConflict && Number.isInteger(getConflictPriority(nextConflict)));
      const previousLabel = previousConflict?.title || previousConflict?.name || "上方冲突 Mod";
      const nextLabel = nextConflict?.title || nextConflict?.name || "下方冲突 Mod";
      const btnText = isWorkshop ? "复制到 addons" : isDisabled ? "启用" : "禁用";
      const btnClass = isWorkshop ? "btn-transfer" : isDisabled ? "btn-enable" : "btn-disable";
      const title = isWorkshop ? "复制到 addons，并关闭 workshop 原件" : isDisabled ? "启用此 Mod" : "禁用此 Mod";
      const gameStateText = file?.gameStateKnown ? (file.gameEnabled ? "游戏开关：开" : "游戏开关：关") : "游戏开关：未记录";

      return `
        <div class="conflict-vpk-item">
          <div class="conflict-vpk-info">
            <span class="conflict-vpk-title" title="${escapeHtml(vpk.title || vpk.name)}">${escapeHtml(displayName)}</span>
            <span class="conflict-vpk-filename" title="${escapeHtml(vpk.name)}">${escapeHtml(fileName)}</span>
            <span class="conflict-vpk-priority ${hasPriority ? "known" : "unknown"}" title="${hasPriority ? "编号来自 addonlist.txt 加载顺序；数字越大通常越靠后加载，覆盖同一资源时更可能生效" : "该 Mod 尚未写入 addonlist.txt"}">${hasPriority ? `优先级 #${priority}` : "优先级：未写入"}</span>
          </div>
          <div class="conflict-vpk-actions" role="group" aria-label="Mod操作">
            <button type="button" class="btn btn-small btn-conflict-action btn-conflict-order" data-path="${escapeHtml(vpk.path)}" data-target-path="${escapeHtml(previousConflict?.path || "")}" data-order-direction="before" data-default-label="↑ 提前" ${canMoveUp ? "" : "disabled"} title="${canMoveUp ? `提前到“${escapeHtml(previousLabel)}”之前` : isDisabled ? "disabled 目录中的 Mod 需先启用后调整优先级" : previousConflict ? "上方冲突 Mod 尚未取得有效优先级" : "已经是当前冲突列表最上方"}">↑ 提前</button>
            <button type="button" class="btn btn-small btn-conflict-action btn-conflict-order" data-path="${escapeHtml(vpk.path)}" data-target-path="${escapeHtml(nextConflict?.path || "")}" data-order-direction="after" data-default-label="↓ 延后" ${canMoveDown ? "" : "disabled"} title="${canMoveDown ? `延后到“${escapeHtml(nextLabel)}”之后；通常提高其覆盖优先级` : isDisabled ? "disabled 目录中的 Mod 需先启用后调整优先级" : nextConflict ? "下方冲突 Mod 尚未取得有效优先级" : "已经是当前冲突列表最下方"}">↓ 延后</button>
            <button type="button" class="btn btn-small btn-conflict-action btn-conflict-detail" data-path="${escapeHtml(vpk.path)}" title="查看 Mod 详情">详情</button>
            <button type="button" class="btn btn-small btn-conflict-action btn-conflict-game" data-path="${escapeHtml(vpk.path)}" ${isDisabled || !file ? "disabled" : ""} title="${isDisabled ? "文件位于 disabled 目录，无法编辑游戏开关" : "编辑 addonlist.txt 中的游戏开关"}">${gameStateText}</button>
            <button type="button" class="btn btn-small btn-conflict-action ${btnClass}" data-path="${escapeHtml(vpk.path)}" data-location="${escapeHtml(vpk.location)}" data-default-label="${escapeHtml(btnText)}" title="${title}">${btnText}</button>
          </div>
        </div>
      `;
    })
    .join("");

    // 严重程度标签文本
    let severityText = "普通";
    if (severity === "critical") severityText = "严重";
    if (severity === "warning") severityText = "警告";

    groupEl.innerHTML = `
            <div class="conflict-header">
                <div class="conflict-title-section">
                    <div class="conflict-severity-row">
                        <span class="severity-badge ${severity}">${severityText}</span>
                        <span class="conflict-file-count">${fileCount} 个冲突文件</span>
                    </div>
                    <div class="conflict-vpk-names">
                        ${vpkListHtml}
                    </div>
                </div>
            </div>
            <div class="conflict-details">
                <div class="conflict-details-inner"></div>
            </div>
        `;

    // 点击展开/收起
    const header = groupEl.querySelector(".conflict-header");
    const details = groupEl.querySelector(".conflict-details");

    header.addEventListener("click", () => {
      if (!details.classList.contains("expanded")) {
        loadConflictDetails(details, group);
      }
      details.classList.toggle("expanded");
    });

    // 添加冲突 Mod 的详情、游戏开关和文件启用/禁用操作
    groupEl.querySelectorAll(".btn-conflict-action").forEach((btn) => {
      btn.addEventListener("click", async (e) => {
        e.stopPropagation(); // 阻止触发 header click

        const path = btn.dataset.path;
        const location = btn.dataset.location;

        try {
          btn.disabled = true;
          const action = btn.classList.contains("btn-conflict-detail")
            ? "detail"
            : btn.classList.contains("btn-conflict-game")
              ? "game"
              : btn.classList.contains("btn-conflict-order")
                ? "order"
              : "file";

          if (action === "detail") {
            showFileDetail?.(path);
            btn.disabled = false;
            return;
          }
          if (action === "game") {
            await toggleGameEnabled?.(path);
            btn.disabled = false;
            if (appState.conflictAnalysisEnabled) await runScopedConflictAnalysis({ silent: true });
            return;
          }
          if (action === "order") {
            const direction = btn.dataset.orderDirection;
            const targetPath = btn.dataset.targetPath;
            const targetVpk = (currentConflictResult?.conflict_groups || [])
              .flatMap((item) => item.vpk_files || [])
              .find((item) => item.path === targetPath);
            if (!targetVpk || (direction !== "before" && direction !== "after")) {
              throw new Error("未找到对应的冲突目标 Mod，请刷新冲突分析后重试");
            }
            const nextPriority = getConflictRelativeDestination(path, targetVpk, direction);
            if (!Number.isInteger(nextPriority)) {
              throw new Error("冲突目标 Mod 尚未取得有效的 addonlist.txt 优先级");
            }
            await SetVPKLoadOrder(path, nextPriority);
            await refreshConflictOrderMap();
            renderFileList?.();
            renderConflictResults(currentConflictResult);
            showNotification?.(
              direction === "before"
                ? "Mod 已提前到上方冲突 Mod 之前"
                : "Mod 已延后到下方冲突 Mod 之后，覆盖优先级通常更高",
              "success",
            );
            return;
          }

          btn.textContent = "处理中...";

          if (location === "workshop") {
            // workshop 文件复制到插件目录，并保留原件作为受保护的关闭来源
            await moveFileToAddons(path);
          } else {
            // 其他位置直接禁用
            await toggleFile(path);
          }

          // 保留当前分析模式：范围分析完成后不要意外切回全量诊断。
          if (appState.conflictAnalysisEnabled) {
            await runScopedConflictAnalysis({ silent: true });
          } else {
            await startConflictCheck();
          }
        } catch (err) {
          showError("操作失败: " + err);
          // 恢复按钮状态
          btn.textContent = btn.dataset.defaultLabel || "重试";
          btn.disabled = false;
        }
      });
    });

  return groupEl;
}

function loadConflictDetails(details, group) {
  if (details.dataset.loaded === "true" || details.dataset.loading === "true") {
    return;
  }

  details.dataset.loading = "true";
  const inner = details.querySelector(".conflict-details-inner");
  if (!inner) {
    delete details.dataset.loading;
    return;
  }
  inner.innerHTML = '<div class="file-tree-loading">正在加载文件树...</div>';

  requestAnimationFrame(() => {
    const tree = buildTree(group.files || []);
    const fileCount = Number(group.file_count ?? (group.files || []).length);
    const truncatedNote = group.files_truncated
      ? `<div class="file-tree-note">当前仅展示前 ${(group.files || []).length} 个文件，完整冲突数为 ${fileCount} 个。</div>`
      : "";
    inner.innerHTML = `<div class="file-tree">${truncatedNote}${renderTree(tree)}</div>`;
    details.dataset.loaded = "true";
    delete details.dataset.loading;
  });
}

function buildTree(paths) {
  const root = [];

  paths.forEach((path) => {
    const normalizedPath = String(path || "");
    const parts = normalizedPath.replace(/\\/g, "/").split("/").filter(Boolean);
    let currentLevel = root;

    parts.forEach((part, index) => {
      const isFile = index === parts.length - 1;
      let node = currentLevel.find((n) => n.name === part);

      if (!node) {
        node = {
          name: part,
          type: isFile ? "file" : "folder",
          children: [],
          path: isFile ? normalizedPath : null,
        };
        currentLevel.push(node);
      }

      if (!isFile) currentLevel = node.children;
    });
  });

  return root;
}

function renderTree(nodes) {
  nodes.sort((a, b) => {
    if (a.type !== b.type) return a.type === "folder" ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  return nodes
    .map((node) => {
      if (node.type === "folder") {
        return `
          <div class="tree-folder">
            <div class="tree-folder-name">
              <span class="folder-icon">${folderIconSvg()}</span>
              <span class="tree-node-name">${escapeHtml(node.name)}</span>
            </div>
            <div class="tree-children">
              ${renderTree(node.children)}
            </div>
          </div>
        `;
      }

      const category = getFileCategory(node.path);
      return `
        <div class="tree-file">
          <span class="file-tag ${category.className}">${category.label}</span>
          <span class="tree-node-name">${escapeHtml(node.name)}</span>
        </div>
      `;
    })
    .join("");
}

function renderConflictPagination(totalCount) {
  let pagination = document.getElementById("conflict-pagination");

  if (!pagination) {
    const toolbar = document.querySelector("#conflict-results .conflict-toolbar");
    pagination = document.createElement("div");
    pagination.id = "conflict-pagination";
    pagination.className = "conflict-pagination";
    toolbar?.appendChild(pagination);
  }

  const pageCount = Math.ceil(totalCount / CONFLICT_PAGE_SIZE);
  if (pageCount <= 1) {
    pagination.classList.add("hidden");
    pagination.innerHTML = "";
    return;
  }

  currentConflictPage = Math.min(Math.max(currentConflictPage, 1), pageCount);

  const visiblePages = getVisiblePageNumbers(currentConflictPage, pageCount);
  const start = (currentConflictPage - 1) * CONFLICT_PAGE_SIZE + 1;
  const end = Math.min(currentConflictPage * CONFLICT_PAGE_SIZE, totalCount);

  pagination.classList.remove("hidden");
  pagination.innerHTML = `
    <div class="conflict-pagination-info">${start}-${end} / ${totalCount}</div>
    <div class="conflict-pagination-controls">
      <button class="conflict-page-btn" data-page="${currentConflictPage - 1}" ${currentConflictPage === 1 ? "disabled" : ""}>上一页</button>
      ${visiblePages
        .map((page) =>
          page === "..."
            ? '<span class="conflict-page-ellipsis">...</span>'
            : `<button class="conflict-page-btn ${page === currentConflictPage ? "active" : ""}" data-page="${page}">${page}</button>`,
        )
        .join("")}
      <button class="conflict-page-btn" data-page="${currentConflictPage + 1}" ${currentConflictPage === pageCount ? "disabled" : ""}>下一页</button>
    </div>
  `;

  pagination.querySelectorAll(".conflict-page-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const page = Number(btn.dataset.page);
      if (!page || page === currentConflictPage) return;

      currentConflictPage = Math.min(Math.max(page, 1), pageCount);
      renderConflictResults(currentConflictResult);
    });
  });
}

function getVisiblePageNumbers(currentPage, pageCount) {
  if (pageCount <= 7) {
    return Array.from({ length: pageCount }, (_, index) => index + 1);
  }

  const pages = [1];
  const start = Math.max(2, currentPage - 1);
  const end = Math.min(pageCount - 1, currentPage + 1);

  if (start > 2) pages.push("...");
  for (let page = start; page <= end; page++) pages.push(page);
  if (end < pageCount - 1) pages.push("...");

  pages.push(pageCount);
  return pages;
}

function truncateText(text, maxLen = 25) {
  if (!text || text.length <= maxLen) return text || "";
  return text.substring(0, maxLen - 2) + "..";
}

function escapeHtml(value) {
  return String(value ?? "").replace(/[&<>"']/g, (char) => {
    const entities = {
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    };
    return entities[char];
  });
}

function registerConflictProgressEvents() {
  if (conflictProgressRegistered || !EventsOn) return;
  conflictProgressRegistered = true;
  EventsOn("conflict_check_progress", (progress) => {
    const bar = document.getElementById("conflict-progress-bar");
    const text = document.getElementById("conflict-progress-text");
  
    if (bar && text) {
      if (progress.total > 0) {
        const percent = (progress.current / progress.total) * 100;
        bar.style.width = percent + "%";
      }
      text.textContent = progress.message;
    }
  });
}

function folderIconSvg() {
  return `<svg class="tree-icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7h7l2 2h9v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"></path><path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h4"></path></svg>`;
}

function getFileCategory(filePath) {
  const lower = filePath.toLowerCase().replace(/\\/g, "/");

  // 🔴 严重 (Critical)
  if (lower === "particles/particles_manifest.txt") {
    return { label: "全局特效", className: "tag-critical" };
  }
  if (lower === "scripts/soundmixers.txt") {
    return { label: "全局混音", className: "tag-critical" };
  }
  if (lower.endsWith(".bsp")) {
    return { label: "地图文件", className: "tag-critical" };
  }
  if (lower.endsWith(".nav")) {
    return { label: "导航网格", className: "tag-critical" };
  }
  if (lower.startsWith("missions/") && lower.endsWith(".txt")) {
    return { label: "任务脚本", className: "tag-critical" };
  }
  if (lower.startsWith("scripts/") && lower.endsWith(".txt")) {
    if (lower.startsWith("scripts/vscripts/")) {
      return { label: "VScript", className: "tag-warning" };
    }
    return { label: "核心脚本", className: "tag-critical" };
  }

  // 🟡 告警 (Warning)
  if (lower === "sound/sound.cache") {
    return { label: "音频缓存", className: "tag-warning" };
  }
  if (lower.endsWith(".phy")) {
    return { label: "物理模型", className: "tag-warning" };
  }
  if (lower.startsWith("resource/") && lower.endsWith(".res")) {
    return { label: "界面资源", className: "tag-warning" };
  }
  if (lower.startsWith("scripts/vscripts/")) {
    return { label: "VScript", className: "tag-warning" };
  }
  if (
    lower.endsWith(".vscript") ||
    lower.endsWith(".nut") ||
    lower.endsWith(".nuc")
  ) {
    return { label: "VScript", className: "tag-warning" };
  }
  if (lower.endsWith(".db")) {
    return { label: "数据库", className: "tag-warning" };
  }
  if (lower.endsWith(".vtx") || lower.endsWith(".vvd")) {
    return { label: "模型数据", className: "tag-warning" };
  }
  if (lower.endsWith(".ttf") || lower.endsWith(".otf")) {
    return { label: "字体文件", className: "tag-warning" };
  }

  // 🟢 一般 (Info)
  if (lower.endsWith(".vtf")) {
    return { label: "纹理", className: "tag-info" };
  }
  if (lower.endsWith(".vmt")) {
    return { label: "材质", className: "tag-info" };
  }
  if (lower.endsWith(".mdl")) {
    return { label: "模型", className: "tag-info" };
  }
  if (lower.endsWith(".wav") || lower.endsWith(".mp3")) {
    return { label: "音频", className: "tag-info" };
  }
  if (lower.endsWith(".cfg")) {
    return { label: "配置", className: "tag-info" };
  }

  return { label: "其他", className: "tag-info" };
}
