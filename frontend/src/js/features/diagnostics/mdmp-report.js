import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { createLocaleCollator } from "../../core/locale-collator.mjs";
// 本地化排序（第 19 项 P3）：寄存器名跟随界面语言。
const REGISTER_COLLATOR = createLocaleCollator();

const TABS = [
  { id: "overview", labelKey: "toolbox.mdmpReport.tab.overview" },
  { id: "exception", labelKey: "toolbox.mdmpReport.tab.exception" },
  { id: "threads", labelKey: "toolbox.mdmpReport.tab.threads" },
  { id: "modules", labelKey: "toolbox.mdmpReport.tab.modules" },
  { id: "memory", labelKey: "toolbox.mdmpReport.tab.memory" },
  { id: "streams", label: "Streams" },
  { id: "raw", label: "Raw JSON" },
];

const PAGE_SIZE = 30;

// TABS 是模块级常量：按 key 延迟取文案（模块求值期不调 t()）。
const tabLabel = (tab) => (tab.labelKey ? t(tab.labelKey) : tab.label);

let activeTab = "overview";
let currentReport = null;
let tableState = {};
let closeBound = false;
// 解析请求可能在窗口关闭或切换到另一个转储后才返回。
// 每次打开/关闭都推进会话号，防止旧结果覆盖当前窗口。
let reportRequestId = 0;

export async function openMDMPReportTool() {
  let filePath = "";
  try {
    filePath = await callApp("SelectMDMPFile");
  } catch (error) {
    showError(t("toolbox.mdmpReport.selectFailed", { error: formatError(error) }));
    return;
  }
  if (!filePath) return;
  await openMDMPReportFromPath(filePath);
}

export async function openMDMPReportFromPath(filePath) {
  const requestId = ++reportRequestId;
  try {
    currentReport = null;
    activeTab = "overview";
    tableState = {};
    openModal();
    renderLoading(filePath);

    const report = await callApp("ParseMDMPFile", filePath);
    if (
      requestId !== reportRequestId ||
      getModal()?.classList.contains("hidden")
    ) {
      return;
    }
    currentReport = report || {};
    renderReport();
    showNotification(t("toolbox.mdmpReport.parseDone"), "success");
  } catch (error) {
    if (
      requestId !== reportRequestId ||
      getModal()?.classList.contains("hidden")
    ) {
      return;
    }
    renderError(t("toolbox.mdmpReport.parseFailed", { error: formatError(error) }));
    showError(t("toolbox.mdmpReport.parseFailed", { error: formatError(error) }));
  }
}

function openModal() {
  ensureModal();
  getModal()?.classList.remove("hidden");
}

function closeModal() {
  reportRequestId += 1;
  currentReport = null;
  tableState = {};
  getBody()?.replaceChildren();
  getFooter()?.replaceChildren();
  getModal()?.classList.add("hidden");
}

function ensureModal() {
  if (getModal()) return;

  const modal = createEl("div", "modal hidden");
  modal.id = "mdmp-report-modal";
  modal.style.zIndex = "20012";

  const content = createEl("div", "modal-content mdmp-report-modal-content");
  const header = createEl("div", "modal-header mdmp-report-header");
  const titleWrap = createEl("div", "mdmp-report-title");
  const title = createEl("h2", "", t("toolbox.card.mdmp.title"));
  const subtitle = createEl("p", "", t("toolbox.mdmpReport.subtitle"));
  titleWrap.append(title, subtitle);

  const closeBtn = createEl("button", "close-btn", "×");
  closeBtn.type = "button";
  closeBtn.id = "close-mdmp-report-modal-btn";
  closeBtn.setAttribute("aria-label", t("toolbox.common.close"));
  header.append(titleWrap, closeBtn);

  const body = createEl("div", "modal-body");
  body.id = "mdmp-report-body";
  const footer = createEl("div", "modal-footer mdmp-report-footer");
  footer.id = "mdmp-report-footer";

  content.append(header, body, footer);
  modal.appendChild(content);
  document.body.appendChild(modal);
  bindCloseControls();
}

function bindCloseControls() {
  if (closeBound) return;
  closeBound = true;
  document.addEventListener("click", (event) => {
    if (event.target?.id === "close-mdmp-report-modal-btn") closeModal();
    if (event.target?.id === "mdmp-report-modal") closeModal();
  });
}

