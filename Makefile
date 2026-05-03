# architon-cli Makefile
BINARY := rv
CMD := ./cmd/rv
PKG := ./...
GOFLAGS :=
GO_BIN := $(shell go env GOBIN)
GO_PATH := $(shell go env GOPATH)
INSTALL_BIN ?= $(if $(GO_BIN),$(GO_BIN),$(GO_PATH)/bin)
ifeq ($(OS),Windows_NT)
EXE := .exe
BIN_CMD := .\bin\$(BINARY)$(EXE)
else
EXE :=
BIN_CMD := ./bin/$(BINARY)$(EXE)
endif

# Optional overrides:
#   make run ARGS="check examples/amr_basic.yaml"
#   make check FILE=examples/amr_basic.yaml
ARGS ?=
FILE ?= examples/amr_parts.yaml

.PHONY: help tidy fmt vet test lint build install install-rv doctor run check validate verify version clean

help:
	@echo "Targets:"
	@echo "  tidy       - go mod tidy"
	@echo "  fmt        - gofmt all go files"
	@echo "  vet        - go vet"
	@echo "  test       - run unit tests"
	@echo "  lint       - golangci-lint (if installed)"
	@echo "  build      - build binary into ./bin/$(BINARY)"
	@echo "  install    - install rv and verify local KiCad CLI discovery"
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
	@echo "  make build && $(BIN_CMD) version"
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
	golangci-lint run ./...

build:
	go run ./scripts/build.go $(GOFLAGS)

install: install-rv doctor
	@echo ""
	@echo "Ready. Try:"
	@echo "  rv scan /path/to/kicad/project --out report.json"

install-rv:
	go install $(GOFLAGS) $(CMD)
	@echo ""
	@echo "Installed rv to: $(INSTALL_BIN)/$(BINARY)$(EXE)"
	@echo "If 'rv' is not found, add this directory to PATH: $(INSTALL_BIN)"

doctor:
	go run $(CMD) doctor
	
run: build
	$(BIN_CMD) $(ARGS)

check: build
	$(BIN_CMD) check $(FILE)

validate: build
	$(BIN_CMD) check $(FILE)

verify: check


version: build
	$(BIN_CMD) version

clean:
	go run ./scripts/build.go --clean
