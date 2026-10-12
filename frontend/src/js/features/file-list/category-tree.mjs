// 分类树：把原来的"预设标签组"升级成带层级、计数与搜索的导航树。
//
// 设计约定（与既有功能不重复）：
//   * 每个内容节点最终落到**已有的二级标签**上（聚合标签由后端在扫描时写出），
//     不新增一套过滤引擎；状态节点落到已有的"位置 / 游戏内状态"筛选上。
//   * 树只负责"选什么"，具体怎么筛仍然由 filters.js 处理（见 applyCategoryFilterSelection）。

import { t } from "../../core/i18n-runtime.js";
import { createLocaleCollator } from "../../core/locale-collator.mjs";

// 分类定义（唯一事实源）：筛选预设菜单与左侧分类树都读这一份。
// 文案走目录（mods.category.*）：静态表只存 key（labelKey / descriptionKey / tagLabelKeys），
// 渲染时经 nodeLabel / nodeDescription / presetTagLabel 取当前语言文案
// —— 模块求值期不调 t()（CATEGORY_TREE 可以在模块级安全构建）。
export const PRESET_GROUPS = [
  {
    id: "firearms",
    labelKey: "mods.category.preset.firearms.label",
    allTag: "所有枪械",
    descriptionKey: "mods.category.preset.firearms.desc",
    groups: [
      { labelKey: "mods.category.preset.firearms.pistol", allTag: "手枪", tags: ["小手枪", "马格南"] },
      { labelKey: "mods.category.preset.firearms.rifle", allTag: "步枪", tags: ["AK47", "M16", "三连发", "sg552"] },
      { labelKey: "mods.category.preset.firearms.smg", allTag: "冲锋枪", tags: ["乌兹", "消音", "MP5"] },
      { labelKey: "mods.category.preset.firearms.sniper", allTag: "狙击枪", tags: ["大狙", "猎枪", "军狙", "鸟狙"] },
      { labelKey: "mods.category.preset.firearms.shotgun", allTag: "霰弹枪", tags: ["木喷", "一代连喷", "铁喷", "二代连喷"] },
      { labelKey: "mods.category.preset.firearms.heavy", tags: ["M60", "榴弹发射器"] },
      // 固定机枪按本体模型分代：一代 = L4D1 Minigun（w_minigun），二代 = L4D2 Heavy MG（50cal）；
      // allTag 用聚合标签，点父节点两代都能看到。
      { labelKey: "mods.category.preset.firearms.minigun", allTag: "固定机关枪", tags: ["一代固定机枪", "二代固定机枪"] },
    ],
  },
  {
    id: "official-melee",
    labelKey: "mods.category.preset.officialMelee.label",
    allTag: "所有官方近战",
    descriptionKey: "mods.category.preset.officialMelee.desc",
    groups: [
      { labelKey: "mods.category.preset.officialMelee.blunt", tags: ["棒球棍", "板球拍", "吉他", "平底锅", "高尔夫球杆", "警棍"] },
      { labelKey: "mods.category.preset.officialMelee.sharp", tags: ["消防斧", "砍刀", "武士刀", "撬棍"] },
      { labelKey: "mods.category.preset.officialMelee.tools", tags: ["电锯", "草叉", "铁铲"] },
      { labelKey: "mods.category.preset.officialMelee.hidden", tags: ["匕首", "防爆盾"] },
    ],
  },
  {
    id: "throwables",
    labelKey: "mods.category.preset.throwables.label",
    allTag: "所有投掷物品",
    groups: [
      { labelKey: "mods.category.preset.throwables.all", allTag: "投掷物", tags: ["土制炸弹", "燃烧瓶", "胆汁"] },
    ],
  },
  {
    id: "medical-items",
    labelKey: "mods.category.preset.medicalItems.label",
    allTag: "所有医疗物品",
    groups: [
      { labelKey: "mods.category.preset.medicalItems.all", allTag: "医疗物品", tags: ["医疗包", "电击器", "止痛药", "肾上腺"] },
    ],
  },
  {
    id: "supply-boxes",
    labelKey: "mods.category.preset.supplyBoxes.label",
    allTag: "盒子",
    descriptionKey: "mods.category.preset.supplyBoxes.desc",
    groups: [
      { labelKey: "mods.category.preset.supplyBoxes.ammo", allTag: "弹药堆", tags: ["一代子弹堆", "二代子弹堆"] },
      { labelKey: "mods.category.preset.supplyBoxes.incendiary", allTag: "燃烧弹", tags: ["燃烧弹盒"] },
      { labelKey: "mods.category.preset.supplyBoxes.explosive", allTag: "高爆弹", tags: ["高爆弹盒"] },
      { labelKey: "mods.category.preset.supplyBoxes.laser", allTag: "镭射", tags: ["激光瞄准盒"] },
    ],
  },
  {
    id: "survivors",
    labelKey: "mods.category.preset.survivors.label",
    allTag: "幸存者",
    descriptionKey: "mods.category.preset.survivors.desc",
    groups: [
      {
        labelKey: "mods.category.preset.survivors.l4d1",
        tags: ["Bill", "Zoey", "Louis", "Francis"],
        tagLabelKeys: {
          Bill: "mods.category.tagLabel.bill",
          Zoey: "mods.category.tagLabel.zoey",
          Louis: "mods.category.tagLabel.louis",
          Francis: "mods.category.tagLabel.francis",
        },
      },
      {
        labelKey: "mods.category.preset.survivors.l4d2",
        tags: ["Nick", "Rochelle", "Coach", "Ellis"],
        tagLabelKeys: {
          Nick: "mods.category.tagLabel.nick",
          Rochelle: "mods.category.tagLabel.rochelle",
          Coach: "mods.category.tagLabel.coach",
          Ellis: "mods.category.tagLabel.ellis",
        },
      },
    ],
  },
  {
    id: "special-infected",
    labelKey: "mods.category.preset.specialInfected.label",
    allTag: "特殊感染者",
    descriptionKey: "mods.category.preset.specialInfected.desc",
    groups: [
      {
        labelKey: "mods.category.preset.specialInfected.control",
        tags: ["smoker", "hunter", "jockey"],
        tagLabelKeys: {
          smoker: "mods.category.tagLabel.smoker",
          hunter: "mods.category.tagLabel.hunter",
          jockey: "mods.category.tagLabel.jockey",
        },
      },
      {
        labelKey: "mods.category.preset.specialInfected.ground",
        tags: ["boomer", "spitter", "charger"],
        tagLabelKeys: {
          boomer: "mods.category.tagLabel.boomer",
          spitter: "mods.category.tagLabel.spitter",
          charger: "mods.category.tagLabel.charger",
        },
      },
      {
        labelKey: "mods.category.preset.specialInfected.highRisk",
        tags: ["tank", "witch"],
        tagLabelKeys: {
          tank: "mods.category.tagLabel.tank",
          witch: "mods.category.tagLabel.witch",
        },
      },
    ],
  },
  {
    id: "common-infected",
    labelKey: "mods.category.preset.commonInfected.label",
    allTag: "普通感染者",
    descriptionKey: "mods.category.preset.commonInfected.desc",
    groups: [
      {
        labelKey: "mods.category.preset.commonInfected.kinds",
        tags: ["common", "uncommon_infected"],
        tagLabelKeys: {
          common: "mods.category.tagLabel.common",
          uncommon_infected: "mods.category.tagLabel.uncommon",
        },
      },
    ],
  },
  {
    id: "interface-and-audio",
    labelKey: "mods.category.preset.interfaceAndAudio.label",
    descriptionKey: "mods.category.preset.interfaceAndAudio.desc",
    groups: [
      {
        labelKey: "mods.category.preset.interfaceAndAudio.ui",
        allTag: "UI",
        tags: ["主菜单", "HUD", "准星", "血条", "伤害指示器", "人物语音表"],
      },
      {
        labelKey: "mods.category.preset.interfaceAndAudio.audio",
        allTag: "声音",
        tags: ["语音包", "人物语音表", "尸潮", "警报", "唱片机"],
      },
      {
        labelKey: "mods.category.preset.interfaceAndAudio.assetTypes",
        tags: ["模型", "贴图", "脚本"],
      },
    ],
  },
  {
    id: "world-and-props",
    labelKey: "mods.category.preset.worldAndProps.label",
    descriptionKey: "mods.category.preset.worldAndProps.desc",
    groups: [
      { labelKey: "mods.category.preset.worldAndProps.levels", tags: ["天空", "过场画面", "载入画面"] },
      { labelKey: "mods.category.preset.worldAndProps.dynamic", tags: ["手电筒", "梯子", "动态箭头", "警报", "尸潮", "唱片机"] },
      { labelKey: "mods.category.preset.worldAndProps.containers", tags: ["汽油桶", "煤气罐", "氧气罐", "烟花盒"] },
      {
        labelKey: "mods.category.preset.worldAndProps.props",
        tags: ["侏儒", "直升机", "海报", "船", "售货机", "电视", "屏幕", "货车", "面包车", "雕像"],
      },
    ],
  },
  {
    id: "map-game-modes",
    labelKey: "mods.category.preset.mapGameModes.label",
    descriptionKey: "mods.category.preset.mapGameModes.desc",
    groups: [
      {
        labelKey: "mods.category.preset.mapGameModes.modes",
        tags: ["战役模式", "对抗模式", "生存模式", "清道夫模式", "写实模式", "突变模式"],
      },
    ],
  },
  {
    id: "xdr-animations",
    labelKey: "mods.category.preset.xdrAnimations.label",
    allTag: "XDR动画",
    descriptionKey: "mods.category.preset.xdrAnimations.desc",
    groups: [
      {
        labelKey: "mods.category.preset.xdrAnimations.slots",
        allTag: "XDR槽位动作",
        tags: ["XDR槽位动作"],
      },
      {
        labelKey: "mods.category.preset.xdrAnimations.base",
        allTag: "XDR基础包",
        tags: ["XDR基础包"],
      },
    ],
  },
  {
    id: "media-and-scene",
    labelKey: "mods.category.preset.mediaAndScene.label",
    allTag: "音画场景",
    descriptionKey: "mods.category.preset.mediaAndScene.desc",
    groups: [
      { labelKey: "mods.category.preset.mediaAndScene.music", allTag: "音乐", tags: ["音乐"] },
      { labelKey: "mods.category.preset.mediaAndScene.particles", allTag: "粒子特效", tags: ["粒子特效"] },
      { labelKey: "mods.category.preset.mediaAndScene.decals", allTag: "贴花", tags: ["贴花"] },
      { labelKey: "mods.category.preset.mediaAndScene.vehicles", allTag: "载具", tags: ["载具"] },
    ],
  },
];

