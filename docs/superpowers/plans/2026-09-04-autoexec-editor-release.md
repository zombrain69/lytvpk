# autoexec.cfg 编辑器修复与 Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复设置页保存 autoexec.cfg 时因 `autoexecInfo` 作用域错误导致的 `ReferenceError`，优化读取/保存状态同步，并构建、校验和发布新的 Windows Community Fork release。

**Architecture:** 将设置页 autoexec 编辑器的元数据状态限定在绑定控制器的闭包内，由显式状态对象承载 reload/save 后的最新配置；保存成功与保存后刷新失败分开报告，避免把刷新错误误报为保存失败。保留诊断页独立编辑器，不修改 Wails 自动生成绑定。

**Tech Stack:** 原生 JavaScript ES modules、Node `node:test`、Vite 3、Go 1.25.x、Wails v2.10.2、PowerShell、GitHub Actions release workflow。

**Spec:** 用户请求与截图中的 `ReferenceError: autoexecInfo is not defined` 故障线索；仓库 `AGENTS.md` 的 CodeGraph、构建和发布约束。

## Global Constraints

- 先用 CodeGraph 调查并在修改后再次复核调用链。
- 不修改 `frontend/wailsjs/**`、`frontend/dist/**` 或自动生成安装器文件。
- 保留 autoexec.cfg 原有编码、BOM 和换行格式；前端只写文件，不执行控制台命令。
- 生产构建必须注入正式语义化版本，不得分发 `0.0.0-dev`。
- 发布前运行前端回归测试、`npm run build`、`go test ./...`、release archive contract 验证。

### Task 1: 建立 autoexec 状态回归测试

**Files:**
- Create: `frontend/src/js/features/settings/autoexec-settings-controller.mjs`
- Create: `frontend/src/js/features/settings/autoexec-settings-controller.test.mjs`

**Interfaces:**
- Produces `createAutoexecSettingsController(initialInfo, operations)` with `getInfo()`, `reload()`, and `save(content)` methods for the settings page.

- [ ] **Step 1: Write the failing test**

  Cover that reload updates the held metadata without reading an undeclared variable, save delegates content then refreshes metadata, and refresh errors are distinguishable from save errors.

- [ ] **Step 2: Run the focused test to verify it fails**

  Run: `node --test frontend/src/js/features/settings/autoexec-settings-controller.test.mjs`
  Expected: FAIL because the controller module/API does not exist yet.

### Task 2: Implement and integrate the autoexec controller

**Files:**
- Modify: `frontend/src/js/features/settings/settings-page.js:1,523-570,1216-1262`
- Modify: `frontend/src/js/features/settings/autoexec-settings-controller.mjs`
- Test: `frontend/src/js/features/settings/autoexec-settings-controller.test.mjs`

**Interfaces:**
- Consumes `deps.autoexecInfo`, `deps.GetAutoexecConfig`, and `deps.SaveAutoexecConfig`.
- Produces explicit `autoexecInfo` state local to `bindSettingsPage`, preserving the latest successful metadata and separating save/reload failure paths.

- [ ] **Step 1: Write the minimal controller implementation**

  Keep `autoexecInfo` in controller state, make `reload()` replace it only after a successful read, and make `save()` report the original save error separately from a post-save reload error.

- [ ] **Step 2: Integrate the controller into settings-page bindings**

  Initialize with `deps.autoexecInfo || null`; replace bare `autoexecInfo` references in reload/open handlers with the controller-backed local state; keep button state and metadata synchronized.

- [ ] **Step 3: Run focused tests and frontend build**

  Run: `node --test frontend/src/js/features/settings/autoexec-settings-controller.test.mjs`; `npm run build` from `frontend`.
  Expected: PASS and successful Vite build.

### Task 3: Full verification and CodeGraph review

**Files:**
- No additional production files.

- [ ] **Step 1: Re-query CodeGraph**

  Run: `codegraph explore "settings-page.js autoexecInfo reloadAutoexecEditor settings-autoexec-save"`.
  Confirm no remaining unbound `autoexecInfo` reference in `bindSettingsPage` and that all callers remain wired.

- [ ] **Step 2: Run repository tests**

  Run: `go test ./...` from repository root and repeat the focused Node test.

- [ ] **Step 3: Inspect diff and release prerequisites**

  Run: `git diff --check`, `git status --short`, and the repository identity guard in release mode before creating a tag.

### Task 4: Build and publish the Windows release

**Files:**
- Generated: `build/bin/LytVPK-Community-Fork.exe`
- Generated: `build/release/LytVPK-Community-Fork_v<version>_windows_amd64.zip`
- Generated: `build/release/SHA256SUMS.txt`

- [ ] **Step 1: Choose the next semantic version**

  Use `2.5.14-community.59`, incrementing the current `v2.5.14-community.58` tag.

- [ ] **Step 2: Build the canonical release package**

  Run: `pwsh -ExecutionPolicy Bypass -File .\\scripts\\build-release.ps1 -Version 2.5.14-community.59`.

- [ ] **Step 3: Verify the archive contract and checksum**

  Run: `pwsh -ExecutionPolicy Bypass -File .\\scripts\\verify-release.ps1 -Version 2.5.14-community.59 -ArchivePath .\\build\\release\\LytVPK-Community-Fork_v2.5.14-community.59_windows_amd64.zip` and inspect `SHA256SUMS.txt`.

- [ ] **Step 4: Commit, tag, and push**

  Commit only the intended source/plan changes, create tag `v2.5.14-community.59`, and push the commit plus tag to `origin`; do not push unrelated worktree changes.

- [ ] **Step 5: Confirm GitHub Actions release publication**

  Inspect the tag workflow/release status with `gh run list` and `gh release view v2.5.14-community.59` until the archive and checksum assets are visible; report any external authentication or workflow blocker explicitly.
