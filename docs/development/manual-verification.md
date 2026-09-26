# 手工验证清单（本地维护者用）

自动化检查（`go test ./...`、`node --test`、`npm run build`、`wails build`、
`npm run docs:build`、`go vet ./...`）覆盖不到真实点击流程。
本文列出需要在运行中的应用里逐项确认的功能，按“操作 → 预期”编写。
证据分级：**已自动验证**（本文第一部分与各节"已经自动化覆盖的部分"）、
**只能人工验证**（各节"待人工验证"）、**仍未验证**（当前没有此类项）。

## 最近一次全量验证基线（2026-09-26，本机）

复核时先跑这一组；全绿再看下面的"只能人工验证"清单。这份基线与
`docs/development/fireaxe-parity.md` §5 各轮记录、`CHANGELOG.md` 的"验证"小节对应。

| # | 命令 | 期望 | 最近结果 |
| --- | --- | --- | --- |
| 1 | `go test ./... -count=1` | 全部 ok（含仓库一致性审计） | ok（`internal/app` 等 6 个包） |
| 2 | `go vet ./...` | 无输出 | 无输出 |
| 3 | `node --test`（在 `frontend/`） | 全通过 | **338 项 / 0 失败** |
| 4 | `npm run build`（在 `frontend/`） | 产物正常（仅有既有的 chunk 体积警告） | 通过 |
| 5 | `wails build` | 产出 `build/bin/LytVPK-Community-Fork.exe` | 通过（18,466,816 字节） |
| 6 | `npm run docs:build`（在 `docs/`） | VitePress 构建完成 | 7.70s |

另外两项"不靠命令"的复核点：

- **仓库级一致性**：`internal/app/repo_consistency_test.go` 会检查"对照表引用的文件都存在""体检类型
  在 Go / 前端标签 / 用户文档三层一致""文档里的验证命令与工程脚本一致"，这两份审计都做过变异验证
  （故意制造回归会失败）。
- **交付产物卫生**：`build/bin/LytVPK-Community-Fork.exe` 里不应出现 `cua-bridge`（临时调试桥残留）。

> **读历史轮次时注意**：下面各轮记录里的"仍未验证：`IValidity` 的增量级联传播"是**当时**的状态。
> 该项已在 `fireaxe-parity.md` §5.13 用真实库实测闭环（定位到根因是索引缓存容量、修复后热重算
> 558ms → 177ms，并判定不需要再做增量传播）——**当前没有任何"计划内但未验证"的项**。

## 已经自动化、无需手工重复的部分

下面这些语义已经有测试守着，手工验证时不必逐条重跑，只在其失败时才需要人看：

- `addonlist.txt` 与磁盘一致性、依赖声明/启用、工坊 `.meta` 检查、方案与策略组的读写与写盘规则（`go test ./...`）
- 冲突分析的分层判定、忽略清单匹配、策略组联动（同上）
- 统一优先级模型：未分层时的黄金回归、有效分层与组权重 `min`、显式"按分层应用"的稳定排序与
  GBK 编码保真、方案快照携带分层（`go test ./...` → `internal/app/priority_test.go`）
- 优先级文案层：`优先级 #顺序（分层 T）`、有效分层解释、分层输入解析、设置页依赖透传
  （`node --test` → `priority-label.test.mjs`、`settings-deps.test.mjs`）
- 单 Mod 冲突忽略文件：只让该 Mod 退出对应路径的判定、与全局清单/内置规则取并集、
  “被自身规则跳过”的标注（`go test ./...` → `mod_ignore_test.go`）
- 详情页忽略清单的 addonlist 键推导（`node --test` → `detail-ignore-key.test.mjs`）
- 本地记录备份轮转：最短间隔、内容相同跳过、数量上限、溢出走可注入的删除回调
  （`go test ./...` → `local_store_backup_test.go`；真实回收站行为属于人工项）
- 自动复检：首次按需重算、缓存有效时不重复扫描、写入 `addonlist.txt` / 移动 / 重命名 / 删除后失效、
  仅保存分层不触发失效（`go test ./...` → `conflict_recheck_test.go`）
- 修复建议：同层冲突给出 `分层 - 1` 建议、覆盖关系给出可逆向的分层建议、
  生成建议不改写 `addonlist.txt`（同上）
- 冲突角标文案与键推导（`node --test` → `conflict-badge.test.mjs`）
- 原版文件白名单：内置批次生效、增量批次可增可删、目录缺失/批次损坏时降级且仍能检测冲突、
  从夹具 `pak01_dir.vpk` 按类生成批次且不改动游戏文件
  （`go test ./...` → `stock_whitelist_test.go`、`internal/stockfiles/stockfiles_test.go`）
- 自动重下：默认关闭、失败且开启时只自动重试一次、用户取消不重试、
  全局默认值持久化、单任务开关（`go test ./...` → `auto_redownload_test.go`，
  通过注入的下载启动器覆盖，不依赖真实网络）
- 工坊合集实体化：成员差异计算、保存/重复保存校验、跟随节点刷新（新增+下架）、
  批量检查、只下载缺失成员、删除记录不删文件
  （`go test ./...` → `workshop_collections_test.go`，工坊接口用假 fetcher）
- 合集文案层（`node --test` → `workshop-collection-format.test.mjs`）
- 策略组树形层级：无上级时等价扁平、嵌套顺序、坏数据（上级被删/成环）降级、
  自环/成环/超深校验、持久化与清除（`go test ./...` → `mod_group_tree_test.go`、
  `frontend/src/js/features/settings/strategy-group-tree.test.mjs`）
- i18n 文案层：嵌套 key 取值、默认语言回退、缺失 key 返回 key、参数插值、
  语言切换与目录枚举（`node --test` → `frontend/src/js/core/i18n.test.mjs`）
- 崩溃上报：panic → 报告（来源/原因/堆栈/版本/系统/日志尾部）、日志环形缓冲只保留最近 N 行、
  前端未处理异常走同一写入路径、报告数量上限与删除、非法文件名（含路径穿越）被拒绝
  （`go test ./...` → `crash_reporter_test.go`）
- 设置页文案与体检结果行构建、依赖提示文案、冲突忽略清单解析（`node --test`）
- 前端打包与绑定完整性（`npm run build`，会拦住“Go 新增了方法但没重新生成绑定”这类问题）
- 文档站构建（`cd docs && npm ci && npm run docs:build`）

手工验证的重点因此是：**真实点击流程、跨页面状态、真实游戏/工坊数据**。

## 准备

1. 构建：`wails build`，产物 `build/bin/LytVPK-Community-Fork.exe`。
2. 准备两个测试用 VPK（内容随意，但要有同名文件以制造覆盖），放进 `addons` 目录。
3. 记录当前 `addonlist.txt`（备份或截图），便于恢复。

## 1. 冲突分析与忽略清单

- [ ] 打开“Mod 冲突检测”，在范围弹窗勾选“按加载顺序判定胜负”，应用并分析。
  - 预期：能在 `addonlist.txt` 中判定先后的重叠进入“已判定覆盖关系”，卡片上有“生效 / 被覆盖”徽标；无法判定胜负的重叠仍在冲突列表。
- [ ] 在“设置 → 游戏配置 → Mod 冲突分析”打开同名开关，重启应用后回到范围弹窗。
  - 预期：开关仍是打开状态（配置已持久化）。
- [ ] 在忽略清单里填一行 `materials/`，保存后重新体检/分析。
  - 预期：该目录下的重叠不再出现；重启后清单仍在。

## 2. 启用方案

### 单 Mod 冲突忽略文件（Task 2）

- [ ] 打开一个 Mod 的详情，在“该 Mod 的冲突忽略文件”里填入两条路径（一条文件、一条目录），保存。
  - 预期：提示已保存；关闭详情再打开，内容仍在。
- [ ] 重新做一次冲突检测。
  - 预期：该 Mod 不再出现在被忽略路径的冲突里；**其它 Mod 之间的同名路径冲突仍然存在**；
    结果区出现“因 Mod 自身的忽略规则跳过”并列出该路径与该 Mod。
- [ ] 清空该 Mod 的忽略清单并保存，再检测一次。
  - 预期：该 Mod 重新参与判定，忽略提示消失。

- [ ] 在“设置 → 游戏配置 → 启用方案”输入名称并保存当前为方案。
  - 预期：方案出现在列表中，条目数与当前 `addonlist.txt` 一致。
- [ ] 手动改乱开关与顺序（或删掉一个条目），再点该方案的“应用”。
  - 预期：开关与顺序恢复成保存时的状态；方案中没有、当时不存在的 Mod 被补回；
    方案保存后新增的 Mod 保留原开关并排在最后；页面提示“已存在/补回/保留”的数量。
- [ ] 到“历史备份”里查看。
  - 预期：出现一条类型为“应用启用方案前”的备份，可恢复。
- [ ] 导出方案到文件，再“导入方案”。
  - 预期：导入后是新的条目、名称与描述保持。

## 3. 策略组与自动联动

- [ ] 在 Mod 管理页勾选 3 个 Mod，打开「分组 → 策略组管理…」窗口，填名称、选“互斥单选”并建组。
  - 预期：按钮显示当前选中数量；建组成功后列表出现该组。
- [ ] 点“随机单选”。
  - 预期：组内恰好一个开启，其余关闭；提示“已应用策略组”。
- [ ] 点“按策略应用”。
  - 预期：保留当前已启用的那个（若都不在启用状态则保留第一个）。
- [ ] 打开该组的“自动联动”，回到 Mod 管理页开启组内另一个成员。
  - 预期：原先开启的成员被自动关闭；出现“已按策略组自动联动另外 N 个 Mod”提示；列表状态刷新后正确。
- [ ] 关闭“自动联动”，再开关组内成员。
  - 预期：其它成员不再被改动。

## 4. Mod 体检

- [ ] 点“开始体检”。
  - 预期：列出条目缺少文件 / 只剩 disabled 副本 / 未写入开关记录 / 重复条目等结果，严重问题排在最前。
- [ ] 勾选“深度扫描”再体检一次。
  - 预期：损坏的 VPK 被标为“VPK 无法解析”；耗时可接受。
- [ ] 手工在 `addonlist.txt` 里复制一行制造重复，然后点“清理重复条目”。
  - 预期：提示已清理的数量；到“历史备份”能看到类型为“体检修复前”的备份。
- [ ] 手工写一个不存在的条目，再点“清理失效条目”。
  - 预期：确认框出现；确认后失效条目被删除，`disabled` 目录中仍有副本的条目会被保留。

## 5. Mod 依赖

- [ ] 在 Mod 管理页勾选两个 Mod，到“设置 → 游戏配置 → Mod 依赖”选择主 Mod 并保存。
  - 预期：列表出现“A 依赖 B”；重启后仍然存在。
- [ ] 手动把依赖项在游戏内关闭，然后跑一次体检。
  - 预期：出现“依赖未开启”警告，主 Mod 名称正确；点“启用缺失依赖”后依赖被开启，再体检不再提示。
- [ ] 把依赖文件移走（或改名），再跑一次体检。
  - 预期：出现“依赖缺失”警告；点“启用依赖”时该项会被跳过并提示“文件缺失”。
- [ ] 关闭主 Mod 自身，再体检。
  - 预期：不再提示它的依赖问题。

## 6. 回归项

- [ ] 没有任何“自动联动”策略组时，在 Mod 管理页单独开关一个 Mod。
  - 预期：行为与改动前一致（写入 `addonlist.txt`、无额外通知、无额外刷新卡顿）。
- [ ] 关闭应用再打开，检查各面板开关状态是否与关闭前一致。

## 7. 统一优先级模型（分层）

### 已经自动化覆盖的部分（不必手工重跑）

- 未设置分层时冲突/覆盖结果与引入分层前逐字节一致（黄金快照
  `internal/app/testdata/conflict-priority-golden.json`）。
- 有效分层 = `min(自身分层或顺序号, 组权重)`；多个组取最小值；组权重为 `nil` 时不参与。
- 保存/清除分层不写 `addonlist.txt`；只有“按分层应用”才会重排，且同层稳定、GBK/BOM 保真。
- 方案快照携带并恢复分层；导入时丢弃空键并按键去重。

### 待人工验证

- [ ] 在“编辑加载顺序”里给两个互相覆盖的 Mod 分别设置不同分层，点“按分层应用”。
  - 预期：`addonlist.txt` 中分层更小的条目排在前面，两个 Mod 的开关状态没有变化。
- [ ] 上一步之后，把其中一个分层改成与另一个相同，再点“按分层应用”。
  - 预期：两者相对顺序保持不变（同层稳定），冲突检测把它们视为“同层冲突”。
- [ ] 到「分组 → 策略组管理…」窗口给某个组填权重 `-1` 保存，回到加载顺序弹窗点“按分层应用”。
  - 预期：组内成员整体前移；未设置分层的其它 Mod 仍按原顺序号排在其后。
- [ ] 在策略组里清空权重并保存，再点“按分层应用”。
  - 预期：组内成员回到按顺序号排序，与设置权重前一致。
- [ ] 真实游戏内覆盖方向确认（本项目唯一未实测的语义假设，见
  `docs/superpowers/specs/2026-09-21-priority-aware-conflict-design.md` 第 8 节）：
  做两个含同名文件的小 VPK，交换 `addonlist.txt` 顺序，观察游戏内实际生效的是哪一个。
  - 预期：确认"越靠后覆盖前面的资源"这一方向；若结论相反，只需修改
    `internal/app/conflict.go` 的 `conflictOrderWins` 一处。

## 8. 自动复检与修复建议（Task 4）

### 已经自动化覆盖的部分

- 首次按需重算、缓存有效时不重复扫描、写入 `addonlist.txt` / 重命名 / 删除后失效、
  仅保存分层不触发失效（`internal/app/conflict_recheck_test.go`）。
- 同层冲突给出 `分层 - 1` 建议、覆盖关系给出可逆向的分层建议、生成建议不改写 `addonlist.txt`（同上）。
- 角标文案与 addonlist 键推导（`frontend/src/js/features/conflicts/conflict-badge.test.mjs`）。

### 待人工验证

- [ ] 在 Mod 管理页开关一个 Mod，观察列表角标是否自动刷新（不打开冲突检测）。
  - 预期：该 Mod 及其对手的 `N 处冲突 / N 处覆盖` 角标在约半秒内更新。
- [ ] 打开冲突检测，点一条建议里的“设置分层 N”。
  - 预期：提示已保存分层；`addonlist.txt` **没有**被改动；
    到“编辑加载顺序”点“按分层应用”后顺序才变化。

## 9. 游戏原版文件白名单（Task 5）

### 已经自动化覆盖的部分

- 内置引擎胶水批次会抑制 `sound/sound.cache`、原版 `*_addon.nut` 等路径的重叠，
  同时保留其它新增重叠（`internal/app/stock_whitelist_test.go`）。
- 用户增量批次可新增、可删除；目录缺失或批次不可读时降级且不影响检测。
- 按类别生成批次：用夹具 `pak01_dir.vpk` 验证生成 `models.txt` / `materials.txt` / `_root.txt`，
  并断言游戏文件字节未变化。

### 待人工验证

- [ ] 在真实游戏目录上点“从原版 pak01 生成批次”，然后做一次冲突检测。
  - 预期：配置目录出现 `stock-files/*.txt`；原版路径（如 `models/...`）的重叠不再报冲突；
    新增资源路径的冲突仍照常报告。真实 `pak01_dir.vpk` 的解析耗时属于人工观察项。
- [ ] 删除其中一个类别文件（例如 `models.txt`）后点“刷新白名单状态”，再检测一次。
  - 预期：该类别重新参与冲突判定，状态行里的自定义批次数量随之减少。

## 10. 自动重下（Task 6）

### 已经自动化覆盖的部分

- 默认关闭；开启后失败只自动重试一次；用户主动取消不重试；第二次失败不再重试
  （`internal/app/auto_redownload_test.go`，下载启动器被替换为纯记录替身）。
- 全局默认值持久化到 `config.json`，单任务开关生效。

### 待人工验证

- [ ] 在“设置 → 工坊数据”打开自动重下，然后故意用不可达的直链下载一次。
  - 预期：任务先失败、随后自动重试一次；重试仍失败后停留在失败状态并显示错误；
    任务里能看到 `redownload_attempts` 为 1。
- [ ] 关闭自动重下后新建一个失败任务。
  - 预期：不会自动重试（行为与改动前一致）。

## 11. 工坊合集实体化（Task 7）

### 已经自动化覆盖的部分

- 保存合集时解析出全部可下载成员（含嵌套子合集）并记录；重复保存同一合集会报错。
- 跟随节点刷新能识别新增与下架的成员；批量检查覆盖全部记录。
- 下载只对"本地缺失"的成员入队；删除记录不会删除已下载的 VPK。
- 全部使用假 fetcher 与注入的下载启动器，不依赖真实工坊。

### 待人工验证

- [ ] 用一个真实工坊合集链接保存，检查成员数量与工坊页面一致。
  - 预期：列表显示 `N 个成员，其中 M 个未下载`（真实接口返回与页面一致）。
- [ ] 点“下载缺失成员”。
  - 预期：只对缺失成员入队；已下载的成员不会重复出现在下载队列。
- [ ] 删除其中一两个本地 VPK 后点“检查更新”，再点“下载缺失成员”。
  - 预期：检查结果提示“本地缺少 N 个”，随后这些成员被重新下载。

## 12. 策略组树形层级（Task 8）

### 已经自动化覆盖的部分

- 没有上级分组时树展开结果与扁平列表逐项一致（顺序、深度）。
- 嵌套、孤儿子树（上级被删）、环、超深的降级行为；自环 / 成环 / 超过 4 层的设置会被拒绝。
- 上层 API 的持久化与"移动回顶层"。

### 待人工验证

- [ ] 在「分组 → 策略组管理…」窗口里把两个组设置成父子关系。
  - 预期：下级分组缩进显示在上级下面；点击上级的“按策略应用”只影响该组成员，不级联到下级。
- [ ] 尝试把上级组设置到自己下级组的下面。
  - 预期：操作被拒绝并给出提示，列表层级保持不变（不会出现环或界面卡死）。
- [ ] 删除一个还有下级分组的上级。
  - 预期：下级分组自动回到顶层显示，成员与策略不受影响。

## 13. 文案层（Task 9）

### 已经自动化覆盖的部分

- key 取值、默认语言回退、缺失 key 返回 key 本身、参数插值、语言切换
  （`frontend/src/js/core/i18n.test.mjs`）。
- 工具箱页面已改用 `t()`；`npm run build` 通过说明文案目录可被正常打包。

### 待人工验证

- [ ] 打开“工具箱”页面。
  - 预期：标题、分区说明、三张卡片的标题/说明/状态/按钮文案与迁移前完全一致。
- [ ] （迁移完成后）切换语言。
  - 预期：所有已迁移页面的文案整体切换；未迁移页面保持中文原文，不出现空白。

## 14. 自身崩溃上报（Task 10）

### 已经自动化覆盖的部分

- `runGuarded` 捕获 panic 后写出报告，报告包含来源、原因、堆栈、版本、Go 版本、系统与架构。
- 报告附带日志尾部（环形缓冲只保留最近 80 行，已用单元测试固定）。
- 前端未处理异常走同一写入路径（`ReportFrontendError`）。
- 报告数量上限 30 份并自动清理最旧的；`ReadCrashReport` / `DeleteCrashReport`
  拒绝路径穿越等非法文件名。

### 待人工验证

- [ ] 打开“关于”页面。
  - 预期：显示本机崩溃报告数量；点“打开崩溃报告目录”能打开配置目录下的 `crashes/`。
- [ ] 制造一次真实的后台失败（例如断开网络后开始一次工坊下载并触发解析异常），查看报告文件。
  - 预期：`crashes/crash-*.json` 里能看到该次失败的堆栈与日志尾部；
    应用本身不会崩溃退出（除非是 Go 运行时级别的致命错误）。
- [ ] 删除其中几份报告后重启应用，再回到“关于”页面。
  - 预期：数量与实际文件一致；超过 30 份时最旧的被自动清理。

## 记录方式

## 15. 界面接线回归（在真实运行的应用里驱动验证）

### 背景：三个"注入点不同步"的缺陷

设置页/关于页把后端绑定分成三处维护，任何一处漏项都会让控件在运行时静默失效
（历史上表现为"设置页面加载失败：GetWorkshopAutoRedownload is not defined"）：

1. `renderSettingsPage(deps)` 的解构列表；
2. `bindSettingsPage({...})` 的调用点字面量（曾遗漏全部新增绑定，导致白名单、合集、
   组权重、上级分组、自动重下在运行时都是 `deps.X is not a function`）；
3. `renderAboutPage({...})` 的**多个**调用点（`modals/info.js` 曾漏传崩溃报告三项）。

现在 1、2 改为整体透传（`...deps`），并由 `frontend/src/js/features/settings/settings-bindings.test.mjs`
在 `node --test` 中静态守住：裸用未声明的绑定、`deps.X` 未注入、解构了未注入的名字、
以及**每个** `renderAboutPage` 调用点漏传参数都会直接失败。

### 已经自动化覆盖的部分

- 契约测试 5 项（同上），会在 `node --test` 中拦住这三类问题。
- 打包产物自检：`wails build` 后的 `frontend/dist` 中确认包含本轮修复的界面文案
  （关于页提示、白名单按钮、修复建议标题、分层面板按钮等）。

### 已在真实运行的应用里驱动验证（`wails dev` + 浏览器自动化，使用真实数据）

- 设置页四个面板全部渲染，**不再出现"设置页面加载失败"**，也没有"部分设置状态无法读取"提示。
- 原版白名单状态显示 `内置 1 批 / 自定义 0 批，共 9 条路径`；点"刷新白名单状态"后仍正常。
- 工坊设置面板：自动重下开关可读（默认关闭）、工坊合集输入与按钮存在。
- 关于页：显示 `v2.5.14-community.62` 与 `本机还没有崩溃报告`（证明崩溃报告绑定已注入）。
- 工具箱页面正常（i18n 迁移后文案不变）；打开 Mod 冲突检测后：
  `发现 24 组冲突`，并渲染 **50 条修复建议**与"因 Mod 自身的忽略规则跳过"区块。
- Mod 列表角标显示 `优先级 #N` 与 `N 处覆盖`（真实 1963 个 Mod 数据）。
- Mod 详情弹窗的"该 Mod 的冲突忽略文件"显示 `尚未设置：该 Mod 参与全部资源重叠判定`。
- 加载顺序弹窗（全局模式）包含"按分层应用（重排 addonlist.txt）"与分层面板。

### 已在**打包后的 EXE**上通过 cua 原生通道验证（坐标点击 + UIA 文本 + PowerShell 截图）

本机是 Windows 10，`@oai/sky` 的截图与 `element_index` 点击不可用
（`SetIsBorderRequired failed: 不支持此接口`），因此改用「PowerShell 抓窗口截图定位 + 坐标点击」：

- MOD 管理页：1963 个 Mod，卡片角标 `优先级 #N` 正常。
- 设置页：无「设置页面加载失败」、无 `is not defined`、无「部分设置状态无法读取」；
  网络设置 / 界面设置 / 工坊设置 / 游戏配置 四个面板均可切换。
