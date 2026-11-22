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

On the Ubuntu 24.04 runner, the journal write returned `EDQUOT` with 6,414 filesystem blocks available. With the quota removed, a new single-block allocation returned `ENOSPC` at zero available blocks and native journal `fsync` returned success. The case with injected `ENOSPC` left the append pending. Repair finalized the existing event while the image was still full and wrote zero additional event bytes. [Hosted Linux job](https://github.com/fpokrzywa/AppRivet/actions/runs/45074823532/job/137369610433)

## Native writeback errors

Run the writeback harness on Linux with the same tools plus `dmsetup` and kernel device-mapper support. It creates a separate disposable ext4 image and points one device-mapper range at the kernel `error` target. It uses `filefrag` to select a single initialized journal data block while leaving filesystem metadata and sealed history on the normal mapping. The journal runs without root privileges. [Linux device-mapper linear target](https://docs.kernel.org/admin-guide/device-mapper/linear.html)

```sh
bash .github/scripts/linux-journal-writeback.sh
```

The append test requires a successful buffered write followed by native `fsync` `EIO`. It checks the exact intent, restores the device and cleanly unmounts and remounts the image to read the persisted bytes. If the tail matches the intended event prefix, repair must complete it. If the persisted tail diverges, the store must refuse repair and leave the journal and intent intact. Reserve and ownership checks stay blocked until the state can be verified.

The repair test starts with a fully synced event and a deliberately withheld acknowledgement. It redirties the same event bytes to trigger native writeback failure during repair and requires the real `fsync` to return `EIO`. The intent must remain pending. After restoration and a clean remount, the test finalizes the exact event and remounts again to verify replay rejection, the original worker, claim and fence, and rejection of stale ownership. The device mapper injects the fault below the filesystem. The test does not substitute an errno for the two native sync results. [Linux fsync error semantics](https://man7.org/linux/man-pages/man2/fsync.2.html)

After a device reports writeback failure, restore and independently revalidate the storage before repair. A successful read may be served from cache, and a later file descriptor can miss an earlier asynchronous writeback error. The harness uses clean remounts to verify what survived. Do not alter a divergent tail or delete the intent to force progress.

Still open are failures involving filesystem metadata or journal blocks, intermittent or silent device faults, group and project quotas, additional Linux filesystems and mount options, real storage controller failures and power cuts, and Windows directory metadata and ACL validation. External rollback or clone detection, authenticated execution and verified worker teardown, isolated Mac execution, physical devices, and Apple signing remain outside this coverage.
