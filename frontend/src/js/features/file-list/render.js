import { appState, applyFileSelectionGesture } from "../state.js";
import {
  formatFileSize,
  getLocationDisplayName,
  getActionButton,
  formatTags,
  getUniqueDisplayTags,
  escapeHtml,
} from "../../core/utils.js";
import { showFileDetail } from "../modals/detail.js";
import { getServers } from "../servers/servers.js";
import {
  getCachedVPKCardPreview,
  loadVPKCardPreview,
  cancelVPKCardPreview,
} from "../shared/vpk-preview-cache.js";
import { getGameStateDisplayModel } from "./unrecorded-game-state.mjs";
import { formatPriorityLabel } from "./priority-label.mjs";
import {
  describeMatchReasons,
  describeSearchResult,
  formatMatchReasonChip,
  highlightMatches,
} from "./search-match.mjs";
import { compiledRegex, parseSearchSyntax, positiveTerms } from "./search-syntax.mjs";
import { collectResultPaths, describeResultCursor } from "./result-cursor.mjs";
import {
  conflictBadgeLevel,
  filePriorityKeys,
  formatConflictBadgeLabel,
  shouldShowConflictBadge,
} from "../conflicts/conflict-badge.mjs";
import {
  buildScopedConflictTitle,
  formatScopedConflictLabel,
  matchesConflictBaseline,
} from "../conflicts/scoped-conflict-summary.mjs";
import {
  formatGroupChip,
  formatGroupChipTitle,
  groupsForFile,
} from "../mod-groups/group-view.mjs";

let cardPreviewObserver = null;
const pendingCardPreviews = new Map();
const checkboxByPath = new Map();

// ── 列表分帧渲染 + 回收式窗口 ─────────────────────────────────────────────
//
// 真机量化（2904 个 Mod、卡片视图）：把整份列表一次性造完是一个 519ms 的长任务，
// 期间点什么都没反应 —— 用户感受到的就是"点一下明显卡顿"。
//
// 两层策略：
//   小列表（< FILE_LIST_WINDOW_MIN_ITEMS）：分帧补齐，最终整份留在 DOM，语义最简单。
//   大列表：上下双向占位块 + 只保留"视口 ± overscan"这一段，滚出窗口的卡片会被回收。
//     真机背景（2026-10-02，2912 张卡）：只追加不回收时滚遍全库 DOM 涨到 133,213 个节点，
//     滚动 p90 17–30ms；回收式窗口把它压到常量级，滚动/悬停不再随"看过多少卡"退化。
//     占位块高度用精确分页公式（见 updateWindowSpacers），总滚动高度恒等于真实列表，
//     所以回收不会让滚动位置跳动。
const FILE_LIST_RENDER_CHUNK = 120;
// 单帧最多占用的毫秒数（时间片）。固定"每帧 120 张"在真机上仍是 ~50ms/帧的抖动，
// 按时间预算交还主线程才能让补齐过程保持流畅（滚动、悬停、点击都不卡）。
const FILE_LIST_RENDER_BUDGET_MS = 8;
// 首帧至少造这么多：保证用户第一眼看到的就是"填满一屏"，而不是两三张卡。
const FILE_LIST_RENDER_FIRST_CHUNK = 30;
// 低于这个条目数就老样子"一路补完"：窗口化的复杂度不值得。
const FILE_LIST_WINDOW_MIN_ITEMS = 300;
// 视口上下各多物化几行，避免快速滚动时露白。
const FILE_LIST_WINDOW_OVERSCAN_ROWS = 3;
// 首帧 / 隐藏页面量不到 clientHeight 时的兜底视口高度（否则只会物化一行）。
const FILE_LIST_WINDOW_FALLBACK_VIEWPORT = 600;

function nowMs() {
  return typeof performance !== "undefined" && performance.now ? performance.now() : Date.now();
}

let pendingListRender = null;

// 窗口化状态：容器里始终是 [上占位块, ...窗口内的节点, 下占位块]，
// 节点按 index 存在 nodes 里，滚出 [start, end) 就回收。
// 选中态 / 光标 / 面板高亮都从 appState 重新落到新建的节点上（见 refreshResultCursorHighlight
// 与 createFileCard 的 checkbox 初始化），所以回收不会丢状态。
let listWindowState = null;

function detachWindowState() {
  const state = listWindowState;
  if (!state) return;
  state.container?.removeEventListener?.("scroll", handleListWindowScroll);
  try {
    state.resizeObserver?.disconnect?.();
  } catch (error) {
    // 观察器随窗口状态一起失效，忽略即可
  }
  if (state.windowResizeHandler && typeof window !== "undefined") {
    window.removeEventListener?.("resize", state.windowResizeHandler);
  }
  // 占位块随窗口一起走；节点本身由调用方决定（replaceChildren / flush 各有自己的路径）。
  state.topSpacer?.remove?.();
  state.bottomSpacer?.remove?.();
  listWindowState = null;
}

/** 卡片网格/列表行：一屏几列、行高多少（占位块与"偏移 ↔ 条目"换算都用它）。 */
function measureListMetrics(container) {
  const isGrid = container.classList?.contains("file-list-grid");
  let columns = 1;
  let rowPitch = 0;
  let gap = 0;
  let itemHeight = 0;
  if (isGrid && typeof getComputedStyle === "function") {
    const style = getComputedStyle(container);
    const tracks = String(style.gridTemplateColumns || "")
      .split(/\s+/)
      .filter((track) => track && track !== "none");
    if (tracks.length > 0) columns = tracks.length;
    gap = parseFloat(style.rowGap) || 16;
    const firstCard = container.querySelector(".file-card");
    const cardHeight = firstCard ? firstCard.getBoundingClientRect().height : 280;
    itemHeight = cardHeight || 280;
    rowPitch = itemHeight + gap;
  } else {
    const firstItem = container.querySelector(".file-item");
    itemHeight = (firstItem ? firstItem.getBoundingClientRect().height : 0) || 56;
    rowPitch = itemHeight;
  }
  return {
    isGrid,
    columns: Math.max(1, columns),
    rowPitch: Math.max(1, rowPitch),
    gap,
    itemHeight: Math.max(1, itemHeight),
  };
}

function createWindowSpacer(isGrid, kind) {
  const spacer = document.createElement("div");
  spacer.className = "file-list-window-spacer";
  spacer.dataset.windowSpacer = kind;
  spacer.setAttribute("aria-hidden", "true");
  if (isGrid) spacer.style.gridColumn = "1 / -1";
  spacer.style.display = "none";
  return spacer;
}

function setSpacerHeight(spacer, height) {
  if (!spacer) return;
  const safe = Math.max(0, Number(height) || 0);
  spacer.style.height = `${safe}px`;
  // 高度为 0 时必须真正退出网格流：否则它自己仍占一个网格行 + 一个 gap，
  // 总高度会比真实列表多 16px，回收时会漂。
  spacer.style.display = safe > 0 ? "" : "none";
}

/**
 * updateWindowSpacers 把上下占位块调成"精确分页高度"。
 *
 * 目标：不管窗口里保留了几行，容器总滚动高度恒等于真实列表高度。
 * 网格模式里占位块自己也是网格项（各占一行、各自带一个 gap），所以：
 *   上占位 = startRow * rowPitch - gap
 *   下占位 = remainingRows * rowPitch - gap
 * 结果为 0 时整块退流，公式回到"没有占位块"的真实高度。
 */
function updateWindowSpacers(state) {
  if (!state) return;
  const { pending, columns, rowPitch, gap, isGrid, topSpacer, bottomSpacer } = state;
  const total = pending.specs.length;
  const totalRows = Math.max(1, Math.ceil(total / columns));
  const startRow = Math.floor(state.start / columns);
  const endRow = Math.ceil(state.end / columns);
  const remainingRows = Math.max(0, totalRows - endRow);
  const topHeight = state.full || !isGrid ? startRow * rowPitch : Math.max(0, startRow * rowPitch - gap);
  const bottomHeight = state.full || !isGrid ? remainingRows * rowPitch : Math.max(0, remainingRows * rowPitch - gap);
  setSpacerHeight(topSpacer, topHeight);
  setSpacerHeight(bottomSpacer, bottomHeight);
}

/** 视口 ± overscan 需要哪些条目（回收式窗口的核心换算）。 */
function computeWindowRange(state) {
  const { pending, columns, rowPitch, container } = state;
  const total = pending.specs.length;
  if (total === 0) return { start: 0, end: 0 };
  const minRows = FILE_LIST_WINDOW_OVERSCAN_ROWS * 2 + 1;
  if (state.full || total <= columns * minRows) return { start: 0, end: total };

  const totalRows = Math.max(1, Math.ceil(total / columns));
  const scrollTop = Math.max(0, Number(container.scrollTop) || 0);
  const viewport = Math.max(Number(container.clientHeight) || 0, FILE_LIST_WINDOW_FALLBACK_VIEWPORT);
  const firstVisibleRow = Math.floor(scrollTop / rowPitch);
  const visibleRows = Math.max(1, Math.ceil(viewport / rowPitch) + 1);
  const startRow = Math.max(0, firstVisibleRow - FILE_LIST_WINDOW_OVERSCAN_ROWS);
  const endRow = Math.min(totalRows, firstVisibleRow + visibleRows + FILE_LIST_WINDOW_OVERSCAN_ROWS);
  const start = Math.min(total - 1, startRow * columns);
  const end = Math.min(total, Math.max(start + columns, endRow * columns));
  return { start, end };
}

/**
 * renderListWindow 把 DOM 调整成 [上占位块, 窗口内节点..., 下占位块]。
 *
 * 窗口永远是一段连续区间，所以只需要"头部前插 / 尾部后插 / 回收窗口外节点"，
 * 不需要整段重建；节点按 index 缓存在 state.nodes 里，滚回去时同一个 index 重新建卡。
 */
