# 工坊「批量退订」与 `.url` 快捷方式：可行性评估（路线图第 7 项）

> 评估日期：2026-10-12（Hermes 定时轮次，用户不在场）。
> 结论摘要：`.url` 快捷方式生成 —— 可直接实施，**已于同日实现**（见 CHANGELOG「未发布 — 工坊快捷方式（.url）生成」）；
> 批量退订 —— 自动执行依赖三处前提（Steam 会话、不可逆确认、信任模型），**未实施**，
> 三条技术路径的核查证据与推荐顺序见下文，等待用户在场时决策。

## 1. 背景与参考实现要义

参考实现：`sHartet/l4d2-addons`（clone 到临时目录研究，不进仓库；关键文档 `reference/steam-api.md`）。
它的整理流程在"把工坊 VPK 搬出 `workshop\`"之后会**主动退订**——因为只要订阅还在，
下次启动游戏 Steam 会把搬走的 VPK 再下一遍，整理等于白做；删目录 / 改只读都没用。

从参考实现提炼出的事实（这些是设计任何 LytVPK 实现的硬前提）：

- **执行接口**：`POST https://steamcommunity.com/sharedfiles/unsubscribeall/`，
  form 参数 `sessionid` + `appid=550` + `filetype=18`，返回 `{"success":1}`。
  页面源码里 `UnsubscribeAll()` 的 `appid` 是写死的 → 只清本游戏，不会波及其它游戏。
- **双源核对**：
  - 本地镜像 `<Steam>\userdata\<accountid>\ugc\550_subscriptions.vdf`（退订成功后缩到只剩
    `appid` + `time_last_updated`；`time_last_updated` 可能是占位值，只认有没有 `publishedfileid` 条目）；
  - 服务器端 `https://steamcommunity.com/my/myworkshopfiles/?browsefilter=mysubscriptions&appid=550`
    （必须是 `/my/` 前缀；不带 `/my/` 的筛选写法已失效）。
- **账号级、不可逆**（Steam 原文「此操作无法撤销！」）：每次执行前停下来说明
  「退哪些（ID + 标题 + 总数）/ 为什么 / 怎么回退（= 重新订阅；`.url` 快捷方式可直达工坊页）」；
  授权**不跨会话**。回退死角：`result=9` 的已删除条目与手工文件没有可回退的入口。
- **副作用实测**：不动 addons 里已整理的 Mod；`workshop\` 空目录保留。

本项目现状（真机只读探针，2026-10-12）：`550_subscriptions.vdf` **957 条订阅**、
`addons\workshop\` **947 个 VPK** —— 两边基本对得上（用户目前处于"订阅与本地一致"状态；
批量退订的价值要在下一次整理搬移之后才出现，这正是参考实现把退订放在流程末尾的原因）。

## 2. 技术核查：会话从哪来（全自动执行的唯一前提）

退订 POST 需要一个已登录的 steamcommunity 会话（`sessionid` + 登录 cookie）。
LytVPK 要"全自动"，就必须先拿到会话。三条路径逐一核查：

### 路径 A：应用内 WebView2 登录（在应用里登录 Steam，再在页面上下文里 POST）

核查结论：**技术可行，但需要用户拍板信任模型。**

- Wails v2.10.2 **没有多窗口 API**。在本机模块缓存
  `go/pkg/mod/github.com/wailsapp/wails/v2@v2.10.2` 全量检索：不存在 `WebviewWindow` /
  `MultiWindow` / `WindowCreate` 一类的接口（`pkg/windows` 目录不存在）；`runtime.WindowExecJS`
  只能对**当前唯一窗口**注入脚本、且**没有返回值通道**（`window.go` 绑定只存在于应用自己的页面里，
  跨源页面拿不到）。
- 因此"应用内登录"存在三种落法：
  1. **主窗口接管（成本最低）**：把唯一窗口导航到 `steamcommunity.com` 登录页 → 用户登录完成后，
     轮询 `WindowExecJS` 注入「检查 `window.g_sessionID` → 同源 `fetch` POST →
     把结果通过 `location.href = <本应用地址>#结果` 回跳」的自包含脚本。可行，但 UX 生硬
     （整个应用画面被浏览器页面"替换"、回跳即重载、内存态丢失），开发/生产两种 AssetServer
     地址要分别处理。
  2. **自建第二个 WebView2 原生窗口**（依赖树里已有 `go-webview2`）：自写窗口类 + 消息循环 +
     `CookieManager` 读取，工程中等偏大、纯 Windows 代码、需要真机联调。
  3. **升级 Wails 大版本**（换取官方多窗口）：框架级变更，风险大，不建议为单一功能做。
- 信任模型：凭据由 Steam 官方登录页收集（应用不经手），但"在 mod 管理器里登录 Steam"
  这个动作本身需要用户认可；登录一次后 cookie 持久化在 WebView2 用户目录，后续执行免登录。
- 若实施：无论哪种落法，都必须遵守参考实现的纪律 —— 执行前弹清单确认（ID + 标题 + 总数 +
  不可逆说明 + 回退方式），且只在用户当前会话明确授权后执行。

