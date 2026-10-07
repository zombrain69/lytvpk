// 冲突组内的「长按 → 拖动 → 调加载顺序（优先级）」手势。
//
// 设计要点：
// 1) 长按 CONFLICT_DRAG_LONG_PRESS_MS 才进入拖拽，普通点击仍保持"展开/收起分组"，
//    也不会抢走滚动（移动超过容差就直接放弃长按）。
// 2) 进入拖拽后把被拖的行改成 fixed 跟手，原位插一个等高占位符；占位符随指针越过
//    其它行的中线而移动——落点预览是真实布局，不靠 transform 假装。
// 3) 指针事件只记录坐标，所有 DOM 写入都放在 requestAnimationFrame 里（顺畅、不掉帧）。
// 4) 松手只在"顺序真的变了"时回调 onDrop({ path, targetPath, direction })：
//    顺序写入仍复用「提前/延后」按钮那一套后端调用（SetVPKLoadOrder），语义一致。

export const CONFLICT_DRAG_LONG_PRESS_MS = 280;
export const CONFLICT_DRAG_MOVE_TOLERANCE_PX = 8;

const AUTOSCROLL_EDGE_PX = 56;
const AUTOSCROLL_STEP_PX = 12;
const CLICK_SUPPRESS_MS = 400;

// computeInsertionIndex 返回被拖行应插入到「其它行」里的下标（0..rowRects.length）。
// rowRects 是其它行的矩形，按 DOM 顺序给出。
export function computeInsertionIndex(pointerY, rowRects) {
  // 过滤掉拿不到矩形（元素已卸载）的行，返回值始终是"有效行"里的下标。
  const rects = (Array.isArray(rowRects) ? rowRects : []).filter(Boolean);
  for (let index = 0; index < rects.length; index += 1) {
    const rect = rects[index];
    const middle = rect.top + rect.height / 2;
    if (pointerY < middle) return index;
  }
  return rects.length;
}

// planRowDrop 把落点翻译成「相对哪一行、放前面还是后面」：
//   最前 → 放到第一行之前；中间/末尾 → 放到上一行之后（两种写法等价，后者少一次位移）。
export function planRowDrop(otherPaths, insertIndex) {
  const paths = Array.isArray(otherPaths) ? otherPaths : [];
  if (paths.length === 0) return null;
  const clamped = Math.max(0, Math.min(paths.length, insertIndex));
  if (clamped === 0) return { targetPath: paths[0], direction: "before" };
  return { targetPath: paths[clamped - 1], direction: "after" };
}

// describeRowDrop 用最终顺序判断"这一次拖动到底有没有改变顺序"。
// rows / otherPaths 都是"不含被拖行"的路径数组，draggedPath 是当前拖动的 Mod。
export function describeRowDrop(originalPaths, otherPaths, draggedPath, insertIndex) {
  const others = Array.isArray(otherPaths) ? otherPaths : [];
  const clamped = Math.max(0, Math.min(others.length, insertIndex));
  const dropped = [...others.slice(0, clamped), draggedPath, ...others.slice(clamped)];
  const before = Array.isArray(originalPaths) ? originalPaths : [];
  const moved = dropped.length !== before.length || dropped.some((path, index) => path !== before[index]);
  return { moved, dropped };
}

function findScrollParent(element) {
  let node = element?.parentElement || null;
  while (node) {
    const style = typeof getComputedStyle === "function" ? getComputedStyle(node) : null;
    const overflowY = style?.overflowY || "";
    if (/(auto|scroll|overlay)/.test(overflowY) && node.scrollHeight > node.clientHeight + 1) {
      return node;
    }
    node = node.parentElement;
  }
  return null;
}

