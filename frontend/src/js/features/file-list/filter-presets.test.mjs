// 筛选预设的静态契约：预设是"标签驱动"的，标签必须由后端在扫描时写出来，
// 否则按钮点下去是 0 结果。这里盯住关键预设（动作 XDR / 音画与场景）的存在与形态。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const here = new URL("./", import.meta.url);
// 分类定义已经搬进 category-tree.mjs（唯一事实源），预设菜单与分类树都读它。
const filtersSource = readFileSync(new URL("category-tree.mjs", here), "utf8");
const rulesSource = readFileSync(new URL("../../../../../internal/ruletable/rules.json", here), "utf8");
const xdrSource = readFileSync(new URL("../../../../../internal/parser/xdr_parser.go", here), "utf8");

function presetBlock(id) {
  const start = filtersSource.indexOf(`id: "${id}"`);
  assert.ok(start >= 0, `预设组 ${id} 应存在`);
  const end = filtersSource.indexOf("\n  },", start);
  assert.ok(end > start, `预设组 ${id} 应有结束标记`);
  return filtersSource.slice(start, end);
}

test("预设里有「动作（XDR）」并挂到 XDR 标签上", () => {
  const block = presetBlock("xdr-animations");
  assert.ok(block.includes('"动作（XDR）"'), "预设名称应为「动作（XDR）」");
  assert.ok(block.includes('allTag: "XDR动画"'), "根级“查看全部”应挂既有的 XDR动画 聚合标签");
  assert.ok(block.includes('"XDR槽位动作"'), "应有槽位动作细分");
  assert.ok(block.includes('"XDR基础包"'), "应有基础包细分");
});

test("XDR 标签确实由解析器写出来（不是只在前端写了名字）", () => {
  assert.ok(xdrSource.includes('add("XDR槽位动作"'), "解析器要写 XDR槽位动作 标签");
  assert.ok(xdrSource.includes('add("XDR基础包"'), "解析器要写 XDR基础包 标签");
  const parserSource = readFileSync(new URL("../../../../../internal/parser/parser.go", here), "utf8");
  assert.ok(parserSource.includes('"XDR动画"'), "根级 XDR动画 聚合标签要由解析器写入");
});

test("「音画与场景」预设的标签都由规则表产出", () => {
  const block = presetBlock("media-and-scene");
  for (const tag of ["音乐", "粒子特效", "贴花", "载具", "音画场景"]) {
    assert.ok(block.includes(`"${tag}"`), `预设应包含 ${tag}`);
  }
  for (const prefix of ["sound/music/", "materials/particle/", "materials/decals/", "models/props_vehicles/"]) {
    assert.ok(rulesSource.includes(prefix), `规则表应包含前缀 ${prefix}`);
  }
  assert.ok(rulesSource.includes('"音画场景"') === false, "聚合标签由 Go 生成，不需要写进规则表");
});
