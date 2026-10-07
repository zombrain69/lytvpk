# Mod 标签识别 v2（维护者开发文档）

> 状态：**待开发**（Codex 目标模式已启动）
> 基线版本：`2.7.1-community.42`（`d224707`）
> 本文是维护者文档，不进入公开文档站侧边栏。

## 进度

| 工作包 | 状态 | 证据 |
| --- | --- | --- |
| W0 护栏先行 | ✅ 已完成（2026-09-30） | `internal/app/tag_regression.go`、`internal/app/tag_regression_test.go`、`tools/tag-regression-allowlist.json`；真机 2858 Mod 实测：同基线 → `+0/-0` 退出 0；模拟规则退化 → 精确报出 `disabled/(钢铁轰鸣)榴弹neon.vpk 消失=[…]` 且退出 1；allowlist 登记后 → 放行退出 0。`go test ./... -count=1` 全绿 |
| W1 StockIndex | ✅ 已完成（2026-09-30） | `internal/gamedata/stockindex.go`（扫描 + 查询 + glob + 缓存）、`internal/parser/stock_lookup.go`（解析器侧最小接口，避免包循环）、`internal/app/stock_index.go`（懒加载 + 降级）、CLI `--build-stock-index` / `--stock-index-status` / `--stock-index-query`。真机实测：**80937** 条本体路径、5 个挂载点全部命中（left4dead2 22273+25952 松散、dlc1 1302+4677、dlc2 6036+11847、dlc3 6918+9525、update 3634+7645），构建 5.6s；`sound/player/hunter/voice/**` = 103 条（语音只在松散目录，已覆盖）；`scripts/weapon_*.txt`=45、`scripts/melee/*.txt`=14、`particles/**`=54；D2/D3/D8 的关键路径均可在索引中查到；降级路径有单测：无 Mod 目录 → 返回 nil + 可读原因（`TestLoadStockIndexDegradesWithoutGameDir`）、只有缓存也能查（`TestLoadStockIndexUsesCacheWhenGameRootMissing`）、nil 索引的 `Exists`/`Glob`/`Query` 全部安全返回；非法游戏目录只报中文错误不崩（`--build-stock-index --game-dir E:\not-a-game`）。`go vet ./...` + `go test ./... -count=1` 全绿，且 W0 护栏复跑为 `+0/-0`（磁盘上多了 3 个新 Mod） |
| W2 规则表声明化 + 解析校验 | ✅ 已完成（2026-09-30，含迁移收尾） | 唯一事实源 `internal/ruletable/rules.json`（44 条内容/物品规则 + 88 条武器路径规则 + 56 条标题关键词规则 + **角色六表** + 7 个文件类别），解析器启动时从它构建全部规则表（Go 字面量已删除）；`internal/gamedata/rules` 只做校验（`--validate-tag-rules`）+ 数据完整性测试。**校验器首次真机运行就抓到 D12**：`materials/vgui/loadingscreen/` 前缀在本体 0 命中 → 修复后真机 **+41 / −0**（41 个 Mod 找回「载入画面」）。迁移收尾前后真机结果**逐位一致**：`--check-tag-regression` = `+1338 / −0`、`--validate-tag-rules` = `checked=398 / dead=0 / ok=true`；角色语音目录检查把 **D5 量化**（见 §2 表）。`go vet ./...` + `go test ./... -count=1` + `frontend npm run build` 全绿 |
| W3 脚本/本体驱动实体表 | ✅ 已完成（2026-09-30） | `internal/gamedata/entitygen`（从 `scripts/weapon_*.txt` + `scripts/melee/*.txt` 生成；用模块内现成的 Valve KeyValues 解析器，不手写解析）、`internal/gamedata/entities`（**64 个实体 / 109 条本体锚点**，含 16 个物品实体、7 个特感爪子、8 个幸存者手臂）、`internal/gamedata/read.go`（按挂载点优先级读本体文件内容，W4 读 .mdl 也要用）、CLI `--generate-entity-table` / `--entity-table-status`。解析器新增**精确锚点通道**（命中即加标签，不参与排除）：Chrome→铁喷、`w_golfclub`→高尔夫球杆、`explosive_box001`→烟花盒、`w_cola`→可乐、`v_claw_hunter`→Hunter+特殊感染者、`v_arms_coach_new`→Coach+幸存者。真机校验 **396 条规则（287 规则 + 109 锚点）/ 0 死亡锚点 / ok=true**；真机回归 **+62 / −0**（其中 21 条「木喷」按 `tools/tag-regression-allowlist.json` 的 global 登记放行，见 D1）。`go vet` + `go test ./...` + `frontend npm run build` 全绿 |
| W4 多通道证据 | ✅ 已完成（2026-09-30） | 通道 2「本体基名 token」（`stock_token_evidence.go`：`rifle_ak47` / `codm_krig6icedrake-ak47` / `xx2_c6_custom_rifle_ak47` 都算证据；通用词黑名单避免 `pumpshotgun`/`rifle`/`smg` 回灌错误标签）、通道 3「`.mdl` 材质表」（`mdl_materials.go` 解析模型头部 cdtextures + 材质名，`mdl_evidence.go` 有界读取：只在还没有精确证据时读、每包 ≤4 个模型、单模型 ≤8MB；端到端测试 `TestScanUsesModelMaterialTableForRenamedModels` 用"改名模型 + 本体材质名"复现并验证）、通道 4「标题/描述/文件名证据」（`title_evidence.go`：对所有主类型生效、收集全部命中、分隔符归一让 `riot_shield` 命中 "Riot Shield"）、通道 5「套件命名空间继承」（`internal/app/tag_inheritance.go`：命名空间内 ≥60% 成员带的标签补给缺它的成员，只加不删、成员数 ≥3、单成员上限 20，并记录 `suite:<命名空间>` 证据）。**真机验收：29 个"标题/文件名含型号但缺标签"的 Mod → 0 个残留（100%，目标 ≥90%）**；真机回归 **+1396 / −0**（21 条「木喷」按登记放行；其中通道 5 贡献 +275）；`--validate-tag-rules` 398 条规则 / 0 死亡。`go vet` + `go test ./...` + `frontend npm run build` 全绿 |
| W5 新类别与漏判路径 | ✅ 已完成（2026-09-30） | 文件类别补齐 `particles/`→「粒子特效」、`scripts/vscripts/`→「VScript」、`resource/`（含非 ui）→「UI」（`content_tag_rules.go`）；特感语音识别出**第三种本体布局** `sound/npc/<class>/voice/…`（Witch 的鬼叫就走这里）并补上 `sound/player/<si>/**`、`sound/player/pz/**`（`character_parser.go` + `isCharacterAssetPath` 直接复用 `detectVoiceCharacter`）；missions-only 包改为只要存在 `missions/*.txt` 就提取战役/模式并给「地图配置」兜底（`shouldCollectMissionEvidence` + `collectMissionEvidence`）。**真机验收**：粒子 140/140 带「粒子特效」；语音包 72 个里 **61 个拿到角色身份**（目标 ≥60；其余 8 个去重后是止痛药/喷漆/脚步音效，根本不是角色语音）；resource 17/17 带「UI」；missions-only 3/3 有标签（含战役名与模式）。真机回归 **+1121 / −0**（21 条按登记放行）；全库有标签率 **2859/2861 = 99.9%**，平均 5.40 个二级标签。`go vet` + `go test ./...` + `frontend npm run build` 全绿 |
| W6 呈现与对外清单 | ✅ 已完成（2026-09-30） | `internal/parser/tag_evidence.go`（每个标签记录 rule / level / ≤3 条来源；强度升级时连来源一起换，避免「rule=本体锚点、source=关键词路径」的自相矛盾）、`VPKFile.TagEvidence`；Mod 详情新增「标签依据」区块（`frontend/src/js/features/modals/tag-evidence-view.mjs` 纯逻辑 + 4 个 node 测试，`detail.js` 只用 createElement/textContent，CSS 用 rem 遵守字号档位规则），Wails 绑定用 `wails generate module` 重新生成；`--export-grouping-catalog` 增加 `tagEvidence`（对外每条只带 1 条代表路径：9.36MB→7.83MB）并把 schemaRev 提到 `2026-09-30.1`、capabilities 加 `tagEvidence`；分组 suite 信号追加「共同标签：X」（只加展示信号，不改 Keys/Score/Strategy）。真机：**2855/2861 个 Mod 带证据**（规则前缀分布：entity 1695 / category 7463 / content 1366 / token 359 / custom 212 / weapon 127 / character 99 / title 88）。`go vet` + `go test ./...` + `node --test`（458）+ `frontend npm run build` 全绿 |
| W7 实体槽位（显式 slot 表） | ✅ 已完成（2026-10-07） | §4.2 规划的"槽位"此前只落地了 model 锚点，非模型资源（武器音效 / 贴图目录 / HUD 图标）只能靠手写关键词——2026-10-07 的「匕首」误标（裸关键词 `knife` 命中作者命名空间 `materials/weapons/oe/knife_reborn-a6/**`）就是后果。本轮把归属改由**本体数据**推导并落成**显式槽位表**：`--generate-entity-table` 用本体索引枚举目录 / 文件 stem，凡只被一个实体"归属词"命中的就写成该实体的 `slots[{kind,prefix,hits}]`（`kind ∈ model/sound/material/ui/script`，`hits` = 该前缀在本体索引里的命中数），**任何 0 命中槽位直接让生成失败**（"构建期 0 命中 = 大声失败"）；解析器新增通道「本体独占资源前缀」（`internal/parser/stock_owner_prefix.go`，证据 `stock:<entityID>`）。真机：**110 条槽位 / 38 个实体**（ui 61 / sound 25 / material 24），`--check-tag-regression` 对照规则修正前基线 = 新增 / −0（allowlist 放行 56 条已登记误标）。三次收紧的教训写进代码注释：① 文件 stem 只信 `materials/vgui/**`（一条 `…rifle_fire_1_incendiary.wav` 曾把 217 个包标成「燃烧弹盒」）；② 实体词只取每段 token 的**末词**（`weapon_pain_pills` 只取 `pills`，`pain` 曾把 `sound/npc/witch/voice/pain/**` 归成「止痛药」）；③ 槽位前缀必须落在武器资源根或 UI 根，并沿用 token 通道的泛词黑名单（`shotgun`/`rifle`/`smg`…），否则 `sound/weapons/shotgun/**`（泵动/Chrome/连喷共用）会把铁喷包标成「木喷」——正是 D1 的翻版 |
| W8 地图道具口径细化 + 标签呈现（2026-10-07） | ✅ 已完成（2026-10-07） | 用户口径：① 地图包自带的 `models/props_*` 道具不再算"替换了某个物品"；② 筛选条里不要出现 `#L4D360UI_*` 这种本地化 token；③ 子标签那一行"只显示一些些"没意义，要放有用的东西。实现：`map_resource_tags.go` 新增 `mapOnlyPathMarkers` + `allSourcesMapOnly`，**只对 `entity:` 精确锚点生效**——内容/类别证据（`content:` / `category:`）不看路径，否则会把「模型 / 贴图」这类正当内容标签一起删掉（本轮第一版就是这么写的，被 `TestMapPackDropsResourceCoincidenceTags` 当场抓到并改成 rule 前缀判定）；`dropLocalizationTokenTags` 对**所有主类型**清掉 `#` 开头 / `L4D360UI_` / `L4D_` 前缀；`map_parser.go` 里 token 战役名改为"不进标签集合 + 用任务文件名兜底展示"，章节名是 token 时退回章节代码；新增 `GetSecondaryTagCounts` 绑定与前端「常用子标签」排序（命中 Mod 数降序、同分字典序 + 角标，纯逻辑放在 `secondary-tag-usage.mjs` 并有 node 测试）。真机：冷启动全量重解析 **2962 个 Mod**，`--check-tag-regression` = **+54 / −0**（本轮新增 18 条 allowlist 登记：14 个 `#L4D360UI_CampaignName_C*` + 4 条地图自带道具标签，逐条带 reason/evidence）；`--export-grouping-catalog` 里 `L4D360UI` 出现次数 **14 → 0**；沙箱子标签条实测 120 项按常用度排序（战役模式 54 / 贴图 49 / UI 48 / 脚本 46 / VScript 44 / 载入画面 36）+「展开全部 120 项」 | 

