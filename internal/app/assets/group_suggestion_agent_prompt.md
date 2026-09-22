# 任务：分析 L4D2 的 Mod 并输出"策略组建议文件"

你是一个可以直接读写本机文件的智能体（agent）。目标：读完 Mod 清单后，
判断哪些 Mod **应该被管理成同一组**，并按标准格式写出建议文件，交给 LytVPK 导入。

## 你要做的事

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
   | `workshop.id` / `title` / `author` / `desc` / `tags` / `url` / `previewUrl` | **创意工坊自带资料**（来自 LytVPK 保存的 `.meta`）：工坊标题、作者、简介、工坊标签、详情页与预览图 |
   | `workshop.timeUpdated` / `downloadedAt` | 工坊最后更新时间与本地下载时间 |
   | `workshop.inWatchLater` / `views` / `subscriptions` / `favorited` / `fileType` | 你在 LytVPK 里"稍后再看"保存过的工坊统计（热度、订阅数、收藏数） |
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
      "memberNotes": { "知更鸟晴歌武士刀.vpk": "与流光版二选一" }
    }
  ]
}
```

## 硬性要求

- **只写建议文件**。不要修改、移动、删除任何 Mod 文件，也不要改 `addonlist.txt`。
- `members` 至少 2 个、建议不超过 60 个（上限 300）；每一项用清单里的 `key`；
  同一个文件同时存在于根目录与 `disabled` 时会共用同一个 `key`，这时请改用清单里的
  **`entryId`**（形如 `root/3218181634.vpk`、`disabled/3218181634.vpk`）精确指定；
  也支持 `disabled\xxx.vpk`、`workshop\xxx.vpk`、绝对路径与裸文件名（裸文件名命中多个
  不同键时才算歧义，同一个键的多位置副本会自动合并）。
- 每组 `label` 在同一次导入里不能重复；`members` 里不要重复同一个 Mod。
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
3. 读 `workshop.title` / `workshop.desc` / `workshop.tags` 与 `addonInfo.desc`：
   工坊标题/简介/标签里常直接写着"替换 XX""角色模型""武器皮肤"等关键信息；
4. 检查同一 `title` 或同一 `name` 出现在不同 `key` 的情况（副本 / 多版本）；
5. 用 `structure.targets` / `structure.samplePaths` 交叉验证：同一目标的替换通常在内部路径里
   能看到相同的资源目录（`models/`、`materials/`、`scripts/`…）；
6. 看 `management.groups` / `profiles` / `dependencies` / `ignoredFiles` / `collections`：
   已经在同一方案、同一合集、或互相依赖的 Mod，往往就应该在同一组；
7. 对每个候选簇复核标题、工坊资料与结构，剔除只是标签巧合的成员；
8. 输出建议文件，并列出你**没有**分组的成员与原因（可选）。
