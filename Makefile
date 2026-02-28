# architon-cli Makefile
BINARY := rv
CMD := ./cmd/rv
PKG := ./...
GOFLAGS :=

# Optional overrides:
#   make run ARGS="check examples/amr_basic.yaml"
#   make check FILE=examples/amr_basic.yaml
ARGS ?=
FILE ?= examples/amr_parts.yaml

.PHONY: help tidy fmt vet test lint build install run check validate verify version clean

help:
	@echo "Targets:"
	@echo "  tidy       - go mod tidy"
	@echo "  fmt        - gofmt all go files"
	@echo "  vet        - go vet"
	@echo "  test       - run unit tests"
	@echo "  lint       - golangci-lint (if installed)"
	@echo "  build      - build binary into ./bin/$(BINARY)"
	@echo "  install    - install binary into $${GOBIN:-$$(go env GOPATH)/bin}/$(BINARY)"
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

install:
	go install $(GOFLAGS) $(CMD)
	@echo ""
	@echo "Installed to: $${GOBIN:-$$(go env GOPATH)/bin}/$(BINARY)"
	@echo "If 'rv' is not found, add this to your PATH:"
	@echo '  export PATH="'"$${GOBIN:-$$(go env GOPATH)/bin}"':$$PATH"'
	
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
