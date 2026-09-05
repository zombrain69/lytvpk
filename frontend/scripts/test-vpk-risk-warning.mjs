import assert from "node:assert/strict";
import { formatVPKRiskWarning } from "../src/js/features/file-list/vpk-risk-warning.mjs";

const message = formatVPKRiskWarning("启用", [
  { name: "broken.vpk", detail: "缺少根目录 addoninfo.txt" },
]);

assert.match(message, /风险提示：启用/);
assert.match(message, /broken\.vpk/);
assert.match(message, /游戏可能不会记录或加载/);
assert.match(message, /缺少根目录 addoninfo\.txt/);
assert.match(message, /VPK 完整性检测/);

console.log("vpk risk warning formatting passed");