function renderLoading(filePath) {
  const body = getBody();
  const footer = getFooter();
  if (!body || !footer) return;

  body.replaceChildren();
  const shell = createEl("div", "mdmp-report-loading");
  shell.append(
    createEl("div", "mdmp-report-spinner"),
    createEl("h3", "", t("toolbox.mdmpReport.loading")),
    createTextWithTitle("p", "", filePath || ""),
  );
  body.appendChild(shell);
  footer.replaceChildren(createFooterButton(t("toolbox.common.close"), "btn btn-secondary", closeModal));
}

function renderError(message) {
  const body = getBody();
  const footer = getFooter();
  if (!body || !footer) return;

  body.replaceChildren();
  const shell = createEl("div", "mdmp-report-error");
  shell.append(
    createEl("div", "mdmp-report-error-icon", "!"),
    createEl("h3", "", t("toolbox.mdmpReport.errorTitle")),
    createEl("p", "", message || t("toolbox.mdmpReport.errorFallback")),
  );
  body.appendChild(shell);
  footer.replaceChildren(createFooterButton(t("toolbox.common.close"), "btn btn-secondary", closeModal));
}

function renderReport() {
  const body = getBody();
  const footer = getFooter();
  if (!body || !footer || !currentReport) return;

  body.replaceChildren();
  const shell = createEl("div", "mdmp-report-shell");
  shell.append(createSummaryBar(currentReport), createTabs(), createTabContent());
  body.appendChild(shell);
  footer.replaceChildren(createFooterButton(t("toolbox.common.close"), "btn btn-secondary", closeModal));
}

function createSummaryBar(report) {
  const bar = createEl("div", "mdmp-report-summary");
  bar.append(
    createMetricCard(t("toolbox.mdmpReport.label.file"), basename(report.file?.name || report.file?.path || "")),
    createMetricCard("Streams", formatNumber(report.streams?.length || 0)),
    createMetricCard(t("toolbox.mdmpReport.summary.threads"), formatNumber(report.threads?.length || 0)),
    createMetricCard(t("toolbox.mdmpReport.summary.modules"), formatNumber(report.modules?.length || 0)),
    createMetricCard(t("toolbox.mdmpReport.summary.memoryRanges"), formatNumber(report.memoryRanges?.length || 0)),
  );
  return bar;
}

function createMetricCard(label, value) {
  const card = createEl("div", "mdmp-report-metric");
  card.append(createEl("span", "", label), createTextWithTitle("strong", "", value || "-"));
  return card;
}

function createTabs() {
  const tabs = createEl("div", "mdmp-report-tabs");
  tabs.setAttribute("role", "tablist");
  TABS.forEach((tab) => {
    const btn = createEl("button", `mdmp-report-tab${activeTab === tab.id ? " is-active" : ""}`, tabLabel(tab));
    btn.type = "button";
    btn.dataset.tab = tab.id;
    btn.setAttribute("role", "tab");
    btn.setAttribute("aria-selected", String(activeTab === tab.id));
    btn.addEventListener("click", () => {
      if (activeTab === tab.id) return;
      activeTab = tab.id;
      renderReport();
    });
    tabs.appendChild(btn);
  });
  return tabs;
}

function createTabContent() {
  if (activeTab === "exception") return createExceptionView(currentReport);
  if (activeTab === "threads") return createThreadsView(currentReport);
  if (activeTab === "modules") return createModulesView(currentReport);
  if (activeTab === "memory") return createMemoryView(currentReport);
  if (activeTab === "streams") return createStreamsView(currentReport);
  if (activeTab === "raw") return createRawView(currentReport);
  return createOverviewView(currentReport);
}

