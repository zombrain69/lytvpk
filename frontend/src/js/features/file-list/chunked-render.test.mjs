// 大结果集分帧渲染：首帧只造一屏，其余按帧补齐；需要完整 DOM 的入口先 flush。
//
// 真机量化（2904 个 Mod、卡片视图）：整份列表一次造完 = 519ms 长任务，
// 期间点什么都没反应 —— 用户感受到的就是"点一下明显卡顿"。
// 这里用最小 DOM 桩 import 真实的 render.js 跑一遍分帧逻辑（不是读源码猜行为）。

import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { register } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

// render.js 会（间接）import Wails 生成的绑定，那些模块用 Vite 风格的省略扩展名导入。
register("../../test-utils/extension-resolve-hook.mjs", import.meta.url);

const here = path.dirname(fileURLToPath(import.meta.url));

function makeNode(tag = "div") {
  const classes = new Set();
  const listeners = new Map();
  return {
    tagName: String(tag).toUpperCase(),
    // 真实 DOM 里 className 与 classList 是同一份数据的两种视图；
    // 桩也必须这样（否则 `card.className = "file-card"` 之后
    // classList.contains("file-card") 会是 false，复用判断直接失效）。
    get className() {
      return [...classes].join(" ");
    },
    set className(value) {
      classes.clear();
      String(value || "")
        .split(/\s+/)
        .filter(Boolean)
        .forEach((name) => classes.add(name));
    },
    children: [],
    dataset: {},
    style: {},
    textContent: "",
    innerHTML: "",
    title: "",
    type: "",
    checked: false,
    disabled: false,
    value: "",
    classList: {
      add(...names) {
        names.forEach((name) => classes.add(name));
      },
      remove(...names) {
        names.forEach((name) => classes.delete(name));
      },
      toggle(name, force) {
        const next = force === undefined ? !classes.has(name) : Boolean(force);
        if (next) classes.add(name);
        else classes.delete(name);
        return next;
      },
      contains(name) {
        return classes.has(name);
      },
    },
    // 真实 DOM 插入 DocumentFragment 时会"摊平"成它的子节点，桩也要一致，
    // 否则统计到的永远是 1（只有那个 fragment）。
    appendChild(child) {
      if (child?.tagName === "FRAGMENT") {
        child.children.forEach((item) => this.appendChild(item));
      } else {
        this.children.push(child);
        child.parentNode = this;
      }
      return child;
    },
    insertBefore(child, before) {
      if (child?.tagName === "FRAGMENT") {
        [...child.children].forEach((item) => this.insertBefore(item, before));
        return child;
      }
      const index = this.children.indexOf(before);
      if (index < 0) {
        this.children.push(child);
      } else {
        this.children.splice(index, 0, child);
      }
      child.parentNode = this;
      return child;
    },
    append(...items) {
      items.forEach((item) => this.appendChild(item));
    },
    replaceChildren(...items) {
      this.children = [];
      items.flat().forEach((item) => this.appendChild(item));
    },
    // 原地协调（patch 策略）要用：按位置替换 + 裁掉多余的行。
    replaceChild(newNode, oldNode) {
      const index = this.children.indexOf(oldNode);
      if (index < 0) throw new Error("replaceChild 的目标不是当前子节点");
      const existing = this.children.indexOf(newNode);
      if (existing >= 0) {
        this.children.splice(existing, 1);
        this.children[this.children.indexOf(oldNode)] = newNode;
      } else {
        this.children[index] = newNode;
      }
      newNode.parentNode = this;
      oldNode.parentNode = null;
      return oldNode;
    },
    get lastElementChild() {
      return this.children[this.children.length - 1] || null;
    },
    // 详情/绑定用到的选择器在测试里统一返回"空节点"，避免到处判空。
    querySelector() {
      return makeNode();
    },
    querySelectorAll() {
      return [];
    },
    addEventListener(type, handler) {
      if (typeof handler !== "function") return;
      if (!listeners.has(type)) listeners.set(type, []);
      listeners.get(type).push(handler);
    },
    removeEventListener(type, handler) {
      const list = listeners.get(type);
      if (!list) return;
      const index = list.indexOf(handler);
      if (index >= 0) list.splice(index, 1);
    },
    dispatchEvent(event) {
      (listeners.get(event?.type) || []).forEach((handler) => handler(event));
      return true;
    },
    setAttribute() {},
    getAttribute() {
      return "";
    },
    removeAttribute() {},
    closest() {
      return null;
    },
    remove() {
      const parent = this.parentNode;
      if (!parent) return;
      const index = parent.children.indexOf(this);
      if (index >= 0) parent.children.splice(index, 1);
      this.parentNode = null;
    },
    // 重建卡片时会保留旧预览图：renderedPreview.replaceWith(previousPreview)。
    replaceWith(node) {
      const parent = this.parentNode;
      if (!parent) return;
      const index = parent.children.indexOf(this);
      if (index >= 0) {
        parent.children[index] = node;
        node.parentNode = parent;
      }
      this.parentNode = null;
    },
    scrollIntoView() {},
    getBoundingClientRect() {
      return { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0 };
    },
    scrollTop: 0,
    scrollHeight: 0,
    clientHeight: 0,
  };
}

