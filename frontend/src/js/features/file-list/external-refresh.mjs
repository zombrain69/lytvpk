// 外部改动（别的程序往 Mod 目录里增删文件）→ 静默刷新的调度器。
//
// 两条规则：
//   1. 刷新进行中又来了新变化：合并成"这一轮结束后再补一次"，不会并发重画列表。
//   2. 刷新失败不抛出：这是后台行为，调用点（Wails 事件回调）是 fire-and-forget，
//      抛出去只会变成控制台里的未处理 Promise。
//
// 刻意**不**在窗口最小化时延后：WebView2 真机实测最小化后 document.hidden 仍是
// false（只有 WindowIsMinimised() 为 true），而延后会让用户回到窗口时先看到旧列表；
// 一次暖扫描只有几十毫秒，直接把列表刷新到位更符合使用体验。
export function createExternalRefreshScheduler({ runRefresh, onError } = {}) {
  let pending = false;
  let running = false;

  const reportError = (error) => {
    if (typeof onError === "function") {
      onError(error);
      return;
    }
    console.warn("自动刷新失败:", error);
  };

  const schedule = async () => {
    if (!pending || running) return "deferred";
    pending = false;
    running = true;
    try {
      await runRefresh();
    } catch (error) {
      reportError(error);
    } finally {
      running = false;
    }
    // 刷新期间又来了变化（后端已做防抖，这里不会自激）：再补一轮。
    if (pending) return schedule();
    return "refreshed";
  };

  return {
    // 后端事件：Mod 目录有外部改动
    notify() {
      pending = true;
      return schedule();
    },
    get pending() {
      return pending;
    },
    get running() {
      return running;
    },
  };
}
