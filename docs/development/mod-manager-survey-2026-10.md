# 求生之路 2 Mod 管理器生态调研 + 本项目差距与落地规划（2026-10-07）

> 维护者文档，不进入公开文档站侧边栏。
> 调研执行：2026-10-07，本机库 **2960 个受管 Mod**（root 1707 / workshop 945 / disabled 308）。
> 所有"别人有什么 / 我们有什么"的结论都来自**真实源码或真机命令输出**，不是印象；每条给出可复现证据。

## 0. 结论摘要（先看这一段）

**本项目明显领先现有开源 L4D2 Mod 管理器的地方**（都有源码/测试证据，见 §4）：
冲突判定（严重度 + 优先级感知 + 原版文件白名单 + 忽略清单）、addonlist.txt 生命周期（编码/BOM 保真、
原子写、备份/恢复/监控）、标签识别（多通道证据 + 显式实体槽位 + 回归 allowlist 护栏）、Mod 体检与报告、
启用方案 / 策略组 / 依赖 / 统一优先级模型、服务器面板与工具箱（VPK 解包/打包/完整性/喷漆/autoexec）、
检索语法与结果光标、沙箱只读闸门。

**真正值得补的缺口**（按优先级，详见 §5）：

| 优先级 | 缺口 | 来源 | 状态 |
| --- | --- | --- | --- |
| P0 | VPK 列表排序设置持久化 | 上游 `7b0818c` | **已实现（2.7.1-community.65）**（config `sortType` / `sortOrder`） |
| P0 | 工坊 DNS 设置（列表/详情/链接解析单独选 DNS） | 上游 `5ce7ef5` | **已实现（2.7.1-community.66）**：系统 / 自定义两档 + 前后端校验 |
| P0 | Mod 快照（文件名快照 + 含文件内容的完整备份 + 恢复预览） | 上游 `ba20411` | **已实现（2.7.1-community.66）**：最小闭环（names/full + 恢复计划 + 执行前备份） |
| P1 | 加载顺序编辑器：失效/新增条目标记 + 保存时清理 | 上游 `1b74507` `a50cf4f` | **已实现清理 + 提示（2.7.1-community.66）**；条目级角标仍可再补 |
| P1 | 工坊直连弹窗自动填入剪贴板地址 | 上游 `a17e39a` | **已实现（2.7.1-community.66）** |
| P1 | GitHub 加速源列表更新 | 上游 `d367670` | **已实现（2.7.1-community.66）** |
| P1 | 用户自定义分类规则（正则 + 布尔表达式 + 规则编辑器） | `xavier-cai/L4D2ModManager` | **部分实现（2.7.1-community.66）**：JSON 规则（文件名/标题/标签/单条正则）+ 预览命中；未做可视化表达式编辑器 |
| P1 | 开启 Mod 时的"同类互斥"提示（SingletonResource 思路） | `xavier-cai/L4D2ModManager` | **已实现（2.7.1-community.66）**：三选一提示，判定只认具体角色/武器型号 |
| P1 | 按分类随机（每次启动从每个分类随机选一个） | `pukmajster/funky` | **已实现（2.7.1-community.66）**：轮换新增自定义分类输入 |
| P1 | VPK 内容内联预览（不解包直接看文本/KeyValues/贴图/模型） | `craftablescience/VPKEdit` | **已实现文本/KeyValues/图片/VTF（2.7.1-community.66）**；BS3/模型预览仍未做 |
| P2 | addoninfo `addonContent_*` 兜底分类 | `fdklgbh/L4D2Mod-Manager` 已知问题 | **真机验证收益低，建议不做**（见 §5.4） |
| P2 | 多分片地图识别（part 包没有 maps/ 或 missions/） | `fdklgbh/L4D2Mod-Manager` 已知问题 | 待观察（真机样本少） |
| P2 | 把多个 VPK 合并成一个 pak01_dir.vpk | `kublay-tayro/Mods4Versus` | 建议只做"导出整合包"，不替代 addonlist |
| P2 | 直接对接 Steam 客户端（订阅/取消订阅、下载进度） | `xavier-cai/L4D2ModManager` | 与现有 web/worker 路线冲突，待决策 |

## 1. 调研方法与可复现证据

