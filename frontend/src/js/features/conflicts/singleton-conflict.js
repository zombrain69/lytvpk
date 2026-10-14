// 「同类互斥」提示（三个选项）：
//   关闭所选并启用（逐条勾选）/ 两者共存 / 取消。
//
// 后端负责判定（internal/app/singleton_conflict.go：角色 + 具体武器型号），并用冲突分析的
// 文件口径给每条候选定级：
//   - 严重冲突（severe）：重叠含 critical 文件 / 大面积或整体重叠 → 默认勾选（建议关闭）；
//   - 普通冲突（normal）：仅局部重叠（如只与 HUD 图标类文件重叠）→ 默认不勾选。
// 这里负责把逐条选择交给用户，并把决定返回给调用方（operations.js 的游戏内启用流程）。

const CHOICES = new Set(["replace", "coexist", "cancel"]);

import { t } from "../../core/i18n-runtime.js";
import {
  REMINDER_KEYS,
  ensureReminderSessionLoaded,
  recordReminderSessionChoice,
  reminderToggleHtml,
} from "../../core/reminders.mjs";

function escapeHtmlText(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** renderConflictItem 渲染单条候选项：勾选框（严重默认勾选）+ 定级徽标 + 名称 + 重叠信息。 */
function renderConflictItem(item) {
  const row = document.createElement("li");
  row.className = "singleton-conflict-item";
  const severe = item?.severity === "severe";
  const count = Number(item?.overlapCount) || 0;
  const path = String(item?.path || "");
  const titleParts = [path];
  if (Array.isArray(item?.overlapFiles) && item.overlapFiles.length > 0) {
    titleParts.push("", ...item.overlapFiles);
  }
  row.title = titleParts.join("\n");
  const overlapText =
    count > 0
      ? t("mods.singletonConflict.overlap", { count })
      : t("mods.singletonConflict.overlapNone");
  row.innerHTML =
    `<label class="singleton-conflict-label">` +
    `<input type="checkbox" data-singleton-path="${escapeHtmlText(path)}"${severe ? " checked" : ""}>` +
    `<span class="singleton-severity ${severe ? "is-severe" : "is-normal"}">${escapeHtmlText(
      t(severe ? "mods.singletonConflict.severity.severe" : "mods.singletonConflict.severity.normal"),
    )}</span>` +
    `<span class="singleton-conflict-name">${escapeHtmlText(item?.name || path)}</span>` +
    `<span class="singleton-conflict-overlap">${escapeHtmlText(overlapText)}</span>` +
    `</label>`;
  return row;
}

export async function promptSingletonConflict(conflict) {
  // 「提醒窗口」体系：拉一次会话状态，决定本次要不要带「本次运行期间不再显示」勾选。
  await ensureReminderSessionLoaded();
  const modal = document.getElementById("singleton-conflict-modal");
  if (!modal) return { choice: "coexist", paths: [] };

  const message = document.getElementById("singleton-conflict-message");
  const list = document.getElementById("singleton-conflict-list");
  if (message) {
    message.textContent = t("mods.singletonConflict.prompt", {
      label: conflict?.label || conflict?.key || "",
    });
  }
  if (list) {
    list.innerHTML = "";
    (conflict?.conflicts || []).forEach((item) => list.appendChild(renderConflictItem(item)));
  }
  const sessionRow = document.getElementById("singleton-conflict-session-row");
  if (sessionRow) {
    sessionRow.innerHTML = reminderToggleHtml(
      REMINDER_KEYS.singletonConflict,
      t("mods.reminder.sessionSuppress"),
    );
  }
  modal.classList.remove("hidden");

  const replaceButton = document.getElementById("singleton-conflict-replace");
  const coexistButton = document.getElementById("singleton-conflict-coexist");
  const cancelButton = document.getElementById("singleton-conflict-cancel");
  const closeButton = document.getElementById("singleton-conflict-close-btn");

  const selectedPaths = () =>
    list
      ? [...list.querySelectorAll("input[data-singleton-path]")]
          .filter((box) => box.checked)
          .map((box) => box.getAttribute("data-singleton-path") || "")
          .filter((path) => path !== "")
      : [];

  const updateReplaceButton = () => {
    if (!replaceButton) return;
    const count = selectedPaths().length;
    replaceButton.disabled = count === 0;
    replaceButton.textContent =
      count > 0
        ? t("mods.singletonConflict.replaceSelected", { count })
        : t("mods.singletonConflict.replace");
  };
  const onListChange = () => updateReplaceButton();
  list?.addEventListener("change", onListChange);
  updateReplaceButton();

  return new Promise((resolve) => {
    const cleanup = (choice) => {
      const paths = choice === "replace" ? selectedPaths() : [];
      // 会话级勾选（仅首次弹窗出现）：只保留到应用关闭；静音后按本次的动作自动处理
      // （replace → 本次运行内自动关闭"严重冲突"档；其它 → 仅启用、不动旧的）。
      recordReminderSessionChoice(
        REMINDER_KEYS.singletonConflict,
        choice === "cancel" ? "" : choice,
      );
      modal.classList.add("hidden");
      list?.removeEventListener("change", onListChange);
      replaceButton?.removeEventListener("click", onReplace);
      coexistButton?.removeEventListener("click", onCoexist);
      cancelButton?.removeEventListener("click", onCancel);
      closeButton?.removeEventListener("click", onCancel);
      resolve({ choice: CHOICES.has(choice) ? choice : "cancel", paths });
    };
    const onReplace = () => cleanup("replace");
    const onCoexist = () => cleanup("coexist");
    const onCancel = () => cleanup("cancel");
    replaceButton?.addEventListener("click", onReplace);
    coexistButton?.addEventListener("click", onCoexist);
    cancelButton?.addEventListener("click", onCancel);
    closeButton?.addEventListener("click", onCancel);
  });
}

export function closeSingletonConflictPrompt() {
  document.getElementById("singleton-conflict-modal")?.classList.add("hidden");
}