// 状态分支：位置 / 游戏内开关也能一键筛（与工具栏那两个下拉共用同一份筛选状态）。
export const STATUS_BRANCH = {
  id: "status",
  labelKey: "mods.category.status.label",
  children: [
    { id: "status-enabled", labelKey: "mods.category.status.enabled", locations: ["root", "workshop"] },
    { id: "status-disabled", labelKey: "mods.category.status.disabled", locations: ["disabled"] },
    { id: "status-game-on", labelKey: "mods.category.status.gameOn", gameStates: ["game-enabled"] },
    { id: "status-game-off", labelKey: "mods.category.status.gameOff", gameStates: ["game-disabled"] },
    { id: "status-game-unknown", labelKey: "mods.category.status.gameUnknown", gameStates: ["unknown"] },
  ],
};

/**
 * buildPresetTree 把内容预设原样映射成树：组 → 子组 → 标签。
 * 组/子组的 allTag 就是「查看全部」按钮挂的聚合标签；labelKey / tagLabelKeys 是
 * 显示名（例如「比尔 · Bill」）的目录 key，经 nodeLabel / presetTagLabel 延迟取文案
 * —— 与旧的内容预设菜单逐条对应，一个功能都不少。
 */
export function buildPresetTree(presetGroups = PRESET_GROUPS) {
  return presetGroups.map((group) => {
    const collectTags = (sub) => (Array.isArray(sub?.tags) ? sub.tags : []);
    const children = (group.groups || []).map((sub, index) => ({
      id: `preset:${group.id}:${index}`,
      labelKey: sub.labelKey,
      allTag: sub.allTag || "",
      tags: collectTags(sub),
      children: collectTags(sub).map((tag) => ({
        id: `tag:${tag}`,
        label: tag,
        labelKey: sub.tagLabelKeys && sub.tagLabelKeys[tag],
        tag,
      })),
    }));
    return {
      id: `preset:${group.id}`,
      labelKey: group.labelKey,
      descriptionKey: group.descriptionKey,
      allTag: group.allTag || "",
      tags: (group.groups || []).flatMap(collectTags),
      children,
    };
  });
}