| 对象 | 版本 / HEAD | 星数 | 语言 | 我们怎么读的 |
| --- | --- | --- | --- | --- |
| `LaoYutang/lytvpk`（本项目的上游） | `7b0818c`（2026-10-02） | 90 | JS/Go | `git fetch upstream` + `git log HEAD..upstream/master`（25 个提交） |
| `ktxiaok/FireAxe` | `f8aa1cf`（v0.7.3，2026-01-02） | 177 | C# | 已 shallow clone；与我们 `docs/development/fireaxe-parity.md` 的基线**相同**（上游未前进） |
| `xavier-cai/L4D2ModManager` | `755dd96`（2020-07-21） | 66 | C# | shallow clone + 读 `SourceCode/L4D2ModManager/*.cs` |
| `pukmajster/funky` | `main`（2026-02-15） | 54 | TypeScript | shallow clone + 读 `src/main/*.ts`、`src/renderer/src/features/library/**` |
| `fdklgbh/L4D2-Mod-Manager` | `master`（2026-08-18） | 23 | Python | shallow clone + README（含作者自述已知问题） |
| `auto-created-name/L4d2_Mod_Manager_Tool` | `release`（2023-07-15） | 18 | C# | shallow clone + README |
| `Anokianx/L4D2-Danyria-Integration-Tool` | `a97a343`（2026-10-01） | 13 | Python | shallow clone + README |
| `TheCraZyDuDee/L4D2-Mod-Manager` | `main`（2025-06-18） | 8 | Python | 只读 README（无参考价值，见 §2.8） |
| `kublay-tayro/Mods4Versus` | `main` | 0（新项目） | Rust/Tauri | shallow clone + 读 `src-tauri/src/commands.rs`、`vpk_utils.rs` |
| `kinten108101/steam-vpk` | `main`（2024-03-18） | 2 | TypeScript/GJS | shallow clone + README |
| `bahyqn/l4d2mm` | `ce85b61`（2026-09-13） | 0 | Go | shallow clone + 读 `internal/**` |
| `craftablescience/VPKEdit` | `main`（2026-09-15） | 760 | C++ | GitHub API + README（通用 VPK 工具，非 L4D2 专用） |

检索方式（可复现）：

```powershell
# GitHub 检索（本次用过的主要查询）
$h = @{ 'User-Agent' = 'codex-survey'; 'Accept' = 'application/vnd.github+json' }
Invoke-RestMethod -Headers $h 'https://api.github.com/search/repositories?q=l4d2+mod+manager&sort=stars&per_page=15'
Invoke-RestMethod -Headers $h 'https://api.github.com/search/repositories?q=topic:l4d2&sort=stars&per_page=10'

# 上游缺口分析（在仓库根目录执行）
cd <仓库根目录>
git fetch upstream --tags
git log --date=short --pretty='%h %ad %s' HEAD..upstream/master

# 外部源码（本次 clone 到 .tmp-cua\survey\，属临时目录，不进仓库）
git clone --depth 1 https://github.com/xavier-cai/L4D2ModManager.git
git clone --depth 1 https://github.com/pukmajster/funky.git
```

## 2. 上游 LaoYutang/lytvpk：25 个提交的缺口分析

我们的 fork 基线是 `c977064`（2026-08-18），上游已经走到 `7b0818c`（2026-10-02）——**相差 25 个提交**。
逐条核对结论（"已对齐"= 我们用自己的实现覆盖了同等能力，注释/CHANGELOG 里带 `对齐上游 <sha>` 标记）：

