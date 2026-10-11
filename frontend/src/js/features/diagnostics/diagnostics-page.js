// The toolbox starts with an asynchronous problem-Mod session read.  Keep a
// render identity so a slower earlier navigation cannot replace a newer page.
import { t } from "../../core/i18n-runtime.js";

let diagnosticsRenderGeneration = 0;

export async function renderDiagnosticsPage({
  GetProblemModScanSession,
  openProblemModScanIntro,
  openModelStatsScanModal,
  showConflictModal,
  openVPKUnpackTool,
  openVPKIntegrityTool,
  openMDMPReportTool,
  openVPKPackTool,
  openArchiveManager,
  openSprayTool,
  openAutoexecTool,
  openModSnapshotTool,
  openVPKMergeTool,
  openVPKPreviewTool,
  refreshFilesKeepFilter,
} = {}) {
  const container = document.getElementById("diagnostics-page-content");
  if (!container) return;
  const renderGeneration = ++diagnosticsRenderGeneration;
  const pageView = container.closest(".page-view");

  let problemScanSession = null;
  try {
    problemScanSession = GetProblemModScanSession
      ? await GetProblemModScanSession()
      : null;
  } catch (error) {
    console.warn("读取问题 Mod 查找状态失败:", error);
  }

  if (
    renderGeneration !== diagnosticsRenderGeneration ||
    !container.isConnected ||
    (pageView && !pageView.classList.contains("active"))
  ) return;

  const problemScanActive = Boolean(problemScanSession?.active);

  container.innerHTML = `
    <div class="diagnostics-page-shell toolbox-page-shell">
      <div class="diagnostics-page-header">
        <div>
          <h2>${t("toolbox.title")}</h2>
          <p>${t("toolbox.subtitle")}</p>
        </div>
      </div>

      <section class="toolbox-section">
        <div class="toolbox-section-header">
          <h3>${t("toolbox.section.diagnostics.title")}</h3>
          <p>${t("toolbox.section.diagnostics.description")}</p>
        </div>
        <div class="diagnostics-tool-grid">
          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon">${boltIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.problemScan.title")}</h3>
                <span class="diagnostics-status ${problemScanActive ? "is-active" : ""}">
                  ${problemScanActive ? t("toolbox.card.problemScan.statusActive") : t("toolbox.card.problemScan.statusIdle")}
                </span>
              </div>
              <p>${t("toolbox.card.problemScan.description")}</p>
              ${
                problemScanActive
                  ? `<div class="diagnostics-inline-status">${t("toolbox.card.problemScan.roundStatus", { round: problemScanSession.round || 1, count: problemScanSession.currentCandidates?.length || 0 })}</div>`
                  : ""
              }
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="diagnostics-problem-scan-btn">
              ${problemScanActive ? t("toolbox.card.problemScan.actionActive") : t("toolbox.card.problemScan.actionIdle")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-warning">${conflictIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.conflictCheck.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.card.conflictCheck.status")}</span>
              </div>
              <p>${t("toolbox.card.conflictCheck.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="diagnostics-conflict-check-btn">
              ${t("toolbox.card.conflictCheck.action")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-model">${modelIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.modelStats.title")} <span class="diagnostics-beta-badge">Beta</span></h3>
                <span class="diagnostics-status">${t("toolbox.card.modelStats.status")}</span>
              </div>
              <p>${t("toolbox.card.modelStats.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="diagnostics-model-stats-btn">
              ${t("toolbox.card.modelStats.action")}
            </button>
          </section>
        </div>
      </section>

      <section class="toolbox-section">
        <div class="toolbox-section-header">
          <h3>${t("toolbox.section.gameConfig.title")}</h3>
          <p>${t("toolbox.section.gameConfig.description")}</p>
        </div>
        <div class="diagnostics-tool-grid">
          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-config">${configIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.autoexec.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.card.autoexec.status")}</span>
              </div>
              <p>${t("toolbox.card.autoexec.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-autoexec-btn">
              ${t("toolbox.card.autoexec.action")}
            </button>
          </section>
        </div>
      </section>

      <section class="toolbox-section">
        <div class="toolbox-section-header">
          <h3>${t("toolbox.section.generalTools.title")}</h3>
          <p>${t("toolbox.section.generalTools.description")}</p>
        </div>
        <div class="diagnostics-tool-grid">
          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-general">${unpackIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.vpkUnpack.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.common.statusAvailable")}</span>
              </div>
              <p>${t("toolbox.card.vpkUnpack.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-vpk-unpack-btn">
              ${t("toolbox.card.vpkUnpack.action")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-general">${packIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.vpkPack.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.common.statusAvailable")}</span>
              </div>
              <p>${t("toolbox.card.vpkPack.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-vpk-pack-btn">
              ${t("toolbox.card.vpkPack.action")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-integrity">${integrityIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.vpkIntegrity.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.card.vpkIntegrity.status")}</span>
              </div>
              <p>${t("toolbox.card.vpkIntegrity.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-vpk-integrity-btn">
              ${t("toolbox.card.vpkIntegrity.action")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-archive">${archiveIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.archiveManager.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.card.archiveManager.status")}</span>
              </div>
              <p>${t("toolbox.card.archiveManager.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-archive-manager-btn">
              ${t("toolbox.card.archiveManager.action")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-integrity">${integrityIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.snapshot.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.card.snapshot.status")}</span>
              </div>
              <p>${t("toolbox.card.snapshot.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-mod-snapshot-btn">
              ${t("toolbox.card.snapshot.action")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-pack">${packIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.vpkMerge.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.card.vpkMerge.status")}</span>
              </div>
              <p>${t("toolbox.card.vpkMerge.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-vpk-merge-btn">
              ${t("toolbox.card.vpkMerge.action")}
            </button>
          </section>

          <section class="diagnostics-tool-card">
            <div class="diagnostics-tool-icon is-unpack">${unpackIcon()}</div>
            <div class="diagnostics-tool-main">
              <div class="diagnostics-tool-title-row">
                <h3>${t("toolbox.card.vpkPreview.title")}</h3>
                <span class="diagnostics-status">${t("toolbox.card.vpkPreview.status")}</span>
              </div>
              <p>${t("toolbox.card.vpkPreview.description")}</p>
            </div>
            <button type="button" class="btn btn-primary diagnostics-tool-action" id="toolbox-vpk-preview-btn">
              ${t("toolbox.card.vpkPreview.action")}
            </button>
          </section>
        </div>
      </section>
    </div>
  `;

  appendMDMPReportTool(container, openMDMPReportTool);
  appendSprayTool(container, openSprayTool, refreshFilesKeepFilter);

  document
    .getElementById("toolbox-autoexec-btn")
    ?.addEventListener("click", () => openAutoexecTool?.());

  document
    .getElementById("diagnostics-problem-scan-btn")
    ?.addEventListener("click", () => {
      openProblemModScanIntro?.();
    });

  document
    .getElementById("diagnostics-conflict-check-btn")
    ?.addEventListener("click", () => {
      showConflictModal?.();
    });

  document
    .getElementById("diagnostics-model-stats-btn")
    ?.addEventListener("click", () => {
      openModelStatsScanModal?.();
    });

  document
    .getElementById("toolbox-mod-snapshot-btn")
    ?.addEventListener("click", () => {
      openModSnapshotTool?.();
    });

  document
    .getElementById("toolbox-vpk-merge-btn")
    ?.addEventListener("click", () => {
      openVPKMergeTool?.();
    });

  document
    .getElementById("toolbox-vpk-preview-btn")
    ?.addEventListener("click", () => {
      openVPKPreviewTool?.();
    });

  document
    .getElementById("toolbox-vpk-unpack-btn")
    ?.addEventListener("click", () => {
      openVPKUnpackTool?.();
    });

  document
    .getElementById("toolbox-vpk-pack-btn")
    ?.addEventListener("click", () => {
      openVPKPackTool?.({ refreshFilesKeepFilter });
    });

  document
    .getElementById("toolbox-vpk-integrity-btn")
    ?.addEventListener("click", () => {
      openVPKIntegrityTool?.();
    });

  document
    .getElementById("toolbox-archive-manager-btn")
    ?.addEventListener("click", () => openArchiveManager?.());
}

function boltIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2 4 14h7l-1 8 10-12h-7l1-8z"/></svg>`;
}

function conflictIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M10.3 3.6 2.5 18a2 2 0 0 0 1.8 3h15.4a2 2 0 0 0 1.8-3L13.7 3.6a2 2 0 0 0-3.4 0z"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>`;
}

function modelIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3 4 7.2v9.6L12 21l8-4.2V7.2L12 3Z"/><path d="m4 7.2 8 4.2 8-4.2"/><path d="M12 11.4V21"/><path d="m8.2 5.2 8 4.2"/></svg>`;
}

function unpackIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/></svg>`;
}

function archiveIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M4 4h16v16H4z"/><path d="M8 4v4h8V4M8 12h8M8 16h5"/></svg>`;
}

function appendMDMPReportTool(container, openMDMPReportTool) {
  const grids = container.querySelectorAll(".diagnostics-tool-grid");
  const diagnosticsGrid = grids[0];
  if (!diagnosticsGrid) return;

  const card = document.createElement("section");
  card.className = "diagnostics-tool-card";

  const icon = document.createElement("div");
  icon.className = "diagnostics-tool-icon is-dump";
  icon.appendChild(createDumpIcon());

  const main = document.createElement("div");
  main.className = "diagnostics-tool-main";
  const row = document.createElement("div");
  row.className = "diagnostics-tool-title-row";
  const title = document.createElement("h3");
  title.textContent = t("toolbox.card.mdmp.title");
  const status = document.createElement("span");
  status.className = "diagnostics-status";
  status.textContent = t("toolbox.card.mdmp.status");
  row.append(title, status);
  const desc = document.createElement("p");
  desc.textContent = t("toolbox.card.mdmp.description");
  main.append(row, desc);

  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-primary diagnostics-tool-action";
  button.textContent = t("toolbox.card.mdmp.action");
  button.addEventListener("click", () => openMDMPReportTool?.());

  card.append(icon, main, button);
  diagnosticsGrid.appendChild(card);
}

