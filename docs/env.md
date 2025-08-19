| ENV | Flag | YAML | Description |
| --- | ---- | ---- | ----------- |
| LVMSYNC_ALLOW_INSECURE | `--allow-insecure` | `allow-insecure` | allow insecure connections (disable TLS and host key verification) |
| LVMSYNC_ALLOW_OVERWRITE | `--allow-overwrite` | `allow-overwrite` | Allow overwriting existing data; requires --yes-i-know for non-interactive sessions |
| LVMSYNC_BLOCK_SIZE | `--block-size` | `block-size` | Block size for data transfer; specify 'auto' or 0 for automatic detection |
| LVMSYNC_BLOOM_ENTRIES | `--bloom-entries` | `bloom-entries` | Bloom filter entries |
| LVMSYNC_BLOOM_FP_RATE | `--bloom-fp-rate` | `bloom-fp-rate` | Bloom filter false positive rate |
| LVMSYNC_BLOOM_MBITS | `--bloom-mbits` | `bloom-mbits` | Bloom filter M bits per entry |
| LVMSYNC_CDC_AVG | `--cdc-avg` | `cdc-avg` | Average chunk size for CDC |
| LVMSYNC_CDC_MAX | `--cdc-max` | `cdc-max` | Maximum chunk size for CDC |
| LVMSYNC_CDC_MIN | `--cdc-min` | `cdc-min` | Minimum chunk size for CDC |
| LVMSYNC_CHECKPOINT_BYTES | `--checkpoint-bytes` | `checkpoint-bytes` | Bytes between resume checkpoints |
| LVMSYNC_CHECKPOINT_INTERVAL | `--checkpoint-interval` | `checkpoint-interval` | Duration between checkpoints |
| LVMSYNC_CHUNK_SEED | `--chunk-seed` | `chunk-seed` | Seed for chunking |
| LVMSYNC_COMPRESS | `--compress` | `compress` | Compression algorithm: [none lz4 zstd auto] |
| LVMSYNC_COMPRESS_CONCURRENCY | `--compress-concurrency` | `compress-concurrency` | Compression concurrency |
| LVMSYNC_COMPRESS_THRESHOLD | `--compress-threshold` | `compress-threshold` | Skip compression when estimated ratio exceeds this value |
| LVMSYNC_CONCURRENCY | `--concurrency` | `concurrency` | Number of concurrent connections |
| LVMSYNC_CONFIG | `--config` | `config` | Path to config YAML file |
| LVMSYNC_CREATE_DEST_LV | `--create-dest-lv` | `create-dest-lv` | Create destination logical volume when missing |
| LVMSYNC_DEDUP | `--dedup` | `dedup` | Deduplication mode: [fixed cdc hybrid] |
| LVMSYNC_DEDUP_STATE_FILE | `--dedup-state-file` | `dedup-state-file` | Path to deduplication state file |
| LVMSYNC_DEDUP_STRATEGY | `--dedup-strategy` | `dedup-strategy` | Deduplication strategy: [none auto checksum rolling_hash bloom] |
| LVMSYNC_DELTA | `--delta` | `delta` | Delta algorithm (none, rsync) to precompute byte-level changes |
| LVMSYNC_DEST_TYPE | `--dest-type` | `dest-type` | Destination device type (auto,file,raw,lvm) |
| LVMSYNC_DIGEST | `--digest` | `digest` | Digest algorithm: [sha256 blake3 auto] |
| LVMSYNC_DISCARD | `--discard` | `discard` | Issue BLKDISCARD before writing blocks |
| LVMSYNC_DRY_RUN | `--dry-run` | `dry-run` | Print actions without executing |
| LVMSYNC_FORCE | `--force` | `force` | Override safety checks for offline raw access or filesystem freeze |
| LVMSYNC_FREEZE_TIMEOUT | `--freeze-timeout` | `freeze-timeout` | Timeout for filesystem freeze command |
| LVMSYNC_FS_FREEZE_COMMAND | `--fs-freeze-command` | `fs-freeze-command` | Command to freeze filesystem before reading raw source |
| LVMSYNC_FS_THAW_COMMAND | `--fs-thaw-command` | `fs-thaw-command` | Command to thaw filesystem after reading raw source |
| LVMSYNC_INTRA_DEDUP | `--intra-dedup` | `intra-dedup` | Enable intra-run deduplication |
| LVMSYNC_KNOWN_HOSTS | `--known-hosts` | `known-hosts` | Path to known_hosts file |
| LVMSYNC_LVM_ESCALATION | `--lvm-escalation` | `lvm-escalation` | Command to use for privilege escalation |
| LVMSYNC_LVM_TIMEOUT | `--lvm-timeout` | `lvm-timeout` | Timeout for LVM commands and privilege checks |
| LVMSYNC_LVMSYNC_PATH | `--lvmsync-path` | `lvmsync-path` | Remote command to run |
| LVMSYNC_LZ4_LEVEL | `--lz4-level` | `lz4-level` | LZ4 compression level: fast or hc |
| LVMSYNC_MANIFEST_ALLOW_MOUNTED | `--manifest-allow-mounted` | `manifest-allow-mounted` | Allow rebuilding when device is mounted read-write |
| LVMSYNC_MANIFEST_PATH | `--manifest-path` | `manifest-path` | Path to manifest file |
| LVMSYNC_MANIFEST_PROGRESS_INTERVAL | `--manifest-progress-interval` | `manifest-progress-interval` | Interval between progress logs during manifest rebuild |
| LVMSYNC_MANIFEST_TIMEOUT | `--manifest-timeout` | `manifest-timeout` | Timeout for manifest rebuild (0 to disable) |
| LVMSYNC_MAX_RETRIES | `--max-retries` | `max-retries` | Maximum number of retries per block |
| LVMSYNC_MODE | `--mode` | `mode` | Preset mode: default or throughput |
| LVMSYNC_NUMA_NODE | `--numa-node` | `numa-node` | NUMA node to pin worker goroutines |
| LVMSYNC_NUMA_PIN | `--numa-pin` | `numa-pin` | Pin worker goroutines to device NUMA node |
| LVMSYNC_ODIRECT | `--odirect` | `odirect` | Use O_DIRECT for device I/O when possible |
| LVMSYNC_OFFLINE | `--offline` | `offline` | Assume source raw device is offline |
| LVMSYNC_OUTPUT | `--output` | `output` | Output format: text, json, or yaml |
| LVMSYNC_PARALLEL | `--parallel` | `parallel` | Number of concurrent workers |
| LVMSYNC_PLAN | `--plan` | `plan` | Print configuration plan as JSON and exit |
| LVMSYNC_PROBE_ONLY | `--probe-only` | `probe-only` | Output size_bytes, kernel_uuid, gpt_uuid, fs_uuid, major, minor, and manifest_epoch without writing |
| LVMSYNC_PROGRESS | `--progress` | `progress` | Show progress during transfer |
| LVMSYNC_REMOTE_POST_SCRIPT | `--remote-post-script` | `remote-post-script` | Remote script to run after transfer |
| LVMSYNC_REMOTE_PRE_SCRIPT | `--remote-pre-script` | `remote-pre-script` | Remote script to run before transfer |
| LVMSYNC_RESUME | `--resume` | `resume` | Path to resume state file |
| LVMSYNC_RETRY_DELAY | `--retry-delay` | `retry-delay` | Initial delay between retries |
| LVMSYNC_SANITIZE_ENV | `--sanitize-env` | `sanitize-env` | Drop PATH, LANG, and unsafe variables before privilege escalation (disable with --sanitize-env=false) |
| LVMSYNC_SIG_CACHE_MAX | `--sig-cache-max` | `sig-cache-max` | Maximum LVM signature cache entries |
| LVMSYNC_SIG_CACHE_TTL | `--sig-cache-ttl` | `sig-cache-ttl` | TTL for LVM signature cache entries |
| LVMSYNC_SKIP_DISK_CHECK | `--skip-disk-check` | `skip-disk-check` | Skip disk space check |
| LVMSYNC_SKIP_SNAPSHOT_CREATION | `--skip-snapshot-creation` | `skip-snapshot-creation` | Skip snapshot creation |
| LVMSYNC_SNAPSHOT_SIZE | `--snapshot-size` | `snapshot-size` | Snapshot size (bytes or percentage) |
| LVMSYNC_SOURCE_TYPE | `--source-type` | `source-type` | Source device type (auto,file,raw,lvm) |
| LVMSYNC_SPARSE | `--sparse` | `sparse` | Sparse file handling: auto or never |
| LVMSYNC_SPEED | `--speed` | `speed` | Transfer speed limit |
| LVMSYNC_SSH_HOST | `--ssh-host` | `ssh-host` | SSH host |
| LVMSYNC_SSH_HOST_KEY | `--ssh-host-key` | `ssh-host-key` | Expected SSH host public key |
| LVMSYNC_SSH_HOST_KEY_PATH | `--ssh-host-key-path` | `ssh-host-key-path` | Path to SSH host private key |
| LVMSYNC_SSH_KEEPALIVE | `--ssh-keepalive` | `ssh-keepalive` | SSH keepalive interval |
| LVMSYNC_SSH_KEY | `--ssh-key` | `ssh-key` | Path to SSH private key or use agent |
| LVMSYNC_SSH_PORT | `--ssh-port` | `ssh-port` | SSH port |
| LVMSYNC_SSH_TIMEOUT | `--ssh-timeout` | `ssh-timeout` | SSH connection timeout |
| LVMSYNC_SSH_USER | `--ssh-user` | `ssh-user` | SSH username |
| LVMSYNC_STDOUT | `--stdout` | `stdout` | Write change dump to STDOUT |
| LVMSYNC_STRICT_HOST_KEY_CHECKING | `--strict-host-key-checking` | `strict-host-key-checking` | Require host keys to be present in known_hosts |
| LVMSYNC_SYNC_INTERVAL | `--sync-interval` | `sync-interval` | Bytes between fdatasync calls |
| LVMSYNC_TARGET_VGS | `--target-vgs` | `target-vgs` | Candidate target VGs for volume selection |
| LVMSYNC_TARGET_VOLUME_GROUP | `--target-volume-group` | `target-volume-group` | Target LVM volume group |
| LVMSYNC_TCP_LOWAT | `--tcp-lowat` | `tcp-lowat` | TCP_NOTSENT_LOWAT threshold in bytes |
| LVMSYNC_TCP_PARALLEL | `--tcp-parallel` | `tcp-parallel` | Number of parallel TCP connections per worker |
| LVMSYNC_TCP_PORT | `--tcp-port` | `tcp-port` | TCP port |
| LVMSYNC_THAW_TIMEOUT | `--thaw-timeout` | `thaw-timeout` | Timeout for filesystem thaw command |
| LVMSYNC_TRANSPORT | `--transport` | `transport` | Transport modes (comma-separated) |
| LVMSYNC_VERBOSE | `--verbose` | `verbose` | Verbosity level |
| LVMSYNC_VERIFY | `--verify` | `verify` | Verification level: full, sampled, or none |
| LVMSYNC_VERIFY_CHECKSUM | `--verify-checksum` | `verify-checksum` | Enable checksum verification |
| LVMSYNC_VERIFY_ONLY | `--verify-only` | `verify-only` | Verify destination without writing data |
| LVMSYNC_VOLUME_GROUP | `--volume-group` | `volume-group` | LVM volume group |
| LVMSYNC_YES_I_KNOW | `--yes-i-know` | `yes-i-know` | Confirm destructive write operations in non-interactive sessions |
| LVMSYNC_ZEROCOPY | `--zerocopy` | `zerocopy` | Enable zero-copy transfers |
| LVMSYNC_ZSTD_LEVEL | `--zstd-level` | `zstd-level` | Zstd compression level (1-5) |
