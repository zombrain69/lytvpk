import { showError, showNotification } from "../../core/toast.js";
import { beginMessageModalSession } from "../../core/message-modal.js";

let archiveManagerRunning = false;
let archiveManagerSession = null;
let archiveManagerDirectory = "";
let archiveManagerPackages = [];
const selectedArchivePaths = new Set();
const expandedArchivePaths = new Set();
const archivePasswordByPath = new Map();
let archiveManagerQuery = "";
let archiveManagerSort = "name-asc";
let archiveManagerStateFilter = "all";
let archiveManagerDensity = "compact";
let archiveManagerSearchTimer = 0;

const ARCHIVE_TREE_INITIAL_LIMIT = 160;

export async function openArchiveManager() {
  if (archiveManagerRunning) {
    showNotification("压缩包正在扫描或移动", "info");
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
    await scanArchiveDirectory(directory);
  } catch (error) {
    showError("选择压缩包目录失败: " + formatError(error));
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
    showNotification(`扫描完成：找到 ${archiveManagerPackages.length} 个压缩包`, "success");
  } catch (error) {
    archiveManagerSession?.close("error");
    archiveManagerSession = null;
    showError("扫描压缩包失败: " + formatError(error));
  } finally {
    archiveManagerRunning = false;
  }
}

function showArchiveLoading(directory) {
  const session = createArchiveManagerSession();
  if (!session) return;
  archiveManagerSession = session;
  session.addClass(session.modal.querySelector(".modal-content"), "archive-manager-modal-content");
  session.titleEl.textContent = "压缩包管理";
  session.contentEl.textContent = `正在扫描：${directory}`;
  session.confirmBtn.textContent = "取消";
  session.confirmBtn.onclick = () => session.close("cancel");
  session.closeBtn.onclick = () => session.close("cancel");
  session.show();
}

