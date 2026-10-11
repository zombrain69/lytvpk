import { attachLineNumberGutter } from "../../core/line-number-editor.js";
import { t } from "../../core/i18n-runtime.js";
import { applyUIScale, MAX_UI_SCALE, MIN_UI_SCALE, normalizeUIScale, UI_SCALE_STEP } from "../../core/ui-scale.js";
import {
  READING_COMFORT_OPTIONS,
  TEXT_SIZE_OPTIONS,
  applyReadingComfort,
  normalizeReadingComfort,
  normalizeTextSize,
  readingComfortLabel,
  textSizeLabel,
} from "../../core/reading-comfort.mjs";
import { createAutoexecSettingsController } from "./autoexec-settings-controller.mjs";
import {
  CONFLICT_IGNORE_LIST_DESCRIPTION,
  formatConflictIgnoreList,
  parseConflictIgnoreList,
} from "../conflicts/conflict-ignore-options.mjs";
import {
  formatCollectionCheckAllSummary,
  formatCollectionQueueSummary,
  formatCollectionRefreshSummary,
  formatCollectionSummary,
} from "./workshop-collection-format.mjs";
import {
  buildHealthIssueRows,
  formatHealthReportSummary,
  summarizeHealthAutoFix,
} from "./health-report-format.mjs";
import { describeHealthSearch, healthHighlightSpec, searchHealthIssues } from "./health-search.mjs";
import { highlightMatches } from "../file-list/search-match.mjs";
import {
  HEALTH_SEARCH_HELP_VARIANT,
  buildSearchHelpHtml,
  buildSearchHelpTitle,
} from "../file-list/search-help.mjs";
import {
  formatDependencyBatchEnableSummary,
  formatDependencyEnableSummary,
} from "./dependency-format.mjs";
import { buildHealthReportMarkdown } from "./health-report-markdown.mjs";
import { buildDependencyArgs } from "./selection-args.mjs";
import { formatProfileApplySummary } from "./profile-format.mjs";
import { copyTextToClipboard } from "../file-list/share.js";
// 策略组管理窗口已经独立：设置页只留一个入口按钮，真正的生命周期操作在那个窗口里。
import { openStrategyGroupManager } from "../mod-groups/strategy-group-manager.js";
import { confirmInApp, showConfirmModal } from "../modals/confirm.js";
import { showPromptModal } from "../modals/prompt.js";
import { isValidDNSAddress } from "./workshop-dns.mjs";

const SETTINGS_NAV_ICONS = {
  network: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 0 20"/><path d="M12 2a15.3 15.3 0 0 0 0 20"/></svg>`,
  interface: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="14" rx="2"/><path d="M8 20h8"/><path d="M12 18v2"/></svg>`,
  workshop: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2a7 7 0 0 1 7 7c0 2.38-1.19 4.47-3 5.74V17a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2v-2.26C6.19 13.47 5 11.38 5 9a7 7 0 0 1 7-7z"/><path d="M9 21h6"/></svg>`,
  addonlist: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M5 3h11l3 3v15H5z"/><path d="M16 3v4h4"/><path d="M8 11h8M8 15h8"/></svg>`,
};

let addonListMergePreview = null;
let filterLayoutChangeToken = 0;
let displayModeChangeToken = 0;
let uiScaleChangeToken = 0;
let boxSelectionChangeToken = 0;
let ctrlClickSelectionChangeToken = 0;
// 工坊信息存储与更新检测是级联设置。共享令牌可以阻止一次尚未完成的
// 级联保存被另一项设置的旧请求覆盖。
let workshopSettingsChangeToken = 0;
// 工坊合集那块的状态文字：保存 / 检查更新 / 删除都会整页重渲染，
// 只写在 DOM 里的话消息会立刻被冲掉（保存成功看不到"已保存…"，
// 检查更新也看不到"新增 X 个 / 本地缺少 Y 个"），所以在这里留一份。
let workshopCollectionStatusText = "";
// Settings data is assembled from several asynchronous Wails calls.  Opening
// the page again (or refreshing an addonlist action) can start a newer render
// while an older one is still waiting; only the newest render may replace DOM
// or bind controls.
let settingsRenderGeneration = 0;

function syncWorkshopDataControls(metaEnabled, updateCheckEnabled) {
  const metaToggle = document.getElementById("settings-meta-enabled");
  const updateCheckRow = document.getElementById("settings-update-check-row");
  const updateCheckToggle = document.getElementById("settings-update-check-enabled");
  const toggleSwitch = updateCheckToggle?.closest(".toggle-switch");
  const checkSection = document.getElementById("settings-update-check-section");
  const manualCheckBtn = document.getElementById("settings-manual-check-btn");
  const effectiveUpdateCheckEnabled = Boolean(metaEnabled && updateCheckEnabled);

  if (metaToggle) metaToggle.checked = Boolean(metaEnabled);
  if (updateCheckToggle) {
    updateCheckToggle.checked = effectiveUpdateCheckEnabled;
    updateCheckToggle.disabled = !metaEnabled;
  }
  updateCheckRow?.classList.toggle("setting-row-disabled", !metaEnabled);
  toggleSwitch?.classList.toggle("toggle-switch-disabled", !metaEnabled);
  if (checkSection) checkSection.style.display = effectiveUpdateCheckEnabled ? "" : "none";
  if (manualCheckBtn) manualCheckBtn.disabled = !effectiveUpdateCheckEnabled;
}

