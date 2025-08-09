# Scripts

This directory contains helper scripts and associated tests.

## Go cache warming

The `.github/workflows/super-linter.yml` workflow caches Go build and module directories using [`actions/cache@v4`](https://github.com/actions/cache).
The following paths are persisted between runs:

- `~/.cache/go-build`
- `~/go/pkg/mod`

The cache key is based on `hashFiles('**/go.sum')` so it is refreshed whenever Go
module dependencies change. The test `scripts/tests/cache_warm_test.bats`
verifies that these directories exist after the cache is restored.
