#!/usr/bin/env bash
# Deliberately fail I/O to one allocated data block on a disposable ext4 image.
set -euo pipefail

mount_image() {
  sudo mount -t ext4 -o data=ordered,data_err=ignore,nodev,nosuid,noexec \
    "/dev/mapper/$APPRIVET_WB_NAME" "$APPRIVET_WB_MOUNT"
}

set_table() {
  sudo dmsetup load "$APPRIVET_WB_NAME" --table "$1"
  sudo dmsetup suspend --noflush --nolockfs "$APPRIVET_WB_NAME"
  sudo dmsetup resume "$APPRIVET_WB_NAME"
}

if [[ ${1:-} == control ]]; then
  # Keep control calls scoped to the exact loop image and mapping created below.
  [[ "$APPRIVET_WB_NAME" =~ ^apprivet-wb-[a-zA-Z0-9]+$ ]]
  [[ "$APPRIVET_WB_LOOP" =~ ^/dev/loop[0-9]+$ ]]
  [[ $(sudo losetup -j "$APPRIVET_WB_IMAGE" -n -O NAME) == "$APPRIVET_WB_LOOP" ]]
  [[ $(sudo blockdev --getsz "$APPRIVET_WB_LOOP") == 65536 ]]
  source_device=$(findmnt -n -o SOURCE --mountpoint "$APPRIVET_WB_MOUNT")
  [[ $(readlink -f "$source_device") == "$(readlink -f "/dev/mapper/$APPRIVET_WB_NAME")" ]]
  case "$2" in
    fault)
      # The offset is in 4 KiB filesystem blocks. Device-mapper uses 512 B sectors.
      [[ "$3" =~ ^[1-9][0-9]{0,3}$ ]]
      start=$((8 * $3))
      end=$((start + 8))
      (( end < 65536 ))
      table=$(printf '%s\n' "0 $start linear $APPRIVET_WB_LOOP 0" \
        "$start 8 error" "$end $((65536 - end)) linear $APPRIVET_WB_LOOP $end")
      set_table "$table"
      ;;
    restore) set_table "0 65536 linear $APPRIVET_WB_LOOP 0" ;;
    remount)
      sudo umount "$APPRIVET_WB_MOUNT"
      mount_image
      ;;
    *) exit 2 ;;
  esac
  exit
fi

uid=$(id -u)
if [[ "$uid" == 0 ]]; then
  echo 'Run as a non-root Linux user with sudo for device setup.' >&2
  exit 1
fi

scratch=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/apprivet-writeback.XXXXXX")
export APPRIVET_WB_NAME="apprivet-wb-${scratch##*.}"
export APPRIVET_WB_IMAGE="$scratch/journal.ext4"
export APPRIVET_WB_MOUNT="$scratch/mount"
export APPRIVET_WB_SCRIPT
APPRIVET_WB_SCRIPT=$(realpath "$0")

loop_created=false
mapping_created=false
cleanup() {
  if "$mapping_created"; then
    # Resume a mapping if a control command was interrupted while switching it.
    sudo dmsetup resume "$APPRIVET_WB_NAME" >/dev/null 2>&1 || true
    if mountpoint -q "$APPRIVET_WB_MOUNT"; then
      sudo umount "$APPRIVET_WB_MOUNT" || return 1
    fi
    sudo dmsetup remove "$APPRIVET_WB_NAME" || return 1
  fi
  if "$loop_created"; then
    sudo losetup -d "$APPRIVET_WB_LOOP" || return 1
  fi
  rm -f -- "$APPRIVET_WB_IMAGE"
  rmdir -- "$APPRIVET_WB_MOUNT" "$scratch"
}
trap cleanup EXIT

mkdir "$APPRIVET_WB_MOUNT"
fallocate -l 32M "$APPRIVET_WB_IMAGE"
mkfs.ext4 -q -F -b 4096 -m 0 "$APPRIVET_WB_IMAGE"
export APPRIVET_WB_LOOP
APPRIVET_WB_LOOP=$(sudo losetup --find --show "$APPRIVET_WB_IMAGE")
loop_created=true
sudo dmsetup create "$APPRIVET_WB_NAME" --table "0 65536 linear $APPRIVET_WB_LOOP 0"
mapping_created=true
mount_image
sudo install -d -m 0700 -o "$uid" -g "$(id -g)" "$APPRIVET_WB_MOUNT/test"
APPRIVET_WB_TEST_DIR="$APPRIVET_WB_MOUNT/test" \
  go test ./internal/admission -run '^TestLinuxJournalWriteback$' -count=1 -v -timeout=90s
