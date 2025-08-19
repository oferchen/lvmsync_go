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

## Continuous Integration

Pull requests must pass the required GitHub Actions checks before merge:

- `go` – builds the project, runs `go test -race ./...`, and executes integration tests.
- `lint` – runs `golangci-lint run`.

Branch protection marks both workflows as required. Run these commands locally to catch issues early.
