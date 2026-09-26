// 「文件操作忙碌」的文案与状态名（纯函数，node --test 覆盖）。
//
// 背景：后端（file_op_gate.go，对齐 FireAxe BlockMove）同一时间只允许一个移动/删除/打包，
// 后来者会拿到错误。界面如果什么都不显示，用户只会看到"点了没反应/突然报错"。
// 这里把"忙碌"变成可见状态：状态栏一行提示 + 相关按钮暂时变灰。

/** 忙碌时加到 <body> 上的 class（CSS 用它禁掉会写磁盘的按钮）。 */
export const FILE_OPERATION_BUSY_CLASS = "is-file-operation-busy";

/** 忙碌时状态栏显示的文案。 */
export function describeFileOperationBusy(busy) {
  return busy ? "正在处理文件（移动 / 删除 / 打包）… 完成后可继续" : "";
}

/** 忙碌时用户仍点了文件操作按钮，给出与后端一致的说明。 */
export function formatFileOperationBlockedMessage() {
  return "另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试";
}

/**
 * shouldIgnoreBusyProbeError 判断一次轮询失败要不要清掉忙碌状态。
 * 轮询用的是一次很轻的调用；如果它本身报错（例如窗口正在关闭），
 * 保留上一次状态比"突然全部解除"更安全 —— 这里统一返回"保留"。
 */
export function shouldKeepBusyStateAfterProbeError() {
  return true;
}

/**
 * 兜底轮询间隔：主通道是后端事件（`file_operation_state`），
 * 轮询只用来覆盖"界面启动比事件更晚"之类的边界，所以可以放慢。
 */
export const FILE_OPERATION_POLL_INTERVAL_MS = 3000;
