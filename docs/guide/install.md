# 安装与运行

## 系统要求

- Windows 10 / 11（64 位）。
- 下载发布包、浏览创意工坊、加载预览图、查询服务器和检查更新都需要网络。

## 下载与解压

1. 打开发布页：<https://github.com/zombrain69/lytvpk/releases>（也可以在应用「关于」页点「检查更新」）。
2. 下载最新的 `LytVPK-Community-Fork_v<版本>_windows_amd64.zip`；同一页还有 `SHA256SUMS.txt`，可用于核对下载是否完整。
3. 解压到一个固定目录（例如 `D:\Tools\LytVPK\`），**不要**在压缩包里直接双击运行。解压后目录里包含：

| 文件 | 说明 |
| --- | --- |
| `LytVPK-Community-Fork.exe` | 主程序。**文件名请保持原样**，更新器按这个固定名字校验 |
| `LICENSE`、`THIRD_PARTY_NOTICES.md` | 许可证与第三方声明 |
| `README.md`、`CHANGELOG.md` | 项目说明与每个版本的改动 |
| `SOURCE_CODE.md` | 对应源码与构建方式（GPLv3 要求） |

4. 双击 `LytVPK-Community-Fork.exe`。发布包**没有做代码签名**，首次运行可能被 Windows SmartScreen 拦下：
   点「更多信息」→「仍要运行」即可。

## 首次启动

- 应用会先**自动找一次游戏目录**：读 Steam 注册表 → 顺着 `libraryfolders.vdf` 找每个 Steam 库
  → 都没有时再按盘符扫常见路径；只有确认那里真的装了游戏才会采用。
- 自动找不到（绿色版、非 Steam 安装、库目录改过名）时，点左上角「选择 addons 目录」手动选
  `...\Left 4 Dead 2\left4dead2\addons`。
- 选过的目录会进入历史列表，之后可以下拉快速切换。

## 配置保存位置

应用配置保存在（Windows）：

```text
%APPDATA%\LytVPK\
```

里面包括 `config.json`（目录历史、界面偏好、工坊设置等）、`groups.json` / `priority.json` /
`dependencies.json` / `ignore.json` / `profiles.json`（分组、分层、依赖、忽略清单、启用方案）、
`download_tasks.json`（下载任务快照）、`addonlist` 相关备份，以及崩溃报告目录 `crashes\`。
这些文件都保存在本机，**不会上传**。

## 单实例与更新

- 应用是**单实例**：重复启动不会开出第二个窗口，新实例会把参数交给已经在运行的实例。
- 自动更新只检查本 Fork 的 [Releases](https://github.com/zombrain69/lytvpk/releases)，**不会**回退到上游版本；
  更新包使用统一程序名并做内容校验。更新失败时的排查见[关于与更新](/features/about-update)。

## 使用文档

- 在线文档站：<https://zombrain69.github.io/lytvpk/>（应用里左侧「使用说明」按钮就指向这里）
- 建议顺序：[全功能地图](/guide/feature-tour) → [快速开始](/guide/quick-start) → [本 Fork 新增功能](/guide/whats-new)

## 更新失败时的排查

应用会在启动时检查新版本。发现新版本后，会显示更新内容，并允许选择下载镜像。

- 换一个镜像源。
- 检查网络是否能访问 GitHub 或镜像。
- 关闭正在运行的旧版本后重试。
- 手动到 [Releases](https://github.com/zombrain69/lytvpk/releases) 下载新版压缩包，解压覆盖旧目录（配置文件在 `%APPDATA%`，不受影响）。

## 注意事项

- 管理 Mod 前建议关闭游戏，避免文件正在被占用。
- 删除、移动和批量禁用会真的改动本地 Mod 文件：删除是**进回收站**，移动不可撤销，请先确认选中范围。
- 创意工坊、图片预览、服务器查询和自动更新都依赖网络环境。
- 改名会自动同步策略组 / 分层 / 依赖 / 忽略清单里的引用；`addonlist.txt` 的编码、BOM 与换行会被保留。
