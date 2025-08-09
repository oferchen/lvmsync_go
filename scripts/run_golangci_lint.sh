#!/usr/bin/env bash
set -euo pipefail

# Determine repository root (one level up from this script's directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/.."
cd "${REPO_ROOT}"

# Extract module paths from go.work
modules=$(awk '/^use \(/, /^\)/ {if ($1 != "use" && $1 != ")") print $1}' go.work)

for module in $modules; do
  echo "Running golangci-lint in ${module}"
  (cd "${module}" && golangci-lint run ./...)
done
