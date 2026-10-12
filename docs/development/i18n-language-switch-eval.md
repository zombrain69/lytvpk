# 语言切换入口 + 第二语言目录：可行性评估（路线图第 14 项）

> 结论先行：**i18n 的"机器"已经全部就位**（运行时、目录、静态节点全量回放、3677 条文案全量迁移完毕），
> 语言切换缺的只剩三块：① **多语言目录装载**（小改，几十行）；② **切换后动态界面重绘**（主要工作量，
> 机制可完全复用现有 `app:page-change` 模式）；③ 两个**范围决策**（en 文案产出、标签值与后端文案怎么处理）。
> 无技术障碍、无架构阻力；工作量集中在"逐页接线 + en 机翻产出"。**是否实施属产品决策**（见 §7 决策点）。
>
> 本评估数据基于 `2.7.1-community.67`（`cc79e92`）+ W9 轮次（`17bdd10`），核查命令见 §8，均可复跑。

## 1. 现状盘点：已就绪的部分

| 资产 | 位置 | 状态 |
| --- | --- | --- |
| 纯函数层：取 key / 回退链 / 插值 / 切换 | `frontend/src/js/core/i18n.mjs` | ✅ 支持**多目录**（`catalogs` map）+ **回退链**（当前语言 → `zh-CN` → key 本身）；`setLocale` / `availableLocales` / `hasLocale` 齐备；`i18n.test.mjs` 已用假 en 目录覆盖多目录回退与切换（5 项测试） |
| 运行时：`t()` / `setLocale()` / `applyTranslations()` | `frontend/src/js/core/i18n-runtime.js` | ✅ `setLocale` 已接 `document.lang` + **全量静态节点重放**（启动与切换共用一条路径）；五类属性（text / title / placeholder / aria-label / alt） |
| 目录装配（浏览器） | `frontend/src/js/core/i18n-setup.js` | ⚠️ 只装 `zh-CN` 一份（`installCatalog(zhCN, "zh-CN")`）——**这是"多语言装载"要改的点** |
| 文案目录 | `frontend/src/messages/zh-CN.json` | ✅ **3677 条叶键 / 约 54,336 汉字**，14 个一级组（settings 638 / mods 580 / strategyGroups 551 / toolbox 477 / servers 296 / conflicts 194 / app 174 / loadOrder 170 / modals 165 / search 132 / workshop 126 / downloads 98 / update 41 / about 35） |
| 迁移完成度 | 第 10/12/13/11 项 | ✅ **界面字面量全量迁移完毕**（静态节点 + 模块动态文案二十二批 + shell 导航）；剩余仅为有意的"数据/契约值"豁免（工坊 Steam 标签、搜索语法 token、后端契约值等，见 `i18n.md`「已迁移页面」各批"豁免"注记） |
| 一致性测试 | `i18n-catalog.test.mjs`、`i18n-static-nodes.test.mjs` | ✅ 正向（`t()` key 必须存在）+ 反向（`index.html` 静态节点 key 必须存在 + 回退原文逐字对齐） |
| 测试注入 | `core/i18n-test-catalog.mjs` | ✅ 单测装真实 zh-CN 目录；纯逻辑测试可继续断言中文 |

原实现注释里写的「等所有页面迁移完再暴露入口」这一前置条件，**已经满足**（第 13 项二十二批收口，2026-10-12）。
语言切换的实施方案（含决策点与拆包）见本文 §5–§7；实施属产品决策，**待用户拍板后再开工**。

## 2. 缺口 A：多语言目录装载（小改）

- `installCatalog(messages, locale)` 目前是**单目录**签名；而 `createI18n` 本身已支持多目录 + 回退链。
  需要：新增 `installCatalogs({ "zh-CN": zh, en })`（或放宽 `installCatalog` 接受 catalog map），
  `i18n-setup.js` 一次性装载全部随包目录。