export async function renderSettingsPage(deps) {
  const {
    appState,
    getConfig,
    saveConfig,
    renderFileList,
    renderTagFilters,
    refreshFilesKeepFilter,
    showNotification,
    GetWorkshopPreferredIP,
    GetWorkshopFixedIP,
    GetWorkshopIPOptions,
    GetWorkshopMetaEnabled,
    GetWorkshopUpdateCheckEnabled,
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
    ApplyModEnableProfile,
    DeleteModEnableProfile,
    ExportModEnableProfile,
    ImportModEnableProfile,
    RunModHealthCheck,
    RemoveDuplicateAddonListEntries,
    RemoveMissingFileAddonListEntries,
    ListModDependencies,
    SetModDependencies,
    EnableModDependencies,
    DeleteModDependencies,
    EnableAllMissingModDependencies,
    GetAutoexecConfig,
    SaveAutoexecConfig,
    GetAutoexecCommandHelp,
    AnalyzeAutoexecCommands,
    OpenFileLocation,
    // 下面这些在函数体里当裸变量使用，必须一并解构，否则运行时 ReferenceError
    // （历史缺陷：自动联动、方案自动化快照、体检报告导出都因此失效）。
    GetWorkshopAutoRedownload,
    ListWorkshopCollections,
    CaptureModEnableProfileWithAutomation,
    SaveModHealthReport,
  } = deps;
  const container = document.getElementById("settings-page-content");
  if (!container) return;
  const renderGeneration = ++settingsRenderGeneration;
  const pageView = container.closest(".page-view");
  const isCurrentRender = () =>
    renderGeneration === settingsRenderGeneration &&
    container.isConnected &&
    (!pageView || pageView.classList.contains("active"));
  const config = getConfig();
  // Individual Wails reads must not turn the whole settings page into a blank
  // view. A transient failure (for example while the app is reloading bindings)
  // keeps the relevant control usable with the last persisted frontend value,
  // and makes the degraded source visible to the user.
  const settingsReadErrors = [];
  const readSetting = async (label, reader, fallback) => {
    try {
      if (typeof reader !== "function") {
        throw new Error(t("settings.page.apiUnavailable"));
      }
      return await reader();
    } catch (error) {
      settingsReadErrors.push(`${label}：${String(error?.message || error || t("settings.page.readFailed"))}`);
      return fallback;
    }
  };
  const unrecordedModLoadOrderPlacement = ["start", "after-enabled", "end"].includes(config.unrecordedModLoadOrderPlacement)
    ? config.unrecordedModLoadOrderPlacement
    : "end";

  const enabled = await readSetting(t("settings.readLabels.preferredIP"), GetWorkshopPreferredIP, Boolean(config.workshopPreferredIP));
  const fixedIP = await readSetting(t("settings.readLabels.fixedIP"), GetWorkshopFixedIP, config.workshopFixedIP || "");
  const useFixedIP = enabled && fixedIP !== "";
  // 工坊 DNS（对齐上游 5ce7ef5 的最小版本）：配置缺省/非法时按"系统 DNS"显示。
  const workshopDNS = config.workshopDNS && typeof config.workshopDNS === "object" ? config.workshopDNS : {};
  const workshopDNSMode = workshopDNS.mode === "custom" ? "custom" : "system";
  const workshopDNSAddress = workshopDNSMode === "custom" ? String(workshopDNS.customAddress || "") : "";
  const workshopDNSStatus = workshopDNSMode === "custom"
    ? t("settings.network.dns.statusCustom", { address: workshopDNSAddress })
    : t("settings.network.dns.statusSystem");
  // 自定义标签规则：读不到就显示空模板（读取失败不该让整页渲染失败）。
  let customTagRulesText = "";
  try {
    customTagRulesText = (await deps.GetCustomTagRules?.()) || "";
  } catch (error) {
    console.warn("读取自定义标签规则失败:", error);
    customTagRulesText = "";
  }
  const metaEnabled = await readSetting(t("settings.readLabels.metaEnabled"), GetWorkshopMetaEnabled, Boolean(config.workshopMetaEnabled));
  // 更新检测依赖工坊信息存储。旧配置可能留下两者不一致的组合；渲染时
  // 以实际可执行的状态为准，避免禁用的开关仍显示内容和操作入口。
  const storedUpdateCheckEnabled = await readSetting(t("settings.readLabels.updateCheck"), GetWorkshopUpdateCheckEnabled, Boolean(config.workshopUpdateCheckEnabled));
  const updateCheckEnabled = Boolean(metaEnabled && storedUpdateCheckEnabled);
  const autoRedownloadEnabled = await readSetting(t("settings.readLabels.autoRedownload"), GetWorkshopAutoRedownload, false);
  // 剪贴板工坊链接自动识别（对齐 FireAxe 的默认开启）：存在 config.json，读前端配置即可。
  const autoDetectWorkshopLink = deps.getConfig().autoDetectWorkshopLink !== false;
  const browserTarget = await readSetting(t("settings.readLabels.browserTarget"), GetWorkshopBrowserTarget, config.workshopBrowserTarget || "mirror");
  const translateProvider = await readSetting(t("settings.readLabels.translateProvider"), GetWorkshopTranslateProvider, config.workshopTranslateProvider || "microsoft");
  const customBaseURL = await readSetting(t("settings.readLabels.customBaseUrl"), GetWorkshopTranslateCustomBaseURL, config.workshopTranslateCustomBaseURL || "");
  const customModelId = await readSetting(t("settings.readLabels.customModel"), GetWorkshopTranslateCustomModelId, config.workshopTranslateCustomModelId || "");
  const hasCustomAPIKey = await readSetting(t("settings.readLabels.customApiKey"), HasWorkshopTranslateCustomAPIKey, Boolean(config.workshopTranslateCustomAPIKey));
  const isSelecting = enabled ? await readSetting(t("settings.readLabels.ipSelecting"), IsSelectingIP, false) : false;
  const ipOptions = [];
  const bestIPOption = enabled && !isSelecting ? await readSetting(t("settings.readLabels.bestIP"), GetCurrentBestIPOption, null) : null;
  const bestIP = getIPOptionIP(bestIPOption) || (enabled && !isSelecting
    ? await readSetting(t("settings.readLabels.bestIPAddress"), GetCurrentBestIP, "")
    : "");
  let addonListInfo = null;
  let workshopCollections = [];
  let workshopCollectionsError = "";
  try {
    workshopCollections = (await ListWorkshopCollections()) || [];
  } catch (error) {
    workshopCollectionsError = String(error?.message || error);
  }
  let addonListBackups = [];
  let addonListError = "";
  try {
    const addonListManagerState = parseAddonListManagerState(await GetAddonListManagerState());
    addonListInfo = addonListManagerState.info;
    addonListBackups = addonListManagerState.backups;
  } catch (error) {
    addonListError = String(error?.message || error || t("settings.page.readAddonListFailed"));
  }
  let autoexecInfo = null;
  let autoexecError = "";
  let autoexecHelp = [];
  let autoexecMatches = [];
  try {
    if (typeof GetAutoexecConfig === "function") {
      autoexecInfo = await GetAutoexecConfig();
      if (typeof GetAutoexecCommandHelp === "function") autoexecHelp = await GetAutoexecCommandHelp("");
      if (typeof AnalyzeAutoexecCommands === "function") autoexecMatches = await AnalyzeAutoexecCommands(autoexecInfo?.content || "");
    }
  } catch (error) {
    autoexecError = String(error?.message || error || t("settings.page.readAutoexecFailed"));
  }
  let modEnableProfiles = [];
  let modEnableProfilesError = "";
  try {
    if (typeof ListModEnableProfiles === "function") {
      modEnableProfiles = (await ListModEnableProfiles()) || [];
    }
  } catch (error) {
    modEnableProfilesError = String(error?.message || error || t("settings.page.readProfilesFailed"));
  }
  // 策略组的生命周期已经不在这里：Mod 管理页「分组 → 策略组管理…」打开独立窗口，
  // 由 features/mod-groups/strategy-group-manager.js 直接读写 groups.json。
  let modDependencies = [];
  let modDependenciesError = "";
  try {
    if (typeof ListModDependencies === "function") {
      modDependencies = (await ListModDependencies()) || [];
    }
  } catch (error) {
    modDependenciesError = String(error?.message || error || t("settings.page.readDependenciesFailed"));
  }
  const selectedModOptions = [...(appState.selectedFiles || [])].map((path) => {
    const file = (appState.allVpkFiles || []).find((item) => item.path === path);
    return { path, label: file?.title || file?.name || path };
  });
  if (addonListMergePreview && addonListInfo?.path && addonListMergePreview.targetPath?.toLowerCase() !== addonListInfo.path.toLowerCase()) {
    addonListMergePreview = null;
  }

  // Do not let a slow, superseded render overwrite a page opened more recently.
  if (!isCurrentRender()) return;

  let ipStatusText = "";
  if (enabled) {
    if (isSelecting) {
      ipStatusText = t("settings.network.ip.selecting");
    } else if (bestIP) {
      const optionLabel = formatIPOptionLabel(bestIPOption || { ip: bestIP, category: useFixedIP ? t("settings.network.ip.customCategory") : "" });
      ipStatusText = useFixedIP ? t("settings.network.ip.statusFixed", { label: optionLabel }) : t("settings.network.ip.statusAuto", { label: optionLabel });
    } else {
      ipStatusText = t("settings.network.ip.notReady");
    }
  }

  container.innerHTML = `
    ${settingsReadErrors.length ? `
      <div class="settings-read-warning" role="status">
        <strong>${t("settings.page.readWarningTitle")}</strong>
        <span>${escapeHtml(settingsReadErrors.join("；"))}</span>
      </div>
    ` : ""}
    <div class="settings-layout embedded-settings">
      <div class="settings-sidebar">
        <button class="settings-nav-item active" data-panel="network">${t("settings.nav.network")}</button>
        <button class="settings-nav-item" data-panel="interface">${t("settings.nav.interface")}</button>
        <button class="settings-nav-item" data-panel="workshop">${t("settings.nav.workshop")}</button>
        <button class="settings-nav-item" data-panel="addonlist">${t("settings.nav.addonlist")}</button>
      </div>
      <div class="settings-content">
        <div class="settings-panels-track">
        <div class="settings-panel active" id="settings-panel-network">
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.network.cardTitle")}</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.network.enable.label")}</div>
                <div class="setting-row-desc">${t("settings.network.enable.desc")}</div>
                ${
                  ipStatusText
                    ? `<div class="setting-row-status-line">
                        <div id="settings-ip-status" class="setting-row-status">${escapeHtml(ipStatusText)}</div>
                      </div>`
                    : ""
                }
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-preferred-ip" ${enabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div id="settings-ip-mode-section" class="setting-indent" style="${enabled ? "" : "display:none"}">
              <div class="setting-row-label">${t("settings.network.mode.label")}</div>
              <div class="setting-radio-group">
                <label class="setting-radio-label"><input type="radio" name="settings-ip-mode" value="auto" ${useFixedIP ? "" : "checked"}><span>${t("settings.network.mode.auto")}</span></label>
                <label class="setting-radio-label"><input type="radio" name="settings-ip-mode" value="fixed" ${useFixedIP ? "checked" : ""}><span>${t("settings.network.mode.fixed")}</span></label>
              </div>
              <div id="settings-fixed-ip-tools" class="settings-fixed-ip-tools" style="${useFixedIP ? "" : "display:none"}">
                <div class="single-select-dropdown settings-ip-option-dropdown">
                  <button type="button" id="settings-ip-option-trigger" class="select-trigger"></button>
                  <div id="settings-ip-option-menu" class="select-menu settings-ip-option-menu hidden"></div>
                </div>
                <input type="text" id="settings-fixed-ip" class="form-input" value="${escapeAttr(fixedIP)}" placeholder="${t("settings.network.ip.fixedPlaceholder")}">
              </div>
            </div>
          </div>
        </div>

        <div class="settings-panel" id="settings-panel-interface">
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.interface.display.cardTitle")}</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.displayMode.label")}</div>
                <div class="setting-row-desc">${t("settings.interface.displayMode.desc")}</div>
              </div>
              <div class="mode-toggle-group">
                <label class="mode-option ${appState.displayMode === "list" ? "active" : ""}">
                  <input type="radio" name="settings-display-mode" value="list" ${appState.displayMode === "list" ? "checked" : ""}>
                  <span class="mode-text">${t("settings.interface.displayMode.list")}</span>
                </label>
                <label class="mode-option ${appState.displayMode === "card" ? "active" : ""}">
                  <input type="radio" name="settings-display-mode" value="card" ${appState.displayMode === "card" ? "checked" : ""}>
                  <span class="mode-text">${t("settings.interface.displayMode.card")}</span>
                </label>
              </div>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.uiScale.label")}</div>
                <div class="setting-row-desc">${t("settings.interface.uiScale.desc")}</div>
              </div>
              <div class="ui-scale-control" aria-label="${t("settings.interface.uiScale.label")}">
                <button type="button" class="ui-scale-button" id="settings-ui-scale-decrease" aria-label="${t("settings.interface.uiScale.decrease")}">A−</button>
                <input type="range" id="settings-ui-scale" data-ui-scale-input min="80" max="140" step="5" value="${Math.round(normalizeUIScale(getConfig().uiScale) * 100)}" aria-label="${t("settings.interface.uiScale.percentLabel")}">
                <output class="ui-scale-value" data-ui-scale-value for="settings-ui-scale">${Math.round(normalizeUIScale(getConfig().uiScale) * 100)}%</output>
                <button type="button" class="ui-scale-button" id="settings-ui-scale-increase" aria-label="${t("settings.interface.uiScale.increase")}">A+</button>
              </div>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.boxSelection.label")}</div>
                <div class="setting-row-desc">${t("settings.interface.boxSelection.desc")}</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-box-selection" ${appState.boxSelectionEnabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.textSize.label")}</div>
                <div class="setting-row-desc">${t("settings.interface.textSize.desc")}</div>
              </div>
              <div class="mode-toggle-group" id="settings-text-size" role="radiogroup" aria-label="${t("settings.interface.textSize.label")}">
                ${TEXT_SIZE_OPTIONS.map((option) => `
                  <label class="mode-option ${normalizeTextSize(getConfig().textSize) === option.key ? "active" : ""}" title="${escapeHtml(option.description)}">
                    <input type="radio" name="settings-text-size" value="${option.key}" ${normalizeTextSize(getConfig().textSize) === option.key ? "checked" : ""}>
                    <span class="mode-text">${escapeHtml(option.label)}</span>
                  </label>`).join("")}
              </div>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.readingComfort.label")}</div>
                <div class="setting-row-desc">${t("settings.interface.readingComfort.desc", { current: escapeHtml(readingComfortLabel(getConfig().readingComfort)) })}</div>
              </div>
              <div class="mode-toggle-group" id="settings-reading-comfort" role="radiogroup" aria-label="${t("settings.interface.readingComfort.label")}">
                ${READING_COMFORT_OPTIONS.map((option) => `
                  <label class="mode-option ${normalizeReadingComfort(getConfig().readingComfort) === option.key ? "active" : ""}" title="${escapeHtml(option.description)}">
                    <input type="radio" name="settings-reading-comfort" value="${option.key}" ${normalizeReadingComfort(getConfig().readingComfort) === option.key ? "checked" : ""}>
                    <span class="mode-text">${escapeHtml(option.label)}</span>
                  </label>`).join("")}
              </div>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.ctrlClick.label")}</div>
                <div class="setting-row-desc">${t("settings.interface.ctrlClick.desc")}</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-ctrl-click-selection" ${appState.ctrlClickSelectionEnabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row setting-row-filter-layout">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.filterLayout.label")}</div>
                <div class="setting-row-desc">${t("settings.interface.filterLayout.desc")}</div>
              </div>
              <div class="mode-toggle-group filter-layout-toggle" role="radiogroup" aria-label="${t("settings.interface.filterLayout.label")}">
                <label class="mode-option filter-layout-option filter-layout-option-compact ${appState.filterLayoutMode !== "classic" ? "active" : ""}">
                  <input type="radio" name="settings-filter-layout" value="compact" ${appState.filterLayoutMode !== "classic" ? "checked" : ""}>
                  <span class="filter-layout-option-icon" aria-hidden="true">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="4" rx="1"></rect><rect x="3" y="10" width="18" height="4" rx="1"></rect><rect x="3" y="16" width="11" height="4" rx="1"></rect></svg>
                  </span>
                  <span class="filter-layout-option-copy"><strong>${t("settings.interface.filterLayout.compact.title")}</strong><small>${t("settings.interface.filterLayout.compact.hint")}</small></span>
                  <span class="filter-layout-option-badge">${t("settings.interface.filterLayout.compact.badge")}</span>
                </label>
                <label class="mode-option filter-layout-option filter-layout-option-classic ${appState.filterLayoutMode === "classic" ? "active" : ""}">
                  <input type="radio" name="settings-filter-layout" value="classic" ${appState.filterLayoutMode === "classic" ? "checked" : ""}>
                  <span class="filter-layout-option-icon" aria-hidden="true">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="3" width="7" height="7" rx="1"></rect><rect x="14" y="3" width="7" height="7" rx="1"></rect><rect x="3" y="14" width="7" height="7" rx="1"></rect><path d="M14 17.5h7"></path></svg>
                  </span>
                  <span class="filter-layout-option-copy"><strong>${t("settings.interface.filterLayout.classic.title")}</strong><small>${t("settings.interface.filterLayout.classic.hint")}</small></span>
                  <span class="filter-layout-option-badge">${t("settings.interface.filterLayout.classic.badge")}</span>
                </label>
              </div>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.interface.openWith.cardTitle")}</div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.openWith.label")}</div>
                <div class="setting-row-desc">
                  <strong>留空 = 系统默认</strong>（Windows 用资源管理器定位并选中文件，行为与以前完全一致）。<br>
                  想换成别的程序就填程序路径（例如 <code>C:\Tools\totalcmd\TOTALCMD64.EXE</code>）与参数模板，
                  只填命令名（如 <code>code</code>）时会交给 PATH 查找。<br>
                  参数模板占位符：<code>{path}</code> 完整路径、<code>{dir}</code> 所在目录、<code>{name}</code> 文件名
                  （也兼容 FireAxe 的 <code>{0}</code> = 完整路径）；模板里一个占位符都不写时，会自动把完整路径追加到最后。
                  参数里的空格用双引号包住，例如 <code>/O /T /L="{dir}"</code>。
                </div>
              </div>
            </div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.openWith.programLabel")}</div>
                <input type="text" id="settings-open-with-program" class="form-input"
                  placeholder="${t("settings.interface.openWith.programPlaceholder")}" value="${escapeAttr(getConfig().openWithProgram || "")}">
              </div>
            </div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.interface.openWith.argsLabel")}</div>
                <input type="text" id="settings-open-with-arguments" class="form-input"
                  placeholder='${t("settings.interface.openWith.argsPlaceholder")}' value="${escapeAttr(getConfig().openWithArguments || "")}">
              </div>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-desc" id="settings-open-with-status"></div>
              </div>
              <div class="setting-row-control">
                <button type="button" class="trigger-check-btn" id="settings-open-with-save">
                  <span class="trigger-check-text">${t("settings.interface.openWith.save")}</span>
                </button>
                <button type="button" class="btn btn-outline btn-small" id="settings-open-with-reset">${t("settings.interface.openWith.reset")}</button>
              </div>
            </div>
          </div>
        </div>

        <div class="settings-panel" id="settings-panel-workshop">
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.workshop.data.cardTitle")}</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.data.meta.label")}</div>
                <div class="setting-row-desc">${t("settings.workshop.data.meta.desc")}</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-meta-enabled" ${metaEnabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row ${metaEnabled ? "" : "setting-row-disabled"}" id="settings-update-check-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.data.updateCheck.label")}</div>
                <div class="setting-row-desc">${t("settings.workshop.data.updateCheck.desc")}</div>
              </div>
              <label class="toggle-switch ${metaEnabled ? "" : "toggle-switch-disabled"}">
                <input type="checkbox" id="settings-update-check-enabled" ${updateCheckEnabled ? "checked" : ""} ${metaEnabled ? "" : "disabled"}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div id="settings-update-check-section" class="setting-indent" style="${updateCheckEnabled ? "" : "display:none"}">
              <button class="trigger-check-btn" id="settings-manual-check-btn" ${metaEnabled && updateCheckEnabled ? "" : "disabled"}>
                <svg class="trigger-check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z"/><path d="M12 6v6l4 2"/></svg>
                <span class="trigger-check-text">${t("settings.workshop.data.updateCheck.trigger")}</span>
              </button>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.data.autoRedownload.label")}</div>
                <div class="setting-row-desc">${t("settings.workshop.data.autoRedownload.desc")}</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-auto-redownload" ${autoRedownloadEnabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.data.clipboard.label")}</div>
                <div class="setting-row-desc">${t("settings.workshop.data.clipboard.desc")}</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-auto-detect-workshop-link" ${autoDetectWorkshopLink ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.data.enrich.label")}</div>
                <div class="setting-row-desc">
                  向 Steam 官方接口补取工坊自带的分类标签（如 Survivors / Sounds / Single Player）
                  与订阅数、收藏数、浏览量，写进每个工坊 Mod 的同名 <code>.meta</code>。
                  抓过之后，「准备给智能体材料」的清单里会多出 <code>workshop.steamTags</code>，
                  分组建议的准确率会更高。只读远端数据，不改 Mod 文件与 addonlist。
                </div>
              </div>
              <div class="setting-row-control">
                <button type="button" id="settings-workshop-enrich" class="trigger-check-btn">
                  <span class="trigger-check-text">${t("settings.workshop.data.enrich.action")}</span>
                </button>
              </div>
            </div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.collections.label")}</div>
                <div class="setting-row-desc">${t("settings.workshop.collections.desc")}</div>
              </div>
              <div class="addonlist-action-row settings-profile-capture-row">
                <input
                  type="text"
                  id="settings-collection-input"
                  class="settings-profile-name-input"
                  placeholder="${t("settings.workshop.collections.inputPlaceholder")}"
                  autocomplete="off"
                >
                <button type="button" id="settings-collection-capture" class="trigger-check-btn addonlist-action-btn">${t("settings.workshop.collections.capture")}</button>
                <button type="button" id="settings-collection-check-all" class="trigger-check-btn addonlist-action-btn ${workshopCollections.length > 0 ? "" : "hidden"}">${t("settings.workshop.collections.checkAll")}</button>
              </div>
              <p id="settings-collection-status" class="setting-row-status">${escapeHtml(workshopCollectionStatusText || workshopCollectionsError)}</p>
              ${workshopCollections.length > 0 ? `
                <div class="settings-profile-list">
                  ${workshopCollections.map((link) => `
                    <div class="settings-profile-item">
                      <div class="settings-profile-main">
                        <strong>${escapeHtml(link.title || link.collectionId)}</strong>
                        <span>${t("settings.workshop.collections.itemMeta", { id: escapeHtml(link.collectionId), summary: escapeHtml(formatCollectionSummary(link)), lastChecked: link.lastCheckedAt ? t("settings.workshop.collections.lastChecked", { time: escapeHtml(link.lastCheckedAt) }) : "" })}</span>
                      </div>
                      <div class="settings-profile-actions">
                        <button type="button" class="settings-collection-download" data-collection-id="${escapeAttr(link.id)}">${t("settings.workshop.collections.downloadMissing")}</button>
                        <button type="button" class="settings-collection-refresh" data-collection-id="${escapeAttr(link.id)}">${t("settings.workshop.collections.refresh")}</button>
                        <button type="button" class="settings-collection-delete" data-collection-id="${escapeAttr(link.id)}">${t("settings.workshop.collections.delete")}</button>
                      </div>
                    </div>
                  `).join("")}
                </div>
              ` : `<div class="setting-row-desc">${t("settings.workshop.collections.empty")}</div>`}
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.workshop.browser.cardTitle")}</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.browser.label")}</div>
                <div class="setting-row-desc">${t("settings.workshop.browser.desc")}</div>
              </div>
              <div class="setting-radio-group">
                <label class="setting-radio-label"><input type="radio" name="settings-browser-target" value="mirror" ${browserTarget === "mirror" ? "checked" : ""}><span>${t("settings.workshop.browser.mirror")}</span></label>
                <label class="setting-radio-label"><input type="radio" name="settings-browser-target" value="steam" ${browserTarget === "steam" ? "checked" : ""}><span>${t("settings.workshop.browser.steam")}</span></label>
              </div>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.workshop.translate.cardTitle")}</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.workshop.translate.provider.label")}</div>
                <div class="setting-row-desc">${t("settings.workshop.translate.provider.desc")}</div>
              </div>
              <div class="mode-toggle-group settings-translate-provider-toggle">
                <label class="mode-option ${translateProvider === "microsoft" ? "active" : ""}">
                  <input type="radio" name="settings-translate-provider" value="microsoft" ${translateProvider === "microsoft" ? "checked" : ""}>
                  <span class="mode-text">${t("settings.workshop.translate.provider.microsoft")}</span>
                </label>
                <label class="mode-option ${translateProvider === "yandex" ? "active" : ""}">
                  <input type="radio" name="settings-translate-provider" value="yandex" ${translateProvider === "yandex" ? "checked" : ""}>
                  <span class="mode-text">Yandex</span>
                </label>
                <label class="mode-option ${translateProvider === "custom" ? "active" : ""}">
                  <input type="radio" name="settings-translate-provider" value="custom" ${translateProvider === "custom" ? "checked" : ""}>
                  <span class="mode-text">${t("settings.workshop.translate.provider.custom")}</span>
                </label>
              </div>
            </div>
            <div id="settings-translate-custom-section" class="setting-indent" style="${translateProvider === "custom" ? "" : "display:none"}">
              <div class="setting-row-label" style="margin-bottom:8px">${t("settings.workshop.translate.custom.title")}</div>
              <div class="setting-custom-input-group" style="display:flex;flex-direction:column;gap:8px">
                <input type="text" id="settings-translate-custom-baseurl" class="form-input" value="${escapeAttr(customBaseURL)}" placeholder="${t("settings.workshop.translate.custom.baseUrlPlaceholder")}">
                <input type="password" id="settings-translate-custom-apikey" class="form-input" value="" placeholder="API Key${hasCustomAPIKey ? t("settings.workshop.translate.custom.apiKeySetSuffix") : ""}">
                <input type="text" id="settings-translate-custom-modelid" class="form-input" value="${escapeAttr(customModelId)}" placeholder="${t("settings.workshop.translate.custom.modelPlaceholder")}">
              </div>
            </div>
          </div>
        </div>

        <div class="settings-panel" id="settings-panel-addonlist">
          <div class="setting-card autoexec-card">
            <div class="setting-card-title">${t("settings.addonlist.autoexec.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.autoexec.desc")}</div>
            <div class="autoexec-layout">
              <div class="autoexec-editor-column">
                <div class="autoexec-meta" id="settings-autoexec-meta">
                  <span class="autoexec-path">${escapeHtml(autoexecInfo?.path || t("settings.addonlist.dirNotChosen"))}</span>
                  <span>${autoexecInfo?.exists ? t("settings.addonlist.fileExists") : t("settings.addonlist.fileMissing")}</span>
                  <span>${escapeHtml(autoexecInfo?.encoding || "UTF-8")} · ${escapeHtml(autoexecInfo?.lineEnding || "CRLF")}</span>
                  <span>${formatAddonListBytes(autoexecInfo?.size)}</span>
                  ${autoexecInfo?.lastModified ? `<span>${t("settings.addonlist.modifiedAt", { time: escapeHtml(formatAddonListTime(autoexecInfo.lastModified)) })}</span>` : ""}
                </div>
                ${autoexecError ? `<div class="addonlist-status-error">${escapeHtml(autoexecError)}</div>` : ""}
                <div class="autoexec-editor-shell">
                  <pre class="autoexec-line-numbers" id="settings-autoexec-line-numbers" aria-hidden="true"></pre>
                  <textarea id="settings-autoexec-content" class="autoexec-editor" spellcheck="false" wrap="off" aria-label="${t("settings.addonlist.autoexec.contentLabel")}">${escapeHtml(autoexecInfo?.content || "")}</textarea>
                </div>
                <div class="addonlist-action-row autoexec-action-row">
                  <button type="button" id="settings-autoexec-save" class="trigger-check-btn addonlist-action-btn" ${autoexecInfo ? "" : "disabled"}>${t("settings.addonlist.autoexec.save")}</button>
                  <button type="button" id="settings-autoexec-reload" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.autoexec.reload")}</button>
                  <button type="button" id="settings-autoexec-open" class="trigger-check-btn addonlist-action-btn" ${autoexecInfo?.exists ? "" : "disabled"}>${t("settings.addonlist.openLocation")}</button>
                </div>
                <div class="autoexec-analysis" id="settings-autoexec-analysis">${t("settings.addonlist.autoexec.analysisSummary", { known: autoexecMatches.filter((item) => item.known).length, unknown: autoexecMatches.filter((item) => !item.known).length, risks: "" })}</div>
                <div class="autoexec-matches" id="settings-autoexec-matches"></div>
              </div>
              <aside class="autoexec-help-column">
                <div class="autoexec-help-title">${t("settings.addonlist.autoexec.help.title")}</div>
                <input type="search" id="settings-autoexec-help-search" class="form-input" placeholder="${t("settings.addonlist.autoexec.help.searchPlaceholder")}">
                <div id="settings-autoexec-help-list" class="autoexec-help-list"></div>
              </aside>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.lifecycle.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.lifecycle.desc")}</div>
            <div class="addonlist-lifecycle-steps" aria-label="${t("settings.addonlist.lifecycle.ariaLabel")}">
              <div class="addonlist-lifecycle-step"><strong>${t("settings.addonlist.lifecycle.step1.title")}</strong><span>${t("settings.addonlist.lifecycle.step1.desc")}</span></div>
              <div class="addonlist-lifecycle-step"><strong>${t("settings.addonlist.lifecycle.step2.title")}</strong><span>${t("settings.addonlist.lifecycle.step2.desc")}</span></div>
              <div class="addonlist-lifecycle-step"><strong>${t("settings.addonlist.lifecycle.step3.title")}</strong><span>${t("settings.addonlist.lifecycle.step3.desc")}</span></div>
              <div class="addonlist-lifecycle-step"><strong>${t("settings.addonlist.lifecycle.step4.title")}</strong><span>${t("settings.addonlist.lifecycle.step4.desc")}</span></div>
            </div>
            <div class="setting-row addonlist-status-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.targetFile")}</div>
                <div class="setting-row-desc addonlist-path">${escapeHtml(addonListInfo?.path || t("settings.addonlist.dirNotChosen"))}</div>
                ${addonListError ? `<div class="addonlist-status-error">${escapeHtml(addonListError)}</div>` : ""}
              </div>
            </div>
            <div class="setting-row addonlist-unrecorded-placement-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.unrecordedPlacement.label")}</div>
                <div class="setting-row-desc">${t("settings.addonlist.unrecordedPlacement.desc")}</div>
              </div>
              <select id="settings-unrecorded-mod-load-order-placement" class="form-input addonlist-unrecorded-placement-select" aria-label="${t("settings.addonlist.unrecordedPlacement.ariaLabel")}">
                <option value="end" ${unrecordedModLoadOrderPlacement === "end" ? "selected" : ""}>${t("settings.addonlist.unrecordedPlacement.optionEnd")}</option>
                <option value="start" ${unrecordedModLoadOrderPlacement === "start" ? "selected" : ""}>${t("settings.addonlist.unrecordedPlacement.optionStart")}</option>
                <option value="after-enabled" ${unrecordedModLoadOrderPlacement === "after-enabled" ? "selected" : ""}>${t("settings.addonlist.unrecordedPlacement.optionAfterEnabled")}</option>
              </select>
            </div>
            ${addonListInfo ? `
              <div class="addonlist-status-grid">
                <div><span>${t("settings.addonlist.status.fileState")}</span><strong>${addonListInfo.exists ? t("settings.addonlist.state.exists") : t("settings.addonlist.state.missing")}</strong></div>
                <div><span>${t("settings.addonlist.status.encodingSize")}</span><strong>${escapeHtml(addonListInfo.encoding || "—")} / ${formatAddonListBytes(addonListInfo.size)}</strong></div>
                <div><span>${t("settings.addonlist.status.lastModified")}</span><strong>${escapeHtml(formatAddonListTime(addonListInfo.lastModified))}</strong></div>
                <div><span>${t("settings.addonlist.status.snapshot")}</span><strong>${addonListInfo.managedSnapshotExists ? t("settings.addonlist.status.snapshotSaved") : t("settings.addonlist.status.snapshotFailed")}</strong></div>
              </div>
              <div class="addonlist-action-row">
                <button type="button" id="settings-addonlist-save-snapshot" class="trigger-check-btn addonlist-action-btn" ${addonListInfo.exists ? "" : "disabled"}>${t("settings.addonlist.saveSnapshot")}</button>
                <button type="button" id="settings-addonlist-create-backup" class="trigger-check-btn addonlist-action-btn" ${addonListInfo.exists ? "" : "disabled"}>${t("settings.addonlist.createBackup")}</button>
                <button type="button" id="settings-addonlist-open" class="trigger-check-btn addonlist-action-btn" ${addonListInfo.exists ? "" : "disabled"}>${t("settings.addonlist.openLocation")}</button>
              </div>
              <div class="setting-row addonlist-guard-row">
                <div class="setting-row-info">
                  <div class="setting-row-label">${t("settings.addonlist.guard.label")}</div>
                  <div class="setting-row-desc">${t("settings.addonlist.guard.desc")}</div>
                  ${addonListInfo.lastGuardRestore ? `<div class="setting-row-status">${t("settings.addonlist.guard.lastRestore", { time: escapeHtml(formatAddonListTime(addonListInfo.lastGuardRestore)) })}</div>` : ""}
                  ${addonListInfo.lastGuardError ? `<div class="addonlist-status-error">${t("settings.addonlist.guard.status", { status: escapeHtml(addonListInfo.lastGuardError) })}</div>` : ""}
                </div>
                <label class="toggle-switch">
                  <input type="checkbox" id="settings-addonlist-guard" ${addonListInfo.guardEnabled ? "checked" : ""} ${addonListInfo.exists || addonListInfo.managedSnapshotExists ? "" : "disabled"}>
                  <span class="toggle-slider"></span>
                </label>
              </div>
              <div class="addonlist-danger-zone">
                <button type="button" id="settings-addonlist-delete" class="trigger-check-btn addonlist-danger-btn" ${addonListInfo.exists ? "" : "disabled"}>${t("settings.addonlist.delete.button")}</button>
                <span>${t("settings.addonlist.delete.hint")}</span>
              </div>
            ` : ""}
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.backups.cardTitle")}</div>
            ${addonListInfo ? (addonListBackups.length > 0 ? `
              <div class="addonlist-backup-list">
                ${addonListBackups.map((backup) => `
                  <div class="addonlist-backup-item">
                    <div class="addonlist-backup-main">
                      <strong>${escapeHtml(formatAddonListBackupKind(backup.kind))}</strong>
                      <span>${escapeHtml(formatAddonListTime(backup.createdAt))} · ${formatAddonListBytes(backup.size)}</span>
                    </div>
                    <div class="addonlist-backup-actions">
                      <button type="button" class="addonlist-backup-restore" data-addonlist-backup="${escapeAttr(backup.name)}">${t("settings.addonlist.backups.restore")}</button>
                      <button type="button" class="addonlist-backup-delete" data-addonlist-backup="${escapeAttr(backup.name)}">${t("settings.addonlist.backups.delete")}</button>
                    </div>
                  </div>
                `).join("")}
              </div>
            ` : `<div class="setting-row-desc">${t("settings.addonlist.backups.empty")}</div>`) : ""}
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.merge.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.merge.desc")}</div>
            <div class="addonlist-action-row addonlist-merge-action-row">
              <button type="button" id="settings-addonlist-select-merge-source" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.merge.select")}</button>
              ${addonListMergePreview ? `<button type="button" id="settings-addonlist-cancel-merge" class="trigger-check-btn addonlist-secondary-btn">${t("settings.addonlist.merge.cancel")}</button>` : ""}
            </div>
            ${renderAddonListMergePreview(addonListMergePreview)}
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.conflict.cardTitle")}</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.conflict.priorityAware.label")}</div>
                <div class="setting-row-desc">${t("settings.addonlist.conflict.priorityAware.desc")}</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-conflict-priority-aware" ${getConfig().conflictPriorityAware === true ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.conflict.ignoreList.label")}</div>
                <div class="setting-row-desc">${escapeHtml(CONFLICT_IGNORE_LIST_DESCRIPTION)}${t("settings.addonlist.conflict.ignoreList.descSuffix")}</div>
              </div>
              <textarea id="settings-conflict-ignore-files" class="settings-conflict-ignore-editor" rows="6" spellcheck="false" placeholder="materials/shared.vtf&#10;scripts/vscripts/">${escapeHtml(formatConflictIgnoreList(getConfig().conflictIgnoreFiles))}</textarea>
              <div class="addonlist-action-row">
                <button type="button" id="settings-conflict-save-ignore" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.conflict.ignoreList.save")}</button>
                <span id="settings-conflict-ignore-status" class="setting-row-status"></span>
              </div>
            </div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.stockWhitelist.label")}</div>
                <div class="setting-row-desc">
                  原版文件白名单用于降低“两个 Mod 各带一份原版文件”造成的误报，只影响忽略判定，不会改写任何文件。
                  默认启用内置的引擎胶水批次；可按类别从本机原版 <code>pak01_dir.vpk</code> 生成增量批次，
                  生成结果保存在配置目录的 <code>stock-files</code> 下，删除文件即可停用该类别。
                </div>
              </div>
              <div class="addonlist-action-row">
                <button type="button" id="settings-stock-whitelist-generate" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.stockWhitelist.generate")}</button>
                <button type="button" id="settings-stock-whitelist-reload" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.stockWhitelist.reload")}</button>
                <span id="settings-stock-whitelist-status" class="setting-row-status"></span>
              </div>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.profiles.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.profiles.desc")}</div>
            <div class="addonlist-action-row settings-profile-capture-row">
              <input type="text" id="settings-profile-name" class="settings-profile-name-input" maxlength="60" placeholder="${t("settings.addonlist.profiles.namePlaceholder")}">
              <button type="button" id="settings-profile-capture" class="trigger-check-btn addonlist-action-btn" ${addonListInfo?.exists ? "" : "disabled"}>${t("settings.addonlist.profiles.capture")}</button>
              <button type="button" id="settings-profile-import" class="trigger-check-btn addonlist-secondary-btn">${t("settings.addonlist.profiles.import")}</button>
              <label class="settings-health-deep-toggle" title="${t("settings.addonlist.profiles.includeAutomation.title")}">
                <input type="checkbox" id="settings-profile-include-automation">
                <span>${t("settings.addonlist.profiles.includeAutomation.label")}</span>
              </label>
            </div>
            <p id="settings-profile-status" class="setting-row-status">${escapeHtml(modEnableProfilesError)}</p>
            ${modEnableProfiles.length > 0 ? `
              <div class="settings-profile-list">
                ${modEnableProfiles.map((profile) => `
                  <div class="settings-profile-item">
                    <div class="settings-profile-main">
                      <strong>${escapeHtml(profile.name)}</strong>
                      <span>${t("settings.addonlist.profiles.entryCount", { count: (profile.entries || []).length })}${profile.description ? ` · ${escapeHtml(profile.description)}` : ""}</span>
                    </div>
                    <div class="settings-profile-actions">
                      <button type="button" class="settings-profile-apply" data-profile-id="${escapeAttr(profile.id)}">${t("settings.addonlist.profiles.apply")}</button>
                      <button type="button" class="settings-profile-export" data-profile-id="${escapeAttr(profile.id)}">${t("settings.addonlist.profiles.export")}</button>
                      <button type="button" class="settings-profile-delete" data-profile-id="${escapeAttr(profile.id)}">${t("settings.addonlist.profiles.delete")}</button>
                    </div>
                  </div>
                `).join("")}
              </div>
            ` : `<div class="setting-row-desc">${t("settings.addonlist.profiles.empty")}</div>`}
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.strategyGroups.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.strategyGroups.desc")}</div>
            <div class="addonlist-action-row settings-profile-capture-row">
              <button type="button" id="settings-open-strategy-manager" class="btn btn-small btn-outline">${t("settings.addonlist.strategyGroups.open")}</button>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.dependencies.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.dependencies.desc")}</div>
            <div class="addonlist-action-row settings-profile-capture-row">
              <select id="settings-dependency-target" class="settings-strategy-select" aria-label="${t("settings.addonlist.dependencies.targetAria")}" ${selectedModOptions.length > 0 ? "" : "disabled"}>
                ${selectedModOptions.length > 0
                  ? selectedModOptions.map((item) => `<option value="${escapeAttr(item.path)}">${escapeHtml(item.label)}</option>`).join("")
                  : `<option value="">${t("settings.addonlist.dependencies.noSelection")}</option>`}
              </select>
              <button type="button" id="settings-dependency-save" class="trigger-check-btn addonlist-action-btn" ${selectedModOptions.length > 1 ? "" : "disabled"}>${t("settings.addonlist.dependencies.save", { count: Math.max(selectedModOptions.length - 1, 0) })}</button>
            </div>
            <p id="settings-dependency-status" class="setting-row-status">${escapeHtml(modDependenciesError)}</p>
            ${modDependencies.length > 0 ? `
              <div class="settings-profile-list">
                ${modDependencies.map((record) => `
                  <div class="settings-profile-item">
                    <div class="settings-profile-main">
                      <strong>${escapeHtml(record.name)}</strong>
                      <span>${t("settings.addonlist.dependencies.itemPrefix", { list: escapeHtml((record.dependencies || []).map((item) => item.name).join("、") || t("settings.addonlist.dependencies.none")) })}</span>
                    </div>
                    <div class="settings-profile-actions">
                      <button type="button" class="settings-dependency-enable" data-dependency-key="${escapeAttr(record.key)}" data-dependency-name="${escapeAttr(record.name)}">${t("settings.addonlist.dependencies.enable")}</button>
                      <button type="button" class="settings-dependency-delete" data-dependency-key="${escapeAttr(record.key)}">${t("settings.addonlist.dependencies.delete")}</button>
                    </div>
                  </div>
                `).join("")}
              </div>
            ` : `<div class="setting-row-desc">${t("settings.addonlist.dependencies.empty")}</div>`}
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.health.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.health.desc")}</div>
            <div class="addonlist-action-row settings-profile-capture-row">
              <label class="settings-health-deep-toggle">
                <input type="checkbox" id="settings-health-deep">
                <span>${t("settings.addonlist.health.deepScan")}</span>
              </label>
              <button type="button" id="settings-health-run" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.health.run")}</button>
              <button type="button" id="settings-health-fix-duplicates" class="trigger-check-btn addonlist-secondary-btn" disabled>${t("settings.addonlist.health.fixDuplicates")}</button>
              <button type="button" id="settings-health-fix-missing" class="trigger-check-btn addonlist-secondary-btn" disabled>${t("settings.addonlist.health.fixMissing")}</button>
              <button type="button" id="settings-health-fix-dependencies" class="trigger-check-btn addonlist-secondary-btn" disabled>${t("settings.addonlist.health.fixDependencies")}</button>
              <button type="button" id="settings-health-copy" class="trigger-check-btn addonlist-secondary-btn" disabled>${t("settings.addonlist.health.copyReport")}</button>
              <button type="button" id="settings-health-save" class="trigger-check-btn addonlist-secondary-btn" disabled>${t("settings.addonlist.health.saveReport")}</button>
            </div>
            <p id="settings-health-summary" class="setting-row-status"></p>
            <!-- 体检结果检索：与其它列表同一套语法（对象名 / 路径 / 提示文案 + tag: 严重度与类型） -->
            <div class="settings-health-search-row">
              <input
                type="search"
                id="settings-health-search"
                class="settings-health-search"
                placeholder="${t("settings.addonlist.health.searchPlaceholder")}"
                aria-label="${t("settings.addonlist.health.searchAria")}"
                autocomplete="off"
              >
              <button
                type="button"
                id="settings-health-search-help-btn"
                class="search-help-btn"
                title="${t("settings.addonlist.health.searchHelpTitle")}"
                aria-expanded="false"
                aria-controls="settings-health-search-help-popover"
              >?</button>
              <div
                id="settings-health-search-help-popover"
                class="search-help-popover hidden"
                role="dialog"
                aria-label="${t("settings.addonlist.health.searchHelpAria")}"
              ></div>
              <span id="settings-health-search-count" class="settings-health-search-count" aria-live="polite"></span>
            </div>
            <div id="settings-health-issues" class="settings-health-issues"></div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.maintenance.cardTitle")}</div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.maintenance.suite.label")}</div>
                <div class="setting-row-desc">
                  同一个作者命名空间（套件）里的标签继承结果会记在本机的 <code>suite_inheritance.json</code>：
                  只增不减，所以你删掉几个同套件的 Mod 也不会突然掉标签。
                  重置会清空这份记录，下一次扫描按当前 Mod 重新推导；重置只会少继承、不会少标，
                  Mod 文件与 addonlist.txt 都不受影响。
                </div>
              </div>
            <div class="addonlist-action-row settings-profile-capture-row">
              <button type="button" id="settings-reset-suite-inheritance" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.maintenance.suite.reset")}</button>
              <span id="settings-suite-inheritance-status" class="setting-row-status"></span>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.maintenance.customTags.cardTitle")}</div>
            <div class="setting-row setting-row-stacked">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.maintenance.customTags.label")}</div>
                <div class="setting-row-desc">
                  按<strong>文件名 / 标题 / 已识别标签</strong>再补一层自定义标签，用于你自己的命名习惯（例如"三角洲""写实包"）。
                  口径与内置规则一致：<strong>只增不减</strong>，不改动也不删除自动识别结果，也不参与冲突/优先级判定。
                  保存后会重新扫描一次全库。
                  <br>示例：
                  <code>{"version":1,"rules":[{"tag":"三角洲","nameContains":["三角洲","delta"]},{"tag":"贴图包","anyTags":["贴图"]}]}</code>
                </div>
              </div>
              <textarea id="settings-custom-tag-rules" class="settings-conflict-ignore-editor" rows="6" spellcheck="false" placeholder='${t("settings.addonlist.maintenance.customTags.placeholder")}'>${escapeHtml(customTagRulesText)}</textarea>
              <div class="addonlist-action-row">
                <button type="button" id="settings-custom-tag-rules-preview" class="trigger-check-btn addonlist-secondary-btn">${t("settings.addonlist.maintenance.customTags.preview")}</button>
                <button type="button" id="settings-custom-tag-rules-save" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.maintenance.customTags.save")}</button>
                <span id="settings-custom-tag-rules-status" class="setting-row-status"></span>
              </div>
              <div id="settings-custom-tag-rules-preview-list" class="settings-health-issues"></div>
            </div>
          </div>
        </div>
          <div class="setting-card">
            <div class="setting-card-title">${t("settings.addonlist.dns.cardTitle")}</div>
            <div class="setting-row-desc">${t("settings.addonlist.dns.desc")}</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">${t("settings.addonlist.dns.mode.label")}</div>
                <div class="setting-row-desc">${t("settings.addonlist.dns.mode.desc")}</div>
              </div>
              <div class="setting-radio-group">
                <label class="setting-radio-label"><input type="radio" name="settings-workshop-dns-mode" value="system" ${workshopDNSMode === "custom" ? "" : "checked"}><span>${t("settings.addonlist.dns.mode.system")}</span></label>
                <label class="setting-radio-label"><input type="radio" name="settings-workshop-dns-mode" value="custom" ${workshopDNSMode === "custom" ? "checked" : ""}><span>${t("settings.addonlist.dns.mode.custom")}</span></label>
              </div>
            </div>
            <div id="settings-workshop-dns-section" class="setting-indent" style="${workshopDNSMode === "custom" ? "" : "display:none"}">
              <div class="setting-row-label">${t("settings.addonlist.dns.address.label")}</div>
              <input type="text" id="settings-workshop-dns-address" class="form-input" value="${escapeAttr(workshopDNSAddress)}" placeholder="${t("settings.addonlist.dns.address.placeholder")}" autocomplete="off" spellcheck="false">
            </div>
            <div class="addonlist-action-row">
              <button type="button" id="settings-workshop-dns-save" class="trigger-check-btn addonlist-action-btn">${t("settings.addonlist.dns.save")}</button>
              <span id="settings-workshop-dns-status" class="setting-row-status">${escapeHtml(workshopDNSStatus)}</span>
            </div>
          </div>
        </div>

        </div>
      </div>
    </div>
  `;

  // The DOM can be replaced by a newer render between template construction
  // and binding (for example after a quick settings refresh).
  if (!isCurrentRender()) return;

  bindSettingsPage({
    // 整体透传 deps：早先这里是手写的绑定子集，任何新增绑定都必须两处同步，
    // 漏一处就会在运行时变成 "deps.X is not a function"（设置页控件静默失效）。
    // 下面的同名项是本次渲染计算出来的值，覆盖 deps 里的原始字段。
    ...deps,
    enabled,
    fixedIP,
    ipOptions,
    metaEnabled,
    updateCheckEnabled,
    browserTarget,
    translateProvider,
    autoexecInfo,
    autoexecHelp,
    modEnableProfiles,
    modDependencies,
    refreshAddonListPanel: () => renderSettingsPage(deps),
  });
}

