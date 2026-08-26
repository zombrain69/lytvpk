# LytVPK 知识库

**Generated:** 2026-05-26
**领域:** Windows 桌面端 VPK Mod 管理器（Left 4 Dead 2）
**核心栈:** Go 1.25.x + Wails v2.10.2 + Vite 3 + 原生 JavaScript/CSS

## 概述

基于 Wails v2 的 Windows 桌面应用。Go 后端负责文件系统、VPK 解析、网络请求和系统集成；前端原生 JavaScript + CSS 实现 UI。核心能力：VPK 扫描解析、内容类型识别（地图/人物/武器）、标签管理、创意工坊下载、服务器浏览器、应用更新。

## 结构

```
.
├── main.go                    # Wails 入口：嵌入 frontend/dist、绑定 App、单例检测
├── wails.json                 # Wails 构建配置
├── go.mod                     # Go 1.25.x，模块 vpk-manager
├── internal/
│   ├── app/                   # 后端业务层，208 个 Wails 导出方法
│   │   └── AGENTS.md         # → 后端业务逻辑详细指南
│   ├── parser/                # VPK 解析与内容识别引擎
│   │   └── AGENTS.md         # → 解析器详细指南
│   ├── network/               # Steam 图片代理、IP 优选
│   └── platform/              # Windows 注册表、URL Protocol
├── frontend/
│   ├── src/js/
│   │   ├── main.js            # JS 入口
│   │   ├── core/              # UI shell、主题、toast、通用工具
│   │   └── features/          # 业务功能模块
│   │       └── AGENTS.md     # → 前端功能模块详细指南
│   ├── src/css/               # 全局/布局/功能样式
│   ├── wailsjs/               # Wails 自动生成绑定，禁止手动修改
│   └── index.html             # HTML 入口（含完整 UI 结构）
├── build/                     # Wails 构建资源、图标
├── worker/                    # Steam Workshop 外部脚本
└── .github/workflows/         # Release CI，推送 v* tag 触发
```

## 查询指南

| 任务                   | 位置                            | 备注                                      |
| ---------------------- | ------------------------------- | ----------------------------------------- |
| 修改后端业务逻辑       | `internal/app/*.go`             | 参考 `internal/app/AGENTS.md`             |
| 修改 VPK 解析/类型识别 | `internal/parser/*.go`          | 参考 `internal/parser/AGENTS.md`          |
| 修改前端 UI/交互       | `frontend/src/js/features/*.js` | 参考 `frontend/src/js/features/AGENTS.md` |
| 修改图片代理/IP 优选   | `internal/network/*.go`         | 本地 HTTP 服务器绕过 Steam CORS           |
| 修改 Windows 平台集成  | `internal/platform/*.go`        | URL Protocol、注册表操作                  |
| 修改构建配置           | `wails.json`                    | 前后端构建命令定义                        |
| 修改 CI/发布流程       | `.github/workflows/release.yml` | windows-latest，Go 1.25.x，Node 20        |

## CodeGraph 调查工作流

- **优先使用 CodeGraph**：仓库存在 `.codegraph/` 时，调查现有代码、调用链或影响范围前，先执行 `codegraph explore "<问题或符号>"`；不要先用 `rg` 猜测入口。
- **先确定链路再改动**：查询结果应覆盖入口、关键状态或 Wails 边界、最终执行点和受影响调用方；需要精确源码时可在查询中点名文件或符号。
- **再做局部检索**：只有在 CodeGraph 已确定范围后，才用 `rg` 查样式、HTML、文档、资源或未被索引的动态字符串；不要因为索引存在就自动重建它。
- **修改后复核**：功能完成前再次用 CodeGraph 验证新调用链，并运行与改动相称的测试、前端构建或 Wails 构建。提交说明应区分已验证行为与未做的真实环境验证。

## 核心符号

