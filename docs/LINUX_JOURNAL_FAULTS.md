# Linux quota and sync tests

The Linux workflow runs the journal against a disposable 32 MiB loopback ext4 image with delayed allocation. The journal runs as the non-root test user. Privileged commands only mount the image and configure its quota. The image and mount point are created for the test and cleaned up afterward.

On a Linux host with Bash, Go, e2fsprogs, util-linux, quota tools and permission to mount loopback images, run from the repository root as a non-root user with `sudo` access.

```sh
bash .github/scripts/linux-journal-faults.sh
```

The first test sets a 256 KiB user hard quota in the image, unmounts and remounts it, and verifies enforcement without setting the limit again. It fills the user's quota after the append intent is synced. The actual journal write must return `EDQUOT` with free blocks still available on the volume. Repair must remain pending while the quota is exhausted. Removing the test's filler file permits exact completion using the same inspection hashes.

The remaining tests remove the quota for the test user. They write the complete event, allocate extents until the kernel returns `ENOSPC` even for a single block, and then call the real journal `fsync`. The test logs the native result. A successful sync must finalize the append. If native sync returns `ENOSPC`, the intent must remain available and exact repair must succeed after freeing space. A separate case injects `ENOSPC` at the application sync boundary after attempting the native sync. With a complete durable event, repair can finalize it while the volume is still full without writing event bytes again.

Each path checks the journal head and sequence, the original worker, claim and fence tuple, rejection of a stale owner, and rejection of the original request and nonce. Retrying the ownership recovery must leave the journal bytes unchanged. These checks exercise the local accounting boundary described in [append repair](APPEND_REPAIR.md).

The test distinguishes a kernel allocation failure, the native result of `fsync`, and a deliberately injected sync error. Delayed allocation can reserve space when a write is accepted, so a full filesystem can still flush those previously accepted bytes. A clean unmount used to verify the quota configuration does not simulate a crash. [Linux ext4 allocation documentation](https://docs.kernel.org/admin-guide/ext4.html)

Still open are native writeback errors at `fsync` on a filesystem and device configuration that reproduces them, group and project quotas, additional Linux filesystems and mount options, real storage controller failures and power cuts, and Windows directory metadata and ACL validation. This harness also leaves external rollback or clone detection, authenticated execution and verified worker teardown, isolated Mac execution, physical devices, and Apple signing outside its coverage.
