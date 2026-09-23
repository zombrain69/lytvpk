import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 「策略组管理」是一个独立窗口（不再塞在设置页里），并且窗口里的多选批量管理
// （批量删除 / 批量开关自动联动 / 批量设置·清除权重）必须真的接上线。
// 语义由 Go 侧 BatchUpdateModStrategyGroups 测试覆盖，这里只锁界面结构与接线。

const here = path.dirname(fileURLToPath(import.meta.url));
const managerSource = readFileSync(path.join(here, "strategy-group-manager.js"), "utf8");
const indexHtml = readFileSync(path.resolve(here, "../../../../index.html"), "utf8");
const runtimeSource = readFileSync(path.resolve(here, "../app-runtime.js"), "utf8");
const stateSource = readFileSync(path.resolve(here, "../state.js"), "utf8");
const settingsSource = readFileSync(
  path.resolve(here, "../settings/settings-page.js"),
  "utf8",
);
const groupUiSource = readFileSync(path.join(here, "group-ui.js"), "utf8");
const cssSource = readFileSync(path.resolve(here, "../../../css/app/mods.css"), "utf8");

test("独立窗口的结构在 index.html 里（头部 / 滚动区 / 底部）", () => {
  assert.match(indexHtml, /id="strategy-group-modal"/, "缺少策略组管理窗口");
  assert.match(indexHtml, /class="modal-content strategy-group-content"/, "窗口需要独立尺寸");
  assert.match(indexHtml, /class="strategy-group-body"/, "窗口需要自己的滚动区");
  [
    "strategy-group-close-btn",
    "strategy-group-close-footer-btn",
    "strategy-group-name",
    "strategy-group-strategy",
    "strategy-group-capture",
    "strategy-group-status",
    "strategy-group-list",
  ].forEach((id) => {
    assert.match(indexHtml, new RegExp(`id="${id}"`), `缺少 #${id}`);
  });
});

test("窗口提供多选与批量工具条", () => {
  [
    "strategy-group-batch",
    "strategy-group-select-all",
    "strategy-group-selection",
    "strategy-group-batch-delete",
    "strategy-group-batch-enforce-on",
    "strategy-group-batch-enforce-off",
    "strategy-group-batch-tier-input",
    "strategy-group-batch-tier-save",
    "strategy-group-batch-tier-clear",
  ].forEach((id) => {
    assert.match(indexHtml, new RegExp(`id="${id}"`), `缺少 #${id}`);
  });
  assert.match(managerSource, /class="settings-strategy-pick-input"/, "每个组需要多选框");
  assert.match(managerSource, /function renderManager/, "需要统一的组列表渲染函数");
});

