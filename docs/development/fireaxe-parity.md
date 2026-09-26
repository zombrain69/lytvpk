# FireAxe 能力对照与落地映射（维护者文档）

> 对照基线：`ktxiaok/FireAxe` commit `f8aa1cf1391802c64525412fdf326dbe0ecc98b1`（v0.7.3），
> 许可 Apache-2.0。
> 本文只记录**设计借鉴点与落点**。所有实现均为 Go / 原生 JS 重写，不复制上游代码。
> 本文是维护者文档，不进入公开文档站侧边栏。

## 0. 阅读方式

每一行的"FireAxe 证据"都指向真实文件与行号，便于回溯核对：
`FireAxe.Core/AddonConflictUtils.cs:96` 表示该文件第 96 行附近。
本地参照源码位于仓库之外，请用下面的命令重新获取：

```powershell
git clone https://github.com/ktxiaok/FireAxe.git
git -C FireAxe checkout f8aa1cf1391802c64525412fdf326dbe0ecc98b1
```

## 1.0 完整对照表（一页版）

想看"FireAxe 有什么 / 我们落在哪 / 什么状态"，读这一张就够；每一行的细节证据在 §1、§4.1、§4.4。
状态口径：
**已对齐**=按上游语义落地并有自动化测试；**已对齐并加强**=落地的同时比上游更宽或更严；
**已覆盖**=本项目早就有等价或更强的能力（本轮只补记）；**有意取舍 / 有意不做**=列了理由，不做；
**本轮补齐**=这一轮新发现（来自上游 CHANGELOG 的能力级清单）并已实现。

| # | FireAxe 能力 | 本项目落点 | 状态 |
| --- | --- | --- | --- |
| 1 | 显式可编辑优先级 `Priority` | `priority.json` + `SetModPriority` / `ClearModPriority` | 已对齐 |
| 2 | 层级优先级累加 `PriorityInHierarchy` | `pathTier(g)` 沿上级链累加；跨链取 `min` | 已对齐并加强 |
| 3 | 冲突只在同优先级内判定 | `decideConflictOwners`：同层=真冲突，全不同=覆盖 | 已对齐并加强 |
| 4 | 内置忽略清单（`addoninfo.*` 等） | `isIgnoredConflictFile` + 设置页全局清单 | 已对齐 |
| 5 | 忽略集合取并集 | 内置 ∪ 全局 ∪ 单 Mod | 已对齐 |
| 6 | 单个 Mod 的忽略文件 | `ignore.json` + `GetModIgnoreFiles` / `SetModIgnoreFiles` | 已对齐 |
| 7 | 备份轮转 `BackUpIfNeed` | 最短间隔 + 内容相同跳过 + 数量上限 + 溢出进回收站 | 已对齐 |
| 8 | 变更驱动的问题失效 + 自动修复建议 | `conflict_recheck.go`：失效 → 按需重算 + 建议（不自动改文件） | 已对齐 |
| 9 | Problem 自动修复分级 | 体检白名单（重复条目 / 缺失条目 / 依赖未开启）+ 逐行修复按钮 | 已对齐（白名单更窄） |
| 10 | 组启用策略 `all/off/single/random` | 策略组 + 可选自动联动 | 已对齐 |
| 11 | 策略可满足性预检 | `CheckModStrategyGroupApply` 应用前拦不可执行 | 已对齐 |
| 12 | 组权重 | `Tier *int`（未设置= nil）+ 祖先累加 + 跨链 `min` | 已对齐 |
| 13 | push 写盘顺序（按优先级降序） | 按有效分层稳定排序写 `addonlist.txt` | 已对齐 |
| 14 | push 事务化 | `runAddonListTransaction`：写盘 → 派生刷新 → 快照同步，失败回滚 | 已对齐 |
| 15 | 方案快照 | 启用方案（含分层、策略组、依赖） | 已对齐 |
| 16 | 本地记录 schema 版本化 | 5 个 JSON 带 `schemaVersion` + 迁移器 + 读时自愈 | 已对齐 |
| 17 | 工坊自动更新 | `shouldAutoRedownload`（默认关）+ 检查任务 in-flight 去重 | 已对齐 |
| 18 | 工坊官方资料（tags / 统计） | `EnrichWorkshopMetadata` 走官方接口写进 `.meta` | 已对齐（上游没有官方抓取） |
| 19 | 循环引用检测 | `dependencyCycles` + 体检项 `dependency_cycle` | 已对齐 |
| 20 | 文件名安全化 | 改名前置校验（非法字符 / 控制字符 / 结尾空格点 / 保留设备名） | 已对齐 |
| 21 | 下载服务（进度 + 暂停 + 续传） | 分块下载 + 任务快照 1s 节流 + 区块检查点续传 | 已对齐并加强 |
| 22 | 工坊合集实体化 | `collections.json` + 跟随节点刷新/补下 | 已对齐 |
| 23 | 树形分组 / 嵌套层级 | `ParentID` + 最多 4 层 + 环校验 | 已对齐 |
| 24 | 树视图拖放排序 | 行级拖放 + 纯函数落点判定 + 后端改上级/排序 | 已对齐 |
| 25 | 容器内名称唯一 | 建组自动加序号；重命名撞名报错 | 已对齐 |
| 26 | 子节点问题向父节点汇报 | 父组行显示"子组里 N 个缺失（涉及 M 个子组）" | 已对齐 |
| 27 | VPK 内嵌预览图 | 索引 `addonimage.jpg` / `addonpreview*` + 三套预览接口 | 已覆盖（更宽） |
| 28 | 输出文件自动去重命名 | 打包/修复另存自动加序号；ZIP 包内 basename 去重 | 已覆盖 |
| 29 | 任务期间的移动互斥 | `file_op_gate.go`：移动/删除/打包共用 TryLock + 忙碌状态进界面 | 已对齐 |
| 30 | 导入/移动失败逐项报告 | `MoveResult.errors[]` + 界面前 3 条 + "另有 N 条" | 已对齐并加强 |
| 31 | 依赖问题的自动修复 | `dependency_disabled` 带 Target + 一键启用依赖 + 写前备份 | 已对齐 |
| 32 | 文件类型不符的独立诊断 | 体检项 `file_type_mismatch` | 已对齐 |
| 33 | 工坊记录身份一致性 | 体检项 `meta_id_mismatch` | 已对齐 |
| 34 | 受管根内的路径守卫 | `path_guard.go` + 体检项 `outside_root` | 已对齐并加强 |
| 35 | 失效目标上的异步任务自动放弃（`ValidRef`/`ValidTaskCreator`） | `task_target_guard.go` + `SkippedStale` | 已对齐 |
| 36 | 游戏目录自动探测 | 注册表 + `libraryfolders.vdf` + 盘符兜底 | 已对齐并加强 |
| 37 | 对象解释层（`FireAxe.GUI`） | `action-explanation.mjs`：失败=人话+下一步、灰按钮=为什么+出路 | 已对齐 |
| 38 | 编辑快捷键（`Ctrl+F` / `Delete` / `F2` / `Enter` / `Esc`） | `Ctrl+F`/`Enter`/`Esc` 早有；**本轮补齐 F2 重命名与 Delete 删除**（多选走批量删除） | 本轮补齐 |
| 39 | 主窗口宽高 + 最大化状态记忆 | `mainWindowWidth/Height/Maximised` + 按屏幕钳制后恢复 | 本轮补齐 |
| 40 | File Cleaner：工坊条目的冗余 VPK | 体检项 `duplicate_vpk_copy`（只提示 + 指引，删除交给用户/归档管理） | 本轮补齐（不自动删） |
| 41 | 单例模式 | `StartSingletonListener` | 已覆盖 |
| 42 | 代理 / 代理凭据设置 | 优选 IP + 固定 IP + 系统代理 | 已覆盖 |
| 43 | 视图/排序/布局记忆 | `displayMode` / `filterLayoutMode` / 排序记忆 | 已覆盖 |
| 44 | 右键"打开工坊页面" | 工坊跳转目标（镜像 / 官方） | 已覆盖 |
| 45 | 高级面板（优先级/标签/依赖/图片集中编辑） | Mod 详情窗口（比上游更全：忽略清单、依赖、分层、预览） | 已覆盖并加强 |
| 46 | 就地按作品标题批量改名（Name Auto Setter） | 只有 ZIP 导出的"按名称重命名"（不动原文件）+ 单个 F2 改名 | 有意不做 |
| 47 | 猜文件名式修复（File Name Fixer） | 体检给出明确诊断（缺文件 / 类型不符 / 工坊记录不一致）+ 手动 F2 改名 | 有意不做 |
| 48 | 空目录清理 / 下载临时文件清理 | 下载临时文件：启动后自动清 7 天前残留；空目录：不清理 | 部分已覆盖 + 有意不做 |
| 49 | 自定义预览图（手动指定任意图片） | 支持"把图片按同名放在 VPK 旁边"（上游 issue #17 的替代方案）；不引入"图片路径"第二套真相 | 已覆盖 + 有意不做 |
| 50 | `Workshop VPK Finder`（找工坊 VPK 实体） | 本项目直接管理 `workshop` 目录，不存在"找不到实体"的场景 | 不适用 |
| 51 | AppImage / Linux 发布 | 本项目是 Windows 桌面工具（直接管理游戏真实文件） | 不适用 |
| 52 | 剪贴板工坊链接自动识别（v0.4.0） | `App.CheckClipboardWorkshopLink` + 前端轮询（开关 + 前台 + 间隔 + 去重）+ 设置页开关 | 本轮补齐 |
| 53 | `Ctrl+X` / `Ctrl+V` 移动（v0.5.1） | 「待移动」标记 + 复用「移动到…」目录选择；快捷键总览同步登记 | 本轮补齐（第 17 轮核对发现） |
| 54 | 自定义打开文件 / 目录的外部程序（v0.7.2 process file customization） | 设置 → 界面设置 →「打开文件方式」：程序 + 参数模板（`{path}` / `{dir}` / `{name}`，兼容 `{0}`）；留空 = 系统默认（行为不变） | 已对齐（第 18 轮补齐） |

**逐项理由**（46 / 47 / 48 / 49 为什么不做）：

- **46 批量按标题改名**：工坊 VPK 的文件名就是作品 ID，改名会破坏 `.meta` 配对、更新检测与
  `workshop\<ID>.vpk` 约定；本地 Mod 想改名用 F2 即可（改名会自动迁移策略组/分层/依赖/忽略清单的引用）。
- **47 猜文件名修复**：自动猜错会把正确文件改坏；本项目给的是明确诊断 + 一条命令就能改的入口。
- **48 空目录清理**：L4D2 只加载 `addons` 根与 `workshop`/`disabled` 下的 VPK，子目录不在管理范围，
  空目录既不占空间也不影响加载；递归删用户子目录的风险大于收益。
- **49 手动指定预览图**：图片按同名放在 VPK 旁边就能生效、还会跟着移动；再引入一份"图片路径"记录
  等于给"文件在哪"造第二套真相。
- **54 自定义外部程序（已实现）**：默认留空 → `OpenFileLocation` 完全保持原来的系统默认行为；
  填了程序才替换（`internal/app/open_with.go`：`buildOpenWithCommand` 纯函数组装命令、
  `validateOpenWithProgram` 拦"带路径但文件不存在"、`startOpenWithCommand` 不经 shell 直接启动）。
  参数模板支持 `{path}` / `{dir}` / `{name}`（兼容 FireAxe 的 `{0}`），模板里没有占位符时自动把完整路径追加到末尾。

## 1. FireAxe 概念 → 本项目落点

