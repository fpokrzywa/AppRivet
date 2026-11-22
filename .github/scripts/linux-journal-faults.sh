#!/usr/bin/env bash
# Exercise ext4 quota and sync on a disposable image. The Go tests run as the
# caller so the kernel enforces the caller's quota and reserved-space limits.
set -euo pipefail

uid=$(id -u)
if [[ "$uid" == 0 ]]; then
  echo 'Run as a non-root Linux user with sudo for mount and quota setup.' >&2
  exit 1
fi

scratch=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/apprivet-ext4.XXXXXX")
image="$scratch/journal.ext4"
mount_dir="$scratch/mount"
cleanup() {
  if mountpoint -q "$mount_dir"; then
    sudo umount "$mount_dir" || return 1
  fi
  rm -f -- "$image"
  rmdir -- "$mount_dir" "$scratch"
}
trap cleanup EXIT

mkdir "$mount_dir"
fallocate -l 32M "$image"
mkfs.ext4 -q -F -b 4096 -m 0 -O quota -E quotatype=usrquota "$image"
mount_image() {
  sudo mount -t ext4 -o loop,usrquota,delalloc,nodev,nosuid,noexec "$image" "$mount_dir"
}

mount_image
sudo setquota -u "$uid" 0 256 0 0 "$mount_dir"
# Verify that quota enforcement comes back from the filesystem's on-disk quota
# state. Do not set the limit again before exercising it.
sudo umount "$mount_dir"
mount_image
sudo install -d -m 0700 -o "$uid" -g "$(id -g)" "$mount_dir/quota-test"
APPRIVET_EXT4_TEST_DIR="$mount_dir/quota-test" APPRIVET_EXT4_TEST_MODE=quota \
  go test ./internal/admission -run '^TestLinuxExt4JournalFaults$' -count=1 -v

sudo setquota -u "$uid" 0 0 0 0 "$mount_dir"
sudo install -d -m 0700 -o "$uid" -g "$(id -g)" "$mount_dir/full-test"
APPRIVET_EXT4_TEST_DIR="$mount_dir/full-test" APPRIVET_EXT4_TEST_MODE=full \
  go test ./internal/admission -run '^TestLinuxExt4JournalFaults$' -count=1 -v
