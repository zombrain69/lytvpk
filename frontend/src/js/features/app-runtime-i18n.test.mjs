// 第 11 项（app-runtime 协议/事件提示 + shell 导航文案）的接线契约：
//   - 运行时的提示消息走目录 key：i18n-catalog.test.mjs 兜"key 是否存在"，
//     这里兜"渲染出来还是不是原来那句话"—— 参数名写错（{count} vs {cleared}）只有
//     实际插值才看得见；
//   - shell 导航按钮由 ui-shell.js 运行时重建、theme.js 维护动态主题标签（开灯/关灯），
//     两者都必须带 data-i18n*，语言切换靠 applyTranslations() 全量重放才能更新。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { installTestCatalog } from "../core/i18n-test-catalog.mjs";
import { t } from "../core/i18n-runtime.js";

const runtime = readFileSync(new URL("./app-runtime.js", import.meta.url), "utf8");
const shell = readFileSync(new URL("../core/ui-shell.js", import.meta.url), "utf8");
const theme = readFileSync(new URL("../core/theme.js", import.meta.url), "utf8");

const NAV_LABELS = {
  mods: "MOD 管理",
  workshop: "创意工坊",
  downloads: "下载与解析",
  servers: "收藏服务器",
  diagnostics: "工具箱",
  settings: "设置",
  docs: "使用说明",
  about: "关于",
};

test("shell 导航标签全部走目录 key，目录值与既有文案一致", () => {
  installTestCatalog();
  for (const [page, label] of Object.entries(NAV_LABELS)) {
    assert.match(
      shell,
      new RegExp(`labelKey: "app\\.shell\\.nav\\.${page}"`),
      `${page} 要接目录 key`,
    );
    assert.equal(t(`app.shell.nav.${page}`), label, `${page} 的目录值`);
  }
  assert.equal(t("app.shell.navAriaLabel"), "主菜单");
  assert.equal(t("app.shell.launch.label"), "启动L4D2");
  assert.equal(t("app.shell.launch.recentServers"), "最近服务器");
  assert.equal(t("app.shell.theme.ariaLabel"), "切换主题");
  assert.equal(t("app.shell.theme.toLight"), "开灯模式");
  assert.equal(t("app.shell.theme.toDark"), "关灯模式");
});

test("重建出的 shell 节点带 data-i18n*，语言切换可重放", () => {
  assert.match(shell, /setAttribute\("data-i18n", item\.labelKey\)/, "导航标签 span 要带 data-i18n");
  assert.match(
    shell,
    /setAttribute\("data-i18n-title", item\.labelKey\)/,
    "导航按钮 title 要带 data-i18n-title",
  );
  assert.match(shell, /data-i18n-aria-label="app\.shell\.navAriaLabel"/, "侧栏主菜单 aria 要可重放");
  assert.match(
    shell,
    /setAttribute\("data-i18n-aria-label", "app\.shell\.launch\.label"\)/,
    "启动按钮 aria 要可重放",
  );
  assert.match(
    shell,
    /setAttribute\("data-i18n-aria-label", "app\.shell\.theme\.ariaLabel"\)/,
    "主题按钮 aria 要可重放",
  );
  // 主题标签是动态的（随深色/浅色切换）：ui-shell 建初始值、theme.js 在切换时更新，
  // 两处都要把「当前状态 → key」写对，否则语言切换后的重放会填错文案。
  for (const file of [shell, theme]) {
    assert.match(file, /"data-i18n", "app\.shell\.theme\.toLight"/, "深色状态要用 toLight key");
    assert.match(file, /"data-i18n", "app\.shell\.theme\.toDark"/, "浅色状态要用 toDark key");
  }
});

test("app-runtime 的协议/事件提示在目录中可渲染成原句", () => {
  installTestCatalog();
  const cases = [
    ["app.error.initFailed", { error: "boom" }, "应用初始化失败: boom"],
    ["app.crash.unknownError", undefined, "未知前端错误"],
    ["app.crash.unhandledRejection", undefined, "未处理的 Promise 拒绝"],
    [
      "app.windowGeometry.resetDone",
      { count: 3 },
      "已重置 3 个窗口的位置与大小；重新打开窗口会回到默认位置",
    ],
    ["app.windowGeometry.resetNoop", undefined, "窗口位置本来就是默认值"],
    ["app.shortcuts.renameHint", undefined, "先用 ↑↓ 把光标移到要重命名的 Mod 上（或只选中一个）"],
    ["app.shortcuts.deleteHint", undefined, "先用 ↑↓ 把光标移到要删除的 Mod 上（或勾选要删除的那些）"],
    ["app.shell.commandPalette.notImplemented", undefined, "这条命令暂时没有实现"],
    ["mods.batch.selectFilesFirst", undefined, "请先选择文件"],
    ["downloads.modal.notification.pasted", undefined, "已粘贴"],
    ["downloads.modal.error.pasteFailed", undefined, "粘贴失败，请使用 Ctrl+V"],
    [
      "settings.interface.openWith.statusSaved",
      { program: "MyViewer.exe" },
      '已保存：以后"打开所在位置"会用 MyViewer.exe',
    ],
    ["settings.interface.openWith.statusDefault", undefined, "当前使用系统默认（资源管理器定位文件）"],
    ["settings.interface.openWith.notifySaved", undefined, "已保存：用自定义程序打开/定位"],
    ["settings.interface.openWith.notifyReset", undefined, "已恢复系统默认打开方式"],
    ["settings.interface.openWith.statusSaveFailed", { error: "denied" }, "保存失败：denied"],
    ["settings.interface.openWith.saveFailed", { error: "denied" }, "保存打开方式失败: denied"],
    ["settings.addonlist.guard.restored", undefined, "检测到游戏覆盖 addonlist.txt，已恢复受保护版本"],
    ["app.protocol.failed", { message: "bad payload" }, "协议处理失败: bad payload"],
    ["servers.notification.favoriteAdded", { name: "My Server" }, "已收藏服务器：My Server"],
    ["servers.notification.favoriteDuplicate", { name: "My Server" }, "服务器已在收藏里：My Server"],
    ["servers.error.favoriteRefreshFailed", { error: "offline" }, "已添加服务器，但刷新列表失败: offline"],
    ["servers.panel.error.difficultyUnsupported", undefined, "当前后端不支持修改难度"],
  ];
  for (const [key, params, expected] of cases) {
    assert.equal(t(key, params), expected, `${key} 渲染结果`);
    assert.ok(runtime.includes(`t("${key}"`), `app-runtime 未接线 ${key}`);
  }
});