| # | 提交 | 内容 | 我们的状态 |
| --- | --- | --- | --- |
| 1 | `d0d406b` | 创意工坊批量转移 + 自动 meta 获取 | 已自研对齐（`internal/app/workshop_transfer.go` + `meta.go`，v2.5.14-community.58 发布） |
| 2 | `5f2eb49` | 窗口保持关闭前大小 | 已覆盖（config `mainWindowWidth/Height/Maximised`，真机 config.json 实测有值） |
| 3 | `c1b4972` | `favoriteServer` 外部协议 | 已对齐（`internal/platform/protocol/url_protocol.go` 有 `对齐上游 c1b4972` 注释） |
| 4 | `cd93ea6` | CF 工坊接口 IP 令牌桶 | **未覆盖**：`worker/steam_workshop_worker.js` 无令牌桶（只影响自建 worker 的限流，不影响桌面端） |
| 5 | `8b9dc8b` | 服务端依赖/合集改用官方 API | 未逐行对比（我们 `worker/steam_api_worker.js` 10931 字节 vs 上游重写；功能面已由 `required_items`/`child_items` 覆盖） |
| 6 | `3008a30` | 详情接口默认不带图集 | 未采用：我们 `WorkshopItemDetail.Previews` 只有 URL（无二进制），收益低 |
| 7 | `c9e8a25` | 合集解析不显示无法下载的主物品 | 未覆盖（合集解析展示细节；非核心） |
| 8 | `361dc9a` | 工坊详情显示依赖物品 | 已对齐（`workshop_browser.go` 的 `RequiredItems`） |
| 9 | `ce2b268` | 工坊 ID 直达按钮 | 已对齐（`frontend/src/js/features/workshop/id-jump.js`） |
| 10 | `b635ea3` | 工坊解析历史 | 已对齐（`internal/app/workshop_history.go` + `downloads/workshop-history.js`） |
| 11 | `d367670` | GitHub 加速源列表更新 + 下拉自动关闭 | **部分未覆盖**：我们的 `MirrorList` 仍是旧 4 条（含上游已移除的 `gh.llkk.cc`） |
| 12 | `e9069c8` | 更新弹框资源预热 | 未覆盖（更新弹框首次延迟；体验问题） |
| 13 | `932d5bc` | 文档更新 | 无需 |
| 14 | `8c171c6` | addonlist 写入编码问题（288 行重写） | **我们更强**：`writeAddonList` 保留 GBK/ANSI/UTF-16 与 BOM + 原子写 + 黄金回归测试 |
| 15 | `1b74507` | 全新的加载顺序编辑器 | 部分覆盖：我们有加载顺序弹窗 + 约束/策略/分层/预览（`load-order-policy.js`、`addon_list_load_order.go`），但**没有失效/新增条目标记** |
| 16 | `a50cf4f` | 标记 addonlist 中失效/新增文件 | **未覆盖**：我们刻意"保存时保留未出现的条目"；我们有体检里的"清理失效条目" |
| 17 | `ba20411` | Mod 快照（文件名快照 + 完整备份 + 恢复预览） | **未覆盖**（最大缺口，见 §5.2） |
| 18 | `074fe3d` | 文档更新 | 无需 |
| 19 | `da4a71b` | parser：界面资源角色误判 + 关键词优先级 | **已覆盖且更严**：我们的 `isCharacterAssetPath`（`archive_index.go`）只认角色资源根 + 语音目录，且武器/角色规则来自 `ruletable`（有序切片，无 Go map 随机序） |
| 20 | `be3edfc` | CI：GitHub Actions 依赖版本 | 未核对（发布工作流本地验证过：`scripts/verify-release.ps1`） |
| 21 | `1d581aa` | IP 直连服务器 | 已对齐（`direct-connect.js` + 接线守卫测试） |
| 22 | `a17e39a` | 直连弹窗自动填入剪贴板地址 | **未覆盖**（小功能，见 §5.3） |
| 23 | `5ce7ef5` | 工坊 DNS 设置 | **未覆盖**（1316 行含测试，见 §5.1） |
| 24 | `7b0818c` | 保存并恢复 VPK 列表排序设置 | **本轮已实现**（下一条） |
| 25 | （其余 docs/CI 提交） | 文档、CI | 无需 |

### 2.1 本轮已实现：排序设置持久化（对齐 `7b0818c`）

- 后端：`internal/app/app.go`（`App.sortType/sortOrder` + `ConfigFile.SortType/SortOrder`）、
  `internal/app/config.go`（`normalizeFileSort`，非法/缺失一律回落"优先级 + 顺序"）。
- 前端：`core/config.js`（默认值 + 白名单归一化）、`features/state.js`（启动读配置、`applyConfigToAppState` 同步）、
  `features/file-list/sorting.js`（`saveSortPreference`，名称/日期/大小、优先级、模型复杂度三类入口都写盘）、
  `filters.js` 的"重置筛选"也同步写盘。
