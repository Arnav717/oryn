PREFIX ?= $(HOME)/.local

.PHONY: build install test

build:
	go build -o bin/oryn ./cmd/oryn

install:
	GOBIN=$(PREFIX)/bin go install ./cmd/oryn

test:
	go test ./...
