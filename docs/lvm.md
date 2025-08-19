# LVM Snapshots

LVMSync manages snapshot lifecycles to provide crash-consistent transfers.

## Snapshot Creation

LVMSync automatically creates a temporary snapshot of the source device before
copying blocks. Specify the size with `--snapshot-size` or
`LVMSYNC_SNAPSHOT_SIZE`. When not running as root, LVMSync escalates using
`sudo -n` by default.

```sh
sudo lvmsync run --snapshot-size 5G /dev/vg0/origin /dev/vg1/target
```

The command above creates a 5 GiB snapshot, streams the changed blocks, and
removes the snapshot on completion.

## Snapshot Extension

Snapshot usage is monitored during transfers. When the copy-on-write device
approaches the configured threshold, LVMSync extends the snapshot to avoid
aborting the run.

## Snapshot Discard

After the transfer completes or the process is interrupted, LVMSync discards the
snapshot to free space:

```sh
lvmsync run /dev/vg0/origin /dev/vg1/target
# snapshot removed automatically
```

Use `--skip-snapshot-creation` to supply an existing snapshot and manage cleanup
manually.

Snapshots are registered with a global handler and are removed automatically if
LVMSync receives `SIGINT`, `SIGTERM`, or encounters a panic.

## Mount Checks

Before writing to a destination logical volume, LVMSync verifies that it is not
mounted read-write. The command aborts unless `--force` is used, preventing
concurrent filesystem modifications from corrupting the copy.

## Destination Volume Creation

Enable `--create-dest-lv` (or `LVMSYNC_CREATE_DEST_LV`) to automatically create
the destination logical volume when it does not exist. The new volume matches
the source snapshot size. This operation prompts for confirmation unless
`--allow-overwrite` is provided alongside `--force`:

```sh
lvmsync run --create-dest-lv --force /dev/vg0/origin /dev/vg1/target
```

Accidentally creating a volume in the wrong group can destroy data; verify the
destination path and available space before using this option.
=======
## Destination Creation

`--create-dest-lv` (or `LVMSYNC_LVM_CREATE_DEST_LV=true`) creates the
destination logical volume when it does not exist. The new LV matches the source
size and is ready for data before the transfer begins.

## Environment Example

Configuration can also come from environment variables. This example creates a
snapshot sized at 25 % of the origin and uses `sudo -n` for any required
privileges:

```sh
LVMSYNC_SNAPSHOT_SIZE=25% LVMSYNC_LVM_ESCALATION="sudo -n" \
  lvmsync run /dev/vg0/origin /dev/vg1/target
```

