#!/usr/bin/env python3
"""输出验证使用的源码和 API 输入，作为运行产物归档。"""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys


def git_sha(directory="."):
    return subprocess.check_output(["git", "-C", str(directory), "rev-parse", "HEAD"], text=True).strip()


root = Path(__file__).resolve().parent.parent
os.chdir(root)
candidate = git_sha()
event_sha = os.environ.get("GITHUB_SHA", candidate)
if candidate != event_sha:
    raise SystemExit("Checkout SHA does not match the workflow event SHA")
result = {"candidate_sha": candidate, "event_sha": event_sha,
          "run_id": os.environ.get("GITHUB_RUN_ID"),
          "run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT"),
          "event": os.environ.get("GITHUB_EVENT_NAME"),
          "platform": subprocess.check_output(["go", "env", "GOOS"], text=True).strip(),
          "go_version": subprocess.check_output(["go", "version"], text=True).strip()}
if "--consumers" in sys.argv[2:]:
    refs = json.loads((root / ".github/consumer-baselines.json").read_text())
    result["consumers"] = {name: git_sha(root.parent / name) for name in refs}
if os.environ.get("API_BASE_TAG"):
    baseline = Path(os.environ["API_BREAKING_BASELINE"])
    result.update(base_tag=os.environ["API_BASE_TAG"], base_sha=os.environ["API_BASE_SHA"],
                  baseline_sha256=hashlib.sha256(baseline.read_bytes()).hexdigest(),
                  gorelease_version=os.environ["GORELEASE_VERSION"])
Path(sys.argv[1]).write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result))
