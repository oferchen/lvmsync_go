#!/usr/bin/env bats

setup() {
  cd "$BATS_TEST_DIRNAME/../.."
}

@test "run_golangci_lint succeeds on clean code" {
  run scripts/run_golangci_lint.sh
  [ "$status" -eq 0 ]
}

@test "run_golangci_lint fails on invalid code in root module" {
  cat <<'EOGO' > bad.go
package main
func bad(){ fmt.Println("oops") }
EOGO
  run scripts/run_golangci_lint.sh
  [ "$status" -ne 0 ]
  rm bad.go
}

@test "run_golangci_lint fails on invalid code in internal module" {
  cat <<'EOGO' > internal/multierr/bad.go
package multierr
func bad(){ fmt.Println("oops") }
EOGO
  run scripts/run_golangci_lint.sh
  [ "$status" -ne 0 ]
  rm internal/multierr/bad.go
}