const HEALTH_ISSUE_DISPLAY_LIMIT = 100;

// 行内「修复」按钮的动作回调：由 bindModHealthCheckSettings 注入（那里才有 deps 里的后端绑定）。
let healthIssueAutoFix = null;

// 体检结果的检索词与最近一次检索（切换页签后保留，和列表其它面板一致）。
let healthSearchQuery = "";
let healthSearchResult = null;

function renderModHealthIssues(container, report) {
  if (!container) return;
  container.replaceChildren();

  // 检索在"行构建"之前：先按统一语法过滤问题，再应用 100 条显示上限，
  // 这样"搜出来的条目"不会被上限挡掉（上限只针对显示量）。
  const issues = Array.isArray(report?.issues) ? report.issues : [];
  healthSearchResult = searchHealthIssues(issues, healthSearchQuery);
  const filteredReport = { ...(report || {}), issues: healthSearchResult.items };
  const { rows, hiddenCount } = buildHealthIssueRows(filteredReport, HEALTH_ISSUE_DISPLAY_LIMIT);

  // 计数单独刷新（搜索框在容器外面，输入时不重建 DOM，也就不会丢焦点）。
  const countEl = document.getElementById("settings-health-search-count");
  if (countEl) {
    const base = describeHealthSearch({
      total: healthSearchResult.total,
      matched: healthSearchResult.matched,
      query: healthSearchQuery,
      regexInvalid: healthSearchResult.regexInvalid,
    });
    countEl.textContent = [base, hiddenCount > 0 ? t("settings.addonlist.health.hiddenMore", { count: hiddenCount }) : ""].filter(Boolean).join(" · ");
  }

  if (rows.length === 0) {
    const empty = document.createElement("div");
    empty.className = "setting-row-desc";
    const invalidRegex = healthSearchResult?.regexInvalid || "";
    empty.textContent = !report
      ? t("settings.addonlist.health.emptyHint")
      : invalidRegex
        ? t("settings.addonlist.health.regexInvalid", { error: invalidRegex })
        : healthSearchQuery.trim()
          ? t("settings.addonlist.health.noMatch", { query: healthSearchQuery.trim(), total: issues.length })
          : t("settings.addonlist.health.clean");
    container.appendChild(empty);
    return;
  }
  // 白名单统计（对齐 FireAxe 的 CanAutomaticallyFix 分级）：
  // 只有重复条目 / 缺失条目 / 依赖未开启能自动修。
  const autoFix = summarizeHealthAutoFix(report);
  if (autoFix.total > 0) {
    const bar = document.createElement("div");
    bar.className = "settings-health-autofix";
    bar.textContent = t("settings.addonlist.health.autofixBar", {
      total: autoFix.total,
      actions: autoFix.actions
        .map((item) => t("settings.addonlist.health.autofixEntry", { label: item.shortLabel, count: item.count }))
        .join("、"),
    });
    container.appendChild(bar);
  }

  rows.forEach((item) => {
    const row = document.createElement("div");
    row.className = "settings-health-issue";

    const badge = document.createElement("span");
    badge.className = `severity-badge ${item.severity}`;
    badge.textContent = item.severityLabel;

    const main = document.createElement("div");
    main.className = "settings-health-issue-main";
    const title = document.createElement("div");
    title.className = "settings-health-issue-title";
    const highlightSpec = healthHighlightSpec(healthSearchResult);
    if (highlightSpec) {
      // highlightMatches 自己负责转义，与其它列表同一套高亮。
      title.innerHTML = highlightMatches(item.title, highlightSpec);
    } else {
      title.textContent = item.title;
    }
    const message = document.createElement("div");
    message.className = "settings-health-issue-message";
    if (highlightSpec) {
      message.innerHTML = highlightMatches(item.message, highlightSpec);
    } else {
      message.textContent = item.message;
    }
    main.append(title, message);

    row.append(badge, main);
    // 逐条问题的"自动修复"（FireAxe `Problem.TryAutomaticallyFix` 的落点）：
    // 只有白名单里的两类才给按钮，其余一律只展示建议。
    if (item.autoFix && typeof healthIssueAutoFix === "function") {
      const fix = document.createElement("button");
      fix.type = "button";
      fix.className = "btn btn-small btn-outline settings-health-issue-fix";
      fix.dataset.fixAction = item.autoFix.id;
      fix.textContent = t("settings.addonlist.health.fixButton");
      fix.title = item.autoFix.title;
      fix.addEventListener("click", () => void healthIssueAutoFix(item.autoFix.id, item.fixTarget));
      row.appendChild(fix);
    }
    container.appendChild(row);
  });

  if (hiddenCount > 0) {
    const more = document.createElement("div");
    more.className = "setting-row-desc";
    more.textContent = t("settings.addonlist.health.displayLimit", { limit: HEALTH_ISSUE_DISPLAY_LIMIT, hidden: hiddenCount });
    container.appendChild(more);
  }
}

