# =====================================================================
#  XXC The Cleaners — Makefile
#  Targets: build build-all install uninstall test deb deb-all
#           apt-thugs-red run tui clean help
# =====================================================================

APP          := xxc-cleaners
PKG          := github.com/kawaiipantsu/xxc-cleaners
MAIN         := ./cmd/$(APP)
MAINTAINER   := Kawaiipantsu <12233528+kawaiipantsu@users.noreply.github.com>
HOMEPAGE     := https://thugs.red

# Version: prefer the latest git tag, else the VERSION file, else 0.0.0-dev.
# (A bare commit hash is intentionally not used as the package version.)
VERSION      ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' | grep . || cat VERSION 2>/dev/null || echo 0.0.0-dev)
COMMIT       := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE   := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Install locations (override with e.g. `make install PREFIX=/usr/local`).
PREFIX       ?= /usr
BINDIR       := $(DESTDIR)$(PREFIX)/bin
SYSCONFDIR   := $(DESTDIR)/etc/xxc
DOCDIR       := $(DESTDIR)$(PREFIX)/share/doc/$(APP)

# Output dirs.
DIST         := dist
BIN          := $(DIST)/$(APP)

# Cross-build matrix (64-bit only, as requested).
ARCHES       := amd64 arm64

VERPKG       := $(PKG)/internal/version
LDFLAGS      := -s -w \
  -X '$(VERPKG).Version=$(VERSION)' \
  -X '$(VERPKG).Commit=$(COMMIT)' \
  -X '$(VERPKG).BuildDate=$(BUILD_DATE)'

GO           ?= go
GOFLAGS      ?=

# apt.thugs.red deploy settings. The API key is read from the environment or a
# local .apt-deploy.env file (both git-ignored) — it is NEVER committed.
APT_ENDPOINT ?= https://apt.thugs.red/api/upload
APT_COMPONENT?= main
APT_DIST     ?= stable

.DEFAULT_GOAL := build

## ---------------------------------------------------------------- build

.PHONY: build
build: ## Build the binary for the host architecture into dist/
	@mkdir -p $(DIST)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(MAIN)
	@echo "built $(BIN) ($(VERSION))"

