# LytVPK：`addonlist.txt` 加载顺序与 Mod 启用/禁用调查

调查日期：2026-08-27（编码兼容性补充复核）
调查对象：`LaoYutang/lytvpk`，提交 `c977064445c46ce4c69d1b88c038bb95c9240c9f`
调查方式：使用仓库 `.codegraph` 做调用链调查，并只读核对本机 L4D2 游戏目录、VPK 目录树和文本载荷编码。

## 结论摘要

项目中原有的“物理禁用”和 `addonlist.txt` 的游戏内开关，仍是两套**独立**状态；本次已在不混淆两者的前提下补齐后者。

1. **加载顺序排序**：当前选定的 `addons` 目录的父目录与 `addonlist.txt` 拼接，读取 `"AddonList"` 块中条目的**物理出现顺序**，在前端按这个顺序重排列表。
2. **物理禁用（原有能力）**：程序将 `.vpk` 在 `addons` 根目录和 `addons\\disabled` 之间移动；它的 `Enabled` 字段只表示这一物理位置状态。
3. **游戏内开关（本次实现）**：扫描完成后，程序把 `addonlist.txt` 中各键的 `"1"` / `"0"` 合并为 `GameEnabled` / `GameStateKnown`，支持根目录 VPK 以及 `workshop\\*.vpk`。前端会显示绿/红/灰状态、可直接切换、可按状态筛选。
4. **安全写入范围**：游戏内开关经过保真文档路径，会保留 UTF-8（含 BOM）、UTF-16 LE/BE、GBK 或 Windows-1252/ANSI 编码、注释、缩进、行尾和原始键名，并在首次写入前创建 `addonlist.txt.lytvpk.bak` 原始字节备份。加载顺序写回现在也会识别并保留原编码/BOM 及 `workshop\\` 键前缀，但仍重建条目文本，不承诺保留注释、缩进和行尾。两者都不会移动 VPK。
5. **VPK 文本兼容性**：`addoninfo.txt`/`missions/*.txt` 载荷现在按 UTF-8 BOM、UTF-16 BOM、UTF-8、GBK、Windows-1252 顺序解码；VPK 条目名按 UTF-8、GBK、Windows-1252 解码。打包时优先 GBK，无法表示的名称保留 UTF-8，避免静默改名。
6. **Windows 写回可靠性**：配置写回统一通过 `replaceFile`；Windows 使用 `MoveFileEx(MOVEFILE_REPLACE_EXISTING|MOVEFILE_WRITE_THROUGH)` 替换已有目标，避免 `os.Rename` 在目标已存在时失败。

因此，界面现在会明确区分“文件位置状态”和“游戏内开关”：不要把旧的 `Enabled` 字段或 VPK 所在目录误读为 `addonlist.txt` 的 `"1"`。

## 本次实现的交付范围

```text
ScanVPKFiles()
  └─ applyAddonListGameStates()
       └─ addonlist.txt 的 "0" / "1"
            └─ VPKFile.GameStateKnown / GameEnabled
                 └─ 列表、卡片、状态栏和筛选器

“游戏内开启 / 关闭”按钮
  └─ SetVPKGameEnabled(filePath, enabled)
       └─ 只改 addonlist.txt 对应条目的值（或新增条目）
            └─ 更新前端内存状态并按当前筛选重绘
```

- 列表与卡片上的按钮按绿色（游戏内开启）、红色（游戏内关闭）和灰色（未记录）展示；位于 `addons\\disabled` 的 VPK 不允许写游戏内开关，避免把两套机制混在一起。
- 筛选支持“游戏内开启 / 游戏内关闭 / 未记录”多选，状态栏分别显示已记录开启和关闭数量。
- 排序菜单支持 **VPK 大小** 和 **模型复杂度**。模型复杂度在首次请求排序时按需解析 VPK 中模型的 LOD0 总顶点数，并缓存模型数、总顶点和总三角形，后续排序不重复解析。

## 已核对的本机文件