/** nodeLabel 取节点当前语言的显示名（labelKey → t()；数据标签回退到原文）。 */
export function nodeLabel(node) {
  if (!node) return "";
  if (node.labelKey) return t(node.labelKey, node.labelParams);
  return String(node.label ?? "");
}

/** nodeDescription 取节点说明文案（没有说明返回空串）。 */
export function nodeDescription(node) {
  return node?.descriptionKey ? t(node.descriptionKey) : "";
}

/** presetTagLabel 取预设子组里某个标签的显示名（tagLabelKeys → t()；否则原文）。 */
export function presetTagLabel(subgroup, tag) {
  const key = subgroup?.tagLabelKeys?.[tag];
  return key ? t(key) : String(tag ?? "");
}

// 本地化排序（第 19 项 P3）：兜底分支标签跟随界面语言。
const TAG_COLLATOR = createLocaleCollator();

/**
 * buildOtherTagsBranch 把「扫描到、但没有任何树节点用到」的标签收进兜底分支。
 * 标签可达性因此天然是 100%：宁可多一个分支，也不让某个标签在树里搜不到。
 */
export function buildOtherTagsBranch(allTags = [], usedTags = new Set(), limit = 400) {
  const used = usedTags instanceof Set ? usedTags : new Set(usedTags);
  const rest = [...new Set(allTags)]
    .filter((tag) => tag && !used.has(tag))
    .sort((a, b) => TAG_COLLATOR.compare(a, b));
  if (rest.length === 0) return null;
  return {
    id: "other-tags",
    labelKey: "mods.category.otherTags",
    labelParams: { count: rest.length },
    children: rest.slice(0, limit).map((tag) => ({ id: `tag:${tag}`, label: tag, tag })),
  };
}

