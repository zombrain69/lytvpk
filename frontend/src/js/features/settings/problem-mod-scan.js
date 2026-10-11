import {
  GetProblemModScanSession,
  LaunchL4D2ForProblemScan,
  RestoreProblemModScan,
  StartProblemModScan,
  SubmitProblemModScanResult,
  ToggleVPKFile,
} from "../../../../wailsjs/go/app/App";
import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { refreshFilesKeepFilter } from "../file-list/filters.js";

let activeSession = null;

export async function initProblemModScanAutoRestore() {
  try {
    const session = await GetProblemModScanSession();
    if (session?.active) {
      renderActiveSession(session, { resumed: true });
    }
  } catch (error) {
    console.warn("恢复问题 Mod 查找会话失败:", error);
  }
}

export async function openProblemModScanIntro() {
  try {
    const session = await GetProblemModScanSession();
    if (session?.active) {
      renderActiveSession(session);
      return;
    }
  } catch (error) {
    console.warn("读取问题 Mod 查找会话失败:", error);
  }

  activeSession = null;
  const modal = getModal();
  setModalMode("intro");
  setTitle(t("settings.problemScan.title"));

  getBody().innerHTML = `
    <div class="problem-scan-intro">
      <div class="problem-scan-hero">
        <div class="problem-scan-hero-icon" aria-hidden="true">${boltIcon()}</div>
        <div>
          <h3>${t("settings.problemScan.intro.heading")}</h3>
          <p>${t("settings.problemScan.intro.body")}</p>
        </div>
      </div>
      <div class="problem-scan-note-grid">
        <section class="problem-scan-note">
          <span class="problem-scan-note-icon">${checkIcon()}</span>
          <div>
            <h4>${t("settings.problemScan.intro.suitable.title")}</h4>
            <p>${t("settings.problemScan.intro.suitable.body")}</p>
          </div>
        </section>
        <section class="problem-scan-note">
          <span class="problem-scan-note-icon">${folderIcon()}</span>
          <div>
            <h4>${t("settings.problemScan.intro.scope.title")}</h4>
            <p>${t("settings.problemScan.intro.scope.body")}</p>
          </div>
        </section>
        <section class="problem-scan-note">
          <span class="problem-scan-note-icon">${alertIcon()}</span>
          <div>
            <h4>${t("settings.problemScan.intro.boundary.title")}</h4>
            <p>${t("settings.problemScan.intro.boundary.body")}</p>
          </div>
        </section>
        <section class="problem-scan-note">
          <span class="problem-scan-note-icon">${lockIcon()}</span>
          <div>
            <h4>${t("settings.problemScan.intro.during.title")}</h4>
            <p>${t("settings.problemScan.intro.during.body")}</p>
          </div>
        </section>
      </div>
      <div class="problem-scan-warning">
        ${t("settings.problemScan.intro.warning")}
      </div>
    </div>
  `;

  getFooter().innerHTML = `
    <button type="button" class="btn btn-secondary" id="problem-scan-intro-cancel">${t("settings.problemScan.intro.cancel")}</button>
    <button type="button" class="btn btn-primary" id="problem-scan-start">${t("settings.problemScan.intro.start")}</button>
  `;

  document.getElementById("problem-scan-intro-cancel")?.addEventListener("click", hideModal);
  document.getElementById("problem-scan-start")?.addEventListener("click", startProblemScan);
  modal.classList.remove("hidden");
}

async function startProblemScan() {
  const startBtn = document.getElementById("problem-scan-start");
  setButtonPending(startBtn, t("settings.problemScan.start.preparing"));
  try {
    const session = await StartProblemModScan();
    await safeRefreshFiles();
    if (session?.status === "found") {
      renderFoundSession(session);
    } else {
      renderActiveSession(session);
    }
  } catch (error) {
    showError(t("settings.problemScan.start.failed", { error }));
    setButtonReady(startBtn, t("settings.problemScan.intro.start"));
  }
}