> W1 实现说明（与 §4.1 原文的差异）：新鲜度用 `generatedAt` + 各挂载点条目数表达，**没有**接入 Steam build 号；
> 缓存默认永不过期，游戏更新后由 `--build-stock-index` 显式重建。缓存位置 `%APPDATA%\LytVPK\stock_index.json`。
>
> W2 实现说明（**迁移收尾已完成**）：`internal/ruletable/rules.json` 现在是**唯一事实源** ——
> 解析器（`contentTagRules` / `weaponPathRules` / `weaponMetadataRules` / 角色六表 / 文件类别）
> 全部在启动时从它构建，Go 里的规则字面量已删除；`internal/gamedata/rules` 只负责用本体索引校验它。
> 迁移按"双读比对 → 真机 diff"的顺序落地：迁移前后真机结果**逐位一致**
> （`--check-tag-regression` 均为 `+1338 / −0`，`--validate-tag-rules` 均为 `checked=398 / dead=0 / ok=true`）。
> 原来的逐条 parity 测试替换为关键条目 + 规模下限的数据完整性测试；"没有少标"由真机护栏兜底。
>
> W4 实现说明：通道 3（`.mdl` 材质表）按"材质名 token 匹配本体基名"计级为 `pattern`
> （只有"路径 == 本体路径"或官方脚本声明才计 `exact`）。通道 5（套件命名空间继承）落在 App 层
> （`internal/app/tag_inheritance.go`），因为它是跨 Mod 推断；实现用**快照 + 命名空间排序**，
> 避免 map 迭代顺序影响"成员上限"截断 —— 真机连续 3 次 `--check-tag-regression` 与两次清单导出
> 的标签/证据签名完全一致（确定性已验证）。
>
> §6 第 3 条「召回夹具」的落点：没有单独放 `testdata/tag_recall_fixtures.json`，
> 而是按类别写成 Go 回归用例 —— `upstream_parity_test.go`（界面误判/关键词优先级）、
> `tag_rules_regression_test.go`（武器/聚合/混合包/自定义标签）、`stock_entity_anchor_test.go`（D1–D8 真实路径）、
> `title_evidence_test.go` + `stock_token_evidence_test`（通道 2/4）、`mdl_materials_test.go`（通道 3）、
> `w5_paths_test.go`（D5/D6/D9/D10）。夹具路径一律来自游戏本体脚本，不是编造样例。

