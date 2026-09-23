# 分组建议与外部导入（v1 格式）

这份文档写给"外部推导方"——大模型、人工整理或外部脚本。程序本身只负责
**读取标准格式 → 严格校验 → 与内置推导合并展示 → 用户确认后创建策略组**，
所以你可以用自己的判断（语义、用途、搭配关系）写出比启发式更准确的分组建议。

> 程序不会因为导入文件而改动任何 Mod 文件或 `addonlist.txt`；
> 只有用户在界面上点「创建为组」才会写入 `groups.json`。

## 1. 文件放哪里

- **收件箱（推荐）**：`%AppData%\LytVPK\group_suggestions.json`
  放进这个文件后，打开「Mod 管理 → 分组 → 分组建议」并点「重新推导」就会自动读取。
  界面上的「导入建议文件…」也可以选择任意路径的 JSON，程序会校验并复制到收件箱。
- 当前收件箱路径可以在界面上通过 `GetGroupSuggestionInboxPath()`（前端绑定）确认；
  应用「分组建议」弹窗里点「导入建议文件…」后的提示也会显示它。

## 2. 导出"给推导方看的" Mod 清单

界面「分组建议」→「导出 Mod 清单…」会写出一份 `grouping_catalog.json`：

> 导出（以及「准备给智能体的材料」）会**先自动重新扫描一遍 mod 目录**
> （`addons` 根目录 + `workshop` + `disabled`；未变化的文件走缓存，所以是增量扫描）。
> 也就是说：你往目录里加/删 Mod 之后，即使忘了点刷新，导出的清单也是最新状态，
> `modCount` / `generatedAt` / `scope` 都会随之更新；导出期间按钮显示「正在扫描并导出…」。
> 清单里的 `mods[]` 只包含这三处扫到的文件，`addons` 下其它子目录依旧不参与。

```json
{
  "version": 1,
  "generatedAt": "2026-09-22T18:40:00+08:00",
  "generator": "LytVPK 2.5.14-community.63",
  "modCount": 1968,
  "mods": [
    {
      "key": "workshop\\2985551390.vpk",
      "name": "2985551390.vpk",
      "title": "Nick 语音增强",
      "author": "某人",
      "primaryTag": "人物",
      "secondaryTags": ["Nick", "语音"],
      "subjectSummary": "主体：Nick 语音",
      "subjectConfidence": "高",
      "contentSubjects": ["Nick 语音"],
      "voiceCharacters": ["Nick"],
      "xdrSummary": "",
      "folder": "workshop",
      "location": "workshop",
      "gameEnabled": false,
      "gameStateKnown": true,
      "loadOrder": 12,
      "effectiveLayer": 12,
      "prioritySource": "order",
      "size": 12345678,
      "lastModified": "2026-09-14T12:04:28Z",
      "workshopId": "2985551390",
      "structure": {
        "topDirs": ["materials(49)", "models(4)", "（根）(2)", "sound(1)"],
        "fileCount": 56,
        "totalSize": 8123456,
        "targets": ["props_interiors/medicalcabinet02", "honkai3/theresa/body"],
        "samplePaths": ["models/props_interiors/medicalcabinet02.mdl", "materials/honkai3/theresa/body.vmt"]
      }
    }
  ],
  "clusterHints": [
    { "label": "Nick 语音", "confidence": "medium", "signals": ["语音角色", "主体识别"], "memberKeys": ["workshop\\2985551390.vpk", "workshop\\3004070051.vpk"] }
  ],
  "duplicateGroups": [
    { "reason": "同名同体积", "keys": ["123.vpk", "workshop\\123.vpk"] }
  ],
  "notes": "清单已包含分组所需全部信息……"
}
```

`key` 就是 addonlist 键（相对 `addons` 目录，反斜杠分隔、小写），
写建议时**直接引用 `key` 最稳**。

### 清单里都有什么（智能体不需要自己打开 VPK）

