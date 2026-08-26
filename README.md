# LytVPK Community Fork

一个专为 Left 4 Dead 2 (L4D2) 设计的现代化 VPK 插件管理工具。
> 该项目通篇使用AI生成，本人只在AI陷入困境时进行少量修改。仔细阅读代码你就会发现大量无用变量、不符合规范的函数定义，一个文件几千行的屎山，均不代表本人水平，谢谢！

这是基于 [LaoYutang/lytvpk](https://github.com/LaoYutang/lytvpk) 的非官方 Community Fork。原项目作者、版权声明和 GPL-3.0-only 许可证均予以保留；本仓库中的后续修改由本 Fork 维护者发布，不代表原作者官方版本。

- 本 Fork 仓库：[zombrain69/lytvpk](https://github.com/zombrain69/lytvpk)
- 上游项目：[LaoYutang/lytvpk](https://github.com/LaoYutang/lytvpk)
- 问题反馈：[本 Fork Issues](https://github.com/zombrain69/lytvpk/issues)
- 修改记录：[CHANGELOG.md](./CHANGELOG.md)

![LytVPK](https://img.shields.io/badge/Platform-Windows-blue)
![Build](https://img.shields.io/badge/Build-Wails_v2-green)
![Language](https://img.shields.io/badge/Language-Go_+_JavaScript-orange)
![License](https://img.shields.io/badge/License-GPL--3.0--only-blue)

## 📖 文档范围

基础功能及一般操作方式延续上游项目的[使用文档](https://lytvpk-docs.laoyutang.cn)。本 README 只记录 `zombrain69/lytvpk` 相对上游新增或显著强化的部分，避免把上游已有能力重复列为本 Fork 功能。

## ✨ 本 Fork 的重点优化（相对上游）

### `addonlist.txt` 全生命周期管理

- 根据目录类型准确定位配置：真实游戏 `left4dead2/addons` 使用游戏侧的 `left4dead2/addonlist.txt`；独立 Mod 收藏目录使用该目录自己的 `addonlist.txt`。
- 支持受保护快照、手动备份、恢复和删除；恢复或删除前会先保留当前配置，降低误操作丢失开关状态的风险。
- 可启用运行时保护：稳定检测到外部程序覆盖或删除配置后，先保存外部版本，再原子恢复受保护快照并同步游戏内开关状态。
- 可在写入前对比另一个 `addonlist.txt` 的新增项和开关冲突；冲突项默认保留当前状态，也可逐项选择采用来源状态。

### 可预览、可约束的加载顺序

- 加载顺序支持根目录与 Workshop 分组、完整顺序预览，以及明确的“必须在前 / 必须在后”约束。
- 应用策略前会校验约束并拒绝循环依赖；写入时原子更新 `addonlist.txt`，同时同步受保护快照。

### Workshop 安全标签与归档级内容识别

- 为 VPK 建立归档路径索引，无须把整个包解压即可基于真实内部路径分析内容。
- Workshop 中以纯数字发布 ID 命名的 VPK 不再被重命名；人工标签保存在同名 `.meta` 伴随文件，避免 Steam 或游戏失去识别。
- 扩展为适合 Workshop 大型库的细粒度标签：UI、声音、脚本、模型、贴图、物品、投掷物，以及角色、武器、地图模式等分类。
- 混合内容包仍保留稳定的主分类优先级（地图 → 人物 → 武器），并追加高置信度细分类，便于交叉筛选；规则回归测试覆盖常见名称误匹配。

### 面向大型 Mod 库的状态可视化与筛选

- 列表行和卡片使用颜色、徽标直观区分“游戏内开启 / 关闭 / 未记录”，并可直接按该状态筛选。
- 支持游戏状态、所在位置、一级标签与二级标签的组合筛选；文件处于 `disabled` 目录时会阻止不安全的游戏内开关编辑。
- 提供“集合 → 分类 → 具体物品”的多级预设筛选，覆盖枪械、官方近战、投掷物、医疗物品和补给盒；支持“任一匹配 / 全部匹配”且刷新后保留选择。
- 小窗口（约 1366px 宽）会收紧列和操作按钮，减少筛选与列表内容被裁切的情况。

### 分析、稳定性与 Fork 发布保障

- 按需统计 VPK 内模型数量、LOD0 顶点数和三角面数，不让普通目录扫描为每个 VPK 解析几何数据。
- 扫描时将根目录与 Workshop 中的同名 VPK 作为独立条目处理，避免不同位置的 Mod 相互覆盖。
- 增加单实例启动辅助逻辑，减少重复启动产生的并发管理风险。
- 应用内更新源固定为本 Fork 的 `zombrain69/lytvpk` Release，拒绝回退到上游；Windows 更新包使用统一程序名并进行内容校验。
- 发布版本会附带对应源码、许可证和修改说明，便于用户核对并按 GPLv3 重新构建。

## 🛠️ 技术架构

### 后端 (Go)
- **框架**: Wails v2
- **VPK解析**: 使用 `l4d2-manager-next/pkg/valve/vpk` 和 `l4d2-manager-next/pkg/vpkmission`
- **并发处理**: `github.com/panjf2000/ants/v2` 协程池
- **配置管理**: JSON 格式的持久化配置

### 前端 (JavaScript + CSS)
- **原生 JavaScript**: 无框架依赖，轻量高效
- **现代 CSS**: 基于 CSS 变量的设计系统
- **响应式设计**: 支持桌面端和移动端
- **实时通信**: 通过 Wails 事件系统与后端通信

## 📦 安装和使用

### 系统要求
- Windows 10/11

### 从源码构建

本项目使用 Wails 构建。应用内更新默认只检查本 Fork 的 [Releases](https://github.com/zombrain69/lytvpk/releases)，不会安装上游项目的版本。开发者可按本仓库的构建配置执行：

```text
wails build
```

发布二进制或安装包时，请同时提供对应版本的完整源码、构建脚本、`LICENSE` 和第三方声明；具体版本信息以仓库 Release 页面为准。

## 🙏 致谢

- [Wails](https://wails.io/) - 跨平台应用框架
- [l4d2-server-next](https://github.com/LaoYutang/l4d2-server-next/tree/master/backend/pkg) - VPK 与 mission 文件解析库
- [ants](https://github.com/panjf2000/ants) - 高性能协程池

## 📄 开源协议

本项目自 2026-06-29 起以 [GNU General Public License v3.0 only](./LICENSE) 授权发布。

分发二进制、修改版或衍生版本时，请遵守 GPLv3 关于对应源码、版权声明、许可证文本和修改说明等要求。第三方依赖与资源遵循各自许可证。
