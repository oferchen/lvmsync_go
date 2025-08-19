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
