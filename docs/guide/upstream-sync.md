# 上游同步状态（v2.5.15 → v2.7.1）

本 Fork 从上游 `LaoYutang/lytvpk` 的 **v2.5.14** 分叉，上游此后发布了
**v2.5.15 / v2.6.0 / v2.7.0 / v2.7.1**（共 20 个提交、88 个文件）。

这一页记录我们对每个上游改动的处置：**直接同步 / 按我们的架构改写 / 我们已有且更强 / 暂未同步**，
方便你自己判断"该不该更新"，也方便后续维护者继续对账。

> 判断原则：**上游修的是真 bug → 一定同步；上游新增能力而我们已经做得更完整 → 保留我们的实现**；
> 上游换了一套做法但我们的设计更贴合"直接管理真实文件 + 可验证"的路线 → 记录差异而不是照搬。

## 一、本轮已同步（2.7.1-community.1）

| 上游提交 | 能力 | 我们的落地 | 证据 |
| --- | --- | --- | --- |
| `ce2b268` 工坊添加 ID 直达按钮 | 输入作品 ID / 链接直接打开详情，不用先搜索 | 「创意工坊」工具栏新增 **ID 直达** 按钮 + 弹窗；复用后端 `ParseWorkshopID`，回车即前往 | `frontend/src/js/features/workshop/id-jump.js`、`workshop-history-wiring.test.mjs` |
| `b635ea3` 解析历史 | 记住最近解析过的工坊结果，一键切回 | 「下载与解析」页新增 **最近解析** 下拉（最多 10 条，最新在前）；点一条用**存下来的快照**重画结果，不再请求接口 | `internal/app/workshop_history.go`（+ `schemaVersion`）、`frontend/src/js/features/downloads/workshop-history.{mjs,js}` |

同步时按本项目约定做了两处增强：

- 解析历史文件带 `schemaVersion`（上游没有），与其它 5 个本地记录一致；
- 历史列表额外显示**解析时间**与 `#ID`，并处理了"文件损坏/为空"时的降级（不会让界面报错）。

## 二、上游修了、我们已有且更严（保留我们的实现）

| 上游提交 | 上游做了什么 | 我们的做法（更强/更早） |
| --- | --- | --- |
| `da4a71b` `fix(parser): 修正界面资源导致人物误判并稳定关键词优先级` | 新增"界面/HUD 路径不作为角色证据"的黑名单；把关键词表从 `map` 改成有序切片（`map` 迭代顺序随机 → 同一 VPK 每次标签可能不同） | 我们用的是**白名单**：只有 `models/survivors/`、`materials/models/infected/`、`sound/player/survivor/` 等内容根才算角色证据；规则表本来就是有序切片。本轮补 `internal/parser/upstream_parity_test.go` 把上游的具体场景钉死：8 类界面路径（vgui / sprites / hud / particles / sound-ui / scripts / resource）+ `common_male_ceda` 非普通感染者优先 + 50 次运行结果一致 + 真角色仍能识别 |
| `8c171c6` `fix addonlist写入编码问题` | 记录原编码（ANSI / UTF-8 / UTF-8 BOM）并按原编码写回 | 我们更早就实现了这套，且覆盖面更大：UTF-8 / UTF-8 BOM / **GBK** / **Windows-1252** / **UTF-16LE / UTF-16BE** 检测 + 保真写回，并有逐项回归测试（`TestSetVPKGameEnabledPreservesGBKLayoutAndWorkshopKey`、`TestAddonListDocumentPreservesUTF16LE`、`TestAddonListDocumentPreservesWindows1252ANSI` …） |
| `5f2eb49` 窗口保持关闭前大小 | 记住窗口尺寸 | 我们同时记住**宽高 + 最大化状态**，并按当前屏幕可用区域钳制（换小屏不会把按钮顶到屏幕外） |
| `a50cf4f` 标记 addonlist 中失效文件和新增文件 | 在加载顺序编辑器里标出"文件没了/新加的" | 我们用体检项 `missing_file`（`addonlist` 里有、磁盘上没有）+ 列表行"未记录"状态（磁盘上有、`addonlist` 里没有）覆盖同一判断，且能一键修复 |
| `be3edfc` 更新 GitHub Actions 依赖版本 | 升 action 版本 | 我们已升到 Node 24 运行时（`checkout@v7` / `setup-go@v7` / `setup-node@v7` / `upload-artifact@v7`），并把文档工作流固定在 `ubuntu-24.04` |
| `d0d406b` 工坊批量转移与自动 meta 获取 | 批量把工坊 VPK 转移出来并自动补 meta | 我们的"转移"保留 `workshop` 原件（Fork 语义：不破坏 Steam 识别）并同步 `addonlist.txt`；meta 与官方标签由「抓取工坊官方标签与统计」原生抓 Steam 官方接口 |

## 三、上游新增能力、我们有等价实现（保留我们的）

| 上游提交 | 上游能力 | 我们的等价实现 |
| --- | --- | --- |
| `1b74507` 全新的加载顺序编辑器 | 重做加载顺序编辑界面 | 我们的「加载顺序」窗口：**显式分层** + 组权重（祖先累加、跨链取 min）+ 「必须在前/必须在后」约束 + 「按分层应用」（未分层时逐字节不变）+ 事务写盘 + 黄金回归测试 |
| `ba20411` mod 快照工具（+1793 行后端 / 大套 UI） | 备份与恢复批量 Mod 状态，带预览页 | 我们的**启用方案**：捕获（含分层等自动化信息）、应用、导出、导入；组的"自动联动"负责批量开关 |
| `8b9dc8b` / `3008a30` / `c9e8a25` / `361dc9a` 工坊解析链路 | 服务端依赖与合集改用官方 API、图集按需取、合集隐藏不可下载主物品、详情显示依赖物品 | 我们已经原生抓 Steam 官方 `GetPublishedFileDetails`（标签与统计写进 `.meta`）+ 工坊合集实体化（记录/刷新/补下/更新检查）；合集解析已按"可下载项"过滤 |
| `cd93ea6` cf 工坊接口添加 ip 令牌桶 | 服务端限流 | 客户端不依赖该限流；我们有自己的优选 IP / 固定 IP / 镜像列表 |
| `d367670` / `e9069c8` 更新加速源、预热 | 换镜像源、更新弹框预热 | 我们的网络设置提供优选 IP、固定 IP、系统代理与测速结果 |

## 四、暂未同步（已记录，按需再做）

| 上游提交 | 能力 | 为什么先不做 |
| --- | --- | --- |
| `c1b4972` `favoriteServer` 外部协议 | 让外部程序/网页把服务器推进收藏列表 | 需要动协议注册 + 服务器地址解析 + 前端表单三处；我们的"收藏服务器"已支持手动添加与导入导出，优先级不高 |
| `361dc9a` 详情显示依赖物品 | 工坊详情里列出该物品依赖的其它物品 | 依赖数据在外层 worker 接口里，需要 worker 与前端一起改；我们已有 Mod 依赖管理（本地层面） |

## 五、维护者：怎么继续做同步

```powershell
git fetch upstream --tags --prune
git log --oneline (git merge-base HEAD upstream/master)..upstream/master   # 看上游新提交
git diff --stat (git merge-base HEAD upstream/master)..upstream/master     # 看改动面
```

逐条判断并落到四类之一（**直接同步 / 按我们的架构改写 / 保留我们的 / 记为未同步**），
然后把结论写回本页；同步入库的代码必须带自动化测试，并按需更新
[本 Fork 新增功能](/guide/whats-new) 与 [全功能地图](/guide/feature-tour)。
