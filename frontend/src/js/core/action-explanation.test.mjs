import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  EXPLANATION_SCENES,
  explainActionAvailability,
  explainOperationError,
  formatExplanation,
  sceneFromErrorType,
} from "./action-explanation.mjs";

test("后端错误被翻成「发生了什么 + 接下来怎么做」", () => {
  const cases = [
    ["这个文件不在当前受管的 addons / workshop / disabled 目录里：E:\\x.vpk", /不在当前管理/, /刷新/],
    ["另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试", /上一个文件操作/, /状态栏/],
    ["workshop文件需要先转移到插件目录才能启用/禁用", /创意工坊 Mod 不能直接开关/, /复制到 addons/],
    ["未选择L4D2目录", /还没有选择游戏目录/, /选择 addons 目录/],
    ["addonlist.txt 记录了 a.vpk，但磁盘上找不到对应文件", /已经不在了/, /刷新列表/],
    ["目标文件已存在: b.vpk", /已经有同名文件/, /替换/],
    ["删除文件失败: Access is denied", /系统拒绝/, /占用/],
    // 这条说的是"目标路径不可用"，别和"源文件不在了"混为一谈
    ["移动 a.vpk 失败: The system cannot find the path specified", /目标路径当前不可用/, /目标目录还在/],
  ];
  for (const [raw, summaryPattern, hintPattern] of cases) {
    const explained = explainOperationError(raw);
    assert.match(explained.summary, summaryPattern, `摘要不对：${raw}`);
    assert.match(explained.hint, hintPattern, `建议不对：${raw}`);
    assert.equal(explained.raw, raw, "原文要原样保留（控制台排查用）");
    assert.equal(explained.matched, true);
  }

  // 文件名不能被解释层吞掉：批量操作要知道"是谁失败了"
  assert.equal(explainOperationError("移动 a.vpk 失败: 目标已存在").subject, "a.vpk");
  assert.equal(explainOperationError("跳过 b.vpk: 这个文件不在当前受管的 addons / workshop / disabled 目录里").subject, "b.vpk");
  assert.equal(explainOperationError("某种没见过的后端错误 XYZ-123").subject, "");
});

test("场景区分：输入类错误只在输入场景按「输入不合法」解释", () => {
  const raw = "文件名不合法：文件名不能包含 \\ 这类字符（Windows 命名规则）";
  const asInput = explainOperationError(raw, { scene: EXPLANATION_SCENES.input });
  assert.match(asInput.summary, /这个名字不能用作文件名/);

  // 同一个错误在"操作"场景里不会命中输入规则 → 走兜底，但仍有话说
  const asOperation = explainOperationError(raw);
  assert.equal(asOperation.summary, raw, "非输入场景保留原文摘要");
  assert.match(asOperation.hint, /反馈/);
  assert.equal(asOperation.matched, false);

  // 正则写错同样只在输入场景
  assert.match(explainOperationError("正则表达式无效：Unterminated character class", { scene: "input" }).summary, /正则写错了/);
});

test("找不到规则时也必须给出一句话（对齐上游「永远有话说」）", () => {
  const weird = explainOperationError("某种没见过的后端错误 XYZ-123");
  assert.equal(weird.summary, "某种没见过的后端错误 XYZ-123");
  assert.ok(weird.hint.length > 0);

  const empty = explainOperationError("   ");
  assert.equal(empty.summary, "操作失败");
  assert.ok(empty.hint.length > 0);

  // 对象形态（Wails 抛出的 errorInfo）也能吃
  assert.equal(explainOperationError({ message: "未选择L4D2目录" }).summary, "还没有选择游戏目录");
});

