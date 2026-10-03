// 加载顺序窗口里两个"规则选择列表"：可搜索 + 虚拟化的 listbox。
//
// 为什么换掉原生 <select multiple size="7">（2026-10-03）：
//   addonlist 有 2599 条时，两个下拉各塞 2599 个 <option>（合计 5,217 个节点），
//   而且 Chromium 每插一批 option 都会重建 <select> 的内部列表；搜索重填要分帧、
//   原生弹层的展开响应也不可控。现在只渲染"视口 ± overscan"的行，
//   选择状态由调用方的 Set 持有（搜索/刷新不丢选择）。
//
// 参考的成熟做法：
//   · WAI-ARIA APG Listbox 模式（role=listbox/option、aria-selected、
//     aria-activedescendant、↑↓/Home/End/Enter/Space 键盘操作）；
//   · TanStack Virtual / react-window 的"只渲染可见项 + 上下占位块"。

import { computePreviewWindow, previewSpacerHeights } from "./load-order-preview-window.mjs";

const RULE_LIST_OVERSCAN_ROWS = 4;
const RULE_LIST_FALLBACK_ROW_PITCH = 30;

/** applyRuleToggle 计算"点一下某一行"之后的选择集（纯函数，便于单测）。 */
export function applyRuleToggle(selected, key, multiple = false) {
  const next = new Set(selected instanceof Set ? selected : []);
  if (!key) return next;
  if (!multiple) {
    return new Set([key]);
  }
  if (next.has(key)) next.delete(key);
  else next.add(key);
  return next;
}

/** nextActiveIndex 键盘移动用：在 [0, total) 内夹紧（total=0 返回 -1）。 */
export function nextActiveIndex(current, delta, total) {
  const count = Math.max(0, Math.floor(Number(total) || 0));
  if (count === 0) return -1;
  const step = Number(delta) || 0;
  const base = Number.isInteger(current) && current >= 0 ? current : step >= 0 ? -1 : count;
  return Math.max(0, Math.min(count - 1, base + (step >= 0 ? 1 : -1)));
}

/**
 * createRuleListbox 建一个虚拟化 listbox。
 *
 * @param {HTMLElement} container 容器（HTML 里是空的 div，运行时补 role/tabindex）
 * @param {{
 *   multiple?: boolean,
 *   getId?: (item:any)=>string,
 *   getLabel?: (item:any)=>string,
 *   onSelectionChange?: (selected: Set<string>)=>void,
 * }} config
 */
