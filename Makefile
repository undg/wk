BINARY_NAME := wk
MANPAGE := $(BINARY_NAME).1
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
MANDIR ?= $(PREFIX)/share/man
MAN1DIR ?= $(MANDIR)/man1
DESTDIR ?=
INSTALL ?= install
DIRTY := $(shell test -z "$$(git status --porcelain 2>/dev/null)" || echo -dirty)
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)$(DIRTY)

.PHONY: help
help:
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: build the binary into ./bin
.PHONY: build
build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/$(BINARY_NAME) .

## install: install wk and its man page (override PREFIX or DESTDIR as needed)
.PHONY: install
install: build
	$(INSTALL) -d "$(DESTDIR)$(BINDIR)" "$(DESTDIR)$(MAN1DIR)"
	$(INSTALL) -m 755 bin/$(BINARY_NAME) "$(DESTDIR)$(BINDIR)/$(BINARY_NAME)"
	$(INSTALL) -m 644 $(MANPAGE) "$(DESTDIR)$(MAN1DIR)/$(MANPAGE)"

## uninstall: remove wk and its man page (use the same PREFIX and DESTDIR as install)
.PHONY: uninstall
uninstall:
	$(RM) "$(DESTDIR)$(BINDIR)/$(BINARY_NAME)" "$(DESTDIR)$(MAN1DIR)/$(MANPAGE)"

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## e2e: run compiled-binary end-to-end tests
.PHONY: e2e
e2e:
	go test -run '^TestE2E' -count=1

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	go fmt ./...
	go mod tidy -v