function installDomStub() {
  const elements = new Map();
  globalThis.document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, makeNode());
      return elements.get(id);
    },
    querySelector() {
      return makeNode();
    },
    querySelectorAll() {
      return [];
    },
    createElement: (tag) => makeNode(tag),
    createDocumentFragment: () => makeNode("fragment"),
    addEventListener() {},
    dispatchEvent() {},
    removeEventListener() {},
    body: makeNode(),
  };
  globalThis.CSS = { escape: (value) => String(value) };
  globalThis.getComputedStyle = () => ({
    gridTemplateColumns: "300px 300px 300px",
    rowGap: "16px",
    overflowY: "auto",
  });
  // 卡片预览用 IntersectionObserver 惰性加载：桩里只要不抛错即可。
  globalThis.IntersectionObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  // 有些模块（例如 workshop/detail-images.js）在顶层就往 window 上挂函数，
  // 给它一个最小 window，避免 import 阶段就炸。
  globalThis.window = globalThis.window || {
    addEventListener() {},
    removeEventListener() {},
    matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
    localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
  };
  return elements;
}

function buildFiles(count) {
  return Array.from({ length: count }, (_, index) => ({
    path: `C:\\addons\\bulk-${index}.vpk`,
    name: `bulk-${index}.vpk`,
    title: `bulk-${index}`,
    location: "root",
    size: 1024 + index,
    enabled: true,
    gameEnabled: index % 2 === 0,
    gameStateKnown: true,
  }));
}

test("大结果集只物化首屏 + 底部占位块；flush 才补齐全部", async () => {
  const elements = installDomStub();
  const { appState } = await import("../state.js");
  const { renderFileList, flushFileListRender, hasPendingFileListRender } = await import(
    "./render.js"
  );

  const files = buildFiles(500);
  appState.displayMode = "list";
  appState.vpkFiles = files;
  appState.allVpkFiles = files;
  appState.searchCursorPath = "";
  const container = elements.get("file-list") || document.getElementById("file-list");

  renderFileList();
  const firstPaint = container.children.length;
  assert.ok(firstPaint > 0, "首帧必须已经有内容（不能是空列表）");
  assert.ok(
    firstPaint < files.length,
    `首帧不应一次造完：实际 ${firstPaint}/${files.length}`,
  );
  assert.equal(hasPendingFileListRender(), true, "应留下待补齐的分帧渲染");
  // 未物化的部分用底部占位块撑出滚动条（否则用户根本滚不到后面）。
  const spacer = container.children[container.children.length - 1];
  assert.ok(
    spacer?.className?.includes("file-list-window-spacer"),
    "大列表末尾要有窗口占位块",
  );
  assert.ok(normalize(spacer.style.height) > 0, "占位块要有高度");

  flushFileListRender();
  assert.equal(container.children.length, files.length, "flush 后应补齐全部行");
  assert.equal(hasPendingFileListRender(), false, "flush 后不再有待补项");
  assert.equal(
    container.children.some((node) => node.className?.includes("file-list-window-spacer")),
    false,
    "补齐后占位块要移除",
  );
});

const normalize = (value) => parseFloat(String(value || "0")) || 0;

test("窗口化列表：滚到附近才继续物化（不是一次性补完）", async () => {
  const elements = installDomStub();
  const { appState } = await import("../state.js");
  const { renderFileList, hasPendingFileListRender } = await import("./render.js");

  const files = buildFiles(1200);
  appState.displayMode = "card";
  appState.vpkFiles = files;
  appState.allVpkFiles = files;
  const container = elements.get("file-list") || document.getElementById("file-list");

  renderFileList();
  const afterRender = container.children.length;
  assert.ok(afterRender < 200, `首屏不应超过约两屏：实际 ${afterRender}`);
  assert.equal(hasPendingFileListRender(), true, "还有大量条目没物化");

  // 模拟"滚到底部附近"：容器几何由测试桩提供
  container.clientHeight = 800;
  container.scrollHeight = 40000;
  container.scrollTop = 39000;
  container.dispatchEvent?.(new Event("scroll"));
  // render.js 用 rAF/定时器兜底调度；等一拍
  await new Promise((resolve) => setTimeout(resolve, 120));
  assert.ok(
    container.children.length > afterRender,
    `滚到附近应继续物化：${afterRender} → ${container.children.length}`,
  );
});

