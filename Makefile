BINARY  := outpost
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X github.com/korneza/outpost/internal/version.Version=$(VERSION) \
  -X github.com/korneza/outpost/internal/version.Commit=$(COMMIT) \
  -X github.com/korneza/outpost/internal/version.Date=$(DATE)

.PHONY: build test lint tidy

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/outpost

# Race detector requires cgo (and on Windows, a mingw-w64 gcc). Product
# builds stay CGO_ENABLED=0; tests do not force it.
test:
	go test -race ./...

test-norace:
	go test ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
	  echo "golangci-lint not installed: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run

tidy:
	go mod tidy
	git diff --exit-code go.mod go.sum
