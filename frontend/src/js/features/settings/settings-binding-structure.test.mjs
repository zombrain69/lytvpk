import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// 真机回归（2026-09-30）：
// 「设置 → 工坊设置」里的「下载失败后自动重下一次」「剪贴板工坊链接自动识别」
// 以及「抓取工坊官方标签与统计」三个控件点了完全没反应（勾选状态会变，但配置不写、
// 也没有任何通知）。根因是 bindSettingsPage 里「立即触发检测」的绑定头被写在了这三个
// 绑定的前面、又没紧跟自己的函数体，于是这三个 addEventListener 被解析成了那个
// 回调的**函数体** —— 只有点「立即触发检测」时才会被"注册"，等于永远不生效。
//
// 护栏：写成 `xxx?.addEventListener(...)` 的绑定头，下一行必须就是它自己的函数体；
// 被它挡在后面的绑定，必须在它之前、处于同一层。

const here = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(here, "settings-page.js");
const lines = readFileSync(sourcePath, "utf8").replace(/\r\n/g, "\n").split("\n");

function bindingLineIndex(id) {
  return lines.findIndex((line) => line.includes(`"${id}")?.addEventListener`));
}

test("设置页的绑定头不会被另一个回调吞掉函数体", () => {
  const manualIndex = bindingLineIndex("settings-manual-check-btn");
  assert.ok(manualIndex >= 0, "找不到「立即触发检测」的绑定");

  const nextMeaningful = lines
    .slice(manualIndex + 1)
    .find((line) => line.trim() && !line.trim().startsWith("//"));
  assert.match(
    nextMeaningful || "",
    /const btn = document\.getElementById\("settings-manual-check-btn"\)/,
    "「立即触发检测」的绑定头必须紧跟它自己的函数体；否则写在中间的设置绑定会被解析成它的回调体，永远不会注册",
  );

  for (const id of [
    "settings-auto-redownload",
    "settings-auto-detect-workshop-link",
    "settings-workshop-enrich",
  ]) {
    const index = bindingLineIndex(id);
    assert.ok(index >= 0, `找不到 ${id} 的绑定`);
    assert.ok(
      index < manualIndex,
      `${id} 的绑定必须写在「立即触发检测」绑定之前（同一层），否则会被吞进它的回调里`,
    );
  }
});
