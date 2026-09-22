// 应用内文本输入弹窗：替代 WebView2 下不可靠的 window.prompt，
// 与已有 confirm 弹窗保持一致的会话隔离（旧回调不能关闭新弹窗）。

let promptSessionId = 0;

/**
 * showPromptModal 打开一个输入弹窗。
 * @param {string} title 标题
 * @param {string} message 说明文字
 * @param {object} [options]
 * @param {string} [options.defaultValue] 初始值
 * @param {string} [options.placeholder] 占位文字
 * @param {string} [options.confirmText] 确认按钮文案
 * @param {(value: string) => (void | Promise<void>)} options.onConfirm 确认回调（空值不会触发）
 * @param {() => void} [options.onCancel] 取消回调
 * @returns {object | null} 会话对象，缺少 DOM 时返回 null
 */
export function showPromptModal(title, message, options = {}) {
  const { defaultValue = "", placeholder = "", confirmText = "确定", onConfirm, onCancel } = options;

  const modal = document.getElementById("prompt-modal");
  const titleEl = document.getElementById("prompt-modal-title");
  const messageEl = document.getElementById("prompt-modal-message");
  const inputEl = document.getElementById("prompt-modal-input");
  const okBtn = document.getElementById("prompt-modal-ok-btn");
  const cancelBtn = document.getElementById("prompt-modal-cancel-btn");
  const closeBtn = document.getElementById("close-prompt-modal-btn");
  if (!modal || !titleEl || !messageEl || !inputEl || !okBtn || !cancelBtn || !closeBtn) {
    return null;
  }

  const sessionId = ++promptSessionId;
  let pending = false;
  const isCurrentSession = () => sessionId === promptSessionId;

  const setPending = (value) => {
    if (!isCurrentSession()) return;
    pending = value;
    okBtn.disabled = value;
    cancelBtn.disabled = value;
    closeBtn.disabled = value;
    inputEl.disabled = value;
  };

  const cleanup = (reason = "close") => {
    if (!isCurrentSession()) return;
    setPending(false);
    modal.classList.add("hidden");
    okBtn.onclick = null;
    cancelBtn.onclick = null;
    closeBtn.onclick = null;
    inputEl.onkeydown = null;
    inputEl.value = "";
    if ((reason === "cancel" || reason === "close") && typeof onCancel === "function") {
      try {
        onCancel();
      } catch (error) {
        console.error("Prompt cancel action failed:", error);
      }
    }
  };

  const submit = async () => {
    if (!isCurrentSession() || pending) return;
    const value = String(inputEl.value || "").trim();
    if (!value) {
      inputEl.focus();
      return;
    }
    setPending(true);
    try {
      const result = await onConfirm?.(value);
      if (!isCurrentSession()) return;
      if (result !== false) cleanup("confirm");
      else setPending(false);
    } catch (error) {
      if (!isCurrentSession()) return;
      setPending(false);
      console.error("Prompt action failed:", error);
    }
  };

  titleEl.textContent = title || "输入";
  messageEl.textContent = message || "";
  inputEl.value = defaultValue;
  inputEl.placeholder = placeholder;
  okBtn.textContent = confirmText;
  inputEl.onkeydown = (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      void submit();
    } else if (event.key === "Escape") {
      event.preventDefault();
      cleanup("cancel");
    }
  };
  okBtn.onclick = () => void submit();
  cancelBtn.onclick = () => cleanup("cancel");
  closeBtn.onclick = () => cleanup("close");

  modal.classList.remove("hidden");
  // 让用户可以直接输入，无需再点一次输入框。
  requestAnimationFrame(() => {
    if (!isCurrentSession()) return;
    inputEl.focus();
    inputEl.select();
  });

  return {
    modal,
    inputEl,
    isCurrent: isCurrentSession,
    close: cleanup,
  };
}