| FireAxe 概念 | FireAxe 证据（file:line） | 本项目落点 | 状态 |
| --- | --- | --- | --- |
| 显式可编辑优先级 `Priority` | `AddonNode.cs:139`（setter 标记 `RequestSave`）、`AddonNodeSave.cs:22`（持久化字段） | `internal/app/priority.go` 的 `ModPriorityEntry` + `priority.json`；`SetModPriority` / `ClearModPriority` | 已对齐（Task 1） |
| 层级优先级累加 `PriorityInHierarchy` | `AddonNode.cs:154-168`（自身 + 全部祖先的 `Priority` 求和） | `pathTier(g)`：沿上级链**累加**组权重（父 -2 + 子 -3 ⇒ -5，子组自动继承父组权重）；跨链之间取 `min` 收敛（可重叠集合语义，见设计文档第 3 节） | 已对齐（分支内累加；跨链取 min 是本项目特化） |
| 冲突只在同优先级内判定 | `AddonConflictUtils.cs:96`（按 `priority` 分桶）、`:249-262`（同优先级才互为冲突方） | `decideConflictOwners`：有效分层相同 → 真冲突；全部不同 → 覆盖关系 | 已对齐并加强（Task 1） |
| 内置忽略清单 | `AddonConflictUtils.cs:48-55`（`addoninfo.*`、`sound/sound.cache`、4 个 `*_addon.nut`） | `conflict.go` 的 `isIgnoredConflictFile` + 设置页全局清单 | 已对齐，逐项补齐（Task 2） |
| 忽略集合取并集 | `AddonConflictUtils.cs:11-46`（静态集合 + 动态 provider 依次命中即忽略） | 内置规则 ∪ 全局清单 ∪ 单 Mod 清单 | 已对齐（Task 2） |
| 单个 Mod 的忽略文件 | `VpkAddon.cs:32`（`ConflictIgnoringFiles`）、`:270`/`:278`（保存/恢复）、`VpkAddonSave.cs:17` | `ignore.json` + `GetModIgnoreFiles` / `SetModIgnoreFiles` | 已对齐（Task 2） |
| 备份轮转 `BackUpIfNeed` | `AddonRoot.cs:972-1050`：最短间隔、与上一份内容相同则跳过、数量上限、溢出进回收站 | `internal/app/local_store_backup.go` | 已对齐（Task 3） |
| 变更驱动的问题失效与自动修复建议 | `Problem.cs:6-34`（`CanAutomaticallyFix` 默认 false、`ShouldFixAutomatically` 默认 false、`TryAutomaticallyFix` 调 `OnAutomaticallyFix`）、`IValidity.cs:5-8` | `conflict_recheck.go`：状态变化 → 失效 → 按需重算 + 修复建议（不自动改文件）；**自动修复分级**见下行 | 已对齐（Task 4） |
| 问题的"能不能自动修 / 该不该自动修" | `Problem.cs:11`（`CanAutomaticallyFix`）、`:13`（`ShouldFixAutomatically`）、`:15-25`（`TryAutomaticallyFix` 成功后 `IsValid=false`） | 体检项按 `kind` 分白名单：`duplicate_entry` / `missing_file` 可一键修复（写前先备份），其余仍只给建议；`healthIssueAutoFixAction` + 逐行「修复」按钮 | 已对齐（2026-09-24，白名单比上游窄） |
| 组启用策略 | `AddonGroup.cs:34`（`EnableStrategy`）、`:130`（`CheckEnableStrategy` → `AddonGroupEnableStrategyProblem.TryCreate`）、`:161-200`（应用策略） | `mod_groups.go` 的策略组（`all` / `off` / `single` / `single_random` + 可选自动联动）；**可满足性预检**见下行 | 已对齐，语义改为显式动作 |
| 策略可满足性预检 | `AddonGroup.cs:130`（`CheckEnableStrategy`：把"策略是否还成立"变成一个 Problem，而不是静默失效） | `internal/app/mod_group_apply_check.go`：`CheckModStrategyGroupApply` 在应用前算出可执行性（空组 / 成员全丢 / 全在 disabled），不可执行直接拦下并说明原因，可执行但有风险则列警告让用户确认 | 已对齐（2026-09-24） |
| 组权重 | `AddonNode.cs:154-168`（祖先链累加，等价于组权重） | `ModStrategyGroup.Tier *int` + `pathTier` 祖先链累加 + 跨链 `min` | 已对齐（2026-09-24 起与 FireAxe 同为累加） |
| push 写盘顺序 | `AddonRoot.cs:754`（`Push`）、`:899`（按 `PriorityInHierarchy` **降序**写 addonlist.txt） | `ApplyModPriorityLayers`：按有效分层**升序**稳定排序写回 | 已对齐，方向见 §3 待实测项 |
| push 的"事务化" | `AddonRoot.cs:754`（`Push()` 先算出完整目标列表再一次性落盘，任一步异常都不会留下半套结果） | `internal/app/addon_list_transaction.go`：`runAddonListTransaction` 把「写盘 → 派生状态刷新 → 受保护快照同步」包成一个事务，快照同步失败就用写前内容回滚并让派生步骤按旧内容重跑；已覆盖优先级应用、策略组应用、加载顺序、合并、依赖、体检修复、方案应用等全部写盘入口 | 已对齐（2026-09-24） |
| 方案快照 | `AddonRootSave.cs:6-13`、`AddonNodeSave.cs:7-30`（`IsEnabled` / `Priority` / `Tags` / `DependentAddonIds`） | `ModEnableProfile` + `IncludesAutomation`（含 `Priorities`） | 已对齐（Task 1） |
| 本地记录 schema 版本化 | `AddonRoot.cs:18`（`VersionFileName = ".addonrootversion"`）、`:1166-1168`（写存档同时写版本文件）、`:1199-1215`（读旧版本时做字段迁移） | `internal/app/local_store_schema.go`：`groups.json` / `priority.json` 带 `schemaVersion`，读取时迁移（补时间戳、丢重复 ID / 自指父级 / 悬空 parentId、成员去重、策略归一化、同键保留最后），写盘时盖当前版本且不降级 | 已对齐（2026-09-24） |
| 引用型节点 `RefAddonNode` | `RefAddonNode.cs:7`、`:21`（`SourceAddonId`）、`:207`（循环引用检查） | 不引入：同一 Mod 的多方案由"启用方案"覆盖 | 有意不做 |
| 符号链接 push | `AddonRoot.cs` `Push()` 内 `File.CreateSymbolicLink` | 不引入：本项目直接管理真实文件 | 有意不做 |
| 自动更新工坊条目 | `WorkshopVpkAddon.cs:88-115`（`IsAutoUpdate` / 继承根设置）、`:428-466`（`CheckDownloadAsync`：进行中的检查任务存进 `DownloadCheckTask`，重复调用复用同一个 Task） | `shouldAutoRedownload` / `maybeAutoRedownload`（默认关闭，只重试一次）+ `workshop_update_check.go` 的 `CheckModUpdates` in-flight 去重（并发触发只跑一次，其它调用等结果复用） | 已对齐（Task 6 + 2026-09-24） |
| 工坊条目的官方资料 | `PublishedFileUtils.cs`（`GetPublishedFileDetailsAsync`）、`PublishedFileDetails.cs:20-70`（`tags[].tag`、`subscriptions`、`favorited`、`lifetime_*`、`views`） | `internal/app/workshop_steam_details.go`：`EnrichWorkshopMetadata` / `EnrichAllWorkshopMetadata` 走 Steam 官方 `ISteamRemoteStorage/GetPublishedFileDetails`，把官方标签与统计写进 `.meta`（`steam_tags` / `subscriptions` / `favorited` / `views` …），并进入分组建议材料 `workshop.steamTags` 与智能体提示词 | 已对齐（2026-09-24，实测镜像接口没有 tags、官方接口有） |
| 循环引用/成环检测 | `AddonCircularRefProblem.cs`（`RefAddonNode` 互相引用时给出 Problem） | `dependencyCycles`（纯函数，DFS 找环 + 旋转去重）+ 体检项 `dependency_cycle`，报告写出完整链路 `甲 → 乙 → 甲` | 已对齐（2026-09-24；本项目没有 `RefAddonNode`，落地在"依赖声明"上） |
| 文件名安全化 | `FileSystemUtils.cs`（`SanitizeFileName` / `GetUniqueFileName` / `ThrowIfPathInvalid`）、`AddonNode.cs`（`SanitizeName`） | `windowsFileNameProblem` + `validateRenameInputFilename`：非法字符、控制字符、结尾空格/点、Windows 保留设备名都在重命名前拦下并给出中文原因；目标已存在时拒绝覆盖（原有行为） | 已对齐（2026-09-24，只校验不改名） |
| 下载服务 | `DownloadService.cs:523`（`Download(url, filePath)`）、`:230`（`Resume`）、`:244`（`Cancel`）、`:11`（`SaveDownloadProgressIntervalMs = 1000`）、`:374-384`（进度回调里超过 1s 才落盘） | 既有分块下载 + 任务列表 + 可注入下载启动器（测试不触网）；**新增** `internal/app/download_task_store.go`：`download_tasks.json`（带 `schemaVersion`）按 1s 节流落盘、内容相同跳过、原子替换；重启后未完成任务显示为「已中断」并可一键重试；入队按作品 ID / 目标文件去重 | 已对齐（Task 6 + 2026-09-24） |
| 工坊合集实体化 | `WorkshopCollectionUtils.cs:10`（`GetWorkshopCollectionContentAsync`，支持嵌套合集） | `workshop_collections.go`：`collections.json` + 跟随节点刷新/下载缺失成员 | 已对齐（Task 7） |
| 树形分组 / 嵌套层级 | `AddonGroup.cs` 的父子容器 + `AddonNode.Parent` | `mod_group_tree.go`（`ParentID` + 树构建 + 环/深度校验，最多 4 层）；「策略组管理」窗口每行有「＋ 子组」快捷入口与「上级分组」下拉；扁平视图仍为默认 | 已对齐并加强（Task 8 + 2026-09-24 快捷入口） |
| 树视图拖放排序 | FireAxe 的树视图交互（`AddonGroup` 的子节点容器 + 拖放移动） | `strategy-group-manager.js` 行级 `draggable` + `strategy-group-tree.mjs` 纯函数判定落点（放进去 / 排前 / 排后 / 回顶层，拒绝成环与超 4 层）；后端 `MoveModStrategyGroup`（改上级）+ `ReorderModStrategyGroups`（写同级顺序） | 已对齐（2026-09-24） |
| 依赖问题的自动修复 | `AddonDependencyProblem.cs:12`（`CanAutomaticallyFix => true`）、`:14`（`ShouldFixAutomatically => true`）、`:16-26`（`OnAutomaticallyFix` → `EnableAllDependencies()` 后再复查 `IsDependenciesAllEnabled`）。**全仓只有这一个类把 `CanAutomaticallyFix` 覆盖成 true** | `dependency_disabled` 体检项带上 `Target`（主 Mod 的 addonlist 键）+ 行内「修复」按钮 → `EnableModDependencies`；写盘前建 `before-dependency-fix` 备份并走 addonlist 事务 | 已对齐（2026-09-26；上游是"自动修"，本项目保留"用户确认后修"） |
| 路径类型不符的独立诊断 | `AddonFileMissingProblem.cs:9-14`（`AddonFileTypeMismatch = None / ShouldBeDirectory / ShouldBeFile`）、`:26`（`FileTypeMismatch` 属性） | 体检项 `file_type_mismatch`：`addonlist.txt` 记录的条目在磁盘上是**同名文件夹**时单独报一类（游戏只加载 `*.vpk` 文件），并在 `disabled` 里还有可用副本时把出路写进提示 | 已对齐（2026-09-26） |
| 本地工坊记录的身份一致性 | `WorkshopVpkMetaInfo.cs:7`（`PublishedFileId`）、`InvalidPublishedFileIdProblem.cs:5-16`、`WorkshopVpkAddon.cs:693-696`（`metaInfo.PublishedFileId != publishedFileId` 即判定本地记录不可信）、`:710-713`（记录的 `CurrentFile` 不存在也要重下） | 体检项 `meta_id_mismatch`：`workshop\<ID>.meta` 里的作品 ID 与文件名不是同一个作品时提示"更新检测会拿另一个作品的时间戳比较"，并指出用「抓取官方标签与统计」重建 | 已对齐（2026-09-26） |
| 失效目标上的异步任务自动放弃 | `ValidRef.cs:14-38`（取用时复查 `IsValid`，失效即清空引用）、`ValidTaskCreator.cs:22-36`（回写前 `StartNew`，目标失效返回 true 就整段 return）、`IValidityExtensions.cs:8-42`（`RegisterInvalidHandler`）、`WorkshopVpkAddon.cs:662-673` 与 `:730-740`（每个写盘点都包在守卫里） | `internal/app/task_target_guard.go` 的 `modTaskTarget`（捕获路径 + 大小 + 修改时间，回写前 `stillValid()` 复查）+ `runModUpdateCheck` 落点：检测期间 Mod 被移动 / 替换就**不写** `.meta`，数量记进 `UpdateCheckResult.SkippedStale` | 已对齐（2026-09-26；`IValidity` 家族的"有效引用 / 任务守卫"部分） |
| 游戏目录自动探测 | `GamePathUtils.cs:9-31`（`CheckValidity`：必须存在 `left4dead2` 子目录）、`:33-81`（`TryFind`：先读注册表 `SOFTWARE\WOW6432Node\Valve\Steam` 的 `InstallPath`，再逐盘找 `SteamLibrary\steamapps\common\Left 4 Dead 2`） | `internal/app/steam_locations.go`（纯函数：解析 `libraryfolders.vdf` 的两种格式、库候选、`addonsPathIfGame` 的"必须真的装了游戏"判断）+ `steam_locations_windows.go`（注册表读取，`xxx_windows.go` + `xxx_other.go` 双实现）+ `AutoDiscoverAddons` 改为"注册表 → 库清单 → 盘符扫描兜底" | 已对齐并加强（2026-09-26：多解析一层 `libraryfolders.vdf`，自定义库名也能找到） |
| 受管根内的路径守卫 | `FileSystemUtils.cs:90-113`（`IsValidPath` / `ThrowIfPathInvalid` → `InvalidFilePathException`）、`AddonNode.cs:405-413`（`value.StartsWith("..") \|\| Path.IsPathRooted(value)` → `FileOutOfAddonRootException`） | `internal/app/path_guard.go`：`managedFilePathProblem` 要求绝对路径、且落在 addons / workshop / disabled 之内（拒绝 `..` 逃逸、拒绝把受管目录本身当操作目标），接在 `DeleteVPKFile` / `MoveVpkFiles`（源）/ `ToggleVPKVisibility` 上；体检新增 `outside_root`：addonlist 条目解析到受管目录之外时单独报一类 | 已对齐并加强（2026-09-26：**目标目录保持自由** —— "移动到用户自选目录"是既有能力，只约束源文件） |
| 对象解释层（"为什么不能做 / 刚刚为什么失败"） | **`FireAxe.GUI`**：`ObjectExplanationManager.cs:27-49`（按类型链回退，最后一定给出一句话）、`ExceptionExplanationScene.cs`（场景：`Default` / `Input`）、`ExceptionExplanations.cs:12-44`（同一异常在不同场景给不同解释）、`AddonProblemExplanations.cs:10-30`（每种问题一句人话）；消费点见 `TaskOperationsProgressNotifier.cs:57`、`AddonImportResultViewModel.cs:48`、`Views/CommonMessageBoxes.cs:275` | `frontend/src/js/core/action-explanation.mjs`：`explainOperationError`（后端错误 → 人话 + 下一步，规则按顺序匹配、认不出保留原文）、`sceneFromErrorType`（按错误类型推断场景）、`explainActionAvailability`（按钮为什么灰的 + 出路）。接在 `toast.js`（全局错误事件）、`move-result-format.mjs`（逐项失败，保留文件名）、`operations.js`（游戏内开关 / 禁用前置检查）、`model-stats-scan.js`（禁用按钮状态与提示共用同一判断） | 已对齐（2026-09-26；比上游多：失败项保留文件名、"下一步怎么做"与原因一起给） |

## 2. 本项目已经更强的部分（不得退化）

以下能力是 FireAxe 没有或更弱的，本次补齐过程中必须保持现状。
**"守护测试"列就是"别丢失"的执行证据**：改动碰到这些能力时，对应测试会失败。

| 能力 | 证据位置 | 相对 FireAxe 的优势 | 守护测试 |
| --- | --- | --- | --- |
| 直接管理真实文件 | `internal/app/vpk_actions.go`、`internal/app/addon_list.go` | FireAxe 依赖符号链接 push；本项目直接读写游戏真实文件 | `internal/app/fireaxe_parity_e2e_test.go`（真实 VPK 夹具走导出方法）、`internal/app/path_guard_test.go`（只动受管目录） |
| addonlist 编码 / BOM 保真 + 原子写 | `internal/app/addon_list.go`（`writeAddonList`：GBK/ANSI/UTF-16 探测与回写） | FireAxe 直接以 UTF-8 重写 `addonlist.txt` | `internal/app/addon_list_test.go`、`internal/app/priority_test.go`（黄金回归：未分层时逐字节不变） |
| 多类型备份与运行时监控恢复 | `internal/app/addon_list_manager.go`、`internal/app/lifecycle.go` | FireAxe 只备份 `.addonroot` 序列化结果 | `internal/app/addon_list_manager_test.go`、`internal/app/local_store_backup_test.go`、`internal/app/addon_list_transaction_test.go` |
| 冲突严重度分级与可组合基线 | `internal/app/conflict.go`（`getConflictSeverity` + 基线规则） | FireAxe 只给出"同优先级即冲突"，没有严重度与基线范围 | `internal/app/conflict_priority_test.go`、`internal/app/conflict_index_capacity_test.go` |
| 工坊链路（翻译 / IP 优选 / 分块下载 / 转移 / 更新检测） | `internal/app/workshop_download.go`、`internal/app/workshop_transfer.go`、`internal/app/workshop_translate.go` | FireAxe 使用 Steam 客户端回调，缺少翻译与转移链路 | `internal/app/download_resume_test.go`、`internal/app/workshop_steam_details_test.go`、`internal/app/workshop_collections_test.go` |
| 服务器面板与工具箱 | `internal/app/server_panel.go`、`internal/app/archive_manager.go`、`internal/app/model_stats_scan.go`、`internal/app/autoexec.go` | FireAxe 无对应能力 | `internal/app/server_panel_test.go`、`internal/app/panel_upload_test.go`、`internal/app/model_stats_scan_test.go`、`internal/app/archive_manager_test.go`、`internal/app/vpk_pack_test.go`、`internal/app/vpk_unpack_test.go`、`internal/app/spray_test.go`、`internal/app/autoexec_test.go` |
| 体检与报告导出 | `internal/app/health_check.go`、`internal/app/problem_scan.go` | FireAxe 只有 Problem 体系，无报告导出 | `internal/app/health_check_test.go`、`internal/app/health_check_precision_test.go`、`frontend/src/js/features/settings/health-report-format.test.mjs` |
| 检索体验（6 面板统一语法 + 光标 + 提示同源） | `frontend/src/js/core/field-search.mjs`、`frontend/src/js/core/shortcuts.mjs` | FireAxe 只有一套搜索选项，没有跨面板统一与"为什么命中" | `frontend/src/js/core/search-consistency.test.mjs`、`internal/app/search_query_test.go`、`frontend/src/js/features/file-list/result-cursor.test.mjs` |
| 阅读舒适度（字号档位 + 三档对比度 + 不变量） | `frontend/src/js/core/reading-comfort.mjs`、`frontend/src/css/app/reading-comfort.css` | FireAxe 只有浅色/深色主题 | `frontend/src/js/core/reading-comfort.test.mjs`、`frontend/src/js/core/reading-contrast-css.test.mjs`、`frontend/src/js/core/font-size-units.test.mjs` |
| 浮动窗口与命令面板 | `frontend/src/js/core/floating-modal.js`、`frontend/src/js/core/command-palette.mjs` | FireAxe 是固定布局的桌面窗口 | `frontend/src/js/core/floating-window-coverage.test.mjs`、`frontend/src/js/core/command-palette.test.mjs` |
| 策略组的可操作性（按组筛选 / 整组平移 / 分组推导 / 组内检索） | `internal/app/mod_group_insights.go`、`frontend/src/js/features/mod-groups/strategy-group-filter.mjs` | FireAxe 的 `AddonGroup` 只能应用策略 | `internal/app/mod_group_insights_test.go`、`frontend/src/js/features/mod-groups/strategy-group-filter.test.mjs` |
| 策略组的可操作性（分组筛选 / 整组优先级移动 / 分组推导） | `mod_group_insights.go`、`frontend/src/js/features/mod-groups/` | FireAxe 的 `AddonGroup` 只有应用策略，没有"按组筛选列表""整组平移优先级""从合集/前缀/标签/作者/主体推导可能同组"的入口 |

