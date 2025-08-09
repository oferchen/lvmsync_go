#!/usr/bin/env bats

setup() {
  cd "$BATS_TEST_DIRNAME/../.."
  rm -rf "${HOME}/.cache/go-build" "${HOME}/go/pkg/mod"
}

@test "cache directories exist after warming" {
  mkdir -p "${HOME}/.cache/go-build" "${HOME}/go/pkg/mod"
  [ -d "${HOME}/.cache/go-build" ]
  [ -d "${HOME}/go/pkg/mod" ]
}
