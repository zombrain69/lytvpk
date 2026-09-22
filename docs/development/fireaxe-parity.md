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

## 1. FireAxe 概念 → 本项目落点

| FireAxe 概念 | FireAxe 证据（file:line） | 本项目落点 | 状态 |
| --- | --- | --- | --- |
| 显式可编辑优先级 `Priority` | `AddonNode.cs:139`（setter 标记 `RequestSave`）、`AddonNodeSave.cs:22`（持久化字段） | `internal/app/priority.go` 的 `ModPriorityEntry` + `priority.json`；`SetModPriority` / `ClearModPriority` | 已对齐（Task 1） |
| 层级优先级累加 `PriorityInHierarchy` | `AddonNode.cs:154-168`（自身 + 全部祖先的 `Priority` 求和） | `computeEffectivePriority`：自身分层 + 策略组权重取 `min`（可重叠集合语义，见设计文档第 3 节） | 有意取舍（Task 1） |
| 冲突只在同优先级内判定 | `AddonConflictUtils.cs:96`（按 `priority` 分桶）、`:249-262`（同优先级才互为冲突方） | `decideConflictOwners`：有效分层相同 → 真冲突；全部不同 → 覆盖关系 | 已对齐并加强（Task 1） |
| 内置忽略清单 | `AddonConflictUtils.cs:48-55`（`addoninfo.*`、`sound/sound.cache`、4 个 `*_addon.nut`） | `conflict.go` 的 `isIgnoredConflictFile` + 设置页全局清单 | 已对齐，逐项补齐（Task 2） |
| 忽略集合取并集 | `AddonConflictUtils.cs:11-46`（静态集合 + 动态 provider 依次命中即忽略） | 内置规则 ∪ 全局清单 ∪ 单 Mod 清单 | 已对齐（Task 2） |
| 单个 Mod 的忽略文件 | `VpkAddon.cs:32`（`ConflictIgnoringFiles`）、`:270`/`:278`（保存/恢复）、`VpkAddonSave.cs:17` | `ignore.json` + `GetModIgnoreFiles` / `SetModIgnoreFiles` | 已对齐（Task 2） |
| 备份轮转 `BackUpIfNeed` | `AddonRoot.cs:972-1050`：最短间隔、与上一份内容相同则跳过、数量上限、溢出进回收站 | `internal/app/local_store_backup.go` | 已对齐（Task 3） |
| 变更驱动的问题失效与自动修复建议 | `Problem.cs:6-34`（`CanAutomaticallyFix` / `ShouldFixAutomatically` / `TryAutomaticallyFix`）、`IValidity.cs:5-8` | `conflict_recheck.go`：状态变化 → 失效 → 按需重算 + 修复建议（不自动改文件） | 已对齐（Task 4） |
| 组启用策略 | `AddonGroup.cs:34`（`EnableStrategy`）、`:130`（`CheckEnableStrategy`）、`:161-200`（应用策略） | `mod_groups.go` 的策略组（`all` / `off` / `single` / `single_random` + 可选自动联动） | 已对齐，语义改为显式动作 |
| 组权重 | `AddonNode.cs:154-168`（祖先链累加，等价于组权重） | `ModStrategyGroup.Tier *int` + 有效分层 `min` | 已对齐（Task 1） |
| push 写盘顺序 | `AddonRoot.cs:754`（`Push`）、`:899`（按 `PriorityInHierarchy` **降序**写 addonlist.txt） | `ApplyModPriorityLayers`：按有效分层**升序**稳定排序写回 | 已对齐，方向见 §3 待实测项 |
| 方案快照 | `AddonRootSave.cs:6-13`、`AddonNodeSave.cs:7-30`（`IsEnabled` / `Priority` / `Tags` / `DependentAddonIds`） | `ModEnableProfile` + `IncludesAutomation`（含 `Priorities`） | 已对齐（Task 1） |
| 引用型节点 `RefAddonNode` | `RefAddonNode.cs:7`、`:21`（`SourceAddonId`）、`:207`（循环引用检查） | 不引入：同一 Mod 的多方案由"启用方案"覆盖 | 有意不做 |
| 符号链接 push | `AddonRoot.cs` `Push()` 内 `File.CreateSymbolicLink` | 不引入：本项目直接管理真实文件 | 有意不做 |
| 自动更新工坊条目 | `WorkshopVpkAddon.cs:88-115`（`IsAutoUpdate` / 继承根设置）、`:428-466`（下载检查任务去重） | 既有更新检测 + `shouldAutoRedownload` / `maybeAutoRedownload`（默认关闭，只重试一次） | 已对齐（Task 6） |
| 下载服务 | `DownloadService.cs:523`（`Download(url, filePath)`）、`:230`（`Resume`）、`:244`（`Cancel`）、`:11`（进度落盘间隔 1s） | 既有分块下载 + 任务列表 + 可注入下载启动器（测试不触网） | 已对齐（Task 6 起） |
| 工坊合集实体化 | `WorkshopCollectionUtils.cs:10`（`GetWorkshopCollectionContentAsync`，支持嵌套合集） | `workshop_collections.go`：`collections.json` + 跟随节点刷新/下载缺失成员 | 已对齐（Task 7） |
| 树形分组 / 嵌套层级 | `AddonGroup.cs` 的父子容器 + `AddonNode.Parent` | `mod_group_tree.go`（`ParentID` + 树构建 + 环/深度校验）；扁平视图仍为默认 | 已对齐（Task 8） |

