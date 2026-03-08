.PHONY: build install install-scanners test lint clean docs

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
SCANNERS_DEST := $(HOME)/.abacus/scanners

build:
	go build -ldflags "-X github.com/mjn/abacus/internal/cli.Version=$(VERSION)" -o bin/abacus ./cmd/abacus

install: install-scanners
	go install -ldflags "-X github.com/mjn/abacus/internal/cli.Version=$(VERSION)" ./cmd/abacus

install-scanners:
	@echo "Building scanners..."
	@bash scanners/build-all.sh
	@echo "Installing scanners to $(SCANNERS_DEST)..."
	@mkdir -p $(SCANNERS_DEST)
	@for scanner in express orpc prisma react-router linker-ts; do \
		mkdir -p "$(SCANNERS_DEST)/$$scanner/dist"; \
		cp -r "scanners/$$scanner/dist/." "$(SCANNERS_DEST)/$$scanner/dist/"; \
	done
	@echo "Scanners installed."

test:
	go test ./...

lint:
	golangci-lint run

docs:
	@mkdir -p docs/man docs/md
	go run ./cmd/gendocs

clean:
	rm -rf bin/