function bindModDependencySettings(deps) {
  const status = document.getElementById("settings-dependency-status");
  const targetSelect = document.getElementById("settings-dependency-target");
  const saveButton = document.getElementById("settings-dependency-save");

  const setStatus = (message) => {
    if (status) status.textContent = message;
  };
  const refresh = () => {
    if (typeof deps.refreshAddonListPanel === "function") {
      deps.refreshAddonListPanel();
    }
  };

  saveButton?.addEventListener("click", async () => {
    const { target, dependencies } = buildDependencyArgs(
      [...(deps.appState.selectedFiles || [])],
      targetSelect?.value || "",
    );
    if (!target || dependencies.length === 0) {
      setStatus(t("settings.addonlist.dependencies.pickFirst"));
      return;
    }
    saveButton.disabled = true;
    try {
      await deps.SetModDependencies(target, dependencies);
      deps.showNotification(t("settings.addonlist.dependencies.saved", { count: dependencies.length }), "success");
      refresh();
    } catch (error) {
      setStatus(t("settings.addonlist.dependencies.saveFailed", { error: String(error?.message || error) }));
    } finally {
      saveButton.disabled = false;
    }
  });

  document.querySelectorAll("#settings-page-content .settings-dependency-enable").forEach((button) => {
    button.addEventListener("click", async () => {
      const key = button.dataset.dependencyKey;
      const name = button.dataset.dependencyName || key;
      if (!key) return;
      button.disabled = true;
      try {
        const result = await deps.EnableModDependencies(key);
        deps.showNotification(
          formatDependencyEnableSummary(name, result),
          result?.missing?.length ? "warning" : "success",
        );
        deps.refreshFilesKeepFilter?.();
        refresh();
      } catch (error) {
        setStatus(t("settings.addonlist.dependencies.enableFailed", { error: String(error?.message || error) }));
        button.disabled = false;
      }
    });
  });

  document.querySelectorAll("#settings-page-content .settings-dependency-delete").forEach((button) => {
    button.addEventListener("click", async () => {
      const key = button.dataset.dependencyKey;
      if (!key) return;
      if (
        !(await confirmInApp(t("settings.addonlist.dependencies.deleteConfirm"), {
          title: t("settings.addonlist.dependencies.deleteConfirmTitle"),
        }))
      ) {
        return;
      }
      button.disabled = true;
      try {
        await deps.DeleteModDependencies(key);
        deps.showNotification(t("settings.addonlist.dependencies.deleted"), "success");
        refresh();
      } catch (error) {
        setStatus(t("settings.addonlist.dependencies.deleteFailed", { error: String(error?.message || error) }));
        button.disabled = false;
      }
    });
  });
}

function bindModHealthCheckSettings(deps) {
  const runButton = document.getElementById("settings-health-run");
  const deepInput = document.getElementById("settings-health-deep");
  const summary = document.getElementById("settings-health-summary");
  const list = document.getElementById("settings-health-issues");
  const fixButton = document.getElementById("settings-health-fix-duplicates");
  const fixMissingButton = document.getElementById("settings-health-fix-missing");
  const fixDependenciesButton = document.getElementById("settings-health-fix-dependencies");
  const copyReportButton = document.getElementById("settings-health-copy");
  const saveReportButton = document.getElementById("settings-health-save");
  let currentReport = null;

  renderModHealthIssues(list, null);

  // 体检结果检索：绑定一次即可（搜索框在列表容器外，不会被列表重画带走）。
  const searchInput = document.getElementById("settings-health-search");
  if (searchInput) {
    searchInput.title = buildSearchHelpTitle(HEALTH_SEARCH_HELP_VARIANT);
    const popover = document.getElementById("settings-health-search-help-popover");
    if (popover && popover.dataset.filled !== "1") {
      popover.innerHTML = buildSearchHelpHtml(HEALTH_SEARCH_HELP_VARIANT);
      popover.dataset.filled = "1";
    }
    const helpBtn = document.getElementById("settings-health-search-help-btn");
    const setHelpOpen = (open) => {
      popover?.classList.toggle("hidden", !open);
      helpBtn?.setAttribute("aria-expanded", String(open));
      helpBtn?.classList.toggle("is-active", open);
    };
    helpBtn?.addEventListener("click", (event) => {
      event.stopPropagation();
      setHelpOpen(popover?.classList.contains("hidden") ?? false);
    });
    document.addEventListener("click", (event) => {
      if (!popover || popover.classList.contains("hidden")) return;
      if (popover.contains(event.target) || helpBtn?.contains(event.target)) return;
      setHelpOpen(false);
    });
    searchInput.addEventListener("input", () => {
      healthSearchQuery = searchInput.value;
      renderModHealthIssues(list, currentReport);
    });
    searchInput.addEventListener("keydown", (event) => {
      if (event.key !== "Escape") return;
      if (!searchInput.value) return;
      event.stopPropagation();
      searchInput.value = "";
      healthSearchQuery = "";
      renderModHealthIssues(list, currentReport);
    });
  }

  const applyReport = (report) => {
    currentReport = report || null;
    if (summary) summary.textContent = formatHealthReportSummary(report);
    renderModHealthIssues(list, report);
    const duplicates = Number(report?.counts?.duplicate_entry) || 0;
    if (fixButton) fixButton.disabled = duplicates === 0;
    const missing = Number(report?.counts?.missing_file) || 0;
    if (fixMissingButton) fixMissingButton.disabled = missing === 0;
    const dependencyIssues =
      (Number(report?.counts?.dependency_disabled) || 0) + (Number(report?.counts?.dependency_missing) || 0);
    if (fixDependenciesButton) fixDependenciesButton.disabled = dependencyIssues === 0;
    if (copyReportButton) copyReportButton.disabled = !currentReport;
    if (saveReportButton) saveReportButton.disabled = !currentReport;
  };
  const runCheck = async () => {
    const report = await deps.RunModHealthCheck({ deepScan: Boolean(deepInput?.checked) });
    applyReport(report);
    return report;
  };

  runButton?.addEventListener("click", async () => {
    runButton.disabled = true;
    if (summary) summary.textContent = t("settings.addonlist.health.running");
    try {
      const report = await runCheck();
      deps.showNotification(
        formatHealthReportSummary(report),
        Number(report?.totalIssues) > 0 ? "warning" : "success",
      );
    } catch (error) {
      if (summary) summary.textContent = t("settings.addonlist.health.runFailed", { error: String(error?.message || error) });
    } finally {
      runButton.disabled = false;
    }
  });

  // 两个"可自动修复"动作抽成函数：卡片上的按钮与问题行里的「修复」按钮共用同一套实现，
  // 与 FireAxe 的 `Problem.TryAutomaticallyFix` 一样，只走白名单、且写盘前后都有备份。
  const fixDuplicateEntries = async () => {
    fixButton.disabled = true;
    try {
      const removed = await deps.RemoveDuplicateAddonListEntries();
      deps.showNotification(
        removed > 0 ? t("settings.addonlist.health.fixDuplicatesDone", { count: removed }) : t("settings.addonlist.health.fixDuplicatesNone"),
        "success",
      );
      deps.refreshFilesKeepFilter?.();
      await runCheck();
    } catch (error) {
      if (summary) summary.textContent = t("settings.addonlist.health.fixDuplicatesFailed", { error: String(error?.message || error) });
    } finally {
      fixButton.disabled = false;
    }
  };

  const fixMissingEntries = async () => {
    // 用应用内确认弹窗：WebView2 的原生 confirm 在锁屏 / 自动化环境下会阻塞页面。
    showConfirmModal(
      t("settings.addonlist.health.fixMissingConfirmTitle"),
      t("settings.addonlist.health.fixMissingConfirm"),
      async () => {
        fixMissingButton.disabled = true;
        try {
          const removed = await deps.RemoveMissingFileAddonListEntries();
          deps.showNotification(
            removed > 0 ? t("settings.addonlist.health.fixMissingDone", { count: removed }) : t("settings.addonlist.health.fixMissingNone"),
            "success",
          );
          deps.refreshFilesKeepFilter?.();
          await runCheck();
        } catch (error) {
          if (summary) summary.textContent = t("settings.addonlist.health.fixMissingFailed", { error: String(error?.message || error) });
        } finally {
          fixMissingButton.disabled = false;
        }
      },
    );
  };

  // 依赖未开启（对齐 FireAxe `AddonDependencyProblem` 的 CanAutomaticallyFix）：
  // 只给"这一条问题"的主 Mod 启用它声明的依赖，不动别的 Mod。
  const fixDependenciesFor = async (target) => {
    const key = String(target || "").trim();
    if (!key) {
      if (summary) summary.textContent = t("settings.addonlist.health.noFixTarget");
      return;
    }
    // 与其它修复动作一致：写盘前先用应用内确认框说明会改什么（WebView2 的原生 confirm 会阻塞页面）。
    showConfirmModal(
      t("settings.addonlist.health.fixDependenciesConfirmTitle"),
      t("settings.addonlist.health.fixDependenciesConfirm", { name: key }),
      async () => {
        try {
          const result = await deps.EnableModDependencies(key);
          deps.showNotification(
            formatDependencyEnableSummary(key, result),
            result?.missing?.length ? "warning" : "success",
          );
          deps.refreshFilesKeepFilter?.();
          await runCheck();
        } catch (error) {
          if (summary) summary.textContent = t("settings.addonlist.dependencies.enableFailed", { error: String(error?.message || error) });
        }
      },
    );
  };

  // 问题行里的「修复」按钮 → 复用上面三个白名单动作。
  healthIssueAutoFix = async (actionId, target) => {
    if (actionId === "remove-duplicates") return fixDuplicateEntries();
    if (actionId === "remove-missing") return fixMissingEntries();
    if (actionId === "enable-dependencies") return fixDependenciesFor(target);
  };

  fixButton?.addEventListener("click", () => void fixDuplicateEntries());
  fixMissingButton?.addEventListener("click", () => void fixMissingEntries());

  fixDependenciesButton?.addEventListener("click", async () => {
    fixDependenciesButton.disabled = true;
    try {
      const result = await deps.EnableAllMissingModDependencies();
      deps.showNotification(
        formatDependencyBatchEnableSummary(result),
        result?.missing?.length ? "warning" : "success",
      );
      deps.refreshFilesKeepFilter?.();
      await runCheck();
    } catch (error) {
      if (summary) summary.textContent = t("settings.addonlist.health.fixDependenciesFailed", { error: String(error?.message || error) });
    } finally {
      fixDependenciesButton.disabled = false;
    }
  });

  const buildCurrentReportMarkdown = () =>
    buildHealthReportMarkdown(currentReport, { generatedAt: new Date().toLocaleString("zh-CN") });

  copyReportButton?.addEventListener("click", async () => {
    if (!currentReport) return;
    try {
      await copyTextToClipboard(buildCurrentReportMarkdown());
      deps.showNotification(t("settings.addonlist.health.reportCopied"), "success");
    } catch (error) {
      if (summary) summary.textContent = t("settings.addonlist.health.copyReportFailed", { error: String(error?.message || error) });
    }
  });

  saveReportButton?.addEventListener("click", async () => {
    if (!currentReport) return;
    saveReportButton.disabled = true;
    try {
      const path = await deps.SaveModHealthReport(buildCurrentReportMarkdown());
      deps.showNotification(t("settings.addonlist.health.reportSaved", { path }), "success");
      if (typeof deps.OpenFileLocation === "function") {
        await deps.OpenFileLocation(path);
      }
    } catch (error) {
      if (summary) summary.textContent = t("settings.addonlist.health.saveReportFailed", { error: String(error?.message || error) });
    } finally {
      saveReportButton.disabled = false;
    }
  });
}

function bindModEnableProfileSettings(deps) {
  const status = document.getElementById("settings-profile-status");
  const nameInput = document.getElementById("settings-profile-name");
  const captureButton = document.getElementById("settings-profile-capture");
  const importButton = document.getElementById("settings-profile-import");
  const automationInput = document.getElementById("settings-profile-include-automation");

  const setStatus = (message) => {
    if (status) status.textContent = message;
  };
  const refresh = () => {
    if (typeof deps.refreshAddonListPanel === "function") {
      deps.refreshAddonListPanel();
    }
  };

  captureButton?.addEventListener("click", async () => {
    const name = (nameInput?.value || "").trim();
    if (!name) {
      setStatus(t("settings.addonlist.profiles.needName"));
      nameInput?.focus();
      return;
    }
    captureButton.disabled = true;
    try {
      if (automationInput?.checked && typeof deps.CaptureModEnableProfileWithAutomation === "function") {
        await deps.CaptureModEnableProfileWithAutomation(name, "", true);
      } else {
        await deps.CaptureModEnableProfile(name, "");
      }
      if (nameInput) nameInput.value = "";
      deps.showNotification(t("settings.addonlist.profiles.saved", { name }), "success");
      refresh();
    } catch (error) {
      setStatus(t("settings.addonlist.profiles.captureFailed", { error: String(error?.message || error) }));
    } finally {
      captureButton.disabled = false;
    }
  });

  importButton?.addEventListener("click", async () => {
    importButton.disabled = true;
    try {
      const imported = await deps.ImportModEnableProfile();
      if (!imported?.id) {
        return; // 用户取消
      }
      deps.showNotification(t("settings.addonlist.profiles.imported", { name: imported.name }), "success");
      refresh();
    } catch (error) {
      setStatus(t("settings.addonlist.profiles.importFailed", { error: String(error?.message || error) }));
    } finally {
      importButton.disabled = false;
    }
  });

  document.querySelectorAll("#settings-page-content .settings-profile-apply").forEach((button) => {
    button.addEventListener("click", async () => {
      const id = button.dataset.profileId;
      if (!id) return;
      if (
        !(await confirmInApp(t("settings.addonlist.profiles.applyConfirm"), {
          title: t("settings.addonlist.profiles.applyConfirmTitle"),
        }))
      ) {
        return;
      }
      button.disabled = true;
      try {
        const result = await deps.ApplyModEnableProfile(id);
        deps.showNotification(formatProfileApplySummary(result), "success");
        deps.refreshFilesKeepFilter?.();
        refresh();
      } catch (error) {
        setStatus(t("settings.addonlist.profiles.applyFailed", { error: String(error?.message || error) }));
        button.disabled = false;
      }
    });
  });

  document.querySelectorAll("#settings-page-content .settings-profile-export").forEach((button) => {
    button.addEventListener("click", async () => {
      const id = button.dataset.profileId;
      if (!id) return;
      button.disabled = true;
      try {
        const targetPath = await deps.ExportModEnableProfile(id);
        if (!targetPath) {
          return; // 用户取消
        }
        deps.showNotification(t("settings.addonlist.profiles.exported", { path: targetPath }), "success");
      } catch (error) {
        setStatus(t("settings.addonlist.profiles.exportFailed", { error: String(error?.message || error) }));
      } finally {
        button.disabled = false;
      }
    });
  });

  document.querySelectorAll("#settings-page-content .settings-profile-delete").forEach((button) => {
    button.addEventListener("click", async () => {
      const id = button.dataset.profileId;
      if (!id) return;
      if (
        !(await confirmInApp(t("settings.addonlist.profiles.deleteConfirm"), {
          title: t("settings.addonlist.profiles.deleteConfirmTitle"),
        }))
      ) {
        return;
      }
      button.disabled = true;
      try {
        await deps.DeleteModEnableProfile(id);
        deps.showNotification(t("settings.addonlist.profiles.deleted"), "success");
        refresh();
      } catch (error) {
        setStatus(t("settings.addonlist.profiles.deleteFailed", { error: String(error?.message || error) }));
        button.disabled = false;
      }
    });
  });
}

