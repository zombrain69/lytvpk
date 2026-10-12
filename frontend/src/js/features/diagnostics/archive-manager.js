import { showError, showNotification } from "../../core/toast.js";
import { createLocaleCollator } from "../../core/locale-collator.mjs";
import { t } from "../../core/i18n-runtime.js";
import { beginMessageModalSession } from "../../core/message-modal.js";
import { createImeAwareSearchController } from "./archive-search-controller.mjs";
import { formatMoveFailures } from "../file-list/move-result-format.mjs";
import { highlightMatches } from "../file-list/search-match.mjs";
import { collectCursorKeys, describeResultCursor, nextResultPath, syncCursorHighlight } from "../file-list/result-cursor.mjs";
import {
  ARCHIVE_SEARCH_HELP_VARIANT,
  buildSearchHelpHtml,
  buildSearchHelpTitle,
} from "../file-list/search-help.mjs";
import {
  archivePackageStateTags,
  archiveHighlightSpec,
  describeArchiveSearchResult,
  searchArchivePackages,
} from "./archive-search.mjs";
// 本地化排序（第 19 项 P3）：归档包名跟随界面语言（数字感知）。
const ARCHIVE_NAME_COLLATOR = createLocaleCollator({ numeric: true, sensitivity: "base" });

let archiveManagerRunning = false;
let archiveManagerSession = null;
let archiveManagerDirectory = "";
let archiveManagerPackages = [];
// 最近一次文本检索的结果（计数文案与高亮都用它，避免重复解析同一段查询）。
let archiveManagerSearchResult = null;
// 键盘光标：搜索框里用 ↑↓ 移动、Enter 展开当前压缩包（与 Mod 列表同一套）。
let archiveCursorPath = "";
const selectedArchivePaths = new Set();
const expandedArchivePaths = new Set();
const archivePasswordByPath = new Map();
let archiveManagerQuery = "";
let archiveManagerSort = "name-asc";
let archiveManagerStateFilter = "all";
let archiveManagerDensity = "compact";
let archiveManagerSearchController = null;

const ARCHIVE_TREE_INITIAL_LIMIT = 160;

// 说明书浮层的"点外部 / Esc 关闭"只绑定一次：归档面板每次输入都会重画 DOM，
// 在这里挂 document 监听就会越叠越多，所以事件里按 id 现查元素。
let archiveHelpDismissBound = false;

function bindArchiveHelpDismissOnce() {
  if (archiveHelpDismissBound) return;
  archiveHelpDismissBound = true;
  const closeHelp = () => {
    document.getElementById("archive-search-help-popover")?.classList.add("hidden");
    const btn = document.getElementById("archive-search-help-btn");
    btn?.setAttribute("aria-expanded", "false");
    btn?.classList.remove("is-active");
  };
  document.addEventListener("click", (event) => {
    const popover = document.getElementById("archive-search-help-popover");
    const btn = document.getElementById("archive-search-help-btn");
    if (!popover || !btn || popover.classList.contains("hidden")) return;
    if (popover.contains(event.target) || btn.contains(event.target)) return;
    closeHelp();
  });
  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    const popover = document.getElementById("archive-search-help-popover");
    if (!popover || popover.classList.contains("hidden")) return;
    event.stopPropagation();
    closeHelp();
  });
}

export async function openArchiveManager() {
  if (archiveManagerRunning) {
    showNotification(t("toolbox.archiveManager.busy"), "info");
    return;
  }
  try {
    const directory = await callApp("SelectArchiveDirectory");
    if (!directory) return;
    archivePasswordByPath.clear();
    expandedArchivePaths.clear();
    archiveManagerQuery = "";
    archiveManagerSort = "name-asc";
    archiveManagerStateFilter = "all";
    archiveManagerDensity = "compact";
    archiveManagerSearchController?.cancel();
    archiveManagerSearchController = null;
    await scanArchiveDirectory(directory);
  } catch (error) {
    showError(t("toolbox.archiveManager.selectDirFailed", { error: formatError(error) }));
  }
}

