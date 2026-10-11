import { normalizePanelUrl } from "./panel-url.js";
import { t } from "../../core/i18n-runtime.js";

let showError;
let showNotification;
let showConfirmModal;
let FetchPanelServerStatus;
let RestartPanelServer;
let FetchPanelMapList;
let FetchPanelMapFiles;
let FetchPanelMapIssues;
let ClearPanelMaps;
let DeletePanelMapFile;
let ChangePanelMap;
let FetchPanelMapHotReloadStatus;
let HotReloadPanelMaps;
let ChangePanelDifficulty;
let SendPanelRconCommand;
let SelectPanelMapUploadFiles;
let StartPanelMapUpload;
let GetPanelMapUploadTasks;
let RetryPanelMapUpload;
let CancelPanelMapUpload;
let ClearCompletedPanelMapUploads;
let BrowserOpenURL;
let resolveMapName;
let escapeHtml;
let escapeAttr;
let getIPHost;
let getServers;
let SERVER_ICONS;
let PANEL_MODE_LABEL_KEYS;
let OFFICIAL_CAMPAIGNS;

export function configurePanelModal(deps) {
  ({
    showError,
    showNotification,
    showConfirmModal,
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
    BrowserOpenURL,
    resolveMapName,
    escapeHtml,
    escapeAttr,
    getIPHost,
    getServers,
    SERVER_ICONS,
    PANEL_MODE_LABEL_KEYS,
    OFFICIAL_CAMPAIGNS,
  } = deps);
}

let currentPanelServer = null;
let currentPanelServerIndex = -1;
let currentPanelMaps = [];
let currentPanelMapFiles = [];
const currentPanelMapIssues = new Map();
const deletingPanelMapFiles = new Set();
let currentPanelDifficulty = "";
let panelOfficialMapsHidden = false;
let panelMapRequestToken = 0;
let panelMapFileRequestToken = 0;
let panelStatusRequestToken = 0;
let panelServerDetailsSessionToken = 0;
// 这些子窗口会复用同一组 DOM。操作返回时必须确认仍属于原服务器和原窗口，
// 否则旧响应会把新打开的窗口关闭或写入错误内容。
let panelDifficultySessionToken = 0;
let panelMapActionSessionToken = 0;
let panelRconSessionToken = 0;
let panelUploadSessionToken = 0;
let panelMapHotReloadSessionToken = 0;
let panelMapFilesRefreshTimer = null;
const completedPanelUploadNotifications = new Set();

const PANEL_MAP_ISSUE_BATCH_SIZE = 10;

// 难度值（"简单"…）是后端指令参数（internal/app/server_panel.go 按此校验），不随界面语言；
// 显示名走目录（值显分离）。
const PANEL_DIFFICULTIES = [
  { value: "简单", labelKey: "servers.panel.difficulty.easy", desc: "Easy", aliases: ["easy"] },
  { value: "普通", labelKey: "servers.panel.difficulty.normal", desc: "Normal", aliases: ["normal"] },
  { value: "高级", labelKey: "servers.panel.difficulty.hard", desc: "Hard", aliases: ["hard", "advanced"] },
  { value: "专家", labelKey: "servers.panel.difficulty.impossible", desc: "Impossible", aliases: ["impossible", "expert"] },
];

const PANEL_UPLOAD_ICON = `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="17 8 12 3 7 8"></polyline><line x1="12" y1="3" x2="12" y2="15"></line></svg>`;
const PANEL_CANCEL_ICON = `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>`;
const PANEL_RETRY_ICON = `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-2.64-6.36"></path><path d="M21 3v6h-6"></path></svg>`;

export function openPanelServerDetailsModal(index) {
  const server = getServers()[index];
  if (!server) return;

  panelServerDetailsSessionToken += 1;
  // 详情窗口切换服务器时，所有子窗口及其未完成操作都属于旧服务器，必须先
  // 统一关闭并失效，避免确认框或文件选择返回后读取新的 currentPanelServer。
  closePanelMapModal();
  closePanelDifficultyModal();
  closePanelUploadModal();
  closePanelRconModal();

  currentPanelServer = server;
  currentPanelServerIndex = index;
  currentPanelDifficulty = "";

  const modal = document.getElementById("panel-server-details-modal");
  const title = document.getElementById("panel-details-server-name");
  const loading = document.getElementById("panel-details-loading");
  const content = document.getElementById("panel-details-content");
  const error = document.getElementById("panel-details-error");

  title.textContent = server.name;
  loading.textContent = t("servers.panel.details.loading");
  loading.classList.remove("hidden");
  content.classList.add("hidden");
  error.classList.add("hidden");
  error.innerHTML = "";
  document.getElementById("panel-restart-btn")?.removeAttribute("disabled");
  modal.classList.remove("hidden");

  loadPanelStatus(server);
}

export function closePanelServerDetailsModal() {
  panelServerDetailsSessionToken += 1;
  panelStatusRequestToken += 1;
  closePanelMapModal();
  closePanelDifficultyModal();
  closePanelUploadModal();
  closePanelRconModal();
  document.getElementById("panel-server-details-modal")?.classList.add("hidden");
  currentPanelServer = null;
  currentPanelServerIndex = -1;
}

function isCurrentPanelServerDetailsSession(serverID, sessionToken) {
  const modal = document.getElementById("panel-server-details-modal");
  return (
    sessionToken === panelServerDetailsSessionToken &&
    String(currentPanelServer?.id || "") === String(serverID || "") &&
    !modal?.classList.contains("hidden")
  );
}

function isPanelStatusRequestActive(requestToken, server) {
  const modal = document.getElementById("panel-server-details-modal");
  return (
    requestToken === panelStatusRequestToken &&
    String(currentPanelServer?.id || "") === String(server?.id || "") &&
    !modal?.classList.contains("hidden")
  );
}

async function loadPanelStatus(server = currentPanelServer) {
  if (!server) return;
  const requestToken = ++panelStatusRequestToken;

  const loading = document.getElementById("panel-details-loading");
  const content = document.getElementById("panel-details-content");
  const error = document.getElementById("panel-details-error");
  const refreshBtn = document.getElementById("panel-refresh-btn");

  loading.classList.remove("hidden");
  content.classList.add("hidden");
  error.classList.add("hidden");
  error.innerHTML = "";
  refreshBtn?.setAttribute("disabled", "true");

  try {
    const status = await FetchPanelServerStatus(server.id);
    if (!isPanelStatusRequestActive(requestToken, server)) return;

    const rendered = await renderPanelStatus(server, status || {}, requestToken);
    if (!rendered || !isPanelStatusRequestActive(requestToken, server)) return;

    loading.classList.add("hidden");
    content.classList.remove("hidden");
  } catch (err) {
    if (!isPanelStatusRequestActive(requestToken, server)) return;

    console.error("获取面板状态失败:", err);
    loading.classList.add("hidden");
    error.textContent = t("servers.panel.error.statusFailed", { error: err });
    error.classList.remove("hidden");
    renderPanelStatusError(err);
  } finally {
    if (isPanelStatusRequestActive(requestToken, server)) {
      refreshBtn?.removeAttribute("disabled");
    }
  }
}

