import { getConfig, saveConfig } from "../core/config.js";
import { refreshActiveIndicator } from "../core/ui-shell.js";

export function getDefaultDirectory() {
  return getConfig().defaultDirectory || "";
}

export function setDefaultDirectory(directory) {
  const config = getConfig();
  config.defaultDirectory = directory;
  saveConfig(config);
}

// 应用状态
export const appState = {
  allVpkFiles: [],
  vpkFiles: [],
  primaryTags: [],
  selectedPrimaryTag: "",
  selectedSecondaryTags: [],
  secondaryMatchMode: "any",
  selectedLocations: [],
  selectedGameStates: [],
  searchQuery: "",
  selectedFiles: new Set(),
  selectionAnchorPath: "",
  currentDirectory: "",
  isLoading: false,
  showHidden: false,
  sortType: "name",
  sortOrder: "asc",
  loadOrderMap: new Map(),
  displayMode: getConfig().displayMode || "list",
  boxSelectionEnabled: getConfig().boxSelectionEnabled || false,
  ctrlClickSelectionEnabled: getConfig().ctrlClickSelectionEnabled || false,
  filterLayoutMode: getConfig().filterLayoutMode || "compact",
  workshopUpdateCheckEnabled: getConfig().workshopUpdateCheckEnabled || false,
  // 当前筛选结果的可选冲突分析。默认关闭，避免扫描大量 VPK。
  conflictAnalysisEnabled: false,
  conflictAnalysisLoading: false,
  conflictAnalysisResult: null,
  conflictByPath: new Map(),
  conflictAnalysisOptions: {
    matchMode: "or",
    baselineRules: [{ type: "enabled" }],
  },
  conflictAnalysisScopeLabel: "游戏内开启",
};

export function applyConfigToAppState(config = getConfig()) {
  appState.displayMode = config.displayMode || "list";
  appState.boxSelectionEnabled = config.boxSelectionEnabled || false;
  appState.ctrlClickSelectionEnabled =
    config.ctrlClickSelectionEnabled || false;
  appState.filterLayoutMode = config.filterLayoutMode || "compact";
  appState.workshopUpdateCheckEnabled = config.workshopUpdateCheckEnabled || false;
}

export function toggleFileSelection(filePath, selected) {
  if (selected) {
    appState.selectedFiles.add(filePath);
  } else {
    appState.selectedFiles.delete(filePath);
  }
  updateSelectedFilesStatus();
}

export function updateSelectedFilesStatus() {
  const selectedEl = document.getElementById("selected-files");
  if (selectedEl) selectedEl.textContent = `已选择: ${appState.selectedFiles.size}`;
}

// 应用常见桌面文件选择手势：普通点击单选/取消，Ctrl（或 macOS 的
// Command）保留其它选择并切换当前项，Shift 按当前排序后的列表选择范围。
// selected 只用于普通点击；Shift 选择始终把范围加入选择集。
export function applyFileSelectionGesture(
  filePath,
  { shiftKey = false, ctrlKey = false, metaKey = false, selected = true } = {},
) {
  const orderedPaths = (appState.vpkFiles || []).map((file) => file.path);
  const targetIndex = orderedPaths.indexOf(filePath);
  const additive = ctrlKey || metaKey;
  const previousSelected = shiftKey ? new Set(appState.selectedFiles) : null;

  if (shiftKey && targetIndex >= 0) {
    const anchorIndex = orderedPaths.indexOf(appState.selectionAnchorPath);
    const start = anchorIndex >= 0 ? Math.min(anchorIndex, targetIndex) : targetIndex;
    const end = anchorIndex >= 0 ? Math.max(anchorIndex, targetIndex) : targetIndex;
    if (!additive) {
      appState.selectedFiles.clear();
    }
    for (let index = start; index <= end; index += 1) {
      appState.selectedFiles.add(orderedPaths[index]);
    }
  } else if (additive) {
    if (appState.selectedFiles.has(filePath)) {
      appState.selectedFiles.delete(filePath);
    } else {
      appState.selectedFiles.add(filePath);
    }
  } else if (selected) {
    appState.selectedFiles.add(filePath);
  } else {
    appState.selectedFiles.delete(filePath);
  }

  appState.selectionAnchorPath = filePath;
  updateSelectedFilesStatus();

  if (previousSelected) {
    const changedPaths = new Set([...previousSelected, ...appState.selectedFiles]);
    return changedPaths;
  }
  return new Set([filePath]);
}

