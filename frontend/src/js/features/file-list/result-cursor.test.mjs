import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { collectCursorKeys, describeResultCursor, nextResultPath, syncCursorHighlight } from "./result-cursor.mjs";

const paths = ["a.vpk", "b.vpk", "c.vpk"];

test("光标工具是通用的：任意列表都能收集键 / 同步高亮 / 自定义 Enter 提示", () => {
  // 位置说明的"Enter 做什么"由面板决定（详情 / 展开压缩包 / 展开模型）
  assert.equal(describeResultCursor(paths, "b.vpk", "Enter 展开"), "第 2 / 3 个结果（Enter 展开）");
  assert.equal(describeResultCursor(paths, "b.vpk"), "第 2 / 3 个结果（Enter 打开详情）", "默认仍是打开详情");

  // 用一个假容器验证收集与高亮同步（不依赖 DOM 实现）
  const makeRow = (key) => ({
    dataset: { cursorKey: key },
    classes: new Set(),
    scrolled: false,
    classList: {
      toggle(name, on) {
        if (on) this.owner.classes.add(name);
        else this.owner.classes.delete(name);
      },
    },
    scrollIntoView() {
      this.scrolled = true;
    },
  });
  const rows = [makeRow("p1"), makeRow("p2"), makeRow("")];
  rows.forEach((row) => {
    row.classList.owner = row;
  });
  const container = { querySelectorAll: () => rows };

  assert.deepEqual(collectCursorKeys(container, ".row"), ["p1", "p2"], "空键要被过滤掉");
  assert.equal(collectCursorKeys(null, ".row").length, 0);
  assert.equal(collectCursorKeys(container, "").length, 0);

  const active = syncCursorHighlight(container, ".row", "p2");
  assert.equal(active, "p2");
  assert.ok(rows[1].classes.has("is-cursor"), "当前行要打上光标类");
  assert.ok(!rows[0].classes.has("is-cursor"), "其它行不能留光标类");
  assert.ok(rows[1].scrolled, "当前行要滚进可视区");

  // 键不在列表里 → 没有行被点亮
  assert.equal(syncCursorHighlight(container, ".row", "不存在"), "");
  assert.ok(!rows[1].classes.has("is-cursor"));
});

test("nextResultPath：向下 / 向上移动并环绕", () => {
  assert.equal(nextResultPath(paths, "a.vpk", 1), "b.vpk");
  assert.equal(nextResultPath(paths, "c.vpk", 1), "a.vpk", "最后一行再向下应回到第一行");
  assert.equal(nextResultPath(paths, "a.vpk", -1), "c.vpk", "第一行再向上应跳到最后一行");
  assert.equal(nextResultPath(paths, "", 1), "a.vpk", "没有光标时向下从第一条开始");
  assert.equal(nextResultPath(paths, "", -1), "c.vpk", "没有光标时向上从最后一条开始");
  assert.equal(nextResultPath(paths, "不存在的.vpk", 1), "a.vpk", "光标失效时从头开始");
});

test("nextResultPath：空列表与脏数据不炸", () => {
  assert.equal(nextResultPath([], "a.vpk", 1), "");
  assert.equal(nextResultPath(null, "a.vpk", 1), "");
  assert.equal(nextResultPath(["", "a.vpk", null], "", 1), "a.vpk", "空路径会被跳过");
  // delta 为 0 或非数字时按"向下一步"处理，避免出现不动又不报错的状态。
  assert.equal(nextResultPath(paths, "a.vpk", 0), "b.vpk");
  assert.equal(nextResultPath(paths, "a.vpk", "x"), "b.vpk");
});

test("describeResultCursor：位置文案", () => {
  assert.equal(describeResultCursor(paths, "b.vpk"), "第 2 / 3 个结果（Enter 打开详情）");
  assert.equal(describeResultCursor(paths, ""), "");
  assert.equal(describeResultCursor([], "a.vpk"), "");
});

test("渲染层与快捷键都接上了键盘光标", () => {
  const renderSource = readFileSync(new URL("./render.js", import.meta.url), "utf8");
  const runtimeSource = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
  const cssSource = readFileSync(new URL("../../../css/app/reading-comfort.css", import.meta.url), "utf8");

  assert.match(renderSource, /export function applySearchResultCursor/, "渲染层要导出光标应用函数");
  assert.match(renderSource, /applySearchResultCursor\(\);/, "每次重画后要恢复光标");
  assert.match(renderSource, /is-result-cursor/, "要有光标行的 class");
  assert.match(renderSource, /dataset\.baseLabel/, "计数文案要保留基准，便于追加位置");

  assert.match(runtimeSource, /event\.key === "ArrowDown" \|\| event\.key === "ArrowUp"/, "缺少上下键处理");
  assert.match(runtimeSource, /nextResultPath\(/, "上下键要走纯函数");
  assert.match(runtimeSource, /event\.key === "Enter" && document\.activeElement === searchInput/, "缺少 Enter 打开详情");
  assert.match(runtimeSource, /\.detail-btn/, "Enter 应复用详情按钮的点击逻辑");
  assert.match(runtimeSource, /appState\.searchCursorPath = ""/, "Esc 清空时要一起清光标");

  assert.match(cssSource, /\.file-item\.is-result-cursor/, "缺少光标行样式");
});
