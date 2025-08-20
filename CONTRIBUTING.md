# Contributing

Thanks for your interest in contributing to lvmsync_go!

## Commit Message Policy

This project follows the [Conventional Commits](https://www.conventionalcommits.org/) specification.

Commit messages should be structured as:

```text
type(scope): subject
```

Use a short, imperative subject line and include a scope when helpful. Examples:

```text
feat(lvm): add volume snapshot support
chore(ci): update lint configuration
```

## Architecture support

Loopback and LVM integration tests currently run only on `amd64` hardware. The
CI workflow skips these checks on other architectures, so maintainers working on
`arm64` or similar platforms can still run unit tests but should verify
integration behavior on `amd64` when possible.

## Linting

Go code is formatted and linted with dedicated tooling rather than the
GitHub Super Linter. Run the following before submitting changes:

```sh
gofmt -w .
goimports -w .
golangci-lint run
go vet ./...
staticcheck ./...
```

These commands are enforced in CI via the Go workflows.

## Sudoers file validation

CI also checks the syntax of the sample sudoers rules:

```sh
visudo -cf docs/sudoers.d/lvmsync
```

Run this locally when modifying sudoers entries to ensure they parse correctly.
