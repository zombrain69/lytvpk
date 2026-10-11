// i18n 启动接线：把中文文案目录装进运行时。
//
// 为什么独立成文件：见 `i18n-runtime.js` 顶部注释 —— runtime 保持 Node 可导入
// （纯模块单测要用），JSON 的导入就留在这里；本文件只在浏览器入口（main.js）里
// 最先被导入，保证任何页面渲染前目录已经装好。
import zhCN from "../../messages/zh-CN.json";
import { installCatalog } from "./i18n-runtime.js";

installCatalog(zhCN, "zh-CN");