function renderPanelStatusError(err) {
  const error = document.getElementById("panel-details-error");
  if (!error) return;

  error.innerHTML = `
    <div class="panel-error-content">
      <span>${t("servers.panel.error.statusFailed", { error: escapeHtml(err) })}</span>
      <button id="panel-details-retry-btn" class="btn btn-secondary btn-small panel-action-btn" type="button">
        ${t("servers.panel.refresh")}
      </button>
    </div>
  `;
  error
    .querySelector("#panel-details-retry-btn")
    ?.addEventListener("click", refreshCurrentPanelStatus);
  error.classList.remove("hidden");
}

async function renderPanelStatus(server, status, requestToken) {
  const summary = document.getElementById("panel-status-summary");
  const playerList = document.getElementById("panel-player-list");
  const rawMap = status.map || "Unknown";
  let displayMap = rawMap;
  try {
    const resolved = await resolveMapName(rawMap);
    if (resolved && resolved !== rawMap) {
      displayMap = resolved;
    }
  } catch {
    displayMap = rawMap;
  }

  if (!isPanelStatusRequestActive(requestToken, server)) return false;

  currentPanelDifficulty = status.difficulty || "";

  summary.innerHTML = `
    ${renderPanelStatusItem(t("servers.panel.status.hostname"), status.hostname || server.name)}
    ${renderPanelStatusItem(t("servers.panel.status.map"), displayMap, rawMap)}
    ${renderPanelStatusItem(t("servers.panel.status.players"), status.players || "0/0")}
    ${renderPanelStatusItem(t("servers.panel.status.mode"), status.gameMode || t("servers.panel.unknown"))}
    ${renderPanelStatusItem(t("servers.panel.status.difficulty"), status.difficulty || t("servers.panel.unknown"))}
  `;

  const users = Array.isArray(status.users) ? status.users : [];
  if (users.length === 0) {
    playerList.innerHTML =
      `<tr><td colspan="6" class="empty-state">${t("servers.panel.noPlayers")}</td></tr>`;
    return true;
  }

  playerList.innerHTML = users
    .map(
      (user) => `
        <tr>
          <td class="player-name">${escapeHtml(user.name || "Unknown")}</td>
          <td class="panel-player-steamid">${escapeHtml(user.steamid || "-")}</td>
          <td>${escapeHtml(user.location || getIPHost(user.ip) || "-")}</td>
          <td class="text-right">${Number(user.delay) || 0}ms</td>
          <td class="text-right">${Number(user.loss) || 0}%</td>
          <td class="text-right">${escapeHtml(user.duration || "-")}</td>
        </tr>
      `
    )
    .join("");
  return true;
}

function renderPanelStatusItem(label, value, title = "") {
  return `
    <div class="server-info-item panel-status-item" title="${escapeAttr(title || value)}">
      <span class="server-info-label">${escapeHtml(label)}</span>
      <span class="server-info-value">${escapeHtml(value)}</span>
    </div>
  `;
}

export function refreshCurrentPanelStatus() {
  loadPanelStatus(currentPanelServer);
}

export function restartCurrentPanelServer() {
  if (!currentPanelServer) return;
  const serverID = currentPanelServer.id;
  const serverName = currentPanelServer.name;
  const sessionToken = panelServerDetailsSessionToken;
  showConfirmModal(
    t("servers.panel.restart.title"),
    t("servers.panel.restart.confirm", { name: serverName }),
    async () => {
      if (!isCurrentPanelServerDetailsSession(serverID, sessionToken)) return;
      const btn = document.getElementById("panel-restart-btn");
      btn?.setAttribute("disabled", "true");
      try {
        const text = await RestartPanelServer(serverID);
        if (!isCurrentPanelServerDetailsSession(serverID, sessionToken)) return;
        showNotification(text || t("servers.panel.restart.sent"), "success");
      } catch (err) {
        if (!isCurrentPanelServerDetailsSession(serverID, sessionToken)) return;
        console.error("重启失败:", err);
        showError(t("servers.panel.error.restartFailed", { error: err }));
      } finally {
        if (isCurrentPanelServerDetailsSession(serverID, sessionToken) && btn?.isConnected) {
          btn.removeAttribute("disabled");
        }
      }
    }
  );
}

export function openCurrentPanelInBrowser() {
  if (!currentPanelServer?.panelUrl) return;
  if (typeof BrowserOpenURL === "function") {
    BrowserOpenURL(normalizePanelUrl(currentPanelServer.panelUrl));
  }
}

export function openPanelDifficultyModal() {
  if (!currentPanelServer) return;

  panelDifficultySessionToken += 1;

  const modal = document.getElementById("panel-difficulty-modal");
  const title = document.getElementById("panel-difficulty-title");
  if (title) {
    title.textContent = t("servers.panel.difficulty.title", { name: currentPanelServer.name });
  }
  renderPanelDifficultyOptions();
  modal?.classList.remove("hidden");
}

export function closePanelDifficultyModal() {
  panelDifficultySessionToken += 1;
  document.getElementById("panel-difficulty-modal")?.classList.add("hidden");
}

function isCurrentPanelDifficultySession(serverID, sessionToken) {
  const modal = document.getElementById("panel-difficulty-modal");
  return (
    sessionToken === panelDifficultySessionToken &&
    String(currentPanelServer?.id || "") === String(serverID || "") &&
    !modal?.classList.contains("hidden")
  );
}

function renderPanelDifficultyOptions() {
  const list = document.getElementById("panel-difficulty-options");
  if (!list) return;

  list.replaceChildren();
  const activeValue = normalizePanelDifficultyValue(currentPanelDifficulty);

  PANEL_DIFFICULTIES.forEach((difficulty) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "panel-difficulty-option";
    button.dataset.difficulty = difficulty.value;
    button.setAttribute("aria-pressed", String(difficulty.value === activeValue));
    if (difficulty.value === activeValue) {
      button.classList.add("active");
    }

    const label = document.createElement("span");
    label.textContent = t(difficulty.labelKey);
    const desc = document.createElement("small");
    desc.textContent = difficulty.desc;
    button.append(label, desc);
    list.appendChild(button);
  });
}

function normalizePanelDifficultyValue(value) {
  const normalized = String(value || "").trim().toLowerCase();
  const entry = PANEL_DIFFICULTIES.find(
    (item) => item.value === normalized || item.aliases.includes(normalized)
  );
  return entry ? entry.value : "";
}

// 显示名解析：通知等只拿得到值的场景走这里（按钮渲染直接取 labelKey）。
function getPanelDifficultyLabel(value) {
  const entry = PANEL_DIFFICULTIES.find((item) => item.value === value);
  return entry ? t(entry.labelKey) : String(value || "");
}

async function handlePanelDifficultyClick(event) {
  const button = event.target.closest(".panel-difficulty-option");
  if (!button || !currentPanelServer) return;

  const difficulty = button.dataset.difficulty;
  if (!difficulty) return;
  const serverID = currentPanelServer.id;
  const sessionToken = panelDifficultySessionToken;

  button.disabled = true;
  try {
    const text = await ChangePanelDifficulty(serverID, difficulty);
    if (!isCurrentPanelDifficultySession(serverID, sessionToken)) return;
    currentPanelDifficulty = difficulty;
    showNotification(text || t("servers.panel.difficulty.switched", { difficulty: getPanelDifficultyLabel(difficulty) }), "success");
    closePanelDifficultyModal();
  } catch (err) {
    if (!isCurrentPanelDifficultySession(serverID, sessionToken)) return;
    console.error("修改难度失败:", err);
    showError(t("servers.panel.error.difficultyFailed", { error: err }));
  } finally {
    if (isCurrentPanelDifficultySession(serverID, sessionToken) && button.isConnected) {
      button.disabled = false;
    }
  }
}

