// 轮询后端"文件操作忙碌"状态，并把它画到界面上。
//
// 为什么用轮询而不是在每个调用点包一层：忙碌状态可能来自任何面板
// （列表页移动、归档页移动、工具箱打包、完整性修复…）。轮询一次 `IsFileOperationBusy()`
// 是加锁检查，开销可以忽略；这样新增入口时不用记得"别忘了接忙碌状态"。

import {
  FILE_OPERATION_BUSY_CLASS,
  FILE_OPERATION_POLL_INTERVAL_MS,
  describeFileOperationBusy,
} from "./file-operation-busy.mjs";

let watcherStarted = false;

/**
 * startFileOperationWatcher 启动轮询；返回停止函数（测试/退出时用）。
 * deps.isBusy 注入后端查询，deps.intervalMs 可覆盖间隔。
 */
export function startFileOperationWatcher(deps = {}) {
  const isBusy = deps.isBusy;
  if (typeof isBusy !== "function") return () => {};
  if (watcherStarted) return () => {};
  watcherStarted = true;

  const interval = Number(deps.intervalMs) > 0 ? Number(deps.intervalMs) : FILE_OPERATION_POLL_INTERVAL_MS;
  let lastBusy = null;
  let timer = null;

  const apply = (busy) => {
    if (busy === lastBusy) return;
    lastBusy = busy;
    if (typeof document === "undefined") return;
    document.body?.classList.toggle(FILE_OPERATION_BUSY_CLASS, Boolean(busy));
    const badge = document.getElementById("file-operation-busy");
    if (badge) {
      const text = describeFileOperationBusy(Boolean(busy));
      badge.textContent = text;
      badge.hidden = !busy;
    }
  };

  const probe = async () => {
    // 刻意不判断 document.hidden：窗口被遮挡 / 最小化时 WebView2 也会把页面标成 hidden，
    // 那样"后台正在打包"就永远不显示提示（真机验收时踩到过）。
    // 这个查询只是一次加锁检查，每秒一次的开销可以忽略，换来的是状态始终准确。
    try {
      apply(Boolean(await isBusy()));
    } catch (error) {
      // 轮询失败（例如窗口正在关闭）保留上一次状态，不要突然解除灰化。
      console.warn("查询文件操作忙碌状态失败:", error);
    }
  };

  // 主通道：后端在进入/退出文件操作时推事件（毫秒级任务也不会被错过）。
  let unsubscribe = null;
  if (typeof deps.subscribe === "function") {
    try {
      unsubscribe = deps.subscribe((busy) => apply(Boolean(busy)));
    } catch (error) {
      console.warn("订阅文件操作状态失败，改用轮询:", error);
    }
  }

  // 兜底通道：低频轮询，覆盖"界面启动比事件晚"之类的边界。
  timer = setInterval(() => void probe(), interval);
  void probe();

  return () => {
    if (timer) clearInterval(timer);
    timer = null;
    if (typeof unsubscribe === "function") unsubscribe();
    unsubscribe = null;
    watcherStarted = false;
    apply(false);
  };
}
