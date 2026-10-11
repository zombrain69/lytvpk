import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import {
  DEFAULT_SPRAY_OPTIONS,
  FORMAT_LABELS,
  TEXTURE_FORMATS,
  buildSprayOutputs,
  decodeFilesAsBatch,
  decodeFilesAsProject,
  decodeMipmapOverride,
  drawFrameToCanvas,
  estimateVTFSize,
  getMipmapDimensions,
  resolveOutputSize,
  sanitizeOutputName,
  shouldShortenOriginalVTF,
  shouldUseOriginalMipmaps,
} from "./vtf.js";

const RESOLUTION_PRESETS = [1024, 768, 720, 512, 256, 128, 64, 32, 16, 8, 4];
const ACCEPTED_FILES = "image/*,.tga";
const SPRAY_IMPORT_EXTENSIONS = new Set([
  ".png",
  ".jpg",
  ".jpeg",
  ".webp",
  ".bmp",
  ".gif",
  ".tga",
]);

const state = {
  projects: [],
  activeProjectId: null,
  options: { ...DEFAULT_SPRAY_OPTIONS },
  frameIndex: 0,
  playing: false,
  timer: null,
  busy: false,
  refreshFilesKeepFilter: null,
};

let refs = {};

export function openSprayTool({ refreshFilesKeepFilter } = {}) {
  state.refreshFilesKeepFilter = refreshFilesKeepFilter || null;
  ensureSprayModal();
  refs.modal.classList.remove("hidden");
  renderAll();
  updatePreview();
}

export function isSprayImportPath(path) {
  const ext = getPathExtension(path);
  return SPRAY_IMPORT_EXTENSIONS.has(ext);
}

export function isSprayImportFile(file) {
  if (!file) return false;
  const type = String(file.type || "").toLowerCase();
  return type.startsWith("image/") || isSprayImportPath(file.name);
}

export async function importSprayFiles(files = [], { refreshFilesKeepFilter } = {}) {
  const sprayFiles = Array.from(files || []).filter(isSprayImportFile);
  if (!sprayFiles.length) return false;

  openSprayTool({ refreshFilesKeepFilter });
  await importPrimaryFiles(sprayFiles);
  return true;
}

export async function importSprayPaths(paths = [], { refreshFilesKeepFilter } = {}) {
  const sprayPaths = Array.from(paths || []).filter(isSprayImportPath);
  if (!sprayPaths.length) return false;

  openSprayTool({ refreshFilesKeepFilter });
  setBusy(true, t("toolbox.spray.busy.readDropped"));
  try {
    const payloads = await callApp("LoadSprayImportFiles", sprayPaths);
    const files = sprayPayloadsToFiles(payloads || []);
    await importPrimaryFiles(files);
    return true;
  } catch (error) {
    showError(t("toolbox.spray.import.failedDropped", { error: formatError(error) }));
    return false;
  } finally {
    setBusy(false);
  }
}

function ensureSprayModal() {
  let modal = document.getElementById("spray-tool-modal");
  if (modal) {
    refs.modal = modal;
    collectRefs();
    return;
  }

  modal = el("div", "modal hidden spray-tool-modal");
  modal.id = "spray-tool-modal";
  modal.appendChild(createModalContent());
  document.body.appendChild(modal);
  refs.modal = modal;
  collectRefs();
  bindStaticEvents();
}

function createModalContent() {
  const content = el("div", "modal-content spray-tool-content");

  const header = el("div", "modal-header spray-tool-header");
  const titleWrap = el("div", "spray-tool-title-wrap");
  titleWrap.append(
    el("h3", "", t("toolbox.card.spray.title")),
    el("p", "", t("toolbox.spray.modal.subtitle"))
  );
  const closeBtn = button("spray-tool-close", "close-btn", "×");
  closeBtn.setAttribute("aria-label", t("toolbox.common.close"));
  header.append(titleWrap, closeBtn);

  const body = el("div", "modal-body spray-tool-body");
  body.append(createImportPanel(), createPreviewPanel(), createSettingsPanel());

  const footer = el("div", "modal-footer spray-tool-footer");
  const status = el("div", "spray-tool-footer-status");
  status.id = "spray-footer-status";
  const actions = el("div", "spray-tool-footer-actions");
  actions.append(
    button("spray-save-vtf-btn", "btn btn-primary", t("toolbox.spray.action.saveVtf")),
    button("spray-save-vmt-btn", "btn btn-secondary", t("toolbox.spray.action.saveVmt"))
  );
  footer.append(status, actions);

  content.append(header, body, footer);
  return content;
}

function createImportPanel() {
  const panel = el("section", "spray-panel spray-import-panel");
  panel.append(panelTitle(t("toolbox.spray.import.panelTitle"), t("toolbox.spray.import.panelSubtitle")));

  const primaryBtn = button("spray-primary-import-btn", "btn btn-primary", t("toolbox.spray.import.select"));
  const primaryInput = input("file", "spray-primary-input");
  primaryInput.accept = ACCEPTED_FILES;
  primaryInput.multiple = true;
  primaryInput.classList.add("hidden");

  const batchBtn = button("spray-batch-import-btn", "btn btn-secondary", t("toolbox.spray.import.batch"));
  const batchInput = input("file", "spray-batch-input");
  batchInput.accept = ACCEPTED_FILES;
  batchInput.multiple = true;
  batchInput.classList.add("hidden");

  const buttonRow = el("div", "spray-import-buttons");
  buttonRow.append(primaryBtn, batchBtn, primaryInput, batchInput);

  const list = el("div", "spray-project-list");
  list.id = "spray-project-list";

  panel.append(buttonRow, list);
  return panel;
}

