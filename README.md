##**ATTENTION: This is actively WIP Do not use in production systems, please be patient.**

# LVMSync

LVMSync is a high-performance incremental data replication tool for LVM snapshots. It efficiently transfers only changed blocks using metadata from snapshot COW (Copy-On-Write) devices.

## Features

- **Incremental Block-Level Synchronization**: Transfers only changed blocks.
- **Zero-Copy Transfers**: Utilizes `splice()` for efficient data movement.
- **Parallel Execution**: Configurable concurrency for optimal performance.
- **Rate-Limiting**: Control bandwidth usage during transfers.
- **Compression**: Supports LZ4 and Zstd (with configurable compression levels and an auto mode based on CPU features).
- **Checksum Verification**: Ensures data integrity using SHA-256.
 - **Deduplication Strategies**: Detect unchanged blocks using checksum, rolling hash, or Bloom filter with persistent state.
 - **Remote Execution via SSH**: Replicates data over SSH with support for pre/post-scripts.
- **Resume Support**: Ability to resume interrupted transfers.
- **LVM Snapshot Management**:
  - Automatic snapshot creation and removal.
  - Configurable snapshot size (absolute or percentage-based).
  - Configurable volume group for constructing the snapshot device path.
  - Configurable privilege escalation (defaulting to `sudo -n`).
  - Snapshot health monitoring that fails fast if usage exceeds a threshold.
- **Graceful Shutdown**: Signal handling ensures snapshots are cleaned up on interruption.
- **Configuration Validation**: Checks key parameters (e.g., volume group existence, escalation command) before starting operations.

## Architecture

LVMSync is organized into modular packages to keep concerns separated:

- `lvm` – manages snapshot creation, monitoring, and cleanup.
- `transfer` – performs block-level synchronization, compression, deduplication, and resume logic.
- `remote` – wraps SSH functionality for running commands on remote hosts and coordinating transfers.
- `config` – parses and validates configuration files and CLI options.
- `common` and `internal` – shared helpers and internal utilities such as multi-error handling.

This structure allows individual packages to be developed and tested in isolation.

## Installation

### Requirements

- Go 1.18+
- LVM2 (for snapshot support)
- SSH client & server (for remote transfers)

### Build

Clone the repository and build the binary using Go modules:

```sh
git clone https://github.com/oferchen/lvmsync_go.git
cd lvmsync_go
go mod tidy
go build -o lvmsync .
```

## Usage

### Basic Syntax

```sh
lvmsync [options] <snapshot|lvm device> <destination>
```

The tool supports both local and remote transfers, as well as an "apply mode" for applying change dumps.

### Options

#### General Options

| Option        | Description                                                                                          | Default   |
|---------------|------------------------------------------------------------------------------------------------------|-----------|
| `--config`    | Path to a YAML configuration file                                                                  | `""`      |
| `--apply`     | Apply mode: read change dump from file (`-` for STDIN) and apply to destination device               | `""`      |
| `--stdout`    | Write change dump to STDOUT                                                                          | `false`   |
| `--parallel`  | Number of concurrent workers                                                                         | `4`       |
| `--zerocopy`  | Enable zero-copy transfers (only used in sequential mode)                                           | `false`   |
| `--max_retries` | Maximum number of retries per block                                                                 | `3`       |
| `--resume`    | Path to resume state file                                                                            | `""`      |
| `--speed`     | Transfer speed limit (e.g., `"100MB"`)                                                               | `"100MB"` |
| `-v, --verbose` | Verbosity level (e.g., `-v`, `-vv`, `-vvv`)                                                         | `0`       |
| `--verify_checksum` | Enable checksum verification for data integrity                                               | `false`   |
| `--progress`  | Show progress percentage during the transfer                                                       | `true`    |
| `--block_size` | Block size for data transfer (e.g., "4K", "64K", "512K", "1M")                                     | `"4K"`    |

#### SSH Options