| 区块 | 内容 | 用途 |
| --- | --- | --- |
| `mods[].structure.topDirs` | VPK 内部顶层目录与条目数，如 `materials(49) / models(4) / sound(1)` | 一眼看出这个 Mod 改了什么类型的内容 |
| `mods[].structure.targets` | 压缩后的**替换目标**，如 `props_interiors/medicalcabinet02`、`honkai3/theresa/body` | 判断两个 Mod 是否在替换同一批资源 |
| `mods[].structure.samplePaths` | ≤5 条原始内部路径（截断到 100 字符） | 需要人工/精确核对时的证据 |
| `mods[].structure.fileCount` / `totalSize` | 内部条目数与体积合计 | 判断"轻量贴图包"还是"完整模型包" |
| `mods[].size` / `lastModified` | VPK 文件大小与时间 | 同体积常意味着同一份文件 |
| `mods[].location` / `gameEnabled` / `gameStateKnown` | 位置与 `addonlist.txt` 开关状态 | 判断当前是否生效 |
| `mods[].loadOrder` / `effectiveLayer` / `prioritySource` | addonlist 顺序号（1 基）与有效分层 | 判断优先级关系 |
| `mods[].xdrSummary` / `modelCount` / `modelTriangles` / `campaign` | 骨骼槽证据、模型规模、战役名 | 角色模型包与地图 Mod 的分组依据 |
| `clusterHints` | 内置推导给出的紧凑候选簇（默认 60 条，只有约 8 KB） | 智能体的**入口**：先读这一段 |
| `duplicateGroups` | 预计算的副本分组（`同名同体积` / `同名不同位置`，最多 400 组） | 直接可用的候选组 |
| `addonInfo.version` / `desc` / `url` / `hasUpdate` / `chapters` / `mode` | VPK 内 `addoninfo.txt` 的作者声明 | 作者自述用途、版本与更新状态 |
| `workshop.id` / `title` / `author` / `desc` / `tags` / `url` / `previewUrl` | 本地 `.meta` 里保存的**创意工坊资料**（标题、作者、简介、工坊标签、详情页、预览图） | 工坊描述与标签常直接写明"替换 XX / 角色 / 武器" |
| `workshop.timeUpdated` / `downloadedAt` / `inWatchLater` / `views` / `subscriptions` / `favorited` / `fileType` | 工坊更新时间与"稍后再看"里的热度统计 | 判断作品热度与更新时间 |
| `management.groups` / `profiles` / `dependencies` / `ignoredFiles` / `collections` | 该 Mod 已属于哪些策略组 / 启用方案 / 已声明依赖 / 冲突忽略清单 / 已保存工坊合集 | 已有的组织关系往往就是同一组 |
| `coverage` | 各类信息的可用条数（如 `withStructure` / `withWorkshopMeta` / `withAddonInfo` / `withProfileOrGroup`） | 让智能体知道哪些字段真的可用、哪些需要先补数据 |

> 完整清单含每个 Mod 的结构明细，可能有几 MB。智能体应当**先读 `clusterHints` 与
> `duplicateGroups`**，再用脚本按 `key` 去 `mods` 里取需要的明细，而不是整份读入上下文。

> `workshop.*` 只对本地存在 `.meta` 的 Mod 有值（LytVPK「工坊设置 → 开启工坊信息存储」后下载/刷新
> 过的条目才会带）。实测本机 1968 个 Mod 中 `withStructure=1968`、`withAddonInfo=1590`、
> `withWorkshopMeta=13`——缺工坊资料时可以让智能体改用文件名、`structure.targets`、标签与主体判断，
> 或者你在 LytVPK 里补齐 `.meta` 后重新导出。

### 清单版本与能力声明（v2）

清单顶层包含 `version`（当前 `2`）、`schemaRev`（结构修订号）与 `capabilities`（本版具备的能力列表），
外部推导方据此判断"这份清单能提供什么"，不必再用字段是否存在来试探。

| 顶层 / 记录字段 | 含义 |
| --- | --- |
| `entryId`（每条记录） | 唯一寻址标识 `root/…`、`disabled/…`、`workshop/…`；**同一个 `key` 在根目录与 `disabled` 各有一份时，用它精确指定成员** |
| `relativePath`（每条记录） | 物理相对路径（保留 `disabled/`、`workshop/` 前缀） |
| `scope` | 覆盖范围：`addons` 根目录（不递归）+ `workshop`（递归）+ `disabled`（递归）；同时给出被排除的子目录数量与 VPK 数量 |
| `ungroupedKeys` | 内置推导未覆盖的 Mod 键（建议推导方优先分析） |
| `themeHints` | 跨槽位同主题套装候选（同一主题名出现在多个替换目标上，通常适合 `all`） |
| `preloadHints` | 疑似前置库（名称含 音频库 / 前置 / Base / KSEP / xdReanimsBase 等） |
| `unreadableMods` | 磁盘上有但解析失败的文件（例如扩展名 `.vpk` 实为 ZIP）：它们不在 `mods` 里，属于预期差异 |
| `xdrSlots`（每条记录） | xdReanims 骨骼槽证据（角色 / 模型 / 槽位 / 动作 / 置信度） |