export function openPanelMapModal() {
  if (!currentPanelServer) return;
  panelMapActionSessionToken += 1;
  panelMapHotReloadSessionToken += 1;
  const modal = document.getElementById("panel-map-modal");
  const title = document.getElementById("panel-map-title");
  const search = document.getElementById("panel-map-search");
  title.textContent = t("servers.panel.map.title", { name: currentPanelServer.name });
  search.value = "";
  currentPanelMaps = [];
  updatePanelOfficialToggle();
  modal.classList.remove("hidden");
  loadPanelMaps();
}

export function closePanelMapModal() {
  panelMapRequestToken += 1;
  panelMapActionSessionToken += 1;
  panelMapHotReloadSessionToken += 1;
  setPanelMapHotReloading(false);
  document.getElementById("panel-map-modal")?.classList.add("hidden");
}

function isCurrentPanelMapActionSession(serverID, sessionToken) {
  return (
    sessionToken === panelMapActionSessionToken &&
    String(currentPanelServer?.id || "") === String(serverID || "") &&
    isPanelMapModalOpen()
  );
}

function isCurrentPanelMapHotReloadSession(
  serverID,
  detailsSessionToken,
  hotReloadSessionToken
) {
  return (
    hotReloadSessionToken === panelMapHotReloadSessionToken &&
    isCurrentPanelServerDetailsSession(serverID, detailsSessionToken) &&
    isPanelMapModalOpen()
  );
}

async function loadPanelMaps() {
  if (!currentPanelServer) return;
  const serverID = currentPanelServer.id;
  const requestToken = ++panelMapRequestToken;
  const loading = document.getElementById("panel-map-loading");
  const list = document.getElementById("panel-map-list");
  const refreshBtn = document.getElementById("panel-map-refresh-btn");
  loading.classList.remove("hidden");
  list.innerHTML = "";
  refreshBtn.disabled = true;
  refreshBtn.querySelector(".icon-svg")?.classList.add("spinning");

  try {
    const customMaps = await FetchPanelMapList(serverID);
    if (!isCurrentPanelMapRequest(serverID, requestToken)) return;
    currentPanelMaps = [
      ...OFFICIAL_CAMPAIGNS.map((campaign) => normalizeCampaign(campaign, false)),
      ...(Array.isArray(customMaps) ? customMaps : []).map((campaign) =>
        normalizeCampaign(campaign, true)
      ),
    ];
    renderPanelMapList();
  } catch (err) {
    if (!isCurrentPanelMapRequest(serverID, requestToken)) return;
    console.error("获取地图列表失败:", err);
    list.innerHTML = `<div class="panel-error-box">${t("servers.panel.error.mapListFailed", { error: escapeHtml(err) })}</div>`;
  } finally {
    if (!isCurrentPanelMapRequest(serverID, requestToken)) return;
    loading.classList.add("hidden");
    refreshBtn.disabled = false;
    refreshBtn.querySelector(".icon-svg")?.classList.remove("spinning");
  }
}

function isCurrentPanelMapRequest(serverID, requestToken) {
  return (
    requestToken === panelMapRequestToken &&
    currentPanelServer?.id === serverID &&
    !document.getElementById("panel-map-modal")?.classList.contains("hidden")
  );
}

function setPanelMapHotReloading(loading) {
  const button = document.getElementById("panel-map-hot-reload-btn");
  if (!button) return;

  button.disabled = loading;
  button.setAttribute("aria-busy", loading ? "true" : "false");
  button.querySelector(".icon-svg")?.classList.toggle("spinning", loading);
}

async function executePanelMapHotReload(
  serverID,
  detailsSessionToken,
  hotReloadSessionToken
) {
  if (
    !isCurrentPanelMapHotReloadSession(
      serverID,
      detailsSessionToken,
      hotReloadSessionToken
    )
  ) {
    return;
  }

  setPanelMapHotReloading(true);
  try {
    const result = await HotReloadPanelMaps(serverID);
    if (
      !isCurrentPanelMapHotReloadSession(
        serverID,
        detailsSessionToken,
        hotReloadSessionToken
      )
    ) {
      return;
    }
    showNotification(result?.message || t("servers.panel.hotReload.sent"), "success");
  } catch (err) {
    if (
      !isCurrentPanelMapHotReloadSession(
        serverID,
        detailsSessionToken,
        hotReloadSessionToken
      )
    ) {
      return;
    }
    console.error("热重载地图失败:", err);
    showError(t("servers.panel.error.hotReloadFailed", { error: err }));
  } finally {
    if (
      isCurrentPanelMapHotReloadSession(
        serverID,
        detailsSessionToken,
        hotReloadSessionToken
      )
    ) {
      setPanelMapHotReloading(false);
    }
  }
}

async function confirmPanelMapHotReload() {
  if (!currentPanelServer) return;
  const serverID = currentPanelServer.id;
  const detailsSessionToken = panelServerDetailsSessionToken;
  const hotReloadSessionToken = ++panelMapHotReloadSessionToken;

  setPanelMapHotReloading(true);
  let status;
  try {
    status = await FetchPanelMapHotReloadStatus(serverID);
    if (
      !isCurrentPanelMapHotReloadSession(
        serverID,
        detailsSessionToken,
        hotReloadSessionToken
      )
    ) {
      return;
    }
  } catch (err) {
    if (
      !isCurrentPanelMapHotReloadSession(
        serverID,
        detailsSessionToken,
        hotReloadSessionToken
      )
    ) {
      return;
    }
    console.error("获取地图热重载状态失败:", err);
    showError(t("servers.panel.error.hotReloadStatusFailed", { error: err }));
    return;
  } finally {
    if (
      isCurrentPanelMapHotReloadSession(
        serverID,
        detailsSessionToken,
        hotReloadSessionToken
      )
    ) {
      setPanelMapHotReloading(false);
    }
  }

  let message = t("servers.panel.hotReload.message");
  if (status?.using_default) {
    message += t("servers.panel.hotReload.defaultHint");
  }

  showConfirmModal(
    t("servers.panel.hotReload.confirmTitle"),
    message,
    () =>
      executePanelMapHotReload(
        serverID,
        detailsSessionToken,
        hotReloadSessionToken
      ),
    false,
    "panel-hot-reload-confirm"
  );
}

export function openPanelUploadModal() {
  if (!currentPanelServer) return;
  panelUploadSessionToken += 1;

  const modal = document.getElementById("panel-upload-modal");
  const title = document.getElementById("panel-upload-title");
  if (title) {
    title.textContent = t("servers.panel.upload.title", { name: currentPanelServer.name });
  }
  modal?.classList.remove("hidden");
  refreshPanelUploadTasks();
  loadPanelMapFiles();
}