function bindConflictAnalysisSettings(deps) {
  const priorityToggle = document.getElementById("settings-conflict-priority-aware");
  priorityToggle?.addEventListener("change", async () => {
    const enabled = priorityToggle.checked;
    priorityToggle.disabled = true;
    try {
      const config = deps.getConfig();
      config.conflictPriorityAware = enabled;
      await deps.saveConfig(config);
      deps.appState.conflictAnalysisOptions = {
        ...deps.appState.conflictAnalysisOptions,
        priorityAware: enabled,
      };
      // 口径变了，列表角标必须立刻按新口径重算：同一批重叠会从“冲突”
      // 变成“覆盖关系”（或反过来），不重算就会停在旧结论上。
      if (deps.appState.conflictAnalysisEnabled) {
        try {
          const { runScopedConflictAnalysis } = await import("../conflicts/conflicts.js");
          await runScopedConflictAnalysis({ silent: true });
        } catch (error) {
          console.warn("按新口径重算冲突分析失败:", error);
        }
      }
      deps.showNotification(
        enabled ? t("settings.addonlist.conflict.priorityAware.on") : t("settings.addonlist.conflict.priorityAware.off"),
        "success",
      );
    } catch (error) {
      priorityToggle.checked = !enabled;
      deps.showNotification(t("settings.addonlist.conflict.priorityAware.saveFailed", { error }), "error");
    } finally {
      priorityToggle.disabled = false;
    }
  });

  const editor = document.getElementById("settings-conflict-ignore-files");
  const saveButton = document.getElementById("settings-conflict-save-ignore");
  const status = document.getElementById("settings-conflict-ignore-status");
  saveButton?.addEventListener("click", async () => {
    const entries = parseConflictIgnoreList(editor?.value || "");
    saveButton.disabled = true;
    try {
      const config = deps.getConfig();
      config.conflictIgnoreFiles = entries;
      await deps.saveConfig(config);
      if (editor) editor.value = formatConflictIgnoreList(entries);
      if (status) {
        status.textContent = entries.length
          ? t("settings.addonlist.conflict.ignoreList.saved", { count: entries.length })
          : t("settings.addonlist.conflict.ignoreList.cleared");
      }
      deps.showNotification(t("settings.addonlist.conflict.ignoreList.notify"), "success");
    } catch (error) {
      if (status) status.textContent = t("settings.addonlist.conflict.ignoreList.retry");
      deps.showNotification(t("settings.addonlist.conflict.ignoreList.saveFailed", { error }), "error");
    } finally {
      saveButton.disabled = false;
    }
  });

  // 原版文件白名单：只读状态 + 按需生成增量批次（只写配置目录）。
  const whitelistStatus = document.getElementById("settings-stock-whitelist-status");
  const describeWhitelistStatus = (state) => {
    if (!state) return "";
    const userCount = (state.userBatches || []).length;
    const degraded = state.degraded ? t("settings.addonlist.stockWhitelist.degraded") : "";
    return t("settings.addonlist.stockWhitelist.status", {
      builtin: (state.builtinBatches || []).length,
      user: userCount,
      total: state.totalPaths || 0,
      degraded,
    });
  };
  const reloadWhitelistStatus = async () => {
    try {
      const state = await deps.GetStockWhitelistStatus();
      if (whitelistStatus) whitelistStatus.textContent = describeWhitelistStatus(state);
    } catch (error) {
      if (whitelistStatus) whitelistStatus.textContent = t("settings.addonlist.stockWhitelist.statusFailed", { error: String(error?.message || error) });
    }
  };
  void reloadWhitelistStatus();

  const generateButton = document.getElementById("settings-stock-whitelist-generate");
  generateButton?.addEventListener("click", async () => {
    generateButton.disabled = true;
    if (whitelistStatus) whitelistStatus.textContent = t("settings.addonlist.stockWhitelist.parsing");
    try {
      const state = await deps.GenerateStockWhitelistBatchesFromGame();
      if (whitelistStatus) whitelistStatus.textContent = describeWhitelistStatus(state);
      deps.showNotification(t("settings.addonlist.stockWhitelist.generated"), "success");
    } catch (error) {
      if (whitelistStatus) whitelistStatus.textContent = t("settings.addonlist.stockWhitelist.generateFailed", { error: String(error?.message || error) });
    } finally {
      generateButton.disabled = false;
    }
  });
  document.getElementById("settings-stock-whitelist-reload")?.addEventListener("click", async () => {
    try {
      await deps.ReloadStockWhitelist();
    } catch (error) {
      console.warn("刷新原版白名单失败:", error);
    }
    await reloadWhitelistStatus();
  });

  // 工坊合集实体化：保存 / 跟随节点检查 / 下载缺失成员 / 删除记录。
  const collectionStatus = document.getElementById("settings-collection-status");
  const setCollectionStatus = (message) => {
    workshopCollectionStatusText = message || "";
    if (collectionStatus) collectionStatus.textContent = workshopCollectionStatusText;
  };
  // 保存 / 删除 / 检查之后要把这块列表重新渲染一遍。
  // 这里原来调用的那个刷新函数不存在于本作用域（它只定义在相邻两个绑定函数里），
  // 真机上抛 "refresh is not defined"：保存明明成功，
  // 状态却显示「保存合集失败: refresh is not defined」，记录列表也不刷新，
  // 记录行里的「检查更新 / 下载缺失成员 / 删除记录」按钮一直不出现。
  const refreshCollections = () => Promise.resolve(deps.refreshAddonListPanel?.());
  document.getElementById("settings-collection-capture")?.addEventListener("click", async (event) => {
    const input = document.getElementById("settings-collection-input");
    const raw = String(input?.value || "").trim();
    if (!raw) {
      setCollectionStatus(t("settings.workshop.collections.needInput"));
      return;
    }
    const button = event.currentTarget;
    button.disabled = true;
    setCollectionStatus(t("settings.workshop.collections.parsing"));
    try {
      const collectionId = await deps.ParseWorkshopID(raw);
      const link = await deps.CaptureWorkshopCollection(collectionId);
      if (input) input.value = "";
      setCollectionStatus(
        t("settings.workshop.collections.saved", {
          title: link.title || link.collectionId,
          summary: formatCollectionSummary(link),
        }),
      );
      deps.showNotification(t("settings.workshop.collections.savedNotify"), "success");
      void refreshCollections();
    } catch (error) {
      setCollectionStatus(t("settings.workshop.collections.saveFailed", { error: String(error?.message || error) }));
    } finally {
      button.disabled = false;
    }
  });

  document.getElementById("settings-collection-check-all")?.addEventListener("click", async (event) => {
    const button = event.currentTarget;
    button.disabled = true;
    setCollectionStatus(t("settings.workshop.collections.checkingAll"));
    try {
      const results = (await deps.CheckWorkshopCollectionUpdates()) || [];
      // 后端现在逐条检查：坏记录不再让整轮失败，失败原因在 result.error 里，
      // 汇总文案由 formatCollectionCheckAllSummary 负责（含"N 个检查失败：原因"）。
      setCollectionStatus(formatCollectionCheckAllSummary(results));
      void refreshCollections();
    } catch (error) {
      setCollectionStatus(t("settings.workshop.collections.checkFailed", { error: String(error?.message || error) }));
    } finally {
      button.disabled = false;
    }
  });

  document.querySelectorAll("#settings-page-content .settings-collection-refresh").forEach((button) => {
    button.addEventListener("click", async () => {
      button.disabled = true;
      try {
        const result = await deps.RefreshWorkshopCollection(button.dataset.collectionId);
        setCollectionStatus(`${result.title || result.collectionId}：${formatCollectionRefreshSummary(result)}`);
        void refreshCollections();
      } catch (error) {
        setCollectionStatus(t("settings.workshop.collections.checkFailed", { error: String(error?.message || error) }));
        button.disabled = false;
      }
    });
  });
  document.querySelectorAll("#settings-page-content .settings-collection-download").forEach((button) => {
    button.addEventListener("click", async () => {
      button.disabled = true;
      try {
        const queued = await deps.DownloadWorkshopCollection(button.dataset.collectionId);
        setCollectionStatus(formatCollectionQueueSummary(queued));
        deps.showNotification(formatCollectionQueueSummary(queued), "success");
      } catch (error) {
        setCollectionStatus(t("settings.workshop.collections.downloadFailed", { error: String(error?.message || error) }));
      } finally {
        button.disabled = false;
      }
    });
  });
  document.querySelectorAll("#settings-page-content .settings-collection-delete").forEach((button) => {
    button.addEventListener("click", async () => {
      if (
        !(await confirmInApp(t("settings.workshop.collections.deleteConfirm"), {
          title: t("settings.workshop.collections.deleteConfirmTitle"),
        }))
      ) {
        return;
      }
      button.disabled = true;
      try {
        await deps.DeleteWorkshopCollection(button.dataset.collectionId);
        setCollectionStatus(t("settings.workshop.collections.deleted"));
        void refreshCollections();
      } catch (error) {
        setCollectionStatus(t("settings.workshop.collections.deleteFailed", { error: String(error?.message || error) }));
        button.disabled = false;
      }
    });
  });
}

