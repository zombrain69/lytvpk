// 「同类互斥」提示（三个选项）：
//   关闭旧的并启用 / 两者共存 / 取消。
//
// 后端只负责判定（internal/app/singleton_conflict.go：角色 + 具体武器型号），
// 这里负责把选择交给用户并把决定返回给调用方（operations.js 的游戏内启用流程）。

const CHOICES = new Set(["replace", "coexist", "cancel"]);

import { t } from "../../core/i18n-runtime.js";
import {
  REMINDER_KEYS,
  ensureReminderSessionLoaded,
  recordReminderSessionChoice,
  reminderToggleHtml,
} from "../../core/reminders.mjs";

export async function promptSingletonConflict(conflict) {
  // 「提醒窗口」体系：拉一次会话状态，决定本次要不要带「本次运行期间不再显示」勾选。
  await ensureReminderSessionLoaded();
  const modal = document.getElementById("singleton-conflict-modal");
  if (!modal) return Promise.resolve("coexist");

  const message = document.getElementById("singleton-conflict-message");
  const list = document.getElementById("singleton-conflict-list");
  if (message) {
    message.textContent = t("mods.singletonConflict.prompt", {
      label: conflict?.label || conflict?.key || "",
    });
  }
  if (list) {
    list.innerHTML = "";
    (conflict?.conflicts || []).forEach((item) => {
      const row = document.createElement("li");
      row.textContent = item.name || item.path;
      row.title = item.path || "";
      list.appendChild(row);
    });
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

  return new Promise((resolve) => {
    const cleanup = (choice) => {
      // 会话级勾选（仅首次弹窗出现）：只保留到应用关闭；静音后按本次的动作自动处理
      // （replace 沿用；其它 → 仅启用、不动旧的）。
      recordReminderSessionChoice(
        REMINDER_KEYS.singletonConflict,
        choice === "cancel" ? "" : choice,
      );
      modal.classList.add("hidden");
      replaceButton?.removeEventListener("click", onReplace);
      coexistButton?.removeEventListener("click", onCoexist);
      cancelButton?.removeEventListener("click", onCancel);
      closeButton?.removeEventListener("click", onCancel);
      resolve(CHOICES.has(choice) ? choice : "cancel");
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