| 项目 | 结果 |
| --- | --- |
| `addons` 目录 | 本机 L4D2 `left4dead2/addons`，存在 |
| `addonlist.txt` | 本机 L4D2 `left4dead2/addonlist.txt`，存在 |
| 文件大小 | 32,445 B |
| 最后写入时间 | 本次探针未将文件时间作为结论依据 |
| UTF-8 BOM | 无 |
| 编码实测 | 原始字节不是有效 UTF-8；按 GBK（CP936）解码可正确解析 |
| 条目数 / 文件名去重数 | 1,024 / 1,024 |
| 值为 `"1"` 的条目 | 366 |
| 值为 `"0"` 的条目 | 658 |
| SHA-256 快照 | `A60D25AE177563DBA0A9D6ABEED2B1DB957C38E3D67C2B3EA2A24C4DD9D5FB04` |

这验证了 `internal/app/addon_list.go` 的 GBK 分支在当前游戏文件上确实会被使用。

### VPK 与游戏文本编码探针（2026-08-27）

按 `ScanVPKFiles()` 的真实范围扫描：`addons` 根目录直下，以及 `workshop`/`disabled` 递归目录。

| 项目 | 结果 |
| --- | --- |
| VPK 文件 | 1,040 |
| VPK 条目 | 118,994 |
| VPK 条目名 | UTF-8 118,650；GBK 344 |
| `addoninfo.txt` / `missions/*.txt` | 941 |
| 文本载荷 | UTF-8 917；UTF-8 BOM 18；GBK 5；Windows-1252 1 |
| 代表性 ANSI 样本 | `addons\\7hours_later_l4d2.vpk` 内 `missions/7_hours_later.txt` |

该 ANSI mission 文件此前按 GBK 解码会产生替换字符；新增 Windows-1252 回退后，`ParseVPKFileMetadata()` 实测成功得到战役 `7 Hours Later II` 和 5 个章节。当前实际 `addonlist.txt` 为 GBK（32,445 B），`cfg\\autoexec.cfg` 为 UTF-8（2,183 B）。

## 1. 从所选 `addons` 目录到 `addonlist.txt` 的路径链

项目要求用户选择的是 `addons` 目录本身，而不是 `left4dead2` 目录。

```text
前端保存的 lastActiveDirectory / 手动选择 / 自动探测
  └─ SetRootDirectory(path)
       └─ App.rootDir = path               // 例如 ...\left4dead2\addons
            └─ filepath.Dir(App.rootDir)   // 例如 ...\left4dead2
                 └─ filepath.Join(..., "addonlist.txt")
                      └─ ...\left4dead2\addonlist.txt
```

对应实现：

- `frontend/src/js/features/app-init.js:39` 的 `checkInitialDirectory()` 依次尝试前端配置的 `lastActiveDirectory`、旧版 `defaultDirectory`，最后调用 `AutoDiscoverAddons()` 自动查找。
- `frontend/src/js/features/app-init.js:124` 的 `selectDirectory()` 通过 Wails 目录选择框取得目录。
- `frontend/src/js/features/directory-dropdown.js:217` 的 `switchToDirectory()` 支持在已保存目录之间切换。
- 三个入口都会先调用 `ValidateDirectory(path)`，再经过 Wails 绑定调用 `SetRootDirectory(path)`。
- `internal/app/vpk_scan.go:17` 的 `SetRootDirectory()` 仅将路径保存到运行时字段 `a.rootDir`；前端才负责持久化最近使用目录。
- `internal/app/addon_list.go:22` 的 `readAddonList()` 和 `:195` 的 `GetAddonListOrder()` 都使用 `filepath.Dir(a.rootDir)` 与 `filepath.Join(..., "addonlist.txt")` 得到目标文件。

`AutoDiscoverAddons()` 还明确包含 Steam 安装目录下 `left4dead2/addons` 这一候选相对路径（`internal/app/filesystem.go:30`）。

## 2. “加载顺序排序”的完整调用链

```text
“加载顺序排序”按钮
  └─ setupSortEvents()                         sorting.js:30-31
       └─ handleLoadOrderSort()                sorting.js:36
            └─ Wails GetAddonListOrder()
                 └─ App.GetAddonListOrder()    addon_list.go:195
                      └─ 读取并按文本行解析 addonlist.txt
            └─ appState.loadOrderMap[小写文件名] = 0-based 位置
            └─ applySort(appState.vpkFiles)
            └─ renderFileList()
```

