# Contributing to stackdiag

Thanks for your interest in contributing to stackdiag! This guide will help you get started.

## Development Setup

### Prerequisites

- Go 1.24 or later
- Make

### Clone and Build

```bash
git clone https://github.com/muras3/stackdiag.git
cd stackdiag
make build
# Binary at ./bin/stackdiag
```

### Run Tests

```bash
# Unit tests
make test

# Unit tests with race detector
make test-race

# End-to-end tests (builds binary first)
make e2e

# Fuzz tests (30s)
make test-fuzz
```

### Format and Lint

```bash
# Format code
make fmt

# Check formatting (CI uses this)
make fmt-check

# Run go vet
make lint
```

## Project Structure

```
cmd/stackdiag/          Entry point
internal/
  cli/              Argument parsing
  core/             Data model (Result, LayerResult, Target)
  layers/
    dns/            DNS resolution layer
    tcp/            TCP connection layer
    tls/            TLS handshake layer
    http/           HTTP request layer
  runner/           Sequential layer execution
  render/
    json/           JSON output renderer
    table/          Table (human-readable) renderer
  exitcode/         Exit code mapping
docs/               Design documents and schema reference
test/e2e/           End-to-end tests
```

## Making Changes

### Workflow

1. Fork the repository
2. Create a feature branch from `main`
3. Write tests first (TDD is required — no exceptions)
4. Implement the change
5. Run `make test && make lint` locally
6. Open a pull request

### Coding Conventions

- **Standard library preferred** — avoid external dependencies unless strongly justified
- **`CGO_ENABLED=0`** — all builds must work without CGO
- **stdout = data, stderr = logs** — this is a strict rule; never mix them
- **Schema contract** — never remove fields, never change types, additions only
- **Error codes** — follow the `{LAYER}_{CONDITION}` pattern (see [docs/schema.md](docs/schema.md))

### Test Requirements

- Every new feature or bug fix needs tests
- Unit tests go next to the code they test (`*_test.go`)
- E2E tests go in `test/e2e/`
- Use table-driven tests where appropriate
- Test failure cases, not just happy paths

### Commit Messages

Keep commits small and focused. Use conventional-style messages:

```
fix: handle DNS SERVFAIL responses
feat: add --insecure flag for TLS skip
test: add e2e test for TCP timeout
docs: update schema reference for v0.1
```

## Pull Request Guidelines

- Keep PRs focused — one logical change per PR
- Include test results in the PR description
- Link to any relevant issues
- PRs are reviewed by the maintainer team before merge

## Architecture Notes

stackdiag's internal data model **is** the JSON output. The table renderer is a view layer on top of the same `core.Result` struct. Any new layer or observation field should be added to `internal/core/types.go` first, then to the layer implementation, and finally to the renderers.

See [docs/schema.md](docs/schema.md) for the complete output specification.

## License

By contributing to stackdiag, you agree that your contributions will be licensed under the [MIT License](LICENSE).
