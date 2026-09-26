import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// 「文字大小」档位是靠根字号（html { font-size: N% }）生效的：
// 只有用 rem / em / 变量的字号才会跟着变，硬编码 px 不会。
// 这条测试就是防线：新增硬编码 px 字号会被拦下来（titlebar 例外见下）。

const here = path.dirname(fileURLToPath(import.meta.url));
const cssRoot = path.resolve(here, "../../css");

// 标题栏是窗口外观（等价于操作系统标题栏），刻意不跟随内容字号 —— 见 titlebar.css 里的注释。
const CHROME_FILES = new Set(["titlebar.css"]);

function listCSSFiles(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) result.push(...listCSSFiles(full));
    else if (entry.name.endsWith(".css")) result.push(full);
  }
  return result;
}

test("CSS 里不再有硬编码 px 字号（titlebar 除外）", () => {
  const offenders = [];
  for (const file of listCSSFiles(cssRoot)) {
    if (CHROME_FILES.has(path.basename(file))) continue;
    const source = readFileSync(file, "utf8");
    for (const match of source.matchAll(/font-size:\s*[0-9.]+px/g)) {
      offenders.push(`${path.relative(cssRoot, file)}: ${match[0]}`);
    }
  }
  assert.deepEqual(offenders, [], `这些字号不会跟随「文字大小」设置：\n${offenders.join("\n")}`);
});

test("内容区的关键字号确实用了 rem", () => {
  const mods = readFileSync(path.join(cssRoot, "app/mods.css"), "utf8");
  assert.match(mods, /\.card-title\s*\{[^}]*font-size:\s*0\.875rem/, "卡片标题应跟随文字档位");
  assert.match(mods, /\.card-filename\s*\{[^}]*font-size:\s*0\.75rem/, "卡片文件名应跟随文字档位");
  const downloads = readFileSync(path.join(cssRoot, "app/downloads.css"), "utf8");
  assert.match(downloads, /font-size:\s*0\.75rem/, "下载面板应跟随文字档位");
});
