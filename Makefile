VERSION ?= dev

# Note: CGO_ENABLED=1 is required for go-tree-sitter
# Windows builds use CGO_ENABLED=0 (limited parser support)
CGO ?= 1

.PHONY: build test install clean

build:
	CGO_ENABLED=$(CGO) go build -ldflags "-s -w -X main.version=$(VERSION)" -o mimir ./cmd/mimir

test:
	go test ./... -count=1

install:
	CGO_ENABLED=$(CGO) go build -ldflags "-s -w -X main.version=$(VERSION)" -o /usr/local/bin/mimir ./cmd/mimir

clean:
	rm -f mimir
