// 分类侧边栏的 DOM 层：把 category-tree.mjs 的树画出来，并把操作翻译成筛选。
//
// 与既有功能的关系：树上的每个节点最终都落到**已有的**二级标签 / 位置 / 游戏内状态筛选上
// （toggleSelection / selectAggregate / clearGroup 由 app-runtime 注入，内部走 filters.js），
// 这里不复制任何过滤逻辑；分类定义来自 PRESET_GROUPS（唯一事实源）。

import { appState } from "../state.js";
import {
  buildCategoryTree,
  countCategoryNode,
  flattenCategoryTree,
  nodeIsSelected,
  selectionForNode,
} from "./category-tree.mjs";

const PANEL_ID = "category-sidebar";
const TREE_ID = "category-tree";
const SEARCH_ID = "category-search";
const TOGGLE_ID = "category-sidebar-toggle";
const CLEAR_ID = "category-clear-btn";

let deps = {
  toggleSelection: async () => {},
  clearSelection: async () => {},
  selectAggregate: async () => {},
  clearGroup: async () => {},
  loadVisible: () => null, // null = 没设置过 → 默认收起
  saveVisible: () => {},
};
let expandedIds = new Set();
let query = "";
let bound = false;
// 上一次算好的数量：切换选中/展开时只重画高亮，不再重复统计（数字也不会消失）。
let cachedCounts = new Map();

export function setupCategorySidebar(options = {}) {
  deps = { ...deps, ...options };
  const panel = document.getElementById(PANEL_ID);
  if (!panel) return;
  ensureWorkspaceWrapper();
  if (!bound) {
    bound = true;
    // 展开/收起状态持久化：记住上次的选择；从未设置过则默认收起，不占列表空间。
    setSidebarVisible(deps.loadVisible() === true, { persist: false });
    document.getElementById(TOGGLE_ID)?.addEventListener("click", () => toggleCategorySidebar());
    document.getElementById(CLEAR_ID)?.addEventListener("click", () => {
      void deps.clearSelection();
      renderCategorySidebar({ counts: false });
    });
    const search = document.getElementById(SEARCH_ID);
    search?.addEventListener("input", () => {
      query = search.value || "";
      renderCategorySidebar({ counts: false });
    });
  }
  renderCategorySidebar();
}

/**
 * ensureWorkspaceWrapper 在运行时把"分类侧边栏 + 文件列表"包进同一个 flex 行。
 *
 * 为什么不用静态 HTML 嵌套：真实事故 —— index.html 里手工嵌套少/多一个 </div>，
 * 浏览器解析后把包裹层提到了 #main-screen 层级，整页被挤到下半屏、上半屏一片空白。
 * 运行时包裹只看"这两个元素还在不在、是否已经包好"，不受手写层级影响，且可重复调用。
 */
function ensureWorkspaceWrapper() {
  const aside = document.getElementById(PANEL_ID);
  const container = document.querySelector(".file-list-container");
  if (!aside || !container) return;
  const existing = document.querySelector(".mods-workspace");
  if (existing && existing.contains(aside) && existing.contains(container)) return;

  const wrapper = document.createElement("div");
  wrapper.className = "mods-workspace";
  const parent = container.parentElement;
  if (!parent) return;
  parent.insertBefore(wrapper, container);
  wrapper.append(aside, container);
}

export function toggleCategorySidebar(force) {
  const panel = document.getElementById(PANEL_ID);
  if (!panel) return;
  const willShow = typeof force === "boolean" ? force : panel.classList.contains("hidden");
  setSidebarVisible(willShow, { persist: true });
}

function setSidebarVisible(visible, options = {}) {
  const panel = document.getElementById(PANEL_ID);
  if (!panel) return;
  panel.classList.toggle("hidden", !visible);
  const toggle = document.getElementById(TOGGLE_ID);
  if (toggle) {
    toggle.classList.toggle("active", visible);
    toggle.setAttribute("aria-expanded", String(visible));
  }
  if (visible) renderCategorySidebar({ counts: false });
  if (options.persist) {
    try {
      deps.saveVisible(visible);
    } catch (error) {
      console.warn("保存分类侧边栏状态失败:", error);
    }
  }
}

/** refreshCategorySidebar 在扫描/刷新之后重算数量（分组并集，不重复计数）。 */
export function refreshCategorySidebar() {
  renderCategorySidebar({ counts: true });
}

