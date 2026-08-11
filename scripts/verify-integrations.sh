#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "$script_dir/.." && pwd -P)"
events_dir="$(mktemp -d)"

mysql_container="toolkit-mysql-${PPID}-$$"
redis_container="toolkit-redis-${PPID}-$$"
mysql_password="toolkit-integration-password"
redis_username="toolkit"
redis_password="toolkit-integration-password"
# 2026-08-11 核验 Docker Hub 官方多架构索引，固定到 MySQL 8.4.11 与 Redis 7.4.10。
mysql_image="mysql:8.4@sha256:b3b90af2a6552ae30c266fdb7d5dd55f3afb72404bb78d37fe8a23eb857fd3fb"
redis_image="redis:7.4@sha256:e9b2e45ecd47fbb69b877cf8d045d5cccaaaed52524b6e098b4abe8212994f73"

cleanup() {
  docker rm -f "$mysql_container" "$redis_container" >/dev/null 2>&1 || true
  if [[ -n "${events_dir:-}" && -d "$events_dir" && "$events_dir" != "/" ]]; then
    rm -rf -- "$events_dir"
  fi
}
trap cleanup EXIT

command -v docker >/dev/null 2>&1 || {
  printf '%s\n' "docker is required to run integration tests" >&2
  exit 1
}
command -v python3 >/dev/null 2>&1 || {
  printf '%s\n' "python3 is required to validate integration test events" >&2
  exit 1
}
docker info >/dev/null

docker run --detach --name "$mysql_container" \
  --publish 127.0.0.1::3306 \
  --env MYSQL_ROOT_PASSWORD="$mysql_password" \
  --env MYSQL_DATABASE=toolkit_test \
  "$mysql_image" >/dev/null

docker run --detach --name "$redis_container" \
  --publish 127.0.0.1::6379 \
  "$redis_image" \
  redis-server --save "" --appendonly no \
  --user default off \
  --user "$redis_username" on ">$redis_password" "~*" "+@all" >/dev/null

for _ in $(seq 1 90); do
  if docker exec "$mysql_container" mysqladmin ping \
    --host=127.0.0.1 --user=root --password="$mysql_password" --silent >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$mysql_container" mysqladmin ping \
  --host=127.0.0.1 --user=root --password="$mysql_password" --silent >/dev/null

for _ in $(seq 1 30); do
  if docker exec --env REDISCLI_AUTH="$redis_password" "$redis_container" \
    redis-cli --user "$redis_username" ping 2>/dev/null | grep -qx PONG; then
    break
  fi
  sleep 1
done
docker exec --env REDISCLI_AUTH="$redis_password" "$redis_container" \
  redis-cli --user "$redis_username" ping 2>/dev/null | grep -qx PONG

mysql_port="$(docker port "$mysql_container" 3306/tcp | sed -n 's/^127\.0\.0\.1://p' | head -n 1)"
redis_port="$(docker port "$redis_container" 6379/tcp | sed -n 's/^127\.0\.0\.1://p' | head -n 1)"
if [[ -z "$mysql_port" || -z "$redis_port" ]]; then
  printf '%s\n' "failed to resolve integration container ports" >&2
  exit 1
fi

cd -- "$repo_root"

mysql_tests=(
  TestIntegration_BasicOperations
  TestIntegration_Transaction
  TestIntegration_Health
  TestIntegration_Stats
  TestIntegration_Timeout
  TestTransaction_Panic
  TestStats_NonNilDB
  TestExecWithTimeout_Success
  TestQueryWithTimeout_Success
  TestClose_ValidDB
)
mysql_alternation="$(IFS='|'; echo "${mysql_tests[*]}")"
mysql_pattern="^(${mysql_alternation})$"
mysql_listed_tests="$(GOWORK=off go test -list "$mysql_pattern" ./infra/db/mysql)"
for test_name in "${mysql_tests[@]}"; do
  if ! grep -Fxq "$test_name" <<<"$mysql_listed_tests"; then
    printf '%s\n' "Required MySQL integration test is missing: $test_name" >&2
    exit 1
  fi