test("列表重画会作废上一轮没补完的分帧渲染", async () => {
  const elements = installDomStub();
  const { appState } = await import("../state.js");
  const { renderFileList, flushFileListRender, hasPendingFileListRender } = await import(
    "./render.js"
  );

  appState.displayMode = "list";
  appState.vpkFiles = buildFiles(400);
  appState.allVpkFiles = appState.vpkFiles;
  const container = elements.get("file-list") || document.getElementById("file-list");

  renderFileList();
  assert.equal(hasPendingFileListRender(), true);
  // 第二轮：内容减半（模拟搜索），上一轮的待补项必须被丢弃，只补新的一轮。
  appState.vpkFiles = buildFiles(200);
  renderFileList();
  flushFileListRender();
  assert.equal(
    container.children.length,
    200,
    "只应剩第二轮的行：上一轮的待补项不能继续往列表里塞",
  );
  assert.equal(hasPendingFileListRender(), false);
});

// 条目数没变的重画（刷新 / 复检 / 单个开关）必须走"原地协调"：
// 原来一律 replaceChildren(fragment)，哪怕只改了 1 张卡也会把 2904 个节点
// 整批摘下来再插回去 —— 真机实测那是"自动复检后重绘"那一波 121ms 长任务。
test("同长度重画只替换变化的卡片，未变化的是同一批 DOM 节点", async () => {
  const elements = installDomStub();
  const { appState } = await import("../state.js");
  const { renderFileList, flushFileListRender } = await import("./render.js");

  const files = buildFiles(60);
  appState.displayMode = "list";
  appState.vpkFiles = files;
  appState.allVpkFiles = files;
  const container = elements.get("file-list") || document.getElementById("file-list");

  renderFileList();
  flushFileListRender();
  const firstPass = [...container.children];
  assert.equal(firstPass.length, files.length);

  // 改一张卡的状态（只有它会变）后重画：条目数没变 → 走 patch。
  // 行模式下每次都会新建节点，所以这里改用"卡片模式"来验证节点复用。
  appState.displayMode = "card";
  renderFileList();
  flushFileListRender();
  const cardsPass1 = [...container.children];
  assert.equal(cardsPass1.length, files.length, "卡片模式也要补齐全部");

  // 只改一个文件的状态：其余卡片应当原样复用（同一个 DOM 对象）。
  appState.vpkFiles[5].gameEnabled = !appState.vpkFiles[5].gameEnabled;
  appState.vpkFiles = [...appState.vpkFiles];
  renderFileList();
  flushFileListRender();
  const cardsPass2 = [...container.children];
  assert.equal(cardsPass2.length, files.length);
  const reused = cardsPass1.filter((node, index) => node === cardsPass2[index]).length;
  // 调试信息：确认真的是"卡片模式 + 同长度"，而不是走了别的分支。
  assert.equal(appState.displayMode, "card");
  assert.ok(cardsPass1[0]?.className?.includes("file-card"), `第一轮应是卡片：${cardsPass1[0]?.className}`);
  assert.ok(cardsPass2[0]?.className?.includes("file-card"), `第二轮应是卡片：${cardsPass2[0]?.className}`);
  assert.ok(
    reused >= files.length - 3,
    `未变化的卡片必须复用同一批 DOM 节点：复用 ${reused}/${files.length}`,
  );
});

test("列表渲染接线：按需物化 + 只有真正需要整表的入口才 flush", () => {
  const read = (relative) => readFileSync(path.resolve(here, relative), "utf8");
  const renderSource = read("./render.js");
  assert.match(renderSource, /const FILE_LIST_RENDER_CHUNK = \d+/, "缺少分帧批次大小");
  assert.match(
    renderSource,
    /startChunkedListRender\(container, specs, "card"/,
    "卡片模式也要分帧（真机最卡的正是卡片视图）",
  );
  assert.match(renderSource, /startChunkedListRender\(\s*\n?\s*container,/, "列表模式也要分帧");
  // 按需物化：大列表首屏之后不再自动补，改成滚到附近才补（见 maybeMaterializeForScroll）。
  assert.match(renderSource, /function startListWindow/, "缺少窗口化（按需物化）实现");
  assert.match(renderSource, /function maybeMaterializeForScroll/, "缺少滚动触发的物化");

  // 键盘 ↑↓ 的结果集必须来自 appState：DOM 里只有已物化的一部分。
  assert.match(
    read("../app-runtime.js"),
    /const paths = \(appState\.vpkFiles \|\| \[\]\)/,
    "键盘 ↑↓ 的结果集要从 appState 取（窗口化后 DOM 不完整）",
  );
  // 只有"按优先级定位"这种明确的单点跳转才补齐整表（它就是要把某一行动到视野里）。
  assert.match(read("./sorting.js"), /flushFileListRender\(\);/, "按优先级定位前要先补齐");
  // 框选与检查面板高亮只看已渲染的行：为它们把 2904 张卡补齐会让一次点击变成一秒级卡顿。
  assert.equal(
    /flushFileListRender\(\)/.test(read("./box-selection.js")),
    false,
    "框选不应再触发整表补齐",
  );
  assert.match(
    read("../mods/mod-panel.js"),
    /file-list:materialized[\s\S]{0,80}syncSelectedRow/,
    "新物化的行出现时要重新同步检查面板高亮",
  );
});