`members` 现在也接受上面这些身份写法（`entryId`、`disabled\xxx.vpk`、`workshop\xxx.vpk`、
绝对路径、裸文件名）；**同一个键的多个位置副本会自动合并，不再产生"命中多处"的假警告**。

### 命令行 dry-run 校验（推荐给外部智能体）

```
LytVPK-Community-Fork.exe --validate-group-suggestions "<建议文件>" [--out <结果.json>]
```

- 只读校验，不写收件箱、不改任何文件；
- 结果 JSON 逐条给出 `valid` / `problems` / 每个成员的 `resolved` / `matched` / `ambiguous`，
  以及**全部**未匹配警告（不再截断前 3 条）；
- 全部有效退出码 `0`，存在无效建议退出码 `1`；GUI 子系统下 stdout 不可见，
  因此默认把结果写到 `<文件>.validation.json`；
- 另提供 `--export-grouping-catalog [输出路径]` 供脚本直接导出清单；
- ⚠️ 首次校验会**解析全部 VPK 并建立缓存**（本机 1968 个 Mod 约 2–3 秒，冷启动更慢），
  属于正常现象；后续调用会复用缓存，明显更快。

### 清单里的 hints 与身份字段

- `clusterHints` / `themeHints` / `duplicateGroups` 现在都提供 **`memberEntryIds` / `entryIds`**：
  同一个 `key` 在 `root` 与 `disabled` 各有一份时，只有 `entryId` 能精确指向其中一条；`memberKeys`
  仅作为兼容展示保留。
- `clusterHints` 已去重（同批成员只出现一次），并按"导出侧重广度"的策略生成：
  每个信号最多 120 条、总量最多 600 条（UI 的 40 条列表仍用 20/信号 的多样性配额）。
  实测本机 1968 个 Mod：`clusterHints=340`、覆盖 **1051 个 Mod（53%）**，
  剩余 914 个未覆盖的键列在 `ungroupedKeys` 里供推导方接力。
- `duplicateGroups` 按成员集合去重：同一对副本只在更强的判据下出现一次（`同名同体积` 优先），
  实测从 400 条降到 229 组。

### hints 的身份字段与"退化副本"标注

### 套装 / 配套模块：用「共享资源前缀」识别（实测踩过的坑）

同一个套件经常拆成几十个 VPK：本体（替换 `survivors/*` 等官方目标）+ 一堆配件 / 服装 /
材质 / 开关件（替换套件自己的目录）。这类**必须建成 `all` 组**（一起开），而不是 `single`。

判定依据在清单里就有 —— `structure.targets` 里**共同出现的非官方路径片段**：

| 套件 | `structure.targets` 里的共同片段 | 说明 |
| --- | --- | --- |
| airi 初代恶堕战斗员 | `airi_evilfall/…`（部分条目是 `913limod/airi_evilfall/…`） | 24 个配件共享该前缀；本体替换 `survivors/survivor_*` |
| shinano 维纳斯 | `limod/shinano/…` | 透明版 / 油光渲染等模块共享 |
| 死库水校服 | `mo/sikushui/…` | 外套 / 透明版本等 |
| 星雪特效平台 | `xx_particle/…`、`xx_b1.2_boomer_explode` | 粒子模块按套件一起启用 |

注意片段的**位置不固定**：可能是第一段（`airi_evilfall/swtich/…`），也可能在第二段
（`913limod/airi_evilfall/type2`），要按片段匹配。官方目录
（`survivors/ weapons/ models/ materials/ sound/ scripts/ particles/ missions/ scenes/ resource/`）
不能当套件标识 —— 它们下面的多个替换才是互斥（`single`）。