function renderListWindow(state, range = computeWindowRange(state), { force = false } = {}) {
  const { container, pending, nodes, bottomSpacer } = state;
  const total = pending.specs.length;
  if (total === 0) return false;
  const start = Math.max(0, Math.min(Number(range.start) || 0, total - 1));
  const end = Math.max(start + 1, Math.min(Number(range.end) || total, total));
  // 记住"视口顶部那一条"的索引：窗口尺寸/列数变化时用它把滚动位置钉回原处
  // （不能现算 —— 重排期间浏览器自身的 scroll anchoring 会先动 scrollTop）。
  state.anchorIndex = windowIndexAtOffset(state, container.scrollTop);
  if (!force && start === state.start && end === state.end) return false;

  // ① 回收窗口外的节点：摘 DOM、断开预览观察、清掉按路径的复选框缓存，别把旧节点留在内存里。
  for (const [index, node] of [...nodes]) {
    if (index >= start && index < end) continue;
    unobserveCardPreview(node);
    const path = pending.paths[index];
    if (path) checkboxByPath.delete(path);
    node.remove?.();
    nodes.delete(index);
  }

  // ② 补齐窗口内的缺口。剩下的节点一定是连续区间，前插/后插即可保持顺序。
  const kept = [...nodes.keys()].sort((a, b) => a - b);
  if (kept.length === 0) {
    const fragment = document.createDocumentFragment();
    for (let index = start; index < end; index += 1) {
      const node = buildListSpecNode(pending.specs[index], pending);
      nodes.set(index, node);
      fragment.appendChild(node);
    }
    container.insertBefore(fragment, bottomSpacer);
  } else {
    const firstKept = kept[0];
    const lastKept = kept[kept.length - 1];
    if (start < firstKept) {
      const anchor = nodes.get(firstKept);
      for (let index = start; index < firstKept; index += 1) {
        const node = buildListSpecNode(pending.specs[index], pending);
        nodes.set(index, node);
        container.insertBefore(node, anchor);
      }
    }
    if (end > lastKept + 1) {
      for (let index = lastKept + 1; index < end; index += 1) {
        const node = buildListSpecNode(pending.specs[index], pending);
        nodes.set(index, node);
        container.insertBefore(node, bottomSpacer);
      }
    }
  }

  state.start = start;
  state.end = end;
  updateWindowSpacers(state);
  refreshResultCursorHighlight();
  return true;
}

/** syncWindowNodes 同一批条目重画（刷新 / 单卡开关 / 复检）时的原地协调，只动窗口内的节点。 */
function syncWindowNodes(state) {
  const { container, pending, nodes } = state;
  const entries = [...nodes.entries()].sort((a, b) => a[0] - b[0]);
  for (const [index, node] of entries) {
    const spec = pending.specs[index];
    if (!spec) continue;
    const desired = spec.node || buildListSpecNode(spec, pending);
    if (desired === node) continue;
    container.replaceChild?.(desired, node);
    nodes.set(index, desired);
  }
}

/** 光标行必须在窗口里；不在就先把它挪进视口，再按新位置重画窗口。 */
function ensureWindowIndex(index, { block = "nearest" } = {}) {
  const state = listWindowState;
  if (!state?.pending) return null;
  const total = state.pending.specs.length;
  if (total === 0) return null;
  const safeIndex = Math.max(0, Math.min(total - 1, Number(index) || 0));
  if (!state.full && safeIndex >= state.start && safeIndex < state.end) {
    return state.nodes.get(safeIndex) || null;
  }
  const container = state.container;
  const viewport = Math.max(Number(container.clientHeight) || 0, FILE_LIST_WINDOW_FALLBACK_VIEWPORT);
  const viewTop = Math.max(0, Number(container.scrollTop) || 0);
  const viewBottom = viewTop + viewport;
  const rowTop = Math.floor(safeIndex / state.columns) * state.rowPitch;
  const rowBottom = rowTop + state.rowPitch;
  let nextTop = viewTop;
  if (block === "center") {
    // 先让目标行露在视口底部附近，余下的一段交给 scrollIntoView 平滑完成。
    nextTop = Math.max(0, rowTop - viewport + state.rowPitch * 2);
  } else if (rowTop < viewTop) {
    nextTop = Math.max(0, rowTop - state.rowPitch);
  } else if (rowBottom > viewBottom) {
    nextTop = Math.max(0, rowBottom - viewport + state.rowPitch);
  }
  if (nextTop !== viewTop) {
    container.scrollTop = nextTop;
  }
  renderListWindow(state, computeWindowRange(state), { force: true });
  return state.nodes.get(safeIndex) || null;
}

function findWindowNodeByPath(container, path) {
  if (!path) return null;
  const state = listWindowState;
  if (state && state.container === container) {
    const index = state.paths.indexOf(path);
    // 窗口状态下索引表就是权威答案：物化了就是那个节点，没物化就是 null，
    // 不要退回 querySelector（那会把"还没画出来"误判成"找不到 DOM"）。
    if (index >= 0) return state.nodes.get(index) || null;
  }
  return (
    container.querySelector?.(
      `.file-item[data-path="${CSS.escape(path)}"], .file-card[data-path="${CSS.escape(path)}"]`,
    ) || null
  );
}

/** 回收再建卡之后，光标行要重新画上（不会滚动页面，滚动由调用方决定）。 */
function refreshResultCursorHighlight() {
  const cursorPath = String(appState.searchCursorPath || "");
  if (!cursorPath) return;
  const container = document.getElementById("file-list");
  if (!container) return;
  container
    .querySelectorAll?.(".file-item.is-result-cursor, .file-card.is-result-cursor")
    ?.forEach((row) => row.classList.remove("is-result-cursor"));
  findWindowNodeByPath(container, cursorPath)?.classList.add("is-result-cursor");
}

function windowIndexAtOffset(state, offset) {
  const row = Math.max(0, Math.floor((Number(offset) || 0) / state.rowPitch));
  return Math.max(0, Math.min(state.pending.specs.length - 1, row * state.columns));
}

function windowOffsetForIndex(state, index) {
  const row = Math.max(0, Math.floor((Number(index) || 0) / state.columns));
  return row * state.rowPitch;
}

/** 窗口尺寸/列数变化（拉窗口、切卡片/列表）后重新量行高，并把滚动位置钉在同一条目上。 */
function attachWindowResizeWatch(state) {
  const remeasure = () => {
    if (listWindowState !== state) return;
    const container = state.container;
    const anchor = Number.isInteger(state.anchorIndex)
      ? state.anchorIndex
      : windowIndexAtOffset(state, container.scrollTop);
    const metrics = measureListMetrics(state.container);
    if (!metrics.columns || !metrics.rowPitch) return;
    const geometryChanged =
      metrics.columns !== state.columns ||
      metrics.rowPitch !== state.rowPitch ||
      metrics.isGrid !== state.isGrid;
    if (!geometryChanged) {
      // 只变了高度：窗口范围可能要跟着变，但锚点不用动。
      renderListWindow(state, computeWindowRange(state));
      return;
    }
    state.columns = metrics.columns;
    state.rowPitch = metrics.rowPitch;
    state.gap = metrics.gap;
    state.isGrid = metrics.isGrid;
    if (state.full) {
      renderListWindow(state, { start: 0, end: state.pending.specs.length }, { force: true });
      return;
    }
    // 先把新几何的窗口/占位块摆好，再落滚动锚点 —— 反过来的话，
    // 新 scrollTop 会在"占位块还是旧几何"的中间态里被浏览器夹断（真机踩过：跳回顶部）。
    renderListWindow(state, computeWindowRange(state), { force: true });
    const maxTop = Math.max(0, container.scrollHeight - container.clientHeight);
    const targetTop = Math.min(windowOffsetForIndex(state, anchor), maxTop);
    if (targetTop !== container.scrollTop) {
      container.scrollTop = targetTop;
      renderListWindow(state, computeWindowRange(state), { force: true });
    }
    notifyFileListMaterialized();
  };
  if (typeof ResizeObserver === "function") {
    try {
      state.resizeObserver = new ResizeObserver(remeasure);
      state.resizeObserver.observe(state.container);
      return;
    } catch (error) {
      state.resizeObserver = null;
    }
  }
  if (typeof window !== "undefined" && typeof window.addEventListener === "function") {
    state.windowResizeHandler = remeasure;
    window.addEventListener("resize", state.windowResizeHandler);
  }
}

function syncListWindow(state = listWindowState) {
  if (!state || state !== listWindowState) return;
  const changed = renderListWindow(state, computeWindowRange(state));
  if (changed) notifyFileListMaterialized();
}

function handleListWindowScroll() {
  const state = listWindowState;
  if (!state || state.scrollPending) return;
  state.scrollPending = true;
  const run = () => {
    state.scrollPending = false;
    if (listWindowState !== state) return;
    syncListWindow(state);
  };
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(run);
  setTimeout(run, 50);
}

/** notifyFileListMaterialized 通知"这一批卡片已经进 DOM"（检查面板靠它重刷行高亮）。 */
function notifyFileListMaterialized() {
  try {
    document.dispatchEvent(new CustomEvent("file-list:materialized"));
  } catch (error) {
    console.warn("广播列表物化事件失败:", error);
  }
}

/**
 * materializeThroughIndex 保证第 index 个条目已经物化。
 *
 * 回收式窗口下不需要"把前面几千条都造出来"：直接把窗口搬到目标行附近即可；
 * 小列表仍然沿用"按时间预算逐段补齐"的老逻辑。
 */
function materializeThroughIndex(index, budgetMs = FILE_LIST_RENDER_BUDGET_MS) {
  if (listWindowState) {
    const node = ensureWindowIndex(index, { block: "nearest" });
    return Boolean(node) || Number(index) < 0;
  }
  const pending = pendingListRender;
  if (!pending) return true;
  const target = Math.min(index + 1, pending.specs.length);
  if (pending.index >= target) return true;
  const started = nowMs();
  while (pending.index < target && nowMs() - started < budgetMs) {
    const before = pending.index;
    appendListRenderChunk(pending, FILE_LIST_RENDER_CHUNK, FILE_LIST_RENDER_BUDGET_MS);
    if (pending.index === before) break;
  }
  notifyFileListMaterialized();
  if (pending.index >= pending.specs.length) {
    pendingListRender = null;
  }
  return pending.index >= target;
}

/** hasPendingFileListRender 还有没有没补齐的列表项（诊断 / 测试用）。 */
export function hasPendingFileListRender() {
  return Boolean(pendingListRender);
}

/**
 * flushFileListRender 同步补齐剩余列表项（显式"我要完整 DOM"的逃生口）。
 *
 * 回收式窗口之后这条路已经很少有人走：整表物化会把 DOM 拉回 13 万节点，
 * 只有确实要"一次拿到所有行"的调用方才该用（按优先级定位已改成按索引定位）。
 * 返回是否真的补过（没补过说明列表本来就已经完整）。
 */