test("操作可用性：没选目录 / 忙碌 / 工坊 Mod / disabled / 没选中，各有理由与出路", () => {
  const noRoot = explainActionAvailability({ action: "move", context: { hasRoot: false } });
  assert.equal(noRoot.available, false);
  assert.match(noRoot.reason, /还没有选择游戏目录/);

  const busy = explainActionAvailability({ action: "delete", context: { busy: true } });
  assert.equal(busy.available, false);
  assert.match(busy.reason, /正在处理另一个文件操作/);
  assert.match(busy.hint, /状态栏/);

  // 只读操作不受忙碌影响
  assert.equal(explainActionAvailability({ action: "open-location", context: { busy: true } }).available, true);

  const workshop = explainActionAvailability({ action: "toggle", file: { location: "workshop" } });
  assert.equal(workshop.available, false);
  assert.match(workshop.reason, /创意工坊 Mod 不能直接启用/);
  assert.match(workshop.hint, /复制到 addons/);

  const disabledGameState = explainActionAvailability({ action: "game-state", file: { location: "disabled" } });
  assert.equal(disabledGameState.available, false);
  assert.match(disabledGameState.reason, /disabled 目录/);

  const noSelection = explainActionAvailability({ action: "batch", context: { selectionCount: 0 } });
  assert.equal(noSelection.available, false);
  assert.match(noSelection.reason, /还没有选中任何 Mod/);
  assert.equal(explainActionAvailability({ action: "batch", context: { selectionCount: 3 } }).available, true);

  // 「禁用」按钮：缺路径 / 已在 disabled / 非根目录各有理由
  assert.match(explainActionAvailability({ action: "disable-file", file: { location: "root" } }).reason, /没有文件路径/);
  assert.match(explainActionAvailability({ action: "disable-file", file: { path: "d:\\x.vpk", location: "disabled" } }).reason, /已经在 disabled/);
  assert.match(
    explainActionAvailability({ action: "disable-file", file: { path: "d:\\w\\x.vpk", location: "workshop" } }).reason,
    /只有 addons 根目录/,
  );
  assert.equal(explainActionAvailability({ action: "disable-file", file: { path: "d:\\x.vpk", location: "root" } }).available, true);

  // 正常情况：可用且没有多余文案
  const ok = explainActionAvailability({ action: "toggle", file: { location: "root", enabled: true } });
  assert.deepEqual(ok, { available: true, reason: "", hint: "" });
});

test("formatExplanation 拼成一行；没有建议时不留空括号", () => {
  assert.equal(formatExplanation({ summary: "发生了什么", hint: "怎么办" }), "发生了什么（怎么办）");
  assert.equal(formatExplanation({ summary: "发生了什么", hint: "" }), "发生了什么");
  assert.equal(formatExplanation({ summary: "", hint: "只有建议" }), "只有建议");
  assert.equal(formatExplanation(null), "");
});

test("错误类型决定场景：输入类用「输入不合法」的说法", () => {
  assert.equal(sceneFromErrorType(""), "operation");
  assert.equal(sceneFromErrorType("文件操作"), "operation");
  assert.equal(sceneFromErrorType("输入"), "input");
  assert.equal(sceneFromErrorType("重命名"), "input");
  assert.equal(sceneFromErrorType("VPK解析"), "input");
  // 接上后："输入" 类型的文件名错误会走输入场景的解释
  const explained = explainOperationError("文件名不合法：文件名不能包含 \\ 这类字符（Windows 命名规则）", {
    scene: sceneFromErrorType("重命名"),
  });
  assert.match(explained.summary, /这个名字不能用作文件名/);
});

test("界面真的接上了这层解释（失败文案 + 全局错误 + 按钮理由）", () => {
  const moveFormat = readFileSync(new URL("../features/file-list/move-result-format.mjs", import.meta.url), "utf8");
  const operations = readFileSync(new URL("../features/file-list/operations.js", import.meta.url), "utf8");
  const toast = readFileSync(new URL("./toast.js", import.meta.url), "utf8");
  const modelStats = readFileSync(new URL("../features/diagnostics/model-stats-scan.js", import.meta.url), "utf8");

  assert.match(moveFormat, /explainOperationError/, "逐项失败原因要带「人话 + 下一步」");
  assert.match(toast, /explainOperationError/, "全局错误提示要带解释");
  assert.match(modelStats, /explainActionAvailability/, "灰掉的按钮要说清为什么");
  assert.match(operations, /explainActionAvailability\(\{ action: "game-state", file \}\)/, "游戏内开关的前置检查要走同一层");
  assert.match(operations, /explainActionAvailability\(\{ action: "disable-file", file \}\)/, "禁用前置检查要走同一层");
  // 旧的重复文案不该再留一份（避免两处说法漂移）
  assert.ok(!operations.includes("该 Mod 位于 disabled 目录,请先恢复文件后再编辑游戏内开关"), "旧文案应已移除");
  assert.ok(!operations.includes("该 Mod 位于 disabled 目录，请先恢复文件后再编辑游戏内开关"), "旧文案应已移除");
});
