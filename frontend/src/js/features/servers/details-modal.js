import { openPanelServerDetailsModal } from "./panel-modal.js";

let showError;
let FetchServerInfo;
let FetchPlayerList;
let resolveMapName;
let escapeHtml;
let formatDuration;
let getServers;
let SERVER_ICONS;
let basicDetailsSessionId = 0;

export function configureDetailsModal(deps) {
  ({
    showError,
    FetchServerInfo,
    FetchPlayerList,
    resolveMapName,
    escapeHtml,
    formatDuration,
    getServers,
    SERVER_ICONS,
  } = deps);
}

export function openServerDetailsModal(index) {
  const servers = getServers();
  const server = servers[index];
  if (server?.panelUrl && server?.panelPasswordSet) {
    openPanelServerDetailsModal(index);
    return;
  }
  openBasicServerDetailsModal(index);
}

function closeBasicServerDetailsModal() {
  basicDetailsSessionId += 1;
  document.getElementById("server-details-modal")?.classList.add("hidden");
}

function isBasicDetailsSessionActive(sessionId, server) {
  const modal = document.getElementById("server-details-modal");
  return (
    sessionId === basicDetailsSessionId &&
    !modal?.classList.contains("hidden") &&
    modal?.dataset.serverAddress === String(server.address || "")
  );
}

async function openBasicServerDetailsModal(index) {
  const servers = getServers();
  const server = servers[index];
  if (!server) return;
  const sessionId = ++basicDetailsSessionId;

  const modal = document.getElementById("server-details-modal");
  const title = document.getElementById("details-server-name");
  const loading = document.getElementById("server-details-loading");
  const content = document.getElementById("server-details-content");
  const mapEl = document.getElementById("details-map");
  const playersEl = document.getElementById("details-players");
  const listEl = document.getElementById("details-player-list");

  modal.dataset.serverAddress = String(server.address || "");
  title.textContent = server.name;
  loading.textContent = "正在获取服务器信息...";
  loading.classList.remove("hidden");
  content.classList.add("hidden");
  modal.classList.remove("hidden");

  try {
    const info = await FetchServerInfo(server.address);
    if (!isBasicDetailsSessionActive(sessionId, server)) return;

    mapEl.textContent = info.map;
    mapEl.title = `地图: ${info.map}`;
    playersEl.textContent = `${info.players}/${info.max_players}`;

    void resolveMapName(info.map)
      .then((realName) => {
        if (
          isBasicDetailsSessionActive(sessionId, server) &&
          realName !== info.map &&
          document.body.contains(mapEl)
        ) {
          mapEl.textContent = realName;
          mapEl.title = `地图: ${info.map}`;
        }
      })
      .catch(() => {});

    const players = await FetchPlayerList(server.address);
    if (!isBasicDetailsSessionActive(sessionId, server)) return;

    listEl.innerHTML = "";
    if (players && players.length > 0) {
      players.sort((a, b) => b.score - a.score);

      players.forEach((p) => {
        const tr = document.createElement("tr");
        tr.innerHTML = `
          <td class="player-name">${escapeHtml(p.name)}</td>
          <td class="text-right">${p.score}</td>
          <td class="text-right">${formatDuration(p.duration)}</td>
        `;
        listEl.appendChild(tr);
      });
    } else {
      listEl.innerHTML =
        '<tr><td colspan="3" class="empty-state">暂无玩家信息</td></tr>';
    }

    loading.classList.add("hidden");
    content.classList.remove("hidden");
  } catch (err) {
    if (!isBasicDetailsSessionActive(sessionId, server)) return;
    console.error(err);
    loading.textContent = "获取失败: " + err;
  }
}

export function setupDetailsModalListeners() {
  document
    .getElementById("global-details-server-btn")
    .addEventListener("click", () => {
      const dropdown = document.getElementById("global-dropdown");
      const index = parseInt(dropdown.dataset.index);
      if (!isNaN(index)) {
        openServerDetailsModal(index);
        dropdown.classList.add("hidden");
      }
    });

  document
    .getElementById("close-server-details-modal-btn")
    .addEventListener("click", () => {
      closeBasicServerDetailsModal();
    });

  document
    .getElementById("server-details-modal")
    .addEventListener("click", function (e) {
      if (e.target === this) {
        closeBasicServerDetailsModal();
      }
    });
}