export function flushFileListRender() {
  const state = listWindowState;
  const pending = pendingListRender;
  if (state && pending && state.pending === pending) {
    state.full = true;
    renderListWindow(state, { start: 0, end: pending.specs.length }, { force: true });
    state.topSpacer?.remove?.();
    state.bottomSpacer?.remove?.();
    state.topSpacer = null;
    state.bottomSpacer = null;
    detachWindowState();
    pendingListRender = null;
    notifyFileListMaterialized();
    applySearchResultCursor();
    return true;
  }
  if (!pending) return false;
  pendingListRender = null;
  appendListRenderChunk(pending, pending.specs.length, Infinity);
  notifyFileListMaterialized();
  applySearchResultCursor();
  return true;
}

function cancelPendingListRender() {
  detachWindowState();
  pendingListRender = null;
}

/**
 * revealFileByPath 把某个 Mod 的卡片/行物化出来并滚进视野（按优先级定位用）。
 *
 * 回收式窗口下这是 O(1)：直接把窗口搬到目标行附近画一屏，不再整表同步补齐
 * （旧实现要把前面几千条全造出来，大库上是一次几百毫秒的冻结）。
 * 返回落点节点；找不到（列表正在更新 / 当前筛选没有它）返回 null。
 */
export function revealFileByPath(filePath, { behavior = "auto", block = "center" } = {}) {
  const container = document.getElementById("file-list");
  const path = String(filePath || "");
  if (!container || !path) return null;
  let node = findWindowNodeByPath(container, path);
  if (!node && listWindowState) {
    const index = listWindowState.paths.indexOf(path);
    if (index >= 0) node = ensureWindowIndex(index, { block });
  }
  if (!node) return null;
  node.scrollIntoView?.({ behavior, block });
  return node;
}

function startChunkedListRender(container, specs, mode, panelServersAvailable = false) {
  const previousWindow = listWindowState;
  const paths = specs.map((spec) => String(spec.file?.path || spec.node?.dataset?.path || ""));
  // 同一批条目（刷新 / 单卡开关 / 复检后的角标变化）：原地协调窗口内的节点，
  // 滚动位置与已物化的那一段都保持不动 —— 否则用户滚到中间时点一下开关，
  // 列表会被整批换掉、滚动位置跳回顶部。
  const samePaths = Boolean(
    previousWindow?.container === container &&
      previousWindow.paths &&
      previousWindow.paths.length === paths.length &&
      previousWindow.paths.every((path, index) => path === paths[index]),
  );
  const pending = {
    container,
    specs,
    mode,
    panelServersAvailable,
    index: 0,
    strategy: "replace",
    replaceOnNextAppend: true,
    paths,
  };

  // 空结果集：必须显式清空容器，不能"什么都不做"。
  //
  // appendListRenderChunk 对空 specs 会走 `end <= pending.index` 的短路（0 <= 0），
  // 于是 replace 分支的 replaceChildren 永远不会执行 —— 容器里上一次渲染的卡片会
  // 一直留着。真机现象：筛「游戏内：未记录」而库里 0 个未记录时，筛选条已经高亮、
  // 状态栏也更新了，列表却还是上一批（甚至 30 张「游戏内开启」）卡片；
  // 用户逐张把未记录 Mod 点成「游戏内关闭」点到最后一张时也会停在旧卡片上，
  // 看起来就是"界面卡在这了"。
  if (specs.length === 0) {
    detachWindowState();
    pendingListRender = null;
    container.replaceChildren();
    notifyFileListMaterialized();
    return;
  }

  // 大列表：回收式窗口。首屏就是"一屏 + overscan"，随滚动双向回收，
  // DOM 规模与"看过多少卡"无关（真机 2912 张卡：只追加时 13 万节点）。
  if (specs.length >= FILE_LIST_WINDOW_MIN_ITEMS) {
    if (samePaths && previousWindow) {
      previousWindow.pending = pending;
      previousWindow.paths = paths;
      pendingListRender = pending;
      syncWindowNodes(previousWindow);
      updateWindowSpacers(previousWindow);
      notifyFileListMaterialized();
      return;
    }
    detachWindowState();
    pendingListRender = pending;
    startListWindow(pending);
    notifyFileListMaterialized();
    return;
  }

  // 小列表：沿用"首帧 30 条 + 分帧补齐"，最终整份留在 DOM。
  // 两种落地策略，按"到底要重建多少张卡"来选（真机实测出来的分界）：
  //   patch   —— 变化很小（刷新 / 单个开关 / 复检后少数角标变化）：按位置原地替换。
  //              原来一律 replaceChildren(fragment)：哪怕只改了 1 张卡，也会把 2904 个
  //              节点整批摘下来再插回去（"自动复检后重绘"那一波 121ms 长任务）。
  //   replace —— 变化很大（搜索/筛选换了结果集）：整批插入走一次布局更快。
  // 从回收式窗口切到小列表：先把旧窗口的节点清掉，避免和新的分帧补齐混在一起。
  const previousWasWindow = Boolean(listWindowState);
  detachWindowState();
  if (previousWasWindow) container.replaceChildren();
  pendingListRender = pending;
  let rebuildCount = 0;
  for (const spec of specs) {
    if (!spec.node) rebuildCount += 1;
  }
  const materialized = Math.max(0, container.children.length);
  const sameLength = materialized === specs.length;
  const mostlyRebuilt = rebuildCount * 2 >= specs.length;
  pending.strategy = sameLength && !mostlyRebuilt ? "patch" : "replace";
  pending.replaceOnNextAppend = pending.strategy !== "patch";
  // 第一屏同步造出来：用户看到的还是"点完就有内容"，不是空列表。
  appendListRenderChunk(pending, FILE_LIST_RENDER_FIRST_CHUNK, Infinity);
  if (pending.index >= specs.length) {
    pendingListRender = null;
    return;
  }
  scheduleListRenderStep(pending);
}

/**
 * startListWindow 给大列表装上"回收式窗口"：上下占位块 + 只保留视口 ± overscan 的节点。
 * 首屏直接按当前滚动位置画一屏（不是固定 30 张），否则大屏窗口会露白。
 */
function startListWindow(pending) {
  const container = pending.container;
  const metrics = measureListMetrics(container);
  const topSpacer = createWindowSpacer(metrics.isGrid, "top");
  const bottomSpacer = createWindowSpacer(metrics.isGrid, "bottom");
  const state = {
    container,
    pending,
    paths: pending.paths,
    nodes: new Map(),
    start: 0,
    end: 0,
    columns: metrics.columns,
    rowPitch: metrics.rowPitch,
    gap: metrics.gap,
    isGrid: metrics.isGrid,
    topSpacer,
    bottomSpacer,
    scrollPending: false,
    full: false,
    resizeObserver: null,
    windowResizeHandler: null,
  };
  listWindowState = state;
  // 容器里只放两个占位块，窗口节点稍后插在它们之间。
  container.replaceChildren(topSpacer, bottomSpacer);
  renderListWindow(state, computeWindowRange(state), { force: true });
  container.addEventListener("scroll", handleListWindowScroll, { passive: true });
  attachWindowResizeWatch(state);
}

function scheduleListRenderStep(pending) {
  const step = () => {
    // 期间又渲染了一次（搜索 / 筛选 / 切换视图）：这一批已经过期，直接放弃。
    if (pendingListRender !== pending) return;
    appendListRenderChunk(pending, FILE_LIST_RENDER_CHUNK);
    if (pending.index < pending.specs.length) {
      scheduleListRenderStep(pending);
      return;
    }
    pendingListRender = null;
    // 键盘光标可能落在后面几批才补出来的行上：全部补齐后再画一次。
    applySearchResultCursor();
  };
  // 调度策略：requestAnimationFrame 优先（窗口可见时正好贴着帧走，补齐过程流畅），
  // 同时挂一个 50ms 定时器兜底 —— 窗口最小化 / 被完全遮挡时 rAF 会被节流到几乎不触发，
  // 只靠 rAF 会让列表永远停在半截。两者谁先到谁执行（done 保证只跑一次）。
  let done = false;
  const run = () => {
    if (done) return;
    done = true;
    step();
  };
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(run);
  setTimeout(run, 50);
}

function appendListRenderChunk(pending, limit, budgetMs = FILE_LIST_RENDER_BUDGET_MS) {
  if (pending.strategy === "patch") {
    patchListRenderChunk(pending, limit, budgetMs);
    return;
  }
  // 空结果集（兜底）：replace 策略下也要真的把容器清掉，见 startChunkedListRender 的说明。
  if (pending.specs.length === 0) {
    if (pending.replaceOnNextAppend) {
      pending.replaceOnNextAppend = false;
      pending.container.replaceChildren();
    }
    return;
  }
  const end = Math.min(pending.specs.length, pending.index + limit);
  if (end <= pending.index) return;
  const started = nowMs();
  let appended = 0;
  const fragment = document.createDocumentFragment();
  for (; pending.index < end; pending.index += 1) {
    // 时间片用完就把剩下的交还主线程：下一帧接着补。
    // 至少造 1 个（appended === 0 时不做预算判断），否则会死循环。
    if (appended > 0 && nowMs() - started >= budgetMs) break;
    fragment.appendChild(buildListSpecNode(pending.specs[pending.index], pending));
    appended += 1;
  }
  if (pending.replaceOnNextAppend) {
    pending.replaceOnNextAppend = false;
    pending.container.replaceChildren(fragment);
    return;
  }
  // 回收式窗口不走这条路径（窗口节点由 renderListWindow 维护）；
  // 这里只服务小列表的分帧补齐。
  pending.container.appendChild(fragment);
}

/**
 * patchListRenderChunk 按位置原地协调（keyed 之外最省的一种）：
 * 逐位比较"现有的第 i 个节点"和"第 i 个 spec 应该用的节点"，相同就一个 DOM 操作都不做。
 *
 * 为什么安全：specs 里的节点互不重复，从左到右逐位修正只会把节点搬到它该在的位置
 * （搬走留下的空位由后面的 appendChild 补回来），最后再裁掉多余的行。
 * 代价：只和"真正变化的卡片数 + 需要移动的节点数"成正比，而不是整表规模。
 */
function patchListRenderChunk(pending, limit, budgetMs) {
  const end = Math.min(pending.specs.length, pending.index + limit);
  if (end <= pending.index) return;
  const started = nowMs();
  let handled = 0;
  const container = pending.container;
  for (; pending.index < end; pending.index += 1) {
    // 时间片用完就把剩下的交还主线程（至少处理 1 个，否则会死循环）。
    if (handled > 0 && nowMs() - started >= budgetMs) break;
    const spec = pending.specs[pending.index];
    const node = spec.node || buildListSpecNode(spec, pending);
    const current = container.children[pending.index];
    if (current === node) continue;
    if (current) {
      container.replaceChild(node, current);
    } else {
      container.appendChild(node);
    }
    handled += 1;
  }
  if (pending.index >= pending.specs.length) {
    // 列表变短（或中途搬动留下的空位）时裁掉多余的行。
    while (container.children.length > pending.specs.length) {
      container.lastElementChild?.remove();
    }
  }
}

