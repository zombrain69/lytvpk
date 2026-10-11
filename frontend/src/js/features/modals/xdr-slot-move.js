// XDR「移到空槽」弹窗（详情面板 → 同槽冲突行上的按钮）。
//
// 流程：预览（后端算好可选空槽 + 将改名的文件）→ 选择目标槽 → 确认执行 →
// 全量静默刷新列表 → 回到详情面板（数据已是重建后的）。
//
// 后端纪律（internal/app/xdr_slot_move.go）：重建走流式搬运 + CRC 校验，
// 替换前留首次编辑备份（<文件>.vpk.lytvpk.bak），同内容副本一起处理。
import { t } from "../../core/i18n-runtime.js";
import { showError, showNotification } from "../../core/toast.js";
import { beginMessageModalSession } from "../../core/message-modal.js";
import {
  MoveXDRModToSlot,
  PreviewXDRSlotMove,
} from "../../../../wailsjs/go/app/App";
import { refreshFilesKeepFilter } from "../file-list/filters.js";
import {
  buildSlotOptionLabel,
  formatMoveNotice,
  splitMoveFiles,
} from "./xdr-slot-move-view.mjs";

function errorText(error) {
  return String(error?.message || error || "");
}

/**
 * openXDRSlotMoveDialog 打开「移到空槽」弹窗。
 *
 * filePath：源 VPK 路径；character/fromSlot：要调整的角色与槽位；
 * onFinished：弹窗收尾（预览失败 / 取消 / 执行完成都只回调一次），
 *             调用方通常用它把详情面板重新打开。
 */
