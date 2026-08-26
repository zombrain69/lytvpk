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
  workshopUpdateCheckEnabled: false,
  // 当前筛选结果的可选冲突分析。默认关闭，避免扫描大量 VPK。
  conflictAnalysisEnabled: false,
  conflictAnalysisLoading: false,
  conflictAnalysisResult: null,
  conflictByPath: new Map(),
};

export function applyConfigToAppState(config = getConfig()) {
  appState.displayMode = config.displayMode || "list";
  appState.boxSelectionEnabled = config.boxSelectionEnabled || false;
  appState.ctrlClickSelectionEnabled =
    config.ctrlClickSelectionEnabled || false;
  appState.filterLayoutMode = config.filterLayoutMode || "compact";
}

export function toggleFileSelection(filePath, selected) {
  if (selected) {
    appState.selectedFiles.add(filePath);
  } else {
    appState.selectedFiles.delete(filePath);
  }
  updateStatusBar();
}

export function updateStatusBar() {
  const totalFiles = appState.allVpkFiles.length;
  const enabledFiles = appState.allVpkFiles.filter((f) => f.enabled).length;
  const disabledFiles = totalFiles - enabledFiles;
  const gameEnabledFiles = appState.allVpkFiles.filter(
    (file) => file.gameStateKnown && file.gameEnabled
  ).length;
  const gameDisabledFiles = appState.allVpkFiles.filter(
    (file) => file.gameStateKnown && !file.gameEnabled
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
