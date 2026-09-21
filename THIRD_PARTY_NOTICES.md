# Third Party Notices

This project is licensed under GPL-3.0-only. The following third-party code or assets are used by LytVPK and keep their own notices.

## VTF-Editor conversion logic

- Upstream project: https://github.com/Mishcatt/VTF-Editor
- Modified web version referenced during implementation: https://zhrradiant.com/wp-content/tools/VTF-Editor-modified/
- License: GNU General Public License v3.0
- Imported purpose: VTF spray conversion behavior, VTF header/data layout, texture-format choices, mipmap and animation workflow.
- LytVPK changes: rewritten as ES modules for the native tool UI, old HTML/CSS UI removed, DOM-coupled conversion flow separated from rendering, backend export/install integration added.
- Modification date: 2026-06-29

The LytVPK spray tool does not copy the VTF-Editor page interface. It uses a project-native interface and keeps conversion-related logic under GPL-compatible project licensing.

## FireAxe（设计借鉴，未复制代码）

- Upstream project: https://github.com/ktxiaok/FireAxe
- Baseline commit referenced for design review: `f8aa1cf1391802c64525412fdf326dbe0ecc98b1` (v0.7.3)
- License: Apache License 2.0
- Imported purpose: **只借鉴设计与算法**，全部使用 Go / 原生 JavaScript 重写，未复制上游源代码、注释或资源。
- 具体借鉴点（对应上游文件与行号见 `docs/development/fireaxe-parity.md`）：
  - 显式可编辑优先级与"优先级在层级中累加"的思想（`FireAxe.Core/AddonNode.cs` 的 `Priority` / `PriorityInHierarchy`）；
  - 冲突只在同一优先级内判定，以及内置忽略清单 + 全局清单 + 单 Mod 清单取并集（`FireAxe.Core/AddonConflictUtils.cs`、`FireAxe.Core/VpkAddon.cs`）；
  - 备份轮转策略：最短间隔、与上一份内容相同则跳过、数量上限、溢出进回收站（`FireAxe.Core/AddonRoot.cs` 的 `BackUpIfNeed`）；
  - Problem / IValidity 的"可失效、可建议自动修复"模型（`FireAxe.Core/Problem.cs`、`IValidity.cs`）；
  - 组启用策略与工坊条目自动更新检查的组织方式（`FireAxe.Core/AddonGroup.cs`、`WorkshopVpkAddon.cs`）。
- LytVPK 的差异：本项目直接管理游戏真实文件，addonlist.txt 的顺序仍是游戏侧唯一权威；
  分层只承担"意图 + 判定 + 排序"，不引入上游的符号链接 push 与 `RefAddonNode`。
- Modification date: 2026-09-22

## Bundled font

- File: `frontend/src/assets/fonts/OFL.txt`
- License: SIL Open Font License 1.1
