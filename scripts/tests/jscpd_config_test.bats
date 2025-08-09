#!/usr/bin/env bats

setup() {
  cd "$BATS_TEST_DIRNAME/../.."
}

@test "jscpd runs successfully" {
  run npx --yes jscpd
  [ "$status" -eq 0 ]
}