/** collectTreeTags 收集树里用到的全部标签（用于算「其它标签」分支）。 */
export function collectTreeTags(tree) {
  const tags = new Set();
  const walk = (nodes) => {
    for (const node of nodes || []) {
      for (const tag of nodeTags(node)) tags.add(tag);
      if (node.allTag) tags.add(node.allTag);
      walk(node.children);
    }
  };
  walk(tree);
  return tags;
}

/** buildCategoryTree 组成完整分类树：全部 Mod + 内容预设 + 状态 + 其它标签兜底。 */
export function buildCategoryTree(options = {}) {
  const nodes = [
    { id: "all", labelKey: "mods.category.allMods", kind: "all" },
    ...buildPresetTree(options.presetGroups || PRESET_GROUPS),
    STATUS_BRANCH,
  ];
  const other = buildOtherTagsBranch(options.allTags || [], collectTreeTags(nodes));
  if (other) nodes.push(other);
  return nodes;
}

// 默认树（不含「其它标签」）：模块级兜底与测试用；界面每次渲染用 buildCategoryTree 现算。
export const CATEGORY_TREE = buildCategoryTree({ allTags: [] });

/** nodeTags 返回节点挂的标签（单个 tag 或多标签）。 */
export function nodeTags(node) {
  if (!node) return [];
  if (Array.isArray(node.tags) && node.tags.length > 0) return node.tags;
  if (node.tag) return [node.tag];
  return [];
}

/** fileMatchesGameState 与后端"游戏内状态"筛选口径一致。 */
function fileMatchesGameState(file, state) {
  const known = Boolean(file?.gameStateKnown);
  const enabled = Boolean(file?.gameEnabled);
  if (state === "game-enabled") return known && enabled;
  if (state === "game-disabled") return known && !enabled;
  if (state === "unknown") return !known;
  return false;
}

/** nodePaths 返回该节点命中的 Mod 路径集合（父节点是子节点的并集）。 */
export function nodePaths(node, files) {
  const matched = new Set();
  if (!node) return matched;

  // 父节点：先把子节点结果并起来（每个子孙只扫一遍文件列表），避免逐文件递归。
  if (Array.isArray(node.children) && node.children.length > 0) {
    for (const child of node.children) {
      for (const path of nodePaths(child, files)) matched.add(path);
    }
    for (const path of nodeOwnPaths(node, files)) matched.add(path);
    return matched;
  }
  return nodeOwnPaths(node, files);
}

