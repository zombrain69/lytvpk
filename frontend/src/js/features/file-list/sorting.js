import { appState } from "../state.js";
import { showNotification, showError } from "../../core/toast.js";
import { saveConfig } from "../../core/config.js";
import { renderFileList, revealFileByPath } from "./render.js";
import { GetAddonListOrder, GetModPriorityPlan } from "../../../../wailsjs/go/app/App";
import { buildPriorityPlanMap } from "./priority-label.mjs";
import { applySortOrder, compareByPriority, compareNames, nextSortState } from "./priority-sort.mjs";
import { modelMetricScanTargets } from "./model-metric-targets.mjs";

let loadOrderHighlightTimer = null;

// saveSortPreference 把当前排序写进 config.json（对齐上游 7b0818c）。
// 排序是"用户习惯"：不持久化的话每次重启都回到默认排序，用户得重新点一遍。
// 写盘失败只提示、不打断排序本身（本次会话内的排序已经生效）。
export function saveSortPreference() {
  return saveConfig({
    sortType: appState.sortType,
    sortOrder: appState.sortOrder,
  }).catch((error) => {
    showError("保存排序设置失败: " + error);
  });
}

export function setupSortEvents() {
  const sortBtn = document.getElementById("sort-btn");
  const dropdown = document.getElementById("sort-dropdown-content");

  if (sortBtn && dropdown) {
    sortBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      dropdown.classList.toggle("hidden");
    });

    document.addEventListener("click", (e) => {
      if (!sortBtn.contains(e.target) && !dropdown.contains(e.target)) {
        dropdown.classList.add("hidden");
      }
    });
  }

  document
    .getElementById("sort-name-btn")
    ?.addEventListener("click", () => handleSortChange("name"));
  document
    .getElementById("sort-date-btn")
    ?.addEventListener("click", () => handleSortChange("date"));
  document
    .getElementById("sort-size-btn")
    ?.addEventListener("click", () => handleSortChange("size"));
  document
    .getElementById("sort-model-complexity-btn")
    ?.addEventListener("click", () => handleModelComplexitySort());
  document
    .getElementById("sort-load-order-btn")
    ?.addEventListener("click", () => handleLoadOrderSort());

  setupLoadOrderLocator();

  updateSortButtonUI();
}

function setupLoadOrderLocator() {
  const input = document.getElementById("load-order-locate-input");
  const button = document.getElementById("load-order-locate-btn");
  if (!input || !button) return;

  const locate = () => locateFileByLoadOrder(input);
  button.addEventListener("click", locate);
  input.addEventListener("keydown", (event) => {
    if (event.key !== "Enter") return;
    event.preventDefault();
    locate();
  });
}

async function locateFileByLoadOrder(input) {
  const priority = Number(input.value);
  if (!Number.isSafeInteger(priority) || priority < 1) {
    showError("请输入大于 0 的优先级编号");
    input.focus();
    return;
  }

  try {
    await refreshLoadOrderMap();
    const targetIndex = priority - 1;
    const allFiles = appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
    const targetFile = allFiles.find((file) => getFileLoadOrderIndex(file) === targetIndex);
    if (!targetFile) {
      showNotification(`优先级 #${priority} 当前没有扫描到对应的 Mod`, "info");
      return;
    }

    const isVisible = (appState.vpkFiles || []).some((file) => file.path === targetFile.path);
    if (!isVisible) {
      showNotification(`当前筛选隐藏了优先级 #${priority} 的 Mod，请清除筛选后再定位`, "info");
      return;
    }

    // 回收式窗口下按索引定位：直接把窗口搬到目标行附近，不再整表同步补齐
    // （旧实现大库上要先造出几千张卡，实测是一次几百毫秒的冻结）。
    const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches;
    const element = revealFileByPath(targetFile.path, {
      behavior: reducedMotion ? "auto" : "smooth",
      block: "center",
    });
    if (!element) {
      showNotification("列表正在更新，请稍后再次定位", "info");
      return;
    }

    document.querySelectorAll(".load-order-locate-highlight").forEach((item) => item.classList.remove("load-order-locate-highlight"));
    element.classList.add("load-order-locate-highlight");
    if (loadOrderHighlightTimer) window.clearTimeout(loadOrderHighlightTimer);
    loadOrderHighlightTimer = window.setTimeout(() => {
      element.classList.remove("load-order-locate-highlight");
      loadOrderHighlightTimer = null;
    }, 2200);
  } catch (error) {
    console.error("按优先级定位 Mod 失败:", error);
    showError("读取 addonlist.txt 优先级失败: " + error);
  }
}

