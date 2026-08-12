#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
cd -- "$script_dir/.."

# actionlint 在缺少 ShellCheck 时会静默略过 shell 检查，本地与 CI 必须具备相同能力。
if ! command -v shellcheck >/dev/null 2>&1; then
  echo "ShellCheck is required for workflow validation. Install ShellCheck and run this command again." >&2
  exit 1
fi

GOWORK=off go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck shellcheck "$@"