function buildListSpecNode(spec, pending) {
  if (spec.node) return spec.node;
  if (pending.mode === "card") {
    const card = createFileCard(spec.file, spec.previous || null, pending.panelServersAvailable);
    card.dataset.renderSignature = spec.signature;
    return card;
  }
  return createFileItem(spec.file);
}

function hasPanelServers() {
  return getServers().some((s) => s.panelUrl && s.panelPasswordSet);
}

function getGameStateInfo(file) {
  if (!file.gameStateKnown) {
    return { className: "game-state-unknown", label: "未记录", title: "addonlist.txt 中未记录此 Mod；点击选择游戏内关闭、游戏内启用或禁用" };
  }
  if (file.gameEnabled) {
    return { className: "game-state-enabled", label: "游戏内开启", title: "addonlist.txt：1；点击关闭游戏内 Mod" };
  }
  return { className: "game-state-disabled", label: "游戏内关闭", title: "addonlist.txt：0；点击开启游戏内 Mod" };
}

function applySelectionGesture(filePath, event, selected) {
  const ctrlEnabled = Boolean(appState.ctrlClickSelectionEnabled);
  const changedPaths = applyFileSelectionGesture(filePath, {
    shiftKey: Boolean(event.shiftKey),
    ctrlKey: ctrlEnabled && Boolean(event.ctrlKey),
    metaKey: ctrlEnabled && Boolean(event.metaKey),
    selected,
  });
  const pathsToSync = changedPaths instanceof Set ? changedPaths : new Set([filePath]);
  pathsToSync.forEach((path) => {
    const checkbox = checkboxByPath.get(path);
    if (checkbox) checkbox.checked = appState.selectedFiles.has(path);
  });
  // click 事件在浏览器完成 checkbox 预切换后触发；即使缓存映射暂时为空，
  // 也必须立即同步当前控件，避免用户看到“点击无反应”。
  const currentCheckbox = event.currentTarget?.classList?.contains("file-checkbox")
    ? event.currentTarget
    : event.target?.closest?.(".file-checkbox");
  if (currentCheckbox) currentCheckbox.checked = appState.selectedFiles.has(filePath);
}

function applyModStateClasses(element, file, prefix) {
  const state = getGameStateInfo(file).className.replace("game-state-", "");
  element.classList.add(`${prefix}-state-${state}`);
  if (file.location === "disabled") {
    element.classList.add(`${prefix}-location-disabled`);
  }
}

function getGameStateBadge(file, className = "game-state-badge") {
  const state = getGameStateInfo(file);
  return `<span class="${className} ${state.className}" title="${state.title}">${state.label}</span>`;
}

function getLoadOrderBadge(file, className = "load-order-badge") {
  const order = getLoadOrderValue(file);
  const priority = getFilePriorityEntry(file);
  if (priority) {
    const label = formatPriorityLabel(priority);
    const explanation = priority.source === "group" ? "（分层来自策略组权重）" : "";
    return `<span class="${className}" title="编号来自 addonlist.txt 顺序；分层是显式设置的覆盖意图。数字越大表示越靠后加载，实际覆盖结果还取决于游戏资源与 Mod 规则${explanation}">${escapeHtml(label)}</span>`;
  }
  return Number.isInteger(order)
    ? `<span class="${className}" title="编号来自 addonlist.txt 加载顺序；数字越大表示越靠后加载。实际覆盖结果还取决于游戏资源与 Mod 规则">优先级 #${order + 1}</span>`
    : "";
}

// getConflictRecheckBadge 显示变更驱动自动复检算出的冲突角标。
// 没有角标（未复检过或该 Mod 不参与重叠）时返回空串，不占位。
function getConflictRecheckBadge(file, className = "conflict-recheck-badge") {
  const badge = findConflictRecheckBadge(file);
  if (!shouldShowConflictBadge(badge)) return "";
  const label = formatConflictBadgeLabel(badge);
  if (!label) return "";
  const level = conflictBadgeLevel(badge);
  return `<span class="${className} ${level}" title="来自变更驱动的自动复检；打开“Mod 冲突检测”可查看详情与修复建议">${escapeHtml(label)}</span>`;
}

// findConflictRecheckBadge 先按完整路径命中，再按 addonlist 键命中。
function findConflictRecheckBadge(file) {
  const byPath = appState.conflictBadgeByPath;
  const byKey = appState.conflictBadgeByKey;
  if (!byPath?.size && !byKey?.size) return null;
  const path = String(file?.path || "");
  if (path && byPath?.get(path)) return byPath.get(path);
  for (const key of filePriorityKeys(file, appState.currentDirectory)) {
    const badge = byKey?.get(key);
    if (badge) return badge;
  }
  return null;
}

// getModGroupBadge 显示该 Mod 所属的策略组；单击打开「策略组管理」窗口并定位到这一组
// （由 group-ui.js 代理点击；整组开关在工具栏「分组」菜单与那个窗口的定位条上）。
function getModGroupBadge(file, className = "mod-group-badge") {
  const groups = groupsForFile(file, appState.modGroupIndex, appState.currentDirectory);
  if (groups.length === 0) return "";
  const badges = groups
    .map((membership) => {
      const label = formatGroupChip(membership);
      const title = formatGroupChipTitle(membership);
      return `<button type="button" class="${className}" data-group-id="${escapeHtml(membership.groupId)}" data-file-path="${escapeHtml(file.path)}" title="${escapeHtml(title)}">${escapeHtml(label)}</button>`;
    })
    .join("");
  return `<span class="mod-group-badges">${badges}</span>`;
}