> 实测教训：某轮 189 组建议里，`airi 初代恶堕战斗员` 的 8 个角色本体进了「XX 模型替换合集」
> （single，正确），但 **24 个配件一个都没进组**（全落在 `ungroupedKeys`）—— 因为内置的
> "文件名前缀"信号对**没有空格的中文长名**几乎不生效（`airi初代战斗员下肢小玩具` 与
> `airi初代战斗员臂甲` 的首词不同），"共同标签"又要求两个标签相同（这些配件只有 `贴图` 一个标签），
> 主体的"泛材质资源（无法确认具体对象）"属于低置信度会被跳过。**只有 `structure.targets` 的
> 共享前缀能可靠识别这类套装**，提示词已把这条列为必扫项。

**这条现在已经是内置信号了**（`suite-namespace`，信号名「套装资源目录」）：

- 解析 VPK 时顺带提取 `materials/models/<作者>/<套件>/…` 或 `models/<作者>/<套件>/…` 里的
  **作者/套件命名空间**，写进清单的 `mods[].structure.resourceRoots`；
- 同一个命名空间下 ≥3 个模块 → 直接产出一条 **`all` 组**；
- 再把"本体"附着进来：本体只替换官方目标（没有命名空间），但与配件共享很长的**文件名前缀**
  （例如 `airi初代…`），会被自动纳入同一组；
- 官方根（`weapons/ survivors/ w_models/ v_models/ props…`）在解析阶段就被排除，
  通用目录名（`weapons` / `characters` / `vehicles`…）与纯数字（工坊 ID）不会被当成套件名。

### 「文件名前缀」信号现在也认中文系列名（2026-09-23 补）

早先这条信号要求"文件名至少两个词"（靠 `_` / 空格 / `-` 分词），所以
`[Milfy]白银审判里内衣关.vpk` 这种**中文名整体一个词**的系列直接判空，
只剩套件目录 / 合集 / 标签能救。现在改成：

1. 保留旧的"完整首段"精确键（`series_a/b/c` 这类行为不变）；
2. **去掉 `[标签]` / `【标签】` / `（标签）` / `《标签》` 之后**，按 4~12 字生成系列前缀键 ——
   `[Milfy]白银审判_Rochelle`、`[Milfy]白银审判上衣关`、`[Milfy]白银审判里内衣关` 都落进 `白银审判`；
3. 纯数字前缀（工坊 ID / 时间戳）不产出键，避免 `3788635187` 这种组名；
4. 同一套只会产出一条建议：按"成员多 → 前缀长"排序，跳过被更大集合完全覆盖的子前缀。

顺带说明一个容易误判的现象：**分组建议默认隐藏"标签已覆盖"的建议**
（弹窗里「标签已覆盖」下拉默认「隐藏标签已覆盖」，摘要行会写
`N 条标签已覆盖（可直接用标签筛选）`）。`[Milfy]…` 这种带方括号标签的系列
正好会被标签精确覆盖，所以要在下拉里切到「显示全部」才会列出来 —— 不是没推导出来。

本机 2018 个 Mod 的实测效果：**26 个套件组、139 个成员**，其中

| 套件组 | 成员 | 内容 |
| --- | --- | --- |
| `airi初代 套装` | **35** | 8 个幸存者本体 + 24 个配件 + 基础材质 / 油光渲染 / neko |
| `!花火零食-2 套装` | 7 | 一整套开关模块 |
| `shinano 套装` | 6 | `neko.vpk` / `油光渲染*` / 透明版 |
| `ice 套装` | 5 | **武器**：`codm冰霜巨龙贴图包` + `ak47/m16 冰龙参数包` |
| `hyxc 套装` | 4 | **武器**：黑耀星辰贴图包 + 三把枪参数包 |
| `mao 套装` | 4 | 甜美海妖替换 mac-10 / uzi |

也就是说：**"武器贴图包与模型/参数包分开"这种形态，现在同样会被自动收进一个 `all` 组**
（判据是它们共享同一个 `models/<作者>/<套件>` 目录，例如 `codm/ice`）。

### 套件识别的当前口径（含两条本体关联线索）

经过两轮实测校准，现在的规则是：