- **启动顺序**（现状）：`main.js` 最先同步导入 `i18n-setup.js`（装 zh-CN）→ `initializeApp()` 里
  异步 `initConfig()`（读 `GetAppConfig`）→ …… → `applyTranslations()`。若语言选择持久化在
  `config.json`（推荐，见 §6），则启动时先装全部目录、`initConfig()` 之后立即 `setLocale(config.locale)`
  ——首帧前生效；`index.html` 静态骨架在 JS 前仍是中文（早于 JS 的一瞬，可接受，与现状一致）。
- 目录命名：拟 `frontend/src/messages/en.json`（locale 串 `en`；将来需要区分区域再加 `en-US`）。
  加第二份目录**不影响现有测试**：`i18n-catalog.test.mjs` 只查 zh-CN 的 key 存在性；
  `i18n.test.mjs` 用假目录不读磁盘。需新增的测试见 §5。

## 3. 缺口 B：切换后的动态界面重绘（主要工作量）

**原理**：`setLocale()` 只做两件事——换 locale + `applyTranslations()`（静态节点）。
所有"渲染期用 `t()` 取文案"的动态面（列表卡片、筛选条、目录下拉、面板内容……）**不会**自动重画，
切完语言这些区域仍是旧语言，直到下次重渲染。这就是要盘的核心缺口。

### 逐页盘点（重绘入口几乎全部现成）

| 页面 / 面 | 动态内容 | 现成重绘入口 | 现状 |
| --- | --- | --- | --- |
| **mods 默认页** | 文件列表卡片 / 命中计数 | `performSearch()` → `renderFileList()`（纯内存态，不触发扫描） | ❌ **不在任何重绘路径**（`app:page-change` 的 `mods` 分支只调 `updateClassicSecondaryTagsCollapse`），需新接线 |
| | 标签筛选条 | `renderTagFilters()` | ❌ 同上（切换页回来也不重画，但内容静态性高） |
| | 目录下拉列表 | `renderDirectoryList()` | ❌ 同上 |
| | 批量操作栏 / 选择计数 | 模块内渲染（随 `renderFileList` 更新） | 由 `renderFileList()` 覆盖 |
| 工坊页 | 侧边分类 / 卡片列表 | `renderWorkshopSidebar()` + `loadWorkshopList()` | ✅ 已有 `app:page-change` 回调（app-init.js） |
| 下载页 | 任务列表 | `refreshTaskList()` | ✅ 已有回调 |
| 服务器页 | 服务器表 | `renderServers()` + `refreshAllServers()` | ✅ 已有回调 |
| 设置页 | 整页（JS 渲染） | `renderSettingsPageWithDeps()` | ✅ 已有回调（app-runtime.js） |
| 工具箱 | 工具卡片 | `renderDiagnosticsPage({...})` | ✅ 已有回调 |
| 关于页 | hero / 更新状态 | `renderAboutPage({...})` | ✅ 已有回调 |
| 常驻 chrome | 侧边栏菜单 / 导航 | 重建节点带 `data-i18n*`，`applyTranslations()` 即可 | ✅ 静态重放已覆盖（第 11 项） |
| 打开的弹窗 / 浮动窗 | 静态骨架 | `applyTranslations()` | ✅ 自动重放 |
| | 动态内容（列表、计数、正文） | 无（打开时构建） | ⚠️ **重开生效**（v1 建议接受，见下） |
| Toast / 命令面板 | 瞬时 / 打开时构建 | 无 | ⚠️ 同"重开生效"语义，接受 |

### 推荐机制（与现有模式对齐，不引入新抽象）

仿照 `app:page-change`：**`setLocale()` 内额外派发 `app:locale-change` 事件**（`document.dispatchEvent`），
各模块把现有 `page-change` 回调**并列监听**该事件（同一批重绘函数、同一副作用基线），mods 页补一条新监听：