| Option                   | Description                                                      | Default                                |
|--------------------------|------------------------------------------------------------------|----------------------------------------|
| `--ssh_user`             | SSH username                                                     | `"root"`                               |
| `--ssh_key`              | Path to SSH private key or use the SSH agent                     | `""`                                   |
| `--ssh_port`             | SSH port number                                                  | `22`                                   |
| `--known_hosts`          | Path to known_hosts file (defaults to `$HOME/.ssh/known_hosts`)    | `$HOME/.ssh/known_hosts`               |
| `--stricthostkeychecking`| Enable SSH StrictHostKeyChecking                                 | `true`                                 |

#### Remote Options

| Option                | Description                                             | Default   |
|-----------------------|---------------------------------------------------------|-----------|
| `--lvmsync_path`      | Remote command to run (e.g., `"lvmsync"`)               | `"lvmsync"`|
| `--remote_pre_script` | Remote script to run before starting the transfer       | `""`      |
| `--remote_post_script`| Remote script to run after finishing the transfer        | `""`      |

#### Deduplication Options

| Option               | Description                                                                             | Default          |
|----------------------|-----------------------------------------------------------------------------------------|------------------|
| `--deduplication`    | Enable deduplication to avoid re-transferring unchanged blocks                          | `false`          |
| `--dedup_strategy`   | Deduplication strategy (`"auto"`, `"checksum"`, `"rolling_hash"`, or `"bloom"`)        | `"auto"`        |
| `--dedup_state_file` | Path to deduplication state file                                                        | `~/.lvmsync_dedup` |

By default, `--dedup_strategy auto` inspects CPU capabilities via
`golang.org/x/sys/cpu`. If hardware SHA support is detected, the checksum
strategy is selected; otherwise a rolling hash is used. Experts can override
this behaviour with `--dedup_strategy checksum`, `--dedup_strategy rolling_hash`,
or `--dedup_strategy bloom`.

#### Compression Options

| Option            | Description                                                                  | Default   |
|-------------------|------------------------------------------------------------------------------|-----------|
| `--compress`      | Compression type (options: `"none"`, `"lz4"`, `"zstd"`, `"auto"`)            | `"lz4"`   |
| `--compress_level`| Compression level for Zstd (ignored for LZ4)                                 | `3`       |

#### LVM Options

| Option                     | Description                                                            | Default      |
|----------------------------|------------------------------------------------------------------------|--------------|
| `--skip_snapshot_creation` | Skip automatic snapshot creation                                       | `false`      |
| `--skip_disk_check`        | Skip disk space check before snapshot creation                        | `false`      |
| `--snapshot_size`          | Snapshot size as an absolute value (e.g., `"20G"`) or as a percentage (e.g., `"20%"`) | `"20%"`      |
| `--volume_group`           | Source volume group. If empty, the group with the most free space is selected automatically. | `"vg0"`      |
| `--target_volume_group`    | Volume group name of the target LVM volume                             | `""`       |
| `--source_vgs`             | Candidate source volume groups for auto-selection                      | `[]`       |
| `--target_vgs`             | Candidate target volume groups for auto-selection                      | `[]`       |
| `--lvm_escalation`         | Command used to escalate privileges for LVM commands (e.g., `"sudo -n"`) | `"sudo -n"`  |

### Examples

#### Local Transfer

Transfer changes from a snapshot to a destination device locally:

```sh
lvmsync /dev/vg0/snap0 /dev/vg0/data
```

#### Remote Transfer

Replicate data to a remote host (the destination is specified as `<user@host>:<device>`):

```sh
lvmsync /dev/vg0/snap0 user@remote:/dev/vg0/data
```

#### Applying Changes

Apply a change dump to a destination device (read from a file or STDIN):

```sh
lvmsync --apply dumpfile.lvm /dev/vg0/data
```

#### Using Compression

Enable Zstd compression with a specified compression level:

```sh
lvmsync --compress zstd --compress_level 3 /dev/vg0/snap0 /dev/vg0/data
```

#### Rate Limiting

Limit the transfer speed to 50MB/s:

```sh
lvmsync --speed 50MB /dev/vg0/snap0 /dev/vg0/data
```

#### Resuming a Transfer

Resume an interrupted transfer using a resume state file:

