# Build and test entrypoints for go-ios-vm. `make check` is the gate CI runs.
# See docs/development.md for the test tiers and their prerequisites.

GO ?= go
BIN ?= bin/go-ios-vm
GUEST_BIN ?= bin/go-ios-vm-guestd
IOS_MIN ?= 26.0

.PHONY: help build check fmt-check tidy-check vet lint test test-race test-native test-live guest

help:
	@echo "make build        build $(BIN) for this host with CGO_ENABLED=0"
	@echo "make check        formatting, module tidiness, no-cgo builds, vet, lint and portable tests (CI gate)"
	@echo "make test         portable and fixture tests"
	@echo "make test-race    portable tests with the race detector"
	@echo "make test-native  nonprivileged macOS runtime tests; fails if prerequisites are unmet or none ran"
	@echo "make test-live    live research-host tests; fails if prerequisites are unmet or none ran"
	@echo "make guest        cross-build the Go guest daemon for iOS with external linking"

build:
	CGO_ENABLED=0 $(GO) build -o $(BIN) ./cmd/go-ios-vm

check: fmt-check tidy-check
	CGO_ENABLED=0 $(GO) build ./...
	CGO_ENABLED=0 $(GO) build -tags pins ./internal/pins
	$(MAKE) vet lint test

fmt-check:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi

tidy-check:
	$(GO) mod tidy -diff

vet:
	$(GO) vet ./...
	$(GO) vet -tags pins ./internal/pins

lint:
	golangci-lint run ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-native:
	scripts/test-tier.sh native

test-live:
	scripts/test-tier.sh live

# The guest daemon is the one component built with cgo and external linking:
# GOOS=ios needs the iPhoneOS SDK's clang and linker.
guest:
	@xcrun --sdk iphoneos --show-sdk-path >/dev/null 2>&1 || { echo "guest: the iPhoneOS SDK is required (install Xcode and run xcode-select)"; exit 1; }
	sdk="$$(xcrun --sdk iphoneos --show-sdk-path)"; \
	GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC="$$(xcrun --sdk iphoneos -f clang)" \
	CGO_CFLAGS="-isysroot $$sdk -miphoneos-version-min=$(IOS_MIN)" \
	CGO_LDFLAGS="-isysroot $$sdk -miphoneos-version-min=$(IOS_MIN)" \
	$(GO) build -trimpath -ldflags=-linkmode=external -o $(GUEST_BIN) ./cmd/go-ios-vm-guestd
