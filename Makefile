BINARY_NAME := wk
INSTALL_PATH := ~/.dot/bin/bin/$(BINARY_NAME)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help
help:
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: build the binary into ./bin
.PHONY: build
build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY_NAME) .

## install: build the binary; ~/.dot/bin/bin/wk is a symlink into bin/, so build already deploys it
.PHONY: install
install: build

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	go fmt ./...
	go mod tidy -v
