.PHONY: proto build test lint verify

proto:
	protoc --go_out=. --go-grpc_out=. --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative proto/replication.proto

build:
		@mkdir -p bin
        go build -o bin/lvmsync .
        go build -o bin/lvmsync_grpcd ./cmd/grpcd

test:
	go test -coverprofile=coverage.out ./...

lint:
	golangci-lint run

verify:
	go build ./...
	go test -coverprofile=coverage.out ./...
	golangci-lint run
