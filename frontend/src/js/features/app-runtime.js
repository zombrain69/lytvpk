import {
  initAppShell,
  switchAppPage,
  refreshActiveIndicator,
} from "../core/ui-shell.js";
import {
  getConfig,
  initConfig,
  migrateLegacyLocalStorageIfNeeded,
  saveConfig,
} from "../core/config.js";
import { applyUIScale, setupUIScaleShortcuts } from "../core/ui-scale.js";
import { applyReadingComfort } from "../core/reading-comfort.mjs";
import { setupModalResizers } from "../core/modal-resizer.js";
import { setupFloatingModal } from "../core/floating-modal.js";
import { initTheme, setupThemeToggle } from "../core/theme.js";
import { renderAboutPage } from "./about/about.js";
import { renderDiagnosticsPage } from "./diagnostics/diagnostics-page.js";
import { openVPKUnpackTool } from "./diagnostics/vpk-unpack.js";
import { openMDMPReportTool } from "./diagnostics/mdmp-report.js";
import { openVPKPackTool } from "./diagnostics/vpk-pack.js";
import { openVPKIntegrityTool } from "./diagnostics/vpk-integrity.js";
import { openAutoexecTool } from "./diagnostics/autoexec-tool.js";
import { openArchiveManager } from "./diagnostics/archive-manager.js";
import {
  importSprayFiles,
  importSprayPaths,
  isSprayImportFile,
  isSprayImportPath,
  openSprayTool,
} from "./spray/spray-tool.js";
import {
  configureDropImport,
  handleDropImportPaths,
} from "./drop-import.js";
import {
  configureModelStatsScan,
  openModelStatsScanModal,
} from "./diagnostics/model-stats-scan.js";
import { renderSettingsPage } from "./settings/settings-page.js";
import {
  configureServers,
  setupServerModalListeners,
  openServerModal,
  closeServerModal,
  renderServers,
  refreshAllServers,
  setupLaunchServerMenu,
  initServerStorage,
  getServers,
} from "./servers/servers.js";
import {
  updatePanelUploadTaskInList,
  updatePanelUploadProgress,
  handlePanelUploadTasksCleared,
} from "./servers/panel-modal.js";
import {
  configureUpdates,
  checkAndInstallUpdate,
  manualCheckUpdate,
  showUpdateModal,
} from "./update/updates.js";
import {
  configureConflicts,
  showConflictModal,
  hideConflictModal,
  startConflictCheck,
  toggleScopedConflictAnalysis,
  openConflictScopeModal,
  closeConflictScopeModal,
  applyConflictScopeOptions,
} from "./conflicts/conflicts.js";
import { initConflictRecheck } from "./conflicts/conflict-recheck.js";
import { initModGroupUI, refreshModGroupMembership } from "./mod-groups/group-ui.js";
import { initGroupPicker } from "./mod-groups/group-picker.js";
import { initGroupTagDialog } from "./mod-groups/group-tag.js";
import { initAgentPromptEditor } from "./mod-groups/agent-prompt.js";
import { initStrategyGroupManager } from "./mod-groups/strategy-group-manager.js";
import {
  configureSettings,
  showGlobalSettings,
  renderSettingsPageWithDeps,
} from "./settings/settings.js";
import {
  initProblemModScanAutoRestore,
  openProblemModScanIntro,
} from "./settings/problem-mod-scan.js";
import {
  configureWorkshopBrowser,
  browserState,
  openBrowser,
  loadWorkshopList,
  renderWorkshopSidebar,
  handleProtocolParse,
  handleProtocolWorkshop,
  initWatchLaterStorage,
} from "./workshop/workshop-browser.js";
import { showError, showNotification, handleError } from "../core/toast.js";
import { appState, applyConfigToAppState } from "./state.js";
import { applySearchResultCursor, renderFileList } from "./file-list/render.js";
import { collectResultPaths, describeResultCursor, nextResultPath } from "./file-list/result-cursor.mjs";
import { buildSearchHelpHtml, buildSearchHelpTitle } from "./file-list/search-help.mjs";
import { buildShortcutsHtml, buildShortcutsTitle, isTextEntryElement } from "../core/shortcuts.mjs";
import { startFileOperationWatcher } from "../core/file-operation-watch.js";
import { openCommandPalette, setupCommandPalette } from "../core/command-palette-ui.js";
import { resetAllWindowGeometry } from "../core/floating-modal.js";
import {
  handleSearch,
  performSearch,
  resetFilters,
  renderTagFilters,
  refreshFilesKeepFilter,
} from "./file-list/filters.js";
import { setupSortEvents } from "./file-list/sorting.js";
import {
  selectAll,
  deselectAll,
  enableSelected,
  disableSelected,
  setSelectedGameEnabled,
  exportZipSelected,
  deleteSelected,
  moveSelected,
  cutSelected,
  pasteMoveClipboard,
  clearMoveClipboard,
  transferSelectedWorkshopFiles,
  batchToggleVisibility,
  disableAllMods,
} from "./file-list/actions.js";
import { setupFileListEventDelegation } from "./file-list/events.js";
import { initBoxSelection } from "./file-list/box-selection.js";
import { showServerSubmenu } from "./file-list/context-menu.js";
import { shareSelectedWorkshopItems } from "./file-list/share.js";
import {
  toggleFile,
  toggleGameEnabled,
  moveFileToAddons,
  deleteFile,
  openFileLocation,
  toggleFileVisibility,
  renameFile,
} from "./file-list/operations.js";
import {
  manualRotate,
  initModRotationState,
  toggleModRotation,
} from "./mods/mod-rotation.js";
import {
  openWorkshopModal,
  closeWorkshopModal,
  checkWorkshopUrl,
  downloadWorkshopFile,
  copyCurrentDownloadUrls,
} from "./downloads/workshop-modal.js";
import { startWorkshopClipboardWatch } from "./downloads/clipboard-watch.js";
import {
  refreshTaskList,
  updateTaskInList,
  updateTaskProgress,
  setupClearCompletedTasks,
} from "./downloads/task-list.js";
import { openSetTagsModal, setupTagModalListeners } from "./file-list/tags.js";
import {
  openBatchSetTagsModal,
  setupBatchTagsModalListeners,
} from "./file-list/batch-tags.js";
import { showConfirmModal } from "./modals/confirm.js";
import { showExitModal, closeExitModal, confirmExit } from "./modals/exit.js";
import { showInfoModal, closeInfoModal } from "./modals/info.js";
import { closeModal, showFileDetail } from "./modals/detail.js";
import {
  openLoadOrderModal,
  closeLoadOrderModal,
  saveLoadOrder,
} from "./modals/load-order.js";
import {
  checkInitialDirectory,
  selectDirectory,
  handleUpload,
  launchL4D2,
  initWorkshopState,
  setupPageChangeListeners,
  disableGlobalContextMenu,
  setupInputContextMenu,
} from "./app-init.js";