async function scanArchiveDirectory(directory) {
  archiveManagerRunning = true;
  archiveManagerDirectory = directory;
  selectedArchivePaths.clear();
  showArchiveLoading(directory);
  try {
    archiveManagerPackages = (await callApp(
      "ScanArchiveDirectoryWithPasswords",
      directory,
      Object.fromEntries(archivePasswordByPath),
    )) || [];
    renderArchiveManager();
    showNotification(t("toolbox.archiveManager.scanDone", { count: archiveManagerPackages.length }), "success");
  } catch (error) {
    archiveManagerSession?.close("error");
    archiveManagerSession = null;
    showError(t("toolbox.archiveManager.scanFailed", { error: formatError(error) }));
  } finally {
    archiveManagerRunning = false;
  }
}

function showArchiveLoading(directory) {
  const session = createArchiveManagerSession();
  if (!session) return;
  archiveManagerSession = session;
  session.addClass(session.modal.querySelector(".modal-content"), "archive-manager-modal-content");
  session.titleEl.textContent = t("toolbox.archiveManager.title");
  session.contentEl.textContent = t("toolbox.archiveManager.scanning", { directory });
  session.confirmBtn.textContent = t("toolbox.common.cancel");
  session.confirmBtn.onclick = () => session.close("cancel");
  session.closeBtn.onclick = () => session.close("cancel");
  session.show();
}

function renderArchiveManager() {
  archiveManagerSearchController?.cancel();
  archiveManagerSearchController = null;
  const session = archiveManagerSession || createArchiveManagerSession();
  if (!session) return;
  archiveManagerSession = session;
  session.addClass(session.modal.querySelector(".modal-content"), "archive-manager-modal-content");
  session.titleEl.textContent = t("toolbox.archiveManager.titleCount", { count: archiveManagerPackages.length });
  session.contentEl.replaceChildren(createArchiveContent());
  session.confirmBtn.textContent = t("toolbox.common.close");
  session.confirmBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  session.show();
}