### 路径 B：读取本机 Steam 客户端会话（htmlcache Cookies + DPAPI）

核查结论：**技术可绕行，但不推荐。**

- 实测阻断：Steam 运行中 `%LOCALAPPDATA%\Steam\htmlcache\Default\Network\Cookies`
  **被独占锁定** —— `cp` 与 `Copy-Item` 均报「文件正由另一进程使用」
  （本机 steamwebhelper ×7 进程在跑）；要读需先关 Steam（或走卷影副本 / 备份权限，均非应用应有行为）。
- 加密本身：`Local State` 的 `os_crypt.encrypted_key` 前缀为 DPAPI（`v10`），与本项目
  现有 DPAPI 用法（`secret_windows.go`）同源、理论上可解。
- 代价：会话 cookie 属用户隐私面（读它等于"替用户登录"）；Steam 客户端升级可能更换
  存储位置与加密方案（Chromium 新版有 app-bound 加密路线）；"关 Steam 才能读"的流程很糟。
- 结论：**不推荐**；如果未来 Valve 提供官方接口再议。

### 路径 C：引导模式（只读清单 + 打开订阅页，人工在浏览器完成退订）

核查结论：**可行、零账号风险；唯一待定的是 UI 形态（产品决策）。**

- 不需要任何 cookie：
  1. 读 `550_subscriptions.vdf`（只读）得到订阅 ID 全集；
  2. 批量详情接口（项目已有 `fetchSteamPublishedFileDetails`，官方 + worker 回退）补标题；
  3. 与本地库交叉比对：`workshop\` 里还在 = "未整理"；已移走 = "已搬走但订阅还在"（重点提醒对象）；
     本地没有 = "订阅未下载"；
  4. 界面提供「打开我的订阅页」（默认浏览器，用户通常已登录）+ 逐项工坊链接；
     退订动作由用户在浏览器里点（或继续交给 agent 流程）。
- 与参考实现的差异：参考实现由 agent 浏览器全自动执行；本项目的定位是"看清有什么"，
  执行保持人工/半自动——双源核对里的服务器端来源（`/my/myworkshopfiles/`）也需要登录态，
  同样只能由用户的浏览器提供。
- 本项目可先行复用的基建（已核查）：
  - `internal/app/steam_locations.go` 的 Steam 安装目录发现（注册表 + `libraryfolders.vdf`）；
  - `userdata\*\ugc\550_subscriptions.vdf` 的存在性可作为"哪个账号在玩 L4D2"的判据
    （本机 8 个账号目录里只有 1 个有 550 的订阅文件）；
  - `runtime.BrowserOpenURL`（打开订阅页 / 条目页）。

## 3. 结论与建议

| 子项 | 结论 | 状态 |
| --- | --- | --- |
| `.url` 快捷方式生成 | 可行、低风险 | ✅ 已实现（本期，解析链同详情面板 `resolveWorkshopID`） |
| 批量退订 · 引导模式（路径 C） | 可行、零风险 | ⬜ 待用户确认 UI 形态后实施 |
| 批量退订 · 应用内登录（路径 A） | 技术可行（主窗口接管成本最低） | 🚦 待用户决策（信任模型） |
| 批量退订 · 读本机会话（路径 B） | 可绕行但脆弱、涉隐私 | ❌ 不建议 |

**给用户的三个决策点**

1. 是否需要「订阅清单」视图（路径 C）？如果要，放在哪里：MOD 管理页筛选（按"订阅状态"）、
   工具箱新卡片、还是设置 → 工坊设置？
2. 是否接受"在 LytVPK 里登录 Steam"（路径 A）？如果接受，选主窗口接管（快但生硬）
   还是自建 WebView2 子窗口（体验好但工程大）？
3. 自动执行一旦实现，纪律固定：每次执行前带清单确认（ID + 标题 + 总数）、说明不可逆与回退方式、
   授权不跨会话 —— 这部分不设选项，是硬要求。

## 4. 真机只读探针记录（2026-10-12，均可复跑）

```text
# Steam 安装目录（注册表，双视图）
reg query "HKLM\SOFTWARE\WOW6432Node\Valve\Steam" /v InstallPath
→ 本机 Steam 安装目录（实际路径不入库，以下用 <Steam> 代称）

# 订阅镜像（只读）
<Steam>\userdata\<accountid>\ugc\550_subscriptions.vdf：957 条 publishedfileid
（其余 7 个 userdata 账号目录没有 550_subscriptions.vdf）

# 本地库对照（只读）
addons\workshop\：947 个 .vpk

# 会话文件可达性实测（只读尝试，失败是结论的一部分）
cp "…\Steam\htmlcache\Default\Network\Cookies" → 「Device or resource busy」
Copy-Item 同文件 → 「文件正由另一进程使用，因此该进程无法访问此文件」
（steam.exe / steamwebhelper ×7 运行中）