### 后端读取与解析

`internal/app/addon_list.go:195` 的 `GetAddonListOrder()`：

1. 检查 `a.rootDir` 已设置，计算 `<a.rootDir 的父目录>\\addonlist.txt`。
2. 读取原始字节；若有 UTF-8 BOM 则去除。
3. 文档读取按 BOM、UTF-8、GBK、Windows-1252 顺序解码；游戏内开关写回使用原编码，加载顺序写回也使用原编码但会重建条目文本。
4. 逐行扫描 `"AddonList"` 块，用正则 `"([^"]+)"\s+"([^"]+)"` 捕获键值对。
5. 将每个**文件名键**按出现次序追加到 `[]string order` 后返回。

注意：该方法忽略第二列的值，`"0"` 与 `"1"` 都会进入 `order`。因此，本机文件中的全部 1,024 条记录都会参与“加载顺序排序”，而不是仅 366 条值为 `"1"` 的记录。

### 前端排序规则

`frontend/src/js/features/file-list/sorting.js:36` 的 `handleLoadOrderSort()` 获取后端数组后，将文件名转小写并建立 `appState.loadOrderMap`，值为 0-based 顺序下标。随后 `applySort()`（`:110`）按以下规则比较：

1. 两个文件均在 `loadOrderMap` 中：按 `addonlist.txt` 中的下标升序。
2. 两个文件均不在映射中：按中文本地化、数字感知的文件名排序。
3. 只有一个文件在映射中：在映射中的文件排在前面。
4. 最终相等时：以完整 `path` 作为稳定的后备比较。

这只是 LytVPK 列表的排序实现；本调查没有将它证明为 Left 4 Dead 2 引擎实际的 VPK 覆盖/加载规则。

## 3. 在 UI 中修改某个 VPK 的“加载顺序”

调用链如下：

```text
列表菜单 / 快捷入口
  └─ openLoadOrderModal(filePath)                   modals/load-order.js:7
       └─ GetVPKLoadOrder(file.name)                addon_list.go:112
  └─ saveLoadOrder()
       └─ SetVPKLoadOrder(file.name, 输入的 1-based 序号)
            └─ readAddonList()
            └─ 删除同名旧条目
            └─ 在截断后的目标位置插入条目
            └─ writeAddonList()                     addon_list.go:96
```

`SetVPKLoadOrder(filename, newOrder)` 的具体行为：

- 比较文件名时忽略大小写，并通过 `filepath.Base()` 去除路径。
- 已存在时先从全部列表移除该同名项，再插到 `newOrder - 1`（限制在 `[0, len(cleanList)]`）。
- 不存在时新建 `{ Name: 文件名, Value: "1" }`，然后插入。
- 最后将整个列表写回。`newOrder` 是对 UI 暴露的 1-based 序号；读回的 `GetVPKLoadOrder()` 也返回 1-based 序号。

`writeAddonList()` 固定输出：

```text
"AddonList"
{
    "<filepath.Base(item.Name)>"        "<item.Value>"
}
```

并用 `os.WriteFile(path, ..., 0644)` 覆盖目标文件。

## 4. 新增：`addonlist.txt` 游戏内开关的实现

### 扫描时的状态映射

`internal/app/vpk_scan.go` 在所有 VPK 完成扫描和缓存后调用 `applyAddonListGameStates()`。该方法通过 `readAddonList()` 读取 `addonlist.txt`，把键名统一为小写反斜杠路径后建立状态表：

| VPK 所在位置 | 用于匹配 `addonlist.txt` 的键 |
| --- | --- |
| `addons\\foo.vpk` | `foo.vpk` |
| `addons\\workshop\\123.vpk` | `workshop\\123.vpk` |

无记录、文件不存在或无法读取时，扫描不会失败；对应项保持 `GameStateKnown=false`，前端显示“未记录”。`"1"` 映射为 `GameEnabled=true`，`"0"` 映射为 `false`。

