#!/usr/bin/env python3
r"""对 CUA 调试桥求值一段 JS：在真实页面里读 DOM / 派发事件 / 量几何。

用法（先按 manual-verification.md §0 起沙箱，端口要与 -Port 一致）：

    python scripts/devtools/cua-eval.py "document.title"
    python scripts/devtools/cua-eval.py --file probe.js
    python scripts/devtools/cua-eval.py --base http://127.0.0.1:38998 "location.href"
    Get-Content probe.js | python scripts/devtools/cua-eval.py -

为什么要有这个脚本（而不是每次现写）：桥的 `exec` 把入参包成 `await ( <js> )`，
所以多语句 JS 会变成语法错误 —— 页面根本不执行，表现为"求值超时"，很容易被误判成
应用卡死。这里统一用 `eval()` 包一层：既能收多语句，也能把最后一个表达式的值带回来，
并且把错误也当成正常返回值（前缀 `ERR: `）。

约定：调试产物只落在这三处 —— `.tmp-cua/`（沙箱与探针）、`%TEMP%\lytvpk-cua-*.log`
（桥与子进程日志）、`build/bin/*-cua.exe`（带桥调试 EXE）。收尾时删掉即可，脚本会按需重建。
"""

import argparse
import io
import json
import sys
import urllib.error
import urllib.request


def eval_sync(base, js, timeout_ms):
    """把 JS 包成 eval() 后交给 /eval-sync，返回解析后的响应。"""
    wrapped = (
        "(async () => { try { return await eval(%s); } "
        'catch (error) { return "ERR: " + String((error && error.stack) || error); } })()'
    ) % json.dumps(js)
    payload = json.dumps({"js": wrapped, "timeoutMs": timeout_ms}).encode("utf-8")
    request = urllib.request.Request(
        base.rstrip("/") + "/eval-sync",
        data=payload,
        headers={"Content-Type": "application/json"},
    )
    # 绕开系统代理：桥只监听回环，代理会把请求带偏。
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        raw = opener.open(request, timeout=timeout_ms / 1000 + 20).read()
    except urllib.error.HTTPError as error:
        return {"ok": False, "error": "HTTP %s" % error.code}
    except urllib.error.URLError as error:
        return {"ok": False, "error": "连不上桥（%s）：先跑 scripts/devtools/launch-cua-sandbox.ps1" % error.reason}
    return json.loads(raw.decode("utf-8"))


def main():
    # Windows 控制台默认 GBK，页面里回来的 emoji（比如列表徽标 📦）会让 print 直接抛
    # UnicodeEncodeError。这里把 stdout/stderr 切到 UTF-8 并允许替换，保证脚本能跑完；
    # 需要精确字节时用 --json-out 落文件。
    for stream in (sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding="utf-8", errors="replace")
        except (AttributeError, ValueError):
            pass

    parser = argparse.ArgumentParser(description="对 CUA 调试桥求值 JS")
    parser.add_argument("js", nargs="?", help="要执行的 JS，或放脚本的文件路径，或 - 表示从 stdin 读")
    parser.add_argument("--file", dest="js_file", help="从文件读取 JS")
    parser.add_argument("--base", default="http://127.0.0.1:38999", help="桥地址（默认 38999）")
    parser.add_argument("--timeout-ms", type=int, default=60000, help="求值超时（默认 60s）")
    parser.add_argument("--json-out", help="把完整响应写到这个文件（默认打印到 stdout）")
    args = parser.parse_args()

    if args.js_file:
        with io.open(args.js_file, "r", encoding="utf-8") as handle:
            js = handle.read()
    elif args.js in (None, "-"):
        js = sys.stdin.read()
    elif args.js.endswith(".js"):
        with io.open(args.js, "r", encoding="utf-8") as handle:
            js = handle.read()
    else:
        js = args.js

    if not js.strip():
        parser.error("没有要执行的 JS")

    result = eval_sync(args.base, js, args.timeout_ms)
    text = json.dumps(result, ensure_ascii=False, indent=2)
    if args.json_out:
        with io.open(args.json_out, "w", encoding="utf-8") as handle:
            handle.write(text)
        print("->", args.json_out)
    else:
        print(text)
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
