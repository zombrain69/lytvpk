// 组归属刷新必须"内容没变就不通知"。
//
// 背景（真机量化）：每次搜索结束后 filters.js 都会在后台刷一次归属
// （refreshModGroupMembershipState），而监听方（group-ui）收到通知会**重画整份 Mod 列表**。
// 归属其实没变时，这一次重画把 2904 张卡白白造第二遍 —— 点一下搜索要等两轮渲染。

import assert from "node:assert/strict";
import test from "node:test";
import { register } from "node:module";

register("../../test-utils/extension-resolve-hook.mjs", import.meta.url);

function installStubs(membershipsRef) {
  globalThis.document = {
    getElementById: () => null,
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener() {},
    body: {},
  };
  globalThis.window = {
    go: {
      app: {
        App: {
          GetModGroupMembership: async () => (typeof membershipsRef.value === "function"
            ? membershipsRef.value()
            : membershipsRef.value),
        },
      },
    },
    addEventListener() {},
    localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
  };
}

test("归属内容没变时不通知监听方（否则每次搜索都白白重画一遍列表）", async () => {
  const holder = {
    value: [
      {
        key: "a.vpk",
        groupId: "g1",
        groupName: "组A",
        strategy: "all",
        enforce: false,
        memberCount: 1,
        missing: false,
      },
    ],
  };
  installStubs(holder);
  const { onModGroupMembershipChanged, refreshModGroupMembershipState } = await import(
    "./group-state.mjs"
  );

  let notifications = 0;
  const off = onModGroupMembershipChanged(() => {
    notifications += 1;
  });
  try {
    await refreshModGroupMembershipState({ silent: true });
    assert.equal(notifications, 1, "首次加载必须通知（徽标索引与筛选菜单靠它建立）");

    await refreshModGroupMembershipState({ silent: true });
    await refreshModGroupMembershipState({ silent: true });
    assert.equal(notifications, 1, "内容没变不应该再通知");

    holder.value = [
      ...holder.value,
      {
        key: "b.vpk",
        groupId: "g1",
        groupName: "组A",
        strategy: "all",
        enforce: false,
        memberCount: 2,
        missing: false,
      },
    ];
    await refreshModGroupMembershipState({ silent: true });
    assert.equal(notifications, 2, "成员变化必须通知");

    // 改名也算变化：只有 groupName 改了（groupId / key 都没变）。
    holder.value = holder.value.map((item) => ({ ...item, groupName: "组A（改名）" }));
    await refreshModGroupMembershipState({ silent: true });
    assert.equal(notifications, 3, "组名变化必须通知");

    // 缺失标记翻转也要通知：界面上的 ⚠️ 提示靠它。
    holder.value = holder.value.map((item) => ({ ...item, missing: true }));
    await refreshModGroupMembershipState({ silent: true });
    assert.equal(notifications, 4, "缺失状态变化必须通知");
  } finally {
    off();
  }
});