function renderActiveSession(session, options = {}) {
  activeSession = session;
  const modal = getModal();
  const disabledNames = new Set((session.currentDisabled || []).map((item) => item.name));
  const candidateCount = session.currentCandidates?.length || 0;
  const appliedDisabled = getAppliedDisabled(session);
  const disabledCount = appliedDisabled.length;
  const enabledCandidates = session.currentEnabled || [];

  setModalMode("active");
  setTitle(options.resumed ? t("settings.problemScan.active.titleResumed") : t("settings.problemScan.active.title"));
  getBody().innerHTML = `
    <div class="problem-scan-active">
      <div class="problem-scan-progress">
        <div class="problem-scan-stat">
          <span>${t("settings.problemScan.active.round")}</span>
          <strong>${session.round || 1}</strong>
        </div>
        <div class="problem-scan-stat">
          <span>${t("settings.problemScan.active.candidates")}</span>
          <strong>${candidateCount}</strong>
        </div>
        <div class="problem-scan-stat">
          <span>${t("settings.problemScan.active.disabled")}</span>
          <strong>${disabledCount}</strong>
        </div>
      </div>
      <div class="problem-scan-round-card">
        <div class="problem-scan-round-title">
          <div>
            <h3>${t("settings.problemScan.active.listTitle")}</h3>
            <p id="problem-scan-list-desc">${t("settings.problemScan.active.listDescDefault")}</p>
          </div>
          <button type="button" class="btn btn-small btn-outline problem-scan-toggle-list" id="problem-scan-toggle-candidates">
            ${t("settings.problemScan.active.showAll")}
          </button>
        </div>
        <div class="problem-scan-candidate-list" id="problem-scan-candidate-list">
          ${renderCandidateList(enabledCandidates, disabledNames)}
        </div>
      </div>
      <div class="problem-scan-actions">
        <button type="button" class="btn btn-primary" id="problem-scan-launch">${playIcon()}<span>${t("settings.problemScan.active.launch")}</span></button>
        <button type="button" class="btn btn-outline" id="problem-scan-tested">${t("settings.problemScan.active.tested")}</button>
      </div>
      <div class="problem-scan-feedback hidden" id="problem-scan-feedback">
        <label class="problem-scan-closed-check">
          <input type="checkbox" id="problem-scan-game-closed">
          <span>${t("settings.problemScan.active.gameClosed")}</span>
        </label>
        <div class="problem-scan-result-actions">
          <button type="button" class="btn btn-primary problem-scan-result-btn" data-result="still_exists" disabled>${t("settings.problemScan.active.stillExists")}</button>
          <button type="button" class="btn btn-success problem-scan-result-btn" data-result="gone" disabled>${t("settings.problemScan.active.gone")}</button>
          <button type="button" class="btn btn-secondary" id="problem-scan-exit" disabled>${t("settings.problemScan.active.exit")}</button>
        </div>
      </div>
    </div>
  `;
  getFooter().innerHTML = `<div class="problem-scan-footer-hint">${t("settings.problemScan.active.footerHint")}</div>`;

  document.getElementById("problem-scan-launch")?.addEventListener("click", launchProblemScanGame);
  document.getElementById("problem-scan-tested")?.addEventListener("click", showProblemScanFeedback);
  document.getElementById("problem-scan-toggle-candidates")?.addEventListener("click", () => {
    toggleProblemScanCandidateList(session, disabledNames);
  });
  document.getElementById("problem-scan-game-closed")?.addEventListener("change", syncProblemScanFeedbackButtons);
  document.querySelectorAll(".problem-scan-result-btn").forEach((button) => {
    button.addEventListener("click", () => submitProblemScanResult(button.dataset.result, button));
  });
  document.getElementById("problem-scan-exit")?.addEventListener("click", exitProblemScan);

  modal.classList.remove("hidden");
  if (options.resumed) {
    showNotification(t("settings.problemScan.resumed.notice"), "info");
  }
}

