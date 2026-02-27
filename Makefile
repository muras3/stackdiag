VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build lint test test-race test-fuzz e2e fmt fmt-check clean release-dry release-check

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/probe ./cmd/probe

lint:
	go vet ./...
	gofumpt -l -d . | grep . && exit 1 || true

test:
	go test ./...

test-race:
	go test -race $(shell go list ./... | grep -v /test/e2e)

test-fuzz:
	go test -fuzz=. -fuzztime=30s ./internal/core/...

e2e: build
	go test ./test/e2e/...

fmt:
	gofumpt -l -w .

fmt-check:
	@diff=$$(gofumpt -l -d .); if [ -n "$$diff" ]; then echo "$$diff"; exit 1; fi

clean:
	rm -rf bin/ dist/

release-dry:
	goreleaser release --snapshot --clean

release-check:
	goreleaser check
