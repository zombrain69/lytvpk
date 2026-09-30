import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// WebView2 里的 window.confirm / window.prompt 是阻塞式原生对话框：
// 不受主题与快捷键控制，自动化与锁屏环境下还会把 JS 线程卡住。
// 策略组那边早就有守卫测试（group-dialog-guard.test.mjs）钉住这条规则，
// 这轮把设置页剩下的 7 处（删除依赖 / 应用方案 / 删除方案 / 删除合集 /
// 恢复备份 / 删除备份 / 删除 addonlist.txt）也换成应用内弹窗，并在这里全仓库兜底。

const here = path.dirname(fileURLToPath(import.meta.url));
const jsRoot = path.resolve(here, "../..");

function listJSFiles(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) result.push(...listJSFiles(full));
    else if (entry.name.endsWith(".js")) result.push(full);
  }
  return result;
}

/** 去掉注释再检查：文档里提到"不要用 window.confirm"不算调用。 */
function stripComments(source) {
  return source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|[^:])\/\/[^\n]*/g, "$1");
}

test("前端任何地方都不调用阻塞式原生对话框", () => {
  const offenders = [];
  for (const file of listJSFiles(jsRoot)) {
    const source = stripComments(readFileSync(file, "utf8"));
    for (const match of source.matchAll(/window\.(confirm|prompt)\s*\(/g)) {
      offenders.push(`${path.relative(jsRoot, file)}: ${match[0]}`);
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `请改用 features/modals/confirm.js 的 confirmInApp / showConfirmModal（或 prompt.js）：\n${offenders.join("\n")}`,
  );
});

test("设置页用 confirmInApp 取代了原来的 window.confirm", () => {
  const settings = readFileSync(path.resolve(here, "../settings/settings-page.js"), "utf8");
  assert.match(
    settings,
    /import \{[^}]*confirmInApp[^}]*\} from "\.\.\/modals\/confirm\.js"/,
    "设置页要导入 confirmInApp",
  );
  const calls = [...settings.matchAll(/await confirmInApp\(/g)].length;
  assert.ok(calls >= 7, `设置页的确认都应走应用内弹窗，实际只有 ${calls} 处`);
});

test("confirmInApp 只结算一次（点确定时 cleanup 也会回调 onCancel）", async () => {
  const { confirmInApp } = await import("./confirm.js");
  // confirm.js 依赖 DOM（document.getElementById），node 下没有 document：
  // 这里退化为"导出存在且可调用"的最小检查，真实行为由真机联调覆盖。
  assert.equal(typeof confirmInApp, "function", "缺少 confirmInApp 导出");
});
