#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "$script_dir/.." && pwd -P)"

mysql_container="toolkit-mysql-${PPID}-$$"
redis_container="toolkit-redis-${PPID}-$$"
mysql_password="toolkit-integration-password"
redis_username="toolkit"
redis_password="toolkit-integration-password"

cleanup() {
  docker rm -f "$mysql_container" "$redis_container" >/dev/null 2>&1 || true
}
trap cleanup EXIT

command -v docker >/dev/null 2>&1 || {
  printf '%s\n' "docker is required to run integration tests" >&2
  exit 1
}
docker info >/dev/null

docker run --detach --name "$mysql_container" \
  --publish 127.0.0.1::3306 \
  --env MYSQL_ROOT_PASSWORD="$mysql_password" \
  --env MYSQL_DATABASE=toolkit_test \
  mysql:8.4 >/dev/null

docker run --detach --name "$redis_container" \
  --publish 127.0.0.1::6379 \
  redis:7.4 \
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

TEST_MYSQL_DSN="root:${mysql_password}@tcp(127.0.0.1:${mysql_port})/toolkit_test?parseTime=true&charset=utf8mb4" \
  GOWORK=off go test -count=1 -timeout=120s ./infra/db/mysql -run '^(TestIntegration_|TestTransaction_Panic|TestStats_NonNilDB|TestExecWithTimeout_|TestQueryWithTimeout_|TestQueryRowWithTimeout_)'

REDISCONN_TEST_ADDR="127.0.0.1:${redis_port}" \
  REDISCONN_TEST_USERNAME="$redis_username" \
  REDISCONN_TEST_PASSWORD="$redis_password" \
  GOWORK=off go test -count=1 -timeout=60s ./infra/redisconn -run '^TestOpenAgainstExternalNamedACL$'