```
app:locale-change →
  列表/命中计数: performSearch()      标签条: renderTagFilters()
  目录下拉: renderDirectoryList()     其余页: 复用各自 page-change 回调
```

- 事件语义 = "语言变了，按你的入口重画"，模块自己决定重画范围；`applyTranslations()` 仍由 `setLocale` 统一先跑（静态面）。
- **测试可行性**：事件派发/监听是纯逻辑（可 node 测）；逐页最终效果走 CUA 沙箱人工核对（登记 `manual-verification.md`）。
- **风险**：重绘函数必须幂等、不触发 I/O 副作用。核对过：`performSearch/renderTagFilters` 只读内存态；
  workshop/servers 的重绘本就跑在 page-change 路径上，行为基线不变。

### 备选方案：整页重载（`location.reload()`）

实现约 10 行（切换 → 写配置 → reload），零缺口（新语言完整重启 UI）；代价：回默认页、关全部弹窗、
丢未提交表单状态；Wails 下重载只重启 WebView（后端不动，扫描走缓存，速度可接受）。
**取舍**：定向重放（推荐，体验连续）vs 整页重载（最小实现，适合"先能用"）。二者可组合：v1 上重载，
后续按缺口清单升级为定向重放。

## 4. 缺口 C：数据型字符串（产品决策，不是 catalog 问题）

### 4.1 标签值（影响面最大的可见缺口）

Mod 标签（主标签 + 子标签）是**后端规则产出的封闭词汇表**，前端按原值直接展示
（`render.js` 的 `file.primaryTag` / secondary tag chips，无 `t()`——i18n.md 记为"数据，非界面文案"）。

实测词汇表规模（`--export-tag-baseline` 全量，3017 Mod，2026-10-12）：

- **主标签 36 个**去重（武器 1082 / 其他 1033 / 人物 628 / 地图 58 / 动画版 48 + 角色名与"Milfy"类内容名）；
- **子标签 197 个**去重（贴图 2677 / 模型 2082 / UI 1688 / 声音 1239 / HUD 961 / 所有枪械 816 / 幸存者 703 / 近战 370 / 音画场景 320 …），其中 **67 个纯 ASCII**（UI、VScript 等，天然免翻）。

选项：ⓐ **不做**（en 界面下标签仍为中文，作为"数据"解释）；ⓑ **前端映射层**（zh→en 表，与目录一起维护；
标题/搜索高亮等处同步）；ⓒ 后端目录化（大改，不建议）。
若选 ⓑ，注意**与搜索语法联动**：`tag:贴图` 类检索按标签值匹配（Go `internal/app/search_query.go` ↔ 前端
`search-syntax.mjs` 双端镜像），en 用户输入英文标签需要映射回中文值再检索——这是单独一块工作量，建议拆项（P2）。

### 4.2 后端文案（错误 / 提示的混合来源）

后端（`internal/app`、`internal/parser`）大量中文字符串会经错误链/返回值进入界面
（粗估 **~900 处**含中文的 `fmt.Errorf/errors.New/return "..."` 行，含错误、返回值与提示；未逐条审）。

选项：ⓐ v1 **接受**（错误路径低频；模板文案已是 i18n，嵌入的 `{error}` 明细为中文）；
ⓑ 高频错误做映射表（同 `action-explanation` 思路）；ⓒ 后端目录化（工程大，不建议）。
推荐 ⓐ + 观察（en 用户实际反馈决定是否升级到 ⓑ）。

### 4.3 持久化文案

少量按"创建时语言"落盘的数据型文案（如策略组新建默认描述）——已登记为语义（数据不随切换），保持即可。

## 5. 目标语言与 en 文案产出

- **规模**：3677 key / ~54k 汉字 → 英文约 85–100k 字符。机翻（LLM 分块 + 术语表 + 校验）**一轮可出初稿**。
- **产出管线（拟）**：zh-CN.json 分块 → 带术语表的翻译提示 → 产出 `en.json` → 自动校验 → 人工抽查。
  自动校验项：key 集合一一对应（无多无缺）、`{placeholder}` 集合一致、非空、**中文残渣扫描**
  （白名单：示例 JSON、专名、有意保留值）、JSON 结构等深。
