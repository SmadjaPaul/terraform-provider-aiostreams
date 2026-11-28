#!/usr/bin/env bash
# Toolchain preparation of the Optio agent pod.
#
# Optio runs this script once per pod, from the repository checkout
# (/opt/optio/repo-init.sh looks for .optio/setup.sh). The agent image has no
# Go: without this script the agent cannot run the provider tests.
#
#   version    : the `go` directive of go.mod, cross-checked with mise.lock
#   source     : the official dl.google.com archive named in mise.lock
#   checksum   : the sha256 of mise.lock, verified before extraction
#   install    : $HOME/.local/optio/go, linked in $HOME/.local/bin (on the PATH
#                of Optio task runs), without privilege, nothing outside $HOME
#   idempotent : an up-to-date installation downloads nothing
#
# A failure never stops the pod: repo-init.sh runs with `set -e` and a task
# waits 120 s at most for the pod, so the download is time-bounded and the exit
# status is 0. The result is one `SETUP_STATUS=OK|FAIL` line in the pod log.
# OPTIO_SETUP_STRICT=1 propagates the failure (tests, manual runs).

install_go() (
  set -euo pipefail

  fail() {
    printf '[optio/setup] ERROR: %s\n' "$*" >&2
    exit 1
  }
  # lock_value SECTION KEY: value of a key of a mise.lock section.
  lock_value() {
    awk -v section="$1" -v key="$2" '
      /^[[:space:]]*\[/ { inside = ($0 == section) }
      inside && $1 == key { gsub(/"/, "", $3); print $3; exit }
    ' "$repo/mise.lock"
  }

  [ -n "${HOME:-}" ] || fail "HOME is not set"
  repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
  root="$HOME/.local/optio/go"
  bin="$HOME/.local/bin"
  mirror="${OPTIO_GO_DOWNLOAD_BASE_URL:-https://dl.google.com/go}"

  [ -f "$repo/go.mod" ] || fail "go.mod not found"
  [ -f "$repo/mise.lock" ] || fail "mise.lock not found"
  version="$(awk '$1 == "go" { print $2; exit }' "$repo/go.mod")"
  [ -n "$version" ] || fail "no go directive in go.mod"
  locked="$(lock_value '[[tools.go]]' version)"
  [ "$locked" = "$version" ] ||
    fail "mise.lock pins go '$locked' but go.mod requires $version: run 'mise lock'"

  case "$(uname -s)/$(uname -m)" in
    Linux/x86_64) platform=linux-x64 ;;
    Linux/aarch64 | Linux/arm64) platform=linux-arm64 ;;
    Darwin/arm64) platform=macos-arm64 ;;
    *) fail "unsupported platform $(uname -s)/$(uname -m)" ;;
  esac

  if "$root/bin/go" version 2>/dev/null | grep -qF "go$version "; then
    printf '[optio/setup] go %s is already installed\n' "$version"
  else
    section="[tools.go.\"platforms.$platform\"]"
    url="$(lock_value "$section" url)"
    expected="$(lock_value "$section" checksum)"
    expected="${expected#sha256:}"
    [ -n "$url" ] && [ -n "$expected" ] || fail "mise.lock has no archive or checksum for $platform"
    case "$url" in
      https://dl.google.com/go/*) ;;
      *) fail "$url is not the official Go download" ;;
    esac
    archive="${url##*/}"

    tmp="$(mktemp -d "${TMPDIR:-/tmp}/optio-go-XXXXXX")" || fail "cannot create a temporary directory"
    trap 'rm -rf "$tmp"' EXIT
    printf '[optio/setup] downloading %s\n' "$archive"
    curl -fsSL --connect-timeout 10 --max-time "${OPTIO_GO_DOWNLOAD_TIMEOUT:-60}" \
      -o "$tmp/$archive" "$mirror/$archive" || fail "download of $archive failed"
    if command -v sha256sum >/dev/null 2>&1; then
      actual="$(sha256sum "$tmp/$archive")"
    else
      actual="$(shasum -a 256 "$tmp/$archive")"
    fi
    actual="${actual%% *}"
    [ "$actual" = "$expected" ] ||
      fail "checksum mismatch for $archive: expected $expected, got $actual"
    printf '[optio/setup] checksum verified\n'
    tar -C "$tmp" -xzf "$tmp/$archive" || fail "cannot extract $archive"
    [ -x "$tmp/go/bin/go" ] || fail "$archive holds no go command"
    mkdir -p "$(dirname "$root")"
    rm -rf "$root"
    mv "$tmp/go" "$root"
  fi

  mkdir -p "$bin"
  ln -sf "$root/bin/go" "$bin/go"
  ln -sf "$root/bin/gofmt" "$bin/gofmt"
  if ! command -v gcc >/dev/null 2>&1 && ! command -v cc >/dev/null 2>&1; then
    printf '[optio/setup] WARNING: no C compiler in this image, "go test -race" cannot run (go test ./... can)\n' >&2
  fi
)

# Not in an `if` or `||`: bash would then ignore `set -e` inside the function.
install_go
status=$?
if [ "$status" -eq 0 ]; then
  echo "[optio/setup] SETUP_STATUS=OK go"
  exit 0
fi
echo "[optio/setup] SETUP_STATUS=FAIL go (the pod starts without the Go toolchain)" >&2
[ "${OPTIO_SETUP_STRICT:-0}" = "1" ] && exit "$status"
exit 0
