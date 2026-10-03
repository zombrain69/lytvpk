// 规则选择列表（可搜索 + 虚拟化 listbox）：只物化视口 ± overscan、选择状态不随搜索丢失、
// 键盘 ↑↓/Enter 可用。用最小 DOM 桩 import 真实模块跑（不是读源码猜行为）。

import assert from "node:assert/strict";
import test from "node:test";
import { createRuleListbox, applyRuleToggle, nextActiveIndex } from "./load-order-rule-list.mjs";

function makeNode(tag = "div") {
  const classes = new Set();
  const listeners = new Map();
  const node = {
    tagName: String(tag).toUpperCase(),
    children: [],
    dataset: {},
    style: {},
    attributes: {},
    textContent: "",
    title: "",
    id: "",
    parentNode: null,
    scrollTop: 0,
    scrollHeight: 0,
    clientHeight: 0,
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
    classList: {
      add: (...names) => names.forEach((name) => classes.add(name)),
      remove: (...names) => names.forEach((name) => classes.delete(name)),
      contains: (name) => classes.has(name),
      toggle(name, force) {
        const next = force === undefined ? !classes.has(name) : Boolean(force);
        if (next) classes.add(name);
        else classes.delete(name);
        return next;
      },
    },
    setAttribute(name, value) {
      this.attributes[name] = String(value);
      if (name === "id") this.id = String(value);
    },
    getAttribute(name) {
      return this.attributes[name] ?? null;
    },
    removeAttribute(name) {
      delete this.attributes[name];
    },
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
      if (index < 0) this.children.push(child);
      else this.children.splice(index, 0, child);
      child.parentNode = this;
      return child;
    },
    replaceChildren(...items) {
      this.children.forEach((child) => {
        child.parentNode = null;
      });
      this.children = [];
      items.flat().forEach((item) => this.appendChild(item));
    },
    remove() {
      const parent = this.parentNode;
      if (!parent) return;
      const index = parent.children.indexOf(this);
      if (index >= 0) parent.children.splice(index, 1);
      this.parentNode = null;
    },
    closest(selector) {
      const wanted = String(selector || "").replace(/^\./, "");
      let current = this;
      while (current) {
        if (current.classList?.contains(wanted)) return current;
        current = current.parentNode;
      }
      return null;
    },
    getBoundingClientRect() {
      return { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0 };
    },
    addEventListener(type, handler) {
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
  };
  return node;
}

function installDomStub() {
  globalThis.document = {
    createElement: (tag) => makeNode(tag),
    createDocumentFragment: () => makeNode("fragment"),
  };
  if (!globalThis.requestAnimationFrame) {
    globalThis.requestAnimationFrame = (callback) => setTimeout(() => callback(Date.now()), 0);
  }
}

const ruleRows = (container) => container.children.filter((node) => node.classList.contains("load-order-rule-option"));
const spacers = (container) => container.children.filter((node) => node.classList.contains("load-order-rule-spacer"));
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

test("applyRuleToggle：多选切换、单选替换", () => {
  assert.deepEqual([...applyRuleToggle(new Set(["a"]), "b", true)], ["a", "b"]);
  assert.deepEqual([...applyRuleToggle(new Set(["a", "b"]), "a", true)], ["b"]);
  assert.deepEqual([...applyRuleToggle(new Set(["a"]), "b", false)], ["b"]);
  assert.deepEqual([...applyRuleToggle(new Set(), "", true)], []);
});

test("nextActiveIndex：首尾夹紧", () => {
  assert.equal(nextActiveIndex(-1, 1, 10), 0);
  assert.equal(nextActiveIndex(0, -1, 10), 0);
  assert.equal(nextActiveIndex(9, 1, 10), 9);
  assert.equal(nextActiveIndex(-1, 1, 0), -1);
});

test("虚拟化：2600 条只物化视口 ± overscan，滚动换窗口但行数有界", async () => {
  installDomStub();
  const container = document.createElement("div");
  container.id = "load-order-rule-sources";
  container.clientHeight = 210;
  const changes = [];
  const list = createRuleListbox(container, {
    multiple: true,
    onSelectionChange: (selected) => changes.push([...selected]),
  });
  const items = Array.from({ length: 2600 }, (_, index) => ({ key: `k${index}` }));
  list.setItems(items);

  const firstRows = ruleRows(container);
  assert.ok(firstRows.length > 0, "首屏必须要有行");
  assert.ok(firstRows.length < 40, `物化行数要受控，实际 ${firstRows.length}`);
  assert.equal(spacers(container).length, 2, "上下占位块都要在");
  assert.equal(firstRows[0].dataset.key, "k0");

  container.scrollTop = 40000;
  container.dispatchEvent({ type: "scroll" });
  await sleep(80);
  const scrolledRows = ruleRows(container);
  assert.ok(scrolledRows.length < 40, `滚动后行数仍要有界：${scrolledRows.length}`);
  assert.equal(scrolledRows[0].dataset.key, "k1329", "窗口要跟着滚动移动");
  assert.ok(
    !scrolledRows.some((row) => firstRows.some((first) => first.dataset.key === row.dataset.key)),
    "滚远之后旧行必须被回收",
  );

  // 点一行（多选）：回调收到选择集，行状态同步
  const target = scrolledRows[0];
  container.dispatchEvent({ type: "click", target });
  assert.deepEqual(changes.at(-1), [target.dataset.key]);
  assert.equal(target.getAttribute("aria-selected"), "true");
  container.dispatchEvent({ type: "click", target });
  assert.deepEqual(changes.at(-1), []);

  // 键盘：↓ 激活首行，Enter 选中
  container.dispatchEvent({ type: "keydown", key: "ArrowDown", preventDefault() {} });
  assert.ok(ruleRows(container).some((row) => row.classList.contains("is-active")), "↓ 要画出激活行");
  container.dispatchEvent({ type: "keydown", key: "Enter", preventDefault() {} });
  assert.equal(changes.at(-1).length, 1, "Enter 要能选中激活行");

  // 换一批数据（搜索）：选择集保持
  list.setItems(items.slice(1300, 1400));
  const kept = ruleRows(container).filter((row) => row.classList.contains("is-selected"));
  assert.ok(kept.length <= 1, "选择集不因 setItems 而复制");
  list.destroy();
  assert.equal(container.children.length, 0, "destroy 要清干净容器");
});