export async function handleLoadOrderSort() {
  document.getElementById("sort-dropdown-content")?.classList.add("hidden");

  try {
    const orderList = await refreshLoadOrderMap();
    console.log("获取到加载顺序:", orderList.length, "个条目");

    // 与其它排序一致：再点一次就在 顺序 / 倒序 之间切换
    //（上游与早期实现都写死 asc，导致"优先级排序（倒序）"这条入口不可达）。
    const next = nextSortState(appState.sortType, appState.sortOrder, "loadOrder");
    appState.sortType = next.type;
    appState.sortOrder = next.order;
    saveSortPreference();

    updateSortButtonUI();
    applySort(appState.vpkFiles);
    renderFileList();

    showNotification(
      `已按优先级排序（${appState.sortOrder === "asc" ? "顺序" : "倒序"}）`,
      "success",
    );
  } catch (err) {
    console.error("获取加载顺序失败:", err);
    showError("addonlist.txt 错误: " + err);
  }
}

export function handleSortChange(type) {
  const next = nextSortState(appState.sortType, appState.sortOrder, type);
  appState.sortType = next.type;
  appState.sortOrder = next.order;
  saveSortPreference();

  updateSortButtonUI();
  document.getElementById("sort-dropdown-content")?.classList.add("hidden");

  applySort(appState.vpkFiles);
  renderFileList();
}

/**
 * fetchModelMetrics 调后端取模型指标（后端对"已分析过"的文件直接回内存里的值，
 * 不会重新打开 VPK；冷启动时才会真正扫盘，且结果会随扫描缓存落盘）。
 */
async function fetchModelMetrics(paths) {
  const method = window?.go?.app?.App?.GetVPKModelMetrics;
  if (typeof method !== "function" || !Array.isArray(paths) || paths.length === 0) return [];
  return (await method(paths)) || [];
}

// 本会话内分析失败过的路径（典型：扩展名是 .vpk、实际是压缩包的包）。
// 记住它们，避免每次筛选变化都重试 + 重排；用户显式再点一次「模型复杂度排序」会清空重试。
const modelMetricFailedPaths = new Set();

/** applyModelMetrics 把后端返回的指标写回 appState 里的文件对象（列表两处都写）。 */
function applyModelMetrics(metrics) {
  const metricMap = new Map((metrics || []).map((metric) => [metric.path, metric]));
  [appState.allVpkFiles, appState.vpkFiles].forEach((files) => {
    (files || []).forEach((file) => {
      const metric = metricMap.get(file.path);
      if (!metric) return;
      if (metric.error) {
        modelMetricFailedPaths.add(file.path);
        return;
      }
      file.modelStatsKnown = true;
      file.modelCount = metric.modelCount || 0;
      file.modelVertices = metric.totalVertices || 0;
      file.modelTriangles = metric.totalTriangles || 0;
    });
  });
}

let modelMetricEnsureRun = 0;

/**
 * ensureVisibleModelMetrics 在"模型复杂度排序"处于激活状态时，保证当前可见结果的
 * 指标已就绪。筛选/搜索改变可见集合后调用：命中的文件是纯内存操作，只有新出现的
 * 文件才真的去扫盘；补齐后重排一次，避免新文件因缺指标被当成 0 挤到一端。
 */
