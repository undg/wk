BINARY_NAME := wk
INSTALL_PATH := ~/.dot/bin/bin/$(BINARY_NAME)

.PHONY: help
help:
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: build the binary into ./bin
.PHONY: build
build:
	go build -o bin/$(BINARY_NAME) .

## install: build and copy the binary into ~/.dot/bin/bin, replacing the bash POC
.PHONY: install
install: build
	cp bin/$(BINARY_NAME) $(INSTALL_PATH)

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	go fmt ./...
	go mod tidy -v
