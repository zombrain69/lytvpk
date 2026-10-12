// i18n 启动接线：把全部随包文案目录装进运行时。
//
// 为什么独立成文件：见 `i18n-runtime.js` 顶部注释 —— runtime 保持 Node 可导入
// （纯模块单测要用），JSON 的导入就留在这里；本文件只在浏览器入口（main.js）里
// 最先被导入，保证任何页面渲染前目录已经装好。
//
// 多目录（路线图第 19 项 P0）：一次装 zh-CN + en；en 采用「缺翻回退中文」策略——
// 目录里只放已翻译的 key，其余由运行时回退链兜底（全量机翻产出在 P2）。
import zhCN from "../../messages/zh-CN.json";
import en from "../../messages/en.json";
import { installCatalogs } from "./i18n-runtime.js";

installCatalogs({ "zh-CN": zhCN, en }, "zh-CN");