## 3. 已知语义假设（必须显式记录）

- **有效分层方向**：`effective` 越小越先加载，写在 addonlist.txt 越靠前。
  FireAxe 的 `Push()` 同样把高优先级写在前面（`AddonRoot.cs:899` 降序），方向一致。
- **覆盖方向**：本项目既有的用户文档语义是"越靠后加载越容易覆盖前面的资源"，
  因此冲突判定里"有效分层更大者获胜"集中在 `conflict.go` 的 `conflictOrderWins` 一处。
  该方向仍属于待实测项（见 `manual-verification.md`），一旦受控实验给出相反结论，只需改这一处。

## 4. 逐项落地状态

对照口径：**已对齐**=按上游语义落地并有自动化测试；**有意取舍**=只借鉴设计、按本项目模型改写；
**有意不做**=与既有优势冲突或按上面的理由不采纳；**未完成**=尚未实现。
核对范围覆盖三层：**源码文件**（§4.2.2 Core 75 个 / §4.2.3 GUI 177 个）、
**能力清单**（§4.4：上游 CHANGELOG v0.7.0–v0.7.3 的工具 / 快捷键 / 发布方式）、
**一页对照**（§1.0：51 行完整表）。当前**没有任何"未完成"项**。

| # | 功能 | 实现 | 自动化测试 | 文档 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 1 | 统一优先级模型 | `internal/app/priority.go`、`conflict.go`、`mod_groups.go` | `internal/app/priority_test.go`（含黄金回归）、`frontend/src/js/features/file-list/priority-label.test.mjs` | 本文 + `docs/features/mod-management.md` | 已完成 |
| 2 | 每 Mod 冲突忽略文件 | `internal/app/mod_ignore.go`、`conflict.go` 扫描分支 | `internal/app/mod_ignore_test.go`、`frontend/src/js/features/modals/detail-ignore-key.test.mjs` | `docs/toolbox/conflict-check.md` | 已完成 |
| 3 | 本地记录备份轮转 | `internal/app/local_store_backup.go` + 5 个 store 写入路径 | `internal/app/local_store_backup_test.go` | `docs/features/settings.md` | 已完成 |
| 4 | 变更驱动自动复检与修复建议 | `internal/app/conflict_recheck.go` + addonlist/文件操作失效钩子 | `internal/app/conflict_recheck_test.go`、`frontend/src/js/features/conflicts/conflict-badge.test.mjs` | 本文 + `docs/toolbox/conflict-check.md` | 已完成 |
| 5 | 游戏原版文件白名单（分批可增量） | `internal/stockfiles/`、`internal/app/stock_whitelist.go` | `internal/app/stock_whitelist_test.go`、`internal/stockfiles/stockfiles_test.go` | `docs/toolbox/conflict-check.md` | 已完成（比上游多出“按类生成增量批次”） |
| 6 | 自动重下策略（默认关闭） | `internal/app/workshop.go`（`shouldAutoRedownload` / `maybeAutoRedownload`）、`workshop_download.go` 失败钩子 | `internal/app/auto_redownload_test.go`（注入下载启动器） | `docs/features/downloads.md` | 已完成 |
| 7 | 工坊合集实体化 | `internal/app/workshop_collections.go`（`collections.json`） | `internal/app/workshop_collections_test.go`、`frontend/src/js/features/settings/workshop-collection-format.test.mjs` | `docs/features/downloads.md` | 已完成 |
| 8 | 树形分组 / 嵌套层级 | `internal/app/mod_group_tree.go`（`ParentID` + 树构建 + 校验）、「策略组管理」窗口（`frontend/src/js/features/mod-groups/strategy-group-manager.js` + `#strategy-group-modal`）的层级渲染 | `internal/app/mod_group_tree_test.go`、`frontend/src/js/features/settings/strategy-group-tree.test.mjs` | `docs/features/mod-management.md` | 已完成（扁平视图仍为默认；窗口已从设置页搬到独立窗口） |
| 9 | i18n 文案层 | `frontend/src/messages/zh-CN.json`、`frontend/src/js/core/i18n.mjs`、`i18n-runtime.js`；工具箱页面已迁移 | `frontend/src/js/core/i18n.test.mjs` | `docs/development/i18n.md` | 已完成第一阶段（运行时+目录+首屏迁移；其余页面按同一约定逐页迁移） |
| 10 | 自身崩溃上报 | `internal/app/crash_reporter.go`（日志环形缓冲 + 协程池守卫 + 启动守卫 + 前端上报）、关于页入口 | `internal/app/crash_reporter_test.go` | 本文 + `docs/features/about-update.md` | 已完成 |

### 4.1 2026-09-24 增补（第二轮"值得学 FireAxe"清单）

| # | 功能 | 实现 | 自动化测试 | 文档 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 11 | push 事务化（整批失败回滚到最后一致状态） | `internal/app/addon_list_transaction.go`；接入 `priority.go`、`mod_groups.go`、`addon_list.go`、`addon_list_load_order.go`、`addon_list_merge.go`、`dependencies.go`、`health_check.go`、`profiles.go` | `internal/app/addon_list_transaction_test.go`（回滚 + 派生步骤重跑 + 写失败短路） | 本文 §1、`docs/features/mod-management.md` | 已完成 |
| 12 | 策略可满足性预检 | `internal/app/mod_group_apply_check.go`（`CheckModStrategyGroupApply`）、`strategy-group-manager.js` 应用前预检 | `internal/app/mod_group_apply_check_test.go`、`frontend/src/js/features/mod-groups/strategy-group-batch.test.mjs` | `docs/features/mod-management.md` | 已完成 |
| 13 | Problem 的自动修复分级 | `frontend/src/js/features/settings/health-report-format.mjs`（`healthIssueAutoFixAction` / `summarizeHealthAutoFix`）、`settings-page.js` 逐行「修复」按钮 | `frontend/src/js/features/settings/health-report-format.test.mjs` | `docs/features/mod-management.md` | 已完成（白名单比上游窄：只有"删重复条目""删缺失条目"） |
| 14 | 本地记录 schema 版本化 | `internal/app/local_store_schema.go`（`groups.json` / `priority.json` 迁移 + 写盘盖版本） | `internal/app/local_store_schema_test.go` | 本文 §1 | 已完成 |
| 15 | 下载进度落盘节流 + 任务去重 | `internal/app/download_task_store.go`（1s 节流 + 相同内容跳过 + 原子写 + 快照恢复）、`workshop_update_check.go`（检查任务 in-flight 去重） | `internal/app/download_task_store_test.go` | `docs/features/downloads.md` | 已完成 |
| 16 | 树拖放排序 | `strategy-group-tree.mjs`（`resolveStrategyGroupDrop` / `applyStrategyGroupDropOrder`）、`strategy-group-manager.js` 行级拖放、`internal/app/mod_group_order.go`（`ReorderModStrategyGroups`） | `internal/app/mod_group_order_test.go`、`frontend/src/js/features/settings/strategy-group-tree.test.mjs`、`strategy-group-batch.test.mjs` | `docs/features/mod-management.md` | 已完成 |
| 17 | 工坊官方标签与统计（`steamTags`） | `internal/app/workshop_steam_details.go`、`meta.go`（`steam_tags` 等字段）、`mod_group_suggestion_import.go`（catalog `workshop.steamTags`）、设置页「抓取官方标签与统计」按钮 | `internal/app/workshop_steam_details_test.go`（用**真实抓取的响应**做夹具）、`frontend/src/js/features/settings/settings-bindings.test.mjs` | `docs/features/settings.md` | 已完成 |
| 18 | 依赖成环检测 | `internal/app/dependencies.go`（`dependencyCycles` + 体检项 `dependency_cycle`） | `internal/app/dependency_cycle_test.go`、`frontend/src/js/features/settings/health-report-format.test.mjs`（确认不在自动修复白名单） | `docs/toolbox/mod-health-check.md` | 已完成 |
| 19 | 文件名合法性前置校验 | `internal/app/vpk_actions.go`（`windowsFileNameProblem` / `validateRenameInputFilename`） | `internal/app/vpk_actions_rename_validation_test.go` | `docs/features/mod-management.md` | 已完成 |
| 20 | 下载暂停 / 断点续传 | `internal/app/download_block_checkpoint.go`（区块检查点）、`download_worker.go`（`newBlockManagerWithResume`）、`workshop_transfer.go` / `workshop_download.go` / `workshop.go`（`PauseDownloadTask` / `ResumeDownloadTask`）、任务列表按钮 | `internal/app/download_resume_test.go`（本地 Range 服务器：字节一致 / 只请求缺失区块 / 暂停保留断点 / 截断时重下）、`frontend/src/js/features/downloads/task-list-pause.test.mjs` | `docs/features/downloads.md` | 已完成 |
| 21 | 搜索语法（正则 + 标签或/且/排除） | FireAxe `AddonNodeSearchUtils.cs` 的 `AddonNodeSearchOptions`（`IsRegex` / `IncludeName` / `IgnoreCase` / `Tags` / `TagFilterMode = Or·And·Not`）与 `DefaultStringMatcher`·`RegexStringMatcher`·`OrModeTagMatcher`·`AndModeTagMatcher`·`NotModeTagMatcher` | `internal/app/search_query.go`（`tag:a|b` = Or、`tag:a tag:b` = And、`-tag:a` = Not、`re:` = IsRegex、引号短语、`-词` 排除）接入 `SearchVPKFiles`；前端 `search-syntax.mjs` 同语义镜像 + `search-match.mjs` 高亮/命中字段/chip；搜索框 placeholder 与 tooltip 写明语法 | `internal/app/search_query_test.go`（**Go 与前端共用** `testdata/search_query_cases.json`）、`search-syntax.test.mjs`、`search-match.test.mjs` | `docs/features/mod-management.md` | 已完成（比上游多出：多个词 AND、`-词` 排除、非法正则明确报错） |
| 22 | 容器内名称唯一 | `AddonNodeContainerService.cs` 的 `GetUniqueChildName` / `NameExists` / `ThrowIfChildNewNameDisallowed` | `internal/app/mod_group_names.go`：建组/＋子组**自动加序号**（`名字 (2)`、`名字 (3)`…），重命名撞名**直接报错**并指出是哪一个已存在；比较时忽略大小写与首尾空白 | `internal/app/mod_group_names_test.go`（纯函数 + 走导出方法的端到端） | `docs/features/mod-management.md` | 已完成 |
| 23 | 子节点问题向父节点汇报 | `AddonChildrenProblem.cs`（子节点有 Problem ⇒ 父节点也标出问题） | `GetModStrategyGroupMissingMembers` 新增 `subtreeMissingCount` / `affectedChildCount` / `parentId`，父组行显示「子组里有 N 个缺失文件（涉及 M 个子组）」；样式与"本组缺失"区分开 | `internal/app/mod_group_subtree_missing_test.go`、`group-view.test.mjs`、`strategy-group-batch.test.mjs` | `docs/features/mod-management.md` | 已完成 |
| 24 | VPK 内嵌预览图 | `VpkUtils.cs` 的 `GetAddonImage`（读 `addonimage.jpg`） | **本项目早已实现且更宽**：`internal/parser/archive_index.go` 索引 `addonimage.jpg` 与 `addonpreview*`，`card_preview.go` 提取并缓存，列表/卡片/详情各有预览接口 | `internal/parser` 相关测试、`internal/app` 预览缓存测试 | `docs/features/mod-management.md` | 已对齐（本轮补记，无需改动） |
| 25 | 输出文件自动去重命名 | `FileSystemUtils.cs` 的 `GetUniqueFileName` / `GetUniquePath` | **本项目早已覆盖**：`createUniqueVPKOutputFileWithBaseName` 给打包/修复另存自动加 `(1)`/`(2)`…（上限 10000，有测试）；ZIP 导出走系统保存对话框 + 包内 basename 去重；体检报告重复保存自动加序号 | `internal/app/vpk_pack_test.go`、`internal/app/health_check.go` 报告保存路径 | `docs/features/mod-management.md` | 已对齐（本轮核对后补记） |
| 26 | 任务期间的移动互斥 | `WorkshopVpkAddon.cs` 的 `BlockMove()`（长任务运行期间挡住节点移动） | `internal/app/file_op_gate.go`：移动 / 删除 / 打包共用一个 `TryLock` 闸门，后来者拿到「另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试」；`IsFileOperationBusy()` 供界面查询。修复流程**刻意不加锁**（它内部会调用解包/打包，加了会自锁死，理由写在文件注释里） | `internal/app/file_op_gate_test.go`（占住闸门后四类操作全被挡下且文件零改动；修复流程仍可跑） | `docs/features/mod-management.md` | 已对齐（2026-09-26） |
| 27 | 导入/移动失败逐项报告 | `AddonRoot.Import` + `FailedImportResultItem`（每个失败条目独立原因） | `MoveResult` 带 `successCount / failCount / skippedCount / cancelled / errors[]`（逐项原因带文件名）；列表页与归档页改用 `formatMoveFailures` 展示**前 3 条原因 + 还有 N 条**，不再只显示 `errors[0]` | `frontend/src/js/features/file-list/move-result-format.test.mjs`、`file_op_gate_test.go` 里的逐项断言 | `docs/features/mod-management.md` | 已对齐（2026-09-26；比上游多了"合并同类提示 + 控制台留全量"） |
| 28 | 依赖问题的自动修复 | `internal/app/health_check.go`（`ModHealthIssue.Target` + `addIssueWithTarget`）、`internal/app/dependencies.go`（`dependency_disabled` 带 `record.Key` 目标 + `createAddonListPreWriteBackupLocked` 写前备份）、`frontend/src/js/features/settings/health-report-format.mjs`（第三个白名单动作 `enable-dependencies`）、`frontend/src/js/features/settings/settings-page.js`（确认框 + 调 `EnableModDependencies`） | `internal/app/health_check_precision_test.go`（用体检结果里的 Target 驱动修复 → 依赖被开启、`before-dependency-fix` 备份存在、问题消失）、`frontend/src/js/features/settings/health-report-format.test.mjs`、`frontend/src/js/features/settings/settings-bindings.test.mjs` | `docs/toolbox/mod-health-check.md` | 已完成 |
| 29 | 路径类型不符的独立诊断 | `internal/app/health_check.go`（`modHealthKindFileTypeMismatch`） | `internal/app/health_check_precision_test.go`（目录占位只报 `file_type_mismatch` 不报 `missing_file`；`disabled` 有副本时提示出路） | `docs/toolbox/mod-health-check.md` | 已完成 |
| 30 | 本地工坊记录的身份一致性 | `internal/app/health_check.go`（`modHealthKindMetaIDMismatch` + `checkWorkshopMetaIDMatch`：只在文件名是纯数字时判断） | `internal/app/health_check_precision_test.go`（ID 不一致报 1 条、对得上不报、不误判成孤立 / 缺失） | `docs/toolbox/mod-health-check.md` | 已完成 |
| 31 | 失效目标守卫（`IValidity` 家族） | `internal/app/task_target_guard.go`、`internal/app/workshop_update_check.go`（`workshopItemDetailFetcher` 注入点 + 回写前 `stillValid()` + `SkippedStale`） | `internal/app/task_target_guard_test.go`（4 类失效判定 + 端到端：检测中途把 VPK 移走 → 不写 `.meta`、`SkippedStale=1`；目标没动 → 正常写回） | 本文 §1、`docs/features/downloads.md` | 已完成 |
| 32 | 游戏目录自动探测（注册表 + Steam 库清单） | `internal/app/steam_locations.go`（`parseSteamLibraryFolders` / `steamLibraryCandidates` / `addonsPathIfGame` / `findAddonsInSteamLibraries`）、`internal/app/steam_locations_windows.go` 与 `internal/app/steam_locations_other.go`、`internal/app/filesystem.go`（`AutoDiscoverAddons` 改为多级探测，保留盘符扫描兜底） | `internal/app/steam_locations_test.go`（两种 VDF 格式 / 去重 / 脏数据不猜 / 注册表优先于库清单 / 没有 `addons` 不返回 / 候选顺序） | `docs/guide/quick-start.md` | 已完成 |
| 33 | 受管根内的路径守卫 + `outside_root` 体检项 | `internal/app/path_guard.go`（`managedRootDirectories` / `pathWithinBase` / `managedFilePathProblem`）、`internal/app/filesystem.go`（删除、移动源）、`internal/app/vpk_actions.go`（隐藏改名）、`internal/app/health_check.go`（`modHealthKindOutsideRoot`）、`frontend/src/js/features/settings/health-report-format.mjs` | `internal/app/path_guard_test.go`（10 类路径判定 + 大小写 + 端到端：越界删除/改名被拒且文件零改动、越界源文件逐项失败、**移动到用户自选目录仍然可用**、体检报 `outside_root` 且不误报 `missing_file`）、`frontend/src/js/features/settings/health-report-format.test.mjs` | `docs/toolbox/mod-health-check.md` | 已完成 |
| 34 | 对象解释层（`FireAxe.GUI`） | `frontend/src/js/core/action-explanation.mjs`（`explainOperationError` / `sceneFromErrorType` / `explainActionAvailability` / `formatExplanation`）、`frontend/src/js/core/toast.js`、`frontend/src/js/features/file-list/move-result-format.mjs`、`frontend/src/js/features/file-list/operations.js`、`frontend/src/js/features/diagnostics/model-stats-scan.js` | `frontend/src/js/core/action-explanation.test.mjs`（错误规则 / 场景区分 / 兜底非空 / 文件名不被吞掉 / 可用性 8 类 + 接线断言）、`frontend/src/js/features/file-list/move-result-format.test.mjs` | `docs/features/mod-management.md` | 已完成 |
| 35 | 剪贴板工坊链接自动识别（v0.4.0，`MainWindowViewModel.cs:299,796-855`、`AppSettings.cs:348`） | `internal/app/workshop_clipboard.go`（`parseWorkshopClipboardLink` 纯函数 + `CheckClipboardWorkshopLink` 走 `runtime.ClipboardGetText`）、`frontend/src/js/core/workshop-clipboard.mjs`（解析 / 节流 / 去重）、`frontend/src/js/features/downloads/clipboard-watch.js`（轮询接线）、`frontend/src/js/features/downloads/workshop-modal.js`（`openWorkshopModalWithUrl`）、设置页开关 `settings-auto-detect-workshop-link` | `internal/app/workshop_clipboard_test.go`（10 类输入）、`frontend/src/js/core/workshop-clipboard.test.mjs`（4 项）、`frontend/src/js/core/shortcuts-wiring.test.mjs`（轮询真的接在启动流程里） | `docs/features/settings.md` | 已完成 |
| 36 | `Ctrl+X` / `Ctrl+V` 移动（v0.5.1） | `frontend/src/js/features/file-list/actions.js`（`cutSelected` / `pasteMoveClipboard` / `clearMoveClipboard` / `movePathsToDestination`）、`frontend/src/js/features/state.js`（`moveClipboard` + 状态栏「待移动」）、`frontend/src/js/features/app-runtime.js`（按键分发 + Esc 取消）、`frontend/src/js/core/shortcuts.mjs`（`isTextEntryElement` + 两行快捷键） | `frontend/src/js/core/shortcut-guards.test.mjs`、`frontend/src/js/core/shortcuts-wiring.test.mjs`、`frontend/src/js/features/status-bar-pending-move.test.mjs`（真实 import `state.js` 跑一遍） | `docs/features/mod-management.md` | 已完成 |
| 37 | 自定义外部打开程序（v0.7.2，`AppSettings.cs:497/513/529/545`、`Utils.cs:141-153`） | `internal/app/open_with.go`（`splitOpenWithArguments` / `buildOpenWithCommand` / `validateOpenWithProgram` / `GetOpenWithSettings` / `SetOpenWithSettings`）、`internal/app/filesystem.go`（`OpenFileLocation` 先查配置，未配置走原系统默认）、设置页「打开文件方式」卡片（`settings-open-with-program` / `settings-open-with-arguments` + 保存 / 恢复默认，**事件委托绑定**） | `internal/app/open_with_test.go`（分词 11 类 + 组装 5 类 + 校验 4 类 + 配置往返 + **真启动 xcopy 复制文件的端到端**）、`frontend/src/js/features/settings/open-with-settings.test.mjs` | `docs/features/settings.md` | 已完成 |