- 测试：`internal/app/config_test.go`（`TestFileSortPreferenceRoundTrip`、`TestNormalizeFileSort`）、
  `frontend/src/js/features/file-list/priority-sort.test.mjs`（新增"排序设置会持久化并在启动时恢复"）。

## 3. 其它管理器逐个分析

### 3.1 FireAxe（177★，C#，Apache-2.0）——已经逐项对齐过，上游未前进

`ktxiaok/FireAxe` HEAD 仍是 `f8aa1cf`（v0.7.3），与我们 `docs/development/fireaxe-parity.md` 的对照基线**同一个 commit**。
结论：这一轮的增量是 **0**，我们那份对照表（30+ 行概念映射 + 我们更强者清单）仍然有效，不需要重做。
值得保留的判断：FireAxe 的强项是"对象解释层"（每个失败都给人话 + 下一步）与符号链接式 push 工作流；
我们的**直接管理真实文件**、**编码保真 addonlist**、**冲突严重度**、**工坊链路**都超出它。

### 3.2 xavier-cai/L4D2ModManager（66★，C#，MIT，2020 年后停更）——"用户自定义分类规则"最值得学

技术栈：WPF + `Facepunch.Steamworks`（**直接对接 Steam 客户端**）+ fork 的 `SharpVPK`。

值得学的三点：

1. **分类规则是用户可编辑的正则 + 布尔表达式**：
   `Fork/.../L4D2ModManager/L4D2Type.cs` 里 `CustomContent.CustomRegex` 把"正则 → 引用名"存成 `regex.ini`，
   分类规则用 `<Maps>`、`<Scripts>` 这类引用名组合成表达式（`RegexClassifierHandle.PeekOne` 是个手写表达式求值器），
   配套 `WindowClassify.xaml` 规则编辑器。默认规则就是 `models/survivors/survivor_(.+)\.mdl`、`models/infected/(.+)\.mdl` 这类。
   → 我们的 `internal/ruletable/rules.json` 是**内置**唯一事实源，用户只能给单个 Mod 打自定义标签，不能写规则。
2. **`Category.SingletonResource`（互斥资源类别）**：标记"同类只能开一个"的分类（皮肤/角色替换），
   开启冲突项时走 `Manager.ChangeModState` 的碰撞处理（自动关旧的或提示），这是"开 Mod 时就拦住"的体验。
   → 我们只有事后的冲突分析/优先级，没有"开启瞬间的互斥提示"。
3. **按类别判定冲突**：同类（Category 树）内多个启用即冲突，粒度比"文件路径重叠"更粗也更符合直觉。
   → 我们的冲突判定更精确（文件级 + 严重度 + 优先级），但**缺少"同类互斥"这个粗粒度视角**，两者可以并存。

比我们差的：停更 5 年、依赖 Steam 客户端登录、无编码保真、无备份/监控、无体检、无策略组/分层。

### 3.3 pukmajster/funky（54★，TypeScript/Electron，MIT）——UX 与"随机/播放列表"值得学

`src/renderer/src/features/library/conflicts/conflicts.ts` 的冲突判定非常朴素：
把启用中的 Mod 按"是否有任意相同文件路径"分组（`arraysShareValues`），只排除**根目录文件**和 4 个
`scripts/vscripts/*_addon.nut`（`vscriptNonConflictingFiles`），没有严重度、没有优先级、没有原版白名单。

值得学的：

1. **Playlists（播放列表）+ Shuffles（随机）**：每个 profile 可以挂多个 shuffle，每个 shuffle 绑一个分类，
   启动时**从每个分类里随机挑一个**（`utils/shuffles.ts` + `api/api.ts` 的 "Choose a random mod from each shuffled ..."）。
   我们的"Mod 轮换"只有 人物/武器 两个布尔开关（`internal/app/rotation.go`），粒度和可玩性都不如它。
2. **manifest 缓存**：把每个 Mod 的完整文件列表、分类、工坊信息落成一个 manifest（`src/main/manifest.ts`），
   之后只增量扫描；我们的 `vpk_scan_cache.json` 是同一思路（真机 2960 个 Mod 冷启动 0.6–2s）。
3. **批量退订**：走 Steam Web API（需要用户 API key），我们目前没有退订能力。

比我们差的：冲突判定（见上）、无 addonlist 编码/备份/监控、无体检、无分层/策略组、必须重启游戏、
工坊只能靠 Steam 客户端下载。