/** nodeOwnPaths 只按节点自身的条件（标签 / 位置 / 游戏内状态 / 全部）扫一遍文件列表。 */
function nodeOwnPaths(node, files) {
  const matched = new Set();
  if (!node || !Array.isArray(files)) return matched;
  const tags = nodeTags(node);
  for (const file of files) {
    const path = file?.path || "";
    if (!path) continue;
    if (node.kind === "all") {
      matched.add(path);
      continue;
    }
    if (Array.isArray(node.locations) && node.locations.includes(file?.location)) {
      matched.add(path);
      continue;
    }
    if (Array.isArray(node.gameStates) && node.gameStates.some((state) => fileMatchesGameState(file, state))) {
      matched.add(path);
      continue;
    }
    if (tags.length > 0 && Array.isArray(file?.secondaryTags) && file.secondaryTags.some((tag) => tags.includes(tag))) {
      matched.add(path);
    }
  }
  return matched;
}

/** countCategoryNode 给节点算数量（父节点是子节点的并集，不会重复计数）。 */
export function countCategoryNode(node, files) {
  return nodePaths(node, files).size;
}

/**
 * flattenCategoryTree 按"展开状态 + 搜索词"把树压成可渲染的行。
 * 搜索时忽略折叠状态（命中的分支全部展开），否则用户得先一层层点开才能看到结果。
 */
export function flattenCategoryTree(tree = CATEGORY_TREE, options = {}) {
  const expanded = options.expanded instanceof Set ? options.expanded : new Set(options.expanded || []);
  const query = String(options.query || "").trim().toLowerCase();
  const rows = [];

  const matchesQuery = (node) => {
    if (!query) return true;
    if (String(nodeLabel(node)).toLowerCase().includes(query)) return true;
    return (node.children || []).some(matchesQuery);
  };

  const walk = (nodes, depth) => {
    for (const node of nodes) {
      if (!matchesQuery(node)) continue;
      const children = node.children || [];
      // 默认全部折叠（像资源管理器）：根级先给 9 个大类，用户点开自己关心的那支；
      // 搜索时强制展开命中的分支，避免"搜到了却看不见"。
      const isExpanded = query ? true : expanded.has(node.id);
      rows.push({ node, depth, hasChildren: children.length > 0, expanded: isExpanded });
      if (children.length > 0 && isExpanded) walk(children, depth + 1);
    }
  };
  walk(tree, 0);
  return rows;
}

/** selectionForNode 把一个节点翻译成 filters.js 能直接应用的筛选描述。 */
export function selectionForNode(node) {
  if (!node) return null;
  if (node.kind === "all") return { tags: [], locations: [], gameStates: [] };
  if (Array.isArray(node.locations)) return { tags: [], locations: [...node.locations], gameStates: [] };
  if (Array.isArray(node.gameStates)) return { tags: [], locations: [], gameStates: [...node.gameStates] };
  const tags = nodeTags(node);
  if (tags.length > 0) return { tags: [...tags], locations: [], gameStates: [] };
  return null;
}

/**
 * nodeIsSelected 判断节点的筛选是否已生效（用于树上的高亮）。
 *
 * 语义与内容预设一致：点一下「加上这一类」，再点一下取消；可以同时选多类。
 * 所以高亮是「这个节点的标签/位置/状态都在当前筛选里」，不是「最后点过谁」。
 */
export function nodeIsSelected(node, state = {}) {
  if (!node) return false;
  const tags = Array.isArray(state.tags) ? state.tags : [];
  const locations = Array.isArray(state.locations) ? state.locations : [];
  const gameStates = Array.isArray(state.gameStates) ? state.gameStates : [];
  if (node.kind === "all") return tags.length === 0 && locations.length === 0 && gameStates.length === 0;
  // 组/子组的「查看全部」选中的是聚合标签，也要算"这一组已选中"。
  if (node.allTag && tags.includes(node.allTag)) return true;
  const ownedTags = nodeTags(node);
  if (ownedTags.length > 0) return ownedTags.every((tag) => tags.includes(tag));
  if (Array.isArray(node.locations)) return node.locations.every((value) => locations.includes(value));
  if (Array.isArray(node.gameStates)) return node.gameStates.every((value) => gameStates.includes(value));
  return false;
}

/** toggleValues 返回「加上（若未全选）或去掉（若已全选）」之后的新数组。 */
export function toggleValues(current, values) {
  const list = Array.isArray(current) ? current : [];
  const targets = Array.isArray(values) ? values : [];
  if (targets.length === 0) return [...list];
  const allSelected = targets.every((value) => list.includes(value));
  if (allSelected) return list.filter((value) => !targets.includes(value));
  return [...new Set([...list, ...targets])];
}