### 4.2 仍然"有意取舍 / 有意不做"的部分

| 项目 | 结论 | 原因 |
| --- | --- | --- |
| 符号链接 push | 有意不做 | 本项目直接管理真实文件；引入软链会把"文件在哪"变成第二套真相 |
| `RefAddonNode` 引用型节点 | 有意不做 | 同一 Mod 的多方案由"启用方案"覆盖，事件与冲突全部落在 addonlist 这一条主线上 |
| 把优先级做成独立于 addonlist 的平行状态 | 有意不做 | `addonlist.txt` 顺序仍是游戏侧唯一权威，分层只是"意图 + 判定 + 排序"层 |
| FireAxe 的 `ShouldFixAutomatically`（默认自动修） | 有意取舍 | 本项目所有修复都要用户确认（体检一键修复也要先确认并先备份） |
| FireAxe 的 `.addonroot` 单存档 | 有意取舍 | 本项目拆成 `groups.json` / `priority.json` / `ignore.json` / `profiles.json` / `dependencies.json`，各自带 `schemaVersion` |

### 4.2.1 还值得学、但本轮判定"先不做"的

按"值不值 / 会不会退化现有优势"排过一遍之后，剩下的候选与结论：

| 项目 | FireAxe 证据 | 本轮结论 |
| --- | --- | --- |
| ~~下载暂停 / 断点续传~~ | `DownloadService.cs:230`（`Resume`）、`:244`（`Cancel`）、`IDownloadService` 的 `Pause` / `Resume` / `WaitAsync` | **已完成（第 20 项）**：按区块检查点做真正的续传，暂停/失败/重启都能接着下 |
| ~~任务期间阻止移动~~ | `WorkshopVpkAddon.cs` 的 `BlockMove()` | **已完成（第 26 项）**：移动/删除/打包共用 TryLock 闸门，并把忙碌状态推进界面 |
| ~~导入结果逐项报告~~ | `AddonRoot.Import` + `FailedImportResultItem` | **已完成（第 27 项）**：`MoveResult.errors[]` + 界面列出前 3 条原因 |
| ~~有效引用的失效语义~~ | `ValidRef.cs:14-38`、`ValidTaskCreator.cs:22-36`、`IValidityExtensions.cs:8-42` | **已完成（第 31 项）**：`modTaskTarget` 捕获目标身份、回写前复查；检测期间被移动 / 替换就放弃写入 |
| 有效性级联的**增量传播** | `IValidity.cs`、`ObservableValidRefCollection.cs`、各 `Problem` 的 `Invalidate()` 级联 | **已实测并判定不做**：先按"测量 → 定位 → 修根因"的顺序查了一遍（见 §5.13）——真正的瓶颈不是缺少增量传播，而是索引缓存容量写死 1024 < 库里的 2409 个 Mod，导致每轮重算 LRU 抖动、重复解析（热重算 558ms 里 435ms 是重复解析）。修复容量策略后热重算降到 **177ms**，且前端本身有 400ms 防抖（连续开关合并成一次），剩余 173ms 是 24.1 万条路径的分组累加；再做增量传播要额外维护"文件 → 提供者"反查索引与失效簿记，收益约 170ms/次突发，风险大于收益 |
| 单存档多态序列化 | `AddonRootSave.cs` + `AddonNodeSave.cs` 的 `$type` 多态 | 与本项目"拆成 5 个带版本文件"的方向冲突，且我们已有迁移器，不采用 |
| 操作期间阻止文件被移动 | `WorkshopVpkAddon.cs` 的 `BlockMove()`（任务运行期间挡住移动） | 本项目当前是"长任务只在自家面板里串行"，没有全局移动互斥。若后续出现"下载/迁移过程中用户改文件名"的实际故障，再补这一层 |
| 导入结果逐项报告 | `AddonRoot.Import` + `FailedImportResultItem`（每个失败条目带原因） | 本项目的批量移动/复制已经按条目返回结果与冲突动作；差异只在错误文案粒度，暂不单列 |

### 4.2.2 上游 75 个源文件的收口核对（2026-09-26）

"还有没有值得学的"这个问题不能靠感觉回答，所以本轮把 `FireAxe.Core` 的 **75 个 `.cs` 文件**
逐个过了一遍，并按处置结果分类。结论：**没有"看到了但没处理"的空白**；剩下的都是
契约 / 标记 / 基础设施类型，或者已经由既有能力覆盖。

| 类别 | 上游文件 | 处置与理由 |
| --- | --- | --- |
| 已按语义落地（见 §1 / §4） | 其余 38 个文件 | 对应 §1 与 §4 的 33 项，逐项有实现、自动化测试与文档 |
| 有意不做的模型差异 | `RefAddonNodeSave`、`AddonInvalidRefSourceProblem`、`AddonNodeFileFinder` | 与"引用型节点""文件夹树导入"绑定，前者由"启用方案"覆盖、后者需要库目录 + 符号链接 push（§4.2 已写明冲突点） |
| 问题（Problem）子类，本项目用**类型字段**表达 | `AddonProblem`、`VpkAddonConflictProblem`、`InvalidVpkFileProblem`、`AddonDownloadFailedProblem`、`WorkshopVpkNotLoadedProblem` | 上游每种问题是一个 C# 类；本项目是 `ModHealthIssue.Kind`（`invalid_vpk` / `missing_meta` / `conflict` …）与下载任务状态。语义相同，表示方式随语言与既有模型 |
| 异常 / 接口 / 工具类型，本项目用 error + 校验函数表达 | `AddonNameExistsException`、`AddonNodeIdExistsException`、`FileNameExistsException`、`AddonNodeInvalidMoveException`、`AddonNodeMoveDeniedException`、`InvalidGamePathException`、`ArgumentNullExceptionExtensions`、`DisposableUtils`、`ISaveable`、`IHierarchyNode`、`IAddonNodeContainer`、`IAddonNodeContainerExtensions` | 分别对应第 19 / 22 / 26 / 32 / 33 项的校验与守卫（例如名字唯一性、移动互斥、路径合法性与根内约束） |
| 语言/框架自带的基础设施 | `ObservableObject`、`ObservableCollectionAdvanced`、`TaskUtils`、`KVObjectExtensions` | C# 的 `INotifyPropertyChanged`、集合通知、`TaskFactory` 与 KeyValues 转换；Go + 原生 JS 侧对应 Wails 事件、协程池与既有的 addonlist 解析器 |
| 存档 / 模型辅助 | `AddonGroupSave`、`VpkAddonInfo`、`WorkshopVpkAddonSave`、`LocalVpkAddon`、`LocalVpkAddonSave`、`AddonRootExtensions` | 上游是 `.addonroot` 单存档里的 DTO；本项目拆成 5 个带 `schemaVersion` 的 JSON（第 14 项） |
| 下载契约与设置 | `IDownloadItem`、`IDownloadService`、`DownloadStatus`、`DownloadServiceSettings` | 对应第 15 / 20 项的下载任务快照、状态机、断点续传与暂停；上游只有一个 `Proxy` 设置，本项目另有优选 IP / 固定 IP |
| 根设置向子节点继承 | `IAddonRootParentSettings` | 上游让工坊节点继承根的"自动更新"开关；本项目是全局默认 + 单任务覆盖（第 6 项），语义更显式 |
| 官方标签词汇表 | `AddonTags` | 上游内置一份固定的 Steam 标签词表；本项目**直接抓取 Steam 官方 `tags[]`**（第 17 项），比内置词表更新更准，因此不照搬 |
| 存档反序列化失败 | `AddonRootDeserializationException` | 上游对坏存档抛强类型异常；本项目是"schema 迁移 + 读时自愈（未知策略值归一化）+ 备份轮转"（第 3 / 14 项），并已有真机验证记录 |
| 遍历 / 任务守卫辅助 | `AddonNodeExtensions`、`IHierarchyNodeExtensions` | `GetValidTaskCreator` / `GetAllNodesEnabledInHierarchy` 的语义由第 31 项（失效目标守卫）落地；DFS 前序/后序与祖先迭代器对应本项目 `internal/app/mod_group_tree.go` 的树构建与遍历、前端 `frontend/src/js/features/settings/strategy-group-tree.mjs`（第 8 / 16 项） |

