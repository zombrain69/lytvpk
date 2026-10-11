// Mod 快照（对齐上游 ba20411 的最小闭环）。
//
// 两种类型：
//   - 文件名快照（names）：只记 addonlist + Mod 清单，创建快、占用小；
//   - 完整备份（full）：额外把 addons 根目录里的 VPK 与同名图片/.meta 复制进快照目录，可恢复文件内容。
//
// 恢复永远先给"恢复计划"，逐项标注 启用 / 禁用 / 补回 / 覆盖 / 跳过 / 缺失，用户确认后才执行；
// 执行前后端会自动留 addonlist 备份，并且遵守沙箱只读闸门。

import { showError, showNotification } from "../../core/toast.js";
import { confirmInApp } from "../modals/confirm.js";
import { t } from "../../core/i18n-runtime.js";

let deps = {};
let modal = null;
let currentPlan = null;

export function configureModSnapshotTool(nextDeps = {}) {
  deps = nextDeps || {};
}

const MODE_LABEL_KEYS = { names: "toolbox.snapshot.modeLabelNames", full: "toolbox.snapshot.modeLabelFull" };

/** modeLabel 按当前语言取快照类型名（t() 延迟取，模块求值期不调）。 */
function modeLabel(mode) {
  return MODE_LABEL_KEYS[mode] ? t(MODE_LABEL_KEYS[mode]) : mode;
}
const ACTION_LABEL_KEYS = {
  enable: "toolbox.snapshot.actionEnable",
  disable: "toolbox.snapshot.actionDisable",
  restore: "toolbox.snapshot.actionRestore",
  overwrite: "toolbox.snapshot.actionOverwrite",
  skip: "toolbox.snapshot.actionSkip",
  missing: "toolbox.snapshot.actionMissing",
};

/** actionLabel 按当前语言取恢复动作名（t() 延迟取，模块求值期不调）。 */
function actionLabel(action) {
  return ACTION_LABEL_KEYS[action] ? t(ACTION_LABEL_KEYS[action]) : action;
}