/** syncCategorySidebarSelection 在筛选状态被别处改动时（工具栏标签菜单等）只更新高亮。 */
export function syncCategorySidebarSelection() {
  renderCategorySidebar({ counts: false });
}

/** collectScannedTags 从当前扫描结果里收集所有二级标签（"其它标签"兜底分支用）。 */
function collectScannedTags(files) {
  const tags = new Set();
  for (const file of files) {
    for (const tag of file?.secondaryTags || []) {
      if (tag) tags.add(tag);
    }
  }
  return [...tags];
}

function createNodeAction(label, title, onClick) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "category-node-action";
  button.textContent = label;
  button.title = title;
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    void Promise.resolve(onClick()).finally(() => renderCategorySidebar({ counts: false }));
  });
  return button;
}

function renderCategorySidebar(options = {}) {
  const container = document.getElementById(TREE_ID);
  if (!container) return;
  const files = Array.isArray(appState.allVpkFiles) ? appState.allVpkFiles : [];
  const withCounts = options.counts !== false;
  const tree = buildCategoryTree({ allTags: collectScannedTags(files) });
  const rows = flattenCategoryTree(tree, { expanded: expandedIds, query });

  if (withCounts) cachedCounts = new Map();
  const countOf = (node) => {
    if (withCounts && !cachedCounts.has(node.id)) {
      cachedCounts.set(node.id, countCategoryNode(node, files));
    }
    return cachedCounts.has(node.id) ? cachedCounts.get(node.id) : null;
  };

  container.replaceChildren();
  for (const row of rows) {
    const { node, depth, hasChildren, expanded } = row;
    const line = document.createElement("div");
    line.className = "category-node";
    line.style.setProperty("--category-depth", String(depth));
    line.dataset.categoryId = node.id;
    if (hasChildren) line.classList.add("has-children");
    if (
      nodeIsSelected(node, {
        tags: appState.selectedSecondaryTags || [],
        locations: appState.selectedLocations || [],
        gameStates: appState.selectedGameStates || [],
      })
    ) {
      line.classList.add("is-active");
    }

    if (hasChildren) {
      const chevron = document.createElement("button");
      chevron.type = "button";
      chevron.className = "category-node-chevron";
      chevron.textContent = expanded ? "▾" : "▸";
      chevron.title = expanded ? "收起" : "展开";
      chevron.addEventListener("click", (event) => {
        event.stopPropagation();
        if (expandedIds.has(node.id)) expandedIds.delete(node.id);
        else expandedIds.add(node.id);
        renderCategorySidebar({ counts: false });
      });
      line.appendChild(chevron);
    } else {
      const spacer = document.createElement("span");
      spacer.className = "category-node-spacer";
      line.appendChild(spacer);
    }

    const selection = selectionForNode(node);
    const label = document.createElement("button");
    label.type = "button";
    label.className = "category-node-label";
    label.textContent = node.label;
    label.disabled = !selection;
    label.title = selection ? `只看：${node.label}（再点一次取消）` : `${node.label}（展开后选子分类）`;
    if (selection) {
      label.addEventListener("click", () => {
        if (hasChildren) expandedIds.add(node.id);
        const action = node.kind === "all" ? deps.clearSelection() : deps.toggleSelection(selection);
        void Promise.resolve(action).finally(() => renderCategorySidebar({ counts: false }));
      });
    }
    line.appendChild(label);

    // 组/子组保留旧「内容预设」菜单的两个快捷动作：查看全部（聚合标签）与清空本组。
    if (hasChildren && node.allTag) {
      line.appendChild(
        createNodeAction("全部", `只看${node.label}（用聚合标签）`, () => deps.selectAggregate(node.allTag)),
      );
    }
    if (hasChildren && Array.isArray(node.tags) && node.tags.length > 0) {
      line.appendChild(createNodeAction("清空", `取消${node.label}里已选的标签`, () => deps.clearGroup(node.tags)));
    }

    const count = countOf(node);
    if (count !== null) {
      const badge = document.createElement("span");
      badge.className = "category-node-count";
      badge.textContent = String(count);
      badge.title = `${count} 个 Mod`;
      line.appendChild(badge);
    }
    container.appendChild(line);
  }
}
