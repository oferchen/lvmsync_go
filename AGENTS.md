# Repository Guidelines

This project maintains a set of conventions to keep contributions consistent and maintainable.

## Logging
- Use [zap](https://github.com/uber-go/zap) for structured logging.
- Always call `Sync()` (e.g., `defer logger.Sync()`) before program exit to flush buffers.

## CLI Patterns
- Prefer [`pflag`](https://github.com/spf13/pflag) for flag parsing.
- Bind flags to [`viper`](https://github.com/spf13/viper) to support configuration from flags, config files, and environment variables.
- Group related options into `FlagSet`s to share common configuration across commands.
- Expose configuration via both config files and environment variables for easy automation.

## Testing and Linting
- Every function should have accompanying unit tests.
- Ensure [`golangci-lint`](https://golangci-lint.run/) v2 passes before submitting changes.

## Contribution and Commit Messages
- Follow the guidelines in `CONTRIBUTING.md`.
- Commit messages must follow the Conventional Commits format: `type(scope): description`.

## TODO
- TODO: Document structured logging field conventions.
- TODO: Provide detailed examples for complex flag grouping.
- TODO: Expand guidance on measuring test coverage.
- TODO: Clarify release tagging workflow.
