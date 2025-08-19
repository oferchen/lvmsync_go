.RECIPEPREFIX := >
.PHONY: build test lint verify bench bench-lan bench-wan vet staticcheck test-race integration release

build:
> @mkdir -p bin
> go build -o bin/lvmsync .
> go build -o bin/lvmsyncd ./cmd/lvmsyncd

test:
> go test -coverprofile=coverage.out ./...

lint:
> golangci-lint run

vet:
> go vet ./...

staticcheck:
> staticcheck ./...

verify:
> go build ./...
> go test -coverprofile=coverage.out ./...
> golangci-lint run

test-race:
> go test -race ./...

integration:
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