import {
  HandleFileDrop,
  FetchServerInfo,
  FetchPlayerList,
  ConnectToServer,
  ExportServersToFile,
  GetMapName,
  CheckUpdate,
  GetMirrorsInitial,
  TestMirrorsLatency,
  GetWorkshopPreferredIP,
  GetWorkshopFixedIP,
  GetWorkshopIPOptions,
  GetWorkshopMetaEnabled,
  GetWorkshopUpdateCheckEnabled,
  GetWorkshopAutoRedownload,
  GetOpenWithSettings,
  SetOpenWithSettings,
  ParseWorkshopID,
  SetWorkshopAutoRedownload,
  EnrichAllWorkshopMetadata,
  IsFileOperationBusy,
  GetStockWhitelistStatus,
  GenerateStockWhitelistBatchesFromGame,
  ReloadStockWhitelist,
  ListWorkshopCollections,
  CaptureWorkshopCollection,
  RefreshWorkshopCollection,
  CheckWorkshopCollectionUpdates,
  DownloadWorkshopCollection,
  DeleteWorkshopCollection,
  GetWorkshopBrowserTarget,
  GetWorkshopTranslateProvider,
  GetWorkshopTranslateCustomBaseURL,
  GetWorkshopTranslateCustomModelId,
  HasWorkshopTranslateCustomAPIKey,
  IsSelectingIP,
  GetCurrentBestIP,
  GetCurrentBestIPOption,
  CheckConflicts,
  CheckConflictsForPaths,
  CheckConflictsWithOptions,
  SetWorkshopPreferredIP,
  SetWorkshopFixedIP,
  SetWorkshopMetaEnabled,
  SetWorkshopUpdateCheckEnabled,
  SetWorkshopBrowserTarget,
  SetWorkshopTranslateProvider,
  SetWorkshopTranslateCustomBaseURL,
  SetWorkshopTranslateCustomModelId,
  SetWorkshopTranslateCustomAPIKey,
  DoUpdate,
  RestartApplication,
  FetchWorkshopList,
  FetchWorkshopDetail,
  TranslateWorkshopDescription,
  GetAppVersion,
  CheckModUpdates,
  GetServerStorage,
  SaveServerStorage,
  FetchPanelServerStatus,
  RestartPanelServer,
  FetchPanelMapList,
  FetchPanelMapFiles,
  FetchPanelMapIssues,
  ClearPanelMaps,
  DeletePanelMapFile,
  ChangePanelMap,
  FetchPanelMapHotReloadStatus,
  HotReloadPanelMaps,
  SendPanelRconCommand,
  SelectPanelMapUploadFiles,
  StartPanelMapUpload,
  GetPanelMapUploadTasks,
  RetryPanelMapUpload,
  CancelPanelMapUpload,
  ClearCompletedPanelMapUploads,
  GetWorkshopWatchLaterStorage,
  SaveWorkshopWatchLaterStorage,
  GetProblemModScanSession,
  GetAddonListInfo,
  SaveAddonListManagedSnapshot,
  ListAddonListBackups,
  CreateAddonListBackup,
  RestoreAddonListBackup,
  DeleteAddonListBackup,
  DeleteAddonList,
  SetAddonListGuardEnabled,
  SelectAddonListMergeSource,
  PreviewAddonListMerge,
  ApplyAddonListMerge,
  ListModEnableProfiles,
  CaptureModEnableProfile,
  CaptureModEnableProfileWithAutomation,
  ApplyModEnableProfile,
  DeleteModEnableProfile,
  ExportModEnableProfile,
  ImportModEnableProfile,
  ReportFrontendError,
  GetCrashReportDirectory,
  ListCrashReports,
  RunModHealthCheck,
  RemoveDuplicateAddonListEntries,
  RemoveMissingFileAddonListEntries,
  ListModDependencies,
  SetModDependencies,
  EnableModDependencies,
  DeleteModDependencies,
  EnableAllMissingModDependencies,
  SaveModHealthReport,
  GetAutoexecConfig,
  SaveAutoexecConfig,
  GetAutoexecCommandHelp,
  AnalyzeAutoexecCommands,
  OpenFileLocation,
} from "../../../wailsjs/go/app/App";

import {
  EventsOn,
  OnFileDrop,
  BrowserOpenURL,
  WindowMinimise,
  WindowToggleMaximise,
  WindowGetSize,
  WindowSetSize,
  WindowIsMaximised,
  WindowMaximise,
  WindowUnmaximise,
  ScreenGetAll,
  Quit,
} from "../../../wailsjs/runtime/runtime";
import {
  pickPrimaryScreen,
  sanitizeMainWindowGeometry,
  shouldApplyMainWindowGeometry,
} from "../core/main-window-geometry.mjs";

// 暴露给全局使用，以便在 onclick 中调用
window.BrowserOpenURL = BrowserOpenURL;

const SPRAY_FILE_DROP_FALLBACK_DELAY_MS = 300;
const DOM_SPRAY_DROP_SUPPRESS_MS = 1200;

let fileDropGuardsConfigured = false;
let pendingSprayFileDropFallback = null;
let sprayFileDropFallbackToken = 0;
let lastDomSprayFallbackAt = 0;
let lastDomSprayFallbackNames = new Set();
let eventListenersSetup = false;
let wailsEventsSetup = false;

const ChangePanelDifficulty = (serverID, difficulty) => {
  const method = window?.go?.app?.App?.ChangePanelDifficulty;
  if (typeof method !== "function") {
    return Promise.reject(new Error("当前后端不支持修改难度"));
  }
  return method(serverID, difficulty);
};

const GetAddonListManagerState = async () => {
  const [info, backups] = await Promise.all([
    GetAddonListInfo(),
    ListAddonListBackups(),
  ]);
  return { info, backups };
};

configureServers({
  showError,
  showNotification,
  showConfirmModal,
  switchAppPage,
  FetchServerInfo,
  FetchPlayerList,
  ConnectToServer,
  ExportServersToFile,
  GetMapName,
  GetServerStorage,
  SaveServerStorage,
  BrowserOpenURL,
  FetchPanelServerStatus,
  RestartPanelServer,
  FetchPanelMapList,
  FetchPanelMapFiles,
  FetchPanelMapIssues,
  ClearPanelMaps,
  DeletePanelMapFile,
  ChangePanelMap,
  FetchPanelMapHotReloadStatus,
  HotReloadPanelMaps,
  ChangePanelDifficulty,
  SendPanelRconCommand,
  SelectPanelMapUploadFiles,
  StartPanelMapUpload,
  GetPanelMapUploadTasks,
  RetryPanelMapUpload,
  CancelPanelMapUpload,
  ClearCompletedPanelMapUploads,
});

configureUpdates({
  getConfig,
  saveConfig,
  EventsOn,
  CheckUpdate,
  GetMirrorsInitial,
  TestMirrorsLatency,
  DoUpdate,
  RestartApplication,
});

configureConflicts({
  EventsOn,
  showError,
  CheckConflicts,
  CheckConflictsForPaths,
  CheckConflictsWithOptions,
  toggleFile,
  toggleGameEnabled,
  moveFileToAddons,
  showFileDetail,
  renderFileList,
  showNotification,
});

configureModelStatsScan({
  EventsOn,
  showError,
});

configureDropImport({
  EventsOn,
  HandleFileDrop,
});

configureSettings({
  appState,
  getConfig,
  saveConfig,
  renderFileList,
  renderTagFilters,
  refreshFilesKeepFilter,
  showNotification,
  renderSettingsPage,
  GetWorkshopPreferredIP,
  GetWorkshopFixedIP,
  GetWorkshopIPOptions,
  GetWorkshopMetaEnabled,
  GetWorkshopUpdateCheckEnabled,
  GetWorkshopAutoRedownload,
  GetOpenWithSettings,
  SetOpenWithSettings,
  ParseWorkshopID,
  SetWorkshopAutoRedownload,
  EnrichAllWorkshopMetadata,
  GetStockWhitelistStatus,
  GenerateStockWhitelistBatchesFromGame,
  ReloadStockWhitelist,
  ListWorkshopCollections,
  CaptureWorkshopCollection,
  RefreshWorkshopCollection,
  CheckWorkshopCollectionUpdates,
  DownloadWorkshopCollection,
  DeleteWorkshopCollection,
  GetWorkshopBrowserTarget,
  GetWorkshopTranslateProvider,
  GetWorkshopTranslateCustomBaseURL,
  GetWorkshopTranslateCustomModelId,
  HasWorkshopTranslateCustomAPIKey,
  IsSelectingIP,
  GetCurrentBestIP,
  GetCurrentBestIPOption,
  SetWorkshopPreferredIP,
  SetWorkshopFixedIP,
  SetWorkshopMetaEnabled,
  SetWorkshopUpdateCheckEnabled,
  SetWorkshopBrowserTarget,
  SetWorkshopTranslateProvider,
  SetWorkshopTranslateCustomBaseURL,
  SetWorkshopTranslateCustomModelId,
  SetWorkshopTranslateCustomAPIKey,
  CheckModUpdates,
  EventsOn,
  switchAppPage,
  GetAddonListManagerState,
  SaveAddonListManagedSnapshot,
  CreateAddonListBackup,
  RestoreAddonListBackup,
  DeleteAddonListBackup,
  DeleteAddonList,
  SetAddonListGuardEnabled,
  SelectAddonListMergeSource,
  PreviewAddonListMerge,
  ApplyAddonListMerge,
  ListModEnableProfiles,
  CaptureModEnableProfile,
  CaptureModEnableProfileWithAutomation,
  ApplyModEnableProfile,
  DeleteModEnableProfile,
  ExportModEnableProfile,
  ImportModEnableProfile,
  ReportFrontendError,
  RunModHealthCheck,
  RemoveDuplicateAddonListEntries,
  RemoveMissingFileAddonListEntries,
  ListModDependencies,
  SetModDependencies,
  EnableModDependencies,
  DeleteModDependencies,
  EnableAllMissingModDependencies,
  SaveModHealthReport,
  GetAutoexecConfig,
  SaveAutoexecConfig,
  GetAutoexecCommandHelp,
  AnalyzeAutoexecCommands,
  OpenFileLocation,
});

