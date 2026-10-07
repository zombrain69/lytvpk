// 静态回归：这三件事以后不能被改回去（真机验收过的行为，见 docs/development/manual-verification.md）：
//   1) 冲突卡片能长按拖动调优先级，且与「提前/延后」共用同一套写入路径；
//   2) 修复建议默认收起，展开按钮才铺开；
//   3) 子标签默认只占一行，展开按钮显示总数，且渲染的是完整标签集合。
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
// here = <repo>/frontend/src/js/features/conflicts → 退四层到 <repo>/frontend
const frontendRoot = path.resolve(here, "../../../..");
const read = (relative) => readFileSync(path.join(frontendRoot, relative), "utf8");

test("冲突卡片接了长按拖拽，并且复用「提前/延后」的写入路径", () => {
  const source = read("src/js/features/conflicts/conflicts.js");
  assert.match(source, /import \{[^}]*attachConflictRowDrag[^}]*\} from "\.\/conflict-row-drag\.mjs"/);
  assert.match(source, /attachConflictRowDrag\(vpkNamesEl, \{/);
  assert.match(source, /canDrag:/);
  assert.match(source, /onDrop:/);
  assert.match(source, /async function applyConflictOrderMove\(/);
  // 按钮分支与拖拽分支都必须走 applyConflictOrderMove
  assert.match(source, /await applyConflictOrderMove\(path, btn\.dataset\.targetPath, btn\.dataset\.orderDirection\)/);
  assert.match(source, /applyConflictOrderMove\(path, targetPath, direction, \{ source: "drag" \}\)/);
  assert.match(source, /长按拖动可排序/);
});

test("修复建议默认收起，点按钮才展开（且展开时给全部建议）", () => {
  const html = read("index.html");
  assert.match(html, /id="conflict-fix-toggle"[\s\S]{0,200}aria-expanded="false"/);
  assert.match(html, /id="conflict-fix-body" class="conflict-fix-body hidden"/);

  const source = read("src/js/features/conflicts/conflicts.js");
  assert.match(source, /let conflictFixExpanded = false;/);
  assert.match(source, /if \(!conflictFixExpanded\) \{\n\s*list\.replaceChildren\(\);\n\s*return;/);
  assert.match(source, /currentConflictFixSuggestions\.forEach\(/);
  // 旧的"只显示前 5 条"限制不应该回来
  assert.doesNotMatch(source, /CONFLICT_FIX_LIMIT/);
});

test("子标签默认收起：一行 + 「展开全部 N 项」，展开时渲染完整集合", () => {
  const source = read("src/js/features/file-list/filters.js");
  assert.match(source, /let secondaryTagsExpanded = false;/);
  assert.match(source, /function syncSecondaryTagsCollapse\(container, actionSlot\)/);
  // 布局没就绪要重试，而不是"量不到就整片铺开"
  assert.match(source, /if \(attempts < 12\) setTimeout\(run, 80\);/);
  assert.match(source, /展开全部\$\{count \? ` \$\{count\} 项` : ""\}/);
  // 渲染侧不截断：所有次级标签都进容器（完整集合）
  assert.match(source, /secondaryTags\.forEach\(\(tag\) => \{/);
  assert.doesNotMatch(source, /secondaryTags\.slice\(/);
  assert.match(source, /container\.classList\.add\("collapsed"\)/);
});
