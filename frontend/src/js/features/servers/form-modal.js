import { normalizePanelUrl } from "./panel-url.js";
import { findDuplicateServer, normalizeServerAddress } from "./address.js";
import { showConfirmModal } from "../modals/confirm.js";
import { t } from "../../core/i18n-runtime.js";

let showError;
let showNotification;
let getServers;
let saveServers;
let initServerStorage;
let renderServers;
let renderLaunchServerMenu;
let fetchServerInfo;

export function configureFormModal(deps) {
  ({
    showError,
    showNotification,
    getServers,
    saveServers,
    initServerStorage,
    renderServers,
    renderLaunchServerMenu,
    fetchServerInfo,
  } = deps);
}

let currentEditIndex = -1;
let isEditMode = false;
let currentEditID = "";
let formSessionId = 0;
let savingSessionId = 0;

function createServerID() {
  if (globalThis.crypto?.randomUUID) {
    return `srv_${globalThis.crypto.randomUUID().replace(/-/g, "")}`;
  }
  return `srv_${Date.now().toString(36)}_${Math.random().toString(36).slice(2)}`;
}

function isCurrentFormSession(sessionId) {
  return sessionId === formSessionId;
}

function setFormSaving(sessionId, pending) {
  if (!isCurrentFormSession(sessionId)) return;
  const saveButton = document.getElementById("save-server-form-btn");
  if (saveButton) saveButton.disabled = pending;
  const closeButton = document.getElementById("close-server-form-modal-btn");
  if (closeButton) closeButton.disabled = pending;
  const cancelButton = document.getElementById("cancel-server-form-btn");
  if (cancelButton) cancelButton.disabled = pending;
}

export function openServerFormModal(index = -1) {
  const sessionId = ++formSessionId;
  const modal = document.getElementById("server-form-modal");
  const title = document.getElementById("server-form-title");
  const nameInput = document.getElementById("form-server-name");
  const addressInput = document.getElementById("form-server-address");
  const weightInput = document.getElementById("form-server-weight");
  const panelUrlInput = document.getElementById("form-server-panel-url");
  const panelPasswordInput = document.getElementById("form-server-panel-password");
  const clearPasswordInput = document.getElementById("form-clear-panel-password");
  const passwordStatus = document.getElementById("panel-password-status");
  const advancedContent = document.getElementById("server-advanced-content");
  const advancedToggle = document.getElementById("server-advanced-toggle");

  nameInput.value = "";
  addressInput.value = "";
  weightInput.value = "0";
  if (panelUrlInput) panelUrlInput.value = "";
  if (panelPasswordInput) {
    panelPasswordInput.value = "";
    panelPasswordInput.placeholder = t("servers.form.panelPasswordPlaceholder");
  }
  if (clearPasswordInput) clearPasswordInput.checked = false;
  if (passwordStatus) {
    passwordStatus.textContent = t("servers.form.passwordNotSaved");
    passwordStatus.classList.remove("active");
  }
  advancedContent?.classList.add("hidden");
  advancedToggle?.setAttribute("aria-expanded", "false");

  if (index >= 0) {
    isEditMode = true;
    currentEditIndex = index;
    currentEditID = "";
    title.textContent = t("servers.form.editTitle");

    const servers = getServers();
    const server = servers[index];
    if (server) {
      currentEditID = String(server.id || "").trim();
      nameInput.value = server.name;
      addressInput.value = server.address;
      weightInput.value = server.weight || 0;
      if (panelUrlInput) panelUrlInput.value = server.panelUrl || "";
      if (panelPasswordInput && server.panelPasswordSet) {
        panelPasswordInput.placeholder = t("servers.form.passwordKeepPlaceholder");
      }
      if (passwordStatus) {
        passwordStatus.textContent = server.panelPasswordSet
          ? t("servers.form.passwordSaved")
          : t("servers.form.passwordNotSaved");
        passwordStatus.classList.toggle("active", Boolean(server.panelPasswordSet));
      }
    }
  } else {
    isEditMode = false;
    currentEditIndex = -1;
    currentEditID = "";
    title.textContent = t("servers.form.addTitle");
  }

  setFormSaving(sessionId, false);
  modal.classList.remove("hidden");
  document.getElementById("global-dropdown").classList.add("hidden");
}

export function closeServerFormModal() {
  formSessionId += 1;
  document.getElementById("server-form-modal").classList.add("hidden");
  currentEditIndex = -1;
  isEditMode = false;
  currentEditID = "";
}

