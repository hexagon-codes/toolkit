#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
cd -- "$script_dir/.."
evidence_dir="${RUNNER_TEMP:?RUNNER_TEMP is required}"
# 身份切换后的挂载解析不能依赖 runner 私有目录的遍历权限。
# 共享父目录只提供遍历，HOME/cache/tmp 仍由各身份以 0700 独占。
runtime_dir="$(mktemp -d /tmp/toolkit-linux-ci.XXXXXX)"
chmod 0755 "$runtime_dir"
trap 'sudo rm -rf -- "$runtime_dir"' EXIT
trusted_path="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
go_bin="$(command -v go)"
go_root="$(go env GOROOT)"
expected_go="go$(awk '$1 == "go" {print $2; exit}' go.mod)"
[[ "$(go env GOOS)" == linux && "$(go env GOVERSION)" == "$expected_go" ]] || {
  echo "Expected the native Linux minimum Go toolchain." >&2
  exit 1
}

# 探针只绑定真实载荷及其加载依赖，避免空根文件系统造成能力误判。
payload=/usr/bin/true
bindings=(--ro-bind "$payload" "$payload")
dependencies="$(ldd "$payload")"
while IFS= read -r dependency; do
  [[ -f "$dependency" ]] || { echo "Missing probe dependency: $dependency" >&2; exit 1; }
  bindings+=(--ro-bind "$dependency" "$dependency")
done < <(awk '{for (i=1; i<=NF; i++) if ($i ~ /^\//) print $i}' <<<"$dependencies")

root_tests=(
  TestLinuxBoundaryProbeRootDropsCapabilitiesButRejectsIneffectiveProcessLimit
  TestLinuxRootExecReportsProcessesNotRequestedWithoutFalseEnforcement
  TestLinuxRootRequiredProcessesRejectsBeforePayload
  TestLinuxBwrapArgsDropCapabilitiesAndIsolateNamespaces
  TestLinuxMissingDeniedPathRejectsPayloadBeforeLaunch
  TestLinuxDeniedPathCreatedAfterNewRemainsHiddenAndReadOnly
  TestLinuxDeniedDirectoryCannotBeReadOrWritten
  TestLinuxWorkspaceRejectsHardlinkToExternalFile
  TestLinuxPreLaunchIdentityRecheckRejectsDirectoryReplacement
  TestLinuxPreLaunchIdentityRecheckRejectsCommandReplacement
  TestLinuxPreLaunchAuditRejectsNewExternalHardlink
  TestLinuxProcessContainmentTerminationIsBackedByPIDNamespace
  TestLinuxRootBwrapHidesHostUnixSocket
)
nonroot_tests=(
  TestLinuxBoundaryProbeNonRootProvesProcessLimitWithEAGAIN
  TestLinuxNonRootExecSeparatesAvailableProcessesCapabilityFromRequest
  TestLinuxNonRootConfiguredProcessLimitReachesEAGAIN
  TestLinuxBwrapHidesExternalWorkspaceWithoutPrivilege
)

module_cache="$runtime_dir/modcache"
cache_ready=false
for identity in root nonroot; do
  home_dir="$runtime_dir/$identity-home"
  cache_dir="$runtime_dir/$identity-cache"
  tmp_dir="$runtime_dir/$identity-tmp"
  launcher=(sudo)
  required=("${root_tests[@]}")
  if [[ "$identity" == nonroot ]]; then
    launcher+=(-u nobody)
    required=("${nonroot_tests[@]}")
    sudo install -d -o nobody -g "$(id -gn nobody)" -m 0700 "$home_dir" "$cache_dir" "$tmp_dir"
  else
    sudo install -d -m 0700 "$home_dir" "$cache_dir" "$tmp_dir"
  fi
  probe_log="$evidence_dir/linux-$identity-bwrap-probe.log"
  if "${launcher[@]}" env -i HOME="$home_dir" PATH="$trusted_path" \
    /usr/bin/bwrap "${bindings[@]}" --unshare-user --unshare-pid --unshare-ipc --unshare-uts \
    --disable-userns --assert-userns-disabled --unshare-net --new-session --die-with-parent \
    --cap-drop ALL --uid 65534 --gid 65534 --proc /proc --dev /dev -- "$payload" >"$probe_log" 2>&1; then
    echo "$identity bubblewrap capability probe passed"
  else
    probe_exit=$?
    cat "$probe_log" >&2
    if ! grep -Eqi 'namespace|operation not permitted|permission denied|uid map|gid map' "$probe_log"; then
      echo "Sandbox probe failed before proving an environment capability limitation." >&2
      exit "$probe_exit"
    fi
    echo "::notice::$identity bubblewrap unavailable (exit $probe_exit); dedicated checks skipped"
    if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
      echo "- $identity sandbox: skipped; capability probe exit $probe_exit (see probe log)" >> "$GITHUB_STEP_SUMMARY"
    fi
    continue
  fi
  if [[ "$cache_ready" == false ]]; then
    GOMODCACHE="$module_cache" GOCACHE="$runtime_dir/prewarm-cache" \
      go test -mod=readonly -run '^$' ./os/sandbox
    chmod -R a+rX "$module_cache"
    cache_ready=true
  fi
  alternation="$(IFS='|'; echo "${required[*]}")"
  events="$evidence_dir/linux-$identity-sandbox-security.jsonl"
  "${launcher[@]}" env -i HOME="$home_dir" TMPDIR="$tmp_dir" \
    PATH="$go_root/bin:$trusted_path" GOROOT="$go_root" GOTOOLCHAIN=local GOWORK=off GOENV=off \
    GOPATH="$home_dir/go" GOCACHE="$cache_dir" GOMODCACHE="$module_cache" GOPROXY=off GOSUMDB=off \
    "$go_bin" test -json -count=1 -timeout 600s ./os/sandbox -run "^($alternation)$" | tee "$events"
  python3 "$script_dir/verify-go-test-events.py" "$events" github.com/hexagon-codes/toolkit/os/sandbox "${required[@]}"
done
