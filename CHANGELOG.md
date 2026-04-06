# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.1] - 2026-04-05

### Changed

#### Wrapper-Aware Stateful Mode
- Stateful mode now works with complex real-world specs (Asana, Stripe-style) that use response wrappers and non-standard ID fields
- Object-wrapped responses (e.g., `{data: {...}}`) are unwrapped before storage and re-wrapped on read — store canonical resources, format at handler boundary
- Object-wrapped list responses (e.g., `{results: [...], has_more: true}`) generate envelope from schema and inject stored resources into the detected array slot
- Request body unwrapping for update operations — prevents corrupted merge when specs wrap request bodies
- Delete operations now use the spec-defined success code and generate a response body for non-204 deletes (e.g., Asana returns 200 with `{data: {}}`)
- All stateful handlers use `entry.SuccessCode` from the behavior map instead of hardcoded status codes

#### Expanded Resource Identity Extraction
- Resource ID extraction expanded from top-level `id` / path parameter / UUID fallback to a 6-strategy algorithm
- Strategy order: exact body field match → suffix strip → ID field hint → body `id` fallback → path param value → deterministic UUID
- Covers: Notion (`id`), Spotify (`id`), Asana (`gid` via suffix strip), Twilio (`sid`), Stripe (`{customer}` → body `id`), api.video (`liveStreamId`)

#### Startup Metadata Detection
- New startup pipeline annotates behavior map with wrapper keys, list array keys, and ID field hints
- Structural detection: wrapper key = single-property object resolving to object type; list array key = single array-typed property
- `allOf`-aware type checking for specs using JSON Schema composition (Asana, Spotify)

#### Deterministic List Ordering
- `List()` now returns resources sorted by resource ID — previously relied on Go map iteration order which is nondeterministic

## [0.2.0] - 2026-04-04

### Added

#### Stateful CRUD Mode
- `--mode stateful` flag enables state-aware mock responses for testing CRUD workflows
- Default mode remains `deterministic` — existing behavior is unchanged when upgrading from 0.1.0
- POST creates resources in an in-memory store, returns 201 with generated body
- GET retrieves stored resources by ID, returns 404 if not found
- GET on collection endpoints returns all stored resources of that type (empty array until first POST — unlike deterministic mode which always generates data)
- PUT and PATCH use shallow merge: request fields overwrite stored fields, unmentioned fields preserved; returns 404 if the resource does not exist (no upsert)
- DELETE removes resources from store, returns 204 (or 404 if missing)
- Generic behavior types fall through to deterministic generation

#### State Store
- In-memory state store keyed by resource type + resource ID
- LRU eviction when `--max-resources` capacity is reached (default: 10,000)
- Resource identity inference: top-level `id` field → last path parameter → deterministic UUID fallback
- Resource type derived from URL path pattern (collection segment preceding last path parameter)
- Server restart clears all state

#### Mode Integration
- `--mode` flag selects operating mode (`deterministic` or `stateful`, default: `deterministic`)
- `--max-resources` flag to configure state store capacity
- `X-Mimikos-Status` header bypasses stateful logic — uses deterministic generation, no state mutation
- Request validation runs before stateful logic (invalid requests still return 400)
- Startup banner shows active operating mode

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
