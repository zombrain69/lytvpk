import assert from "node:assert/strict";
import test from "node:test";

import {
  formatPromptStatus,
  insertPromptPlaceholder,
  summarizePromptChange,
} from "./agent-prompt-view.mjs";

test("formatPromptStatus 区分默认与自定义", () => {
  assert.equal(
    formatPromptStatus({ isCustom: false, text: "abc", defaultText: "abc", placeholders: [] }),
    "当前使用：LytVPK 内置默认提示词",
  );
  assert.equal(
    formatPromptStatus({
      isCustom: true,
      text: "abc",
      defaultText: "default",
      placeholders: [],
      savedAt: "2026-09-23T01:00:00+08:00",
      path: "C:/Users/x/AppData/Roaming/LytVPK/group_suggestion_agent_prompt.md",
    }),
    "当前使用：你自己的提示词（保存于 2026-09-23T01:00:00+08:00）",
  );
  // 有改动但还没保存时要提示
  assert.equal(
    formatPromptStatus({ isCustom: true, text: "原始内容", defaultText: "default", placeholders: [] }, "改过了"),
    "当前使用：你自己的提示词 · 有未保存的修改",
  );
  assert.equal(
    formatPromptStatus({ isCustom: true, text: "原始内容", defaultText: "default", placeholders: [] }, "原始内容"),
    "当前使用：你自己的提示词",
  );
});

test("insertPromptPlaceholder 在光标处插入并返回新的光标位置", () => {
  assert.deepEqual(insertPromptPlaceholder("清单：\n", "{{CATALOG_PATH}}", 3, 3), {
    text: "清单：{{CATALOG_PATH}}\n",
    caret: 3 + "{{CATALOG_PATH}}".length,
  });
  // 选中一段内容时替换选区
  assert.deepEqual(insertPromptPlaceholder("清单：OLD", "{{INBOX_PATH}}", 3, 6), {
    text: "清单：{{INBOX_PATH}}",
    caret: 3 + "{{INBOX_PATH}}".length,
  });
  // 越界的光标要被夹到合法范围
  assert.deepEqual(insertPromptPlaceholder("abc", "{{MOD_COUNT}}", 99, 99), {
    text: "abc{{MOD_COUNT}}",
    caret: 3 + "{{MOD_COUNT}}".length,
  });
});

test("summarizePromptChange 说明保存/恢复默认的结果", () => {
  assert.equal(
    summarizePromptChange("save", { isCustom: true }),
    "已保存：以后「准备给智能体的材料」「复制提示词」「保存提示词…」都用这一套",
  );
  assert.equal(
    summarizePromptChange("reset", { isCustom: false }),
    "已恢复 LytVPK 内置默认提示词",
  );
});
