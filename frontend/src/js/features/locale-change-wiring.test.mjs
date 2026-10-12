// 语言切换动态重绘（路线图第 19 项 P1）的接线契约：
//   - 运行时在语言真正变化时派发 app:locale-change（行为测试见 core/i18n-runtime.test.mjs）；
//   - 各「没有重进自愈路径」的面必须监听该事件并调用对应重绘入口：mods 页
//     （filters.js 列表/标签条/排序按钮 + directory-dropdown.js 目录下拉）、
//     工坊页（app-init.js 侧边分类 + 缓存卡片）、设置页（app-runtime.js 整页重画 + 分区保持）。
//   真机最终效果走 CUA 沙箱核对（manual-verification.md「语言切换动态重绘」）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (relative) => readFileSync(new URL(relative, import.meta.url), "utf8");

const filters = read("./file-list/filters.js");
const dropdown = read("./directory-dropdown.js");
const appInit = read("./app-init.js");
const appRuntime = read("./app-runtime.js");
const workshopList = read("./workshop/list.js");
const workshopBrowser = read("./workshop/workshop-browser.js");
const settingsPage = read("./settings/settings-page.js");
const runtime = read("../core/i18n-runtime.js");

/** 从监听注册点起截一段窗口，断言「监听存在 + 紧跟着调用某个重绘入口」。 */
function listenerWindow(source, size = 600) {
  const start = source.indexOf('addEventListener("app:locale-change"');
  assert.ok(start >= 0, "没有找到 app:locale-change 监听");
  return source.slice(start, start + size);
}

test("运行时在语言变化时派发事件；启动应用持久化语言显式静默", () => {
  assert.match(runtime, /CustomEvent\("app:locale-change"/, "要派发 app:locale-change");
  assert.match(runtime, /next !== previous/, "只在语言真正变化时派发");
  assert.match(
    runtime,
    /if \(options\.notify !== false && next !== previous\)/,
    "notify:false 要能关掉广播（启动路径）"
  );
  assert.match(
    appRuntime,
    /setLocale\(normalizeLocale\(getConfig\(\)\.locale\), \{ notify: false \}\)/,
    "启动应用持久化语言必须静默（首帧前页面按当前语言首渲染）"
  );
});

test("mods 页：filters.js 与 directory-dropdown.js 分别接上重绘入口", () => {
  const modsWindow = listenerWindow(filters);
  assert.match(modsWindow, /renderTagFilters\(\)/, "标签条要重画");
  assert.match(modsWindow, /performSearch\(\)/, "列表/命中计数要重画");
  assert.match(modsWindow, /updateSortButtonUI\(\)/, "排序按钮文案要重画");

  const dropdownWindow = listenerWindow(dropdown);
  assert.match(dropdownWindow, /renderDirectoryList\(\)/, "目录下拉要重画");
});

test("工坊页：侧边分类 + 缓存卡片重画；缓存重画不触发网络请求", () => {
  assert.match(
    appInit,
    /import \{[^}]*rerenderWorkshopBrowserFromCache[^}]*\} from "\.\/workshop\/workshop-browser\.js";/,
    "app-init 要从聚合出口导入缓存重画"
  );
  const windowed = listenerWindow(appInit);
  assert.match(windowed, /renderWorkshopSidebar\(\)/, "侧边分类要重画");
  assert.match(windowed, /rerenderWorkshopBrowserFromCache\(\)/, "缓存卡片要重画");

  assert.match(workshopBrowser, /rerenderWorkshopBrowserFromCache/, "聚合出口要转出缓存重画");
  assert.match(workshopList, /export function rerenderWorkshopBrowserFromCache\(\)/);
  const fnStart = workshopList.indexOf("export function rerenderWorkshopBrowserFromCache()");
  const body = workshopList.slice(fnStart, fnStart + 800);
  assert.ok(!body.includes("loadWorkshopList("), "缓存重画不允许触发网络加载");
  assert.ok(!body.includes("FetchWorkshopList"), "缓存重画不允许触发网络加载");
});

test("设置页：locale-change 时整页重画并保持当前分区；写配置先于广播", () => {
  const windowed = listenerWindow(appRuntime);
  assert.match(windowed, /renderSettingsPageWithDeps\(\)/, "设置页要整页重画");
  assert.match(windowed, /settings-nav-item\.active/, "重画前要记住当前分区");
  assert.match(windowed, /dataset\.panel/, "重画后要恢复到同一分区");

  const apply = settingsPage.slice(settingsPage.indexOf("applyAndSaveLanguage = async"));
  const localeWrite = apply.indexOf("config.locale = next;");
  const setLocaleCall = apply.indexOf("setLocale(next);");
  assert.ok(localeWrite >= 0, "applyAndSaveLanguage 要写 config.locale");
  assert.ok(setLocaleCall >= 0, "applyAndSaveLanguage 要调用 setLocale");
  assert.ok(
    localeWrite < setLocaleCall,
    "先写 config.locale 再 setLocale：否则 locale-change 重绘出旧选中态"
  );
});
