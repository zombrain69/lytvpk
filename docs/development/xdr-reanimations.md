# XDR（xdReanimsBase）动作 Mod：机制、优先级与管理要点

本文是 2026-10-07 对 L4D2 第三方动作（re-animation）框架的调研记录，作为
`internal/parser/xdr_slots.go` 与 `internal/app/health_check_xdr.go` 的依据。

## 1. 这是什么

- **XDR = xdReanimsBase**（工坊 `2121557118`，作者 xdshot）：给幸存者与特感装第三方动作替换的**基础框架**。
  它是"Base dependency"——没有它（或它在游戏内被关掉），`models/xdreanims/` 下的动作都不会生效。
- 框架提供 **槽位（slot）系统**：每个角色 **最多 48 个槽**，每个槽可以放一个动作替换；
  没有提供的槽会播原版序列。
- 框架已预定义序列顺序，mod 作者不需要自己重排（原话：*no more of painful sequence reordering for mod makers*）。

## 2. 文件命名约定（作者官方说明）

来自基础包讨论区 "How to create reanimation mods"：

```
$modelname "xdreanims/<CHARACTER_NAME>_slot_<SLOT_NUMBER>.mdl"   // 槽位 1–48，三位数
例：bill 的第 7 槽 → xdreanims/bill_slot_007.mdl
```

- 通常同时有同名 `.ani`（动画数据）。
- 序列名要写成**被替换的原版序列名**（如 `Heal_Self_Standing`），并保留原来的 `activity`
  （如果原 QC 有）。本机实测的 `.qc` 证据：
  `models/xdreanims/decompiled 0.71/rochelle_slot_030.qc` 里是
  `$sequence "Heal_Self_Standing" { ... "activity" "ACT_TERROR_HEAL_SELF" 1 ... }`。
- 角色 token 用的是模型名：`namvet`(Bill)、`biker`(Francis)、`manager`(Louis)、`teenangst`(Zoey)、
  `coach`、`mechanic`(Ellis)、`gambler`(Nick)、`producer`(Rochelle)，以及
  `boomer`/`charger`/`hunter`/`jockey`/`smoker`/`spitter`/`tank`/`witch`/`common` 等。

## 3. 优先级：内部是按什么排的（官方原文）

基础包说明页（"Bugs and Limitations"）与官方指南 "Modify xdR animation addons"：

1. **两个 Mod 替换同一动作（同一序列）时：slot 数字小的那个生效。**
   原文：*"If both mods replace same sequence, the one from replacement with lowest slot number will have priority."*
   指南补充：*"One with currently lowest slot will play anims that both addons replace. You can move another addon
   to any lower slot to play it's anims instead (or higher slot to allow other addon play it's anims)."*
   —— **与 addonlist.txt 的加载顺序、文件时间都无关**，只看槽位号。
2. **同一个角色的同一个槽被两个 Mod 占用：随机生效一个。**
   原文：*"Combining both mods on same character slot won't work. Random one will be picked instead.
   Ask modder to recompile to different slot or unpack and move the anim file to another slot."*
   —— 这是必须处理的冲突（本项目的体检里报 `xdr_slot_collision`）。
3. **移植"整包替换"型（非 xdR）动作 Mod 时，建议放最高槽（044–047）**，
   这样它不会盖掉别人放在具体槽位的动作。
4. 特例：
   - 想替换 Tank 的 `Idle`，序列要命名为 `Idle_xdr`。
   - 原版 Francis / Louis 的攻击动作**内嵌在模型里**，需要装 "Reanims Compatible" 模型
     才能替换 Healing / Rifle 动作。
   - 普通感染者不能**新增**序列（会破坏序列顺序），只能替换已有序列。

### 官方槽位建议表（1–48）

来自讨论区 "Slots designations"（作者原文是 `$definevariable SLOT_xxx NNN`）：

| 槽 | 建议用途 | 槽 | 建议用途 |
| --- | --- | --- | --- |
| 001 | noise（闲置呼吸层） | 025 | CalmRun |
| 002 | Pistol | 026 | LimpWalk |
| 003 | Elites | 027 | LimpRun |
| 004 | Rifles | 028 | Shoot / Grenade throw |
| 005 | Shotgun | 029 | Melee / Sweep / Shove / Stomp |
| 006 | PumpShotgun | 030 | Reload |
| 007 | Grenade | 031 | Rescue（被扶起 / 救援） |
| 008 | FirstAidKit | 032 | Heal_self |
| 009 | SMG | 033 | Heal_Other |
| 010 | HuntingRifle | 034 | Jump / Fall / landing |
| 011 | SniperMilitary | 035 | Ladder |
| 012 | idle_FyringPan | 036 | Incap（倒地） |
| 013 | idle_Bat | 037 | Flinch |
| 014 | idle_Axe | 038 | Shoved |
| 015 | idle_guitar | 039 | gestures（手势） |
| 016 | Chainsaw | 040 | Pounced |
| 017 | idle_standing | 041 | Smoked |
| 018 | idle_crouching | 042 | Crushed |
| 019 | idle_calm | 043 | Ridden |
| 020 | idle_Injured | 044–047 | 其它（移植整包动作建议放这里） |
| 021 | walk | 048 | animfixes（动画修复） |
| 022 | CrouchWalk | | |
| 023 | Run | | |
| 024 | CalmWalk | | |

