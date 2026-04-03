# 🎭 Mimikos

**Deterministic mock server from OpenAPI specs**

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.25+-blue.svg)](https://golang.org)
[![Status](https://img.shields.io/badge/Status-MVP-blue.svg)]()
[![Go Report Card](https://goreportcard.com/badge/github.com/mimikos-io/mimikos)](https://goreportcard.com/report/github.com/mimikos-io/mimikos)

---

## What It Does

Point it at an OpenAPI spec. Get a working mock server:

- Generates realistic, schema-valid responses for every endpoint
- Produces the same response for the same request, every time
- Classifies endpoints automatically — no mock definitions to write
- Validates incoming requests and returns useful error diagnostics
- Works with OpenAPI 3.0 and 3.1

---

## Installation

**Go install** (requires Go 1.25+):

```bash
go install github.com/mimikos-io/mimikos/cmd/mimikos@latest
```

**Pre-built binaries:**

Download from [GitHub Releases](https://github.com/mimikos-io/mimikos/releases) for Linux, macOS, and Windows:

```bash
# macOS / Linux
tar -xzf mimikos_<os>_<arch>.tar.gz
sudo mv mimikos /usr/local/bin/

# Verify
mimikos --version
```

On Windows, download the `.zip`, extract `mimikos.exe`, and add it to your `PATH`.

---

## Quick Start

```bash
mimikos start petstore.yaml
```

```
🎭 mimikos 0.1.0
Spec: Petstore (OpenAPI 3.1.0)
Operations: 5 endpoints classified

  GET     /pets                           → list       high
  POST    /pets                           → create     high
  GET     /pets/{petId}                   → fetch      high
  DELETE  /pets/{petId}                   → delete     high
  PATCH   /pets/{petId}                   → update     high

Listening on :8080 (deterministic mode, strict=false)
```

```bash
curl http://localhost:8080/pets
curl http://localhost:8080/pets/42
curl -X POST http://localhost:8080/pets \
  -H "Content-Type: application/json" \
  -d '{"name": "Buddy"}'
```

---

## Why Mimikos

**The Problem:**
Mock servers either require you to hand-write every response, or generate shallow, random data that drifts from your
actual API. When the spec changes, your mocks break — or worse, silently become wrong.

**What You Get:**
A single binary that reads your OpenAPI spec and serves realistic responses immediately. No configuration files. No
mock definitions. No maintenance when your API evolves.

**Key Benefits:**

- **Zero configuration** — your OpenAPI spec is the only input
- **Deterministic** — same request always returns the same response, safe for snapshot testing
- **Schema evolution safe** — update your spec, responses update automatically, existing field values stay stable
- **Realistic data** — field-aware generation produces emails for `email`, names for `name`, URLs for `url`
- **Useful errors** — invalid requests get RFC 7807 Problem Details with field-level diagnostics
- **Single binary** — no runtime dependencies, no containers, no services to manage

---

## CLI Reference

```
mimikos start [flags] <spec-path>
```

| Flag          | Description                                       | Default |
|---------------|---------------------------------------------------|---------|
| `--port`      | Server port                                       | `8080`  |
| `--strict`    | Return 500 if generated response fails validation | `false` |
| `--max-depth` | Max depth for nested/circular schemas             | `3`     |
| `--log-level` | Logging verbosity (debug, info, warn, error)      | `info`  |

**Request an error response:**

```bash
curl -H "X-Mimikos-Status: 404" http://localhost:8080/pets/42
```

Returns the error response defined in your spec for that status code, or an RFC 7807 fallback if no schema is defined.

---

## Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **0.x.y versions** indicate **initial development**:
    - The API and output format may change between minor versions
    - Pin a version that works for your environment

- **1.0.0 and above** will indicate **stable output guarantees**:
    - MAJOR version for changes that alter generated responses
    - MINOR version for new features with backward-compatible output
    - PATCH version for bug fixes

The current version is in early development. Response output may change between releases until 1.0.0.

---

## Development

```bash
make build          # Build binary
make run test       # Run all tests
make run test unit  # Unit tests only
make check          # Lint + vet + test
make fix            # Auto-format + tidy
```

---

## Requirements

- Go 1.25+ (for building from source)

---

## Links

- **Changelog**: [CHANGELOG.md](CHANGELOG.md) — release history
- **Issues**: https://github.com/mimikos-io/mimikos/issues

---

## License

Apache 2.0 — See [LICENSE](LICENSE) for details.
