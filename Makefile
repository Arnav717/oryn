PREFIX ?= $(HOME)/.local
VERSION ?= dev
TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build install test check dist screenshots

build:
	go build -o bin/oryn ./cmd/oryn

install:
	GOBIN=$(PREFIX)/bin go install ./cmd/oryn

test:
	go test ./...

# check is what CI runs.
check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet ./...
	go test -race ./...

# screenshots renders the README images from made-up demo data (needs Pillow).
screenshots:
	rm -rf docs/screenshots/.ansi && mkdir -p docs/screenshots/.ansi
	DEMO_OUT=$(CURDIR)/docs/screenshots/.ansi go test ./internal/tui -run TestDemoScreens -count=1
	python3 docs/screenshots/render.py docs/screenshots/.ansi docs/screenshots
	rm -rf docs/screenshots/.ansi

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
