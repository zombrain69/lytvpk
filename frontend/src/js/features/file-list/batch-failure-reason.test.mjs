import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";
import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";
import { t } from "../../core/i18n-runtime.js";

// 真机联调（用一个外部进程独占锁住某个 VPK）里确认过这条链路：
// 批量禁用只报"1 个文件禁用失败"时，用户不知道是被游戏占用、没权限还是目标同名。
// 后端已经把 os.Rename 的原始错误翻译成中文可行动提示，界面必须把它一起说出来。
// 文案已迁移到目录（mods.ops.*）：断言改为「源码接线 + 目录内容」双查，行为要求原样保留。
const actions = readFileSync(new URL("./actions.js", import.meta.url), "utf8");
const here = path.dirname(fileURLToPath(import.meta.url));
const catalog = JSON.parse(
  readFileSync(path.resolve(here, "../../../messages/zh-CN.json"), "utf8"),
);

test("批量禁用失败时带上第一个失败原因", () => {
  assert.match(actions, /const failures = \[\];/);
  assert.match(
    actions,
    /failures\.push\(String\(error\?\.message \|\| error\)\)/,
    "失败原因要取自真实错误",
  );
  assert.match(
    actions,
    /showNotification\(t\("mods\.ops\.disable\.failed", \{ count: failedCount, reason \}\), "error"\)/,
    "禁用失败的提示要带上 reason",
  );
  assert.match(catalog.mods.ops.disable.failed, /禁用失败\{reason\}$/, "目录模板要保留 reason 位");
});

test("批量启用与「全部禁用」同样带上原因", () => {
  assert.match(
    actions,
    /showNotification\(t\("mods\.ops\.enable\.failed", \{ count: failedCount, reason \}\), "error"\)/,
  );
  assert.match(
    actions,
    /showNotification\(t\("mods\.ops\.disableAll\.failed", \{ count: failedCount, scope: scopeLabel, reason \}\), "error"\)/,
  );
  assert.match(catalog.mods.ops.enable.failed, /启用失败\{reason\}$/);
  assert.match(catalog.mods.ops.disableAll.failed, /禁用失败\{reason\}$/);
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
    /import \{ labelSeparator \} from "\.\.\/\.\.\/core\/format-list\.mjs";/,
    "分隔符要走 locale 感知工具（第 21 项）",
  );
  assert.match(
    batchTags,
    /const reason = errors\[0\] \? `\$\{labelSeparator\(\)\}\$\{errors\[0\]\}` : "";/,
    "失败原因要取自真实错误",
  );
  assert.match(
    batchTags,
    /t\("mods\.ops\.batchTags\.done", \{ success, fail, reason \}\),/,
    "通知要带上 reason",
  );
  assert.match(catalog.mods.ops.batchTags.done, /失败 \{fail\} 个\{reason\}$/);
});

test("批量隐藏 / 取消隐藏失败时也带上原因", () => {
  assert.match(
    actions,
    /t\("mods\.ops\.visibility\.done", \{ success: successCount, fail: failCount, reason, skipped: skippedText \}\),/,
  );
  assert.match(catalog.mods.ops.visibility.done, /\{reason\}\{skipped\}$/);
});

test("失败原因渲染成原句（目录插值实测）", () => {
  installTestCatalog();
  assert.equal(
    t("mods.ops.disable.failed", { count: 2, reason: "：目标同名" }),
    "2 个文件禁用失败：目标同名",
  );
  assert.equal(
    t("mods.ops.visibility.done", {
      success: 1,
      fail: 2,
      reason: "：被占用",
      skipped: "，3 个已处于目标状态未变更",
    }),
    "操作完成: 成功 1 个, 失败 2 个：被占用，3 个已处于目标状态未变更",
  );
});
