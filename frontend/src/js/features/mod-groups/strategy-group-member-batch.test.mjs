import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 「策略组管理」窗口的成员级批量：界面入口 + 真实后端调用各锁一条。
// 背景：这个窗口以前只能一个一个点每行右侧的按钮，批量动作（启用 / 禁用 /
// 游戏开关 / 移出本组）后端早就有，但界面没有入口。

const manager = readFileSync(new URL("./strategy-group-manager.js", import.meta.url), "utf8");
const html = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");
const memberRow = readFileSync(new URL("./member-row.js", import.meta.url), "utf8");

test("窗口里有成员批量条与全选入口", () => {
  for (const id of [
    "strategy-group-member-batch",
    "strategy-group-member-select-all",
    "strategy-group-member-selection",
    "strategy-group-member-batch-game-on",
    "strategy-group-member-batch-game-off",
    "strategy-group-member-batch-enable",
    "strategy-group-member-batch-disable",
    "strategy-group-member-batch-transfer",
    "strategy-group-member-batch-remove",
    "strategy-group-member-batch-hint",
  ]) {
    assert.match(html, new RegExp(`id="${id}"`), `index.html 缺少 ${id}`);
  }
});

test("组级 / 成员级批量条都固定在滚动区之外（否则滚下去就看不见）", () => {
  // 真实缺陷：两条批量条原本在 .strategy-group-body（可滚动）内部、位于列表之前，
  // 用户滚到列表下半部分时它们已经滚出视野，只看到组头的「全关」，以为没有批量入口。
  const divDepth = (fragment) =>
    (fragment.match(/<div\b/g) || []).length - (fragment.match(/<\/div>/g) || []).length;

  const bodyOpen = html.indexOf('class="strategy-group-body"');
  const footerIndex = html.indexOf('<div class="modal-footer">', bodyOpen);
  assert.ok(bodyOpen > 0, "index.html 里应能找到 .strategy-group-body");

  for (const id of ["strategy-group-batch", "strategy-group-member-batch"]) {
    const barIndex = html.indexOf(`id="${id}"`);
    assert.ok(barIndex > bodyOpen && footerIndex > barIndex, `${id} 应位于滚动区与 modal-footer 之间`);
    assert.equal(divDepth(html.slice(bodyOpen, barIndex)), 0,
      `${id} 必须已经在滚动区之外（滚动区已闭合），不能放在 .strategy-group-body 里面`);
  }
});

test("成员行渲染勾选框，勾选状态进入选择集合", () => {
  assert.match(memberRow, /options\.pick|pick = null/, "成员行实现要支持勾选框");
  assert.match(manager, /pick:\s*\{/, "展开成员时要传 pick");
  assert.match(manager, /memberSelection\.add\(normalizedKey\)/, "勾选要写进选择集合");
  assert.match(manager, /memberSelection\.delete\(normalizedKey\)/, "取消勾选要移出集合");
});

test("批量按钮真的调用后端（不是只改界面）", () => {
  assert.match(manager, /SetVPKGameEnabledBatch\(targets\.game, enabled\)/, "游戏开关要走批量接口");
  assert.match(manager, /await ToggleVPKFile\(path\)/, "启用/禁用要真的搬文件");
  assert.match(manager, /moveWorkshopFilesToAddons\(targets\.transfer\)/, "复制到 addons 要复用批量转移");
  assert.match(
    manager,
    /RemoveModStrategyGroupMembers\(item\.groupId, item\.keys\)/,
    "移出本组要按组调用后端",
  );
  assert.match(manager, /planMemberRemoval\(\[\.\.\.memberSelection\]/, "移出前要先算出会影响哪些组");
});

test("禁用与启用共用主列表的风险确认，而不是静默搬文件", () => {
  assert.match(manager, /confirmVPKOperationWarning\(paths, `批量\$\{label\}策略组成员`\)/);
});

test("批量游戏开关复用主列表的确认与结果文案", () => {
  assert.match(manager, /formatBatchGameStateConfirm\(/, "确认弹窗要与主列表同源");
  assert.match(manager, /formatBatchGameStateSummary\(result, enabled\)/, "结果提示要与主列表同源");
});

test("按钮可用性由纯函数算出并带「为什么不能点」", () => {
  assert.match(manager, /describeMemberBatchActions\(currentMemberTargets\(\)\)/);
  assert.match(manager, /button\.title = action\.title/, "灰掉的按钮要说明原因");
});