function bindSettingsPage(deps) {
  enhanceSettingsNav();
  bindConflictAnalysisSettings(deps);
  bindModEnableProfileSettings(deps);
  bindModDependencySettings(deps);
  bindModHealthCheckSettings(deps);
  // 设置页只保留入口：策略组的编辑界面在独立的「策略组管理」窗口里。
  document
    .getElementById("settings-open-strategy-manager")
    ?.addEventListener("click", () => void openStrategyGroupManager());

  document.querySelectorAll("#settings-page-content .settings-nav-item").forEach((item) => {
    item.addEventListener("click", () => {
      const target = item.dataset.panel;
      updateSettingsPanelDirection(item);
      document.querySelectorAll("#settings-page-content .settings-nav-item").forEach((nav) => nav.classList.remove("active"));
      document.querySelectorAll("#settings-page-content .settings-panel").forEach((panel) => panel.classList.remove("active"));
      item.classList.add("active");
      document.getElementById(`settings-panel-${target}`)?.classList.add("active");
      updateSettingsNavIndicator();
      // 常用指令说明（193 条）不参与分区首次布局：等这个分区真的可见了再分片补上。
      if (target === "addonlist") renderAutoexecHelpListIfNeeded();
    });
  });

  const ipToggle = document.getElementById("settings-preferred-ip");
  const ipSection = document.getElementById("settings-ip-mode-section");
  const fixedTools = document.getElementById("settings-fixed-ip-tools");
  const fixedInput = document.getElementById("settings-fixed-ip");
  const ipOptionTrigger = document.getElementById("settings-ip-option-trigger");
  const ipOptionMenu = document.getElementById("settings-ip-option-menu");
  const ipStatus = document.getElementById("settings-ip-status");
  let ipOptions = normalizeIPOptions(deps.ipOptions);
  const refreshIPStatus = async () => {
    await updateIPStatus({
      statusEl: ipStatus,
      ipOptions,
      getUseFixedIP: () => {
        const fixedMode = document.querySelector('input[name="settings-ip-mode"][value="fixed"]')?.checked;
        return Boolean(fixedMode && fixedInput?.value.trim());
      },
      IsSelectingIP: deps.IsSelectingIP,
      GetCurrentBestIP: deps.GetCurrentBestIP,
      GetCurrentBestIPOption: deps.GetCurrentBestIPOption,
    });
  };

  renderIPOptionDropdown({
    options: ipOptions,
    fixedIP: deps.fixedIP,
    trigger: ipOptionTrigger,
    menu: ipOptionMenu,
    fixedInput,
    SetWorkshopFixedIP: deps.SetWorkshopFixedIP,
    getConfig: deps.getConfig,
    saveConfig: deps.saveConfig,
    showNotification: deps.showNotification,
    onStatusUpdate: refreshIPStatus,
  });
  loadIPOptionsForDropdown({
    GetWorkshopIPOptions: deps.GetWorkshopIPOptions,
    fixedIP: deps.fixedIP,
    trigger: ipOptionTrigger,
    menu: ipOptionMenu,
    fixedInput,
    setOptions: (nextOptions) => {
      ipOptions = nextOptions;
    },
    SetWorkshopFixedIP: deps.SetWorkshopFixedIP,
    getConfig: deps.getConfig,
    saveConfig: deps.saveConfig,
    showNotification: deps.showNotification,
    onStatusUpdate: refreshIPStatus,
  });

  if (typeof window._settingsIPSelectionCleanup === "function") {
    window._settingsIPSelectionCleanup();
    window._settingsIPSelectionCleanup = null;
  }
  if (typeof deps.EventsOn === "function") {
    window._settingsIPSelectionCleanup = deps.EventsOn("ip_selection_end", refreshIPStatus);
  }

  ipToggle?.addEventListener("change", async () => {
    const enabled = ipToggle.checked;
    const previousEnabled = !enabled;
    ipToggle.disabled = true;
    try {
      await deps.SetWorkshopPreferredIP(enabled);
      const config = deps.getConfig();
      config.workshopPreferredIP = enabled;
      await deps.saveConfig(config);
      ipSection.style.display = enabled ? "block" : "none";
      deps.showNotification(
        enabled ? t("settings.network.ip.ipOn") : t("settings.network.ip.ipOff"),
        enabled ? "success" : "info",
      );
    } catch (error) {
      ipToggle.checked = previousEnabled;
      ipSection.style.display = previousEnabled ? "block" : "none";
      deps.showNotification(t("settings.network.ip.updateFailed", { error }), "error");
    } finally {
      ipToggle.disabled = false;
    }
  });

  document.querySelectorAll('input[name="settings-ip-mode"]').forEach((radio) => {
    radio.addEventListener("change", async () => {
      const useFixed = radio.value === "fixed" && radio.checked;
      const previousFixedIP = deps.getConfig().workshopFixedIP || "";
      if (!useFixed) {
        radio.disabled = true;
        try {
          await deps.SetWorkshopFixedIP("");
          await deps.SetWorkshopPreferredIP(true);
          const config = deps.getConfig();
          config.workshopFixedIP = "";
          await deps.saveConfig(config);
          if (fixedTools) fixedTools.style.display = "none";
          await refreshIPStatus();
        } catch (error) {
          const fixedRadio = document.querySelector('input[name="settings-ip-mode"][value="fixed"]');
          const autoRadio = document.querySelector('input[name="settings-ip-mode"][value="auto"]');
          if (previousFixedIP) {
            if (fixedRadio) fixedRadio.checked = true;
            if (fixedTools) fixedTools.style.display = "";
          } else {
            if (autoRadio) autoRadio.checked = true;
            if (fixedTools) fixedTools.style.display = "none";
          }
          deps.showNotification(t("settings.network.ip.modeFailed", { error }), "error");
        } finally {
          radio.disabled = false;
        }
      } else if (fixedTools) {
        fixedTools.style.display = "";
      }
    });
  });

  fixedInput?.addEventListener("change", async () => {
    const fixedIP = fixedInput.value.trim();
    const previousFixedIP = deps.getConfig().workshopFixedIP || "";
    fixedInput.disabled = true;
    try {
      await deps.SetWorkshopFixedIP(fixedIP);
      const config = deps.getConfig();
      config.workshopFixedIP = fixedIP;
      await deps.saveConfig(config);
      updateIPOptionTrigger(ipOptionTrigger, ipOptions, fixedIP);
      syncIPOptionMenuActive(ipOptionMenu, fixedIP);
      await refreshIPStatus();
      deps.showNotification(t("settings.network.ip.fixedUpdated"), "success");
    } catch (error) {
      fixedInput.value = previousFixedIP;
      updateIPOptionTrigger(ipOptionTrigger, ipOptions, previousFixedIP);
      syncIPOptionMenuActive(ipOptionMenu, previousFixedIP);
      deps.showNotification(t("settings.network.ip.fixedFailed", { error }), "error");
    } finally {
      fixedInput.disabled = false;
    }
  });

  // ---- 自定义标签规则（导入 JSON + 预览命中 + 保存并重扫） ----
  const customRulesEditor = document.getElementById("settings-custom-tag-rules");
  const customRulesStatus = document.getElementById("settings-custom-tag-rules-status");
  const customRulesPreviewList = document.getElementById("settings-custom-tag-rules-preview-list");

  const renderCustomRulesPreview = (preview) => {
    if (!customRulesPreviewList) return;
    customRulesPreviewList.innerHTML = "";
    (preview?.issues || []).forEach((issue) => {
      const row = document.createElement("div");
      row.className = "addonlist-status-error";
      row.textContent = t("settings.addonlist.maintenance.customTags.ruleIssue", {
        index: Number(issue.index) + 1,
        message: issue.message,
      });
      customRulesPreviewList.appendChild(row);
    });
    (preview?.items || []).forEach((item) => {
      const row = document.createElement("div");
      row.className = "setting-row-desc";
      const samples = (item.samples || []).slice(0, 3).join("、");
      const samplesPart = samples
        ? t("settings.addonlist.maintenance.customTags.ruleHitSamples", { samples })
        : "";
      row.textContent = t("settings.addonlist.maintenance.customTags.ruleHit", {
        tag: item.tag,
        count: item.matchCount,
        samples: samplesPart,
      });
      customRulesPreviewList.appendChild(row);
    });
    if ((preview?.items || []).length === 0 && (preview?.issues || []).length === 0) {
      const row = document.createElement("div");
      row.className = "setting-row-desc";
      row.textContent = t("settings.addonlist.maintenance.customTags.empty");
      customRulesPreviewList.appendChild(row);
    }
  };

  document.getElementById("settings-custom-tag-rules-preview")?.addEventListener("click", async (event) => {
    const button = event.currentTarget;
    button.disabled = true;
    if (customRulesStatus) customRulesStatus.textContent = t("settings.addonlist.maintenance.customTags.previewing");
    try {
      const preview = await deps.PreviewCustomTagRules(customRulesEditor?.value || "");
      if (customRulesStatus) {
        const issuesPart = (preview?.issues || []).length
          ? t("settings.addonlist.maintenance.customTags.previewIssues", { count: preview.issues.length })
          : "";
        customRulesStatus.textContent = t("settings.addonlist.maintenance.customTags.previewSummary", {
          count: preview?.ruleCount || 0,
          issues: issuesPart,
        });
      }
      renderCustomRulesPreview(preview);
    } catch (error) {
      if (customRulesStatus) customRulesStatus.textContent = "";
      deps.showNotification(t("settings.addonlist.maintenance.customTags.previewFailed", { error }), "error");
    } finally {
      button.disabled = false;
    }
  });

  document.getElementById("settings-custom-tag-rules-save")?.addEventListener("click", async (event) => {
    const button = event.currentTarget;
    button.disabled = true;
    if (customRulesStatus) customRulesStatus.textContent = t("settings.addonlist.maintenance.customTags.saving");
    try {
      const count = await deps.SaveCustomTagRules(customRulesEditor?.value || "");
      await deps.ScanVPKFiles();
      await deps.refreshFilesKeepFilter?.({ silent: true });
      if (customRulesStatus) customRulesStatus.textContent = t("settings.addonlist.maintenance.customTags.saved", { count });
      deps.showNotification(t("settings.addonlist.maintenance.customTags.savedNotify", { count }), "success");
    } catch (error) {
      if (customRulesStatus) customRulesStatus.textContent = "";
      deps.showNotification(t("settings.addonlist.maintenance.customTags.saveFailed", { error }), "error");
    } finally {
      button.disabled = false;
    }
  });

  // ---- 工坊 DNS（对齐上游 5ce7ef5） ----
  const dnsModeInputs = document.querySelectorAll('input[name="settings-workshop-dns-mode"]');
  const dnsAddressInput = document.getElementById("settings-workshop-dns-address");
  const dnsSection = document.getElementById("settings-workshop-dns-section");
  const dnsSaveButton = document.getElementById("settings-workshop-dns-save");
  const syncDNSSectionVisibility = () => {
    const mode = document.querySelector('input[name="settings-workshop-dns-mode"]:checked')?.value || "system";
    if (dnsSection) dnsSection.style.display = mode === "custom" ? "" : "none";
  };
  dnsModeInputs.forEach((input) => input.addEventListener("change", syncDNSSectionVisibility));
  dnsSaveButton?.addEventListener("click", async () => {
    const mode = document.querySelector('input[name="settings-workshop-dns-mode"]:checked')?.value === "custom" ? "custom" : "system";
    const address = (dnsAddressInput?.value || "").trim();
    // 前端先做一次同样的校验：错误立刻提示，不用等后端往返。
    if (mode === "custom" && !isValidDNSAddress(address)) {
      deps.showNotification(t("settings.addonlist.dns.invalid"), "error");
      dnsAddressInput?.focus();
      return;
    }
    const status = document.getElementById("settings-workshop-dns-status");
    dnsSaveButton.disabled = true;
    try {
      const saved = await deps.SetWorkshopDNSConfig({ mode, customAddress: mode === "custom" ? address : "" });
      // 后端已经持久化，这里同步前端缓存（saveConfig 会合并进缓存），
      // 避免后续其它设置保存时把旧值写回去。
      await deps.saveConfig({ workshopDNS: saved });
      if (status) {
        status.textContent = saved?.mode === "custom"
          ? t("settings.network.dns.statusCustom", { address: saved.customAddress })
          : t("settings.network.dns.statusSystem");
      }
      deps.showNotification(
        saved?.mode === "custom"
          ? t("settings.addonlist.dns.switchedCustom", { address: saved.customAddress })
          : t("settings.addonlist.dns.switchedSystem"),
        "success",
      );
    } catch (error) {
      deps.showNotification(t("settings.addonlist.dns.saveFailed", { error }), "error");
    } finally {
      dnsSaveButton.disabled = false;
    }
  });

  const uiScaleInput = document.getElementById("settings-ui-scale");
  const updateUIScale = (value, persist = false) => {
    const scale = applyUIScale(Number(value) / 100);
    if (!persist) return scale;
    const previousScale = normalizeUIScale(deps.getConfig().uiScale);
    const changeToken = ++uiScaleChangeToken;
    const config = deps.getConfig();
    config.uiScale = scale;
    let savePromise;
    try {
      savePromise = deps.saveConfig(config);
    } catch (error) {
      savePromise = Promise.reject(error);
    }
    Promise.resolve(savePromise).catch((error) => {
        if (changeToken !== uiScaleChangeToken) return;
        applyUIScale(previousScale);
        if (uiScaleInput?.isConnected) {
          uiScaleInput.value = String(Math.round(previousScale * 100));
        }
        const rollbackConfig = deps.getConfig();
        rollbackConfig.uiScale = previousScale;
        Promise.resolve(deps.saveConfig(rollbackConfig)).catch((rollbackError) => console.error("界面缩放配置回滚失败:", rollbackError));
        deps.showNotification(t("settings.interface.uiScale.saveFailed", { error }), "error");
      });
    return scale;
  };
  uiScaleInput?.addEventListener("input", () => {
    updateUIScale(uiScaleInput.value);
  });
  uiScaleInput?.addEventListener("change", () => {
    updateUIScale(uiScaleInput.value, true);
  });

  // 阅读舒适度：字号档位只改文字、不动布局；舒适度改行距与对比度。
  // 两者与 uiScale 相乘后写到根字号上，互不覆盖。
  const applyAndSaveReadingComfort = async (patch) => {
    const config = deps.getConfig();
    const applied = applyReadingComfort({
      textSize: patch.textSize ?? config.textSize,
      comfort: patch.comfort ?? config.readingComfort,
      uiScale: normalizeUIScale(config.uiScale),
    });
    config.textSize = applied.textSize;
    config.readingComfort = applied.comfort;
    try {
      await deps.saveConfig(config);
      deps.showNotification(
        t("settings.interface.readingComfort.saved", {
          textSize: textSizeLabel(applied.textSize),
          comfort: readingComfortLabel(applied.comfort),
        }),
        "success",
      );
    } catch (error) {
      deps.showNotification(t("settings.interface.readingComfort.failed", { error }), "error");
    }
  };

  document.querySelectorAll("#settings-text-size input[name='settings-text-size']").forEach((input) => {
    input.addEventListener("change", () => {
      if (!input.checked) return;
      document.querySelectorAll("#settings-text-size .mode-option").forEach((label) => {
        label.classList.toggle("active", label.contains(input));
      });
      void applyAndSaveReadingComfort({ textSize: input.value });
    });
  });

  document.querySelectorAll("#settings-reading-comfort input[name='settings-reading-comfort']").forEach((input) => {
    input.addEventListener("change", () => {
      if (!input.checked) return;
      document.querySelectorAll("#settings-reading-comfort .mode-option").forEach((label) => {
        label.classList.toggle("active", label.contains(input));
      });
      void applyAndSaveReadingComfort({ comfort: input.value });
    });
  });
  document.getElementById("settings-ui-scale-decrease")?.addEventListener("click", () => {
    const current = normalizeUIScale(deps.getConfig().uiScale);
    updateUIScale(Math.round(Math.max(MIN_UI_SCALE, current - UI_SCALE_STEP) * 100), true);
  });
  document.getElementById("settings-ui-scale-increase")?.addEventListener("click", () => {
    const current = normalizeUIScale(deps.getConfig().uiScale);
    updateUIScale(Math.round(Math.min(MAX_UI_SCALE, current + UI_SCALE_STEP) * 100), true);
  });

  document.querySelectorAll('input[name="settings-display-mode"]').forEach((radio) => {
    radio.addEventListener("change", async () => {
      if (!radio.checked) return;
      const previousMode = deps.getConfig().displayMode === "card" ? "card" : "list";
      const nextMode = radio.value === "card" ? "card" : "list";
      if (previousMode === nextMode) return;
      const changeToken = ++displayModeChangeToken;
      const group = radio.closest(".mode-toggle-group");
      const controls = Array.from(group?.querySelectorAll('input[name="settings-display-mode"]') || []);
      controls.forEach((input) => { input.disabled = true; });
      deps.appState.displayMode = nextMode;
      group?.querySelectorAll(".mode-option").forEach((option) => option.classList.toggle("active", option.querySelector("input")?.value === nextMode));
      deps.renderFileList();
      try {
        const config = deps.getConfig();
        config.displayMode = nextMode;
        await deps.saveConfig(config);
      } catch (error) {
        if (changeToken !== displayModeChangeToken) return;
        deps.appState.displayMode = previousMode;
        const previousRadio = group?.querySelector(`input[value="${previousMode}"]`);
        if (previousRadio) previousRadio.checked = true;
        group?.querySelectorAll(".mode-option").forEach((option) => option.classList.toggle("active", option.querySelector("input")?.value === previousMode));
        deps.renderFileList();
        const rollbackConfig = deps.getConfig();
        rollbackConfig.displayMode = previousMode;
        Promise.resolve(deps.saveConfig(rollbackConfig)).catch((rollbackError) => console.error("显示模式配置回滚失败:", rollbackError));
        deps.showNotification(t("settings.interface.displayMode.saveFailed", { error }), "error");
      } finally {
        if (changeToken === displayModeChangeToken) controls.forEach((input) => { input.disabled = false; });
      }
    });
  });

  document.querySelectorAll('input[name="settings-filter-layout"]').forEach((radio) => {
    radio.addEventListener("change", () => {
      if (!radio.checked) return;

      const changeToken = ++filterLayoutChangeToken;
      // saveConfig() updates its in-memory cache before the asynchronous Wails
      // write completes. Keep only the previous layout value so a failed write
      // cannot overwrite unrelated settings changed while the request is queued.
      const previousConfig = deps.getConfig();
      const previousMode = previousConfig.filterLayoutMode === "classic" ? "classic" : "compact";
      const nextMode = radio.value === "classic" ? "classic" : "compact";
      const toggleGroup = radio.closest(".mode-toggle-group");

      const syncLayoutControls = (mode) => {
        const selectedRadio = toggleGroup?.querySelector(`input[value="${mode}"]`);
        if (selectedRadio) selectedRadio.checked = true;
        toggleGroup?.querySelectorAll(".mode-option").forEach((option) => {
          option.classList.toggle("active", option.querySelector("input")?.value === mode);
        });
      };

      // Apply the visual layout before waiting for disk/Wails I/O. renderTagFilters
      // updates the layout classes and clears/rebuilds its containers synchronously
      // before its first backend await, so the click has immediate feedback.
      deps.appState.filterLayoutMode = nextMode;
      syncLayoutControls(nextMode);
      Promise.resolve(deps.renderTagFilters?.()).catch((error) => {
        console.error("切换筛选布局时渲染失败:", error);
      });

      const config = deps.getConfig();
      config.filterLayoutMode = nextMode;
      Promise.resolve()
        .then(() => deps.saveConfig(config))
        .then(() => {
          if (changeToken !== filterLayoutChangeToken) return;
          deps.showNotification(
        nextMode === "classic"
          ? t("settings.interface.filterLayout.classicOn")
          : t("settings.interface.filterLayout.compactOn"),
        "success",
      );
        })
        .catch((error) => {
          // A newer click owns the UI and must not be rolled back by an older
          // request finishing late.
          if (changeToken !== filterLayoutChangeToken) return;
          deps.appState.filterLayoutMode = previousMode;
          syncLayoutControls(previousMode);

          const rollbackConfig = deps.getConfig();
          rollbackConfig.filterLayoutMode = previousMode;
          Promise.resolve()
            .then(() => deps.saveConfig(rollbackConfig))
            .catch((rollbackError) => console.error("筛选布局配置回滚失败:", rollbackError));
          Promise.resolve(deps.renderTagFilters?.()).catch((renderError) => {
            console.error("筛选布局回滚渲染失败:", renderError);
          });
          deps.showNotification(t("settings.interface.filterLayout.saveFailed", { error }), "error");
        });
    });
  });

  document.getElementById("settings-box-selection")?.addEventListener("change", async (e) => {
    const toggle = e.currentTarget;
    const enabled = toggle.checked;
    const previousEnabled = !enabled;
    const changeToken = ++boxSelectionChangeToken;
    toggle.disabled = true;
    deps.appState.boxSelectionEnabled = enabled;
    try {
      const config = deps.getConfig();
      config.boxSelectionEnabled = enabled;
      await deps.saveConfig(config);
      deps.showNotification(
        enabled ? t("settings.interface.boxSelection.on") : t("settings.interface.boxSelection.off"),
        "info",
      );
    } catch (error) {
      if (changeToken !== boxSelectionChangeToken) return;
      toggle.checked = previousEnabled;
      deps.appState.boxSelectionEnabled = previousEnabled;
      const rollbackConfig = deps.getConfig();
      rollbackConfig.boxSelectionEnabled = previousEnabled;
      Promise.resolve(deps.saveConfig(rollbackConfig)).catch((rollbackError) => console.error("框选模式配置回滚失败:", rollbackError));
      deps.showNotification(t("settings.interface.boxSelection.saveFailed", { error }), "error");
    } finally {
      if (changeToken === boxSelectionChangeToken) toggle.disabled = false;
    }
  });

  document.getElementById("settings-ctrl-click-selection")?.addEventListener("change", async (e) => {
    const toggle = e.currentTarget;
    const enabled = toggle.checked;
    const previousEnabled = !enabled;
    const changeToken = ++ctrlClickSelectionChangeToken;
    toggle.disabled = true;
    deps.appState.ctrlClickSelectionEnabled = enabled;
    try {
      const config = deps.getConfig();
      config.ctrlClickSelectionEnabled = enabled;
      await deps.saveConfig(config);
      deps.showNotification(
        enabled ? t("settings.interface.ctrlClick.on") : t("settings.interface.ctrlClick.off"),
        "info",
      );
    } catch (error) {
      if (changeToken !== ctrlClickSelectionChangeToken) return;
      toggle.checked = previousEnabled;
      deps.appState.ctrlClickSelectionEnabled = previousEnabled;
      const rollbackConfig = deps.getConfig();
      rollbackConfig.ctrlClickSelectionEnabled = previousEnabled;
      Promise.resolve(deps.saveConfig(rollbackConfig)).catch((rollbackError) => console.error("Ctrl+单击选择配置回滚失败:", rollbackError));
      deps.showNotification(t("settings.interface.ctrlClick.saveFailed", { error }), "error");
    } finally {
      if (changeToken === ctrlClickSelectionChangeToken) toggle.disabled = false;
    }
  });

  document.getElementById("settings-meta-enabled")?.addEventListener("change", async (event) => {
    const metaToggle = event.currentTarget;
    const metaEnabled = metaToggle.checked;
    const previousMetaEnabled = !metaEnabled;
    const updateCheckToggle = document.getElementById("settings-update-check-enabled");
    const previousUpdateCheckEnabled = Boolean(updateCheckToggle?.checked);
    const nextUpdateCheckEnabled = metaEnabled ? previousUpdateCheckEnabled : false;
    const previousConfig = deps.getConfig();
    const changeToken = ++workshopSettingsChangeToken;
    metaToggle.disabled = true;
    // Meta 开关正在提交时，更新检测不能再并发写入，否则两个 setter 和
    // SaveAppConfig 的返回顺序会让界面与实际配置走向不同的状态。
    if (updateCheckToggle) updateCheckToggle.disabled = true;
    document.getElementById("settings-manual-check-btn")?.setAttribute("disabled", "true");
    try {
      await deps.SetWorkshopMetaEnabled(metaEnabled);
      if (changeToken !== workshopSettingsChangeToken) return;
      if (nextUpdateCheckEnabled !== previousUpdateCheckEnabled) {
        await deps.SetWorkshopUpdateCheckEnabled(nextUpdateCheckEnabled);
      }
      if (changeToken !== workshopSettingsChangeToken) return;
      const config = deps.getConfig();
      config.workshopMetaEnabled = metaEnabled;
      config.workshopUpdateCheckEnabled = nextUpdateCheckEnabled;
      await deps.saveConfig(config);
      if (changeToken !== workshopSettingsChangeToken) return;
      deps.appState.workshopUpdateCheckEnabled = nextUpdateCheckEnabled;
      syncWorkshopDataControls(metaEnabled, nextUpdateCheckEnabled);
      deps.showNotification(
        metaEnabled ? t("settings.workshop.data.meta.on") : t("settings.workshop.data.meta.off"),
        metaEnabled ? "success" : "info",
      );
    } catch (error) {
      if (changeToken !== workshopSettingsChangeToken) return;
      // 若后续级联步骤失败，尽力恢复已成功写入的 setter；即使恢复同样失败，
      // UI 也仍回到提交前的明确状态，而不会遗留半禁用的显示。
      try {
        if (nextUpdateCheckEnabled !== previousUpdateCheckEnabled) {
          await deps.SetWorkshopUpdateCheckEnabled(previousUpdateCheckEnabled);
        }
        await deps.SetWorkshopMetaEnabled(previousMetaEnabled);
        const rollbackConfig = {
          ...previousConfig,
          workshopMetaEnabled: previousMetaEnabled,
          workshopUpdateCheckEnabled: previousUpdateCheckEnabled,
        };
        await deps.saveConfig(rollbackConfig);
      } catch (rollbackError) {
        console.error("工坊数据设置回滚失败:", rollbackError);
      }
      deps.appState.workshopUpdateCheckEnabled = previousUpdateCheckEnabled;
      syncWorkshopDataControls(previousMetaEnabled, previousUpdateCheckEnabled);
      deps.showNotification(t("settings.workshop.data.meta.saveFailed", { error }), "error");
    } finally {
      if (changeToken === workshopSettingsChangeToken) {
        // 失败路径已将控件恢复到旧状态；这里必须按当前 DOM 状态重新启用，
        // 不能按本次请求的目标状态覆盖它。
        const currentMetaEnabled = Boolean(document.getElementById("settings-meta-enabled")?.checked);
        const currentUpdateCheckEnabled = Boolean(document.getElementById("settings-update-check-enabled")?.checked);
        syncWorkshopDataControls(currentMetaEnabled, currentUpdateCheckEnabled);
        metaToggle.disabled = false;
      }
    }

    // 列表刷新属于已保存设置后的派生工作。它失败不应回滚已经成功写入的设置。
    if (changeToken !== workshopSettingsChangeToken || metaToggle.checked !== metaEnabled) return;
    try {
      await deps.refreshFilesKeepFilter();
    } catch (refreshError) {
      console.error("工坊信息存储设置已保存，但列表刷新失败:", refreshError);
      deps.showNotification(t("settings.workshop.data.meta.refreshFailed"), "warning");
    }
  });

  document.getElementById("settings-update-check-enabled")?.addEventListener("change", async (event) => {
    const updateToggle = event.currentTarget;
    const enabled = updateToggle.checked;
    const previousEnabled = !enabled;
    const metaEnabled = Boolean(document.getElementById("settings-meta-enabled")?.checked);
    const previousConfig = deps.getConfig();
    const changeToken = ++workshopSettingsChangeToken;
    updateToggle.disabled = true;
    try {
      await deps.SetWorkshopUpdateCheckEnabled(enabled);
      if (changeToken !== workshopSettingsChangeToken) return;
      deps.appState.workshopUpdateCheckEnabled = enabled;
      const config = deps.getConfig();
      config.workshopUpdateCheckEnabled = enabled;
      await deps.saveConfig(config);
      if (changeToken !== workshopSettingsChangeToken) return;
      syncWorkshopDataControls(metaEnabled, enabled);
      deps.showNotification(
        enabled ? t("settings.workshop.data.updateCheck.on") : t("settings.workshop.data.updateCheck.off"),
        enabled ? "success" : "info",
      );
    } catch (error) {
      if (changeToken !== workshopSettingsChangeToken) return;
      try {
        await deps.SetWorkshopUpdateCheckEnabled(previousEnabled);
        const rollbackConfig = {
          ...previousConfig,
          workshopUpdateCheckEnabled: previousEnabled,
        };
        await deps.saveConfig(rollbackConfig);
      } catch (rollbackError) {
        console.error("Mod 更新检测设置回滚失败:", rollbackError);
      }
      deps.appState.workshopUpdateCheckEnabled = previousEnabled;
      syncWorkshopDataControls(metaEnabled, previousEnabled);
      deps.showNotification(t("settings.workshop.data.updateCheck.saveFailed", { error }), "error");
    } finally {
      if (changeToken === workshopSettingsChangeToken) {
        const currentMetaEnabled = Boolean(document.getElementById("settings-meta-enabled")?.checked);
        const currentUpdateCheckEnabled = Boolean(document.getElementById("settings-update-check-enabled")?.checked);
        syncWorkshopDataControls(currentMetaEnabled, currentUpdateCheckEnabled);
        updateToggle.disabled = !currentMetaEnabled;
      }
    }
  });

  // 下载失败自动重下：默认关闭；只影响新建任务的默认值。
  document.getElementById("settings-auto-redownload")?.addEventListener("change", async (event) => {
    const toggle = event.currentTarget;
    const enabled = toggle.checked;
    toggle.disabled = true;
    try {
      await deps.SetWorkshopAutoRedownload(enabled);
      deps.showNotification(
        enabled ? t("settings.workshop.data.autoRedownload.on") : t("settings.workshop.data.autoRedownload.off"),
        "success",
      );
    } catch (error) {
      toggle.checked = !enabled;
      deps.showNotification(t("settings.workshop.data.autoRedownload.saveFailed", { error }), "error");
    } finally {
      toggle.disabled = false;
    }
  });

  // 「打开文件方式」（对齐 FireAxe v0.7.2 的 process file customization）的点击处理
  // 放在 app-runtime 的事件委托里：设置面板会整块重渲染，直接绑在按钮上的监听器
  // 会在重渲染后失效（真机上表现为"按钮有焦点但点了没反应"）。

  // 抓取工坊官方标签与统计（对齐 FireAxe 直接问 Steam 官方接口要 tags/stats）：
  // 只补 .meta，不动 Mod 文件与 addonlist；抓完把结果数量说清楚。
  // 剪贴板工坊链接自动识别：默认开启；只影响"要不要提示"，关掉后不再读剪贴板。
  document.getElementById("settings-auto-detect-workshop-link")?.addEventListener("change", async (event) => {
    const toggle = event.currentTarget;
    const enabled = toggle.checked;
    toggle.disabled = true;
    try {
      const config = deps.getConfig();
      config.autoDetectWorkshopLink = enabled;
      await deps.saveConfig(config);
      deps.showNotification(
        enabled ? t("settings.workshop.data.clipboard.on") : t("settings.workshop.data.clipboard.off"),
        enabled ? "success" : "info",
      );
    } catch (error) {
      toggle.checked = !enabled;
      deps.showNotification(t("settings.workshop.data.clipboard.saveFailed", { error }), "error");
    } finally {
      toggle.disabled = false;
    }
  });

  document.getElementById("settings-workshop-enrich")?.addEventListener("click", async (event) => {
    const button = event.currentTarget;
    const label = button.querySelector(".trigger-check-text") || button;
    const original = label.textContent;
    button.disabled = true;
    label.textContent = t("settings.workshop.data.enrich.running");
    try {
      const result = await deps.EnrichAllWorkshopMetadata();
      const parts = [t("settings.workshop.data.enrich.updated", { count: result?.updated || 0 })];
      if (result?.unchanged) parts.push(t("settings.workshop.data.enrich.unchanged", { count: result.unchanged }));
      if (result?.missing?.length) parts.push(t("settings.workshop.data.enrich.missing", { count: result.missing.length }));
      if (result?.failed?.length) parts.push(t("settings.workshop.data.enrich.failed", { count: result.failed.length }));
      deps.showNotification(t("settings.workshop.data.enrich.done", { parts: parts.join("，") }), "success");
    } catch (error) {
      deps.showNotification(t("settings.workshop.data.enrich.failure", { error }), "error");
    } finally {
      button.disabled = false;
      label.textContent = original;
    }
   });

  document.getElementById("settings-manual-check-btn")?.addEventListener("click", async () => {
    const btn = document.getElementById("settings-manual-check-btn");
    if (!btn || btn.disabled) return;
    btn.disabled = true;
    btn.innerHTML = `<span class="btn-spinner"></span> ${t("settings.workshop.data.updateCheck.checking")}`;
    btn.style.pointerEvents = "none";
    try {
      const result = await deps.CheckModUpdates();
      const config = deps.getConfig();
      config.lastUpdateCheckTime = String(Date.now());
      await deps.saveConfig(config);
      const count = result?.total_updates || 0;
      deps.showNotification(
        count > 0
          ? t("settings.workshop.data.updateCheck.checkDone", { count })
          : t("settings.workshop.data.updateCheck.checkClean"),
        count > 0 ? "info" : "success",
      );
      await deps.refreshFilesKeepFilter();
    } catch (err) {
      deps.showNotification(t("settings.workshop.data.updateCheck.checkFailed", { error: err }), "error");
    } finally {
      // 无论检测、保存时间戳还是刷新失败，都必须恢复按钮，避免窗口永久卡在“检测中”。
      btn.disabled = false;
      const checkIcon = '<svg class="trigger-check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z"/><path d="M12 6v6l4 2"/></svg>';
      btn.innerHTML = checkIcon + `<span class="trigger-check-text">${t("settings.workshop.data.updateCheck.trigger")}</span>`;
      btn.style.pointerEvents = "";
    }
  });

  document.querySelectorAll('input[name="settings-browser-target"]').forEach((radio) => {
    radio.addEventListener("change", async () => {
      if (!radio.checked) return;
      const previousTarget = deps.getConfig().workshopBrowserTarget || "mirror";
      radio.disabled = true;
      try {
        await deps.SetWorkshopBrowserTarget(radio.value);
        const config = deps.getConfig();
        config.workshopBrowserTarget = radio.value;
        await deps.saveConfig(config);
        deps.showNotification(
        radio.value === "mirror"
          ? t("settings.workshop.browser.switchedMirror")
          : t("settings.workshop.browser.switchedSteam"),
        "success",
      );
      } catch (error) {
        const previousRadio = document.querySelector(`input[name="settings-browser-target"][value="${previousTarget}"]`);
        if (previousRadio) previousRadio.checked = true;
        deps.showNotification(t("settings.workshop.browser.switchFailed", { error }), "error");
      } finally {
        radio.disabled = false;
      }
    });
  });

  const customSection = document.getElementById("settings-translate-custom-section");

  document.querySelectorAll('input[name="settings-translate-provider"]').forEach((radio) => {
    radio.addEventListener("change", async () => {
      if (!radio.checked) return;
      const previousProvider = deps.getConfig().workshopTranslateProvider || "microsoft";
      radio.disabled = true;
      try {
        await deps.SetWorkshopTranslateProvider(radio.value);
        const config = deps.getConfig();
        config.workshopTranslateProvider = radio.value;
        await deps.saveConfig(config);
        radio.closest(".mode-toggle-group")?.querySelectorAll(".mode-option").forEach((option) => option.classList.remove("active"));
        radio.closest(".mode-option")?.classList.add("active");
        if (customSection) {
          customSection.style.display = radio.value === "custom" ? "" : "none";
        }
        const messages = {
          microsoft: t("settings.workshop.translate.switched.microsoft"),
          yandex: t("settings.workshop.translate.switched.yandex"),
          custom: t("settings.workshop.translate.switched.custom"),
        };
        deps.showNotification(messages[radio.value] || t("settings.workshop.translate.switched.default"), "success");
      } catch (error) {
        const previousRadio = document.querySelector(`input[name="settings-translate-provider"][value="${previousProvider}"]`);
        if (previousRadio) {
          previousRadio.checked = true;
          radio.closest(".mode-toggle-group")?.querySelectorAll(".mode-option").forEach((option) => option.classList.remove("active"));
          previousRadio.closest(".mode-option")?.classList.add("active");
        }
        if (customSection) customSection.style.display = previousProvider === "custom" ? "" : "none";
        deps.showNotification(t("settings.workshop.translate.switchFailed", { error }), "error");
      } finally {
        radio.disabled = false;
      }
    });
  });

  // 自定义AI配置输入框事件
  const customBaseURLInput = document.getElementById("settings-translate-custom-baseurl");
  const customAPIKeyInput = document.getElementById("settings-translate-custom-apikey");
  const customModelIdInput = document.getElementById("settings-translate-custom-modelid");

  customBaseURLInput?.addEventListener("change", async () => {
    const url = customBaseURLInput.value.trim();
    const previousURL = deps.getConfig().workshopTranslateCustomBaseURL || "";
    customBaseURLInput.disabled = true;
    try {
      await deps.SetWorkshopTranslateCustomBaseURL(url);
      const config = deps.getConfig();
      config.workshopTranslateCustomBaseURL = url;
      await deps.saveConfig(config);
      deps.showNotification(t("settings.workshop.translate.custom.baseUrlUpdated"), "success");
    } catch (error) {
      customBaseURLInput.value = previousURL;
      deps.showNotification(t("settings.workshop.translate.custom.baseUrlFailed", { error }), "error");
    } finally {
      customBaseURLInput.disabled = false;
    }
  });

  customAPIKeyInput?.addEventListener("change", async () => {
    const key = customAPIKeyInput.value.trim();
    if (!key) return;
    customAPIKeyInput.disabled = true;
    try {
      await deps.SetWorkshopTranslateCustomAPIKey(key);
      const config = deps.getConfig();
      config.workshopTranslateCustomAPIKey = "已设置";
      await deps.saveConfig(config);
      customAPIKeyInput.value = "";
      // 直接赋值，避免用户重复更新密钥时把“（已设置）”叠加多次。
      customAPIKeyInput.setAttribute("placeholder", t("settings.workshop.translate.custom.apiKeySetPlaceholder"));
      deps.showNotification(t("settings.workshop.translate.custom.apiKeyUpdated"), "success");
    } catch (error) {
      deps.showNotification(t("settings.workshop.translate.custom.apiKeyFailed", { error }), "error");
    } finally {
      customAPIKeyInput.disabled = false;
    }
  });

  customModelIdInput?.addEventListener("change", async () => {
    const modelId = customModelIdInput.value.trim();
    const previousModelID = deps.getConfig().workshopTranslateCustomModelId || "";
    customModelIdInput.disabled = true;
    try {
      await deps.SetWorkshopTranslateCustomModelId(modelId);
      const config = deps.getConfig();
      config.workshopTranslateCustomModelId = modelId;
      await deps.saveConfig(config);
      deps.showNotification(t("settings.workshop.translate.custom.modelUpdated"), "success");
    } catch (error) {
      customModelIdInput.value = previousModelID;
      deps.showNotification(t("settings.workshop.translate.custom.modelFailed", { error }), "error");
    } finally {
      customModelIdInput.disabled = false;
    }
  });

  const autoexecEditor = document.getElementById("settings-autoexec-content");
  const autoexecMeta = document.getElementById("settings-autoexec-meta");
  const autoexecLineNumbers = document.getElementById("settings-autoexec-line-numbers");
  const autoexecAnalysis = document.getElementById("settings-autoexec-analysis");
  const autoexecMatchesEl = document.getElementById("settings-autoexec-matches");
  const autoexecLineNumberEditor = attachLineNumberGutter(autoexecEditor, autoexecLineNumbers);
  const autoexecController = createAutoexecSettingsController(deps.autoexecInfo, {
    getConfig: deps.GetAutoexecConfig,
    saveConfig: deps.SaveAutoexecConfig,
  });
  let autoexecAnalysisRequest = 0;
  let autoexecAnalysisTimer = null;
  const updateAutoexecMeta = (info) => {
    if (!autoexecMeta) return;
    const data = info || {};
    autoexecMeta.innerHTML = `
      <span class="autoexec-path">${escapeHtml(data.path || t("settings.addonlist.dirNotChosen"))}</span>
      <span>${data.exists ? t("settings.addonlist.fileExists") : t("settings.addonlist.fileMissing")}</span>
      <span>${escapeHtml(data.encoding || "UTF-8")} · ${escapeHtml(data.lineEnding || "CRLF")}</span>
      <span>${formatAddonListBytes(data.size)}</span>
      ${data.lastModified ? `<span>${t("settings.addonlist.modifiedAt", { time: escapeHtml(formatAddonListTime(data.lastModified)) })}</span>` : ""}
    `;
  };
  const updateAutoexecAnalysis = async () => {
    if (!autoexecEditor?.isConnected || typeof deps.AnalyzeAutoexecCommands !== "function") return;
    const requestId = ++autoexecAnalysisRequest;
    try {
      const matches = await deps.AnalyzeAutoexecCommands(autoexecEditor.value);
      if (requestId !== autoexecAnalysisRequest || !autoexecEditor.isConnected) return;
      const known = matches.filter((item) => item.known).length;
      const unknown = matches.length - known;
      const highRisk = matches.filter((item) => item.known && item.help?.risk?.startsWith("高")).length;
      const mediumRisk = matches.filter((item) => item.known && item.help?.risk?.startsWith("中")).length;
      if (autoexecAnalysis) {
        const risks =
          (highRisk ? t("settings.addonlist.autoexec.riskHigh", { count: highRisk }) : "") +
          (mediumRisk ? t("settings.addonlist.autoexec.riskMedium", { count: mediumRisk }) : "");
        autoexecAnalysis.textContent = t("settings.addonlist.autoexec.analysisSummary", { known, unknown, risks });
        autoexecAnalysis.classList.toggle("is-warning", unknown > 0 || highRisk > 0);
      }
      renderAutoexecMatches(autoexecMatchesEl, matches);
    } catch (error) {
      if (requestId !== autoexecAnalysisRequest) return;
      if (autoexecAnalysis)
        autoexecAnalysis.textContent = t("settings.addonlist.autoexec.analysisFailed", {
          error: String(error?.message || error),
        });
    }
  };
  const scheduleAutoexecAnalysis = () => {
    if (autoexecAnalysisTimer) window.clearTimeout(autoexecAnalysisTimer);
    autoexecAnalysisTimer = window.setTimeout(() => {
      autoexecAnalysisTimer = null;
      void updateAutoexecAnalysis();
    }, 180);
  };
  autoexecEditor?.addEventListener("input", scheduleAutoexecAnalysis);
  void updateAutoexecAnalysis();
  const applyAutoexecInfo = (info) => {
    if (!info || !autoexecEditor?.isConnected) return;
    autoexecEditor.value = info.content || "";
    updateAutoexecMeta(info);
    const openButton = document.getElementById("settings-autoexec-open");
    if (openButton) openButton.disabled = !info.exists;
    autoexecLineNumberEditor.refresh();
    void updateAutoexecAnalysis();
  };
  const reloadAutoexecEditor = async () => {
    if (!autoexecEditor?.isConnected) return;
    const result = await autoexecController.reload();
    if (result.stale || !autoexecEditor.isConnected) return;
    applyAutoexecInfo(result.info);
  };
  document.getElementById("settings-autoexec-save")?.addEventListener("click", async () => {
    if (!autoexecEditor?.isConnected) return;
    const button = document.getElementById("settings-autoexec-save");
    if (button) button.disabled = true;
    try {
      const result = await autoexecController.save(autoexecEditor.value);
      if (result.reloadError) {
        deps.showNotification(t("settings.addonlist.autoexec.savedReloadFailed", { error: result.reloadError }), "warning");
      } else {
        applyAutoexecInfo(result.info);
        deps.showNotification(t("settings.addonlist.autoexec.saved"), "success");
      }
    } catch (error) {
      deps.showNotification(t("settings.addonlist.autoexec.saveFailed", { error }), "error");
    } finally {
      if (button) button.disabled = false;
    }
  });
  document.getElementById("settings-autoexec-reload")?.addEventListener("click", async (event) => {
    const button = event.currentTarget;
    if (button.disabled) return;
    button.disabled = true;
    try {
      await reloadAutoexecEditor();
      deps.showNotification(t("settings.addonlist.autoexec.reloaded"), "info");
    } catch (error) {
      deps.showNotification(t("settings.addonlist.autoexec.reloadFailed", { error }), "error");
    } finally {
      button.disabled = false;
    }
  });
  document.getElementById("settings-autoexec-open")?.addEventListener("click", async () => {
    const info = autoexecController.getInfo();
    if (!info?.path || typeof deps.OpenFileLocation !== "function") return;
    try {
      await deps.OpenFileLocation(info.path);
    } catch (error) {
      deps.showNotification(t("settings.addonlist.autoexec.openFailed", { error }), "error");
    }
  });
  const autoexecHelpSearch = document.getElementById("settings-autoexec-help-search");
  const autoexecHelpList = document.getElementById("settings-autoexec-help-list");
  let autoexecHelpSearchRequest = 0;
  let autoexecHelpSearchTimer = null;
  const renderHelpList = (items) => {
    if (autoexecHelpList) renderAutoexecHelpList(autoexecHelpList, items);
  };
  const searchAutoexecHelp = async () => {
    if (typeof deps.GetAutoexecCommandHelp !== "function") return;
    if (!autoexecHelpSearch?.isConnected) return;
    const requestId = ++autoexecHelpSearchRequest;
    try {
      const items = await deps.GetAutoexecCommandHelp(autoexecHelpSearch.value);
      if (requestId !== autoexecHelpSearchRequest || !autoexecHelpSearch.isConnected) return;
      renderHelpList(items);
    } catch (error) {
      if (requestId !== autoexecHelpSearchRequest) return;
      deps.showNotification(t("settings.addonlist.autoexec.searchFailed", { error }), "error");
    }
  };
  autoexecHelpSearch?.addEventListener("input", () => {
    if (autoexecHelpSearchTimer) window.clearTimeout(autoexecHelpSearchTimer);
    autoexecHelpSearchTimer = window.setTimeout(() => {
      autoexecHelpSearchTimer = null;
      void searchAutoexecHelp();
    }, 160);
  });
  autoexecHelpList?.addEventListener("click", (event) => {
    const button = event.target.closest("[data-autoexec-command]");
    if (!button || !autoexecEditor) return;
    const command = button.dataset.autoexecCommand;
    if (!command) return;
    const start = autoexecEditor.selectionStart ?? autoexecEditor.value.length;
    const end = autoexecEditor.selectionEnd ?? start;
    autoexecEditor.setRangeText(`${command} `, start, end, "end");
    autoexecEditor.focus();
    autoexecLineNumberEditor.refresh();
    updateAutoexecAnalysis();
  });
  // 常用指令说明（真机 193 条）：不进模板、也不在页面打开时渲染 —— 等「游戏配置」
  // 分区第一次可见时再分片补（首屏 40 条同步，其余按帧补），这样分区首次布局更小。
  autoexecHelpItems = deps.autoexecHelp || [];
  autoexecHelpListRendered = false;
  if (document.getElementById("settings-panel-addonlist")?.classList.contains("active")) {
    renderAutoexecHelpListIfNeeded();
  }

  const refreshAddonListPanel = async () => {
    await deps.refreshAddonListPanel?.();
  };
  const refreshAddonListFiles = async () => {
    await deps.refreshFilesKeepFilter?.();
  };
  const runAddonListAction = async (button, action, successMessage, errorPrefix, refresh = true) => {
    if (!button || button.disabled) return;
    button.disabled = true;
    try {
      await action();
      if (successMessage) deps.showNotification(successMessage, "success");
      if (refresh) await refreshAddonListPanel();
    } catch (error) {
      deps.showNotification(`${errorPrefix}: ${error}`, "error");
    } finally {
      button.disabled = false;
    }
  };

  document.getElementById("settings-addonlist-save-snapshot")?.addEventListener("click", (event) => {
    void runAddonListAction(event.currentTarget, () => deps.SaveAddonListManagedSnapshot(), t("settings.addonlist.actions.snapshotSaved"), t("settings.addonlist.actions.snapshotFailed"));
  });

  document.getElementById("settings-addonlist-create-backup")?.addEventListener("click", (event) => {
    void runAddonListAction(event.currentTarget, () => deps.CreateAddonListBackup(), t("settings.addonlist.actions.backupCreated"), t("settings.addonlist.actions.backupFailed"));
  });

  // 套件继承快照的手动重置入口（用户口径：唯一会"少继承"的操作，所以必须显式确认）。
  // 重置后立刻重扫一次，否则当前列表还是旧快照算出来的标签。
  document.getElementById("settings-reset-suite-inheritance")?.addEventListener("click", async (event) => {
    const button = event.currentTarget;
    if (button.disabled) return;
    const confirmed = await confirmInApp(
      t("settings.addonlist.maintenance.suite.resetConfirm"),
      { title: t("settings.addonlist.maintenance.suite.resetConfirmTitle") },
    );
    if (!confirmed) return;

    const status = document.getElementById("settings-suite-inheritance-status");
    button.disabled = true;
    if (status) status.textContent = t("settings.addonlist.maintenance.suite.resetting");
    try {
      const namespaces = await deps.ResetSuiteInheritanceSnapshot();
      await deps.ScanVPKFiles();
      await deps.refreshFilesKeepFilter?.({ silent: true });
      const detail = namespaces > 0
        ? t("settings.addonlist.maintenance.suite.resetDetail", { count: namespaces })
        : "";
      if (status) status.textContent = t("settings.addonlist.maintenance.suite.resetDone", { detail });
      deps.showNotification(t("settings.addonlist.maintenance.suite.resetDoneNotify", { detail }), "success");
    } catch (error) {
      if (status) status.textContent = "";
      deps.showNotification(t("settings.addonlist.maintenance.suite.resetFailed", { error }), "error");
    } finally {
      button.disabled = false;
    }
  });

  document.getElementById("settings-addonlist-open")?.addEventListener("click", async () => {
    const path = document.querySelector("#settings-panel-addonlist .addonlist-path")?.textContent?.trim();
    if (!path || typeof deps.OpenFileLocation !== "function") return;
    try {
      await deps.OpenFileLocation(path);
    } catch (error) {
      deps.showNotification(t("settings.addonlist.actions.openFailed", { error }), "error");
    }
  });

  document.getElementById("settings-unrecorded-mod-load-order-placement")?.addEventListener("change", async (event) => {
    const select = event.currentTarget;
    const placement = ["start", "after-enabled", "end"].includes(select.value) ? select.value : "end";
    const previousPlacement = deps.getConfig().unrecordedModLoadOrderPlacement || "end";
    select.disabled = true;
    try {
      const config = deps.getConfig();
      config.unrecordedModLoadOrderPlacement = placement;
      await deps.saveConfig(config);
      deps.showNotification(t("settings.addonlist.unrecordedPlacement.updated"), "success");
    } catch (error) {
      select.value = previousPlacement;
      deps.showNotification(t("settings.addonlist.unrecordedPlacement.failed", { error }), "error");
    } finally {
      select.disabled = false;
    }
  });

  document.getElementById("settings-addonlist-guard")?.addEventListener("change", async (event) => {
    const toggle = event.currentTarget;
    const enabled = toggle.checked;
    toggle.disabled = true;
    try {
      await deps.SetAddonListGuardEnabled(enabled);
      // SetAddonListGuardEnabled 会立即持久化后端状态；同时更新前端缓存，
      // 防止用户随后修改其他设置时，把旧的监控开关快照再次提交。
      const config = deps.getConfig();
      config.addonListGuardEnabled = enabled;
      await deps.saveConfig(config);
      deps.showNotification(
        enabled ? t("settings.addonlist.guard.on") : t("settings.addonlist.guard.off"),
        enabled ? "success" : "info",
      );
      await refreshAddonListPanel();
    } catch (error) {
      toggle.checked = !enabled;
      deps.showNotification(t("settings.addonlist.guard.failed", { error }), "error");
    } finally {
      toggle.disabled = false;
    }
  });

  document.querySelectorAll(".addonlist-backup-restore").forEach((button) => {
    button.addEventListener("click", async () => {
      const name = button.dataset.addonlistBackup;
      if (
        !name ||
        !(await confirmInApp(t("settings.addonlist.backups.restoreConfirm"), {
          title: t("settings.addonlist.backups.restoreConfirmTitle"),
        }))
      ) {
        return;
      }
      await runAddonListAction(button, async () => {
        await deps.RestoreAddonListBackup(name);
        await refreshAddonListFiles();
      }, t("settings.addonlist.backups.restored"), t("settings.addonlist.backups.restoreFailed"));
    });
  });

  document.querySelectorAll(".addonlist-backup-delete").forEach((button) => {
    button.addEventListener("click", async () => {
      const name = button.dataset.addonlistBackup;
      if (
        !name ||
        !(await confirmInApp(t("settings.addonlist.backups.deleteConfirm"), {
          title: t("settings.addonlist.backups.deleteConfirmTitle"),
        }))
      ) {
        return;
      }
      await runAddonListAction(button, () => deps.DeleteAddonListBackup(name), t("settings.addonlist.backups.deleted"), t("settings.addonlist.backups.deleteFailed"));
    });
  });

  document.getElementById("settings-addonlist-delete")?.addEventListener("click", async (event) => {
    if (
      !(await confirmInApp(
        t("settings.addonlist.delete.confirm"),
        { title: t("settings.addonlist.delete.confirmTitle") },
      ))
    ) {
      return;
    }
    await runAddonListAction(event.currentTarget, async () => {
      await deps.DeleteAddonList();
      await refreshAddonListFiles();
    }, t("settings.addonlist.delete.done"), t("settings.addonlist.delete.failed"));
  });

  document.getElementById("settings-addonlist-select-merge-source")?.addEventListener("click", async (event) => {
    await runAddonListAction(event.currentTarget, async () => {
      const sourcePath = await deps.SelectAddonListMergeSource();
      if (!sourcePath) return;
      addonListMergePreview = await deps.PreviewAddonListMerge(sourcePath);
      deps.showNotification(t("settings.addonlist.merge.readDone"), "info");
      await refreshAddonListPanel();
    }, null, t("settings.addonlist.merge.readFailed"), false);
  });

  document.getElementById("settings-addonlist-cancel-merge")?.addEventListener("click", async () => {
    addonListMergePreview = null;
    await refreshAddonListPanel();
  });

  document.getElementById("settings-addonlist-apply-merge")?.addEventListener("click", async (event) => {
    if (!addonListMergePreview) return;
    const sourceWinsKeys = Array.from(document.querySelectorAll(".addonlist-merge-conflict input:checked"))
      .map((input) => input.dataset.addonlistMergeKey)
      .filter(Boolean);
    await runAddonListAction(event.currentTarget, async () => {
      await deps.ApplyAddonListMerge(addonListMergePreview.sourcePath, sourceWinsKeys);
      addonListMergePreview = null;
      await refreshAddonListFiles();
    }, t("settings.addonlist.merge.applied"), t("settings.addonlist.merge.applyFailed"));
  });

}

