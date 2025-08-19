#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

missing=()

# collect lvm commands from Go source excluding tests
auth_commands=$(rg --type go -g '!*_test.go' -o 'lvcreate|lvremove' | sed 's/.*://' | sort -u)
for cmd in $auth_commands; do
  if ! rg -q "$cmd" docs/sudoers.md; then
    missing+=("$cmd")
  fi
done

# collect lvmsync-helper subcommands from Go source
helper_subs=$(rg --type go -g '!*_test.go' -o 'lvmsync-helper\s+[a-z]+' | sed -E 's/.*lvmsync-helper\s+//' | sort -u)
for sub in $helper_subs; do
  if ! rg -q "lvmsync-helper\s+$sub" docs/sudoers.md; then
    missing+=("lvmsync-helper $sub")
  fi
done

if [ ${#missing[@]} -ne 0 ]; then
  echo "Missing sudoers entries for:" >&2
  printf '  %s\n' "${missing[@]}" >&2
  exit 1
fi