function createArchiveContent() {
  const wrapper = document.createElement("div");
  wrapper.className = "archive-manager-content-body";
  wrapper.classList.add(`is-${archiveManagerDensity}`);
  const toolbar = document.createElement("div");
  toolbar.className = "archive-manager-toolbar is-sticky";
  const summary = document.createElement("span");
  summary.className = "archive-manager-summary";
  const visiblePackages = getVisibleArchivePackages();
  const searchText = describeArchiveSearchResult({
    total: archiveManagerSearchResult?.total ?? archiveManagerPackages.length,
    matched: archiveManagerSearchResult?.matched ?? visiblePackages.length,
    query: archiveManagerQuery,
    regexInvalid: archiveManagerSearchResult?.regexInvalid || "",
  });
  summary.textContent = t("toolbox.archiveManager.summary", {
    directory: archiveManagerDirectory,
    visible: visiblePackages.length,
    total: archiveManagerPackages.length,
  });
  // 命中计数单独一个 span：键盘移动光标时只改这一处，不必整块重画（否则会丢焦点）。
  const searchCount = document.createElement("span");
  searchCount.className = "archive-manager-search-count";
  searchCount.id = "archive-manager-search-count";
  if (searchText) searchCount.textContent = searchText;
  const search = document.createElement("input");
  search.type = "search";
  search.className = "archive-manager-search";
  search.placeholder = t("toolbox.archiveManager.searchPlaceholder");
  search.value = archiveManagerQuery;
  search.setAttribute("aria-label", t("toolbox.archiveManager.searchAria"));
  // 悬停提示与 `?` 浮层都和 Mod 列表同源（search-help.mjs），只是字段与 tag: 的含义不同。
  search.title = buildSearchHelpTitle(ARCHIVE_SEARCH_HELP_VARIANT);
  const searchWrap = document.createElement("div");
  searchWrap.className = "archive-manager-search-wrap";
  const helpBtn = document.createElement("button");
  helpBtn.type = "button";
  helpBtn.id = "archive-search-help-btn";
  helpBtn.className = "search-help-btn";
  helpBtn.textContent = "?";
  helpBtn.title = t("toolbox.archiveManager.searchHelpTitle");
  helpBtn.setAttribute("aria-label", t("toolbox.archiveManager.searchHelpAria"));
  helpBtn.setAttribute("aria-expanded", "false");
  const helpPopover = document.createElement("div");
  helpPopover.id = "archive-search-help-popover";
  helpPopover.className = "search-help-popover hidden";
  helpPopover.innerHTML = buildSearchHelpHtml(ARCHIVE_SEARCH_HELP_VARIANT);
  const setHelpOpen = (open) => {
    helpPopover.classList.toggle("hidden", !open);
    helpBtn.setAttribute("aria-expanded", String(open));
    helpBtn.classList.toggle("is-active", open);
  };
  helpBtn.addEventListener("click", (event) => {
    event.stopPropagation();
    setHelpOpen(helpPopover.classList.contains("hidden"));
  });
  // 点外部 / Esc 关闭：只在模块里绑定一次，按 id 现查元素 —— 归档面板每次输入都会重画，
  // 如果在这里挂 document 监听就会越叠越多。
  bindArchiveHelpDismissOnce();
  searchWrap.append(search, helpBtn, helpPopover);
  const searchController = createImeAwareSearchController(() => {
    renderArchiveManager();
    requestAnimationFrame(() => {
      const next = document.querySelector(".archive-manager-search");
      next?.focus();
      if (next) next.setSelectionRange(archiveManagerQuery.length, archiveManagerQuery.length);
    });
  });
  archiveManagerSearchController = searchController;
  search.oninput = () => {
    archiveManagerQuery = search.value;
    searchController.input();
  };
  search.addEventListener("compositionstart", () => searchController.compositionStart());
  search.addEventListener("compositionend", () => {
    archiveManagerQuery = search.value;
    searchController.compositionEnd();
  });
  // ↑ / ↓ 在结果里移动光标（焦点仍在搜索框），Enter 展开当前行，Esc 清空检索。
  search.addEventListener("keydown", (event) => {
    const list = document.querySelector(".archive-manager-list");
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const keys = collectCursorKeys(list, ".archive-manager-package[data-cursor-key]");
      archiveCursorPath = nextResultPath(keys, archiveCursorPath, event.key === "ArrowDown" ? 1 : -1);
      syncCursorHighlight(list, ".archive-manager-package[data-cursor-key]", archiveCursorPath);
      updateArchiveSearchCountText();
      return;
    }
    if (event.key === "Enter") {
      const target = list?.querySelector(`.archive-manager-package[data-cursor-key="${escapeAttrSelector(archiveCursorPath)}"]`);
      if (!target) return;
      event.preventDefault();
      target.querySelector(".archive-manager-package-header")?.click();
      return;
    }
    if (event.key === "Escape" && search.value) {
      search.value = "";
      archiveManagerQuery = "";
      archiveCursorPath = "";
      renderArchiveManager();
    }
  });
  const sort = document.createElement("select");
  sort.className = "archive-manager-sort";
  sort.setAttribute("aria-label", t("toolbox.archiveManager.sortAria"));
  [
    ["name-asc", t("toolbox.archiveManager.sortNameAsc")],
    ["name-desc", t("toolbox.archiveManager.sortNameDesc")],
    ["status", t("toolbox.archiveManager.sortStatus")],
    ["size-desc", t("toolbox.archiveManager.sortSizeDesc")],
    ["size-asc", t("toolbox.archiveManager.sortSizeAsc")],
  ].forEach(([value, label]) => {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    option.selected = value === archiveManagerSort;
    sort.appendChild(option);
  });
  sort.onchange = () => {
    archiveManagerSort = sort.value;
    renderArchiveManager();
  };
  const stateFilter = document.createElement("select");
  stateFilter.className = "archive-manager-state-filter";
  stateFilter.setAttribute("aria-label", t("toolbox.archiveManager.stateFilterAria"));
  [
    ["all", t("toolbox.archiveManager.filterAll")],
    ["existing", t("toolbox.archiveManager.filterExisting")],
    ["new", t("toolbox.archiveManager.filterNew")],
    ["password", t("toolbox.archiveManager.filterPassword")],
    ["error", t("toolbox.archiveManager.filterError")],
  ].forEach(([value, label]) => {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    option.selected = value === archiveManagerStateFilter;
    stateFilter.appendChild(option);
  });
  stateFilter.onchange = () => {
    archiveManagerStateFilter = stateFilter.value;
    renderArchiveManager();
  };
  const density = document.createElement("button");
  density.type = "button";
  density.className = "btn btn-secondary archive-manager-density";
  density.textContent = archiveManagerDensity === "compact" ? t("toolbox.archiveManager.densityComfortable") : t("toolbox.archiveManager.densityCompact");
  density.title = archiveManagerDensity === "compact" ? t("toolbox.archiveManager.densityComfortableTitle") : t("toolbox.archiveManager.densityCompactTitle");
  density.onclick = () => {
    archiveManagerDensity = archiveManagerDensity === "compact" ? "comfortable" : "compact";
    renderArchiveManager();
  };
  const selectAll = document.createElement("button");
  selectAll.type = "button";
  selectAll.className = "btn btn-secondary";
  selectAll.textContent = t("toolbox.archiveManager.selectAll");
  selectAll.onclick = () => {
    archiveManagerPackages.forEach((item) => selectedArchivePaths.add(item.path));
    renderArchiveManager();
  };
  const clearAll = document.createElement("button");
  clearAll.type = "button";
  clearAll.className = "btn btn-secondary";
  clearAll.textContent = t("toolbox.archiveManager.deselectAll");
  clearAll.onclick = () => {
    selectedArchivePaths.clear();
    renderArchiveManager();
  };
  const move = document.createElement("button");
  move.type = "button";
  move.className = "btn btn-primary archive-manager-move";
  move.textContent = t("toolbox.archiveManager.moveSelected", { count: selectedArchivePaths.size });
  move.disabled = selectedArchivePaths.size === 0;
  move.onclick = () => moveSelectedArchives();
  const refresh = document.createElement("button");
  refresh.type = "button";
  refresh.className = "btn btn-secondary";
  refresh.textContent = t("toolbox.archiveManager.refresh");
  refresh.title = t("toolbox.archiveManager.refreshTitle");
  refresh.onclick = () => scanArchiveDirectory(archiveManagerDirectory);
  toolbar.append(summary, searchCount, searchWrap, sort, stateFilter, density, refresh, selectAll, clearAll, move);
  wrapper.appendChild(toolbar);

  const list = document.createElement("div");
  list.className = "archive-manager-list";
  if (archiveManagerPackages.length === 0) {
    const empty = document.createElement("div");
    empty.className = "archive-manager-empty";
    empty.textContent = t("toolbox.archiveManager.empty");
    list.appendChild(empty);
  }
  visiblePackages.forEach((item) => list.appendChild(createArchivePackage(item)));
  wrapper.appendChild(list);
  // 重画后恢复键盘光标（检索没变时保持在原来那一行）。
  syncCursorHighlight(list, ".archive-manager-package[data-cursor-key]", archiveCursorPath);
  return wrapper;
}

