#!/usr/bin/env bash
#
# Builds the controller for each supported GX architecture in Docker, assembles
# one install package per architecture, and optionally publishes them as GitHub
# release assets.
#
# Usage:
#   scripts/build.sh            # build + package both Cerbo architectures
#   scripts/build.sh --publish  # also create/upload the GitHub release
#   scripts/build.sh mac        # build a local binary to test on this Mac
#
# Version comes from the VERSION env var (the release tag in CI) or the version
# file. The binary is never compiled on the host; the Docker builder owns that.

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"

version="${VERSION:-$(tr -d ' \n' < version)}"
if [ -z "$version" ]; then
  echo "version is empty (set VERSION or the version file)" >&2
  exit 1
fi

dist="dist"
pkg_name="ruuvi-victron-control"

# Local test build: cross-compiles a macOS binary in Docker that runs natively
# on this machine. D-Bus is absent off-device, so the UI shows that state but
# everything else works.
if [ "${1:-}" = "mac" ]; then
  case "$(uname -m)" in
    arm64|aarch64) host_arch=arm64 ;;
    x86_64|amd64)  host_arch=amd64 ;;
    *) echo "unsupported host architecture: $(uname -m)" >&2; exit 1 ;;
  esac
  echo "Building local test binary (darwin/${host_arch})"
  docker build \
    --build-arg "VERSION=${version}" \
    --build-arg "GOOS=darwin" \
    --build-arg "GOARCH=${host_arch}" \
    --build-arg "GOARM=" \
    --target artifact \
    --output "type=local,dest=${dist}/mac" \
    .
  chmod +x "${dist}/mac/ruuvi-control"
  echo "Run it with: ./${dist}/mac/ruuvi-control   (UI on http://localhost:8088)"
  exit 0
fi

rm -rf "$dist"
built_tarballs=""

# build_one <label> <GOARCH> <GOARM>
build_one() {
  local label="$1" goarch="$2" goarm="$3"
  local out="${dist}/bin-${label}"
  local pkg_dir="${dist}/pkg-${label}/${pkg_name}"
  local tarball="${dist}/${pkg_name}-${label}.tgz"

  echo "Building ${pkg_name} ${version} (${label})"

  docker build \
    --build-arg "VERSION=${version}" \
    --build-arg "GOARCH=${goarch}" \
    --build-arg "GOARM=${goarm}" \
    --target artifact \
    --output "type=local,dest=${out}" \
    .

  mkdir -p "$pkg_dir"
  mv "${out}/ruuvi-control" "${pkg_dir}/ruuvi-control"
  chmod +x "${pkg_dir}/ruuvi-control"

  cp version setup gitHubInfo "${pkg_dir}/"
  cp -r services "${pkg_dir}/services"
  chmod +x "${pkg_dir}/setup"
  find "${pkg_dir}/services" -name run -exec chmod +x {} +

  tar -czf "$tarball" -C "${dist}/pkg-${label}" "$pkg_name"
  echo "Package: ${tarball}"
  built_tarballs="${built_tarballs} ${tarball}"
}

build_one armv7 arm 7
build_one arm64 arm64 ""

if [ "${1:-}" = "--publish" ]; then
  echo "Publishing release ${version}"
  if gh release view "$version" >/dev/null 2>&1; then
    gh release upload "$version" $built_tarballs --clobber
  else
    gh release create "$version" $built_tarballs \
      --title "ruuvi-victron-control ${version}" \
      --notes "ruuvi-victron-control ${version}"
  fi
  echo "Published ${version}"
fi
