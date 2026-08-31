let activeMessageModalSession = null;
let messageModalSessionCounter = 0;

const DEFAULT_CONFIRM_TEXT = "确定";

// #message-modal is intentionally shared by diagnostics and update flows. A
// session owns its buttons and callbacks so an older async flow cannot clear
// or close a message opened later by another feature.
export function beginMessageModalSession({ onClose } = {}) {
  activeMessageModalSession?.close("replaced");

  const modal = document.getElementById("message-modal");
  const titleEl = document.getElementById("message-modal-title");
  const contentEl = document.getElementById("message-modal-content");
  const confirmBtn = document.getElementById("message-modal-confirm-btn");
  const closeBtn = document.getElementById("close-message-modal-btn");
  const footer = confirmBtn?.parentElement;
  if (!modal || !titleEl || !contentEl || !confirmBtn || !closeBtn || !footer) {
    return null;
  }

  const sessionId = ++messageModalSessionCounter;
  const ownedButtons = new Set();
  const ownedClasses = new Map();
  let closed = false;

  const isCurrent = () => activeMessageModalSession === session && !closed;

  const close = (reason = "close") => {
    if (!isCurrent()) return false;

    closed = true;
    activeMessageModalSession = null;
    modal.classList.add("hidden");
    ownedButtons.forEach((button) => button.remove());
    ownedButtons.clear();
    ownedClasses.forEach((classNames, target) => {
      classNames.forEach((className) => target.classList.remove(className));
    });
    ownedClasses.clear();
    contentEl.replaceChildren();
    confirmBtn.textContent = DEFAULT_CONFIRM_TEXT;
    confirmBtn.disabled = false;
    closeBtn.disabled = false;
    confirmBtn.onclick = null;
    closeBtn.onclick = null;
    delete modal.dataset.messageModalSession;

    try {
      onClose?.(reason);
    } catch (error) {
      console.error("消息弹窗关闭回调失败:", error);
    }
    return true;
  };

  const addActionButton = (button, before = confirmBtn) => {
    if (!button || !isCurrent()) return null;
    button.dataset.messageModalSession = String(sessionId);
    ownedButtons.add(button);
    footer.insertBefore(button, before);
    return button;
  };

  const addClass = (target, className) => {
    if (!target || !className || !isCurrent()) return;
    let classNames = ownedClasses.get(target);
    if (!classNames) {
      classNames = new Set();
      ownedClasses.set(target, classNames);
    }
    className.split(" ").filter(Boolean).forEach((name) => {
      classNames.add(name);
      target.classList.add(name);
    });
  };

  const addModalClass = (className) => addClass(modal, className);

  const setPending = (pending) => {
    if (!isCurrent()) return;
    const disabled = !!pending;
    confirmBtn.disabled = disabled;
    closeBtn.disabled = disabled;
    ownedButtons.forEach((button) => {
      button.disabled = disabled;
    });
  };

  const show = () => {
    if (!isCurrent()) return false;
    modal.dataset.messageModalSession = String(sessionId);
    modal.classList.remove("hidden");
    return true;
  };

  const session = {
    modal,
    titleEl,
    contentEl,
    confirmBtn,
    closeBtn,
    footer,
    isCurrent,
    close,
    addActionButton,
    addClass,
    addModalClass,
    setPending,
    show,
  };
  activeMessageModalSession = session;

  return session;
}

export function showMessageModal(title, message, onConfirm) {
  const session = beginMessageModalSession();
  if (!session) return null;

  session.titleEl.textContent = title || "提示";
  session.contentEl.textContent = message || "";
  session.confirmBtn.onclick = async () => {
    if (!session.isCurrent()) return;
    session.setPending(true);
    try {
      session.close("confirm");
      await onConfirm?.();
    } catch (error) {
      console.error("消息弹窗确认回调失败:", error);
    }
  };
  session.closeBtn.onclick = () => session.close("close");
  session.show();
  return session;
}