.PHONY: build-all
build-all: ## Cross-build static binaries for amd64 and arm64
	@mkdir -p $(DIST)
	@for arch in $(ARCHES); do \
	  echo "building linux/$$arch ..."; \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch \
	    $(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
	    -o $(DIST)/$(APP)-linux-$$arch $(MAIN); \
	done
	@echo "cross-build done -> $(DIST)/"

## ---------------------------------------------------------------- quality

.PHONY: test
test: ## Run the test suite
	$(GO) test ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format the code
	$(GO) fmt ./...

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

## ---------------------------------------------------------------- install

.PHONY: install
install: build ## Install binary + default config onto this system
	install -d $(BINDIR)
	install -m 0755 $(BIN) $(BINDIR)/$(APP)
	install -d $(SYSCONFDIR)
	@if [ ! -f $(SYSCONFDIR)/cleaners.conf ]; then \
	  install -m 0644 configs/cleaners.conf $(SYSCONFDIR)/cleaners.conf; \
	  echo "installed default config -> $(SYSCONFDIR)/cleaners.conf"; \
	else \
	  install -m 0644 configs/cleaners.conf $(SYSCONFDIR)/cleaners.conf.dist; \
	  echo "config exists; shipped new default as cleaners.conf.dist"; \
	fi
	install -d $(DOCDIR)
	install -m 0644 README.md $(DOCDIR)/README.md 2>/dev/null || true
	@echo "installed $(APP) -> $(BINDIR)/$(APP)"

.PHONY: uninstall
uninstall: ## Remove the installed binary (keeps /etc/xxc config)
	rm -f $(BINDIR)/$(APP)
	@echo "removed $(BINDIR)/$(APP) (config in /etc/xxc left intact)"

## ---------------------------------------------------------------- packaging

.PHONY: deb
deb: ## Build a .deb for the host architecture
	@$(MAKE) --no-print-directory _deb DEB_ARCH=$(shell dpkg --print-architecture 2>/dev/null || echo amd64)

.PHONY: deb-all
deb-all: ## Build .deb packages for amd64 and arm64
	@for arch in $(ARCHES); do \
	  $(MAKE) --no-print-directory _deb DEB_ARCH=$$arch; \
	done

# Internal: build a single-arch .deb. Expects DEB_ARCH.
.PHONY: _deb
_deb:
	@test -n "$(DEB_ARCH)" || { echo "DEB_ARCH not set"; exit 1; }
	@echo "packaging $(APP) $(VERSION) for $(DEB_ARCH) ..."
	@rm -rf $(DIST)/pkg-$(DEB_ARCH)
	@mkdir -p $(DIST)/pkg-$(DEB_ARCH)/DEBIAN
	@mkdir -p $(DIST)/pkg-$(DEB_ARCH)/usr/bin
	@mkdir -p $(DIST)/pkg-$(DEB_ARCH)/etc/xxc
	@mkdir -p $(DIST)/pkg-$(DEB_ARCH)/usr/share/doc/$(APP)
	CGO_ENABLED=0 GOOS=linux GOARCH=$(DEB_ARCH) \
	  $(GO) build $(GOFLAGS) -trimpath -ldflags "$(LDFLAGS)" \
	  -o $(DIST)/pkg-$(DEB_ARCH)/usr/bin/$(APP) $(MAIN)
	@sed -e 's/@VERSION@/$(VERSION)/' \
	     -e 's/@ARCH@/$(DEB_ARCH)/' \
	     -e 's#@MAINTAINER@#$(MAINTAINER)#' \
	     -e 's#@HOMEPAGE@#$(HOMEPAGE)#' \
	     packaging/debian/control.in > $(DIST)/pkg-$(DEB_ARCH)/DEBIAN/control
	@install -m 0644 configs/cleaners.conf $(DIST)/pkg-$(DEB_ARCH)/etc/xxc/cleaners.conf
	@echo "/etc/xxc/cleaners.conf" > $(DIST)/pkg-$(DEB_ARCH)/DEBIAN/conffiles
	@install -m 0755 packaging/debian/postinst $(DIST)/pkg-$(DEB_ARCH)/DEBIAN/postinst
	@install -m 0755 packaging/debian/prerm    $(DIST)/pkg-$(DEB_ARCH)/DEBIAN/prerm
	@install -m 0644 configs/cleaners.conf $(DIST)/pkg-$(DEB_ARCH)/usr/share/doc/$(APP)/cleaners.conf.example
	@install -m 0644 packaging/debian/crontab.example $(DIST)/pkg-$(DEB_ARCH)/usr/share/doc/$(APP)/crontab.example 2>/dev/null || true
	@install -m 0644 README.md $(DIST)/pkg-$(DEB_ARCH)/usr/share/doc/$(APP)/README.md 2>/dev/null || true
	@dpkg-deb --root-owner-group --build $(DIST)/pkg-$(DEB_ARCH) \
	  $(DIST)/$(APP)_$(VERSION)_$(DEB_ARCH).deb
	@echo "built $(DIST)/$(APP)_$(VERSION)_$(DEB_ARCH).deb"

## ---------------------------------------------------------------- deploy

.PHONY: apt-thugs-red
apt-thugs-red: deb-all ## Build debs and upload them to apt.thugs.red
	@# Load the deploy key from env or .apt-deploy.env (never committed).
	@if [ -z "$$XXC_APT_KEY" ] && [ -f .apt-deploy.env ]; then \
	  set -a; . ./.apt-deploy.env; set +a; \
	fi; \
	if [ -z "$$XXC_APT_KEY" ]; then \
	  echo "ERROR: XXC_APT_KEY not set (export it or create .apt-deploy.env)"; \
	  echo "       cp .apt-deploy.env.example .apt-deploy.env  and add your key"; \
	  exit 1; \
	fi; \
	for deb in $(DIST)/$(APP)_$(VERSION)_*.deb; do \
	  echo "uploading $$deb -> $(APT_ENDPOINT) ($(APT_DIST)/$(APT_COMPONENT)) ..."; \
	  XXC_APT_KEY="$$XXC_APT_KEY" APT_ENDPOINT="$(APT_ENDPOINT)" \
	  APT_DIST="$(APT_DIST)" APT_COMPONENT="$(APT_COMPONENT)" \
	    scripts/apt-deploy.sh "$$deb" || exit 1; \
	done
	@echo "deploy complete."

## ---------------------------------------------------------------- misc

.PHONY: run
run: build ## Build and run a dry-run preview
	$(BIN) --dry-run

.PHONY: tui
tui: build ## Build and launch the interactive TUI
	$(BIN) --interactive

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(DIST)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
