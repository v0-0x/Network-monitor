# Network Monitor
#
#   make            the app for this system, in build/bin (Linux: needs
#                   webkit2gtk-4.1 and gtk3)
#   make test       the Go tests
#   make arch       build and install the Arch Linux package
#   make demo       run it with made-up connections (to try the globe)
#   make windows    cross-compile the Windows app and installer (needs the
#                   wails CLI and nsis)
#
# Releases are built by .github/workflows/release.yml on each system.

GO ?= go
NPM ?= npm
VERSION ?= 0.1.$(shell git rev-list --count HEAD 2>/dev/null || echo 0)
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
CHANNEL ?= beta
LDX = -X main.version=$(VERSION) -X main.channel=$(CHANNEL) -X main.commit=$(COMMIT)
UNAME := $(shell uname -s)
ifeq ($(UNAME),Linux)
TAGS = desktop,production,webkit2_41
else
TAGS = desktop,production
endif

.PHONY: all frontend app test arch demo windows clean

all: app

frontend:
	cd frontend && $(NPM) ci --no-audit --no-fund && $(NPM) run build

app: frontend
	mkdir -p build/bin
	$(GO) build -tags $(TAGS) -trimpath -ldflags "-s -w $(LDX)" -o build/bin/network-monitor .

test:
	$(GO) test ./internal/...

arch:
	cd packaging/arch && makepkg -sif

demo: app
	NETMON_DEMO=1 ./build/bin/network-monitor

windows:
	wails build -platform windows/amd64 -nsis -ldflags "$(LDX)"

clean:
	rm -rf build/bin frontend/dist/assets frontend/dist/index.html packaging/arch/src packaging/arch/pkg packaging/arch/*.pkg.tar.*
