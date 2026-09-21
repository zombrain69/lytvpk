# Mod 策略组（Strategy Group）设计

> 参考来源：FireAxe（Apache-2.0）`AddonGroup.EnableStrategy`
> （`None / Single / SingleRandom / All`）。FireAxe 能直接改子节点状态，
> 因为它的 addon 布局由自己维护；LytVPK 直接管理游戏真实文件，
> 因此本设计把策略落到**显式动作**上，而不是长期隐式约束。

## 1. 目标与非目标

目标：

1. 把一组 Mod 保存为命名策略组，并记录策略：
   `single`（组内只保留一个开启）/ `single_random`（随机保留一个）/
   `all`（全部开启）/ `off`（全部关闭）。
2. 组内成员按 addonlist 键记录，跨 root / workshop / disabled 目录都能定位。
3. 应用策略时只改这组成员的开关：已存在的条目原位改值，未记录的成员在
   “开启”时按既有未记录条目插入规则补写；**不改动其它 Mod，也不重排顺序**。

非目标（本阶段不做）：

- 默认不做隐式约束。FireAxe 会在用户单独开关某个成员时自动改同组其它成员；
  LytVPK 把它做成**每个策略组单独开启的“自动联动”**（默认关闭），
  且只联动一层、不做跨组级联，避免重叠分组互相冲突或形成循环。
- 不把策略组接入随机轮换（`rotation.go`）与启用方案（Profiles）的快照，
  见第 6 节未决项。

## 2. 数据模型

```go
type ModStrategyGroup struct {
    ID          string                    `json:"id"`
    Name        string                    `json:"name"`
    Description string                    `json:"description,omitempty"`
    Strategy    string                    `json:"strategy"`
    Members     []ModStrategyGroupMember  `json:"members"`
    CreatedAt   string                    `json:"createdAt"`
    UpdatedAt   string                    `json:"updatedAt"`
}

type ModStrategyGroupMember struct {
    Key  string `json:"key"`  // normalizeAddonListKey 归一化键
    Name string `json:"name"` // 写入 addonlist 时的显示名（保留原始大小写）
}

type ModStrategyGroupApplyOptions struct {
    Strategy string `json:"strategy"` // 覆盖策略组自身策略；空 = 按配置
    PickKey  string `json:"pickKey"`  // single：指定保留的成员
}

type ModStrategyGroupApplyResult struct {
    GroupID    string   `json:"groupId"`
    GroupName  string   `json:"groupName"`
    Strategy   string   `json:"strategy"`
    Enabled    []string `json:"enabled"`
    Disabled   []string `json:"disabled"`
    PickedName string   `json:"pickedName"`
}
```

## 3. 存储

配置目录 `groups.json`，结构 `{ "groups": [...] }`，复用
`readJSONFile` / `writeJSONFile`，独立互斥锁串行化读写。

## 4. 应用语义

1. `all` / `off`：组内成员全部设为 `1` / `0`。
2. `single`：恰好保留一个成员。
   - 指定了 `PickKey`：保留该成员（不在组内则报错）。
   - 未指定：优先保留当前已启用的第一个成员；若都不在启用状态，取第一个成员。
3. `single_random`：在组内随机保留一个成员。
4. 写入规则（与 `SetVPKGameEnabled` 保持一致）：
   - 开启：`replaceAddonListValueWithPlacementAndName`（未记录条目按
     `unrecordedModLoadOrderPlacement` 插入）。
   - 关闭：`replaceAddonListValueWithName`（只改已存在条目，不新建条目）。
5. 只有内容真正变化时才写盘；写盘后刷新缓存状态并同步受保护快照。
   `writeAddonListDocument` 自身会触发既有的短时保护备份。
6. 整个过程持有 `addonListGuardMu`，与监控线程、其它写入路径互斥。

## 5. API

| 方法 | 作用 |
| --- | --- |
| `ListModStrategyGroups()` | 读取策略组列表 |
| `CaptureModStrategyGroup(name, description, strategy, memberPaths)` | 用选中的 Mod 路径建组 |
| `ApplyModStrategyGroup(id, options)` | 应用（或覆盖）策略 |
| `DeleteModStrategyGroup(id)` | 删除策略组 |

## 6. 测试策略与未决

测试：路径到成员的映射（含 workshop 与 disabled 目录、去重）、名称/成员/策略校验、
持久化与删除、`all`/`off` 对已存在与未记录成员的行为、`single` 的指定与自动挑选、
`single_random` 恰好保留一个、顺序不被重排。

未决：

- 是否让“随机单选”复用随机轮换的游戏启动时机。
- 是否需要跨组级联联动（当前只应用第一个命中的联动组，且不级联）。

已解决：策略组（与依赖声明）可以随启用方案一起保存与恢复——在“启用方案”里勾选
“同时保存策略组与依赖”即可；未勾选的方案不会改动本地的策略组与依赖。