function renderAddonListMergePreview(preview) {
  if (!preview) {
    return `<div class="setting-row-desc addonlist-merge-empty">${t("settings.addonlist.merge.empty")}</div>`;
  }
  const added = Array.isArray(preview.added) ? preview.added : [];
  const conflicts = Array.isArray(preview.conflicts) ? preview.conflicts : [];
  return `
    <div class="addonlist-merge-preview">
      <div class="addonlist-merge-source">${t("settings.addonlist.merge.source", { value: escapeHtml(preview.sourcePath || "") })}</div>
      <div class="addonlist-merge-summary">
        <span>${t("settings.addonlist.merge.added", { count: added.length })}</span>
        <span>${t("settings.addonlist.merge.conflicts", { count: conflicts.length })}</span>
      </div>
      ${conflicts.length > 0 ? `
        <div class="addonlist-merge-conflicts">
          ${conflicts.map((conflict) => `
            <label class="addonlist-merge-conflict">
              <input type="checkbox" data-addonlist-merge-key="${escapeAttr(conflict.key || "")}">
              <span class="addonlist-merge-conflict-name">${escapeHtml(conflict.key || "")}</span>
              <span class="addonlist-merge-state">${t("settings.addonlist.merge.currentState", {
                state: conflict.currentEnabled ? t("settings.addonlist.merge.stateOn") : t("settings.addonlist.merge.stateOff"),
              })}</span>
              <span class="addonlist-merge-state">${t("settings.addonlist.merge.source", {
                value: conflict.sourceEnabled ? t("settings.addonlist.merge.stateOn") : t("settings.addonlist.merge.stateOff"),
              })}</span>
              <span class="addonlist-merge-adopt">${t("settings.addonlist.merge.adoptSource")}</span>
            </label>
          `).join("")}
        </div>
      ` : ""}
      <button type="button" id="settings-addonlist-apply-merge" class="trigger-check-btn addonlist-action-btn" ${added.length || conflicts.length ? "" : "disabled"}>${t("settings.addonlist.merge.apply")}</button>
    </div>
  `;
}

