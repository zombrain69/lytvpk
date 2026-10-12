// 后端错误展示桥（路线图第 21 项阶段 B ④）的测试：
//   1. 行为：zh 原样直通（逐字节）；en 已知模板翻英文 / 认不出原样；输入形态容错；
//   2. 保鲜：扫描 internal/app 的 Go 源 —— formatFileMoveError 的动作词、
//      describeFileMoveFailure 的六条原因、若干整句模板，全部必须有英文映射，
//      且映射后不得残留中文（后端文案改动会让这里先失败，提示补词条）。
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

import { formatBackendErrorDetail } from "./backend-error.mjs";
import { installCatalogs, setLocale } from "./i18n-runtime.js";

const here = path.dirname(fileURLToPath(import.meta.url)); // frontend/src/js/core
const repoRoot = path.resolve(here, "../../../..");
const appGoDir = path.join(repoRoot, "internal", "app");

installCatalogs(
  {
    "zh-CN": JSON.parse(readFileSync(path.join(here, "../../messages/zh-CN.json"), "utf8")),
    en: JSON.parse(readFileSync(path.join(here, "../../messages/en.json"), "utf8")),
  },
  "zh-CN",
);

const CJK = /[\u4e00-\u9fff]/;

function readGo(name) {
  return readFileSync(path.join(appGoDir, name), "utf8");
}

function allAppGoSources() {
  const files = [];
  for (const entry of readdirSync(appGoDir, { withFileTypes: true })) {
    if (!entry.isFile() || !entry.name.endsWith(".go") || entry.name.endsWith("_test.go")) continue;
    files.push(entry.name);
  }
  return files;
}

test("zh 默认直通：字符串、Error、空值都原样（逐字节不翻译）", () => {
  setLocale("zh-CN");
  const samples = [
    "禁用 a.vpk 失败：磁盘空间不足",
    "文件未找到: C:\\addons\\a.vpk",
    "workshop文件需要先转移到插件目录才能启用/禁用",
    "另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试",
    "某种没见过的后端错误 XYZ-123",
  ];
  for (const sample of samples) {
    assert.equal(formatBackendErrorDetail(sample), sample, `zh 必须逐字直通：${sample}`);
  }
  assert.equal(formatBackendErrorDetail(new Error("禁用 a.vpk 失败：磁盘空间不足")), "禁用 a.vpk 失败：磁盘空间不足");
  assert.equal(formatBackendErrorDetail(null), "");
  assert.equal(formatBackendErrorDetail(undefined), "");
  assert.equal(formatBackendErrorDetail(""), "");
});

test("en：句子族翻译动作 / 文件名 / 规范原因", () => {
  setLocale("en");
  assert.equal(
    formatBackendErrorDetail("禁用 a.vpk 失败：文件正被其它程序占用（游戏、杀毒软件、资源管理器预览都可能占用它），关闭占用的程序后重试"),
    "Failed to disable a.vpk: The file is in use by another program (the game, antivirus, or Explorer preview may be holding it); close it and retry",
  );
  assert.equal(
    formatBackendErrorDetail("启用 my mod.vpk 失败：磁盘空间不足"),
    "Failed to enable my mod.vpk: Not enough disk space",
  );
  assert.equal(
    formatBackendErrorDetail("重命名 a.vpk 失败：目标目录里已经有同名文件，请先改名或删除它"),
    "Failed to rename a.vpk: A file with the same name already exists in the target folder; rename or delete it first",
  );
  // 未识别的原因保持原样，但「哪个动作、哪个文件」要翻出来。
  assert.equal(
    formatBackendErrorDetail("移动 a.vpk 失败：The system cannot find the path specified"),
    "Failed to move a.vpk: The system cannot find the path specified",
  );
  // 动作词不在表里 → 整句原样（不半翻）。
  assert.equal(
    formatBackendErrorDetail("压缩 a.vpk 失败：磁盘空间不足"),
    "压缩 a.vpk 失败：磁盘空间不足",
  );
});

