// favoriteServer 外部协议的接线契约（对齐上游 c1b4972）：
// 后端负责写收藏，前端负责"切到收藏服务器页 + 刷新列表 + 给出提示"。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const runtime = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
const formModal = readFileSync(new URL("./form-modal.js", import.meta.url), "utf8");
const backend = readFileSync(
  new URL("../../../../../internal/app/server_favorite.go", import.meta.url),
  "utf8",
);
const protocol = readFileSync(
  new URL("../../../../../internal/platform/protocol/url_protocol.go", import.meta.url),
  "utf8",
);

test("后端认得 lytvpk://favoriteServer/名字/地址", () => {
  assert.match(protocol, /ProtocolActionFavoriteServer/, "协议要新增 favoriteServer 动作");
  assert.match(protocol, /case "favoriteserver":/, "解析要识别大小写不敏感的动作名");
  assert.match(protocol, /serveraddress\.Normalize/, "地址要统一规范化（补默认端口）");
  assert.match(backend, /func \(a \*App\) addFavoriteServer/, "要能写入收藏");
  assert.match(backend, /normalizeStoredAddress\(server\.Address\) == target/, "按规范化地址幂等");
});

test("前端收到 protocol:favoriteServer 会刷新并提示", () => {
  assert.match(runtime, /EventsOn\("protocol:favoriteServer"/, "要监听新的协议事件");
  assert.match(runtime, /initServerStorage\(\)/, "要先加载服务器存储");
  assert.match(runtime, /renderServers\(\)/, "要重画服务器列表");
  assert.match(runtime, /switchAppPage\("servers"/, "要切到收藏服务器页");
  assert.match(runtime, /已收藏服务器/, "新增时要提示");
  assert.match(runtime, /已在收藏里/, "重复时也要说清楚");
});

test("手动填写的服务器地址与协议走同一套规范化", () => {
  assert.match(
    formModal,
    /import \{[^}]*normalizeServerAddress[^}]*\} from "\.\/address\.js"/,
    "表单要复用地址规范化",
  );
  assert.match(formModal, /address = normalizeServerAddress\(rawAddress\)/, "保存前要规范化");
  assert.match(formModal, /findDuplicateServer\(servers, address/, "保存前要按规范化地址查重");
});