- 设置 → 工坊设置：`下载失败后自动重下一次`、`工坊合集`（含"跟随节点"说明）渲染正常。
- 设置 → 游戏配置（滚动到「Mod 冲突分析」）：`游戏原版文件白名单` 与
  `从原版 pak01 生成批次` 按钮存在，状态显示 `内置 1 批 / 自定义 0 批，共 9 条路径`。
- 关于页：`崩溃报告` 面板显示 `本机还没有崩溃报告`，`打开崩溃报告目录` 按钮存在，版本 `v2.5.14-community.62`。
- 工具箱页：诊断工具三卡 + 崩溃转储查看器 + 游戏配置卡片全部渲染（i18n 迁移后文案不变）。
- 工具箱 → Mod 冲突检测：弹窗显示 `发现 24 组冲突` 与
  `修复建议（需要你确认） 50 条`（含逐条 `设置分层 N` 按钮）。
- 卡片右键 →「详情」：`该 Mod 的冲突忽略文件` 编辑器与
  `尚未设置：该 Mod 参与全部资源重叠判定` 状态正常。
- 卡片右键 →「调整加载顺序」：单 Mod 弹窗显示 `优先级分层（可选）`、
  `−1 / +1 / 保存分层 / 清除分层`，以及 `顺序号 #998 · 有效分层 997`（读自 `GetModPriorityPlan`）；
  全局模式弹窗含 `按分层应用（重排 addonlist.txt）`。

### 仍然只能人工验证

- 会写入用户配置或产生外网请求的按钮（切换自动重下、生成原版白名单批次、
  新建策略组、检查工坊合集更新），本轮只验证了读取路径与函数接线，未实际触发写入。
- 界面视觉细节（间距、配色、长文案换行）仍需人眼确认。

### 第二轮调试（打包 EXE，写入路径）发现并修复的三个缺陷

1. **保存 / 清除分层后列表角标不刷新**：`load-order-priority.js` 只刷新了弹窗里的
   "有效分层"文字；而宿主刷新函数 `refreshModListAfterLoadOrderChange()` 在
   "不是按加载顺序排序"时直接 `return`（默认就是按文件名排序），列表根本没重绘。
   更隐蔽的是该函数里调用的 `renderFileList` **漏了 import**，即使走到也会抛
   ReferenceError 被 try/catch 吞掉。现在：保存/清除分层统一走
   `refreshAfterTierChange()`，宿主函数在任何排序方式下都会刷新映射并重绘。
2. **卡片复用导致角标永不更新**：`getFileCardRenderSignature()` 注释写着"包含卡片内
   渲染的每个值"，但后加的「分层角标」与「变更驱动复检角标」没有进签名，
   卡片被判定为未变化而跳过重绘。签名已补上这两项。
3. **`filters.js` 漏 import `showNotification`**（既有缺陷）：在未选择目录时点击刷新
   会抛 `ReferenceError` 而不是提示"请先选择目录"。

新增的自动化守卫：

- `features/file-list/render-signature.test.mjs`：签名必须覆盖分层与复检角标。
- `features/modals/load-order-priority.test.mjs`：保存/清除分层必须刷新列表；
  宿主刷新函数不得在非加载顺序排序时提前 return。
- `core/cross-module-imports.test.mjs`：**全仓扫描**"调用了项目内导出的函数但没 import"
  （已用"临时移除 import"验证过它会真实失败并精确指出文件与行号）。

端到端结果（在本机打包 EXE 上用坐标点击验证）：给 `!!医疗箱-0主文件.vpk` 保存分层 42 →
角标**立即**变为 `优先级 #998（分层 42）`；清除分层 → 角标**立即**回到 `优先级 #998`，
`priority.json` 变为 `{ "entries": [] }`。

### 第三轮调试（分组管理 + 分组推导，打包 EXE，真实 1968 个 Mod 数据）

本轮桌面处于 Windows 锁屏状态（`LockApp.exe` / `LogonUI.exe` 在跑），`@oai/sky` 的坐标输入
落在安全桌面上无法触达应用。因此改用两条不依赖前台的通道，并都记录在本节：

1. **PrintWindow 抓真实像素**：`PrintWindow(hwnd, hdc, PW_RENDERFULLCONTENT)` 可以抓到
   WebView2 的实际渲染结果，用来做视觉确认（脚本：`scripts/devtools/capture-window.ps1`）。
2. **PostMessage 注入鼠标消息**：向 WebView2 子窗口 `Chrome_WidgetWin_1` 投递
   `WM_MOUSEMOVE / WM_LBUTTONDOWN / WM_LBUTTONUP`（窗口内坐标，DPI=1 时与 CSS 像素一致），
   应用侧等同于真实点击（脚本：`scripts/devtools/click-window-at.ps1`）。点击坐标都先用 DOM 的
   `getBoundingClientRect()` 实测，而不是估计。
3. **临时 JS 调试桥**（仅调试构建，验证后删除）：`internal/app/cua_bridge_debug.go` +
   `LYTVPK_CUA_BRIDGE=1` 时在 `127.0.0.1:9223` 暴露 `POST /eval-sync`，
   内部用 `runtime.WindowExecJS` 在页面里求值并把结果回传，用来读取真实 DOM、监听
   `error` / `unhandledrejection` / `console.error`。

### 已经自动化覆盖的部分

- Go：`internal/app/mod_group_insights_test.go` 12 项，覆盖组归属、整组开关、整组平移、
  推导信号、置信度分级、排序配方、混合包过滤、已建组沉底。
- 前端：`features/mod-groups/*.test.mjs`（组视图纯函数、菜单宽度与对齐约束、禁用原生弹窗）、
  `features/file-list/toolbar-layout.test.mjs`（工具栏换行与目录选择器最小宽度）、
  `features/file-list/priority-sort.test.mjs`（默认按优先级排序）。

### 已在打包 EXE 上驱动验证（真实数据）

- **工具栏布局**：修复前 `.filter-actions` 实测宽 1306px 而可用宽度只有 1122px，
  目录选择器被压成 0 宽度（`clientWidth=0`、`scrollWidth=347`）后溢出去压住动作按钮；
  修复后 `document.querySelectorAll('.filter-row-tools button')` 无任何相交，
  最右元素 1359px < 窗口 1400px。
- **分组菜单**：修复前菜单实测 161px 宽（被 `.dropdown-content { min-width: 120px }` 覆盖），
  且右对齐后左半截溢出到 Mod 管理页之外、被 `.page-view { overflow: hidden }` 裁剪——
  点击"分组建议"实际落到侧边栏「下载与解析」，说明该区域**既看不见也点不到**；
  修复后菜单 420px 宽、`elementFromPoint()` 命中 `#mod-group-suggest-btn`。
- **分组推导**：真实 1968 个 Mod 上返回 40 条建议；修复前列表顶部全是 35–52 个成员的
  "低置信度 · 主体：混合包（…）"，修复后为 2–4 个成员、带「共同标签 / 同一作者」的中置信度小组。
- **创建为组**：真实点击「创建为组」→ `groups.json` 写入 `猎枪 武器`（root + `workshop\` 两份同名条目），
  卡片立即出现 `组：猎枪 武器` 徽标，分组菜单出现 `猎枪 武器（2）`，
  建议列表已把该条标记「已建组」并沉底。
- **按组筛选**：勾选该组后列表从 1968 张卡片收敛到 2 张，筛选按钮文案变为 `猎枪 武器`；
  点「清空」恢复 1968 张。
- **整组开关（安全路径）**：真实点击组徽标 → 应用内确认弹窗出现
  （标题「整组开关」+ 成员数 + 当前开关统计 + "只改 addonlist.txt 的 0/1"）；点「取消」后
  `addonlist.txt` 的 SHA-256 保持 `f4ac529e…9df89b9` 不变，确认没有误写。
- **整组优先级移动**：点「↑ 前移」→ `priority.json` 出现 `workshop\3245634710.vpk` 的 `tier: 490`，
  卡片角标同步变为 `优先级 #486`，`addonlist.txt` 哈希不变（符合"只写分层、不重排"的设计）。
  验证后已把 `priority.json` 还原为 `{ "entries": [] }`。
- **应用内输入弹窗**：真实点击「用选中的 Mod 建组」→ 弹出应用内弹窗（不再是 `window.prompt`），
  填入组名后「创建为组」成功写入 `groups.json`（测试组已用 `DeleteModStrategyGroup` 删除）。
- **整站巡检**：用坐标点击依次进入 MOD 管理 / 创意工坊 / 下载与解析 / 收藏服务器 / 工具箱 /
  设置（网络、界面、工坊、游戏配置四个分页）/ 使用说明 / 关于，全程
  `window.onerror`、`unhandledrejection`、`console.error` **零新增**（截图见
  `.tmp-cua/shots/sweep/`）。

### 只能人工验证 / 仍未验证

### 第四轮调试（分组推导的内容化与降噪，真实 1968 个 Mod）

本轮把推导从"元数据启发式"改成"内容证据优先"，并用用户真实库做了整轮验证
（临时 Go 用例直接把 `rootDir` 指向真实 `addons`，跑完即删）：

- 扫描 1968 个文件用时 ≈2.2s；`SuggestModGroups()` 用时 ≈7ms。
- 修改前的前 30 条里，内容是"混合包主体聚类 + 大量占位符/联名作者"
  （`AUTHOR_NAME`、`Animal33/zmg/momo`、`Dazzle_白麒麟 + All_calm`）。
- 修改后前 40 条**全部**是内容聚类：`Francis 语音`、`Coach 语音`、`Tank 模型`、
  `Boomer 模型`、`吉他 武器`、`M16 武器`、`军狙 武器`、`sg552 武器`、`准星`、`煤气罐`、
  `一代子弹堆`、`战役地图：…` 等，弱启发式（同作者 / 同前缀）被挤到 40 条之外。
- 验证的三条降噪规则：占位符与多人联名作者被过滤、同一作者超过 12 个成员不成组、
  横跨主分类（武器 + HUD）的候选整桶丢弃。

同时修掉一个真实缺陷：`LogError` 在 `a.ctx == nil`（启动早期 / 后台任务 / 自动化场景）时调用
`runtime.EventsEmit`，Wails 会用无效上下文直接中断进程；现在只写日志。新增
`internal/app/log_error_guard_test.go` 守住。

### 第五轮调试（推导更广更准 + 建议弹窗重做）

先用真实库统计了各信号的数据可用性（临时用例，跑完即删）：

```
总文件 1968；语音角色 21；XDR 74；主体摘要 1968（高 1463 / 中 0 / 低 505）；
有 ≥2 标签 1622；有作者 1689；有主分类 1968
主分类分布: 武器 910 / 其他 670 / 人物 307 / 地图 59 / … 另有脏值（三角洲 4、Milfy 4、m16 1 …）
```

据此做了三类改动，并逐条加了 Go 回归测试：

- **更广**：新增「语音角色」信号（`VoiceCharacters`）；「共同标签」从"只比前两个标签"
  改成枚举**任意两个**标签（真实库里同一套件常出现标签顺序不同）。
- **更准**：跳过解析器标注"低置信度"的主体（505 个，例如「脚本资源（无法确认具体对象）」）；
  主分类字段里的脏值按"未知"处理，避免把同套件的 Mod 误判成跨分类而拆开。
- **界面**：分组建议弹窗重做成冲突检测那种大界面，每个成员一行并带
  详情 / 游戏开关 / 启用·禁用·复制到 addons 按钮，支持全选 / 全不选。

真实数据复核（1968 个 Mod）：前 40 条全部是内容聚类，
`Ellis 语音 / Nick 语音 / Francis 语音 / Coach 语音`（语音角色 + 主体 + 标签 多信号命中）
排在前面，其余为 `M16 武器`、`吉它 武器`、`Tank 模型`、`准星`、`战役地图：…` 等。

打包 EXE 视觉复核截图（PrintWindow）：弹窗宽度 1180px、每个成员行含
`创意工坊 / 游戏开关：关 / 优先级 #221` 徽标与三个按钮，按钮无重叠、文字不截断。

### 第六轮调试（外部组建议导入：让大模型 / 人工来推导）

新增标准格式的「组建议导入」（格式说明见 `docs/features/group-suggestion-import.md`）：

- 后端：`internal/app/mod_group_suggestion_import.go`，含
  `GetExternalGroupSuggestions` / `ImportGroupSuggestionsFromFile` /
  `ImportGroupSuggestionsOpenDialog` / `ClearExternalGroupSuggestions` /
  `ExportGroupingCatalog` / `ExportGroupingCatalogDialog` / `GetGroupSuggestionInboxPath`；
- 收件箱：`%AppData%\LytVPK\group_suggestions.json`，放进去后点「重新推导」即自动读取；
- 合并策略：导入的建议带 `source=external`、分数 90，排在内置启发式之前，并同样参与
  「已建组」判定与成员规模限制（300）。

**已自动验证**（`internal/app/mod_group_suggestion_import_test.go` 7 项）：
addonlist 键 / 裸文件名 / 绝对路径三种成员写法都能解析；裸文件名命中根目录与 workshop 两份时
会同时纳入并给出提示；匹配不到的成员只记警告不阻断；有效成员不足、成员过多、重名标签、
版本不支持、JSON 非法、`suggestions` 为空都会按预期拒绝或跳过；导出清单包含
键 / 标题 / 作者 / 标签 / 主体 / 语音角色 / 工坊 ID；清除导入后不再返回外部建议；
导入后的建议确实排在内置建议之前。

**已在真实数据上端到端跑通**（1968 个 Mod）：

1. 用真实库导出 `grouping_catalog.json`（907 KB，1968 条）；
2. 由模型逐组核对后写出 6 条建议（`MAC-10 皮肤两个版本`、`肾上腺素塔菲/花来`、
   `武士刀替换合集`、`Witch 模型替换`、`击中反馈 HUD 两份副本`、`玲纱 AA12 两个版本`），
   放进收件箱；
3. 打包 EXE 打开「分组建议」：概览显示 `共 40 条建议（其中 6 条来自导入的建议文件）`，
   6 条导入建议排在最前并带「导入建议」徽标，成员行的位置 / 游戏开关 / 优先级 / 操作按钮
   全部正常渲染（截图：`.tmp-cua/shots/import-demo.png`）。

**只能人工验证**：

- 「导入建议文件…」的**系统文件选择框**本身（本机桌面处于锁屏，无法点击原生对话框）；
  同一条路径的后端方法 `ImportGroupSuggestionsFromFile` 已由 Go 测试覆盖。
- 导入后点「创建为组」把建议落盘成策略组（会写 `groups.json`，未在本轮自动触发）。

### 第七轮（智能体提示词 + 推导引擎重构）

两块内容：

1. **智能体提示词**（随 EXE 发布，`internal/app/assets/group_suggestion_agent_prompt.md`）：
   弹窗新增「准备给智能体的材料」（导出清单 + 复制已填好路径的提示词 + 弹窗给出两条路径）、
   「复制智能体提示词」「保存提示词…」；绑定 `GetGroupSuggestionAgentPrompt` /
   `SaveGroupSuggestionAgentPromptDialog` / `PrepareGroupingWorkspace`。
2. **推导引擎重构**：算法搬进独立纯逻辑包 `internal/grouping`
   （`grouping.go` / `index.go` / `providers.go`），App 只做适配
   （`internal/app/mod_group_suggest.go`）。信号集中登记（Catalog）、
   一次遍历建倒排索引、合集与外部建议通过注入候选走同一条流水线、
   新增单信号配额与推导统计、收件箱按 mtime+size 缓存。

**已自动验证**：

- `internal/grouping` 9 项测试：7 个信号都能产出候选、同成员集合合并并保留最强理由、
  单信号配额生效、结果确定（两次推导一致）、Catalog 与展示名/ID 双向映射、
  可信度分级、作者与主体过滤、排序配方（合集 120 / 主体 70 / 粗糙标签 49 / 弱启发式 45）。
- `internal/app` 新增 3 项：提示词自包含（含格式、策略、硬性要求、真实路径，且占位符已替换）、
  `PrepareGroupingWorkspace` 能导出清单并返回提示词、App 信号常量与 `internal/grouping`
  目录一致（防漂移）。
- 基准测试：2000 个合成 Mod（覆盖全部信号）单次推导 **≈6.9 ms**
  （`go test ./internal/grouping -bench BenchmarkSuggest`）。

**已在打包 EXE 上驱动验证**：点「准备给智能体的材料」→
`%AppData%\LytVPK\grouping_catalog.json` 生成（939 KB / 1968 个 Mod），
剪贴板拿到提示词，弹窗显示清单路径与建议文件路径（截图
`.tmp-cua/shots/agent-prepared.png`）。

**只能人工验证**：剪贴板内容在外部应用（Codex / Claude）里粘贴后的实际效果，
以及「保存提示词…」的系统保存对话框。

### 第八轮（清单信息补全：让智能体不用自己打开 VPK）

在扫描阶段顺带统计 VPK 内部结构（零额外 IO），并把它写进导出的清单：

- `structure.topDirs`（顶层目录 + 条目数）、`structure.fileCount` / `totalSize`；
- `structure.targets`（压缩后的替换目标，如 `props_interiors/medicalcabinet02`，≤6 条）；
- `structure.samplePaths`（≤5 条原始路径，截断到 100 字符，过滤 `addoninfo` / 预览图）；
- 每个 Mod 追加 `contentSubjects` / `xdrSummary` / `modelCount` / `modelTriangles` / `campaign` /
  `location` / `gameEnabled` / `gameStateKnown` / `loadOrder` / `effectiveLayer` /
  `prioritySource` / `size` / `lastModified`；
- 清单顶层追加 `clusterHints`（内置推导候选簇，约 8 KB）与 `duplicateGroups`
  （`同名同体积` / `同名不同位置`，最多 400 组），`notes` 里写明"先读小段、再按需查明细"。

规模与耗时（真实库 1968 个 Mod，临时用例，跑完即删）：

```
mods=1968 有结构摘要=1968 有代表性路径=1968 有有效分层=1679 重复组=400
清单 3.05 MB（补结构前 0.94 MB；未压缩路径时 5.46 MB）
clusterHints 60 条 = 8.3 KB，duplicateGroups 400 组
样例 targets: doors/medkit_doors_open | props_interiors/medicalcabinet02 | theresa/shader/toon_normal …
```

**已自动验证**：`TestExportGroupingCatalogIncludesStructureAndState`（顶层目录、条目数、体积、
替换目标、代表性路径、`addoninfo` 被过滤）、`TestExportGroupingCatalogPrecomputesDuplicateGroups`、
`TestExportGroupingCatalogIncludesClusterHints`、提示词包含 `structure.targets` /
`clusterHints` / "你不需要打开 VPK" 等关键说明。

**只能人工验证**：智能体真实读这份清单后的分组质量（需要在外部 Codex / Claude 会话里跑一遍）。

### 第十轮（按外部智能体实测反馈修正）

反馈文件：`E:\SteamLibrary\steamapps\common\Left 4 Dead 2\program\LytVPK-分组建议-实测反馈-20260922.md`
（外部 agent 用 1968 个 Mod 产出 187 组 / 1714 成员后的复盘）。本轮按"只处理 addons / workshop /
disabled"的范围落地的项与证据：

| 反馈项 | 处理 | 证据 |
| --- | --- | --- |
| P0-1 同 `version: 1` 字段扩张 | 清单升到 `version: 2` + `schemaRev` + `capabilities`（15 项能力） | CLI 导出：`version=2 schemaRev=2026-09-22.2` |
| P0-2 `key` 不唯一 / location 矛盾 | 新增唯一 `entryId`（`root/…`、`disabled/…`、`workshop/…`）与 `relativePath`；location 改为按物理路径现算 | CLI 导出：**1968 条记录 / 1968 个唯一 entryId**，9 个同键多位置各有独立 entryId |
| P0-2 假歧义警告（229 条） | `resolve()` 改为 entryId → 精确键 → 裸文件名，且同键多位置不算歧义；`members` 支持 `entryId` / `disabled\…` / `workshop\…` | CLI dry-run：该文件 **total=187 valid=187 invalid=0 members=1714 warnings=0** |
| P0-3 扫描范围不透明 | 顶层 `scope` 给出覆盖位置 + 被排除子目录数与 VPK 数（本轮 110 个目录 / 1645 个 VPK） | CLI 导出 `scope` 字段 |
| P2-1 `clusterHints` 覆盖太低（60 → 124） | 上限提升到 600（实测 481 条），并新增 `ungroupedKeys`（1738 条）供接力 | CLI 导出统计 |
| P1-3 缺"主题套装 / 前置"表达 | 新增 `themeHints`（60 条，如 绣春刀/爱弥斯 类跨槽位套装）与 `preloadHints`（观察到的 xdReanimsBase / KSEP / 音频库等前置） | CLI 导出统计 |
| P2-2 XDR 缺槽位 | 每条记录新增 `xdrSlots`（角色/模型/槽位/动作/置信度，67 条覆盖） | CLI 导出统计 |
| P2-3 .vpk 实为 ZIP | 扫描时记录解析失败文件，清单 `unreadableMods` 显式列出（实测 1 条） | CLI 导出 `unreadable=1` |
| P2-4 缺 dry-run 入口 | 新增 CLI `--validate-group-suggestions <文件> [--out <json>]`（逐成员 resolved/matched/ambiguous + 全部警告 + 退出码）与 `--export-grouping-catalog [路径]` | 上述 CLI 实跑；结果文件 `group_suggestions.json.validation.json` |

**未做（有意）**：`addons` 下其它子目录的扫描（用户明确暂不处理，见 `scope.excluded*` 字段说明）。

**已在打包 EXE 上验证**：导入外部 agent 的 187 组建议后，弹窗显示「共 40 条建议（其中 40 条来自导入的建议文件）」，
导入建议排在最前并带「导入建议」徽标，成员行的位置 / 游戏开关 / 优先级 / 详情 / 启用·禁用按钮正常
（截图 `.tmp-cua/shots/final-verify.png`）。

### 第九轮（清单覆盖"LytVPK 已解析的全部信息"）

在第八轮基础上继续补：

- `addonInfo.*`：VPK 内 `addoninfo.txt` 的 `version` / `desc` / `url` / `hasUpdate` / `chapters` / `mode`；
- `workshop.*`：本地 `.meta` 的工坊 `title` / `author` / `desc` / `tags` / `url` / `previewUrl` /
  `timeUpdated` / `downloadedAt`，以及"稍后再看"里的 `views` / `subscriptions` / `favorited` / `fileType`；
- `management.*`：`groups`（策略组）/ `profiles`（启用方案）/ `dependencies`（已声明依赖）/
  `ignoredFiles`（冲突忽略清单）/ `collections`（已保存工坊合集）；
- 顶层 `coverage`：各类信息的可用条数，供智能体判断数据是否充分。

真实库统计（1968 个 Mod，配置目录指向本机 `%AppData%\LytVPK`）：

```
coverage = {"mods":1968,"withStructure":1968,"withStructureTargets":1968,"withAddonInfo":1590,
            "withWorkshopMeta":13,"withWorkshopTags":0,"withWatchLaterStats":0,
            "withVoiceCharacters":21,"withSubject":1968,"withManagement":2,
            "withProfileOrGroup":2,"clusterHints":60,"duplicateGroups":400}
清单 3.23 MB
```