仍未被采纳的唯一"功能型"设计是 `IValidity` 的**增量级联传播**，理由见 §4.2.1。

### 4.2.3 `FireAxe.GUI` 的收口核对（2026-09-26）

第十轮把 `FireAxe.GUI` 也扫了一遍（机器统计 **177** 个源码类文件：`.cs` 129 / `.axaml` 46 / `.resx` 2）。
按基名在本文里逐个提及的有 **35** 个（含下表列出的代表文件），其余按下表按目录分类核对 —— 结论：**只有"对象解释层"是可直接落地的增量**
（已作为第 34 项落地），其余属于 **Web 技术栈 ↔ Avalonia 桌面栈** 的实现差异。

| 目录 / 类别 | 文件数 | 内容 | 结论 |
| --- | --- | --- | --- |
| `Views/` | 90（47 `.cs` + 43 `.axaml`） | Avalonia 视图与代码后置（`AddonNodeView.axaml`、`AddonNodeExplorerView.axaml`、`AddonNodeGridRowView.axaml` …） | 本项目是原生 JS + CSS 的页面/弹窗体系（浮动窗口、命令面板等见 §2），**没有可移植增量** |
| `ViewModels/` | 43（含 4 个 `*Design.cs` 设计时桩） | 视图模型（`AddonNodeViewModel`、`AddonNodePickerViewModel`、`AddonNodeSortMethod` …） | 对应本项目 `frontend/src/js/features/file-list/render.js`、`frontend/src/js/features/mod-groups/strategy-group-manager.js` 这类页面模块，实现方式不同（Web 技术栈没有 ViewModel 层） |
| 根目录 | 28 | 应用装配与基础设施（`Program.cs`、`AppWindowManager.cs`、`AppSettings.cs`、`SaveManager.cs`、`AppMutex.cs` …） | 其中 `ObjectExplanationManager` / `ExceptionExplanations` / `AddonProblemExplanations` / `ExceptionExplanationScene` / `DescriptiveObject` 已作为第 34 项落地；`CrashReporter.cs`→第 10 项、`CultureManager.cs`→第 9 项、`AppColorTheme.cs`→主题与阅读舒适度、`SaveManager.cs`→第 14 项、`TaskOperationsProgressNotifier.cs`/`IOperationsProgressObserver.cs`→进度提示、`AppMutex.cs`→单例 |
| `ValueConverters/` | 8 | Avalonia 值转换器（`ReadableBytesConverter`、`EnumDescriptionConverter` …） | 对应本项目 JS 里的格式化纯函数（`formatCollectionSummary`、`describeArchiveSearchResult`、`formatExplanation` …） |
| `Resources/` | 3 | `Texts.resx` + 设计器类 | 对应本项目 i18n 目录 `frontend/src/messages/zh-CN.json`（第 9 项） |
| `DataTemplates/`、`MarkupExtensions/`、`Assets/` | 5 | XAML 绑定基础设施（`ExceptionExplainer`、`ViewLocator`、`EnumValues` …） | 绑定层是 XAML 特有；其中 `ExceptionExplainer` 的语义已由第 34 项覆盖（异常 → 一句话解释） |

### 4.4 CHANGELOG 级能力对照（2026-09-26 增补）

§4.2.2 / §4.2.3 是按**源码文件**逐个核对的；但上游的能力有一部分只体现在 `CHANGELOG.md`
（工具名、快捷键、发布方式），文件层面看不出来。这一轮把上游 CHANGELOG（v0.7.0 – v0.7.3）
里的能力逐条过了一遍，结果如下（对应 §1.0 表格的 38 – 51 行）：

| 能力（上游 CHANGELOG 出处） | 本项目处置 | 说明 |
| --- | --- | --- |
| v0.7.0：`Ctrl+F` 聚焦搜索 / `Delete` 删除选中 / 弹窗 `Enter` 提交 | **本轮补齐** | `Ctrl+F`、弹窗 `Enter`/`Esc` 早就有；新增 **F2 重命名**与 **Delete 删除**（多选走批量删除），并进快捷键总表 |
| v0.7.2：`F2` 重命名、`Enter` 提交、`Esc` 取消编辑 | **本轮补齐** | F2 作用于"搜索结果光标行"，没有光标时退化为唯一选中项；改名会自动迁移策略组 / 分层 / 依赖 / 忽略清单的引用 |
| v0.7.3：主窗口宽高 + 最大化状态写进设置 | **本轮补齐** | `config.json` 新增三项；恢复时按当前屏幕钳制（换小屏不会把按钮顶到屏幕外），变化防抖 600ms 落盘 |
| v0.7.0：File Cleaner — 清理工坊条目的冗余 VPK | **本轮补齐（只提示）** | 新增体检项「工坊作品有多份副本」：`addons\<ID>.vpk` 与 `addons\workshop\<ID>.vpk` 同时存在时报告并指路；删除仍由用户（移动 / 删除进回收站） |
| v0.7.0：File Cleaner — 下载临时文件 / 空目录 | 部分已覆盖 + 有意不做 | 下载临时文件启动后自动清 7 天前残留；空目录不清理（子目录不在加载范围，风险大于收益） |
| v0.7.0：Addon Name Auto Setter（就地批量按标题改名） | 有意不做 | 工坊 VPK 文件名就是作品 ID，改名会破坏 `.meta` 配对与更新检测；本项目保留 ZIP 导出的"按名称重命名"（不动原文件）+ F2 单个改名 |
| v0.7.1：File Name Fixer（猜文件名修复） | 有意不做 | 猜错会把正确文件改坏；本项目给明确诊断（缺失 / 类型不符 / 工坊记录不一致）+ 手动 F2 |
| v0.7.0 / issue #17：自定义预览图（同名图片文件） | 已覆盖 | 图片按同名放在 VPK 旁边即生效、并跟随移动；不再引入"图片路径"第二套真相 |
| v0.7.0：`Workshop VPK Finder` | 不适用 | 本项目直接管理 `workshop` 目录，不存在"找不到工坊实体"的场景 |
| v0.7.0：单例模式 | 已覆盖 | `StartSingletonListener` |
| v0.7.1：Web 代理地址 / 凭据 | 已覆盖 | 优选 IP + 固定 IP + 系统代理 |
| v0.7.0：视图 / 排序 / 布局记忆 | 已覆盖 | `displayMode` / `filterLayoutMode` / 排序记忆 |
| v0.7.2：右键"打开工坊页面" | 已覆盖 | 工坊跳转目标（镜像 / 官方） |
| v0.7.3：AppImage / Linux 发布 | 不适用 | 本项目是 Windows 桌面工具 |
| v0.4.0：自动识别剪贴板里的工坊链接 | **本轮补齐** | `CheckClipboardWorkshopLink`（读 + 解析，含纯数字 ID）+ 前端节流轮询与确认框；设置页可关。上游是 0.5s 轮询，本项目改成"开关 + 窗口前台 + 1.5s 间隔 + 同一条链接只提示一次" |
| v0.5.1：`Ctrl+X` / `Ctrl+V` 移动 Mod、鼠标侧键回上级组 | 部分补齐 | `Ctrl+X`（标记待移动）/ `Ctrl+V`（选目标目录移动）已落地；**鼠标侧键回上级组不适用** —— 本项目列表是"目录视图"不是"组树视图"，没有"上级组"这一层（策略组的层级在独立窗口里，由「上级分组」下拉与拖放负责） |
| v0.7.2：process file customization（自定义打开文件/目录的外部程序） | **第 18 轮补齐** | 设置 → 界面设置 →「打开文件方式」：程序 + 参数模板（`{path}` / `{dir}` / `{name}`，兼容 `{0}`）；**留空 = 系统默认，行为不变**；带路径的程序保存前校验存在性，启动不经 shell |

### 4.3 自动化验收套件（"模拟用户操作"的端到端测试）

除了每个功能自己的聚焦测试，还额外加了两份**用真实 fixture 驱动导出方法**的端到端套件
（真实 VPK 文件 + 真实 `addonlist.txt` + 真实配置目录，语义上等价于"用户在界面上点一遍"）：

| 文件 | 覆盖 |
| --- | --- |
| `internal/app/fireaxe_parity_e2e_test.go` | §4 的 1-8 项：优先级分层与逐字节不变、单 Mod 忽略、备份轮转、复检失效/重算、原版白名单、自动重下、合集实体化、树形层级与权重累加 |
| `internal/app/fireaxe_parity_flow_test.go` | §4 的 10-16 项：崩溃上报、事务回滚（真造"快照写不进去"）、策略预检、体检一键修复、schema 迁移、下载任务节流/去重/重启恢复、拖放排序写盘 |

跑法：`go test ./internal/app/ -run TestParity -count=1 -v`。

## 5. 验证记录

全部命令在 2026-09-22 于本机（Windows，Go 1.26.7，Node v24.19.0，Wails v2.10.2）执行：

| # | 命令 | 结果 | 说明 |
| --- | --- | --- | --- |
| 1 | `go test ./...` | 通过 | `internal/app`、`internal/minidump`、`internal/network`、`internal/parser`、`internal/platform/protocol`、`internal/stockfiles` 全部 ok |
| 2 | `node --test`（在 `frontend/`） | 通过 | 63 项，0 失败 |
| 3 | `npm run build`（在 `frontend/`） | 通过 | Vite 产物正常；仅有既有的 chunk 体积警告 |
| 4 | `wails build` | 通过 | 生成 `build/bin/LytVPK-Community-Fork.exe`（17,570,304 字节） |
| 5 | `npm run docs:build`（在 `docs/`） | 通过 | VitePress 16.76s 构建完成 |
| 6 | `go vet ./...` | 通过 | 无输出 |
| 7 | 真实运行的应用（`wails dev` + 浏览器自动化，真实 1963 个 Mod 数据） | 通过 | 设置页四面板无报错、白名单状态 `内置 1 批 / 自定义 0 批`、关于页 `本机还没有崩溃报告`、冲突检测 `24 组冲突` 且渲染 50 条修复建议、Mod 详情忽略清单编辑器可用、列表角标 `优先级 #N` / `N 处覆盖` |
| 8 | 打包产物自检（`frontend/dist`） | 通过 | 本轮修复的界面文案全部出现在打包资源中（`\uXXXX` 转义形式比对） |

### 5.1 2026-09-24 第二轮验证记录（本机：Windows，Go 1.26.7，Node v24.19.0，Wails v2.10.2）

| # | 命令 | 结果 | 说明 |
| --- | --- | --- | --- |
| 1 | `go test ./... -count=1` | 通过 | `internal/app` 等全部 ok（含新增的事务化 / 预检 / schema / 排序 / 下载任务测试） |
| 2 | `node --test`（在 `frontend/`） | 通过 | 215 项，0 失败 |
| 3 | `npm run build`（在 `frontend/`） | 通过 | Vite 产物正常，仅保留既有的 chunk 体积警告 |
| 4 | `wails generate module` | 通过 | `App.d.ts` 出现 `ReorderModStrategyGroups` / `CheckModStrategyGroupApply` 等新导出 |
| 5 | `wails build` | 通过 | 产出 `build/bin/LytVPK-Community-Fork.exe` |
| 6 | `npm run docs:build`（在 `docs/`） | 通过 | VitePress 构建完成 |

本轮证据分级：

- **已自动验证**：第 11-16 项的全部语义（含"未设置分层时行为不变"的黄金回归、事务回滚、in-flight 去重、拖放落点判定）。
- **真机驱动验证**（打包 EXE + 临时调试桥，验收后已删除并重新构建）：见 §5.2。
- **只能人工验证**：真实鼠标拖拽的手感、下载进度在真实断网/关进程下的表现、真实游戏内加载顺序。
  清单见 `docs/development/manual-verification.md` 的第十八轮小节。
- **仍未验证**：本轮没有"计划内但完全未验证"的项目。

### 5.2 2026-09-24 真机交互验收（打包 EXE，沙箱配置目录）

用临时调试桥把 JavaScript 送进**真实 WebView**（打包后的 `LytVPK-Community-Fork.exe`）执行，
驱动真实的 DOM 事件与后端调用；沙箱 `%AppData%` 下只放 3 个夹具策略组 + 1 条未完成下载任务，
**没有碰用户的真实 Mod、`addonlist.txt` 与 `%AppData%\LytVPK`**。验收脚本与桥接文件在交付前已删除，
最终 EXE 已确认不含 `cua-bridge` 字样。

| 场景 | 操作 | 结果 |
| --- | --- | --- |
| 打开策略组管理窗口 | 点「分组 → 策略组管理…」 | 窗口以浮动模式打开（`modal is-floating`），列出 3 个夹具组 |
| 组行可拖动 | 检查渲染结果 | 3 行全部 `draggable="true"`，底部「拖到这里 → 变成顶层组」与拖动说明都在 |
| 放进组里 | 在「CUA 甲」行的中段 `dragover` 后 `drop`（源：CUA 乙） | 行上出现 `is-drop-inside` 描边；`groups.json` 写入 `"parentId": "cua-a"`；界面重画为缩进 `1.25rem`、上级下拉变成甲 |
| 同级排序 | 把「CUA 乙」拖到「CUA 丙」的下边缘 | 出现 `is-drop-after`；顺序从 `[甲,乙,丙]` 变为 `[甲,丙,乙]`，且上级被清空（同一条 `MoveModStrategyGroup` + `ReorderModStrategyGroups` 链路） |
| 非法拖放 | 把「CUA 甲」拖到它自己身上 | 状态零变化（`groups.json` 未被改写）；不会产生幽灵写入 |
| 回到顶层 | 把「CUA 丙」拖到底部落点 | 落点高亮 `is-drop-active`；`groups.json` 里 `parentId` 被清除，顺序更新为 `[甲,乙,丙]` |
| 重启后仍在 | 关闭并重新启动 EXE | 拖放结果被保留（新顺序来源于 `groups.json`，不是内存） |
| 未完成下载恢复 | 预置一条 `status=downloading, progress=37` 的快照 | 真实 EXE 里 `GetDownloadTasks()` 返回该任务，状态为 `interrupted`、进度 37、`fileUrl` 保留（可重试） |

**真机验收发现并修掉的两个问题**（这就是"实际测试"和"只看测试通过"的差别）：

1. **底部"回到顶层"落点会被挤到可视区外**：夹具只有 3 个组时，滚动区内容高 725px、
   可视高 478px，落点位于 y=929（视口外），必须滚到底才能用。
   已把落点改成 `position: sticky; bottom: 0`（贴住滚动区底部），并在真机复测：
   落点稳定停在可视区内（y≈682～716，滚动区底部 742）。
2. **未知策略值不会自愈**：`groups.json` 里如果出现无法识别的 `strategy`（手改或跨版本写入），
   运行时会被兜底成「互斥单选」执行，但存档里会一直留着这个非法值。
   已在 `local_store_schema.go` 增加与版本无关的读时修复（归一化为 `single` 并记日志）。

### 5.3 2026-09-24 第三轮：工坊官方标签 / 依赖环 / 文件名校验