configureWorkshopBrowser({
  switchAppPage,
  showNotification,
  showError,
  BrowserOpenURL,
  FetchWorkshopList,
  FetchWorkshopDetail,
  TranslateWorkshopDescription,
  GetWorkshopTranslateProvider,
  GetWorkshopBrowserTarget,
  IsSelectingIP,
  closeModal,
  openWorkshopModal,
  EventsOn,
  GetWorkshopWatchLaterStorage,
  SaveWorkshopWatchLaterStorage,
});

// 初始化应用
document.addEventListener("DOMContentLoaded", function () {
  initializeApp().catch((error) => {
    console.error("应用初始化失败:", error);
    showError("应用初始化失败: " + error);
  });
});

const UPDATE_CHECK_INTERVAL = 24 * 60 * 60 * 1000; // 24小时

// installFrontendCrashReporting 把前端的未处理异常/未处理 Promise 拒绝
// 交给后端写成与 Go panic 同格式的本地崩溃报告（不联网、不上传）。
let frontendCrashReportingInstalled = false;
let lastFrontendCrashSignature = "";
function installFrontendCrashReporting() {
  if (frontendCrashReportingInstalled) return;
  frontendCrashReportingInstalled = true;

  const report = (reason, stack) => {
    const message = String(reason?.message || reason || "未知前端错误");
    const detail = String(reason?.stack || stack || "");
    // 同一处错误连续触发（例如渲染循环）时只上报一次，避免刷爆 crashes 目录。
    const signature = `${message}\u0000${detail.slice(0, 200)}`;
    if (signature === lastFrontendCrashSignature) return;
    lastFrontendCrashSignature = signature;
    try {
      const result = ReportFrontendError(message, detail);
      if (result && typeof result.catch === "function") {
        result.catch((error) => console.warn("上报前端异常失败:", error));
      }
    } catch (error) {
      console.warn("上报前端异常失败:", error);
    }
  };

  window.addEventListener("error", (event) => {
    report(event.error || event.message, event.error?.stack || "");
  });
  window.addEventListener("unhandledrejection", (event) => {
    report(event.reason || "未处理的 Promise 拒绝", event.reason?.stack || "");
  });
}

let _updateCheckTimer = null;

async function initUpdateCheck() {
  try {
    const updateCheckEnabled = await GetWorkshopUpdateCheckEnabled();
    appState.workshopUpdateCheckEnabled = updateCheckEnabled;
    if (!updateCheckEnabled) return;

    const config = getConfig();
    const lastCheck = config.lastUpdateCheckTime;
    const now = Date.now();
    if (!lastCheck || now - Number(lastCheck) >= UPDATE_CHECK_INTERVAL) {
      // 延迟30秒后首次检测，避免和IP优选等其他启动任务冲突
      setTimeout(async () => {
        try {
          await CheckModUpdates();
          const nextConfig = getConfig();
          nextConfig.lastUpdateCheckTime = String(Date.now());
          saveConfig(nextConfig);
        } catch (err) {
          console.warn("更新检测失败:", err);
        }
      }, 30000);
    }

    // 每小时检查一次是否超过24小时
    if (_updateCheckTimer) clearInterval(_updateCheckTimer);
    _updateCheckTimer = setInterval(
      async () => {
        const last = getConfig().lastUpdateCheckTime;
        if (!last || Date.now() - Number(last) >= UPDATE_CHECK_INTERVAL) {
          try {
            await CheckModUpdates();
            const nextConfig = getConfig();
            nextConfig.lastUpdateCheckTime = String(Date.now());
            saveConfig(nextConfig);
          } catch (err) {
            console.warn("定时更新检测失败:", err);
          }
        }
      },
      60 * 60 * 1000
    );
  } catch (err) {
    console.warn("更新检测初始化失败:", err);
  }
}

function setupFileDropGuards() {
  if (fileDropGuardsConfigured) return;
  fileDropGuardsConfigured = true;

  window.addEventListener("dragenter", guardFileDragEvent, true);
  window.addEventListener("dragover", guardFileDragEvent, true);
  window.addEventListener("drop", handleGlobalDropEvent, true);
}

function guardFileDragEvent(event) {
  if (!hasExternalFileDragData(event)) return;
  event.preventDefault();
  setDropEffect(event, "copy");
}

function handleGlobalDropEvent(event) {
  const hasExternalFiles = hasExternalFileDragData(event);

  event.preventDefault();
  if (!hasExternalFiles) return;

  setDropEffect(event, "copy");

  const sprayFiles = getDroppedFiles(event).filter(isSprayImportFile);
  if (sprayFiles.length > 0) {
    scheduleSprayFileDropFallback(sprayFiles);
  }
}

function hasExternalFileDragData(event) {
  return getDataTransferTypes(event).includes("Files");
}

function getDataTransferTypes(event) {
  return Array.from(event.dataTransfer?.types || []);
}

function getDroppedFiles(event) {
  return Array.from(event.dataTransfer?.files || []);
}

function setDropEffect(event, effect) {
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = effect;
  }
}

function scheduleSprayFileDropFallback(files) {
  clearSprayFileDropFallback();

  const token = ++sprayFileDropFallbackToken;
  pendingSprayFileDropFallback = window.setTimeout(async () => {
    if (token !== sprayFileDropFallbackToken) return;

    pendingSprayFileDropFallback = null;
    lastDomSprayFallbackAt = Date.now();
    lastDomSprayFallbackNames = new Set(
      files
        .map((file) => String(file?.name || "").toLowerCase())
        .filter(Boolean)
    );
    await importSprayFiles(files, { refreshFilesKeepFilter });
  }, SPRAY_FILE_DROP_FALLBACK_DELAY_MS);
}

function clearSprayFileDropFallback() {
  if (pendingSprayFileDropFallback !== null) {
    window.clearTimeout(pendingSprayFileDropFallback);
    pendingSprayFileDropFallback = null;
  }
  sprayFileDropFallbackToken++;
}

function consumeRecentDomSprayFallbackForPaths(paths) {
  if (Date.now() - lastDomSprayFallbackAt > DOM_SPRAY_DROP_SUPPRESS_MS) {
    return false;
  }
  if (lastDomSprayFallbackNames.size === 0) return false;

  const allPathsMatched = paths.every((path) =>
    lastDomSprayFallbackNames.has(getPathBasename(path).toLowerCase())
  );
  if (!allPathsMatched) return false;

  lastDomSprayFallbackAt = 0;
  lastDomSprayFallbackNames = new Set();
  return true;
}

function getPathBasename(path) {
  return String(path || "").split(/[\\/]/).pop() || "";
}

/**
 * resolveShortcutTargetPath 解析 F2 / Delete 要作用在哪个 Mod 上：
 * 优先用搜索光标那一行（键盘流程），没有光标时退化为"唯一选中项"。
 */
function resolveShortcutTargetPath() {
  const cursorPath = String(appState.searchCursorPath || "");
  if (cursorPath) return cursorPath;
  const selected = Array.from(appState.selectedFiles || []);
  if (selected.length === 1) return String(selected[0]);
  return "";
}

/**
 * restoreMainWindowGeometry 用 config.json 里记住的宽高 / 最大化状态恢复主窗口。
 * - 没记录过（null）或尺寸非法 → 什么都不做，保持 Wails 默认尺寸；
 * - 换到更小的屏幕时按屏幕钳制，避免窗口比屏幕还大、按钮够不着。
 */
async function restoreMainWindowGeometry() {
  const config = getConfig();
  const saved = {
    width: config.mainWindowWidth,
    height: config.mainWindowHeight,
    maximised: config.mainWindowMaximised === true,
  };
  if (!shouldApplyMainWindowGeometry(saved)) return;

  let screen = null;
  try {
    screen = pickPrimaryScreen(await ScreenGetAll());
  } catch (error) {
    console.warn("读取屏幕信息失败，按硬上限恢复主窗口尺寸:", error);
  }
  const geometry = sanitizeMainWindowGeometry(saved, screen);
  if (!geometry) return;
  try {
    WindowSetSize(geometry.width, geometry.height);
    if (geometry.maximised) WindowMaximise();
    else WindowUnmaximise();
  } catch (error) {
    console.warn("恢复主窗口几何失败:", error);
  }
}