function renderIPOptionDropdown({
  options,
  fixedIP,
  trigger,
  menu,
  fixedInput,
  SetWorkshopFixedIP,
  getConfig,
  saveConfig,
  showNotification,
  onStatusUpdate,
}) {
  if (!trigger || !menu || !fixedInput) return;

  const normalizedOptions = normalizeIPOptions(options);
  trigger.classList.remove("is-disabled");
  trigger.disabled = false;
  updateIPOptionTrigger(trigger, normalizedOptions, fixedIP);
  menu.textContent = "";

  if (normalizedOptions.length === 0) {
    trigger.classList.add("is-disabled");
    trigger.disabled = true;
    return;
  }

  normalizedOptions.forEach((option) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "select-option settings-ip-option";
    button.dataset.value = option.ip;

    const content = document.createElement("span");
    content.className = "settings-ip-option-main";

    const category = document.createElement("span");
    category.className = "settings-ip-option-category";
    category.textContent = option.category;

    const ip = document.createElement("span");
    ip.className = "settings-ip-option-address";
    ip.textContent = option.ip;

    content.appendChild(category);
    content.appendChild(ip);
    button.appendChild(content);

    if (option.ip === fixedIP) {
      button.classList.add("active");
    }

    button.addEventListener("click", async () => {
      const previousFixedIP = getConfig().workshopFixedIP || "";
      const optionButtons = Array.from(menu.querySelectorAll(".settings-ip-option"));
      optionButtons.forEach((item) => {
        item.disabled = true;
      });
      trigger.disabled = true;
      try {
        await SetWorkshopFixedIP(option.ip);
        const config = getConfig();
        config.workshopFixedIP = option.ip;
        await saveConfig(config);
        fixedInput.value = option.ip;
        updateIPOptionTrigger(trigger, normalizedOptions, option.ip);
        syncIPOptionMenuActive(menu, option.ip);
        await onStatusUpdate?.();
        menu.classList.add("hidden");
        showNotification(t("settings.network.ip.fixedUpdated"), "success");
      } catch (error) {
        fixedInput.value = previousFixedIP;
        updateIPOptionTrigger(trigger, normalizedOptions, previousFixedIP);
        syncIPOptionMenuActive(menu, previousFixedIP);
        showNotification(t("settings.network.ip.fixedFailed", { error }), "error");
      } finally {
        optionButtons.forEach((item) => {
          item.disabled = false;
        });
        trigger.disabled = false;
      }
    });

    menu.appendChild(button);
  });

  trigger.onclick = (event) => {
    event.stopPropagation();
    menu.classList.toggle("hidden");
  };
}

async function loadIPOptionsForDropdown({
  GetWorkshopIPOptions,
  fixedIP,
  trigger,
  menu,
  fixedInput,
  setOptions,
  SetWorkshopFixedIP,
  getConfig,
  saveConfig,
  showNotification,
  onStatusUpdate,
}) {
  if (!trigger || !menu || typeof GetWorkshopIPOptions !== "function") return;

  trigger.textContent = t("settings.network.ip.loadingOptions");
  trigger.classList.add("is-disabled");
  trigger.disabled = true;
  menu.textContent = "";

  try {
    const nextOptions = normalizeIPOptions(await GetWorkshopIPOptions());
    setOptions(nextOptions);
    renderIPOptionDropdown({
      options: nextOptions,
      fixedIP,
      trigger,
      menu,
      fixedInput,
      SetWorkshopFixedIP,
      getConfig,
      saveConfig,
      showNotification,
      onStatusUpdate,
    });
    await onStatusUpdate?.();
  } catch (error) {
    trigger.textContent = t("settings.network.ip.optionsFailed");
    trigger.classList.add("is-disabled");
    trigger.disabled = true;
  }
}

function normalizeIPOptions(options = []) {
  const seen = new Set();
  const normalized = [];
  options.forEach((option) => {
    const ip = getIPOptionIP(option);
    if (!ip || seen.has(ip)) return;
    const category = getIPOptionCategory(option);
    normalized.push({ ip, category });
    seen.add(ip);
  });
  return normalized;
}

function updateIPOptionTrigger(trigger, options, ip) {
  if (!trigger) return;
  const option = findIPOption(options, ip);
  if (option) {
    trigger.textContent = formatIPOptionLabel(option);
  } else if (ip) {
    trigger.textContent = formatIPOptionLabel({ ip, category: t("settings.network.ip.customCategory") });
  } else if (options?.length) {
    trigger.textContent = t("settings.network.ip.triggerPrompt");
  } else {
    trigger.textContent = t("settings.network.ip.triggerEmpty");
  }
}

function syncIPOptionMenuActive(menu, ip) {
  menu?.querySelectorAll(".settings-ip-option").forEach((button) => {
    button.classList.toggle("active", button.dataset.value === ip);
  });
}

async function updateIPStatus({
  statusEl,
  ipOptions,
  getUseFixedIP,
  IsSelectingIP,
  GetCurrentBestIP,
  GetCurrentBestIPOption,
}) {
  if (!statusEl) return;

  const isSelecting = await IsSelectingIP();
  if (isSelecting) {
    statusEl.textContent = t("settings.network.ip.selecting");
    return;
  }

  const bestIPOption = await GetCurrentBestIPOption();
  const bestIP = getIPOptionIP(bestIPOption) || (await GetCurrentBestIP());
  if (!bestIP) {
    statusEl.textContent = t("settings.network.ip.notReady");
    return;
  }

  const useFixedIP = getUseFixedIP?.() || false;
  const option = bestIPOption ||
    findIPOption(ipOptions, bestIP) || { ip: bestIP, category: useFixedIP ? t("settings.network.ip.customCategory") : "" };
  statusEl.textContent = useFixedIP
    ? t("settings.network.ip.statusFixed", { label: formatIPOptionLabel(option) })
    : t("settings.network.ip.statusAuto", { label: formatIPOptionLabel(option) });
}

function findIPOption(options = [], ip) {
  if (!ip) return null;
  return options.find((option) => option.ip === ip) || null;
}

function formatIPOptionLabel(option) {
  const ip = getIPOptionIP(option);
  if (!ip) return "";
  const category = getIPOptionCategory(option, "");
  return category ? `${category} / ${ip}` : ip;
}

function getIPOptionIP(option) {
  return String(option?.ip || option?.IP || "").trim();
}

function getIPOptionCategory(option, fallback = t("settings.network.ip.uncategorized")) {
  const rawCategory = option?.category ?? option?.Category;
  const category = String(rawCategory ?? fallback).trim();
  return category || fallback;
}

function enhanceSettingsNav() {
  const sidebar = document.querySelector("#settings-page-content .settings-sidebar");
  if (!sidebar) return;

  if (!sidebar.querySelector(".settings-active-indicator")) {
    sidebar.insertAdjacentHTML("afterbegin", `<div class="settings-active-indicator" aria-hidden="true"></div>`);
  }

  sidebar.querySelectorAll(".settings-nav-item").forEach((item) => {
    const panel = item.dataset.panel;
    if (!panel || item.querySelector(".settings-nav-icon")) return;
    item.insertAdjacentHTML(
      "afterbegin",
      `<span class="settings-nav-icon">${SETTINGS_NAV_ICONS[panel] || SETTINGS_NAV_ICONS.interface}</span>`
    );
  });

  updateSettingsNavIndicator(true);
  requestAnimationFrame(() => updateSettingsNavIndicator(true));
}

function updateSettingsNavIndicator(skipTransition = false) {
  const sidebar = document.querySelector("#settings-page-content .settings-sidebar");
  const indicator = sidebar?.querySelector(".settings-active-indicator");
  const activeItem = sidebar?.querySelector(".settings-nav-item.active");
  if (!sidebar || !indicator || !activeItem) return;

  if (skipTransition) {
    indicator.style.transition = "none";
  }

  const sidebarRect = sidebar.getBoundingClientRect();
  const itemRect = activeItem.getBoundingClientRect();

  if (window.matchMedia("(max-width: 640px)").matches) {
    indicator.style.width = `${itemRect.width}px`;
    indicator.style.height = `${itemRect.height}px`;
    indicator.style.transform = `translate(${itemRect.left - sidebarRect.left + sidebar.scrollLeft}px, ${itemRect.top - sidebarRect.top}px)`;
  } else {
    indicator.style.width = "";
    indicator.style.height = `${itemRect.height}px`;
    indicator.style.transform = `translateY(${itemRect.top - sidebarRect.top + sidebar.scrollTop}px)`;
  }

  if (skipTransition) {
    indicator.offsetHeight;
    indicator.style.transition = "";
  }
}

window.addEventListener("resize", () => {
  updateSettingsNavIndicator(true);
});

function updateSettingsPanelDirection(nextItem) {
  const content = document.querySelector("#settings-page-content .settings-content");
  const activeItem = document.querySelector("#settings-page-content .settings-nav-item.active");
  if (!content || !activeItem || !nextItem || activeItem === nextItem) return;
  const items = Array.from(document.querySelectorAll("#settings-page-content .settings-nav-item"));
  content.dataset.settingsDirection = items.indexOf(nextItem) >= items.indexOf(activeItem) ? "down" : "up";
}

function escapeAttr(value) {
  return String(value || "").replace(/"/g, "&quot;");
}

// ── 常用指令说明列表：分片渲染 ─────────────────────────────────────────────
// 真机 193 条：原来整段塞进设置页模板的 HTML 字符串，插入时是"一次 parse + 一次布局"，
// 首开「游戏配置」分区要为此付一次 100ms 级停顿。现在首屏 40 条同步、其余按 6ms
// 时间片补齐（与仓库其它长列表同一套做法）；搜索重填走同一个入口，token 作废旧一轮。
const AUTOEXEC_HELP_FIRST_CHUNK = 30;
const AUTOEXEC_HELP_CHUNK = 40;
const AUTOEXEC_HELP_BUDGET_MS = 5;
let autoexecHelpRenderToken = 0;
// 「游戏配置」分区第一次可见前不渲染帮助列表（见分区点击处理里的调用）。
let autoexecHelpItems = [];
let autoexecHelpListRendered = false;

function renderAutoexecHelpListIfNeeded() {
  if (autoexecHelpListRendered) return;
  autoexecHelpListRendered = true;
  const run = () => {
    const container = document.getElementById("settings-autoexec-help-list");
    if (container) renderAutoexecHelpList(container, autoexecHelpItems);
  };
  // 延后两帧再开始填充：分区"第一次布局/首帧"里不带这几百个节点，
  // 点分区的这一下（真机实测 66ms 长任务出在点击任务里）就不用为帮助列表买单。
  if (typeof requestAnimationFrame === "function") {
    requestAnimationFrame(() => requestAnimationFrame(run));
    return;
  }
  setTimeout(run, 32);
}

function autoexecHelpNow() {
  return typeof performance !== "undefined" && performance.now ? performance.now() : Date.now();
}

function scheduleAutoexecHelpChunk(step) {
  let done = false;
  const run = () => {
    if (done) return;
    done = true;
    step();
  };
  if (typeof requestAnimationFrame === "function") requestAnimationFrame(run);
  setTimeout(run, 50);
}

function buildAutoexecHelpItem(item) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "autoexec-help-item";
  button.dataset.autoexecCommand = item.command || "";
  button.dataset.autoexecRisk = item.risk || "";
  const command = document.createElement("span");
  command.className = "autoexec-help-command";
  command.textContent = item.command || "";
  const summary = document.createElement("span");
  summary.className = "autoexec-help-summary";
  summary.textContent = item.summary || "";
  const meta = document.createElement("span");
  meta.className = "autoexec-help-meta";
  meta.textContent = [item.scope, item.risk, item.source].filter(Boolean).join(" · ");
  button.append(command, summary, meta);
  return button;
}

function renderAutoexecHelpList(container, items) {
  if (!container) return;
  const token = ++autoexecHelpRenderToken;
  container.replaceChildren();
  const list = Array.isArray(items) ? items : [];
  if (list.length === 0) {
    const empty = document.createElement("div");
    empty.className = "autoexec-help-empty";
    empty.textContent = t("settings.addonlist.autoexec.help.empty");
    container.appendChild(empty);
    return;
  }
  let index = 0;
  const appendChunk = (limit, budgetMs) => {
    if (token !== autoexecHelpRenderToken) return false;
    const started = autoexecHelpNow();
    const end = Math.min(list.length, index + limit);
    let added = 0;
    const fragment = document.createDocumentFragment();
    for (; index < end; index += 1) {
      if (added > 0 && autoexecHelpNow() - started >= budgetMs) break;
      fragment.appendChild(buildAutoexecHelpItem(list[index]));
      added += 1;
    }
    if (added === 0) return index < list.length;
    container.appendChild(fragment);
    return index < list.length;
  };
  if (!appendChunk(AUTOEXEC_HELP_FIRST_CHUNK, Infinity)) return;
  const step = () => {
    if (token !== autoexecHelpRenderToken) return;
    if (appendChunk(AUTOEXEC_HELP_CHUNK, AUTOEXEC_HELP_BUDGET_MS)) scheduleAutoexecHelpChunk(step);
  };
  scheduleAutoexecHelpChunk(step);
}

function renderAutoexecMatches(container, items) {
  if (!container) return;
  container.replaceChildren();
  if (!Array.isArray(items) || items.length === 0) return;
  items.forEach((item) => {
    const row = document.createElement("div");
    row.className = `autoexec-match ${item.known ? "is-known" : "is-unknown"}`;
    const title = document.createElement("div");
    title.className = "autoexec-match-title";
    title.textContent = t("settings.addonlist.autoexec.matchLine", {
      line: item.line,
      command: item.command,
      unknown: item.known ? "" : t("settings.addonlist.autoexec.matchUnknownSuffix"),
    });
    row.appendChild(title);
    const detail = document.createElement("div");
    detail.className = "autoexec-match-detail";
    detail.textContent = item.known && item.help
      ? `${item.help.summary} · ${item.help.scope} · ${item.help.risk} · ${item.help.source}`
      : t("settings.addonlist.autoexec.matchAdvice");
    row.appendChild(detail);
    container.appendChild(row);
  });
}

function escapeHtml(value) {
  return String(value || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function formatAddonListBytes(size) {
  const bytes = Number(size || 0);
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function formatAddonListTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatAddonListBackupKind(kind) {
  const keys = {
    manual: "settings.addonlist.backups.kind.manual",
    external: "settings.addonlist.backups.kind.external",
    "game-save": "settings.addonlist.backups.kind.gameSave",
    before: "settings.addonlist.backups.kind.before",
    "before-profile-apply": "settings.addonlist.backups.kind.beforeProfileApply",
    "before-health-fix": "settings.addonlist.backups.kind.beforeHealthFix",
  };
  const key = keys[String(kind || "")];
  return key ? t(key) : t("settings.addonlist.backups.kind.fallback");
}

function parseAddonListManagerState(payload) {
  const parsed = typeof payload === "string" ? JSON.parse(payload || "{}") : payload || {};
  return {
    info: parsed?.info || null,
    backups: Array.isArray(parsed?.backups) ? parsed.backups : [],
  };
}
