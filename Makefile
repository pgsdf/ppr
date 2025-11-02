# Makefile for ppr (PGSD pkg repair)
# Works on FreeBSD/GhostBSD/PGSD with /usr/bin/make (bsdmake)

# Tools and settings
GO          ?= go
APP         ?= ppr
BINDIR      ?= bin
GOFLAGS     ?= -trimpath -buildvcs=false
LDFLAGS     ?= -s -w
CGO         ?= 0

# Optional version stamping (uncomment main var and -X if you add it in main.go)
# VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# LDFLAGS    += -X 'main.buildVersion=$(VERSION)'

.PHONY: all build build-amd64 build-arm64 release fmt clean install help

all: build

## Build for the current host (native FreeBSD/GhostBSD/PGSD)
build: fmt
	@echo "==> Building $(APP) (native)…"
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(APP) .

## Cross-compile for FreeBSD amd64
build-amd64: fmt
	@echo "==> Building $(APP) for freebsd/amd64…"
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=$(CGO) \
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(APP)-freebsd-amd64 .

## Cross-compile for FreeBSD arm64 (aarch64)
build-arm64: fmt
	@echo "==> Building $(APP) for freebsd/arm64…"
	GOOS=freebsd GOARCH=arm64 CGO_ENABLED=$(CGO) \
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(APP)-freebsd-arm64 .

## Build both target architectures and produce checksums
release: clean build-amd64 build-arm64
	@echo "==> Preparing release artifacts in $(BINDIR)/"
	mkdir -p $(BINDIR)
	mv $(APP)-freebsd-amd64 $(BINDIR)/
	mv $(APP)-freebsd-arm64 $(BINDIR)/
	sha256 -q $(BINDIR)/$(APP)-freebsd-amd64 > $(BINDIR)/$(APP)-freebsd-amd64.sha256
	sha256 -q $(BINDIR)/$(APP)-freebsd-arm64 > $(BINDIR)/$(APP)-freebsd-arm64.sha256
	@echo "==> Artifacts:"
	@ls -lh $(BINDIR)/$(APP)-freebsd-amd64 $(BINDIR)/$(APP)-freebsd-arm64
	@echo "==> Checksums:"
	@sed 's/^/  /' $(BINDIR)/$(APP)-freebsd-amd64.sha256
	@sed 's/^/  /' $(BINDIR)/$(APP)-freebsd-arm64.sha256

## Code formatting
fmt:
	@echo "==> go fmt"
	$(GO) fmt ./...

## Install native build to /usr/local/bin (use DESTDIR to stage)
install: build
	@echo "==> Installing $(APP) to $(DESTDIR)/usr/local/bin/"
	install -d $(DESTDIR)/usr/local/bin
	install -s -m 755 $(APP) $(DESTDIR)/usr/local/bin/$(APP)

## Clean build artifacts
clean:
	@echo "==> Cleaning"
	rm -f $(APP) $(APP)-freebsd-amd64 $(APP)-freebsd-arm64
	rm -rf $(BINDIR)

help:
	@echo "Targets:"
	@echo "  build           - build native binary for this system"
	@echo "  build-amd64     - cross-build FreeBSD/amd64 binary"
	@echo "  build-arm64     - cross-build FreeBSD/arm64 (aarch64) binary"
	@echo "  release         - build both arches and write sha256 files into $(BINDIR)/"
	@echo "  install         - install native build to /usr/local/bin (use DESTDIR to stage)"
	@echo "  fmt             - run go fmt"
	@echo "  clean           - remove artifacts"