### 前端与后端切换链

`file-list/operations.js` 的 `toggleGameEnabled()` 在用户点击主列表或卡片的按钮时，通过 Wails 运行时调用 `App.SetVPKGameEnabled(filePath, enabled)`。成功后仅更新该 VPK 的 `GameEnabled` / `GameStateKnown` 内存值，再重新执行当前筛选、排序和渲染。

`SetVPKGameEnabled()` 的写入规则：

1. 仅接受当前扫描缓存中的 VPK；`Location=disabled` 直接拒绝。
2. 按 VPK 相对 `addons` 的路径生成键，因此 workshop 键会保留 `workshop\\` 前缀。
3. 读取原始文件，识别 UTF-8、UTF-8 BOM 或 GBK；若文件不存在则创建最小 `AddonList` 文档。
4. 在已有行中只替换值部分；无对应行则在 `AddonList` 的右花括号前插入新行。
5. 首次修改既有文件前创建 `addonlist.txt.lytvpk.bak`，随后将编码后的内容写入同目录临时文件，再替换目标文件。

这一条路径不会改动条目的相对顺序，不会重建整个文件，也不会移动 VPK。

## 5. 原有的物理 Mod 启用/禁用实现

### 状态的来源：物理位置，而非 `addonlist.txt`

`internal/app/vpk_scan.go` 扫描：

- `a.rootDir`：仅扫描根目录直接包含的 `.vpk`；
- `a.rootDir\\workshop`：递归扫描；
- `a.rootDir\\disabled`：递归扫描。

`getLocationFromPath()`（`vpk_scan.go:241`）根据相对于 `a.rootDir` 的第一层目录返回 `root`、`workshop` 或 `disabled`。`processVPKFileWithCache()` 据此设置：

```text
VPKFile.Enabled = (Location != "disabled")
```

所以当前实现会把 `root` 和 `workshop` 都标为 `Enabled = true`，但 workshop 文件又不允许直接切换，见下一节。

### 后端切换：`ToggleVPKFile`

`internal/app/vpk_actions.go:51` 是实际的单一切换方法，使用 `a.mu` 互斥锁和缓存中的 `VPKFile` 作为状态来源。

| 原状态 | 文件操作 | 新状态 |
| --- | --- | --- |
| `Enabled=true` 且 `Location=root` | 创建 `addons\\disabled`（如缺失），`os.Rename(root\\name.vpk, disabled\\name.vpk)`，并移动同名图片 sidecar | `Enabled=false`，`Location=disabled` |
| `Enabled=false` 且 `Location=disabled` | `os.Rename(disabled\\name.vpk, root\\name.vpk)`，并移动同名图片 sidecar | `Enabled=true`，`Location=root` |
| `Location=workshop` | 返回错误，要求先转移 | 不变 |

移动后，方法删除旧路径缓存，并以新路径重新保存缓存。它**没有调用** `readAddonList()`、`writeAddonList()` 或任何 `addonlist.txt` 写入逻辑。

`MoveWorkshopToAddons()`（`vpk_actions.go:122`）是 workshop 文件的前置操作：将其移动到 `a.rootDir`，再标记为 `Location=root`、`Enabled=true`；之后才可用 `ToggleVPKFile()` 禁用。

### 前端操作编排

所有常规启用/禁用最终都调用 Wails 绑定的 `ToggleVPKFile()`，然后调用 `refreshFilesKeepFilter()` 重新扫描、重新排序、恢复筛选并重绘列表。

| 场景 | 前端入口 | 编排方式 |
| --- | --- | --- |
| 单个文件 | `file-list/operations.js:15` 的 `toggleFile(path)` | 切换一次，刷新列表，显示通知 |
| 批量启用 | `file-list/actions.js:43` 的 `enableSelected()` | 仅选 `disabled` 中的未启用文件；`Promise.all` 并发调用切换；刷新 |
| 批量禁用 | `file-list/actions.js:88` 的 `disableSelected()` | 仅选根目录中已启用文件；`Promise.all` 并发调用切换；刷新 |
| 禁用全部 / 按主标签禁用 | `file-list/actions.js:133` 的 `disableAllMods()` | 弹确认框；逐个顺序调用；刷新 |
| 问题 Mod 查找 | `settings/problem-mod-scan.js` | 在二分排查与最终“禁用问题 Mod”中调用同一切换方法，并刷新 |
| 启动/连接前轮换 | `internal/app/rotation.go:73` | 按人物/武器的官方二级标签确定候选，先禁用、再启用，并触发 `refresh_files` 事件 |