export async function saveServerForm() {
  const sessionId = formSessionId;
  if (savingSessionId === sessionId) return;

  const name = document.getElementById("form-server-name").value.trim();
  const rawAddress = document.getElementById("form-server-address").value.trim();
  const weight =
    parseInt(document.getElementById("form-server-weight").value) || 0;
  const panelUrl = normalizePanelUrl(
    document.getElementById("form-server-panel-url")?.value
  );
  const panelPassword =
    document.getElementById("form-server-panel-password")?.value.trim() || "";
  const clearPanelPassword = Boolean(
    document.getElementById("form-clear-panel-password")?.checked
  );

  if (!name || !rawAddress) {
    showError(t("servers.form.error.nameAndAddressRequired"));
    return;
  }

  // 地址统一走规范化：没写端口补 27015，非法地址在这里就拦下。
  // 与后端 serveraddress.Normalize 同一套规则，避免"手填的"和"协议推进来的"写法不一致。
  let address;
  try {
    address = normalizeServerAddress(rawAddress);
  } catch (error) {
    showError(String(error?.message || error));
    return;
  }

  const editMode = isEditMode;
  const editID = currentEditID;
  const editIndex = currentEditIndex;
  const servers = getServers();
  const buildServerPayload = (existing = {}) => {
    const next = {
      ...existing,
      name,
      address,
      weight,
      panelUrl,
      panelPasswordSet: clearPanelPassword
        ? false
        : Boolean(existing.panelPasswordSet || panelPassword),
    };
    if (panelPassword) {
      next.panelPassword = panelPassword;
      next.clearPanelPassword = false;
    } else if (clearPanelPassword) {
      next.clearPanelPassword = true;
    }
    return next;
  };

  const commit = async () => {
    let savedServerID = editID;
    if (editMode) {
      const targetIndex = editID
        ? servers.findIndex((server) => server.id === editID)
        : editIndex;
      if (targetIndex < 0 || targetIndex >= servers.length) {
        showError(t("servers.form.error.listChanged"));
        return;
      }
      servers[targetIndex] = buildServerPayload(servers[targetIndex]);
      savedServerID = String(servers[targetIndex].id || "").trim();
    } else {
      const newServer = buildServerPayload({ id: createServerID() });
      servers.push(newServer);
      savedServerID = newServer.id;
    }

    savingSessionId = sessionId;
    setFormSaving(sessionId, true);
    try {
      await saveServers(servers);
      // The backend normalizes legacy IDs and removes the plaintext panel password
      // before returning storage to the UI. Reload that authoritative shape only
      // after the ordered write succeeds.
      await initServerStorage();
    } catch (err) {
      console.error("保存服务器失败:", err);
      if (isCurrentFormSession(sessionId)) {
        showError(t("servers.form.error.saveFailed", { error: String(err?.message || err || t("servers.error.unknown")) }));
      }
      return;
    } finally {
      if (savingSessionId === sessionId) {
        savingSessionId = 0;
      }
      setFormSaving(sessionId, false);
    }

    renderServers();
    renderLaunchServerMenu();

    const newServers = getServers();
    const newIndex = newServers.findIndex((server) => server.id === savedServerID);
    if (newIndex !== -1) {
      fetchServerInfo(address, newIndex);
    }
    if (isCurrentFormSession(sessionId)) {
      showNotification(editMode ? t("servers.form.notification.updated") : t("servers.form.notification.added"), "success");
      closeServerFormModal();
    }
  };

  // 同一个地址本来可以无限重复保存（真机联调里同一台测试机存了 3 份：
  // 列表重复显示、每次打开收藏服务器都各查一遍）。这里先提醒一次，
  // 由用户决定要不要真的再存一条。
  const duplicate = findDuplicateServer(servers, address, {
    excludeId: editMode ? editID : "",
    excludeIndex: editMode ? editIndex : -1,
  });
  if (duplicate) {
    const existingLabel = String(duplicate.name || "").trim() || String(duplicate.address || "");
    showConfirmModal(
      t("servers.form.duplicate.title"),
      t("servers.form.duplicate.message", { address, label: existingLabel }),
      () => void commit(),
    );
    return;
  }

  await commit();
}

function toggleServerAdvancedConfig() {
  const content = document.getElementById("server-advanced-content");
  const toggle = document.getElementById("server-advanced-toggle");
  const expanded = content.classList.toggle("hidden") === false;
  toggle.setAttribute("aria-expanded", String(expanded));
}

export function setupFormModalListeners() {
  document
    .getElementById("open-add-server-modal-btn")
    .addEventListener("click", () => openServerFormModal(-1));

  document
    .getElementById("close-server-form-modal-btn")
    .addEventListener("click", closeServerFormModal);
  document
    .getElementById("cancel-server-form-btn")
    .addEventListener("click", closeServerFormModal);
  document
    .getElementById("save-server-form-btn")
    .addEventListener("click", saveServerForm);

  document
    .getElementById("global-edit-server-btn")
    .addEventListener("click", () => {
      const dropdown = document.getElementById("global-dropdown");
      const index = parseInt(dropdown.dataset.index);
      if (!isNaN(index)) {
        openServerFormModal(index);
      }
    });

  document
    .getElementById("server-advanced-toggle")
    ?.addEventListener("click", toggleServerAdvancedConfig);
}