function renderFoundSession(session) {
  activeSession = null;
  const suspect = session.suspiciousMod;
  setModalMode("found");
  setTitle(t("settings.problemScan.found.title"));
  getBody().innerHTML = `
    <div class="problem-scan-found">
      <div class="problem-scan-found-icon">${checkIcon()}</div>
      <h3>${suspect ? escapeHtml(displayName(suspect)) : t("settings.problemScan.found.fallbackName")}</h3>
      ${suspect ? `<p class="problem-scan-found-path">${escapeHtml(suspect.name)}</p>` : ""}
      <p>${t("settings.problemScan.found.body")}</p>
    </div>
  `;
  renderFoundFooter(suspect);
  document.getElementById("problem-scan-finish")?.addEventListener("click", hideModal);
  document.getElementById("problem-scan-disable-suspect")?.addEventListener("click", (event) => {
    disableSuspiciousMod(suspect, event.currentTarget);
  });
  getModal().classList.remove("hidden");
}

function renderFoundFooter(suspect) {
  const footer = getFooter();
  footer.replaceChildren();

  if (suspect?.path) {
    const disableBtn = document.createElement("button");
    disableBtn.type = "button";
    disableBtn.className = "btn btn-danger";
    disableBtn.id = "problem-scan-disable-suspect";
    disableBtn.textContent = t("settings.problemScan.found.disable");
    footer.appendChild(disableBtn);
  }

  const finishBtn = document.createElement("button");
  finishBtn.type = "button";
  finishBtn.className = "btn btn-primary";
  finishBtn.id = "problem-scan-finish";
  finishBtn.textContent = t("settings.problemScan.found.finish");
  footer.appendChild(finishBtn);
}

function toggleProblemScanCandidateList(session, disabledNames) {
  const list = document.getElementById("problem-scan-candidate-list");
  const button = document.getElementById("problem-scan-toggle-candidates");
  const desc = document.getElementById("problem-scan-list-desc");
  if (!list || !button) return;

  const showingAll = list.dataset.showingAll === "true";
  if (showingAll) {
    list.dataset.showingAll = "false";
    list.innerHTML = renderCandidateList(session.currentEnabled || [], disabledNames);
    button.textContent = t("settings.problemScan.active.showAll");
    if (desc) desc.textContent = t("settings.problemScan.active.listDescDefault");
  } else {
    list.dataset.showingAll = "true";
    list.innerHTML = renderCandidateList(session.currentCandidates || [], disabledNames);
    button.textContent = t("settings.problemScan.active.showEnabledOnly");
    if (desc) desc.textContent = t("settings.problemScan.active.listDescAll");
  }
}

function renderCandidateList(items, disabledNames) {
  if (!items || items.length === 0) {
    return `<div class="problem-scan-empty-list">${t("settings.problemScan.emptyList")}</div>`;
  }
  return items.map((item) => renderCandidateItem(item, disabledNames.has(item.name))).join("");
}

function renderCandidateItem(item, disabled) {
  const tags = [item.primaryTag, ...(item.secondaryTags || [])].filter(Boolean).slice(0, 4);
  return `
    <div class="problem-scan-candidate ${disabled ? "is-disabled-round" : ""}">
      <div class="problem-scan-candidate-main">
        <span class="problem-scan-candidate-name">${escapeHtml(displayName(item))}</span>
        <span class="problem-scan-candidate-file">${escapeHtml(item.name || "")}</span>
      </div>
      <div class="problem-scan-candidate-meta">
        ${tags.map((tag) => `<span>${escapeHtml(tag)}</span>`).join("")}
        <strong>${disabled ? t("settings.problemScan.candidate.disabledRound") : t("settings.problemScan.candidate.enabledRound")}</strong>
      </div>
    </div>
  `;
}

async function launchProblemScanGame() {
  try {
    await LaunchL4D2ForProblemScan();
    showNotification(t("settings.problemScan.launching"), "success");
  } catch (error) {
    showError(t("settings.problemScan.launchFailed", { error }));
  }
}

function showProblemScanFeedback() {
  document.querySelector(".problem-scan-active")?.classList.add("is-feedback-open");
  const feedback = document.getElementById("problem-scan-feedback");
  feedback?.classList.remove("hidden");
  document.getElementById("problem-scan-tested")?.setAttribute("disabled", "true");
}

function syncProblemScanFeedbackButtons() {
  const checked = Boolean(document.getElementById("problem-scan-game-closed")?.checked);
  document.querySelectorAll(".problem-scan-result-btn").forEach((button) => {
    button.disabled = !checked;
  });
  const exitBtn = document.getElementById("problem-scan-exit");
  if (exitBtn) exitBtn.disabled = !checked;
}

