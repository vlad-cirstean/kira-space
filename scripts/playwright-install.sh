#!/bin/sh
# playwright-install.sh <browser>... — Playwright install with system libraries, safe to retry (P372).
# A timed-out attempt can leave its apt/dpkg child running under sudo, outside timeout's process
# group. Wait for it to exit, then repair dpkg; apt reuses the .deb files it already downloaded.
set -eu

# A conf file, not -o flags: Playwright's own apt-get calls read it too.
printf '%s\n' 'DPkg::Lock::Timeout "300";' 'Acquire::Retries "3";' 'Acquire::http::Timeout "30";' 'Acquire::https::Timeout "30";' |
  sudo tee /etc/apt/apt.conf.d/81-playwright-install >/dev/null

# Never kill dpkg mid-unpack: that leaves it interrupted. Let the leftover finish.
waited=0
while pgrep -x 'apt|apt-get|dpkg|dpkg-preconfigure' >/dev/null 2>&1 && [ "$waited" -lt 300 ]; do
  sleep 5
  waited=$((waited + 5))
done

sudo dpkg --configure -a
bunx playwright install --with-deps "$@"