function createOverviewView(report) {
  const view = createEl("div", "mdmp-report-view");
  view.append(
    createSection(t("toolbox.mdmpReport.section.fileInfo"), [
      [t("toolbox.mdmpReport.label.path"), report.file?.path],
      [t("toolbox.mdmpReport.label.size"), formatBytes(report.file?.size)],
      [t("toolbox.mdmpReport.label.lastModified"), report.file?.lastModified],
    ]),
    createSection("Header", [
      [t("toolbox.mdmpReport.label.signature"), `${report.header?.signatureAscii || ""} ${report.header?.signature || ""}`.trim()],
      [t("toolbox.mdmpReport.label.version"), report.header?.version],
      ["Streams", report.header?.numberOfStreams],
      ["Stream Directory RVA", report.header?.streamDirectoryRva],
      ["TimeDateStamp UTC", report.header?.timeDateStampUtc],
      ["Flags", joinValue(report.header?.flags, report.header?.flagNames)],
    ]),
  );
  if (report.system) {
    view.appendChild(createSection(t("toolbox.mdmpReport.section.system"), [
      [t("toolbox.mdmpReport.label.architecture"), joinValue(report.system.architectureName, report.system.processorArchitecture)],
      ["CPU Vendor", report.system.cpuVendor],
      [t("toolbox.mdmpReport.label.processorCount"), report.system.numberOfProcessors],
      ["Windows", `${report.system.majorVersion}.${report.system.minorVersion}.${report.system.buildNumber}`],
      ["Platform", joinValue(report.system.platformName, report.system.platformId)],
      ["Product Type", joinValue(report.system.productTypeName, report.system.productType)],
    ]));
  }
  if (report.misc) {
    view.appendChild(createSection(t("toolbox.mdmpReport.section.process"), [
      ["Process ID", report.misc.processId],
      ["Process Create UTC", report.misc.processCreateTimeUtc],
      ["User Time", `${report.misc.processUserTimeSeconds || 0}s`],
      ["Kernel Time", `${report.misc.processKernelTimeSeconds || 0}s`],
      ["Flags", joinValue(report.misc.flags1, report.misc.flagNames)],
    ]));
  }
  if (report.exception) {
    view.appendChild(createSection(t("toolbox.mdmpReport.section.exceptionSummary"), [
      ["Thread ID", report.exception.threadId],
      ["Exception Code", joinValue(report.exception.code, report.exception.codeName)],
      ["Address", report.exception.address],
      ["Module", formatModuleHit(report.exceptionModule)],
    ]));
  }
  if ((report.parseWarnings || []).length > 0) {
    view.appendChild(createListSection(t("toolbox.mdmpReport.section.parseWarnings"), report.parseWarnings));
  }
  return view;
}

function createExceptionView(report) {
  const view = createEl("div", "mdmp-report-view");
  const exception = report.exception;
  if (!exception) {
    view.appendChild(createEmptyState(t("toolbox.mdmpReport.empty.noExceptionStream")));
    return view;
  }

  view.append(
    createSection(t("toolbox.mdmpReport.section.exceptionInfo"), [
      ["Thread ID", exception.threadId],
      ["Code", joinValue(exception.code, exception.codeName)],
      ["Flags", exception.flags],
      ["Record", exception.record],
      ["Address", exception.address],
      [t("toolbox.mdmpReport.label.hitModule"), formatModuleHit(report.exceptionModule)],
      ["Parameters", (exception.parameters || []).join(", ")],
    ]),
    createRegisterSection(t("toolbox.mdmpReport.section.exceptionContext"), exception.context),
  );
  return view;
}

function createThreadsView(report) {
  const rows = (report.threads || []).map((thread) => ({
    id: thread.threadId,
    name: thread.name || "",
    teb: thread.teb,
    stack: `${thread.stack?.startAddress || ""} / ${formatBytes(thread.stack?.size)}`,
    eip: thread.context?.registers?.EIP || thread.context?.registers?.RIP || "",
    priority: thread.priority,
  }));
  return createDataTable("threads", rows, [
    ["TID", (row) => row.id],
    [t("toolbox.mdmpReport.label.name"), (row) => row.name],
    ["TEB", (row) => row.teb],
    ["Stack", (row) => row.stack],
    ["IP", (row) => row.eip],
    ["Priority", (row) => row.priority],
  ]);
}

function createModulesView(report) {
  const rows = report.modules || [];
  return createDataTable("modules", rows, [
    ["#", (row) => row.index],
    [t("toolbox.mdmpReport.label.file"), (row) => row.fileName || basename(row.path)],
    ["Base", (row) => row.baseAddress],
    ["Size", (row) => formatBytes(row.sizeOfImage)],
    ["Timestamp UTC", (row) => row.timeDateStampUtc],
    ["Version", (row) => row.version?.fileVersion],
    ["PDB", (row) => row.codeView?.pdbPath || ""],
    [t("toolbox.mdmpReport.label.path"), (row) => row.path],
  ]);
}

