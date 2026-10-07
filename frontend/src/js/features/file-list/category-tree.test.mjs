import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  PRESET_GROUPS,
  buildCategoryTree,
  buildOtherTagsBranch,
  buildPresetTree,
  countCategoryNode,
  flattenCategoryTree,
  nodeIsSelected,
  nodePaths,
  selectionForNode,
  toggleValues,
} from "./category-tree.mjs";

function file(path, tags = [], extra = {}) {
  return { path, secondaryTags: tags, location: "root", gameStateKnown: true, gameEnabled: true, ...extra };
}

const files = [
  file("a.vpk", ["模型", "XDR动画", "XDR槽位动作", "Zoey"]),
  file("b.vpk", ["模型", "XDR动画", "XDR基础包"]),
  file("c.vpk", ["音乐", "声音"]),
  file("d.vpk", ["贴图"], { location: "disabled", gameStateKnown: false, gameEnabled: false }),
  file("e.vpk", ["贴图", "某个没进树的标签"], { gameStateKnown: true, gameEnabled: false }),
];

const TREE = buildCategoryTree({ allTags: files.flatMap((item) => item.secondaryTags) });

function findNode(id, nodes = TREE) {
  for (const node of nodes) {
    if (node.id === id) return node;
    const hit = findNode(id, node.children || []);
    if (hit) return hit;
  }
  return null;
}

test("数量按标签统计，父节点是子节点并集（不重复计数）", () => {
  assert.equal(countCategoryNode(findNode("all"), files), 5);
  assert.equal(countCategoryNode(findNode("tag:XDR槽位动作"), files), 1, "槽位动作只有 a.vpk");
  assert.equal(countCategoryNode(findNode("tag:音乐"), files), 1);
  // 组节点（预设派生）= 子节点并集：XDR 组 = 槽位动作 + 基础包 = 2
  assert.equal(countCategoryNode(findNode("preset:xdr-animations"), files), 2);
});

test("状态节点走已有的位置/游戏内口径", () => {
  assert.equal(countCategoryNode(findNode("status-enabled"), files), 4);
  assert.equal(countCategoryNode(findNode("status-disabled"), files), 1);
  assert.equal(countCategoryNode(findNode("status-game-on"), files), 3);
  assert.equal(countCategoryNode(findNode("status-game-off"), files), 1);
  assert.equal(countCategoryNode(findNode("status-game-unknown"), files), 1);
});

test("多标签节点按「任一命中」统计（与筛选默认的 any 一致）", () => {
  const survivors = findNode("preset:survivors:0"); // 一代幸存者
  assert.ok(survivors, "一代幸存者节点应该存在");
  assert.equal(countCategoryNode(survivors, files), 1, "Zoey 属于一代幸存者");
});

test("选中节点翻译成 filters 能直接用的筛选描述", () => {
  assert.deepEqual(selectionForNode(findNode("all")), { tags: [], locations: [], gameStates: [] });
  assert.deepEqual(selectionForNode(findNode("tag:XDR槽位动作")), {
    tags: ["XDR槽位动作"],
    locations: [],
    gameStates: [],
  });
  assert.deepEqual(selectionForNode(findNode("status-disabled")), {
    tags: [],
    locations: ["disabled"],
    gameStates: [],
  });
  assert.deepEqual(selectionForNode(findNode("status-game-off")), {
    tags: [],
    locations: [],
    gameStates: ["game-disabled"],
  });
  // 组节点点一下 = 选它下面所有具体标签（配合"任一命中"就是"看这一类"）；
  // 「全部」按钮才是聚合标签那条路径。
  assert.deepEqual(
    [...selectionForNode(findNode("preset:xdr-animations")).tags].sort(),
    ["XDR基础包", "XDR槽位动作"].sort(),
  );
  assert.equal(selectionForNode(findNode("status")), null, "纯分组节点（没有自己的标签）不可直接点");
});

// 「内容预设」菜单的全部能力都必须还在：聚合标签（查看全部）、自定义显示名、逐项勾选。
test("预设树保留「查看全部」聚合标签与自定义显示名（不丢功能）", () => {
  const tree = buildPresetTree(PRESET_GROUPS);
  assert.equal(tree.length, PRESET_GROUPS.length, "每个预设组都要出现在树里");

  const firearms = tree.find((node) => node.id === "preset:firearms");
  assert.ok(firearms, "枪械组应该在");
  assert.equal(firearms.allTag, "所有枪械", "组级「查看全部」仍挂聚合标签");

  const pistol = firearms.children.find((node) => node.label === "手枪");
  assert.ok(pistol, "手枪子组应该在");
  assert.equal(pistol.allTag, "手枪", "子组级「查看全部」仍挂聚合标签");
  assert.ok(pistol.tags.includes("马格南"), "具体标签仍是叶子");

  const survivors = tree.find((node) => node.id === "preset:survivors");
  const l4d1 = survivors.children.find((node) => node.label === "一代幸存者");
  const bill = l4d1.children.find((node) => node.tag === "Bill");
  assert.equal(bill.label, "比尔 · Bill", "自定义显示名要保留");
});