/**
 * trackMainWindowGeometry 监听窗口尺寸 / 最大化变化，防抖后写进 config.json。
 * 只在真的变了时才写，避免每次启动都触发一次配置写盘。
 */
function trackMainWindowGeometry() {
  let timer = null;
  const save = async () => {
    timer = null;
    try {
      const [size, maximised] = await Promise.all([WindowGetSize(), WindowIsMaximised()]);
      const geometry = sanitizeMainWindowGeometry({ width: size?.w, height: size?.h, maximised: Boolean(maximised) });
      if (!geometry) return;
      const config = getConfig();
      if (
        config.mainWindowWidth === geometry.width &&
        config.mainWindowHeight === geometry.height &&
        (config.mainWindowMaximised === true) === geometry.maximised
      ) {
        return;
      }
      config.mainWindowWidth = geometry.width;
      config.mainWindowHeight = geometry.height;
      config.mainWindowMaximised = geometry.maximised;
      saveConfig(config);
    } catch (error) {
      console.warn("保存主窗口几何失败:", error);
    }
  };
  const schedule = () => {
    if (timer) window.clearTimeout(timer);
    timer = window.setTimeout(() => void save(), 600);
  };
  window.addEventListener("resize", schedule);
  // 最大化 / 还原不一定会触发 resize（尤其是"还原"），额外听一次 Wails 的窗口事件。
  try {
    EventsOn("wails:window-maximise", schedule);
    EventsOn("wails:window-unmaximise", schedule);
  } catch (error) {
    console.warn("订阅窗口最大化事件失败（改用 resize 兜底）:", error);
  }
}

async function initializeApp() {
  let migratedLegacyConfig = false;
  try {
    migratedLegacyConfig = await migrateLegacyLocalStorageIfNeeded();
  } catch (error) {
    console.error("旧配置迁移失败，已保留旧浏览器存储数据:", error);
  }

  await initConfig();
  // 先定"文字大小 / 阅读舒适度"，再应用整体缩放：根字号是两者相乘的结果。
  applyReadingComfort({
    textSize: getConfig().textSize,
    comfort: getConfig().readingComfort,
    uiScale: getConfig().uiScale,
  });
  applyUIScale(getConfig().uiScale);
  setupUIScaleShortcuts({ getConfig, saveConfig });
  applyConfigToAppState();
  await initServerStorage();
  await initWatchLaterStorage();

  initTheme();
  initAppShell();
  setupModalResizers();
  // 主窗口几何记忆（对齐 FireAxe v0.7.3）：恢复上次的宽高 / 最大化状态，之后跟着保存。
  void restoreMainWindowGeometry();
  trackMainWindowGeometry();
  // 剪贴板工坊链接自动识别（对齐 FireAxe v0.4.0）：复制链接切回来会提示是否解析。
  startWorkshopClipboardWatch();
  // 复杂的管理类窗口统一支持"浮动"：打开后去掉背景虚化、可以直接操作主界面，
  // 标题栏可拖动、边缘可缩放（缩放由 setupModalResizers 提供），偏好逐个窗口记住。
  // 策略组管理窗口在 initStrategyGroupManager 里自己注册（默认值来自 config.json）。
  [
    "mod-group-suggest-modal",
    "load-order-modal",
    "conflict-modal",
    "file-conflict-modal",
    "model-stats-modal",
    // 下面这些同为"复杂管理窗口"：详情 / 提示词编辑 / 标签 / 分组选择 / 服务器面板。
    // 它们都有 .modal-header，浮动按钮会自动插到标题栏里。
    "file-detail-modal",
    "agent-prompt-modal",
    "set-tags-modal",
    "batch-set-tags-modal",
    "group-picker-modal",
    "group-tag-modal",
    "server-details-modal",
    "panel-map-modal",
    "panel-upload-modal",
    "panel-rcon-modal",
    "panel-difficulty-modal",
    "panel-server-details-modal",
  ].forEach((modalId) => setupFloatingModal(modalId, { defaultFloating: true }));
  // 注意：`browser-modal` / `workshop-modal` / `server-modal` 三个**不是弹窗**——
  // ui-shell.js 会把它们的 .modal-content 搬进独立页面（创意工坊 / 下载 / 收藏服务器），
  // 原 modal 只留一个隐藏的壳。给它们注册浮动没有意义（也没有 .modal-content 可浮动）。
  // 「问题 Mod 查找」会真的切换 Mod 开关，要求排查期间保持打开（它自己的底部文案也这么写），
  // 所以只装浮动能力，不允许"点窗口外关闭"。
  setupFloatingModal("problem-scan-modal", { defaultFloating: true, closeOnBackdrop: false });
  setupThemeToggle();
  setupPageChangeListeners();
  setupSettingsAndAboutListeners();
  setupEventListeners();
  setupWailsEvents();
  installFrontendCrashReporting();
  // 文件操作忙碌状态：轮询后端闸门（移动 / 删除 / 打包共用一个锁），
  // 忙碌时状态栏出提示、会写磁盘的批量按钮暂时变灰（CSS 见 reading-comfort.css）。
  startFileOperationWatcher({
    isBusy: IsFileOperationBusy,
    // 后端进入/退出文件操作时会推事件（毫秒级任务也不会被轮询错过）。
    subscribe: (handler) => EventsOn("file_operation_state", (busy) => handler(Boolean(busy))),
  });
  setupCommandPaletteWithDeps();
  // 标题栏的搜索图标 = 命令面板入口（否则只有 Ctrl+K 是"藏起来"的功能）。
  document
    .getElementById("command-palette-btn")
    ?.addEventListener("click", () => openCommandPalette());
  // 变更驱动的冲突自动复检：只注册事件与首次拉取，重算由后端按需触发。
  initConflictRecheck();
  // 策略组：绑定分组筛选/建议弹窗，并拉取一次组归属（扫描完成后 filters 会再刷新）。
  initModGroupUI();
  initGroupPicker();
  initGroupTagDialog();
  initAgentPromptEditor();
  // 「策略组管理」独立窗口：绑定批量工具条 / 建组 / 关窗，并订阅勾选与组归属变化。
  initStrategyGroupManager();
  setupInputContextMenu();
  disableGlobalContextMenu();
  await checkInitialDirectory();
  checkAndInstallUpdate();
  initModRotationState();
  if (migratedLegacyConfig) {
    await initWorkshopState();
  }
  initBoxSelection();
  initUpdateCheck();
  await initProblemModScanAutoRestore();

  if (!window._ipEventsRegistered) {
    EventsOn("ip_selection_start", () => {
      console.log("IP优选开始");
    });

    EventsOn("ip_selection_end", () => {
      console.log("IP优选结束");
      const mainScreen = document.getElementById("main-screen");
      const loadingScreen = document.getElementById("loading-screen");
      if (mainScreen && loadingScreen) {
        loadingScreen.classList.add("hidden");
        mainScreen.classList.remove("hidden");
      }

      const browserModal = document.getElementById("browser-modal");
      if (browserModal && !browserModal.classList.contains("hidden")) {
        browserState.page = 1;
        browserState.data = [];
        loadWorkshopList();
      }
    });
    window._ipEventsRegistered = true;
  }

  if (!window._addonListGuardEventsRegistered) {
    EventsOn("addonlist_guard_restored", async () => {
      showNotification("检测到游戏覆盖 addonlist.txt，已恢复受保护版本", "info");
      try {
        await refreshFilesKeepFilter();
      } catch (error) {
        console.error("自动恢复后刷新 Mod 状态失败:", error);
      }
    });
    window._addonListGuardEventsRegistered = true;
  }

}