export function updateStatusBar() {
  const totalFiles = appState.allVpkFiles.length;
  const enabledFiles = appState.allVpkFiles.filter((f) => f.enabled).length;
  const disabledFiles = totalFiles - enabledFiles;
  const gameEnabledFiles = appState.allVpkFiles.filter(
    // disabled 目录中的文件即使 addonlist.txt 仍残留 1，也不会被游戏加载。
    (file) => file.enabled !== false && file.gameStateKnown && file.gameEnabled
  ).length;
  const gameDisabledFiles = appState.allVpkFiles.filter(
    (file) => file.enabled !== false && file.gameStateKnown && !file.gameEnabled
  ).length;
  const selectedCount = appState.selectedFiles.size;

  const totalEl = document.getElementById("total-files");
  const enabledEl = document.getElementById("enabled-files");
  const disabledEl = document.getElementById("disabled-files");
  const gameEnabledEl = document.getElementById("game-enabled-files");
  const gameDisabledEl = document.getElementById("game-disabled-files");
  const selectedEl = document.getElementById("selected-files");

  if (totalEl) totalEl.textContent = `总文件数: ${totalFiles}`;
  if (enabledEl) enabledEl.textContent = `已启用: ${enabledFiles}`;
  if (disabledEl) disabledEl.textContent = `已禁用: ${disabledFiles}`;
  if (gameEnabledEl) gameEnabledEl.textContent = `游戏内开启: ${gameEnabledFiles}`;
  if (gameDisabledEl) gameDisabledEl.textContent = `游戏内关闭: ${gameDisabledFiles}`;
  if (selectedEl) selectedEl.textContent = `已选择: ${selectedCount}`;
}

export function showFileListLoading(message = "正在加载...") {
  const loading = document.getElementById("file-list-loading");
  const loadingMessage = document.getElementById("file-list-loading-message");
  if (!loading) return;
  if (loadingMessage) loadingMessage.textContent = message;
  loading.classList.remove("hidden");
  disableActionButtons();
}

export function hideFileListLoading() {
  document.getElementById("file-list-loading")?.classList.add("hidden");
  enableActionButtons();
}

function disableActionButtons() {
  const buttons = [
    "refresh-btn",
    "reset-filter-btn",
    "select-directory-btn",
    "select-all-btn",
    "deselect-all-btn",
    "enable-selected-btn",
    "disable-selected-btn",
    "batch-disable-menu-btn",
  ];
  buttons.forEach((id) => {
    const btn = document.getElementById(id);
    if (btn) {
      btn.disabled = true;
      btn.style.opacity = "0.5";
      btn.style.cursor = "not-allowed";
    }
  });
}

export function enableActionButtons() {
  const buttons = [
    "refresh-btn",
    "reset-filter-btn",
    "select-directory-btn",
    "select-all-btn",
    "deselect-all-btn",
    "enable-selected-btn",
    "disable-selected-btn",
    "batch-disable-menu-btn",
  ];
  buttons.forEach((id) => {
    const btn = document.getElementById(id);
    if (btn) {
      btn.disabled = false;
      btn.style.opacity = "";
      btn.style.cursor = "";
    }
  });
}

export function showLoadingScreen() {
  document.getElementById("loading-screen")?.classList.remove("hidden");
  document.getElementById("main-screen")?.classList.add("hidden");
  disableActionButtons();
}

export function showMainScreen() {
  document.getElementById("loading-screen")?.classList.add("hidden");
  document.getElementById("file-list-loading")?.classList.add("hidden");
  document.getElementById("main-screen")?.classList.remove("hidden");
  enableActionButtons();
  refreshActiveIndicator();
}

export function updateLoadingMessage(message) {
  const el = document.getElementById("loading-message");
  if (el) el.textContent = message;
}
