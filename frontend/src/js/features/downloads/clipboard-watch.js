// 剪贴板工坊链接自动识别（对齐 FireAxe v0.4.0 / MainWindowViewModel.CheckClipboard）。
//
// 上游每 0.5s 读一次剪贴板；本项目把"什么时候读"交给纯函数判定：
// 只有"设置里开着 + 窗口在前台 + 距上次超过间隔 + 当前没有别的弹窗"才真的去读。
// 判定与去重都在 core/workshop-clipboard.mjs（有单测），这里只做接线。

import { getConfig } from "../../core/config.js";
import {
  CLIPBOARD_POLL_INTERVAL_MS,
  shouldOfferClipboardLink,
  shouldPollClipboard,
} from "../../core/workshop-clipboard.mjs";
import { CheckClipboardWorkshopLink } from "../../../../wailsjs/go/app/App";
import { showConfirmModal } from "../modals/confirm.js";
import { openWorkshopModalWithUrl } from "./workshop-modal.js";

let clipboardWatchTimer = null;
let lastPolledAt = 0;
let lastOfferedLink = "";
let pollInFlight = false;

/** hasVisibleLayer 当前是否有别的弹窗/浮动窗口占着界面（有就先不打扰）。 */
function hasVisibleLayer() {
  return Boolean(
    document.querySelector(".modal:not(.hidden), .floating-modal:not(.hidden)"),
  );
}

async function pollClipboardOnce() {
  if (pollInFlight) return;
  const enabled = getConfig().autoDetectWorkshopLink !== false;
  const focused = document.hasFocus();
  const now = Date.now();
  if (!shouldPollClipboard({ enabled, focused, now, lastPolledAt })) return;
  if (hasVisibleLayer()) return;
  pollInFlight = true;
  lastPolledAt = now;
  try {
    const link = await CheckClipboardWorkshopLink();
    const input = document.getElementById("workshop-url");
    if (!shouldOfferClipboardLink(link, lastOfferedLink, input?.value)) return;
    lastOfferedLink = link;
    showConfirmModal(
      "检测到创意工坊链接",
      `剪贴板里是一条创意工坊链接：\n${link}\n\n要打开下载页并解析这个作品吗？`,
      async () => {
        await openWorkshopModalWithUrl(link);
      },
    );
  } catch (error) {
    // 剪贴板被其它程序占用等情况很常见，静默跳过这一拍即可，不打扰用户。
    console.debug("读取剪贴板失败:", error);
  } finally {
    pollInFlight = false;
  }
}

/** startWorkshopClipboardWatch 启动轮询（重复调用是安全的，会先停掉旧的）。 */
export function startWorkshopClipboardWatch(intervalMs = CLIPBOARD_POLL_INTERVAL_MS) {
  stopWorkshopClipboardWatch();
  clipboardWatchTimer = setInterval(() => {
    void pollClipboardOnce();
  }, intervalMs);
}

export function stopWorkshopClipboardWatch() {
  if (clipboardWatchTimer != null) {
    clearInterval(clipboardWatchTimer);
    clipboardWatchTimer = null;
  }
}

/** resetWorkshopClipboardWatch 忘掉"已经提示过哪条链接"（测试与重置用）。 */
export function resetWorkshopClipboardWatch() {
  lastPolledAt = 0;
  lastOfferedLink = "";
  pollInFlight = false;
}
