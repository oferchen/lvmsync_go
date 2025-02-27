# LVMSync

LVMSync is a high-performance incremental data replication tool for LVM snapshots. It efficiently transfers changed blocks using metadata from snapshot COW (Copy-On-Write) devices.

## Features

- **Incremental Block-Level Synchronization**: Transfers only changed blocks.
- **Zero-Copy Transfers**: Uses `splice()` for efficient data movement.
- **Parallel Execution**: Configurable concurrency for optimal performance.
- **Rate-Limiting**: Control bandwidth usage.
- **Compression**: Supports LZ4 for fast, low-overhead compression.
- **Checksum Verification**: Ensures data integrity.
- **Remote Execution via SSH**: Works over SSH for remote replication.
- **Pre/Post-Scripts**: Run scripts before and after transfers.
- **Resume Support**: Ability to resume interrupted transfers.

## Installation

### Requirements

- Go 1.18+
- LVM2 (for snapshot support)
- SSH client & server (for remote transfers)
- `lz4` (if using compression)

### Build

```sh
git clone https://github.com/oferchen/lvmsync_go.git
cd lvmsync
go build -o lvmsync ./cmd/lvmsync
```

## Usage

### Basic Syntax

```sh
lvmsync [options] <snapshot device> <destination>
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--apply` | Apply changes from a dump file (`-` for stdin) | "" |
| `--compress` | Compression method (e.g., `lz4`) | `"lz4"` |
| `--config` | Path to YAML config file | None |
| `--known_hosts` | Path to known_hosts file | `"/root/.ssh/known_hosts"` |
| `--lvmsync_path` | Remote command path | `"lvmsync"` |
| `--max_retries` | Max retries per block | `3` |
| `--parallel` | Number of parallel workers | `4` |
| `--remote_post_script` | Script to run after transfer | `""` |
| `--remote_pre_script` | Script to run before transfer | `""` |
| `--resume` | Resume state file path | `""` |
| `--speed` | Transfer speed limit (e.g., `"100MB"`) | `"100MB"` |
| `--ssh_key` | Path to SSH key (or use agent) | `""` |
| `--ssh_port` | SSH port number | `22` |
| `--ssh_user` | SSH username | `"root"` |
| `--stdout` | Output to STDOUT | `false` |
| `--stricthostkeychecking` | Enable SSH host key verification | `true` |
| `-v, --verbose` | Verbosity level (`-v`, `-vv`, `-vvv`) | `0` |
| `--verify_checksum` | Enable checksum verification | `false` |
| `--zerocopy` | Enable zero-copy transfers | `false` |

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
lvmsync --compress lz4 /dev/vg0/snap0 /dev/vg0/data
```

#### Rate Limiting

```sh
lvmsync --speed 50MB /dev/vg0/snap0 /dev/vg0/data
```

#### Resuming a Transfer

```sh
lvmsync --resume statefile /dev/vg0/snap0 /dev/vg0/data
```

## Configuration File (`config.yaml`)

Example:

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
ssh_verify: true                       # Enable SSH StrictHostKeyChecking

lvmsync_path: "lvmsync"                # Remote command to run (e.g. "lvmsync", "sudo lvmsync", etc.)

remote_pre_script: ""                  # Remote script to run before starting transfer
remote_post_script: ""                 # Remote script to run after finishing transfer

compress: "lz4"                        # Compression method (e.g. "lz4")
speed: "100MB"                         # Speed limit (e.g. "100MB")
verify_checksum: false                 # Enable checksum verification for data integrity

verbose: 0                             # Verbosity level (0 = silent, higher numbers = more verbose)
```

## Credits
Go version written by Ofer Chen inspired by https://github.com/mpalmer/lvmsync

## Contributing
Contributions are welcome.

## License

GPLv3 License. See `LICENSE` for details.
