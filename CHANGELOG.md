# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.5] - 2026-04-13

### Fixed

#### Nested Resource Namespace Collision
- `/projects/{project_gid}/tasks` and `/tasks` no longer share the same store namespace in stateful mode
- Previously, `POST /tasks` created a task visible under `GET /projects/{any}/tasks` and vice versa — all resources of the same leaf type (e.g., "tasks") shared one pool regardless of path hierarchy
- Store namespaces now include the full path structure: `/projects/{gid}/tasks` uses namespace `projects/*/tasks`, while `/tasks` uses `tasks`
- Affects any spec with nested resources (like `/projects/{gid}/sections`, `/teams/{gid}/projects`)

### Changed

#### Version Segments Preserved in Store Namespace
- Path version segments (`v1`, `v2`, `api`) are no longer stripped from store namespace keys
- `/v1/pets` now uses namespace `v1/pets` instead of `pets` — if a spec defines both `/v1/pets` and `/v2/pets`, they are separate namespaces
- Consistent with the "spec is law" principle: the spec's paths are taken literally
- No impact on specs with a single version prefix (the common case) — the namespace key changes but all CRUD operations use the same key, so behavior is identical

## [0.3.4] - 2026-04-11

### Changed

#### Router: ServeMux replaced with chi
- Replaced Go's `net/http.ServeMux` with [chi](https://github.com/go-chi/chi) for HTTP routing
- Removed ~35 lines of workaround code for ServeMux's panic on literal/wildcard sibling paths
- 404 and 405 responses are now handled by chi's native `NotFound` and `MethodNotAllowed` handlers
- No behavioral changes — all existing routes, responses, and error formats are identical

### Added

#### Graceful panic recovery
- Endpoints that panic during startup are now skipped instead of crashing the server — the server starts with the remaining endpoints and logs warnings for each failure
- Requests to failed endpoints return an actionable RFC 7807 error: `"This endpoint failed to register at startup: <error>"`
- Runtime panics in any handler are caught by recovery middleware and return RFC 7807 500 responses instead of dropping the connection
- Startup banner shows failed endpoint count when > 0

#### Degraded schema handling
- Endpoints whose response schema fails to compile at startup now return an actionable RFC 7807 error instead of silently returning `{}`
- Error detail includes the schema name and compilation error so the developer knows exactly what to fix in their spec
- In strict mode (`--strict`), endpoints with failed request schemas also return RFC 7807 for body-bearing methods (POST/PUT/PATCH)
- In non-strict mode, request schema failures are tolerated — the endpoint works but skips request body validation
- Media-type examples bypass degradation — if the spec defines an example for the response, it is served even when the schema failed to compile
- Startup banner shows degraded schema count when > 0
- Builder warning messages now describe the impact: response schema failures say "endpoint will return an error", request schema failures say "validation will be skipped"

## [0.3.3] - 2026-04-10

### Fixed

#### Required Request Body Validation
- Requests to endpoints with `requestBody.required: true` that send no body now return a clear 400 error: `"Request body is required"`
- Previously, missing required bodies fell through to libopenapi-validator which produced confusing error messages like `"POST operation request content type '' does not exist"` (misdiagnoses the problem) or `"POST request body is empty for '/pets'"` (leaks internal path)
- The check short-circuits before content-type and schema validation for faster feedback

## [0.3.2] - 2026-04-10

### Added

#### Media-Type Example Responses
- When an OpenAPI spec defines `example` or `examples` on a response media type, Mimikos now returns the authored example directly instead of generating data from the schema
- Supports both singular `example` (inline value) and plural `examples` (named Example Objects, first entry used)
- Works at two levels:
  - **Level 1 (router):** Complete media-type examples bypass generation entirely — returned as-is with correct status code
  - **Level 2 (generator):** Object/array schema-level examples replace sub-property generation for that subtree
- Per-status-code examples: each response code can have its own example, selectable via `X-Mimikos-Status` header
- `$ref` examples (e.g., `$ref: '#/components/examples/FooExample'`) resolve correctly via libopenapi
- POST/PUT/PATCH responses with examples work identically to GET
- Media-type examples bypass response validation — the spec author wrote both the schema and the example

