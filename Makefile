# Build and test entrypoints for go-ios-vm. `make check` is the gate CI runs.
# See docs/development.md for the test tiers and their prerequisites.

GO ?= go
BIN ?= bin/go-ios-vm

.PHONY: help build check fmt-check tidy-check vet lint test test-race test-native test-live guest

help:
	@echo "make build        build $(BIN) for this host with CGO_ENABLED=0"
	@echo "make check        formatting, module tidiness, no-cgo builds, vet, lint and portable tests (CI gate)"
	@echo "make test         portable and fixture tests"
	@echo "make test-race    portable tests with the race detector"
	@echo "make test-native  nonprivileged macOS runtime tests; fails if prerequisites are unmet or none ran"
	@echo "make test-live    live research-host tests; fails if prerequisites are unmet or none ran"
	@echo "make guest        guest daemon build for iOS (external linking); fails until its prerequisites exist"

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

guest:
	@xcrun --sdk iphoneos --show-sdk-path >/dev/null 2>&1 || { echo "guest: the iPhoneOS SDK is required (install Xcode and run xcode-select)"; exit 1; }
	@echo "guest: no guest daemon package exists yet; GUEST-01 (#92) adds it and this target's build"; exit 1