function createMemoryView(report) {
  const view = createEl("div", "mdmp-report-view");
  if (report.systemMemory || report.processVmCounters) {
    const grid = createEl("div", "mdmp-report-two-col");
    if (report.systemMemory) grid.appendChild(createRawFieldSection("SystemMemoryInfoStream", report.systemMemory));
    if (report.processVmCounters) grid.appendChild(createRawFieldSection("ProcessVmCountersStream", report.processVmCounters));
    view.appendChild(grid);
  }
  view.appendChild(createDataTable("memory", report.memoryRanges || [], [
    ["#", (row) => row.index],
    ["Source", (row) => row.source],
    ["Start", (row) => row.startAddress],
    ["End", (row) => row.endAddress],
    ["Size", (row) => formatBytes(row.size)],
    ["RVA", (row) => row.rva],
  ]));
  if ((report.memoryInfo || []).length > 0) {
    view.appendChild(createDataTable("memoryInfo", report.memoryInfo || [], [
      ["#", (row) => row.index],
      ["Base", (row) => row.baseAddress],
      ["Region", (row) => formatBytes(row.regionSize)],
      ["State", (row) => joinValue(row.state, row.stateName)],
      ["Protect", (row) => joinValue(row.protect, row.protectName)],
      ["Type", (row) => joinValue(row.type, row.typeName)],
    ]));
  }
  return view;
}

function createStreamsView(report) {
  return createDataTable("streams", report.streams || [], [
    ["#", (row) => row.index],
    ["Type", (row) => row.type],
    ["Name", (row) => row.name],
    ["Size", (row) => formatBytes(row.size)],
    ["RVA", (row) => row.rva],
    ["End", (row) => row.end],
  ]);
}

function createRawView(report) {
  const view = createEl("div", "mdmp-report-view");
  const pre = createEl("pre", "mdmp-report-raw");
  pre.textContent = JSON.stringify(report, null, 2);
  view.appendChild(pre);
  return view;
}

function createSection(title, rows) {
  const section = createEl("section", "mdmp-report-section");
  section.appendChild(createEl("h3", "", title));
  const grid = createEl("div", "mdmp-report-detail-grid");
  rows.forEach(([label, value]) => {
    const item = createEl("div", "mdmp-report-detail");
    item.append(createEl("span", "", label), createTextWithTitle("strong", "", formatValue(value)));
    grid.appendChild(item);
  });
  section.appendChild(grid);
  return section;
}

function createListSection(title, values) {
  const section = createEl("section", "mdmp-report-section");
  section.appendChild(createEl("h3", "", title));
  const list = createEl("div", "mdmp-report-warning-list");
  values.forEach((value) => list.appendChild(createEl("div", "", value)));
  section.appendChild(list);
  return section;
}

function createRegisterSection(title, context = {}) {
  const rows = Object.entries(context.registers || {}).sort(([a], [b]) => REGISTER_COLLATOR.compare(a, b));
  const section = createSection(title, [
    ["Architecture", context.architecture],
    ["Context RVA", context.rva],
    ["Context Size", context.size],
    ["Context Flags", context.contextFlags],
  ]);
  if (rows.length > 0) {
    const regs = createEl("div", "mdmp-report-register-grid");
    rows.forEach(([name, value]) => {
      const item = createEl("div", "mdmp-report-register");
      item.append(createEl("span", "", name), createEl("strong", "", value));
      regs.appendChild(item);
    });
    section.appendChild(regs);
  }
  if (context.preview?.text) {
    const pre = createEl("pre", "mdmp-report-hex");
    pre.textContent = context.preview.text;
    section.appendChild(pre);
  }
  return section;
}

function createRawFieldSection(title, stream = {}) {
  const section = createEl("section", "mdmp-report-section");
  section.appendChild(createEl("h3", "", title));
  const rows = (stream.fields || []).slice(0, 16).map((field) => `${field.name}: ${field.hex || field.value}${field.display ? ` (${field.display})` : ""}`);
  const list = createEl("div", "mdmp-report-warning-list");
  rows.forEach((row) => list.appendChild(createEl("div", "", row)));
  section.appendChild(list);
  return section;
}