“批量启用/禁用”虽然由前端并发发起，但每次后端 `ToggleVPKFile()` 都持有同一个 `a.mu`，所以真正的文件移动会被后端串行化。

## 6. 启动前的随机轮换

这是普通按钮启用/禁用之外的另一条编排路径：

```text
LaunchL4D2() / ConnectToServer()
  └─ RotateMods()
       └─ rotateModsInternal(config)
            ├─ 收集当前启用的“人物”“武器” Mod 的官方二级标签
            ├─ 每个标签从包含该标签的 Mod 池随机选择一个
            ├─ 先对未选中的当前启用项调用 ToggleVPKFile() 禁用
            ├─ 再对被选中且当前未启用的项调用 ToggleVPKFile() 启用
            └─ EventsEmit("refresh_files", nil)
```

这条逻辑同样只移动根目录与 `disabled` 目录中的文件，不调整 `addonlist.txt` 的条目顺序或值。

## 7. 两套机制的边界与遗留风险

### 已确认的边界

- `GetAddonListOrder()` 的排序数组含 `"0"` 和 `"1"` 两类条目；它不依据值过滤。
- `ToggleVPKFile()` 的启用状态仅由目录位置控制；它不更新 `addonlist.txt`。
- `SetVPKLoadOrder()` 会保留该条目当前的值；只有新建条目时默认写 `"1"`。
- 因此，同一个 Mod 可能出现“列表中按 addonlist 顺序排列、物理上在 disabled 目录而被 UI 标为禁用”的状态。

### 遗留风险

1. **条目顺序不等于已证明的引擎加载规则**：前端的“加载顺序”仅根据文本出现次序，而非本调查对 Source 引擎最终覆盖规则的证明。
2. **加载顺序写回仍会重建格式**：`SetVPKLoadOrder()` 已不再强制改成 UTF-8，也保留 `workshop\\` 前缀，但仍会丢失原有注释、空白布局和行尾风格；如需完全原位排序，仍应实现条目级移动。
3. **解析逻辑仍有轻微重复**：`readAddonList()`、`GetAddonListOrder()` 与保真编辑路径仍分层调用，但编码识别已统一落到文档读取函数。
4. **无 charset 标记的文本存在歧义**：VPK 文本只能按启发式顺序识别；极少数同时可被 GBK 与 Windows-1252 解码的字节序列仍可能需要人工确认。

## 8. 后续可决定的产品规则

在开始修改业务代码前，建议明确以下产品规则：

1. 是否要将旧的“移动至 `disabled`”操作隐藏、重命名为“物理归档”，或在 UI 中增加更醒目的双状态说明？现在两个状态会并存，但不会互相写入。
2. “加载顺序”是否只展示 `"1"` 条目，还是展示全部条目并为关闭项标记状态？
3. 是否将旧的 `SetVPKLoadOrder()` 也迁移到同一套保真、带备份的行级编辑器？
4. 是否需要在游戏内开关写入前显示差异预览，或提供“恢复 `.lytvpk.bak`”按钮？
5. 是否将“游戏引擎的真实加载/覆盖规则”作为独立验证项？当前代码把文件文本顺序当作 UI 排序依据，但源码本身并未证明这就是引擎最终的覆盖顺序。

## 9. autoexec 工具箱与命令来源