// attachConflictRowDrag 给冲突组的行容器挂上拖拽手势；返回 detach（用于清理）。
export function attachConflictRowDrag(container, options = {}) {
  if (!container) return () => {};
  const { onDrop, canDrag } = options;
  let pending = null;
  let active = null;
  let suppressClickUntil = 0;

  const rowsInContainer = () => Array.from(container.querySelectorAll(".conflict-vpk-item"));
  const pathOf = (row) => String(row?.dataset?.path || "");
  const isInteractiveTarget = (target) =>
    Boolean(target?.closest?.("button, a, input, select, textarea, label"));

  function clearPending() {
    if (pending?.timer) clearTimeout(pending.timer);
    pending = null;
  }

  function handlePointerDown(event) {
    if (event.button !== 0 || active || pending) return;
    const row = event.target?.closest?.(".conflict-vpk-item");
    if (!row || !container.contains(row)) return;
    if (isInteractiveTarget(event.target)) return;
    if (typeof canDrag === "function" && !canDrag(row)) return;
    pending = {
      row,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      timer: setTimeout(() => {
        const candidate = pending;
        pending = null;
        if (candidate) startDrag(candidate);
      }, CONFLICT_DRAG_LONG_PRESS_MS),
    };
  }

  function handlePointerMove(event) {
    if (pending && event.pointerId === pending.pointerId) {
      const distance = Math.hypot(event.clientX - pending.startX, event.clientY - pending.startY);
      if (distance > CONFLICT_DRAG_MOVE_TOLERANCE_PX) clearPending();
      return;
    }
    if (!active || event.pointerId !== active.pointerId) return;
    active.pointerX = event.clientX;
    active.pointerY = event.clientY;
    active.slotDirty = true;
  }

  function handlePointerUp(event) {
    if (pending && event.pointerId === pending.pointerId) {
      clearPending();
      return;
    }
    if (active && event.pointerId === active.pointerId) finishDrag(true);
  }

  function handlePointerCancel(event) {
    if (pending && event.pointerId === pending.pointerId) {
      clearPending();
      return;
    }
    if (active && event.pointerId === active.pointerId) finishDrag(false);
  }

  function handleClickCapture(event) {
    if (Date.now() >= suppressClickUntil) return;
    // 拖拽（或长按）之后紧跟的那次 click 不能当作"展开/收起分组"。
    event.stopPropagation();
    event.preventDefault();
  }

  function handleScroll() {
    if (active) active.slotDirty = true;
  }

  function handleWindowBlur() {
    finishDrag(false);
  }

  function startDrag(candidate) {
    const { row, pointerId } = candidate;
    if (!document.body.contains(row)) return;
    const rect = row.getBoundingClientRect();
    if (rect.height <= 0) return;

    const originalPaths = rowsInContainer().map(pathOf);
    const placeholder = document.createElement("div");
    placeholder.className = "conflict-drag-placeholder";
    placeholder.style.height = `${rect.height}px`;
    row.parentElement.insertBefore(placeholder, row);

    row.classList.add("is-dragging");
    row.style.position = "fixed";
    row.style.left = `${rect.left}px`;
    row.style.top = `${rect.top}px`;
    row.style.width = `${rect.width}px`;
    row.style.height = `${rect.height}px`;
    row.style.margin = "0";
    container.classList.add("is-reordering");
    document.body.classList.add("conflict-row-dragging");

    active = {
      row,
      placeholder,
      pointerId,
      path: pathOf(row),
      pointerX: candidate.startX,
      pointerY: candidate.startY,
      startPointerY: candidate.startY,
      originalPaths,
      insertIndex: Math.max(0, originalPaths.indexOf(pathOf(row))),
      slotDirty: true,
      raf: 0,
      scrollParent: findScrollParent(container),
    };

    window.addEventListener("pointermove", handlePointerMove, true);
    window.addEventListener("pointerup", handlePointerUp, true);
    window.addEventListener("pointercancel", handlePointerCancel, true);
    window.addEventListener("blur", handleWindowBlur);
    active.raf = requestAnimationFrame(frame);
  }

  function frame() {
    if (!active) return;
    active.raf = requestAnimationFrame(frame);

    if (active.scrollParent) {
      const rect = active.scrollParent.getBoundingClientRect();
      let delta = 0;
      if (active.pointerY < rect.top + AUTOSCROLL_EDGE_PX) delta = -AUTOSCROLL_STEP_PX;
      else if (active.pointerY > rect.bottom - AUTOSCROLL_EDGE_PX) delta = AUTOSCROLL_STEP_PX;
      if (delta) {
        const before = active.scrollParent.scrollTop;
        active.scrollParent.scrollTop = before + delta;
        if (active.scrollParent.scrollTop !== before) active.slotDirty = true;
      }
    }

    const dy = active.pointerY - active.startPointerY;
    active.row.style.transform = `translate3d(0, ${dy}px, 0)`;

    if (active.slotDirty) {
      active.slotDirty = false;
      updateInsertSlot();
    }
  }

  function updateInsertSlot() {
    if (!active) return;
    const others = rowsInContainer().filter((el) => el !== active.row);
    const rects = others.map((el) => el.getBoundingClientRect());
    const index = computeInsertionIndex(active.pointerY, rects);
    if (index === active.insertIndex) return;
    active.insertIndex = index;
    const reference = index < others.length ? others[index] : null;
    active.placeholder.parentElement?.insertBefore(active.placeholder, reference);
  }

  function finishDrag(commit) {
    if (!active) return;
    const state = active;
    active = null;
    if (state.raf) cancelAnimationFrame(state.raf);
    window.removeEventListener("pointermove", handlePointerMove, true);
    window.removeEventListener("pointerup", handlePointerUp, true);
    window.removeEventListener("pointercancel", handlePointerCancel, true);
    window.removeEventListener("blur", handleWindowBlur);

    const others = rowsInContainer().filter((el) => el !== state.row);
    const otherPaths = others.map(pathOf);
    const { moved } = describeRowDrop(state.originalPaths, otherPaths, state.path, state.insertIndex);

    // 先把行放回占位符的位置，再撤掉拖拽期的所有内联样式。
    state.placeholder.parentElement?.insertBefore(state.row, state.placeholder);
    state.placeholder.remove();
    state.row.classList.remove("is-dragging");
    state.row.style.position = "";
    state.row.style.left = "";
    state.row.style.top = "";
    state.row.style.width = "";
    state.row.style.height = "";
    state.row.style.margin = "";
    state.row.style.transform = "";
    container.classList.remove("is-reordering");
    document.body.classList.remove("conflict-row-dragging");
    suppressClickUntil = Date.now() + CLICK_SUPPRESS_MS;

    if (!commit || !moved || typeof onDrop !== "function") return;
    const plan = planRowDrop(otherPaths, state.insertIndex);
    if (!plan) return;
    onDrop({ path: state.path, targetPath: plan.targetPath, direction: plan.direction });
  }

  container.addEventListener("pointerdown", handlePointerDown);
  container.addEventListener("pointermove", handlePointerMove);
  container.addEventListener("pointerup", handlePointerUp);
  container.addEventListener("pointercancel", handlePointerCancel);
  container.addEventListener("click", handleClickCapture, true);
  container.addEventListener("scroll", handleScroll, true);
  container.classList.add("is-drag-enabled");

  return function detachConflictRowDrag() {
    clearPending();
    if (active) finishDrag(false);
    container.classList.remove("is-drag-enabled", "is-reordering");
    container.removeEventListener("pointerdown", handlePointerDown);
    container.removeEventListener("pointermove", handlePointerMove);
    container.removeEventListener("pointerup", handlePointerUp);
    container.removeEventListener("pointercancel", handlePointerCancel);
    container.removeEventListener("click", handleClickCapture, true);
    container.removeEventListener("scroll", handleScroll, true);
  };
}
