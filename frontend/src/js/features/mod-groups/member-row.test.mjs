import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 成员行（详情 / 游戏开关 / 启用·禁用 / 复制到 addons）必须是**一份实现**：
// 「分组建议」卡片与「策略组管理」窗口都复用它，各写一遍必然走样。

const here = path.dirname(fileURLToPath(import.meta.url));
const source = readFileSync(path.join(here, "member-row.js"), "utf8");
const groupUi = readFileSync(path.join(here, "group-ui.js"), "utf8");
const manager = readFileSync(path.join(here, "strategy-group-manager.js"), "utf8");
const css = readFileSync(path.resolve(here, "../../../css/app/mods.css"), "utf8");

test("共享成员行覆盖四种 Mod 操作，并复用冲突检测的语义类名", () => {
  assert.match(source, /export function buildModMemberRow/);
  assert.match(source, /describeSuggestionMember\(/, "位置 / 游戏开关 / 可用动作要复用同一套语义判定");
  assert.match(source, /showFileDetail\(/, "详情按钮");
  assert.match(source, /toggleGameEnabled\(/, "游戏开关按钮");
  assert.match(source, /toggleFile\(/, "启用 / 禁用按钮");
  assert.match(source, /moveFileToAddons\(/, "复制到 addons 按钮");
  // 历史类名要保留：现有 CSS 与"折叠时隐藏成员明细"的规则都按它匹配。
  assert.match(source, /mod-group-suggest-member"/, "需要保留历史类名以复用样式");
  assert.match(source, /mod-member-row/, "同时要有通用类名");
  assert.match(source, /extraActions/, "要支持追加按钮（管理窗口的「移出本组」）");
});

test("分组建议卡片改为复用共享成员行，不再自己拼 DOM", () => {
  assert.match(groupUi, /import \{ buildModMemberRow \} from "\.\/member-row\.js"/);
  assert.match(groupUi, /buildModMemberRow\(\{/, "建议卡片要用共享实现");
  assert.equal(
    /"复制到 addons"/.test(groupUi),
    false,
    "按钮文案应该只存在于 member-row.js，避免两处各写一份",
  );
  assert.match(groupUi, /onActionDone: refreshAfterSuggestionMemberAction/);
});

test("策略组管理窗口的「展开成员」接的是同一套成员行", () => {
  assert.match(manager, /import \{ buildModMemberRow \} from "\.\/member-row\.js"/);
  assert.match(manager, /class="settings-strategy-expand"/, "每个组要有展开按钮");
  assert.match(manager, /class="strategy-group-members"/, "展开后的成员容器");
  assert.match(manager, /function renderExpandedMembers/);
  assert.match(manager, /buildSuggestionFileIndex\(/, "要用当前文件列表补齐位置 / 游戏开关 / 优先级");
  assert.match(manager, /RemoveModStrategyGroupMembers\(group\.id, \[key\]\)/, "每个成员要能移出本组");
  assert.match(manager, /strategyGroupExpanded/, "展开状态要存在 appState（重画后保持）");
  assert.match(css, /\.strategy-group-members\s*\{/, "展开区需要自己的样式");
});
