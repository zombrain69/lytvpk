// 界面语言设置（路线图第 19 项 P0）的接线测试，沿用「源码接线 + 目录内容」双查模式：
//   - 设置页：语言行渲染受 LANGUAGE_SWITCH_ENABLED 控制（P0 保持隐藏）、
//     变更处理走 normalizeLocale + setLocale + saveConfig；
//   - 启动接线：app-runtime 在 initConfig 之后应用持久化 locale；i18n-setup 一次装齐目录；
//   - 目录内容：zh-CN 与 en 都有 settings.interface.language.* 文案；
//   - 双端白名单：core/locale.mjs 与后端 internal/app/config.go 的取值集合一致。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (rel) => readFileSync(new URL(rel, import.meta.url), "utf8");
const readJson = (rel) => JSON.parse(read(rel));

test("设置页：语言行由 LANGUAGE_SWITCH_ENABLED 控制，切换走 setLocale + 落盘", () => {
  const source = read("./settings-page.js");
  assert.match(source, /LANGUAGE_SWITCH_ENABLED \? `/, "语言行必须受开关控制（P0 隐藏）");
  assert.match(source, /id="settings-language"/);
  assert.match(source, /setLocale\(next\)/, "切换要即时生效");
  assert.match(source, /config\.locale = next/, "locale 要写回配置");
  assert.match(source, /settings\.interface\.language\.saved/);
});

test("启动接线：initConfig 之后应用持久化 locale，setup 一次装齐目录", () => {
  const runtime = read("../app-runtime.js");
  assert.match(
    runtime,
    /setLocale\(normalizeLocale\(getConfig\(\)\.locale\), \{ notify: false \}\)/,
    "启动应用持久化 locale；P1 起要为运行中切换保留广播，所以启动路径显式静默"
  );
  const setup = read("../../core/i18n-setup.js");
  assert.match(setup, /installCatalogs\(\{ "zh-CN": zhCN, en \}, "zh-CN"\)/, "启动要一次装齐目录");
});

test("目录内容：zh-CN 与 en 都有 language 文案且占位符一致", () => {
  const zh = readJson("../../../messages/zh-CN.json").settings.interface.language;
  const en = readJson("../../../messages/en.json").settings.interface.language;
  for (const key of ["label", "desc", "saved", "failed"]) {
    assert.ok(zh[key]?.trim(), `zh 缺失 settings.interface.language.${key}`);
    assert.ok(en[key]?.trim(), `en 缺失 settings.interface.language.${key}`);
  }
  assert.match(zh.saved, /\{language\}/);
  assert.match(en.saved, /\{language\}/);
});

test("双端白名单同套取值：core/locale.mjs ↔ internal/app/config.go", () => {
  const locale = read("../../core/locale.mjs");
  assert.match(locale, /id: "zh-CN"/);
  assert.match(locale, /id: "en"/);
  const go = read("../../../../../internal/app/config.go");
  assert.match(go, /"zh-cn":\s+"zh-CN"/);
  assert.match(go, /"en":\s+"en"/);
});