function createPreviewPanel() {
  const panel = el("section", "spray-panel spray-preview-panel");
  const header = panelTitle(t("toolbox.spray.preview.panelTitle"), t("toolbox.spray.preview.panelSubtitle"));
  const meta = el("div", "spray-preview-meta");
  meta.id = "spray-preview-meta";
  header.appendChild(meta);

  const stage = el("div", "spray-preview-stage");
  const canvas = document.createElement("canvas");
  canvas.id = "spray-preview-canvas";
  canvas.width = 512;
  canvas.height = 512;
  stage.appendChild(canvas);

  const controls = el("div", "spray-playback-controls");
  controls.append(
    button("spray-prev-frame-btn", "btn btn-icon", "‹"),
    button("spray-play-btn", "btn btn-secondary", t("toolbox.spray.preview.play")),
    button("spray-next-frame-btn", "btn btn-icon", "›"),
    el("span", "spray-frame-label", "0 / 0")
  );
  controls.querySelector(".spray-frame-label").id = "spray-frame-label";

  const frameStrip = el("div", "spray-frame-strip");
  frameStrip.id = "spray-frame-strip";

  panel.append(header, stage, controls, frameStrip);
  return panel;
}

function createSettingsPanel() {
  const panel = el("section", "spray-panel spray-settings-panel");
  panel.append(panelTitle(t("toolbox.spray.settings.panelTitle"), t("toolbox.spray.settings.panelSubtitle")));

  const form = el("div", "spray-settings-form");
  const sizeNote = el("div", "spray-size-note", "");
  sizeNote.id = "spray-size-note";
  sizeNote.hidden = true;
  form.append(
    textControl(t("toolbox.spray.settings.name"), "spray-project-name", "", t("toolbox.spray.settings.namePlaceholder")),
    dimensionControl(t("toolbox.spray.settings.width"), "spray-width-value"),
    dimensionControl(t("toolbox.spray.settings.height"), "spray-height-value"),
    checkboxControl(t("toolbox.spray.settings.rescale"), "spray-rescale", true),
    checkboxControl(t("toolbox.spray.settings.mipmaps"), "spray-mipmaps", false),
    createMipmapControls(),
    checkboxControl(t("toolbox.spray.settings.singleFrame"), "spray-single-frame", false),
    numberControl(t("toolbox.spray.settings.singleFrameCount"), "spray-single-frame-count", 8, 2, 64),
    formatControl(),
    selectControl(t("toolbox.spray.settings.sampling"), "spray-sampling", [
      ["0", t("toolbox.spray.settings.samplingDefault")],
      ["1", t("toolbox.spray.settings.samplingPoint")],
      ["2", t("toolbox.spray.settings.samplingTrilinear")],
      ["16", t("toolbox.spray.settings.samplingAniso")],
    ]),
    checkboxControl(t("toolbox.spray.settings.dither"), "spray-dither", true),
    selectControl(t("toolbox.spray.settings.dxtQuality"), "spray-dxt-quality", [
      ["0", t("toolbox.spray.settings.dxtFast")],
      ["1", t("toolbox.spray.settings.dxtNormal")],
      ["2", t("toolbox.spray.settings.dxtSlow")],
      ["3", t("toolbox.spray.settings.dxtVerySlow")],
    ]),
    sizeNote
  );
  panel.appendChild(form);
  return panel;
}

function createMipmapControls() {
  const wrap = el("div", "spray-mipmap-controls");
  const row = el("div", "spray-control spray-control-inline");
  const label = el("label", "", t("toolbox.spray.settings.mipLevel"));
  label.setAttribute("for", "spray-mipmap-level");
  row.append(label, customSelectInput("spray-mipmap-level", []));

  const actions = el("div", "spray-mipmap-actions");
  const inputEl = input("file", "spray-mipmap-input");
  inputEl.accept = ACCEPTED_FILES;
  inputEl.multiple = true;
  inputEl.classList.add("hidden");
  actions.append(
    button("spray-mipmap-import-btn", "btn btn-secondary", t("toolbox.spray.settings.mipReplace")),
    button("spray-mipmap-clear-btn", "btn btn-secondary", t("toolbox.spray.settings.mipClear")),
    inputEl
  );
  wrap.append(row, actions);
  return wrap;
}

function formatControl() {
  const pairs = [
    [TEXTURE_FORMATS.RGBA8888, "RGBA8888"],
    [TEXTURE_FORMATS.RGB888, "RGB888"],
    [TEXTURE_FORMATS.RGB565, "RGB565"],
    [TEXTURE_FORMATS.BGRA5551, "BGRA5551"],
    [TEXTURE_FORMATS.BGRA4444, "BGRA4444"],
    [TEXTURE_FORMATS.DXT1, t("toolbox.spray.settings.formatDxt1")],
    [TEXTURE_FORMATS.DXT5, t("toolbox.spray.settings.formatDxt5")],
  ];
  return selectControl(t("toolbox.spray.settings.format"), "spray-format", pairs);
}