```sh
lvmsync --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

#### Full LVM Operation Example

```sh
lvmsync --skip_disk_check=false --snapshot_size "25%" --volume_group "vg_data" --lvm_escalation "sudo -n" /dev/vg_data/original /dev/vg_data/destination
```

In this example, LVMSync will:
- Validate that the volume group `vg_data` exists.
- Create a snapshot of `/dev/vg_data/original` sized at 25% of the original volume.
- Use the escalation command `sudo -n` if not running as root.
- Monitor snapshot usage (failing fast if usage exceeds 80%).
- Perform the block-level transfer.
- Remove the snapshot upon completion.
- Clean up gracefully if interrupted.

## Configuration File (`config.yaml`)

You can supply configuration via a YAML file. Below is an example configuration file covering all options:

```yaml
# config.yaml - LVMSync default configuration

config: ""                              # Optional: specify an alternative configuration file path

apply: ""                               # Apply mode: read change dump from file ('-' for STDIN) and apply to destination device
stdout: false                           # Write change dump to STDOUT
parallel: 4                             # Number of concurrent workers
zerocopy: false                         # Enable zero-copy transfers (only in sequential mode)
max_retries: 3                          # Maximum number of retries per block
resume: ""                              # Path to resume state file
speed: "100MB"                          # Transfer speed limit (e.g., "100MB")
verbose: 0                              # Verbosity level
verify_checksum: false                  # Enable checksum verification
progress: true                          # Show progress percentage during the transfer

# Deduplication Options:
deduplication: false                    # Enable deduplication to avoid re-transferring unchanged blocks
dedup_strategy: "auto"                   # Strategy: "auto", "checksum", "rolling_hash", or "bloom"
dedup_state_file: "~/.lvmsync_dedup"    # Path to deduplication state file

# SSH Options:
ssh_user: "root"                        # SSH username
ssh_key: ""                             # Path to SSH private key or use SSH agent
ssh_port: 22                            # SSH port
known_hosts: "~/.ssh/known_hosts"       # Path to known_hosts file
stricthostkeychecking: true             # Enable SSH StrictHostKeyChecking

# Remote Options:
lvmsync_path: "lvmsync"                 # Remote command to run
remote_pre_script: ""                   # Remote script to run before starting transfer
remote_post_script: ""                  # Remote script to run after finishing transfer

# Compression Options:
compress: "lz4"                         # Compression type (options: "none", "lz4", "zstd", "auto")
compress_level: 3                       # Compression level for zstd (ignored for lz4)

# LVM Options:
skip_snapshot_creation: false           # Skip automatic snapshot creation
skip_disk_check: false                  # Skip disk space check before snapshot creation
snapshot_size: "20%"                    # Snapshot size as an absolute value (e.g., "20G") or as a percentage (e.g., "20%")
volume_group: "vg0"                     # Source volume group. Auto-selected by free space when empty
target_volume_group: ""                 # Volume group name of the target LVM volume
source_vgs: []                          # Candidate source VGs for auto-selection
target_vgs: []                          # Candidate target VGs for auto-selection
lvm_escalation: "sudo -n"               # Command used to escalate privileges for LVM commands
```

LVMSync installs signal handlers for SIGINT and SIGTERM. If an interruption occurs, the tool will attempt to remove any created snapshot before exiting, ensuring no orphaned snapshots remain.

## Configuration Validation

Before starting, LVMSync validates key configuration parameters:
- Verifies that the specified volume groups exist.
- Ensures the escalation command is available if not running as root.

Invalid configurations will cause the tool to abort with a clear error message.

## Credits

LVMSync is written in Go by Ofer Chen, inspired by [mpalmer/lvmsync](https://github.com/mpalmer/lvmsync).

## Contributing

Contributions are welcome. Please follow the project's coding guidelines, include appropriate logging and error handling, and update documentation as needed.


## Development

To run the tests and static checks locally:

```sh
go fmt ./...
go vet ./...
go test ./...
```

The same steps run in GitHub Actions under `.github/workflows/go.yml`.

## License

GPLv3 License. See the `LICENSE` file for details.
