# Mod Risk Warning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow VPKs with game-relevant metadata problems to remain operable while warning before enablement and other state-changing operations.

**Architecture:** Keep `SetVPKGameEnabled` and existing file operations as permissive state-changing APIs. Add one backend preflight method that returns a structured, non-blocking warning based on `ValidateVPKAddonInfo`; the frontend calls it from shared file-operation helpers and uses the existing confirmation modal/toast system. No generated Wails bindings are edited.

**Tech Stack:** Go, Wails runtime exposure, native JavaScript modules, existing confirmation modal/toast CSS, PowerShell release scripts.

**Spec:** Approved chat design from 2026-09-05 user confirmation.

## Global Constraints

- Preserve existing user changes in `frontend/src/css/app/modals-details.css`, `frontend/src/js/core/toast.js`, `internal/app/addon_list.go`, and `internal/app/addon_list_test.go`.
- Do not modify `frontend/wailsjs/**` manually.
- Missing or malformed root `addoninfo.txt` must never block `SetVPKGameEnabled` or file operations.
- Warning text must state that the game may not record/load the Mod, but the user can continue and may repair it with the VPK integrity tool.
- Use CodeGraph evidence for the call chain and run Go tests, frontend build, Wails build, release packaging, and archive verification.

---

### Task 1: Add backend VPK operation risk report

**Files:**
- Create: `internal/app/vpk_warnings.go`
- Test: `internal/app/vpk_warnings_test.go`
- Modify: `internal/app/app.go` only if the result type needs a shared exported declaration.

**Interfaces:**
- Produces `type VPKOperationWarning struct { HasWarning bool; Summary string; Detail string; Repairable bool }` with JSON fields `hasWarning`, `summary`, `detail`, `repairable`.
- Produces `func (a *App) GetVPKOperationWarning(filePath string) (VPKOperationWarning, error)`.

- [ ] Write a failing test for a root VPK whose `addoninfo.txt` is missing: the method returns `HasWarning=true`, `Repairable=true`, and a detail containing `缺少根目录 addoninfo.txt`.
- [ ] Run `go test ./internal/app -run TestGetVPKOperationWarning -count=1` and confirm it fails because the method/result is absent.
- [ ] Implement the smallest method: resolve the cached file, return a not-found error when absent, skip non-root files, call `parser.ValidateVPKAddonInfo`, and convert validation errors into a warning result rather than an error.
- [ ] Add a malformed-addoninfo test and a valid-addoninfo test; valid and non-root files must return `HasWarning=false`.
- [ ] Run the focused tests and then `go test ./internal/app -run 'Test(GetVPKOperationWarning|SetVPKGameEnabled)' -count=1`.

### Task 2: Add shared frontend warning confirmation

**Files:**
- Modify: `frontend/src/js/features/file-list/operations.js`
- Modify: `frontend/src/js/features/file-list/actions.js`
- Modify: `frontend/src/js/features/file-list/context-menu.js` only if menu labels need a visible risk marker.
- Modify: `frontend/src/css/app/modals-details.css` only for warning-modal emphasis if existing styles are insufficient.

**Interfaces:**
- Produces `export function confirmVPKOperationWarning(filePaths, operationLabel)` resolving `true`/`false`.
- Uses runtime lookup `window.go.app.App.GetVPKOperationWarning` so generated bindings remain untouched.

- [ ] Add a frontend-executable regression fixture or module-level test harness for a warning result and verify the confirmation message contains the Mod name, operation, `游戏可能不会记录或加载`, and repair guidance.
- [ ] Run the focused frontend test/harness and confirm it fails before the helper exists.
- [ ] Implement a shared helper that deduplicates paths, fetches warnings, shows the existing warning toast for lookup failures, and opens `showConfirmModal` only when at least one warning exists; cancellation must prevent the operation.
- [ ] Wrap `toggleFile`, `toggleGameEnabled`/`setGameEnabled`, single-file delete/rename/hide/unpack/move, batch enable/disable/delete/move/visibility, and workshop-to-root transfer with the helper. Read-only detail/open-location actions remain unblocked and unprompted.
- [ ] Use the existing `warning` toast style for successful-but-risky batch outcomes and ensure no `console.log` is added.
- [ ] Run `npm run build` from `frontend`.

### Task 3: Update release metadata and documentation

**Files:**
- Modify: `main.go` default `AppVersion` to `2.5.14-community.61`.
- Modify: `CHANGELOG.md` with a `2.5.14-community.61 — 2026-09-05` entry.

- [ ] Document permissive enablement, warning behavior, and repair path without exposing local maintainer details.
- [ ] Run `git diff --check` and scan the public diff for local paths, emails, credentials, and internal publishing details.

### Task 4: Build, verify, commit, tag, and publish

**Files/artifacts:**
- Generate: `build/bin/LytVPK-Community-Fork.exe`
- Generate: `build/release/LytVPK-Community-Fork_v2.5.14-community.61_windows_amd64.zip`

- [ ] Run `go test ./...`.
- [ ] Run `npm run build` from `frontend`.
- [ ] Run `pwsh -ExecutionPolicy Bypass -File .\\scripts\\build-release.ps1 -Version 2.5.14-community.61`.
- [ ] Run `pwsh -ExecutionPolicy Bypass -File .\\scripts\\verify-release.ps1 -Version 2.5.14-community.61 -ArchivePath .\\build\\release\\LytVPK-Community-Fork_v2.5.14-community.61_windows_amd64.zip`.
- [ ] Verify `git status`, diff, release checksum, and generated EXE metadata.
- [ ] Commit with a focused message, create tag `v2.5.14-community.61`, push `master` and the tag to `origin`, then create the GitHub release with the canonical ZIP and checksum.
