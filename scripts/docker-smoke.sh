#!/usr/bin/env bash
set -euo pipefail

# Smoke test for the Architon container image.
# Runs the image with --network none on a copy of examples/agent-loop/broken,
# expects exit 2 with both Studio files, and compares them to rv export on the host.
#
# Run from anywhere:
#   bash scripts/docker-smoke.sh
#
# Environment:
#   ARCHITON_IMAGE  image to test; default builds architon:smoke from this repo
#   EXPECT_VERSION  version `rv version` must report inside the image
#   REQUIRE_DOCKER  set to 1 to fail instead of skip when Docker is unavailable

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$repo_root"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  if [[ "${REQUIRE_DOCKER:-0}" == "1" ]]; then
    fail "Docker is not available and REQUIRE_DOCKER=1"
  fi
  echo "SKIP: Docker is not available; the container image smoke test did not run."
  exit 0
fi

image="${ARCHITON_IMAGE:-}"
expect_version="${EXPECT_VERSION:-}"
if [[ -z "$image" ]]; then
  image="architon:smoke"
  expect_version="${expect_version:-v0.0.0-smoke}"
  echo "==> Building $image (VERSION=$expect_version)"
  docker build --build-arg VERSION="$expect_version" -t "$image" .
fi

tmp="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$tmp"' EXIT

# A bind mount keeps host ownership. On Linux the container uid must be able to
# write .architon/studio, so run as the caller, as documented.
user_args=()
if [[ "$(uname -s)" == "Linux" ]]; then
  user_args=(--user "$(id -u):$(id -g)")
fi

run_image() {
  docker run --rm --network none ${user_args[@]+"${user_args[@]}"} "$@"
}

echo "==> Image config"
cmd_json="$(docker image inspect --format '{{json .Config.Cmd}}' "$image")"
[[ "$cmd_json" == '["rv","export","/project"]' ]] || fail "default command is $cmd_json"
workdir="$(docker image inspect --format '{{.Config.WorkingDir}}' "$image")"
[[ "$workdir" == "/project" ]] || fail "working directory is $workdir"
default_uid="$(docker run --rm --network none "$image" id -u)"
[[ "$default_uid" != "0" ]] || fail "image runs as root by default"
echo "cmd=$cmd_json workdir=$workdir uid=$default_uid"

echo "==> rv version and kicad-cli inside the image (no network)"
version_line="$(run_image "$image" rv version)"
echo "$version_line"
image_version="${version_line#rv version: }"
image_version="${image_version%% *}"
if [[ -n "$expect_version" && "$image_version" != "$expect_version" ]]; then
  fail "rv version in image is $image_version, expected $expect_version"
fi
run_image "$image" kicad-cli version || fail "kicad-cli is not runnable in the image"

echo "==> rv export /project on examples/agent-loop/broken (expect exit 2)"
mkdir -p "$tmp/container" "$tmp/host"
cp -R examples/agent-loop/broken/. "$tmp/container/"
cp -R examples/agent-loop/broken/. "$tmp/host/"
rm -rf "$tmp/container/.architon/studio" "$tmp/host/.architon/studio"

set +e
run_image -v "$tmp/container":/project "$image"
container_status=$?
set -e
[[ "$container_status" -eq 2 ]] || fail "container exited $container_status, expected 2"
for f in report.json graph.json; do
  [[ -s "$tmp/container/.architon/studio/$f" ]] || fail "container did not write .architon/studio/$f"
done

echo "==> rv export /project on an empty directory (expect exit 3, nothing written)"
mkdir -p "$tmp/empty"
set +e
run_image -v "$tmp/empty":/project "$image"
empty_status=$?
set -e
[[ "$empty_status" -eq 3 ]] || fail "empty project exited $empty_status, expected 3"
[[ -z "$(ls -A "$tmp/empty")" ]] || fail "exit 3 wrote files: $(ls -A "$tmp/empty")"

echo "==> Compare with rv export on the host"
if command -v go >/dev/null 2>&1; then
  go build -trimpath \
    -ldflags "-X github.com/badimirzai/architon-cli/internal/version.Version=$image_version" \
    -o "$tmp/rv" ./cmd/rv
  set +e
  "$tmp/rv" export "$tmp/host"
  host_status=$?
  set -e
  [[ "$host_status" -eq "$container_status" ]] || fail "host exited $host_status, container exited $container_status"
  # Reports embed absolute paths and import timestamps. Map the host project
  # root to /project and blank the timestamps before comparing.
  strip_time='s/"(imported|parsed_at)": "[^"]*"/"\1": ""/'
  for f in report.json graph.json; do
    sed -E -e "s|$tmp/host|/project|g" -e "$strip_time" "$tmp/host/.architon/studio/$f" >"$tmp/host-$f"
    sed -E -e "$strip_time" "$tmp/container/.architon/studio/$f" >"$tmp/container-$f"
    diff -u "$tmp/host-$f" "$tmp/container-$f" || fail "$f differs between host and container"
  done
  echo "host and container Studio files match (exit $host_status)"
else
  echo "NOTE: go not found; skipped the host comparison."
fi

echo "OK: $image exported examples/agent-loop/broken with --network none (exit 2, report.json and graph.json written)"
