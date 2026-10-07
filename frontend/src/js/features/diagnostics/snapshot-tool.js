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

let deps = {};
let modal = null;
let currentPlan = null;

export function configureModSnapshotTool(nextDeps = {}) {
  deps = nextDeps || {};
}

const MODE_LABELS = { names: "文件名快照", full: "完整备份" };
const ACTION_LABELS = {
  enable: "启用",
  disable: "禁用",
  restore: "补回",
  overwrite: "覆盖",
  skip: "跳过",
  missing: "缺失",
};

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
      showError("打开快照目录失败: " + error);
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
    showError("找不到 Mod 快照窗口（界面未加载）");
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
    showError("读取快照列表失败: " + error);
    return;
  }
  if (snapshots.length === 0) {
    const empty = document.createElement("div");
    empty.className = "setting-row-desc";
    empty.textContent = "还没有快照。创建之后这里会列出可恢复的快照。";
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
    meta.textContent = `${MODE_LABELS[snapshot.mode] || snapshot.mode} · ${formatTime(snapshot.createdAt)} · ${snapshot.modCount} 个 Mod · ${snapshot.addonListEntries} 条 addonlist`;
    main.append(title, meta);
    const actions = document.createElement("div");
    actions.className = "settings-profile-actions";
    const previewBtn = document.createElement("button");
    previewBtn.type = "button";
    previewBtn.textContent = "恢复预览";
    previewBtn.addEventListener("click", () => previewSnapshot(snapshot.id));
    const deleteBtn = document.createElement("button");
    deleteBtn.type = "button";
    deleteBtn.textContent = "删除";
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
    "完整备份会把 addons 根目录里的 VPK 与同名图片/.meta 复制进快照目录，Mod 多、体积大时需要更长时间。继续创建？",
    { title: "创建完整备份" },
  ))) {
    return;
  }
  button.disabled = true;
  if (status) status.textContent = "正在创建快照…";
  try {
    const meta = await deps.CreateModSnapshot(nameInput?.value || "", mode);
    if (nameInput) nameInput.value = "";
    if (status) status.textContent = `已创建快照「${meta.name}」（${MODE_LABELS[meta.mode] || meta.mode}，${meta.modCount} 个 Mod）`;
    showNotification(`已创建快照「${meta.name}」`, "success");
    await refreshList();
  } catch (error) {
    if (status) status.textContent = "";
    showError("创建快照失败: " + error);
  } finally {
    button.disabled = false;
  }
}

async function previewSnapshot(id) {
  try {
    currentPlan = await deps.PreviewModSnapshotRestore(id);
  } catch (error) {
    showError("生成恢复计划失败: " + error);
    return;
  }
  const plan = document.getElementById("mod-snapshot-plan");
  const summary = document.getElementById("mod-snapshot-plan-summary");
  const items = document.getElementById("mod-snapshot-plan-items");
  if (!plan || !summary || !items) return;
  const counts = currentPlan?.counts || {};
  const parts = Object.entries(counts)
    .map(([action, count]) => `${ACTION_LABELS[action] || action} ${count}`)
    .join(" · ");
  summary.textContent = `快照「${currentPlan?.snapshot?.name || id}」：${parts || "无需改动"}；恢复会同时写回 addonlist.txt（执行前自动备份当前文件）。`;
  items.innerHTML = "";
  (currentPlan?.items || []).forEach((item) => {
    const row = document.createElement("div");
    row.className = `mod-snapshot-plan-item is-${item.action}`;
    const action = document.createElement("span");
    action.className = "mod-snapshot-plan-action";
    action.textContent = ACTION_LABELS[item.action] || item.action;
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
    `按恢复计划执行？\n\n启用 ${counts.enable || 0} · 禁用 ${counts.disable || 0} · 补回 ${counts.restore || 0} · ` +
      `覆盖 ${counts.overwrite || 0} · 跳过 ${counts.skip || 0} · 缺失 ${counts.missing || 0}\n\n` +
      "快照之后新增的 Mod 会被移到 disabled，内容不同的文件会被覆盖；执行前会自动备份当前 addonlist.txt。",
    { title: "执行快照恢复" },
  );
  if (!confirmed) return;
  const button = document.getElementById("mod-snapshot-restore-btn");
  button.disabled = true;
  try {
    const result = await deps.RestoreModSnapshot(currentPlan.snapshot.id);
    showNotification(
      `已恢复快照：启用 ${result.enabled} · 禁用 ${result.disabled} · 补回 ${result.restored} · 覆盖 ${result.overwritten} · 跳过 ${result.skipped}` +
        (result.missing ? ` · 缺失 ${result.missing}` : ""),
      "success",
    );
    await deps.refreshFilesKeepFilter?.({ silent: true });
    currentPlan = null;
    document.getElementById("mod-snapshot-plan")?.classList.add("hidden");
    await refreshList();
  } catch (error) {
    showError("恢复快照失败: " + error);
  } finally {
    button.disabled = false;
  }
}

async function deleteSnapshot(snapshot) {
  if (!(await confirmInApp(`删除快照「${snapshot.name || snapshot.id}」？快照会移入回收站，Mod 文件不受影响。`, { title: "删除快照" }))) {
    return;
  }
  try {
    await deps.DeleteModSnapshot(snapshot.id);
    showNotification("已删除快照", "success");
    await refreshList();
  } catch (error) {
    showError("删除快照失败: " + error);
  }
}