test("en：整句模板翻译；未收录的整句原样返回", () => {
  setLocale("en");
  const cases = [
    ["文件未找到: a.vpk", "File not found: a.vpk"],
    ["文件不存在: a.vpk", "File does not exist: a.vpk"],
    ["未选择L4D2目录", "No L4D2 folder selected"],
    [
      "workshop文件需要先转移到插件目录才能启用/禁用",
      "Workshop files must be moved to the addons folder before they can be enabled or disabled",
    ],
    ["无效的文件状态转换", "Invalid file state transition"],
    ["目标文件已存在: a.vpk", "A file with the same name already exists: a.vpk"],
    [
      "文件已移动，但 addonlist.txt 同步失败: 写入被拒绝",
      "The file was moved, but syncing addonlist.txt failed: 写入被拒绝",
    ],
    [
      "另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试",
      "Another file operation is in progress (move / delete / pack); wait for it to finish and try again",
    ],
    [
      "这个文件不在当前受管的 addons / workshop / disabled 目录里：C:\\addons\\a.vpk",
      "This file is not inside the managed addons / workshop / disabled folders: C:\\addons\\a.vpk",
    ],
    ["删除文件失败: Access is denied", "Failed to delete the file: Access is denied"],
  ];
  for (const [raw, expected] of cases) {
    assert.equal(formatBackendErrorDetail(raw), expected, `en 翻译不对：${raw}`);
  }
  // 未收录 → 原样（不吞原因）。
  const unknown = "有些错误暂时还没有英文映射 OVERFLOW-42";
  assert.equal(formatBackendErrorDetail(unknown), unknown);
});

test("保鲜：formatFileMoveError 的动作词全部有英文映射", () => {
  setLocale("en");
  const verbs = new Set();
  for (const name of allAppGoSources()) {
    for (const match of readGo(name).matchAll(/formatFileMoveError\("([^"]+)"/g)) {
      verbs.add(match[1]);
    }
  }
  assert.ok(verbs.size >= 10, `扫描到的动作词太少（${verbs.size}），检查扫描正则是否失效`);
  for (const verb of verbs) {
    const translated = formatBackendErrorDetail(`${verb} sample.vpk 失败：磁盘空间不足`);
    assert.ok(!CJK.test(translated), `动作词「${verb}」没有英文映射：${translated}`);
  }
});

test("保鲜：describeFileMoveFailure 的六条规范原因全部有英文映射", () => {
  setLocale("en");
  const source = readGo("file_operation_errors.go");
  const start = source.indexOf("func describeFileMoveFailure");
  assert.ok(start >= 0, "找不到 describeFileMoveFailure（重命名了？）");
  const body = source.slice(start, source.indexOf("\n}", start));
  // 注意：同一句原因在两个 switch 分支（Windows errno 与 os.ErrPermission）各写一次，
  // 去重后才是「规范原因」全集。
  const reasons = [...new Set([...body.matchAll(/return "((?:[^"\\]|\\.)*)"/g)].map((m) => m[1]).filter(Boolean))];
  assert.equal(reasons.length, 6, `规范原因应为 6 条（去重后），实际 ${reasons.length}`);
  for (const reason of reasons) {
    const translated = formatBackendErrorDetail(`禁用 sample.vpk 失败：${reason}`);
    assert.ok(!CJK.test(translated), `原因「${reason.slice(0, 12)}…」没有英文映射：${translated}`);
  }
});

test("保鲜：文件操作链路的整句模板在 Go 源里存在且有英文映射", () => {
  setLocale("en");
  const pins = [
    ["vpk_actions.go", "文件未找到: %s"],
    ["vpk_actions.go", "workshop文件需要先转移到插件目录才能启用/禁用"],
    ["vpk_actions.go", "无效的文件状态转换"],
    ["vpk_actions.go", "目标文件已存在: %s"],
    ["vpk_actions.go", "文件已移动，但 addonlist.txt 同步失败: %w"],
    ["vpk_actions.go", "已复制到 addons，但 addonlist.txt 同步失败: %w"],
    ["filesystem.go", "文件不存在: %s"],
    ["filesystem.go", "文件已移入回收站，但 addonlist.txt 同步失败: %w"],
    ["filesystem.go", "删除文件失败: %s"],
    ["filesystem.go", "文件路径为空"],
    ["file_op_gate.go", "另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试"],
    ["path_guard.go", "这个文件不在当前受管的 addons / workshop / disabled 目录里：%s"],
    ["path_guard.go", "还没有选择 addons 目录，无法确认这个文件是否受管"],
    ["path_guard.go", "只接受完整路径，收到的是相对路径: %s"],
    ["file_move_conflicts.go", "目标目录不能为空"],
    ["file_move_conflicts.go", "目标路径不是目录: %s"],
    ["file_move_conflicts.go", "源文件不存在或无法访问: %s"],
    ["addon_list.go", "未选择L4D2目录"],
  ];
  for (const [goFile, fragment] of pins) {
    assert.ok(readGo(goFile).includes(fragment), `${goFile} 里找不到「${fragment}」，词条可能已过期`);
    const sample = fragment.replace(/%[sw]/g, "sample.vpk");
    const translated = formatBackendErrorDetail(sample);
    assert.ok(!CJK.test(translated), `「${fragment}」没有英文映射：${translated}`);
  }
});