function setupSettingsAndAboutListeners() {
  document.addEventListener("app:page-change", (event) => {
    const page = event.detail.page;

    if (page === "settings") {
      renderSettingsPageWithDeps();
    } else if (page === "diagnostics") {
      renderDiagnosticsPage({
        GetProblemModScanSession,
        openProblemModScanIntro,
        openModelStatsScanModal,
        showConflictModal,
        openVPKUnpackTool,
        openVPKIntegrityTool,
        openMDMPReportTool,
        openVPKPackTool,
        openArchiveManager,
        openAutoexecTool,
        openSprayTool,
        refreshFilesKeepFilter,
      });
    } else if (page === "about") {
      renderAboutPage({
        BrowserOpenURL,
        GetAppVersion,
        CheckUpdate,
        showUpdateModal,
        GetCrashReportDirectory,
        ListCrashReports,
        OpenFileLocation,
      });
    }
  });
}

// 设置事件监听器
// waitForSelector 在页面刚切换、内容还在渲染时等待元素出现（命令面板要跳转到设置页的各面板）。
async function waitForSelector(selector, timeoutMs = 1500) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const found = document.querySelector(selector);
    if (found) return found;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  return null;
}

// 命令面板：把"页面跳转 + 常用动作"做成一句检索（Ctrl+K 打开）。
// 动作全部走**已有的按钮 / 页面切换**，不复制业务逻辑，避免两处实现跑偏。
function setupCommandPaletteWithDeps() {
  const gotoSettingsPanel = async (panel, targetId) => {
    switchAppPage("settings");
    const navItem = await waitForSelector(`.settings-nav-item[data-panel="${panel}"]`);
    navItem?.click();
    if (!targetId) return;
    const target = await waitForSelector(`#${targetId}`);
    target?.click();
  };

  const actions = {
    "focus-mod-search": () => {
      switchAppPage("mods");
      document.getElementById("search-input")?.focus();
    },
    "page-mods": () => switchAppPage("mods"),
    "page-workshop": () => switchAppPage("workshop"),
    "page-downloads": () => switchAppPage("downloads"),
    "page-servers": () => switchAppPage("servers"),
    "page-diagnostics": () => switchAppPage("diagnostics"),
    "page-settings": () => switchAppPage("settings"),
    "page-about": () => switchAppPage("about"),
    "interface-settings": () => void gotoSettingsPanel("interface", null),
    "strategy-group-manager": () => document.getElementById("mod-group-manager-btn")?.click(),
    "group-suggest": () => document.getElementById("mod-group-suggest-btn")?.click(),
    "load-order": () => {
      switchAppPage("mods");
      document.getElementById("load-order-toolbar-btn")?.click();
    },
    "conflict-analysis": () => {
      switchAppPage("mods");
      document.getElementById("conflict-analysis-checkbox")?.click();
    },
    "health-check": () => void gotoSettingsPanel("addonlist", "settings-health-run"),
    "workshop-enrich": () => void gotoSettingsPanel("workshop", "settings-workshop-enrich"),
    "toggle-theme": () => document.getElementById("theme-toggle-btn")?.click(),
    // 快捷键总览：直接用标题栏那个按钮，保证"命令面板点开的"和"按钮点开的"是同一个东西。
    "show-shortcuts": () => document.getElementById("shortcuts-help-btn")?.click(),
    "reset-window-geometry": () => {
      const cleared = resetAllWindowGeometry();
      showNotification(
        cleared > 0
          ? `已重置 ${cleared} 个窗口的位置与大小；重新打开窗口会回到默认位置`
          : "窗口位置本来就是默认值",
        "success",
      );
    },
  };

  setupCommandPalette({
    runCommand: async (id) => {
      const action = actions[id];
      if (!action) {
        showNotification("这条命令暂时没有实现", "info");
        return;
      }
      await action();
    },
  });
}