注：`withWorkshopMeta` 只有 13 是因为本机 `addons` 目录里实际只有 18 个 `.meta`
（工坊资料由 LytVPK 在下载/刷新时保存）；提示词里已说明这种缺失属于数据缺失，
可改用文件名 / `structure.targets` / 标签 / 主体判断，或开启工坊信息存储后补齐。

**已自动验证**：`TestExportGroupingCatalogIncludesWorkshopAndManagement`（`.meta` 的标题/作者/标签/
详情页、稍后再看统计、addoninfo 版本与描述、策略组 / 依赖 / 忽略清单 / 合集全部出现在清单里）、
提示词包含 `workshop.title` / `workshop.tags` / `addonInfo.desc` / `management.dependencies` /
`management.collections` / `coverage` / `withWorkshopMeta` 等新字段说明。

**注意：本轮明确不做 `addons` 其它子目录的扫描。** `ScanVPKFiles` 仍然只扫
`addons` 根目录（不递归）+ `workshop` + `disabled`（后两者递归），
因此像 `addons\Airi初代恶堕战斗员八人\...` 这种自建套件文件夹里的 VPK
**不会出现在列表里，也不会参与推导**（实测磁盘上有 3614 个 .vpk，应用里是 1968 个）。

- 真机游戏内加载顺序实测：`addonlist.txt` 顺序仍是游戏侧唯一权威，分层只在应用侧生效，
  需要进游戏确认实际生效顺序。
- 整组开关的**执行分支**：本轮真实点击只走到确认弹窗并取消（避免改动用户 Mod 开关），
  写入语义由 `internal/app/mod_group_insights_test.go` 的 Go 端到端用例覆盖。
- 蒸汽工坊合集的真实刷新 / 下载（需要真实合集链接与网络）。
- 纯视觉判断（配色、间距在大字体 / 高分屏下的观感）仍建议人眼过一遍。

### 第十一轮（策略组生命周期补全 + 分组建议阅览层，打包 EXE 驱动验证）

本轮补的是"组这个对象本身"的生命周期（此前只有创建 / 应用 / 删除，且删除用的是原生 `confirm`），
以及 234 条建议时的浏览器（可缩放、可搜索、可折叠、分页）。

**已自动验证**：

- Go（`internal/app/mod_group_lifecycle_test.go` 新增 5 项）：
  `RenameModStrategyGroup` 改名 + 改描述 + 落盘 + 空名/未知组报错；
  `AddModStrategyGroupMembers` 加入后原有成员保持、显示名取自缓存、重复加入幂等；
  `RemoveModStrategyGroupMembers` 幂等、且**不允许把成员全部移出**；
  `SetModStrategyGroupMembers` 整体替换、`workshop\…` 键显示名回退为 `id.vpk`；
  `DeleteModStrategyGroup` 把下级分组提升为顶层（子组不被连带删除）、重复删除报错、
  删除前后 `addonlist.txt` **逐字节相同**。
- 前端纯函数（`features/mod-groups/suggestion-filter.test.mjs`，10 项）：搜索命中标题 / 成员名 /
  成员键 / 理由；置信度与来源筛选；置信度优先排序（同级按成员数再按分数）；成员数排序；
  折叠预览；分页与"还有 N 条"文案；弹窗尺寸解析（坏 JSON / 缺字段返回 null）。
- 前端结构约束（`features/mod-groups/group-suggest-layout.test.mjs`，6 项）：筛选控件与"显示更多"
  必须存在；弹窗 CSS 必须 `resize: both` 且有最小尺寸；必须用 `ResizeObserver` + localStorage
  记住尺寸；折叠态隐藏成员明细；必须复用 `suggestion-filter.mjs` 的分页；筛选只重画列表不重新推导；
  分组菜单必须提供重命名 / 删除 / 加入 / 移出四个入口。
- 绑定契约（既有 `settings-bindings.test.mjs`）：新增的 `RenameModStrategyGroup` 被解构但没注入时，
  测试会直接失败（本轮就是这样发现并补上 `app-runtime` 注入的）。

**已在打包 EXE 上驱动验证**（`build/bin/LytVPK-Community-Fork.exe`，1400×900，PrintWindow 抓真实像素 +
PostMessage 投递鼠标 / 字符消息，脚本见 `scripts/devtools/`）：

1. 工具栏「分组」菜单展开后，每个组一行显示
   `↑前移 ↓后移 整组开关 ＋选中的 N 个 －选中的 N 个 重命名 删除`（截图 `.tmp-cua/shots/r11-group-menu2.png`）。
2. 「分组建议」弹窗默认折叠：标题 + 高置信度 + 导入建议 + `11 个 Mod` + 「展开详情」+ 成员预览
   （`…还有 8 个`），筛选栏显示「显示 24 / 234 条建议（还有 210 条未显示）」
   （`.tmp-cua/shots/r11-suggest-modal.png`）。
3. 「展开详情」后出现完整成员行（`详情 / 游戏开关 开 / 禁用` + 位置徽标 + `优先级 #1036`），
   标题变为可编辑输入框（`.tmp-cua/shots/r11-suggest-expanded.png`）。
4. 拖右下角把弹窗拉大（`drag-window-at.ps1`），关闭再打开后尺寸保持不变 —— 缩放与
   localStorage 记忆都生效（`.tmp-cua/shots/r11-suggest-resized2.png` 与 `r11-suggest-reopen.png`）。
5. 「全部折叠」把已展开的卡片收回到摘要；搜索框输入 `M16` 后计数变为「显示 9 / 9 条建议」，
   列表只剩 9 条命中（含成员名带 `MW22 M16` 的主题套装）
   （`.tmp-cua/shots/r11-suggest-search2.png`）。
6. 「重命名」弹出应用内输入框并预填当前组名；「删除」弹出应用内确认框，正文列出
   「只删除 groups.json 里的这一条记录（6 个成员）/ 不会改动 addonlist.txt，也不会删除任何 Mod 文件 /
   组成员已经写入的 Mod 分层（priority.json）会保留 / 下级分组会回到顶层」
   （`.tmp-cua/shots/r11-rename-dialog.png`、`.tmp-cua/shots/r11-delete-dialog.png`）。

**本轮的人工点击只走到弹窗并取消**，没有真的删除任何策略组；删除语义由上面的 Go 用例覆盖。

**只能人工验证**：

- 置信度 / 来源 / 排序三个下拉的**原生下拉弹层**无法用 PostMessage 驱动（WebView2 的原生控件），
  其筛选与排序逻辑由纯函数测试覆盖，下拉本身的观感需要人眼确认。
- 大建议量下的主观流畅度（拖缩放条、连续展开多张卡片）建议在真机上体感一次。

### 第十二轮（单选 / 多选直接改分组：加入 / 移出 / 换组，打包 EXE 驱动验证）

**已自动验证**：

- Go（`internal/app/mod_group_lifecycle_test.go` 新增 4 项）：
  `MoveModStrategyGroupMembers` 正常移动（报告 `removedFromSource` / `addedToTarget` / `moved` /
  两侧剩余成员数）、重复移动幂等、把源组移空时整体拒绝且两边都不变、源组=目标组 / 未知组 /
  空键全部报错、移动前后 `addonlist.txt` 逐字节相同。
- 前端纯函数（`features/mod-groups/group-picker-view.test.mjs`，7 项）：add 模式下"全都已在组里"
  置灰、remove 模式下"一个都不在组里"置灰、"把整组移空"置灰、move 模式下源组不能当目标、
  汇总文案（已选中 N 个 / 可以加入 · 移出 · 移动 M 个组）与结果文案。
- 接线约束（`features/mod-groups/group-picker.test.mjs`，6 项）：弹窗结构与脚本 id 一致、
  复用四个后端方法并刷新组归属、单文件菜单与多选菜单都挂了三个入口、只在属于某组时才出现
  移出/移动、多选时"只属于一个组就直接带上源组"、启动时 `initGroupPicker()` 被调用、
  「新建并加入」位于滚动区之外（`.group-picker-new-row` 是 `flex: 0 0 auto` 且不在 body 内）。
- 静态守卫（`core/module-import-contract.test.mjs`，1 项）：全仓扫描命名导入是否真实存在。
  用"把 `GROUP_PICKER_MODES` 改回从 `group-picker.js` 导入"验证过它会失败并精确报出
  `context-menu.js: "GROUP_PICKER_MODES" 来自 ../mod-groups/group-picker.js，但该模块没有导出它`。

**已在打包 EXE 上驱动验证**（右键 = `click-window-at.ps1 -RightButton`）：

1. 不属于任何组的 Mod（星雪特效平台整合版）：右键菜单出现「加入策略组…」
   （`.tmp-cua/shots/r12-context-menu.png`）；
2. 点击后弹出「加入策略组」选择器：`已选中 1 个 Mod；下面 17 个策略组里 17 个可以加入`，
   每行显示组名 + 成员数，未选中目标时「加入这个组」为禁用态（`.tmp-cua/shots/r12-group-picker.png`）；
3. 用「分组」菜单筛出 `!!医疗箱 系列` 后，组内 Mod 的右键菜单同时出现
   「加入策略组… / 从策略组移出… / 移动到其它策略组…」（`.tmp-cua/shots/r12b-menu-in-group.png`）；
4. 点「移动到其它策略组…」弹出移动选择器：
   `已选中 1 个 Mod；从「!!医疗箱 系列（主文件+音频+材质+开关模块）」移动到下面 16 个策略组之一`，
   源组那一行显示为禁用并注明「就是当前所在的组」（`.tmp-cua/shots/r12b-move-dialog.png`）。

**本轮所有真机点击都只到"打开选择器"为止，最后都点了取消**：没有真正写 `groups.json`，
也没有改动任何 Mod 或 `addonlist.txt`（写入语义由上面的 Go 用例覆盖）。

**只能人工验证**：点「加入 / 移出 / 移动」后的最终确认动作（会写 `groups.json`）没有在真机上执行，
建议你自己挑一个 Mod 走一遍，确认提示文案与实际结果一致。

### 第十三轮（标签 × 分组双向互补，打包 EXE + 真实 1969 个 Mod 验证）

**已自动验证**：

- Go（`internal/grouping/tags_test.go` 7 项）：精确覆盖（标签恰好只筛出这一批）、
  有集合外同标签时降级为 partial/wide、单成员命中的标签不算证据、一级标签同样参与判定、
  标签频次排序、`Suggest` 会给建议贴上 `tagKey`/`tagScope`/`tagInSet`/`tagOutside`
  且**不改动引擎既有排序**。
- Go（`internal/app/mod_group_tags_test.go` 6 项）：`SuggestModGroups` 的标注；
  `GetGroupTagSuggestions` 从组名 / 主体识别推导标签、跳过"全都有该标签"与单成员组；
  `ApplyTagToModKeys` 打标签 + 幂等 + 缺失文件单独报告 + 空标签/空目标报错；
  **打标签后组成员键自动改绑**（回归本轮修复的真实缺陷）。
- 前端纯函数（`suggestion-filter.test.mjs` 新增 3 项）：`exact` 才叫"标签已覆盖"、
  默认隐藏、可切换显示 / 只看、计数；`group-tag-view.test.mjs` 3 项：打标签结果汇总
  （已应用 / 已有 / 缺失 / 失败）与标签来源文案。
- 结构约束（`group-suggest-layout.test.mjs` 新增 2 项）：筛选栏必须有「标签已覆盖」开关、
  卡片必须能「用标签筛选」并写入二级标签筛选；分组菜单必须有「打标签…」入口与对话框结构。

**已在打包 EXE 上驱动验证**（真实库：1969 个 Mod、234 条建议、15 条标签已覆盖）：

1. 分组建议弹窗概要行显示「共 234 条建议（其中 189 条来自导入的建议文件）·
   **15 条标签已覆盖（可直接用标签筛选）**」，筛选栏出现下拉「隐藏标签已覆盖」，
   计数为「显示 24 / **219** 条建议（还有 195 条未显示）」——234 − 15 = 219，
   说明 15 条标签已覆盖的建议确实被默认隐藏（`.tmp-cua/shots/r13-suggest-tag-filter.png`）；
2. 「分组」菜单里每个组都出现「打标签…」（`.tmp-cua/shots/r13-group-menu.png`）；
3. 点开「打标签…」后对话框显示：`给「!!医疗箱 系列（主文件+音频+材质+开关模块）」打标签`、
   `建议标签：!!医疗箱 系列（主文件_音频_材质_开关模块）（来自组名）；可以直接改成别的名字。`、
   这一组共 6 个成员的预览，以及"会改 addonlist 键、工具会自动改绑本地记录"的说明
   （`.tmp-cua/shots/r13-tag-dialog.png`）。

**本轮真机点击都在对话框上取消了**：没有真的给任何 Mod 打标签（那会改文件名），
写入语义由 Go 用例在临时夹具上覆盖。

**只能人工验证**：在真实 Mod 上点一次「给这组打标签」，确认标签前缀符合预期
（`[标签]文件名.vpk`）且游戏内仍能正常加载；以及「用标签筛选」后的列表观感。

### 第十四轮（智能体在建议文件里一起产出标签；真实 EXE 跑 CLI 校验）

**已自动验证**：

- Go（`internal/app/mod_group_import_tags_test.go` 5 项）：导入文件里的 `tag` / `tagReason` /
  `memberTags` 会透出到建议与标签提案（`tagOrigin=imported`）；文件没给标签时降级到主体识别；
  校验输出统计 `withTags`（只算合法标签）与 `taggedMembers`；
  **非法标签**（超过 24 字符或含 `+`）会把该条判为 invalid 并在 `problems` 里写明；
  没有 `tag` 字段的旧文件仍然 valid（向后兼容）。
- 提示词（既有 `TestGroupSuggestionAgentPromptIsSelfContained` 扩充）：必须包含 `"tag"`、
  `tagReason`、`memberTags`、`24 个字符`、`降级`、`withTags`、`taggedMembers`。
- 前端纯函数（`group-tag-view.test.mjs` 新增 2 项）：`buildTagApplyPlan` 把整组标签套用到全部成员、
  逐成员标签按标签归并、大小写去重、空提案返回空计划；结构约束测试断言卡片会用
  `buildTagApplyPlan` + `ApplyTagToModKeys` 并显示「导入标签：」徽标。

**已在打包 EXE 上验证（真实 Mod 目录 + 临时建议文件，不动用户数据）**：

用 `--validate-group-suggestions` 跑一份**放在 %TEMP% 里**的带标签建议文件，
确认真实二进制：`withTags` / `taggedMembers` 统计正确、非法标签会被判 invalid 并给出原因。
（该命令只读 Mod 目录、只写同目录下的 `.validation.json`，不写收件箱、不改任何 Mod。）

**只能人工验证**：

- 外部智能体读完新提示词后**实际写出的标签质量**（需要在真实 Codex / Claude 会话里跑一遍）；
- 在真实 Mod 上点「应用标签」，确认 `[标签]文件名.vpk` 前缀与游戏内加载都正常
  （本轮只用临时夹具覆盖了写入语义）。

### 第十五轮（提示词写明处理范围 + 可编辑提示词，打包 EXE 驱动验证）

**已自动验证**：

