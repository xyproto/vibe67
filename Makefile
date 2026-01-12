.PHONY: install test clean

PROGRAM := vibe67
SOURCES := $(wildcard *.go)

GO ?= go
MODULE_FILES := go.mod $(wildcard go.sum)
GOFLAGS ?=

PREFIX ?= /usr
DESTDIR ?=
BINDIR ?= $(PREFIX)/bin

$(PROGRAM): $(SOURCES) $(MODULE_FILES)
	$(GO) build $(GOFLAGS)

install: $(PROGRAM)
	install -d "$(DESTDIR)$(BINDIR)"
	install -m 755 $(PROGRAM) "$(DESTDIR)$(BINDIR)/$(PROGRAM)"

test:
	@echo "Running tests..."
	$(GO) test -failfast -timeout 1m ./...

clean:
	rm -rf $(PROGRAM) build/
