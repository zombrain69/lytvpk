// i18n 运行时（installCatalogs / 多目录）的行为测试：运行时模块不含 JSON 导入，
// 可以直接被 Node 导入（见 i18n-runtime.js 顶部注释）。
import assert from "node:assert/strict";
import test from "node:test";

import {
  availableLocales,
  getLocale,
  installCatalog,
  installCatalogs,
  setLocale,
  t,
} from "./i18n-runtime.js";

const zh = { greeting: { hello: "你好 {name}" }, onlyZh: "只有中文" };
const en = { greeting: { hello: "Hello {name}" } };

test("installCatalogs：一次装多份目录，切换/回退链按运行时定义工作", () => {
  assert.equal(installCatalogs({ "zh-CN": zh, en }, "zh-CN"), true);
  assert.deepEqual(availableLocales(), ["en", "zh-CN"]);

  assert.equal(t("greeting.hello", { name: "Lyt" }), "你好 Lyt");
  assert.equal(getLocale(), "zh-CN");

  // 切 English：有翻译用翻译；缺翻回退 zh-CN（这就是 P0 的「en 占位」策略）。
  setLocale("en");
  assert.equal(getLocale(), "en");
  assert.equal(t("greeting.hello", { name: "Lyt" }), "Hello Lyt");
  assert.equal(t("onlyZh"), "只有中文");

  // 两边都没有：返回 key 本身，界面不会空白。
  assert.equal(t("missing.key"), "missing.key");

  setLocale("zh-CN");
});

test("installCatalogs：非对象目录被忽略；缺省 locale 回落默认语言", () => {
  assert.equal(installCatalogs({ en: "not-a-catalog", "zh-CN": zh }), true);
  assert.deepEqual(availableLocales(), ["zh-CN"]);

  // 完全不传目录也不应抛错。
  assert.equal(installCatalogs(), true);
  assert.deepEqual(availableLocales(), []);
  assert.equal(t("anything"), "anything");
});

test("installCatalog 兼容单目录调用（旧入口），语义 = 只装一份", () => {
  assert.equal(installCatalog(zh, "zh-CN"), true);
  assert.deepEqual(availableLocales(), ["zh-CN"]);
  assert.equal(t("greeting.hello", { name: "Lyt" }), "你好 Lyt");
  assert.equal(installCatalog(null), false);
});

test("setLocale：语言变化派发 app:locale-change；同语言与 notify:false 不派发", () => {
  const previousDocument = globalThis.document;
  const dispatched = [];
  globalThis.document = {
    documentElement: { setAttribute() {} },
    querySelectorAll: () => [],
    dispatchEvent: (event) => {
      dispatched.push(event);
      return true;
    },
  };
  try {
    installCatalogs({ "zh-CN": zh, en }, "zh-CN");

    setLocale("zh-CN"); // 同语言：不派发
    assert.equal(dispatched.length, 0);

    setLocale("en"); // 变化：派发一次，detail 带新旧语言
    assert.equal(dispatched.length, 1);
    assert.equal(dispatched[0].type, "app:locale-change");
    assert.equal(dispatched[0].detail.locale, "en");
    assert.equal(dispatched[0].detail.previous, "zh-CN");

    setLocale("en"); // 再设同一语言：不派发
    assert.equal(dispatched.length, 1);

    setLocale("zh-CN", { notify: false }); // 显式静默（启动应用路径）：不派发
    assert.equal(dispatched.length, 1);
  } finally {
    globalThis.document = previousDocument;
    installCatalogs({ "zh-CN": zh }, "zh-CN");
  }
});