test("批量操作走同一个后端方法，并用应用内确认弹窗", () => {
  assert.match(managerSource, /BatchUpdateModStrategyGroups\(/, "需要调用批量接口");
  ["delete", "enforce_on", "enforce_off", "set_tier", "clear_tier"].forEach((action) => {
    assert.match(managerSource, new RegExp(`"${action}"`), `缺少 ${action} 动作`);
  });
  assert.match(managerSource, /showConfirmModal\(\s*"批量管理策略组"/, "批量操作要应用内确认");
  assert.match(managerSource, /formatStrategyGroupBatchResult/, "批量结果要有提示文案");
});

test("选中集合存在 appState 里，跨重新渲染保留", () => {
  assert.match(stateSource, /strategyGroupSelection:\s*new Set\(\)/, "appState 需要保存选中集合");
  assert.match(managerSource, /strategyGroupSelection/, "绑定逻辑要复用 appState 里的集合");
  assert.match(managerSource, /function pruneSelection/, "需要剔除已被删除的组");
  // 「全选」必须跟随选中集合：全选时 checked，部分选中时半选（indeterminate）。
  assert.match(managerSource, /function syncSelectAll/, "缺少全选框状态同步");
  assert.match(managerSource, /indeterminate = selected > 0 && selected < total/, "部分选中要显示半选");
});

test("组生命周期动作都在窗口里，删除仍只删 groups.json 记录", () => {
  ["按策略应用", "随机单选", "全关", "重命名", "删除"].forEach((label) => {
    assert.match(indexHtml + managerSource, new RegExp(label), `缺少「${label}」`);
  });
  assert.match(managerSource, /DeleteModStrategyGroup\(/, "窗口需要删除");
  assert.match(managerSource, /RenameModStrategyGroup\(/, "窗口需要重命名");
  assert.match(managerSource, /SetModStrategyGroupTier\(/, "窗口需要组权重");
  assert.match(managerSource, /MoveModStrategyGroup\(/, "窗口需要上级分组");
  assert.match(
    managerSource,
    /不会改动 addonlist\.txt，也不会删除任何 Mod 文件/,
    "删除确认要写清影响范围",
  );
});

test("组行可以展开成员：直接配 Mod 选项，与分组建议窗口一致", () => {
  // 展开按钮 + 成员容器 + 共享成员行（详情 / 游戏开关 / 启用禁用 / 复制到 addons / 移出本组）。
  assert.match(managerSource, /settings-strategy-expand/, "缺少「展开成员」按钮");
  assert.match(indexHtml + managerSource, /展开成员/, "按钮文案要说明能展开");
  assert.match(managerSource, /strategy-group-members/, "缺少成员容器");
  assert.match(managerSource, /buildModMemberRow\(/, "成员行要复用共享实现");
  assert.match(managerSource, /RemoveModStrategyGroupMembers\(/, "成员要能移出本组");
  assert.match(stateSource, /strategyGroupExpanded:\s*new Set\(\)/, "展开状态要存在 appState");
  assert.match(managerSource, /function pruneExpanded/, "已删除的组要从展开集合里剔除");
});

test("窗口有三个入口：分组菜单、设置页入口按钮、窗口自身初始化", () => {
  assert.match(indexHtml, /id="mod-group-manager-btn"/, "分组菜单缺少入口");
  assert.match(groupUiSource, /openStrategyGroupManager\(\)/, "分组菜单入口没有接线");
  // 设置页是运行时渲染的，入口按钮写在 settings-page.js 的模板里。
  assert.match(settingsSource, /id="settings-open-strategy-manager"/, "设置页缺少入口按钮");
  assert.match(settingsSource, /openStrategyGroupManager\(\)/, "设置页入口没有接线");
  assert.match(runtimeSource, /initStrategyGroupManager\(\)/, "app-runtime 要初始化窗口");
});

test("浮动窗口：打开时就能直接操作主界面的分组筛选", () => {
  // 与其它管理窗口完全一致：开关是通用模块自动插到标题栏的按钮，页面里不再有自定义那一行。
  assert.equal(
    /strategy-group-toolbar|strategy-group-floating/.test(indexHtml),
    false,
    "策略组窗口不应再有自定义的浮动开关行（统一走通用设计）",
  );
  assert.equal(
    /strategy-group-toolbar|strategy-group-floating-toggle/.test(cssSource),
    false,
    "自定义浮动行的样式也应删除，避免留下死代码",
  );
  const configSource = readFileSync(path.resolve(here, "../../core/config.js"), "utf8");
  assert.match(configSource, /strategyGroupFloating:\s*true/, "默认应为浮动（不挡主界面）");
  // 接线：浮动能力统一走 core/floating-modal.js（去掉虚化、可拖动、位置记忆、可缩放）。
  assert.match(managerSource, /setupFloatingModal\(element\("strategy-group-modal"\)/, "缺少浮动窗口注册");
  assert.match(managerSource, /getConfig\(\)\.strategyGroupFloating !== false/, "打开时读取配置");
  assert.match(managerSource, /getFloatingModal\("strategy-group-modal"\)\?\.refresh\(\)/, "打开时重新应用偏好");
  assert.match(managerSource, /saveConfig\(config\)/, "开关状态要持久化");
  assert.match(managerSource, /defaultPosition: \(\) =>/, "默认停靠在工具栏下方、菜单右侧");
  const floatingSource = readFileSync(path.resolve(here, "../../core/floating-modal.js"), "utf8");
  assert.match(floatingSource, /startModalDrag/, "拖动实现在通用模块里");
  assert.match(floatingSource, /modal-float-toggle/, "通用按钮由模块自动插入标题栏");
});

test("在管理窗口里就能把组变成筛选（单组 + 多选批量）", () => {
  // 工具条：批量筛选 + 清除筛选 + 当前筛选文案。
  ["strategy-group-batch-filter", "strategy-group-batch-filter-clear", "strategy-group-filter-state"].forEach((id) => {
    assert.match(indexHtml, new RegExp(`id="${id}"`), `index.html 缺少 id="${id}"`);
  });
  assert.match(indexHtml, /用选中的组筛选/, "缺少批量筛选按钮文案");
  // 组行：筛选这组 / 取消筛选。
  assert.match(managerSource, /settings-strategy-filter/, "组行缺少筛选按钮");
  assert.match(managerSource, /筛选这组/, "按钮文案要说明用途");
  assert.match(managerSource, /取消筛选/, "再点一次要能取消");
  // 接线：改的是 appState.activeGroupFilter 并真的重新筛选列表。
  assert.match(managerSource, /function applyGroupFilterIds/);
  assert.match(managerSource, /appState\.activeGroupFilter = new Set\(/);
  assert.match(managerSource, /await refreshFilesKeepFilter\(\)/, "筛选后要重新过滤列表");
  assert.match(managerSource, /formatStrategyGroupFilterState/, "要显示当前筛选");
  // 样式：正在筛选的行与按钮要有可见反馈。
  assert.match(cssSource, /\.settings-strategy-filter\.is-active/);
  assert.match(cssSource, /\.settings-profile-item\.is-filtered/);
});

test("设置页不再自带策略组管理界面", () => {
  // 回归守卫：设置页曾经把整块策略组生命周期塞在卡片里，还会因为绑定缺失整页变白。
  assert.equal(
    /id="settings-strategy-group-capture"/.test(settingsSource),
    false,
    "设置页不能再出现建组输入与按钮",
  );
  assert.equal(
    /id="settings-strategy-batch/.test(settingsSource),
    false,
    "设置页不能再出现批量工具条",
  );
  assert.equal(
    /bindModStrategyGroupSettings/.test(settingsSource),
    false,
    "设置页不应再绑定策略组生命周期",
  );
  assert.equal(
    /class="settings-strategy-pick-input"/.test(settingsSource),
    false,
    "设置页不应再渲染组列表",
  );
  // 指针式入口允许保留，并且只能有一个按钮，避免又长回一整块管理界面。
  assert.match(settingsSource, /策略组已经搬到独立窗口/, "设置页要说明入口去哪了");
});

test("勾选与组归属变化会驱动窗口刷新", () => {
  assert.match(stateSource, /onFileSelectionChanged/, "state 需要广播勾选变化");
  assert.match(managerSource, /onFileSelectionChanged\(/, "窗口要跟着勾选更新建组按钮");
  assert.match(managerSource, /updateCaptureButton\(\)/, "缺少建组按钮状态刷新");
  assert.match(managerSource, /onModGroupMembershipChanged\(/, "组归属变化要同步窗口");
  assert.match(managerSource, /refreshStrategyGroupManagerIfOpen/, "窗口开着才刷新");
});

test("窗口直接调用后端绑定，并由 app-runtime 初始化", () => {
  // 策略组不再依赖设置页的 deps 注入：窗口自己 import wailsjs 绑定（与 group-ui.js 一致）。
  assert.match(
    managerSource,
    /from "\.\.\/\.\.\/\.\.\/\.\.\/wailsjs\/go\/app\/App"/,
    "窗口要直接从 wailsjs 绑定导入",
  );
  assert.match(managerSource, /BatchUpdateModStrategyGroups\(/, "批量接口要由窗口直接调用");
  assert.match(runtimeSource, /import \{ initStrategyGroupManager \}/, "缺少窗口初始化导入");
});

test("窗口里改组之后，外面的列表与筛选菜单要一起刷新", () => {
  // 真实缺陷：在管理窗口点「保存权重」只 reload 了本窗口，
  // 外面「按分组筛选」的顺序（由组权重决定）要手动点工具栏「刷新」才更新。
  const start = managerSource.indexOf('.settings-strategy-rename"');
  const end = managerSource.indexOf("async function captureGroupFromSelection");
  assert.ok(start > 0 && end > start, "找不到管理窗口的动作绑定区段");
  const block = managerSource.slice(start, end);
  const calls = (block.match(/await refreshAfterChange\(\)/g) || []).length;
  assert.ok(
    calls >= 4,
    `重命名 / 删除 / 自动联动 / 组权重 / 上级分组 都要刷新外部列表，实际只有 ${calls} 处`,
  );
  // 组权重这一段尤其要刷新（它决定筛选菜单顺序）。
  const tierBlock = managerSource.slice(
    managerSource.indexOf('.settings-strategy-tier-save"'),
    end,
  );
  assert.match(tierBlock, /await refreshAfterChange\(\)/, "保存权重后必须刷新外部列表");
});
