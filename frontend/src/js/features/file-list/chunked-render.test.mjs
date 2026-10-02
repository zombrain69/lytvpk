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

// 窗口化列表现在是"回收式"的：滚到哪画到哪，滚出视口的卡片会被回收，
// 所以 DOM 规模与"用户看过多少张卡"无关（只追加的旧实现真机涨到 133,213 个节点）。
const windowPaths = (container) =>
  container.children
    .filter((node) => node.dataset?.path)
    .map((node) => node.dataset.path);

test("回收式窗口：滚动时窗口跟着走、节点数有界，滚回去能重建", async () => {
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
  const firstWindow = windowPaths(container);
  assert.ok(firstWindow.length > 0, "首屏必须有内容");

  // 模拟"滚到列表中后段"：容器几何由测试桩提供
  container.clientHeight = 800;
  container.scrollTop = 56000;
  container.dispatchEvent?.(new Event("scroll"));
  // render.js 用 rAF/定时器兜底调度；等一拍
  await new Promise((resolve) => setTimeout(resolve, 120));
  const scrolledWindow = windowPaths(container);
  assert.ok(scrolledWindow.length > 0, "滚动后窗口里必须有内容");
  assert.notDeepEqual(scrolledWindow, firstWindow, "窗口要跟着滚动换一批行");
  assert.equal(
    scrolledWindow.some((path) => firstWindow.includes(path)),
    false,
    "滚远之后旧窗口的节点必须被回收（换一批全新的行）",
  );
  assert.ok(
    container.children.length < 200,
    `回收后节点数要有界：${afterRender} → ${container.children.length}`,
  );
  assert.equal(
    new Set(scrolledWindow).size,
    scrolledWindow.length,
    "窗口里不能出现重复行",
  );

  // 滚回顶部：被回收的行要能重新物化出来（数据来自 specs，不依赖旧 DOM）
  container.scrollTop = 0;
  container.dispatchEvent?.(new Event("scroll"));
  await new Promise((resolve) => setTimeout(resolve, 120));
  assert.deepEqual(windowPaths(container), firstWindow, "滚回顶部应重建同一批行");
});

// 回收式窗口最容易翻车的地方是占位块算错 → 总高度随窗口位置漂移 → 滚动条乱跳。
// 列表模式下（无 gap）总高度必须严格守恒：上占位 + 行数*行高 + 下占位 = 条目数*行高。
test("回收式窗口：占位块高度精确守恒（滚动条不会随窗口漂移）", async () => {
  const elements = installDomStub();
  const { appState } = await import("../state.js");
  const { renderFileList } = await import("./render.js");

  const files = buildFiles(600);
  appState.displayMode = "list";
  appState.vpkFiles = files;
  appState.allVpkFiles = files;
  appState.searchCursorPath = "";
  const container = elements.get("file-list") || document.getElementById("file-list");
  const ROW_PITCH = 56; // 列表模式无行高信息时的兜底值（见 measureListMetrics）

  const totalHeight = () => {
    const spacers = container.children
      .filter((node) => node.className?.includes("file-list-window-spacer"))
      .map((node) => parseFloat(node.style.height) || 0);
    const rows = windowPaths(container).length;
    const spacerSum = spacers.reduce((sum, value) => sum + value, 0);
    return spacerSum + rows * ROW_PITCH;
  };

  renderFileList();
  container.clientHeight = 800;
  const expected = files.length * ROW_PITCH;
  assert.equal(totalHeight(), expected, "首屏总高度必须是完整列表高度");

  for (const scrollTop of [10000, 33000, 56000]) {
    container.scrollTop = scrollTop;
    container.dispatchEvent?.(new Event("scroll"));
    await new Promise((resolve) => setTimeout(resolve, 120));
    assert.equal(totalHeight(), expected, `滚到 ${scrollTop} 后总高度仍要守恒`);
  }
});

