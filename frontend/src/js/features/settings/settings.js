let appState;
let getConfig;
let saveConfig;
let renderFileList;
let renderTagFilters;
let refreshFilesKeepFilter;
let showNotification;
let renderSettingsPage;
let GetWorkshopPreferredIP;
let GetWorkshopFixedIP;
let GetWorkshopIPOptions;
let GetWorkshopMetaEnabled;
let GetWorkshopUpdateCheckEnabled;
let GetWorkshopBrowserTarget;
let GetWorkshopTranslateProvider;
let GetWorkshopTranslateCustomBaseURL;
let GetWorkshopTranslateCustomModelId;
let HasWorkshopTranslateCustomAPIKey;
let IsSelectingIP;
let GetCurrentBestIP;
let GetCurrentBestIPOption;
let SetWorkshopPreferredIP;
let SetWorkshopFixedIP;
let SetWorkshopMetaEnabled;
let SetWorkshopUpdateCheckEnabled;
let SetWorkshopBrowserTarget;
let SetWorkshopTranslateProvider;
let SetWorkshopTranslateCustomBaseURL;
let SetWorkshopTranslateCustomModelId;
let SetWorkshopTranslateCustomAPIKey;
let CheckModUpdates;
let EventsOn;
let switchAppPage;
let GetAddonListManagerState;
let SaveAddonListManagedSnapshot;
let CreateAddonListBackup;
let RestoreAddonListBackup;
let DeleteAddonListBackup;
let DeleteAddonList;
let SetAddonListGuardEnabled;
let SelectAddonListMergeSource;
let PreviewAddonListMerge;
let ApplyAddonListMerge;
let GetAutoexecConfig;
let SaveAutoexecConfig;
let GetAutoexecCommandHelp;
let AnalyzeAutoexecCommands;
let OpenFileLocation;

export function configureSettings(deps) {
  ({ appState, getConfig, saveConfig, renderFileList, renderTagFilters, refreshFilesKeepFilter, showNotification, renderSettingsPage, GetWorkshopPreferredIP, GetWorkshopFixedIP, GetWorkshopIPOptions, GetWorkshopMetaEnabled, GetWorkshopUpdateCheckEnabled, GetWorkshopBrowserTarget, GetWorkshopTranslateProvider, GetWorkshopTranslateCustomBaseURL, GetWorkshopTranslateCustomModelId, HasWorkshopTranslateCustomAPIKey, IsSelectingIP, GetCurrentBestIP, GetCurrentBestIPOption, SetWorkshopPreferredIP, SetWorkshopFixedIP, SetWorkshopMetaEnabled, SetWorkshopUpdateCheckEnabled, SetWorkshopBrowserTarget, SetWorkshopTranslateProvider, SetWorkshopTranslateCustomBaseURL, SetWorkshopTranslateCustomModelId, SetWorkshopTranslateCustomAPIKey, CheckModUpdates, EventsOn, switchAppPage, GetAddonListManagerState, SaveAddonListManagedSnapshot, CreateAddonListBackup, RestoreAddonListBackup, DeleteAddonListBackup, DeleteAddonList, SetAddonListGuardEnabled, SelectAddonListMergeSource, PreviewAddonListMerge, ApplyAddonListMerge, GetAutoexecConfig, SaveAutoexecConfig, GetAutoexecCommandHelp, AnalyzeAutoexecCommands, OpenFileLocation } = deps);
}

export async function showGlobalSettings() {
  switchAppPage("settings", { silent: true });
  await renderSettingsPageWithDeps();
}

export async function renderSettingsPageWithDeps() {
  try {
    await renderSettingsPage({
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
      });
    } catch (error) {
      console.error("设置页面渲染失败:", error);
      const container = document.getElementById("settings-page-content");
      if (container) {
        const message = document.createElement("div");
        message.className = "settings-read-warning";
        message.setAttribute("role", "alert");
        const title = document.createElement("strong");
        title.textContent = "设置页面加载失败。";
        const detail = document.createElement("span");
        detail.textContent = `请稍后重试；若问题持续存在，请重启应用。原因：${String(error?.message || error || "未知错误")}`;
        message.append(title, detail);
        container.replaceChildren(message);
      }
    }
  }