function appendSprayTool(container, openSprayTool, refreshFilesKeepFilter) {
  const grids = container.querySelectorAll(".diagnostics-tool-grid");
  const generalGrid = grids[1];
  if (!generalGrid) return;

  const card = document.createElement("section");
  card.className = "diagnostics-tool-card";

  const icon = document.createElement("div");
  icon.className = "diagnostics-tool-icon is-spray";
  icon.appendChild(createSprayIcon());

  const main = document.createElement("div");
  main.className = "diagnostics-tool-main";
  const row = document.createElement("div");
  row.className = "diagnostics-tool-title-row";
  const title = document.createElement("h3");
  title.textContent = t("toolbox.card.spray.title");
  const status = document.createElement("span");
  status.className = "diagnostics-status";
  status.textContent = t("toolbox.card.spray.status");
  row.append(title, status);
  const desc = document.createElement("p");
  desc.textContent = t("toolbox.card.spray.description");
  main.append(row, desc);

  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-primary diagnostics-tool-action";
  button.textContent = t("toolbox.card.spray.action");
  button.addEventListener("click", () =>
    openSprayTool?.({ refreshFilesKeepFilter })
  );

  card.append(icon, main, button);
  generalGrid.appendChild(card);
}

function createDumpIcon() {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("class", "icon-svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2.3");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  [
    [
      "path",
      { d: "M14 2H7a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7z" },
    ],
    ["path", { d: "M14 2v5h5" }],
    ["path", { d: "M8 13h8" }],
    ["path", { d: "M8 17h5" }],
    ["path", { d: "M9 9h1" }],
  ].forEach(([tag, attrs]) => {
    const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
    Object.entries(attrs).forEach(([key, value]) =>
      node.setAttribute(key, value)
    );
    svg.appendChild(node);
  });
  return svg;
}

function createSprayIcon() {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("class", "icon-svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2.3");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  [
    ["path", { d: "M7 3h7l3 3v5H7z" }],
    ["path", { d: "M9 11v3" }],
    ["path", { d: "M15 11v3" }],
    ["path", { d: "M8 14h8v7H8z" }],
    ["path", { d: "M10 18h4" }],
  ].forEach(([tag, attrs]) => {
    const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
    Object.entries(attrs).forEach(([key, value]) =>
      node.setAttribute(key, value)
    );
    svg.appendChild(node);
  });
  return svg;
}

function packIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2v8"/><path d="m9 7 3 3 3-3"/><path d="M3 14h18"/><path d="M5 14v5a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-5"/></svg>`;
}

function integrityIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3 20 6v5c0 5-3.4 8.7-8 10-4.6-1.3-8-5-8-10V6l8-3z"></path><path d="m8.5 12 2.2 2.2 4.8-5"></path></svg>`;
}

function configIcon() {
  return `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.3" stroke-linecap="round" stroke-linejoin="round"><path d="M5 3h10l4 4v14H5z"/><path d="M15 3v5h5"/><path d="M8 12h8M8 16h6"/></svg>`;
}