export function closePanelUploadModal() {
  panelMapFileRequestToken += 1;
  panelUploadSessionToken += 1;
  currentPanelMapFiles = [];
  currentPanelMapIssues.clear();
  if (panelMapFilesRefreshTimer !== null) {
    clearTimeout(panelMapFilesRefreshTimer);
    panelMapFilesRefreshTimer = null;
  }
  document.getElementById("panel-select-upload-files-btn")?.removeAttribute("disabled");
  document.getElementById("panel-clear-maps-btn")?.removeAttribute("disabled");
  const taskList = document.getElementById("panel-upload-tasks-list");
  taskList?.replaceChildren();
  taskList?.classList.remove("has-tasks", "has-multiple-tasks");
  document.getElementById("panel-upload-modal")?.classList.add("hidden");
}

function isCurrentPanelUploadSession(
  serverID,
  detailsSessionToken,
  uploadSessionToken
) {
  return (
    uploadSessionToken === panelUploadSessionToken &&
    isCurrentPanelServerDetailsSession(serverID, detailsSessionToken) &&
    isPanelUploadModalOpen()
  );
}

function normalizePanelMapIssueKey(vpkName) {
  return String(vpkName || "").trim().toLowerCase();
}

function normalizePanelMapFile(file) {
  return {
    name: String(file?.name || file?.Name || "").trim(),
    size: String(file?.size || file?.Size || "unknown").trim() || "unknown",
  };
}

function getPanelMapFileDeletionKey(serverID, mapName) {
  return `${serverID}\n${normalizePanelMapIssueKey(mapName)}`;
}

function isCurrentPanelMapFileRequest(
  serverID,
  requestToken,
  detailsSessionToken,
  uploadSessionToken
) {
  return (
    requestToken === panelMapFileRequestToken &&
    isCurrentPanelUploadSession(
      serverID,
      detailsSessionToken,
      uploadSessionToken
    )
  );
}