// 「不漏包含」：扫描到但没进任何分类的标签，必须有兜底分支，且不与已收录的重复。
test("其它标签兜底分支：扫描到的标签一个都不能漏", () => {
  const tags = files.flatMap((item) => item.secondaryTags);
  const tree = buildCategoryTree({ allTags: tags });
  const other = tree.find((node) => node.id === "other-tags");
  assert.ok(other, "存在未收录标签时应出现兜底分支");
  const otherTags = other.children.map((node) => node.tag);
  assert.ok(otherTags.includes("某个没进树的标签"), "未收录标签要出现在兜底分支");

  const known = buildCategoryTree({ allTags: [] });
  const knownTags = new Set();
  const walk = (nodes) => nodes.forEach((node) => { if (node.tag) knownTags.add(node.tag); if (node.allTag) knownTags.add(node.allTag); walk(node.children || []); });
  walk(known);
  for (const tag of otherTags) {
    assert.ok(!knownTags.has(tag), `兜底分支不该重复已收录标签 ${tag}`);
  }
  assert.equal(buildOtherTagsBranch([], new Set()), null, "没有剩余标签时不产生空分支");
});

test("折叠与搜索：搜索时自动展开命中的分支", () => {
  const collapsed = flattenCategoryTree(TREE, { expanded: new Set() });
  assert.ok(collapsed.some((row) => row.node.id === "preset:xdr-animations"), "根级分支应该可见");
  assert.ok(!collapsed.some((row) => row.node.id === "tag:XDR槽位动作"), "未展开时叶子不出现");

  const expanded = flattenCategoryTree(TREE, { expanded: new Set(["preset:xdr-animations", "preset:xdr-animations:0"]) });
  assert.ok(expanded.some((row) => row.node.id === "tag:XDR槽位动作"), "展开后叶子可见");

  const searched = flattenCategoryTree(TREE, { expanded: new Set(), query: "槽位" });
  assert.ok(searched.some((row) => row.node.id === "tag:XDR槽位动作"), "搜索命中要自动展开");
  assert.ok(!searched.some((row) => row.node.id === "tag:音乐"), "不相关的分支被过滤掉");
});

test("nodePaths 对父节点取并集，不重复", () => {
  const paths = nodePaths(findNode("preset:xdr-animations"), files);
  assert.deepEqual([...paths].sort(), ["a.vpk", "b.vpk"]);
});

test("点一下加上、再点取消：与「内容预设」一致的多选语义", () => {
  let selected = toggleValues([], ["XDR槽位动作"]);
  assert.deepEqual(selected, ["XDR槽位动作"], "第一次点是加上");
  assert.equal(nodeIsSelected(findNode("tag:XDR槽位动作"), { tags: selected }), true);

  selected = toggleValues(selected, ["XDR槽位动作"]);
  assert.deepEqual(selected, [], "再点一次是取消");
  assert.equal(nodeIsSelected(findNode("tag:XDR槽位动作"), { tags: selected }), false);

  const multi = toggleValues(toggleValues([], ["XDR槽位动作"]), ["音乐"]);
  assert.equal(nodeIsSelected(findNode("tag:音乐"), { tags: multi }), true);
  assert.equal(nodeIsSelected(findNode("tag:XDR槽位动作"), { tags: multi }), true);

  // 「查看全部」选的是聚合标签：组/子组也要高亮。
  assert.equal(nodeIsSelected(findNode("preset:xdr-animations:1"), { tags: ["XDR基础包"] }), true);

  assert.equal(nodeIsSelected(findNode("status-game-off"), { gameStates: ["game-disabled"] }), true);
  assert.equal(nodeIsSelected(findNode("status-game-off"), { gameStates: ["game-enabled"] }), false);
  assert.equal(nodeIsSelected(findNode("all"), {}), true);
  assert.equal(nodeIsSelected(findNode("all"), { tags: ["音乐"] }), false);
});

// 真实事故回归守卫：侧边栏与列表曾经靠 index.html 手工嵌套，
// 层级写错后被浏览器解析到 #main-screen 顶层，整页被挤到下半屏、上半屏一片空白。
// 现在必须由 category-tree.js 在运行时把它们包进容器所在的父节点里。
test("侧边栏包裹层由运行时创建，且插在列表原来的父节点里", () => {
  const html = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");
  assert.ok(!/<div class="mods-workspace"/.test(html), "index.html 里不该再手写 mods-workspace 包裹层");

  const source = readFileSync(new URL("./category-tree.js", import.meta.url), "utf8");
  assert.ok(source.includes("function ensureWorkspaceWrapper()"), "应有运行时包裹函数");
  assert.ok(source.includes("ensureWorkspaceWrapper();"), "初始化时必须调用它");
  const start = source.indexOf("function ensureWorkspaceWrapper()");
  const body = source.slice(start, source.indexOf("\n}", start));
  assert.ok(body.includes("container.parentElement"), "包裹层要插在列表原来的父节点里（而不是提到顶层）");
  assert.ok(body.includes('wrapper.append(aside, container)'), "侧边栏与列表必须包进同一个 flex 行");
});
