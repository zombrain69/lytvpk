// 剪贴板工坊链接识别的纯逻辑（对齐 FireAxe v0.4.0 的"自动识别剪贴板里的工坊链接"）。
//
// 拆成纯函数的原因：
// ① 判定规则（连接/间隔/去重）可以脱离 DOM 与 Wails 单测；
// ② 与 Go 侧 internal/app/workshop_clipboard.go 的解析保持同一套语义（同一批用例两侧都跑）。

/** 轮询间隔：上游是 0.5s，这里放宽到 1.5s —— 只是"复制链接后切回应用"的提示，不需要那么急。 */
export const CLIPBOARD_POLL_INTERVAL_MS = 1500;

const ITEM_LINK_BASE = "https://steamcommunity.com/sharedfiles/filedetails/?id=";
const LINK_PATTERN =
  /steamcommunity\.com\/(?:sharedfiles|workshop)\/filedetails\/?\?[^\s]*?\bid=(\d+)/i;
const ID_PATTERN = /^\d+$/;

/**
 * parseWorkshopLink 从任意文本里提取工坊物品 ID，返回规范链接；认不出返回空字符串。
 * 与 Go 侧 parseWorkshopClipboardLink 行为一致：认链接（sharedfiles / workshop），也认纯数字 ID。
 */
export function parseWorkshopLink(text) {
  const trimmed = String(text ?? "").trim();
  if (trimmed === "") return "";
  if (ID_PATTERN.test(trimmed)) return ITEM_LINK_BASE + trimmed;
  const match = trimmed.match(LINK_PATTERN);
  if (!match) return "";
  return ITEM_LINK_BASE + match[1];
}

/**
 * shouldPollClipboard 决定这一拍要不要真的去读剪贴板。
 * 只有"开关打开 + 窗口在前台 + 距上次超过间隔"才读，避免后台空转与打扰。
 */
export function shouldPollClipboard({
  enabled,
  focused,
  now,
  lastPolledAt,
  intervalMs = CLIPBOARD_POLL_INTERVAL_MS,
} = {}) {
  if (!enabled || !focused) return false;
  const current = Number(now);
  const last = Number(lastPolledAt) || 0;
  if (!Number.isFinite(current)) return false;
  return current - last >= intervalMs;
}

/**
 * shouldOfferClipboardLink 决定要不要弹"检测到工坊链接"的确认框。
 * 去重规则：同一条链接只提示一次；输入框里已经是这条链接时也不再提示（用户已手动粘贴）。
 */
export function shouldOfferClipboardLink(link, lastOffered, currentInputValue = "") {
  const normalized = String(link ?? "").trim();
  if (normalized === "") return false;
  if (normalized === String(lastOffered ?? "").trim()) return false;
  if (normalized === String(currentInputValue ?? "").trim()) return false;
  return true;
}
