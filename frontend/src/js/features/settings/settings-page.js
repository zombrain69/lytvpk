import { attachLineNumberGutter } from "../../core/line-number-editor.js";
import { applyUIScale, MAX_UI_SCALE, MIN_UI_SCALE, normalizeUIScale, UI_SCALE_STEP } from "../../core/ui-scale.js";

const SETTINGS_NAV_ICONS = {
  network: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 0 20"/><path d="M12 2a15.3 15.3 0 0 0 0 20"/></svg>`,
  interface: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="14" rx="2"/><path d="M8 20h8"/><path d="M12 18v2"/></svg>`,
  workshop: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2a7 7 0 0 1 7 7c0 2.38-1.19 4.47-3 5.74V17a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2v-2.26C6.19 13.47 5 11.38 5 9a7 7 0 0 1 7-7z"/><path d="M9 21h6"/></svg>`,
  addonlist: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M5 3h11l3 3v15H5z"/><path d="M16 3v4h4"/><path d="M8 11h8M8 15h8"/></svg>`,
};

let addonListMergePreview = null;
let filterLayoutChangeToken = 0;

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
    GetAutoexecConfig,
    SaveAutoexecConfig,
    GetAutoexecCommandHelp,
    AnalyzeAutoexecCommands,
    OpenFileLocation,
  } = deps;
  const container = document.getElementById("settings-page-content");
  if (!container) return;
  const config = getConfig();
  // Individual Wails reads must not turn the whole settings page into a blank
  // view. A transient failure (for example while the app is reloading bindings)
  // keeps the relevant control usable with the last persisted frontend value,
  // and makes the degraded source visible to the user.
  const settingsReadErrors = [];
  const readSetting = async (label, reader, fallback) => {
    try {
      if (typeof reader !== "function") {
        throw new Error("当前后端未提供此设置接口");
      }
      return await reader();
    } catch (error) {
      settingsReadErrors.push(`${label}：${String(error?.message || error || "读取失败")}`);
      return fallback;
    }
  };
  const unrecordedModLoadOrderPlacement = ["start", "after-enabled", "end"].includes(config.unrecordedModLoadOrderPlacement)
    ? config.unrecordedModLoadOrderPlacement
    : "end";

  const enabled = await readSetting("优选 IP", GetWorkshopPreferredIP, Boolean(config.workshopPreferredIP));
  const fixedIP = await readSetting("固定 IP", GetWorkshopFixedIP, config.workshopFixedIP || "");
  const useFixedIP = enabled && fixedIP !== "";
  const metaEnabled = await readSetting("工坊信息存储", GetWorkshopMetaEnabled, Boolean(config.workshopMetaEnabled));
  const updateCheckEnabled = await readSetting("Mod 更新检测", GetWorkshopUpdateCheckEnabled, Boolean(config.workshopUpdateCheckEnabled));
  const browserTarget = await readSetting("工坊跳转目标", GetWorkshopBrowserTarget, config.workshopBrowserTarget || "mirror");
  const translateProvider = await readSetting("翻译服务", GetWorkshopTranslateProvider, config.workshopTranslateProvider || "microsoft");
  const customBaseURL = await readSetting("自定义 AI Base URL", GetWorkshopTranslateCustomBaseURL, config.workshopTranslateCustomBaseURL || "");
  const customModelId = await readSetting("自定义 AI 模型", GetWorkshopTranslateCustomModelId, config.workshopTranslateCustomModelId || "");
  const hasCustomAPIKey = await readSetting("自定义 AI 密钥状态", HasWorkshopTranslateCustomAPIKey, Boolean(config.workshopTranslateCustomAPIKey));
  const isSelecting = enabled ? await readSetting("优选 IP 状态", IsSelectingIP, false) : false;
  const ipOptions = [];
  const bestIPOption = enabled && !isSelecting ? await readSetting("当前优选 IP", GetCurrentBestIPOption, null) : null;
  const bestIP = getIPOptionIP(bestIPOption) || (enabled && !isSelecting
    ? await readSetting("当前优选 IP 地址", GetCurrentBestIP, "")
    : "");
  let addonListInfo = null;
  let addonListBackups = [];
  let addonListError = "";
  try {
    const addonListManagerState = parseAddonListManagerState(await GetAddonListManagerState());
    addonListInfo = addonListManagerState.info;
    addonListBackups = addonListManagerState.backups;
  } catch (error) {
    addonListError = String(error?.message || error || "无法读取 addonlist.txt 状态");
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
    autoexecError = String(error?.message || error || "无法读取 autoexec.cfg");
  }
  if (addonListMergePreview && addonListInfo?.path && addonListMergePreview.targetPath?.toLowerCase() !== addonListInfo.path.toLowerCase()) {
    addonListMergePreview = null;
  }

  let ipStatusText = "";
  if (enabled) {
    if (isSelecting) {
      ipStatusText = "正在优选最佳线路...";
    } else if (bestIP) {
      const optionLabel = formatIPOptionLabel(bestIPOption || { ip: bestIP, category: useFixedIP ? "自定义" : "" });
      ipStatusText = useFixedIP ? `当前固定 IP: ${optionLabel}` : `当前优选 IP: ${optionLabel}`;
    } else {
      ipStatusText = "尚未获取到优选 IP";
    }
  }

  container.innerHTML = `
    ${settingsReadErrors.length ? `
      <div class="settings-read-warning" role="status">
        <strong>部分设置状态暂时无法从后端读取，已显示上次保存的值：</strong>
        <span>${escapeHtml(settingsReadErrors.join("；"))}</span>
      </div>
    ` : ""}
    <div class="settings-layout embedded-settings">
      <div class="settings-sidebar">
        <button class="settings-nav-item active" data-panel="network">网络设置</button>
        <button class="settings-nav-item" data-panel="interface">界面设置</button>
        <button class="settings-nav-item" data-panel="workshop">工坊设置</button>
        <button class="settings-nav-item" data-panel="addonlist">游戏配置</button>
      </div>
      <div class="settings-content">
        <div class="settings-panels-track">
        <div class="settings-panel active" id="settings-panel-network">
          <div class="setting-card">
            <div class="setting-card-title">网络加速</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">开启优选 IP 加速</div>
                <div class="setting-row-desc">加速创意工坊图片与文件下载</div>
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
              <div class="setting-row-label">加速模式</div>
              <div class="setting-radio-group">
                <label class="setting-radio-label"><input type="radio" name="settings-ip-mode" value="auto" ${useFixedIP ? "" : "checked"}><span>自动优选最佳 IP（推荐）</span></label>
                <label class="setting-radio-label"><input type="radio" name="settings-ip-mode" value="fixed" ${useFixedIP ? "checked" : ""}><span>手动指定 IP</span></label>
              </div>
              <div id="settings-fixed-ip-tools" class="settings-fixed-ip-tools" style="${useFixedIP ? "" : "display:none"}">
                <div class="single-select-dropdown settings-ip-option-dropdown">
                  <button type="button" id="settings-ip-option-trigger" class="select-trigger"></button>
                  <div id="settings-ip-option-menu" class="select-menu settings-ip-option-menu hidden"></div>
                </div>
                <input type="text" id="settings-fixed-ip" class="form-input" value="${escapeAttr(fixedIP)}" placeholder="例如: 23.59.72.59">
              </div>
            </div>
          </div>
        </div>

        <div class="settings-panel" id="settings-panel-interface">
          <div class="setting-card">
            <div class="setting-card-title">显示偏好</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">显示模式</div>
                <div class="setting-row-desc">切换文件列表的显示布局</div>
              </div>
              <div class="mode-toggle-group">
                <label class="mode-option ${appState.displayMode === "list" ? "active" : ""}">
                  <input type="radio" name="settings-display-mode" value="list" ${appState.displayMode === "list" ? "checked" : ""}>
                  <span class="mode-text">列表</span>
                </label>
                <label class="mode-option ${appState.displayMode === "card" ? "active" : ""}">
                  <input type="radio" name="settings-display-mode" value="card" ${appState.displayMode === "card" ? "checked" : ""}>
                  <span class="mode-text">卡片</span>
                </label>
              </div>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">界面缩放</div>
                <div class="setting-row-desc">调整整个应用的文字和间距；也可使用 Ctrl +、Ctrl -、Ctrl 0</div>
              </div>
              <div class="ui-scale-control" aria-label="界面缩放">
                <button type="button" class="ui-scale-button" id="settings-ui-scale-decrease" aria-label="减小界面缩放">A−</button>
                <input type="range" id="settings-ui-scale" data-ui-scale-input min="80" max="140" step="5" value="${Math.round(normalizeUIScale(getConfig().uiScale) * 100)}" aria-label="界面缩放百分比">
                <output class="ui-scale-value" data-ui-scale-value for="settings-ui-scale">${Math.round(normalizeUIScale(getConfig().uiScale) * 100)}%</output>
                <button type="button" class="ui-scale-button" id="settings-ui-scale-increase" aria-label="增大界面缩放">A+</button>
              </div>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">框选模式</div>
                <div class="setting-row-desc">拖拽绘制选择框，批量选择 VPK 文件</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-box-selection" ${appState.boxSelectionEnabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">Ctrl+单击选择</div>
                <div class="setting-row-desc">按住 Ctrl 键并单击 Mod，可快速选中或取消选中；Shift+单击可按当前排序选择连续范围</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-ctrl-click-selection" ${appState.ctrlClickSelectionEnabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row setting-row-filter-layout">
              <div class="setting-row-info">
                <div class="setting-row-label">筛选布局</div>
                <div class="setting-row-desc">切换后立即应用到 Mod 列表。两种布局保留相同的筛选能力：简洁模式更省空间；经典模式直接展示常用条件与预设入口。</div>
              </div>
              <div class="mode-toggle-group filter-layout-toggle" role="radiogroup" aria-label="筛选布局">
                <label class="mode-option filter-layout-option filter-layout-option-compact ${appState.filterLayoutMode !== "classic" ? "active" : ""}">
                  <input type="radio" name="settings-filter-layout" value="compact" ${appState.filterLayoutMode !== "classic" ? "checked" : ""}>
                  <span class="filter-layout-option-icon" aria-hidden="true">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="4" width="18" height="4" rx="1"></rect><rect x="3" y="10" width="18" height="4" rx="1"></rect><rect x="3" y="16" width="11" height="4" rx="1"></rect></svg>
                  </span>
                  <span class="filter-layout-option-copy"><strong>简洁下拉</strong><small>节省空间，适合专注浏览</small></span>
                  <span class="filter-layout-option-badge">紧凑</span>
                </label>
                <label class="mode-option filter-layout-option filter-layout-option-classic ${appState.filterLayoutMode === "classic" ? "active" : ""}">
                  <input type="radio" name="settings-filter-layout" value="classic" ${appState.filterLayoutMode === "classic" ? "checked" : ""}>
                  <span class="filter-layout-option-icon" aria-hidden="true">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="3" y="3" width="7" height="7" rx="1"></rect><rect x="14" y="3" width="7" height="7" rx="1"></rect><rect x="3" y="14" width="7" height="7" rx="1"></rect><path d="M14 17.5h7"></path></svg>
                  </span>
                  <span class="filter-layout-option-copy"><strong>经典展开</strong><small>常用条件直观可点</small></span>
                  <span class="filter-layout-option-badge">详细</span>
                </label>
              </div>
            </div>
          </div>
        </div>

        <div class="settings-panel" id="settings-panel-workshop">
          <div class="setting-card">
            <div class="setting-card-title">工坊数据</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">开启工坊信息存储</div>
                <div class="setting-row-desc">为工坊文件创建 .meta 文件，存储名称、作者等信息</div>
              </div>
              <label class="toggle-switch">
                <input type="checkbox" id="settings-meta-enabled" ${metaEnabled ? "checked" : ""}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div class="setting-row ${metaEnabled ? "" : "setting-row-disabled"}" id="settings-update-check-row">
              <div class="setting-row-info">
                <div class="setting-row-label">开启Mod更新检测</div>
                <div class="setting-row-desc">每天自动检测含有工坊信息的Mod是否有新版本，需要先开启工坊信息存储</div>
              </div>
              <label class="toggle-switch ${metaEnabled ? "" : "toggle-switch-disabled"}">
                <input type="checkbox" id="settings-update-check-enabled" ${updateCheckEnabled ? "checked" : ""} ${metaEnabled ? "" : "disabled"}>
                <span class="toggle-slider"></span>
              </label>
            </div>
            <div id="settings-update-check-section" class="setting-indent" style="${updateCheckEnabled ? "" : "display:none"}">
              <button class="trigger-check-btn" id="settings-manual-check-btn" ${metaEnabled && updateCheckEnabled ? "" : "disabled"}>
                <svg class="trigger-check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z"/><path d="M12 6v6l4 2"/></svg>
                <span class="trigger-check-text">立即触发检测</span>
              </button>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">浏览器跳转</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">跳转目标</div>
                <div class="setting-row-desc">选择“使用浏览器打开”时跳转的网站</div>
              </div>
              <div class="setting-radio-group">
                <label class="setting-radio-label"><input type="radio" name="settings-browser-target" value="mirror" ${browserTarget === "mirror" ? "checked" : ""}><span>镜像站 l4d2ws.com</span></label>
                <label class="setting-radio-label"><input type="radio" name="settings-browser-target" value="steam" ${browserTarget === "steam" ? "checked" : ""}><span>Steam 官方工坊</span></label>
              </div>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">描述翻译</div>
            <div class="setting-row">
              <div class="setting-row-info">
                <div class="setting-row-label">翻译服务</div>
                <div class="setting-row-desc">用于创意工坊详情页的描述翻译</div>
              </div>
              <div class="mode-toggle-group settings-translate-provider-toggle">
                <label class="mode-option ${translateProvider === "microsoft" ? "active" : ""}">
                  <input type="radio" name="settings-translate-provider" value="microsoft" ${translateProvider === "microsoft" ? "checked" : ""}>
                  <span class="mode-text">微软</span>
                </label>
                <label class="mode-option ${translateProvider === "yandex" ? "active" : ""}">
                  <input type="radio" name="settings-translate-provider" value="yandex" ${translateProvider === "yandex" ? "checked" : ""}>
                  <span class="mode-text">Yandex</span>
                </label>
                <label class="mode-option ${translateProvider === "custom" ? "active" : ""}">
                  <input type="radio" name="settings-translate-provider" value="custom" ${translateProvider === "custom" ? "checked" : ""}>
                  <span class="mode-text">自定义AI</span>
                </label>
              </div>
            </div>
            <div id="settings-translate-custom-section" class="setting-indent" style="${translateProvider === "custom" ? "" : "display:none"}">
              <div class="setting-row-label" style="margin-bottom:8px">自定义AI配置</div>
              <div class="setting-custom-input-group" style="display:flex;flex-direction:column;gap:8px">
                <input type="text" id="settings-translate-custom-baseurl" class="form-input" value="${escapeAttr(customBaseURL)}" placeholder="Base URL，如 https://api.openai.com/v1">
                <input type="password" id="settings-translate-custom-apikey" class="form-input" value="" placeholder="API Key${hasCustomAPIKey ? "（已设置）" : ""}">
                <input type="text" id="settings-translate-custom-modelid" class="form-input" value="${escapeAttr(customModelId)}" placeholder="模型ID，如 gpt-4o">
              </div>
            </div>
          </div>
        </div>

        <div class="settings-panel" id="settings-panel-addonlist">
          <div class="setting-card autoexec-card">
            <div class="setting-card-title">autoexec.cfg 编辑器与控制台指令小贴士</div>
            <div class="setting-row-desc">编辑后保存会保留游戏文件原有的编码、BOM 和换行格式；内容只写入文件，不会在本程序中执行。未知指令可能来自插件或 Mod。</div>
            <div class="autoexec-layout">
              <div class="autoexec-editor-column">
                <div class="autoexec-meta">
                  <span class="autoexec-path">${escapeHtml(autoexecInfo?.path || "尚未选择 Left 4 Dead 2 的 addons 目录")}</span>
                  <span>${autoexecInfo?.exists ? "文件存在" : "文件不存在，将在保存时创建"}</span>
                  <span>${escapeHtml(autoexecInfo?.encoding || "UTF-8")} · ${escapeHtml(autoexecInfo?.lineEnding || "CRLF")}</span>
                  <span>${formatAddonListBytes(autoexecInfo?.size)}</span>
                  ${autoexecInfo?.lastModified ? `<span>修改于 ${escapeHtml(formatAddonListTime(autoexecInfo.lastModified))}</span>` : ""}
                </div>
                ${autoexecError ? `<div class="addonlist-status-error">${escapeHtml(autoexecError)}</div>` : ""}
                <div class="autoexec-editor-shell">
                  <pre class="autoexec-line-numbers" id="settings-autoexec-line-numbers" aria-hidden="true"></pre>
                  <textarea id="settings-autoexec-content" class="autoexec-editor" spellcheck="false" wrap="off" aria-label="autoexec.cfg 内容">${escapeHtml(autoexecInfo?.content || "")}</textarea>
                </div>
                <div class="addonlist-action-row autoexec-action-row">
                  <button type="button" id="settings-autoexec-save" class="trigger-check-btn addonlist-action-btn" ${autoexecInfo ? "" : "disabled"}>保存 autoexec.cfg</button>
                  <button type="button" id="settings-autoexec-reload" class="trigger-check-btn addonlist-action-btn">重新读取</button>
                  <button type="button" id="settings-autoexec-open" class="trigger-check-btn addonlist-action-btn" ${autoexecInfo?.exists ? "" : "disabled"}>打开文件位置</button>
                </div>
                <div class="autoexec-analysis" id="settings-autoexec-analysis">已识别 ${autoexecMatches.filter((item) => item.known).length} 个已收录指令，${autoexecMatches.filter((item) => !item.known).length} 个未知指令</div>
                <div class="autoexec-matches" id="settings-autoexec-matches"></div>
              </div>
              <aside class="autoexec-help-column">
                <div class="autoexec-help-title">常用指令说明</div>
                <input type="search" id="settings-autoexec-help-search" class="form-input" placeholder="搜索命令或含义">
                <div id="settings-autoexec-help-list" class="autoexec-help-list">
                  ${renderAutoexecHelpItems(autoexecHelp)}
                </div>
              </aside>
            </div>
          </div>
          <div class="setting-card">
            <div class="setting-card-title">addonlist.txt 生命周期管理</div>
            <div class="setting-row-desc">这里管理游戏实际读取的配置文件：先保存受保护版本，再按需创建历史备份；开启监控后，检测到游戏覆盖或删除时会自动恢复并保留外部内容。</div>
            <div class="addonlist-lifecycle-steps" aria-label="addonlist.txt 生命周期">
              <div class="addonlist-lifecycle-step"><strong>1. 保存</strong><span>记录当前字节和编码</span></div>
              <div class="addonlist-lifecycle-step"><strong>2. 备份</strong><span>保留手动/恢复前版本</span></div>
              <div class="addonlist-lifecycle-step"><strong>3. 监控</strong><span>覆盖后自动恢复</span></div>
              <div class="addonlist-lifecycle-step"><strong>4. 删除</strong><span>删除前仍可恢复</span></div>
            </div>
            <div class="setting-row addonlist-status-row">
              <div class="setting-row-info">
                <div class="setting-row-label">目标文件</div>
                <div class="setting-row-desc addonlist-path">${escapeHtml(addonListInfo?.path || "尚未选择 Left 4 Dead 2 的 addons 目录")}</div>
                ${addonListError ? `<div class="addonlist-status-error">${escapeHtml(addonListError)}</div>` : ""}
              </div>
            </div>
            <div class="setting-row addonlist-unrecorded-placement-row">
              <div class="setting-row-info">
                <div class="setting-row-label">未记录 Mod 首次开启时的位置</div>
                <div class="setting-row-desc">仅影响点击“未记录”后首次写入 addonlist.txt 的 Mod；已记录 Mod 的开关不会移动。默认放在列表末尾，避免改变已有 Mod 的相对加载顺序；成功后会提示实际优先级编号。</div>
              </div>
              <select id="settings-unrecorded-mod-load-order-placement" class="form-input addonlist-unrecorded-placement-select" aria-label="未记录 Mod 首次开启时的加载顺序位置">
                <option value="end" ${unrecordedModLoadOrderPlacement === "end" ? "selected" : ""}>列表末尾（默认，不重排已有 Mod）</option>
                <option value="start" ${unrecordedModLoadOrderPlacement === "start" ? "selected" : ""}>列表顶部（优先级 #1）</option>
                <option value="after-enabled" ${unrecordedModLoadOrderPlacement === "after-enabled" ? "selected" : ""}>紧跟最后一个游戏内已开启 Mod</option>
              </select>
            </div>
            ${addonListInfo ? `
              <div class="addonlist-status-grid">
                <div><span>文件状态</span><strong>${addonListInfo.exists ? "存在" : "不存在"}</strong></div>
                <div><span>编码 / 大小</span><strong>${escapeHtml(addonListInfo.encoding || "—")} / ${formatAddonListBytes(addonListInfo.size)}</strong></div>
                <div><span>最后修改</span><strong>${escapeHtml(formatAddonListTime(addonListInfo.lastModified))}</strong></div>
                <div><span>受保护版本</span><strong>${addonListInfo.managedSnapshotExists ? "已保存" : "未保存"}</strong></div>
              </div>
              <div class="addonlist-action-row">
                <button type="button" id="settings-addonlist-save-snapshot" class="trigger-check-btn addonlist-action-btn" ${addonListInfo.exists ? "" : "disabled"}>保存当前配置为受保护版本</button>
                <button type="button" id="settings-addonlist-create-backup" class="trigger-check-btn addonlist-action-btn" ${addonListInfo.exists ? "" : "disabled"}>创建历史备份</button>
                <button type="button" id="settings-addonlist-open" class="trigger-check-btn addonlist-action-btn" ${addonListInfo.exists ? "" : "disabled"}>打开文件位置</button>
              </div>
              <div class="setting-row addonlist-guard-row">
                <div class="setting-row-info">
                  <div class="setting-row-label">监控并自动恢复</div>
                  <div class="setting-row-desc">默认关闭。仅在你手动开启后，应用运行期间才会检测游戏稳定写入或删除 addonlist.txt 的情况，恢复受保护版本并保留被覆盖内容的备份。</div>
                  ${addonListInfo.lastGuardRestore ? `<div class="setting-row-status">最近自动恢复：${escapeHtml(formatAddonListTime(addonListInfo.lastGuardRestore))}</div>` : ""}
                  ${addonListInfo.lastGuardError ? `<div class="addonlist-status-error">监控状态：${escapeHtml(addonListInfo.lastGuardError)}</div>` : ""}
                </div>
                <label class="toggle-switch">
                  <input type="checkbox" id="settings-addonlist-guard" ${addonListInfo.guardEnabled ? "checked" : ""} ${addonListInfo.exists || addonListInfo.managedSnapshotExists ? "" : "disabled"}>
                  <span class="toggle-slider"></span>
                </label>
              </div>
              <div class="addonlist-danger-zone">
                <button type="button" id="settings-addonlist-delete" class="trigger-check-btn addonlist-danger-btn" ${addonListInfo.exists ? "" : "disabled"}>删除 addonlist.txt</button>
                <span>删除前会自动保留一份可恢复的历史备份，同时关闭监控并移除受保护版本。</span>
              </div>
            ` : ""}
          </div>
          <div class="setting-card">
            <div class="setting-card-title">历史备份</div>
            ${addonListInfo ? (addonListBackups.length > 0 ? `
              <div class="addonlist-backup-list">
                ${addonListBackups.map((backup) => `
                  <div class="addonlist-backup-item">
                    <div class="addonlist-backup-main">
                      <strong>${escapeHtml(formatAddonListBackupKind(backup.kind))}</strong>
                      <span>${escapeHtml(formatAddonListTime(backup.createdAt))} · ${formatAddonListBytes(backup.size)}</span>
                    </div>
                    <div class="addonlist-backup-actions">
                      <button type="button" class="addonlist-backup-restore" data-addonlist-backup="${escapeAttr(backup.name)}">恢复</button>
                      <button type="button" class="addonlist-backup-delete" data-addonlist-backup="${escapeAttr(backup.name)}">删除</button>
                    </div>
                  </div>
                `).join("")}
              </div>
            ` : `<div class="setting-row-desc">暂无历史备份。创建备份、恢复备份、自动恢复和删除配置前都会在这里保留记录。</div>`) : ""}
          </div>
          <div class="setting-card">
            <div class="setting-card-title">融合其他 Mod 文件夹配置</div>
            <div class="setting-row-desc">选择另一份 addonlist.txt。新增 Mod 会保留来源开关；相同 Mod 的冲突默认保留当前开关，可逐项勾选采用来源开关。</div>
            <div class="addonlist-action-row addonlist-merge-action-row">
              <button type="button" id="settings-addonlist-select-merge-source" class="trigger-check-btn addonlist-action-btn">选择其他 addonlist.txt</button>
              ${addonListMergePreview ? `<button type="button" id="settings-addonlist-cancel-merge" class="trigger-check-btn addonlist-secondary-btn">取消本次融合</button>` : ""}
            </div>
            ${renderAddonListMergePreview(addonListMergePreview)}
          </div>
        </div>

        </div>
      </div>
    </div>
  `;

  bindSettingsPage({
    enabled,
    fixedIP,
    ipOptions,
    metaEnabled,
    updateCheckEnabled,
    browserTarget,
    translateProvider,
    appState,
    getConfig,
    saveConfig,
    renderFileList,
    renderTagFilters,
    refreshFilesKeepFilter,
    showNotification,
    GetWorkshopIPOptions,
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
    GetAutoexecConfig,
    SaveAutoexecConfig,
    GetAutoexecCommandHelp,
    AnalyzeAutoexecCommands,
    OpenFileLocation,
    autoexecInfo,
    autoexecHelp,
    refreshAddonListPanel: () => renderSettingsPage(deps),
  });
}