function formatBytes(bytes) {
  const value = Number(bytes) || 0;
  if (value <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let index = 0;
  let size = value;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size >= 10 || index === 0 ? Math.round(size) : size.toFixed(1)} ${units[index]}`;
}

function formatTime(iso) {
  if (!iso) return "—";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function ensureModal() {
  if (modal) return modal;
  // 弹窗结构在 index.html 里（与其它窗口一致；modal-escape 的注册表要求 id 真实存在）。
  modal = document.getElementById("mod-snapshot-modal");
  if (!modal) return null;
  document.getElementById("mod-snapshot-close-btn")?.addEventListener("click", close);
  document.getElementById("mod-snapshot-create-btn")?.addEventListener("click", createSnapshot);
  document.getElementById("mod-snapshot-open-folder-btn")?.addEventListener("click", async () => {
    try {
      await deps.OpenModSnapshotsFolder();
    } catch (error) {
      showError(t("toolbox.snapshot.openFolderFailed", { error }));
    }
  });
  document.getElementById("mod-snapshot-restore-btn")?.addEventListener("click", restoreSnapshot);
  document.getElementById("mod-snapshot-plan-cancel")?.addEventListener("click", () => {
    currentPlan = null;
    document.getElementById("mod-snapshot-plan")?.classList.add("hidden");
  });
  return modal;
}

export function closeModSnapshotTool() {
  modal?.classList.add("hidden");
  currentPlan = null;
}

function close() {
  closeModSnapshotTool();
}

export async function openModSnapshotTool() {
  if (!ensureModal()) {
    showError(t("toolbox.snapshot.modalMissing"));
    return;
  }
  modal.classList.remove("hidden");
  document.getElementById("mod-snapshot-plan")?.classList.add("hidden");
  await refreshList();
}

async function refreshList() {
  const list = document.getElementById("mod-snapshot-list");
  if (!list) return;
  list.innerHTML = "";
  let snapshots = [];
  try {
    snapshots = (await deps.ListModSnapshots()) || [];
  } catch (error) {
    showError(t("toolbox.snapshot.listReadFailed", { error }));
    return;
  }
  if (snapshots.length === 0) {
    const empty = document.createElement("div");
    empty.className = "setting-row-desc";
    empty.textContent = t("toolbox.snapshot.listEmpty");
    list.appendChild(empty);
    return;
  }
  snapshots.forEach((snapshot) => {
    const item = document.createElement("div");
    item.className = "settings-profile-item";
    const main = document.createElement("div");
    main.className = "settings-profile-main";
    const title = document.createElement("strong");
    title.textContent = snapshot.name || snapshot.id;
    const meta = document.createElement("span");
    meta.textContent = t("toolbox.snapshot.metaLine", {
      mode: modeLabel(snapshot.mode),
      time: formatTime(snapshot.createdAt),
      modCount: snapshot.modCount,
      entries: snapshot.addonListEntries,
    });
    main.append(title, meta);
    const actions = document.createElement("div");
    actions.className = "settings-profile-actions";
    const previewBtn = document.createElement("button");
    previewBtn.type = "button";
    previewBtn.textContent = t("toolbox.snapshot.preview");
    previewBtn.addEventListener("click", () => previewSnapshot(snapshot.id));
    const deleteBtn = document.createElement("button");
    deleteBtn.type = "button";
    deleteBtn.textContent = t("toolbox.snapshot.deleteAction");
    deleteBtn.addEventListener("click", () => deleteSnapshot(snapshot));
    actions.append(previewBtn, deleteBtn);
    item.append(main, actions);
    list.appendChild(item);
  });
}

async function createSnapshot() {
  const nameInput = document.getElementById("mod-snapshot-name");
  const modeSelect = document.getElementById("mod-snapshot-mode");
  const button = document.getElementById("mod-snapshot-create-btn");
  const status = document.getElementById("mod-snapshot-status");
  const mode = modeSelect?.value === "full" ? "full" : "names";
  if (mode === "full" && !(await confirmInApp(
    t("toolbox.snapshot.fullBackupConfirm"),
    { title: t("toolbox.snapshot.createFullTitle") },
  ))) {
    return;
  }
  button.disabled = true;
  if (status) status.textContent = t("toolbox.snapshot.creating");
  try {
    const meta = await deps.CreateModSnapshot(nameInput?.value || "", mode);
    if (nameInput) nameInput.value = "";
    if (status) status.textContent = t("toolbox.snapshot.createdWithMode", { name: meta.name, mode: modeLabel(meta.mode), count: meta.modCount });
    showNotification(t("toolbox.snapshot.created", { name: meta.name }), "success");
    await refreshList();
  } catch (error) {
    if (status) status.textContent = "";
    showError(t("toolbox.snapshot.createFailed", { error }));
  } finally {
    button.disabled = false;
  }
}

async function previewSnapshot(id) {
  try {
    currentPlan = await deps.PreviewModSnapshotRestore(id);
  } catch (error) {
    showError(t("toolbox.snapshot.previewFailed", { error }));
    return;
  }
  const plan = document.getElementById("mod-snapshot-plan");
  const summary = document.getElementById("mod-snapshot-plan-summary");
  const items = document.getElementById("mod-snapshot-plan-items");
  if (!plan || !summary || !items) return;
  const counts = currentPlan?.counts || {};
  const parts = Object.entries(counts)
    .map(([action, count]) => `${actionLabel(action)} ${count}`)
    .join(" · ");
  summary.textContent = t("toolbox.snapshot.planSummary", { name: currentPlan?.snapshot?.name || id, parts: parts || t("toolbox.snapshot.noChanges") });
  items.innerHTML = "";
  (currentPlan?.items || []).forEach((item) => {
    const row = document.createElement("div");
    row.className = `mod-snapshot-plan-item is-${item.action}`;
    const action = document.createElement("span");
    action.className = "mod-snapshot-plan-action";
    action.textContent = actionLabel(item.action);
    const key = document.createElement("span");
    key.className = "mod-snapshot-plan-key";
    key.textContent = item.key;
    const detail = document.createElement("span");
    detail.className = "mod-snapshot-plan-detail";
    detail.textContent = item.detail || "";
    row.append(action, key, detail);
    items.appendChild(row);
  });
  plan.classList.remove("hidden");
}

async function restoreSnapshot() {
  if (!currentPlan?.snapshot?.id) return;
  const counts = currentPlan.counts || {};
  const blocked = currentPlan.blocked;
  if (blocked) {
    showError(blocked);
    return;
  }
  const confirmed = await confirmInApp(
    t("toolbox.snapshot.restoreConfirmBody", {
      enable: counts.enable || 0,
      disable: counts.disable || 0,
      restore: counts.restore || 0,
      overwrite: counts.overwrite || 0,
      skip: counts.skip || 0,
      missing: counts.missing || 0,
    }),
    { title: t("toolbox.snapshot.restoreTitle") },
  );
  if (!confirmed) return;
  const button = document.getElementById("mod-snapshot-restore-btn");
  button.disabled = true;
  try {
    const result = await deps.RestoreModSnapshot(currentPlan.snapshot.id);
    showNotification(
      t("toolbox.snapshot.restored", {
        enabled: result.enabled,
        disabled: result.disabled,
        restored: result.restored,
        overwritten: result.overwritten,
        skipped: result.skipped,
        missing: result.missing ? t("toolbox.snapshot.restoredMissing", { count: result.missing }) : "",
      }),
      "success",
    );
    await deps.refreshFilesKeepFilter?.({ silent: true });
    currentPlan = null;
    document.getElementById("mod-snapshot-plan")?.classList.add("hidden");
    await refreshList();
  } catch (error) {
    showError(t("toolbox.snapshot.restoreFailed", { error }));
  } finally {
    button.disabled = false;
  }
}

async function deleteSnapshot(snapshot) {
  if (!(await confirmInApp(t("toolbox.snapshot.deleteConfirm", { name: snapshot.name || snapshot.id }), { title: t("toolbox.snapshot.deleteTitle") }))) {
    return;
  }
  try {
    await deps.DeleteModSnapshot(snapshot.id);
    showNotification(t("toolbox.snapshot.deleted"), "success");
    await refreshList();
  } catch (error) {
    showError(t("toolbox.snapshot.deleteFailed", { error }));
  }
}