function setupEventListeners() {
	if (eventListenersSetup) return;
	eventListenersSetup = true;

	// 窗口控制
  const minBtn = document.getElementById("w-min-btn");
  const maxBtn = document.getElementById("w-max-btn");
  const closeBtn = document.getElementById("w-close-btn");

  if (minBtn) minBtn.addEventListener("click", WindowMinimise);
  if (maxBtn) maxBtn.addEventListener("click", WindowToggleMaximise);
  if (closeBtn) closeBtn.addEventListener("click", Quit);

  // 标题栏双击最大化/还原
  const titleBar = document.querySelector(".title-drag-region");
  if (titleBar) {
    titleBar.addEventListener("dblclick", WindowToggleMaximise);
  }

  // 目录选择
  document
    .getElementById("select-directory-btn")
    ?.addEventListener("click", selectDirectory);

  // 刷新按钮
  document
    .getElementById("refresh-btn")
    ?.addEventListener("click", refreshFilesKeepFilter);
  document
    .getElementById("load-order-toolbar-btn")
    ?.addEventListener("click", () => {
      const selectedPaths = Array.from(appState.selectedFiles);
      openLoadOrderModal(
        selectedPaths.length > 0
          ? { mode: "selection", selectedPaths }
          : { mode: "global" }
      );
    });

  // 搜索框
  document
    .getElementById("search-input")
    ?.addEventListener("input", handleSearch);

  // 搜索语法说明书：悬停提示与 `?` 浮层都从 search-help.mjs 生成，避免"界面说的和实际行为不一致"。
  const searchInput = document.getElementById("search-input");
  if (searchInput) searchInput.title = buildSearchHelpTitle();
  const searchHelpBtn = document.getElementById("search-help-btn");
  const searchHelpPopover = document.getElementById("search-help-popover");
  if (searchHelpPopover) searchHelpPopover.innerHTML = buildSearchHelpHtml();
  if (searchHelpBtn && searchHelpPopover) {
    const setHelpOpen = (open) => {
      searchHelpPopover.classList.toggle("hidden", !open);
      searchHelpBtn.setAttribute("aria-expanded", String(open));
      searchHelpBtn.classList.toggle("is-active", open);
    };
    searchHelpBtn.addEventListener("click", (event) => {
      event.stopPropagation();
      setHelpOpen(searchHelpPopover.classList.contains("hidden"));
    });
    document.addEventListener("click", (event) => {
      if (searchHelpPopover.classList.contains("hidden")) return;
      if (searchHelpPopover.contains(event.target) || searchHelpBtn.contains(event.target)) return;
      setHelpOpen(false);
    });
    document.addEventListener("keydown", (event) => {
      if (event.key !== "Escape" || searchHelpPopover.classList.contains("hidden")) return;
      setHelpOpen(false);
    });
  }

  // 快捷键总览（标题栏 `?` 按钮 / 按 `?` / 命令面板）：内容来自 core/shortcuts.mjs，
  // 与搜索说明书里的快捷键段同源 —— 改键位说明只改那一处。
  const shortcutsBtn = document.getElementById("shortcuts-help-btn");
  const shortcutsPopover = document.getElementById("shortcuts-help-popover");
  if (shortcutsPopover) shortcutsPopover.innerHTML = buildShortcutsHtml();
  if (shortcutsBtn) shortcutsBtn.title = buildShortcutsTitle();
  if (shortcutsBtn && shortcutsPopover) {
    const setShortcutsOpen = (open) => {
      shortcutsPopover.classList.toggle("hidden", !open);
      shortcutsBtn.setAttribute("aria-expanded", String(open));
      shortcutsBtn.classList.toggle("is-active", open);
    };
    shortcutsBtn.addEventListener("click", (event) => {
      event.stopPropagation();
      setShortcutsOpen(shortcutsPopover.classList.contains("hidden"));
    });
    document.addEventListener("click", (event) => {
      if (shortcutsPopover.classList.contains("hidden")) return;
      if (shortcutsPopover.contains(event.target) || shortcutsBtn.contains(event.target)) return;
      setShortcutsOpen(false);
    });
    document.addEventListener("keydown", (event) => {
      // Esc 关掉总览；`?` 键切换。光标在输入框里时不当快捷键用（否则打不了问号）。
      if (event.key === "Escape" && !shortcutsPopover.classList.contains("hidden")) {
        setShortcutsOpen(false);
        return;
      }
      if (event.key !== "?" || event.ctrlKey || event.metaKey || event.altKey) return;
      const target = event.target;
      if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) return;
      event.preventDefault();
      setShortcutsOpen(shortcutsPopover.classList.contains("hidden"));
    });
  }

  // 检索快捷键：Ctrl+F 聚焦搜索框，Esc 清空（沿用"搜索后能一键回到全部"的习惯）。
  document.addEventListener("keydown", (event) => {
    const searchInput = document.getElementById("search-input");
    if (!searchInput) return;

    // Ctrl+K：打开命令面板（页面跳转 + 常用动作的统一入口）。
    if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === "k") {
      const palette = document.getElementById("command-palette-modal");
      if (!palette) return;
      event.preventDefault();
      openCommandPalette();
      return;
    }

    if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === "f") {
      event.preventDefault();
      searchInput.focus();
      searchInput.select();
      return;
    }

    // Ctrl+X / Ctrl+V：对齐 FireAxe v0.5.1 的"剪切 / 移动"。
    // 本项目没有"当前组"概念，粘贴时用现有"移动到…"的目录选择器确定目标。
    // 只把"正在输入文字"的控件算作输入框：列表行的复选框/按钮拿到焦点时，
    // Ctrl+X / Ctrl+V 仍然要生效（真机踩坑：勾选一行后快捷键失灵）。
    const isEditableTarget = isTextEntryElement(event.target);
    if (
      (event.ctrlKey || event.metaKey) &&
      !event.altKey &&
      !isEditableTarget &&
      event.key.toLowerCase() === "x"
    ) {
      event.preventDefault();
      cutSelected();
      return;
    }
    if (
      (event.ctrlKey || event.metaKey) &&
      !event.altKey &&
      !isEditableTarget &&
      event.key.toLowerCase() === "v"
    ) {
      event.preventDefault();
      void pasteMoveClipboard();
      return;
    }

    // ↑ / ↓ 在结果里移动键盘光标（焦点仍留在搜索框，手不用离开键盘）。
    if (
      (event.key === "ArrowDown" || event.key === "ArrowUp") &&
      document.activeElement === searchInput
    ) {
      const container = document.getElementById("file-list");
      const paths = collectResultPaths(container);
      if (paths.length === 0) return;
      event.preventDefault();
      appState.searchCursorPath = nextResultPath(
        paths,
        appState.searchCursorPath,
        event.key === "ArrowDown" ? 1 : -1,
      );
      applySearchResultCursor();
      return;
    }

    // Enter 打开光标所在行的详情（等价于点那一行的"详情"按钮）。
    if (event.key === "Enter" && document.activeElement === searchInput) {
      const container = document.getElementById("file-list");
      const path = String(appState.searchCursorPath || "");
      if (!path) return;
      const row = container?.querySelector(`.file-item[data-path="${CSS.escape(path)}"]`);
      const detailButton = row?.querySelector(".detail-btn");
      if (!detailButton) return;
      event.preventDefault();
      detailButton.click();
      return;
    }

    if (event.key === "Escape" && document.activeElement === searchInput && searchInput.value) {
      event.preventDefault();
      searchInput.value = "";
      handleSearch({ target: searchInput });
      appState.searchCursorPath = "";
      applySearchResultCursor();
      searchInput.blur();
      return;
    }

    // Esc 取消"待移动"标记（只在没有别的可关闭对象时生效）。
    if (event.key === "Escape" && appState.moveClipboard?.size > 0) {
      event.preventDefault();
      clearMoveClipboard();
      return;
    }

    // F2 / Delete：对齐 FireAxe v0.7.0 的编辑快捷键。
    // 目标是"光标所在那一行"，没有光标时退化为"唯一选中的那个"。
    if (event.key === "F2" || event.key === "Delete") {
      const selectedCount = (appState.selectedFiles || new Set()).size;
      // Delete 支持批量：勾选了多个就直接走批量删除（和"批量删除"按钮同一条链路）。
      if (event.key === "Delete" && !appState.searchCursorPath && selectedCount > 1) {
        event.preventDefault();
        void deleteSelected();
        return;
      }
      const target = resolveShortcutTargetPath();
      if (!target) {
        showNotification(
          event.key === "F2"
            ? "先用 ↑↓ 把光标移到要重命名的 Mod 上（或只选中一个）"
            : "先用 ↑↓ 把光标移到要删除的 Mod 上（或勾选要删除的那些）",
          "info",
        );
        return;
      }
      event.preventDefault();
      if (event.key === "F2") void renameFile(target);
      else void deleteFile(target);
    }
  });

  // 显示隐藏文件复选框
  const showHiddenCheckbox = document.getElementById("show-hidden-checkbox");
  if (showHiddenCheckbox) {
    showHiddenCheckbox.checked = appState.showHidden;
    showHiddenCheckbox.addEventListener("change", (e) => {
      appState.showHidden = e.target.checked;
      deselectAll();
      performSearch();
    });
  }

  // 排序功能
  setupSortEvents();
  setupFilterDropdownEvents();

  // 批量操作按钮
  setupBatchActionEvents();

  // 文件列表事件委托
  setupFileListEventDelegation();

  // 标签模态框事件
  setupTagModalListeners();
  setupBatchTagsModalListeners();

  // 清除已完成任务
  setupClearCompletedTasks();

  // ESC 键取消所有 mod 选择
  document.addEventListener("keydown", function (e) {
    if (e.key !== "Escape") return;

    // 如果有模态框打开，不处理（让模态框的 ESC 处理优先）
    const visibleModal = document.querySelector(".modal:not(.hidden)");
    if (visibleModal) return;

    // 如果图片预览弹窗打开，不处理
    const imagePreview = document.getElementById("image-preview-modal");
    if (imagePreview && imagePreview.style.display === "flex") return;

    // 如果焦点在输入元素上，不处理
    const activeElement = document.activeElement;
    if (
      activeElement &&
      (activeElement.tagName === "INPUT" ||
        activeElement.tagName === "TEXTAREA" ||
        activeElement.tagName === "SELECT" ||
        activeElement.isContentEditable)
    ) {
      return;
    }

    // 如果右键菜单打开，不处理
    const contextMenu = document.querySelector(".context-menu");
    if (contextMenu) return;

    // 如果当前不在主页面，不处理
    const mainScreen = document.getElementById("main-screen");
    if (!mainScreen || mainScreen.classList.contains("hidden")) return;

    // 取消所有选择
    if (appState.selectedFiles.size > 0) {
      deselectAll();
    }
  });
}

function setupFilterDropdownEvents() {
  document.addEventListener("click", (event) => {
    if (
      !event.target.closest(".multi-select-dropdown, .single-select-dropdown")
    ) {
      document
        .querySelectorAll(".select-menu, .multi-select-menu")
        .forEach((menu) => {
          menu.classList.add("hidden");
        });
    }
  });
}