function bindStaticEvents() {
  refs.closeBtn.addEventListener("click", closeSprayTool);
  refs.modal.addEventListener("click", (event) => {
    if (event.target === refs.modal) closeSprayTool();
  });
  refs.settingsPanel?.addEventListener("scroll", () => closeSprayDropdowns());

  refs.primaryImportBtn.addEventListener("click", () => refs.primaryInput.click());
  refs.batchImportBtn.addEventListener("click", () => refs.batchInput.click());
  refs.primaryInput.addEventListener("change", async () => {
    await importPrimaryFiles(refs.primaryInput.files);
    refs.primaryInput.value = "";
  });
  refs.batchInput.addEventListener("change", async () => {
    await importBatchFiles(refs.batchInput.files);
    refs.batchInput.value = "";
  });

  refs.prevBtn.addEventListener("click", () => moveFrame(-1));
  refs.nextBtn.addEventListener("click", () => moveFrame(1));
  refs.playBtn.addEventListener("click", togglePlayback);
  refs.saveVTFBtn.addEventListener("click", saveSprayVTF);
  refs.saveVMTBtn.addEventListener("click", saveSprayVMT);
  refs.mipmapImportBtn.addEventListener("click", () => refs.mipmapInput.click());
  refs.mipmapInput.addEventListener("change", async () => {
    await importMipmapOverride(refs.mipmapInput.files);
    refs.mipmapInput.value = "";
  });
  refs.mipmapClearBtn.addEventListener("click", clearMipmapOverride);
  document.addEventListener("click", (event) => {
    if (!event.target.closest(".spray-select-dropdown, .spray-combo-field, .spray-select-menu")) {
      closeSprayDropdowns();
    }
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") closeSprayDropdowns();
  });

  bindOptionControls();
}

function bindOptionControls() {
  refs.projectName.addEventListener("input", () => {
    const project = activeProject();
    if (project) {
      project.name = refs.projectName.value;
      renderProjectList();
      updateFooter();
    }
  });
  bindDimensionControl("width", refs.widthValue);
  bindDimensionControl("height", refs.heightValue);

  const optionBindings = [
    ["rescale", refs.rescale, "checked"],
    ["mipmaps", refs.mipmaps, "checked"],
    ["singleFrameAnimation", refs.singleFrame, "checked"],
    ["singleFrameCount", refs.singleFrameCount, "number"],
    ["format", refs.format, "number"],
    ["sampling", refs.sampling, "number"],
    ["dither", refs.dither, "checked"],
    ["dxtQuality", refs.dxtQuality, "number"],
  ];

  optionBindings.forEach(([key, control, kind]) => {
    control.addEventListener("change", () => {
      if (kind === "checked") state.options[key] = control.checked;
      else if (kind === "number") state.options[key] = Number(control.value);
      else state.options[key] = control.value;
      enforceMipmapSizeLimit(key === "mipmaps");
      renderMipmapLevels();
      updatePreview();
    });
    control.addEventListener("input", () => {
      if (kind === "number") {
        state.options[key] = Number(control.value);
        updatePreview();
      }
    });
  });

  refs.mipmapLevel.addEventListener("change", () => {
    renderMipmapControlsState();
  });
}

function bindDimensionControl(axis, control) {
  control.addEventListener("input", () => {
    if (!applyDimensionValue(axis, control.value, false)) return;
    enforceMipmapSizeLimit(false);
    renderMipmapLevels();
    updatePreview();
  });
  control.addEventListener("change", () => {
    if (applyDimensionValue(axis, control.value, true)) {
      enforceMipmapSizeLimit(false);
      renderMipmapLevels();
      updatePreview();
    }
    control.value = dimensionValueFromOptions(axis);
  });
}

function applyDimensionValue(axis, rawValue, normalize) {
  const modeKey = `${axis}Mode`;
  const customKey = axis === "width" ? "customWidth" : "customHeight";
  const value = String(rawValue || "").trim();
  if (!value || value === t("toolbox.spray.dimension.auto") || value.toLowerCase() === "auto") {
    state.options[modeKey] = "auto";
    return true;
  }

  const numeric = Number(value.replace(/px$/i, ""));
  if (!Number.isFinite(numeric)) return false;

  const clamped = Math.max(4, Math.min(1024, Math.round(numeric)));
  state.options[modeKey] = String(clamped);
  state.options[customKey] = clamped;
  if (normalize) {
    const control = axis === "width" ? refs.widthValue : refs.heightValue;
    control.value = String(clamped);
  }
  return true;
}

function dimensionValueFromOptions(axis) {
  const modeKey = `${axis}Mode`;
  const customKey = axis === "width" ? "customWidth" : "customHeight";
  const mode = state.options[modeKey];
  if (!mode || mode === "auto") return t("toolbox.spray.dimension.auto");
  if (mode === "custom") return String(state.options[customKey] || 512);
  return String(mode);
}

function enforceMipmapSizeLimit(showMessage) {
  if (!state.options.mipmaps || canUseMipmaps()) return;
  state.options.mipmaps = false;
  refs.mipmaps.checked = false;
  if (showMessage) {
    showNotification(t("toolbox.spray.notify.mipmapRule"), "info");
  }
}

async function importPrimaryFiles(files) {
  if (!files || !files.length) return;
  setBusy(true, t("toolbox.spray.busy.reading"));
  try {
    const project = await decodeFilesAsProject(files, {
      requestClipOptions: showClipOptionsModal,
    });
    state.projects.push(project);
    state.activeProjectId = project.id;
    state.frameIndex = 0;
    enforceMipmapSizeLimit(true);
    renderAll();
    showNotification(t("toolbox.spray.notify.imported"), "success");
  } catch (error) {
    showError(t("toolbox.spray.import.failed", { error: formatError(error) }));
  } finally {
    setBusy(false);
  }
}

