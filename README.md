# 🎭 Mimikos

**Zero-config mock server from OpenAPI specs**

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.25+-blue.svg)](https://golang.org)
[![Status](https://img.shields.io/badge/Status-Alpha-blue.svg)]()
[![Go Report Card](https://goreportcard.com/badge/github.com/mimikos-io/mimikos)](https://goreportcard.com/report/github.com/mimikos-io/mimikos)
[![codecov](https://codecov.io/github/mimikos-io/mimikos/graph/badge.svg?token=WZ4E26OXM8)](https://codecov.io/github/mimikos-io/mimikos)

---

## What It Does

Point it at an OpenAPI spec. Get a working mock server:

- Generates realistic, schema-valid responses for every endpoint
- Produces the same response for the same request, every time
- **Stateful mode** — POST creates resources, GET retrieves them, DELETE removes them
- Classifies endpoints automatically — no mock definitions to write
- Validates incoming requests and returns useful error diagnostics
- Works with OpenAPI 3.0 and 3.1

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
xattr -d com.apple.quarantine mimikos  # macOS only — remove Gatekeeper quarantine
sudo mv mimikos /usr/local/bin/

# Verify
mimikos --version
```

> **macOS note:** The pre-built binary is not code-signed, so macOS Gatekeeper will block it on first run. The `xattr`
> command above removes the quarantine flag. Alternatively, install via `go install` which builds from source and avoids
> this entirely.

On Windows, download the `.zip`, extract `mimikos.exe`, and add it to your `PATH`.

---

## Quick Start

```bash
# Download a sample spec (or use your own OpenAPI 3.x spec)
curl -o petstore.yaml https://raw.githubusercontent.com/mimikos-io/mimikos/main/testdata/specs/petstore-3.1.yaml

mimikos start petstore.yaml
```

```
🎭 mimikos v0.3.9
Spec: Petstore 3.1 (OpenAPI 3.1.0)
Operations: 5 endpoints classified

  METHOD PATH          BEHAVIOR CONFIDENCE
  GET    /pets/{petId} → fetch    high
  DELETE /pets/{petId} → delete   high
  PATCH  /pets/{petId} → update   high
  GET    /pets         → list     high
  POST   /pets         → create   high

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

## Stateful Mode

By default, Mimikos runs in **deterministic mode** — the same request always returns the same generated response. For
testing workflows that depend on state (create → read → update → delete), use **stateful mode**:

```bash
mimikos start --mode stateful petstore.yaml
```

In stateful mode:

- **POST** creates a resource and stores it in memory
- **GET** (item) retrieves a previously created resource, or 404 if it doesn't exist
- **GET** (list) returns all created resources of that type
- **PUT/PATCH** updates a stored resource (shallow merge)
- **DELETE** removes a resource from the store

```bash
# Create a pet — returns 201 with a generated resource
curl -s -X POST http://localhost:8080/pets \
  -H "Content-Type: application/json" \
  -d '{"name": "Buddy"}'
# → {"id": 7, "name": "Buddy", "tag": "...", ...}

# Use the returned ID to fetch
curl http://localhost:8080/pets/7
# → 200 with the stored pet

# List all pets
curl http://localhost:8080/pets
# → 200 with array of created pets

# Delete
curl -X DELETE http://localhost:8080/pets/7
# → 204

# Fetch after delete
curl http://localhost:8080/pets/7
# → 404
```

Resources are stored in memory with LRU eviction. Use `--max-resources` to control capacity (default: 10,000).
Restarting the server clears all state.

---

## Explore the Full Feature Set

The Quick Start uses a minimal petstore spec. To see everything Mimikos handles — nested resources, wrapper-aware
responses, example precedence, polymorphism, error schemas, and stateful patterns — try the
[PetShop Pro](testdata/specs/petshop.yaml) demo spec:

```bash
curl -o petshop.yaml https://raw.githubusercontent.com/mimikos-io/mimikos/main/testdata/specs/petshop.yaml

mimikos start petshop.yaml
```

**Example precedence** — media-type examples are returned as-is, property-level examples override generation, semantic
mapping fills the rest:

```bash
# Media-type example: returned verbatim from the spec
curl http://localhost:8080/store/inventory

# Semantic mapping + format-aware generation
curl http://localhost:8080/owners/1

# Request a specific error response defined in the spec
curl -H "X-Mimikos-Status: 404" http://localhost:8080/pets/42
```

**Stateful mode with nested resources:**

```bash
mimikos start --mode stateful petshop.yaml

# Create a clinic
curl -s -X POST http://localhost:8080/clinics \
  -H "Content-Type: application/json" \
  -d '{"name": "Downtown Vet"}'
# → {"data": {"clinic_id": "<id>", "name": "Downtown Vet", ...}}

# Add rooms scoped to that clinic (use the returned clinic_id)
curl -s -X POST http://localhost:8080/clinics/<id>/rooms \
  -H "Content-Type: application/json" \
  -d '{"name": "Exam Room A"}'

# Rooms are scoped to their parent clinic
curl http://localhost:8080/clinics/<id>/rooms
```

See the full [feature coverage matrix](testdata/README.md) for what each endpoint exercises, or visit the
[Getting Started guide](https://mimikos.dev/getting-started/) on the documentation site.

---

## AI Agent Skills

Mimikos ships two AI agent skills for Claude Code and Cursor — one improves your spec, the other populates your mock
server with test data.

### Spec Enhancement

Improves your OpenAPI spec by adding examples, error responses, request body markers, and format annotations. The agent
gathers context from your source code and tests before making changes, so examples reflect your actual domain — not
generic placeholders.

**What it adds:** property-level examples, media-type examples, 404/422 error responses with shared schemas,
`required: true` on POST/PUT request bodies, `format` annotations on typed fields.

Setup and usage: [skills/mimikos-enhance/](skills/mimikos-enhance/README.md)

### Stateful Mode Seeding

Seeds a running Mimikos instance in stateful mode with test data. The agent reads your spec, determines resource
dependency order, constructs valid request bodies, and sends POST requests — replacing manual curl commands with a
single agent invocation.

**What it does:** discovers create endpoints, resolves parent-child ordering, constructs schema-valid request bodies,
sends requests, extracts generated IDs, and verifies the seeded state.

Setup and usage: [skills/mimikos-seed/](skills/mimikos-seed/README.md)

---

## CLI Reference

```
mimikos start [flags] <spec-path>
```

| Flag              | Description                                          | Default         |
|-------------------|------------------------------------------------------|-----------------|
| `--port`          | Server port                                          | `8080`          |
| `--mode`          | Operating mode: `deterministic`, `stateful`          | `deterministic` |
| `--max-resources` | Max stored resources in stateful mode (LRU eviction) | `10000`         |
| `--strict`        | Return 500 if generated response fails validation    | `false`         |
| `--max-depth`     | Max depth for nested/circular schemas                | `3`             |
| `--log-level`     | Logging verbosity (debug, info, warn, error)         | `info`          |

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

- **Documentation**: https://mimikos.dev/getting-started/ — guides, reference, and troubleshooting
- **Changelog**: [CHANGELOG.md](CHANGELOG.md) — release history
- **Issues**: https://github.com/mimikos-io/mimikos/issues

---

## License

Apache 2.0 — See [LICENSE](LICENSE) for details.