// getFilePriorityEntry 查找该文件在统一优先级模型里的有效分层记录。
// 未加载分层计划或该 Mod 未记录时返回 null，调用方退回纯顺序号展示。
function getFilePriorityEntry(file) {
  const plan = appState.priorityPlanMap;
  if (!plan?.size) return null;
  const name = String(file?.name || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const path = String(file?.path || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const root = String(appState.currentDirectory || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const keys = [];
  if (root && path.startsWith(`${root}\\`)) keys.push(path.slice(root.length + 1));
  if (file?.location === "workshop" && name) keys.push(`workshop\\${name}`);
  if (file?.location === "disabled" && name) keys.push(`disabled\\${name}`);
  if (name) keys.push(name);
  for (const key of [...new Set(keys)]) {
    const entry = plan.get(key);
    if (entry) return entry;
  }
  return null;
}

function getLoadOrderValue(file) {
  if (!appState.loadOrderMap?.size) return undefined;
  const name = String(file?.name || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const path = String(file?.path || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const root = String(appState.currentDirectory || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const keys = [];
  if (root && path.startsWith(`${root}\\`)) keys.push(path.slice(root.length + 1));
  if (file?.location === "workshop" && name) keys.push(`workshop\\${name}`);
  if (file?.location === "disabled" && name) keys.push(`disabled\\${name}`);
  if (name) keys.push(name);
  return [...new Set(keys)]
    .map((key) => appState.loadOrderMap.get(key))
    .find((value) => Number.isInteger(value));
}

function getCardPreviewRevision(file) {
	const revision = String(file?.previewRevision || "").trim();
	if (revision) return revision;
	return `${String(file?.name || "")}\u0000${String(file?.size || "")}\u0000${String(file?.lastModified || "")}`;
}

// The signature contains every value rendered inside a card. Keeping this
// separate from the file object lets a scan return fresh objects without
// forcing Chromium to rebuild hundreds of unchanged image elements.
function getFileCardRenderSignature(file, panelServersAvailable) {
  // 角标内容同样参与签名：分层（优先级 #N（分层 T））与变更驱动复检角标
  // 都是卡片里渲染出来的值，漏掉它们会导致卡片被复用、角标停留在旧状态。
  const priority = getFilePriorityEntry(file);
  const conflictRecheck = findConflictRecheckBadge(file);
  // 「当前筛选 × 对比范围」角标同样要进签名：汇总同时包含冲突组与覆盖组，
  // 只记录冲突数会让“0 组冲突 → 覆盖 9 处”这种变化不触发重绘。
  const scopedConflict = appState.conflictAnalysisEnabled
    ? getScopedConflictBadgeModel(file)
    : null;
  const scopedConflictSignature = scopedConflict
    ? [
        scopedConflict.label,
        scopedConflict.summary
          ? scopedConflict.summary.groups > 0
            ? scopedConflict.summary.severity || "info"
            : "override"
          : "none",
      ]
    : null;
  // 组徽标同样参与签名：加入/移出分组、整组改名后卡片必须重绘。
  const groupBadges = groupsForFile(file, appState.modGroupIndex, appState.currentDirectory)
    .map((membership) => membership.groupId)
    .sort();
  return JSON.stringify({
    name: file.name,
    title: file.title,
    location: file.location,
    enabled: Boolean(file.enabled),
    gameStateKnown: Boolean(file.gameStateKnown),
    gameEnabled: Boolean(file.gameEnabled),
    hasUpdate: Boolean(file.hasUpdate),
    workshopId: file.workshopId || "",
    primaryTag: file.primaryTag || "",
    secondaryTags: file.secondaryTags || [],
    subjectSummary: file.subjectSummary || "",
    xdrSummary: file.xdrSummary || "",
    previewRevision: getCardPreviewRevision(file),
    loadOrder: getLoadOrderValue(file),
    priority: priority ? [priority.order ?? null, priority.tier ?? null, priority.effective ?? null, priority.source ?? null] : null,
    conflictRecheck: conflictRecheck
      ? [conflictRecheck.conflictFiles ?? 0, conflictRecheck.overrideFiles ?? 0, conflictRecheck.severity ?? ""]
      : null,
    groups: groupBadges,
    conflictEnabled: Boolean(appState.conflictAnalysisEnabled),
    conflict: scopedConflictSignature,
    panelServersAvailable,
  });
}

function syncReusedCardSelection(card, file) {
  // 引用在 createFileCard 里就缓存在元素上：复用路径每张卡少一次 querySelector
  // （真机 2904 张卡 = 2904 次查询，实测是"点一下游戏开关"里 200ms 长任务的一部分）。
  card.__file = file;
  const checkbox = card.__selectionCheckbox || card.querySelector(".file-checkbox.card-checkbox");
  if (!checkbox) return;
  checkbox.checked = appState.selectedFiles.has(file.path);
  checkboxByPath.set(file.path, checkbox);
}

function takeExistingCard(file, existingCards, cardsByIdentity) {
  const exact = existingCards.get(file.path);
  if (exact) {
    existingCards.delete(file.path);
    removeCardFromIdentityIndex(exact, cardsByIdentity);
    return exact;
  }

  const identity = getCardPreviewRevision(file);
  const candidates = cardsByIdentity.get(identity);
  while (candidates?.length) {
    const candidate = candidates.shift();
    if (!candidate) continue;
    const oldPath = candidate.dataset.path;
    if (existingCards.get(oldPath) !== candidate) continue;
    existingCards.delete(oldPath);
    return candidate;
  }
  return null;
}

function removeCardFromIdentityIndex(card, cardsByIdentity) {
  if (!card) return;
  const identity = card.dataset.cardIdentity || card.dataset.previewRevision;
  const candidates = cardsByIdentity.get(identity);
  if (!candidates) return;
  const index = candidates.indexOf(card);
  if (index >= 0) candidates.splice(index, 1);
  if (candidates.length === 0) cardsByIdentity.delete(identity);
}

function ensureCardPreviewObservation(card, file) {
  const image = card.__previewImage || card.querySelector(".card-preview-img");
  const placeholder = card.__previewPlaceholder || card.querySelector(".card-preview-placeholder");
  if (!image || !placeholder || image.getAttribute("src")) return;
  if (getCachedVPKCardPreview(file) === undefined) {
    observeCardPreview(card, file, image, placeholder);
  }
}

function getGameToggleButton(file) {
  const state = getGameStateInfo(file);
  const isFileDisabled = file.location === "disabled";
  const label = isFileDisabled ? "游戏开关不可用" : state.label;
  const title = isFileDisabled
    ? "文件位于 disabled 目录，请先恢复文件后再编辑 addonlist.txt"
    : state.title;
  return `
    <button class="btn-small action-btn game-toggle-btn ${state.className}"
            data-file-path="${file.path}" data-action="toggle-game"
            title="${title}" ${isFileDisabled ? "disabled" : ""}>
      <span class="btn-icon">${iconSvg("power")}</span>
      <span class="btn-text">${label}</span>
    </button>
  `;
}

function getGameStateActionIcon(actionId) {
  if (actionId === "game-enabled") return iconSvg("check");
  if (actionId === "game-disabled") return iconSvg("x");
  return iconSvg("power");
}

export function getGameStateActionControls(file) {
  const model = getGameStateDisplayModel(file);
  if (model.mode !== "unrecorded") {
    return '<div class="game-state-controls">' + getGameToggleButton(file) + "</div>";
  }

  const fileInDisabledDirectory = file.location === "disabled";
  const options = model.options
    .map((option) => {
      const disabled = fileInDisabledDirectory || Boolean(option.disabled);
      const disabledReason = fileInDisabledDirectory
        ? "文件位于 disabled 目录，请先恢复文件后再编辑 addonlist.txt"
        : option.disabledReason;
      const title = disabled ? disabledReason : option.description;
      const disabledAttribute = disabled ? " disabled" : "";
      const disabledClass = disabled ? " is-disabled" : "";
      return [
        '<button type="button"',
        ' class="btn-small action-btn game-state-action-btn game-state-action-',
        option.id,
        disabledClass,
        '" data-file-path="',
        escapeHtml(file.path),
        '" data-action="set-game-state" data-game-state="',
        option.id,
        '" title="',
        escapeHtml(title || option.label),
        '"',
        disabledAttribute,
        '>',
        '<span class="btn-icon">',
        getGameStateActionIcon(option.id),
        '</span><span class="btn-text">',
        escapeHtml(option.label),
        '</span></button>',
      ].join("");
    })
    .join("");

  return '<div class="game-state-controls game-state-action-group" role="group" aria-label="未记录 Mod 游戏内状态">' + options + "</div>";
}

// getScopedConflictBadgeModel 把「当前筛选 × 对比范围」的结果整理成角标模型。
// 同时覆盖两类重叠：冲突组（胜负未定）与覆盖组（已按 addonlist 判定）。
function getScopedConflictBadgeModel(file) {
  const summary = appState.conflictByPath?.get(file.path) || null;
  const options = appState.conflictAnalysisOptions || {};
  // 本轮分析是否已有结果：没有结果时角标显示“待分析”，不能谎报“无冲突”。
  const analyzed = Boolean(appState.conflictAnalysisResult);
  const matchedBaseline = matchesConflictBaseline(
    file,
    options.baselineRules,
    options.matchMode,
  );
  return {
    summary,
    matchedBaseline,
    analyzed,
    label: formatScopedConflictLabel(summary, { matchedBaseline, analyzed }),
    title: buildScopedConflictTitle(summary, {
      matchedBaseline,
      analyzed,
      scopeLabel: appState.conflictAnalysisScopeLabel || "游戏内开启",
    }),
  };
}

function getConflictSummaryBadge(file, className = "mod-conflict-badge") {
  if (!appState.conflictAnalysisEnabled) return "";
  // 刻意不在这里渲染"分析中…"占位角标：那会让 2904 张卡在每一轮分析开始时全部重建
  // （真机实测：开关一次冲突分析 = 两次全表重建 = 4 秒的 70ms 抖动帧）。
  // 分析进度由工具栏的状态文字负责（"正在分析…"），卡片角标只在有结果时更新。

  const model = getScopedConflictBadgeModel(file);
  if (!model.summary) {
    // 分析被打断/还没结果时用 pending 样式，避免和“确实没有重叠”混淆。
    const stateClass = model.analyzed ? "none" : "pending";
    return `<span class="${className} ${stateClass}" title="${escapeHtml(model.title)}">${escapeHtml(model.label)}</span>`;
  }

  // 只有覆盖关系（没有未判定冲突）时用 dashed 边框，与列表里的覆盖角标同一语义。
  const level =
    model.summary.groups > 0 ? model.summary.severity || "info" : "override";
  return `
    <button class="${className} has-conflict ${level}"
            data-action="view-conflicts"
            data-file-path="${escapeHtml(file.path)}"
            title="${escapeHtml(model.title)}">
      <span class="mod-conflict-dot" aria-hidden="true"></span>
      ${escapeHtml(model.label)}
    </button>
  `;
}

export function renderFileList() {
  const container = document.getElementById("file-list");
  const listHeader = document.querySelector(".file-list-header");
  const statusBar = document.querySelector(".status-bar");

  if (!container) return;
  checkboxByPath.clear();

  // 「明确的匹配」：搜索框旁边实时给出命中数量，避免用户只看到一屏结果却不知道命中多少。
  const hitCountLabel = document.getElementById("search-hit-count");
  if (hitCountLabel) {
    const shown = appState.vpkFiles?.length || 0;
    const label = describeSearchResult({
      total: appState.allVpkFiles?.length || 0,
      shown,
      query: appState.searchQuery,
      regexInvalid: parseSearchSyntax(appState.searchQuery).regexInvalid,
    });
    hitCountLabel.textContent = label;
    // 键盘光标会在这句话后面追加"第 N / M 个结果"，所以基准文案要单独存一份。
    hitCountLabel.dataset.baseLabel = label;
    hitCountLabel.classList.toggle("has-hits", Boolean(label) && shown > 0);
  }

  if (appState.displayMode === "card") {
    container.classList.add("file-list-grid");
    container.classList.remove("file-list");
    if (listHeader) listHeader.style.display = "none";
    if (statusBar) statusBar.style.display = "flex";

    const panelServersAvailable = hasPanelServers();
    const existingCards = new Map();
    const cardsByIdentity = new Map();
    const currentChildren = Array.from(container.children);
    currentChildren.forEach((child) => {
      if (child.classList?.contains("file-card") && child.dataset.path) {
        existingCards.set(child.dataset.path, child);
        const identity = child.dataset.cardIdentity || child.dataset.previewRevision;
        if (identity) {
          const candidates = cardsByIdentity.get(identity) || [];
          candidates.push(child);
          cardsByIdentity.set(identity, candidates);
        }
      }
    });

    const specs = appState.vpkFiles.map((file) => {
      const existingCard = takeExistingCard(file, existingCards, cardsByIdentity);
      const signature = getFileCardRenderSignature(file, panelServersAvailable);

      if (existingCard?.dataset.renderSignature === signature) {
        syncReusedCardSelection(existingCard, file);
        ensureCardPreviewObservation(existingCard, file);
        return { node: existingCard };
      }

      // A changed card may still have an in-flight intersection-observer task.
      // Remove only this card's task; unchanged cards keep their observation
      // and do not churn the observer on every refresh/filter action.
      unobserveCardPreview(existingCard);
      return { file, previous: existingCard, signature };
    });

    // Any old cards not present in the next result must no longer retain an
    // observer entry. Their detached image nodes are otherwise kept alive by
    // pendingCardPreviews until the next full render.
    existingCards.forEach((card) => unobserveCardPreview(card));

    const orderUnchanged =
      currentChildren.length === specs.length &&
      specs.every((spec, index) => spec.node === currentChildren[index]);
    if (orderUnchanged) {
      // 顺序与内容都没变：一个节点都不动（这条快路径必须留着，否则每次
      // 选择/刷新都要重排 2904 个节点）。
      cancelPendingListRender();
    } else {
      startChunkedListRender(container, specs, "card", panelServersAvailable);
    }
  } else {
    // Switching away from card mode detaches every observed card. Release
    // those observer entries immediately so the old card graph is collectible.
    clearPendingCardPreviews();
    container.classList.add("file-list");
    container.classList.remove("file-list-grid");
    if (listHeader) listHeader.style.display = "grid";
    if (statusBar) statusBar.style.display = "flex";

    startChunkedListRender(
      container,
      (appState.vpkFiles || []).map((file) => ({ file })),
      "row",
    );
  }

  // 每次重画后恢复键盘光标：搜索结果刷新（输入中）不应把光标弄丢。
  applySearchResultCursor();
}

/**
 * applySearchResultCursor 把键盘光标（↑ / ↓ 选中的那一行）画到列表上，
 * 并把"第 N / M 个结果"补进命中计数文案里。
 *
 * 之所以由 render.js 统一负责：重画会清掉行上的 class，
 * 键盘处理（app-runtime.js）只负责改 appState.searchCursorPath，然后调用这里。
 */
export function applySearchResultCursor() {
  const container = document.getElementById("file-list");
  const cursorPath = String(appState.searchCursorPath || "");
  container
    ?.querySelectorAll(".file-item.is-result-cursor, .file-card.is-result-cursor")
    .forEach((row) => {
      row.classList.remove("is-result-cursor");
    });

  const label = document.getElementById("search-hit-count");
  const baseLabel = label?.dataset.baseLabel || label?.textContent || "";
  if (label && baseLabel) label.dataset.baseLabel = baseLabel;

  if (!cursorPath) {
    if (label && label.dataset.baseLabel) label.textContent = label.dataset.baseLabel;
    return;
  }

  // 用 appState 的结果集而不是 DOM：窗口化之后 DOM 里只有"已物化"的一部分，
  // 拿 DOM 当结果集会漏掉后面几千条。
  const paths = (appState.vpkFiles || []).map((file) => String(file?.path || "")).filter(Boolean);
  const position = describeResultCursor(paths, cursorPath);
  if (!position) {
    // 光标指向的行已经被筛掉了：清掉状态，避免 Enter 打开一个看不见的 Mod。
    appState.searchCursorPath = "";
    if (label && label.dataset.baseLabel) label.textContent = label.dataset.baseLabel;
    return;
  }

  // 光标行可能是列表模式的行，也可能是卡片模式的卡片。
  // 回收式窗口下旧节点可能已经被回收：先查窗口索引表（O(1)），再退回 DOM 查询。
  let target = findWindowNodeByPath(container, cursorPath);
  if (!target && pendingListRender) {
    // 光标落在还没物化的部分：窗口化时直接把窗口搬过去（O(1)），
    // 小列表则按时间预算补齐；都补完再重新走一次，把这行滚进视野。
    const index = paths.indexOf(cursorPath);
    if (index >= 0 && !materializeThroughIndex(index, 60)) {
      scheduleCursorReveal(cursorPath);
      return;
    }
    target = findWindowNodeByPath(container, cursorPath);
  }
  target?.classList.add("is-result-cursor");
  target?.scrollIntoView({ block: "nearest" });
  if (label) label.textContent = `${label.dataset.baseLabel || ""} · ${position}`.trim();
}

// scheduleCursorReveal 分帧继续物化，直到光标那行出现（避免一次补 2500 张卡卡住）。
let cursorRevealPending = "";
function scheduleCursorReveal(cursorPath) {
  if (cursorRevealPending === cursorPath) return;
  cursorRevealPending = cursorPath;
  const step = () => {
    cursorRevealPending = "";
    if (String(appState.searchCursorPath || "") !== cursorPath) return;
    applySearchResultCursor();
  };
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(step);
  else setTimeout(step, 16);
}

/**
 * getArchivePackBadge 给"扩展名是 .vpk、实际是压缩包"的条目生成显眼标记。
 *
 * 这类包（工坊作者特意做的插件/工具/教程包）游戏不会加载，但它常常占着 addonlist.txt
 * 的一行 —— 也就是占了加载顺序里的位置。所以它照常出现在列表里，必须一眼就能看出
 * "这不是 VPK Mod"，而不是让人以为自己的 Mod 坏了。
 */
function getArchivePackBadge(file, className = "archive-pack-tag") {
  const pack = file?.archivePack;
  if (!pack) return "";
  const label = escapeHtml(pack.label || "压缩包");
  const note = escapeHtml(pack.note || "游戏不会加载它，也不参与 addonlist.txt");
  const format = escapeHtml(String(pack.format || "").toUpperCase());
  const evidence = Array.isArray(pack.evidence) && pack.evidence.length > 0
    ? `；判定依据：${escapeHtml(pack.evidence.slice(0, 3).join("、"))}`
    : "";
  return `<span class="${className}" title="${label}（${format}，不是 VPK）：${note}${evidence}">📦 ${label} · 非 VPK</span>`;
}

// 卡片/行右侧「⋮」菜单的内容改成**第一次点开时才生成**。
//
// 真机实测（2904 个 Mod）：这段菜单占一张卡 HTML 的 44%、节点数的 46%、创建耗时的 36%，
// 而绝大多数卡片的菜单从来没被打开过 —— 2904 张卡就是 12.5 万个白造的节点，
// 它们还会拖慢每一次全文档样式重算 / querySelectorAll。触发器留在标记里（样式与
// 事件委托都不变），内容交给 ensureMoreActionsMenu 在第一次展开前填。
const MORE_ACTIONS_TRIGGER_HTML = `
    <div class="more-actions-dropdown">
      <button class="btn-small action-btn more-btn" title="更多操作">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
          <path d="M12 8c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zm0 2c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0 6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z"/>
        </svg>
      </button>
      <div class="dropdown-content hidden"></div>
    </div>
  `;

function buildMoreActionsMenuHtml(file) {
  const hideBtnIcon = file?.enabled ? iconSvg("eyeOff") : iconSvg("eye");
  const hideBtnText = file?.enabled ? "隐藏" : "取消隐藏";
  return `
        <button class="dropdown-item detail-btn" data-file-path="${file.path}">
          <span class="btn-icon">${iconSvg("info")}</span> 详情
        </button>
        ${file.workshopId ? `
        <button class="dropdown-item workshop-btn" data-file-path="${file.path}" data-workshop-id="${file.workshopId}">
          <span class="btn-icon">${iconSvg("external")}</span> 跳转工坊
        </button>
        <button class="dropdown-item share-workshop-btn" data-file-path="${file.path}" data-action="share-workshop">
          <span class="btn-icon">${iconSvg("share")}</span> 分享物品
        </button>
        ` : ""}
        <button class="dropdown-item set-tags-btn" data-file-path="${file.path}" data-action="set-tags">
          <span class="btn-icon">${iconSvg("tag")}</span> 设置标签
        </button>
        ${hasPanelServers() ? `
        <button class="dropdown-item upload-server-btn" data-file-path="${file.path}" data-action="upload-server">
          <span class="btn-icon">${iconSvg("upload")}</span>
          <span class="menu-item-text">上传服务器</span>
          <span class="menu-item-arrow">${iconSvg("chevronRight")}</span>
        </button>
        ` : ""}
        <button class="dropdown-item rename-btn" data-file-path="${file.path}" data-action="rename">
          <span class="btn-icon">${iconSvg("edit")}</span> 重命名
        </button>
        <button class="dropdown-item load-order-btn" data-file-path="${file.path}" data-action="load-order">
          <span class="btn-icon">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="10" y1="6" x2="21" y2="6"></line>
              <line x1="10" y1="12" x2="21" y2="12"></line>
              <line x1="10" y1="18" x2="21" y2="18"></line>
              <path d="M4 6h1v4"></path>
              <path d="M4 10h2"></path>
              <path d="M6 18H4c0-1 2-2 2-3s-1-1.5-2-1"></path>
            </svg>
          </span> 加载顺序
        </button>
        <button class="dropdown-item unpack-btn" data-file-path="${file.path}" data-action="unpack">
          <span class="btn-icon">${iconSvg("package")}</span> 解包
        </button>
        <button class="dropdown-item open-location-btn" data-file-path="${file.path}" data-action="open-location">
          <span class="btn-icon">${iconSvg("folderOpen")}</span> 打开位置
        </button>
        <button class="dropdown-item hide-btn" data-file-path="${file.path}" data-action="hide">
          <span class="btn-icon">${hideBtnIcon}</span> ${hideBtnText}
        </button>
        <div class="dropdown-divider"></div>
        <button class="dropdown-item delete-btn" data-file-path="${file.path}" data-action="delete">
          <span class="btn-icon">${iconSvg("trash")}</span> 删除
        </button>
      </div>
    `;
}

/**
 * ensureMoreActionsMenu 首次展开前把菜单内容填进容器（已有内容就直接返回）。
 * 文件对象由渲染时挂在行/卡片元素上的 __file 提供（复用卡片时会刷新引用）。
 */
export function ensureMoreActionsMenu(dropdown, fileContainer) {
  if (!dropdown || dropdown.dataset.menuFilled === "1") return;
  const file = fileContainer?.__file || null;
  if (!file) return;
  dropdown.innerHTML = buildMoreActionsMenuHtml(file);
  dropdown.dataset.menuFilled = "1";
}

export function createFileItem(file) {
  const item = document.createElement("div");
  item.className = "file-item";
  item.dataset.path = file.path;
  // 惰性菜单要用它：卡片/行复用时会刷新这个引用（见 syncReusedCardSelection）。
  item.__file = file;
  applyModStateClasses(item, file, "mod-row");

  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.className = "file-checkbox";
  checkbox.checked = appState.selectedFiles.has(file.path);
  checkbox.addEventListener("click", function (event) {
    event.stopPropagation();
    applySelectionGesture(file.path, event, event.currentTarget.checked);
  });

  const displayTitle = file.title || file.name;
  // 搜索时：标题/文件名高亮命中片段，并说明命中字段（"为什么这行被搜出来"）。
  const searchSyntax = parseSearchSyntax(appState.searchQuery);
  const searchTerms = positiveTerms(searchSyntax);
  const searchHighlight = { terms: searchTerms, regex: compiledRegex(searchSyntax) };
  const titleHighlighted = highlightMatches(displayTitle, searchHighlight);
  const nameHighlighted = highlightMatches(file.name, searchHighlight);
  const matchReasonChip = searchSyntax.raw
    ? (() => {
        const chip = formatMatchReasonChip(describeMatchReasons(file, searchSyntax.raw));
        return chip ? `<span class="search-reason-chip" title="这行是被这些字段匹配到的">${escapeHtml(chip)}</span>` : "";
      })()
    : "";
  const isHidden = file.name.startsWith("_");
  const hideBtnText = isHidden ? "取消隐藏" : "隐藏";
  const hideBtnIcon = isHidden ? iconSvg("eye") : iconSvg("eyeOff");
  const locationBadgeClass = `location-${file.location || "unknown"}`;
  const hasUpdate = file.hasUpdate;

  const updateTagHtml = hasUpdate
    ? `<span class="update-available-tag" data-workshop-id="${file.workshopId}" title="点击更新此Mod">待更新</span>`
    : "";

  // 列表模式：更新标签放在文件名后面
  const moreActionsHtml = MORE_ACTIONS_TRIGGER_HTML;

  item.innerHTML = `
    <div class="file-checkbox-container"></div>
    <div class="file-name" title="${file.path}">
      <div class="file-title">${titleHighlighted}</div>
      <div class="file-filename">${nameHighlighted}${updateTagHtml}${getArchivePackBadge(file)}</div>
    </div>
    <div class="file-size">${formatFileSize(file.size)}</div>
    <div class="file-location">
      <span class="location-state-tag ${locationBadgeClass}">
        ${getLocationSvg(file.location)}
        <span>${getLocationDisplayName(file.location)}</span>
      </span>
    </div>
      <div class="file-game-state">${getGameStateBadge(file)}${getLoadOrderBadge(file)}${getConflictRecheckBadge(file)}${getModGroupBadge(file, "mod-group-badge file-mod-group-badge")}</div>
    <div class="file-tags">
      ${matchReasonChip}
      ${formatTags(file.primaryTag, file.secondaryTags, file.voiceCharacters, file.subjectSummary, file.xdrSummary)}
      ${getConflictSummaryBadge(file)}
    </div>
    <div class="file-actions">
      <button class="btn-small action-btn detail-btn" data-file-path="${file.path}">
        <span class="btn-icon">${iconSvg("info")}</span>
        <span class="btn-text">详情</span>
      </button>
      ${getGameStateActionControls(file)}
      ${getActionButton(file)}
      ${moreActionsHtml}
    </div>
  `;

  const checkboxContainer = item.querySelector(".file-checkbox-container");
  checkboxContainer.appendChild(checkbox);
  checkboxByPath.set(file.path, checkbox);
  // 只有真正点击 checkbox 才改变选择；点击容器空白不能穿透到行处理器。
  checkboxContainer.addEventListener("click", function (event) {
    event.stopPropagation();
  });

  item.addEventListener("click", function (e) {
    if (
      e.target.closest(".file-checkbox-container") ||
      e.target.closest(".file-actions") ||
      e.target.type === "checkbox" ||
      e.target.closest("button")
    ) {
      return;
    }

    if (e.shiftKey || (appState.ctrlClickSelectionEnabled && (e.ctrlKey || e.metaKey))) {
      e.preventDefault();
      e.stopPropagation();
      // Shift/Ctrl 点击行主体恢复桌面文件管理器的整行选择语义；
      // 普通点击仍不会因为点到文字或空白而误选。
      applySelectionGesture(file.path, e, !appState.selectedFiles.has(file.path));
    }
  });

  item.addEventListener("dblclick", function (e) {
    if (
      e.target.closest(".file-checkbox-container") ||
      e.target.closest(".file-actions") ||
      e.target.type === "checkbox" ||
      e.target.closest("button")
    ) {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
    showFileDetail(file.path);
  });

  return item;
}

export function createFileCard(file, existingCard = null, panelServersAvailable = hasPanelServers()) {
  const card = existingCard || document.createElement("div");
  // 惰性菜单要用它（复用路径由 syncReusedCardSelection 刷新）。
  card.__file = file;
  const previousPreview =
    existingCard?.__previewImage || existingCard?.querySelector(".card-preview-img") || null;
  const previewRevision = getCardPreviewRevision(file);
  const canPreservePreview = Boolean(
    previousPreview &&
      existingCard.dataset.previewRevision === previewRevision,
  );

  card.className = "file-card";
  card.dataset.path = file.path;
  card.dataset.previewRevision = previewRevision;
  card.dataset.cardIdentity = previewRevision;
  applyModStateClasses(card, file, "mod-card");

  if (!file.enabled) {
    card.classList.add("disabled");
  }

  const displayTitle = file.title || file.name;
  // 卡片视图同样高亮命中，并复用列表视图的"命中字段"文案。
  const cardSearchSyntax = parseSearchSyntax(appState.searchQuery);
  const cardTitleHighlighted = highlightMatches(displayTitle, {
    terms: positiveTerms(cardSearchSyntax),
    regex: compiledRegex(cardSearchSyntax),
  });
  const cardMatchReasonChip = cardSearchSyntax.raw
    ? (() => {
        const chip = formatMatchReasonChip(describeMatchReasons(file, cardSearchSyntax.raw));
        return chip ? `<span class="search-reason-chip" title="这行是被这些字段匹配到的">${escapeHtml(chip)}</span>` : "";
      })()
    : "";
  const isHidden = file.name.startsWith("_");
  const hideBtnText = isHidden ? "取消隐藏" : "隐藏";
  const hideBtnIcon = isHidden ? iconSvg("eye") : iconSvg("eyeOff");
  const hasUpdate = file.hasUpdate;

  const updateBtnHtml = hasUpdate
    ? `<button class="btn-small action-btn update-btn" data-workshop-id="${file.workshopId}" title="点击更新此Mod">
        <span class="btn-icon">
          <svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
            <polyline points="7 10 12 15 17 10"></polyline>
            <line x1="12" y1="15" x2="12" y2="3"></line>
          </svg>
        </span>
        <span class="btn-text">待更新</span>
      </button>`
    : "";

  const cachedPreview = getCachedVPKCardPreview(file);
  const previewSrc = canPreservePreview
    ? previousPreview.getAttribute("src")
    : cachedPreview || "";
  // An empty src points an <img> at the current document in some browsers.
  // Omit the attribute until the queued preview request actually returns.
  const previewSrcAttribute = previewSrc ? ` src="${previewSrc}"` : "";
  const showPlaceholder = !previewSrc;

  let secondaryTagsHtml = "";
  const subjectSummary = String(file.subjectSummary || "").trim();
  const xdrSummary = String(file.xdrSummary || "").trim();
  const subjectBadgeHtml = subjectSummary
    ? `<span class="card-badge subject-badge" title="${escapeHtml(subjectSummary)}">${escapeHtml(subjectSummary)}</span>`
    : "";
  const xdrBadgeHtml = xdrSummary
    ? `<span class="card-badge xdr-badge" title="${escapeHtml(xdrSummary)}">${escapeHtml(xdrSummary)}</span>`
    : "";
  const uniqueDisplayTags = getUniqueDisplayTags(file.primaryTag, file.secondaryTags);
  if (uniqueDisplayTags.secondary.length > 0) {
    const displayTags = uniqueDisplayTags.secondary.slice(0, 2);
    const hasMore = uniqueDisplayTags.secondary.length > 2;

    secondaryTagsHtml = displayTags
      .map((tag) => {
        const longTagClass = tag.length > 16 ? " is-long" : "";
        return `<span class="card-badge secondary-tag-badge${longTagClass}" title="${escapeHtml(tag)}">${escapeHtml(tag)}</span>`;
      })
      .join("");

    if (hasMore) {
      secondaryTagsHtml += `<span class="card-badge more-tag-badge" title="${uniqueDisplayTags.secondary
        .slice(2)
        .map(escapeHtml)
        .join(", ")}">+${uniqueDisplayTags.secondary.length - 2}</span>`;
    }
  }

  let actionBtn = "";
  if (file.location === "workshop") {
    actionBtn = `
      <button class="btn-small action-btn move-btn" data-file-path="${file.path}" data-action="move" title="复制到 addons">
        <span class="btn-icon">${iconSvg("package")}</span>
        <span class="btn-text">复制到 addons</span>
      </button>
    `;
  } else {
    actionBtn = `
      <button class="btn-small action-btn toggle-btn ${file.enabled ? "toggle-disable" : "toggle-enable"}"
              data-file-path="${file.path}" data-action="toggle"
              title="${file.enabled ? "点击禁用" : "点击启用"}">
        <span class="btn-icon">${iconSvg("power")}</span>
        <span class="btn-text">${file.enabled ? "禁用" : "启用"}</span>
      </button>
    `;
  }

  const moreActionsHtml = MORE_ACTIONS_TRIGGER_HTML;

  card.innerHTML = `
    <div class="card-preview-container">
      <div class="card-preview-placeholder ${showPlaceholder ? "" : "hidden"}">
        <svg class="icon-svg placeholder-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
          <circle cx="8.5" cy="8.5" r="1.5"></circle>
          <polyline points="21 15 16 10 5 21"></polyline>
        </svg>
      </div>
      <img class="card-preview-img ${showPlaceholder ? "hidden" : ""}"${previewSrcAttribute} alt="${escapeHtml(displayTitle)}" loading="lazy" decoding="async" fetchpriority="low" />
      <div class="card-checkbox-container"></div>
      <div class="card-badges">
        <span class="card-badge location-badge">${getLocationDisplayName(file.location)}</span>
        ${getArchivePackBadge(file, "card-badge archive-pack-tag")}
        ${getGameStateBadge(file, "card-badge game-state-badge")}
        ${getLoadOrderBadge(file, "card-badge load-order-badge")}
        ${getConflictRecheckBadge(file, "card-badge conflict-recheck-badge")}
        ${getModGroupBadge(file, "card-badge mod-group-badge")}
        ${
          file.primaryTag
            ? `<span class="card-badge tag-badge" title="${escapeHtml(file.primaryTag)}">${escapeHtml(file.primaryTag)}</span>`
            : ""
        }
        ${xdrBadgeHtml}${subjectBadgeHtml}${cardMatchReasonChip}
        ${secondaryTagsHtml}
        ${getConflictSummaryBadge(file, "card-badge mod-conflict-badge")}
      </div>
    </div>
    <div class="card-content">
      <div class="card-title" title="${escapeHtml(displayTitle)}">${cardTitleHighlighted}</div>
      <div class="card-filename" title="${file.name}">${file.name}</div>
      <div class="card-actions">
        <div class="card-actions-left">
          ${getGameStateActionControls(file)}
          ${actionBtn}
          ${updateBtnHtml}
        </div>
        ${moreActionsHtml}
      </div>
    </div>
  `;

  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.className = "file-checkbox card-checkbox";
  checkbox.checked = appState.selectedFiles.has(file.path);
  checkbox.addEventListener("click", function (event) {
    event.stopPropagation();
    applySelectionGesture(file.path, event, event.currentTarget.checked);
  });
  const checkboxContainer = card.querySelector(".card-checkbox-container");
  checkboxContainer.appendChild(checkbox);
  checkboxByPath.set(file.path, checkbox);
  card.__selectionCheckbox = checkbox;
  checkboxContainer.addEventListener("click", function (e) {
    e.stopPropagation();
  });

  let img = card.querySelector(".card-preview-img");
  const placeholder = card.querySelector(".card-preview-placeholder");
  card.__previewImage = img;
  card.__previewPlaceholder = placeholder;

  if (canPreservePreview) {
    const renderedPreview = img;
    previousPreview.className = renderedPreview.className;
    previousPreview.alt = displayTitle;
    previousPreview.loading = "lazy";
    previousPreview.decoding = "async";
    renderedPreview.replaceWith(previousPreview);
    img = previousPreview;
    card.__previewImage = img;
  }

  // A preserved image can still be waiting for its first load (for example
  // when a Mod moved between addons and disabled). Re-observe it when there is
  // no cached result and no source yet; otherwise the move would preserve a
  // blank placeholder without ever starting the request again.
  if (cachedPreview === undefined && !img.getAttribute("src")) {
    observeCardPreview(card, file, img, placeholder);
  }

  if (!existingCard) {
    card.addEventListener("click", function (e) {
      if (
        e.target.closest("button") ||
        e.target.closest(".more-actions-dropdown") ||
        e.target.closest(".card-checkbox-container") ||
        e.target.closest(".card-badge")
      ) {
        return;
      }

      if (e.shiftKey || (appState.ctrlClickSelectionEnabled && (e.ctrlKey || e.metaKey))) {
        e.preventDefault();
        e.stopPropagation();
        // Shift/Ctrl 点击卡片主体也可以选择；普通点击仍打开详情。
        // 卡片会在 Mod 在 addons / disabled 等目录间移动时按预览身份复用。
        // 不能捕获首次创建时的 file.path，否则复用后的主体点击仍会操作旧路径。
        const currentPath = card.dataset.path;
        if (!currentPath) return;
        applySelectionGesture(currentPath, e, !appState.selectedFiles.has(currentPath));
        return;
      }

      const currentPath = card.dataset.path;
      if (currentPath) showFileDetail(currentPath);
    });
  }

  return card;
}

export function iconSvg(name) {
  const icons = {
    search: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="7"></circle><path d="m20 20-3.5-3.5"></path></svg>`,
    info: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="M12 11v5"></path><path d="M12 8h.01"></path></svg>`,
    external: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 3h6v6"></path><path d="M10 14 21 3"></path><path d="M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"></path></svg>`,
    eye: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z"></path><circle cx="12" cy="12" r="3"></circle></svg>`,
    eyeOff: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m3 3 18 18"></path><path d="M10.6 10.6A2 2 0 0 0 13.4 13.4"></path><path d="M9.9 4.2A10.4 10.4 0 0 1 12 4c6.5 0 10 8 10 8a18 18 0 0 1-2.2 3.2"></path><path d="M6.6 6.6C3.6 8.6 2 12 2 12s3.5 8 10 8a10.6 10.6 0 0 0 4.1-.8"></path></svg>`,
    tag: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z"></path><circle cx="7.5" cy="7.5" r=".8"></circle></svg>`,
    edit: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 20h9"></path><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"></path></svg>`,
    folderOpen: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 14 8 8h13l-2 8a2 2 0 0 1-2 1.5H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5l2 2h4"></path></svg>`,
    trash: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 6h18"></path><path d="M8 6V4h8v2"></path><path d="m19 6-1 14H6L5 6"></path><path d="M10 11v5"></path><path d="M14 11v5"></path></svg>`,
    package: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"></path><path d="m3.3 7 8.7 5 8.7-5"></path><path d="M12 22V12"></path></svg>`,
    power: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 2v10"></path><path d="M18.4 6.6a9 9 0 1 1-12.8 0"></path></svg>`,
    check: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"></path></svg>`,
    x: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"></path><path d="m6 6 12 12"></path></svg>`,
    chevronRight: `<svg class="icon-svg submenu-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"></path></svg>`,
    upload: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="17 8 12 3 7 8"></polyline><line x1="12" y1="3" x2="12" y2="15"></line></svg>`,
    share: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="18" cy="5" r="3"></circle><circle cx="6" cy="12" r="3"></circle><circle cx="18" cy="19" r="3"></circle><path d="m8.6 10.5 6.8-4"></path><path d="m8.6 13.5 6.8 4"></path></svg>`,
  };
  return icons[name] || "";
}

export function getLocationSvg(location) {
  if (location === "workshop") {
    return `<svg class="location-tag-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h16"></path><path d="M5 7l1-3h12l1 3"></path><path d="M6 7v12h12V7"></path><path d="M9 11h6"></path></svg>`;
  }
  if (location === "disabled") {
    return `<svg class="location-tag-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="m5.7 5.7 12.6 12.6"></path></svg>`;
  }
  return `<svg class="location-tag-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7h7l2 2h9v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"></path><path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h4"></path></svg>`;
}

export async function loadCardPreview(file, imgElement) {
  try {
    const imgData = await loadVPKCardPreview(file);
    // A card can refresh while an archive read is running. The image element is
    // deliberately preserved across that refresh; resolve the current
    // placeholder at completion instead of holding a stale DOM reference.
    if (imgData && imgElement.isConnected) {
      imgElement.src = imgData;
      imgElement.classList.remove("hidden");
      imgElement
        .closest(".card-preview-container")
        ?.querySelector(".card-preview-placeholder")
        ?.classList.add("hidden");
    }
  } catch (err) {
    if (err?.message === "卡片预览已离开可视区域") return;
    console.warn("加载预览图失败:", file.name);
  }
}

/**
 * updateSingleFileDisplay 只重画一张卡（/ 一行）的状态：游戏内开关、位置标签、
 * 禁用·启用按钮与游戏状态徽标。
 *
 * 为什么需要它：改一个 Mod 的游戏内开关本来只需要动一张卡，但走整表搜索刷新
 * 会把 2904 张卡全部重画 —— 那一次重画就是用户点下去感到的 200ms 卡顿。
 * 批量操作（actions.js）与单个开关（operations.js）共用这一份实现，语义必须一致。
 */
export function updateSingleFileDisplay(file) {
  if (!file?.path) return;
  const item = document.querySelector(
    `.file-item[data-path="${CSS.escape(file.path)}"], .file-card[data-path="${CSS.escape(file.path)}"]`,
  );
  if (!item) return;

  const rowPrefix = item.classList.contains("file-card") ? "mod-card" : "mod-row";
  item.classList.remove(
    `${rowPrefix}-state-enabled`,
    `${rowPrefix}-state-disabled`,
    `${rowPrefix}-state-unknown`,
    `${rowPrefix}-location-disabled`,
    "disabled",
  );
  const stateClass = !file.gameStateKnown
    ? "unknown"
    : file.gameEnabled
      ? "enabled"
      : "disabled";
  item.classList.add(`${rowPrefix}-state-${stateClass}`);
  if (file.location === "disabled") item.classList.add(`${rowPrefix}-location-disabled`);
  if (item.classList.contains("file-card") && file.enabled === false) {
    item.classList.add("disabled");
  }

  const stateBadge = item.querySelector(".game-state-badge");
  if (stateBadge) {
    const stateLabels = {
      enabled: "游戏内开启",
      disabled: "游戏内关闭",
      unknown: "未记录",
    };
    const stateTitles = {
      enabled: "addonlist.txt：1；点击关闭游戏内 Mod",
      disabled: "addonlist.txt：0；点击开启游戏内 Mod",
      unknown: "addonlist.txt 中未记录此 Mod；点击选择游戏内关闭、游戏内启用或禁用",
    };
    stateBadge.classList.remove("game-state-enabled", "game-state-disabled", "game-state-unknown");
    stateBadge.classList.add(`game-state-${stateClass}`);
    stateBadge.textContent = stateLabels[stateClass];
    stateBadge.title = stateTitles[stateClass];
  }

  const locationEl = item.querySelector(".file-location");
  if (locationEl) {
    const locationNames = { root: "根目录", workshop: "创意工坊", disabled: "已禁用" };
    locationEl.innerHTML = `
      <span class="location-state-tag location-${file.location}">
        ${getLocationSvg(file.location)}
        <span>${locationNames[file.location] || file.location}</span>
      </span>
    `;
  }

  const actionBtn = item.querySelector(".toggle-btn, .move-btn");
  if (actionBtn) {
    if (file.location === "workshop") {
      actionBtn.outerHTML = `
        <button class="btn-small action-btn move-btn" data-file-path="${file.path}" data-action="move">
          <span class="btn-icon">${iconSvg("package")}</span>
          <span class="btn-text">复制到 addons</span>
        </button>
      `;
    } else {
      actionBtn.outerHTML = `
        <button class="btn-small action-btn toggle-btn ${file.enabled ? "toggle-disable" : "toggle-enable"}"
                data-file-path="${file.path}" data-action="toggle">
          <span class="btn-icon">${file.enabled ? iconSvg("x") : iconSvg("check")}</span>
          <span class="btn-text">${file.enabled ? "禁用" : "启用"}</span>
        </button>
      `;
    }
  }

  const gameStateControls = item.querySelector(".game-state-controls");
  if (gameStateControls) {
    gameStateControls.outerHTML = getGameStateActionControls(file);
  }
  // 卡片行还有"待更新/组徽标"等其它角标：它们的渲染签名跟着状态一起失效，
  // 下一次整表刷新会按需重建（这里显式清掉，避免复用一张过期的卡）。
  delete item.dataset.renderSignature;
}

function getCardPreviewObserver() {
  if (cardPreviewObserver) return cardPreviewObserver;
  cardPreviewObserver = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) return;
      const target = pendingCardPreviews.get(entry.target);
      cardPreviewObserver.unobserve(entry.target);
      pendingCardPreviews.delete(entry.target);
      if (target) {
        void loadCardPreview(target.file, target.img);
      }
    });
  }, {
    root: document.getElementById("file-list") || null,
    // Keep a modest lead-in window: a large margin eagerly starts many image
    // decodes during fast filtering/scrolling and defeats the bounded queue.
    // The card cache still makes already-seen previews instant.
    rootMargin: "160px 0px",
    threshold: 0.01,
  });
  return cardPreviewObserver;
}

function observeCardPreview(card, file, img, placeholder) {
  pendingCardPreviews.set(card, { file, img, placeholder });
  getCardPreviewObserver().observe(card);
}

function unobserveCardPreview(card) {
  if (!card) return;
  if (cardPreviewObserver) {
    cardPreviewObserver.unobserve(card);
  }
  const pending = pendingCardPreviews.get(card);
  pendingCardPreviews.delete(card);
  if (pending) cancelVPKCardPreview(pending.file);
}

function clearPendingCardPreviews() {
  if (!cardPreviewObserver) return;
  pendingCardPreviews.forEach((pending, card) => {
    cardPreviewObserver.unobserve(card);
    cancelVPKCardPreview(pending.file);
  });
  pendingCardPreviews.clear();
}
