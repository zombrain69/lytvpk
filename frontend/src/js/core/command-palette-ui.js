// 命令面板的 DOM 层：打开 / 过滤 / ↑↓ 选择 / Enter 执行 / Esc 关闭。
// 纯逻辑（命令表、检索、排序、状态文案）在 command-palette.mjs 里，这里只做接线。

import { highlightMatches } from "../features/file-list/search-match.mjs";
import {
  COMMANDS,
  RECENT_COMMAND_LIMIT,
  describeCommandPaletteState,
  nextCommandId,
  pushRecentCommand,
  searchCommands,
} from "./command-palette.mjs";

let bound = false;
let activeId = "";
let visibleCommands = [];
// 最近用过的命令（最近在前）。持久化在 localStorage，命名空间独立，坏数据一律当空。
let recentCommandIds = [];
let paletteStorage = null;

const RECENT_STORAGE_KEY = "lytvpk.commandPalette.recent";

function defaultPaletteStorage() {
  try {
    return window.localStorage;
  } catch {
    // 隐私模式 / 无存储权限时退化为"只在本次会话里记"。
    return null;
  }
}

function loadRecentCommands() {
  const storage = paletteStorage;
  if (!storage) return [];
  try {
    const raw = storage.getItem(RECENT_STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.map((value) => String(value ?? "")).filter(Boolean).slice(0, RECENT_COMMAND_LIMIT);
  } catch {
    return [];
  }
}

function saveRecentCommands(ids) {
  const storage = paletteStorage;
  if (!storage) return;
  try {
    storage.setItem(RECENT_STORAGE_KEY, JSON.stringify(ids));
  } catch {
    // 写不进去不影响使用，只是下次打开没有"最近"。
  }
}

/** recordCommandUse 记录一次执行（下一次打开面板就会排在最上面）。 */
function recordCommandUse(id) {
  recentCommandIds = pushRecentCommand(recentCommandIds, id);
  saveRecentCommands(recentCommandIds);
}

function element(id) {
  return document.getElementById(id);
}

/** renderCommandPalette 按当前输入重画列表，并保持光标落在合法命令上。 */
function renderCommandPalette(query) {
  const list = element("command-palette-list");
  const hint = element("command-palette-hint");
  if (!list) return;

  visibleCommands = searchCommands(COMMANDS, query, 10, recentCommandIds);
  if (!visibleCommands.some((command) => command.id === activeId)) {
    activeId = visibleCommands[0]?.id || "";
  }
  const recentSet = new Set(recentCommandIds);

  list.replaceChildren(
    ...visibleCommands.map((command) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = `command-palette-item${command.id === activeId ? " is-active" : ""}`;
      item.dataset.commandId = command.id;
      item.setAttribute("role", "option");
      item.setAttribute("aria-selected", String(command.id === activeId));

      const title = document.createElement("span");
      title.className = "command-title";
      // 命中片段高亮：与 Mod 搜索同一套（连续命中优先，其次逐字）。
      title.innerHTML = highlightMatches(command.title, query);
      item.appendChild(title);

      const hintSpan = document.createElement("span");
      hintSpan.className = "command-hint";
      // 最近用过的加个标记：一眼能看出"这几条是我常点的"。
      hintSpan.textContent = recentSet.has(command.id) ? `最近 · ${command.hint || ""}` : command.hint || "";
      item.appendChild(hintSpan);
      return item;
    }),
  );

  if (visibleCommands.length === 0) {
    const empty = document.createElement("div");
    empty.className = "command-palette-empty";
    empty.textContent = "没有匹配的命令；换个关键词，或清空后浏览全部。";
    list.appendChild(empty);
  }

  if (hint) {
    hint.textContent = describeCommandPaletteState({
      total: COMMANDS.length,
      shown: visibleCommands.length,
      query,
      recentCount: recentCommandIds.length,
    });
  }
}

function setActiveCommand(id) {
  activeId = id || "";
  element("command-palette-list")
    ?.querySelectorAll(".command-palette-item")
    .forEach((item) => {
      const isActive = item.dataset.commandId === activeId;
      item.classList.toggle("is-active", isActive);
      item.setAttribute("aria-selected", String(isActive));
      if (isActive) item.scrollIntoView({ block: "nearest" });
    });
}

/**
 * openCommandPalette / closeCommandPalette 控制面板显隐。
 * 关闭时清空输入与光标，下次打开是干净状态。
 */
export function openCommandPalette() {
  const modal = element("command-palette-modal");
  const input = element("command-palette-input");
  if (!modal || !input) return;
  recentCommandIds = loadRecentCommands();
  modal.classList.remove("hidden");
  input.value = "";
  activeId = "";
  renderCommandPalette("");
  input.focus();
}

export function closeCommandPalette() {
  element("command-palette-modal")?.classList.add("hidden");
}

export function isCommandPaletteOpen() {
  const modal = element("command-palette-modal");
  return Boolean(modal) && !modal.classList.contains("hidden");
}

/**
 * setupCommandPalette 绑定一次事件；runCommand 由调用方注入（id → 实际动作）。
 * 返回 { open, close } 方便测试与快捷键复用。
 */
export function setupCommandPalette({ runCommand, storage = null }) {
  if (bound) return { open: openCommandPalette, close: closeCommandPalette };
  bound = true;
  paletteStorage = storage || defaultPaletteStorage();
  recentCommandIds = loadRecentCommands();

  const modal = element("command-palette-modal");
  const input = element("command-palette-input");
  const list = element("command-palette-list");
  if (!modal || !input || !list) return { open: openCommandPalette, close: closeCommandPalette };

  input.addEventListener("input", () => renderCommandPalette(input.value));
  input.addEventListener("keydown", (event) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setActiveCommand(
        nextCommandId(visibleCommands.map((command) => command.id), activeId, event.key === "ArrowDown" ? 1 : -1),
      );
      return;
    }
    if (event.key === "Enter") {
      event.preventDefault();
      const target = visibleCommands.find((command) => command.id === activeId);
      if (!target) return;
      closeCommandPalette();
      recordCommandUse(target.id);
      void runCommand(target.id);
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      closeCommandPalette();
    }
  });

  list.addEventListener("click", (event) => {
    const item = event.target.closest?.(".command-palette-item");
    const id = String(item?.dataset.commandId || "");
    if (!id) return;
    closeCommandPalette();
    recordCommandUse(id);
    void runCommand(id);
  });

  list.addEventListener("mousemove", (event) => {
    const item = event.target.closest?.(".command-palette-item");
    if (!item) return;
    if (String(item.dataset.commandId || "") === activeId) return;
    setActiveCommand(String(item.dataset.commandId || ""));
  });

  // 点面板外关闭（与其它临时弹层一致）。
  modal.addEventListener("click", (event) => {
    if (event.target === modal) closeCommandPalette();
  });

  return { open: openCommandPalette, close: closeCommandPalette };
}
