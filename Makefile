.PHONY: build test lint verify

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
