import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import test from "node:test";
import { fileURLToPath } from "node:url";

import {
  DEFAULT_LOCALE_ID,
  LANGUAGE_SWITCH_ENABLED,
  SUPPORTED_LOCALES,
  localeLabel,
  normalizeLocale,
} from "./locale.mjs";

test("normalizeLocale：白名单内原样、大小写/分隔符宽松、未知回落 zh-CN", () => {
  assert.equal(normalizeLocale("zh-CN"), "zh-CN");
  assert.equal(normalizeLocale("zh-cn"), "zh-CN");
  assert.equal(normalizeLocale(" en "), "en");
  assert.equal(normalizeLocale("EN"), "en");
  assert.equal(normalizeLocale("en-US"), "en");
  assert.equal(normalizeLocale("en_US"), "en");
  assert.equal(normalizeLocale("zh"), "zh-CN");
  assert.equal(normalizeLocale("zh-TW"), "zh-CN");
  assert.equal(normalizeLocale("ja"), "zh-CN");
  assert.equal(normalizeLocale(""), "zh-CN");
  assert.equal(normalizeLocale(null), "zh-CN");
  assert.equal(normalizeLocale(undefined), "zh-CN");
  assert.equal(DEFAULT_LOCALE_ID, "zh-CN");
});

test("localeLabel：返回语言自称（端名），未知输入归一化后兜底", () => {
  assert.equal(localeLabel("zh-CN"), "简体中文");
  assert.equal(localeLabel("en"), "English");
  assert.equal(localeLabel("EN-us"), "English");
  assert.equal(localeLabel("ja"), "简体中文");
});

test("SUPPORTED_LOCALES 与随包目录一一对应（files == locale ids）", () => {
  for (const locale of SUPPORTED_LOCALES) {
    const path = fileURLToPath(new URL(`../../messages/${locale.id}.json`, import.meta.url));
    assert.ok(existsSync(path), `缺少目录文件 messages/${locale.id}.json`);
  }
  const ids = SUPPORTED_LOCALES.map((locale) => locale.id);
  assert.deepEqual([...ids].sort(), ["en", "zh-CN"]);
});

test("LANGUAGE_SWITCH_ENABLED 是布尔（P0 阶段语义为隐藏，翻转在 P1/P2 收口时进行）", () => {
  assert.equal(typeof LANGUAGE_SWITCH_ENABLED, "boolean");
});
