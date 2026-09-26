// 快捷键总览的接线测试：表里写的键位必须真的被绑定，界面入口也要真的存在。
// 这一层专门防"说明书说了但没实现"（对齐 FireAxe 里 ObjectExplanation 的"说的和做的一致"）。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { SHORTCUT_GROUPS } from "./shortcuts.mjs";

const html = readFileSync(new URL("../../../index.html", import.meta.url), "utf8");
const runtime = readFileSync(new URL("../features/app-runtime.js", import.meta.url), "utf8");
const uiScale = readFileSync(new URL("./ui-scale.js", import.meta.url), "utf8");
const floating = readFileSync(new URL("./floating-modal.js", import.meta.url), "utf8");
const css = readFileSync(new URL("../../css/app/reading-comfort.css", import.meta.url), "utf8");

test("标题栏有 `?` 按钮与浮层容器，并且内容由共享表生成", () => {
  assert.match(html, /id="shortcuts-help-btn"/, "标题栏缺少快捷键入口");
  assert.match(html, /id="shortcuts-help-popover"/, "缺少浮层容器");
  assert.match(html, /aria-controls="shortcuts-help-popover"/, "按钮要声明它控制哪个浮层");
  assert.match(runtime, /shortcutsPopover\.innerHTML = buildShortcutsHtml\(\)/, "浮层内容应由共享表生成");
  assert.match(runtime, /shortcutsBtn\.title = buildShortcutsTitle\(\)/, "按钮提示应同源");
  assert.match(css, /\.header-help-popover/, "缺少标题栏浮层样式（原来 left:0 会跑到屏幕外）");
});

test("总览里写的每个键位在代码里都有真实绑定", () => {
  const keys = SHORTCUT_GROUPS.flatMap((group) => group.rows.map((row) => row.keys));

  // Ctrl+K / Ctrl+F：app-runtime 的全局 keydown
  assert.ok(keys.includes("Ctrl + K") && /event\.key\.toLowerCase\(\) === "k"/.test(runtime), "Ctrl+K 要真的绑定");
  assert.ok(keys.includes("Ctrl + F") && /event\.key\.toLowerCase\(\) === "f"/.test(runtime), "Ctrl+F 要真的绑定");
  // Ctrl + = / - / 0：ui-scale
  assert.ok(keys.includes("Ctrl + =") && /key === "\+" \|\| key === "="/.test(uiScale), "Ctrl+= 要真的绑定");
  assert.ok(keys.includes("Ctrl + -") && /key === "-"/.test(uiScale), "Ctrl+- 要真的绑定");
  assert.ok(keys.includes("Ctrl + 0") && /key === "0"/.test(uiScale), "Ctrl+0 要真的绑定");
  // ?：app-runtime 里只在光标不在输入框时触发
  assert.ok(keys.includes("?") && /event\.key !== "\?"/.test(runtime), "? 要真的绑定");
  assert.match(runtime, /target\.tagName === "INPUT" \|\| target\.tagName === "TEXTAREA"/, "输入框里不该抢 ? 键");
  // 搜索框里的三个键
  assert.ok(keys.includes("↑ / ↓") && /event\.key === "ArrowDown" \|\| event\.key === "ArrowUp"/.test(runtime));
  assert.ok(keys.includes("Enter") && /event\.key === "Enter"/.test(runtime));
  assert.ok(keys.includes("Esc") && /event\.key === "Escape"/.test(runtime));
  // 双击标题栏复位窗口几何：floating-modal
  assert.ok(keys.includes("双击标题栏") && /dblclick/.test(floating), "双击标题栏要真的绑定");
  // F2 重命名 / Delete 删除（对齐 FireAxe v0.7.0/v0.7.2 的编辑快捷键）
  assert.ok(keys.includes("F2") && /event\.key === "F2"/.test(runtime), "F2 要真的绑定到重命名");
  assert.match(runtime, /F2[\s\S]{0,120}renameFile\(target\)/, "F2 要调用重命名");
  assert.ok(keys.includes("Delete") && /event\.key === "F2" \|\| event\.key === "Delete"/.test(runtime), "Delete 要真的绑定");
  assert.match(runtime, /deleteSelected\(\)/, "多选时 Delete 要走批量删除");
  assert.match(runtime, /resolveShortcutTargetPath\(\)/, "F2/Delete 的目标要先解析（光标行 → 唯一选中）");
  // Ctrl+X 剪切 / Ctrl+V 移动（对齐 FireAxe v0.5.1 的 Ctrl+X / Ctrl+V）
  assert.ok(keys.includes("Ctrl + X") && /key\.toLowerCase\(\) === "x"/.test(runtime), "Ctrl+X 要真的绑定");
  assert.ok(keys.includes("Ctrl + V") && /key\.toLowerCase\(\) === "v"/.test(runtime), "Ctrl+V 要真的绑定");
  assert.match(runtime, /cutSelected\(\)/, "Ctrl+X 要调用剪切标记");
  assert.match(runtime, /pasteMoveClipboard\(\)/, "Ctrl+V 要调用移动到目标目录");
});

test("剪贴板工坊链接识别有真实的轮询接线（对齐 FireAxe 的自动识别）", () => {
  const watcher = readFileSync(
    new URL("../features/downloads/clipboard-watch.js", import.meta.url),
    "utf8",
  );
  assert.match(watcher, /CheckClipboardWorkshopLink/, "要调用后端读取剪贴板");
  assert.match(watcher, /shouldPollClipboard/, "轮询节奏要走共享判定（开关 + 聚焦 + 间隔）");
  assert.match(runtime, /startWorkshopClipboardWatch/, "应用启动时要启动轮询");
  assert.match(watcher, /openWorkshopModalWithUrl\(link\)/, "确认后要能带着链接打开工坊页");
});

test("命令面板里有「查看键盘快捷键」，且复用标题栏按钮", () => {
  const commandSource = readFileSync(new URL("./command-palette.mjs", import.meta.url), "utf8");
  assert.match(commandSource, /id: "show-shortcuts"/, "命令表缺少快捷键总览入口");
  assert.match(runtime, /"show-shortcuts": \(\) => document\.getElementById\("shortcuts-help-btn"\)\?\.click\(\)/, "动作要复用已有按钮");
});