export async function ensureVisibleModelMetrics() {
  if (appState.sortType !== "modelComplexity") return;
  const paths = (appState.vpkFiles || []).map((file) => String(file?.path || "")).filter(Boolean);
  if (paths.length === 0) return;
  const knownByPath = new Map(
    (appState.allVpkFiles || []).map((file) => [file.path, Boolean(file.modelStatsKnown)]),
  );
  const unknownCount = paths.filter(
    (path) => !knownByPath.get(path) && !modelMetricFailedPaths.has(path),
  ).length;
  // 全部已就绪：连 IPC 都不用发（筛选来回切时这是常态）。
  if (unknownCount === 0) return;
  const quiet = unknownCount <= 50;
  if (!quiet) {
    showNotification(
      `当前筛选带出 ${unknownCount} 个还没算过模型指标的 Mod，正在后台补齐…`,
      "info",
    );
  }
  const runId = ++modelMetricEnsureRun;
  try {
    const metrics = await fetchModelMetrics(paths);
    if (runId !== modelMetricEnsureRun || appState.sortType !== "modelComplexity") return;
    applyModelMetrics(metrics);
    applySort(appState.vpkFiles);
    renderFileList();
    if (!quiet) {
      showNotification(`已补齐 ${unknownCount} 个 Mod 的模型指标，列表已按复杂度重排`, "success");
    }
  } catch (error) {
    console.warn("补齐模型复杂度指标失败（下次排序时会重试）:", error);
  }
}

export async function handleModelComplexitySort() {
  document.getElementById("sort-dropdown-content")?.classList.add("hidden");

  try {
    if (typeof window?.go?.app?.App?.GetVPKModelMetrics !== "function") {
      throw new Error("当前应用未提供模型复杂度统计，请重新构建并启动 LytVPK");
    }

    // 只分析"当前可见结果"（有筛选时）——排序本来也只作用于这份结果；
    // 指标会随扫描缓存落盘，冷启动后命中缓存就不再重扫（见 vpk_model_metrics.go）。
    // 显式点击 = 用户主动重试：清掉"失败过"的记忆，让不可读的文件再试一次。
    modelMetricFailedPaths.clear();
    const paths = modelMetricScanTargets(appState.allVpkFiles, appState.vpkFiles);
    const knownByPath = new Map(
      (appState.allVpkFiles || []).map((file) => [file.path, Boolean(file.modelStatsKnown)]),
    );
    const cachedCount = paths.filter((path) => knownByPath.get(path)).length;
    showNotification(
      `正在分析 ${paths.length} 个 Mod 的模型复杂度（其中 ${cachedCount} 个可命中缓存）...`,
      "info",
    );
    const started = performance.now();
    const metrics = await fetchModelMetrics(paths);
    applyModelMetrics(metrics);
    const elapsedSeconds = Math.max(1, Math.round((performance.now() - started) / 1000));

    const next = nextSortState(appState.sortType, appState.sortOrder, "modelComplexity");
    appState.sortType = next.type;
    appState.sortOrder = next.order;
    saveSortPreference();

    updateSortButtonUI();
    applySort(appState.vpkFiles);
    renderFileList();
    showNotification(
      `已按模型复杂度排序（LOD0 总顶点，${appState.sortOrder === "desc" ? "高到低" : "低到高"}）：` +
        `新分析 ${Math.max(0, paths.length - cachedCount)} 个 · 命中缓存 ${cachedCount} 个 · ${elapsedSeconds}s`,
      "success",
    );
  } catch (error) {
    console.error("模型复杂度排序失败:", error);
    showError("模型复杂度排序失败: " + error);
  }
}

/**
 * 重新读取 addonlist.txt 的顺序映射。
 *
 * 列表排序使用的是 addonlist.txt 的相对键（例如
 * workshop\\123456.vpk），不能只按文件名建立映射，否则根目录与工坊中
 * 同名 VPK 会互相覆盖。该函数单独导出，供加载顺序写入后先同步映射，
 * 再触发完整文件列表刷新。
 */
