export function createAutoexecSettingsController(initialInfo, operations = {}) {
  let info = initialInfo || null;
  let reloadRequestId = 0;

  const getInfo = () => info;

  const reload = async () => {
    if (typeof operations.getConfig !== "function") {
      throw new Error("当前后端未提供 GetAutoexecConfig 接口");
    }
    const requestId = ++reloadRequestId;
    const next = await operations.getConfig();
    if (requestId !== reloadRequestId) return { info, stale: true };
    if (next && typeof next === "object") info = next;
    return { info, stale: false };
  };

  const save = async (content) => {
    if (typeof operations.saveConfig !== "function") {
      throw new Error("当前后端未提供 SaveAutoexecConfig 接口");
    }
    await operations.saveConfig(content);
    try {
      const refreshed = await reload();
      return { saved: true, info: refreshed.info, stale: refreshed.stale, reloadError: null };
    } catch (reloadError) {
      return { saved: true, info, stale: false, reloadError };
    }
  };

  return { getInfo, reload, save };
}