### 3.4 fdklgbh/L4D2-Mod-Manager（23★，Python/Qt，GPL-3.0，中文）

用"把 VPK 移到禁用在文件夹"实现启停（README 明确写"需要配置一个禁用文件夹"），
有"一键切换配置"（≈我们的启用方案）。README 自己列的**已知问题**很有价值：
多分片地图（某个 part 没有 `maps/` 或 `missions/`，addoninfo 字段可能为 0）会被分错类；材质需要人工确认。

→ 这条对我们是个**对照检查项**：我们用路径证据 + 任务文件 + 标题证据，判定口径比"只看 maps/missions + addoninfo 字段"更稳；
但"资产型 part 包"确实会落到「其他」（真机 1031 个「其他」）。想用 addoninfo 字段补救的收益，
已经用真机量化过（§5.4）：只覆盖 1.2% 的包，**不值得做**。

### 3.5 auto-created-name/L4d2_Mod_Manager_Tool（18★，C#/.NET）

扫描 VPK 入库（本地数据库）→ 搜索（名称/作者/VPKID）→ 标签过滤（需从创意工坊抓标签）→
右键"打开位置/用 GCFScape 打开" → 按 addonlist.txt 启停。依赖外部 `no_vtf` 做缩略图。

→ 我们已经有同样的能力（预览图内建、无外部依赖），并且多了冲突/体检/工具箱。
**唯一可借鉴**：它把"模组库"存成数据库（我们直接扫盘 + JSON 缓存），在超大库上数据库查询更快——
真机 2960 个 Mod 我们的扫描是 0.6–2s，暂不需要。

### 3.6 Anokianx/L4D2-Danyria-Integration-Tool（13★，Python）

定位是"整合工具箱"：Mod 管理 + **外置 HUD 覆盖层**（速度/敌人血量/评分）+ 武器数值编辑 + 本地插件安装。
外置 HUD 属于**实时读游戏内存/日志**的独立方向，和我们的"文件与配置管理"没有重叠，
参考价值在于"工具箱可以很宽"这一产品思路（我们已经在做：服务器面板/VPK 工具/喷漆）。

### 3.7 kublay-tayro/Mods4Versus（Tauri v2 + Rust）

"把多个工坊 VPK 合并成一个 `pak01_dir.vpk`"：`extract_vpk` 逐个解包到临时目录（后覆盖先）→
`pack_vpk_v1` 打包 → 放到 `mods/` → 注入 `gameinfo.txt` 搜索路径。

→ 这是**另一条路线**：绕过 addonlist 与逐 Mod 开关，换取"对抗模式少冲突"。代价是丢掉单 Mod 启停、
冲突不可见、排序不可控。我们已经有"VPK 打包"工具（`vpk_pack.go`），可以做"导出整合包"，
**但不建议把合并当成主路**（会破坏我们的优先级/冲突模型）。保留为 P2 工具箱选项。

### 3.8 kinten108101/steam-vpk（2★，GJS/Flatpak）与 TheCraZyDuDee/L4D2-Mod-Manager（8★，Python）

- `steam-vpk` 的架构是"UI（GJS/GTK）+ 独立 daemon（addon-box）"的客户端/服务端拆分，
  作者自己标注"under development - not ready for practical usage"。→ 我们的单进程 Wails 架构更省事，
  但它的"daemon 常驻、UI 可换"值得记一笔：**如果我们将来要做 CLI/第三方集成，应当复用同一套后端**——
  我们已经有 `HandleCommandLine` 的只读 CLI 通道，方向一致。
- `TheCraZyDuDee` 那个 8★ 项目只是简单的开关脚本（README 级），无参考价值。

### 3.9 craftablescience/VPKEdit（760★，C++20，通用 Source 工具）

不是 L4D2 管理器，但它的**"不解包直接预览/编辑"**能力是整个生态里最强的：
预览音频/文本（任意编码）/KeyValues（带高亮）/图片/Source 1 贴图/模型/DMX；
可写 VPK 直接改文本、加删文件；CLI + GUI 双形态；i18n 走 POEditor（20+ 语言）。

