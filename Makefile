VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o ttui ./cmd/ttui

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/ttui

test:
	go vet ./... && go test ./...

# Release archives + checksums.txt in dist/ for the current git tag (upload them to a GitHub release by hand).
dist:
	go run github.com/goreleaser/goreleaser/v2@latest release --clean --skip=publish

.PHONY: build install test dist
