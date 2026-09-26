// 剪贴板工坊链接识别（对齐 FireAxe 的"自动识别剪贴板里的工坊链接"）的纯逻辑测试。
// 运行时由 features/downloads/clipboard-watch.js 做定时轮询，这里只测判定规则。

import assert from "node:assert/strict";
import test from "node:test";

import {
  CLIPBOARD_POLL_INTERVAL_MS,
  parseWorkshopLink,
  shouldOfferClipboardLink,
  shouldPollClipboard,
} from "./workshop-clipboard.mjs";

const CANONICAL = "https://steamcommunity.com/sharedfiles/filedetails/?id=";

test("parseWorkshopLink 认得链接与纯 ID，认不出普通文本", () => {
  assert.equal(parseWorkshopLink(`${CANONICAL}2302720558`), `${CANONICAL}2302720558`);
  assert.equal(
    parseWorkshopLink("看看这个 https://steamcommunity.com/workshop/filedetails/?l=schinese&id=42 很好"),
    `${CANONICAL}42`,
  );
  assert.equal(parseWorkshopLink("  2302720558  "), `${CANONICAL}2302720558`);
  assert.equal(parseWorkshopLink("https://example.com/?id=5"), "");
  assert.equal(parseWorkshopLink("2302720558abc"), "");
  assert.equal(parseWorkshopLink(""), "");
  assert.equal(parseWorkshopLink(null), "");
});

test("shouldPollClipboard 只在开关打开、窗口聚焦、且到了间隔时才轮询", () => {
  const base = { enabled: true, focused: true, now: 10_000, lastPolledAt: 1_000 };
  assert.equal(shouldPollClipboard(base), true, "开关打开 + 聚焦 + 超过间隔 → 轮询");
  assert.equal(shouldPollClipboard({ ...base, enabled: false }), false, "开关关闭不轮询");
  assert.equal(shouldPollClipboard({ ...base, focused: false }), false, "窗口不在前台不轮询（不打扰用户）");
  assert.equal(
    shouldPollClipboard({ ...base, now: base.lastPolledAt + CLIPBOARD_POLL_INTERVAL_MS - 1 }),
    false,
    "未到间隔不轮询",
  );
  assert.equal(
    shouldPollClipboard({ enabled: true, focused: true, now: 10_000, lastPolledAt: 0 }),
    true,
    "首次（lastPolledAt=0）允许立即轮询",
  );
});

test("shouldOfferClipboardLink 去重：同一条链接不重复弹窗，空链接不弹", () => {
  assert.equal(shouldOfferClipboardLink(`${CANONICAL}1`, ""), true, "新链接要提示");
  assert.equal(shouldOfferClipboardLink(`${CANONICAL}1`, `${CANONICAL}1`), false, "同一条链接不重复提示");
  assert.equal(shouldOfferClipboardLink("", ""), false, "空链接不提示");
  assert.equal(shouldOfferClipboardLink(`${CANONICAL}2`, `${CANONICAL}1`), true, "换了一条要提示");
});

test("shouldOfferClipboardLink 跳过已经在输入框里的链接（用户已手动粘贴）", () => {
  assert.equal(
    shouldOfferClipboardLink(`${CANONICAL}1`, "", `${CANONICAL}1`),
    false,
    "输入框里已经是这条链接时不再打扰",
  );
});