**为什么补这三项（先做了实测对比，不是照着清单抄）**：

| 探针 | 结果 |
| --- | --- |
| 现有镜像接口 `POST https://l4d2-workshop-parse.laoyutang.cn`（真实作品 `2302720558`） | 返回字段只有 `result / publishedfileid / file_type / filename / file_size / file_url / preview_url / title / file_description / children` —— **没有 tags** |
| Steam 官方 `ISteamRemoteStorage/GetPublishedFileDetails`（同一个作品） | 返回 `tags:[{tag:"Survivors"},{tag:"Sounds"},{tag:"Single Player"},{tag:"Other"}]` 以及 `subscriptions=21564`、`views=53208`、`favorited=6515` |

也就是说 FireAxe 的 `PublishedFileDetails.Tags` 在本项目里**一直是空的**，而分组建议与自动标签正好最缺这种"作者/玩家自己填的分类"。

**本轮落地**：

1. **工坊官方标签与统计**（第 17 项）：`EnrichWorkshopMetadata` / `EnrichAllWorkshopMetadata` + 设置页按钮；
   官方标签写进 `.meta` 的 `steam_tags`（与本项目自己的 `tags` 分开），统计写进 `subscriptions / favorited / views …`；
   分组建议材料里新增 `workshop.steamTags`，智能体提示词同步说明"判断类型时优先信官方标签"。
2. **依赖成环检测**（第 18 项）：`dependencyCycles` 纯函数 + 体检项 `dependency_cycle`（只提示、不自动修）。
3. **文件名合法性前置校验**（第 19 项）：非法字符 / 控制字符 / 结尾空格或点 / Windows 保留设备名在重命名前拦下，
   并顺手去掉用户输入的首尾空白（不再生成 `name .vpk` 这种名字）。

**证据分级**：

- **已自动验证**：第 17 项用**真实抓取下来的官方响应**（`internal/app/testdata/steam_published_file_details.json`）
  当夹具，断言 `steamTags`、订阅/收藏/浏览与"第二次不重复写盘"；第 18 项覆盖无环/两节点环/三节点环/自环/双环；
  第 19 项覆盖 8 个合法名与 21 个非法名 + 真实重命名入口。
- **真网络验证（一次性，跑完删除脚本）**：用真实 Steam 官方接口抓 `2302720558`、`3153860853`，
  结果 `requested=2 updated=2 failed=[]`，落盘后读出
  `steamTags=[Survivors Sounds Single Player Other] subs=21564 views=53208 fav=6515` 与
  `steamTags=[Campaigns Common Infected] subs=10061 views=62250 fav=3562`。
  验证用的临时测试文件已删除（避免 CI 依赖外网）。
- **只能人工验证**：真实网络受限时（需要代理/加速）按钮的失败提示是否够清楚；Steam 官方接口在国内的连通性。

### 5.4 2026-09-26 第二轮：搜索语法（对齐 AddonNodeSearchUtils）+ 字号档位真正全局生效

**FireAxe 侧新增对齐项**：`AddonNodeSearchUtils.cs` 的 `AddonNodeSearchOptions`
（`IsRegex` / `IncludeName` / `IgnoreCase` / `Tags` / `TagFilterMode = Or·And·Not`）
与五个匹配器（`DefaultStringMatcher` / `RegexStringMatcher` / `OrMode` / `AndMode` / `NotMode`）。
它把"文本 + 正则 + 标签三种模式"放在同一套搜索选项里；本项目落到**搜索框语法**，
因为我们的搜索框没有地方放"模式下拉"，而逐 token 写法更直接：

| FireAxe 能力 | 本项目写法 |
| --- | --- |
| `IsRegex` | `re:^ak\d+$`（编译失败会明确说"正则表达式无效"，不是静默空列表） |
| `TagFilterMode.Or` | `tag:步枪|狙击` |
| `TagFilterMode.And` | `tag:武器 tag:步枪` |
| `TagFilterMode.Not` | `-tag:材质` |
| `IncludeName` / `IgnoreCase` | 默认行为（标题/文件名/标签/主体/发音角色/动作槽都参与，大小写不敏感） |
| —（上游没有） | 多个普通词之间是"且"、`"ak 47"` 引号短语、`-词` 排除普通文本 |

解析器在 Go（`search_query.go`，过滤权威）与前端（`search-syntax.mjs`，只负责高亮与解释）各一份，
**两边共用 `internal/app/testdata/search_query_cases.json` 的用例**并各自校验，防止实现漂移。

**真机验证（打包 EXE + 临时调试桥，验收后已删除并重建，产物确认无 `cua-bridge`）**：

```text
tag:其他       → 匹配 910 / 2408，chip「匹配：一级标签」
-tag:其他      → 匹配 1498 / 2408（正好是 910 的补集），不产生 chip（排除条件不该说"匹配到"）
re:^!!医疗箱   → 匹配 6 / 2408，12 处 <mark data-match="regex">，chip「匹配：标题 · 文件名」
re:[          → 计数直接显示「正则表达式无效：...Unterminated character class」，0 行
清空搜索       → 计数归零、列表恢复
```

**同一轮的真机发现**：正则查询一开始**只筛选、不高亮**（高亮只跟普通词走），
与"命中的片段会高亮"的承诺不一致；已把正则命中并入高亮（`highlightRangesFor`），复测得到上面的 12 处高亮。

**UI 侧同时补的一项**：「文字大小」档位此前对**硬编码 px 字号**无效 ——
任务列表（11/12/14px）与若干面板（卡片标题、下载面板、表单、更新、工坊浏览器）都不跟随。
本轮把它们改成 `rem` / 主题变量，并加了一条 CSS 守卫测试
（`font-size-units.test.mjs`：除窗口标题栏外，出现新的 `font-size: Npx` 即失败）。

### 5.5 2026-09-26 第三轮：容器命名唯一性 / 子问题上报 / 搜索结果键盘导航

**FireAxe 侧新增对齐**：

1. `AddonNodeContainerService.GetUniqueChildName` / `NameExists` / `ThrowIfChildNewNameDisallowed`
   → 本项目建组与「＋子组」自动加序号、重命名撞名报错。
2. `AddonChildrenProblem`（子节点有 Problem ⇒ 父节点也标记）
   → 缺失成员汇总新增 `subtreeMissingCount` / `affectedChildCount` / `parentId`，父组行给出「子组里有 N 个缺失文件（涉及 M 个子组）」。
3. `VpkUtils.GetAddonImage` → 本项目**早已覆盖**（`addonimage.jpg` / `addonpreview*` 索引 + 三套预览接口），本轮只补进对照表。

**判定为"不适用/不做"的一项（写清理由）**：`AddonNodeFileFinder` 的"文件夹树导入成组"
只在 FireAxe 的模型里成立 —— 它的受管根目录是**自己的库目录**，子文件夹本身就是组节点，
最后由 `Push()` 用符号链接送进游戏目录。本项目直接管理游戏真实 `addons` 目录（L4D2 只加载
`addons/*.vpk`，子目录不生效），要靠"子目录=组"就得引入库目录 + 符号链接 push，
正是我们明确不做的方向，因此该能力**有意不采纳**。

**UI 侧同时补的一项**：搜索结果键盘导航（↑ / ↓ 移动、Enter 打开详情、Esc 清空并复位光标），
计数文案会追加「第 2 / 7 个结果（Enter 打开详情）」，结果行有独立描边样式。

**真机验证（打包 EXE + 临时调试桥，验收后已删除并重建，产物确认无 `cua-bridge`）**：

```text
组名唯一性：建两个同名组 → 第一个「重名测试」、第二个自动「重名测试 (2)」
            把第二个改名成「重名测试」→ 报错：已有同名策略组「重名测试」：换个名字，或先重命名那一个
            改名成它自己原来的名字 → 通过
子树缺失提示：父组行显示「⚠️ 子组里有 3 个缺失文件（涉及 2 个子组）：展开子组即可看到」
            （数据源是打桩的，因为真机上不能删用户的 Mod；聚合逻辑由 Go 端到端测试覆盖）
键盘导航：搜「医疗箱」→ 计数「匹配 7 / 2408 个 Mod」
          ↓↓ 后 → 「匹配 7 / 2408 个 Mod · 第 2 / 7 个结果（Enter 打开详情）」，光标行 = 第 2 条
          Enter → 详情弹窗打开，标题与光标行一致
          Esc → 输入框清空、光标清除、计数归零
本轮创建的 4 个测试组在脚本里已删除（沙箱 groups.json 残留 0）
```

### 5.6 2026-09-26 第四轮：窗口交互全覆盖 + 搜索语法说明书

**FireAxe 侧**：本轮核对后**只补记、无改动**一项 —— `FileSystemUtils.GetUniqueFileName`
（输出文件重名自动加序号）：本项目的打包/修复另存已经这么做（`createUniqueVPKOutputFileWithBaseName`，
上限 10000 且有测试），ZIP 导出走系统保存对话框 + 包内 basename 去重，体检报告重复保存自动加序号。

**UI 侧**（对齐用户"复杂管理窗口都要能浮动、能拉伸、背景不虚化"的要求）：

- 浮动窗口从 7 个扩到 **19 个**：新增 Mod 详情、智能体提示词、单/批量标签、加入组选择器、组标签、
  服务器详情，以及服务器面板的地图 / 上传 / RCON / 难度 / 服务器详情。
- **发现并纠正一个错判**：`browser-modal` / `workshop-modal` / `server-modal` 三个"弹窗"其实已经被
  `ui-shell.js` 搬成了**独立页面**（`.modal-content` 被移进页面容器，原 modal 只剩隐藏外壳），
  给它们注册浮动是无效代码 —— 已移除，并在测试里锁住"这三个是页面、不注册浮动"。
- 新增 `?` 搜索语法说明书：悬停提示与浮层内容都由 `search-help.mjs` 生成（单一事实来源），
  列出匹配范围、8 条语法、4 个快捷键；点按钮开、点外部或 Esc 关。

**真机验证（打包 EXE + 临时调试桥，验收后已删除并重建，产物确认无 `cua-bridge`）**：

```text
窗口审计（19 个注册项逐个查 DOM）：
  全部 存在=true / 是弹窗=true / 有浮动按钮=true；除"详情"外都已浮动
  （详情显示 已浮动=false 是因为上一轮真机我点过"停靠窗口"，偏好被按窗口记住了 —— 这本身说明记忆生效）
详情窗口：浮动=true，容器可穿透=true，窗口可点=true，按钮插在 modal-header，尺寸 820×702（视口 1400×900，在视口内）
  点浮动按钮 → 浮动=false，按钮文案变回「浮动窗口」（双向切换可用）
搜索语法说明书：初始隐藏 → 点击后打开（aria-expanded=true、按钮高亮）
  三段（匹配范围/语法/快捷键）、12 行、含 re:^ak 示例；宽 480px 在视口内
  点外部 → 关闭；再开 → Esc → 关闭；搜索框悬停提示由同一份数据生成
```

### 5.7 2026-09-26 第五轮：文件操作互斥（BlockMove）+ 失败逐项报告

**FireAxe 侧新增对齐两项**：

1. `WorkshopVpkAddon.BlockMove()` → `internal/app/file_op_gate.go`
   （移动 / 删除 / 打包共用 TryLock 闸门 + `IsFileOperationBusy()`）。
2. `AddonRoot.Import` + `FailedImportResultItem` → 本项目的 `MoveResult.errors[]`（逐项原因带文件名）
   本来就有，但界面只显示了第一条；现改为 `formatMoveFailures` 展示前 3 条 + "还有 N 条，详见控制台日志"。

**为什么修复流程不加锁（写进代码注释）**：`RepairVPKIntegrity` 内部调用 `UnpackVPKFile`，
`RepairVPKIntegrityBatch` 又调用 `RepairVPKIntegrity` —— 加同一把锁会自锁死；而修复写的是临时目录 +
`.repaired.vpk` 新文件，不参与移动/删除/打包的争抢。测试 `TestRepairFlowStillRunsWhileGateIsFree` 守住这一点。

**真机验证（打包 EXE + 临时调试桥，验收后已删除并重建，产物确认无 `cua-bridge`）**：

```text
空闲时 busy=false
后台启动 300MB 目录打包 → 轮询到 busy=true（闸门真的被占住）
  此时 MoveVpkFiles → 「另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试」
打包结束后 busy=false
逐项失败：把两个 Mod 移动到一个"其实是文件"的目标路径 →
  failCount=2、errors 长度 2，两条各自带文件名与系统原因（"The system cannot find the path specified"）
收尾核对：两个真实 Mod 文件仍在原位（True / True）—— 被挡下与被拒绝的操作零改动
```

### 5.8 2026-09-26 第六轮：FireAxe 清单收敛 + 忙碌状态进界面 + 命令面板

**FireAxe 侧：清单已收敛。** 本轮把 §4.2.1 里"先不做"的三项逐一核对后全部转为已完成（下载暂停/续传已是第 20 项、
任务期间移动互斥已是第 26 项、导入逐项报告已是第 27 项），剩下**只有一项**仍属"有意留到以后"：
`IValidity` 的增量级联失效（理由：本项目当前是脏标记 + 按需全量重算，2400+ Mod 仍是亚秒级；
增量传播要维护"文件 → 提供者"反查索引，收益不足时引入的风险大于收益）。另有两条**有意不做**
（`$type` 单存档多态、`AddonNodeFileFinder` 的文件夹树导入）已在 §4.2.1 写明冲突点。

> 后续（§5.13）：这一项已用**真实库实测**闭环 —— 先量出热重算 558ms，定位到根因是"索引缓存容量
> 写死 1024 < 库里的 2409 个 Mod"造成的重复解析，修好容量策略后降到 177ms（3.1×）；
> 剩下的 173ms 是 24.1 万条路径的分组累加，配合前端 400ms 防抖，判定**不需要**再做增量传播。

**UI 侧本轮两项**：

1. **把第 26 项的忙碌状态推进界面**：状态栏出现「正在处理文件（移动 / 删除 / 打包）… 完成后可继续」，
   会写磁盘的批量按钮同时变灰。
2. **命令面板（Ctrl+K）**：16 条命令覆盖 7 个页面 + 常用动作（搜索 Mod、策略组管理、分组建议、
   加载顺序、冲突分析、体检、抓取工坊官方标签、界面设置、切换主题…），支持模糊检索、↑↓ 选择、
   Enter 执行、Esc 关闭，命中片段高亮（与 Mod 搜索同一套）。

**真机过程中修掉的两个问题**（都是"只有真跑才会暴露"）：

- 忙碌提示**一次都没出现**：① 1 秒轮询会整段错过 783ms 的打包 → 改成**后端事件驱动**
  （`file_operation_state`），轮询降为 3 秒兜底；② 监视器里 `document.hidden` 判断在窗口被遮挡时
  会永远跳过更新（WebView2 把被遮挡页面也标成 hidden）→ 去掉该判断。
- 命令面板的命中高亮**按类名数不到**：一度另起了 `mark.command-hit`，实际生成的是
  `mark.search-hit`（与 Mod 搜索同一套）→ 统一为一份样式，删掉多余类。

