#!/bin/bash

set -euo pipefail

export PATH="/usr/sbin:/usr/bin:/sbin:/bin"
unset CURL_HOME XDG_CONFIG_HOME CURL_CA_BUNDLE SSL_CERT_FILE SSL_CERT_DIR

readonly BUBBLEWRAP_VERSION="0.11.2"
readonly BUBBLEWRAP_SHA256="69abc30005d2186baf7737feacd8da35633b93cf5af38838ecff17c5f8e924f6"
readonly BUBBLEWRAP_URL="https://github.com/containers/bubblewrap/releases/download/v${BUBBLEWRAP_VERSION}/bubblewrap-${BUBBLEWRAP_VERSION}.tar.xz"
readonly BUBBLEWRAP_INSTALL_PATH="/usr/bin/bwrap"

if [[ "$(/usr/bin/uname -s)" != "Linux" ]]; then
  echo "Bubblewrap installation requires Linux." >&2
  exit 1
fi
if [[ ! -x /usr/bin/apt-get ]]; then
  echo "Bubblewrap installation requires apt-get." >&2
  exit 1
fi

if ((EUID == 0)); then
  readonly -a privilege=()
else
  if [[ ! -x /usr/bin/sudo ]]; then
    echo "Bubblewrap installation requires root privileges or sudo." >&2
    exit 1
  fi
  readonly -a privilege=(/usr/bin/sudo)
fi

# 只安装构建与 Linux 边界测试必需的依赖，不安装发行版自带的旧 Bubblewrap。
"${privilege[@]}" /usr/bin/env DEBIAN_FRONTEND=noninteractive /usr/bin/apt-get update
"${privilege[@]}" /usr/bin/env DEBIAN_FRONTEND=noninteractive /usr/bin/apt-get install -y --no-install-recommends \
  ca-certificates \
  curl \
  gcc \
  libc6-dev \
  libcap-dev \
  meson \
  ninja-build \
  pkg-config \
  util-linux \
  xz-utils

readonly temporary_base="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
if [[ ! -d "$temporary_base" ]]; then
  echo "Temporary directory does not exist: $temporary_base" >&2
  exit 1
fi
build_root="$(/usr/bin/mktemp -d "${temporary_base%/}/toolkit-bubblewrap.XXXXXX")"
readonly build_root
cleanup() {
  if [[ -n "${build_root:-}" && -d "$build_root" ]]; then
    /bin/rm -rf -- "$build_root"
  fi
}
trap cleanup EXIT

readonly archive="$build_root/bubblewrap-${BUBBLEWRAP_VERSION}.tar.xz"
readonly source_dir="$build_root/bubblewrap-${BUBBLEWRAP_VERSION}"
readonly primary_output_dir="$build_root/build-primary"
readonly verification_output_dir="$build_root/build-verification"

/usr/bin/curl --disable --fail --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 \
  --output "$archive" \
  "$BUBBLEWRAP_URL"
/usr/bin/printf '%s  %s\n' "$BUBBLEWRAP_SHA256" "$archive" | /usr/bin/sha256sum --check --strict -
/usr/bin/tar --extract --xz --file "$archive" --directory "$build_root"
if [[ ! -d "$source_dir" ]]; then
  echo "Bubblewrap source directory is missing after extraction." >&2
  exit 1
fi

build_bubblewrap() {
  local output_dir="$1"

  # Jammy GCC 11 在 release 的 -O3 下会误报 format-overflow；debugoptimized 保持 O2 和断言。
  # 两个 prefix-map 消除随机构建目录进入二进制和调试信息，确保同源码双构建可复现。
  /usr/bin/meson setup "$output_dir" "$source_dir" \
    --buildtype=debugoptimized \
    --prefix=/usr \
    -Db_ndebug=false \
    -Dman=disabled \
    -Dselinux=disabled \
    -Dsupport_setuid=false \
    -Dtests=false \
    "-Dc_args=-ffile-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source -fdebug-prefix-map=${source_dir}=/usr/src/toolkit-bubblewrap/source -ffile-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build -fdebug-prefix-map=${output_dir}=/usr/src/toolkit-bubblewrap/build"
  /usr/bin/meson compile -C "$output_dir"
  if [[ ! -x "$output_dir/bwrap" ]]; then
    echo "Bubblewrap build did not produce an executable." >&2
    exit 1
  fi
}

verify_reproducible_binary() {
  local primary_binary="$1"
  local verification_binary="$2"
  local primary_hash_file="$build_root/primary.sha256"
  local verification_hash_file="$build_root/verification.sha256"
  local primary_hash
  local verification_hash

  /usr/bin/sha256sum "$primary_binary" >"$primary_hash_file"
  /usr/bin/sha256sum "$verification_binary" >"$verification_hash_file"
  read -r primary_hash _ <"$primary_hash_file"
  read -r verification_hash _ <"$verification_hash_file"
  if [[ "$primary_hash" != "$verification_hash" ]]; then
    echo "Bubblewrap builds are not reproducible: first '$primary_hash', second '$verification_hash'." >&2
    exit 1
  fi
}

build_bubblewrap "$primary_output_dir"
build_bubblewrap "$verification_output_dir"
verify_reproducible_binary "$primary_output_dir/bwrap" "$verification_output_dir/bwrap"

"${privilege[@]}" /usr/bin/install -o root -g root -m 0755 "$primary_output_dir/bwrap" "$BUBBLEWRAP_INSTALL_PATH"

readonly expected_version="bubblewrap ${BUBBLEWRAP_VERSION}"
actual_version="$($BUBBLEWRAP_INSTALL_PATH --version)"
readonly actual_version
if [[ "$actual_version" != "$expected_version" ]]; then
  echo "Unexpected Bubblewrap version: expected '$expected_version', got '$actual_version'." >&2
  exit 1
fi
if [[ ! -f "$BUBBLEWRAP_INSTALL_PATH" || -L "$BUBBLEWRAP_INSTALL_PATH" || ! -x "$BUBBLEWRAP_INSTALL_PATH" ]]; then
  echo "Bubblewrap must be a regular executable file." >&2
  exit 1
fi
if [[ -u "$BUBBLEWRAP_INSTALL_PATH" ]]; then
  echo "Bubblewrap must not have the setuid bit." >&2
  exit 1
fi

actual_owner="$(/usr/bin/stat --format='%U:%G' "$BUBBLEWRAP_INSTALL_PATH")"
readonly actual_owner
if [[ "$actual_owner" != "root:root" ]]; then
  echo "Unexpected Bubblewrap owner: expected 'root:root', got '$actual_owner'." >&2
  exit 1
fi
actual_mode="$(/usr/bin/stat --format='%a' "$BUBBLEWRAP_INSTALL_PATH")"
readonly actual_mode
if [[ "$actual_mode" != "755" ]]; then
  echo "Unexpected Bubblewrap mode: expected '755', got '$actual_mode'." >&2
  exit 1
fi

help_output="$($BUBBLEWRAP_INSTALL_PATH --help 2>&1)"
readonly help_output
for required_flag in --disable-userns --assert-userns-disabled; do
  if ! /usr/bin/grep -Fq -- "$required_flag" <<<"$help_output"; then
    echo "Bubblewrap is missing required flag: $required_flag" >&2
    exit 1
  fi
done

echo "Installed reproducible Bubblewrap ${BUBBLEWRAP_VERSION} at ${BUBBLEWRAP_INSTALL_PATH}."