## 2. 本项目已经更强的部分（不得退化）

以下能力是 FireAxe 没有或更弱的，本次补齐过程中必须保持现状：

| 能力 | 证据位置 | 相对 FireAxe 的优势 |
| --- | --- | --- |
| 直接管理真实文件 | `internal/app/vpk_actions.go`、`addon_list.go` | FireAxe 依赖符号链接 push；本项目直接读写游戏真实文件 |
| addonlist 编码 / BOM 保真 + 原子写 | `addon_list.go:122`（`writeAddonList`：GBK/ANSI/UTF-16 探测与回写） | FireAxe 直接以 UTF-8 重写 addonlist.txt |
| 多类型备份与运行时监控恢复 | `addon_list_manager.go`、`lifecycle.go` | FireAxe 只备份 `.addonroot` 序列化结果 |
| 冲突严重度分级与可组合基线 | `conflict.go:354`（`getConflictSeverity`）、基线规则 | FireAxe 只给出"同优先级即冲突"，没有严重度与基线范围 |
| 工坊链路（翻译 / IP 优选 / 分块下载 / 转移 / 更新检测） | `workshop_*.go`、`workshop_translate.go` | FireAxe 使用 Steam 客户端回调，缺少翻译与转移链路 |
| 服务器面板与工具箱 | `server_panel.go`、`archive_manager.go`、`model_stats_scan.go`、`autoexec.go` 等 | FireAxe 无对应能力 |
| 体检与报告导出 | `health_check.go`、`problem_scan.go` | FireAxe 只有 Problem 体系，无报告导出 |

## 3. 已知语义假设（必须显式记录）

- **有效分层方向**：`effective` 越小越先加载，写在 addonlist.txt 越靠前。
  FireAxe 的 `Push()` 同样把高优先级写在前面（`AddonRoot.cs:899` 降序），方向一致。
- **覆盖方向**：本项目既有的用户文档语义是"越靠后加载越容易覆盖前面的资源"，
  因此冲突判定里"有效分层更大者获胜"集中在 `conflict.go` 的 `conflictOrderWins` 一处。
  该方向仍属于待实测项（见 `manual-verification.md`），一旦受控实验给出相反结论，只需改这一处。

## 4. 逐项落地状态

对照口径：**已对齐**=按上游语义落地并有自动化测试；**有意取舍**=只借鉴设计、按本项目模型改写；
**有意不做**=与既有优势冲突；**未完成**=尚未实现（本次交付后不存在此类项）。