| 线索 | 说明 |
| --- | --- |
| `models/<作者>/<套件>/…` | 主信号；≥3 个 Mod 共享即成包（`913limod/airi_evilfall`、`codm/ice`、`limod/shinano`…） |
| `materials/<作者>/<套件>/…`（没有 `models` 段） | 取"第一个 ≥4 字符的非通用目录段"当候选，两处层级不同也能收敛（`materials/sikushui/mo/…` 与 `materials/qkl/mo/sikushui/…` → `sikushui`）；单段候选还必须同时有 ≥3 字符的文件名公共前缀才成组 |
| 本体关联 ①（文件名公共前缀） | 本体只替换官方目标、没有套件目录，但名字与配件共享前缀：`airi初代…`、`死库水…` |
| 本体关联 ②（套件名关键词） | 配件名字里没有套件名时用它：`limod/shinano` 的关键词 `shinano` 命中 `shinano维纳斯bill.vpk` 等 8 个本体 |
| 负面清单 | 官方根（`weapons`/`survivors`/`w_models`/`v_models`/`props*`）、地图与临时容器（`static`/`tmp_mod`/`graffiti`/`brick`）、通用子目录名（`props`/`models`/`textures`/`vgui`…）、纯数字段与纯数字文件名前缀，都不会当套件名 |

**不要跨套件合并**：`死库水`（`sikushui`）与 `shinano`（`limod/shinano`）是两个套件，
即使都是"服装/材质类模块"也要分开成组。

当前实测：**34 个套件组 / 202 个成员**，其中 `airi初代 套装`35、`shinano 套装`14（6 配件 + 8 本体）、
`死库水 套装`4（3 材质 + 本体）、`Chiffon 下午茶 套装`24、`!花火零食-2 套装`7、`ice 套装`5、`hyxc 套装`4。

所有 hint 都能精确寻址（root / disabled 同名副本也区分得开）：

| hint | 身份字段 |
| --- | --- |
| `clusterHints[].memberEntryIds` / `entryIds` | 与 `keys` 一一对应 |
| `themeHints[].memberEntryIds` / `entryIds` | 同上 |
| `preloadHints[].entryId` / `memberEntryIds` | 同上（前置库通常是单个 Mod，所以 `memberEntryIds` 只有一个元素） |
| `duplicateGroups[].entryIds` | 与 `keys` 一一对应 |

`duplicateGroups` 里有一类**不能用来建组**的条目：同一文件在 `root` 与 `disabled`（以及
`workshop`）各有一份时，它们的 `key` 归一化后是**同一个 addonlist 条目**，应用会把它们视为
同一个成员。这类条目会带：

```json
{ "reason": "同名同体积", "entryIds": ["disabled/x.vpk", "root/x.vpk"],
  "singleAddonListKey": true,
  "note": "……不存在「二选一」；真要二选一请直接删掉其中一份（游戏只认得一个 addonlist 条目）。" }
```

推导方只要过滤掉 `singleAddonListKey: true` 的条目即可，不必再自己比对 key
（实测本机 229 组副本里有 9 组属于这一类）。

**为什么不给这类"同键双位置"做位置化分组？** 因为 `disabled` 是 LytVPK 的停用区，
**游戏不会加载里面的任何文件**（体检也是按这个口径判定的：「`x` 只存在于 disabled 目录，
但 addonlist.txt 仍记录着它：游戏不会加载该文件」）。也就是说这类"副本对"里只有一份会被游戏读取，
根本不存在"二选一"；正确做法是删掉多余的那一份，而不是为它建一个策略组。
如果将来需要"同一个 Mod 的多份不同版本二选一"，那应该给不同版本**不同的文件名**
（放进 `disabled` 或改版本后缀），这样它们就是不同的 key，自然会出现在普通的分组建议里。

### 清单与磁盘的时间差

清单是"导出那一刻"的快照。导出之后如果又往目录里加/删/移动过 Mod：

- 现在点「准备给智能体的材料」/「导出 Mod 清单…」会**先自动重新扫描**（见文档开头的说明），
  所以重新导出一次就能拿到新状态；
- 用旧清单写出来的建议里，失效成员会在校验时被判为 `matched=false`；校验输出会给出
  「**重新导出材料**再让智能体更新成员引用」的提示，而不是让你去猜文件名；
- 因此推荐的节奏是：**导出材料 → 不要改动 mod 目录 → 智能体写建议 → 导入/校验**。

## 3. 建议文件格式