async function importBatchFiles(files) {
  if (!files || !files.length) return;
  setBusy(true, t("toolbox.spray.busy.batchImport"));
  try {
    const projects = await decodeFilesAsBatch(files, {
      requestClipOptions: showClipOptionsModal,
    });
    state.projects.push(...projects);
    state.activeProjectId = projects[0]?.id || state.activeProjectId;
    state.frameIndex = 0;
    enforceMipmapSizeLimit(true);
    renderAll();
    showNotification(t("toolbox.spray.notify.batchImported", { count: projects.length }), "success");
  } catch (error) {
    showError(t("toolbox.spray.import.failedBatch", { error: formatError(error) }));
  } finally {
    setBusy(false);
  }
}

async function importMipmapOverride(files) {
  const project = activeProject();
  if (!project || !files || !files.length) return;
  const level = Number(refs.mipmapLevel.value || 0);
  if (level <= 0) return;
  setBusy(true, t("toolbox.spray.busy.mipOverride"));
  try {
    project.mipOverrides[level] = await decodeMipmapOverride(files, {
      requestClipOptions: showClipOptionsModal,
    });
    renderMipmapControlsState();
    updatePreview();
    showNotification(t("toolbox.spray.notify.mipReplaced"), "success");
  } catch (error) {
    showError(t("toolbox.spray.settings.mipFailed", { error: formatError(error) }));
  } finally {
    setBusy(false);
  }
}

function clearMipmapOverride() {
  const project = activeProject();
  if (!project) return;
  const level = Number(refs.mipmapLevel.value || 0);
  if (level <= 0) return;
  delete project.mipOverrides[level];
  renderMipmapControlsState();
  updatePreview();
}

async function saveSprayVTF() {
  const output = await buildOutputForActiveProject();
  if (!output) return;
  setBusy(true, t("toolbox.spray.busy.saveVtf"));
  try {
    const path = await callApp("SaveSprayVTF", {
      name: output.name,
      vtfBase64: output.vtfBase64,
    });
    if (path) {
      showNotification(t("toolbox.spray.notify.vtfSaved"), "success");
    }
  } catch (error) {
    showError(t("toolbox.spray.save.vtfFailed", { error: formatError(error) }));
  } finally {
    setBusy(false);
  }
}

async function saveSprayVMT() {
  const project = activeProject();
  if (!project) {
    showError(t("toolbox.spray.error.importFirst"));
    return;
  }
  setBusy(true, t("toolbox.spray.busy.saveVmt"));
  try {
    const path = await callApp("SaveSprayVMT", {
      name: sanitizeOutputName(getProjectDisplayName(project)),
    });
    if (path) {
      showNotification(t("toolbox.spray.notify.vmtSaved"), "success");
    }
  } catch (error) {
    showError(t("toolbox.spray.save.vmtFailed", { error: formatError(error) }));
  } finally {
    setBusy(false);
  }
}

async function buildOutputForActiveProject() {
  const project = activeProject();
  if (!project) {
    showError(t("toolbox.spray.error.importFirst"));
    return null;
  }
  setBusy(true, t("toolbox.spray.busy.converting"));
  try {
    return await buildSprayOutputs(project, state.options);
  } catch (error) {
    showError(t("toolbox.spray.error.convertFailed", { error: formatError(error) }));
    return null;
  } finally {
    setBusy(false);
  }
}

function renderAll() {
  collectRefs();
  renderProjectList();
  renderSettings();
  renderMipmapLevels();
  renderFrameStrip();
  updatePreview();
}

function renderProjectList() {
  refs.projectList.replaceChildren();
  if (!state.projects.length) {
    refs.projectList.appendChild(el("div", "spray-empty-state", t("toolbox.spray.project.empty")));
    return;
  }

  state.projects.forEach((project) => {
    const row = el("button", "spray-project-row");
    row.type = "button";
    if (project.id === state.activeProjectId) row.classList.add("is-active");
    const main = el("span", "spray-project-row-main");
    main.append(el("strong", "", getProjectDisplayName(project)));
    main.append(el("span", "", t("toolbox.spray.project.frameCount", { count: project.frames.length })));
    const remove = button("", "spray-project-remove", "×");
    remove.setAttribute("aria-label", t("toolbox.spray.project.remove"));
    remove.addEventListener("click", (event) => {
      event.stopPropagation();
      removeProject(project.id);
    });
    row.append(main, remove);
    row.addEventListener("click", () => {
      state.activeProjectId = project.id;
      state.frameIndex = 0;
      renderAll();
    });
    refs.projectList.appendChild(row);
  });
}

function renderSettings() {
  const project = activeProject();
  refs.projectName.value = project?.name || "";
  refs.widthValue.value = dimensionValueFromOptions("width");
  refs.heightValue.value = dimensionValueFromOptions("height");
  refs.rescale.checked = !!state.options.rescale;
  refs.mipmaps.checked = !!state.options.mipmaps;
  refs.singleFrame.checked = !!state.options.singleFrameAnimation;
  refs.singleFrameCount.value = state.options.singleFrameCount;
  setCustomSelectValue(refs.format, String(state.options.format), false);
  setCustomSelectValue(refs.sampling, String(state.options.sampling), false);
  refs.dither.checked = !!state.options.dither;
  setCustomSelectValue(refs.dxtQuality, String(state.options.dxtQuality), false);
}

