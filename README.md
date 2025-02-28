# LVMSync

LVMSync is a high-performance incremental data replication tool for LVM snapshots. It efficiently transfers only changed blocks using metadata from snapshot COW (Copy-On-Write) devices.

## Features

- **Incremental Block-Level Synchronization**: Transfers only changed blocks.
- **Zero-Copy Transfers**: Uses `splice()` for efficient data movement.
- **Parallel Execution**: Configurable concurrency for optimal performance.
- **Rate-Limiting**: Control bandwidth usage during transfers.
- **Compression**: Supports LZ4 and Zstd (with configurable compression levels and an auto mode based on CPU features).
- **Checksum Verification**: Ensures data integrity.
- **Remote Execution via SSH**: Replicates data over SSH with pre/post-scripts.
- **Resume Support**: Ability to resume interrupted transfers.
- **LVM Snapshot Management**:
  - Automatic snapshot creation and removal.
  - Configurable snapshot size (absolute or percentage-based).
  - Configurable volume group for constructing the snapshot device path.
  - Configurable privilege escalation (defaulting to `sudo -n`).
  - Snapshot health monitoring that fails fast if usage exceeds a threshold.
- **Graceful Shutdown**: Signal handling ensures snapshots are cleaned up on interruption.
- **Configuration Validation**: Verifies key parameters (e.g., volume group existence, escalation command) before running.

## Installation

### Requirements

- Go 1.18+
- LVM2 (for snapshot support)
- SSH client & server (for remote transfers)

### Build

```sh
git clone https://github.com/oferchen/lvmsync_go.git
cd lvmsync
go build -o lvmsync ./cmd/lvmsync
```

## Usage

### Basic Syntax

```sh
lvmsync [options] <snapshot|lvm device> <destination>
```

### Options

#### General Options

| Option              | Description                                                                               | Default    |
|---------------------|-------------------------------------------------------------------------------------------|------------|
| `--apply`           | Apply mode: read change dump from file (`-` for STDIN) and apply to destination device     | `""`       |
| `--stdout`          | Write change dump to STDOUT                                                               | `false`    |
| `--parallel`        | Number of concurrent workers                                                              | `4`        |
| `--zerocopy`        | Enable zero-copy transfers (only in sequential mode)                                      | `false`    |
| `--max_retries`     | Maximum number of retries per block                                                       | `3`        |
| `--resume`          | Path to resume state file                                                                 | `""`       |
| `--speed`           | Transfer speed limit (e.g., `"100MB"`)                                                      | `"100MB"`  |
| `-v, --verbose`     | Verbosity level (e.g., `-v`, `-vv`, `-vvv`)                                                 | `0`        |
| `--verify_checksum` | Enable checksum verification                                                              | `false`    |

#### SSH Options

| Option                     | Description                                           | Default                      |
|----------------------------|-------------------------------------------------------|------------------------------|
| `--ssh_user`               | SSH username                                          | `"root"`                     |
| `--ssh_key`                | Path to SSH private key or use agent                  | `""`                         |
| `--ssh_port`               | SSH port number                                       | `22`                         |
| `--known_hosts`            | Path to known_hosts file                              | `"/root/.ssh/known_hosts"`   |
| `--stricthostkeychecking`  | Enable SSH StrictHostKeyChecking                      | `true`                       |

#### Remote Options

| Option                  | Description                                              | Default   |
|-------------------------|----------------------------------------------------------|-----------|
| `--lvmsync_path`        | Remote command to run (e.g., `"lvmsync"`)                | `"lvmsync"`|
| `--remote_pre_script`   | Remote script to run before transfer                   | `""`      |
| `--remote_post_script`  | Remote script to run after transfer                    | `""`      |

#### Compression Options

| Option               | Description                                                                                          | Default   |
|----------------------|------------------------------------------------------------------------------------------------------|-----------|
| `--compress`         | Compression type (options: `"none"`, `"lz4"`, `"zstd"`, `"auto"`)                                     | `"lz4"`   |
| `--compress_level`   | Compression level for zstd (ignored for lz4)                                                           | `3`       |

#### LVM Options

| Option                     | Description                                                                                                              | Default      |
|----------------------------|--------------------------------------------------------------------------------------------------------------------------|--------------|
| `--skip_snapshot_creation` | Skip automatic snapshot creation                                                                                         | `false`      |
| `--skip_disk_check`        | Skip disk space check before snapshot creation                                                                           | `false`      |
| `--snapshot_size`          | Snapshot size as an absolute value (e.g., `"20G"`) or as a percentage of the original volume (e.g., `"20%"`)                | `"20%"`      |
| `--volume_group`           | Volume group name of the source LVM volume                                                                               | `"vg0"`      |
| `--lvm_escalation`         | Command used to escalate privileges for LVM commands (e.g., `"sudo -n"`)                                                 | `"sudo -n"`  |

