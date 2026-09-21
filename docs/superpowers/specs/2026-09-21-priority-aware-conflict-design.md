# 优先级感知 Mod 冲突检测（目标模式）设计

> 参考来源：FireAxe（Apache-2.0）`FireAxe.Core/AddonConflictUtils.cs`。
> 该实现只在**同一优先级内**统计 VPK 路径重叠：不同优先级之间的重叠属于用户
> 有意的覆盖，不算冲突。本设计把同一洞察适配到 LytVPK 的"总顺序"模型。

## 1. 背景与问题

- 现状：`internal/app/conflict.go` 只按归档内路径是否重复判定冲突
  （`fileFirstOwner` + `conflictOwners`），完全不知道加载顺序。
- 后果一：用户刻意用高优先级 Mod 覆盖另一个 Mod 时，结果仍然报"冲突"。
- 后果二：结果不告诉用户"谁生效"，用户要自己回列表看加载顺序。
- 文档侧证据：`docs/toolbox/conflict-check.md` 写明冲突检测的目的就是
  "列出这些覆盖关系"。当前实现只列了重叠，没有给出覆盖结论。

与 FireAxe 的差异：FireAxe 的 addon 优先级是**用户显式指定的数值**，同一数值内
先后次序未定义，所以同优先级重叠 = 真冲突。LytVPK 的 addonlist 提供的是**总顺序**，
任意两个已记录 Mod 都能判定胜负，因此"无法判定胜负"的情形只剩：参与者不在
addonlist 中（未分层）、或参与者所在条目被标记为未加载。

## 2. 目标与非目标

目标：

1. 新增可选的"优先级感知"分析模式：把重叠结果分流为**覆盖关系**与**冲突**。
2. 覆盖关系给出胜者及其加载顺序号；冲突只保留真正无法判定胜负的重叠。
3. 支持可扩展忽略清单（内置规则不变，新增调用方传入的清单）。
4. 未开启该模式时，现有行为与结果结构完全不变。

非目标（本阶段不做）：

- 不引入 FireAxe 式显式数值优先级字段（留待阶段 3，视实测结果决定）。
- 不改文件布局、不改 `addonlist.txt` 写入语义、不改默认分析范围。
- 不改 `disabled/` 目录语义与筛选基线语义。

## 3. 术语

| 术语 | 含义 |
| --- | --- |
| 参与者 participant | 提供同一归档内路径的某个 VPK |
| 顺序号 order index | 该 VPK 对应条目在 `addonlist.txt` 中的 0 基位置 |
| 可判定 decided | 所有参与者顺序号已知且互不相同 |
| 未判定 undecided | 至少一个参与者顺序号未知 |
| 覆盖 override | 可判定的重叠，胜者 = 顺序号最大者 |
| 冲突 conflict | 未判定的重叠，胜负无法确定 |

## 4. 语义规则（目标模式）

1. `addonlist.txt` 中值为 `0` 的参与者**不参与**判定（游戏未加载该 Mod）。
2. 顺序号未知的参与者**仍视为已加载**（与 `GameStateKnown` 语义一致），
   但它会让该重叠变为"冲突"——这是唯一保留的冲突来源。
3. 过滤后参与者不足 2 个 → 不产出任何结果。
4. 其余情况：可判定 → 覆盖组（含胜者）；否则 → 冲突组。
5. 严重度分级沿用 `getConflictSeverity`，覆盖组同样按严重度、文件数排序。
6. 胜者方向集中在 `conflictOrderWins` 一个函数内（见第 8 节待实测项）。

## 5. 数据结构

```go
// 选项（Additive，前端不传即为关闭）
type ConflictAnalysisOptions struct {
    TargetPaths   []string
    BaselineRules []ConflictBaselineRule
    MatchMode     string
    PriorityAware bool     `json:"priorityAware"`
    IgnoreFiles   []string `json:"ignoreFiles"`
}

// 结果（Additive）
type ConflictVPKFile struct {
    Name, Path, Title, Location string
    Order int `json:"order"` // addonlist 顺序号；-1 = 未记录
}

type ConflictOverrideGroup struct {
    VpkFiles       []ConflictVPKFile `json:"vpk_files"`
    Winner         ConflictVPKFile   `json:"winner"`
    Files          []string          `json:"files"`
    FileCount      int               `json:"file_count"`
    FilesTruncated bool              `json:"files_truncated"`
    Severity       string            `json:"severity"`
}

type ConflictResult struct {
    TotalConflicts int
    ConflictGroups []ConflictGroup
    TotalOverrides int                    `json:"total_overrides"`
    OverrideGroups []ConflictOverrideGroup `json:"override_groups"`
}
```