- `autoexec.cfg` 编辑器把文件内容统一以 UTF-8 传到 Wails 边界，保存时恢复原编码、BOM 和换行；GBK、Windows-1252、UTF-16 和 UTF-8 BOM 均有测试覆盖。
- 编辑器对每次输入立即分析首个命令 token，列出行号、含义、作用域、风险和来源；异步分析带序列号保护，快速输入时不会用旧结果覆盖新内容。未知命令保留为警告，因为它们可能来自插件或 Mod。
- 内置命令目录对照本机 `readme_l4n.txt`（当前版本记录至 2.44.2）维护，并将已移除的旧命令标为高风险旧版提示；基础 L4D2/Source 命令以 Valve Developer Community 的 L4D2 控制台命令列表及 Source 命令说明为在线核对依据。
- `addonlist.txt` 页面增加保存→备份→监控→删除的生命周期提示、目标文件打开入口，并在监控/未知命令等状态下用警示色强化反馈。

## 关键文件索引

| 目的 | 文件与入口 |
| --- | --- |
| 解析 / 游戏内开关写回 `addonlist.txt` | `internal/app/addon_list.go`：`readAddonList`、`readAddonListDocument`、`applyAddonListGameStates`、`SetVPKGameEnabled`、`backupAddonListDocument` |
| 旧加载顺序读取 / 重建写回 | `internal/app/addon_list.go`：`GetVPKLoadOrder`、`SetVPKLoadOrder`、`GetAddonListOrder`、`writeAddonList` |
| 设置 `addons` 根目录和扫描 VPK | `internal/app/vpk_scan.go`：`SetRootDirectory`、`ScanVPKFiles`、`getLocationFromPath` |
| 按需模型复杂度统计 | `internal/app/vpk_model_metrics.go`：`GetVPKModelMetrics`；`internal/parser/model_stats.go`：`AnalyzeVPKModelStats` |
| 实际启用/禁用与 workshop 转移 | `internal/app/vpk_actions.go`：`ToggleVPKFile`、`MoveWorkshopToAddons` |
| 自动探测路径与启动前轮换触发 | `internal/app/filesystem.go`：`AutoDiscoverAddons`、`LaunchL4D2`、`ConnectToServer` |
| 启动前随机轮换 | `internal/app/rotation.go`：`RotateMods`、`rotateModsInternal` |
| 前端加载顺序、VPK 大小、模型复杂度排序 | `frontend/src/js/features/file-list/sorting.js`：`handleLoadOrderSort`、`handleModelComplexitySort`、`applySort` |
| 前端手动设置顺序弹窗 | `frontend/src/js/features/modals/load-order.js`：`openLoadOrderModal`、`saveLoadOrder` |
| 前端游戏内开关、状态筛选与渲染 | `frontend/src/js/features/file-list/operations.js`：`toggleGameEnabled`；`filters.js`、`render.js`、`events.js`、`state.js` |
| 选择与切换 `addons` 目录 | `frontend/src/js/features/app-init.js`、`frontend/src/js/features/directory-dropdown.js` |

## 验证记录

- CodeGraph 已成功建立索引并在本次改动后同步；新增 `SetVPKGameEnabled`、`applyAddonListGameStates` 和 `GetVPKModelMetrics` 均可被查询到。
- 已通过 CodeGraph 复核上述关键函数、扫描调用链、前端按钮委托和模型统计入口，并在编码改动后重新同步索引。
- 已只读检查本机 L4D2 `left4dead2/addonlist.txt`、`cfg/autoexec.cfg`、1,040 个 VPK 及其文本载荷编码。
- 已新增测试覆盖 GBK、Windows-1252、UTF-8 BOM、UTF-16、VPK 条目名和 mission/addoninfo 解码，以及 `addonlist.txt`/`autoexec.cfg` 原编码保真写回。
- 本地已运行 `go test -count=1 ./...`、`go vet ./...`、`npm run build` 和 `wails build -clean -platform windows/amd64`，均成功。GUI accessibility 点击因系统未提供坐标几何而未完成；尚未实际启动 L4D2 游戏本体验证。
- 本调查及实现过程未修改 L4D2 游戏目录、实际 `addonlist.txt` 或任何 VPK 文件；所有写入行为只在临时测试目录中验证。已尝试启动本地 EXE 做真实 GUI 检查，但桌面自动化 helper 在窗口状态捕获阶段返回 `SetIsBorderRequired failed: 不支持此接口 (0x80004002)`，因此未执行点击操作。
