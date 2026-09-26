# 任务：分析 L4D2 的 Mod 并输出"策略组建议文件"

你是一个可以直接读写本机文件的智能体（agent）。目标：读完 Mod 清单后，
判断哪些 Mod **应该被管理成同一组**，并按标准格式写出建议文件，交给 LytVPK 导入。

## 你要做的事

### 处理范围（硬性，先读这条）

**本次只处理这三个位置里的 Mod；其它位置一律不处理，也不要写进建议文件**：

- `addons` **根目录**（不递归子目录）；
- `addons\workshop\`（创意工坊，递归）；
- `addons\disabled\`（被禁用的 Mod，递归）。

`addons` 下**其它子目录**（例如 `addons\Airi初代恶堕战斗员八人\…` 这类自建套件文件夹）**不参与扫描**：
它们的 VPK 既不在清单的 `mods` 里，也不应出现在你的建议里。清单顶层的 `scope` 字段会写明被排除的
目录数与 VPK 数，`unreadableMods` 会列出"磁盘上有但解析失败"的文件 —— 这两类都属于**预期差异**：
不要为它们编造成员、也不要在建议里引用它们；确实需要说明时写进 `notes`。

**关于 `disabled` 目录（重要）**：`disabled` 是 LytVPK 的"停用区"，**游戏不会加载里面的任何文件**
（LytVPK 自己的体检也是按这个口径判定的）。所以"同一个文件名同时出现在根目录与 `disabled`"
**不是"二选一"**：它们的 `key` 归一化后是同一个 addonlist 条目，应用会把两份文件视作同一个成员，
你也不要为它们建组。清单里这类 `duplicateGroups` 会带 `singleAddonListKey: true` 与说明
（直接过滤掉即可）；真要二选一，请让用户删掉其中一份。

1. 读取 Mod 清单（由 LytVPK「分组建议 → 准备给智能体的材料」生成，路径见下文的
   `[清单文件]`）。**清单里已经准备好分组所需的全部信息，你不需要打开 VPK**：

   | 字段 | 含义 |
   | --- | --- |
   | `key` | addonlist 键（相对 addons 目录、反斜杠分隔、小写）——**建议里直接引用它** |
   | `name` / `title` | 文件名与显示标题（文件名本身常带系列名、角色名、版本号） |
   | `author` / `primaryTag` / `secondaryTags` | 作者与 LytVPK 解析出的一二级标签 |
   | `subjectSummary` / `subjectConfidence` / `contentSubjects` | 替换目标、置信度与基于资源路径的主体证据 |
   | `voiceCharacters` | 替换了哪些角色的语音（标准 `sound/player` 目录） |
   | `xdrSummary` / `modelCount` / `modelTriangles` | 骨骼槽证据与模型规模（角色模型包有用） |
   | `campaign` | 地图 Mod 的战役名 |
   | `folder` / `location` | 所在目录与位置（`root` / `workshop` / `disabled`） |
   | `gameEnabled` / `gameStateKnown` | 当前 `addonlist.txt` 里的游戏内开关状态 |
   | `loadOrder` / `effectiveLayer` / `prioritySource` | addonlist 顺序号（1 基）与有效分层 |
   | `size` / `lastModified` | VPK 文件大小与修改时间（同体积常意味着同一份文件） |
   | `structure.topDirs` | **VPK 内部顶层目录与条目数**，例如 `["models(12)","materials(40)","scripts(2)"]` |
   | `structure.targets` | **压缩后的替换目标**，例如 `props_interiors/medicalcabinet02` —— 判断"抢同一批资源"最直接 |
   | `structure.samplePaths` | 内部代表性原始路径（≤5 条，已过滤 `addoninfo`/预览图） |
   | `structure.fileCount` / `structure.totalSize` | 内部条目总数与体积合计 |
   | `addonInfo.version` / `desc` / `url` / `hasUpdate` / `chapters` / `mode` | VPK 内 `addoninfo.txt` 的作者声明：版本、描述、主页、是否有新版、战役章节数、模式 |
   | `workshop.id` / `title` / `author` / `desc` / `tags` / `steamTags` / `url` / `previewUrl` | **创意工坊自带资料**（来自 LytVPK 保存的 `.meta`）：工坊标题、作者、简介、LytVPK 标签、**工坊官方标签**（`steamTags`，作者/玩家在工坊上填的原始分类，如 `Survivors` / `Sounds` / `Single Player`）、详情页与预览图 |
   | `workshop.timeUpdated` / `downloadedAt` | 工坊最后更新时间与本地下载时间 |
   | `workshop.inWatchLater` / `views` / `subscriptions` / `favorited` / `fileType` | 工坊统计（浏览量、订阅数、收藏数）：来自"稍后再看"，或你在「设置 → 工坊数据 → 抓取官方标签与统计」里补齐的官方数据 |
   | `management.groups` | 这个 Mod 现在属于哪些策略组 |
   | `management.profiles` | 出现在哪些"启用方案"里 |
   | `management.dependencies` | 你自己声明的依赖（它需要哪些 Mod） |
   | `management.ignoredFiles` | 这个 Mod 的冲突忽略清单（说明它不提供哪些文件） |
   | `management.collections` | 它属于哪些已保存的工坊合集 |

   清单顶层还有：
   - `clusterHints`：内置推导给出的**紧凑候选簇**（组名、置信度、信号、成员键），只有几十条；
   - `duplicateGroups`：预计算好的"疑似同一 Mod 的多个副本"
     （`同名同体积` / `同名不同位置`），可以直接当作候选组；
   - `notes`：重申这份清单包含哪些信息。

   ⚠️ 清单可能有好几 MB（每个 Mod 的结构明细）。**不要一次性把 `mods` 数组读进上下文**：
   先读 `clusterHints` 与 `duplicateGroups`（都很小），再用脚本 / `jq` 之类按 key 去
   `mods` 里取你真正要看的那几条明细。

   清单顶层的 `coverage` 会告诉你各类信息实际有多少条可用，例如：

   ```json
   { "mods": 1968, "withStructure": 1968, "withWorkshopMeta": 13,
     "withAddonInfo": 1590, "withProfileOrGroup": 2, "clusterHints": 60 }
   ```

   如果 `withWorkshopMeta` 很小，说明大多数 Mod 本地没有 `.meta`（工坊标题/简介/标签缺失）——
   这属于数据缺失，不是分析错误：可以用别的字段（文件名、`structure.targets`、标签、主体）判断，
   并在汇报里说明；需要补齐时由用户在 LytVPK 的「工坊设置」里开启工坊信息存储后重新下载/刷新。

   另外这些顶层字段可以帮你快速定位工作范围：

   | 字段 | 含义 |
   | --- | --- |
   | `version` / `schemaRev` / `capabilities` | 清单格式版本与本版具备的能力（如 `entryId`、`structure.targets`、`workshopMeta`）；字段含义变化时递增 `schemaRev` |
   | `scope` | 本次覆盖的位置（`addons` 根目录 / `workshop` / `disabled`）与被有意排除的范围（`addons` 下其它子目录及其 VPK 数） |
   | `ungroupedKeys` | 内置推导没覆盖到的 Mod 键 —— 最需要你用语义判断，建议优先分析 |
   | `themeHints` | 跨槽位的"同主题套装"候选（同一主题名出现在多个替换目标上，通常适合 `all`） |
   | `preloadHints` | 可能是"前置库/依赖"的 Mod（名称含 音频库 / 前置 / xdReanimsBase / KSEP 等），建议单独成组并常开 |
   | `unreadableMods` | 磁盘上有但解析失败的文件（例如扩展名 `.vpk` 实为 ZIP）——它们**不在** `mods` 里，属于预期差异 |
   | `mods[].entryId` / `mods[].relativePath` | 唯一寻址标识与物理相对路径（`disabled/…`、`workshop/…`），用于精确指定成员 |

### 写完一定要先自检（不需要打开界面）

```
LytVPK-Community-Fork.exe --validate-group-suggestions "<建议文件路径>"
```

它按应用的真实解析规则做 dry-run：结果写到 `<文件>.validation.json`（可用 `--out <路径>` 指定），
逐条给出 `valid` / `problems` / 每个成员的 `resolved` / `matched` / `ambiguous` 以及**全部**未匹配警告；
全部有效退出码 0，否则 1。修正到 0 个 invalid 再交给用户导入。

> 首次校验会解析全部 VPK 并写缓存（1968 个 Mod 约 2–3 秒、冷启动更慢），属于正常现象；之后再跑会快很多。
> `clusterHints` / `themeHints` / `duplicateGroups` 里的成员同时给了 `memberEntryIds` / `entryIds`，
> 需要精确指定 root / disabled 同名副本时请用它们。`clusterHints` 已去重并按信号放宽到 120 条/信号
> （覆盖约一半 Mod），没覆盖到的键在 `ungroupedKeys` 里，优先分析这些。
2. 判断哪些 Mod 属于同一组。判断时可以自由使用你的语义理解，例如：
   - 同一件武器 / 同一个角色 / 同一件道具的替换（互斥，只能生效一个）；
   - 同一个皮肤的多个版本（原版 / `.repaired` / 无音频版 / 去描边版）；
   - 同一 Mod 在根目录与 `workshop` 各有一份副本；
   - 名称、标题、标签、主体、语音角色都指向同一对象的变体；
   - `structure.samplePaths` / `structure.topDirs` 显示它们覆盖同一批资源
     （例如都替换 `models/v_models/*` 或同一角色的材质）；
   - `duplicateGroups` 里列出的副本对。
   **不要**把内容无关、只是作者相同的 Mod 拼成一组；成员之间至少要有真实的"同类或互斥"关系。

   **套装 / 配套模块（很容易漏，务必专门扫一遍）**：同一个套件经常拆成很多 VPK ——
   本体（替换 `survivors/*`、`weapons/*` 等官方目标）+ 一堆配件 / 服装 / 材质 / 开关件
   （替换套件自己的目录）。它们**不是互斥候选，而是要一起开的模块**，应该建成 `all` 组。
   判定方法是看 `structure.targets`（必要时配合 `structure.samplePaths` 与文件名）里
   **共同出现的非官方路径片段**：
   - 官方目录（`survivors/`、`weapons/`、`models/`、`materials/`、`sound/`、`scripts/`、
     `particles/`、`missions/`、`scenes/`、`resource/`）不能当作套件标识；
   - 作者/套件目录（例如 `airi_evilfall`、`limod/shinano`、`mo/sikushui`、
     `lingmendalao/vrc_eku_freeegg`、`woolywinter`、`xx_particle`）被 3 个以上 Mod 共享时，
     基本就是同一个套件；它可能出现在 `targets` 的第一段（`airi_evilfall/swtich/…`），
     也可能在第二段（`913limod/airi_evilfall/…`），要按"片段"匹配而不是只看第一段；
   - 把**本体**也并进来：本体的 `targets` 指向官方目录，但它与配件通常共享同一个系列名
     （文件名/标题里都有 `airi初代…`），或 `samplePaths` 里出现同一个套件目录；
   - 命名建议：组名用套件名 +「套装」，例如「airi 初代恶堕战斗员 套装」，
     `tag` 用短一点的同名标签（如 `airi 套装`），`reason` 里写明"同一套件的本体+配件，需要一起启用"；
   - 一句话区分：**同一个官方目标**下的多个替换 = 互斥（`single`）；
     **同一个自定义前缀**下的多个模块 = 配套（`all`）。

   清单已经把最硬的证据整理好了，直接用 **`mods[].structure.resourceRoots`**
   （形如 `913limod/airi_evilfall`、`codm/ice`）= VPK 内部路径里的"作者/套件命名空间"：
   - 同一个 `resourceRoots` 值出现在 ≥3 个 Mod 上 → 基本就是同一个套件；
     LytVPK 自己也会把它们作为 `clusterHints`（信号名「套装资源目录」）给出，可以对照；
   - **本体**（只替换 `survivors/*`、`weapons/*` 等官方目标、没有 resourceRoots 的那些）
     通常与配件共享很长的**文件名前缀**（例如 `airi初代…`）或同一个系列名，请把它们并进同一个 `all` 组；
     另一条同样可靠的线索是**套件名关键词**：本体文件名里往往直接写着套件名 ——
     例如套件 `limod/shinano` 的本体叫 `shinano维纳斯bill.vpk`、`shinano维纳斯nick.vpk`…，
     而配件叫 `neko.vpk` / `油光渲染.vpk`（公共前缀为空，只能靠关键词 `shinano` 匹配）。
     **两条线索都要试**：文件名公共前缀（≥3 字符）与套件名关键词（≥6 字符，要求出现在文件名开头）。
   - **绝对不要跨套件合并**：一个 Mod 只属于它自己那个 `resourceRoots`；
     例如 `死库水*`（内部路径是 `materials/sikushui/…` 与 `materials/qkl/mo/sikushui/…`）
     属于「死库水」套件，**不属于** `limod/shinano`；把两个不同套件并成一组会让用户误开一堆无关 Mod。
     如果某个 Mod 同时出现在多个 `resourceRoots` 下，请以它的**主要替换目标**（`structure.targets`
     里最靠前的官方目标）判断归属，并在 `reason` 里说明，不要合并成一个组。
   - 真实例子（本机 2018 个 Mod 实测）：
     - `913limod/airi_evilfall` → 「airi初代 套装」= 8 个幸存者本体 + 24 个配件 + 基础材质 / 油光渲染 / neko，共 35 个；
     - `codm/ice` → 「冰霜巨龙 套装」= `codm冰霜巨龙贴图包` + `ak47冰龙参数包(蓝/紫)` + `m16冰龙参数包(蓝/紫)`
       —— 这就是"**武器贴图包与模型/参数包分开**"的典型形态，必须建成 `all` 组一起启用；
     - `codm/hyxc`（黑耀星辰：贴图包 + 三把枪参数包）、`sjz/k416`（贴图包 + 参数包成对）同理。
     - `limod/shinano` → 「shinano 套装」= `neko.vpk` / `油光渲染*.vpk` / 透明版 **+ 8 个本体**
       （`shinano维纳斯bill/coach/eills/francis/louis/nick/rochelle/zoey.vpk`，靠关键词 `shinano` 关联）；
     - `sikushui`（死库水）→ 「死库水 套装」= `死库水校服基础材质` / `死库水校服外套透明` /
       `死库水透明` + 本体 `死库水校服-教练.vpk`（靠文件名前缀 `死库水` 关联）——
       注意它的内部路径没有 `models` 段、层级也不统一，属于 `structure.resourceRoots` 里的单段候选。
   - 反面例子：`models/<作者>/weapons`、`models/<作者>/characters` 这类"通用目录名"**不是**套件名；
     纯数字（工坊 ID）也不能当套件名 —— LytVPK 已经过滤，你自己写建议时也请遵守。
3. 为每组挑一个好的组名（≤60 字），写清理由，并选择策略：
   - `single`：互斥单选（默认；同类替换、多个版本都该用它）；
   - `single_random`：随机单选；
   - `all`：全部开启（整套一起启用）；
   - `off`：全部关闭。
4. 按下面的标准格式写入 `[建议文件]`（UTF-8 JSON）：

```json
{
  "version": 1,
  "generator": "<你的名字>",
  "generatedAt": "<ISO8601 时间>",
  "notes": "本次推导的口径（可选）",
  "suggestions": [
    {
      "label": "武士刀替换合集",
      "reason": "10 个包都替换官方武士刀，游戏里只会生效一个",
      "confidence": "high",
      "strategy": "single",
      "signals": ["agent 推导", "同一把近战"],
      "members": ["workshop\\2815492899.vpk", "知更鸟晴歌武士刀.vpk"],
      "memberNotes": { "知更鸟晴歌武士刀.vpk": "与流光版二选一" },
      "tag": "武士刀",
      "tagReason": "这一组都是替换官方武士刀，打上「武士刀」后就能直接用标签筛出来",
      "memberTags": { "知更鸟晴歌武士刀.vpk": ["武士刀", "皮肤"] }
    }
  ]
}
```

### 同时给这些 Mod 打标签（重要，强烈建议）

LytVPK 的标签直接写在文件名里（`[标签]名字.vpk`，工坊 Mod 写进同名 `.meta`），
**你没有权限改用户磁盘上的文件**，所以标签也通过建议文件来表达：请在每条建议上给
`tag`（整组统一标签）与/或 `memberTags`（逐成员标签）。

为什么值得做：自动标签来自规则匹配（文件名/结构/主体），未必准；而你是在读过清单之后
做的判断，**你的标签就是这批 Mod 最精确的分类**。用户点一下「应用这个标签」即可让
以后用标签筛选就能直接选出这一批，不必再建组。

标签规则（与 LytVPK 的写入规则一致，写错会让整条建议校验失败）：

- 长度 ≤ 24 个字符；**不要**包含 `+` `,` `[` `]` `<` `>` `:` `"` `/` `\` `|` `?` `*`
  （这些字符会破坏文件名或标签解析）；
- 用"这一类是什么"来命名，而不是"这一组由谁组成"：`sg552`、`M16 武器`、`Francis 语音`、
  `下午茶套装` 都比 `替换合集（10 个）` 好；
- **不要**用过于宽泛的词（`其他`、`MOD`、`模型`、`皮肤` 单独出现时没有区分度）；
- 优先复用清单里 `mods[].primaryTag` / `mods[].secondaryTags` 已经用过的词法风格，
  但不要给一个成员打上它已经有的一级标签（避免一张文件里出现两个一级标签）；
- 只给**你确实判断过**的组打标签；不确定就省略 `tag` 字段 —— LytVPK 会降级用
  "这一组已有的共同标签 → 组名 → 成员共同的主体识别"去推导，只是没有你给的准。
- `memberTags` 是可选加强项：同一组里如果某个成员与其他成员所属的细分类不同
  （例如都是武士刀，但其中两个是"皮肤"、其余是"模型"），用它可以分别表达。

自检时留意：校验输出里的 `withTags` 是"带合法标签的建议数"，`taggedMembers` 是
"逐成员标签覆盖的成员数"；两个都是 0 说明你没有给标签（不报错，但推荐补上）。

### 清单与磁盘的时间差（重要）

清单是"导出那一刻"的快照。如果用户在你分析期间又往 mod 目录里加 / 删 / 移动了文件，
你写的成员可能已经对不上：校验会把它们标成 `matched=false`，并在警告里提示重新导出材料。

你这边要做的是：

- 写建议时**只用清单里出现过的 `key` / `entryId`**，不要凭记忆拼文件名；
- 如果校验报了大量未匹配，先让用户重新点「准备给智能体的材料」（导出前会自动重新扫描）
  再按新清单更新成员引用，不要硬改文件名去凑；
- 清单顶层的 `generatedAt` 就是导出时间，可以在汇报里带上，方便用户判断是否需要重跑。

## 硬性要求

- **只写建议文件**。不要修改、移动、删除任何 Mod 文件，也不要改 `addonlist.txt`。
- `members` 至少 2 个、建议不超过 60 个（上限 300）；每一项用清单里的 `key`；
  同一个文件同时存在于根目录与 `disabled` 时会共用同一个 `key`，这时请改用清单里的
  **`entryId`**（形如 `root/3218181634.vpk`、`disabled/3218181634.vpk`）精确指定；
  也支持 `disabled\xxx.vpk`、`workshop\xxx.vpk`、绝对路径与裸文件名（裸文件名命中多个
  不同键时才算歧义，同一个键的多位置副本会自动合并）。
- 每组 `label` 在同一次导入里不能重复；`members` 里不要重复同一个 Mod。
- `tag` / `memberTags` 是**可选但强烈建议**的：`tag` ≤ 24 字符且不含 `+ , [ ] < > : " / \ | ? *`；
  省略时 LytVPK 会用规则推导兜底（共同标签 → 组名 → 主体识别），只是不如你准。
- 只写你**有把握**的分组；模糊的可以放到 `notes` 里说明，不要硬凑。
- 写完后校验 JSON 合法，并汇报：写入了哪个文件、包含多少组、成员总数、依据是什么。

## 上下文占位符（LytVPK 会自动替换成真实路径）

- 清单文件：`{{CATALOG_PATH}}`
- 建议文件（放进这里，应用里点「重新推导」即可自动读取）：`{{INBOX_PATH}}`
- 当前已扫描 Mod 数：`{{MOD_COUNT}}`
- 建议文件格式文档：`docs/features/group-suggestion-import.md`

## 建议的分析顺序（可按需调整）

1. 先按 `subjectSummary` 聚一遍，找出"同一替换目标"的簇；
2. 再看 `voiceCharacters`（同角色语音包）、`secondaryTags`（同类内容）、
   `xdrSummary`（同骨骼槽的角色模型）；
3. 读 `workshop.title` / `workshop.desc` / `workshop.tags` / `workshop.steamTags` 与 `addonInfo.desc`：
   `steamTags` 是工坊官方分类，**判断"这是什么类型的 Mod"时优先信它**（例如带 `Survivors` 的通常
   是角色/幸存者相关，带 `Sounds` 的更可能是音效包）；`tags` 则是 LytVPK 侧整理的层级标签。
   工坊标题/简介/标签里常直接写着"替换 XX""角色模型""武器皮肤"等关键信息；
4. 检查同一 `title` 或同一 `name` 出现在不同 `key` 的情况（副本 / 多版本）；
5. 用 `structure.targets` / `structure.samplePaths` 交叉验证：同一目标的替换通常在内部路径里
   能看到相同的资源目录（`models/`、`materials/`、`scripts/`…）；
6. 看 `management.groups` / `profiles` / `dependencies` / `ignoredFiles` / `collections`：
   已经在同一方案、同一合集、或互相依赖的 Mod，往往就应该在同一组；
7. 对每个候选簇复核标题、工坊资料与结构，剔除只是标签巧合的成员；
8. 输出建议文件，并列出你**没有**分组的成员与原因（可选）。