async function loadPanelMapFiles() {
  if (
    !currentPanelServer ||
    !isPanelUploadModalOpen() ||
    typeof FetchPanelMapFiles !== "function"
  ) {
    return;
  }

  const serverID = currentPanelServer.id;
  const detailsSessionToken = panelServerDetailsSessionToken;
  const uploadSessionToken = panelUploadSessionToken;
  const requestToken = ++panelMapFileRequestToken;
  const list = document.getElementById("panel-vpk-list");
  const loading = document.getElementById("panel-vpk-loading");
  const refreshBtn = document.getElementById("panel-vpk-refresh-btn");
  const count = document.getElementById("panel-vpk-count");
  if (!list) return;

  currentPanelMapFiles = [];
  currentPanelMapIssues.clear();
  list.replaceChildren();
  if (count) count.textContent = t("servers.panel.upload.reading");
  loading?.classList.remove("hidden");
  refreshBtn?.setAttribute("disabled", "true");
  refreshBtn?.querySelector(".icon-svg")?.classList.add("spinning");

  try {
    const files = await FetchPanelMapFiles(serverID);
    if (
      !isCurrentPanelMapFileRequest(
        serverID,
        requestToken,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    currentPanelMapFiles = (Array.isArray(files) ? files : [])
      .map(normalizePanelMapFile)
      .filter((file) => file.name);
    renderPanelMapFiles();
    void loadPanelMapFileIssues(
      serverID,
      currentPanelMapFiles,
      requestToken,
      detailsSessionToken,
      uploadSessionToken
    );
  } catch (err) {
    if (
      !isCurrentPanelMapFileRequest(
        serverID,
        requestToken,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    const error = document.createElement("div");
    error.className = "panel-error-box";
    error.textContent = t("servers.panel.error.mapFilesReadFailed", { error: err });
    list.replaceChildren(error);
    if (count) count.textContent = t("servers.panel.upload.readFailed");
  } finally {
    if (
      !isCurrentPanelMapFileRequest(
        serverID,
        requestToken,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    loading?.classList.add("hidden");
    refreshBtn?.removeAttribute("disabled");
    refreshBtn?.querySelector(".icon-svg")?.classList.remove("spinning");
  }
}

async function loadPanelMapFileIssues(
  serverID,
  files,
  requestToken,
  detailsSessionToken,
  uploadSessionToken
) {
  if (typeof FetchPanelMapIssues !== "function") return;
  const vpkNames = files.map((file) => file.name);
  for (let index = 0; index < vpkNames.length; index += PANEL_MAP_ISSUE_BATCH_SIZE) {
    if (
      !isCurrentPanelMapFileRequest(
        serverID,
        requestToken,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    const batch = vpkNames.slice(index, index + PANEL_MAP_ISSUE_BATCH_SIZE);
    let response;
    try {
      response = await FetchPanelMapIssues(serverID, batch);
    } catch {
      return;
    }
    if (
      !isCurrentPanelMapFileRequest(
        serverID,
        requestToken,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    if (!response?.supported) return;

    Object.entries(response.items || {}).forEach(([vpkName, issue]) => {
      const key = normalizePanelMapIssueKey(vpkName);
      if (!key) return;
      currentPanelMapIssues.set(key, {
        dictionaryMissing: Math.max(0, Number(issue?.dictionaryMissing) || 0),
        dictionaryUnreadable: Boolean(issue?.dictionaryUnreadable),
        globalScripts: Math.max(0, Number(issue?.globalScripts) || 0),
        scriptOverrides: Math.max(0, Number(issue?.scriptOverrides) || 0),
      });
    });
    renderPanelMapFiles();
  }
}

function renderPanelMapFiles() {
  const list = document.getElementById("panel-vpk-list");
  const count = document.getElementById("panel-vpk-count");
  if (!list) return;
  if (count) count.textContent = t("servers.panel.upload.fileCount", { count: currentPanelMapFiles.length });

  if (currentPanelMapFiles.length === 0) {
    const empty = document.createElement("div");
    empty.className = "panel-vpk-empty";
    empty.textContent = t("servers.panel.upload.emptyVpk");
    list.replaceChildren(empty);
    return;
  }

  list.replaceChildren(...currentPanelMapFiles.map(createPanelMapFileElement));
}

function createPanelMapFileElement(file) {
  const item = document.createElement("div");
  item.className = "panel-vpk-item";

  const main = document.createElement("div");
  main.className = "panel-vpk-main";

  const titleRow = document.createElement("div");
  titleRow.className = "panel-vpk-title-row";
  const title = document.createElement("span");
  title.className = "panel-vpk-name";
  title.title = file.name;
  title.textContent = file.name;
  const size = document.createElement("span");
  size.className = "panel-vpk-size";
  size.textContent = file.size === "unknown" ? t("servers.panel.upload.sizeUnknown") : file.size;
  titleRow.append(title, size);
  main.appendChild(titleRow);

  const issue = currentPanelMapIssues.get(normalizePanelMapIssueKey(file.name));
  const riskTags = createPanelMapRiskTags(issue);
  if (riskTags) main.appendChild(riskTags);

  const deleteButton = document.createElement("button");
  deleteButton.type = "button";
  deleteButton.className = "btn btn-danger btn-small panel-vpk-delete-btn";
  deleteButton.dataset.vpkName = file.name;
  const deleting = Boolean(
    currentPanelServer &&
      deletingPanelMapFiles.has(getPanelMapFileDeletionKey(currentPanelServer.id, file.name))
  );
  deleteButton.disabled = deleting;
  deleteButton.textContent = deleting ? t("servers.panel.upload.deleting") : t("servers.panel.upload.delete");

  item.append(main, deleteButton);
  return item;
}

function createPanelMapRiskTags(issue) {
  if (!issue) return null;
  const tags = [];
  if (issue.dictionaryMissing > 0) {
    tags.push(createPanelMapRiskTag(t("servers.panel.risk.dictionaryMissing", { count: issue.dictionaryMissing }), "danger"));
  }
  if (issue.dictionaryUnreadable) {
    tags.push(createPanelMapRiskTag(t("servers.panel.risk.dictionaryUnreadable"), "warning"));
  }
  if (issue.globalScripts > 0) {
    tags.push(createPanelMapRiskTag(t("servers.panel.risk.globalScripts", { count: issue.globalScripts }), "warning"));
  }
  if (issue.scriptOverrides > 0) {
    tags.push(createPanelMapRiskTag(t("servers.panel.risk.scriptOverrides", { count: issue.scriptOverrides }), "danger"));
  }
  if (tags.length === 0) return null;

  const container = document.createElement("div");
  container.className = "panel-map-risk-tags";
  tags.forEach((tag) => container.appendChild(tag));
  return container;
}

function handlePanelMapFileClick(event) {
  const button = event.target.closest(".panel-vpk-delete-btn");
  const mapName = String(button?.dataset.vpkName || "").trim();
  if (!button || !mapName || !currentPanelServer) return;
  const serverID = currentPanelServer.id;
  const detailsSessionToken = panelServerDetailsSessionToken;
  const uploadSessionToken = panelUploadSessionToken;

  showConfirmModal(
    t("servers.panel.deleteMap.title"),
    t("servers.panel.deleteMap.confirm", { name: mapName }),
    async () => {
      if (
        !isCurrentPanelUploadSession(
          serverID,
          detailsSessionToken,
          uploadSessionToken
        )
      ) {
        return;
      }
      const key = getPanelMapFileDeletionKey(serverID, mapName);
      let deleteSucceeded = false;
      deletingPanelMapFiles.add(key);
      renderPanelMapFiles();
      try {
        const text = await DeletePanelMapFile(serverID, mapName);
        deleteSucceeded = true;
        if (
          !isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          )
        ) {
          return;
        }
        showNotification(text || t("servers.panel.deleteMap.done", { name: mapName }), "success");
        await loadPanelMapFiles();
        if (
          isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          ) &&
          isPanelMapModalOpen()
        ) {
          loadPanelMaps();
        }
      } catch (err) {
        if (
          !isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          )
        ) {
          return;
        }
        console.error("删除 VPK 地图失败:", err);
        showError(t("servers.panel.error.deleteMapFailed", { error: err }));
      } finally {
        deletingPanelMapFiles.delete(key);
        if (
          !deleteSucceeded &&
          isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          )
        ) {
          renderPanelMapFiles();
        }
      }
    }
  );
}

function schedulePanelMapFilesRefresh() {
  if (!isPanelUploadModalOpen()) return;
  if (panelMapFilesRefreshTimer !== null) clearTimeout(panelMapFilesRefreshTimer);
  panelMapFilesRefreshTimer = setTimeout(() => {
    panelMapFilesRefreshTimer = null;
    loadPanelMapFiles();
  }, 150);
}

async function selectPanelUploadFiles() {
  if (!currentPanelServer) {
    showError(t("servers.panel.error.noPanelServer"));
    return;
  }

  const serverID = currentPanelServer.id;
  const detailsSessionToken = panelServerDetailsSessionToken;
  const uploadSessionToken = panelUploadSessionToken;
  if (
    !isCurrentPanelUploadSession(
      serverID,
      detailsSessionToken,
      uploadSessionToken
    )
  ) {
    return;
  }

  const btn = document.getElementById("panel-select-upload-files-btn");
  btn?.setAttribute("disabled", "true");
  try {
    const paths = await SelectPanelMapUploadFiles();
    if (!paths || paths.length === 0) return;

    if (
      !isCurrentPanelUploadSession(
        serverID,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    await StartPanelMapUpload(serverID, paths);
    if (
      !isCurrentPanelUploadSession(
        serverID,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    showNotification(t("servers.panel.upload.added", { count: paths.length }), "success");
    await refreshPanelUploadTasks();
  } catch (err) {
    if (
      !isCurrentPanelUploadSession(
        serverID,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    console.error("添加上传任务失败:", err);
    showError(t("servers.panel.error.addUploadFailed", { error: err }));
  } finally {
    if (
      isCurrentPanelUploadSession(
        serverID,
        detailsSessionToken,
        uploadSessionToken
      ) &&
      btn?.isConnected
    ) {
      btn.removeAttribute("disabled");
    }
  }
}

export async function refreshPanelUploadTasks() {
  const list = document.getElementById("panel-upload-tasks-list");
  if (
    !list ||
    !currentPanelServer ||
    typeof GetPanelMapUploadTasks !== "function" ||
    !isPanelUploadModalOpen()
  ) {
    return;
  }
  const serverID = currentPanelServer.id;
  const detailsSessionToken = panelServerDetailsSessionToken;
  const uploadSessionToken = panelUploadSessionToken;

  try {
    const tasks = await GetPanelMapUploadTasks();
    if (
      !isCurrentPanelUploadSession(
        serverID,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    renderPanelUploadTasks(tasks || []);
  } catch (err) {
    if (
      !isCurrentPanelUploadSession(
        serverID,
        detailsSessionToken,
        uploadSessionToken
      )
    ) {
      return;
    }
    console.error("刷新上传任务失败:", err);
    list.classList.remove("has-tasks");
    list.classList.remove("has-multiple-tasks");
    list.innerHTML = `<div class="panel-error-box">${t("servers.panel.error.refreshTasksFailed", { error: escapeHtml(err) })}</div>`;
  }
}

function renderPanelUploadTasks(tasks) {
  const list = document.getElementById("panel-upload-tasks-list");
  if (!list) return;
  const hasTasks = Array.isArray(tasks) && tasks.length > 0;
  list.classList.toggle("has-tasks", hasTasks);
  list.classList.toggle(
    "has-multiple-tasks",
    Array.isArray(tasks) && tasks.length > 1
  );

  if (!hasTasks) {
    list.innerHTML = `
      <div class="panel-upload-empty">
        <div class="panel-upload-empty-icon">${PANEL_UPLOAD_ICON}</div>
        <div class="panel-upload-empty-title">${t("servers.panel.upload.emptyTasks")}</div>
        <div class="panel-upload-empty-text">${t("servers.panel.upload.emptyTasksHint")}</div>
      </div>
    `;
    return;
  }

  list.innerHTML = "";
  tasks.forEach((task) => {
    list.appendChild(createPanelUploadTaskElement(task));
    handlePanelUploadCompletion(task);
  });
}

function createPanelUploadTaskElement(task) {
  const div = document.createElement("div");
  const status = task.status || "pending";
  div.id = `panel-upload-task-${task.id}`;
  div.className = `panel-upload-task status-${status}`;
  div.dataset.status = status;

  const progress = Number(task.progress) || 0;
  const isActive = ["pending", "compressing", "uploading", "merging"].includes(status);
  // 重启后留下的 interrupted 任务同样可以点重试续传。
  const canRetry = status === "failed" || status === "cancelled" || status === "interrupted";
  const actionHtml = isActive
    ? `
      <button class="panel-upload-icon-btn panel-upload-cancel-btn" data-id="${escapeAttr(task.id)}" title="${t("servers.panel.upload.cancelTip")}" type="button">
        ${PANEL_CANCEL_ICON}
      </button>
    `
    : canRetry
      ? `
        <button class="panel-upload-icon-btn panel-upload-retry-btn" data-id="${escapeAttr(task.id)}" title="${t("servers.panel.upload.retryTip")}" type="button">
          ${PANEL_RETRY_ICON}
        </button>
      `
      : "";

  div.innerHTML = `
    <div class="panel-upload-task-icon">${PANEL_UPLOAD_ICON}</div>
    <div class="panel-upload-task-main">
      <div class="panel-upload-task-top">
        <div class="panel-upload-task-title" title="${escapeAttr(task.filename || "")}">
          ${escapeHtml(task.filename || "Unknown")}
        </div>
        <div class="panel-upload-task-actions">
          <span class="panel-upload-status">${getPanelUploadStatusText(status)}</span>
          ${actionHtml}
        </div>
      </div>
      <div class="panel-upload-task-meta">
        <span title="${escapeAttr(task.server_name || "")}">${escapeHtml(task.server_name || t("servers.panel.upload.fallbackServerName"))}</span>
        <span>${formatPanelUploadBytes(task.uploaded_size || 0)} / ${formatPanelUploadBytes(task.total_size || 0)}</span>
        ${task.speed ? `<span>${escapeHtml(task.speed)}</span>` : ""}
      </div>
      <div class="panel-upload-progress">
        <div class="panel-upload-progress-fill" style="width: ${progress}%"></div>
      </div>
      <div class="panel-upload-task-foot">
        <span>${task.total_chunks ? t("servers.panel.upload.chunks", { done: (task.uploaded_chunks || []).length, total: task.total_chunks }) : t("servers.panel.upload.waiting")}</span>
        <span class="panel-upload-percent">${progress}%</span>
      </div>
      ${task.error ? `<div class="panel-upload-error">${escapeHtml(task.error)}</div>` : ""}
    </div>
  `;

  div.querySelector(".panel-upload-cancel-btn")?.addEventListener("click", (event) => {
    event.stopPropagation();
    showConfirmModal(t("servers.panel.upload.cancelTitle"), t("servers.panel.upload.cancelConfirm"), async () => {
      try {
        await CancelPanelMapUpload(task.id);
        showNotification(t("servers.panel.upload.cancelled"), "info");
      } catch (err) {
        console.error("取消上传失败:", err);
        showError(t("servers.panel.error.cancelUploadFailed", { error: err }));
      }
    });
  });

  div.querySelector(".panel-upload-retry-btn")?.addEventListener("click", async (event) => {
    event.stopPropagation();
    try {
      await RetryPanelMapUpload(task.id);
      showNotification(t("servers.panel.upload.resumed"), "success");
    } catch (err) {
      console.error("继续上传失败:", err);
      showError(t("servers.panel.error.resumeUploadFailed", { error: err }));
    }
  });

  return div;
}

export function updatePanelUploadTaskInList(task) {
  if (!isPanelUploadModalOpen()) {
    handlePanelUploadCompletion(task);
    return;
  }
  const existing = document.getElementById(`panel-upload-task-${task.id}`);
  if (existing) {
    existing.replaceWith(createPanelUploadTaskElement(task));
  } else if (isPanelUploadModalOpen()) {
    refreshPanelUploadTasks();
  }
  handlePanelUploadCompletion(task);
}

export function updatePanelUploadProgress(task) {
  if (!isPanelUploadModalOpen()) return;
  const existing = document.getElementById(`panel-upload-task-${task.id}`);
  if (!existing) {
    if (isPanelUploadModalOpen()) refreshPanelUploadTasks();
    return;
  }

  existing.dataset.status = task.status || "uploading";
  existing.className = `panel-upload-task status-${task.status || "uploading"}`;
  const progress = Number(task.progress) || 0;
  const fill = existing.querySelector(".panel-upload-progress-fill");
  const percent = existing.querySelector(".panel-upload-percent");
  const meta = existing.querySelector(".panel-upload-task-meta");
  const foot = existing.querySelector(".panel-upload-task-foot span:first-child");
  const status = existing.querySelector(".panel-upload-status");

  if (fill) fill.style.width = `${progress}%`;
  if (percent) percent.textContent = `${progress}%`;
  if (status) status.textContent = getPanelUploadStatusText(task.status || "uploading");
  if (meta) {
    meta.innerHTML = `
      <span title="${escapeAttr(task.server_name || "")}">${escapeHtml(task.server_name || t("servers.panel.upload.fallbackServerName"))}</span>
      <span>${formatPanelUploadBytes(task.uploaded_size || 0)} / ${formatPanelUploadBytes(task.total_size || 0)}</span>
      ${task.speed ? `<span>${escapeHtml(task.speed)}</span>` : ""}
    `;
  }
  if (foot) {
    foot.textContent = task.total_chunks
      ? t("servers.panel.upload.chunks", { done: (task.uploaded_chunks || []).length, total: task.total_chunks })
      : t("servers.panel.upload.waiting");
  }
}

export function handlePanelUploadTasksCleared() {
  if (isPanelUploadModalOpen()) refreshPanelUploadTasks();
}

async function clearCompletedPanelUploads() {
  try {
    await ClearCompletedPanelMapUploads();
    showNotification(t("servers.panel.upload.cleared"), "success");
  } catch (err) {
    console.error("清理上传任务失败:", err);
    showError(t("servers.panel.error.clearUploadsFailed", { error: err }));
  }
}

async function clearPanelMaps() {
  if (!currentPanelServer) {
    showError(t("servers.panel.error.noPanelServer"));
    return;
  }
  const serverID = currentPanelServer.id;
  const detailsSessionToken = panelServerDetailsSessionToken;
  const uploadSessionToken = panelUploadSessionToken;

  showConfirmModal(
    t("servers.panel.clearMaps.title"),
    t("servers.panel.clearMaps.confirm"),
    async () => {
      if (
        !isCurrentPanelUploadSession(
          serverID,
          detailsSessionToken,
          uploadSessionToken
        )
      ) {
        return;
      }
      const btn = document.getElementById("panel-clear-maps-btn");
      btn?.setAttribute("disabled", "true");
      try {
        const text = await ClearPanelMaps(serverID);
        if (
          !isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          )
        ) {
          return;
        }
        showNotification(text || t("servers.panel.clearMaps.done"), "success");
        await loadPanelMapFiles();
        if (
          isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          ) &&
          isPanelMapModalOpen()
        ) {
          loadPanelMaps();
        }
      } catch (err) {
        if (
          !isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          )
        ) {
          return;
        }
        console.error("清空地图失败:", err);
        showError(t("servers.panel.error.clearMapsFailed", { error: err }));
      } finally {
        if (
          isCurrentPanelUploadSession(
            serverID,
            detailsSessionToken,
            uploadSessionToken
          ) &&
          btn?.isConnected
        ) {
          btn.removeAttribute("disabled");
        }
      }
    }
  );
}

function handlePanelUploadCompletion(task) {
  if (task.status !== "completed" || completedPanelUploadNotifications.has(task.id)) {
    return;
  }
  completedPanelUploadNotifications.add(task.id);
  showNotification(t("servers.panel.upload.uploaded", { name: task.filename || t("servers.panel.upload.fallbackMapName") }), "success");
  if (String(task.server_id || "") !== String(currentPanelServer?.id || "")) {
    return;
  }
  schedulePanelMapFilesRefresh();
  if (isPanelMapModalOpen()) {
    loadPanelMaps();
  }
}

function isPanelUploadModalOpen() {
  const modal = document.getElementById("panel-upload-modal");
  return Boolean(modal && !modal.classList.contains("hidden"));
}

function isPanelMapModalOpen() {
  const modal = document.getElementById("panel-map-modal");
  return Boolean(modal && !modal.classList.contains("hidden"));
}

function getPanelUploadStatusText(status) {
  const labels = {
    pending: t("servers.panel.upload.status.pending"),
    compressing: t("servers.panel.upload.status.compressing"),
    uploading: t("servers.panel.upload.status.uploading"),
    merging: t("servers.panel.upload.status.merging"),
    completed: t("servers.panel.upload.status.completed"),
    failed: t("servers.panel.upload.status.failed"),
    cancelled: t("servers.panel.upload.status.cancelled"),
    // 应用退出时还没跑完：任务记录会留在 upload_tasks.json，点重试即可续传。
    interrupted: t("servers.panel.upload.status.interrupted"),
  };
  return labels[status] || status || t("servers.panel.unknown");
}

function formatPanelUploadBytes(bytes, decimals = 2) {
  const value = Number(bytes) || 0;
  if (value <= 0) return "0 Bytes";
  const k = 1024;
  const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(value) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((value / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`;
}

function normalizeCampaign(campaign, isCustom) {
  return {
    // 官方战役表存 titleKey（显示名随界面语言）；服务器返回的三方地图只有字面 title。
    title: campaign.titleKey
      ? t(campaign.titleKey)
      : campaign.title || campaign.Title || "Unknown Campaign",
    vpkName: campaign.vpkName || campaign.VpkName || "",
    isCustom,
    chapters: (campaign.chapters || campaign.Chapters || []).map((chapter) => ({
      code: chapter.code || chapter.Code || "",
      title: chapter.titleKey
        ? t(chapter.titleKey)
        : chapter.title || chapter.Title || chapter.code || chapter.Code || "",
      modes: chapter.modes || chapter.Modes || [],
    })),
  };
}

export function togglePanelOfficialMaps() {
  panelOfficialMapsHidden = !panelOfficialMapsHidden;
  renderPanelMapList();
}

function updatePanelOfficialToggle() {
  const toggleBtn = document.getElementById("panel-map-official-toggle-btn");
  if (!toggleBtn) return;

  toggleBtn.textContent = panelOfficialMapsHidden ? t("servers.panel.map.toggleShow") : t("servers.panel.map.toggleHide");
  toggleBtn.classList.toggle("active", panelOfficialMapsHidden);
}

function normalizePanelModes(modes) {
  const values = Array.isArray(modes)
    ? modes
    : String(modes || "").split(/[,\s/|]+/);
  return values.map((mode) => String(mode).trim()).filter(Boolean);
}

function getPanelModeLabel(mode) {
  const normalized = String(mode).trim().toLowerCase();
  const key = PANEL_MODE_LABEL_KEYS[normalized];
  return key ? t(key) : String(mode).trim();
}

function getPanelModeSearchText(modes) {
  return normalizePanelModes(modes)
    .flatMap((mode) => [mode, getPanelModeLabel(mode)])
    .join(" ")
    .toLowerCase();
}

function getFilteredPanelMaps() {
  const query = document
    .getElementById("panel-map-search")
    .value.trim()
    .toLowerCase();

  return currentPanelMaps
    .filter((campaign) => !panelOfficialMapsHidden || campaign.isCustom)
    .map((campaign) => ({
      ...campaign,
      chapters: campaign.chapters.filter((chapter) => {
        if (!query) return true;
        return (
          campaign.title.toLowerCase().includes(query) ||
          chapter.title.toLowerCase().includes(query) ||
          chapter.code.toLowerCase().includes(query) ||
          campaign.vpkName.toLowerCase().includes(query) ||
          getPanelModeSearchText(chapter.modes).includes(query)
        );
      }),
    }))
    .filter((campaign) => campaign.chapters.length > 0);
}

function renderPanelMapModes(modes) {
  const normalizedModes = normalizePanelModes(modes);
  if (normalizedModes.length === 0) return "";

  return `
    <span class="panel-map-mode-list" aria-label="${t("servers.panel.map.modesLabel")}">
      ${normalizedModes
        .map(
          (mode) =>
            `<span class="panel-map-mode" title="${escapeAttr(mode)}">${escapeHtml(getPanelModeLabel(mode))}</span>`
        )
        .join("")}
    </span>
  `;
}

function renderPanelMapList() {
  const list = document.getElementById("panel-map-list");
  updatePanelOfficialToggle();
  const filtered = getFilteredPanelMaps();

  if (filtered.length === 0) {
    list.innerHTML = `<div class="panel-empty-state">${t("servers.panel.map.empty")}</div>`;
    return;
  }

  list.innerHTML = filtered
    .map(
      (campaign) => `
        <section class="panel-map-campaign">
          <div class="panel-map-campaign-header">
            <div>
              <h4>${escapeHtml(campaign.title)}</h4>
              ${
                campaign.vpkName
                  ? `<span>${escapeHtml(campaign.vpkName)}</span>`
                  : ""
              }
            </div>
            <span class="panel-map-type ${campaign.isCustom ? "custom" : ""}">
              ${campaign.isCustom ? t("servers.panel.map.typeCustom") : t("servers.panel.map.typeOfficial")}
            </span>
          </div>
          <div class="panel-map-chapters">
            ${campaign.chapters
              .map(
                (chapter) => `
                  <button
                    class="panel-map-chapter"
                    type="button"
                    data-map-code="${escapeAttr(chapter.code)}"
                  >
                    <span class="panel-map-chapter-main">
                      <strong>${escapeHtml(chapter.title || chapter.code)}</strong>
                      <small>${escapeHtml(chapter.code)}</small>
                      ${renderPanelMapModes(chapter.modes)}
                    </span>
                    <em>${t("servers.panel.map.switch")}</em>
                  </button>
                `
              )
              .join("")}
          </div>
        </section>
      `
    )
    .join("");
}

function createPanelMapRiskTag(text, level) {
  const tag = document.createElement("span");
  tag.className = `panel-map-risk-tag is-${level}`;
  tag.textContent = text;
  return tag;
}

async function handlePanelMapClick(event) {
  const button = event.target.closest(".panel-map-chapter");
  if (!button || !currentPanelServer) return;
  const mapCode = button.dataset.mapCode;
  if (!mapCode) return;
  const serverID = currentPanelServer.id;
  const sessionToken = ++panelMapActionSessionToken;

  button.disabled = true;
  try {
    await ChangePanelMap(serverID, mapCode);
    if (!isCurrentPanelMapActionSession(serverID, sessionToken)) return;
    const text = t("servers.panel.map.switchSentLong");
    showNotification(text || t("servers.panel.map.switchSent"), "success");
    closePanelMapModal();
  } catch (err) {
    if (!isCurrentPanelMapActionSession(serverID, sessionToken)) return;
    console.error("切换地图失败:", err);
    showError(t("servers.panel.error.switchMapFailed", { error: err }));
  } finally {
    if (isCurrentPanelMapActionSession(serverID, sessionToken) && button.isConnected) {
      button.disabled = false;
    }
  }
}

export function openPanelRconModal() {
  if (!currentPanelServer) return;
  panelRconSessionToken += 1;
  const output = document.getElementById("panel-rcon-output");
  document.getElementById("panel-rcon-title").textContent = `RCON - ${currentPanelServer.name}`;
  document.getElementById("panel-rcon-command").value = "";
  document.getElementById("panel-rcon-output").textContent = t("servers.panel.rcon.waiting");
  document.getElementById("panel-rcon-send-btn")?.removeAttribute("disabled");
  output.classList.add("panel-rcon-output-muted");
  document.getElementById("panel-rcon-modal").classList.remove("hidden");
  document.getElementById("panel-rcon-command").focus();
}

export function closePanelRconModal() {
  panelRconSessionToken += 1;
  document.getElementById("panel-rcon-modal")?.classList.add("hidden");
}

function isCurrentPanelRconSession(serverID, sessionToken) {
  const modal = document.getElementById("panel-rcon-modal");
  return (
    sessionToken === panelRconSessionToken &&
    String(currentPanelServer?.id || "") === String(serverID || "") &&
    !modal?.classList.contains("hidden")
  );
}

async function sendPanelRconCommand() {
  if (!currentPanelServer) return;
  const input = document.getElementById("panel-rcon-command");
  const output = document.getElementById("panel-rcon-output");
  const sendBtn = document.getElementById("panel-rcon-send-btn");
  const command = input.value.trim();
  if (!command) {
    showError(t("servers.panel.rcon.inputRequired"));
    return;
  }
  const serverID = currentPanelServer.id;
  const sessionToken = panelRconSessionToken;

  sendBtn.disabled = true;
  output.classList.add("panel-rcon-output-muted");
  output.textContent = t("servers.panel.rcon.sending");
  try {
    const result = await SendPanelRconCommand(serverID, command);
    if (!isCurrentPanelRconSession(serverID, sessionToken)) return;
    output.classList.remove("panel-rcon-output-muted");
    output.textContent = result || t("servers.panel.rcon.sentNoContent");
  } catch (err) {
    if (!isCurrentPanelRconSession(serverID, sessionToken)) return;
    console.error("RCON 指令失败:", err);
    output.textContent = t("servers.panel.rcon.sendFailed", { error: err });
  } finally {
    if (!isCurrentPanelRconSession(serverID, sessionToken)) return;
    output.classList.remove("panel-rcon-output-muted");
    sendBtn.disabled = false;
  }
}

export function setupPanelModalListeners() {
  document
    .getElementById("close-panel-server-details-modal-btn")
    ?.addEventListener("click", closePanelServerDetailsModal);
  document
    .getElementById("panel-refresh-btn")
    ?.addEventListener("click", refreshCurrentPanelStatus);
  document
    .getElementById("panel-restart-btn")
    ?.addEventListener("click", restartCurrentPanelServer);
  document
    .getElementById("panel-map-btn")
    ?.addEventListener("click", openPanelMapModal);
  document
    .getElementById("panel-difficulty-btn")
    ?.addEventListener("click", openPanelDifficultyModal);
  document
    .getElementById("panel-upload-btn")
    ?.addEventListener("click", openPanelUploadModal);
  document
    .getElementById("panel-open-btn")
    ?.addEventListener("click", openCurrentPanelInBrowser);
  document
    .getElementById("panel-rcon-btn")
    ?.addEventListener("click", openPanelRconModal);

  document
    .getElementById("close-panel-map-modal-btn")
    ?.addEventListener("click", closePanelMapModal);
  document
    .getElementById("panel-map-refresh-btn")
    ?.addEventListener("click", loadPanelMaps);
  document
    .getElementById("panel-map-hot-reload-btn")
    ?.addEventListener("click", confirmPanelMapHotReload);
  document
    .getElementById("panel-map-official-toggle-btn")
    ?.addEventListener("click", togglePanelOfficialMaps);
  document
    .getElementById("panel-map-search")
    ?.addEventListener("input", renderPanelMapList);
  document
    .getElementById("panel-map-list")
    ?.addEventListener("click", handlePanelMapClick);

  document
    .getElementById("close-panel-difficulty-modal-btn")
    ?.addEventListener("click", closePanelDifficultyModal);
  document
    .getElementById("panel-difficulty-options")
    ?.addEventListener("click", handlePanelDifficultyClick);

  document
    .getElementById("close-panel-upload-modal-btn")
    ?.addEventListener("click", closePanelUploadModal);
  document
    .getElementById("panel-select-upload-files-btn")
    ?.addEventListener("click", selectPanelUploadFiles);
  document
    .getElementById("panel-clear-maps-btn")
    ?.addEventListener("click", clearPanelMaps);
  document
    .getElementById("panel-upload-clear-completed-btn")
    ?.addEventListener("click", clearCompletedPanelUploads);
  document
    .getElementById("panel-vpk-refresh-btn")
    ?.addEventListener("click", loadPanelMapFiles);
  document
    .getElementById("panel-vpk-list")
    ?.addEventListener("click", handlePanelMapFileClick);

  document
    .getElementById("close-panel-rcon-modal-btn")
    ?.addEventListener("click", closePanelRconModal);
  document
    .getElementById("panel-rcon-send-btn")
    ?.addEventListener("click", sendPanelRconCommand);
  document
    .getElementById("panel-rcon-command")
    ?.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        sendPanelRconCommand();
      }
    });

  ["panel-server-details-modal", "panel-map-modal", "panel-difficulty-modal", "panel-upload-modal", "panel-rcon-modal"].forEach(
    (modalId) => {
      document.getElementById(modalId)?.addEventListener("click", function (e) {
        if (e.target === this) {
          // Route backdrop closes through the same cleanup functions as the
          // explicit close buttons. This invalidates in-flight requests and
          // clears upload refresh timers before the modal is opened again.
          const closeModal = {
            "panel-server-details-modal": closePanelServerDetailsModal,
            "panel-map-modal": closePanelMapModal,
            "panel-difficulty-modal": closePanelDifficultyModal,
            "panel-upload-modal": closePanelUploadModal,
            "panel-rcon-modal": closePanelRconModal,
          }[modalId];
          closeModal?.();
        }
      });
    }
  );
}