function createDataTable(key, rows, columns) {
  const state = getTableState(key);
  const gridTemplate = `repeat(${columns.length}, minmax(9rem, 1fr))`;

  const wrap = createEl("section", "mdmp-report-table-section");
  const toolbar = createEl("div", "mdmp-report-table-toolbar");
  const input = createEl("input", "form-input mdmp-report-search");
  input.type = "search";
  input.placeholder = t("toolbox.mdmpReport.table.searchPlaceholder");
  input.value = state.query;
  const countLabel = createEl("span", "", "");
  toolbar.append(input, countLabel);
  wrap.appendChild(toolbar);

  const table = createEl("div", "mdmp-report-table");
  const pager = createEl("div", "mdmp-report-pagination");
  wrap.append(table, pager);

  // 只重建表格行/计数/分页，保留 input 自身，避免输入时丢失焦点。
  function render() {
    const filtered = filterRows(rows, state.query);
    const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
    state.page = Math.min(Math.max(1, state.page), totalPages);
    const start = (state.page - 1) * PAGE_SIZE;
    const pageRows = filtered.slice(start, start + PAGE_SIZE);

    countLabel.textContent = t("toolbox.mdmpReport.table.count", { count: formatNumber(filtered.length) });

    table.replaceChildren();
    const header = createEl("div", "mdmp-report-table-row is-header");
    header.style.gridTemplateColumns = gridTemplate;
    columns.forEach(([label]) => header.appendChild(createEl("span", "", label)));
    table.appendChild(header);

    if (pageRows.length === 0) {
      table.appendChild(createEmptyState(t("toolbox.mdmpReport.empty.noData")));
    } else {
      pageRows.forEach((row) => {
        const line = createEl("div", "mdmp-report-table-row");
        line.style.gridTemplateColumns = gridTemplate;
        columns.forEach(([, getValue]) => line.appendChild(createTextWithTitle("span", "", formatValue(getValue(row)))));
        table.appendChild(line);
      });
    }

    pager.replaceChildren(
      createPagerButton("‹", t("toolbox.common.paginationPrev"), state.page <= 1, () => {
        state.page = Math.max(1, state.page - 1);
        render();
      }),
      createEl("span", "", t("toolbox.mdmpReport.table.pageStatus", { page: state.page, totalPages, count: formatNumber(filtered.length) })),
      createPagerButton("›", t("toolbox.common.paginationNext"), state.page >= totalPages, () => {
        state.page = Math.min(totalPages, state.page + 1);
        render();
      }),
    );
  }

  input.addEventListener("input", () => {
    state.query = input.value;
    state.page = 1;
    render();
  });

  render();
  return wrap;
}

function createPagerButton(label, title, disabled, onClick) {
  const button = createEl("button", "mdmp-report-page-btn", label);
  button.type = "button";
  button.title = title;
  button.disabled = disabled;
  button.addEventListener("click", onClick);
  return button;
}

function getTableState(key) {
  if (!tableState[key]) tableState[key] = { page: 1, query: "" };
  return tableState[key];
}

function filterRows(rows, query) {
  if (!query) return rows;
  const needle = query.toLowerCase();
  return rows.filter((row) => JSON.stringify(row).toLowerCase().includes(needle));
}

function createEmptyState(text) {
  return createEl("div", "mdmp-report-empty", text);
}

function createFooterButton(label, className, onClick) {
  const button = createEl("button", className, label);
  button.type = "button";
  button.addEventListener("click", onClick);
  return button;
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") {
    return Promise.reject(new Error(t("toolbox.common.backendUnsupported", { method: methodName })));
  }
  return method(...args);
}

function getModal() {
  return document.getElementById("mdmp-report-modal");
}

function getBody() {
  return document.getElementById("mdmp-report-body");
}

function getFooter() {
  return document.getElementById("mdmp-report-footer");
}

function createEl(tag, className = "", text = "") {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== "") element.textContent = text;
  return element;
}

function createTextWithTitle(tag, className = "", text = "") {
  const element = createEl(tag, className, formatValue(text));
  element.title = formatValue(text);
  return element;
}

function formatError(error) {
  if (error?.message) return error.message;
  return String(error || t("toolbox.common.unknownError"));
}

function formatValue(value) {
  if (value === null || value === undefined || value === "") return "-";
  if (Array.isArray(value)) return value.join(", ");
  return String(value);
}

function joinValue(primary, secondary) {
  if (Array.isArray(secondary)) secondary = secondary.join(", ");
  if (!secondary) return formatValue(primary);
  if (!primary) return formatValue(secondary);
  return `${primary} · ${secondary}`;
}

function formatModuleHit(hit) {
  if (!hit) return "-";
  return `${hit.fileName || basename(hit.path)} ${hit.offset || ""}`.trim();
}

function formatNumber(value) {
  return Number(value || 0).toLocaleString("zh-CN");
}

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = bytes;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index++;
  }
  return `${size.toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
}

function basename(path = "") {
  return String(path).split(/[\\/]/).pop() || path;
}
