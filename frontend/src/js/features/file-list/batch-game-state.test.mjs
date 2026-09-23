import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 用户反馈：多选之后不能批量"游戏内启用"（只有搬文件的批量启用/禁用）。
// 这条链路：工具栏按钮 → actions.setSelectedGameEnabled → SetVPKGameEnabledBatch。

const here = path.dirname(fileURLToPath(import.meta.url));
const frontendRoot = path.resolve(here, "../../../..");
const indexHtml = readFileSync(path.join(frontendRoot, "index.html"), "utf8");
const actionsSource = readFileSync(path.join(here, "actions.js"), "utf8");
const runtimeSource = readFileSync(
  path.resolve(here, "../app-runtime.js"),
  "utf8",
);
const stateSource = readFileSync(path.resolve(here, "../state.js"), "utf8");

test("工具栏固定提供「游戏内启用 / 游戏内关闭」两个批量按钮", () => {
  assert.match(indexHtml, /id="game-enable-selected-btn"/, "缺少批量游戏内启用按钮");
  assert.match(indexHtml, /id="game-disable-selected-btn"/, "缺少批量游戏内关闭按钮");
  assert.match(indexHtml, /id="game-enable-selected-btn"[\s\S]{0,600}?游戏内启用/, "按钮文案要说清是游戏内开关");
  assert.match(indexHtml, /id="game-disable-selected-btn"[\s\S]{0,600}?游戏内关闭/);
  // 与搬文件的"批量启用 / 批量禁用"分开：两者的 tooltip 必须写明差异。
  assert.match(indexHtml, /只改 addonlist\.txt 的 0\/1，不移动、不删除文件/);
  assert.match(indexHtml, /id="enable-selected-btn"/);
  assert.match(indexHtml, /id="disable-selected-btn"/);
});

test("批量游戏内开关走一次性写盘的后端接口", () => {
  assert.match(actionsSource, /export async function setSelectedGameEnabled/);
  assert.match(actionsSource, /SetVPKGameEnabledBatch\(\s*targets\.map\(\(file\) => file\.path\),\s*enabled/, "要一次调用批量接口");
  // disabled 目录里的文件不能直接改游戏开关，要提示先放回 addons。
  assert.match(actionsSource, /file\.location !== "disabled"/);
  assert.match(actionsSource, /disabled 目录里的先用「批量启用」放回 addons/);
  // 未记录的条目要提示。
  assert.match(actionsSource, /formatBatchGameStateConfirm\(\{ total: targets\.length, skipped, unrecorded \}, enabled\)/);
  assert.match(actionsSource, /refreshFilesKeepFilter\(\)/, "改完要刷新列表状态");
});

test("按钮在 app-runtime 绑定、并纳入选择相关的启用/禁用清单", () => {
  assert.match(runtimeSource, /import \{[\s\S]*setSelectedGameEnabled[\s\S]*\} from "\.\/file-list\/actions\.js"/);
  assert.match(runtimeSource, /getElementById\("game-enable-selected-btn"\)[\s\S]{0,120}setSelectedGameEnabled\(true\)/);
  assert.match(runtimeSource, /getElementById\("game-disable-selected-btn"\)[\s\S]{0,120}setSelectedGameEnabled\(false\)/);
  const occurrences = stateSource.match(/game-(enable|disable)-selected-btn/g) || [];
  assert.ok(occurrences.length >= 2, "两个新按钮要同时加进 disableActionButtons 与 enableActionButtons");
});
