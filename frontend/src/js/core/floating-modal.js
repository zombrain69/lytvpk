// 通用「浮动窗口」能力：复杂的管理类弹窗可以变成"压在主界面之上、但不遮挡主界面"的窗口。
//
// 解决的问题（用户反馈）：打开管理窗口后主界面被遮罩 + backdrop-filter 虚化，
// 想同时看主界面（列表、分组筛选、Mod 开关）就没法看。
//
// 浮动模式做三件事：
//   1. 去掉遮罩与背景模糊（CSS：.modal.is-floating）→ 主界面看得清；
//   2. 容器 pointer-events: none、窗口本体 auto → 主界面点得到（等同"非模态"）；
//   3. 标题栏可拖动移动窗口，位置按窗口记住（localStorage）。
//
// 缩放不需要在这里做：core/modal-resizer.js 已经给每个 .modal-content 装了八个方向的把手。
import { clampWindowSize } from "./window-geometry.mjs";


const FLOATING_CLASS = "is-floating";
const POSITION_KEY_PREFIX = "lytvpk.floatingPos.";
const DEFAULT_KEY_PREFIX = "lytvpk.floating.";

const registry = new Map();

// 标题栏选择器：绝大多数弹窗是 .modal-header；「加载顺序优化」用的是 .load-order-header；
// 再兜底到内容区第一个子元素，保证任何弹窗都能拿到拖动把手与开关按钮的位置。
const DEFAULT_HEADER_SELECTOR = ".modal-header, .load-order-header, .modal-content > :first-child";

// 浮动时给内容设过的内联样式。必须用连字符写法：
// CSSStyleDeclaration.removeProperty 不认驼峰（"maxHeight" 会静默失败），
// 真实缺陷：残留的 max-height: none 让「加载顺序优化」停靠后撑到 1901px 高、
// 标题栏被顶到视口外，用户点不到「浮动窗口」按钮。
const INLINE_POSITION_PROPS = [
  "position",
  "left",
  "top",
  "right",
  "bottom",
  "width",
  "height",
  "max-width",
  "max-height",
  "transform",
];

function resolveModal(modal) {
  if (!modal) return null;
  if (typeof modal === "string") return document.getElementById(modal);
  return modal;
}

function readStored(key) {
  try {
    return window.localStorage?.getItem(key);
  } catch (error) {
    return null;
  }
}

function writeStored(key, value) {
  try {
    window.localStorage?.setItem(key, value);
  } catch (error) {
    /* 隐私模式 / 存储被禁用时静默降级：不记住偏好，但功能仍然可用 */
  }
}

/** isFloatingModal 当前是不是浮动状态。 */
export function isFloatingModal(modal) {
  return Boolean(resolveModal(modal)?.classList.contains(FLOATING_CLASS));
}

/**
 * applyFloatingModal 切换浮动 / 停靠（模态）两种状态。
 * 只改样式与位置，不做持久化 —— 持久化由调用方（或 setupFloatingModal）负责。
 */
export function applyFloatingModal(modal, enabled, options = {}) {
  const element = resolveModal(modal);
  const content = element?.querySelector(".modal-content");
  if (!element || !content) return false;
  const on = Boolean(enabled);
  element.classList.toggle(FLOATING_CLASS, on);
  if (!on) {
    // 回到"居中模态"：清掉拖动留下的内联定位。
    clearInlinePosition(content);
    // 兜底：内容仍然比视口高时，改成顶部对齐 —— 否则 flex 居中会把标题栏顶到视口外
    // （标题栏里就是「浮动窗口」按钮，顶出去就点不回来了）。
    if (content.getBoundingClientRect().top < 0) {
      element.style.alignItems = "flex-start";
    } else {
      element.style.removeProperty("align-items");
    }
    return on;
  }
  element.style.removeProperty("align-items");
  const defaultPosition =
    typeof options.defaultPosition === "function" ? options.defaultPosition(element) : null;
  if (defaultPosition) {
    pinContent(content, defaultPosition);
  }
  return on;
}

function clearInlinePosition(content) {
  INLINE_POSITION_PROPS.forEach((key) => content.style.removeProperty(key));
}

function pinContent(content, position) {
  const rect = content.getBoundingClientRect();
  const { width, height } = clampWindowSize(
    Number(position?.width || rect.width),
    Number(position?.height || rect.height),
    window.innerWidth,
    window.innerHeight,
  );
  const left = Number(position?.left ?? rect.left);
  const top = Number(position?.top ?? rect.top);
  const maxLeft = Math.max(0, window.innerWidth - width);
  const maxTop = Math.max(0, window.innerHeight - Math.min(height, window.innerHeight));
  Object.assign(content.style, {
    position: "fixed",
    left: `${Math.round(Math.min(Math.max(0, left), maxLeft))}px`,
    top: `${Math.round(Math.min(Math.max(0, top), maxTop))}px`,
    width: `${Math.round(width)}px`,
    height: `${Math.round(height)}px`,
    right: "auto",
    bottom: "auto",
    maxWidth: "none",
    maxHeight: "none",
    transform: "none",
  });
}