done
mysql_collected_count="$(awk '/^Test/ { count++ } END { print count + 0 }' <<<"$mysql_listed_tests")"
if [[ "$mysql_collected_count" -ne "${#mysql_tests[@]}" ]]; then
  printf '%s\n' "MySQL integration filter collected $mysql_collected_count tests; expected ${#mysql_tests[@]}" >&2
  exit 1
fi

mysql_events="$events_dir/mysql.jsonl"
set +e
TEST_MYSQL_DSN="root:${mysql_password}@tcp(127.0.0.1:${mysql_port})/toolkit_test?parseTime=true&charset=utf8mb4" \
  GOWORK=off go test -json -count=1 -timeout=120s ./infra/db/mysql -run "$mysql_pattern" | tee "$mysql_events"
mysql_test_exit=${PIPESTATUS[0]}
set -e
if [[ "$mysql_test_exit" -ne 0 ]]; then
  exit "$mysql_test_exit"
fi

redis_tests=(TestOpenAgainstExternalNamedACL)
redis_pattern='^(TestOpenAgainstExternalNamedACL)$'
redis_listed_tests="$(GOWORK=off go test -list "$redis_pattern" ./infra/redisconn)"
if ! grep -Fxq "${redis_tests[0]}" <<<"$redis_listed_tests"; then
  printf '%s\n' "Required Redis integration test is missing: ${redis_tests[0]}" >&2
  exit 1
fi
redis_collected_count="$(awk '/^Test/ { count++ } END { print count + 0 }' <<<"$redis_listed_tests")"
if [[ "$redis_collected_count" -ne "${#redis_tests[@]}" ]]; then
  printf '%s\n' "Redis integration filter collected $redis_collected_count tests; expected ${#redis_tests[@]}" >&2
  exit 1
fi

redis_events="$events_dir/redis.jsonl"
set +e
REDISCONN_TEST_ADDR="127.0.0.1:${redis_port}" \
  REDISCONN_TEST_USERNAME="$redis_username" \
  REDISCONN_TEST_PASSWORD="$redis_password" \
  GOWORK=off go test -json -count=1 -timeout=60s ./infra/redisconn -run "$redis_pattern" | tee "$redis_events"
redis_test_exit=${PIPESTATUS[0]}
set -e
if [[ "$redis_test_exit" -ne 0 ]]; then
  exit "$redis_test_exit"
fi

# 解析 JSON 事件，严格禁止必需测试被跳过或缺少唯一顶层通过事件。
python3 - "$mysql_events" "${mysql_tests[@]}" -- "$redis_events" "${redis_tests[@]}" <<'PY'
import json
import sys


def verify(path, required):
    events = []
    with open(path, encoding="utf-8") as stream:
        for line_number, line in enumerate(stream, 1):
            line = line.strip()
            if not line:
                continue
            try:
                events.append(json.loads(line))
            except json.JSONDecodeError as error:
                raise SystemExit(f"Invalid Go test JSON at {path}:{line_number}: {error}")

    for test_name in required:
        skipped = [
            event for event in events
            if event.get("Action") == "skip"
            and (event.get("Test") == test_name or str(event.get("Test", "")).startswith(test_name + "/"))
        ]
        if skipped:
            raise SystemExit(f"Required integration test was skipped: {test_name}")
        passed = [
            event for event in events
            if event.get("Action") == "pass" and event.get("Test") == test_name
        ]
        if len(passed) != 1:
            raise SystemExit(
                f"Required integration test must produce exactly one top-level pass event: {test_name}; got {len(passed)}"
            )


separator = sys.argv.index("--")
verify(sys.argv[1], sys.argv[2:separator])
verify(sys.argv[separator + 1], sys.argv[separator + 2:])
PY