export async function refreshLoadOrderMap({ silent = false } = {}) {
  try {
    const orderList = await GetAddonListOrder();
    appState.loadOrderMap.clear();
    (orderList || []).forEach((name, index) => {
      const key = normalizeLoadOrderKey(name);
      if (key) {
        appState.loadOrderMap.set(key, index);
      }
    });
    await refreshPriorityPlanMap();
    return orderList || [];
  } catch (error) {
    // 刷新文件列表时 addonlist.txt 可能尚未生成；不能继续沿用旧映射，
    // 否则新扫描到的 Mod 会显示过期的优先级，且新条目永远没有编号。
    appState.loadOrderMap.clear();
    appState.priorityPlanMap = null;
    if (!silent) throw error;
    console.warn("刷新加载顺序映射失败，暂不显示优先级:", error);
    return [];
  }
}

// refreshPriorityPlanMap 拉取统一优先级模型的有效分层明细。
// 失败时清空而不是沿用旧值：宁可不显示分层，也不能展示过期意图。
async function refreshPriorityPlanMap() {
  try {
    const plan = await GetModPriorityPlan();
    appState.priorityPlanMap = buildPriorityPlanMap(plan);
  } catch (error) {
    appState.priorityPlanMap = null;
    console.warn("刷新优先级分层失败，列表仅显示顺序号:", error);
  }
}

function normalizeLoadOrderKey(value) {
  return String(value || "")
    .trim()
    .replaceAll("/", "\\")
    .replace(/^\.\\/, "")
    .toLowerCase();
}

function getFileLoadOrderKeys(file) {
  const keys = [];
  const name = normalizeLoadOrderKey(file?.name);
  const path = normalizeLoadOrderKey(file?.path);
  const root = normalizeLoadOrderKey(appState.currentDirectory);

  if (root && path.startsWith(`${root}\\`)) {
    keys.push(path.slice(root.length + 1));
  }
  if (file?.location === "disabled" && name) {
    // disabled 目录中的文件在受管理的 addonlist 中仍对应其启用前的键。
    keys.push(`disabled\\${name}`);
    keys.push(name);
  } else if (file?.location === "workshop" && name) {
    keys.push(`workshop\\${name}`);
    keys.push(name);
  } else if (name) {
    keys.push(name);
  }

  return [...new Set(keys.filter(Boolean))];
}

export function getFileLoadOrderIndex(file) {
  for (const key of getFileLoadOrderKeys(file)) {
    const index = appState.loadOrderMap.get(key);
    if (index !== undefined) return index;
  }
  return undefined;
}

// getFileEffectiveLayer 返回该 Mod 的有效分层（显式分层或组权重，来自 priority.json）。
// 未加载分层计划或该 Mod 没有记录时返回 undefined，排序会退化为按顺序号。
export function getFileEffectiveLayer(file) {
  const plan = appState.priorityPlanMap;
  if (!plan?.size) return undefined;
  for (const key of getFileLoadOrderKeys(file)) {
    const entry = plan.get(key);
    if (entry && Number.isInteger(entry.effective)) return entry.effective;
  }
  return undefined;
}

// 每种排序两个方向的人话标签（工具栏按钮与菜单方向 chip 共用同一份）。
const SORT_DIRECTION_LABELS = {
  name: { asc: "A-Z", desc: "Z-A" },
  date: { asc: "最旧", desc: "最新" },
  size: { asc: "由小到大", desc: "由大到小" },
  modelComplexity: { asc: "低到高", desc: "高到低" },
  loadOrder: { asc: "顺序", desc: "倒序" },
};

function sortDirectionLabel(type, order) {
  const labels = SORT_DIRECTION_LABELS[type];
  if (!labels) return "";
  return labels[order === "desc" ? "desc" : "asc"] || "";
}

/**
 * applySortMenuItem 更新一个排序菜单项：active 状态 + 方向 chip。
 *
 * chip 的语义：
 *   · 未激活 → 显示"点下去会用的方向"（与 nextSortState 的默认规则一致）；
 *   · 已激活 → 显示当前方向，并高亮；title 里说明"再点一次切换为另一个方向"。
 * 这样"顺序 / 逆序"两个方向在菜单里都是可见、可达的。
 */
