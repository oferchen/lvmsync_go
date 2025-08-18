| Path | Why removable | Evidence | Impact | Safe removal steps |
|------|---------------|---------|--------|--------------------|

## Rollback strategy

```sh
sudo systemctl daemon-reload
```

## Regenerating prune candidates

Run the reachmap tool to refresh `.prune_candidates.txt`:

```sh
go run ./cmd/reachmap > .prune_candidates.txt
```
