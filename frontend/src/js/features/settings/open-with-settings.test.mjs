// 「自定义外部打开程序」的设置页接线契约（对齐 FireAxe v0.7.2 的 process file customization）。
// 约定：默认留空 = 完全走系统默认（explorer /select），配置了才替换。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const settingsPage = readFileSync(new URL("./settings-page.js", import.meta.url), "utf8");
const runtime = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
const configSource = readFileSync(new URL("../../core/config.js", import.meta.url), "utf8");

test("设置页有程序 / 参数两个输入框与保存、恢复默认按钮", () => {
  assert.match(settingsPage, /id="settings-open-with-program"/, "缺少程序输入框");
  assert.match(settingsPage, /id="settings-open-with-arguments"/, "缺少参数输入框");
  assert.match(settingsPage, /id="settings-open-with-save"/, "缺少保存按钮");
  assert.match(settingsPage, /id="settings-open-with-reset"/, "缺少恢复默认按钮");
  assert.match(settingsPage, /\{path\}|\{dir\}|\{name\}/, "界面要说明占位符写法");
  assert.match(settingsPage, /留空.*系统默认|系统默认.*留空/, "要写清留空等于系统默认");
});

test("保存/恢复默认用事件委托绑定并调用后端 SetOpenWithSettings", () => {
  // 设置面板整块重渲染会换掉按钮节点，直接绑定会失效（真机踩过），所以必须是委托。
  assert.match(
    runtime,
    /addEventListener\("click",[\s\S]{0,200}closest\?\.\("#settings-open-with-save"\)/,
    "保存按钮要用事件委托",
  );
  assert.match(
    runtime,
    /closest\?\.\("#settings-open-with-reset"\)/,
    "恢复默认按钮也要用事件委托",
  );
  assert.match(runtime, /await SetOpenWithSettings\(program, argumentsTemplate\)/, "保存要调用后端");
  assert.match(runtime, /GetOpenWithSettings/, "读取配置要注入 GetOpenWithSettings");
  assert.match(runtime, /SetOpenWithSettings/, "保存要注入 SetOpenWithSettings");
  assert.ok(
    !/settings-open-with-save"\)\?\.addEventListener/.test(settingsPage),
    "不要在设置页里直接绑定（会被重渲染冲掉）",
  );
});

test("前端配置默认值是空字符串（= 系统默认）", () => {
  assert.match(configSource, /openWithProgram:\s*""/, "默认程序应为空");
  assert.match(configSource, /openWithArguments:\s*""/, "默认参数应为空");
});