> 交给智能体的提示词在应用里可以**直接编辑**：「分组建议 → 编辑提示词…」，
> 默认是 LytVPK 填好的一套（含处理范围、格式、标签规则与自检要求），
> 你可以改成自己的版本并保存，也可以随时「恢复默认」。自定义提示词保存在
> `%AppData%\LytVPK\group_suggestion_agent_prompt.md`，其中的
> <code v-pre>{{CATALOG_PATH}}</code> / <code v-pre>{{INBOX_PATH}}</code> /
> <code v-pre>{{MOD_COUNT}}</code> 会在发送给智能体时替换成本机真实值。

```json
{
  "version": 1,
  "generator": "codex",
  "generatedAt": "2026-09-22T18:45:00+08:00",
  "notes": "按角色语音包与武器替换目标整理",
  "suggestions": [
    {
      "label": "Nick 语音",
      "reason": "三个包都只替换 Nick 的语音，互相冲突，同一时间只应启用一个",
      "confidence": "high",
      "strategy": "single",
      "signals": ["人工/模型推导"],
      "members": [
        "workshop\\2985551390.vpk",
        "workshop\\3004070051.vpk"
      ],
      "memberNotes": {
        "workshop\\2985551390.vpk": "新版，优先保留"
      },
      "tag": "Nick 语音",
      "tagReason": "这三个包都属于 Nick 语音，打上标签以后可以直接用标签筛选",
      "memberTags": {
        "workshop\\2985551390.vpk": ["Nick 语音", "新版"]
      }
    }
  ]
}
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `version` | 是 | 固定 `1`（缺省按 1 处理；其它值会被拒绝） |
| `generator` / `generatedAt` / `notes` | 否 | 仅用于展示与排查 |
| `suggestions[].label` | 是 | 组名，最长 60 个字符；同一次导入里不允许重名 |
| `suggestions[].reason` | 否 | 展示在卡片上的理由，建议写清楚"为什么它们是一组" |
| `suggestions[].confidence` | 否 | `high` / `medium` / `low`（也接受 高/中/低），缺省 `high` |
| `suggestions[].strategy` | 否 | 建组策略：`single`（互斥单选，默认）/ `single_random`（随机单选）/ `all`（全部开启）/ `off`（全部关闭） |
| `suggestions[].signals` | 否 | 追加展示的信号标签，例如 `["模型推导", "同一角色"]` |
| `suggestions[].members` | 是 | 成员数组，至少 2 个、最多 300 个 |
| `suggestions[].memberNotes` | 否 | 给单个成员加备注（当前仅保留在文件里，便于人工复核） |
| `suggestions[].tag` | 否（**强烈建议**） | 整组统一标签：≤ 24 个字符，且不能含 `+` `,` `[` `]` `<` `>` `:` `"` `/` `\` `|` `?` `*`。导入后卡片上显示「导入标签：xxx」，可以一键应用 |
| `suggestions[].tagReason` | 否 | 为什么打这个标签（展示在徽标提示里） |
| `suggestions[].memberTags` | 否 | 逐成员标签：`{"键": ["标签A", "标签B"]}`，用于"同一组里还要再细分"的情况 |

### 标签：为什么建议你一起产出，没产出会怎样

自动标签来自 LytVPK 的规则匹配（文件名 / 结构 / 主体识别），命中不一定准；
而推导方（大模型 / 人工）是读过清单后做的语义判断，**你给的标签就是这批 Mod 最精确的分类**。

所以标准做法是：每条建议都尽量给出 `tag`（整组统一），需要更细时再给 `memberTags`。
导入后：

- 卡片上会出现「导入标签：xxx」徽标，点「应用标签」即可把这些标签写进文件名（工坊 Mod 写进 `.meta`）；
- 建立策略组时，「分组 → 打标签…」会**优先采用导入的标签**，其次才是这一组已有的共同标签、
  组名、成员共同的主体识别；
- 应用标签会改变 addonlist 键，LytVPK 会自动把策略组 / 分层 / 依赖 / 忽略清单里的旧键改绑到新键。

**没有给标签也能正常导入**：LytVPK 会降级用规则推导（共同标签 → 组名 → 成员共同主体识别），
只是精度不如你给的。校验结果里的 `withTags` / `taggedMembers` 就是给你自检用的：
两者都是 0 说明这次一个标签都没给。

`members` 里的每一项支持三种写法：

1. **addonlist 键**（推荐）：`workshop\2985551390.vpk`，斜杠方向不限、大小写不限；
2. **裸文件名**：`2985551390.vpk`。如果根目录与 `workshop` 各有一份同名文件，
   程序会把两份都纳入，并在导入结果里提示"命中多个位置"；
