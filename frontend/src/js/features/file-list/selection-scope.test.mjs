import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// 真机复现过的缺陷：选了 Mod 之后再改筛选，批量启用/禁用只处理"筛选后仍然可见"的那几个，
// 状态栏却还写着 已选择: N —— 用户以为整批都处理了。
//
// 选择集（appState.selectedFiles）是跨筛选保留的，所以解析路径时必须以完整列表为准：
//   实测宽窄筛选两种情况下，10 个可禁用文件里只有 3 个（筛选命中数）真的被禁用。

const here = path.dirname(fileURLToPath(import.meta.url));
const actionsSource = readFileSync(path.resolve(here, "actions.js"), "utf8");

/** 抠出 `const file = ...;` 这种一次性解析语句。 */
function fileLookupStatements(source) {
  const statements = [];
  const marker = "const file =";
  let index = source.indexOf(marker);
  while (index >= 0) {
    const end = source.indexOf(";", index);
    if (end < 0) break;
    statements.push(source.slice(index, end + 1));
    index = source.indexOf(marker, end);
  }
  return statements;
}

test("解析已选 Mod 时必须回退到完整列表（否则筛选会静默缩小批量作用域）", () => {
  const statements = fileLookupStatements(actionsSource);
  assert.ok(statements.length > 0, "没找到 const file = ... 形式的路径解析");

  const offenders = statements.filter(
    (statement) => statement.includes("appState.vpkFiles") && !statement.includes("allVpkFiles"),
  );
  assert.deepEqual(
    offenders,
    [],
    `这些解析只看当前筛选结果，会丢已选项：\n${offenders.join("\n")}`,
  );
});

test("批量启用/禁用按整个选择取候选，而不是当前可见列表", () => {
  for (const fn of ["enableSelected", "disableSelected"]) {
    const start = actionsSource.indexOf(`export async function ${fn}()`);
    assert.ok(start >= 0, `没找到 ${fn}`);
    const body = actionsSource.slice(start, start + 1200);
    assert.match(
      body,
      /Array\.from\(appState\.selectedFiles\)/,
      `${fn} 的候选应该来自整个选择`,
    );
  }
});