**这只是建议，不是事实**：真实库里有作者把"蹲姿"动作放进 031（官方建议 Rescue）、
把"治疗"放进 026/030（官方建议 LimpWalk / Reload）。所以界面只能写"官方建议：xxx"，
不能用它断言"这个 Mod 替换了什么动作"。

## 4. 本机实测（2026-10-07，2958 个 Mod）

| 观察 | 数据 |
| --- | --- |
| 带 XDR 槽位证据的 Mod | 68 |
| 同角色同槽冲突（不同 Mod） | 9 组（slot 002 / 008 / 029 / 030 / 040 / 041 / 043 …） |
| 多版本动作包（loader + 变体，无 slot 命名） | 2 个（`3006131197.vpk` 202 个模型、`3006559778.vpk` 13 个模型） |
| 基础包状态 | `xdReanimsBase` 存在，但**在游戏内被关闭** → 68 个动作当时全部没生效 |

## 5. 其它管理器现状（GitHub 调研）

用 GitHub 代码搜索（`gh api search/code`）逐仓库确认：

| 管理器 | XDR / 动作槽处理 |
| --- | --- |
| `ktxiaok/FireAxe`（177★，最主流） | 无（`reanim`/`xdreanims`/`slot` 0 命中）——只做文件级冲突 |
| `Jackzmc/l4d2-addon-manager` | 无 |
| `coffeegrind123/l4d2-mod-manager` | 无（它做的是"同一个文件被多个 addon 覆盖"的通用冲突） |
| `TheCraZyDuDee/L4D2-Mod-Manager`、`DestinyKubbyP/L4D2-Tool` | 无 |
| `fdklgbh/L4D2-Mod-Manager`（Python） | 仅按路径归类：`"动作": {'path': ['models/xdreanims']}` |
| `Starfelll/NekoVpk` | 资源标签库里出现 `xdreanims` 词条，不含槽位语义 |
| 上游 `LaoYutang/lytvpk` | 无（`internal/parser/xdr_parser.go` 是本 fork 新增） |

结论：**槽位级解析与同槽冲突提示目前只有本 fork 有**；官方规则只能从基础包作者自己的
讨论区/指南取得（本文已固化，避免以后再去翻 Steam）。

## 6. 本项目的落地

- `internal/parser/xdr_slots.go`：官方槽位建议表 + `XDRSlotDesignationFor`。
- `XDRSlotInfo` 新增 `slotName` / `slotGroup`（导出清单、详情面板都会带上；详情面板写"官方建议"）。
- `XDRSummary` 文案统一成："同角色同 slot 只会随机生效一个；替换同一动作时 slot 数字小的优先"。
- `internal/app/health_check_xdr.go`（体检）：
  - `xdr_slot_collision`：同角色同槽被多个已启用 Mod 占用（按"槽位 + 冲突 Mod 组合"聚合，一行列出受影响角色）。
  - `xdr_missing_base`：有动作 Mod 但基础包缺失，或基础包在游戏内被关闭。
  - `xdr_variant_pack`：多版本动作包（loader + 变体，深扫描时识别，根/工坊同一份只报一次）。
- `internal/app/xdr_priority.go` + 前端角标：回答"这组动作最后会播哪一个"。
  - 只做确定的判定：同角色同槽多份 → 全部标 `⚠ 动作随机生效`（官方原文 random）；
    槽位唯一 → `▶ 动作生效`；两者混合 → `◐ 动作部分生效 x/y`。
  - `disabled` 目录 / addonlist 记 0 的 Mod 不参与统计；同一份 Mod 的根/工坊副本算一件事。
  - 卡片角标 + 详情面板逐槽标签 + 搜索字段（搜"随机"能直接筛出冲突项）。
  - **不做**跨槽推断：官方"低槽优先"要序列级证据（不同槽、同一序列），本项目扫描拿不到序列名，
    按官方用途表反推会产生误报（实测本机只有 1 组、还是"其它(44–47)"这种无意义分组）。

## 7. 还没做的（后续可继续）

- **序列级冲突**：要做到"不同槽、替换同一序列 → 低槽胜"的精确提示，需要读出 `.mdl` 里被替换的序列名。
  实测 v44+ 的 seqdesc 里标签是**指针**（`baseptr` + `szlabelindex`），且本机编译产物里
  没有明文序列名（`--find=Heal_Self` 在 .mdl 里 0 命中），要精确解析还得继续逆格式或改用 `.qc` 证据
  （只有部分 Mod 带 `.qc`）。
- 搜索语法（如 `slot:heal`）与按槽位排序。
- 一键"把某个 Mod 移到空槽"（需要解包 → 改 `_slot_NNN` → 重打包；本项目的 VPK 解包/打包工具已具备）。