- Go（`internal/app/mod_group_agent_prompt_edit_test.go` 4 项）：默认状态返回内置模板
  （保留占位符、含「处理范围」一节、给出 3 个占位符）；保存自定义后 `GetGroupSuggestionAgentPrompt()`
 会把占位符替换成真实路径（<code v-pre>{{</code> 不再残留）；空白内容被拒绝；恢复默认会删掉自定义文件且幂等；
  `PrepareGroupingWorkspace` 会把自定义提示词一起交给「准备给智能体的材料」。
- 提示词内容（既有 `TestGroupSuggestionAgentPromptIsSelfContained` 扩充）：必须包含
  「处理范围（硬性」「`addons\workshop\`」「`addons\disabled\`」「不参与扫描」。
- 前端纯函数（`agent-prompt-view.test.mjs` 3 项）：状态行区分内置/自定义/未保存修改；
  占位符在光标处插入（含选中替换与越界夹取）；保存/恢复默认的结果文案。
- 结构约束（`group-suggest-layout.test.mjs`）：编辑器入口、弹窗结构、三个后端方法、
  占位符插入函数都必须接好，且 `#agent-prompt-modal` 必须用 `--z-popover` 提高层级。

**已在打包 EXE 上驱动验证**（真实 1969 个 Mod；本轮临时启用了一个只监听 127.0.0.1 的调试桥来
读取真实 DOM / 调用绑定，验证结束**已删除**该文件并重新构建）：

1. 「分组建议」工具栏出现「编辑提示词…」；按钮矩形 `500,156,72×32`，`elementFromPoint`
   命中的就是该按钮（真实鼠标不会点到别的东西）；
2. 点开后弹窗显示：`当前使用：LytVPK 内置默认提示词`、三个占位符芯片、
   7925 字符的可编辑提示词（开头就是「### 处理范围（硬性，先读这条）」）、
   `恢复默认 / 取消 / 保存`（截图 `.tmp-cua/shots/r14-editor-open.png`、`r14-editor-scope.png`）；
3. 改内容后状态变成 `… · 有未保存的修改`；点「保存」→ 落盘
   `%AppData%\LytVPK\group_suggestion_agent_prompt.md`、`isCustom=true`、记录保存时间，
 随后 `GetGroupSuggestionAgentPrompt()` 返回的内容里已把
 <code v-pre>{{CATALOG_PATH}}</code> 换成真实路径、且不再含 <code v-pre>{{</code>；
4. 点「恢复默认」→ 先弹确认框，确认后 `isCustom=false`、编辑器内容回到内置版本、
   状态行回到「当前使用：LytVPK 内置默认提示词」，并且内置版本里确实包含
   「处理范围（硬性」与 `memberTags` 两段。

**注意**：本轮对 `%AppData%\LytVPK\group_suggestion_agent_prompt.md` 的写入与删除都是
本功能自身的产物（自定义提示词文件），未触碰任何 Mod、`addonlist.txt` 或 `groups.json`。

**只能人工验证**：真实鼠标点击「编辑提示词…」的观感、以及在大字号/高分屏下编辑长文本的体验。

### 第十六轮（导出材料前自动重新扫描，回答"重复点按钮会不会更新"）

**背景**：用户问"点过一次『准备给智能体的材料』，之后加/删 Mod、刷新列表，再点一次会不会更新"。
结论是**会**（导出始终按当前缓存重建），但当时的实现要求"用户先自己刷新"——按钮本身不扫描。
本轮把它改成"导出前自动扫一遍"。

**已自动验证**：

- Go（`internal/app/mod_group_catalog_freshness_test.go` 4 项）：
  - 加/删文件 + 重新扫描后清单跟随（`TestGroupingCatalogReflectsAddedAndRemovedModsAfterRescan`）；
  - **不手动刷新**直接导出，新增的 Mod 在清单里、删掉的 Mod 不在
    （`TestExportGroupingCatalogScansBeforeExport`）；
  - **不手动刷新**直接「准备材料」，`modCount` 与清单都跟上
    （`TestPrepareGroupingWorkspaceScansWithoutManualRefresh`）；
  - 再次准备材料仍写同一路径、覆盖为最新内容（`TestPrepareGroupingWorkspaceUsesLatestScan`）。
- 结构约束（`group-suggest-layout.test.mjs`）：两个按钮的 tooltip 必须写明"先重新扫描一遍 mod 目录"、
  必须有「正在扫描并导出…」进行中状态、成功提示必须是"已按最新扫描导出"，
  且准备材料 / 导出清单都要在 `finally` 里恢复按钮。

**已在打包 EXE 上验证（CLI 冒烟，真实库 2018 个 Mod，只读 + 写临时路径）**：

```
--export-grouping-catalog %TEMP%\lytvpk-catalog-smoke-2.json
→ exit=0，冷启动全量扫描 + 导出共 1.51s
→ modCount=2018，version=2，schemaRev=2026-09-22.2，generatedAt 为本次执行时间
```

（GUI 里是**热缓存增量扫描**：只有目录遍历 + 逐文件 stat，未变化的文件不重新解析；
冷启动那种"全部重新解析 2018 个 VPK"的情况不会出现在重复点击上。）

**注意**：CLI 的 `--export-grouping-catalog` 走的是"已经扫过就不再扫"的内部路径，
所以这次冒烟本身没有额外多扫一遍。

**只能人工验证**：在界面上点「准备给智能体的材料」时，体感上多出来的那一小段扫描耗时是否可以接受
（按钮会显示「正在扫描并导出…」）。

### 第十七轮（按外部智能体 v3 反馈补齐 hints 身份字段与时间差提示）

**背景**：外部智能体用新清单（2018 个 Mod / community.64）产出 v3 建议文件
（189 组 / 1689 成员 / `withTags=189`、`taggedMembers=150`），校验 0 invalid 后给出 3 条清单侧观察。
本轮逐条处理。

**已自动验证**（`internal/app/mod_group_catalog_hints_test.go` 4 项）：

- `preloadHints` 必须带 `entryId` 且 `memberEntryIds` 与之一致；
- root/disabled 同名（同键）副本会被标注 `singleAddonListKey: true` + 说明性 `note`，
  而真正不同键的副本（root + workshop 同名）不被标注；
- `importGroupSuggestionsFresh`（界面「导入建议文件…」走的路径）会先重新扫描：
  刚放进目录、未手动刷新的成员不会被判成"未匹配"；
- 校验出现未匹配成员时，警告里会附带「重新导出材料」的处置建议。
- 提示词自检新增断言：「清单与磁盘的时间差」一节与 `generatedAt` 必须在提示词里。

**已在打包 EXE 上用真实库验证（2018 个 Mod，只读 + 写临时路径）**：

```
--export-grouping-catalog %TEMP%\lytvpk-catalog-v3check.json
→ exit=0
→ modCount=2018 / schemaRev=2026-09-22.2
→ clusterHints=343（343 条带 memberEntryIds）
→ themeHints=60（60 条带 memberEntryIds）
→ preloadHints=10（10 条带 entryId + memberEntryIds）
→ duplicateGroups=229，其中 9 组标注 singleAddonListKey=true

--validate-group-suggestions %AppData%\LytVPK\group_suggestions.json
→ exit=0，total=189 valid=189 invalid=0 memberCount=1689
   withTags=189 taggedMembers=150 warnings=0
```

其中 `duplicateGroups` 标出的 **9 组**与外部智能体报告的"9 组同键双位置"完全吻合，
说明这一判定与他们的独立分析一致。

**有意不做**：让"同键双位置"参与分组（位置化 key / 按 entryId 分组）。游戏侧只认一个
addonlist 条目，这类副本本质上是同一个 Mod 的两份文件，不存在"二选一"；
应用的做法是明确标注并建议"要二选一请删掉其中一份"，而不是再造一套位置化优先级。

**注意**：CLI 是 GUI 子系统程序，用 `Start-Process -Wait -PassThru` 才能可靠拿到退出码与输出
（直接 `& exe ... | Out-Null` 在部分 PowerShell 调用形式下不会等待）。

**只能人工验证**：材料就绪弹窗里那条黄字时效性提示的观感。

### 第十八轮（v4 建议文件复核 + hint 命名对齐 + disabled 语义写清）

**背景**：外部智能体把旧建议归档（`archive\group_suggestions-v3-…-stale-indices.json`），
并按"文件名/标题 + 主体 + entryId/key"（不再依赖清单序号）重建了 v4：
195 组 / 1761 成员 / `withTags=195`、`taggedMembers=211`。他们留了两条遗留项。

**已在打包 EXE 上复核（真实库，只读 + 写临时路径）**：

```
--validate-group-suggestions %AppData%\LytVPK\group_suggestions.json
→ exit=0，total=195 valid=195 invalid=0 memberCount=1761
   withTags=195 taggedMembers=211 warnings=0

--export-grouping-catalog %TEMP%\lytvpk-catalog-v4check.json
→ exit=0，modCount=2018，generatedAt=2026-09-23T01:57:58
→ preloadHints=10：entryId 10 / entryIds 10 / memberEntryIds 10
→ duplicateGroups=229，其中 singleAddonListKey=true 的有 9 组
```

**两条遗留项的处理**：

1. **`preloadHints` 缺 `entryIds`**：他们用的那份清单（01:50:14）其实已经带
   `entryId` + `memberEntryIds`，缺的是与 `duplicateGroups[].entryIds` 同名的数组写法。
   本轮补上 `entryIds`（长度 1），三种写法齐全，外部推导方可以用同一套字段名处理所有 hint。
2. **root/disabled 同键 9 组"无法二选一"**：本轮把语义写进提示词与文档，而不是去做位置化 key。
   依据（代码里本来就有）：`disabled` 是停用区，游戏不会加载里面的文件 ——
   `health_check.go` 会直接提示「`x` 只存在于 disabled 目录，但 addonlist.txt 仍记录着它：
   游戏不会加载该文件」，`mod_groups.go` 的策略组联动也明确跳过 disabled 副本。
   所以这两份文件里只有一份会被游戏读取，不存在二选一；正确做法是删掉多余的那份。
   文档同时给出替代方案：真要"多版本二选一"，让不同版本用不同文件名即可（它们就是不同的 key，
   会自然进入普通分组建议）。

**注意**：CLI 是 GUI 子系统程序，本轮统一用 `Start-Process -Wait -PassThru` 调用，
才能可靠拿到退出码与输出（详见第十七轮记录）。

**只能人工验证**：在界面上重新点「准备给智能体的材料」后，材料就绪弹窗里的时效性提示观感。

### 第十九轮（l4n 角色包是否被归组的复核 + 提示词补套装规则）

**问题**：用户问"新增的 l4n 角色 Mod（截图里那批 airi / shinano / neko / 油光渲染）有没有成功归为一组"。

**复核方法**：只读取三份数据并交叉比对 —— `%AppData%\LytVPK\grouping_catalog.json`
（或刚导出的新清单）、`%AppData%\LytVPK\group_suggestions.json`（外部智能体的建议）、
以及截图里的文件清单；按 52 个匹配文件逐个统计"进了内置 clusterHints / 进了导入建议 /
仍在 `ungroupedKeys`"。

**结论（52 个 Mod）**：

| 分类 | 数量 | 归属 |
| --- | --- | --- |
| 角色本体 `airi初代恶堕战斗员{8 个幸存者}.vpk` | 8 | 内置「XX 模型」簇 + 导入建议「XX 模型替换合集」（**single 互斥**，语义正确） |
| `shinano维纳斯*` | 8 | 同上（与 airi 版本互为同角色候选） |
| 配件 / 服装 / 材质 `airi初代战斗员*` | 24 | **全部未归组**（`ungroupedKeys`） |
| `neko - 高透明` / `neko-低透明` / `油光渲染*` | 4 | 两个 low 置信度前缀小簇（2+2），**未进导入建议**；`neko.vpk`、`油光渲染.vpk` 本体未归组 |
| `airi初代恶堕战斗员neko/基础材质/油光渲染.vpk`、`[airi]woolywinter毛衣关*` | 8 | **未归组** |

**根因（都在数据里）**：

- 这 24 个配件的 `structure.targets` **全部共享非官方前缀 `airi_evilfall/…`**（部分为
  `913limod/airi_evilfall/…`），这就是"同一套件"的强证据；
- 但内置信号在这批文件上全部失灵：**文件名前缀**对"中文无空格长名"几乎不生效
  （`airi初代战斗员下肢小玩具` 与 `airi初代战斗员臂甲` 首词不同）；**共同标签**要求两个标签相同，
  而这些配件只有 `贴图` 一个标签；**主体识别**结果是"泛材质资源（无法确认具体对象）"（低置信度，
  引擎有意跳过）；**文件夹**信号不适用（都在 `addons` 根目录）；
- 外部智能体的规则是"按 subject 的互斥槽位 + duplicateGroups/themeHints/preloadHints/xdrSlots"，
  **没有"共享自定义资源前缀 → all 套装"这条**，所以配件被整体漏掉。

**本轮已做**：提示词新增必扫项「套装 / 配套模块」（共享非官方 `targets` 片段 ≥3 个 → `all` 组，
并把本体一并纳入，附命名建议）；文档补充对照表与"为什么其它信号会漏"。
提示词自检新增相应断言（`TestGroupSuggestionAgentPromptIsSelfContained`）。

**尚未做（待决定）**：把这条规则也做成**内置引擎信号**（需要给 `grouping.Mod` 增加 targets 字段
并新增 provider：排除官方前缀、≥3 成员、按片段匹配、产出 `all` 候选）。目前内置推导仍抓不到这类套装，
需要外部智能体按新提示词重跑才能补上。

### 第二十轮（内置「套装资源目录」信号：把 l4n 角色包这类套件收成一组）

**背景**：用户指出"l4n 角色配件要靠 VPK 内部文件树的命名来关联"，并要求
① 单个 l4n 角色（替换各幸存者的本体 + 配件 + 贴图）归为一组；
② 武器"贴图包与模型/参数包分开"的情况也要处理。

**调查（真实清单 2018 个 Mod）**：

- 配件 `airi初代战斗员臂甲.vpk` 等的 `samplePaths` 全部落在
  `materials/models/913limod/airi_evilfall/swtich/…`；本体 `airi初代恶堕战斗员coach.vpk`
  则只有 `models/survivors/survivor_coach.*` 这类官方路径 —— 二者**只靠"套件命名空间 + 文件名前缀"
  能关联**；
- 全库统计 `models/<seg1>/<seg2>`：官方根（`weapons` 267 / `survivors` 216 / `w_models` 134 /
  `v_models` 105 / `props*` …）与作者套件根（`913limod` 27 / `codm` 9 / `bzmfs` 8 /
  `star_rail` 8 / `limod` 6 / `sjz` 6 …）能干净区分开；
- 武器侧同样成立：`codm/ice`（`codm冰霜巨龙贴图包` + `ak47/m16 冰龙参数包`）、
  `codm/hyxc`（黑耀星辰贴图包 + 三把枪参数包）、`sjz/k416`（贴图包 + 参数包成对）。

**实现**：

- 解析器 `structureSuiteNamespace()`：在一次遍历里提取 `models/<作者>/<套件>`，官方根、
  单层目录、文件名残留段都被过滤；结果写进 `VPKFile.StructureResourceRoots`（≤6 条）。
- 新内置信号 `suite-namespace`（展示名「套装资源目录」）：同命名空间 ≥3 个模块 → `all` 组；
  再按**文件名公共前缀**（≥4 字符，纯数字前缀丢弃）把本体附着进来；通用套件名
  （weapons / characters / vehicles / texturas … 多语言）与纯数字段在引擎侧二次过滤。
- 清单新增 `structure.resourceRoots` 并列入 `capabilities`；提示词补规则 + 真实例子。

**已在打包 EXE 上验证（真实库 2018 个 Mod，只读 + 临时路径）**：

```
--export-grouping-catalog %TEMP%\lytvpk-catalog-suite2.json   → exit=0
→ clusterHints 里带「套装资源目录」信号的有 26 组 / 139 个成员，置信度全部 medium
→ 最大的：airi初代 套装(35)、!花火零食-2 套装(7)、chscm 套装(7)、nzsg 套装(6)、shinano 套装(6)
→ 武器：ice 套装(5)、hyxc 套装(4)、mao 套装(4)、saber 套装(3 激光剑贴图+武士刀/高尔夫替换)
→ 过滤前曾出现 `weapons 套装(32)`、`characters 套装(12)`、`3788 套装(6)` 三个误报，
  加"通用目录名/纯数字"过滤后消失（`3788*` 回退命名为 `deadoralive 套装`）
```

`airi初代 套装` 的 35 个成员 = 8 个幸存者本体（bill/coach/ellis/francis/louis/nick/rochelle/zoey）
+ 24 个配件（臂甲/大衣/猫耳/目镜/腿环/过滤嘴…）+ 基础材质 + 油光渲染 + neko，
正是用户要的"单个 l4n 角色一整组"。

**自动测试**：解析器 1 项 + 引擎 6 项 + App 端 1 项（清单字段端到端），
`go test ./...` 全绿；`node --test` 147 项；`wails build` / `docs:build` 通过。

**只能人工验证**：在界面上把 `airi初代 套装` 建组后，进游戏确认"8 个幸存者 + 配件一起启用"的实际效果。

### 第二十一轮（按钮点击被伪元素截走 + 套件识别再校准）

**用户报告**：① 分组建议弹窗里"点 mod 选项的按钮有偏移"；② `死库水` 被归进 `shinano 套装`，
而 `shinano` 的 8 个替换角色没进这个组建议。

**① 按钮偏移 = 伪元素吃点击（真实缺陷）**

用临时调试桥读取真实 DOM（验证完已删除该文件并重新构建）：

- 文字与按钮的**纵向中心完全一致**（像素扫描：384/426/468…；DOM 量测：行中心 449、按钮中心 449），
  所以不是"错位"；
- 对弹窗内 191 个可交互元素做命中测试，发现 85 处"看到的元素 ≠ 命中的元素"：
  横向扫描 x=1100…1330 显示 **「游戏开关」的命中区间从 x≈1100 就开始**，而它的可视框在 1175..1258，
  「详情」（1127..1170）完全点不到；
- 根因：`frontend/src/css/app/base.css` 的全局按钮扫光
  `.btn::before { content:""; position:absolute; left:-100%; width:100%; height:100% }`
  **没有 `pointer-events: none`**，它停在按钮左侧、与按钮同宽 → 吃掉左边按钮的点击。

修复：给 `.btn::before` 加 `pointer-events:none`；并用测试扫描全仓，把同类隐患一并修掉
（`.select-trigger::after`、两个 `input:checked::after`、`.file-item::before`、
`.file-checkbox:checked::before`、`.spray-select-trigger::after`）。
修复后重新用桥验证：每行三个按钮在自己的中心都命中自己（`详情→✓自己 / 游戏开关→✓自己 / 禁用→✓自己`），
`::before` 的 `pointer-events` 为 `none`。新增守卫测试 `core/button-pseudo-hit-test.test.mjs`。

**② 套件识别校准**

数据核对：`死库水校服基础材质.vpk` 的内部路径是 `materials/sikushui/mo/…`、
`死库水校服外套透明.vpk` 是 `materials/qkl/mo/sikushui/…`、`死库水校服-教练.vpk`（本体）只有官方路径；
而 `shinano` 的 8 个本体（`shinano维纳斯bill.vpk` 等）只替换官方目标、没有 `resourceRoots`。
旧规则只认 `models/<作者>/<套件>` 且只用"文件名公共前缀"附着本体，所以两头都错。

改进（全部有测试）：

- `materials` 分支：没有 `models` 段时取"第一个 ≥4 字符的非通用目录段"作候选（两处都收敛到 `sikushui`），
  且**单段候选必须同时有 ≥3 字符的文件名公共前缀**才成组（防"零散目录拼凑"）；
- 本体附着改用**两条线索**：文件名公共前缀（≥3 字符）或**套件名关键词**（≥6 字符，出现在文件名开头）——
  `shinano` 因此把 8 个本体收进来；
- 排除地图/临时容器（`static`、`tmp_mod`、`graffiti`、`brick`）与通用子目录名（`props`/`textures`/`vgui`…）；
- 提示词明确"**绝对不要跨套件合并**"，并写入两条线索与 `死库水` 的反例。

**真实库复核（2018 个 Mod，打包 EXE 只读导出）**：

```
套件组 34 个 / 成员 202 个（过滤前曾出现 101 组 / 717 成员，含 weapons/characters/3788/xx_p_codm/nmrih 等误报）
airi初代 套装   35（8 本体 + 24 配件 + 基础材质/油光渲染/neko）
shinano 套装    14（6 配件 + 8 本体）           ← 用户问题 ② 解决
死库水 套装      4（3 材质 + 本体，独立成包）    ← 用户问题 ① 解决
ice 套装 5 / hyxc 套装 4 / Chiffon 下午茶 套装 24 / !花火零食-2 套装 7 / !!医疗箱 套装 6
```

**说明**：截图里那条「shinano 套装（本体+配件一起启用）」来自**外部智能体导入的建议文件**，
把 `死库水` 并进 `shinano` 是他们的合并错误；内置推导现在给出的是正确的两组
（`shinano 套装` 与 `死库水 套装`），提示词也已禁止跨套件合并 —— 让智能体重跑一次即可得到同样的结果。

**只能人工验证**：在界面上点「详情 / 游戏开关 / 禁用」三个按钮的体感（自动化侧已验证命中自身）。

### 第二十二轮（策略组批量管理 + 一次被用户数据核对的"组消失"排查）

**用户报告**：① "之前放进去的组是失效了吗"（截图是「分组」筛选菜单）；
② 希望"多选组之后批量管理，比如批量删除"。

**① 排查结论（基于 groups.json 与备份轮转）**：

| 时间 | 事实 |
| --- | --- |
| 03:51:16 | `backups\groups.backup_2026-09-23_03-51.json` 存有 **17 个组**（备份是写盘前生成的） |
| 03:5x | 只读核对：当时 9 个组、**0 个成员匹配不到文件**（逐个 key 对清单），所以"组本身没失效" |
| 03:56:15 | `groups.json` 被写成 **4 个组** |
| 04:00 以后 | 本轮的验收探针才启动（只勾选 + 打开确认框后点取消），不可能造成 03:56 的写入 |

也就是说：组记录一直有效（成员都能匹配到文件），03:56 那次减少发生在验收探针之前；
**全部 17 个组都能从 `%AppData%\LytVPK\backups\groups.backup_2026-09-23_03-51.json` 恢复**
（备份轮转是"写盘前备份 + 最短间隔 + 内容相同跳过"）。

**② 批量管理（已实现）**：

- 设置页「策略组」列表：每个组前有勾选框 + 工具条（全选 / 已选 N 个组（M 个成员…））；
  批量动作：**批量删除**、开启/关闭自动联动、设置权重、清除权重；
  批量删除走应用内确认框（列出组名与影响范围），选中集合存在 `appState` 里、跨渲染保留。
- 后端 `BatchUpdateModStrategyGroups(ids, action, tier)`：一次加锁一次写盘、去重保序、
  未知 ID 计入 `skipped`、无改动不写盘、批量删除会把被删组的下级提升到顶层。
- Go 测试 5 项；前端纯函数测试 4 项（选择摘要 / 文案 / 提示）+ 结构测试 5 项。

**在打包 EXE 上驱动验证**（临时调试桥读真实 DOM，验证后已删除并重新构建）：

```
设置页正常加载（loadFailed=false）
策略组工具条存在；勾选 2 个组 → 「已选 2 个组（14 个成员）」，批量删除按钮由禁用变可用，
「全选」显示 indeterminate
点「批量删除」→ 确认框出现，正文为：
  「删除选中的 2 个策略组（共 14 个成员）：只删除 groups.json 里的记录，不改动 addonlist.txt、
    也不删除任何 Mod 文件；组成员已写入的分层（priority.json）会保留；被删组的下级分组会回到顶层。
    【牢本】户外厕所 系列…、【BA垃姬桶】垃圾桶系列…」
→ 点「取消」关闭，未执行删除（groups.json 未被改动）
```

**本轮修复的自身回归**：批量代码最初在 bind 里调用了 render 闭包内的
`missingMemberNamesForGroup()`，把整个设置页打成「设置页面加载失败：… is not defined」——
在真机上发现并改为 render 阶段预计算传参，同时加了禁止此类跨作用域调用的结构测试。

**只能人工验证**：批量删除确认后真正执行一次（本轮只验证到"确认框内容 + 取消"）。

## 第五轮调试：策略组管理改成独立窗口（2026-09-23，打包 EXE + 真实 2018 个 Mod 数据）

**用户反馈**：编辑策略组生命周期的界面不该塞在设置页里，应该做成一个窗口。

**改动**（详见 `CHANGELOG.md` 顶部条目）：

- 新增 `frontend/src/js/features/mod-groups/strategy-group-manager.js` + `#strategy-group-modal`：
  建组 / 按策略应用 / 随机单选 / 全关 / 重命名 / 删除 / 上级分组 / 组权重 / 自动联动，
  以及多选批量管理（批量删除、批量开关自动联动、批量设置·清除权重）；
- 设置页原来的整块策略组卡片被删掉，只留一行说明 +「打开策略组管理窗口…」按钮；
- Mod 管理页「分组」菜单新增「策略组管理…」入口；
- 勾选变化（`state.js` 新增 `onFileSelectionChanged`）与组归属变化（`onGroupMembershipChanged`）
  会实时同步窗口内容。

**在打包 EXE 上驱动验证**（临时调试桥 `internal/app/cua_bridge_debug.go` + `LYTVPK_CUA_BRIDGE=1`，
内部用 `WindowExecJS` 求值、页面 `fetch` 回传结果；验证完已删除桥接文件并重新执行 `wails build`，
确认 `frontend/wailsjs/go/**` 里不再含 `MaybeStartCuaBridge`）：

```
设置页：loadFailed=false（无「设置页面加载失败」）；
        旧管理界面已消失（hasOldCapture=false、hasOldBatch=false）；
        新的指针卡片存在（#settings-open-strategy-manager，rect=438,454,144,32）
窗口打开：visible=true，尺寸 1100x702，resize handles=8，行数=4，
          batchHidden=false，selection=「未选择组」，captureDisabled=true
多选 2 个组：selection=「已选 2 个组（14 个成员）」，批量删除按钮由禁用变可用；
          点「批量删除」→ 确认框正文正确（只删 groups.json 记录 / 不动 addonlist.txt /
          不删 Mod 文件 / 分层保留 / 下级回到顶层），点「取消」后未写盘
全选同步：勾 1 个组 → 全选框 indeterminate=true（「已选 1 个组（11 个成员）」）；
          点「全选」→ checked=true（「已选 4 个组（25 个成员）」）；
          取消 1 个 → 回到 indeterminate（「已选 3 个组（14 个成员）」）；清空 → 「未选择组」
单行「删除」：确认框正文列出该组 11 个成员，取消后 groups.json 未变
勾选联动：点一个 Mod 的复选框 → 窗口按钮实时变成「用选中的 1 个 Mod 建组」（disabled=false）；
          取消勾选 → 回到「用选中的 Mod 建组」（disabled=true）
窗口缩放：拖动右下角把手 1100x702 → 1320x842，缩回后 1100x702（PointerEvent 真实驱动 modal-resizer）
ESC 关闭：true；再从「分组」菜单点「策略组管理…」重新打开：true（菜单宽 420px，
          按钮中心命中自身：#mod-group-manager-btn）
错误收集：window.__cuaErrors=0（含 error / unhandledrejection / console.error）
```

**未写盘的证据**：本轮全部结束时 `%AppData%\LytVPK\groups.json` 的时间戳仍是 `03:56:15`、
SHA256 仍是 `32CDA649…7BDBA92`；`priority.json`（09-22 21:21）与游戏侧 `addonlist.txt`（09-23 00:24）
也未被本轮触碰。

**已自动覆盖**：窗口结构、批量动作与后端接线、设置页回归守卫（不能再出现策略组管理界面）、
入口接线、勾选/归属变化驱动刷新（`node --test` → `features/mod-groups/strategy-group-batch.test.mjs` 9 项，
`node --test` 全仓 162 项全绿）。

**只能人工验证**：

- 组窗口在当前 DPI / 分辨率下拖动缩放的"手感"，以及拉大后多列排布是否顺手；
- 真正执行一次批量删除 / 组权重写入后，游戏内加载顺序是否符合预期（沿用第 4 节"真实游戏内覆盖方向"）。

## 第六轮调试：组选择器（加入 / 移出 / 移动）便捷化（2026-09-23，打包 EXE + 沙箱数据目录）

**用户反馈**：Mod 界面里点"组选择"按钮后出现的界面还可以再优化，要更便捷。

**改动**（详见 `CHANGELOG.md` 顶部条目）：选择器新增自动聚焦的**搜索框**（组名 / 上级分组 / 策略名）、
**「只看相关」**（移出模式默认开）、**智能排序**（可点优先 → 含选中项优先 → 名称）、
**行详情标签**（策略 / 成员数 / 权重 / 自动联动 / 上级分组）、**键盘 ↑↓ Enter Esc**、
**双击直接执行**；窗口加宽到 `min(760px, 94vw)` 并补键盘提示。

**验证方式（本轮新增）**：为了能安全地自动化"真的写盘"的动作，用 **`AppData` 环境变量把数据目录重定向到沙箱**
（`AppData=<repo>\.tmp-cua\sandbox\Roaming`），沙箱里放 3 个策略组 + 4 个**用 `genvpks` 生成的真实 VPK**；
因此加入 / 移出 / 移动全部在沙箱 `groups.json` 上真实执行，**用户自己的 `%AppData%\LytVPK` 全程只读**。

**在打包 EXE 上驱动验证**（临时调试桥 + 真实右键菜单 → 选择器）：

```
打开（ddd角色.vpk 右键 →「加入策略组…」）：visible=true，focus=group-picker-search，
  summary=「已选中 1 个 Mod；下面 3 个策略组里 3 个可以加入」，visible=「显示 3 / 3 个组 · 3 个可操作」，
  confirmDisabled=true（不预选任何行）
行标签：测试角色组 → [全部开启, 权重 2, 自动联动, 上级：测试父组]；测试武器组 → [互斥单选]
搜索「角色」→ 1 行、仍聚焦搜索框、可见行提示「显示 1 / 3 个组 · 1 个可操作 · 已按筛选隐藏 2 个」
搜索「不存在」→ 空状态「没有匹配「不存在」的策略组」
键盘 ↓ ↓ ↑ → 高亮 测试父组 → 测试角色组 → 回 测试父组（与排序一致）
搜索「测试角色组」+ ↓ + Enter → 关闭弹窗；沙箱 groups.json：测试角色组 = ccc角色.vpk,ddd角色.vpk ✅
「移动到其它策略组…」+ 双击 测试父组 → 沙箱 groups.json：测试角色组=ddd角色.vpk、测试父组=ccc角色.vpk ✅
「从策略组移出…」→ relevantChecked=true、只剩「测试武器组」；双击该行 → 测试武器组=bbb武器.vpk ✅
守卫：唯一成员的组 → 行禁用，详情「1 个成员 · 选中的 Mod 都在这个组里 · 会把该组成员全部移出
  （组至少要保留 1 个成员）」，↓+Enter 不写盘（弹窗保持打开）✅
错误收集：errors=[]（含 error / unhandledrejection / console.error）
```

**用户数据未被触碰**：全程 `%AppData%\LytVPK\groups.json` 的时间戳仍是 `03:56:15`、
SHA256 仍是 `32CDA649…7BDBA92`（所有写入都落在沙箱目录）。

**已自动覆盖**：`node --test` → `features/mod-groups/group-picker-view.test.mjs`（12 项：搜索、只看相关、
排序、键盘落点、行详情/标签、文案）与 `group-picker.test.mjs`（7 项：结构 id、入口接线、便捷性接线、
样式约束），全仓 168 项全绿。

**只能人工验证**：真实鼠标拖动/滚动的手感，以及你自己库里几十个组时的搜索习惯是否顺手。

## 第七轮调试：策略组管理窗口「展开成员」直接配 Mod 选项（2026-09-23，打包 EXE + 沙箱数据目录）

**用户反馈**：在策略组管理里也应该能随时配置 Mod 选项，跟「分类建议」按钮进去的那个窗口一样展开看详情。

**改动**（详见 `CHANGELOG.md` 顶部条目）：

- 新增共享成员行 `frontend/src/js/features/mod-groups/member-row.js`（详情 / 游戏开关 / 启用·禁用 /
  复制到 addons，位置与游戏开关徽标、禁用条件都复用 `describeSuggestionMember`），
  **「分组建议」卡片与「策略组管理」窗口共用同一份实现**（建议卡片原来的 DOM 拼装代码已删除）；
- 管理窗口每个组行新增「展开成员（N）/ 收起成员」，展开后逐行显示成员并可 **移出本组**；
  展开状态存在 `appState.strategyGroupExpanded`。

**在打包 EXE 上驱动验证**（沙箱 `AppData` + 4 个真实 VPK + 3 个组；调试桥验证后已删除并重建）：

```
组行按钮：3 个组都显示「展开成员（1）」；点开后变为「收起成员」，成员容器可见
成员行：ddd角色.vpk / ddd角色.vpk，徽标 [根目录, 游戏开关：未记录]，
        优先级「未写入 addonlist」，按钮 [详情, 游戏开关 未记录, 禁用, 移出本组]
详情：点「详情」→ 详情弹窗打开，标题 ddd角色.vpk
游戏开关：点「游戏开关 未记录」→「未记录 Mod：选择状态」→ 选「游戏内启用」→
        「启用游戏内 Mod 风险提示」（沙箱 VPK 缺 addoninfo.txt）→ 点「继续操作」→
        沙箱 addons\addonlist.txt 被写入（38 字节），成员行徽标变为 [根目录, 游戏开关：开]、
        优先级变为「优先级 #1」
移出本组：先用选择器把 aaa武器.vpk 加进「测试武器组」（成员 2 个），
        展开后点 aaa 行的「移出本组」→ 沙箱 groups.json 回到 bbb武器.vpk 一个成员，
        窗口保持展开状态
布局：成员行 actions 与行右边界 -8px（= padding），scrollWidth == clientWidth，无横向溢出
错误收集：errors=[]（含 error / unhandledrejection / console.error）
```

**用户数据未被触碰**：全程只有沙箱目录被写；`%AppData%\LytVPK\groups.json` 时间戳仍是 `03:56:15`、
SHA256 仍是 `32CDA649…7BDBA92`。

**已自动覆盖**：`node --test` → `member-row.test.mjs`（共享行契约）+ `strategy-group-batch.test.mjs`
（展开接线、展开状态、移出本组），全仓 172 项全绿；`go test -count=1 ./...`、`npm run build`、
`wails build`、`docs:build` 通过。

**只能人工验证**：你库里几十个成员的大组展开后的滚动体验；真实游戏内加载顺序（沿用第 4 节）。

## 第八轮调试：筛选顺序按组权重 + 管理窗口内直接筛选 + 浮动窗口（2026-09-23，打包 EXE + 沙箱）

**用户问题**：管理窗口里的「上级分组（顶层）」跟外面的「按分组筛选」什么关系？筛选顺序为什么固定不变？
能不能在管理窗口里就把某些组变成筛选？

**改动**（详见 `CHANGELOG.md` 顶部条目）：

- 「按分组筛选」顺序改为 **组权重升序**（未设置权重排最后、同权重按名称），子组用 `└` 缩进但层级不参与排序；
  后端 `ModGroupMembership` 透出 `parentId` 供缩进使用；
- 管理窗口内新增 **组行「筛选这组 / 取消筛选」** + 批量 **「用选中的组筛选 / 清除筛选」** +
  **「当前筛选：A、B」** 文案与高亮；
- 管理窗口默认 **浮动模式**（不挡主界面，标题栏可拖动；开关写入 `config.json`）。

**在打包 EXE 上驱动验证**（沙箱 3 个组：测试武器组 tier=-5 / 测试角色组 tier=2 且 parent=测试父组 / 测试父组未设权重）：

```
筛选菜单顺序（管理器窗口里设的权重直接生效，子组用 └ 标层级）：
  测试武器组（1） · 权重 -5 → └ 测试角色组（1） · 权重 2 → 测试父组（1）
  悬停：互斥单选 · 共 1 个成员 · 组权重 -5 / … · 未设置组权重
  说明行：顺序＝组权重（未设置排最后）· 子组用 └ 标出层级
浮动窗口：is-floating=true、容器 pointer-events=none、窗口本体 auto，
  默认位置 (660,172) 尺寸 716×680（工具栏下方 + 分组菜单右侧），
  该位置下 elementFromPoint 命中 #mod-group-filter-text（按钮没被挡）
拖动标题栏：(508,72) → (248,162)（PointerEvent 真实拖动）；
  关掉浮动开关 → 变回模态（pointer-events=auto），点窗口外可关闭
窗口内筛选（断言都读真实 DOM 与主列表）：
  打开时：「当前筛选：全部分组」、清除筛选禁用、列表 4 个 Mod、每行按钮「筛选这组」
  单组：点「筛选这组」→「当前筛选：测试角色组」、列表 1 个 Mod、分组按钮变「测试角色组」、
        该行按钮变「取消筛选」且整行高亮、清除筛选可用
  清除：「当前筛选：全部分组」、列表回到 4 个 Mod、分组按钮回到「全部分组」
  多选：勾 2 个组 → 「已选 2 个组（2 个成员）」→ 点「用选中的组筛选」→
        「当前筛选：测试武器组、测试角色组」、列表 2 个 Mod、分组按钮「2 个分组」、两行都高亮
错误收集：errors=[]（error / unhandledrejection / console.error）
```

**用户数据未被触碰**：全部写入都落在沙箱 `AppData`；你真实库里的 `groups.json`（18 个组）
只被你自己编辑过，本轮没有任何工具侧写入。

**已自动覆盖**：`node --test` 181 项（`group-view` 层级/权重排序 4 项、`strategy-group-format`
筛选文案、菜单布局与窗口筛选接线）；`go test -count=1 ./...`、`npm run build`、`wails build`、
`docs:build` 通过。

**只能人工验证**：浮动窗口在你屏幕分辨率下的默认位置是否顺手（可拖动 / 可缩放）；
真实游戏内加载顺序（沿用第 4 节）。

## 第九轮调试：浮动窗口通用化 + 背景不再虚化（2026-09-23，打包 EXE + 沙箱）

**用户反馈**：浮动窗口再优化优化；打开管理窗口后主界面变模糊了看不见；其它复杂管理窗口
是不是也应该可浮动、可随便拉伸、背景不虚化？

**根因**：遮罩模糊来自 `.modal` 自身的 `backdrop-filter: blur(8px)`（`app/modals-details.css`）。
上一轮浮动模式只把 `background` 设成透明，**没有关掉 blur** —— 所以主界面仍然糊。

**改动**（详见 `CHANGELOG.md` 顶部条目）：新增通用模块 `frontend/src/js/core/floating-modal.js`
（浮动/停靠、自动插开关按钮、标题栏拖动、位置记忆、打开时自动应用），
`.modal.is-floating` 同时关掉 `backdrop-filter` 与遮罩色并放行鼠标事件；
`分组建议 / 加载顺序优化 / Mod冲突检测 / 文件冲突 / 体检 / 模型统计` 默认浮动，
策略组管理窗口沿用 `config.json` 的开关（其自带的浮动实现已删除，收敛到通用模块）。

**在打包 EXE 上驱动验证**（沙箱数据；调试桥验证后已删除并重建）：

```
策略组管理：floating=true、容器 backdrop-filter=none、background=rgba(0,0,0,0)、
  pointer-events=none；窗口本体 auto；elementFromPoint 命中 #mod-group-filter-btn（主界面可点）；
  拖动标题栏 (580,220) → (500,220)
分组建议：默认 floating=true，标题栏自动出现「停靠窗口」(is-active)；
  容器 backdrop-filter=none / 透明 / pointer-events=none；
  点「停靠」→ floating=false、backdrop-filter=blur(8px)、底色 rgba(15,23,42,0.5)（恢复遮罩）；
  再点「浮动窗口」→ 回到浮动且主界面仍可点
加载顺序优化（标题栏是 .load-order-header，需要兜底选择器）：
  默认 floating=true，标题栏里出现「停靠窗口」；容器无虚化、pointer-events=none；
  拖动标题栏成功；点「停靠」→ blur(8px) 恢复；再点 → 浮动恢复
错误收集：errors=[]
```

**用户数据未被触碰**：全部验证在沙箱 `AppData`；你真实库 `groups.json` 时间戳/哈希未变。

**已自动覆盖**：`node --test` 184 项（新增 `core/floating-modal.test.mjs` 3 项：虚化是否真的关闭、
模块能力、六个窗口是否都接上；并更新了菜单布局与策略组窗口的浮动断言）。

**只能人工验证**：各管理窗口浮动后在你屏幕上的默认落点（可拖动 / 可缩放）；真实游戏内加载顺序。

## 第十轮调试：修复「停靠后浮动按钮点不到」（2026-09-23，打包 EXE + 沙箱）

**用户反馈**：有的窗口点「停靠窗口」后，切回浮动的按钮被挤到边上点不到了。

**根因**：浮动时设了内联 `max-width/max-height: none`，停靠时用
`style.removeProperty("maxWidth")` 清理 —— **`removeProperty` 只认连字符写法**，驼峰静默失败，
`max-height: none` 残留下来；「加载顺序优化」内容本来就高，停靠后撑到 **1901px**，
flex 居中把它顶成 `top=-500`，标题栏连同按钮跑出视口。

**修复**：`core/floating-modal.js` 改用连字符属性名清理，并加"内容仍高于视口就顶部对齐"的兜底。

**在打包 EXE 上驱动验证**（沙箱；逐个窗口在浮动态 / 停靠态各测一次按钮几何与命中）：

```
修复前 load-order：停靠后 contentRect=[55,-500,1280,1901]，按钮 hitSelf=false（点不到）
修复后 load-order：浮动态 [0,12,1280,888] 按钮 hitSelf=true；
                  停靠态 [55,6,1280,888] 按钮 insideContent=true、hitSelf=true；
                  再切回浮动 → [0,12,1280,888] 正常
策略组管理：停靠后 inline=""（内联样式已清空）、backdrop-filter=blur(8px) 恢复、
          勾选框 label 仍在窗口内且 hitSelf=true；再浮动 → 回到记忆位置 (500,220)，无虚化
其余 5 个窗口（分组建议 / Mod冲突检测 / 文件冲突 / 体检 / 模型统计）：
          浮动态与停靠态按钮都在 content 矩形内且 hitSelf=true
```

**用户数据未被触碰**：全部在沙箱 `AppData`；真实库 `groups.json` 哈希未变。

**已自动覆盖**：`node --test` → `core/floating-modal.test.mjs` 新增 1 项回归（清理必须用连字符属性名、
不得出现驼峰、必须有顶部对齐兜底），全仓 185 项。

**只能人工验证**：你自己常用窗口的停靠/浮动手感；真实游戏内加载顺序。

## 第十一轮调试：停靠状态点窗口外统一可关闭（2026-09-23，打包 EXE + 沙箱）

**用户反馈**：有的窗口在停靠状态下点窗口外还是没有关闭。

**根因**：「点窗口外关闭」以前是每个窗口在 `app-runtime.js` 里各写一段处理器，
后接入浮动能力的那批窗口没有，于是停靠（模态）状态下点遮罩没有任何反应。

**修复**：收进 `core/floating-modal.js` 统一提供 —— 停靠时遮罩上的 `mousedown`
（`event.target === modal`）会去点**该窗口自己的关闭按钮** `.close-btn`，
各窗口的收尾逻辑照常执行；浮动状态直接放行（容器 `pointer-events: none`）。
`问题 Mod 查找` 显式 `closeOnBackdrop: false`（它会改 Mod 开关，设计上要保持打开）；
策略组管理窗口自带的那份已删除。

**在打包 EXE 上驱动验证**（沙箱；每个窗口先停靠，再向遮罩派发真实 mousedown）：

```
strategy-group-modal     停靠 → 点外面 → hidden=true  ✅
mod-group-suggest-modal  停靠 → 点外面 → hidden=true  ✅
load-order-modal         停靠 → 点外面 → hidden=true  ✅
conflict-modal           停靠 → 点外面 → hidden=true  ✅
file-conflict-modal      停靠 → 点外面 → hidden=true，且确认走的是窗口自己的清理
                         （该窗口关闭按钮只在真实弹窗流程里用 onclick 绑定，验证时复刻了这条绑定）
model-stats-modal        停靠 → 点外面 → hidden=true  ✅
problem-scan-modal       停靠 → 点外面 → 保持打开（设计如此：查找模式进行中不允许关闭）
```

**用户数据未被触碰**：全部在沙箱 `AppData`；真实库 `groups.json` 哈希未变。

**已自动覆盖**：`node --test` 186 项（`core/floating-modal.test.mjs` 新增"停靠点外关闭"契约：
统一提供、复用窗口关闭按钮、只在停靠态生效、问题查找例外、策略组窗口不再自带）。

**只能人工验证**：各窗口关闭时的收尾行为是否符合你的预期（尤其是排查类窗口）；真实游戏内加载顺序。

## 第十二轮调试：策略组窗口统一用通用浮动开关（2026-09-23，打包 EXE + 沙箱）

**用户反馈**：策略组管理窗口里还在用老的勾选框行，应该跟其它窗口用同一套通用设计。

**改动**：删掉窗口内自定义的「浮动窗口（可直接操作主界面）」勾选框与拖动提示
（`index.html` 的 `.strategy-group-toolbar`、`mods.css` 里对应死样式），
改由 `core/floating-modal.js` 在标题栏自动插入「浮动窗口 / 停靠窗口」按钮。
同时修掉一个真问题：偏好写进 `config.json` 时被 Go 端丢弃
（`ConfigFile` 缺 `strategyGroupFloating` 字段），现在补上 `*bool` 并在
`loadConfig` / `SaveAppConfig` / `snapshotConfig` 三处接线。

**在打包 EXE 上驱动验证**（沙箱）：

```
标题栏子元素：[H3, modal-float-toggle is-active, close-btn]；自定义勾选框行 customRow=false
点「停靠窗口」：is-floating=false、backdrop-filter=blur(8px)、按钮文字「浮动窗口」
            → 沙箱 config.json 写入 strategyGroupFloating = False
重启应用后再打开策略组窗口：afterRestartFloating=false（保持上次的停靠状态）✅
点回「浮动窗口」：is-floating=true、无虚化，config.json 变回 True
errors=[]
```

**用户数据未被触碰**：全部在沙箱 `AppData`；真实库 `groups.json` 哈希未变。

**已自动覆盖**：Go `TestSaveAppConfigPersistsStrategyGroupFloating`（保存后重新读盘仍为停靠）；
前端 `node --test` 186 项（不再有自定义勾选框、按钮由通用模块插入、偏好写回 config、
停止态点外关闭、虚化关闭等契约）。

**只能人工验证**：你屏幕上的默认落点与手感；真实游戏内加载顺序。

## 第十三轮调试：修正「按分组筛选」排序（子树整体移动）+ 保存后立即生效（2026-09-23）

**用户反馈**：截图里「按分组筛选」的排序对吗？

**结论：不对**。上一版是"完全按权重、层级不参与排序"，子组只按名字排；
你数据里「医疗箱」的 `【BA垃姬桶】`、`【牢本】户外厕所` 因为 `【` 前缀被甩到列表最底部，
跟父组、兄弟组脱开。

**修复**（`frontend/src/js/features/mod-groups/group-view.mjs`）：
① 权重升序；② **子组紧跟上级分组**（整棵树一起移动）；③ 子树位置取**子树内最小权重**
（给子组设权重，整棵树一起上浮，子组仍缩进）；④ 成环/缺父按顶层处理，不丢组。
同时修复：管理窗口里「保存权重 / 改上级分组 / 重命名 / 自动联动」以前不刷新外部列表
（要手动点工具栏「刷新」），现在都会 `refreshAfterChange()`。

**在打包 EXE 上驱动验证**（沙箱数据复刻你的结构：`!!医疗箱` + `!花火零食` / `【BA垃姬桶】` / `【牢本】户外厕所` + `Chiffon 下午茶`）：

```
基线（都未设权重）：!!医疗箱 → └ !花火零食 → └ 【BA垃姬桶】 → └ 【牢本】户外厕所 → Chiffon 下午茶
                   （两个【…】子组紧跟父组，修复前会沉到列表最底）
顶层 Chiffon 设 -5：Chiffon（权重 -5）排最前，医疗箱整棵树在后 → 保存后菜单立即更新（无手动刷新）
子组 【BA垃姬桶】设 -9：整棵「医疗箱」子树抬到最前，该子组在子树内第一且仍缩进；Chiffon(-5) 落到后面
清空权重：回到基线
errors=[]
```

**用户数据未被触碰**：全部写入落在沙箱 `AppData`（你自己的 `groups.json` 是你自己在改）。

**已自动覆盖**：`node --test` 188 项（子树整体移动、名字前缀不同不再拆散、
窗口改组的刷新回归守卫、成环降级）。

**只能人工验证**：这套顺序是否符合你"常用组放最上面"的使用习惯；真实游戏内加载顺序。

## 第十四轮调试：`[Milfy]白银审判…` 为什么没进建议（2026-09-23，打包 EXE + 沙箱）

**用户问题**：`[Milfy]白银审判…`（角色本体 + 上衣关 + 内衣关）明明是一套，为什么没进分组建议？

**结论：两个原因**

1. **被"标签已覆盖"筛选藏了**：`[Milfy]` 正好精确覆盖这一批 Mod，而分组建议弹窗的
   「标签已覆盖」下拉默认 `隐藏标签已覆盖`（`suggestion-filter.mjs` 的
   `DEFAULT_SUGGESTION_FILTER.tagCovered = "hide"`），摘要行只提示
   `N 条标签已覆盖（可直接用标签筛选）`。切成「显示全部」就能看到，卡片上有「用标签筛选」。
2. **引擎确实漏了这种命名**：旧 `NamePrefix()` 要求文件名至少两个词（`_` / 空格 / `-` 分隔），
   `[Milfy]白银审判里内衣关.vpk` 只有一个词 → 返回空；`[Milfy]白银审判_Rochelle.vpk`
   的键是 `[milfy]白银审判`（另一个桶），于是整条系列一条建议都出不来。

**修复**（`internal/grouping/grouping.go` + `index.go` + `providers.go`）：
新增 `NamePrefixKeys()`（去掉 `[标签]`/`【标签】` 后按 4~12 字生成系列前缀键，
纯数字键排除），`filename-prefix` 信号改为取"极大成员集合"（成员多的桶优先，
跳过被完全覆盖的子前缀）。

**在打包 EXE 上驱动验证**（沙箱里造出这 4 个 VPK）：

```
分组建议（标签已覆盖=显示全部，重新推导）：
  summary = 共 1 条建议 · 1 条标签已覆盖（可直接用标签筛选） · …
  卡片   = 白银审判 · 低置信度 · 4 个 Mod · 文件名前缀相同：白银审判
           成员 [Milfy]白银审判_Rochelle.vpk / 上衣关 / 内衣关 ×2
           徽标「标签已覆盖：Milfy」+「用标签筛选」
修复前：这 4 个 Mod 一条建议都推不出来（单元测试先复现为 0 条）
errors=[]
```

**已自动覆盖**：`TestFilenamePrefixGroupsSeriesWithoutSeparators`、
`TestFilenamePrefixIgnoresBracketTagAsSeries`（反向保护：只共享 `[标签]` 的不同套件不能被凑一起）。

**只能人工验证**：你真实库里这类系列名（例如 `死库水` 这种 3 字名）是否也符合预期——
3 字以内的中文系列名仍走原有的套件目录 / 合集 / 属性信号，不会被前缀信号接管。

## 第十五轮调试：多选批量「游戏内启用 / 关闭」（2026-09-23，打包 EXE + 沙箱）

**用户反馈**：多选之后不能批量游戏内启用；「批量启用」按钮是不是该分成两个？

**结论**：确实缺这一组能力，而且**应该分成两组**——文件级与游戏内级语义完全不同：

| 按钮 | 作用 | 是否动文件 |
| --- | --- | --- |
| 游戏内启用 / 游戏内关闭（新增） | 写 `addonlist.txt` 的 `0/1` | 不动 |
| 批量启用 / 批量禁用（原有） | 在 `addons` 与 `disabled` 目录之间搬文件 | 动文件 |

**实现**：Go 新增 `SetVPKGameEnabledBatch(paths, enabled)`（一次加锁、**一次写盘**，
返回 `requested/updated/unchanged/skipped/enforced`）；前端新增两个按钮 + 应用内确认 +
结果文案（`batch-game-state-format.mjs`）。

**在打包 EXE 上驱动验证**（沙箱，8 个真实 VPK）：

```
多选 3 个（selected-files = 已选择: 3）
点「游戏内启用」→ 确认框：把选中的 3 个 Mod 设为游戏内启用？
                  · 只改 addonlist.txt 的 0/1，不移动、不删除任何文件
                  · 其中 2 个还没记录在 addonlist.txt，会按「未记录 Mod 插入位置」设置写入
  → 列表立刻变成 游戏内开|游戏内开|游戏内开|游戏内关×5
点「游戏内关闭」→ 确认框 → 8 个全部变成 游戏内关
沙箱 addonlist.txt（批量在 addons 根目录生成）：
  "ddd角色.vpk" "0" / "[Milfy]白银审判_Rochelle.vpk" "0" / "[Milfy]白银审判里内衣关-2.vpk" "0"
  （两个原本未记录的条目被正确插入）
errors=[]
```

**用户数据未被触碰**：全部写入落在沙箱；真实库 `groups.json` 只读。

**已自动覆盖**：Go 2 项（去重/unchanged/disabled 跳过/反向批量、未记录条目插入）；
前端 `node --test` 194 项（确认文案、结果文案、按钮存在与绑定、选择相关启停清单）。

**调试环境记录（供后续轮次参考）**：本轮发现临时调试桥原来固定监听 `127.0.0.1:9223`，
会被别的程序（本次是 WPS）占用为**客户端临时端口**，`bind` 直接失败
`An attempt was made to access a socket in a way forbidden by its access permissions`，
表现为"桥突然不可用"。已改成**端口 0 动态分配**并把真实端口写进
`%TEMP%\lytvpk-cua-bridge.log`，验证脚本从日志读端口。

**只能人工验证**：你自己在真实库上跑一次批量游戏内开关（尤其确认 300 个以上的大选择是否够快）。

## 第十六轮调试：策略组管理窗口的「搜索框」与禁用状态（2026-09-24，打包 EXE + 沙箱）

**用户反馈**：窗口里那个框（`策略组名称，例如 角色替换包`）是搜索框吗？能不能把窗口里这些灰掉的
控件弄成正常？

**结论**：那个框是**新建策略组的名称输入**（不是搜索框），而且窗口里压根没有搜索；
灰掉的批量按钮是"没勾选组"的正常禁用，但界面没说清原因，看起来像坏了。两件事都已改：

1. 拆成两个分区：`新建策略组：…`（名称 + 策略 + 建组按钮）与
   `搜索策略组（组名 / 成员名 / 描述）…`（真正的搜索框 + `显示 X / Y 个组`）；
   「全选」只作用于当前搜索结果。
2. 禁用状态自解释：工具条新增提示行（未勾选 → 怎么勾；已勾选 → 绿色"按钮已可用"），
   每个禁用按钮 tooltip 写明原因（勾选后恢复原 tooltip）；
   「用选中的 Mod 建组」没勾 Mod 时文案直接写 `先在 Mod 管理页勾选 Mod`。

**在打包 EXE 上驱动验证**（沙箱 5 个组）：

```
初始：共 5 个组；批量提示 = 先勾选组（每行最左边的方框，或点组名）才能批量删除 / 开关自动联动 /
      设置权重 / 用这些组筛选；批量删除 disabled=true，tooltip=先勾选要批量操作的策略组…；
      建组按钮文案=先在 Mod 管理页勾选 Mod（tooltip 说明去哪勾）
搜索「医疗箱」：显示 4 / 5 个组，列出父组 + 3 个子组
搜索成员名「ccc角色」：命中子组时自动带上父组 !!医疗箱 系列（缩进不会指向看不见的父组）
搜索结果里点全选：已选 4 个组（4 个成员）、可见 4 行全勾、批量按钮 enabled、
      提示行变绿「已勾选 4 个组，下面的批量按钮已可用」
清空搜索：回到「共 5 个组」与初始提示
errors=[]
```

**已自动覆盖**：`node --test` 203 项（新增 `strategy-group-filter.test.mjs` 7 项：
组名/成员名搜索、子组带父组、父组带子树、计数与批量提示文案；并更新窗口结构断言）。

**只能人工验证**：你自己库 19 个组时的搜索手感；真实游戏内加载顺序。

## 第十七轮调试：FireAxe 式祖先权重累加 + 「＋ 子组」快捷入口（2026-09-24，打包 EXE + 沙箱）

**用户问题**：策略组能不能在组内再建子组、子组能不能设权重排序？想更接近 FireAxe。

**结论**：子组与子组权重早就支持（最多 4 层），但**父组权重此前不会作用到子组成员**（当时是
"所有组取 min"的有意取舍）。本轮按用户选择改成 **FireAxe 式祖先累加**，并补上快捷入口：

- `pathTier(g)` = 沿 g 的上级链（g + 全部上级）**累加**已设置的 Tier（缓存 + 环保护）；
  `groupTier(mod)` = 该 Mod 所属各链 `pathTier` 的 **min**；`effective = min(base, groupTier)`。
  链上没有任何 Tier 时按"未设置"处理 → 完全没设权重的库**逐字节不变**。
- 新后端 `CreateModStrategyGroupChild(parentID, name, strategy, memberPaths)`：
  允许空子组、复用无环 / 4 层校验、写盘前自动备份；策略留空默认「互斥单选」。
- 窗口每行新增 **「＋ 子组」**（带 tooltip：「子组会累加它和全部上级分组的权重；最多 4 层」）。

**在打包 EXE 上驱动验证**（沙箱；父组 = !!医疗箱 系列）：

```
行内按钮：＋ 子组 ；tooltip = 在这个组下面新建一个子组（子组会累加它和全部上级分组的权重；最多 4 层）
弹窗正文：在「!!医疗箱 系列（主文件+音频+材质+开关模块）」下面新建一个子组。
          · 当前在 Mod 管理页勾选的 1 个 Mod 会一起放进这个子组
          · 子组的有效分层会累加它自己和全部上级分组的权重（最多 4 层）
创建结果：子组「上衣关」出现且缩进显示（└）、上级分组 = !!医疗箱 系列、成员 1 个
权重累加：父组 -2 + 子组 -3 → GetModPriorityPlan：groupTier=-5、effective=-5、source=group
          主列表角标同步为 优先级 #1（该 Mod 的 ddd角色.vpk）
清理：删除子组 + 清空父组权重后沙箱 groups.json 恢复原状；errors=[]
```

**已自动覆盖**：Go 新增 3 项（`TestGroupTierAccumulatesAncestorTiers`：
父权重继承 → 父子相加 -5 → 清空回到顺序号；`TestCreateModStrategyGroupChildCreatesNestedGroup`
与 `...RejectsBadInput`：空名称 / 父组不存在 / 缺 ID / 第 5 层被拒）；
`TestConflictPriorityGoldenRegressionWithoutLayers` 继续通过（没设权重时行为不变）；
前端 `node --test` 204 项。

**只能人工验证**：真实游戏内加载顺序（沿用第 4 节——交换两个同名文件 VPK 的顺序观察谁覆盖谁）。

发现不符时，请记录：操作步骤、界面截图、`addonlist.txt` 前后内容，以及应用日志中的报错行。
这些信息足以定位是后端语义、绑定参数还是前端渲染的问题。

## 第十八轮调试：事务化写盘 / 策略预检 / 自动修复分级 / schema 版本 / 下载节流 / 拖放排序（2026-09-24）

**用户要求**：把上一轮列出的 6 项"值得学 FireAxe"逐项落地（push 事务化、策略可满足性预检、
Problem 自动修复分级、本地记录 schema 版本化、下载进度落盘节流 + 任务去重、树拖放排序）。

**本轮结论**：6 项全部实现，且都按"先写失败测试再实现"的顺序做；证据与落点见
`docs/development/fireaxe-parity.md` §4.1 的 11-16 行。

### 本次已经自动化覆盖的部分（不需要人工重复）

| 语义 | 覆盖测试 |
| --- | --- |
| addonlist 写盘 + 派生步骤 + 快照同步是一个事务，快照失败会回滚并重跑派生步骤 | `internal/app/addon_list_transaction_test.go` |
| 策略组应用前预检（空组 / 成员全丢 / 全在 disabled / 只有一个可用成员） | `internal/app/mod_group_apply_check_test.go`、`strategy-group-batch.test.mjs` |
| 体检"可自动修复"白名单与统计 | `frontend/.../health-report-format.test.mjs` |
| `groups.json` / `priority.json` 的版本迁移与"不降级" | `internal/app/local_store_schema_test.go` |
| 下载任务 1s 节流、相同内容跳过、重启后标记 interrupted、入队去重、更新检查 in-flight 去重 | `internal/app/download_task_store_test.go` |
| 拖放落点判定（放进 / 排前 / 排后 / 回顶层、拒绝成环与超 4 层）与同级顺序重写 | `frontend/.../strategy-group-tree.test.mjs`、`internal/app/mod_group_order_test.go` |

### 本次已经"真机驱动"验证的部分（打包 EXE + 临时调试桥，详见 fireaxe-parity §5.2）

下面这些不是靠推理，是在打包后的 EXE 里跑真实的 DOM 事件拿到的结果；调试桥在验收后已删除并重建产物：

1. 策略组管理窗口能以浮动模式打开，3 行组全部 `draggable="true"`，底部落点与拖动说明都存在。
2. **在「甲」行的中段 drop「乙」** → 行上出现 `is-drop-inside`，`groups.json` 真的写入 `parentId`，
   界面重画为缩进显示，上级下拉同步。
3. **把「乙」拖到「丙」的下边缘** → `is-drop-after`，顺序从 `[甲,乙,丙]` 变成 `[甲,丙,乙]`。
4. **把「甲」拖到它自己身上** → 状态零变化，`groups.json` 未被改写。
5. **拖到底部落点** → 高亮 `is-drop-active`，`parentId` 被清除。
6. **关闭再启动** → 拖放结果保留（来自 `groups.json`）。
7. 预置的未完成下载任务在真实 EXE 里以 `interrupted` 出现在任务列表数据中（进度 37、`fileUrl` 保留）。

本轮真机验收还发现并修掉了两个问题：底部落点原本会被挤到可视区外（改为 `position: sticky; bottom: 0`）、
未知策略值不会自愈（读时归一化为「互斥单选」）。

### 只能人工验证的部分（在打包 EXE 上逐条确认）

1. **真实鼠标拖拽的手感**：上面第 2-5 条已经用合成事件证明"落点判定 + 写盘 + 重画"是对的，
   但**用真鼠标拖**时的命中区域是否符合手感（例如行的三段比例、拖动时的跟随反馈）
   仍然只能由你自己拖一次确认；如觉得上/下边缘太窄，改 `strategy-group-manager.js` 里
   `positionFor` 的 `0.28 / 0.72` 两个阈值即可（纯前端，不需要动后端）。
2. **下载中断恢复**：真实开始一个较大的工坊下载 → 进度超过 10% → 关闭应用 →
   重新打开 → 下载列表里该任务应显示 **已中断**（保留已下载字节数）并且「重试」可用；
   点重试应继续完成。这一条依赖真实关进程 + 真实网络，自动化只覆盖了快照的读写与合并逻辑。
3. **下载节流的写盘频率**：真实下载时观察 `%AppData%\LytVPK\download_tasks.json` 的修改时间，
   应约每秒最多一次（而不是每 500ms 一次）；内容不变时不应有写入。
4. **体检一键修复**：在真实库上跑体检 → 点某条「修复」→ 应先出现应用内确认框，
   确认后 `addonlist.txt` 变化且备份列表里出现 `before-health-fix` 备份。
5. **策略预检提示**：把一个组的成员文件临时改名/移走，再点「按策略应用」→
   应看到"不可执行 + 原因"，而不是静默失败。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。上述 5 条都是**环境相关、只能人工确认**的部分。

## 第十九轮：工坊官方标签与统计 / 依赖环 / 文件名安全化（2026-09-24）

**本轮先做了实测对比再决定学什么**：

```text
现有镜像接口（真实作品 2302720558）
  → 返回字段：result / publishedfileid / file_type / filename / file_size / file_url /
    preview_url / title / file_description / children   ← 没有 tags
Steam 官方 ISteamRemoteStorage/GetPublishedFileDetails（同一个作品）
  → tags=[Survivors, Sounds, Single Player, Other]
    subscriptions=21564  views=53208  favorited=6515
```

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 官方标签清洗（去空白 / 大小写去重 / 保序）、统计合并、全 0 不清零、相同数据不重复写盘 | `internal/app/workshop_steam_details_test.go`（夹具是**真实抓取的官方响应** `testdata/steam_published_file_details.json`） |
| 抓取→落盘→第二次判定未变化→本地没有的 ID 报 missing→整批失败不改本地文件 | 同上（`TestEnrichWorkshopMetadataEndToEnd` / `...ReportsBatchFailure`） |
| 官方标签与统计进入"给智能体的材料"（`workshop.steamTags` + 热度） | `TestWorkshopSnapshotExposesOfficialTagsAndStats` |
| 依赖环（无环 / 两节点 / 三节点 / 自环 / 两个环）与体检项 | `internal/app/dependency_cycle_test.go` |
| `dependency_cycle` 不在自动修复白名单 | `frontend/.../health-report-format.test.mjs` |
| 文件名合法性（8 个合法 + 21 个非法）与真实重命名入口 | `internal/app/vpk_actions_rename_validation_test.go` |

### 真网络验证（一次性，脚本已删除，不进 CI）

```text
requested=2 updated=2 failed=[] errors=[]
2302720558 → steamTags=[Survivors Sounds Single Player Other] subs=21564 views=53208 fav=6515
3153860853 → steamTags=[Campaigns Common Infected]          subs=10061 views=62250 fav=3562
```

### 只能人工验证的部分

1. **设置 → 工坊数据 → 「抓取工坊官方标签与统计」按钮的真实点击体验**：
   抓取期间按钮显示"抓取中..."、结束后提示"更新 N 个 / 本来就最新 N 个 / 本地没有 N 个 / 接口没返回 N 个"。
   逻辑与后端行为已有自动化覆盖，但按钮文案与等待时长的手感只能自己点一次。
2. **国内网络下 Steam 官方接口的连通性**：接口不可达时应当只报"官方接口没返回 N 个"而不影响其它功能。
   需要代理的话，应用会读取系统代理环境变量（`HTTPS_PROXY` 等）。
3. **抓取对分组建议的实际提升幅度**：抓完后重新导出「准备给智能体材料」，对比 `workshop.steamTags`
   是否让智能体的分组更准——这属于效果评估，只能人看。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。

## 编辑快捷键 / 主窗口记忆 / 冗余副本 / 剪贴板识别 / Ctrl+X·V（2026-09-26，打包 EXE + 沙箱夹具）

### 本次已经自动化覆盖的部分

```text
go test ./... -count=1        全绿（新增 workshop_clipboard_test.go）
node --test                   351 项 / 0 失败
npm run build                 通过（仅既有的 chunk 体积警告）
wails build                   通过；产物 18,481,152 字节，ASCII 扫不到 cua-bridge / DBG 探针
docs: npm run docs:build      通过
```

具体的自动化断言：

| 能力 | 断言位置 |
| --- | --- |
| 剪贴板链接解析（10 类输入：两种链接写法、`id` 非首个参数、纯数字 ID、非工坊链接、空串…） | `internal/app/workshop_clipboard_test.go` |
| 轮询判定（开关 / 前台 / 间隔）、去重（同一条只提示一次、输入框里已有就不再提示） | `frontend/src/js/core/workshop-clipboard.test.mjs` |
| 轮询真的接在启动流程里、确认后带着链接打开工坊页 | `frontend/src/js/core/shortcuts-wiring.test.mjs` |
| `isTextEntryElement`：复选框/按钮不算"输入框"，文本 input / textarea / contenteditable 才算 | `frontend/src/js/core/shortcut-guards.test.mjs` |
| 状态栏「待移动」角标（真实 import `state.js` + DOM 桩；含变异验证） | `frontend/src/js/features/status-bar-pending-move.test.mjs` |
| 快捷键总览里的 `F2` / `Delete` / `Ctrl+X` / `Ctrl+V` 都有真实绑定 | `frontend/src/js/core/shortcuts-wiring.test.mjs` |
| 主窗口几何：按屏幕钳制、尺寸/最大化是否该恢复（6 项） | `frontend/src/js/core/main-window-geometry.test.mjs` |
| 工坊冗余副本体检项（只报 1 条、指向 `addons` 根的那份、提示"只保留一份"、体检只读） | `internal/app/health_check_precision_test.go` |

### 本次已在打包 EXE 上驱动验证（沙箱配置目录 + 合法 VPK 夹具，未碰真实 Mod）

```text
夹具：7 个 VPK（根目录 5 / workshop 1 / disabled 1）+ addonlist.txt，共 5 个可解析 VPK
启动：走沙箱 config.json（优选IP=false 与真实配置不同，日志可区分），列出 7 个 Mod，总文件数 6/7
F2        → 弹出提示"先用 ↑↓ 把光标移到要重命名的 Mod 上（或只选中一个）"（多选态下的预期行为）
Ctrl+F    → 搜索框获得焦点（蓝色描边）
Ctrl+K    → 命令面板打开（页面跳转 + 常用动作列表）
Esc       → 浮层关闭
勾选行    → 状态栏"已选择: N" 同步变化
```

同一轮里由真机驱动发现并修掉的两个缺陷（自动化测试原本照不到）：

1. `updateStatusBar()` 引用了未声明的 `pendingMoveEl` / `pendingMoveCount`（声明被补丁插进了
   `updateSelectedFilesStatus()`）→ 表现是"`Ctrl+X` 标记成功，但状态栏没有角标、也没有提示"。
   已抽 `syncPendingMoveIndicator()`；新增的回归测试做了变异验证（把 bug 放回去即报
   `ReferenceError: pendingMoveEl is not defined`）。
2. 列表行复选框 `<input type="checkbox">` 被当成"正在输入文字"，勾选后 `Ctrl+X` / `Ctrl+V` 直接被跳过。
   已抽 `isTextEntryElement()`（只认文本类 input / textarea / contenteditable）。

同时确认的一件环境事实：**沙箱 `config.json` 如果写的不是合法 JSON，应用会回退到"自动发现游戏目录"**，
也就是会去读真实库（本轮据此核对过用户真实 `addons` 目录，40 分钟内零写入，只有只读扫描）。
以后做沙箱验收时，写配置一律用 `ConvertTo-Json` 之类的转义，不要手写反斜杠。

### 只能人工验证的部分

1. **`Ctrl+X` → `Ctrl+V` 的端到端手感**：自动化键盘注入在打包 WebView2 里时通时不通（同一条 `Ctrl+K`
   也会间歇失效），因此"按下去是否立刻出现角标、弹出目录选择框、移动完成后标记是否清掉"需要你自己点一次。
   纯逻辑（标记集合、状态栏刷新、`movePathsToDestination` 复用）已有自动化覆盖。
2. **剪贴板提示的出现时机**：复制一条工坊链接 → 切回 LytVPK，确认弹窗是否在 1~2 秒内出现；
   以及"已经手动粘贴过同一条链接时不再提示"是否符合你的预期。判定规则（开关 / 前台 / 间隔 / 去重）
   有单测，但真实剪贴板与前台焦点只能人工确认。
3. **换屏幕后的主窗口尺寸**：自动化验证了"超出屏幕会被钳制"，但多显示器之间来回移动时的观感、
   以及与最大化状态的组合仍需人工看一次。
4. **工坊冗余副本的处置**：体检只提示不删除，确认提示文案与位置（`addons\<ID>.vpk` + `addons\workshop\<ID>.vpk`）
   是否够清楚、是否希望将来提供"一键把根目录那份移到回收站"的入口。

### 仍未验证

没有"计划内但完全未验证"的项目；上面 4 条属于只能人工确认的观感/时机类项目。

## 自定义外部打开程序（2026-09-26，打包 EXE + 沙箱夹具 + xcopy 端到端）

### 本次已经自动化覆盖的部分

```text
go test ./... -count=1     全绿（新增 internal/app/open_with_test.go）
node --test                354 项 / 0 失败（新增 open-with-settings.test.mjs 3 项）
npm run build              通过（仅既有的 chunk 体积警告）
wails build                通过；产物 18,496,000 字节，ASCII 扫不到 cua-bridge
docs: npm run docs:build   通过
```

| 断言 | 位置 |
| --- | --- |
| 参数分词 11 类：空白分隔、引号内保留空白、`\"` 得字面量引号、**Windows 路径反斜杠原样保留**、单引号分组 | `internal/app/open_with_test.go` |
| 命令组装 5 类：未配置 → ok=false（走系统默认）、空模板 → 追加路径、`{path}`/`{dir}`/`{name}`、`{0}` 兼容、无占位符 → 追加路径 | 同上 |
| 保存校验 4 类：空（= 未配置）、纯命令名放行、真实路径通过、带路径但不存在报错 | 同上 |
| 配置往返：落盘到 config.json、程序留空时参数一并清掉 | 同上 |
| **真启动外部程序的端到端**：`xcopy.exe /Y {path} <目录>` → `OpenFileLocation` → 夹具文件被复制且内容一致 | 同上 |
| 设置页接线：两个输入框 + 保存/恢复默认按钮存在、说明里写了占位符与"留空 = 系统默认"、**用事件委托绑定**、前端默认值为空串 | `frontend/src/js/features/settings/open-with-settings.test.mjs` |

### 本次已在打包 EXE 上确认的部分

```text
沙箱配置 + 夹具启动 → 设置 → 界面设置：「打开文件方式（可选）」卡片正常渲染
  - 标题 / 说明（留空 = 系统默认、{path} {dir} {name} 占位符、引号用法）都在
  - 程序 / 参数模板两个输入框、保存 / 恢复默认两个按钮都在
  - 整页没有出现"设置页面加载失败"，也没有新的前端错误报告
产物自检：前端包与 EXE 里都能扫到 settings-open-with-save / settings-open-with-program / SetOpenWithSettings
```

### 只能人工验证的部分

1. **在设置页点「保存」与「恢复默认」并看到 config.json 里出现 `openWithProgram`**：
   本环境的自动化鼠标点击对这两个按钮不可靠（同一脚本点设置页导航标签有效，点这两个按钮只拿到焦点，
   保存不落盘），因此这一步需要你自己点一次确认。后端行为已由"真启动 xcopy"的 Go 端到端测试覆盖，
   前端接线由契约测试覆盖。
2. **用你自己的程序实测**：填上你常用的文件管理器 / 编辑器，确认参数模板写法符合它的命令行习惯
   （不同程序对 `/O`、`/T`、`-R` 这类开关的拼法不同）。
3. **路径含空格 / 逗号的程序**：分词与引号规则有单测，但真实程序对引号的处理各有差异，建议实测一个。

### 仍未验证

没有"计划内但完全未验证"的项目；上面 3 条属于只能人工确认的交互与第三方程序差异。

## 体检修复分级（依赖一键修）+ 两类新诊断 + 失效目标守卫 + 游戏目录自动探测（2026-09-26，打包 EXE + 沙箱）

对应 `docs/development/fireaxe-parity.md` 的第 28-32 项。

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 依赖问题的体检项带可操作目标，且**用这个目标就能驱动修复**（开启依赖 → 复查后问题消失） | `internal/app/health_check_precision_test.go`（`TestDependencyDisabledIssueCarriesFixTarget`） |
| 依赖修复写盘前留下 `before-dependency-fix` 备份 | 同上（断言备份存在） |
| 同名路径是文件夹 → 只报 `file_type_mismatch`，真正的缺失仍报 `missing_file`；`disabled` 有副本时提示出路 | 同上（`TestHealthCheckReportsFileTypeMismatchForDirectoryNamedVpk`、`TestHealthCheckFileTypeMismatchPointsAtDisabledCopy`） |
| 工坊记录 ID 与文件名不一致 → 报 1 条 `meta_id_mismatch`，且不误判成"孤立 / 缺失"；对得上的不报 | 同上（`TestHealthCheckReportsWorkshopMetaIDMismatch`） |
| 自动修复白名单（三类）与"依赖问题必须带目标" | `frontend/src/js/features/settings/health-report-format.test.mjs` |
| 设置页把 `fixTarget` 传给修复回调、依赖分支真的调用 `EnableModDependencies` | `frontend/src/js/features/settings/settings-bindings.test.mjs` |
| 失效目标判定（未改动 / 被替换 / 目录 / 被删除） | `internal/app/task_target_guard_test.go`（`TestModTaskTargetGuardDetectsInvalidTargets`） |
| 更新检测中途目标被移走 → 不写 `.meta`、`skipped_stale=1`；目标没动 → 正常写回 | 同上（`TestUpdateCheckSkipsMetaWriteWhenTargetMovedMidFlight`、`TestUpdateCheckWritesMetaWhenTargetUnchanged`） |
| Steam 库清单解析（新旧两种 VDF 写法、转义、大小写去重、脏内容不猜、忽略 `apps`/`label`/`contentid`） | `internal/app/steam_locations_test.go`（`TestParseSteamLibraryFoldersHandlesBothFormats`、`TestParseSteamLibraryFoldersIgnoresUnrelatedPaths`） |
| 自动找 addons 目录的顺序（注册表目录优先 → 库清单登记的库）、注册表为空不猜、没有 `addons` 不返回 | 同上（`TestFindAddonsInSteamLibrariesUsesRegistryAndLibraryFolders`、`TestSteamLibraryCandidatesIncludeConfigFolder`、`TestAddonsPathIfGameRequiresLeft4Dead2Directory`） |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

沙箱 `%AppData%` + fixture 根目录（**没有碰真实 Mod、`addonlist.txt` 与真实配置目录**）：

```text
fixture：addons\real.vpk、master.vpk、dep-off.vpk，addons\ghost.vpk 是文件夹，
        addons\workshop\123.vpk + 123.meta（记录的作品 ID 是 999，与文件名不符）
沙箱 dependencies.json：master.vpk → dep-off.vpk

设置 → 游戏配置 → 开始体检
  摘要：发现 4 个问题：警告 3、提示 1
  汇总条：其中 1 项可以自动修复：启用依赖（1）。其余问题需要人工判断，不会自动改动任何文件。
  行：依赖未开启 · master.vpk（有「修复」，data-fix-action=enable-dependencies）
      同名路径是文件夹 · ghost.vpk（无修复按钮）
      工坊信息对不上作品 · 123.meta（无修复按钮）
      未写入开关记录 · real.vpk

点「修复」→ 确认
  体检摘要 4 → 3；依赖行消失，另外三条不受影响
  addonlist.txt：dep-off.vpk 由 "0" 变 "1"；缩进 / 引号 / 其余条目原样保留
  真实 %AppData%\LytVPK 开跑前后 19 个文件逐文件 SHA256 一致
```

自动找游戏目录（只读探针，不写任何文件；跑完已删除该探针）：

```text
注册表 Steam 安装路径 = "C:\\steam"
libraryfolders.vdf 解析出 5 个库：C:\steam, G:\SteamLibrary, D:\SteamLibrary,
                                  E:\SteamLibrary, F:\SteamLibrary
AutoDiscoverAddons() = E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2\addons
```

### 只能人工验证的部分

1. **真实工坊链接下的更新检测**：第 31 项修的是"检测请求还在飞行中，用户把 Mod 移走"的偶发竞态，
   自动化用注入的详情获取器复现了同一时序；真实网络下这个时间窗有多长（以及你实际会不会撞上）
   只能靠日常使用观察，日志里的「目标已被移动或替换，跳过写入」就是它。
2. **真实库里重新体检的观感**：新加的两类诊断在 2400+ Mod 的真实库里会不会刷出很多条、
   排序与严重度是否符合你的直觉，需要你自己跑一次体检确认。
3. **确认框文案是否够清楚**：「启用依赖」的确认框只说会给哪个主 Mod 开依赖，
   文案是否够直白属于主观判断。
4. **换一台机器 / 换一个 Steam 库的自动发现**：自动化覆盖了解析与优先级，
   但"新装一台机器、Steam 装在非常规位置（绿色版、库清单放在 `config\` 下）"这种组合
   只能在你实际遇到时确认；找不到时按提示手动选目录即可，功能不受影响。

### 仍未验证

- `IValidity` 的**增量级联传播**（FireAxe 侧最后一处没学的设计）：本轮明确不做，
  理由是本项目"脏标记 + 按需全量重算"在 2400+ Mod 下仍是亚秒级，增量传播要额外维护
  "文件 → 提供者"反查索引，风险大于收益（理由写在 `fireaxe-parity.md` §4.2.1）。
- 本轮没有其它"计划内但完全未验证"的项目。

## 受管目录路径守卫 + 归档列表统一检索（2026-09-26，打包 EXE + 沙箱）

对应 `docs/development/fireaxe-parity.md` 的第 33 项与 §5.11。

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 路径判定：addons / workshop / 更深一层 / disabled 放行；父目录、`..` 逃逸、受管目录本身、空路径、相对路径拦下；大小写不敏感；没选目录不放行 | `internal/app/path_guard_test.go`（`TestManagedFilePathProblemDetectsOutsidePaths`，10 类用例） |
| 越界文件删除被拒且文件零改动；受管目录里的删除照常工作 | 同上（`TestDeleteVPKFileRefusesPathsOutsideManagedRoots`） |
| 越界文件隐藏/改名被拒且不产生 `_` 前缀文件 | 同上（`TestToggleVPKVisibilityRefusesPathsOutsideManagedRoots`） |
| 越界源文件逐项失败并留在原处；**移动到用户自选目录仍然成功**（既有能力不退化） | 同上（`TestMoveVpkFilesKeepsFreeDestinationButGuardsSources`） |
| 体检把越界条目报成 `outside_root`，且不重复报成"缺失文件"，体检本身只读 | 同上（`TestHealthCheckReportsAddonListEntriesOutsideManagedRoots`） |
| 新体检项的中文文案与"不给自动修复按钮" | `frontend/src/js/features/settings/health-report-format.test.mjs` |
| 归档检索语义：普通词（且，fuzzy 与后端一致）、引号短语、`-排除`、`re:`、非法正则报错、`tag:` 状态（含 `\|` 或）、计数文案、高亮参数 | `frontend/src/js/features/diagnostics/archive-search.test.mjs`（8 项纯逻辑） |
| 归档管理器真的接上了统一检索（语法提示 / 计数 / 高亮 / 状态 chip） | 同上（第 9 项，静态检查 `archive-manager.js`） |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

沙箱 `%AppData%` + fixture 根目录（真实 Mod 与真实配置目录全程未碰）：

```text
fixture：addons\inside.vpk（受管），fixture\outside\precious.vpk（受管之外），
        addonlist.txt 里除 inside.vpk 外还有一条 "..\outside\precious.vpk"

DeleteVPKFile(<受管之外的 precious.vpk>)
  → 报错："这个文件不在当前受管的 addons / workshop / disabled 目录里：…\outside\precious.vpk"
  → 磁盘核对：precious.vpk 仍在原处（True）
DeleteVPKFile(<受管目录里的 inside.vpk>)
  → 无报错；磁盘核对：inside.vpk 已移入回收站（False）—— 守卫没有误伤正常流程

设置 → 游戏配置 → 开始体检
  → 「条目指向受管目录之外 · ..\outside\precious.vpk」，无「修复」按钮（需人工改回相对路径）
```

### 只能人工验证的部分

1. **真实点击归档管理器的搜索框**：入口是系统"选择目录"对话框，无法由自动化打开，
   所以归档检索只做到"纯逻辑 + 接线"两级验证；语法提示、计数文案、`<mark>` 高亮与
   `tag:` chip 的实际观感需要你自己点一次压缩包管理确认（高亮用的是 Mod 列表同一套样式，
   那套已在之前的轮次真机验证过）。
2. **"真的中途换过 addons 目录"的时机**：自动化用等价路径复现了同一判定，
   但"换目录后立刻点删除"这种真实操作序列只能你遇到时确认。

### 仍未验证

- 本轮计划内项目都已覆盖；`IValidity` 的增量级联传播仍是唯一的"有意留到以后"。

## 命令面板最近使用 + 高对比档只提正文 + 说明书复用（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 最近使用记录：去重、最近在前、上限 5、空 id / 坏数组不破坏记录 | `frontend/src/js/core/command-palette.test.mjs`（`pushRecentCommand`） |
| 空查询时最近用过的排最前，其余保持命令表原顺序，记忆里已不存在的 id 忽略，原数组不被打乱 | 同上（`orderCommandsByRecents`） |
| 带最近使用时只影响空查询；有关键词仍按相关度 | 同上（`searchCommands`） |
| 状态文案在没有记录时不提"最近用过" | 同上（`describeCommandPaletteState`） |
| 面板真的用上最近记录：空查询排序、执行后写回、命名空间存储键、可注入存储、最近标记、状态行计数 | `frontend/src/js/core/command-palette-ui.test.mjs`（静态接线断言 + 执行路径计数） |
| 高对比档三档文字齐全且保持"正文 > 次要 > 装饰"的层级（WCAG 相对亮度） | `frontend/src/js/core/reading-contrast-css.test.mjs` |
| 高对比档只提正文与次要文字，装饰文字不深于 / 不亮于标准档 | 同上 |
| 归档面板复用同一份说明书（字段换成压缩包相关，`tag:` 说明改成包状态，快捷键同源） | `frontend/src/js/features/file-list/search-help.test.mjs` |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

```text
命令面板（真实点击标题栏按钮 + 输入框 + Enter）
  清空 lytvpk.commandPalette.recent 后打开：表首 = focus-mod-search（表顺序）
  输入「重置」→ 只剩 reset-window-geometry → Enter 执行
  再打开：表首 = reset-window-geometry，提示文案「最近 · 浮动窗口自救」
  状态行 = 「最近用过 1 条在最上面 · 共 17 条命令 · …」
  localStorage["lytvpk.commandPalette.recent"] = ["reset-window-geometry"]

高对比档（设置 → 界面设置 → 点「高对比」，暗色主题）
  标准：--text-primary #f1f5f9 / secondary #cbd5e1 / tertiary #94a3b8
  高对比：         #ffffff /           #e6edf7 /           #94a3b8（装饰不变）
  真实元素计算色：standard rgb(203,213,225) → contrast rgb(230,237,247)
  切回标准：三档全部还原为 #f1f5f9 / #cbd5e1 / #94a3b8
```

### 只能人工验证的部分

1. **"最近使用"是否符合你的使用习惯**：上限 5 条、只在空查询时提前，是主观取舍；
   觉得该记更多/更少时改 `RECENT_COMMAND_LIMIT` 即可。
2. **高对比档的观感**：自动化锁住的是"层级与不变量"，
   强光环境下长时间阅读是否舒服只能你自己用一段时间再定。
3. **归档面板的 `?` 浮层**：入口是系统"选择目录"对话框，无法由自动化打开，
   所以归档面板只做到"纯逻辑 + 接线 + 说明书同源"验证；浮层观感请你在压缩包管理里点一次确认。

### 仍未验证

- `IValidity` 的增量级联传播（FireAxe 侧唯一的"有意留到以后"，理由见 `fireaxe-parity.md` §4.2.1）。
- 本轮其它计划项均已覆盖。

## 对象解释层：失败提示 + 灰按钮理由（2026-09-26，打包 EXE + 沙箱）

对应 `docs/development/fireaxe-parity.md` 的第 34 项与 §5.12（对照范围首次扩到 `FireAxe.GUI`）。

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 12 条错误规则各自的人话摘要与建议（路径守卫 / 闸门忙 / 工坊转移 / 没选目录 / 文件不在 / 同名 / 权限 / 目标路径 / 网络…） | `frontend/src/js/core/action-explanation.test.mjs` |
| 场景区分：`输入 / 重命名 / 解析` 类错误只在输入场景按"输入不合法"解释；操作场景走兜底但仍有话说 | 同上 |
| 认不出规则时保留原文 + 通用建议；空消息退化成「操作失败」；对象形态（Wails errorInfo）也能处理 | 同上 |
| 文件名不被解释层吞掉（`移动 a.vpk 失败…` → subject `a.vpk`） | 同上 |
| 可用性 6 类：没选目录 / 忙碌（只读操作不受影响）/ 工坊 toggle / disabled 游戏内开关 / 批量没选中 / `disable-file` 三种情形 | 同上 |
| 逐项失败文案 = 文件名 + 人话 + 下一步；认不出的原因保留原文 | `frontend/src/js/features/file-list/move-result-format.test.mjs` |
| 界面真的接上：`toast.js` 全局错误、`operations.js` 两处前置检查、`model-stats-scan.js` 禁用按钮状态与提示 | `frontend/src/js/core/action-explanation.test.mjs`（接线断言 + 旧重复文案已移除） |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

走的**不是**直接调解释函数，而是真实链路：导出方法 `LogError(type, message, file)`
→ Go 发 `error` 事件 → 页面 `EventsOn("error")` → 解释层 → 渲染到 `#toast-container`。

```text
文件操作 / 移动 a.vpk 失败: The system cannot find the path specified
  toast：内容：目标路径当前不可用  建议：确认目标目录还在、且没有被同名文件占住，然后重试。
文件操作 / 另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试
  toast：内容：上一个文件操作还没结束  建议：等状态栏的提示消失后再试，避免两批操作互相插队。
文件操作 / 这个文件不在当前受管的 addons / workshop / disabled 目录里：E:\x.vpk
  toast：内容：这个文件不在当前管理的 addons / workshop / disabled 里  建议：刷新一次 Mod 列表…
重命名 / 文件名不合法：文件名不能包含 \ 这类字符（Windows 命名规则）
  toast：内容：这个名字不能用作文件名  建议：避开 < > : " / \ | ? * 与控制字符…
文件操作 / 某种没见过的后端错误 XYZ-123
  toast：内容：某种没见过的后端错误 XYZ-123  建议：如果反复出现，把这条消息和对应的 Mod 名一起反馈。
```

### 只能人工验证的部分

1. **措辞是否符合你的语感**：规则表集中在 `frontend/src/js/core/action-explanation.mjs`，
   改文案不需要动业务逻辑；哪一条说法别扭直接说，我改那一行。
2. **灰按钮 tooltip 的实际观感**：自动化覆盖了"判断结果与文案一致"，
   但鼠标停在模型统计面板「禁用」按钮上的观感需要你自己看一眼。

### 仍未验证

- `IValidity` 的增量级联传播（唯一"有意留到以后"的一项）。

## 检索一致性审计 + 工坊"服务端搜索"说明（2026-09-26，打包 EXE + 沙箱）

### 附：冲突复检性能（同轮，真实库只读测量）

FireAxe 对照表最后一项"未做"（`IValidity` 增量传播）这一轮用实测钉死，结论是**不做**：

```text
真实库（2409 Mod / 2107 参与 VPK / 24.1 万路径）
修复前：热重算 552/560/559ms（与冷跑 591ms 几乎相同）
        细分：遍历 7.7ms；取清单循环 436ms/434ms；缓存条目 1024（= 上限）
根因：索引缓存容量写死 1024 < 库里的 2409 → 每轮 LRU 抖动、重复解析
修复后：热重算 173/179/180ms（3.1×）；缓存条目 2107；候选收集 4ms；忽略清单 0ms
剩余：173ms = 2107 Mod × 24.1 万路径的分组累加；前端有 400ms 防抖，一次突发只算一次
```

**已自动验证**：`internal/app/conflict_index_capacity_test.go`（容量规则含硬上限、容量影响淘汰、
默认容量、端到端写入与不超限）；**仍未验证**：无（该对照项已闭环）。

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 统一语法面板 6 个都有 `?` 按钮与浮层，且浮层内容来自 `search-help.mjs` 的对应变体、悬停提示同源 | `frontend/src/js/core/search-consistency.test.mjs`（含"面板清单要与实现同步"的计数断言） |
| 工坊搜索写明"由 Steam 服务端执行、不支持 tag:/re:/-排除"，并列出 6 个可用本地语法的面板 | 同上 |
| 局部小过滤器（加载顺序 / 组选择器 / 建议窗口 / autoexec / mdmp / 服务器地图）的占位文案不宣称支持本地语法 | 同上（静态 + 动态生成的两类都扫） |
| **测试本身有牙**：变异验证 —— 把体检面板的说明书换回默认变体后审计立刻失败 | 手工执行一次变异并还原（记录在本节） |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

```text
工坊搜索框 placeholder = 搜索工坊物品标题（由 Steam 搜索，不支持 tag: / re: 等本地语法）
           title       = 工坊搜索由 Steam 服务端执行：… 想用本地语法请到：Mod 列表、压缩包管理、
                         模型统计、策略组管理、冲突检测、体检结果。
Mod 列表    placeholder = 搜索 VPK（支持 tag:武器、-tag:材质、re:^ak 等写法；…）
           title       = 匹配：标题、文件名、… / 语法：ak47 武器 / "ak 47" / tag:步枪 / … / 快捷键：…
           ? 按钮      = 存在
体检搜索    placeholder = 搜索问题（对象名 / 路径 / 提示文案；支持 -排除 / re: 正则 / tag:严重度或类型）
           title       = 匹配：问题对象名、文件路径、可操作目标、提示文案、严重度与问题类型 / …
           ? 按钮      = 存在
局部过滤器  加载顺序「搜索 Mod 名称或 VPK 路径」、组选择器「搜索策略组（名称 / 上级分组 / 策略）…」
```

### 只能人工验证的部分

1. **实际手感**：自动化验证了"文案与接线一致"，但六个面板轮着用一遍是否顺手（尤其 `Esc`/
   `↑↓` 在各自上下文里的预期）只能你自己体验。
2. **工坊搜索的服务端行为**：关键词搜索、分页、排序本身依赖 Steam 接口，属于外部服务，无法在自动化里覆盖。

### 仍未验证

- `IValidity` 的增量级联传播（唯一"有意留到以后"的一项）。

## 体检结果统一检索 + 覆盖关系区过滤提示（2026-09-26，打包 EXE + 真实 mod 库只读体检）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 体检检索：对象名 / 路径 / 可操作目标 / 文案跨字段匹配、多词"且"、`-排除`（子串，不误伤长路径）、`re:`、`tag:` 严重度与问题类型、计数与高亮参数 | `frontend/src/js/features/settings/health-search.test.mjs` |
| 检索在 100 条显示上限**之前**生效，计数栏区分"匹配"与"另有 N 条未显示" | `frontend/src/js/features/settings/settings-page.js`（接线 + 同上测试的接线断言） |
| 正则写错时计数与空列表都直说原因 | 同上 |
| 覆盖关系区被过滤时显示"显示 / 总数 组（已过滤）" | `frontend/src/js/features/conflicts/conflict-search.test.mjs` |

### 本次真机验证（打包 EXE + 临时调试桥；体检是只读操作，未改动任何 Mod 或 addonlist）

```text
真实库体检：发现 943 个问题（严重 1、警告 1、提示 941）
  列表显示前 100 条 · 另有 843 条未显示（以前这 843 条没有任何入口）
tag:警告                          → 匹配 1 / 943
tag:条目缺少文件                   → 匹配 1 / 943
tag:缺少工坊信息                   → 匹配 940 / 943
-tag:缺少工坊信息                  → 匹配 3 / 943（互补，合计 943）
-tag:缺少工坊信息 -tag:依赖缺失     → 匹配 2 / 943
vpk                              → 匹配 943 / 943，100 行 201 处 <mark class="search-hit">
re:[                             → 计数与空列表均为「正则表达式无效：…」
清空                              → 回到 100 行 + 「另有 843 条未显示」
`?` 说明书                        → 打开、含 tag:条目缺少文件 行、tooltip 同源（匹配：问题对象名、文件路径、可操作目标、提示文案、严重度与问题类型）
```

### 只能人工验证的部分

1. **体检结果里"修复"按钮与搜索的组合**：自动化验证了过滤与计数，
   但"搜完再点某一条的修复"这种操作序列请你实际点一次（修复逻辑本身未改动）。
2. **工坊信息存储是否要开启**：940 条提示源于缺 `.meta`。这是数据状态而非缺陷，
   要不要开启工坊信息存储、开启后重新抓一遍，取决于你的使用习惯。

### 仍未验证

- `IValidity` 的增量级联传播（唯一"有意留到以后"的一项）。

## 冲突弹窗统一检索 + 排除词语义修正（2026-09-26，打包 EXE + 真实 VPK 夹具）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 冲突检索：资源路径 / Mod 名 / 标题跨字段匹配、多词"且"、`-排除`、`re:`、`tag:` 严重度与 `同层`/`未记录`、计数与高亮参数 | `frontend/src/js/features/conflicts/conflict-search.test.mjs` |
| **排除词用子串**（正向词仍模糊）：`vpk -katana`、`vpk -old` 不会在长路径里凑字母误伤；`-ak47`、`-hd.vpk` 仍正确排除 | 同上 + `internal/app/search_query_test.go`（`TestNegativeTermUsesSubstringNotFuzzy`） |
| 后端 `-词` 语义：`-old` 保留、`-材质` 排除、`-ak47` 排除 | `internal/app/search_query_test.go`（`TestModSearchQueryMatchesFields` 新增 2 条用例） |
| 冲突面板接线：统一检索、计数、高亮、同源说明书 | `conflict-search.test.mjs`（接线断言） |

### 本次真机验证（打包 EXE + 临时调试桥 + 真实 VPK 夹具）

夹具：`ak47-hd.vpk` 与 `ak47-cn.vpk` 都含 `materials/models/ak47/ak47.vtf`（真实 VPK，构成真冲突）。

```text
初始            → 1 组冲突（严重度 info），严重度筛选=全部
ak47           → 匹配 1 / 1 个组冲突，标题与文件名共 4 处高亮
tag:提示        → 匹配 1 / 1（该组严重度就是「提示」）
tag:严重        → 没有匹配（并写清原因）
tag:严重|警告|提示 → 匹配 1 / 1（或语义）
vpk -katana    → 修复前 0 组（误伤）；修复后 匹配 1 / 1 + 4 处高亮
re:[           → 「正则表达式无效：…」
清空            → 回到 1 组、无高亮
`?` 说明书      → 打开、含 tag:同层 行、tooltip 同源、Esc 关闭
```

### 只能人工验证的部分

1. **真实大库上的冲突检索**：自动化用 1 组冲突的真实夹具验证语义，
   真实 2400+ Mod 的冲突规模下滚动/分页与检索的组合观感请你实际点一次。
2. **排除词新语义的习惯迁移**：`-词` 从"模糊排除"变成"子串排除"是**行为变更**（更符合直觉，但确实变了）；
   如果你更想要别的口径（例如排除也支持正则 `-re:`）说一声。

### 仍未验证

- `IValidity` 的增量级联传播（唯一"有意留到以后"的一项）。

## 策略组管理统一检索（真实 115 组）（2026-09-26，打包 EXE + 沙箱 + 真实数据副本）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 统一语法：多词"且"、引号短语、`-排除`、`re:` 正则（写错返回空 + 文案直说原因） | `frontend/src/js/features/mod-groups/strategy-group-filter.test.mjs` |
| `tag:` 组状态：策略四值 / 顶层 / 子组 / 自动联动，支持 `|` 或与组合条件 | 同上 |
| **排除优先于树形补全**：`tag:顶层 -tag:子组` 不会把子组又补回来 | 同上 |
| 既有树形行为未退化：子组命中带上所有上级、父组命中带上所有下级 | 同上（原有 7 项断言保持） |
| 窗口接线：说明书生成为组变体、tooltip 同源、正则错误文案、`?` 按钮与浮层存在 | 同上（接线断言） |

### 本次真机验证（打包 EXE + 临时调试桥 + **用户真实 groups.json 的只读副本**）

```text
真实 groups.json：161,018 字节，sha256 5A0BA34E…（验收前后一致，原文件全程只读）

策略组管理窗口：
  初始              → 共 115 个组
  tag:单选          → 显示 51 / 115 个组
  tag:顶层 -tag:子组 → 显示 34 / 115 个组（排除生效）
  套装 -材质        → 显示 68 / 115 个组
  re:[             → 计数与空列表均为「正则表达式无效：…」（不再说"没有匹配"）
  清空              → 回到 115 个组
  `?` 说明书        → 打开、含「tag:单选」行、tooltip 同源（匹配：组名、组描述、成员名、组状态）、Esc 关闭
```

### 只能人工验证的部分

1. **搜索时的滚动与勾选手感**：自动化验证了过滤结果与计数，
   但"边搜边勾选 / 批量操作只作用于搜索结果"的手感请你实际操作一遍。
2. **组状态标签够不够用**：目前是 策略四值 + 顶层/子组 + 自动联动；
   如果你还想要别的维度（例如"有缺失成员"），说一声就加。

### 仍未验证

- `IValidity` 的增量级联传播（唯一"有意留到以后"的一项）。

## 命中高亮的高对比档 + 检索结果键盘光标（归档 / 模型统计）（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 高对比档的命中底色混合比例低于标准档（明亮 22% < 45%，暗色 30% < 55%） | `frontend/src/js/core/reading-contrast-css.test.mjs` |
| 底色变淡后靠 2px 下划线保持可辨识；命中文字用最强色（正文色 / 纯白） | 同上 |
| 通用光标工具：任意容器收集 `data-cursor-key`、同步高亮（只亮当前行）、滚进可视区、可自定义 Enter 提示 | `frontend/src/js/features/file-list/result-cursor.test.mjs` |
| 归档面板接上光标：↑↓ 移动、Enter 展开、Esc 清空、计数显示位置、重画后恢复光标 | `frontend/src/js/features/diagnostics/archive-search.test.mjs` |
| 模型统计接上光标（三个视图同一套；可展开表头用 `role="button"` 定位，不按类名硬编码） | `frontend/src/js/features/diagnostics/model-stats-search.test.mjs` |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

```text
命中高亮（暗色主题，Mod 列表搜「vpk」，读的是真实渲染出来的 <mark>）
  标准档：background = primary / 0.55 · box-shadow 下划线 1px · 文字 rgb(255,255,255)
  高对比：background = primary / 0.30 · box-shadow 下划线 2px · 文字 rgb(255,255,255)

模型统计键盘光标（3 个夹具 Mod）
  ↓        → 第 1 / 3（ak47.vpk），全表只有 1 行带光标类
  ↓        → 第 2 / 3（katana.vpk）
  ↑        → 回到第 1 / 3
  Enter    → 该行 aria-expanded=true（真的展开了）
  输入 ak47 → 计数「匹配 1 / 3 个 Mod」，光标复位
  Esc      → 检索清空、3 行恢复、光标清空
```

### 只能人工验证的部分

1. **归档面板的光标导航**：它的入口是系统"选择目录"对话框，自动化打不开，
   所以只做到"纯逻辑 + 接线"验证；手感请你在压缩包管理里试一次（与 Mod 列表同一套）。
2. **高对比档底色变淡后的观感**：真机只取到了计算色，
   "够不够醒目"是主观判断，觉得偏弱可以调 `reading-comfort.css` 里那两个百分比。

### 仍未验证

- `IValidity` 的增量级联传播（唯一"有意留到以后"的一项）。
- `FireAxe.GUI` 的其余文件（视图 / 值转换器 / 设置模型 / 主题）与既有实现一一对过，
  没有可移植的增量，因此不作为待办。

## 快捷键总览 + 模型统计统一检索（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 快捷键表分组齐全、id 不重复、每行有说明 | `frontend/src/js/core/shortcuts.test.mjs` |
| 浮层 HTML 分组渲染且全部转义；悬停提示与浮层同源 | 同上 |
| **表里写的每个键位在代码里真有绑定**（Ctrl+K/F、Ctrl+±/0、?、↑↓、Enter、Esc、双击标题栏） | `frontend/src/js/core/shortcuts-wiring.test.mjs` |
| `?` 键不抢输入框；命令面板有「查看键盘快捷键」且复用标题栏按钮 | 同上 |
| 搜索说明书里的快捷键段是从总表派生的（不再手写第二份） | `frontend/src/js/core/command-palette-ui.test.mjs` |
| 模型统计检索语义：跨字段普通词（含模型路径）、`-排除`、`re:`、`tag: 有模型/无模型/估算`、计数、高亮参数 | `frontend/src/js/features/diagnostics/model-stats-search.test.mjs` |
| 模型统计面板接上检索（搜索 / 计数 / 高亮 / 同源说明书 / 正则错误文案） | 同上（接线断言） |
| 归档面板改用共享匹配层后语义不变 | `frontend/src/js/features/diagnostics/archive-search.test.mjs`（9 项，回归保护） |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

```text
快捷键总览
  标题栏按钮打开 → 可见、aria-expanded=true、3 组（全局 / 搜索框里 / 窗口与浮层）、11 行
  Esc → 关闭；按 `?`（焦点不在输入框）→ 打开；点窗口外 → 关闭
  在搜索框里按 `?` → 不打开（不抢键，问号能正常打）
  命令面板搜「快捷键」→ 命中 show-shortcuts → Enter → 打开的是同一个浮层

模型统计检索（3 个夹具 Mod：ak47.vpk / katana.vpk / materials-only.vpk）
  初始：3 行、无计数、无高亮
  ak47        → 计数「匹配 1 / 3 个 Mod」，标题与文件名各 1 处 <mark class="search-hit">
  vpk -katana → 0 行，空状态「没有匹配「vpk -katana」的 Mod」
  tag:无模型   → 命中 3 / 3（夹具是假 VPK，读不到模型，状态一致）
  re:[        → 计数栏「正则表达式无效：…」
  清空        → 恢复 3 行、无高亮
```

### 只能人工验证的部分

1. **总览浮层在窄窗口下的观感**：样式是 `min(28rem, 92vw)` 右对齐，
   实际拖小窗口时的换行与遮挡请你扫一眼（自动化只验证了内容与开关行为）。
2. **模型统计在真实 2400+ Mod 结果上的响应**：自动化用的是 3 个夹具，
   真实扫描结果下每次输入都重画工具栏（已做焦点恢复），手感请你确认。

### 仍未验证

- `IValidity` 的增量级联传播（唯一"有意留到以后"的一项）。

## 第二十六轮：对照表机器核对 + 命令面板入口 + 窗口几何重置（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 窗口几何一键重置（清前缀键 + 复位已注册窗口） | `frontend/src/js/core/floating-modal-geometry.test.mjs` |
| 命令表含「重置所有窗口的位置与大小」、空查询提示含快捷键 | `frontend/src/js/core/command-palette.test.mjs` |
| 标题栏按钮与命令注册接线 | `frontend/src/js/core/command-palette-ui.test.mjs` |
| 对照表引用的仓库内文件全部存在（74 个） | 一次性核对脚本（见 `fireaxe-parity.md` §5.9），脚本用完即删 |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建）

```text
标题栏按钮：存在=true → 点击后命令面板可见=true
空查询提示：共 17 条命令 · ↑↓ 选择，Enter 执行，Esc 关闭 · 快捷键：Ctrl+K 命令面板 / Ctrl+F 搜索 / Ctrl+±/0 缩放
重置命令：预置 lytvpk.floatingPos.lightweight-check → 检索「重置」→「重置所有窗口的位置与大小」
          → Enter → 该键被清掉、面板关闭
再次 Ctrl+K → 快捷键提示仍在 → Esc 关闭
```

### 只能人工验证的部分

1. **标题栏图标的观感**：现在用的是放大镜图标（与"搜索"同义）。如果你希望它更像"命令"（例如
   `⌘` 或键盘图标），改 `frontend/index.html` 里 `#command-palette-btn` 的 SVG 即可（一处）。
2. **命令面板是否要收进快捷键总览页**：目前快捷键列在面板底部；若你希望单独一个"快捷键"浮层，
   可以在命令表里再加一条命令（表 + 动作映射各一行）。
3. **重置窗口几何的实际场景**：自动化验证的是"记忆键被清掉 + 面板关闭"，
   你在真实多显示器环境下把窗口拖乱后点一次，感受是否一步到位。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。

## 第二十五轮：命令面板 + 忙碌状态进界面（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 命令表完整性（7 个页面 + 关键动作、id/标题不重复）与模糊检索排序（标题前缀 > 包含 > 顺序命中 > 关键字） | `frontend/src/js/core/command-palette.test.mjs` |
| 光标移动（环绕、无光标时从两端进入）与状态文案 | 同上 |
| 命令面板接线：Ctrl+K、↑↓/Enter/Esc、点外部关闭、与命令表一一对应 | `frontend/src/js/core/command-palette-ui.test.mjs` |
| 忙碌文案、轮询器订阅事件、样式中灰掉哪些按钮 | `frontend/src/js/core/file-operation-busy.test.mjs` |
| 忙碌状态事件在无 Wails 上下文时安全（测试/无界面调用） | `internal/app/file_op_gate_test.go` |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建）

```text
忙碌状态（400MB 打包，耗时 783ms）
  打包中：提示可见=true、文案「正在处理文件（移动 / 删除 / 打包）… 完成后可继续」、
          body.is-file-operation-busy=true、移动按钮 pointer-events=none
  打包后：提示隐藏=true、class 移除
  事件探针（直接监听 file_operation_state）：[true, false]
命令面板
  Ctrl+K → 面板可见、输入框聚焦、列出 10/16 条、底部「共 16 条命令 · ↑↓ 选择，Enter 执行，Esc 关闭」
  检索「冲突」→「冲突分析（开关）」+ 1 处高亮；检索「字号」→「界面设置（文字大小 / 阅读舒适度）」；
  检索「关于」→ Enter → 面板关闭、当前页面 = about；再开 → Esc → 关闭
```

### 只能人工验证的部分

1. **命令面板的命令取舍**：现在 16 条。哪些你更常用、还想加哪些（例如"随机轮换一次""打开喷漆工具"），
   只在 `command-palette.mjs` 的表 + `app-runtime.js` 的动作映射里各加一行即可（测试会检查一一对应）。
2. **Ctrl+K 与其它软件的习惯冲突**：如果你更希望用 `Ctrl+P` 或 `F1`，改 `app-runtime.js` 里那一处判断即可。
3. **忙碌时的按钮灰化范围**：现在灰掉 7 类会写磁盘的按钮；若你觉得某个按钮不该灰（例如想允许并行打开面板），
   改 `reading-comfort.css` 里 `body.is-file-operation-busy` 那段选择器。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。

## 第二十四轮：文件操作互斥 + 失败逐项报告（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 闸门被占时：移动 / 带冲突动作的移动 / 删除 / 打包全部被挡下，且文件零改动 | `internal/app/file_op_gate_test.go` |
| 释放闸门后操作照常；修复流程（内部会解包+打包）不被自己锁死 | 同上 |
| 失败文案：前 3 条 + "另有 N 条"、只有计数无详情、计数缺失兜底 | `frontend/.../move-result-format.test.mjs` |
| 列表页与归档页都改用逐项文案（不再只显示 errors[0]） | 同上（含源码断言） |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建）

```text
空闲时 busy=false
后台启动 300MB 目录打包 → 轮询到 busy=true
  此时调用移动 → 「另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试」
打包结束 → busy=false
把两个 Mod 移动到"其实是文件"的目标路径 → failCount=2、errors 长度 2，
  两条各自带文件名与系统原因（The system cannot find the path specified）
收尾核对：两个真实 Mod 仍在原位（True / True）—— 被挡下与被拒绝的操作零改动
```

### 只能人工验证的部分

1. **真实的长任务观感**：自动化用 300MB 打包验证了"忙碌时被挡下"，但真实场景里
   （例如批量移动 200 个 Mod 时再点删除）的提示是否够醒目、要不要顺手把按钮也禁用，
   需要你自己用一次再定；`IsFileOperationBusy()` 已经导出，界面随时可以接。
2. **失败文案的取舍**：现在列前 3 条 + "另有 N 条，详见控制台日志"。如果你更希望弹窗列出全部，
   改 `move-result-format.mjs` 的 `DEFAULT_FAILURE_LIMIT` 或换成应用内弹窗即可（纯前端，一处）。
3. **是否需要给"打包"单独排队**：当前是"被挡下并提示"，也可以改成"排队等待"；
   这属于交互取舍，按你的实际使用习惯定。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。

## 第二十三轮：浮动窗口全覆盖 + 搜索语法说明书（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 必须支持浮动的 19 个管理窗口都在注册表里 | `frontend/src/js/core/floating-window-coverage.test.mjs` |
| `browser-modal` / `workshop-modal` / `server-modal` 是页面而非弹窗，不得注册浮动 | 同上（同时断言 ui-shell 里确实有 embedModalAsPage） |
| 9 个临时对话框刻意不注册浮动 | 同上 |
| 搜索语法说明书：覆盖面、转义、悬停提示同源、开合接线 | `frontend/.../search-help.test.mjs` |
| 窗口尺寸钳制与双击重置 | `frontend/src/js/core/floating-modal-geometry.test.mjs` |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建）

```text
窗口审计（19 个注册项逐个查 DOM）
  全部：存在=true、是弹窗=true、有浮动按钮=true
  "详情"显示 已浮动=false —— 因为上一轮真机我点过它的「停靠窗口」，偏好被按窗口记住（说明记忆生效）
详情窗口：浮动=true、容器可穿透=true、窗口可点=true、按钮在 modal-header、
          尺寸 820×702（视口 1400×900，在视口内）；点按钮 → 浮动=false 且文案变回「浮动窗口」
搜索语法说明书：初始隐藏 → 点击打开（aria-expanded=true、按钮高亮）；
  三段 12 行、含 re:^ak 示例；宽 480px 在视口内；点外部 → 关闭；再开 → Esc → 关闭
```

### 只能人工验证的部分

1. **19 个窗口逐个真实使用**：自动化只证明"注册了、DOM 里是真弹窗、有按钮、能开合"，
   每个窗口浮动后的**布局观感**（例如服务器面板的表格在窄浮窗里是否好读）需要你自己用一遍；
   哪个窗口你觉得不该浮动，点它的「停靠窗口」即可，偏好会记住。
2. **说明书的措辞**：8 条语法与 4 个快捷键的文案是否够清楚；改文案只需改
   `frontend/src/js/features/file-list/search-help.mjs`（界面与测试同源，改一处即可）。
3. **详情窗口的默认浮动行为**：Mod 详情现在默认浮动（方便对照列表），如果你更希望它默认居中模态，
   点一次「停靠窗口」即可记住；或把该 id 从 `app-runtime.js` 的列表里移到"不浮动"一档。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。

## 第二十二轮：组名唯一 / 子组问题上报 / 搜索结果键盘导航（2026-09-26，打包 EXE + 沙箱）

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 组名唯一性：自动加序号、撞名报错、改成自己名字允许、超长名截断 | `internal/app/mod_group_names_test.go`、`TestModStrategyGroupNamesEndToEnd` |
| 子树缺失汇总：父/子/孙三层逐层汇总、放回文件后清空、ParentID 透出 | `internal/app/mod_group_subtree_missing_test.go` |
| 子树缺失文案与"本组缺失"文案区分 | `frontend/.../group-view.test.mjs`、`strategy-group-batch.test.mjs` |
| 键盘光标：环绕、空列表、脏数据、位置文案、渲染与快捷键接线 | `frontend/.../result-cursor.test.mjs` |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建）

```text
组名唯一性：同名建组 → 「重名测试」/「重名测试 (2)」
            把第二个改名成「重名测试」→ 已有同名策略组「重名测试」：换个名字，或先重命名那一个
            改成它自己的名字 → 通过
子树提示：父组行「⚠️ 子组里有 3 个缺失文件（涉及 2 个子组）：展开子组即可看到」
键盘导航：搜「医疗箱」→「匹配 7 / 2408 个 Mod」；↓↓ →「… · 第 2 / 7 个结果（Enter 打开详情）」
          光标行 = 第 2 条结果；Enter → 详情弹窗标题与光标行一致；Esc → 输入/光标/计数全部复位
清理：脚本创建的 4 个测试组已删除，沙箱残留 0
```

### 只能人工验证的部分

1. **子树缺失提示的真实数据场景**：真机那次的数据源是打桩的（不能为了测试去删用户真实 Mod），
   聚合逻辑由 Go 端到端测试保证；真实场景（你把某个子组成员的文件删掉/移走）下的观感需要你自己看一眼。
2. **键盘导航的手感**：↑↓ 环绕、Enter 打开详情、Esc 复位是否顺手；想改按键位置在
   `app-runtime.js` 的搜索快捷键块里改一处即可。
3. **重名策略是否符合预期**：建组自动加序号、改名撞名报错——如果你更希望改名也自动加序号，
   改 `mod_group_edit.go` 里那一处 `findModStrategyGroupNameConflict` 分支为 `uniqueModStrategyGroupName` 即可。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。

## 第二十一轮：搜索语法 + 字号档位全局生效（2026-09-26，打包 EXE + 沙箱）

**用户要求**：继续找值得学 FireAxe 的；同时让检索更方便、匹配更"明确"。

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 搜索语法解析（Go 与前端**共用用例**，两边必须一致） | `internal/app/search_query_test.go` + `frontend/.../search-syntax.test.mjs`（读同一份 `testdata/search_query_cases.json`） |
| tag 的 或 / 且 / 排除语义、普通词多词"且"、正则、非法正则 | `internal/app/search_query_test.go` |
| 语法真的作用在搜索上（走导出方法 `SearchVPKFiles`） | `TestSearchVPKFilesSupportsQueryGrammar` |
| 高亮：多词合并、正则命中、零长度正则不死循环、转义安全 | `frontend/.../search-match.test.mjs` |
| 命中字段 chip 支持 `tag:`（标成一级/子标签）且排除条件不产生 chip | 同上 |
| 非法正则的计数文案 | 同上 |
| 渲染层接线（语法解析 → 高亮/计数/chip） | `frontend/.../search-match-wiring.test.mjs` |
| 除标题栏外不再有硬编码 px 字号（守卫） | `frontend/src/js/core/font-size-units.test.mjs` |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建）

```text
tag:其他       → 匹配 910 / 2408；chip「匹配：一级标签」
-tag:其他      → 匹配 1498 / 2408（正好是 910 的补集）；无 chip
re:^!!医疗箱   → 匹配 6 / 2408；12 处 <mark data-match="regex">；chip「匹配：标题 · 文件名」
re:[          → 「正则表达式无效：Invalid regular expression: /[/i: Unterminated character class」；0 行
清空搜索       → 计数归零
```

真机过程中发现"正则只筛不亮"（高亮只跟普通词走），已并入高亮后复测得 12 处。

### 只能人工验证的部分

1. **语法的记忆成本**：`tag:` / `-tag:` / `re:` 这套写法是否顺手，只能你自己用一段时间再判断；
   想改默认行为（比如让多个 `tag:` 之间变成"或"）改 `search_query.go` 与 `search-syntax.mjs` 各一处即可，
   用例文件是共享的，两边一起改。
2. **正则的性能体感**：当前实现是"对每个 Mod 的每个字段跑一次正则"，2408 个 Mod 的实测搜索在 1.5 秒内返回；
   若你写很复杂的正则（回溯爆炸），体感可能变慢 —— 这条只能靠实际使用观察。
3. **文字大小的主观档位**：四档里哪一档最舒服，仍然只能自己定。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。

## 第二十轮：下载暂停/断点续传 + 界面可读性与检索体验（2026-09-26，打包 EXE + 沙箱）

**用户要求**：把 FireAxe 剩下的"下载暂停/续传"学完；同时让界面更灵活易操作、
文字大小与颜色更适合阅读、检索更方便且"匹配得明确"。

### 本次已经自动化覆盖的部分

| 语义 | 覆盖测试 |
| --- | --- |
| 检查点清洗与校验（越界/重复/任务 ID/体积/块大小/损坏 JSON 一律拒绝续传） | `internal/app/download_block_checkpoint_test.go` |
| 完整下载字节一致、续传**只请求缺失区块**、暂停保留临时文件与断点、残留文件被截断时强制重下 | `internal/app/download_resume_test.go`（本地 Range 服务器，4 个区块的用例） |
| 暂停/继续/重试/取消状态机，以及"重启后仍是 paused" | 同上 |
| 阅读舒适度档位、根字号合成（界面缩放 × 文字档位）、行高、未知值回落 | `frontend/src/js/core/reading-comfort.test.mjs`、`internal/app/reading_comfort_config_test.go` |
| 命中区间（连续优先 / 逐字兜底）、转义安全、命中字段、计数文案 | `frontend/src/js/features/file-list/search-match.test.mjs` |
| 窗口尺寸钳制（超屏记忆值 / 非法值 / 极小视口兜底） | `frontend/src/js/core/floating-modal-geometry.test.mjs` |
| 设置页与渲染层的接线（控件、写配置、启动恢复、chip、计数、快捷键） | `reading-comfort-settings.test.mjs`、`search-match-wiring.test.mjs` |
| 任务列表暂停/继续按钮与状态样式 | `frontend/src/js/features/downloads/task-list-pause.test.mjs` |

### 本次真机验证（打包 EXE + 临时调试桥，验收后已删除并重建产物）

```text
阅读舒适度（设置 → 界面设置）
  切换前：textSize=standard comfort=standard 根字号 100% 行距 1.65 --text-primary #f1f5f9
  切到「特大 + 柔和」：textSize=xlarge comfort=soft 根字号 125% 行距 1.8 --text-primary #cbd5e1
  控件数量：文字大小 4 档、舒适度 3 档

检索（Mod 管理页搜索「医疗箱」）
  Ctrl+F 聚焦 = true
  结果：匹配 7 / 2408 个 Mod；46→7 行；14 处 <mark class="search-hit">
  chip 文案：匹配：标题 · 文件名
  Esc：输入框清空、计数归零
  截图确认高亮与 chip 在界面上可见（此前 chip 被标题的省略号裁掉，已改到标签区）

窗口几何（策略组管理窗口）
  写入记忆值 5000×4000 后重开：实际恢复 1344×846（视口 1400×900），标题栏可见
  双击标题栏：该窗口的记忆几何被清空
```

### 只能人工验证的部分

1. **真实大文件的暂停/继续体验**：自动化用的是本地 Range 服务器（12–20MB 夹具）验证"字节一致 + 不重下已完成区块"，
   真实工坊大文件（几百 MB~GB）的暂停响应速度、进度条回退表现仍需你自己下一次大的确认。
2. **长时间阅读的主观舒适度**：字号四档与柔和/高对比三档的"哪种最舒服"是个人偏好，只能自己用一段时间再定。
3. **换屏幕后的窗口位置**：自动化验证了"超屏尺寸会被钳制"，但多显示器之间来回移动的观感仍需人工确认。

### 仍未验证

本轮没有"计划内但完全未验证"的项目。