| 符号             | 类型   | 位置                                           | 职责                              |
| ---------------- | ------ | ---------------------------------------------- | --------------------------------- |
| `App`            | struct | `internal/app/app.go`                          | Wails 绑定主结构，208 个导出方法  |
| `ParseVPKFile`   | func   | `internal/parser/parser.go`                    | VPK 解析主入口，单次遍历提取资源  |
| `VPKFile`        | struct | `internal/parser/types.go`                     | 前后端传输核心数据结构            |
| `ScanVPKFiles`   | func   | `internal/app/vpk_scan.go`                     | 扫描缓存逻辑，四重校验            |
| `createFileItem` | func   | `frontend/src/js/features/file-list/render.js` | 列表项 DOM 渲染                   |
| `appState`       | object | `frontend/src/js/features/state.js`            | 全局状态：vpkFiles、selectedFiles |

## 约定

- **无前端框架**: 原生 JavaScript + CSS 变量，禁止引入 React/Vue/Svelte
- **功能模块化**: 新功能放入 `frontend/src/js/features/` 对应子目录，禁止全局逻辑
- **文件命名**: Go 按功能前缀分组 `workshop_*.go`、`vpk_*.go`、`panel_*.go`
- **平台隔离**: `secret_windows.go` / `secret_other.go` 用构建标签分离
- **路径操作**: 使用 `filepath.Join`，优先考虑 Windows 行为
- **并发管理**: 优先使用 `a.goroutinePool.Submit()`，禁止裸 `go func()`
- **状态驱动**: 前端 `state.js` 是单一数据源，修改后触发重新渲染

## 反模式

### 禁止修改
- `frontend/wailsjs/**` — Wails 自动生成，会被覆盖
- `build/windows/installer/*.nsh` — Wails 构建自动生成
- `frontend/dist/**` — Vite 构建产物

### 禁止新增
- ❌ 裸 goroutine — 使用协程池
- ❌ `innerHTML` — 使用 `createElement`/`textContent`
- ❌ `console.log` — 生产代码移除调试日志
- ❌ 全局逻辑 — 放入对应 feature 模块
- ❌ 临时字符串解析 — 复用 `internal/parser`
- ❌ 字符串拼接路径 — 使用 `filepath.Join`

### 安全警告
- ⚠️ 后端操作会移动/删除用户 Mod 文件，必须保留清晰错误处理
- ⚠️ `unsafe.Pointer` 仅允许在 `secret_windows.go`（Windows DPAPI）
- ⚠️ 修改 `VPKFile` 结构后必须同步更新前端类型定义和重新生成 Wails 绑定

## 命令

```powershell
# 本地开发
wails dev

# 验证构建（沙箱中可能报 Access is denied，直接提权运行）
wails build

# 发布构建（CI 自动执行；分发给用户时必须注入正式版本号）
wails build -platform windows/amd64 -ldflags "-X main.AppVersion=<version>" -o "LytVPK MOD管理器.exe"

# 运行测试（通过不代表完整集成验证）
go test ./...

# Go 格式化
gofmt -w <file>
```

## 注意事项

- `main.AppVersion` 默认保持当前 Community Fork 正式版；发布时仍必须通过 `-ldflags` 注入对应 Release 的精确版本。
- 不要分发 `0.0.0-dev` 构建；分发给用户的 EXE 必须使用 `-ldflags "-X main.AppVersion=<version>"` 或 `scripts/build-release.ps1 -Version <version>` 注入正式语义化版本。
- 配置存储在 `%APPDATA%/LytVPK/config.json`，当前迁移版本 v2
- 创意工坊、图片代理、服务器查询依赖外部网络，受限环境可能无法验证
- 单例通过 TCP 端口 `127.0.0.1:19527` 实现，支持 URL Protocol 深度链接 (`lytvpk://`)
- 前端 `index.html` 1200+ 行、`servers.js` 971 行、`app-runtime.js` 906 行 — 大文件现状，修改时保持局部
