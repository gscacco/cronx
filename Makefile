# cronx development tasks.
#
# All targets are intended to be run inside the Nix development shell:
#
#     nix develop
#
# and then, for example:
#
#     make ci

GO       ?= go
BINARY   ?= cronx
PACKAGES ?= ./...

.PHONY: all build test test-full vet fmt fmt-check lint ci clean

all: ci

## build: compile the cronx binary into ./bin
build:
	$(GO) build -o bin/$(BINARY) ./cmd/cronx

## test: run the fast test suite: everything except the tests that wait for a minute boundary
test:
	@echo "fast suite: the tests that wait for a real minute boundary are skipped (make test-full runs everything)"
	$(GO) test -short -timeout 120s $(PACKAGES)

## test-full: run every test, including the ones that wait for a real minute boundary
test-full:
	$(GO) test -timeout 300s $(PACKAGES)

## vet: run go vet
vet:
	$(GO) vet $(PACKAGES)

## fmt: format all Go sources
fmt:
	$(GO) fmt $(PACKAGES)

## fmt-check: fail if any Go source is not gofmt-formatted
fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "the following files are not gofmt-formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

## lint: run static checks (vet + formatting)
lint: vet fmt-check

## ci: everything continuous integration runs, with the full test suite
ci: lint test-full build

## clean: remove build artifacts
clean:
	rm -rf bin
