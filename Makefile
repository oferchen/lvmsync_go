.PHONY: proto build test

proto:
	protoc --go_out=. --go-grpc_out=. --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative proto/replication.proto

build:
	mkdir -p bin
	go build -o bin/lvmsync .
	go build -o bin/grpcd ./cmd/grpcd

test:
	go test ./...
