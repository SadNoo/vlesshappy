#!/bin/sh
set -eu

mode="${1:---check}"
case "$mode" in
  --check|--apply) ;;
  *) echo "usage: $0 [--check|--apply]" >&2; exit 2 ;;
esac

if [ "$mode" = "--apply" ]; then
  echo "SSPanel write integration is deferred until the project owner supplies the target panel files." >&2
  exit 2
fi

overlay_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
panel_root=$(CDPATH= cd -- "$overlay_dir/../.." && pwd)

for patch in \
  "$overlay_dir/patches/sspanel-vless-reality.patch" \
  "$overlay_dir/patches/sspanel-vless-reality-admin.patch"
do
  git -C "$panel_root" apply --check "$patch"
done

echo "SSPanel overlay check passed; no files changed. Actual integration is deferred."