/** escapeAttrSelector 把值安全地放进属性选择器（路径里有引号 / 反斜杠时不能直接拼）。 */
function escapeAttrSelector(value) {
  return String(value ?? "").replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

/**
 * updateArchiveSearchCountText 只刷新命中计数那一小块：
 * 键盘移动光标时用它，避免整块重画把搜索框焦点顶掉。
 */
function updateArchiveSearchCountText() {
  const count = document.getElementById("archive-manager-search-count");
  if (!count) return;
  const base = describeArchiveSearchResult({
    total: archiveManagerSearchResult?.total ?? archiveManagerPackages.length,
    matched: archiveManagerSearchResult?.matched ?? 0,
    query: archiveManagerQuery,
    regexInvalid: archiveManagerSearchResult?.regexInvalid || "",
  });
  const keys = collectCursorKeys(
    document.querySelector(".archive-manager-list"),
    ".archive-manager-package[data-cursor-key]",
  );
  const cursorText = describeResultCursor(keys, archiveCursorPath, t("toolbox.common.enterHint"));
  count.textContent = [base, cursorText].filter(Boolean).join(" · ");
}

function getVisibleArchivePackages() {
  // 文本检索走与 Mod 列表同一套语法（普通词 / 引号短语 / -排除 / re: / tag: 包状态）。
  const searchResult = searchArchivePackages(archiveManagerPackages, archiveManagerQuery);
  archiveManagerSearchResult = searchResult;
  const filtered = searchResult.items.filter((item) => {
    if (archiveManagerStateFilter === "password") return !!item.requiresPassword;
    if (archiveManagerStateFilter === "error") return !!item.error && !item.requiresPassword;
    if (archiveManagerStateFilter === "existing") return (item.vpks || []).some((vpk) => vpk.matchState === "existing");
    if (archiveManagerStateFilter === "new") return (item.vpks || []).some((vpk) => vpk.matchState === "new");
    return true;
  });
  return filtered.sort((left, right) => {
    if (archiveManagerSort === "size-desc" || archiveManagerSort === "size-asc") {
      const delta = Number(left.size || 0) - Number(right.size || 0);
      return archiveManagerSort === "size-desc" ? -delta : delta;
    }
    if (archiveManagerSort === "status") {
      const statusWeight = (item) => {
        if (item.requiresPassword) return 0;
        if (item.error) return 1;
        if ((item.vpks || []).some((vpk) => vpk.matchState === "new")) return 2;
        if ((item.vpks || []).some((vpk) => vpk.matchState === "existing")) return 3;
        return 4;
      };
      const delta = statusWeight(left) - statusWeight(right);
      if (delta) return delta;
    }
    const leftName = String(left.name || "").toLocaleLowerCase();
    const rightName = String(right.name || "").toLocaleLowerCase();
    const delta = ARCHIVE_NAME_COLLATOR.compare(leftName, rightName);
    return archiveManagerSort === "name-desc" ? -delta : delta;
  });
}

function createArchivePackage(item) {
  const section = document.createElement("section");
  section.className = "archive-manager-package";
  // 键盘光标用：每一行都要能被 ↑↓ 找到并高亮。
  section.dataset.cursorKey = String(item.path || "");
  if (item.error) section.classList.add("is-error");
  if (item.requiresPassword) section.classList.add("is-password-required");
  const packageHasExisting = (item.vpks || []).some((vpk) => vpk.matchState === "existing");
  const packageHasNew = (item.vpks || []).some((vpk) => vpk.matchState === "new");
  if (packageHasExisting) section.classList.add("has-existing-vpk");
  if (packageHasNew) section.classList.add("has-new-vpk");
  const header = document.createElement("div");
  header.className = "archive-manager-package-header";
  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.checked = selectedArchivePaths.has(item.path);
  checkbox.title = t("toolbox.archiveManager.checkboxTitle");
  checkbox.onchange = () => {
    if (checkbox.checked) selectedArchivePaths.add(item.path);
    else selectedArchivePaths.delete(item.path);
    section.classList.toggle("is-selected", checkbox.checked);
    updateArchiveSelectionToolbar();
  };
  section.classList.toggle("is-selected", checkbox.checked);
  const title = document.createElement("strong");
  const titleText = `${item.name} · ${String(item.format || "").toUpperCase()}`;
  const highlightSpec = archiveHighlightSpec(archiveManagerSearchResult);
  if (highlightSpec) {
    // highlightMatches 自己负责转义，只把命中区间包进 <mark>（与 Mod 列表同一套样式）。
    title.innerHTML = highlightMatches(titleText, highlightSpec);
  } else {
    title.textContent = titleText;
  }
  title.title = item.path || "";
  const stats = document.createElement("span");
  stats.className = "archive-manager-package-stats";
  const existing = (item.vpks || []).filter((vpk) => vpk.matchState === "existing").length;
  const newer = (item.vpks || []).filter((vpk) => vpk.matchState === "new").length;
  stats.textContent = item.requiresPassword
    ? t("toolbox.archiveManager.statePassword")
    : t("toolbox.archiveManager.statsLine", {
        entries: item.entries?.length || 0,
        vpks: item.vpks?.length || 0,
        existing,
        newer,
      });
  const actions = document.createElement("div");
  actions.className = "archive-manager-package-actions";
  actions.append(
    createArchiveActionButton(
      t("toolbox.archiveManager.actionLocate"),
      t("toolbox.archiveManager.actionLocateTitle"),
      () => callApp("OpenFileLocation", item.path),
    ),
    createArchiveActionButton(
      t("toolbox.archiveManager.actionOpen"),
      t("toolbox.archiveManager.actionOpenTitle"),
      () => callApp("OpenArchivePackage", item.path),
    ),
  );
  header.append(checkbox, title, stats);
  // 用 tag: 搜索时把"命中的状态"贴出来（与 Mod 列表的「匹配：xxx」同一个样式与用意）。
  const activeStateTags = (archiveManagerSearchResult?.syntax?.includeTags || [])
    .flat()
    .map((value) => String(value).toLowerCase());
  if (activeStateTags.length > 0) {
    const hitTags = archivePackageStateTags(item).filter((tag) => activeStateTags.includes(tag.toLowerCase()));
    if (hitTags.length > 0) {
      const chip = document.createElement("span");
      chip.className = "search-reason-chip";
      chip.title = t("toolbox.archiveManager.matchReasonTitle");
      chip.textContent = t("toolbox.archiveManager.matchReasonChip", { tags: hitTags.join(" · ") });
      header.appendChild(chip);
    }
  }
  header.appendChild(actions);
  section.appendChild(header);
  if (item.error) {
    const error = document.createElement("div");
    error.className = "archive-manager-error";
    if (item.requiresPassword) error.classList.add("is-password-required");
    error.textContent = item.error;
    error.title = item.errorDetail || item.error;
    section.appendChild(error);
  }
  if (item.requiresPassword) section.appendChild(createArchivePasswordRetry(item));
  const details = document.createElement("details");
  details.open = expandedArchivePaths.has(item.path);
  section.classList.toggle("is-expanded", details.open);
  details.addEventListener("toggle", () => {
    if (details.open) expandedArchivePaths.add(item.path);
    else expandedArchivePaths.delete(item.path);
    section.classList.toggle("is-expanded", details.open);
  });
  const summary = document.createElement("summary");
  summary.textContent = t("toolbox.archiveManager.detailsSummary", {
    entries: item.entries?.length || 0,
    vpks: item.vpks?.length || 0,
  });
  details.appendChild(summary);
  const tree = document.createElement("div");
  tree.className = "archive-manager-tree";
  const appendEntries = (entries) => entries.forEach((entry) => {
    const row = document.createElement("div");
    row.className = "archive-manager-tree-row";
    row.classList.toggle("is-dir", !!entry.isDir);
    const name = document.createElement("span");
    name.textContent = entry.isDir ? `📁 ${entry.name}` : entry.name;
    const size = document.createElement("span");
    size.textContent = entry.isDir ? t("toolbox.archiveManager.labelDir") : formatBytes(entry.size);
    row.append(name, size);
    tree.appendChild(row);
  });
  const entries = item.entries || [];
  const initialEntries = entries.slice(0, ARCHIVE_TREE_INITIAL_LIMIT);
  appendEntries(initialEntries);
  if (entries.length > initialEntries.length) {
    const more = document.createElement("button");
    more.type = "button";
    more.className = "btn btn-secondary archive-manager-tree-more";
    more.textContent = t("toolbox.archiveManager.showAllEntries", { count: entries.length - initialEntries.length });
    more.onclick = () => {
      appendEntries(entries.slice(initialEntries.length));
      more.remove();
    };
    tree.appendChild(more);
  }
  (item.vpks || []).forEach((vpk) => tree.appendChild(createVPKRow(vpk)));
  details.appendChild(tree);
  section.appendChild(details);
  return section;
}

function updateArchiveSelectionToolbar() {
  const move = document.querySelector(".archive-manager-toolbar .archive-manager-move");
  if (!move) return;
  move.textContent = t("toolbox.archiveManager.moveSelected", { count: selectedArchivePaths.size });
  move.disabled = selectedArchivePaths.size === 0;
}

function createArchiveManagerSession() {
  return beginMessageModalSession({
    onClose: () => {
      archiveManagerSession = null;
      archiveManagerSearchController?.cancel();
      archiveManagerSearchController = null;
      archivePasswordByPath.clear();
      expandedArchivePaths.clear();
    },
  });
}

function createArchiveActionButton(label, title, action) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-secondary archive-manager-package-action";
  button.textContent = label;
  button.title = title;
  button.onclick = async () => {
    button.disabled = true;
    try {
      await action();
    } catch (error) {
      showError(t("toolbox.archiveManager.actionFailed", { label, error: formatError(error) }));
    } finally {
      button.disabled = false;
    }
  };
  return button;
}

