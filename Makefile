# architon-cli Makefile
BINARY := rv
CMD := ./cmd/rv
PKG := ./...
GOFLAGS :=
GO_BIN := $(shell go env GOBIN)
GO_PATH := $(shell go env GOPATH)
INSTALL_BIN ?= $(if $(GO_BIN),$(GO_BIN),$(GO_PATH)/bin)

# Optional overrides:
#   make run ARGS="check examples/amr_basic.yaml"
#   make check FILE=examples/amr_basic.yaml
ARGS ?=
FILE ?= examples/amr_parts.yaml

.PHONY: help tidy fmt vet test lint build install install-rv install-kicad-cli doctor run check validate verify version clean

help:
	@echo "Targets:"
	@echo "  tidy       - go mod tidy"
	@echo "  fmt        - gofmt all go files"
	@echo "  vet        - go vet"
	@echo "  test       - run unit tests"
	@echo "  lint       - golangci-lint (if installed)"
	@echo "  build      - build binary into ./bin/$(BINARY)"
	@echo "  install    - install rv and configure local KiCad CLI discovery"
	@echo "  doctor     - verify rv and KiCad CLI setup"
	@echo "  run        - run CLI (requires ARGS=\"...\")"
	@echo "  check      - run check on FILE (default: $(FILE))"
	@echo "  validate   - alias for check"
	@echo "  version    - print CLI version from ./bin/$(BINARY)"
	@echo "  clean      - remove ./bin"
	@echo ""
	@echo "Examples:"
	@echo "  make check"
	@echo "  make check FILE=examples/amr_basic.yaml"
	@echo "  make run ARGS=\"check examples/amr_basic.yaml\""
	@echo "  make run ARGS=\"version\""
	@echo "  make build && ./bin/$(BINARY) version"
	@echo "  make install && rv scan /path/to/kicad/project --out report.json"
	@echo "  rv version"
	

tidy:
	go mod tidy

fmt:
	gofmt -w .

vet:
	go vet $(PKG)

test:
	go test $(PKG)

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || (echo "golangci-lint not installed. Install: https://golangci-lint.run/usage/install/"; exit 1)

bin/$(BINARY): $(shell find . -name '*.go')
	mkdir -p bin
	go build $(GOFLAGS) -o bin/$(BINARY) $(CMD)

build: bin/$(BINARY)

install: install-rv install-kicad-cli doctor
	@echo ""
	@echo "Ready. Try:"
	@echo "  rv scan /path/to/kicad/project --out report.json"

install-rv:
	go install $(GOFLAGS) $(CMD)
	@echo ""
	@echo "Installed rv to: $(INSTALL_BIN)/$(BINARY)"

install-kicad-cli:
	@set -e; \
	if command -v kicad-cli >/dev/null 2>&1; then \
		echo "KiCad CLI available: $$(command -v kicad-cli)"; \
		exit 0; \
	fi; \
	candidate=""; \
	for path in \
		"/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli" \
		/Applications/KiCad*/KiCad.app/Contents/MacOS/kicad-cli \
		/Applications/KiCad/*.app/Contents/MacOS/kicad-cli; do \
		if [ -x "$$path" ]; then candidate="$$path"; break; fi; \
	done; \
	if [ -z "$$candidate" ]; then \
		echo "KiCad CLI not found in PATH or common macOS app paths."; \
		echo "rv scan can still run if you pass --kicad-cli /full/path/to/kicad-cli."; \
		exit 0; \
	fi; \
	mkdir -p "$(INSTALL_BIN)"; \
	link="$(INSTALL_BIN)/kicad-cli"; \
	if [ -e "$$link" ]; then \
		echo "kicad-cli already exists at $$link"; \
	else \
		ln -s "$$candidate" "$$link"; \
		echo "Linked kicad-cli: $$link -> $$candidate"; \
	fi

doctor:
	@set -e; \
	echo ""; \
	if command -v "$(BINARY)" >/dev/null 2>&1; then \
		echo "rv available: $$(command -v $(BINARY))"; \
	else \
		echo "rv is installed at $(INSTALL_BIN)/$(BINARY), but $(INSTALL_BIN) is not on PATH."; \
		echo 'Add this to your shell profile:'; \
		echo '  export PATH="$(INSTALL_BIN):$$PATH"'; \
	fi; \
	if command -v kicad-cli >/dev/null 2>&1; then \
		echo "kicad-cli available: $$(command -v kicad-cli)"; \
	elif [ -x "/Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli" ]; then \
		echo "kicad-cli available via KiCad app bundle: /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli"; \
	else \
		echo "kicad-cli was not found. Install KiCad or run rv scan with --kicad-cli /full/path/to/kicad-cli."; \
	fi
	
run: build
	@if [ -z "$(strip $(ARGS))" ]; then \
		echo "ERROR: ARGS is required, e.g. make run ARGS=\"check examples/amr_basic.yaml\""; \
		exit 2; \
	fi
	./bin/$(BINARY) $(ARGS)

check: build
	./bin/$(BINARY) check $(FILE)

validate: build
	./bin/$(BINARY) check $(FILE)

verify: check


version: build
	./bin/$(BINARY) version

clean:
	rm -rf bin
