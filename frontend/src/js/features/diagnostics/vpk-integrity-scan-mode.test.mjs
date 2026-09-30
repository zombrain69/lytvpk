// 「完整性校验到底查了什么」必须写在界面上。
//
// 背景：这条校验以前逐条目把整个 VPK 读一遍核对 CRC，真机 1.42GB 的包要 4 分 03 秒；
// 它挂在「启用游戏内 Mod」之前，所以"启用"比"关闭"慢几个数量级。
// 改成索引级校验（读目录表 + 按数据卷核对范围 + addoninfo.txt）之后，
// 界面上那句"已校验 N 个"必须说清是**索引级**还是**整包**，否则用户会以为数据也逐字节核过了。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(new URL("./vpk-integrity.js", import.meta.url), "utf8");
const parserSource = readFileSync(new URL("../../../../../internal/parser/vpk_integrity.go", import.meta.url), "utf8");

test("完整性检测的统计行写清校验级别", () => {
  assert.match(source, /report\.scanMode/, "统计行要读 scanMode");
  assert.match(source, /索引级/, "默认级别要显示成「索引级」");
  assert.match(source, /整包/, "深度校验要显示成「整包」");
  assert.match(source, /已校验 \$\{[^}]*\} 个（\$\{scanLabel\}校验）/, "「已校验 N 个」后面要跟校验级别");
});

test("后端默认走索引级，整包校验不挂在界面路径上", () => {
  assert.match(parserSource, /VPKIntegrityScanIndex = "index"/, "要有索引级标记");
  assert.match(parserSource, /func InspectVPKIntegrity\(filePath string\)/, "默认入口仍是 InspectVPKIntegrity");
  assert.match(parserSource, /return inspectVPKIntegrity\(filePath, false\)/, "默认入口必须是索引级（false）");
  assert.ok(
    !/func InspectVPKIntegrityDeep/.test(parserSource),
    "整包校验不要导出成界面可点的入口：1.42GB 要读 4 分钟",
  );
});
