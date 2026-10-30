PREFIX ?= $(HOME)/.local
VERSION ?= dev
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build install test dist

build:
	go build -o bin/oryn ./cmd/oryn

install:
	GOBIN=$(PREFIX)/bin go install ./cmd/oryn

test:
	go test ./...

# dist cross-builds release archives and writes dist/SHA256SUMS.
# Verify a download with: sha256sum -c --ignore-missing SHA256SUMS
dist:
	rm -rf dist && mkdir -p dist
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=.exe; \
		name=oryn_$(VERSION)_$${os}_$${arch}; \
		mkdir -p dist/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o dist/$$name/oryn$$ext ./cmd/oryn || exit 1; \
		cp README.md dist/$$name/; \
		if [ $$os = windows ]; then (cd dist && zip -qr $$name.zip $$name); else tar -C dist -czf dist/$$name.tar.gz $$name; fi; \
		rm -rf dist/$$name; \
	done
	cd dist && sha256sum * > SHA256SUMS