**真机验证（打包 EXE + 沙箱，调试桥用后即删，产物确认无 `cua-bridge`）**：

```text
忙碌状态：打包 400MB 期间 → 提示可见=true、文案「正在处理文件（移动 / 删除 / 打包）… 完成后可继续」、
          body.is-file-operation-busy=true、移动按钮 pointer-events=none；
          打包结束 → 提示隐藏、class 移除
          事件通道独立探针：[true,false]（进入/退出各一次），打包耗时 783ms
命令面板：Ctrl+K → 面板可见、输入框聚焦、列出 10/16 条、底部「共 16 条命令 …」；
          检索「冲突」→「冲突分析（开关）」且 1 处高亮；检索「字号」→ 界面设置（关键字命中，标题不高亮）；
          检索「关于」→ Enter → 面板关闭、当前页面切到 about；再开 → Esc 关闭
```

### 5.9 2026-09-26 第七轮：对照表机器核对 + 命令面板可发现性 + 窗口几何一键重置

**对照表机器核对（本轮新增的完成度证据）**：写了一次性脚本，把本文 §1 / §4 表格里
**反引号引用的仓库内路径**（`internal/` / `frontend/` / `docs/` 开头，含 `.go` / `.mjs` / `.md` …）
逐个做存在性检查：

```text
对照表引用仓库内文件：74 个
全部存在：ok
```

核对过程中发现并修掉 4 处"缩略路径"（`frontend/.../xxx.test.mjs`）——
这类写法无法被机器校验，已全部改成完整路径。FireAxe 上游的 `.cs` 文件（本地参照源码之外）按设计跳过。

**UI 侧两项**：

1. **命令面板可发现性**：标题栏新增搜索图标按钮（`#command-palette-btn`）打开面板；
   面板空查询时直接把全局快捷键列在底部（`Ctrl+K 命令面板 / Ctrl+F 搜索 / Ctrl+±/0 缩放`）。
2. **窗口几何一键重置**：新增命令「重置所有窗口的位置与大小」→
   `resetAllWindowGeometry()` 清掉 `lytvpk.floatingPos.*` 全部记忆并复位当前浮动窗口，
   是"窗口被拖到屏幕外"的自救入口。

**真机验证（打包 EXE + 沙箱，调试桥用后即删，产物确认无 `cua-bridge`）**：

```text
标题栏按钮：存在=true → 点击后面板可见=true
空查询提示：共 17 条命令 · ↑↓ 选择，Enter 执行，Esc 关闭 · 快捷键：Ctrl+K 命令面板 / Ctrl+F 搜索 / Ctrl+±/0 缩放
重置命令：预置 lytvpk.floatingPos.lightweight-check → 检索「重置」得到「重置所有窗口的位置与大小」
          → Enter → 该键已被清掉（false）、面板关闭
再次 Ctrl+K → 快捷键提示仍在 → Esc 关闭
```

每条结论的证据分级：

- **已自动验证**：上面 8 条证据覆盖的语义，以及各任务测试文件里逐条断言的行为。
- **只能人工验证**：真实游戏内加载顺序/覆盖方向、真实工坊链接与合集、纯视觉布局与交互手感、
  真实回收站与真实网络下载失败重试。清单见 `docs/development/manual-verification.md`。
- **仍未验证**：截至本次交付，没有"计划内但完全未验证"的项目；
  未覆盖的都是上面明确列出的"只能人工验证"项。

需要注意的一条**有意保留的不确定性**：`addonlist.txt` 的覆盖方向（靠后生效还是靠前生效）
仍以实测为准，本项目把它集中在 `conflict.go` 的 `conflictOrderWins` 一处，
并且在 `docs/development/manual-verification.md` 里给出了受控实验步骤。

### 5.10 2026-09-26 第八轮：把对照表"未覆盖文件"重新扫了一遍（第 28-31 项）

**这一轮不是照着清单抄，而是先把 FireAxe.Core 里对照表**没有**引用过的源码读了一遍**
（`AddonDependencyProblem`、`AddonFileMissingProblem`、`InvalidPublishedFileIdProblem`、
`WorkshopVpkMetaInfo`、`ValidRef` / `ValidTaskCreator`、`IValidityExtensions`、`GamePathUtils` 等），
再挑出真正能落到本项目、且不与既有优势冲突的设计：

| # | 学到的东西 | 为什么值得学 |
| --- | --- | --- |
| 28 | `AddonDependencyProblem` | 它是上游**唯一**声明"可自动修复"的问题（全仓 grep `CanAutomaticallyFix => true` 只有这一处）。本项目已有 `dependency_disabled` 体检项与 `EnableModDependencies` 接口，但两者没接起来 —— 补上之后体检结果第一次能"点一下就按你声明过的依赖修好" |
| 29 | `AddonFileMissingProblem.FileTypeMismatch` | 上游把"该是文件却是目录"从"文件不存在"里拆出来。本项目原来对 `addons\foo.vpk` 是个文件夹的情况会报"磁盘上找不到对应文件"，**描述是错的**（文件夹就在那儿），处理方式也不同 |
| 30 | `InvalidPublishedFileIdProblem` + `WorkshopVpkMetaInfo.PublishedFileId` | 上游用 `metaInfo.PublishedFileId != publishedFileId` 判定本地记录不可信。本项目之前完全不查这条：`workshop\123.meta` 里写着别的作品时，更新检测会拿**另一个作品的时间戳**比较，静默漏报 / 误报 |
| 31 | `ValidRef` / `ValidTaskCreator` / `RegisterInvalidHandler` | `IValidity` 家族的另一半：不只是"缓存失效"，而是**异步任务持有的目标失效后不许再落地结果**。本项目 `runModUpdateCheck` 正好有这个洞 —— 检测请求还在飞行中、用户把 Mod 移进 `disabled`，旧代码会把时间戳写回已经不存在的路径（留下孤立 `.meta`） |
| 32 | `GamePathUtils.CheckValidity` / `TryFind` | 上游找游戏目录是"注册表 → 逐盘扫描 → 用 `left4dead2` 子目录验证"。本项目的 `AutoDiscoverAddons` 只有 5 条硬编码相对路径，**自定义库名**（`D:\Games\SteamLibrary`、`F:\steam-libs\lib2`）找不到；而且没有"这里真的装了游戏"的验证，猜中的可能是空目录。补上之后多解析一层 `libraryfolders.vdf`，一次读清单胜过扫全部盘符 |
| 33 | `FileSystemUtils.ThrowIfPathInvalid` / `FileOutOfAddonRootException` | 上游对**每一个**进入模型的路径都做"合法 + 在受管根内"校验（`AddonNode.cs:405-413` 直接 `StartsWith("..")` 就抛异常）。本项目此前只有备份名 / 崩溃报告 / 白名单批次做了同类守卫，**真正的文件操作（删除、移动源、隐藏改名）只检查"文件存在"**：用户中途换过 addons 目录、或前端状态陈旧时，一条过期路径就会被照做。现在这层补上了，同时体检会把"addonlist 条目指向受管目录之外"单独报出来 |

**证据分级**：

- **已自动验证**：第 28-31 项的全部语义。
  - 28：`TestDependencyDisabledIssueCarriesFixTarget`（用体检结果里的 `Target` 直接驱动修复入口 → `dep-off.vpk` 被开启、`before-dependency-fix` 备份存在、复查后该问题消失）；前端 `healthIssueAutoFixAction` 白名单 + `fixTarget` 透传 + 设置页接线断言。
  - 29：`TestHealthCheckReportsFileTypeMismatchForDirectoryNamedVpk`（目录占位只报 `file_type_mismatch`，真正的缺失仍报 `missing_file`）、`TestHealthCheckFileTypeMismatchPointsAtDisabledCopy`（`disabled` 有副本时提示出路）。
  - 30：`TestHealthCheckReportsWorkshopMetaIDMismatch`（ID 不一致报 1 条且不误判成孤立 / 缺失；对得上的不报）。
  - 31：`TestModTaskTargetGuardDetectsInvalidTargets`（未改动有效 / 被替换失效 / 目录无效 / 被删除失效）+ `TestUpdateCheckSkipsMetaWriteWhenTargetMovedMidFlight` / `TestUpdateCheckWritesMetaWhenTargetUnchanged`（用注入的详情获取器复现"检测中途移走"的竞态：跳过写入且 `SkippedStale=1`，目标没动则正常写回）。
  - 32：`TestParseSteamLibraryFoldersHandlesBothFormats`（新版 `path` 写法 + 旧版 `"1" "E:\..."` 写法 + 转义与大小写去重 + 脏内容不猜）、`TestFindAddonsInSteamLibrariesUsesRegistryAndLibraryFolders`（注册表目录优先，其次 `libraryfolders.vdf` 登记的库；注册表为空不猜；没有 `addons` 不返回）、`TestSteamLibraryCandidatesIncludeConfigFolder`、`TestAddonsPathIfGameRequiresLeft4Dead2Directory`、`TestParseSteamLibraryFoldersIgnoresUnrelatedPaths`。
  - 33：`TestManagedFilePathProblemDetectsOutsidePaths`（addons / workshop / 更深一层 / disabled 放行；父目录、`..` 逃逸、受管目录本身、空路径、相对路径拦下；大小写不敏感；没选目录一律不放行）、`TestDeleteVPKFileRefusesPathsOutsideManagedRoots`、`TestToggleVPKVisibilityRefusesPathsOutsideManagedRoots`、`TestMoveVpkFilesKeepsFreeDestinationButGuardsSources`（**同时断言"移动到用户自选目录"没被退化**）、`TestHealthCheckReportsAddonListEntriesOutsideManagedRoots`。
- **真机验证**（打包 EXE + 临时调试桥，**沙箱配置目录**，验收后桥已删除并重建产物，产物确认不含 `cua-bridge`）：

```text
环境：fixture 根目录（addons 下放 real.vpk / master.vpk / dep-off.vpk，ghost.vpk 是文件夹，
      workshop\123.vpk + 指向作品 999 的 123.meta），沙箱 %AppData% 里预置一条依赖声明
      master.vpk → dep-off.vpk

设置 → 游戏配置 → 开始体检
  摘要：发现 4 个问题：警告 3、提示 1
  汇总条：其中 1 项可以自动修复：启用依赖（1）。其余问题需要人工判断，不会自动改动任何文件。
  问题行：依赖未开启 · master.vpk        → 有「修复」按钮（data-fix-action=enable-dependencies）
          同名路径是文件夹 · ghost.vpk   → 无修复按钮（新诊断，第 29 项）
          工坊信息对不上作品 · 123.meta  → 无修复按钮（新诊断，第 30 项）
          未写入开关记录 · real.vpk      → 既有项

点依赖行的「修复」→ 确认 →
  体检摘要 4 → 3（依赖行消失），其余三条不受影响
  磁盘 addonlist.txt：dep-off.vpk 由 "0" 变 "1"，标签 / 缩进 / 其余条目原样保留

隔离性核对：真实 %AppData%\LytVPK 19 个文件在开跑前后逐文件 SHA256 完全一致
           （全部写入都落在沙箱配置目录）
```

**第 32 项的真机数据**（只读探针：真读注册表与真实的 `libraryfolders.vdf`，跑完即删）：

```text
注册表 Steam 安装路径 = "C:\\steam"
libraryfolders.vdf 解析出 5 个库：C:\steam, G:\SteamLibrary, D:\SteamLibrary,
                                  E:\SteamLibrary, F:\SteamLibrary
findAddonsInSteamLibraries(...) = E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2\addons
AutoDiscoverAddons()            = 同上（命中用户真实游戏目录，err=nil）
```

说明：这台机器的库目录名恰好是 `SteamLibrary`，所以旧的"盘符 + 固定路径"扫描**也**能命中；
新逻辑的收益在**自定义库名 / 自定义库位置**（例如 `D:\Games\SteamLibrary`）与
"不再逐盘盲扫"（先读注册表 + 一份清单文件）。

**对照表自检（承接 §5.9 的机器核对）**：本轮新增引用后重新跑一次，本文引用的仓库内文件
`95 个 / 真实缺失 0`（§5.9 里那个 `frontend/.../xxx.test.mjs` 是"缩略路径坏例子"的引文，不计入）。

- **只能人工验证**：真实工坊链接下的更新检测（第 31 项需要一个真实"检测中途移走"的时机，属于偶发竞态，
  自动化用注入的详情获取器复现了同一时序）；真实 2400+ Mod 库上重新体检的新增 / 消失条目是否符合直觉。
- **仍未验证**：`IValidity` 的增量级联传播（见 §4.2.1 最后一行）——这一项本轮明确不做，理由已写明。

### 5.12 2026-09-26 第十轮：把对照范围扩到 `FireAxe.GUI`（第 34 项：对象解释层）

§4.2.2 收口的只是 **`FireAxe.Core`**。这一轮把 **`FireAxe.GUI`（129 个文件）** 也扫了一遍，
挑出与"直觉交互"最相关、且本项目确实缺的一件事：**对象解释层**。

上游做法（`FireAxe.GUI`）：`ObjectExplanationManager.Get(obj, arg)` 按类型链逐级回退，
**最后一定返回一句话**（`ObjectExplanationManager.cs:27-49`）；`ExceptionExplanationScene`
区分"输入场景"和默认场景，同一个异常在输入框里说"输入不合法"、在别处说"发生异常"
（`ExceptionExplanations.cs:12-44`）；每种 Problem 注册一句人话（`AddonProblemExplanations.cs:10-30`）。
它被用在四处：操作失败提示、导入结果逐项原因、自动命名输入校验、通用消息框
（`TaskOperationsProgressNotifier.cs:57`、`AddonImportResultViewModel.cs:48`、
`AddonNameAutoSetJobViewModel.cs:131`、`Views/CommonMessageBoxes.cs:275`）。

本项目此前的问题：错误提示只有后端原文（虽然具体，但**不告诉用户下一步**），
而"按钮为什么是灰的"没有任何解释。落地为 `frontend/src/js/core/action-explanation.mjs`：

| 上游设计 | 本项目落点 |
| --- | --- |
| 类型链回退 + 永远有话说 | 规则按顺序匹配；认不出时**保留原文** + 通用建议（绝不出现空白提示） |
| 场景区分（Default / Input） | `sceneFromErrorType(type)`：`输入 / 校验 / 重命名 / 格式 / 解析` → 输入场景，其余 → 操作场景 |
| 每种 Problem 一句人话 | 12 条错误规则（路径守卫 / 闸门忙 / 工坊需转移 / 没选目录 / 正则写错 / 文件名不合法 / 文件不在 / 目标路径不可用 / 同名已存在 / 权限 / 空间 / 网络） |
| 解释用在"操作失败"上 | `toast.js` 的全局错误事件：`内容：<人话>` + `建议：<下一步>`，原文仍写控制台 |
| —（上游没有） | `explainActionAvailability`：**同一个判断既决定按钮能不能点、又决定提示文案**（游戏内开关 / 禁用 / 批量 / 忙碌 / 无目录） |
| —（上游会丢） | 逐项失败**保留文件名**：`a.vpk：目标路径当前不可用（确认目标目录还在…）` |

**证据分级**：