function applySortMenuItem(button, type) {
  if (!button) return;
  const active = appState.sortType === type;
  button.classList.toggle("active", active);
  const shownOrder = active
    ? appState.sortOrder
    : nextSortState(appState.sortType, appState.sortOrder, type).order;
  let chip = button.querySelector(".sort-direction-chip");
  if (!chip) {
    chip = document.createElement("span");
    chip.className = "sort-direction-chip";
    button.appendChild(chip);
  }
  chip.textContent = sortDirectionLabel(type, shownOrder);
  chip.classList.toggle("is-current", active);
  const otherOrder = shownOrder === "asc" ? "desc" : "asc";
  button.title = active
    ? `当前按${sortDirectionLabel(type, shownOrder)}排序；再点一次切换为${sortDirectionLabel(type, otherOrder)}`
    : `点击按${sortDirectionLabel(type, shownOrder)}排序`;
}

export function updateSortButtonUI() {
  const btnText = document.getElementById("sort-btn-text");

  let text = "文件名排序";
  if (appState.sortType === "name") text = "文件名排序";
  else if (appState.sortType === "date") text = "更新时间排序";
  else if (appState.sortType === "loadOrder") text = "优先级排序";
  else if (appState.sortType === "size") text = "VPK 大小排序";
  else if (appState.sortType === "modelComplexity") text = "模型复杂度排序";

  const arrow = sortDirectionLabel(appState.sortType, appState.sortOrder);
  if (btnText) btnText.textContent = arrow ? `${text} （${arrow}）` : text;

  applySortMenuItem(document.getElementById("sort-name-btn"), "name");
  applySortMenuItem(document.getElementById("sort-date-btn"), "date");
  applySortMenuItem(document.getElementById("sort-size-btn"), "size");
  applySortMenuItem(document.getElementById("sort-model-complexity-btn"), "modelComplexity");
  applySortMenuItem(document.getElementById("sort-load-order-btn"), "loadOrder");
}

export function applySort(files) {
  const writeBack = (decorated) => {
    for (let index = 0; index < decorated.length; index += 1) files[index] = decorated[index].file;
    return files;
  };
  // 装饰-排序：把"每次比较都要重算"的键（addonlist 键 / 日期 / 文件名小写）先算一次。
  // 真机实测（2298 条）：现算 addonlist 键 84ms、日期排序 65ms 长任务。
  if (appState.sortType === "loadOrder") {
    const decorated = files.map((file) => ({
      file,
      layer: getFileEffectiveLayer(file),
      order: getFileLoadOrderIndex(file),
      name: file?.name || "",
    }));
    // 方向必须在这里生效：上一版改装饰-排序时漏了 desc 取反，
    // 导致"优先级排序（倒序）"点了也不反转（用户 2026-10-07 反馈）。
    decorated.sort((left, right) =>
      applySortOrder(compareByPriority(left, right), appState.sortOrder),
    );
    return writeBack(decorated);
  }
  if (appState.sortType === "date" || appState.sortType === "name") {
    const descending = appState.sortOrder === "desc";
    const decorated = files.map((file) => ({
      file,
      name: String(file?.name || "").toLowerCase(),
      time: file?.lastModified ? new Date(file.lastModified).getTime() : 0,
    }));
    decorated.sort((left, right) => {
      let result = applySortOrder(
        appState.sortType === "date" ? left.time - right.time : compareNames(left, right),
        descending ? "desc" : "asc",
      );
      if (result !== 0) return result;
      // 平局：日期相同按名称（与原实现一致），名称相同按路径，保证顺序确定。
      if (appState.sortType === "date") return compareNames(left, right);
      return String(left.file?.path || "").localeCompare(String(right.file?.path || ""));
    });
    return writeBack(decorated);
  }
  return files.sort((a, b) => {
    let result = 0;

    if (appState.sortType === "size") {
      result = Number(a.size || 0) - Number(b.size || 0);
    } else if (appState.sortType === "modelComplexity") {
      result = Number(a.modelVertices || 0) - Number(b.modelVertices || 0);
    } else {
      // 复用 priority-sort 里那个 Intl.Collator（localeCompare+options 每次调用都会新建 collator）。
      result = compareNames(a, b);
    }

    result = applySortOrder(result, appState.sortOrder);

    if (result === 0) {
      return a.path.localeCompare(b.path);
    }

    return result;
  });
}
