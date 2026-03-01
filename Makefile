VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFUMPT ?= $(shell command -v gofumpt 2>/dev/null)
ifeq ($(GOFUMPT),)
GOFUMPT := $(shell go env GOPATH)/bin/gofumpt
endif

.PHONY: build lint test test-race e2e fmt fmt-check clean release-dry release-check bench

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/stdiag ./cmd/stackdiag

lint:
	go vet ./...
	@test -x "$(GOFUMPT)" || (echo "gofumpt not installed: go install mvdan.cc/gofumpt@latest" && exit 1)
	@diff=$$($(GOFUMPT) -l -d .); if [ -n "$$diff" ]; then echo "$$diff"; exit 1; fi

test:
	go test $(shell go list ./... | grep -v /test/e2e)

test-race:
	go test -race $(shell go list ./... | grep -v /test/e2e)

e2e: build
	go test ./test/e2e/...

fmt:
	@$(GOFUMPT) -l -w .

fmt-check:
	@test -x "$(GOFUMPT)" || (echo "gofumpt not installed: go install mvdan.cc/gofumpt@latest" && exit 1)
	@diff=$$($(GOFUMPT) -l -d .); if [ -n "$$diff" ]; then echo "$$diff"; exit 1; fi

clean:
	rm -rf bin/ dist/

release-dry:
	goreleaser release --snapshot --clean

release-check:
	goreleaser check

bench:
	cd bench && ./run.sh