/** startModalDrag 把标题栏变成拖动把手（只在浮动状态下生效）。 */
export function startModalDrag(event, modal, options = {}) {
  const element = resolveModal(modal);
  const content = element?.querySelector(".modal-content");
  if (!element || !content || !element.classList.contains(FLOATING_CLASS)) return;
  if (event.button !== 0) return;
  if (event.target.closest("button, input, select, textarea, label, a")) return;
  event.preventDefault();
  const rect = content.getBoundingClientRect();
  const startX = event.clientX;
  const startY = event.clientY;
  const width = rect.width;
  const height = rect.height;
  const move = (moveEvent) => {
    pinContent(content, {
      left: rect.left + moveEvent.clientX - startX,
      top: rect.top + moveEvent.clientY - startY,
      width,
      height,
    });
  };
  const stop = () => {
    document.removeEventListener("pointermove", move);
    document.removeEventListener("pointerup", stop);
    document.removeEventListener("pointercancel", stop);
    document.body.classList.remove("is-modal-dragging");
    const now = content.getBoundingClientRect();
    const positionKey = options.positionKey;
    if (positionKey) {
      writeStored(
        positionKey,
        JSON.stringify({ left: now.left, top: now.top, width: now.width, height: now.height }),
      );
    }
  };
  document.body.classList.add("is-modal-dragging");
  document.addEventListener("pointermove", move);
  document.addEventListener("pointerup", stop, { once: true });
  document.addEventListener("pointercancel", stop, { once: true });
}

function readStoredPosition(key) {
  const raw = readStored(key);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!Number.isFinite(parsed?.left) || !Number.isFinite(parsed?.top)) return null;
    return parsed;
  } catch (error) {
    return null;
  }
}

/**
 * setupFloatingModal 给一个管理类弹窗装上浮动能力（幂等）。
 *
 * @param {string|HTMLElement} modal 弹窗元素或 id
 * @param {object} [options]
 * @param {string} [options.key] 偏好与位置用的存储键后缀（默认用弹窗 id）
 * @param {boolean} [options.defaultFloating] 首次打开是否浮动（默认 false）
 * @param {() => object|null} [options.defaultPosition] 浮动时默认停靠位置（不传则保持原位置）
 * @param {HTMLElement|null} [options.control] 已有的勾选框（如管理窗口里的那个）；不传则自动在标题栏插一个按钮
 * @param {(enabled: boolean) => void} [options.onChange] 状态变化回调（例如写回 config.json）
 * @param {() => boolean|null} [options.read] 读取偏好（覆盖 localStorage）
 * @param {(enabled: boolean) => void} [options.write] 写入偏好（覆盖 localStorage）
 * @param {boolean} [options.closeOnBackdrop] 停靠（模态）时点窗口外是否关闭（默认 true）。
 *        会改 Mod 状态的窗口（例如「问题 Mod 查找」）应传 false —— 它要求排查期间保持打开。
 */