- **术语表要点**：L4D2 游戏术语（战役 / 角色 / 武器别名）、"Mod / VPK / addonlist" 等保留词、
  界面术语一致性（如"加载顺序"、"策略组"）。
- **长度风险**：英文一般比中文长（按钮 / 标签条 / tooltip 溢出）——逐页抽查优先看按钮与筛选条；
  CSS 已有 `is-long` 类等处（如卡片标签 >16 字符缩略）可复用。
- **产出方式选项**：ⓐ LLM 机翻 + 抽查（推荐：快、可重复）ⓑ 社区翻译（需先有目录结构与贡献文档）
  ⓒ 暂不做。首版建议 en（潜在受众最大）。

## 6. 入口位置与持久化

- **入口**：设置 →「界面」panel 新增一行「界面语言 / Language」（下拉：简体中文 / English；可选加
  「跟随系统」= `navigator.language` 匹配）。切换交互：立即生效 + 写配置 + toast 说明
  「部分已打开的窗口重新打开后生效」。
- **持久化**：`internal/app/config.go` 的 `AppConfig` 增 `Locale *string \`json:"locale"\``
  （指针 + 加载默认 `"zh-CN"`，与 `readingComfort` / `displayMode` 同模式；未知值归一化回默认）。
  前端 `core/config.js` 的 `normalizeConfig` 同步默认值。
- **启动接线**：`initializeApp()` 的 `initConfig()` 之后、各页渲染之前 `setLocale(config.locale)`；
  目录装载改为一次性装全部随包目录（§2）。
- **窗口标题**（P3）：主窗口标题由 Go 端写死（`main.go` `"LytVPK Community Fork — MOD管理器"`），
  不随前端语言；可保持双语或后端读配置，属细节。

## 7. 实施拆包与决策点

| 阶段 | 内容 | 依赖 |
| --- | --- | --- |
| **P0** | 多目录装载 API + `en.json` 占位（键齐平、缺翻回退中文）+ key/占位符一致性测试 + 设置页入口 + `locale` 持久化 + `applyLocaleFromConfig` | 无（**任何路线都需要的公共前置**） |
| **P1** | `app:locale-change` 事件 + 逐页重绘接线（mods 页 3 处 + 其余页复用现有回调）+ 单测 + 沙箱逐页核对 + `manual-verification.md` 登记 | P0 |
| **P2** | en 文案机翻产出与抽查；标签值映射层（含搜索语法联动）；（可选）低频打开窗口的主动重绘 | P0/P1；翻译质量需人工抽查 |
| **P3** | 本地化排序（`localeCompare("zh-CN")` 硬编码 ≥10 处，属排序细节）、窗口标题、长文本溢出收尾 | P0 |

验证口径照常：`go test ./...`、`go vet ./...`、前端显式清单 `node --test`、`npm run build`、docs 构建；
真机验证只走 CUA 沙箱（`launch-cua-sandbox.ps1`）登记到 `manual-verification.md`。

**决策点（需用户拍板）**：

1. **目标语言**：首版就是 en？还是其它（繁中 / 日语）或先不做？
2. **标签值**：接受 en 界面下标签仍为中文（ⓐ），还是做映射层（ⓑ）？若做，搜索语法联动是否纳入首版？
3. **打开中的弹窗**：接受"重开生效"（v1 省一半工作量），还是要求即时重绘？
4. **切换实现**：定向重放（体验连续、工作量中）还是整页重载（最小实现）起步？

> 四个决策点均已有结论（见路线图「用户决策记录」2026-10-12）：首版 en；标签值不做映射层；
> 弹窗接受「重开生效」；实现走定向重放。

## 实施进度