# Wails 能力核查（模块缓存）
go/pkg/mod/github.com/wailsapp/wails/v2@v2.10.2：无 WebviewWindow/MultiWindow/WindowCreate；
runtime.WindowExecJS 存在（仅注入、无返回通道）
```

## 5. `.url` 快捷方式（本期已实现部分）

- 入口：MOD 管理页右键（单文件 / 多选）→「生成工坊快捷方式（.url）」。
- 生成 `<同名>.url`（与 VPK 同目录），内容为 Windows InternetShortcut 三行格式
  （`[InternetShortcut]` / `URL=` / `IconIndex=0`，CRLF），与参考实现同款；双击直达工坊页。
- 解析链与详情面板 `resolveWorkshopID` 同序（.meta → workshop 纯数字文件名 → addonURL0），
  解析不出即跳过，不猜测；幂等（同页不动、异页改写）；`.url` 作为旁车随重命名/移动/删除。
- 实现与测试落点：`internal/app/workshop_shortcut.go` + `workshop_shortcut_test.go`（12 项）、
  `frontend/src/js/features/file-list/workshop-shortcut.mjs` + 测试（7 项）；
  真机验证项见 manual-verification §20。

## 5. 补充调研（2026-10-12 晚）：Steam API 路线的重新评估

> 背景：用户要求评估"涉及 Steam API"的实现方式——要便捷、且尽量少 Steam 接口问题。

**结论 1：官方 Web API 没有订阅 / 退订能力。**
公开 Web API（developer.valvesoftware.com / partner.steamgames.com/doc/webapi）只有只读查询
（GetDetails / GetCollectionDetails 等）；订阅 / 退订只存在于本机 **Steamworks SDK** 接口里。

**结论 2：官方本机接口存在且语义正好——ISteamUGC（Steamworks SDK）**
- `GetNumSubscribedItems` / `GetSubscribedItems` / `SubscribeItem` / `UnsubscribeItem`
  （官方文档确认：这正是游戏内订阅管理 UI 用的同一套接口）。
- 调用条件：**Steam 客户端运行中** + `steam_api64.dll` + steam_appid 上下文
  （`steam_appid.txt` 或 `SteamAppId` 环境变量 = 550）。
- 社区先例：`geekhostuk/SteamWorkshopManager`（C++ CLI：任意 appid 的批量退订 / 备份 / 恢复）；
  Go 侧有 `hajimehoshi/go-steamworks`，但该库现版本**未暴露 UGC 接口**（需扩展或自写 purego 绑定，量不大）。
- **硬约束（Valve SDK Access Agreement）**：`redistributable_bin` 内的 `steam_api64.dll`
  仅允许"随 Licensee 自己的软件（Steamworks 伙伴发行物）"分发 —— **第三方工具不能随包分发**。
  实践路径 = 用户自备或从本机其它 64 位 Steam 游戏目录借用（本机实测：多个已安装的 64 位
  Steam 游戏各自携带一份可用 DLL；L4D2 自带的是 32 位 `steam_api.dll`，64 位进程无法加载）。
- 待实测项：非 Steamworks 进程以 550 init 的接受度；`UnsubscribeItem` 的 EResult 与回执；
  副作用观察（期间 Steam 好友列表可能短暂显示"正在运行 L4D2"）。

**结论 3：网页会话端点仍是可行备选（原路径 A）。**
`POST steamcommunity.com/sharedfiles/unsubscribe {id, appid, sessionid}`（逐项）与
`sharedfiles/unsubscribeall`（按游戏批量）—— 非官方文档化端点，但被社区 userscript 使用十余年，
间接证明长期稳定；风险点集中在 session 获取与批量限流。参考项目（sHartet/l4d2-addons）走此路。

**推荐实施顺序（待用户确认）**：
1. **P0 引导模式**（原路径 C，零接口）：订阅清单视图（只读本机 `550_subscriptions.vdf`）
   ＋ 智能筛选（失效 / 冗余订阅）＋ 一键打开 Steam 官方退订页 —— 信息层先行，零风险。
2. **P1 官方接口 spike**：只读验证（init 550 + GetSubscribedItems 与 VDF 逐条对照），
   通过后再评估 DLL 自备 UX 与单条写验证。
3. **P2 网页会话**：若 P1 不通过再启动（工程最大、涉信任模型）。

**只读 spike 预案**（可执行清单）：
- 从本机任一 64 位游戏复制一份 `steam_api64.dll` 到隔离目录（仅验证用，不入库、不随包）。
- 最小 Go 程序：purego（或扩展 go-steamworks）→ `SteamAPI_Init`（SteamAppId=550）
  → SteamUGC → `GetSubscribedItems` → 打印数量与 ID 片段 → `Shutdown`。
- 与 `550_subscriptions.vdf`（本机实测 957 条）核对一致性与差异。
- 零写操作、零账号影响；结果记录后再决定是否进入写验证。


**实施进度（2026-10-12 晚）**：**P0 引导模式已实现并真机冒烟通过**（工具箱「工坊订阅清单」：
只读解析 `550_subscriptions.vdf` + 本地分类 + 筛选/多选 + Steam 官方退订页引导；全程零写入）。
P1（ISteamUGC 只读 spike）与 P2（网页会话，备选）待后续编排。