function renderMipmapLevels() {
  const project = activeProject();
  if (!project || !state.options.mipmaps || !canUseMipmaps()) {
    renderCustomSelectOptions(refs.mipmapLevel, []);
    setCustomSelectDisabled(refs.mipmapLevel, true);
    refs.mipmapImportBtn.disabled = true;
    refs.mipmapClearBtn.disabled = true;
    return;
  }

  const size = resolveOutputSize(project, state.options);
  const levels = getMipmapDimensions(size.width, size.height);
  const options = levels.slice(1).map((level, index) => [
    String(index + 1),
    `${index + 1}: ${level.width}×${level.height}`,
  ]);
  renderCustomSelectOptions(refs.mipmapLevel, options);
  setCustomSelectDisabled(refs.mipmapLevel, levels.length <= 1);
  refs.mipmapImportBtn.disabled = levels.length <= 1;
  refs.mipmapClearBtn.disabled = levels.length <= 1;
  renderMipmapControlsState();
}

function renderMipmapControlsState() {
  const project = activeProject();
  const level = Number(refs.mipmapLevel.value || 0);
  const hasOverride = !!project?.mipOverrides?.[level];
  refs.mipmapClearBtn.disabled =
    !project || !state.options.mipmaps || level <= 0 || !hasOverride;
  refs.mipmapImportBtn.textContent = hasOverride
    ? t("toolbox.spray.settings.mipReplaceAgain")
    : t("toolbox.spray.settings.mipReplace");
}

function renderFrameStrip() {
  refs.frameStrip.replaceChildren();
  const project = activeProject();
  if (!project) return;
  project.frames.slice(0, 48).forEach((frame, index) => {
    const item = el("button", "spray-frame-thumb");
    item.type = "button";
    if (index === state.frameIndex) item.classList.add("is-active");
    const canvas = document.createElement("canvas");
    canvas.width = 44;
    canvas.height = 44;
    drawFrameToCanvas(frame, canvas, { rescale: true });
    item.appendChild(canvas);
    item.addEventListener("click", () => {
      state.frameIndex = index;
      updatePreview();
      renderFrameStrip();
    });
    refs.frameStrip.appendChild(item);
  });
}

function updatePreview() {
  const project = activeProject();
  const canvas = refs.previewCanvas;
  const ctx = canvas.getContext("2d");

  if (!project) {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    refs.previewMeta.textContent = t("toolbox.spray.preview.waiting");
    refs.frameLabel.textContent = "0 / 0";
    refs.saveVTFBtn.disabled = true;
    refs.saveVMTBtn.disabled = true;
    refs.sizeNote.hidden = true;
    refs.sizeNote.textContent = "";
    updateFooter();
    return;
  }

  const size = resolveOutputSize(project, state.options);
  canvas.width = size.width;
  canvas.height = size.height;
  state.frameIndex = Math.min(state.frameIndex, project.frames.length - 1);
  drawFrameToCanvas(project.frames[state.frameIndex], canvas, state.options);
  refs.previewMeta.textContent = `${size.width}×${size.height}`;
  refs.frameLabel.textContent = `${state.frameIndex + 1} / ${project.frames.length}`;
  refs.saveVTFBtn.disabled = state.busy;
  refs.saveVMTBtn.disabled = state.busy;
  updateSizeNote(project, size);
  updateFooter();
}

function updateSizeNote(project, size) {
  const frames = getEffectiveFrameCount(project);
  const format = Number(state.options.format);
  const shortened = shouldShortenOriginalVTF(size.width, size.height, frames, format);
  const useMipmaps = state.options.mipmaps && canUseMipmaps();
  const estimate = estimateVTFSize(
    size.width,
    size.height,
    frames,
    format,
    useMipmaps,
    shortened
  );
  const kb = Math.round(estimate / 1024);
  const suffix = [
    FORMAT_LABELS.get(format) || "VTF",
    useMipmaps ? "Mipmaps" : "",
    shortened ? t("toolbox.spray.note.shortened") : "",
  ].filter(Boolean).join(" · ");
  refs.sizeNote.hidden = false;
  refs.sizeNote.textContent = t("toolbox.spray.note.size", { kb, suffix });
  refs.sizeNote.classList.toggle("is-warning", kb >= 512);
}

function updateFooter() {
  if (state.busy) return;
  const project = activeProject();
  if (!project) {
    refs.footerStatus.textContent = t("toolbox.spray.footer.empty");
    return;
  }
  refs.footerStatus.textContent = t("toolbox.spray.footer.summary", {
    count: state.projects.length,
    name: getProjectDisplayName(project),
  });
}

function setBusy(busy, message = "") {
  state.busy = busy;
  refs.saveVTFBtn.disabled = busy || !state.projects.length;
  refs.saveVMTBtn.disabled = busy || !state.projects.length;
  refs.footerStatus.textContent = busy ? message : "";
  if (!busy) updateFooter();
}

function moveFrame(delta) {
  const project = activeProject();
  if (!project) return;
  state.frameIndex = (state.frameIndex + delta + project.frames.length) % project.frames.length;
  updatePreview();
  renderFrameStrip();
}

function togglePlayback() {
  if (state.playing) {
    stopPlayback();
    return;
  }
  const project = activeProject();
  if (!project || project.frames.length <= 1) return;
  state.playing = true;
  refs.playBtn.textContent = t("toolbox.spray.preview.pause");
  state.timer = setInterval(() => {
    moveFrame(1);
  }, 200);
}

function stopPlayback() {
  state.playing = false;
  refs.playBtn.textContent = t("toolbox.spray.preview.play");
  if (state.timer) clearInterval(state.timer);
  state.timer = null;
}

function closeSprayTool() {
  stopPlayback();
  closeSprayDropdowns();
  refs.modal.classList.add("hidden");
}

