import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 真机联调（用一个外部进程独占锁住某个 VPK）里确认过这条链路：
// 批量禁用只报"1 个文件禁用失败"时，用户不知道是被游戏占用、没权限还是目标同名。
// 后端已经把 os.Rename 的原始错误翻译成中文可行动提示，界面必须把它一起说出来。
const actions = readFileSync(new URL("./actions.js", import.meta.url), "utf8");

test("批量禁用失败时带上第一个失败原因", () => {
  assert.match(actions, /const failures = \[\];/);
  assert.match(
    actions,
    /failures\.push\(String\(error\?\.message \|\| error\)\)/,
    "失败原因要取自真实错误",
  );
  assert.match(
    actions,
    /showNotification\(`\$\{failedCount\} 个文件禁用失败\$\{reason\}`, "error"\)/,
    "禁用失败的提示要带上 reason",
  );
});

test("批量启用与「全部禁用」同样带上原因", () => {
  assert.match(
    actions,
    /showNotification\(`\$\{failedCount\} 个文件启用失败\$\{reason\}`, "error"\)/,
  );
  assert.match(
    actions,
    /showNotification\(`\$\{failedCount\} 个\$\{scopeLabel\}禁用失败\$\{reason\}`, "error"\)/,
  );
});

test("每个批量入口都各自收集失败原因", () => {
  const pushes = actions.match(/failures\.push\(/g) || [];
  assert.ok(pushes.length >= 3, `至少三处批量入口要收集原因，实际 ${pushes.length} 处`);
});