export function createRuleListbox(container, config = {}) {
  if (!container) return null;
  const multiple = Boolean(config.multiple);
  const getId = config.getId || ((item) => String(item?.key ?? item?.id ?? ""));
  const getLabel = config.getLabel || ((item) => String(item?.key ?? item ?? ""));
  const onSelectionChange = config.onSelectionChange || (() => {});

  container.classList.add("load-order-rule-list");
  container.setAttribute("role", "listbox");
  container.setAttribute("tabindex", "0");
  if (multiple) container.setAttribute("aria-multiselectable", "true");

  const topSpacer = document.createElement("div");
  topSpacer.className = "load-order-rule-spacer";
  topSpacer.dataset.ruleSpacer = "top";
  topSpacer.style.display = "none";
  const bottomSpacer = document.createElement("div");
  bottomSpacer.className = "load-order-rule-spacer";
  bottomSpacer.dataset.ruleSpacer = "bottom";
  bottomSpacer.style.display = "none";

  const state = {
    items: [],
    selected: new Set(),
    rows: new Map(),
    start: 0,
    end: 0,
    rowPitch: 0,
    activeIndex: -1,
    scrollPending: false,
    destroyed: false,
  };

  const setSpacer = (spacer, height) => {
    const safe = Math.max(0, Number(height) || 0);
    spacer.style.height = `${safe}px`;
    spacer.style.display = safe > 0 ? "" : "none";
  };

  const renderWindow = (force = false) => {
    if (state.destroyed) return;
    const total = state.items.length;
    const { start, end } = computePreviewWindow({
      total,
      rowPitch: state.rowPitch,
      scrollTop: container.scrollTop,
      clientHeight: container.clientHeight,
      overscanRows: RULE_LIST_OVERSCAN_ROWS,
    });
    if (!force && start === state.start && end === state.end) return;

    for (const [index, row] of [...state.rows]) {
      if (index >= start && index < end) continue;
      row.remove?.();
      state.rows.delete(index);
    }

    const buildRow = (index) => {
      const item = state.items[index];
      const key = getId(item);
      const row = document.createElement("div");
      row.className = "load-order-rule-option";
      row.setAttribute("role", "option");
      row.dataset.key = key;
      row.dataset.ruleIndex = String(index);
      row.textContent = getLabel(item);
      row.title = getLabel(item);
      if (state.selected.has(key)) {
        row.classList.add("is-selected");
        row.setAttribute("aria-selected", "true");
      } else {
        row.setAttribute("aria-selected", "false");
      }
      if (index === state.activeIndex) row.classList.add("is-active");
      return row;
    };

    const kept = [...state.rows.keys()].sort((a, b) => a - b);
    if (kept.length === 0) {
      const fragment = document.createDocumentFragment();
      for (let index = start; index < end; index += 1) {
        const row = buildRow(index);
        state.rows.set(index, row);
        fragment.appendChild(row);
      }
      container.insertBefore(fragment, bottomSpacer);
    } else {
      const firstKept = kept[0];
      const lastKept = kept[kept.length - 1];
      if (start < firstKept) {
        const anchor = state.rows.get(firstKept);
        for (let index = start; index < firstKept; index += 1) {
          const row = buildRow(index);
          state.rows.set(index, row);
          container.insertBefore(row, anchor);
        }
      }
      if (end > lastKept + 1) {
        for (let index = lastKept + 1; index < end; index += 1) {
          const row = buildRow(index);
          state.rows.set(index, row);
          container.insertBefore(row, bottomSpacer);
        }
      }
    }

    state.start = start;
    state.end = end;
    if (!state.rowPitch) {
      const firstRow = state.rows.get(start);
      const measured = firstRow?.getBoundingClientRect?.().height || 0;
      state.rowPitch = Math.max(1, Math.round(measured) || RULE_LIST_FALLBACK_ROW_PITCH);
    }
    const heights = previewSpacerHeights({
      total,
      start,
      end,
      rowPitch: state.rowPitch,
    });
    setSpacer(topSpacer, heights.top);
    setSpacer(bottomSpacer, heights.bottom);
  };

  const onScroll = () => {
    if (state.scrollPending || state.destroyed) return;
    state.scrollPending = true;
    const run = () => {
      state.scrollPending = false;
      if (state.destroyed) return;
      renderWindow();
    };
    if (typeof requestAnimationFrame === "function") requestAnimationFrame(run);
    setTimeout(run, 50);
  };

  const setSelection = (selected) => {
    state.selected = new Set(selected instanceof Set ? selected : []);
    for (const [index, row] of state.rows) {
      const key = getId(state.items[index]);
      const isSelected = state.selected.has(key);
      row.classList.toggle("is-selected", isSelected);
      row.setAttribute("aria-selected", isSelected ? "true" : "false");
    }
  };

  const setActiveIndex = (index) => {
    if (state.activeIndex >= 0) {
      state.rows.get(state.activeIndex)?.classList.remove("is-active");
    }
    state.activeIndex = index;
    if (index < 0) {
      container.removeAttribute?.("aria-activedescendant");
      return;
    }
    const total = state.items.length;
    const rowPitch = state.rowPitch || RULE_LIST_FALLBACK_ROW_PITCH;
    const rowTop = index * rowPitch;
    const viewTop = Math.max(0, Number(container.scrollTop) || 0);
    const viewport = Math.max(Number(container.clientHeight) || 0, rowPitch * 7);
    if (rowTop < viewTop) container.scrollTop = rowTop;
    else if (rowTop + rowPitch > viewTop + viewport) container.scrollTop = rowTop + rowPitch - viewport;
    renderWindow(true);
    state.rows.get(index)?.classList.add("is-active");
    const row = state.rows.get(index);
    if (row) {
      if (!row.id) {
        row.id = `${container.id || "load-order-rule"}-opt-${index}`;
      }
      container.setAttribute("aria-activedescendant", row.id);
    }
  };

  const commitSelection = (selected) => {
    state.selected = selected;
    setSelection(selected);
    onSelectionChange(new Set(selected));
  };

  const toggleKey = (key) => {
    commitSelection(applyRuleToggle(state.selected, key, multiple));
  };

  const onClick = (event) => {
    const row = event.target?.closest?.(".load-order-rule-option");
    if (!row || row.parentNode !== container) return;
    const index = Number.parseInt(row.dataset.ruleIndex, 10);
    if (Number.isInteger(index) && index >= 0) state.activeIndex = index;
    toggleKey(row.dataset.key);
  };

  const onKeyDown = (event) => {
    const total = state.items.length;
    if (total === 0) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex(nextActiveIndex(state.activeIndex, event.key === "ArrowDown" ? 1 : -1, total));
      return;
    }
    if (event.key === "Home" || event.key === "End") {
      event.preventDefault();
      setActiveIndex(event.key === "Home" ? 0 : total - 1);
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      const index = state.activeIndex >= 0 ? state.activeIndex : 0;
      const item = state.items[index];
      if (!item) return;
      event.preventDefault();
      setActiveIndex(index);
      toggleKey(getId(item));
    }
  };

  container.replaceChildren(topSpacer, bottomSpacer);
  container.addEventListener("scroll", onScroll, { passive: true });
  container.addEventListener("click", onClick);
  container.addEventListener("keydown", onKeyDown);

  return {
    /** setItems 换一批数据（搜索/刷新），选择集原样保留。 */
    setItems(items) {
      state.items = Array.isArray(items) ? items : [];
      state.rows.forEach((row) => row.remove?.());
      state.rows.clear();
      state.start = -1;
      state.end = -1;
      state.activeIndex = Math.min(state.activeIndex, state.items.length - 1);
      renderWindow(true);
    },
    setSelection,
    getSelection() {
      return new Set(state.selected);
    },
    destroy() {
      state.destroyed = true;
      container.removeEventListener("scroll", onScroll);
      container.removeEventListener("click", onClick);
      container.removeEventListener("keydown", onKeyDown);
      state.rows.forEach((row) => row.remove?.());
      state.rows.clear();
      topSpacer.remove?.();
      bottomSpacer.remove?.();
      container.replaceChildren();
    },
  };
}