export async function openXDRSlotMoveDialog({ filePath, character, fromSlot, onFinished }) {
  let finished = false;
  const finish = (reason) => {
    if (finished) return;
    finished = true;
    try {
      onFinished?.(reason);
    } catch (error) {
      console.error("换槽弹窗收尾回调失败:", error);
    }
  };

  // 预览失败时不开弹窗：错误原因（没有证据 / 泛化角色等）直接提示。
  let preview;
  try {
    preview = await PreviewXDRSlotMove(filePath, character, fromSlot, 0);
  } catch (error) {
    showError(errorText(error));
    finish("preview-failed");
    return;
  }

  const session = beginMessageModalSession({ onClose: (reason) => finish(reason) });
  if (!session) {
    showError(t("modals.xdrMove.dialogUnavailable"));
    finish("no-session");
    return;
  }

  session.addModalClass("xdr-slot-move-modal");
  session.titleEl.textContent = t("modals.xdrMove.title");
  session.confirmBtn.textContent = t("modals.xdrMove.confirm");

  // —— 内容骨架 ——
  const content = document.createElement("div");
  content.className = "xdr-slot-move-content";

  const intro = document.createElement("p");
  intro.className = "xdr-slot-move-intro";
  content.appendChild(intro);

  const rivalsLine = document.createElement("p");
  rivalsLine.className = "xdr-slot-move-note";
  content.appendChild(rivalsLine);

  const siblingsLine = document.createElement("p");
  siblingsLine.className = "xdr-slot-move-note";
  content.appendChild(siblingsLine);

  const targetRow = document.createElement("div");
  targetRow.className = "xdr-slot-move-target";
  const targetLabel = document.createElement("label");
  targetLabel.className = "xdr-slot-move-target-label";
  targetLabel.textContent = t("modals.xdrMove.targetLabel");
  targetLabel.htmlFor = "xdr-slot-move-select";
  const select = document.createElement("select");
  select.id = "xdr-slot-move-select";
  select.className = "xdr-slot-move-select";
  targetRow.appendChild(targetLabel);
  targetRow.appendChild(select);
  content.appendChild(targetRow);

  const hint = document.createElement("p");
  hint.className = "xdr-slot-move-hint";
  hint.textContent = t("modals.xdrMove.hint");
  content.appendChild(hint);

  const filesBlock = document.createElement("div");
  filesBlock.className = "xdr-slot-move-files";
  content.appendChild(filesBlock);

  const errorLine = document.createElement("p");
  errorLine.className = "xdr-slot-move-error hidden";
  errorLine.setAttribute("role", "alert");
  content.appendChild(errorLine);

  session.contentEl.appendChild(content);

  const showInlineError = (message) => {
    if (!session.isCurrent()) return;
    errorLine.textContent = message;
    errorLine.classList.remove("hidden");
  };
  const clearInlineError = () => {
    errorLine.textContent = "";
    errorLine.classList.add("hidden");
  };

  // —— 静态文案 ——
  const displayName = String(preview.sourcePath || "").split(/[\\/]/).pop() || filePath;
  intro.textContent = t("modals.xdrMove.intro", {
    name: displayName,
    character: preview.character || character,
    from: preview.fromSlotLabel || String(fromSlot).padStart(3, "0"),
  });
  const rivals = Array.isArray(preview.rivals) ? preview.rivals.slice(0, 4) : [];
  rivalsLine.textContent = rivals.length > 0
    ? t("modals.xdrMove.rivals", { rivals: rivals.join("、") })
    : "";
  rivalsLine.classList.toggle("hidden", rivals.length === 0);
  const siblings = Array.isArray(preview.siblingPaths) ? preview.siblingPaths : [];
  siblingsLine.textContent = siblings.length > 0
    ? t("modals.xdrMove.siblings", { count: siblings.length, paths: siblings.join("；") })
    : "";
  siblingsLine.classList.toggle("hidden", siblings.length === 0);

  // —— 目标槽位下拉 ——
  const availableSlots = Array.isArray(preview.availableSlots) ? preview.availableSlots : [];
  const optionsBySlot = new Map();
  availableSlots.forEach((option) => {
    optionsBySlot.set(option.slot, option);
    const item = document.createElement("option");
    item.value = String(option.slot);
    item.textContent = buildSlotOptionLabel(option, t);
    select.appendChild(item);
  });

  const hasSlots = availableSlots.length > 0;
  if (!hasSlots) {
    select.disabled = true;
    session.confirmBtn.disabled = true;
    showInlineError(t("modals.xdrMove.noSlots"));
  } else if (preview.defaultSlot) {
    select.value = String(preview.defaultSlot);
  }

  // —— 文件清单渲染 ——
  const renderFiles = (files) => {
    filesBlock.replaceChildren();
    const list = Array.isArray(files) ? files : [];
    const title = document.createElement("div");
    title.className = "xdr-slot-move-files-title";
    title.textContent = t("modals.xdrMove.filesTitle", { count: list.length });
    filesBlock.appendChild(title);
    const { visible, hiddenCount } = splitMoveFiles(list);
    visible.forEach((file) => {
      const row = document.createElement("div");
      row.className = "xdr-slot-move-file-row";
      const from = document.createElement("code");
      from.textContent = String(file.from || "");
      const arrow = document.createElement("span");
      arrow.textContent = " → ";
      const to = document.createElement("code");
      to.textContent = String(file.to || "");
      row.appendChild(from);
      row.appendChild(arrow);
      row.appendChild(to);
      filesBlock.appendChild(row);
    });
    if (hiddenCount > 0) {
      const more = document.createElement("div");
      more.className = "xdr-slot-move-files-more";
      more.textContent = t("modals.xdrMove.filesMore", { count: hiddenCount });
      filesBlock.appendChild(more);
    }
  };

  // —— 预览刷新（切换目标槽时重新取文件清单）——
  let loadToken = 0;
  const reloadForSlot = async (slot) => {
    const token = ++loadToken;
    select.disabled = true;
    session.confirmBtn.disabled = true;
    try {
      const next = await PreviewXDRSlotMove(filePath, character, fromSlot, slot);
      if (!session.isCurrent() || token !== loadToken) return;
      clearInlineError();
      renderFiles(next.files);
    } catch (error) {
      if (!session.isCurrent() || token !== loadToken) return;
      showInlineError(errorText(error));
    } finally {
      if (session.isCurrent() && token === loadToken) {
        select.disabled = !hasSlots;
        session.confirmBtn.disabled = !hasSlots;
      }
    }
  };

  renderFiles(preview.files);

  select.addEventListener("change", () => {
    const slot = Number(select.value || 0);
    if (!slot) return;
    void reloadForSlot(slot);
  });

  // —— 确认执行 ——
  session.confirmBtn.onclick = async () => {
    if (!session.isCurrent() || !hasSlots) return;
    const slot = Number(select.value || 0);
    if (!slot) return;
    clearInlineError();
    session.setPending(true);
    try {
      const result = await MoveXDRModToSlot(filePath, character, fromSlot, slot);
      const notice = formatMoveNotice(result, t);
      if (notice.level === "success" || notice.level === "warning") {
        session.close("done");
        showNotification(notice.message, notice.level === "success" ? "success" : "warning");
        void refreshFilesKeepFilter({ silent: true });
        finish("done");
        return;
      }
      session.setPending(false);
      showInlineError(notice.message);
    } catch (error) {
      if (!session.isCurrent()) return;
      session.setPending(false);
      showInlineError(errorText(error));
      // 失败常见原因是"目标槽位刚被别的 Mod 占用"：把可选槽位刷新一遍。
      try {
        const refreshed = await PreviewXDRSlotMove(filePath, character, fromSlot, 0);
        if (!session.isCurrent()) return;
        rebuildOptions(refreshed);
      } catch (refreshError) {
        console.warn("刷新槽位列表失败:", refreshError);
      }
    }
  };

  function rebuildOptions(next) {
    const nextOptions = Array.isArray(next?.availableSlots) ? next.availableSlots : [];
    optionsBySlot.clear();
    select.replaceChildren();
    nextOptions.forEach((option) => {
      optionsBySlot.set(option.slot, option);
      const item = document.createElement("option");
      item.value = String(option.slot);
      item.textContent = buildSlotOptionLabel(option, t);
      select.appendChild(item);
    });
    const stillHasSlots = nextOptions.length > 0;
    select.disabled = !stillHasSlots;
    session.confirmBtn.disabled = !stillHasSlots;
    if (!stillHasSlots) {
      showInlineError(t("modals.xdrMove.noSlots"));
      return;
    }
    if (next.defaultSlot) {
      select.value = String(next.defaultSlot);
    }
  }

  session.show();
}
