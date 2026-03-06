.PHONY: build install test lint clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	go build -ldflags "-X github.com/mjn/abacus/internal/cli.Version=$(VERSION)" -o bin/abacus ./cmd/abacus

install:
	go install -ldflags "-X github.com/mjn/abacus/internal/cli.Version=$(VERSION)" ./cmd/abacus

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/