function createArchivePasswordRetry(item) {
  const panel = document.createElement("div");
  panel.className = "archive-manager-password-retry";
  const input = document.createElement("input");
  input.type = "password";
  input.autocomplete = "current-password";
  input.placeholder = t("toolbox.archiveManager.passwordPlaceholder");
  input.value = archivePasswordByPath.get(item.path) || "";
  input.title = t("toolbox.archiveManager.passwordHint");
  const retry = document.createElement("button");
  retry.type = "button";
  retry.className = "btn btn-primary";
  retry.textContent = t("toolbox.archiveManager.passwordRetry");
  retry.onclick = async () => {
    const password = input.value;
    if (!password) {
      showNotification(t("toolbox.archiveManager.passwordRequired"), "info");
      input.focus();
      return;
    }
    archivePasswordByPath.set(item.path, password);
    retry.disabled = true;
    try {
      const refreshed = await callApp("ScanArchivePackageWithPassword", item.path, password);
      const index = archiveManagerPackages.findIndex((candidate) => candidate.path === item.path);
      if (index >= 0) archiveManagerPackages[index] = refreshed;
      renderArchiveManager();
    } finally {
      retry.disabled = false;
    }
  };
  panel.append(input, retry);
  return panel;
}

function createVPKRow(vpk) {
  const row = document.createElement("div");
  row.className = "archive-manager-vpk";
  row.classList.add(vpk.matchState === "existing" ? "is-existing" : "is-new");
  const inspectionStatus = vpk.inspectionStatus || (vpk.valid ? "valid" : "invalid");
  if (inspectionStatus === "limited") row.classList.add("is-limited");
  else if (inspectionStatus === "unsupported") row.classList.add("is-unsupported");
  else if (!vpk.valid) row.classList.add("is-invalid");
  const label = document.createElement("strong");
  label.textContent = `VPK · ${vpk.name || vpk.entryPath}`;
  const state = document.createElement("span");
  state.className = "archive-manager-vpk-state";
  const locationText = Array.isArray(vpk.existingLocations) && vpk.existingLocations.length
    ? ` · ${vpk.existingLocations.join(" / ")}`
    : "";
  const gameStateText = vpk.existingGameState === "enabled"
    ? ` · ${t("mods.category.status.gameOn")}`
    : vpk.existingGameState === "disabled"
      ? ` · ${t("mods.category.status.gameOff")}`
      : "";
  state.textContent = inspectionStatus === "limited"
    ? t("toolbox.archiveManager.stateLimited")
    : inspectionStatus === "unsupported"
      ? t("toolbox.archiveManager.stateUnsupported")
    : vpk.valid
      ? (vpk.matchState === "existing"
        ? t("toolbox.archiveManager.stateExisting", { location: locationText, gameState: gameStateText })
        : t("toolbox.archiveManager.stateNew"))
      : t("toolbox.archiveManager.stateReadFailed");
  const meta = document.createElement("span");
  meta.textContent = vpk.valid
    ? t("toolbox.archiveManager.vpkMeta", { size: formatBytes(vpk.size), count: vpk.fileCount })
    : (vpk.error || t("toolbox.common.unknownError"));
  row.append(label, state, meta);
  if (vpk.valid && Array.isArray(vpk.internalFiles) && vpk.internalFiles.length) {
    const preview = document.createElement("small");
    preview.textContent = t("toolbox.archiveManager.internalSample", { list: vpk.internalFiles.slice(0, 5).join("、"), more: vpk.internalFiles.length > 5 ? " …" : "" });
    row.appendChild(preview);
  }
  return row;
}

