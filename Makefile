.RECIPEPREFIX := >
.PHONY: deps build test lint verify coverage bench bench-lan bench-wan vet staticcheck test-race integration release docs-check test-root

deps:
> scripts/check_deps.sh
build: deps
> @mkdir -p bin
> go build -o bin/lvmsync .
> go build -o bin/lvmsyncd ./cmd/lvmsyncd

test:
> go test -coverprofile=coverage.out ./...

test-root:
> if [ "$$(id -u)" -eq 0 ]; then go test -tags=root ./escalate ./internal/privilege; else echo "skipping root tests"; fi

coverage:
> go test -coverprofile=coverage.out ./...
> go tool cover -func=coverage.out

lint:
> golangci-lint run

vet:
> LVMSYNC_STRICT_CONFIG=1 go vet ./...

staticcheck:
> staticcheck ./...

verify: deps
> go build ./...
> go test -coverprofile=coverage.out ./...
> golangci-lint run

docs-check:
> go run ./cmd/configdoc
> git diff --exit-code docs/config_env.md
> go test ./docs

test-race:
> go test -race ./...

integration: integration/offline_enforcement.sh
> go test -tags=integration ./...

release:
> goreleaser release --clean --timeout=90m

bench-lan:
> scripts/bench_lan.sh $(SRC) $(DEST)

bench-wan:
> scripts/bench_wan.sh $(SRC) $(DEST)

bench:
> go build -o lvmsync .
> scripts/bench_smoke.sh
> rm -f lvmsync
