#!/bin/sh
#
# One-line installer, run on the GX device:
#
#   wget -qO- https://raw.githubusercontent.com/bolchisb/ruuvi-victron-environmental-control/main/scripts/install.sh | sh
#
# Installs the latest release by default. To install a specific release, set TAG
# on the shell that runs the script:
#
#   wget -qO- .../install.sh | TAG=v.0.1.0-dev1 sh
#
# Checks prerequisites, installs SetupHelper if missing, downloads the package
# for the device architecture, and registers the service. POSIX sh for busybox.

set -xe

repo="https://github.com/bolchisb/ruuvi-victron-environmental-control"
pkg_name="ruuvi-victron-control"
install_dir="/data/${pkg_name}"
ui_port="${UI_PORT:-8088}"

fail() {
  echo "install: $1" >&2
  exit 1
}

[ "$(id -u)" = "0" ] || fail "must run as root"
[ -d /opt/victronenergy ] || fail "not a Venus OS GX device (/opt/victronenergy missing)"
[ -d /data ] || fail "/data not found"
command -v svscan >/dev/null 2>&1 || fail "daemontools (svscan) not found"

case "$(uname -m)" in
  armv7l)        arch=armv7 ;;
  aarch64|arm64) arch=arm64 ;;
  *)             fail "unsupported architecture: $(uname -m)" ;;
esac
echo "Architecture: $(uname -m) -> ${arch}"

if [ ! -d /data/SetupHelper ]; then
  echo "Installing SetupHelper (prerequisite)..."
  wget -qO - https://github.com/kwindrem/SetupHelper/archive/latest.tar.gz | tar -xzf - -C /data
  rm -rf /data/SetupHelper
  mv /data/SetupHelper-latest /data/SetupHelper
  /data/SetupHelper/setup install auto deferReboot deferGuiRestart
fi

if [ -n "$TAG" ]; then
  url="${repo}/releases/download/${TAG}/${pkg_name}-${arch}.tgz"
else
  url="${repo}/releases/latest/download/${pkg_name}-${arch}.tgz"
fi
echo "Downloading ${pkg_name} (${arch}) from release ${TAG:-latest}..."
tgz="/tmp/${pkg_name}-${arch}.tgz"
rm -f "$tgz"
wget -qO "$tgz" "$url" || fail "release asset not found (check the TAG): ${url}"
[ -s "$tgz" ] || fail "downloaded an empty file (release or asset missing): ${url}"
gzip -t "$tgz" 2>/dev/null || fail "downloaded file is not a valid archive (got an error page?): ${url}"
tar -xzf "$tgz" -C /data || fail "extract failed: ${url}"
rm -f "$tgz"

"${install_dir}/setup" install auto deferReboot deferGuiRestart

ip="$(ip -4 addr show 2>/dev/null | awk '/inet / && $2 !~ /^127/ {sub(/\/.*/, "", $2); print $2; exit}')"
[ -n "$ip" ] || ip="<device-ip>"

echo ""
echo "Installed. Open the UI at http://${ip}:${ui_port}"
