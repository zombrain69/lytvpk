// 仅供 `node --test` 使用：把真实中文目录装进 i18n 运行时，供纯模块单测
// 断言真实中文文案（缺 key 时断言会直接失败，等于给纯模块也加了一道对照）。
// 浏览器端不经过这里 —— 应用启动走 `core/i18n-setup.js`。
import { readFileSync } from "node:fs";
import { installCatalog } from "./i18n-runtime.js";

let installed = false;

/** installTestCatalog 幂等安装 messages/zh-CN.json，返回是否安装成功。 */
export function installTestCatalog() {
  if (installed) return true;
  const catalogPath = new URL("../../messages/zh-CN.json", import.meta.url);
  installCatalog(JSON.parse(readFileSync(catalogPath, "utf8")), "zh-CN");
  installed = true;
  return true;
}