function setupBatchActionEvents() {
  document
    .getElementById("select-all-btn")
    ?.addEventListener("click", selectAll);
  document
    .getElementById("deselect-all-btn")
    ?.addEventListener("click", deselectAll);
  document
    .getElementById("enable-selected-btn")
    ?.addEventListener("click", enableSelected);
  document
    .getElementById("disable-selected-btn")
    ?.addEventListener("click", disableSelected);
  // 批量"游戏内开关"（只写 addonlist.txt 的 0/1，不搬文件）。
  document
    .getElementById("game-enable-selected-btn")
    ?.addEventListener("click", () => void setSelectedGameEnabled(true));
  document
    .getElementById("game-disable-selected-btn")
    ?.addEventListener("click", () => void setSelectedGameEnabled(false));

  const closeBatchDisableDropdown = () => {
    const dropdown = document.getElementById("batch-disable-dropdown-content");
    const button = document.getElementById("batch-disable-menu-btn");
    if (dropdown) dropdown.classList.add("hidden");
    button?.setAttribute("aria-expanded", "false");
  };

  const batchDisableMenuBtn = document.getElementById("batch-disable-menu-btn");
  if (batchDisableMenuBtn) {
    batchDisableMenuBtn.addEventListener("click", (e) => {
      e.stopPropagation();

      document.querySelectorAll(".dropdown-content").forEach((d) => {
        if (d.id !== "batch-disable-dropdown-content") {
          d.classList.add("hidden");
          const fileItem = d.closest(".file-item");
          if (fileItem) fileItem.classList.remove("active-dropdown");
        }
      });

      const dropdown = document.getElementById(
        "batch-disable-dropdown-content"
      );
      if (dropdown) {
        const willOpen = dropdown.classList.contains("hidden");
        dropdown.classList.toggle("hidden");
        batchDisableMenuBtn.setAttribute("aria-expanded", String(willOpen));
      }
    });
  }

  document.addEventListener("click", (e) => {
    if (!e.target.closest(".batch-disable-split-container")) {
      closeBatchDisableDropdown();
    }
  });

  // 批量操作下拉菜单
  const batchMoreBtn = document.getElementById("batch-more-btn");
  if (batchMoreBtn) {
    batchMoreBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      closeBatchDisableDropdown();

      document.querySelectorAll(".dropdown-content").forEach((d) => {
        if (d.id !== "batch-dropdown-content") {
          d.classList.add("hidden");
          const fileItem = d.closest(".file-item");
          if (fileItem) fileItem.classList.remove("active-dropdown");
        }
      });

      const batchUploadBtn = document.getElementById("batch-upload-server-btn");
      if (batchUploadBtn) {
        const hasServers = getServers().some(
          (s) => s.panelUrl && s.panelPasswordSet
        );
        batchUploadBtn.style.display = hasServers ? "" : "none";
      }

      const dropdown = document.getElementById("batch-dropdown-content");
      dropdown?.classList.toggle("hidden");
    });
  }

  const closeBatchDropdown = () => {
    const dropdown = document.getElementById("batch-dropdown-content");
    if (dropdown) dropdown.classList.add("hidden");
  };

  const bindBatchDisableAction = (buttonId, primaryTag = "") => {
    const button = document.getElementById(buttonId);
    if (!button) return;

    button.addEventListener("click", () => {
      closeBatchDisableDropdown();
      disableAllMods(primaryTag);
    });
  };

  bindBatchDisableAction("disable-all-mods-btn");
  bindBatchDisableAction("disable-all-character-mods-btn", "人物");
  bindBatchDisableAction("disable-all-weapon-mods-btn", "武器");

  document
    .getElementById("delete-selected-btn")
    ?.addEventListener("click", () => {
      closeBatchDropdown();
      deleteSelected();
    });

  const exportZipSelectedBtn = document.getElementById(
    "export-zip-selected-btn"
  );
  if (exportZipSelectedBtn) {
    exportZipSelectedBtn.addEventListener("click", () => {
      closeBatchDropdown();
      exportZipSelected();
    });
  }

  const batchSetTagsBtn = document.getElementById("batch-set-tags-btn");
  if (batchSetTagsBtn) {
    batchSetTagsBtn.addEventListener("click", () => {
      closeBatchDropdown();
      openBatchSetTagsModal();
    });
  }

  const shareSelectedBtn = document.getElementById("share-selected-btn");
  if (shareSelectedBtn) {
    shareSelectedBtn.addEventListener("click", () => {
      closeBatchDropdown();
      shareSelectedWorkshopItems();
    });
  }

  const moveSelectedBtn = document.getElementById("move-selected-btn");
  if (moveSelectedBtn) {
    moveSelectedBtn.addEventListener("click", () => {
      closeBatchDropdown();
      moveSelected();
    });
  }

  const transferWorkshopSelectedBtn = document.getElementById(
    "transfer-workshop-selected-btn",
  );
  if (transferWorkshopSelectedBtn) {
    transferWorkshopSelectedBtn.addEventListener("click", () => {
      closeBatchDropdown();
      transferSelectedWorkshopFiles();
    });
  }

  const batchUploadServerBtn = document.getElementById(
    "batch-upload-server-btn"
  );
  if (batchUploadServerBtn) {
    batchUploadServerBtn.addEventListener("click", () => {
      const filePaths = Array.from(appState.selectedFiles);
      if (filePaths.length === 0) {
        showNotification("请先选择文件", "info");
        return;
      }
      showServerSubmenu(batchUploadServerBtn, filePaths);
    });
  }

  const hideSelectedBtn = document.getElementById("hide-selected-btn");
  if (hideSelectedBtn) {
    hideSelectedBtn.addEventListener("click", () => {
      closeBatchDropdown();
      batchToggleVisibility(false);
    });
  }

  const unhideSelectedBtn = document.getElementById("unhide-selected-btn");
  if (unhideSelectedBtn) {
    unhideSelectedBtn.addEventListener("click", () => {
      closeBatchDropdown();
      batchToggleVisibility(true);
    });
  }

  // 检查更新按钮
  const checkUpdateBtn = document.getElementById("check-update-btn");
  if (checkUpdateBtn) {
    checkUpdateBtn.addEventListener("click", manualCheckUpdate);
  }

  // 重置筛选按钮
  document
    .getElementById("reset-filter-btn")
    ?.addEventListener("click", resetFilters);

  document
    .getElementById("conflict-analysis-checkbox")
    ?.addEventListener("change", (event) => {
      toggleScopedConflictAnalysis(event.target.checked);
    });

  document
    .getElementById("conflict-analysis-options-btn")
    ?.addEventListener("click", openConflictScopeModal);
  document
    .getElementById("close-conflict-scope-modal")
    ?.addEventListener("click", closeConflictScopeModal);
  document
    .getElementById("cancel-conflict-scope-btn")
    ?.addEventListener("click", closeConflictScopeModal);
  document
    .getElementById("apply-conflict-scope-btn")
    ?.addEventListener("click", applyConflictScopeOptions);
  document.querySelectorAll("[data-conflict-match-mode]").forEach((button) => {
    button.addEventListener("click", () => {
      document.querySelectorAll("[data-conflict-match-mode]").forEach((item) => {
        const active = item === button;
        item.classList.toggle("active", active);
        item.setAttribute("aria-pressed", active ? "true" : "false");
      });
    });
  });
  document.querySelector('[data-conflict-scope-rule="tag"]')?.addEventListener("change", (event) => {
    const tagSelect = document.getElementById("conflict-scope-tag");
    if (tagSelect) tagSelect.disabled = !event.target.checked;
  });

  // 冲突检测弹窗按钮
  document
    .getElementById("close-conflict-modal")
    ?.addEventListener("click", hideConflictModal);
  document
    .getElementById("close-conflict-btn")
    ?.addEventListener("click", hideConflictModal);
  document
    .getElementById("start-conflict-check-btn")
    ?.addEventListener("click", startConflictCheck);

  // Mod随机轮换按钮
  document
    .getElementById("mod-rotation-btn")
    ?.addEventListener("click", toggleModRotation);

  // 服务器收藏按钮
  document
    .getElementById("server-favorites-btn")
    ?.addEventListener("click", openServerModal);

  setupServerModalListeners();
  setupLaunchServerMenu();

  // 启动L4D2按钮
  document
    .getElementById("launch-l4d2-btn")
    ?.addEventListener("click", launchL4D2);

  // 关于信息按钮
  document.getElementById("info-btn")?.addEventListener("click", showInfoModal);

  // 处理关于页面的外部链接
  document.querySelectorAll(".info-link").forEach((link) => {
    link.addEventListener("click", (e) => {
      e.preventDefault();
      const url = link.getAttribute("href");
      if (url) {
        BrowserOpenURL(url);
      }
    });
  });

  setupFileDropGuards();

  // 阻止浏览器默认的拖拽行为
  window.addEventListener("dragover", (e) => e.preventDefault());
  window.addEventListener("drop", (e) => e.preventDefault());
  window.addEventListener("dragstart", (e) => {
    if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA") return;
    e.preventDefault();
  });

  // 退出确认模态框事件
  document
    .getElementById("close-exit-modal-btn")
    ?.addEventListener("click", closeExitModal);
  document
    .getElementById("exit-cancel-btn")
    ?.addEventListener("click", closeExitModal);
  document
    .getElementById("exit-confirm-btn")
    ?.addEventListener("click", confirmExit);

  document
    .getElementById("exit-confirm-modal")
    ?.addEventListener("click", function (e) {
      if (e.target === this) {
        closeExitModal();
      }
    });

  // 模态框关闭按钮
  document
    .getElementById("close-modal-header-btn")
    ?.addEventListener("click", closeModal);
  document
    .getElementById("close-info-modal-btn")
    ?.addEventListener("click", closeInfoModal);
  document
    .getElementById("close-load-order-modal-btn")
    ?.addEventListener("click", closeLoadOrderModal);
  document
    .getElementById("cancel-load-order-btn")
    ?.addEventListener("click", closeLoadOrderModal);
  document
    .getElementById("confirm-load-order-btn")
    ?.addEventListener("click", saveLoadOrder);

  // 创意工坊按钮
  document
    .getElementById("workshop-btn")
    ?.addEventListener("click", openWorkshopModal);

  // 工坊浏览器按钮
  document
    .getElementById("browser-btn")
    ?.addEventListener("click", openBrowser);

  // 上传按钮
  document
    .getElementById("upload-btn")
    ?.addEventListener("click", handleUpload);

  document
    .getElementById("check-workshop-btn")
    ?.addEventListener("click", checkWorkshopUrl);

  // 粘贴按钮事件
  document
    .getElementById("paste-workshop-url-btn")
    ?.addEventListener("click", async function () {
      try {
        const text = await navigator.clipboard.readText();
        document.getElementById("workshop-url").value = text;
        showNotification("已粘贴", "success");
      } catch (err) {
        console.error("粘贴失败:", err);
        showError("粘贴失败，请使用 Ctrl+V");
      }
    });

  document
    .getElementById("paste-download-url-btn")
    ?.addEventListener("click", async function () {
      try {
        const text = await navigator.clipboard.readText();
        const input = document.getElementById("download-url");
        input.value = text;
        input.dispatchEvent(new Event("input"));
        showNotification("已粘贴", "success");
      } catch (err) {
        console.error("粘贴失败:", err);
        showError("粘贴失败，请使用 Ctrl+V");
      }
    });

  // 「打开文件方式」（对齐 FireAxe v0.7.2 的 process file customization）的保存 / 恢复默认。
  // 用事件委托绑定：设置面板会整块重渲染，直接绑在按钮上的监听器会在重渲染后失效
  // （真机上的表现就是"按钮拿到了焦点，但点了没反应"）。
  document.addEventListener("click", async (event) => {
    const saveButton = event.target?.closest?.("#settings-open-with-save");
    const resetButton = event.target?.closest?.("#settings-open-with-reset");
    if (!saveButton && !resetButton) return;
    event.preventDefault();

    const programInput = document.getElementById("settings-open-with-program");
    const argsInput = document.getElementById("settings-open-with-arguments");
    const status = document.getElementById("settings-open-with-status");
    const program = resetButton ? "" : String(programInput?.value || "");
    const argumentsTemplate = resetButton ? "" : String(argsInput?.value || "");

    const button = saveButton || resetButton;
    button.disabled = true;
    try {
      const applied = await SetOpenWithSettings(program, argumentsTemplate);
      const config = getConfig();
      config.openWithProgram = applied?.program ?? program;
      config.openWithArguments = applied?.arguments ?? argumentsTemplate;
      await saveConfig(config);
      if (programInput) programInput.value = config.openWithProgram;
      if (argsInput) argsInput.value = config.openWithArguments;
      if (status) {
        status.textContent = config.openWithProgram
          ? `已保存：以后"打开所在位置"会用 ${config.openWithProgram}`
          : "当前使用系统默认（资源管理器定位文件）";
      }
      showNotification(
        config.openWithProgram ? "已保存：用自定义程序打开/定位" : "已恢复系统默认打开方式",
        "success",
      );
    } catch (error) {
      if (status) status.textContent = `保存失败：${String(error?.message || error)}`;
      showError("保存打开方式失败: " + error);
    } finally {
      button.disabled = false;
    }
  });

  document.getElementById("download-url")?.addEventListener("input", (e) => {
    const val = e.target.value;
    const optimizedIpContainer = document.getElementById(
      "optimized-ip-container"
    );
    if (val.includes("cdn.steamusercontent.com")) {
      optimizedIpContainer?.classList.remove("hidden");
    } else {
      optimizedIpContainer?.classList.add("hidden");
      const checkbox = document.getElementById("use-optimized-ip-global");
      if (checkbox) checkbox.checked = false;
    }
  });

  document
    .getElementById("download-workshop-btn")
    ?.addEventListener("click", downloadWorkshopFile);

  // 复制下载链接按钮
  document
    .getElementById("copy-url-btn")
    ?.addEventListener("click", copyCurrentDownloadUrls);

  // 点击模态框外部关闭
  document
    .getElementById("file-detail-modal")
    ?.addEventListener("click", function (e) {
      if (e.target === this) {
        closeModal();
      }
    });

  document
    .getElementById("workshop-modal")
    ?.addEventListener("click", function (e) {
      if (e.target === this) {
        closeWorkshopModal();
      }
    });

  document
    .getElementById("info-modal")
    ?.addEventListener("click", function (e) {
      if (e.target === this) {
        closeInfoModal();
      }
    });

  document
    .getElementById("load-order-modal")
    ?.addEventListener("click", function (e) {
      if (e.target === this) {
        closeLoadOrderModal();
      }
    });

  // 冲突检测及其范围设置弹窗也支持点击遮罩关闭，避免窄窗口下只能寻找底部按钮。
  document
    .getElementById("conflict-modal")
    ?.addEventListener("click", function (e) {
      if (e.target === this) {
        hideConflictModal();
      }
    });

  document
    .getElementById("conflict-scope-modal")
    ?.addEventListener("click", function (e) {
      if (e.target === this) {
        closeConflictScopeModal();
      }
    });

  console.log("事件监听器已设置");
}