async function moveSelectedArchives() {
  const paths = [...selectedArchivePaths];
  if (!paths.length) return;
  try {
    const destination = await callApp("SelectDirectory");
    if (!destination) return;
    const conflicts = await callApp("CheckArchiveMoveConflicts", paths, destination);
    const action = conflicts?.length ? await chooseConflictAction(conflicts) : "";
    if (action === null) return;
    const result = await callApp("MoveArchiveFiles", paths, destination, action);
    // 有失败时把原因也带出来（只报数量等于把用户丢在半路）。
    const failureText = formatMoveFailures(result);
    showNotification(
      failureText
        ? t("toolbox.archiveManager.moveDoneFailed", {
            success: result.successCount || 0,
            skipped: result.skippedCount || 0,
            failure: failureText,
          })
        : t("toolbox.archiveManager.moveDone", {
            success: result.successCount || 0,
            skipped: result.skippedCount || 0,
          }),
      result.failCount ? "info" : "success",
    );
    if (failureText) console.error("归档移动失败详情:", result.errors);
    await scanArchiveDirectory(archiveManagerDirectory);
  } catch (error) {
    showError(t("toolbox.archiveManager.moveFailed", { error: formatError(error) }));
  }
}

function chooseConflictAction(conflicts) {
  return new Promise((resolve) => {
    const session = beginMessageModalSession();
    if (!session) return resolve(null);
    session.addClass(session.modal.querySelector(".modal-content"), "archive-manager-modal-content");
    session.titleEl.textContent = t("toolbox.archiveManager.conflictTitle", { count: conflicts.length });
    const content = document.createElement("div");
    content.className = "archive-manager-conflict-choice";
    const message = document.createElement("p");
    message.textContent = t("toolbox.archiveManager.conflictMessage");
    const list = document.createElement("ul");
    conflicts.slice(0, 8).forEach((conflict) => {
      const item = document.createElement("li");
      item.textContent = conflict.targetPath || t("toolbox.archiveManager.conflictTargetFallback");
      list.appendChild(item);
    });
    content.append(message, list);
    session.contentEl.replaceChildren(content);
    const replace = document.createElement("button");
    replace.type = "button";
    replace.className = "btn btn-danger";
    replace.textContent = t("toolbox.archiveManager.conflictReplace");
    const skip = document.createElement("button");
    skip.type = "button";
    skip.className = "btn btn-secondary";
    skip.textContent = t("toolbox.archiveManager.conflictSkip");
    const finish = (value) => { session.close("choice"); resolve(value); };
    replace.onclick = () => finish("replace");
    skip.onclick = () => finish("skip");
    session.addActionButton(replace);
    session.addActionButton(skip);
    session.confirmBtn.textContent = t("toolbox.common.cancel");
    session.confirmBtn.onclick = () => finish(null);
    session.closeBtn.onclick = () => finish(null);
    session.show();
  });
}

function formatBytes(value) {
  const size = Number(value) || 0;
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") return Promise.reject(new Error(t("toolbox.common.backendUnsupported", { method: methodName })));
  return method(...args);
}

function formatError(error) {
  return error?.message || String(error || t("toolbox.common.unknownError"));
}
