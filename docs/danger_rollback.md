# Danger Rollback Procedures

Use these steps to safely recover from failed or aborted transfers.

## Snapshot Cleanup

- LVMSync removes source snapshots automatically at the end of a run.
- If a crash leaves a snapshot behind, remove it manually with `lvremove <vg>/<snapshot>`.
- Always verify that the snapshot is no longer mounted before removing it.

## Resuming Transfers

- Start the original command with `--resume statefile` so checkpoints persist.
- After fixing the underlying issue, run the same command with `--resume statefile` to continue.
- The resume file is deleted automatically on success.

## Verify-Only Rollback

- To check that rolling back restored the expected contents, run:
  `lvmsync run --verify-only <snapshot> <target>`
- LVMSync scans both devices and reports mismatched blocks without writing.
- Use this after manual rollback to confirm the destination is consistent.
