#!/usr/bin/env python3
"""核对既有必需行为的实际 Go 测试事件，不重复执行测试。"""

import json
import sys
from pathlib import Path


def verify(path, package, required):
    events = [json.loads(line) for line in Path(path).read_text().splitlines() if line.strip()]
    for name in required:
        selected = [event for event in events if event.get("Package") == package]
        if any(event.get("Action") == "skip" and
               (event.get("Test") == name or event.get("Test", "").startswith(name + "/"))
               for event in selected):
            raise SystemExit(f"Required test was skipped: {name}")
        passed = sum(event.get("Action") == "pass" and event.get("Test") == name for event in selected)
        if passed != 1:
            raise SystemExit(f"Required test must pass exactly once: {name}; got {passed}")
    print(f"Verified {len(required)} required behaviors in {package}")


if __name__ == "__main__":
    if len(sys.argv) < 4:
        raise SystemExit("Usage: verify-go-test-events.py EVENTS PACKAGE TEST...")
    verify(sys.argv[1], sys.argv[2], sys.argv[3:])
