import { normalizePanelUrl } from "./panel-url.js";

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
    panelPasswordInput.placeholder = "面板访问密码";
  }
  if (clearPasswordInput) clearPasswordInput.checked = false;
  if (passwordStatus) {
    passwordStatus.textContent = "未保存密码";
    passwordStatus.classList.remove("active");
  }
  advancedContent?.classList.add("hidden");
  advancedToggle?.setAttribute("aria-expanded", "false");

  if (index >= 0) {
    isEditMode = true;
    currentEditIndex = index;
    currentEditID = "";
    title.textContent = "编辑服务器";

    const servers = getServers();
    const server = servers[index];
    if (server) {
      currentEditID = String(server.id || "").trim();
      nameInput.value = server.name;
      addressInput.value = server.address;
      weightInput.value = server.weight || 0;
      if (panelUrlInput) panelUrlInput.value = server.panelUrl || "";
      if (panelPasswordInput && server.panelPasswordSet) {
        panelPasswordInput.placeholder = "留空则保留已保存密码";
      }
      if (passwordStatus) {
        passwordStatus.textContent = server.panelPasswordSet
          ? "已保存密码"
          : "未保存密码";
        passwordStatus.classList.toggle("active", Boolean(server.panelPasswordSet));
      }
    }
  } else {
    isEditMode = false;
    currentEditIndex = -1;
    currentEditID = "";
    title.textContent = "添加服务器";
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
  const address = document.getElementById("form-server-address").value.trim();
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

  if (!name || !address) {
    showError("请输入服务器名称和地址");
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

  let savedServerID = editID;
  if (editMode) {
    const targetIndex = editID
      ? servers.findIndex((server) => server.id === editID)
      : editIndex;
    if (targetIndex < 0 || targetIndex >= servers.length) {
      showError("服务器列表已变化，请重新打开编辑窗口");
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
      showError("保存服务器失败: " + String(err?.message || err || "未知错误"));
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
    showNotification(editMode ? "服务器修改成功" : "服务器添加成功", "success");
    closeServerFormModal();
  }
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