- **P0 已完成（2026-10-12，commit `b537b3a`）**：多目录装载（`installCatalogs` + 启动一次装
  zh-CN/en）+ `en.json` 起步目录（缺翻回退）+ `locale` 双端持久化（`normalizeLocaleKey` ↔
  `core/locale.mjs`）+ 启动应用 + 设置页语言行（受 `LANGUAGE_SWITCH_ENABLED` 控制，保持隐藏）
  + en 一致性测试（key 对齐 / 占位符一致 / 无中文残渣 / 规模下限）。
- **P1 已完成（2026-10-12，commit `6c7ccd9`）**：`setLocale` 在语言变化时派发 `app:locale-change`（启动应用持久化
  语言传 `{ notify: false }` 静默）；逐页重绘接线只接「没有重进自愈路径」的面 —— mods 页
  （列表/命中计数、标签条、目录下拉、排序按钮）、工坊页（侧边分类 + 缓存卡片
  `rerenderWorkshopBrowserFromCache`，不触发网络请求）、设置页（整页重画 + 保持当前分区）；
  其余页（下载/服务器/工具箱/关于）每次进入都由 `app:page-change` 整页重建，不额外接
  （避免语言切换触发后端/网络查询）。契约测试 `features/locale-change-wiring.test.mjs`；
  真机逐页效果已走 CUA 沙箱核对（双向真实点击；记录见 `manual-verification.md`
  「语言切换动态重绘 P1」）。**入口开关仍保持 false，P2 收口后翻转。**
- **P2 已完成（2026-10-12）**：en 3750/3750 全量机翻（分块并行 + 术语表 + 占位符/key/残渣校验 +
  verbatim 白名单）；语言切换收尾（卡片渲染签名纳入界面语言；搜索说明书 / 快捷键总览 / 冲突与
  策略组弹窗 / 分组筛选 / 轮换按钮 / 分类侧边栏等一次性填充面在切换时重建）；CUA en 全量视觉抽查
  记录见 `manual-verification.md`。**剩余：P3（本地化排序与长文本溢出）+ en 人工审校（迭代）→
  入口开关翻 true。**

## 8. 附录：本轮核查命令（均可复跑）

```powershell
# 目录规模（key 数 / 分组）
python -c "import json
def walk(o,p=''):
    if isinstance(o,dict):
        for k,v in o.items():
            yield from walk(v,(p+'.' if p else '')+k)
    else:
        yield p
d=json.load(open('frontend/src/messages/zh-CN.json',encoding='utf-8'))
print(len(list(walk(d))))"

# 标签词汇表规模：全量基线（--export-tag-baseline）里逐 Mod 收集 primaryTag / secondaryTags
# 去重计数 —— 2026-10-12 实测 36 / 197（67 个纯 ASCII）。基线导出命令见 mod-tag-recognition-v2.md。

# 没有任何 UI 调用方会用 setLocale（入口尚不存在）
rg -n "setLocale" frontend/src/js --glob '!*.test.mjs' --glob '!core/i18n*'

# 硬编码 zh 排序 collator 分布（P3 参考）
rg -n 'localeCompare\([^)]*"zh' frontend/src/js --glob '!*.test.mjs'

# 后端中文串量级（粗估；含错误/返回值，未逐条审）
rg -n --glob '!*_test.go' 'fmt\.Errorf\("[^"]*[\p{Han}]|errors\.New\("[^"]*[\p{Han}]|return "[^"]*[\p{Han}]' internal/app internal/parser | wc -l   # ≈904
```

相关实现位置速查：运行时 `frontend/src/js/core/i18n-runtime.js`；装载 `core/i18n-setup.js`；
入口挂点 `features/settings/settings-page.js`（界面 panel）+ `core/config.js`；
重绘接线 `features/app-init.js`（workshop/downloads/servers）与 `features/app-runtime.js`
（settings/diagnostics/about，`app:page-change` 监听在 `setupSettingsAndAboutListeners`）。
