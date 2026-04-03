# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-04-03

First public release. Stateless behavioral mocking with deterministic, schema-valid response generation
from OpenAPI specifications. Zero configuration required.

### Added

#### Behavioral Inference
- Three-layer heuristic classifier (method + path, response schema signals, operationId keywords)
- Automatic endpoint classification: create, fetch, list, update, delete, generic
- 91.9% accuracy on a 344-endpoint corpus across 11 real-world specs (Stripe, GitHub, Asana, Spotify)
- Fallback to generic behavior for ambiguous endpoints

#### Response Generation
- Schema-valid response data for every endpoint
- Semantic field-name mapping (120+ patterns): emails for `email`, names for `name`, URLs for `url`
- Polymorphism support: `allOf`, `oneOf`, `anyOf` composition and discriminators
- Circular reference handling with configurable depth termination
- Constraint satisfaction: type, format, enum, min/max, required fields, nullable
- Depth-neutral polymorphic composition for allOf-heavy specs

#### Determinism
- Request fingerprinting via SHA-256 (method, path, query params, body)
- Per-field sub-seeding: adding or removing schema fields does not change existing field values
- Same request always produces the same response across server restarts

#### Request Validation
- Automatic validation of incoming requests against OpenAPI schemas
- Field-level error diagnostics with all violations collected (not fail-fast)
- RFC 7807 Problem Details error responses
- Content-type checking (JSON and `+json` variants)
- Request body size limit (10 MB)

#### Error Handling
- Automatic error responses for invalid requests (400, 404, 405, 415)
- RFC 7807 Problem Details as default error format
- Spec-defined error schemas used when available
- Explicit status code selection via `X-Mimikos-Status` header

#### Response Validation
- Generated responses validated against compiled JSON Schema
- Default mode: warn and send on validation failure
- Strict mode (`--strict`): return 500 on validation failure

#### OpenAPI Support
- OpenAPI 3.0 and 3.1
- YAML and JSON spec formats
- `$ref` resolution and circular reference detection
- OpenAPI 3.0 `nullable` normalization

#### CLI
- `mimikos start <spec-path>` — single command to start the mock server
- `--port`, `--strict`, `--max-depth`, `--log-level` flags
- Startup summary with classified endpoints and confidence levels
- Graceful shutdown on SIGINT/SIGTERM

#### Testing
- 383 tests (632 including subtests)
- End-to-end tests against Petstore 3.0, Petstore 3.1, and Asana specs
- Strict mode validation tests
- Sub-seeding stability tests
