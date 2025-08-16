| Path | Why removable | Evidence | Impact | Safe removal steps |
|------|---------------|---------|--------|--------------------|
| `packaging/systemd/lvmsync-grpcd.service` | systemd unit unused in current deployments | `rg lvmsync-grpcd.service` shows no references | none | restore file from Git and re-enable with `systemctl` if needed |

## Rollback strategy

```sh
git checkout HEAD^ -- packaging/systemd/lvmsync-grpcd.service
sudo cp packaging/systemd/lvmsync-grpcd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now lvmsync-grpcd
```

## Regenerating prune candidates

Run the reachmap tool to refresh `.prune_candidates.txt`:

```sh
go run ./cmd/reachmap > .prune_candidates.txt
```