async function submitProblemScanResult(result, button) {
  if (!activeSession) return;
  setButtonPending(button, t("settings.problemScan.submit.processing"));
  try {
    const session = await SubmitProblemModScanResult(result);
    await safeRefreshFiles();
    if (session?.status === "found") {
      renderFoundSession(session);
    } else {
      renderActiveSession(session);
    }
  } catch (error) {
    showError(t("settings.problemScan.submit.failed", { error }));
    renderActiveSession(activeSession);
  }
}

async function disableSuspiciousMod(suspect, button) {
  if (!suspect?.path) {
    showError(t("settings.problemScan.disable.missingPath"));
    return;
  }

  setButtonPending(button, t("settings.problemScan.disable.pending"));
  try {
    await ToggleVPKFile(suspect.path);
    await safeRefreshFiles();

    if (button) {
      button.classList.remove("btn-danger");
      button.classList.add("btn-secondary");
      button.textContent = t("settings.problemScan.found.disableDone");
      button.disabled = true;
    }

    const foundBody = document.querySelector(".problem-scan-found");
    if (foundBody && !foundBody.querySelector(".problem-scan-found-status")) {
      const status = document.createElement("p");
      status.className = "problem-scan-found-status";
      status.textContent = t("settings.problemScan.found.disabledNote");
      foundBody.appendChild(status);
    }

    showNotification(t("settings.problemScan.found.disableDone"), "success");
  } catch (error) {
    showError(t("settings.problemScan.disable.failed", { error }));
    setButtonReady(button, t("settings.problemScan.found.disable"));
  }
}

async function exitProblemScan() {
  const exitBtn = document.getElementById("problem-scan-exit");
  setButtonPending(exitBtn, t("settings.problemScan.exit.pending"));
  try {
    await RestoreProblemModScan();
    await safeRefreshFiles();
    hideModal();
    showNotification(t("settings.problemScan.exit.done"), "success");
  } catch (error) {
    showError(t("settings.problemScan.exit.failed", { error }));
    setButtonReady(exitBtn, t("settings.problemScan.active.exit"));
  }
}

async function safeRefreshFiles() {
  try {
    await refreshFilesKeepFilter();
  } catch (error) {
    console.warn("刷新文件列表失败:", error);
  }
}

function getModal() {
  return document.getElementById("problem-scan-modal");
}

function getBody() {
  return document.getElementById("problem-scan-body");
}

function getFooter() {
  return document.getElementById("problem-scan-footer");
}

function setTitle(title) {
  const titleEl = document.getElementById("problem-scan-title");
  if (titleEl) titleEl.textContent = title;
}

function setModalMode(mode) {
  const modal = getModal();
  modal.dataset.mode = mode;
  modal.classList.toggle("problem-scan-locked", mode === "active");
}

function hideModal() {
  getModal()?.classList.add("hidden");
}

function setButtonPending(button, label) {
  if (!button) return;
  button.disabled = true;
  button.dataset.originalText = button.textContent;
  button.innerHTML = `<span class="btn-spinner"></span>${escapeHtml(label)}`;
}

function setButtonReady(button, label) {
  if (!button) return;
  button.disabled = false;
  button.textContent = label || button.dataset.originalText || t("settings.problemScan.button.confirm");
}

function displayName(item) {
  return item?.title && item.title !== item.name ? item.title : item?.name || t("settings.problemScan.display.unknownMod");
}

function getAppliedDisabled(session) {
  if (Array.isArray(session?.appliedDisabled) && session.appliedDisabled.length > 0) {
    return session.appliedDisabled;
  }
  return session?.currentDisabled || [];
}

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function boltIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2 4 14h7l-1 8 10-12h-7l1-8z"/></svg>`;
}

function checkIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`;
}

function folderIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7h7l2 2h9v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z"/></svg>`;
}

function alertIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.3 3.6 2.5 18a2 2 0 0 0 1.8 3h15.4a2 2 0 0 0 1.8-3L13.7 3.6a2 2 0 0 0-3.4 0z"/></svg>`;
}

function lockIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>`;
}

function playIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polygon points="5 3 19 12 5 21 5 3"/></svg>`;
}
