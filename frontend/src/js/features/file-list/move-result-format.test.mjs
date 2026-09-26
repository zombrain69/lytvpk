import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { formatMoveFailures, formatMoveSummary } from "./move-result-format.mjs";

test("formatMoveFailures：逐条列出失败原因，超出上限时说明还有多少条", () => {
  assert.equal(formatMoveFailures({ failCount: 0, errors: [] }), "");
  assert.equal(formatMoveFailures(null), "");

  assert.equal(
    formatMoveFailures({ failCount: 1, errors: ["移动 a.vpk 失败: 目标已存在"] }),
    // 解释层换成"人话 + 下一步"，但文件名必须留下（否则批量操作时不知道是谁失败）
    "1 个文件操作失败：a.vpk：目标位置已经有同名文件（选择「替换」或「跳过」，或先给其中一个改个名字。）",
  );

  // 认不出的原因保留原文（对齐上游"按类型回退"）
  const many = formatMoveFailures({
    failCount: 5,
    errors: ["e1", "e2", "e3", "e4", "e5"],
  });
  assert.match(many, /^5 个文件操作失败：e1；e2；e3/);
  assert.match(many, /另有 2 条，详见控制台日志$/);

  // 能认出的原因：文件名 + 人话 + 下一步
  const explained = formatMoveFailures({
    failCount: 2,
    errors: ["移动 b.vpk 失败: Access is denied", "移动 c.vpk 失败: The system cannot find the path specified"],
  });
  assert.match(explained, /b\.vpk：系统拒绝了这次读写（关掉正在占用该文件的程序/);
  assert.match(explained, /c\.vpk：目标路径当前不可用（确认目标目录还在/);

  // 只有计数没有详情时也不能显示成"失败：undefined"。
  assert.equal(formatMoveFailures({ failCount: 2, errors: [] }), "2 个文件操作失败");
  // 计数缺失但有原因时按原因条数兜底。
  assert.equal(formatMoveFailures({ errors: ["原因"] }), "1 个文件操作失败：原因");
});

test("formatMoveSummary：成功 / 跳过 / 取消的汇总", () => {
  assert.equal(formatMoveSummary({ successCount: 3, skippedCount: 0 }), "成功 3 个");
  assert.equal(
    formatMoveSummary({ successCount: 3, skippedCount: 2, cancelled: true }),
    "成功 3 个 · 跳过 2 个冲突文件 · 后续已取消",
  );
  assert.equal(formatMoveSummary({}), "");
});

test("列表页移动失败时用的是逐条文案，而不是只看第一条", () => {
  const source = readFileSync(new URL("./actions.js", import.meta.url), "utf8");
  assert.match(source, /formatMoveFailures\(result\)/, "移动失败要用逐条文案");
  assert.ok(
    !/result\.failCount \} 个文件移动失败: \$\{result\.errors\[0\]\}/.test(source),
    "不应再只显示 errors[0]",
  );
});