| # | 功能 | 实现 | 自动化测试 | 文档 | 状态 |
| --- | --- | --- | --- | --- | --- |
| 1 | 统一优先级模型 | `internal/app/priority.go`、`conflict.go`、`mod_groups.go` | `internal/app/priority_test.go`（含黄金回归）、`frontend/src/js/features/file-list/priority-label.test.mjs` | 本文 + `docs/features/mod-management.md` | 已完成 |
| 2 | 每 Mod 冲突忽略文件 | `internal/app/mod_ignore.go`、`conflict.go` 扫描分支 | `internal/app/mod_ignore_test.go`、`frontend/src/js/features/modals/detail-ignore-key.test.mjs` | `docs/toolbox/conflict-check.md` | 已完成 |
| 3 | 本地记录备份轮转 | `internal/app/local_store_backup.go` + 5 个 store 写入路径 | `internal/app/local_store_backup_test.go` | `docs/features/settings.md` | 已完成 |
| 4 | 变更驱动自动复检与修复建议 | `internal/app/conflict_recheck.go` + addonlist/文件操作失效钩子 | `internal/app/conflict_recheck_test.go`、`frontend/src/js/features/conflicts/conflict-badge.test.mjs` | 本文 + `docs/toolbox/conflict-check.md` | 已完成 |
| 5 | 游戏原版文件白名单（分批可增量） | `internal/stockfiles/`、`internal/app/stock_whitelist.go` | `internal/app/stock_whitelist_test.go`、`internal/stockfiles/stockfiles_test.go` | `docs/toolbox/conflict-check.md` | 已完成（比上游多出“按类生成增量批次”） |
| 6 | 自动重下策略（默认关闭） | `internal/app/workshop.go`（`shouldAutoRedownload` / `maybeAutoRedownload`）、`workshop_download.go` 失败钩子 | `internal/app/auto_redownload_test.go`（注入下载启动器） | `docs/features/downloads.md` | 已完成 |
| 7 | 工坊合集实体化 | `internal/app/workshop_collections.go`（`collections.json`） | `internal/app/workshop_collections_test.go`、`frontend/src/js/features/settings/workshop-collection-format.test.mjs` | `docs/features/downloads.md` | 已完成 |
| 8 | 树形分组 / 嵌套层级 | `internal/app/mod_group_tree.go`（`ParentID` + 树构建 + 校验）、设置页层级渲染 | `internal/app/mod_group_tree_test.go`、`frontend/src/js/features/settings/strategy-group-tree.test.mjs` | `docs/features/mod-management.md` | 已完成（扁平视图仍为默认） |
| 9 | i18n 文案层 | `frontend/src/messages/zh-CN.json`、`frontend/src/js/core/i18n.mjs`、`i18n-runtime.js`；工具箱页面已迁移 | `frontend/src/js/core/i18n.test.mjs` | `docs/development/i18n.md` | 已完成第一阶段（运行时+目录+首屏迁移；其余页面按同一约定逐页迁移） |
| 10 | 自身崩溃上报 | `internal/app/crash_reporter.go`（日志环形缓冲 + 协程池守卫 + 启动守卫 + 前端上报）、关于页入口 | `internal/app/crash_reporter_test.go` | 本文 + `docs/features/about-update.md` | 已完成 |

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

每条结论的证据分级：

- **已自动验证**：上面 8 条证据覆盖的语义，以及各任务测试文件里逐条断言的行为。
- **只能人工验证**：真实游戏内加载顺序/覆盖方向、真实工坊链接与合集、纯视觉布局与交互手感、
  真实回收站与真实网络下载失败重试。清单见 `docs/development/manual-verification.md`。
- **仍未验证**：截至本次交付，没有"计划内但完全未验证"的项目；
  未覆盖的都是上面明确列出的"只能人工验证"项。

需要注意的一条**有意保留的不确定性**：`addonlist.txt` 的覆盖方向（靠后生效还是靠前生效）
仍以实测为准，本项目把它集中在 `conflict.go` 的 `conflictOrderWins` 一处，
并且在 `docs/development/manual-verification.md` 里给出了受控实验步骤。
