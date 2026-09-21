import assert from "node:assert/strict";
import test from "node:test";

import { createI18n, DEFAULT_LOCALE, interpolate } from "./i18n.mjs";

const catalogs = {
  "zh-CN": {
    toolbox: {
      title: "工具箱",
      card: { conflict: { title: "Mod 冲突检测" } },
      summary: "共 {count} 个工具",
    },
  },
  en: {
    toolbox: { title: "Toolbox" },
  },
};

test("t resolves nested keys from the active locale", () => {
  const i18n = createI18n({ catalogs, locale: "zh-CN" });
  assert.equal(i18n.t("toolbox.title"), "工具箱");
  assert.equal(i18n.t("toolbox.card.conflict.title"), "Mod 冲突检测");
});

test("t falls back to the default locale and then to the key itself", () => {
  const i18n = createI18n({ catalogs, locale: "en" });
  assert.equal(i18n.t("toolbox.title"), "Toolbox");
  // en 没有这条：回退到 zh-CN 默认语言。
  assert.equal(i18n.t("toolbox.card.conflict.title"), "Mod 冲突检测");
  // 两边都没有：返回 key，避免界面空白。
  assert.equal(i18n.t("toolbox.missing.key"), "toolbox.missing.key");
  assert.equal(i18n.t(""), "");
});

test("t interpolates params and keeps unknown placeholders", () => {
  const i18n = createI18n({ catalogs, locale: "zh-CN" });
  assert.equal(i18n.t("toolbox.summary", { count: 7 }), "共 7 个工具");
  assert.equal(i18n.t("toolbox.summary"), "共 {count} 个工具");
  assert.equal(interpolate("a {x} b {y}", { x: 1 }), "a 1 b {y}");
  assert.equal(interpolate("a {x}", { x: null }), "a ");
});

test("setLocale switches catalogs and reports availability", () => {
  const i18n = createI18n({ catalogs, locale: "zh-CN" });
  assert.deepEqual(i18n.availableLocales(), ["en", "zh-CN"]);
  assert.equal(i18n.getLocale(), "zh-CN");
  i18n.setLocale("en");
  assert.equal(i18n.getLocale(), "en");
  assert.equal(i18n.t("toolbox.title"), "Toolbox");
  assert.equal(i18n.hasLocale("ja"), false);
  assert.equal(i18n.hasKey("toolbox.card.conflict.title"), true);
  assert.equal(i18n.hasKey("toolbox.nope"), false);
});

test("createI18n works without catalogs", () => {
  const i18n = createI18n();
  assert.equal(i18n.getLocale(), DEFAULT_LOCALE);
  assert.equal(i18n.t("anything"), "anything");
  assert.deepEqual(i18n.availableLocales(), []);
});