3. **绝对路径**：`E:\...\addons\a.vpk`，程序会换算成相对键。

## 4. 校验与错误处理

- 无法匹配当前 Mod 列表的成员会被**忽略并写进警告**（导入结果弹窗会列出前几条）；
- 有效成员不足 2 个、成员超过 300 个、重名 label、缺少 label 的建议会被**整条跳过**并说明原因；
- 版本不支持、JSON 非法、`suggestions` 为空时会直接报错，不会覆盖收件箱里已有的文件；
- `tag` 超过 24 个字符、或含 `+ , [ ] < > : " / \ | ? *` 时，**该条建议会被判为 invalid**
  （`problems` 里会写清楚），修正后再导入；
- `memberTags` 里的空成员键、非法标签同样会写进 `problems`；合法标签会计入 `taggedMembers`；
- 导入成功后会**整份复制到收件箱**，界面上用「清除导入」可以移除。

### 展示上限与顺序

- 导入建议**保留文件里的顺序**（分数按文件位置递减），不会被"成员少优先"的规则重排；
- 展示上限按"外部建议数量 + 40 条内置建议"计算（最多 240 条），
  所以导入一份 195 组的大文件时能在弹窗里逐条看到，而不是只看到前 40 条；
- 界面概览会写明"共 N 条建议（其中 M 条来自导入的建议文件）"。
## 5. 展示与排序

- 导入的建议带「导入建议」徽标，排在所有内置启发式建议之前（内置的信号、排序规则见
  `docs/features/mod-management.md`）；
- 每条导入建议同样按"冲突检测"那种行结构展示：标题 / 文件名 / 位置 / 游戏开关 / 优先级，
  并带「详情」「游戏开关」「启用·禁用·复制到 addons」按钮，可以逐个核对后再建组；
- 如果这批 Mod 已经在同一个策略组里，卡片会显示「已建组」并沉到列表末尾。

## 6. 推荐工作流（大模型 / 人工）

1. 在「分组建议」里点「导出 Mod 清单…」，得到 `grouping_catalog.json`；
2. 推导方阅读清单（可以配合 `subjectSummary`、`secondaryTags`、`voiceCharacters`、
   `author`、`folder` 等字段），按上面的格式写出 `group_suggestions.json`；
3. 把文件放到收件箱，或在界面点「导入建议文件…」选择它；
4. 在弹窗里逐条核对（可取消个别成员、改组名），确认后点「创建为组」；
5. 需要重新生成时，点「清除导入」移除收件箱文件即可。

## 7. 一键把材料交给智能体（推荐）

「分组建议」弹窗里的 **「准备给智能体的材料」** 会一次做完三件事：

1. 导出最新的 `grouping_catalog.json`（清单里带键、标题、作者、标签、主体、语音角色、所在文件夹）；
2. 把**智能体提示词**复制到剪贴板 —— 提示词已经填好本机的清单路径与建议文件路径；
3. 弹窗显示两条路径，告诉你把清单交给智能体、以及智能体应该把 `group_suggestions.json`
   写到哪里（`%AppData%\LytVPK\group_suggestions.json`，写完点「重新推导」即可看到）。

想单独拿到提示词时：

- 「复制智能体提示词」：只复制提示词（剪贴板不可用时自动改为另存）；
- 「保存提示词…」：把提示词另存成 `.md`，交给任意支持读写文件的智能体（Codex、Claude 等）。

提示词（`internal/app/assets/group_suggestion_agent_prompt.md`，随 EXE 发布）里写清了：

- 要读哪些字段、可参考哪些判断依据（同武器/同角色/同道具、同皮肤的多个版本、根目录与
  workshop 的副本等）；
- 标准输出格式与四种 `strategy` 的含义；
- 硬性要求（只写建议文件、不改 Mod、不改 `addonlist.txt`、成员 2–300 个、组名不重复）；
- 建议的分析顺序与自检项。

### 为什么这样更准

内置推导只能看元数据（标签、主体、语音角色、文件名），而智能体可以结合标题语义、
皮肤/版本命名习惯、副本关系做判断。程序侧只做"校验 + 展示 + 落盘"，
所以**推导能力可以持续升级，而应用逻辑保持稳定**：这也正是把推导引擎
（`internal/grouping`）与 UI / 存储解耦的原因。