## 0. 目标与硬约束

把标签识别从「手写关键词表 + 目录白名单」升级为
**「本体索引（唯一事实源）+ 声明式实体表 + 多通道证据合并」**，并让「少标」在工程上不可能通过。

硬约束（用户明确要求，优先级高于一切"看起来更干净"的设计）：

1. **可以多标，不能少标。** 任何规则改动只允许「新增标签」，不允许「某个 Mod 少了一个标签」。
2. 现有已产出的标签集合是**下界**；新实现可以加，不可以减。
3. 冲突/排序/置信度可以变，**标签集合本身不能缩**。
4. **唯一例外**：已确认为「错误标签」的修正。这类改动必须在 PR 里显式登记到
   `tools/tag-regression-allowlist.json`（每条含：命中特征、移除的标签、新增的标签、本体证据、原因），
   护栏放行 allowlist 内的移除，其余任何消失一律失败。**没有登记就不许删标签。**

## 1. 外部参考（本轮实测）

| 参考 | 许可 | 值得学的点 |
| --- | --- | --- |
| [Starfelll/NekoVpk](https://github.com/Starfelll/NekoVpk)（C#，94★） | 见仓库 | `NekoVpk/TaggedAssets.jsonc`：**声明式 glob 标签表**，75 条实体；**遍历所有条目收集全部命中标签**（不 return 首个）；标签带 `type`（`Weapon/Rifle`、`Survivor`、`SpecialInfected`、`Item`、`Animation`）；含 `Particle/Sound/Map/Skybox/VScript/Spray` 文件类别；**每个角色的 vgui 选人面板、`v_arms_*` 手臂、死亡姿势都单独列了本体路径** |
| [coffeegrind123/l4d2-mod-manager](https://github.com/coffeegrind123/l4d2-mod-manager)（Python，MIT） | MIT | ① 用**游戏本体 VPK + 松散文件**建引擎路径索引（85 实体 / 417 槽位 / 51264 文件）；② 每条 glob 必须解析到 ≥1 文件，**解析 0 命中就大声报错**；③ 读 `.mdl` 头部**材质表**反查贴图路径（4989 条），使"只换贴图的包"也能归属实体；④ **武器/近战脚本是权威来源**，明确写明"从名字猜会猜反"；⑤ `conflict`（同路径）与 `overlap`（同槽位不同文件）分离；⑥ 覆盖率百分比（primary 槽 ×2、普通 ×1、可选 ×0.5） |

两个参考的共性结论：**规则要锚定在本体真实路径上，而不是锚定在人类对命名的直觉上。**

## 2. 已确认缺陷（本轮真机取证，全部来自游戏本体脚本/路径）

证据来源：`left4dead2/pak01_dir.vpk` 与 dlc1/2/3、update 的 `scripts/weapon_*.txt`、`scripts/melee/*.txt`
（用只读探针 `vpkprobe --cat` 读出，原始记录见 `.tmp-tag-audit/weapon_script_map.txt`）。

| # | 症状 | 本体权威证据 | 影响 | 归属工作包 |
| --- | --- | --- | --- | --- |
| D1 | **Chrome 连喷被标成「木喷」** | `weapon_pumpshotgun` → `models/w_models/weapons/w_shotgun.mdl`；`weapon_shotgun_chrome` → `models/w_models/weapons/w_pumpshotgun_A.mdl` | `weaponPathRules` 里 `{"pumpshotgun","木喷"}` 先于 `{"chrome","铁喷"}` 命中 → 标错且**丢掉「铁喷」** | W3 ✅ 锚点给出「铁喷」+ W4 精度收尾：泛关键词 `pumpshotgun`/`shotgun` 不再把 Chrome 世界模型与共享霰弹枪目录（`sound/weapons/shotgun/**`、`materials/**/weapons/shotgun/**`）判成「木喷」；「木喷」移除按 `tools/tag-regression-allowlist.json`（scope=global，带本体证据）整类放行，真机 23 例。**迁移后抽查：23 个 Chrome 包 23 个带「铁喷」、0 个再带「木喷」**；全库「木喷」60 个（其中 20 个确实替换 `w_shotgun.mdl`） |
| D2 | **烟花盒不命中** | `weapon_fireworkcrate` → `models/props_junk/explosive_box001.mdl` | 关键词表写的是 `firework`，真实模型名里没有该词 | W3 ✅ 锚点命中（真机 +烟花盒） |
| D3 | **可乐（Cola）无实体** | `weapon_cola_bottles` → `models/w_models/weapons/w_cola.mdl` / `v_cola.mdl` | 本体存在、NekoVpk 有 `Cola`，我们没有标签 | W3 ✅ 新实体「可乐」（真机 +6） |
| D4 | **特感爪子/手臂无角色归属** | `weapon_{boomer,hunter,smoker,tank,charger,jockey,spitter}_claw` → `models/v_models/weapons/v_claw_*.mdl`、`models/weapons/arms/v_*_arms.mdl` | 既不在武器关键词表、也不在角色白名单 → 纯爪子包丢失特感身份 | W3 ✅ 7 个特感爪子 + 8 个幸存者手臂锚点（真机 +Rochelle 等） |
| D5 | **特感语音布局不识别** | 本机 `left4dead2/sound/player/` 实测：`boomer charger hunter jockey smoker spitter tank pz footsteps survivor items water`；语音在 `sound/player/<si>/voice/...`。**W2 校验器量化**：假设路径 `sound/player/infected/voice/<dir>/` = **0/10** 命中；真实布局 `sound/player/<dir>/` = **7/10** 命中（boomer 158 / hunter 107 / tank 97 / smoker 87 / charger 64 / jockey 56 / spitter 47 条文件；common/hulk/witch 本体无该目录）。W5 又发现**第三种布局** `sound/npc/<class>/voice/…`（Witch） | `detectVoiceCharacter` 只认 `sound/player/{survivor,infected}/voice/<slot>`；实测 72 个带「语音包」的 Mod 中 **66 个没有任何角色身份** | W5 ✅ 三种布局全部支持；语音包 **61/72 拿到角色身份**（`voiceCharacters` 命中 21 → 50） |
| D6 | **粒子特效无类别** | 本体 `particles/*.pcf` 61 条；NekoVpk 有 `Particle` | 库内 196 个含 `particles/` 的 Mod，**0 个**「粒子/特效」标签（其中 56 个完全零标签） | W5 ✅ `particles/**`→「粒子特效」；真机 140/140 命中 |
| D7 | **多型号只出一个标签** | `DetectWeaponTypeFromMetadata` 命中即 `return`；且只在 `primary==武器` 时调用 | 标题含 `AK47+M16` 的包只出一个型号；`其他/人物` 主类型的包完全不看标题 | W4 ✅ 改为收集全部命中 + 标题/描述/文件名证据对所有主类型生效 |
| D8 | **高尔夫球杆不命中** | `scripts/melee/golfclub.txt` → `models/weapons/melee/w_golfclub.mdl` | 规则写的是 `golf_club`（带下划线），真实路径是 `golfclub` | W3 ✅ 锚点命中 + `DetectWeaponType` 锚点兜底 |
| D9 | **missions-only 无战役/模式** | 本体 `missions/campaign*.txt`；库内 3 个去重后的 missions-only 包 | 只有 `hasMap` 才解析 mission | W5 ✅ 只要有 `missions/*.txt` 就提取并给「地图配置」兜底；真机 3/3 有标签 |
| D10 | **`resource/` 非 ui 无类别** | 库内 `resource/clientscheme.res`、`resource/l4d360ui_*.txt` | 只认 `resource/ui/` | W5 ✅ `resource/**`→「UI」；真机 17/17 命中 |
| D11 | **测试夹具路径与本体不符** | 近战真实路径是 `models/weapons/melee/w_*.mdl`，现有回归测试用 `models/w_models/weapons/w_katana.mdl` | 测试通过 ≠ 真机命中；掩盖了 D8 一类问题 | W3 ✅ 夹具全部换成本体真实路径（`tag_rules_regression_test.go` 的 13 把近战 + `TestOfficialWeaponAggregateTags`） |
| D12 | **「载入画面」规则永远不可能命中** | 规则前缀写 `materials/vgui/loadingscreen/`；本体是平铺文件 `materials/vgui/loadingscreen_*.vmt\|vtf`（索引实测：`materials/vgui/loading*` = 64 条、`**/loadingscreen*` = 60 条，目录形式 0 条） | W2 校验器首次运行即发现；修复后真机 **+41 / −0**（41 个 Mod 找回「载入画面」） | W2 ✅ |
| D13 | **`.meta` 自定义标签硬覆盖自动识别结果** | App 层 `processVPKFileWithCache` 直接 `vpkFile.SecondaryTags = meta.SecondaryTags`。真机案例 `workshop/3582222262.vpk`：`.meta` 写的是 `["武器","普通感染者"]`，而解析器对同一文件给出 `[HUD UI 冲锋枪 声音 所有枪械 模型 消音 贴图]` —— 自动结果被整份丢弃 | **所有带 `.meta` 的 Mod（工坊侧 942 个）都受影响**：解析器再准也显示不出来。修复 = 与文件名标签共用 `parser.ApplyCustomTagOverride`（自定义一级优先、自动一级降级为二级保留、二级取并集）；真机该案例 +8 标签，全库 +13 标签、0 消失 | W4 ✅ |

> 说明：D1–D11 是「本轮查出来的既有问题」，不是"用户现在觉得少了标签"。
> 其中 **D1 属于错误标签**：`w_pumpshotgun_A.mdl` 是 Chrome 连喷，不是木喷。修复方式 =
> **新增「铁喷」+ 在 allowlist 登记「木喷」移除**（两者都写清本体证据）；
> D2/D3/D4/D8 属于纯漏判，只新增标签，不走 allowlist。

## 3. 现状基线（2026-09-30 真机导出 2858 个 Mod）

| 指标 | 数值 |
| --- | --- |
| Mod 总数 | 2858（addons 根目录 + workshop 递归 + disabled 递归；其它自建子目录不参与） |
| 至少有一个二级标签 | 2797 / 2858（97.9%） |
| 人物类有角色标签 | 597 / 597（100%） |
| 武器类有具体武器标签 | 930 / 967 |
| 零标签 Mod | 61（其中 56 个是 `particles/`） |
| 主体识别置信度 | 高 1941 / 低 917 |
| 现有规则文件 | `internal/parser/content_tag_rules.go`、`weapon_parser.go`、`character_parser.go`、`archive_index.go`、`subject_parser.go` |
| 现有护栏 | `upstream_parity_test.go`、`tag_rules_regression_test.go`（用例为合成路径） |

现有架构（保留，不推翻）：单遍 `archivePathIndex` → `determineVPKType` → 类型专属解析 → `collectSupplementaryTypeTags`
→ `mergeTagSet(contentTags)` → 文件名自定义标签**只增不减**（`applyFilenameTagOverrides`）。

## 4. 目标架构

### 4.1 数据层：StockIndex（本体索引）

新增 `internal/gamedata/stockindex.go`：

- 扫描 `update` / `left4dead2_dlc3` / `left4dead2_dlc2` / `left4dead2_dlc1` / `left4dead2` 的 `pak01_dir.vpk` **条目**
  以及松散文件（`sound/`、`scripts/`、`cfg/`、`resource/`、`materials/`、`models/`、`particles/`）。
  真机实测：仅 VPK 条目 = 22273 + 1302 + 6036 + 6918 + 3634 = **40163 条**；语音等资源只存在于松散目录，**必须两边都扫**。
- 缓存到 `%APPDATA%/LytVPK/stock_index.json`（带 `schemaVersion` + 游戏 build 时间戳 + 挂载点列表）；
  游戏目录不可用时**静默降级**为「无本体索引」，所有依赖它的证据通道自动跳过（绝不因缺少本体而少标）。
- 提供 `Lookup(path) (kind, ok)`、`Exists(path)`、`Glob(pattern)`（内部把 glob 预编译成前缀/后缀匹配，避免每次全表扫描）。

用途：精确替换目标、实体词典、**规则解析校验**、新实体发现。

### 4.2 规则层：声明式实体表

新增 `internal/gamedata/catalog/*.json`（`//go:embed` 进 EXE，与 `frontend/dist` 同一模式），schema：

```jsonc
{
  "schemaVersion": 1,
  "entities": [
    {
      "id": "weapon.shotgun.chrome",
      "name": "铁喷",
      "category": "武器",              // 对应现有 PrimaryTag 词表
      "group": "霰弹枪",               // 对应现有 weaponCategoryTag
      "official": true,
      "slots": [                       // 槽位 = 可独立替换的部件（学 l4d2-mod-manager）
        { "kind": "worldmodel", "globs": ["models/w_models/weapons/w_pumpshotgun_A.*"] },
        { "kind": "viewmodel",  "globs": ["models/v_models/v_shotgun_chrome.*"] },
        { "kind": "script",     "globs": ["scripts/weapon_shotgun_chrome.txt"] },
        { "kind": "sound",      "globs": ["sound/weapons/shotgun_chrome/**"] }
      ],
      "keywords": ["shotgun_chrome", "chrome"],   // 兜底：非本体路径下的 token 证据
      "aliases": ["Chrome Shotgun"]
    }
  ],
  "fileKinds": [                       // 文件类别标签（学 NekoVpk）
    { "tag": "粒子特效", "globs": ["particles/**"] },
    { "tag": "声音",     "globs": ["sound/**"] },
    { "tag": "地图资源", "globs": ["maps/**"] },
    { "tag": "天空盒",   "globs": ["materials/skybox/**"] },
    { "tag": "VScript",  "globs": ["scripts/vscripts/**"] },
    { "tag": "喷涂",     "globs": ["scripts/sprays_manifest.txt"] }
  ]
}
```

**生成方式（关键）**：`entities` 不靠手抄，而是由 `tools/gen_catalog`（Go 程序或 `go test` 驱动的生成器）
读游戏本体脚本生成：

- `scripts/weapon_*.txt`：43 个实体脚本，`printname` + `playermodel` + `viewmodel` + `CharacterViewmodelAddon` 全部解析；
- `scripts/melee/*.txt`：13 把近战（`baseball_bat cricket_bat crowbar electric_guitar fireaxe frying_pan katana knife machete tonfa golfclub pitchfork shovel`）；
- `scripts/weapon_*_claw.txt`：7 个特感爪子的 `v_claw_*` / `v_*_arms` 归属；
- 生成后**人工只补中文名与别名**，路径一律来自本体。

校验（学 l4d2-mod-manager 的 "loud failure"）：构建期对每条 glob 跑 `StockIndex.Glob`，
**解析到 0 个文件就失败**，并打印 `entity/slot` 与 glob；CI 用固定夹具验证该逻辑。

### 4.3 证据层：多通道合并 + 证据分级

```go
type EvidenceLevel int

const (
    EvidenceInferred EvidenceLevel = iota // 关键词/标题/命名空间（最弱）
    EvidencePattern                       // 本体基名 token、glob 命中（中）
    EvidenceExact                         // == 本体路径 / .mdl 材质表 / 官方脚本（最强）
)

type TagEvidence struct {
    Tag    string        `json:"tag"`
    Level  EvidenceLevel `json:"level"`
    Rule   string        `json:"rule"`    // 规则 id，便于回归定位
    Source []string      `json:"source"`  // 命中的真实路径（最多 N 条）
}
```

通道（**全部为并集，任何一条命中就加标签**）：

1. **Exact**：Mod 路径 == 本体路径 → 直接映射到实体（D1/D2/D3 由它根治）。
2. **Pattern**：本体基名 token（`w_rifle_ak47`、`smg_silenced`…）出现在**任意目录**下即算证据；
   用于 `materials/models/codm/ice/jc/rifle_ak47/*.vmt`、`particles/…ak47.pcf` 这类"参数包/特效包"。
3. **MaterialTable**：解析 Mod 自带 `.mdl` 的材质表（学 l4d2-mod-manager `mdl.py`），把"只换贴图"的包绑回实体。
4. **Title/Desc**：`addoninfo` 标题/描述的型号 token，**对所有主类型生效**，且**收集全部命中**（修 D7）。
5. **SuiteNamespace**：复用现有 `structure.resourceRoots`（作者/套件命名空间）——同命名空间下已有「AK47」标签时，
   该命名空间的参数包继承「AK47」（修 29 个"标题有型号但无标签"的情形）。
6. **FileKind**：`particles/**` 等文件类别标签（修 D6/D10）。
7. **CharacterAssets**：角色面板/vgui 选人图/手臂/死亡姿势（学 NekoVpk 的 Bill/Zoey 条目）。

标签集合 = 七通道并集。`EvidenceLevel` **不参与筛选**，只用于排序、UI 展示与回归对比。
（实现口径：`.mdl` 材质表按"材质名 token 匹配本体基名"计为 `pattern`；
只有"路径 == 本体路径"或官方脚本声明才计 `exact`。）

### 4.4 输出层：标签不变式

- `VPKFile.SecondaryTags` 仍是唯一对外的标签集合，语义不变（前端与 Wails 绑定不改）。
- 新增只读字段：`VPKFile.TagEvidence []TagEvidence`（同步更新 `frontend/wailsjs/go/models.ts` 与前端类型）。
- **不变式**：对任意 Mod，新版本标签集合 ⊇ 旧版本标签集合。违反即视为回归（见 §6）。

## 5. 工作包

### W0 护栏先行（先做，后改规则）

- 新增 CLI：`--export-tag-baseline <path>`（导出 `name → sorted tags` 的最小 JSON）与
  `--check-tag-regression <baseline> [--out report.json]`：逐 Mod diff，**只允许新增标签**，
  出现消失标签时打印 `(mod, 消失的标签, 旧证据, 新证据)` 并以退出码 1 失败。
- 真机基线不进公开仓库（含用户 Mod 名，隐私）；CI 使用 `internal/parser/testdata/tag_recall_fixtures.json`
  的**合成夹具**，维护者用真机基线做发版前检查（写进 `docs/development/manual-verification.md`）。
- 验收：故意删掉一条规则 → CI 失败并准确指出受影响的 Mod/标签；恢复后通过。

### W1 StockIndex

- 实现 §4.1；接入 `App.Startup` 的后台构建（协程池，不阻塞扫描）；扫描报告写入 catalog。
- CLI：`--build-stock-index [--game-dir <path>]`（只读，供维护者/CI 使用）。
- 验收：真机导出条目数 ≥ 40000；`sound/player/hunter/voice/**` 等松散路径可见；游戏目录缺失时全部降级不报错。

### W2 规则表声明化 + 解析校验

- 把 `contentTagRules` / `weaponPathRules` / character 表迁移到 §4.2 的 JSON（保留 Go 侧解析器与合并逻辑）。
- 迁移采用**双读**：JSON 表 + 现有 Go 表取并集，跑一轮真机 diff 确认无标签消失后再删旧表（分两个 PR）。
- 新增测试：每条 glob 在夹具索引中解析 ≥1；夹具路径全部替换为**本体真实路径**（修 D11）。
- 验收：`go test ./...` 全绿 + 真机 diff 无消失标签。

### W3 脚本/本体驱动的实体表

- 生成器产出 `entities`（§4.2），修 D1（Chrome）、D2（烟花盒）、D3（可乐）、D4（特感爪子）、D8（高尔夫球杆）。
- 特感 claw → 角色归属：`v_claw_hunter` → Hunter，`v_*_arms` → 对应特感。
- 验收：新增 `TestStockScriptCoverage`：43 个 `weapon_*.txt` 实体 + 13 近战，逐个断言「本体 playermodel/viewmodel 路径能命中至少一个标签」。

### W4 多通道证据

- 实现 §4.3 的通道 2/3/4/5；`.mdl` 解析器（只读头部，限制读取字节数，失败静默）。
- 修 D7：`DetectWeaponTypeFromMetadata` 改为收集全部命中；标题证据对所有主类型生效。
- 验收：真机抽样 200 个"标题含型号但当前无对应标签"的 Mod，命中率 ≥90%，且 diff 无消失标签。

### W5 新类别与漏判路径

- `particles/**`（D6）、`sound/**`、`maps/**`、`materials/skybox/**`、`scripts/vscripts/**`、`resource/` 非 ui（D10）；
- 特感语音布局（D5）：`sound/player/<si>/{voice,attack,hit,death,idle,miss,alert,...}` 与 `sound/player/pz/**`；
- missions-only（D9）：`index.missionFiles` 非空即解析战役/模式，无 BSP 时给「地图配置」兜底标签。
- 验收：196 个 `particles/` Mod 全部拿到「粒子特效」；72 个「语音包」Mod 中至少 60 个拿到角色身份；diff 无消失标签。

### W6 呈现与对外清单

- UI：Mod 详情显示「为什么打这个标签」（`TagEvidence`）；筛选行为不变。
- catalog 导出（给外部智能体）增加 `tagEvidence` 与 `stockIndexVersion`；
  沿用 `catalog_now.txt` 里的约定"不要一次性把整个 mods 数组读进上下文"，新增字段保持紧凑（每条证据 ≤3 路径）。
- 分组推导复用：同一 `resourceRoot` 的标签可参与 `internal/grouping` 的 suite 信号（只增不减）。

## 6. 不变式与回归护栏（"不能少标"的机器保证）

1. **标签黄金快照 diff**（W0）：真机 + CI 夹具双轨；只允许新增。
2. **本体覆盖断言**（W3）：每个本体实体/槽位至少被一条规则覆盖；glob 解析 0 命中即失败。
3. **召回夹具**：`internal/parser/testdata/tag_recall_fixtures.json` 按类别记录"必须命中"的合成样例
   （路径取自本体真实布局，不是编造），新增类别必须同时补夹具。
4. **降级不降标签**：StockIndex 缺失、`.mdl` 解析失败、脚本缺失等所有异常路径都只影响"新增证据",
   绝不影响既有标签产出（用测试断言）。
5. **发布前检查**：真机跑 `--check-tag-regression`，结果写入 Release 说明。

## 7. 里程碑

| 里程碑 | 内容 | 完成判据 |
| --- | --- | --- |
| M0 | 本文档 + 目标模式启动 | 文档入库、goal 建立 |
| M1 | W0 + W1 | 护栏可用；本体索引真机 ≥40000 条 |
| M2 | W2 + W3 | D1/D2/D3/D4/D8 修复；双读迁移完成；无消失标签 |
| M3 | W5 | D5/D6/D9/D10 修复；particles/语音包实测达标 |
| M4 | W4 | D7 修复；标题/材质表通道生效 |
| M5 | W6 | 证据字段进 UI 与 catalog 导出 |

每个里程碑必须：`go test ./...` 全绿、`frontend` 侧 `npm run build` 通过、真机 diff 无消失标签。

## 8. Non-goals（明确不做）

- 不改 `PrimaryTag` 词表（地图/人物/武器/其他）；本 Fork 的一级分类语义不动。
- 不做深度学习/模型推理；所有证据必须可解释、可回归。
- 不引入第三方运行期依赖（`.mdl`/KeyValues 解析自己实现，参考实现只用来看思路，不复制代码）。
- 不因"看起来更准"而删除任何现有标签；需要区分强弱时用 `EvidenceLevel`，不用删标签表达。
- 不把用户的真实 Mod 清单/基线提交进公开仓库。

## 9. 附录：本体权威映射（节选，完整记录见 `.tmp-tag-audit/weapon_script_map.txt`）

| weapon 脚本 | playermodel（世界模型） | viewmodel | 当前标签 | 状态 |
| --- | --- | --- | --- | --- |
| `weapon_pumpshotgun` | `models/w_models/weapons/w_shotgun.mdl` | `models/v_models/v_pumpshotgun.mdl` | 木喷 | ✅ |
| `weapon_shotgun_chrome` | `models/w_models/weapons/w_pumpshotgun_A.mdl` | `models/v_models/v_shotgun_chrome.mdl` | **木喷（错）** | ❌ D1 |
| `weapon_smg` | `models/w_models/weapons/w_smg_uzi.mdl` | `models/v_models/v_smg.mdl` | 乌兹 | ✅ |
| `weapon_smg_silenced` | `models/w_models/weapons/w_smg_a.mdl` | `models/v_models/v_silenced_smg.mdl` | 消音 | ✅ |
| `weapon_smg_mp5` | `models/w_models/weapons/w_smg_mp5.mdl` | `models/v_models/v_smg_mp5.mdl` | MP5 | ✅ |
| `weapon_autoshotgun` | `models/w_models/weapons/w_autoshot_m4super.mdl` | `models/v_models/v_autoshotgun.mdl` | 一代连喷 | ✅ |
| `weapon_shotgun_spas` | `models/w_models/weapons/w_shotgun_spas.mdl` | `models/v_models/v_shotgun_spas.mdl` | 二代连喷 | ✅ |
| `weapon_rifle` | `models/w_models/weapons/w_rifle_m16a2.mdl` | `models/v_models/v_rifle.mdl` | M16 | ✅ |
| `weapon_rifle_ak47` | `models/w_models/weapons/w_rifle_ak47.mdl` | `models/v_models/v_rifle_AK47.mdl` | AK47 | ✅ |
| `weapon_rifle_desert` | `models/w_models/weapons/w_desert_rifle.mdl` | `models/v_models/v_desert_rifle.mdl` | 三连发 | ✅ |
| `weapon_rifle_sg552` | `models/w_models/weapons/w_rifle_sg552.mdl` | `models/v_models/v_rif_sg552.mdl` | sg552 | ✅ |
| `weapon_rifle_m60`（dlc1） | `models/w_models/weapons/w_m60.mdl` | `models/v_models/v_m60.mdl` | M60 | ✅ |
| `weapon_sniper_awp` | `models/w_models/weapons/w_sniper_awp.mdl` | `models/v_models/v_snip_awp.mdl` | 大狙 | ✅ |
| `weapon_sniper_military` | `models/w_models/weapons/w_sniper_military.mdl` | `models/v_models/v_sniper_military.mdl` | 军狙 | ✅ |
| `weapon_sniper_scout` | `models/w_models/weapons/w_sniper_scout.mdl` | `models/v_models/v_snip_scout.mdl` | 鸟狙 | ✅ |
| `weapon_hunting_rifle` | `models/w_models/weapons/w_sniper_mini14.mdl` | `models/v_models/v_huntingrifle.mdl` | 猎枪 | ✅ |
| `weapon_pistol` | `models/w_models/weapons/w_pistol_A.mdl` | `models/v_models/v_pistolA.mdl` | 小手枪 | ✅ |
| `weapon_pistol_magnum` | `models/w_models/weapons/w_desert_eagle.mdl` | `models/v_models/v_desert_eagle.mdl` | 马格南 | ✅ |
| `weapon_grenade_launcher` | `models/w_models/weapons/w_grenade_launcher.mdl` | `models/v_models/v_grenade_launcher.mdl` | 榴弹发射器 | ✅ |
| `weapon_chainsaw` | `models/weapons/melee/w_chainsaw.mdl` | `models/weapons/melee/v_chainsaw.mdl` | 电锯 | ✅ |
| `melee/golfclub`（dlc1） | `models/weapons/melee/w_golfclub.mdl` | `models/weapons/melee/v_golfclub.mdl` | **不命中** | ❌ D8 |
| `melee/pitchfork`（update） | `models/weapons/melee/w_pitchfork.mdl` | — | 草叉 | ✅ |
| `melee/shovel`（update） | `models/weapons/melee/w_shovel.mdl` | — | 铁铲 | ✅ |
| `melee/baseball_bat` | `models/weapons/melee/w_bat.mdl` | `models/weapons/melee/v_bat.mdl` | 棒球棍 | ✅ |
| `weapon_first_aid_kit` | `models/w_models/weapons/w_eq_Medkit.mdl` | `models/v_models/v_medkit.mdl` | 医疗包 | ✅ |
| `weapon_fireworkcrate` | `models/props_junk/explosive_box001.mdl` | — | **不命中** | ❌ D2 |
| `weapon_cola_bottles` | `models/w_models/weapons/w_cola.mdl` | `models/v_models/v_cola.mdl` | — | ❌ D3 |
| `weapon_gascan` | `models/props_junk/gascan001a.mdl` | — | 汽油桶 | ✅ |
| `weapon_propanetank` | `models/props_junk/propanecanister001a.mdl` | — | 煤气罐 | ✅ |
| `weapon_oxygentank` | `models/props_equipment/oxygentank01.mdl` | — | 氧气罐 | ✅ |
| `weapon_gnome` | `models/props_junk/gnome.mdl` | `models/weapons/melee/v_gnome.mdl` | 侏儒 | ✅ |
| `weapon_{si}_claw` ×7 | `models/v_models/weapons/v_claw_{Boomer,Hunter,Smoker,hulk}.mdl`、`models/weapons/arms/v_{charger,jockey,spitter}_arms.mdl` | — | **无角色标签** | ❌ D4 |

## 10. 复现命令（维护者）

```powershell
# 1) 导出真机标签基线（不进仓库）
.\build\bin\LytVPK-Community-Fork.exe --export-tag-baseline "$env:TEMP\tag-baseline.json"

# 2) 改动后对比：只允许新增标签
.\build\bin\LytVPK-Community-Fork.exe --check-tag-regression "$env:TEMP\tag-baseline.json" --out "$env:TEMP\tag-report.json"

# 3) 重建本体索引（可选，游戏更新后）
.\build\bin\LytVPK-Community-Fork.exe --build-stock-index

# 4) 常规验证
go test ./... -count=1
cd frontend; npm run build
```
