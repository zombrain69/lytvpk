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
  assert.ok(pushes.length >= 4, `至少四处批量入口要收集原因，实际 ${pushes.length} 处`);
});

// 真机复现：三个文件里有一个被别的进程占用时，「批量设置标签」只报
// 「完成: 成功 2 个, 失败 1 个」——原因只进 console。后端给的是中文可行动提示，界面要带出来。
test("批量设置标签失败时带上第一个失败原因", () => {
  const batchTags = readFileSync(new URL("./batch-tags.js", import.meta.url), "utf8");
  assert.match(
    batchTags,
    /const reason = errors\[0\] \? `：\$\{errors\[0\]\}` : "";/,
    "失败原因要取自真实错误",
  );
  assert.match(
    batchTags,
    /`完成: 成功 \$\{success\} 个, 失败 \$\{fail\} 个\$\{reason\}`/,
    "通知要带上 reason",
  );
});

test("批量隐藏 / 取消隐藏失败时也带上原因", () => {
  assert.match(
    actions,
    /`操作完成: 成功 \$\{successCount\} 个, 失败 \$\{failCount\} 个\$\{reason\}\$\{skippedText\}`/,
  );
});