function removeProject(id) {
  state.projects = state.projects.filter((project) => project.id !== id);
  if (state.activeProjectId === id) {
    state.activeProjectId = state.projects[0]?.id || null;
    state.frameIndex = 0;
  }
  renderAll();
}

function activeProject() {
  return state.projects.find((project) => project.id === state.activeProjectId) || null;
}

function canUseMipmaps() {
  const project = activeProject();
  const size = project
    ? resolveOutputSize(project, state.options)
    : {
      width: getConfiguredMipmapDimension("width"),
      height: getConfiguredMipmapDimension("height"),
    };
  const frames = project ? getEffectiveFrameCount(project) : 1;
  const format = Number(state.options.format);
  const shortened = shouldShortenOriginalVTF(size.width, size.height, frames, format);
  return shouldUseOriginalMipmaps(
    size.width,
    size.height,
    frames,
    format,
    Number(state.options.sampling || 0),
    shortened
  );
}

function getConfiguredMipmapDimension(axis) {
  const mode = state.options[`${axis}Mode`];
  if (!mode || mode === "auto") return 512;
  if (mode === "custom") {
    return Number(state.options[axis === "width" ? "customWidth" : "customHeight"]) || 512;
  }
  return Number(mode) || 512;
}

function getEffectiveFrameCount(project) {
  if (
    state.options.singleFrameAnimation &&
    project.frames.length === 1 &&
    Number(state.options.singleFrameCount) > 1
  ) {
    return Number(state.options.singleFrameCount);
  }
  return project.frames.length;
}

function getProjectDisplayName(project) {
  if (!project) return "spray";
  return project.name || getProjectSourceBaseName(project) || "spray";
}

function getProjectSourceBaseName(project) {
  const sourceName = Array.isArray(project?.sourceNames) ? project.sourceNames[0] : "";
  return sanitizeOutputName(String(sourceName || "").replace(/\.[^/.]+$/, ""));
}

function showClipOptionsModal(file, meta) {
  return new Promise((resolve) => {
    const modal = el("div", "modal spray-clip-modal");
    const content = el("div", "modal-content modal-small spray-clip-content");
    const header = el("div", "modal-header");
    header.append(el("h3", "", t("toolbox.spray.clip.title")));
    const body = el("div", "modal-body spray-clip-body");
    const start = numberControl(t("toolbox.spray.clip.start"), "spray-clip-start", 0, 0);
    const endDefault = meta.frameCount || 1;
    const end = numberControl(t("toolbox.spray.clip.end"), "spray-clip-end", endDefault, 0);
    const fps = numberControl(t("toolbox.spray.clip.fps"), "spray-clip-fps", meta.defaultFps || 1, 1, 30);
    const all = checkboxControl(t("toolbox.spray.clip.allFrames"), "spray-clip-all", false);
    const fileLabel = el("p", "spray-clip-file", file.name || "");
    body.append(fileLabel, start, end, fps, all);
    const footer = el("div", "modal-footer");
    const cancel = button("", "btn btn-secondary", t("toolbox.common.cancel"));
    const accept = button("", "btn btn-primary", t("toolbox.spray.clip.accept"));
    footer.append(cancel, accept);
    content.append(header, body, footer);
    modal.appendChild(content);
    document.body.appendChild(modal);

    const cleanup = () => modal.remove();
    cancel.addEventListener("click", () => {
      cleanup();
      resolve({ cancelled: true });
    });
    accept.addEventListener("click", () => {
      const value = {
        start: Number(start.querySelector("input").value || 0),
        end: Number(end.querySelector("input").value || endDefault),
        fps: Number(fps.querySelector("input").value || meta.defaultFps || 1),
        allFrames: all.querySelector("input").checked,
      };
      cleanup();
      resolve(value);
    });
  });
}

function collectRefs() {
  refs = {
    modal: refs.modal || document.getElementById("spray-tool-modal"),
    settingsPanel: document.querySelector("#spray-tool-modal .spray-settings-panel"),
    closeBtn: document.getElementById("spray-tool-close"),
    primaryImportBtn: document.getElementById("spray-primary-import-btn"),
    primaryInput: document.getElementById("spray-primary-input"),
    batchImportBtn: document.getElementById("spray-batch-import-btn"),
    batchInput: document.getElementById("spray-batch-input"),
    projectList: document.getElementById("spray-project-list"),
    previewCanvas: document.getElementById("spray-preview-canvas"),
    previewMeta: document.getElementById("spray-preview-meta"),
    prevBtn: document.getElementById("spray-prev-frame-btn"),
    playBtn: document.getElementById("spray-play-btn"),
    nextBtn: document.getElementById("spray-next-frame-btn"),
    frameLabel: document.getElementById("spray-frame-label"),
    frameStrip: document.getElementById("spray-frame-strip"),
    projectName: document.getElementById("spray-project-name"),
    widthValue: document.getElementById("spray-width-value"),
    heightValue: document.getElementById("spray-height-value"),
    rescale: document.getElementById("spray-rescale"),
    mipmaps: document.getElementById("spray-mipmaps"),
    mipmapLevel: document.getElementById("spray-mipmap-level"),
    mipmapImportBtn: document.getElementById("spray-mipmap-import-btn"),
    mipmapClearBtn: document.getElementById("spray-mipmap-clear-btn"),
    mipmapInput: document.getElementById("spray-mipmap-input"),
    singleFrame: document.getElementById("spray-single-frame"),
    singleFrameCount: document.getElementById("spray-single-frame-count"),
    format: document.getElementById("spray-format"),
    sampling: document.getElementById("spray-sampling"),
    dither: document.getElementById("spray-dither"),
    dxtQuality: document.getElementById("spray-dxt-quality"),
    sizeNote: document.getElementById("spray-size-note"),
    footerStatus: document.getElementById("spray-footer-status"),
    saveVTFBtn: document.getElementById("spray-save-vtf-btn"),
    saveVMTBtn: document.getElementById("spray-save-vmt-btn"),
  };
}

