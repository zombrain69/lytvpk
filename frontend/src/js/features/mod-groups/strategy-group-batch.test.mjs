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
  // 半选判定按"当前可见（搜索后）的组"来算：搜出 3 个组时勾 1 个就是半选。
  assert.match(
    managerSource,
    /indeterminate = selectedVisible > 0 && selectedVisible < visibleIds\.length/,
    "部分选中要显示半选",
  );
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

test("窗口里有一个真正的搜索框：按组名/成员名过滤，全选只作用于搜索结果", () => {
  // 顶部那个输入框是"新建组名称"，用户会误当搜索框 —— 现在另有明确的搜索行。
  assert.match(indexHtml, /id="strategy-group-search"/, "缺少搜索框");
  // 占位文案现在还要点出统一语法（-排除 / re: 正则 / tag: 状态），与其它列表保持一致
  assert.match(indexHtml, /placeholder="搜索策略组（组名 \/ 成员名 \/ 描述；支持 -排除 \/ re: 正则 \/ tag:状态）/, "搜索框要有明确占位文案");
  assert.match(indexHtml, /id="strategy-group-visible"/, "要显示「显示 X / Y 个组」");
  assert.match(indexHtml, /新策略组的名称，例如 角色替换包/, "名称输入框要区分于搜索框");
  // 接线：搜索 → 过滤渲染；全选 → 只勾当前搜索出的组。
  assert.match(managerSource, /filterStrategyGroupRows\(allRows, managerQuery\)/, "渲染要用过滤后的行");
  assert.match(managerSource, /function visibleGroupIds/, "全选/批量要基于当前可见组");
  assert.match(managerSource, /ids\.forEach\(\(id\) => selection\.add\(id\)\)/, "全选只作用于搜索结果");
});

test("禁用状态要说明原因（别让按钮看起来像坏了）", () => {
  // 批量按钮：没勾选时 tooltip 说明怎么勾；勾上后恢复原 tooltip。
  assert.match(managerSource, /先勾选要批量操作的策略组（每行最左边的方框，或直接点组名）/, "批量按钮要解释禁用原因");
  assert.match(managerSource, /dataset\.defaultTitle/, "禁用提示不能覆盖按钮原有 tooltip");
  assert.match(managerSource, /id="strategy-group-batch-hint"|element\("strategy-group-batch-hint"\)/, "工具条要有提示行");
  assert.match(indexHtml, /id="strategy-group-batch-hint"/, "index.html 要有提示行元素");
  // 建组按钮：没勾 Mod 时把原因写在按钮上。
  assert.match(managerSource, /先在 Mod 管理页勾选 Mod/, "建组按钮禁用时要说清原因");
  assert.match(managerSource, /还没勾选 Mod：先回到 Mod 管理页勾选要归入同一组的 Mod/, "建组按钮要有 tooltip");
});

test("每组都有「＋ 子组」快捷入口，走 CreateModStrategyGroupChild", () => {
  // 组行按钮 + 提示：子组会累加全部上级分组的权重（和 FireAxe 的层级累加一致）。
  assert.match(managerSource, /settings-strategy-add-child/, "组行缺少「＋ 子组」按钮");
  assert.match(managerSource, /＋ 子组/, "按钮文案");
  assert.match(managerSource, /子组会累加它和全部上级分组的权重/, "tooltip 要说明累加语义");
  assert.match(managerSource, /CreateModStrategyGroupChild\(parentId, name, "single", selected\)/, "要调用后端接口并带上勾选的 Mod");
  assert.match(managerSource, /showPromptModal\(\s*"新建子组"/, "新建子组要用应用内输入弹窗");
  // 没勾 Mod 时允许建空子组，并在弹窗里说明。
  assert.match(managerSource, /现在没有勾选 Mod，会先建一个空子组/, "空子组要说明");
});

test("应用策略前先预检（对齐 FireAxe CheckEnableStrategy）", () => {
  assert.match(managerSource, /CheckModStrategyGroupApply\(id, \{/, "应用前要调用后端预检");
  // 不可执行 → 直接拦下并说明原因。
  assert.match(managerSource, /check\.applicable === false/, "不可执行要拦下");
  assert.match(managerSource, /showNotification\(check\.reason \|\| "这个策略现在无法执行", "error"\)/);
  // 能执行但有提醒 → 应用内确认框逐条列出。
  assert.match(managerSource, /"应用前检查"/, "有提醒时要确认");
  assert.match(managerSource, /warnings\.join\("\\n· "\)/, "提醒要逐条列出");
  // 取消确认后按钮要恢复可用（showConfirmModal 的 onCancel 是第 6 个参数）。
  assert.match(managerSource, /() => void run\(\),\s*false,\s*"",\s*\(\) => \{\s*button\.disabled = false;/, "取消要恢复按钮");
});

test("组行可以拖动排序（拖放落点判定走纯函数 + 后端两个接口）", () => {
  // 行本身可拖动，并带上落点提示。
  assert.match(managerSource, /data-group-row="\$\{escapeAttr\(group\.id\)\}"[^>]*draggable="true"/, "组行要可拖动");
  assert.match(managerSource, /拖动这一行/, "行上要有拖动说明");
  // 落点判定：内部/前/后三种位置都要用到纯函数。
  ["resolveStrategyGroupDrop", "applyStrategyGroupDropOrder", "formatStrategyGroupDropMessage"].forEach((name) => {
    assert.match(managerSource, new RegExp(name), `缺少 ${name} 接线`);
  });
  assert.match(managerSource, /positionFor\(row, event\)/, "要按行的上/中/下三段判定落点");
  assert.match(managerSource, /offset < 0\.28/, "顶部 28% 视为排到前面");
  assert.match(managerSource, /offset > 0\.72/, "底部 28% 视为排到后面");
  // 上级变更 + 同级顺序：两个后端接口按顺序调用。
  assert.match(managerSource, /await MoveModStrategyGroup\(movingId, plan\.parentId \|\| ""\)/, "先改上级");
  assert.match(managerSource, /await ReorderModStrategyGroups\(order\)/, "再写同级顺序");
  // 非法落点要有提示，且不写盘。
  assert.match(managerSource, /if \(plan\?\.reason\) showNotification\(plan\.reason, "error"\)/, "非法拖放要说明原因");
  assert.match(managerSource, /if \(plan\.noop\) return;/, "无变化的拖放不应写盘");
  // 底部"回到顶层"是独立落点，且只绑定一次。
  assert.match(indexHtml, /id="strategy-group-drop-root"/, "窗口需要「回到顶层」落点");
  assert.match(managerSource, /strategy-group-drop-root/, "落点要接线");
  assert.match(managerSource, /dropRoot\.dataset\.bound === "1"/, "静态落点只能绑一次");
  assert.match(managerSource, /bindGroupDropRoot\(\);/, "初始化时要绑定落点");
  // 组一多就必须滚到底才能用落点 —— 真机量过（3 组就会把落点顶到可视区外），所以必须粘住。
  const dropRootCss = cssSource.match(/\.strategy-group-drop-root\s*\{[^}]*\}/);
  assert.ok(dropRootCss, "缺少 .strategy-group-drop-root 样式");
  assert.match(dropRootCss[0], /position:\s*sticky/, "落点要粘在滚动区底部");
  assert.match(dropRootCss[0], /bottom:\s*0/, "落点要贴底");
});

// 对齐 FireAxe 的 AddonChildrenProblem：父组行要能看出"子组里有缺失"。
test("父组行汇总子组缺失（AddonChildrenProblem）", () => {
  assert.match(managerSource, /missingSummary/, "要保留完整缺失条目");
  assert.match(managerSource, /formatGroupSubtreeMissingNotice\(/, "要用纯函数生成子树缺失文案");
  assert.match(managerSource, /subtreeMissingCount/, "要读后端新增的子树缺失字段");
  assert.match(managerSource, /settings-strategy-missing is-subtree/, "父组行要有独立的子树提示样式");
  assert.match(managerSource, /展开子组即可看到/, "提示要说明下一步怎么做");
});
