#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "$script_dir/.." && pwd -P)"
examples_dir="$repo_root/examples"

if [[ ! -f "$repo_root/go.mod" || ! -f "$examples_dir/go.mod" ]]; then
  printf '%s\n' "cannot find the toolkit and examples modules from $script_dir" >&2
  exit 1
fi

workspace_dir="$(mktemp -d)"
cleanup() {
  if [[ -n "${workspace_dir:-}" && -d "$workspace_dir" && "$workspace_dir" != "/" ]]; then
    rm -rf -- "$workspace_dir"
  fi
}
trap cleanup EXIT

(
  cd -- "$workspace_dir"
  GOWORK=off go work init "$repo_root" "$examples_dir"
)

export GOWORK="$workspace_dir/go.work"

cd -- "$repo_root"
go list -mod=readonly ./examples/...
go test -mod=readonly -count=1 ./examples/...
go vet -mod=readonly ./examples/...
go build -mod=readonly ./examples/...
