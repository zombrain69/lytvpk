# 手工验证清单（本地维护者用）

自动化检查（`go test ./...`、`node --test`、`npm run build`、`wails build`、
`npm run docs:build`、`go vet ./...`）覆盖不到真实点击流程。
本文列出需要在运行中的应用里逐项确认的功能，按“操作 → 预期”编写。
证据分级：**已自动验证**（本文第一部分与各节"已经自动化覆盖的部分"）、
**只能人工验证**（各节"待人工验证"）、**仍未验证**（当前没有此类项）。

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

- [ ] 在 Mod 管理页勾选 3 个 Mod，到“设置 → 游戏配置 → 策略组”填名称、选“互斥单选”并建组。
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
- [ ] 到“设置 → 游戏配置 → 策略组”给某个组填权重 `-1` 保存，回到加载顺序弹窗点“按分层应用”。
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

- [ ] 在“设置 → 游戏配置 → 策略组”里把两个组设置成父子关系。
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

发现不符时，请记录：操作步骤、界面截图、`addonlist.txt` 前后内容，以及应用日志中的报错行。
这些信息足以定位是后端语义、绑定参数还是前端渲染的问题。