### Examples

#### Local Transfer

```sh
lvmsync /dev/vg0/snap0 /dev/vg0/data
```

#### Remote Transfer

```sh
lvmsync /dev/vg0/snap0 user@remote:/dev/vg0/data
```

#### Applying Changes

```sh
lvmsync --apply dumpfile.lvm /dev/vg0/data
```

#### Using Compression

```sh
lvmsync --compress zstd --compress_level 3 /dev/vg0/snap0 /dev/vg0/data
```

#### Rate Limiting

```sh
lvmsync --speed 50MB /dev/vg0/snap0 /dev/vg0/data
```

#### Resuming a Transfer

```sh
lvmsync --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

#### Full LVM Operation Example

```sh
lvmsync --skip_disk_check=false --snapshot_size "25%" --volume_group "vg_data" --lvm_escalation "sudo -n" /dev/vg_data/original /dev/vg_data/destination
```

In this example, LVMSync will:
- Validate that volume group `vg_data` exists.
- Create a snapshot of `/dev/vg_data/original` sized at 25% of the volume.
- Use the escalation command `sudo -n` if not running as root.
- Monitor snapshot usage (failing fast if usage exceeds 80%).
- Perform the block-level transfer.
- Remove the snapshot upon completion.
- Handle graceful shutdown if interrupted.

## Configuration File (`config.yaml`)

Below is an example configuration file covering all options:

```yaml
# config.yaml - LVMSync default configuration

apply: ""                              # Apply mode: read change dump from file ('-' for STDIN) and apply to destination device
stdout: false                          # Write change dump to STDOUT
parallel: 4                            # Number of concurrent workers
zerocopy: false                        # Enable zero-copy transfers (only in sequential mode)
max_retries: 3                         # Maximum number of retries per block
resume: ""                             # Path to resume state file

ssh_user: "root"                       # SSH username
ssh_key: ""                            # Path to SSH private key or use SSH agent
ssh_port: 22                           # SSH port
known_hosts: "/root/.ssh/known_hosts"  # Path to known_hosts file
strict_host_key_checking: true         # Enable SSH StrictHostKeyChecking

lvmsync_path: "lvmsync"                  # Remote command to run

remote_pre_script: ""                  # Remote script to run before starting transfer
remote_post_script: ""                 # Remote script to run after finishing transfer

compress: "lz4"                        # Compression method (options: "none", "lz4", "zstd", "auto")
compress_level: 3                      # Compression level for zstd (ignored for lz4)
speed: "100MB"                         # Speed limit (e.g., "100MB")
verify_checksum: false                 # Enable checksum verification for data integrity
verbose: 0                             # Verbosity level

# LVM Options:
skip_snapshot_creation: false          # Skip automatic snapshot creation
skip_disk_check: false                 # Skip disk space check before snapshot creation
snapshot_size: "20%"                   # Snapshot size as an absolute value (e.g., "20G") or as a percentage (e.g., "20%")
volume_group: "vg0"                    # Volume group name of the source LVM volume
lvm_escalation: "sudo -n"               # Command used to escalate privileges for LVM commands
```

## Graceful Shutdown

LVMSync installs signal handlers for SIGINT and SIGTERM. If an interruption occurs, the tool will attempt to remove any created snapshot before exiting, ensuring no orphaned snapshots remain.

## Configuration Validation

Before starting, LVMSync validates key configuration parameters:
- It checks that the specified volume group exists (via `vgdisplay`).
- It verifies that the escalation command is available if not running as root.

Invalid configurations will cause the tool to abort with a clear error message.

## File Layout Overview

- **config/**:  
  - `config.go`: Manages configuration, flag grouping, defaults, and validation.

- **lvm/**:  
  - `lvm.go`: Handles LVM snapshot creation, removal, monitoring, disk space and volume size checks, and privilege escalation.

- **remote/**:  
  - `remote.go`: Provides SSH client utilities for remote replication, command validation, and script execution.

- **transfer/**:  
  - Contains modules for block-level data transfer, compression/decompression, rate limiting, and resume support.

- **main.go**:  
  - The entry point that ties configuration, LVM operations, remote execution, and data transfer together. It also implements graceful shutdown.

## Credits

Go version written by Ofer Chen, inspired by [mpalmer/lvmsync](https://github.com/mpalmer/lvmsync).

## Contributing

Contributions are welcome. Please follow the project's coding guidelines and include appropriate logging, error handling.

## License

GPLv3 License. See the `LICENSE` file for details.