内部类型：

```go
type conflictLoadEntry struct{ Index int; Enabled bool }
type conflictOwner struct{ Path string; Index int; Known, Enabled bool }
type conflictDecisionKind string // "ignore" | "conflict" | "override"
type conflictDecision struct {
    Kind        conflictDecisionKind
    Owners      []conflictOwner
    WinnerIndex int
    WinnerPath  string
}
```

## 6. 算法与数据流

1. 扫描阶段与现在一致：并发解析每个 VPK 的归档文件列表，
   维护 `fileFirstOwner` 与 `conflictOwners`（仅保存有 ≥2 个所有者的路径）。
2. 开启优先级感知时，先用 `readAddonList` 构建顺序表
   （键 = `normalizeAddonListKey(条目名)`，重复条目保留第一条）。
3. 对每个重叠路径的参与者列表：
   `addonListKeyForManagedVPKPathFromRoot` 求出顺序号与开关状态，
   交给纯函数 `decideConflictOwners` 判定。
4. `ignore` → 丢弃；`override` → 累加到覆盖组；
   `conflict` → 用过滤后的参与者集合累加到现有冲突组。
5. 覆盖组与冲突组分别按 `severity`、`file_count` 排序后返回。

关键性质：同一条重叠的参与者集合只会落入冲突组或覆盖组之一，两组键空间天然互斥。

## 7. 兼容性与降级

- `PriorityAware` 默认 `false`：`CheckConflicts`、`CheckConflictsForPaths`
  的默认行为与结果结构不变（新增字段为零值）。
- 新增 JSON 字段是附加字段，旧前端忽略即可；`ignoreFiles` 不传 = 只用内置忽略规则。
- `addonlist.txt` 不存在或读取失败 → 顺序表为空 → 全部未判定 →
  结果与旧行为一致（安全降级，不产生误导性的覆盖结论）。
- 未开启时不读取 addonlist、不构建顺序表，无额外磁盘 I/O。

## 8. 待实测（唯一的语义假设）

L4D2 中 `addonlist.txt` 的顺序方向尚未实测确认：
FireAxe 的 `Push()` 按优先级**降序**写入（越靠前越优先），
而 LytVPK 现有文档写的是"加载顺序越靠后，通常越容易覆盖前面的资源"。

本实现按 LytVPK 现有文档语义（靠后覆盖靠前），并把它集中在
`conflictOrderWins(candidate, current int) bool` 一处；
一旦受控实验（两个含同名文件的小 VPK + 交换顺序）给出结论，改一行即可翻转。

## 9. 测试策略

- 纯函数：未知顺序 → 冲突；顺序已知 → 胜者为最大顺序号；未加载参与者被剔除。
- 忽略清单：精确路径与目录前缀匹配，大小写与分隔符不敏感。
- 集成：真实 VPK 夹具（`writeTestVPK`）+ `addonlist.txt`，
  断言覆盖组胜者、冲突组归属，以及关闭开关时与旧行为一致。

## 10. 阶段进展

- 阶段 1（已完成）：后端优先级感知判定、覆盖关系输出、可扩展忽略清单 API。
- 阶段 2（已完成）：
  - 冲突分析范围弹窗新增“按加载顺序判定胜负”开关，结果区新增
    “已判定覆盖关系”列表（生效 / 被覆盖徽标、沿用严重度筛选、最多 50 组）。
  - 设置 → 游戏配置新增“Mod 冲突分析”卡片：持久化开关与用户可编辑的
    `conflictIgnoreFiles` 忽略清单（每行一个路径，`/` 结尾表示目录）。
  - `conflictPriorityAware` / `conflictIgnoreFiles` 写入 `config.json`；
    忽略清单对所有冲突检测入口生效，弹窗开关与设置页开关共用同一份配置。
- 阶段 3（待做）：可选显式优先级字段（对齐 FireAxe `Priority` /
  `PriorityInHierarchy`），以及 FireAxe 的组启用策略、参考 Mod 方案导出。
- 待实测：addonlist 顺序方向（见第 8 节），以及本阶段新 UI 的真机交互验证。