export function setupFloatingModal(modal, options = {}) {
  const element = resolveModal(modal);
  if (!element) return null;
  const id = element.id || options.key || "";
  const positionKey = `${POSITION_KEY_PREFIX}${options.key || id}`;
  const preferenceKey = `${DEFAULT_KEY_PREFIX}${options.key || id}`;
  const existing = registry.get(id);
  if (existing) return existing;

  const readPreference = () => {
    if (typeof options.read === "function") return options.read();
    const stored = readStored(preferenceKey);
    if (stored === null) return null;
    return stored === "1";
  };
  const writePreference = (enabled) => {
    if (typeof options.write === "function") options.write(enabled);
    else writeStored(preferenceKey, enabled ? "1" : "0");
  };

  const control = options.control instanceof HTMLElement ? options.control : null;
  let button = null;
  if (!control) {
    // 自动在标题栏插一个「浮动」按钮：管理类弹窗都能一键切到不遮挡主界面的模式。
    const header = element.querySelector(options.headerSelector || DEFAULT_HEADER_SELECTOR);
    if (header) {
      button = document.createElement("button");
      button.type = "button";
      button.className = "modal-float-toggle";
      button.title =
        "浮动窗口：去掉背景虚化，可以直接操作主界面（拖动标题栏移动、拖动边缘缩放）；再点一次恢复居中模态。";
      button.addEventListener("click", (event) => {
        event.stopPropagation();
        setFloating(!isFloatingModal(element));
      });
      const closeButton = header.querySelector(".close-btn");
      if (closeButton) header.insertBefore(button, closeButton);
      else header.appendChild(button);
    }
  }

  const syncControl = (enabled) => {
    if (control && "checked" in control) control.checked = enabled;
    if (button) {
      button.textContent = enabled ? "停靠窗口" : "浮动窗口";
      button.classList.toggle("is-active", enabled);
      button.setAttribute("aria-pressed", String(enabled));
    }
  };

  function setFloating(enabled, { persist = true, notify = true } = {}) {
    applyFloatingModal(element, enabled, {
      defaultPosition: enabled
        ? () => readStoredPosition(positionKey) || options.defaultPosition?.() || null
        : null,
    });
    syncControl(Boolean(enabled));
    if (persist) writePreference(Boolean(enabled));
    if (notify && typeof options.onChange === "function") options.onChange(Boolean(enabled));
    return Boolean(enabled);
  }

  const applyCurrent = ({ persist = false, notify = false } = {}) => {
    const stored = readPreference();
    const enabled = stored === null ? Boolean(options.defaultFloating) : Boolean(stored);
    setFloating(enabled, { persist, notify });
  };

  control?.addEventListener("change", (event) => setFloating(Boolean(event.target.checked)));
  // 停靠（模态）时点窗口外关闭：以前每个窗口各写一份，新接入的窗口就漏了 —— 这里统一处理。
  // 浮动状态不需要（容器 pointer-events: none，事件根本到不了），所以直接放行。
  if (options.closeOnBackdrop !== false) {
    element.addEventListener("mousedown", (event) => {
      if (element.classList.contains(FLOATING_CLASS)) return;
      if (event.target !== element) return;
      // 走窗口自己的关闭按钮，保证各窗口的收尾逻辑（停任务、清状态）照常执行。
      const closer = element.querySelector(".close-btn");
      if (closer) {
        event.preventDefault();
        closer.click();
      }
    });
  }
  element
    .querySelector(options.dragHandleSelector || DEFAULT_HEADER_SELECTOR)
    ?.addEventListener("pointerdown", (event) => startModalDrag(event, element, { positionKey }));
  // 双击标题栏 = 重置这个窗口记住的位置与大小。
  // 这是"窗口被拖到屏幕外 / 拉得比屏幕还大"时的逃生口：不用去清 localStorage。
  element
    .querySelector(options.dragHandleSelector || DEFAULT_HEADER_SELECTOR)
    ?.addEventListener("dblclick", (event) => {
      if (event.target.closest("button, input, select, textarea, label, a")) return;
      if (positionKey) {
        try {
          window.localStorage?.removeItem(positionKey);
        } catch (error) {
          /* 存储不可用时忽略：下面的样式仍然会复位 */
        }
      }
      clearInlinePosition(content);
      if (element.classList.contains(FLOATING_CLASS)) {
        // 浮动状态下清掉内联几何后，再按当前布局钉一次，避免掉回"半浮动"的样式错位。
        applyCurrent();
      }
    });
  // 弹窗每次打开时重新应用一次：位置可能在上次拖动后变了，偏好也可能被外部改过。
  const observer = new MutationObserver(() => {
    if (!element.classList.contains("hidden")) applyCurrent();
  });
  observer.observe(element, { attributes: true, attributeFilter: ["class"] });

  const api = {
    element,
    id,
    isFloating: () => isFloatingModal(element),
    setFloating,
    refresh: applyCurrent,
  };
  registry.set(id, api);
  applyCurrent();
  return api;
}

/** getFloatingModal 返回已注册的浮动窗口（方便其它模块联动）。 */
/**
 * resetAllWindowGeometry 清掉所有"浮动窗口位置与大小"的记忆，并把当前浮动窗口
 * 恢复到默认（CSS 居中）位置。
 *
 * 用途：把某个窗口拖到屏幕外 / 拉到超大之后的"一键自救"；由命令面板的
 * 「重置所有窗口的位置与大小」调用。返回被清掉的记忆条数。
 */
export function resetAllWindowGeometry() {
  let cleared = 0;
  try {
    for (let index = window.localStorage.length - 1; index >= 0; index -= 1) {
      const key = window.localStorage.key(index);
      if (key && key.startsWith(POSITION_KEY_PREFIX)) {
        window.localStorage.removeItem(key);
        cleared += 1;
      }
    }
  } catch (error) {
    /* 隐私模式 / 存储被禁用：下面的样式复位仍然会执行 */
  }

  registry.forEach((api) => {
    const content = api?.element?.querySelector(".modal-content");
    if (content) clearInlinePosition(content);
    api?.refresh?.();
  });
  return cleared;
}

export function getFloatingModal(id) {
  return registry.get(String(id || "")) || null;
}