function panelTitle(title, subtitle) {
  const header = el("div", "spray-panel-header");
  header.append(el("h4", "", title), el("p", "", subtitle));
  return header;
}

function textControl(labelText, id, value, placeholder = "") {
  const wrap = el("div", "spray-control");
  const label = el("label", "", labelText);
  label.setAttribute("for", id);
  const control = input("text", id);
  control.value = value || "";
  control.placeholder = placeholder;
  control.className = "textInput";
  wrap.append(label, control);
  return wrap;
}

function dimensionControl(labelText, id) {
  const wrap = el("div", "spray-control");
  const label = el("label", "", labelText);
  label.setAttribute("for", id);
  const field = el("div", "spray-combo-field");
  const control = input("text", id);
  control.className = "textInput spray-combo-input";
  control.setAttribute("inputmode", "numeric");
  control.placeholder = t("toolbox.spray.settings.dimensionPlaceholder");
  const trigger = button(`${id}-toggle`, "spray-combo-toggle", "");
  trigger.setAttribute("aria-label", t("toolbox.spray.settings.presetAria", { label: labelText }));
  trigger.setAttribute("aria-expanded", "false");
  const menu = el("div", "select-menu spray-select-menu spray-combo-menu hidden");
  menu.id = `${id}-menu`;

  [t("toolbox.spray.dimension.auto"), ...RESOLUTION_PRESETS].forEach((value) => {
    const option = createSelectOption(String(value), String(value));
    option.addEventListener("click", (event) => {
      event.stopPropagation();
      control.value = option.dataset.value || "";
      control.dispatchEvent(new Event("change", { bubbles: true }));
      closeSprayDropdowns();
    });
    menu.appendChild(option);
  });
  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    toggleSprayDropdown(menu, field, trigger);
  });
  control.addEventListener("focus", () => {
    openSprayDropdown(menu, field, trigger);
  });
  control.addEventListener("click", (event) => {
    event.stopPropagation();
    openSprayDropdown(menu, field, trigger);
  });

  field.append(control, trigger, menu);
  wrap.append(label, field);
  return wrap;
}

function numberControl(labelText, id, value, min, max) {
  const wrap = el("div", "spray-control");
  const label = el("label", "", labelText);
  label.setAttribute("for", id);
  const control = input("number", id);
  control.value = String(value);
  control.min = String(min ?? 0);
  if (max != null) control.max = String(max);
  control.className = "textInput";
  wrap.append(label, control);
  return wrap;
}

function selectControl(labelText, id, options) {
  const wrap = el("div", "spray-control");
  const label = el("label", "", labelText);
  label.setAttribute("for", id);
  wrap.append(label, customSelectInput(id, options));
  return wrap;
}

function customSelectInput(id, options) {
  const dropdown = el("div", "spray-select-dropdown");
  const valueInput = input("hidden", id);
  valueInput.className = "spray-select-value";
  const trigger = button(`${id}-trigger`, "textInput spray-select-trigger", "");
  trigger.setAttribute("aria-haspopup", "listbox");
  trigger.setAttribute("aria-expanded", "false");
  const menu = el("div", "select-menu spray-select-menu hidden");
  menu.id = `${id}-menu`;
  menu.setAttribute("role", "listbox");
  valueInput._spraySelectMenu = menu;
  valueInput._spraySelectTrigger = trigger;
  dropdown.append(valueInput, trigger, menu);
  renderCustomSelectOptions(valueInput, options);
  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    if (trigger.disabled) return;
    toggleSprayDropdown(menu, trigger, trigger);
  });
  return dropdown;
}

function renderCustomSelectOptions(valueInput, options) {
  const menu = getSelectMenu(valueInput);
  if (!menu) return;
  menu.replaceChildren();

  const normalized = options.map(normalizeSelectOption);
  normalized.forEach((item) => {
    const option = createSelectOption(item.value, item.label);
    option.addEventListener("click", (event) => {
      event.stopPropagation();
      setCustomSelectValue(valueInput, item.value, true);
      closeSprayDropdowns();
    });
    menu.appendChild(option);
  });

  const nextValue = normalized.some((item) => item.value === valueInput.value)
    ? valueInput.value
    : normalized[0]?.value || "";
  setCustomSelectValue(valueInput, nextValue, false);
}

function normalizeSelectOption(optionValue) {
  const pair = Array.isArray(optionValue) ? optionValue : [optionValue, String(optionValue)];
  return {
    value: String(pair[0]),
    label: pair[1] === "auto" ? t("toolbox.spray.dimension.auto") : String(pair[1]),
  };
}

function createSelectOption(value, label) {
  const option = button("", "select-option spray-select-option", label);
  option.dataset.value = value;
  option.setAttribute("role", "option");
  return option;
}

function setCustomSelectValue(valueInput, value, dispatchChange) {
  valueInput.value = String(value ?? "");
  const trigger = getSelectTrigger(valueInput);
  const options = Array.from(getSelectMenu(valueInput)?.querySelectorAll(".spray-select-option") || []);
  let selectedLabel = "";
  options.forEach((option) => {
    const isActive = option.dataset.value === valueInput.value;
    option.classList.toggle("active", isActive);
    option.setAttribute("aria-selected", String(isActive));
    if (isActive) selectedLabel = option.textContent || "";
  });
  if (trigger) trigger.textContent = selectedLabel || t("toolbox.spray.select.placeholder");
  if (dispatchChange) {
    valueInput.dispatchEvent(new Event("change", { bubbles: true }));
  }
}

