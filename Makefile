.PHONY: build test lint verify bench-lan bench-wan

build:
        @mkdir -p bin
        go build -o bin/lvmsync .
        go build -o bin/lvmsyncd ./cmd/lvmsyncd

test:
	go test -coverprofile=coverage.out ./...

lint:
        golangci-lint run

verify:
        go build ./...
        go test -coverprofile=coverage.out ./...
        golangci-lint run

bench-lan:
	scripts/bench_lan.sh $(SRC) $(DEST)

bench-wan:
	scripts/bench_wan.sh $(SRC) $(DEST)
