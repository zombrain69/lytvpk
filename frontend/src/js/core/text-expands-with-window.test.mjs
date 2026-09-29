import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// 「窗口拉宽了，文字还是缩着」的防线：
// 标题/文件名一开始用 max-width: 20ch / 15ch 之类的固定字数上限（还有 24/26/34/28ch），
// 于是模态窗口被拉宽、容器变宽之后，文字仍然停在原地被省略号截断。
// 这里把"不许再出现固定 ch 上限"和"这几处必须随容器伸缩"钉住。

const here = path.dirname(fileURLToPath(import.meta.url));
const cssRoot = path.resolve(here, "../../css");

function listCSSFiles(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) result.push(...listCSSFiles(full));
    else if (entry.name.endsWith(".css")) result.push(full);
  }
  return result;
}

const cssFiles = listCSSFiles(cssRoot);
const read = (name) => readFileSync(path.join(cssRoot, name), "utf8");

test("CSS 里不再有 max-width: Nch 这类固定字数上限", () => {
  const offenders = [];
  for (const file of cssFiles) {
    const source = readFileSync(file, "utf8");
    for (const match of source.matchAll(/max-width:\s*[0-9.]+ch/g)) {
      offenders.push(`${path.relative(cssRoot, file)}: ${match[0]}`);
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `固定 ch 上限会让"窗口拉宽但文字不展开"，请改成 max-width: 100%：\n${offenders.join("\n")}`,
  );
});

test("Mod 标题与文件名改成跟随可用宽度伸缩", () => {
  const global = read("global.css");
  const title = global.match(/\.conflict-vpk-title\s*\{[^}]*\}/s)?.[0] || "";
  const filename = global.match(/\.conflict-vpk-filename\s*\{[^}]*\}/s)?.[0] || "";
  for (const [label, block] of [["标题", title], ["文件名", filename]]) {
    assert.match(block, /min-width:\s*0/, `${label}要允许收缩（min-width: 0）`);
    assert.match(block, /max-width:\s*100%/, `${label}最多占满容器（max-width: 100%）`);
    assert.match(block, /text-overflow:\s*ellipsis/, `${label}放不下时仍然用省略号`);
  }
});

test("分组建议窗口的成员列表不再用固定像素高度卡住", () => {
  const mods = read("app/mods.css");
  const block =
    mods.match(/#mod-group-suggest-modal\s+\.mod-group-suggest-members\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(block, "没找到 #mod-group-suggest-modal .mod-group-suggest-members 规则");
  assert.doesNotMatch(block, /max-height:\s*[0-9]+px/, "固定 px 高度会让窗口拉高后列表不变高");
  assert.match(block, /max-height:\s*none/, "应交给外层滚动，跟随窗口高度");
});

test("策略组成员行在加了勾选框之后仍然让中间那列自己撑开", () => {
  const mods = read("app/mods.css");
  const row = mods.match(/#strategy-group-modal\s+\.mod-member-row\s*\{[^}]*\}/s)?.[0] || "";
  assert.ok(row, "没找到 #strategy-group-modal .mod-member-row 规则");
  assert.match(
    row,
    /grid-template-columns:\s*auto\s+minmax\(0,\s*1fr\)\s+auto/,
    "勾选框 + 成员信息 + 行内按钮三列：中间列必须用 minmax(0,1fr) 才能随窗口展开",
  );
});
