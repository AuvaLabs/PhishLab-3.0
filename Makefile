BINARY=evilginx-lab
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"
CGO_ENABLED=1

.PHONY: build clean install test

build:
	CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o dist/$(BINARY) ./cmd/evilginx-lab

clean:
	rm -rf dist/

install: build
	install -m 755 dist/$(BINARY) /usr/local/bin/$(BINARY)

test:
	go test ./...

lint:
	go vet ./...

deps:
	go mod download
	go mod tidy