function setupWailsEvents() {
	if (wailsEventsSetup) return;
	wailsEventsSetup = true;

	console.log("正在初始化 Wails 事件监听...");

  EventsOn("error", handleError);

  EventsOn("task_updated", (task) => {
    updateTaskInList(task);
  });

  EventsOn("task_progress", (task) => {
    updateTaskProgress(task);
  });

  EventsOn("tasks_cleared", () => {
    refreshTaskList();
  });

  EventsOn("panel_upload_task_updated", (task) => {
    updatePanelUploadTaskInList(task);
  });

  EventsOn("panel_upload_task_progress", (task) => {
    updatePanelUploadProgress(task);
  });

  EventsOn("panel_upload_tasks_cleared", () => {
    handlePanelUploadTasksCleared();
  });

  EventsOn("show_exit_confirmation", () => {
    showExitModal();
  });

  OnFileDrop(async (x, y, paths) => {
    const droppedPaths = Array.from(paths || []).filter(Boolean);
    if (droppedPaths.length === 0) return;

    clearSprayFileDropFallback();

    const sprayPaths = droppedPaths.filter(isSprayImportPath);
    const otherPaths = droppedPaths.filter((path) => !isSprayImportPath(path));
    if (
      sprayPaths.length > 0 &&
      !consumeRecentDomSprayFallbackForPaths(sprayPaths)
    ) {
      await importSprayPaths(sprayPaths, { refreshFilesKeepFilter });
    }
    if (otherPaths.length > 0) {
      handleDropImportPaths(otherPaths, { source: "drop" });
    }
  }, false);

  EventsOn("refresh_files", () => {
    refreshFilesKeepFilter();
  });

  EventsOn("show_toast", (data) => {
    if (data.type === "error") {
      showError(data.message);
    } else {
      showNotification(data.message, data.type || "success");
    }
  });

  EventsOn("rotation_log", (msg) => {
    console.log(`[ModRotation] ${msg}`);
  });

  EventsOn("protocol:parse", (data) => {
    console.log("收到协议解析请求:", data);
    if (data && data.workshopId) {
      handleProtocolParse(data.workshopId);
    }
  });

  EventsOn("protocol:workshop", (data) => {
    console.log("收到协议打开工坊请求:", data);
    if (data && data.workshopId) {
      handleProtocolWorkshop(data.workshopId);
    }
  });

  EventsOn("protocol:error", (data) => {
    console.error("协议处理错误:", data);
    if (data && data.message) {
      showError(`协议处理失败: ${data.message}`);
    }
  });

  // 监听Mod更新检测事件
  EventsOn("mod_update_check_complete", () => {
    refreshFilesKeepFilter();
  });
}

// 暴露给全局使用，以便在 onclick 中调用
window.showFileDetail = showFileDetail;
window.toggleFile = toggleFile;
window.manualRotate = manualRotate;
window.openFileLocation = openFileLocation;
window.toggleFileVisibility = toggleFileVisibility;
window.moveFileToAddons = moveFileToAddons;
window.deleteFile = deleteFile;
window.renameFile = renameFile;
window.openSetTagsModal = openSetTagsModal;
window.openLoadOrderModal = openLoadOrderModal;
window.openWorkshopModal = openWorkshopModal;
window.closeWorkshopModal = closeWorkshopModal;
window.checkWorkshopUrl = checkWorkshopUrl;
window.downloadWorkshopFile = downloadWorkshopFile;
