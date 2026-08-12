#!/usr/bin/env python3
"""核对消费者返回的稳定行为合同，避免入口成功掩盖行为缺失。"""

import json
from pathlib import Path
import sys

result = json.loads(Path(sys.argv[1]).read_text())
platform = sys.argv[2]
required = {"metadata", "tool-definition", "missing-packages", "install-command", "sandbox-capabilities"}
if platform in ("linux", "darwin"):
    required.update({"execute-success", "execute-nonzero", "execute-timeout", "crawler-network-policy"})
    required.add("authorized-directory" if platform == "darwin" else "go-module-files")
elif platform != "windows":
    raise SystemExit(f"Unsupported contract platform: {platform}")
if result.get("schema") != 1 or result.get("platform") != platform:
    raise SystemExit("CodeExec contract schema or platform does not match")
items = result.get("behaviors", [])
ids = [item["id"] for item in items]
if len(ids) != len(set(ids)) or not required.issubset(ids):
    raise SystemExit("CodeExec contract has duplicate or missing behaviors")
if any(item.get("status") != "pass" for item in items):
    raise SystemExit("CodeExec contract contains a failed or skipped behavior")
print(f"Verified {len(required)} required CodeExec behaviors on {platform}")