- **已自动验证**：`frontend/src/js/core/action-explanation.test.mjs`（8 条错误规则 + 场景区分 +
  兜底非空 + 文件名提取 + 可用性 6 类 + 三处接线断言 + 旧重复文案已移除）、
  `frontend/src/js/features/file-list/move-result-format.test.mjs`（逐项文案带文件名与人话）。
- **真机验证**（打包 EXE + 临时调试桥，沙箱配置目录，验收后桥已删除并重建，产物确认不含 `cua-bridge`）：
  用导出的 `LogError(type, message, file)` 走**真实链路**（Go 事件 → `EventsOn("error")` → 解释层 → DOM）：

```text
文件操作 / 移动 a.vpk 失败: The system cannot find the path specified
  → 内容：目标路径当前不可用  建议：确认目标目录还在、且没有被同名文件占住，然后重试。
文件操作 / 另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试
  → 内容：上一个文件操作还没结束  建议：等状态栏的提示消失后再试…
文件操作 / 这个文件不在当前受管的 addons / workshop / disabled 目录里：E:\x.vpk
  → 内容：这个文件不在当前管理的 addons / workshop / disabled 里  建议：刷新一次 Mod 列表…
重命名 / 文件名不合法：文件名不能包含 \ 这类字符（Windows 命名规则）
  → 内容：这个名字不能用作文件名（**输入场景**才会这么解释）  建议：避开 < > : " / \ | ? * …
文件操作 / 某种没见过的后端错误 XYZ-123
  → 内容：某种没见过的后端错误 XYZ-123  建议：如果反复出现，把这条消息和对应的 Mod 名一起反馈。
```

- **只能人工验证**：真实使用中这些提示的措辞是否符合你的语感（规则表集中在
  `frontend/src/js/core/action-explanation.mjs`，改文案不需要动业务代码）。
- **仍未验证**：`IValidity` 的增量级联传播（`FireAxe.Core` 侧唯一有意留后项）。

`FireAxe.GUI` 其余文件（视图、值转换器、设置模型、i18n、主题、崩溃上报等）与本项目既有实现
一一对过：视图/主题/字体是 WPF-Android 风格，本项目是 Web 技术栈，没有可移植的增量；
`CrashReporter.cs` 对应第 10 项、`CultureManager.cs` 对应第 9 项、`AppColorTheme.cs` 对应界面设置里的主题与阅读舒适度（本轮刚细化）。

### 5.13 2026-09-26 第十一轮：冲突索引缓存容量（把 `IValidity` 那一项用实测钉死）

这一轮回到 FireAxe 侧最后一项"未做"（增量级联失效），但**先测量再决定**，而不是凭感觉实现或搁置。

**测量（真实库：2409 个 Mod / 2107 个参与 VPK / 24.1 万条归档路径，只读）**：

```text
全量冲突复检：冷 591ms / 热 552ms / 560ms / 559ms      ← 热跑与冷跑一样慢，说明每轮都在重解析
细分：遍历文件系统 7.7ms；取文件清单循环 首次 436ms / 再次 434ms；缓存条目 1024（正好等于上限）
全量索引代价：解析 2408 个 VPK 392ms；路径 241,458 条；内存约 14.4 MB
```

**根因**：`conflictIndexCacheMax = 1024` 写死，而库里 2409 个 Mod > 1024 → LRU 每轮把 1300+ 条目挤出去、
下一轮重新解析。**不是缺少增量传播，是容量 bug**。

**修复**：容量跟随本轮候选数（`conflictIndexCapacityFor`：至少 1024、最多 8192 的硬上限防病态目录），
冲突检测与体检深度扫描两个入口都会设置它。

**修复后同一份真实库复测**：

```text
全量冲突复检：冷 585ms / 热 173ms / 179ms / 180ms      ← 3.1×
细分：缓存条目涨到 2107（= 参与 VPK 数）；候选收集 4ms；忽略清单 0ms
剩余 173ms = 2107 个 Mod × 24.1 万路径的分组累加
```

**为什么仍然不做增量传播**：前端对复检有 **400ms 防抖**（连续开关合并成一次），
所以 177ms 是"一次突发一次"；增量传播要维护"文件 → 提供者"反查索引 + 失效簿记，
收益约 170ms/次突发 —— 风险与复杂度大于收益。这条结论现在有数字支撑，写在 §4.2.1 里。

**证据分级**：

- **已自动验证**：`internal/app/conflict_index_capacity_test.go` —— 容量规则（含硬上限）、
  容量真的影响淘汰（容量 2 只留 2 条 / 容量足够时全部保留）、默认容量、端到端"冲突检测后容量被写入且不超限"。
- **真机测量**：上面两组数字都来自真实库（只读；测量脚本用后即删）。
- **仍未验证**：无（这是 FireAxe 对照表里最后一项，现在有了实测结论）。

### 5.11 2026-09-26 第九轮：受管根路径守卫（第 33 项）+ 归档列表统一检索

**FireAxe 侧（第 33 项）**：把 `FireAxe.Core` 里"对照表未引用"的 42 个文件逐个看完后，
只有三类文件还带真实语义：路径合法性/根内约束、存档反序列化失败的处理、官方标签词汇表。
其中**只有路径守卫在本项目确实缺失**（另两类：损坏存档我们已有 schema 迁移与备份轮转、
官方标签我们直接抓 Steam 官方 `tags[]`，都不需要照搬）。

已落地：

1. `internal/app/path_guard.go`：`managedFilePathProblem(root, path)` 要求绝对路径 +
   落在 addons / workshop / disabled 之内；`..` 逃逸、把受管目录本身当目标、相对路径、空路径一律拦下；
   大小写不敏感（Windows 路径语义）。
2. 接在三个**会改动受管文件**的入口：`DeleteVPKFile`、`MoveVpkFiles`（源文件）、`ToggleVPKVisibility`。
3. **刻意不限制目标目录**：`moveSelected()` 用的是 `SelectDirectory()`，用户可以移到任意目录
   （备份盘、另一个 Mod 库），守卫只约束源文件 —— 测试里专门有一条断言防止这条能力被"顺手收紧"。
4. 配套体检项 `outside_root`：`addonlist.txt` 的条目解析到受管目录之外时单独报一类
   （旧实现会因为"文件确实存在"而什么都不报）。

**证据分级**：

- **已自动验证**：`internal/app/path_guard_test.go`（10 类路径判定 + 3 个端到端断言：
  越界删除/改名被拒且文件零改动、越界源文件逐项失败并留在原处、**移动到用户自选目录仍然成功**、
  体检报 `outside_root` 且不误报 `missing_file`）；前端白名单/文案由
  `frontend/src/js/features/settings/health-report-format.test.mjs` 覆盖。
- **只能人工验证**：真实"中途换 addons 目录"的操作手感（自动化用等价路径复现了同一判定）。
- **仍未验证**：无（本轮计划内项目都已覆盖）。

**UI 侧（同一轮）**：把 Mod 列表的搜索语法推广到**归档管理器**——
新增 `frontend/src/js/features/diagnostics/archive-search.mjs`（复用 `search-syntax.mjs` 解析，
普通词按 `fuzzyMatch` 语义跨"包名 / 路径 / 每个 VPK 名与路径"匹配、`-词` 排除、`re:` 正则、
`tag:` 落在**包状态**上：密码 / 错误 / 已有 / 待导入，支持 `|` 或），
界面补上计数文案（`匹配 2 / 7 个压缩包`、正则写错时直接说原因）、命中的包名高亮
（与 Mod 列表同一套 `<mark class="search-hit">`）、搜索框语法提示与 `tag:` 命中 chip。

```text
node --test → 291 项，0 失败（新增 archive-search.test.mjs 9 项）
```

### 5.14 2026-09-26 第十七轮：把上游 CHANGELOG 里最后两项能力对完（第 35 / 36 项）

**这一轮的起点**：用户问"FireAxe 还有没有值得学的"。前十六轮是按**源码文件**逐个核对
（§4.2.2 Core 75 个 / §4.2.3 GUI 177 个），这轮换成按**上游 CHANGELOG** 逐条对（v0.2.0 – v0.7.3），
结果发现两个只在 CHANGELOG 与 GUI 里出现、文件级核对漏掉的能力：

1. **v0.4.0 自动识别剪贴板里的工坊链接**（`MainWindowViewModel.cs:299` 起 0.5s 轮询，
   `AppSettings.cs:348` 的默认开启开关）。
2. **v0.5.1 `Ctrl+X` / `Ctrl+V` 移动 Mod**（`AddonNodeExplorerView.axaml.cs:223-259`
   的 XButton1 回上级组、Cut / MoveHere 命令）。

**落地方式（按本项目模型改写，不照抄）**：

| 上游 | 本项目 | 差异与理由 |
| --- | --- | --- |
| 0.5s 无条件轮询剪贴板 | `CheckClipboardWorkshopLink`（Go 读 + 解析）+ 前端 `shouldPollClipboard`（开关 + 窗口前台 + 1.5s + 内容去重） | 不打扰用户；解析规则与上游一致（sharedfiles / workshop 链接、纯数字 ID） |
| 认链接 `TryParsePublishedFileIdLink` | 多认一种写法：`id` 不必是第一个查询参数（`?l=schinese&id=42` 也认） | 上游正则要求 `?` 后紧跟 `id=` |
| Ctrl+X 剪切到"当前组" | Ctrl+X 标记 + Ctrl+V 选目标目录（复用现有 `SelectFolder` 链路） | 本项目列表是目录视图，没有"当前组"这一层；策略组层级在独立窗口里 |
| 鼠标侧键回上级组 | 不适用 | 同上，不引入"上级组"概念 |

**真机驱动发现并修掉的两个真 bug**（这轮最有价值的部分，都是自动化测试原本照不到的角落）：

1. **状态栏引用了未声明的变量**：`updateStatusBar()` 里用了 `pendingMoveEl` / `pendingMoveCount`，
   但声明被补丁插到了 `updateSelectedFilesStatus()` 里。表现是"Ctrl+X 标记成功，但界面毫无反应、
   连提示都没有"（`ReferenceError` 吞掉了后续的 toast）。已抽成 `syncPendingMoveIndicator()` 一处读写，
   并新增 `frontend/src/js/features/status-bar-pending-move.test.mjs`（真实 import `state.js` +
   最小 DOM 桩）。**做了变异验证**：把 bug 形态放回去 → 测试报 `ReferenceError: pendingMoveEl is not defined`。
2. **勾选框拿到焦点时 Ctrl+X / Ctrl+V 被当成"在输入框里打字"跳过**：原判定把所有 `<input>` 当输入框，
   而列表行的勾选框就是 `<input type="checkbox">`。已抽 `isTextEntryElement()`（只认文本类 input /
   textarea / contenteditable），并配 `frontend/src/js/core/shortcut-guards.test.mjs`。

**另外确认的一件事（不是 bug）**：上一轮"打包 EXE 启动后立即退出"的现象，本轮复现时环境里已没有其它实例，
应用能正常启动、扫描夹具、响应 F2 / Ctrl+F / Ctrl+K；当时的退出与"用户自己的实例 + 同一份实现"有关，
不再作为疑问挂账。

**证据分级**：

- **已自动验证**：`go test ./... -count=1`（含 `workshop_clipboard_test.go` 的 10 类输入）、
  `node --test`（351 项，0 失败）、`npm run build`、`wails build`、docs 的 `npm run docs:build`。
- **真机验证（打包 EXE + 沙箱配置目录 + 夹具 VPK）**：`F2` 弹出重命名提示、`Ctrl+F` 聚焦搜索框、
  `Ctrl+K` 打开命令面板、`Esc` 关闭浮层、勾选行后状态栏 `已选择: N` 更新、夹具 7 个 VPK 正常列出；
  本轮还顺带发现：**沙箱 config.json 必须写合法 JSON，否则应用会回退到"自动发现游戏目录"**
  （按 Steam 注册表找到真实库；本轮为此核对过用户真实 addons 目录，40 分钟内零写入）。
- **只能人工验证**：`Ctrl+X` / `Ctrl+V` 的端到端手感（自动化键盘注入在打包 WebView2 里时通时不通，
  同一条 `Ctrl+K` 也会间歇失效，无法作为判据）、剪贴板提示在真实前台窗口下的出现时机、
  换屏幕后主窗口尺寸的还原效果。清单见 `docs/development/manual-verification.md` 的对应小节。
- **仍未验证**：无。

### 5.15 2026-09-26 第十八轮：把最后一项"有意不做"变成"已对齐"（自定义外部打开程序）

**起点**：上一轮把 v0.7.2 的 process file customization 记成"有意不做（可选）"。用户确认要加，
但要求**默认仍然用系统默认**。这一轮就按这个约束实现。

| 上游 | 本项目 | 说明 |
| --- | --- | --- |
| 两对设置（打开目录 / 显示文件各一对） | 一对设置 + 占位符：`{path}`（完整路径）· `{dir}`（所在目录）· `{name}`（文件名）· `{0}`（兼容上游，等价 `{path}`） | 一个"打开所在位置"入口即可覆盖两种用法：模板写 `{dir}` 就是开目录、写 `{path}` 就是开文件 |
| `string.Format(args, path)` | `splitOpenWithArguments` + `strings.NewReplacer` | 参数做过分词：空白分隔、引号内保留空白、**反斜杠只在引号前特殊**（Windows 约定） |
| 两项都非空才生效 | 程序为空即"未配置"（参数一并清空，避免残留看起来生效） | `OpenFileLocation` 里先查配置；未配置时原来的 `explorer /select`（macOS `open -R`、Linux `xdg-open`）逻辑**一行没动** |
| 直接 `Process.Start(fileName, args)` | `exec.Command(program, args...)`，不经 shell | 不引入命令行注入面；启动失败给人话提示（指向设置位置） |

**真机 + 端到端证据**：

- **已自动验证**：`internal/app/open_with_test.go` —— 分词 11 类（含"Windows 路径反斜杠必须原样保留"）、
  组装 5 类（未配置 / 空模板 / 三个占位符 / `{0}` / 无占位符追加路径）、校验 4 类（空 / 纯命令名 /
  真实存在 / 带路径但不存在）、配置往返（落盘 + 留空清参数 + 不存在程序报错），
  以及一条**真启动外部程序的端到端**：配置 `xcopy.exe /Y {path} <目录>` → 调 `OpenFileLocation` →
  夹具文件真的被复制出来（内容一致），随后清空配置再断言"回到系统默认（不再走外部程序）"。
- **真机（打包 EXE + 沙箱夹具）**：设置页「界面设置」里新卡片正常渲染、整页无报错（截图确认）。
- **只能人工验证**：在设置页里点「保存 / 恢复默认」并观察 config.json 落值 ——
  本环境的自动化鼠标点击对这两个按钮不可靠（同一脚本点导航标签有效、点按钮只拿到焦点），
  因此这一条列进 `docs/development/manual-verification.md`。
- **仍未验证**：无。

**这一轮修掉的一个真问题**：分词器最初把 `\` 当转义符，`C:\Tools\tool.exe` 会被吃成 `C:Toolstool.exe`。
正是那条"真启动 xcopy"的端到端测试暴露出来的（单测里的模板恰好没有反斜杠）。
另外把设置页两个按钮改成**事件委托**绑定：设置面板会整块重渲染，直接绑在按钮上的监听器会失效。
