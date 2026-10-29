PREFIX ?= $(HOME)/.local

.PHONY: build install test

build:
	go build -o bin/tape ./cmd/tape

install:
	GOBIN=$(PREFIX)/bin go install ./cmd/tape

test:
	go test ./...
