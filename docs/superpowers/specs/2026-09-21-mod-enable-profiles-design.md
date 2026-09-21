# Mod 启用方案（Profile）设计

> 参考来源：FireAxe（Apache-2.0）的 `.addonroot` 导入导出与“参考 Mod”方案切换。
> LytVPK 目前只有 workaround：用二级标签筛选出一批 Mod，再批量启用/禁用
> （见 `docs/features/mod-management.md` 的“有没有预设功能？”）。

## 1. 目标与非目标

目标：

1. 把当前 `addonlist.txt` 的**开关状态与加载顺序**保存为命名方案。
2. 一键应用方案：先备份当前文件，按原编码原子写入，同步受保护快照，
   并刷新内存中的游戏开关状态。
3. 方案可导出为文件、可导入（便于分享给他人），导入时分配新 ID。
4. 不改变既有语义：不动 `disabled/` 目录规则，不改动 VPK 文件本身，
   不自动启动游戏。

非目标（本阶段不做）：

- 不做“组内互斥/随机单选”这样的策略组（FireAxe 的 AddonGroup 策略）。
- 不引入显式数值优先级字段。
- 不处理方案与随机轮换功能的交互（见第 7 节）。

## 2. 数据模型

```go
// ModEnableProfile 是一份可保存、可切换、可分享的启用方案。
// Entries 的顺序就是目标加载顺序，与 addonlist.txt 的条目顺序一致。
type ModEnableProfile struct {
    ID          string                  `json:"id"`
    Name        string                  `json:"name"`
    Description string                  `json:"description,omitempty"`
    CreatedAt   string                  `json:"createdAt"`
    UpdatedAt   string                  `json:"updatedAt"`
    Entries     []ModEnableProfileEntry `json:"entries"`
}

type ModEnableProfileEntry struct {
    Name    string `json:"name"`    // addonlist 条目名（root 为文件名，workshop 为 workshop\<id>.vpk）
    Enabled bool   `json:"enabled"` // 对应 addonlist 的 "1"/"0"
}

type ModEnableProfileApplyResult struct {
    ProfileID     string `json:"profileId"`
    ProfileName   string `json:"profileName"`
    AppliedCount  int    `json:"appliedCount"`  // 方案内且当前存在的条目
    AddedCount    int    `json:"addedCount"`    // 方案内但当前缺失、被补回的条目
    KeptCount     int    `json:"keptCount"`     // 当前有、方案未记录，保留在末尾的条目
    BackupName    string `json:"backupName"`    // 应用前建立的备份文件名
}
```

## 3. 存储

- 方案库：配置目录下的 `profiles.json`，结构 `{ "profiles": [...] }`，
  复用 `readJSONFile` / `writeJSONFile`，并加独立互斥锁串行化读写。
- 导出文件：`.l4d2profile.json`，内容就是单个 `ModEnableProfile` 的 JSON。

## 4. 应用语义（关键规则）

1. 方案内且当前 `addonlist.txt` 中存在的条目：套用方案的开关，并进入方案顺序。
2. 方案内但当前缺失的条目：按方案补回（使用方案中的名字与开关），计入 `AddedCount`。
3. 当前存在、方案未记录的条目（例如方案保存后新增的 Mod）：**保留原开关**，
   统一排在方案条目之后，计入 `KeptCount`。这样应用方案不会静默禁用新 Mod。
4. 应用前对当前 `addonlist.txt` 建立 `before-profile-apply` 备份；文件不存在时跳过备份。
5. 写入复用 `writeAddonList`（保留原编码/BOM 与换行风格），随后
   `syncManagedAddonListSnapshotLocked`（仅在开启监控时同步受保护快照）
   与 `applyAddonListGameStates`（刷新缓存中的游戏开关状态）。
6. 全过程持有 `addonListGuardMu`，避免与监控线程、其它写入路径并发。

## 5. API

| 方法 | 作用 |
| --- | --- |
| `ListModEnableProfiles()` | 读取方案列表 |
| `CaptureModEnableProfile(name, description)` | 用当前 addonlist 生成方案 |
| `ApplyModEnableProfile(id)` | 应用方案并返回统计 |
| `DeleteModEnableProfile(id)` | 删除方案 |
| `ExportModEnableProfile(id)` | 弹保存对话框，导出到文件 |
| `ImportModEnableProfile()` | 弹打开对话框，导入并分配新 ID |
| `ExportModEnableProfileToFile(id, path)` / `ImportModEnableProfileFromFile(path)` | 无对话框版本，供测试与脚本复用 |

导入时校验：名称非空、至少一条条目、按 `normalizeAddonListKey` 去重（保留首次出现）。
导入始终分配新 ID，保留原名称与描述，避免与本地已有方案冲突。

## 6. 测试策略

- 捕获：条目顺序与开关与 `addonlist.txt` 一致，ID/时间戳被填充。
- 持久化：捕获后由新的 App 实例读同一配置目录仍能看到方案；删除后消失。
- 应用：换序 + 翻开关 + 新增 Mod + 缺失 Mod 的混合场景下，顺序、开关、
  统计数字、备份文件、缓存状态全部符合第 4 节规则。
- 导入导出：导出到临时文件后导入到新的配置目录，条目一致、ID 重新分配。
- 边界：`addonlist.txt` 不存在时捕获报错；导入非法 JSON 报错。

## 7. 未决与后续

- 与“随机轮换”的交互：轮换按标签随机启用，方案是静态快照；两者同时使用时的
  优先级与提示文案待定。
- 前端入口（阶段 2）：方案列表、应用/删除/导入导出按钮、应用前的影响预览。
- 阶段 3：策略组（组内互斥单选、随机单选、全开联动）。
