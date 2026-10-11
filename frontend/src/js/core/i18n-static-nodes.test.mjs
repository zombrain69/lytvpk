// 静态节点收尾校验（i18n 统一轮次）：
//   1) index.html 里每个 data-i18n* 节点必须同时满足 —— 目录存在该 key，
//      且节点上保留的「回退原文」与目录里的中文**逐字一致**。zh-CN 是当前唯一
//      目录，回退与目录本就应该同文；把这条钉死，防的是迁移时「key 接对了、
//      文案却抄错/随后漂移」这类只有运行起来才看得见的静默问题。
//   2) 纯文本约束：`data-i18n` 只能挂在「内容就是一段文本」的节点上（applyTranslations
//      用 textContent 回填，挂到带子元素的节点上会把子元素抹掉）。这里直接对
//      index.html 强制这条约束，写错立刻红灯。
//   3) applyTranslations 的 DOM 行为单测（title / placeholder / aria-label / alt 五类回填）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { installTestCatalog } from "./i18n-test-catalog.mjs";
import { applyTranslations } from "./i18n-runtime.js";

const coreDir = dirname(fileURLToPath(import.meta.url)); // src/js/core
const srcRoot = dirname(coreDir); // src/js
const htmlPath = join(srcRoot, "..", "..", "index.html");
const catalogPath = join(srcRoot, "..", "messages", "zh-CN.json");

const ATTR_BINDINGS = [
  ["data-i18n-title", "title"],
  ["data-i18n-placeholder", "placeholder"],
  ["data-i18n-aria-label", "aria-label"],
  ["data-i18n-alt", "alt"],
];

function loadCatalogKeys() {
  const catalog = JSON.parse(readFileSync(catalogPath, "utf8"));
  const flat = new Map();
  const walk = (node, prefix) => {
    for (const [name, value] of Object.entries(node)) {
      const key = prefix ? `${prefix}.${name}` : name;
      if (typeof value === "string") {
        flat.set(key, value);
      } else if (value && typeof value === "object") {
        walk(value, key);
      }
    }
  };
  walk(catalog, "");
  return flat;
}

const collapse = (text) => String(text ?? "").replace(/\s+/g, " ").trim();

test("index.html 静态节点：回退原文与 zh-CN 目录逐字一致（防 key 抄错 / 文案漂移）", () => {
  const html = readFileSync(htmlPath, "utf8");
  const catalog = loadCatalogKeys();
  const problems = [];
  let checked = 0;

  // 逐个开始标签扫描；属性区按「带引号的值优先」解析，避免属性值里的 > 截断标签。
  const tagRe = /<([a-zA-Z][a-zA-Z0-9-]*)((?:"[^"]*"|'[^']*'|[^>"'])*)>/g;
  let match;
  while ((match = tagRe.exec(html))) {
    const attrs = match[2];

    const textKey = /\bdata-i18n="([\w.]+)"/.exec(attrs);
    if (textKey) {
      const expected = catalog.get(textKey[1]);
      if (expected === undefined) {
        problems.push(`${textKey[1]}：目录缺少 key`);
      } else {
        const rest = html.slice(tagRe.lastIndex);
        const textRun = /^([^<]*)/.exec(rest)?.[1] ?? "";
        const fallback = collapse(textRun);
        if (!fallback) {
          problems.push(
            `${textKey[1]}：节点不是纯文本（内容为空或以子元素开头），data-i18n 只能挂在纯文本节点上`,
          );
        } else {
          checked += 1;
          if (fallback !== collapse(expected)) {
            problems.push(`${textKey[1]}：回退="${fallback}" 目录="${expected}"`);
          }
        }
      }
    }

    for (const [sourceAttr, targetAttr] of ATTR_BINDINGS) {
      const keyMatch = new RegExp(`\\b${sourceAttr}="([\\w.]+)"`).exec(attrs);
      if (!keyMatch) continue;
      const expected = catalog.get(keyMatch[1]);
      if (expected === undefined) {
        problems.push(`${keyMatch[1]}：目录缺少 key`);
        continue;
      }
      const valueMatch = new RegExp(`\\b${targetAttr}="([^"]*)"`).exec(attrs);
      if (!valueMatch) {
        problems.push(`${keyMatch[1]}：缺少 ${targetAttr} 回退属性`);
        continue;
      }
      checked += 1;
      if (collapse(valueMatch[1]) !== collapse(expected)) {
        problems.push(`${keyMatch[1]}：回退="${valueMatch[1]}" 目录="${expected}"`);
      }
    }
  }

  assert.ok(checked > 0, "没有检查到任何静态节点，检查正则或文件位置");
  assert.deepEqual(problems, [], `index.html 静态节点回退文案校验失败：\n${problems.join("\n")}`);
});

// ── applyTranslations 的 DOM 行为单测（用最小假节点，不引入 DOM 依赖） ──

function createFakeElement(attrs = {}) {
  const store = { ...attrs };
  return {
    textContent: "",
    getAttribute: (name) => (name in store ? store[name] : null),
    setAttribute: (name, value) => {
      store[name] = value;
    },
    dump: () => ({ ...store }),
  };
}

test("applyTranslations：文本与 title / placeholder / aria-label / alt 五类回填全部生效", () => {
  installTestCatalog();
  const textEl = createFakeElement({ "data-i18n": "toolbox.title" });
  const titleEl = createFakeElement({
    "data-i18n-title": "mods.rotation.btn.tooltip",
    title: "Mod随机轮换",
  });
  const placeholderEl = createFakeElement({
    "data-i18n-placeholder": "update.modal.customMirrorPlaceholder",
    placeholder: "输入镜像地址 (例如 https://mirror.example.com/)",
  });
  const ariaEl = createFakeElement({ "data-i18n-aria-label": "toolbox.title" });
  const altEl = createFakeElement({ "data-i18n-alt": "toolbox.title" });

  const applied = applyTranslations({
    querySelectorAll: (selector) =>
      ({
        "[data-i18n]": [textEl],
        "[data-i18n-title]": [titleEl],
        "[data-i18n-placeholder]": [placeholderEl],
        "[data-i18n-aria-label]": [ariaEl],
        "[data-i18n-alt]": [altEl],
      })[selector] ?? [],
  });

  assert.equal(applied, 5);
  assert.equal(textEl.textContent, "工具箱");
  assert.equal(titleEl.getAttribute("title"), "Mod随机轮换");
  assert.equal(
    placeholderEl.getAttribute("placeholder"),
    "输入镜像地址 (例如 https://mirror.example.com/)",
  );
  assert.equal(ariaEl.getAttribute("aria-label"), "工具箱");
  assert.equal(altEl.getAttribute("alt"), "工具箱");
});

test("applyTranslations：缺失 key 回退为 key 本身；无 querySelectorAll 的 root 返回 0", () => {
  installTestCatalog();
  const missingEl = createFakeElement({ "data-i18n": "no.such.key" });
  const applied = applyTranslations({
    querySelectorAll: (selector) => (selector === "[data-i18n]" ? [missingEl] : []),
  });
  assert.equal(applied, 1);
  assert.equal(missingEl.textContent, "no.such.key");
  assert.equal(applyTranslations(null), 0);
  assert.equal(applyTranslations({}), 0);
});
