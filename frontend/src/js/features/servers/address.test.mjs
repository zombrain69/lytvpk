import assert from "node:assert/strict";
import test from "node:test";

import {
  DEFAULT_SERVER_PORT,
  findDuplicateServer,
  normalizeServerAddress,
  parseServerInput,
} from "./address.js";

test("normalizeServerAddress 补默认端口", () => {
  assert.equal(normalizeServerAddress("127.0.0.1"), `127.0.0.1:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("example.com"), `example.com:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("example.com:"), `example.com:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("example.com:27016"), "example.com:27016");
  assert.equal(normalizeServerAddress("  example.com:27016  "), "example.com:27016");
});

test("normalizeServerAddress 处理 IPv6", () => {
  assert.equal(normalizeServerAddress("2001:db8::1"), `[2001:db8::1]:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("[2001:db8::1]"), `[2001:db8::1]:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("[2001:db8::1]:27016"), "[2001:db8::1]:27016");
});

test("normalizeServerAddress 拒绝非法输入（消息可直接展示）", () => {
  const cases = [
    "",
    ":27015",
    "example.com:not-a-port",
    "example.com:0",
    "example.com:65536",
    "steam://connect/example.com",
    "[not-ipv6]",
    "example.com/evil",
    "example.com?x=1",
    "example .com",
  ];
  for (const input of cases) {
    assert.throws(() => normalizeServerAddress(input), /./u, `${input} 应被拒绝`);
  }
});

test("parseServerInput 兼容 IP 直连的常见写法", () => {
  const cases = [
    ["127.0.0.1", `127.0.0.1:${DEFAULT_SERVER_PORT}`],
    ["127.0.0.1:27016", "127.0.0.1:27016"],
    ["[::1]:27015", "[::1]:27015"],
    ["connect 127.0.0.1:27015", "127.0.0.1:27015"],
    ["connect 127.0.0.1", `127.0.0.1:${DEFAULT_SERVER_PORT}`],
    ["steam://connect/127.0.0.1:27015", "127.0.0.1:27015"],
    ["steam://connect/example.com", `example.com:${DEFAULT_SERVER_PORT}`],
    ['"127.0.0.1:27015"', "127.0.0.1:27015"],
    ["  connect   example.com:27016  ", "example.com:27016"],
  ];
  for (const [input, expected] of cases) {
    assert.equal(parseServerInput(input), expected, input);
  }
});

test("parseServerInput 只有 connect 或空输入时给出可读提示", () => {
  for (const input of ["", "   ", "connect", "connect ", "CONNECT "]) {
    assert.throws(() => parseServerInput(input), /请输入服务器 IP 或域名/u, input);
  }
});

// 真机联调里同一台测试服务器被存了 3 份（每次「添加服务器」都成功）：
// 列表重复显示，而且每次打开收藏服务器都要各查一遍。
test("findDuplicateServer 按规范化后的地址找出重复收藏", () => {
  const servers = [
    { id: "a", name: "主机一", address: "127.0.0.1:27055" },
    { id: "b", name: "主机二", address: "example.com:27016" },
  ];

  // 写法不同但规范化后相同的算重复（不写端口 ⇒ 27015）
  assert.equal(findDuplicateServer(servers, "127.0.0.1:27055")?.id, "a");
  assert.equal(findDuplicateServer([...servers, { id: "c", address: "home.example.com" }], "home.example.com")?.id, "c");
  assert.equal(findDuplicateServer(servers, "example.com"), null, "端口不同的不算重复");
  assert.equal(findDuplicateServer(servers, "127.0.0.2:27055"), null);

  // 编辑自己时不能被自己拦下
  assert.equal(findDuplicateServer(servers, "127.0.0.1:27055", { excludeId: "a" }), null);
  assert.equal(findDuplicateServer(servers, "127.0.0.1:27055", { excludeIndex: 0 }), null);
  assert.equal(findDuplicateServer(servers, "127.0.0.1:27055", { excludeIndex: 1 })?.id, "a");

  // 历史数据里可能有非法地址：退化成去空白的小写比较，仍然能拦下明显重复
  assert.equal(findDuplicateServer([{ id: "x", address: "坏 地址" }], "坏 地址")?.id, "x");
  assert.equal(findDuplicateServer(null, "127.0.0.1:27055"), null);
  assert.equal(findDuplicateServer(servers, ""), null);
});
