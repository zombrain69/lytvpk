import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// "单选 / 多选 -> 加入 / 移出 / 移动到某个策略组"这条链路的接线约束。
// 之前只有「分组」下拉里按组操作（＋/－ 选中的 N 个），单文件与多选菜单里没有任何入口，
// 用户答不出"怎么把一个 Mod 放进某个组 / 换到别的组"。这里把入口锁住。

const here = path.dirname(fileURLToPath(import.meta.url));
const frontendRoot = path.resolve(here, "../../../..");
const indexHtml = readFileSync(path.join(frontendRoot, "index.html"), "utf8");
const cssSource = readFileSync(path.resolve(frontendRoot, "src/css/app/mods.css"), "utf8");
const pickerSource = readFileSync(path.join(here, "group-picker.js"), "utf8");
const contextMenuSource = readFileSync(
  path.resolve(frontendRoot, "src/js/features/file-list/context-menu.js"),
  "utf8",
);
const appRuntimeSource = readFileSync(
  path.resolve(frontendRoot, "src/js/features/app-runtime.js"),
  "utf8",
);

test("选择器弹窗结构存在，且输入/按钮 id 与脚本一致", () => {
  [
    "group-picker-modal",
    "group-picker-title",
    "group-picker-summary",
    "group-picker-visible",
    "group-picker-search",
    "group-picker-relevant-only",
    "group-picker-list",
    "group-picker-new-name",
    "group-picker-new-btn",
    "group-picker-cancel-btn",
    "group-picker-confirm-btn",
  ].forEach((id) => {
    assert.match(indexHtml, new RegExp(`id="${id}"`), `index.html 缺少 id="${id}"`);
  });
});

test("选择器复用四种后端能力（加入 / 移出 / 移动 / 新建）", () => {
  assert.match(pickerSource, /AddModStrategyGroupMembers/);
  assert.match(pickerSource, /RemoveModStrategyGroupMembers/);
  assert.match(pickerSource, /MoveModStrategyGroupMembers/);
  assert.match(pickerSource, /CreateModStrategyGroupFromKeys/);
  assert.match(pickerSource, /refreshModGroupMembershipState/, "改完组归属必须刷新徽标");
});

test("选择器的便捷性：搜索 / 只看相关 / 排序 / 键盘高亮 / 双击执行", () => {
  // 搜索框与筛选开关固定在滚动区之外（.group-picker-toolbar 在 .group-picker-body 之前）。
  assert.match(indexHtml, /class="group-picker-toolbar"[\s\S]*class="group-picker-body"/, "工具条要在滚动区之前");
  assert.match(indexHtml, /class="group-picker-hint"/, "需要键盘提示文案");

  // 渲染与键盘共用同一条"过滤 + 排序"管线，避免两套规则不一致。
  assert.match(pickerSource, /function computeVisibleRows/);
  assert.match(pickerSource, /filterGroupPickerRows\(/);
  assert.match(pickerSource, /sortGroupPickerRows\(/);
  assert.match(pickerSource, /groupPickerRowTags\(/);
  assert.match(pickerSource, /groupPickerRowDetail\(/);
  assert.match(pickerSource, /nextSelectableRowId\(/);

  // 交互：自动聚焦搜索、方向键移动高亮、双击直接执行、行详情标签。
  assert.match(pickerSource, /element\("group-picker-search"\)[\s\S]{0,80}\.focus\(\)/, "打开要聚焦搜索框");
  assert.match(pickerSource, /event\.key === "ArrowDown"/);
  assert.match(pickerSource, /selectRowById\(nextId\)/);
  assert.match(pickerSource, /addEventListener\("dblclick"/, "双击应该直接执行");
  assert.match(pickerSource, /scrollIntoView\(\{ block: "nearest" \}\)/, "键盘移动要把高亮行滚进视野");
  assert.match(pickerSource, /pickerView\.relevantOnly/, "「只看相关」要真的参与过滤");

  // 样式：高亮行与标签要有可见反馈。
  assert.match(cssSource, /\.group-picker-row\.is-active/, "缺少键盘高亮样式");
  assert.match(cssSource, /\.group-picker-row-tag\b/, "缺少行内标签样式");
});

test("单文件菜单与多选菜单都挂了三个策略组入口", () => {
  assert.match(contextMenuSource, /appendGroupMenuItems\(menu, \[file\.path\]\)/, "单文件菜单缺少入口");
  assert.match(contextMenuSource, /appendGroupMenuItems\(menu, selectedPaths\)/, "多选菜单缺少入口");
  assert.match(contextMenuSource, /"加入策略组…"/);
  assert.match(contextMenuSource, /"从策略组移出…"/);
  assert.match(contextMenuSource, /"移动到其它策略组…"/);
  // 单文件 / 多选菜单都要把"组归属变化"反映到列表上。
  assert.match(contextMenuSource, /function refreshAfterGroupChange/);
  assert.match(contextMenuSource, /onDone: refreshAfterGroupChange/);
});

test("多选菜单的三个入口只在选中项确实属于某个组时出现", () => {
  assert.match(contextMenuSource, /const groupIds = groupIdsForPaths\(paths\)/);
  assert.match(contextMenuSource, /if \(groupIds\.length > 0\)/);
  assert.match(
    contextMenuSource,
    /sourceGroupId: groupIds\.length === 1 \? groupIds\[0\] : ""/,
    "只属于一个组时应直接带上源组",
  );
});

test("选择器在应用启动时绑定，并有自己的样式", () => {
  assert.match(appRuntimeSource, /initGroupPicker\(\)/);
  assert.match(cssSource, /\.group-picker-row\s*\{/);
  assert.match(cssSource, /\.group-picker-row\.is-disabled\s*\{/);
});

test("「新建并加入」在滚动区之外，策略组很多时也常显", () => {
  // 真实缺陷：它原本在 .group-picker-body（滚动区）内部，17 个策略组时被挤到
  // 列表末尾、需要滚到底才能看到。现在必须是 body 的兄弟节点（滚动区之外）。
  const bodyStart = indexHtml.indexOf('class="group-picker-body"');
  const bodyEnd = indexHtml.indexOf("</div>", indexHtml.indexOf('id="group-picker-list"'));
  const newRowIndex = indexHtml.indexOf('id="group-picker-new-row"');
  assert.ok(bodyStart > 0 && newRowIndex > 0, "缺少 body 或 new-row 节点");
  assert.ok(newRowIndex > bodyEnd, "「新建并加入」必须放在滚动区之外（body 之后）");

  const rule = cssSource.match(/\.group-picker-new-row\s*\{([^}]*)\}/);
  assert.ok(rule, "缺少 .group-picker-new-row 样式");
  assert.match(rule[1], /flex:\s*0 0 auto/, "「新建并加入」不能被压缩进滚动区");
});