function bindSettingsPage(deps) {
  enhanceSettingsNav();

  document.querySelectorAll("#settings-page-content .settings-nav-item").forEach((item) => {
    item.addEventListener("click", () => {
      const target = item.dataset.panel;
      updateSettingsPanelDirection(item);
      document.querySelectorAll("#settings-page-content .settings-nav-item").forEach((nav) => nav.classList.remove("active"));
      document.querySelectorAll("#settings-page-content .settings-panel").forEach((panel) => panel.classList.remove("active"));
      item.classList.add("active");
      document.getElementById(`settings-panel-${target}`)?.classList.add("active");
      updateSettingsNavIndicator();
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
    ipSection.style.display = ipToggle.checked ? "block" : "none";
    await deps.SetWorkshopPreferredIP(ipToggle.checked);
    const config = deps.getConfig();
    config.workshopPreferredIP = ipToggle.checked;
    deps.saveConfig(config);
    deps.showNotification(ipToggle.checked ? "已开启优选 IP 加速" : "已关闭优选 IP 加速", ipToggle.checked ? "success" : "info");
  });

  document.querySelectorAll('input[name="settings-ip-mode"]').forEach((radio) => {
    radio.addEventListener("change", async () => {
      const useFixed = radio.value === "fixed" && radio.checked;
      if (fixedTools) fixedTools.style.display = useFixed ? "" : "none";
      if (!useFixed) {
        await deps.SetWorkshopFixedIP("");
        const config = deps.getConfig();
        config.workshopFixedIP = "";
        deps.saveConfig(config);
        await deps.SetWorkshopPreferredIP(true);
        await refreshIPStatus();
      }
    });
  });

  fixedInput?.addEventListener("change", async () => {
    const fixedIP = fixedInput.value.trim();
    await deps.SetWorkshopFixedIP(fixedIP);
    const config = deps.getConfig();
    config.workshopFixedIP = fixedIP;
    deps.saveConfig(config);
    updateIPOptionTrigger(ipOptionTrigger, ipOptions, fixedIP);
    syncIPOptionMenuActive(ipOptionMenu, fixedIP);
    await refreshIPStatus();
    deps.showNotification("已更新固定 IP 设置", "success");
  });

  const uiScaleInput = document.getElementById("settings-ui-scale");
  const updateUIScale = (value, persist = false) => {
    const scale = applyUIScale(Number(value) / 100);
    if (!persist) return scale;
    const config = deps.getConfig();
    config.uiScale = scale;
    deps.saveConfig(config);
    return scale;
  };
  uiScaleInput?.addEventListener("input", () => {
    updateUIScale(uiScaleInput.value);
  });
  uiScaleInput?.addEventListener("change", () => {
    updateUIScale(uiScaleInput.value, true);
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
    radio.addEventListener("change", () => {
      deps.appState.displayMode = radio.value;
      const config = deps.getConfig();
      config.displayMode = radio.value;
      deps.saveConfig(config);
      radio.closest(".mode-toggle-group")?.querySelectorAll(".mode-option").forEach((option) => option.classList.remove("active"));
      radio.closest(".mode-option")?.classList.add("active");
      deps.renderFileList();
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
          deps.showNotification(nextMode === "classic" ? "已切换到经典展开筛选布局" : "已切换到简洁下拉筛选布局", "success");
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
          deps.showNotification("保存筛选布局失败: " + error, "error");
        });
    });
  });

  document.getElementById("settings-box-selection")?.addEventListener("change", (e) => {
    deps.appState.boxSelectionEnabled = e.target.checked;
    const config = deps.getConfig();
    config.boxSelectionEnabled = e.target.checked;
    deps.saveConfig(config);
    deps.showNotification(e.target.checked ? "已开启框选模式" : "已关闭框选模式", "info");
  });

  document.getElementById("settings-ctrl-click-selection")?.addEventListener("change", (e) => {
    deps.appState.ctrlClickSelectionEnabled = e.target.checked;
    const config = deps.getConfig();
    config.ctrlClickSelectionEnabled = e.target.checked;
    deps.saveConfig(config);
    deps.showNotification(e.target.checked ? "已开启 Ctrl+单击选择" : "已关闭 Ctrl+单击选择", "info");
  });

  document.getElementById("settings-meta-enabled")?.addEventListener("change", async (event) => {
    await deps.SetWorkshopMetaEnabled(event.target.checked);
    const config = deps.getConfig();
    config.workshopMetaEnabled = event.target.checked;
    deps.showNotification(event.target.checked ? "已开启工坊信息存储" : "已关闭工坊信息存储", event.target.checked ? "success" : "info");

    // 更新更新检测开关的可用状态
    const updateCheckRow = document.getElementById("settings-update-check-row");
    const updateCheckToggle = document.getElementById("settings-update-check-enabled");
    const toggleSwitch = updateCheckToggle?.closest(".toggle-switch");

    if (event.target.checked) {
      updateCheckRow?.classList.remove("setting-row-disabled");
      toggleSwitch?.classList.remove("toggle-switch-disabled");
      updateCheckToggle?.removeAttribute("disabled");
      if (updateCheckToggle?.checked) {
        const checkSection = document.getElementById("settings-update-check-section");
        if (checkSection) checkSection.style.display = "";
        document.getElementById("settings-manual-check-btn")?.removeAttribute("disabled");
      }
    } else {
      updateCheckRow?.classList.add("setting-row-disabled");
      toggleSwitch?.classList.add("toggle-switch-disabled");
      updateCheckToggle?.setAttribute("disabled", "true");
      // 关闭meta时也要关闭更新检测
      if (updateCheckToggle?.checked) {
        updateCheckToggle.checked = false;
        await deps.SetWorkshopUpdateCheckEnabled(false);
        config.workshopUpdateCheckEnabled = false;
        const checkSection = document.getElementById("settings-update-check-section");
        if (checkSection) checkSection.style.display = "none";
      }
    }

    deps.saveConfig(config);
    await deps.refreshFilesKeepFilter();
  });

  document.getElementById("settings-update-check-enabled")?.addEventListener("change", async (event) => {
    await deps.SetWorkshopUpdateCheckEnabled(event.target.checked);
    deps.appState.workshopUpdateCheckEnabled = event.target.checked;
    const config = deps.getConfig();
    config.workshopUpdateCheckEnabled = event.target.checked;
    deps.saveConfig(config);
    deps.showNotification(event.target.checked ? "已开启Mod更新检测" : "已关闭Mod更新检测", event.target.checked ? "success" : "info");

    const checkSection = document.getElementById("settings-update-check-section");
    const manualCheckBtn = document.getElementById("settings-manual-check-btn");
    const metaEnabled = document.getElementById("settings-meta-enabled")?.checked;

    if (event.target.checked && metaEnabled) {
      if (checkSection) checkSection.style.display = "";
      manualCheckBtn?.removeAttribute("disabled");
    } else {
      if (checkSection) checkSection.style.display = "none";
      manualCheckBtn?.setAttribute("disabled", "true");
    }
  });

  document.getElementById("settings-manual-check-btn")?.addEventListener("click", async () => {
    const btn = document.getElementById("settings-manual-check-btn");
    btn.disabled = true;
    btn.innerHTML = '<span class="btn-spinner"></span> 检测中...';
    btn.style.pointerEvents = "none";
    try {
      const result = await deps.CheckModUpdates();
      const config = deps.getConfig();
      config.lastUpdateCheckTime = String(Date.now());
      deps.saveConfig(config);
      const count = result.total_updates || 0;
      deps.showNotification(count > 0 ? `检测完成，发现 ${count} 个Mod有更新` : "检测完成，所有Mod均为最新版本", count > 0 ? "info" : "success");
      await deps.refreshFilesKeepFilter();
    } catch (err) {
      deps.showNotification("检测失败: " + err, "error");
    }
    btn.disabled = false;
    const checkIcon = '<svg class="trigger-check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z"/><path d="M12 6v6l4 2"/></svg>';
    btn.innerHTML = checkIcon + '<span class="trigger-check-text">立即触发检测</span>';
    btn.style.pointerEvents = "";
  });

  document.querySelectorAll('input[name="settings-browser-target"]').forEach((radio) => {
    radio.addEventListener("change", async () => {
      if (!radio.checked) return;
      await deps.SetWorkshopBrowserTarget(radio.value);
      const config = deps.getConfig();
      config.workshopBrowserTarget = radio.value;
      deps.saveConfig(config);
      deps.showNotification(radio.value === "mirror" ? "已切换到镜像站" : "已切换到 Steam 官方", "success");
    });
  });

  const customSection = document.getElementById("settings-translate-custom-section");

  document.querySelectorAll('input[name="settings-translate-provider"]').forEach((radio) => {
    radio.addEventListener("change", async () => {
      if (!radio.checked) return;
      await deps.SetWorkshopTranslateProvider(radio.value);
      const config = deps.getConfig();
      config.workshopTranslateProvider = radio.value;
      deps.saveConfig(config);
      radio.closest(".mode-toggle-group")?.querySelectorAll(".mode-option").forEach((option) => option.classList.remove("active"));
      radio.closest(".mode-option")?.classList.add("active");
      if (customSection) {
        customSection.style.display = radio.value === "custom" ? "" : "none";
      }
      const messages = {
        microsoft: "已切换到微软翻译",
        yandex: "已切换到 Yandex 翻译",
        custom: "已切换到自定义AI翻译",
      };
      deps.showNotification(messages[radio.value] || "已切换翻译服务", "success");
    });
  });

  // 自定义AI配置输入框事件
  const customBaseURLInput = document.getElementById("settings-translate-custom-baseurl");
  const customAPIKeyInput = document.getElementById("settings-translate-custom-apikey");
  const customModelIdInput = document.getElementById("settings-translate-custom-modelid");

  customBaseURLInput?.addEventListener("change", async () => {
    const url = customBaseURLInput.value.trim();
    await deps.SetWorkshopTranslateCustomBaseURL(url);
    const config = deps.getConfig();
    config.workshopTranslateCustomBaseURL = url;
    deps.saveConfig(config);
    deps.showNotification("已更新自定义AI Base URL", "success");
  });

  customAPIKeyInput?.addEventListener("change", async () => {
    const key = customAPIKeyInput.value.trim();
    if (!key) return;
    await deps.SetWorkshopTranslateCustomAPIKey(key);
    const config = deps.getConfig();
    config.workshopTranslateCustomAPIKey = "已设置";
    deps.saveConfig(config);
    customAPIKeyInput.value = "";
    // 直接赋值，避免用户重复更新密钥时把“（已设置）”叠加多次。
    customAPIKeyInput.setAttribute("placeholder", "API Key（已设置）");
    deps.showNotification("已更新自定义AI API Key", "success");
  });

  customModelIdInput?.addEventListener("change", async () => {
    const modelId = customModelIdInput.value.trim();
    await deps.SetWorkshopTranslateCustomModelId(modelId);
    const config = deps.getConfig();
    config.workshopTranslateCustomModelId = modelId;
    deps.saveConfig(config);
    deps.showNotification("已更新自定义AI模型ID", "success");
  });

  const autoexecEditor = document.getElementById("settings-autoexec-content");
  const autoexecLineNumbers = document.getElementById("settings-autoexec-line-numbers");
  const autoexecAnalysis = document.getElementById("settings-autoexec-analysis");
  const autoexecMatchesEl = document.getElementById("settings-autoexec-matches");
  const autoexecLineNumberEditor = attachLineNumberGutter(autoexecEditor, autoexecLineNumbers);
  let autoexecAnalysisRequest = 0;
  let autoexecAnalysisTimer = null;
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
        autoexecAnalysis.textContent = `已识别 ${known} 个已收录指令，${unknown} 个未知指令${highRisk ? `；高风险 ${highRisk}` : ""}${mediumRisk ? `；中风险 ${mediumRisk}` : ""}`;
        autoexecAnalysis.classList.toggle("is-warning", unknown > 0 || highRisk > 0);
      }
      renderAutoexecMatches(autoexecMatchesEl, matches);
    } catch (error) {
      if (requestId !== autoexecAnalysisRequest) return;
      if (autoexecAnalysis) autoexecAnalysis.textContent = `指令识别失败：${String(error?.message || error)}`;
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
  const reloadAutoexecEditor = async () => {
    if (!autoexecEditor || typeof deps.GetAutoexecConfig !== "function") return;
    const next = await deps.GetAutoexecConfig();
    autoexecInfo = next || autoexecInfo;
    autoexecEditor.value = next?.content || "";
    autoexecLineNumberEditor.refresh();
    await updateAutoexecAnalysis();
  };
  document.getElementById("settings-autoexec-save")?.addEventListener("click", async () => {
    if (!autoexecEditor || typeof deps.SaveAutoexecConfig !== "function") return;
    const button = document.getElementById("settings-autoexec-save");
    if (button) button.disabled = true;
    try {
      await deps.SaveAutoexecConfig(autoexecEditor.value);
      deps.showNotification("已保存 autoexec.cfg（编码与换行格式已保留）", "success");
      await reloadAutoexecEditor();
    } catch (error) {
      deps.showNotification("保存 autoexec.cfg 失败: " + error, "error");
    } finally {
      if (button) button.disabled = false;
    }
  });
  document.getElementById("settings-autoexec-reload")?.addEventListener("click", async () => {
    try {
      await reloadAutoexecEditor();
      deps.showNotification("已重新读取 autoexec.cfg", "info");
    } catch (error) {
      deps.showNotification("重新读取 autoexec.cfg 失败: " + error, "error");
    }
  });
  document.getElementById("settings-autoexec-open")?.addEventListener("click", async () => {
    if (!autoexecInfo?.path || typeof deps.OpenFileLocation !== "function") return;
    try {
      await deps.OpenFileLocation(autoexecInfo.path);
    } catch (error) {
      deps.showNotification("打开 autoexec.cfg 位置失败: " + error, "error");
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
      deps.showNotification("搜索指令说明失败: " + error, "error");
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

  const refreshAddonListPanel = async () => {
    await deps.refreshAddonListPanel?.();
  };
  const refreshAddonListFiles = async () => {
    await deps.refreshFilesKeepFilter?.();
  };

  document.getElementById("settings-addonlist-save-snapshot")?.addEventListener("click", async () => {
    try {
      await deps.SaveAddonListManagedSnapshot();
      deps.showNotification("已保存 addonlist.txt 受保护版本", "success");
      await refreshAddonListPanel();
    } catch (error) {
      deps.showNotification("保存受保护版本失败: " + error, "error");
    }
  });

  document.getElementById("settings-addonlist-create-backup")?.addEventListener("click", async () => {
    try {
      await deps.CreateAddonListBackup();
      deps.showNotification("已创建 addonlist.txt 历史备份", "success");
      await refreshAddonListPanel();
    } catch (error) {
      deps.showNotification("创建备份失败: " + error, "error");
    }
  });

  document.getElementById("settings-addonlist-open")?.addEventListener("click", async () => {
    const path = document.querySelector("#settings-panel-addonlist .addonlist-path")?.textContent?.trim();
    if (!path || typeof deps.OpenFileLocation !== "function") return;
    try {
      await deps.OpenFileLocation(path);
    } catch (error) {
      deps.showNotification("打开 addonlist.txt 位置失败: " + error, "error");
    }
  });

  document.getElementById("settings-unrecorded-mod-load-order-placement")?.addEventListener("change", async (event) => {
    const select = event.currentTarget;
    const placement = ["start", "after-enabled", "end"].includes(select.value) ? select.value : "end";
    const previousPlacement = deps.getConfig().unrecordedModLoadOrderPlacement || "end";
    try {
      const config = deps.getConfig();
      config.unrecordedModLoadOrderPlacement = placement;
      await deps.saveConfig(config);
      deps.showNotification("已更新未记录 Mod 首次开启时的加载顺序位置", "success");
    } catch (error) {
      select.value = previousPlacement;
      deps.showNotification("保存未记录 Mod 插入位置失败: " + error, "error");
    }
  });

  document.getElementById("settings-addonlist-guard")?.addEventListener("change", async (event) => {
    const enabled = event.target.checked;
    try {
      await deps.SetAddonListGuardEnabled(enabled);
      // SetAddonListGuardEnabled 会立即持久化后端状态；同时更新前端缓存，
      // 防止用户随后修改其他设置时，把旧的监控开关快照再次提交。
      const config = deps.getConfig();
      config.addonListGuardEnabled = enabled;
      await deps.saveConfig(config);
      deps.showNotification(enabled ? "已开启 addonlist.txt 自动恢复监控" : "已关闭 addonlist.txt 自动恢复监控", enabled ? "success" : "info");
      await refreshAddonListPanel();
    } catch (error) {
      event.target.checked = !enabled;
      deps.showNotification("更新监控状态失败: " + error, "error");
    }
  });

  document.querySelectorAll(".addonlist-backup-restore").forEach((button) => {
    button.addEventListener("click", async () => {
      const name = button.dataset.addonlistBackup;
      if (!name || !window.confirm("恢复这份备份？当前 addonlist.txt 会先自动备份，然后被替换。")) return;
      try {
        await deps.RestoreAddonListBackup(name);
        await refreshAddonListFiles();
        deps.showNotification("已恢复 addonlist.txt 备份并同步受保护版本", "success");
        await refreshAddonListPanel();
      } catch (error) {
        deps.showNotification("恢复备份失败: " + error, "error");
      }
    });
  });

  document.querySelectorAll(".addonlist-backup-delete").forEach((button) => {
    button.addEventListener("click", async () => {
      const name = button.dataset.addonlistBackup;
      if (!name || !window.confirm("删除这份历史备份？该操作不可撤销。")) return;
      try {
        await deps.DeleteAddonListBackup(name);
        deps.showNotification("已删除历史备份", "info");
        await refreshAddonListPanel();
      } catch (error) {
        deps.showNotification("删除备份失败: " + error, "error");
      }
    });
  });

  document.getElementById("settings-addonlist-delete")?.addEventListener("click", async () => {
    if (!window.confirm("删除 addonlist.txt？程序会先创建历史备份；自动恢复监控会关闭，受保护版本也会移除。")) return;
    try {
      await deps.DeleteAddonList();
      await refreshAddonListFiles();
      deps.showNotification("已删除 addonlist.txt，并保留删除前备份", "info");
      await refreshAddonListPanel();
    } catch (error) {
      deps.showNotification("删除 addonlist.txt 失败: " + error, "error");
    }
  });

  document.getElementById("settings-addonlist-select-merge-source")?.addEventListener("click", async () => {
    try {
      const sourcePath = await deps.SelectAddonListMergeSource();
      if (!sourcePath) return;
      addonListMergePreview = await deps.PreviewAddonListMerge(sourcePath);
      deps.showNotification("已读取融合差异，请确认冲突项的开关选择", "info");
      await refreshAddonListPanel();
    } catch (error) {
      deps.showNotification("读取融合差异失败: " + error, "error");
    }
  });

  document.getElementById("settings-addonlist-cancel-merge")?.addEventListener("click", async () => {
    addonListMergePreview = null;
    await refreshAddonListPanel();
  });

  document.getElementById("settings-addonlist-apply-merge")?.addEventListener("click", async () => {
    if (!addonListMergePreview) return;
    const sourceWinsKeys = Array.from(document.querySelectorAll(".addonlist-merge-conflict input:checked"))
      .map((input) => input.dataset.addonlistMergeKey)
      .filter(Boolean);
    try {
      await deps.ApplyAddonListMerge(addonListMergePreview.sourcePath, sourceWinsKeys);
      addonListMergePreview = null;
      await refreshAddonListFiles();
      deps.showNotification("已融合 addonlist.txt 配置", "success");
      await refreshAddonListPanel();
    } catch (error) {
      deps.showNotification("融合 addonlist.txt 失败: " + error, "error");
    }
  });

}

function renderAddonListMergePreview(preview) {
  if (!preview) {
    return `<div class="setting-row-desc addonlist-merge-empty">未选择来源配置。此功能也适用于临时维护的任意 Mod 文件夹。</div>`;
  }
  const added = Array.isArray(preview.added) ? preview.added : [];
  const conflicts = Array.isArray(preview.conflicts) ? preview.conflicts : [];
  return `
    <div class="addonlist-merge-preview">
      <div class="addonlist-merge-source">来源：${escapeHtml(preview.sourcePath || "")}</div>
      <div class="addonlist-merge-summary">
        <span>新增 ${added.length} 项（沿用来源开关）</span>
        <span>开关冲突 ${conflicts.length} 项</span>
      </div>
      ${conflicts.length > 0 ? `
        <div class="addonlist-merge-conflicts">
          ${conflicts.map((conflict) => `
            <label class="addonlist-merge-conflict">
              <input type="checkbox" data-addonlist-merge-key="${escapeAttr(conflict.key || "")}">
              <span class="addonlist-merge-conflict-name">${escapeHtml(conflict.key || "")}</span>
              <span class="addonlist-merge-state">当前：${conflict.currentEnabled ? "开启" : "关闭"}</span>
              <span class="addonlist-merge-state">来源：${conflict.sourceEnabled ? "开启" : "关闭"}</span>
              <span class="addonlist-merge-adopt">勾选采用来源</span>
            </label>
          `).join("")}
        </div>
      ` : ""}
      <button type="button" id="settings-addonlist-apply-merge" class="trigger-check-btn addonlist-action-btn" ${added.length || conflicts.length ? "" : "disabled"}>应用融合</button>
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
      fixedInput.value = option.ip;
      await SetWorkshopFixedIP(option.ip);
      const config = getConfig();
      config.workshopFixedIP = option.ip;
      saveConfig(config);
      updateIPOptionTrigger(trigger, normalizedOptions, option.ip);
      syncIPOptionMenuActive(menu, option.ip);
      await onStatusUpdate?.();
      menu.classList.add("hidden");
      showNotification("已更新固定 IP 设置", "success");
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

  trigger.textContent = "正在加载推荐固定 IP...";
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
    trigger.textContent = "候选 IP 加载失败，可自建服务器并填写 IP";
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
    trigger.textContent = formatIPOptionLabel({ ip, category: "自定义" });
  } else if (options?.length) {
    trigger.textContent = "选择优选 IP，也可自建服务器并填写 IP";
  } else {
    trigger.textContent = "暂无候选 IP，可自建服务器并填写 IP";
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
    statusEl.textContent = "正在优选最佳线路...";
    return;
  }

  const bestIPOption = await GetCurrentBestIPOption();
  const bestIP = getIPOptionIP(bestIPOption) || (await GetCurrentBestIP());
  if (!bestIP) {
    statusEl.textContent = "尚未获取到优选 IP";
    return;
  }

  const useFixedIP = getUseFixedIP?.() || false;
  const option = bestIPOption || findIPOption(ipOptions, bestIP) || { ip: bestIP, category: useFixedIP ? "自定义" : "" };
  statusEl.textContent = `${useFixedIP ? "当前固定 IP" : "当前优选 IP"}: ${formatIPOptionLabel(option)}`;
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

function getIPOptionCategory(option, fallback = "未分类") {
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

function renderAutoexecHelpItems(items) {
  if (!Array.isArray(items) || items.length === 0) {
    return `<div class="autoexec-help-empty">未找到匹配指令</div>`;
  }
  return items
    .map(
      (item) => `
        <button type="button" class="autoexec-help-item" data-autoexec-command="${escapeAttr(item.command || "")}" data-autoexec-risk="${escapeAttr(item.risk || "")}">
          <span class="autoexec-help-command">${escapeHtml(item.command || "")}</span>
          <span class="autoexec-help-summary">${escapeHtml(item.summary || "")}</span>
          <span class="autoexec-help-meta">${escapeHtml(item.scope || "")} · ${escapeHtml(item.risk || "")} · ${escapeHtml(item.source || "")}</span>
        </button>
      `,
    )
    .join("");
}

function renderAutoexecHelpList(container, items) {
  container.replaceChildren();
  if (!Array.isArray(items) || items.length === 0) {
    const empty = document.createElement("div");
    empty.className = "autoexec-help-empty";
    empty.textContent = "未找到匹配指令";
    container.appendChild(empty);
    return;
  }
  for (const item of items) {
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
    container.appendChild(button);
  }
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
    title.textContent = `第 ${item.line} 行 · ${item.command}${item.known ? "" : "（未知指令）"}`;
    row.appendChild(title);
    const detail = document.createElement("div");
    detail.className = "autoexec-match-detail";
    detail.textContent = item.known && item.help
      ? `${item.help.summary} · ${item.help.scope} · ${item.help.risk} · ${item.help.source}`
      : "可能来自插件或 Mod，建议确认来源后再保存。";
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
  const labels = {
    manual: "手动备份",
    external: "游戏覆盖前",
    "game-save": "旧版短时保护写入前",
    before: "恢复/删除前",
  };
  return labels[String(kind || "")] || "历史备份";
}

function parseAddonListManagerState(payload) {
  const parsed = typeof payload === "string" ? JSON.parse(payload || "{}") : payload || {};
  return {
    info: parsed?.info || null,
    backups: Array.isArray(parsed?.backups) ? parsed.backups : [],
  };
}