// 按优先级定位：目标可能在第 1500 行，窗口里根本没它。
// 旧实现 flushFileListRender() 会把 2000 行全部造出来（大库上几百毫秒冻结）；
// 现在按索引把窗口搬过去，DOM 规模不变、仍然保留上下占位块。
test("按索引定位：窗口外的行物化出来，但不整表补齐", async () => {
  const elements = installDomStub();
  const { appState } = await import("../state.js");
  const { renderFileList, hasPendingFileListRender, revealFileByPath } = await import("./render.js");

  const files = buildFiles(2000);
  appState.displayMode = "list";
  appState.vpkFiles = files;
  appState.allVpkFiles = files;
  appState.searchCursorPath = "";
  const container = elements.get("file-list") || document.getElementById("file-list");

  renderFileList();
  container.clientHeight = 800;
  const target = files[1500];
  assert.equal(windowPaths(container).includes(target.path), false, "目标行一开始不该在窗口里");

  const node = revealFileByPath(target.path, { block: "center" });
  assert.ok(node, "定位要返回落点节点");
  assert.equal(node.dataset?.path, target.path, "落点必须是目标行");
  assert.ok(windowPaths(container).includes(target.path), "目标行要真的进 DOM");
  assert.ok(container.children.length < 200, `定位不应把整表都造出来：${container.children.length}`);
  assert.equal(hasPendingFileListRender(), true, "窗口化列表仍有未物化的行");
  assert.equal(
    container.children.some((child) => child.className?.includes("file-list-window-spacer")),
    true,
    "定位后仍然保留占位块（还是回收式窗口）",
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

// 真机回归（2026-10-02）：筛「游戏内：未记录」而库里 0 个未记录时，筛选条已经高亮、
// 状态栏也更新了，列表却一张卡都没清掉（还是上一批 30 张「游戏内开启」卡）；
// 逐张把未记录 Mod 点成「游戏内关闭」点到最后一张时同样停在旧卡片上。
// 根因：appendListRenderChunk 对空 specs 走 `end <= pending.index` 短路，
// replace 分支的 replaceChildren 永远不执行。
test("筛选结果为空时必须真的清空列表（不能停在上一批卡片上）", async () => {
  const elements = installDomStub();
  const { appState } = await import("../state.js");
  const { renderFileList, hasPendingFileListRender } = await import("./render.js");

  for (const displayMode of ["card", "list"]) {
    const files = buildFiles(400);
    // 每轮都换一个干净的容器：同一个桩容器跨模式复用会互相干扰。
    elements.delete("file-list");
    appState.displayMode = displayMode;
    appState.vpkFiles = files;
    appState.allVpkFiles = files;
    const container = document.getElementById("file-list");

    renderFileList();
    assert.ok(container.children.length > 0, `${displayMode}：先要有内容可清`);

    // 筛选后 0 命中：列表必须真的空掉（占位块也不能留下）。
    appState.vpkFiles = [];
    renderFileList();
    assert.equal(
      container.children.length,
      0,
      `${displayMode}：0 命中时列表必须清空，实际还有 ${container.children.length} 个节点`,
    );
    assert.equal(hasPendingFileListRender(), false, `${displayMode}：清空后不应留下待渲染项`);

    // 再回到有内容：要能正常重新物化（不能因为清空而把窗口状态弄坏）。
    appState.vpkFiles = files;
    renderFileList();
    assert.ok(container.children.length > 0, `${displayMode}：清空后重新筛选要能恢复内容`);
  }
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
  // 回收式窗口：只保留视口 ± overscan，滚出窗口的节点回收（见 renderListWindow）。
  assert.match(renderSource, /function startListWindow/, "缺少回收式窗口实现");
  assert.match(renderSource, /function computeWindowRange/, "缺少视口 ± overscan 的窗口换算");
  assert.match(renderSource, /function renderListWindow/, "缺少窗口渲染/回收实现");
  assert.match(
    renderSource,
    /container\.addEventListener\("scroll", handleListWindowScroll/,
    "缺少滚动触发的窗口同步",
  );

  // 键盘 ↑↓ 的结果集必须来自 appState：DOM 里只有已物化的一部分。
  assert.match(
    read("../app-runtime.js"),
    /const paths = \(appState\.vpkFiles \|\| \[\]\)/,
    "键盘 ↑↓ 的结果集要从 appState 取（窗口化后 DOM 不完整）",
  );
  // 按优先级定位改成"按索引把窗口搬过去"（回收式窗口下 O(1)），不再整表补齐。
  assert.match(read("./sorting.js"), /revealFileByPath\(/, "按优先级定位要走按索引定位");
  assert.equal(
    /flushFileListRender\(\)/.test(read("./sorting.js")),
    false,
    "定位不应再整表补齐（大库上是一次几百毫秒的冻结）",
  );
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