function setCustomSelectDisabled(valueInput, disabled) {
  valueInput.disabled = !!disabled;
  const trigger = getSelectTrigger(valueInput);
  const menu = getSelectMenu(valueInput);
  if (trigger) trigger.disabled = !!disabled;
  if (disabled && menu) closeSprayDropdown(menu);
}

function getSelectMenu(valueInput) {
  return valueInput?._spraySelectMenu ||
    valueInput?.closest(".spray-select-dropdown")?.querySelector(".spray-select-menu") ||
    null;
}

function getSelectTrigger(valueInput) {
  return valueInput?._spraySelectTrigger ||
    valueInput?.closest(".spray-select-dropdown")?.querySelector(".spray-select-trigger") ||
    null;
}

function toggleSprayDropdown(menu, anchor, expandedTrigger = anchor) {
  if (!menu) return;
  const opening = menu.classList.contains("hidden");
  closeSprayDropdowns(menu);
  if (opening) openSprayDropdown(menu, anchor, expandedTrigger);
  else closeSprayDropdown(menu);
}

function openSprayDropdown(menu, anchor, expandedTrigger = anchor) {
  closeSprayDropdowns(menu);
  menu._sprayHome ||= {
    parent: menu.parentNode,
    nextSibling: menu.nextSibling,
    trigger: expandedTrigger,
  };
  document.body.appendChild(menu);
  menu.classList.add("spray-floating-menu");
  menu.classList.remove("hidden");
  expandedTrigger?.setAttribute("aria-expanded", "true");
  positionSprayDropdown(menu, anchor);
}

function positionSprayDropdown(menu, anchor) {
  if (!anchor) return;
  const rect = anchor.getBoundingClientRect();
  const gap = 6;
  const width = Math.max(rect.width, 160);
  const edge = 8;
  const left = Math.max(
    edge,
    Math.min(rect.left, window.innerWidth - width - edge)
  );
  menu.style.position = "fixed";
  menu.style.left = `${Math.round(left)}px`;
  menu.style.top = `${Math.round(rect.bottom + gap)}px`;
  menu.style.width = `${Math.round(width)}px`;
  menu.style.maxHeight = "15rem";

  const menuHeight = menu.offsetHeight || menu.scrollHeight || 0;
  const below = window.innerHeight - rect.bottom - gap;
  const above = rect.top - gap;
  if (below < menuHeight && above > below) {
    menu.style.top = `${Math.max(edge, Math.round(rect.top - Math.min(menuHeight, 240) - gap))}px`;
  }
}

function closeSprayDropdown(menu) {
  if (!menu || menu.classList.contains("hidden")) return;
  menu.classList.add("hidden");
  menu.classList.remove("spray-floating-menu");
  menu.style.position = "";
  menu.style.left = "";
  menu.style.top = "";
  menu.style.width = "";
  menu.style.maxHeight = "";
  const home = menu._sprayHome;
  if (home?.trigger) home.trigger.setAttribute("aria-expanded", "false");
  if (home?.parent) home.parent.insertBefore(menu, home.nextSibling || null);
  menu._sprayHome = null;
}

function closeSprayDropdowns(exceptMenu = null) {
  document.querySelectorAll(".spray-select-menu").forEach((menu) => {
    if (menu !== exceptMenu) closeSprayDropdown(menu);
  });
}

function checkboxControl(labelText, id, checked) {
  const wrap = el("label", "spray-checkbox");
  const control = input("checkbox", id);
  control.checked = !!checked;
  wrap.append(control, el("span", "", labelText));
  return wrap;
}

function input(type, id) {
  const control = document.createElement("input");
  control.type = type;
  if (id) control.id = id;
  return control;
}

function sprayPayloadsToFiles(payloads) {
  return payloads.map((payload) => {
    const bytes = base64ToUint8Array(payload.base64 || "");
    return new File([bytes], payload.name || "spray", {
      type: payload.type || inferMimeType(payload.name || ""),
    });
  });
}

function base64ToUint8Array(value) {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function inferMimeType(name) {
  const ext = getPathExtension(name);
  if (ext === ".gif") return "image/gif";
  if (ext === ".tga") return "image/x-tga";
  if ([".png", ".jpg", ".jpeg", ".webp", ".bmp"].includes(ext)) {
    return `image/${ext === ".jpg" ? "jpeg" : ext.slice(1)}`;
  }
  return "application/octet-stream";
}

function getPathExtension(path) {
  const clean = String(path || "").toLowerCase().split(/[\\/]/).pop() || "";
  const index = clean.lastIndexOf(".");
  return index >= 0 ? clean.slice(index) : "";
}

function button(id, className, text) {
  const btn = el("button", className, text);
  btn.type = "button";
  if (id) btn.id = id;
  return btn;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text != null) node.textContent = text;
  return node;
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") {
    return Promise.reject(new Error(t("toolbox.common.backendUnsupported", { method: methodName })));
  }
  return method(...args);
}

function formatError(error) {
  const code = error?.code;
  if (typeof code === "string" && code.startsWith("toolbox.spray.")) return t(code);
  if (error?.message) return error.message;
  if (error) return String(error);
  return t("toolbox.common.unknownError");
}