function renderArchiveManager() {
  const session = archiveManagerSession || createArchiveManagerSession();
  if (!session) return;
  archiveManagerSession = session;
  session.addClass(session.modal.querySelector(".modal-content"), "archive-manager-modal-content");
  session.titleEl.textContent = `压缩包管理 (${archiveManagerPackages.length})`;
  session.contentEl.replaceChildren(createArchiveContent());
  session.confirmBtn.textContent = "关闭";
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
  summary.textContent = `目录：${archiveManagerDirectory} · 显示 ${visiblePackages.length}/${archiveManagerPackages.length} 个压缩包 · 支持 ZIP / RAR / 7Z / TAR / TAR.GZ`;
  const search = document.createElement("input");
  search.type = "search";
  search.className = "archive-manager-search";
  search.placeholder = "搜索压缩包名、路径或 VPK 名称";
  search.value = archiveManagerQuery;
  search.setAttribute("aria-label", "搜索压缩包");
  search.oninput = () => {
    archiveManagerQuery = search.value;
    if (archiveManagerSearchTimer) window.clearTimeout(archiveManagerSearchTimer);
    archiveManagerSearchTimer = window.setTimeout(() => {
      archiveManagerSearchTimer = 0;
      renderArchiveManager();
      requestAnimationFrame(() => {
        const next = document.querySelector(".archive-manager-search");
        next?.focus();
        if (next) next.setSelectionRange(archiveManagerQuery.length, archiveManagerQuery.length);
      });
    }, 120);
  };
  const sort = document.createElement("select");
  sort.className = "archive-manager-sort";
  sort.setAttribute("aria-label", "排序方式");
  [
    ["name-asc", "名称 A-Z"],
    ["name-desc", "名称 Z-A"],
    ["status", "状态"],
    ["size-desc", "大小从大到小"],
    ["size-asc", "大小从小到大"],
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
  stateFilter.setAttribute("aria-label", "状态筛选");
  [
    ["all", "全部状态"],
    ["existing", "已有 Mod"],
    ["new", "待导入"],
    ["password", "需要密码"],
    ["error", "读取失败"],
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
  density.textContent = archiveManagerDensity === "compact" ? "舒适显示" : "紧凑显示";
  density.title = archiveManagerDensity === "compact" ? "增大卡片和文字，方便阅读" : "缩小卡片，显示更多压缩包";
  density.onclick = () => {
    archiveManagerDensity = archiveManagerDensity === "compact" ? "comfortable" : "compact";
    renderArchiveManager();
  };
  const selectAll = document.createElement("button");
  selectAll.type = "button";
  selectAll.className = "btn btn-secondary";
  selectAll.textContent = "全选";
  selectAll.onclick = () => {
    archiveManagerPackages.forEach((item) => selectedArchivePaths.add(item.path));
    renderArchiveManager();
  };
  const clearAll = document.createElement("button");
  clearAll.type = "button";
  clearAll.className = "btn btn-secondary";
  clearAll.textContent = "取消全选";
  clearAll.onclick = () => {
    selectedArchivePaths.clear();
    renderArchiveManager();
  };
  const move = document.createElement("button");
  move.type = "button";
  move.className = "btn btn-primary archive-manager-move";
  move.textContent = `移动已选 (${selectedArchivePaths.size})`;
  move.disabled = selectedArchivePaths.size === 0;
  move.onclick = () => moveSelectedArchives();
  const refresh = document.createElement("button");
  refresh.type = "button";
  refresh.className = "btn btn-secondary";
  refresh.textContent = "刷新全部";
  refresh.title = "重新遍历当前目录下的压缩包；搜索和排序不会触发全量扫描";
  refresh.onclick = () => scanArchiveDirectory(archiveManagerDirectory);
  toolbar.append(summary, search, sort, stateFilter, density, refresh, selectAll, clearAll, move);
  wrapper.appendChild(toolbar);

  const list = document.createElement("div");
  list.className = "archive-manager-list";
  if (archiveManagerPackages.length === 0) {
    const empty = document.createElement("div");
    empty.className = "archive-manager-empty";
    empty.textContent = "没有找到支持的压缩包。";
    list.appendChild(empty);
  }
  visiblePackages.forEach((item) => list.appendChild(createArchivePackage(item)));
  wrapper.appendChild(list);
  return wrapper;
}

function getVisibleArchivePackages() {
  const query = archiveManagerQuery.trim().toLocaleLowerCase();
  const filtered = archiveManagerPackages.filter((item) => {
    const haystack = [
      item.name,
      item.path,
      ...(item.vpks || []).map((vpk) => `${vpk.name || ""} ${vpk.entryPath || ""}`),
    ].join(" ").toLocaleLowerCase();
    if (query && !haystack.includes(query)) return false;
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
    const delta = leftName.localeCompare(rightName, undefined, { numeric: true, sensitivity: "base" });
    return archiveManagerSort === "name-desc" ? -delta : delta;
  });
}

function createArchivePackage(item) {
  const section = document.createElement("section");
  section.className = "archive-manager-package";
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
  checkbox.title = "选择此压缩包以批量移动";
  checkbox.onchange = () => {
    if (checkbox.checked) selectedArchivePaths.add(item.path);
    else selectedArchivePaths.delete(item.path);
    section.classList.toggle("is-selected", checkbox.checked);
    updateArchiveSelectionToolbar();
  };
  section.classList.toggle("is-selected", checkbox.checked);
  const title = document.createElement("strong");
  title.textContent = `${item.name} · ${String(item.format || "").toUpperCase()}`;
  title.title = item.path || "";
  const stats = document.createElement("span");
  stats.className = "archive-manager-package-stats";
  const existing = (item.vpks || []).filter((vpk) => vpk.matchState === "existing").length;
  const newer = (item.vpks || []).filter((vpk) => vpk.matchState === "new").length;
  stats.textContent = item.requiresPassword
    ? "需要密码后读取"
    : `${item.entries?.length || 0} 项 · VPK ${item.vpks?.length || 0} · 已有 ${existing} · 待导入 ${newer}`;
  const actions = document.createElement("div");
  actions.className = "archive-manager-package-actions";
  actions.append(
    createArchiveActionButton("定位", "在文件夹中定位此压缩包", () => callApp("OpenFileLocation", item.path)),
    createArchiveActionButton("打开", "用系统默认程序打开此压缩包", () => callApp("OpenArchivePackage", item.path)),
  );
  header.append(checkbox, title, stats, actions);
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
  summary.textContent = `查看文件树与 VPK 信息（${item.entries?.length || 0} 项 / ${item.vpks?.length || 0} 个 VPK）`;
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
    size.textContent = entry.isDir ? "目录" : formatBytes(entry.size);
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
    more.textContent = `显示全部条目（剩余 ${entries.length - initialEntries.length}）`;
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
  move.textContent = `移动已选 (${selectedArchivePaths.size})`;
  move.disabled = selectedArchivePaths.size === 0;
}

function createArchiveManagerSession() {
  return beginMessageModalSession({
    onClose: () => {
      archiveManagerSession = null;
      if (archiveManagerSearchTimer) {
        window.clearTimeout(archiveManagerSearchTimer);
        archiveManagerSearchTimer = 0;
      }
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
      showError(`${label}压缩包失败: ${formatError(error)}`);
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
  input.placeholder = "输入此 7Z 的密码";
  input.value = archivePasswordByPath.get(item.path) || "";
  input.title = "密码仅用于当前窗口的本次扫描，不会保存到设置或磁盘";
  const retry = document.createElement("button");
  retry.type = "button";
  retry.className = "btn btn-primary";
  retry.textContent = "使用密码重试";
  retry.onclick = async () => {
    const password = input.value;
    if (!password) {
      showNotification("请输入 7Z 密码后再重试", "info");
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
  else if (!vpk.valid) row.classList.add("is-invalid");
  const label = document.createElement("strong");
  label.textContent = `VPK · ${vpk.name || vpk.entryPath}`;
  const state = document.createElement("span");
  state.className = "archive-manager-vpk-state";
  const locationText = Array.isArray(vpk.existingLocations) && vpk.existingLocations.length
    ? ` · ${vpk.existingLocations.join(" / ")}`
    : "";
  const gameStateText = vpk.existingGameState === "enabled"
    ? " · 游戏内开启"
    : vpk.existingGameState === "disabled"
      ? " · 游戏内关闭"
      : "";
  state.textContent = inspectionStatus === "limited"
    ? "目录读取受限"
    : vpk.valid
      ? (vpk.matchState === "existing" ? `已有 Mod${locationText}${gameStateText}` : "待导入 addons")
      : "VPK 读取失败";
  const meta = document.createElement("span");
  meta.textContent = vpk.valid
    ? `${formatBytes(vpk.size)} · ${vpk.fileCount} 个内部文件`
    : (vpk.error || "未知错误");
  row.append(label, state, meta);
  if (vpk.valid && Array.isArray(vpk.internalFiles) && vpk.internalFiles.length) {
    const preview = document.createElement("small");
    preview.textContent = `内部示例：${vpk.internalFiles.slice(0, 5).join("、")}${vpk.internalFiles.length > 5 ? " …" : ""}`;
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
    showNotification(`移动完成：成功 ${result.successCount || 0}，跳过 ${result.skippedCount || 0}，失败 ${result.failCount || 0}`, result.failCount ? "info" : "success");
    await scanArchiveDirectory(archiveManagerDirectory);
  } catch (error) {
    showError("移动压缩包失败: " + formatError(error));
  }
}

function chooseConflictAction(conflicts) {
  return new Promise((resolve) => {
    const session = beginMessageModalSession();
    if (!session) return resolve(null);
    session.addClass(session.modal.querySelector(".modal-content"), "archive-manager-modal-content");
    session.titleEl.textContent = `发现 ${conflicts.length} 个文件冲突`;
    const content = document.createElement("div");
    content.className = "archive-manager-conflict-choice";
    const message = document.createElement("p");
    message.textContent = "目标位置已有同名压缩包，请选择本批次的处理方式：";
    const list = document.createElement("ul");
    conflicts.slice(0, 8).forEach((conflict) => {
      const item = document.createElement("li");
      item.textContent = conflict.targetPath || "目标文件";
      list.appendChild(item);
    });
    content.append(message, list);
    session.contentEl.replaceChildren(content);
    const replace = document.createElement("button");
    replace.type = "button";
    replace.className = "btn btn-danger";
    replace.textContent = "替换";
    const skip = document.createElement("button");
    skip.type = "button";
    skip.className = "btn btn-secondary";
    skip.textContent = "跳过";
    const finish = (value) => { session.close("choice"); resolve(value); };
    replace.onclick = () => finish("replace");
    skip.onclick = () => finish("skip");
    session.addActionButton(replace);
    session.addActionButton(skip);
    session.confirmBtn.textContent = "取消";
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
  if (typeof method !== "function") return Promise.reject(new Error(`当前后端不支持 ${methodName}`));
  return method(...args);
}

function formatError(error) {
  return error?.message || String(error || "未知错误");
}
