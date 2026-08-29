const HANDLE_DIRECTIONS = ["n", "ne", "e", "se", "s", "sw", "w", "nw"];
const VIEWPORT_GUTTER = 8;
const DEFAULT_MIN_WIDTH = 320;
const DEFAULT_MIN_HEIGHT = 220;

export function makeModalResizable(content) {
  if (!content || content.dataset.modalResizable === "true") return content;
  content.dataset.modalResizable = "true";
  content.classList.add("is-modal-resizable");

  HANDLE_DIRECTIONS.forEach((direction) => {
    const handle = document.createElement("span");
    handle.className = `modal-resize-handle modal-resize-handle-${direction}`;
    handle.dataset.resizeDirection = direction;
    handle.setAttribute("aria-hidden", "true");
    handle.addEventListener("pointerdown", (event) => startResize(event, content, direction));
    content.appendChild(handle);
  });

  return content;
}

export function setupModalResizers(root = document) {
  if (!root?.querySelectorAll) return;
  root.querySelectorAll(".modal-content:not(.embedded-page-content)").forEach(makeModalResizable);

  const body = root.body || (root.nodeType === Node.ELEMENT_NODE ? root : null);
  if (!body || body.dataset.modalResizeObserverBound === "true" || typeof MutationObserver === "undefined") return;
  body.dataset.modalResizeObserverBound = "true";
  const observer = new MutationObserver((mutations) => {
    mutations.forEach(({ addedNodes }) => {
      addedNodes.forEach((node) => {
        if (node.nodeType !== Node.ELEMENT_NODE) return;
        if (node.matches?.(".modal-content:not(.embedded-page-content)")) makeModalResizable(node);
        node.querySelectorAll?.(".modal-content:not(.embedded-page-content)").forEach(makeModalResizable);
      });
    });
  });
  observer.observe(body, { childList: true, subtree: true });
}

function startResize(event, content, direction) {
  if (event.button !== 0) return;
  event.preventDefault();
  event.stopPropagation();

  const rect = content.getBoundingClientRect();
  const computed = getComputedStyle(content);
  const availableWidth = Math.max(1, window.innerWidth - VIEWPORT_GUTTER * 2);
  const availableHeight = Math.max(1, window.innerHeight - VIEWPORT_GUTTER * 2);
  const minWidth = Math.min(
    Math.max(DEFAULT_MIN_WIDTH, parsePixels(computed.minWidth, DEFAULT_MIN_WIDTH)),
    availableWidth,
  );
  const minHeight = Math.min(
    Math.max(DEFAULT_MIN_HEIGHT, parsePixels(computed.minHeight, DEFAULT_MIN_HEIGHT)),
    availableHeight,
  );
  const maxWidth = availableWidth;
  const maxHeight = availableHeight;
  const originalTransition = content.style.transition;

  // Pin the current visual rectangle before dragging an edge, so resizing from
  // any side feels direct even though the modal is normally flex-centered.
  Object.assign(content.style, {
    position: "fixed",
    left: `${rect.left}px`,
    top: `${rect.top}px`,
    right: "auto",
    bottom: "auto",
    width: `${rect.width}px`,
    height: `${rect.height}px`,
    maxWidth: "none",
    maxHeight: "none",
    transform: "none",
    transition: "none",
  });

  const startX = event.clientX;
  const startY = event.clientY;
  const startRight = rect.right;
  const startBottom = rect.bottom;
  const onMove = (moveEvent) => {
    const dx = moveEvent.clientX - startX;
    const dy = moveEvent.clientY - startY;
    let left = rect.left;
    let top = rect.top;
    let width = rect.width;
    let height = rect.height;

    if (direction.includes("e")) width = rect.width + dx;
    if (direction.includes("s")) height = rect.height + dy;
    if (direction.includes("w")) {
      width = rect.width - dx;
      left = startRight - width;
    }
    if (direction.includes("n")) {
      height = rect.height - dy;
      top = startBottom - height;
    }

    width = clamp(width, minWidth, maxWidth);
    height = clamp(height, minHeight, maxHeight);
    if (direction.includes("w")) left = startRight - width;
    if (direction.includes("n")) top = startBottom - height;
    left = clamp(left, VIEWPORT_GUTTER, window.innerWidth - width - VIEWPORT_GUTTER);
    top = clamp(top, VIEWPORT_GUTTER, window.innerHeight - height - VIEWPORT_GUTTER);

    content.style.left = `${left}px`;
    content.style.top = `${top}px`;
    content.style.width = `${width}px`;
    content.style.height = `${height}px`;
  };
  const stopResize = () => {
    document.removeEventListener("pointermove", onMove);
    document.removeEventListener("pointerup", stopResize);
    document.removeEventListener("pointercancel", stopResize);
    document.body.classList.remove("is-modal-resizing", `is-modal-resizing-${direction}`);
    // Keep the new size/position for the next open. Restore only transition
    // metadata that would otherwise make the next drag laggy.
    content.style.transition = originalTransition;
  };

  document.body.classList.add("is-modal-resizing", `is-modal-resizing-${direction}`);
  document.addEventListener("pointermove", onMove);
  document.addEventListener("pointerup", stopResize, { once: true });
  document.addEventListener("pointercancel", stopResize, { once: true });
}

function parsePixels(value, fallback) {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}
