let confirmSessionId = 0;

export function showConfirmModal(title, message, onConfirm, useHtml = false, extraClass = "", onCancel = null) {
  const modal = document.getElementById("confirm-modal");
  const modalContent = modal.querySelector(".modal-content");
  const titleEl = document.getElementById("confirm-title");
  const messageEl = document.getElementById("confirm-message");
  const okBtn = document.getElementById("confirm-ok-btn");
  const cancelBtn = document.getElementById("confirm-cancel-btn");
  const closeBtn = document.getElementById("close-confirm-modal-btn");
  const sessionId = ++confirmSessionId;
  let isConfirming = false;

  const isCurrentSession = () => sessionId === confirmSessionId;

  const setPending = (pending) => {
    if (!isCurrentSession()) return;
    isConfirming = pending;
    okBtn.disabled = pending;
    cancelBtn.disabled = pending;
    closeBtn.disabled = pending;
    modal.dataset.confirming = pending ? "true" : "false";
  };

  if (extraClass) {
    extraClass.split(" ").filter(Boolean).forEach((c) => modalContent.classList.add(c));
  }

  setPending(false);
  titleEl.textContent = title;
  if (useHtml) {
    messageEl.innerHTML = message;
  } else {
    messageEl.textContent = message;
  }
  modal.classList.remove("hidden");

  const cleanup = (reason = "close") => {
    // A previous confirmation may finish after another confirmation has
    // already opened. Only the visible session may hide the modal or clear
    // its callbacks; otherwise an old async action closes the new dialog.
    if (!isCurrentSession()) return;
    setPending(false);
    modal.classList.add("hidden");
    okBtn.onclick = null;
    cancelBtn.onclick = null;
    closeBtn.onclick = null;
    if (extraClass) {
      extraClass.split(" ").filter(Boolean).forEach((c) => modalContent.classList.remove(c));
    }
    if ((reason === "cancel" || reason === "close") && typeof onCancel === "function") {
      try {
        onCancel();
      } catch (error) {
        console.error("Confirm cancel action failed:", error);
      }
    }
  };

  okBtn.onclick = async () => {
    if (!isCurrentSession() || isConfirming) return;

    setPending(true);
    try {
      const result = await onConfirm();
      if (!isCurrentSession()) return;
      if (result !== false) {
        cleanup();
      } else {
        setPending(false);
      }
    } catch (error) {
      if (!isCurrentSession()) return;
      setPending(false);
      console.error("Confirm action failed:", error);
    }
  };

  cancelBtn.onclick = () => {
    if (isCurrentSession() && !isConfirming) cleanup("cancel");
  };
  closeBtn.onclick = () => {
    if (isCurrentSession() && !isConfirming) cleanup("close");
  };
}

/**
 * confirmInApp 把应用内确认弹窗包成 Promise，用来替代 window.confirm。
 *
 * WebView2 里的 window.confirm 是**阻塞式原生对话框**：不受主题与快捷键控制，
 * 自动化/锁屏环境下还会把整个 JS 线程卡住（仓库里 group-dialog-guard 测试
 * 已经为策略组钉住过这条规则）。调用方写：
 *
 *   if (!(await confirmInApp("要删除这条记录吗？", { title: "删除记录" }))) return;
 *
 * 注意 cleanup 在"点了确定"时也会回调 onCancel（reason 默认 "close"），
 * 所以这里用 settled 标记保证只结算一次。
 */
export function confirmInApp(message, options = {}) {
  const { title = "确认", extraClass = "" } = options || {};
  return new Promise((resolve) => {
    let settled = false;
    const settle = (value) => {
      if (settled) return;
      settled = true;
      resolve(value);
    };
    showConfirmModal(
      title,
      message,
      () => {
        settle(true);
      },
      false,
      extraClass,
      () => {
        settle(false);
      },
    );
  });
}