### Fixed

#### YAML Integer Type Normalization
- YAML integers in media-type examples are now normalized to `int64` during parsing, matching the generator's output type
- Previously, YAML `int` values (from `yaml.Node.Decode`) could differ from generator `int64` values in Go-level comparisons, despite being identical on the JSON wire

## [0.3.1] - 2026-04-09

### Added

#### Startup Version Check
- Mimikos now checks for newer versions at startup via the GitHub Releases API
- Non-blocking: runs concurrently with server startup and prints a notification only if the check completes in time
- Shows both installation methods: `go install` command and GitHub releases download link
- Silently skipped on network failures, timeouts, or dev builds — never delays startup

### Changed

#### Default Max Depth Increased From 3 to 10
- The `--max-depth` default is now 10 (was 3), allowing nested objects and arrays inside list response items to generate fully
- The previous default of 3 caused depth exhaustion on the common list-response pattern (`Wrapper → array → Item → nested object/array`), producing `null` objects and empty arrays at the fourth level
- The depth guard exists for circular schemas — 10 levels is more than enough for real-world specs while still preventing infinite recursion

### Fixed

#### Nullable Properties No Longer Hide Example Values
- Nullable properties (`nullable: true` + `$ref`, or `anyOf: [{$ref}, {type: "null"}]`) now always generate the non-null branch
- Previously, seed-based branch selection picked null ~50% of the time, hiding all example values behind nulls on real production specs
- Consistent with inline nullable types (`type: ["object", "null"]`) which already preferred non-null

## [0.3.0] - 2026-04-08

### Added

#### Example-Aware Response Generation
- Property-level `example` values from OpenAPI specs are now used in response generation
- Priority chain: const → enum → **example** → semantic mapper → faker — spec-author examples take precedence over heuristic field-name matching
- Works across all primitive types (string, integer, number, boolean) and all OpenAPI versions (3.0, 3.1)
- Type-safe: mismatched examples (e.g., string example on an integer field) fall through gracefully to faker
- Deterministic: example values are constants, producing identical output regardless of request seed

#### Startup Banner Polish
- Endpoint table now includes column headers (METHOD, PATH, BEHAVIOR, CONFIDENCE)
- Column widths computed dynamically from actual entries — no more truncated long paths

## [0.2.3] - 2026-04-07

### Fixed

#### Route Registration Panic With Literal/Wildcard Sibling Paths
- Specs with a literal path (e.g., `/recipes/shared`) alongside a parameterized sibling (e.g., `/recipes/{id}`) no longer crash on startup
- Root cause: Go 1.22+ ServeMux panics when a method-less catch-all pattern conflicts with a method-specific wildcard pattern — replaced with per-method 405 handlers that avoid the ambiguity
- Affects any real-world spec with both literal and parameterized paths under the same prefix (common in production APIs)

## [0.2.2] - 2026-04-07

### Changed

#### Improved Behavioral Classification (92.2% → 96.2%)
- L3 summary scanning: POST-to-item endpoints with CRUD keywords in `summary` (e.g., "Update a customer") are now correctly classified as update instead of create — fixes 6 Stripe-style POST-as-update endpoints
- L3 targeted list→fetch override: singleton endpoints (e.g., `GET /me`) with CRUD keywords in operationId or summary are now correctly classified as fetch instead of list — fixes 6 endpoints across GitHub, API.video, and Twilio specs
- L1 sub-resource delete detection: `DELETE /resource/{id}/sub-resource` with singular last segment is now classified as delete instead of generic — fixes 2 Spotify endpoints
- Zero regressions across the 344-endpoint corpus

### Added

#### Classifier Improvement E2E Tests
- End-to-end tests verifying POST-as-update, sub-resource delete, and singleton fetch patterns through the full pipeline
- Strict mode validation for all classifier improvement patterns
- Determinism verification for newly classified endpoints

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
