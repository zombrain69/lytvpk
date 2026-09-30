import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

import {
  ESC_CLOSABLE_MODAL_IDS,
  ESC_CLOSE_SELECTORS,
  pickEscClosableModalId,
} from "./modal-escape.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));
const html = readFileSync(new URL("../../../index.html", import.meta.url), "utf8");
const runtime = readFileSync(path.resolve(here, "../features/app-runtime.js"), "utf8");

test("注册表里的窗口都真实存在，且顺序里没有重复", () => {
  const seen = new Set();
  for (const id of ESC_CLOSABLE_MODAL_IDS) {
    assert.ok(!seen.has(id), `注册表里重复出现 ${id}`);
    seen.add(id);
    assert.match(html, new RegExp(`id="${id}"`), `index.html 里没有 ${id}`);
  }
});

test("Esc 只挑注册表里的窗口，且最上层优先", () => {
  assert.equal(pickEscClosableModalId([]), "");
  assert.equal(pickEscClosableModalId(null), "");
  assert.equal(pickEscClosableModalId(["loading-screen", "image-preview-modal"]), "", "不该关的窗口不碰");
  assert.equal(pickEscClosableModalId(["load-order-modal"]), "load-order-modal");

  // 确认框叠在加载顺序之上时，Esc 应该关确认框
  assert.equal(
    pickEscClosableModalId(["load-order-modal", "confirm-modal"]),
    "confirm-modal",
    "确认框在上层时要先关它",
  );
  // 详情从冲突检测里打开：详情在上层
  assert.equal(
    pickEscClosableModalId(["conflict-modal", "file-detail-modal"]),
    "file-detail-modal",
  );
});

test("关闭按钮的查找顺序不含确认/删除类按钮", () => {
  const joined = ESC_CLOSE_SELECTORS.join(" ");
  assert.match(joined, /\.close-btn/, "要优先点关闭（×）");
  assert.match(joined, /\.btn-secondary/, "其次点页脚里的次要按钮（取消 / 关闭）");
  assert.ok(!/btn-primary/.test(joined), "Esc 绝不能替用户点主按钮");
  assert.ok(!/btn-danger/.test(joined), "Esc 绝不能替用户点删除类按钮");
});

test("app-runtime 的 Esc 处理接了这张注册表（不再直接 return）", () => {
  assert.match(runtime, /pickEscClosableModalId\(/, "Esc 处理要调用 pickEscClosableModalId");
  assert.match(runtime, /ESC_CLOSE_SELECTORS\.join\(", "\)/, "要按统一选择器查找关闭按钮");
  assert.match(
    runtime,
    /const visibleModal = document\.querySelector\("\.modal:not\(\.hidden\)"\);\s*\n\s*if \(visibleModal\) \{/,
    "有模态框时不能直接 return，要尝试关闭",
  );
});