→ 我们的 VPK 工具目前是"解包/打包/完整性/预览图"，**没有容器内预览**。
对 Mod 玩家最有用的是"打开一个 Mod 直接看 addoninfo / 模型路径 / 材质引用"——这正好能把我们的
"标签依据"和"结构摘要"做得更直观。列为 P1 备选。

### 3.10 bahyqn/l4d2mm（0★，Go，Linux）

2026-09 才起步的 Go + native GUI 项目（mods.json + sqlite + vpk 解析），目标是"Linux 社区缺一个像样的管理器"。
它的 README 直接把某个 VPK 的 `addoninfo.txt` 内容、文件名、BaseName 打印出来——说明**社区最痛的点仍然是
"看不懂一个 VPK 里装了什么"**，和我们 §3.9 的判断一致。

## 4. 本项目的优势（每条都有源码 / 测试 / 真机证据）

| 能力 | 我们的实现 | 与其它管理器的对比 |
| --- | --- | --- |
| 冲突判定 | `internal/app/conflict.go`：严重度分级 + 优先级感知（可判定胜负的算"覆盖"）+ 原版文件白名单（`internal/stockfiles/assets/engine-glue.txt`，含 4 个共用 vscript）+ 用户忽略清单 | Funky 只按"有相同文件"分组并硬编码排除 4 个 vscript；FireAxe 同优先级即冲突；xavier-cai 按类别粗判 |
| addonlist 生命周期 | 编码/BOM/换行保真 + 原子写 + 受保护版本 + 历史备份 + 运行时监控自动恢复 + 只读闸门（`LYTVPK_READONLY_LIBRARY`） | 上游只做 UTF-8 重写（后补编码修复）；fdklgbh 靠移动文件；其它项目直接覆写 |
| 标签识别 | 10 条通道（本体锚点/槽位/基名 token/`.mdl` 材质表/标题/套件继承/…）+ 证据分级 + `--check-tag-regression` allowlist 护栏 + `--validate-tag-rules` | 其它项目：关键词表（xavier-cai 正则、FireAxe 规则文件）或直接抄 Steam 工坊标签 |
| 排序与优先级 | 统一优先级模型（显式分层 + 策略组权重 + 祖先链累加）、加载顺序约束求解、预览、失效体检 | FireAxe 有组权重（我们已对齐）；Funky/其它只有"顺序拖拽" |
| 体检与报告 | 条目缺失/重复/未记录/深度扫描 + Markdown 报告导出 + 一键修复 | 其它项目没有等价能力 |
| 工坊链路 | 下载队列/断点续传/批量转移/meta 富化/合集跟随/翻译/优选 IP | 上游同源（我们已对齐并加强）；Funky 依赖 Steam 客户端 |
| 工具箱 | VPK 解包/打包/完整性/喷漆/autoexec/存档管理/服务器面板 | 其它项目最多有一个"打开 GCFScape"按钮 |
| 安全性 | 路径守卫、去重、原子写、无残留后台进程（本轮真机复验：关闭后 0 个 `LytVPK*` 进程） | 没有可比对象 |
| 检索体验 | 6 面板统一语法（`-排除` / `re:` / `tag:`）+ 结果光标 + 命中高亮 + 语法说明书 | 其它项目只有模糊搜索 |

## 5. 可落地规划

### 5.1 P0：工坊 DNS 设置（对齐上游 `5ce7ef5`）

**问题**：工坊列表/详情/链接解析失败时，我们只能靠"优选 IP"或系统代理；无法只给工坊 API 换 DNS。
**上游实现**：`internal/network/workshop_dns.go`（244 行）+ `internal/app/workshop_network.go`（133 行）+
配置四项（腾讯/阿里/自定义/系统）+ 设置页卡片，配套 598 行测试。
**我们的落点**（建议）：

- 新增 `internal/network/workshop_dns.go`：`Resolver`（自定义 DNS → `net.Resolver{PreferGo: true, Dial: ...}`）。
- 新增 `internal/app/workshop_network.go`：`newWorkshopDialer()` 统一出口，替换
  `download_worker.go` / `workshop_download.go` / `workshop_transfer.go` / `workshop_browser.go` 里 4 处各自 new 的 dialer。
- 配置：`ConfigFile.WorkshopDNS`（`mode` + `customAddress`）+ 迁移版本 +1。
- 设置页：网络设置卡片"工坊 DNS"（腾讯 119.29.29.29 / 阿里 223.5.5.5 / 自定义 / 系统）。
**验收**：`--validate` 配置读写；沙箱里切换 DNS 后能正常拉列表/详情；失败时给出人话错误。
**风险**：中（网络出口统一了才安全）。**工作量**：约 0.5–1 天。

### 5.2 P0：Mod 快照（对齐上游 `ba20411`）

**问题**：我们只有 addonlist 级方案（启用状态 + 顺序）和 addonlist 备份，**没有文件内容级备份**。
用户在"批量整理/更新/删除 Mod 前"想要一个可整体回滚的快照。
**上游实现**：`internal/app/snapshot.go`（1085 行）+ `snapshot_restore.go`（1285 行）+ 前端 `snapshot-tool.js`（1166 行），
两种类型（文件名快照 / 完整备份 zip）、恢复预览（启用/禁用/新增/覆盖/清理/缺失/跳过/列表）、
执行前门禁（游戏运行中 / 问题扫描中 / 快照不完整 / 目录已变化）。
**我们的落点**（建议，**先做最小闭环**）：

1. 快照目录 `%APPDATA%\LytVPK\snapshots\<时间戳-名称>\`，`snapshot.json`（模式 + addonlist 原始字节 + 文件清单 + 哈希）。
2. `mode=names`：只记清单；`mode=full`：把根目录 VPK/同名图片/`.meta` 复制进快照目录（或 zip）。
3. 恢复：先生成**恢复计划**（复用我们已有的移动/备份原语：`MoveVpkFiles*`、`writeAddonList`），
   计划里逐项标注 启用/禁用/新增/覆盖/缺失/跳过，确认后执行，执行前自动做一份 addonlist 备份。
4. UI：工具箱新增"Mod 快照"卡片（列表 + 新建 + 恢复预览 + 删除到回收站）。
**验收**：单测（计划计算纯函数）+ 沙箱真机（镜像库）走一遍"建快照 → 删/移几个 Mod → 恢复 → 目录与 addonlist 回到快照状态"。
**风险**：高（涉及文件移动）。**工作量**：2–4 天（按最小闭环估）。

### 5.3 P1：小项（可以直接做）

| 项 | 落点 | 工作量 |
| --- | --- | --- |
| 直连弹窗自动填入剪贴板地址（`a17e39a`） | `frontend/src/js/features/servers/direct-connect.js` + `address.js` 的 `parseClipboardAddress` | 1–2 小时 |
| GitHub 加速源列表更新（`d367670`） | `internal/app/update.go` 的 `MirrorList`（含"仅下载型镜像不能用于版本检测"的注释口径） | 30 分钟 |
| 更新弹框资源预热（`e9069c8`） | `internal/app/update.go` + `features/update/updates.js` | 2–3 小时 |
| 加载顺序编辑器：失效/新增标记（`a50cf4f`） | `AddonListLoadOrderEntry` 加 `fileExists` / `isNew`；`load-order-policy.js` 渲染角标 | 半天 |

### 5.4 P2：真机数据证明"不值得做"的项

**addoninfo `addonContent_*` 兜底分类**：真机跑 `filepath.WalkDir` 扫 `left4dead2\addons`（4849 个 VPK），
主类型分布 `其他 2718 / 武器 1191 / 人物 657 / 地图 61`；其中**只有 56 个（1.2%）在 addoninfo 里声明了 `addonContent_*`**，
声明 `Map`/`Campaign=1` 的只有 2 个。→ 结论：**收益远小于维护成本，不做**；记录在此备查。

## 6. 决策记录（2026-10-07：用户确认「8 件事都按建议走」）

下表就是当初的 8 个待决策项；**已全部按"建议"落地**，保留原文便于回溯决策过程。

| # | 问题 | 决定 | 落地版本 |
| --- | --- | --- | --- |
| 1 | Mod 快照（文件级备份）要不要做 | A 最小闭环（names + full + 恢复预览） | 2.7.1-community.66 |
| 2 | 工坊 DNS 要不要回移 | B 系统 / 自定义两档 | 2.7.1-community.66 |
| 3 | 加载顺序保存时是否清理失效条目 | B 对齐上游（清理前先备份） | 2.7.1-community.66 |
| 4 | 用户自定义分类规则 | 导入规则文件 + 预览命中（不做可视化表达式编辑器） | 2.7.1-community.66 |
| 5 | 开启 Mod 时的"同类互斥"提示 | A 做（三选一） | 2.7.1-community.66 |
| 6 | 按分类随机 / 播放列表 | A 把轮换扩展成"按分类随机" | 2.7.1-community.66 |
| 7 | VPK 内联预览 | A 只读预览（文本/KeyValues/贴图） | 2.7.1-community.66 |
| 8 | VPK 合并成整合包 | A 作为"导出整合包"工具 | 2.7.1-community.66 |

### 仍然待定的细节（不影响已交付功能）

| # | 问题 | 选项 | 我们的建议 |
| --- | --- | --- | --- |
| 1 | **Mod 快照**（文件级备份）要不要做？ | A. 做最小闭环（names + full + 恢复预览）<br>B. 只做"文件名快照"（不动文件内容）<br>C. 先不做，继续用现有 addonlist 备份 | **A**：这是和其它管理器差距最大的一块，且你明确关心备份 |
| 2 | **工坊 DNS 设置**要不要回移？ | A. 完整回移（含测试）<br>B. 只做"自定义 DNS + 系统 DNS"两档<br>C. 不做（继续用优选 IP / 系统代理） | **B**（先小后大；你本机有代理，收益取决于是否遇到解析问题） |
| 3 | **加载顺序保存时是否清理失效条目**？ | A. 保持现状（保留，体检里手动清理）<br>B. 对齐上游（保存时自动删掉 addonlist 里没有文件/在 disabled 的条目，并先备份） | **B**（更省心；但这是行为变更，需要你点头） |
| 4 | **用户自定义分类规则**（正则 + 表达式）要不要做？ | A. 不做（继续内置规则 + 单 Mod 自定义标签）<br>B. 做"高级规则"页（导入/导出 + 校验 + 预览命中） | **A 或 B 都可以**；如果做，建议只做"导入规则文件 + 预览命中"，不做可视化表达式编辑器 |
| 5 | **开启 Mod 时的"同类互斥"提示**要不要做？ | A. 做（开启第二个同类 Mod 时提示"关掉旧的 / 继续共存"）<br>B. 不做（现有冲突分析 + 优先级已够） | **A**（低成本、高感知） |
| 6 | **按分类随机 / 播放列表**要不要扩？ | A. 把"Mod 轮换"扩展成"按分类随机"<br>B. 不动 | **A**（你的库里角色/皮肤 Mod 占比很高） |
| 7 | **VPK 内联预览**（不解包直接看文本/贴图/模型）要不要做？ | A. 做只读预览（文本/KeyValues/贴图）<br>B. 做到可编辑（对齐 VPKEdit）<br>C. 不做 | **A**（只读预览就能覆盖"这包到底是什么"的高频需求） |
| 8 | **VPK 合并成整合包**要不要加进工具箱？ | A. 作为"导出整合包"工具（明确警告会绕过 addonlist）<br>B. 不做 | **A 低优先**（对抗模式用户可能用得上） |

## 7. 附录：本轮真机探针与命令

```powershell
# 上游缺口（25 个提交）
git fetch upstream --tags; git log --date=short --pretty='%h %ad %s' HEAD..upstream/master

# 受管 Mod 的数量与分类分布（导出清单后统计）
.\build\bin\LytVPK-Community-Fork.exe --export-grouping-catalog '.tmp-cua\refresh-check\grouping-catalog-final.json'
# → primaryTag: 其他 1031 / 武器 1030 / 人物 628 / 地图 58 + 用户自定义一级标签 213
# → location: root 1707 / workshop 945 / disabled 308

# addonContent 声明覆盖率（临时探针，已删除；数据见 §5.4）
# → walk 4849 个 VPK，其他 2718，声明 addonContent_* 仅 56 个

# 排序持久化（本轮实现）
go test ./internal/app/ -run 'FileSort|NormalizeFileSort' -count=1
cd frontend; node --test src/js/features/file-list/priority-sort.test.mjs
```

> 外部仓库源码只 clone 到 `.tmp-cua\survey\`（临时目录），不进仓库、不参与构建。
> 所有引用其它项目结论的文件路径，都是 clone 后的相对路径，便于复核。
