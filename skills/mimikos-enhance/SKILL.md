---
name: mimikos-enhance
description: >-
  Enhance an OpenAPI spec to improve quality for human readers, tools, and Mimikos.
  Gathers context from source code, tests, and sample data before enriching with
  examples, error responses, format annotations, and request body metadata.
  Use when the user wants to improve their spec, get better mock responses, or
  prepare a spec for Mimikos.
---

# Mimikos Spec Enhancement Skill

Enhance an OpenAPI spec so that it produces better results from every consumer — human
readers, documentation tools, code generators, and mock servers like Mimikos. You will
gather context from the project, analyze the spec for gaps, apply safe enrichments, and
produce a summary citing the OpenAPI standard for every change.

## When to Use

- User says "enhance my spec", "improve my OpenAPI spec", "enrich my spec", or similar
- User says "get better responses from Mimikos", "improve mock quality"
- User says "add examples to my spec", "add error responses"
- User has an OpenAPI spec and wants it reviewed for completeness
- User wants to prepare a spec for use with Mimikos

## Prerequisites

You need an OpenAPI 3.x spec file (`.yaml`, `.yml`, or `.json`). Ask the user for the
path if you don't already know it.

## Context Gathering (Human-Collaborative)

**This is the most important phase.** The quality of your enhancements depends directly
on how well you understand the domain. Context comes before analysis — always.

Wrong assumptions during context gathering cascade into rejected changes. A brief
upfront conversation prevents wasted work.

### Step 1: Read the Spec

Read the spec file. Record:

- **OAS version** from the `openapi:` field (3.0.x or 3.1.x). This determines which
  patterns are valid — `nullable: true` is 3.0, `type: [string, null]` is 3.1.
- **Domain hint** from `info.description`. This gives initial vocabulary.
- **Schema names and field names** — these become search terms for context gathering.
- **Existing examples** — note what the spec author already provided.

### Step 2: Ask the Human About Context

Before analyzing gaps, ask the human about their environment:

> Before I analyze the spec, I'd like to gather context to produce better enhancements.
> A few questions:
>
> 1. **Is this spec part of its API project?** (i.e., is the source code for this API
>    in the same repo?) If yes, where are the tests and any fixture/seed data?
> 2. **Do you have sample API responses** (from a running instance, Postman collection,
>    or saved files) I can reference?
>
> If you'd rather skip this and let me work from the spec alone, just say so — I'll
> proceed with what the spec provides.

**Three possible outcomes:**

| Human says                         | Your action                                        |
|------------------------------------|----------------------------------------------------|
| Points to source code / test paths | Search those paths for fixtures and values (step 3) |
| Provides sample responses / URL    | Read those files or make GET requests (step 4)     |
| "Just use the spec" / no answer    | Fall back to auto-detection (step 2b), then proceed |

**Step 2b: Auto-detection fallback (only if the human doesn't respond)**

If the human says to proceed without guidance, check for project markers:

```
Look for: go.mod, package.json, requirements.txt, Cargo.toml, pom.xml,
          Gemfile, composer.json, build.gradle, .csproj
```

If found, tell the human what you see and ask before searching:

> I see this looks like a [Go/Node/Python] project. I'd like to search the test files
> and fixture directories for example data that matches the spec's schemas. Should I
> proceed?

If no project markers are found, proceed with spec-only context.

### Step 3: Search Source Code and Tests

Only after the human has confirmed which sources to search.

Use the spec's paths, operationIds, and schema names as search terms:

1. **Find test files** — search for files matching `*_test.go`, `*.test.ts`,
   `*.test.js`, `test_*.py`, `*_spec.rb`, etc.
2. **Search test content** — grep for spec path segments (`/pets`, `/orders`),
   schema names (`Pet`, `Order`, `NewPet`), and operationIds (`createPet`, `listOrders`).
3. **Find fixture directories** — look for `testdata/`, `fixtures/`, `__fixtures__/`,
   `test/data/`, `seeds/`, `mocks/`.
4. **Read matching files** — extract request/response bodies, assert values, seed data.

**What to extract:**

- Concrete field values (e.g., `name: "Buddy"`, `microchip_id: "985112345678903"`)
- Value patterns (e.g., IDs prefixed with `usr_`, dates in specific ranges)
- Response shapes that confirm schema intent

**Present findings to the human for confirmation before using them:**

> I found these example values from your project:
>
> - `Pet.name`: "Buddy" (from `handlers/pets_test.go:45`)
> - `Pet.microchip_id`: "985112345678903" (from `testdata/fixtures/pets.json:12`)
> - `Owner.email`: "alice@petshop.com" (from `seeds/owners.sql:8`)
>
> I'll use these as examples in the spec. Do any of these look wrong, or are there
> other sources I should check?

This confirmation prevents misinterpreting test values — for example, a test that uses
deliberately invalid data for error-path testing.

### Step 4: Use Sample API Responses (if provided)

If the human provides a base URL: make GET requests to collection endpoints
(safe, read-only) and record response shapes and field values.

If the human provides file paths: read the files and extract values.

Present extracted values to the human for the same confirmation as step 3.

### Step 5: Build the Context Inventory

After the human has confirmed the gathered context, compile the final inventory before
proceeding to analysis:

```
Context tier: [spec-only | spec + source code | spec + live API]

Domain: [from info.description]
OAS version: [3.0.x | 3.1.x]

Confirmed context-sourced example values:
  - Pet.name: "Buddy" (from test_pets.go:45)
  - Pet.microchip_id: "985112345678903" (from testdata/fixtures/pets.json)
  - Owner.email: "alice@petshop.com" (from seeds/owners.sql)
  - ...

Fields with no context source:
  - Order.notes
  - VaccinationRecord.batch_number
  - ...
```

**Key rule:** Never proceed from context gathering to analysis without the human having
had a chance to review what was found. Even "I found nothing, proceeding with spec only"
must be stated explicitly.

## Enhancement Categories

The skill applies four categories of enrichment. All are safe, mechanical, and
standards-backed. No structural changes, no data model decisions, no hallucinated values.

### A. Examples (highest impact)

**Property-level examples** from two sources, in priority order:

1. **Context-sourced values** — from test fixtures, sample responses, or source code.
   Domain-accurate because they come from the project, not AI invention.
2. **Format-implied defaults** — standard placeholder values for known formats:

| Format      | Default example                  |
|-------------|----------------------------------|
| `email`     | `"user@example.com"`             |
| `uri`       | `"https://example.com/resource"` |
| `date-time` | `"2024-01-15T09:30:00Z"`        |
| `date`      | `"2024-01-15"`                   |
| `uuid`      | `"550e8400-e29b-41d4-a716-446655440000"` |
| `ipv4`      | `"192.0.2.1"`                    |
| `hostname`  | `"api.example.com"`              |

Fields with domain-specific meaning, no format/enum, and no context source are
**never** given examples. Skip them.

**Media-type examples** on 200/201 responses that have a schema but zero examples.
Assemble from: existing property examples, context-sourced values, enum first-values,
format defaults. Fields with no source are omitted — partial examples are better than
hallucinated ones.

**OAS reference:** The `example` field on Schema Object provides a free-form property
value sample. The `example`/`examples` field on Media Type Object provides a complete
response sample. Both improve documentation, mock servers, and code generators.
Ref: https://spec.openapis.org/oas/v3.0.3#schema-object
Ref: https://spec.openapis.org/oas/v3.0.3#media-type-object

### B. Error Responses

| Enhancement                                       | Condition                                                   |
|---------------------------------------------------|-------------------------------------------------------------|
| Add `404` response on fetch/update/delete         | GET/PATCH/PUT/DELETE on `/{param}` path, no 404 defined     |
| Add `400` or `422` response on create/update      | POST/PATCH/PUT with requestBody, no 4xx error response      |
| Create shared `ErrorResponse` schema              | Spec has no error schema in components and needs errors     |
| Create shared responses in `components/responses` | 2+ endpoints would use the same error pattern               |

**Boundaries:**

- Error schema uses a minimal pattern: `{code: integer, message: string}`. No opinionated
  RFC 7807 structure.
- If the spec already has any error schema (even a different shape), use that schema for
  new error responses. Do not replace or "improve" existing error schemas.
- Shared responses (`components/responses`) are only created when there's actual
  repetition — 2 or more endpoints with the same error pattern.

**OAS reference:** The Responses Object allows any HTTP status code as a property name.
Documenting error responses makes the API contract complete for consumers.
Ref: https://spec.openapis.org/oas/v3.0.3#responses-object

Shared responses reduce duplication via the Components Object.
Ref: https://spec.openapis.org/oas/v3.0.3#components-object

### C. Request Body Hygiene

Add `required: true` on `requestBody` for POST and PUT operations where the `required`
field is missing.

**Why:** Per the OAS standard, `requestBody.required` defaults to `false` when omitted.
For POST and PUT operations, the body is almost always required — the omission is
typically an oversight, not an intentional "optional body" design.

**Boundaries:**

- Only POST and PUT. PATCH is left as-is — partial updates legitimately have optional
  bodies.
- Schema-level `required` arrays (which fields are mandatory) are **never** modified.
  That is a data model decision. If a create schema has no `required` array, flag it in
  observations — do not fix it.

**OAS reference:** Request Body Object, `required` field: "Determines if the request
body is required in the request. Defaults to `false`."
Ref: https://spec.openapis.org/oas/v3.0.3#request-body-object

### D. Format Annotations

Add `format` to string properties where the field name unambiguously implies a standard
format and no format is currently set.

**Safe field-name to format mappings:**

| Field name pattern                                            | Format      |
|---------------------------------------------------------------|-------------|
| `email`, `*_email`, `email_*`                                 | `email`     |
| `uri`, `url`, `href`, `*_url`, `*_uri`, `website`, `homepage` | `uri`       |
| `*_at`, `*_time`, `created`, `updated`, `timestamp`           | `date-time` |
| `*_date`, `date_*`, `birthday`, `dob`                         | `date`      |
| `*_id`, `uuid`, `guid` (string type, no existing format)      | `uuid`      |
| `ipv4`, `ip_address`, `*_ip`                                  | `ipv4`      |
| `hostname`, `host`, `*_host`                                  | `hostname`  |

**Boundaries:**

- Only applied when `type: string` is already set and no `format` exists.
- `*_id` to `uuid` is only applied when type is `string`. Integer IDs are left alone.
- Ambiguous names (e.g., `address` — street or IP?) are skipped.
- Never modify an existing `format`, even if it seems wrong.

**OAS reference:** Data Types section: "Primitives have an optional modifier property:
`format`." Common formats include `date-time`, `email`, `uri`, etc.
Ref: https://spec.openapis.org/oas/v3.0.3#data-types

## The Hallucination Boundary

This is the line between what the skill does and does not do.

**Safe (the skill DOES these):**

- Add `format: email` on a field named `email` — unambiguous, standards-backed
- Add `example: "user@example.com"` on a `format: email` field — format-implied default
- Add `example: "985112345678903"` when that value was found in a test fixture — context-sourced
- Add `404` response on `GET /things/{id}` — mechanical, pattern-based
- Add `required: true` on a POST requestBody — the OAS default is `false`, almost always wrong for POST

**Unsafe (the skill NEVER does these):**

- Invent example values for domain-specific fields without a context source
- Guess enum values from descriptions or "common sense"
- Infer min/max constraints not explicitly stated in the spec
- Decide which fields should be in a `required` array
- Restructure schemas (extract to `$ref`, split read/write models)
- Change existing values, reorder properties, or rename schemas

**The line:** If a value comes from the project (test fixture, sample response, seed data)
and the human confirmed it, it's safe. If it would require the AI to invent domain
knowledge, it's unsafe.

## Flagged Observations

Issues the skill notices but does not fix. These appear in the enhancement summary
under "Flagged Observations" with a clear statement that they require human review.

| Observation                                         | Why it's flagged, not fixed                                 |
|-----------------------------------------------------|-------------------------------------------------------------|
| Create and read use same schema (no NewX / X split) | Data model decision                                         |
| Schema has properties but no `required` array       | Only the spec author knows which fields are mandatory       |
| Response uses `200` for create instead of `201`     | Intentional design choice in some APIs                      |
| No pagination on list endpoints                     | Architectural decision                                      |
| Inconsistent ID field naming across resources       | Convention choice (`id` vs `gid` vs `resource_id`)          |
| Inline schemas that could be extracted to `$ref`    | Structural refactoring                                      |
| Description mentions enum values not in schema      | Spec author may have reasons for not constraining via enum  |

Each flagged observation includes:

- **Where** in the spec
- **Why** it was flagged (with OAS standard reference)
- A clear statement: "This requires human review — it is outside the scope of automated
  enrichment."

## OAS Version Differences

The skill must respect the OAS version detected in step 1:

| Feature            | OAS 3.0.x                                    | OAS 3.1.x                                 |
|--------------------|-----------------------------------------------|--------------------------------------------|
| Nullable           | `nullable: true` on the property              | `type: [string, null]`                     |
| Examples keyword   | `example` (singular) on Schema Object         | `example` or `examples` (JSON Schema)      |
| Exclusive min/max  | `exclusiveMinimum: true` + `minimum: N`       | `exclusiveMinimum: N` (JSON Schema style)  |
| Schema dialect     | Extended subset of JSON Schema Draft-04       | Full JSON Schema Draft 2020-12             |

**Rule:** Only use patterns valid for the detected version. Never add 3.1 syntax to a 3.0
spec or vice versa.

**OAS reference:**
- 3.0.3: https://spec.openapis.org/oas/v3.0.3
- 3.1.0: https://spec.openapis.org/oas/v3.1.0

## Enhancement Workflow

After context gathering is complete, follow this sequence:

### Step 6: Analyze the Spec

Walk the spec systematically. For each operation, note:

1. Path pattern — collection (`/things`) vs item (`/things/{id}`) vs nested
2. HTTP method — POST = create, GET = fetch/list, PATCH/PUT = update, DELETE = delete
3. Response codes defined — which status codes have responses
4. Request body — present? `required` set? Schema has `required` array?
5. Response examples — any media-type, property-level, or schema-root examples?
6. Schema properties — types, formats, field names
7. Components reuse — what's in `components/schemas`, `responses`, `requestBodies`

### Step 7: Determine Enhancements

Apply in this order — earlier changes inform later ones:

1. **Error schema + shared responses** — establishes reusable components
2. **Error responses on operations** — references shared components from step 1
3. **Format annotations** — adds format before examples (so examples are format-aware)
4. **Property-level examples** — context-sourced first, then format defaults
5. **Media-type examples** — assembles from property examples + other sources
6. **Request body `required: true`** — standalone, no dependency

### Step 8: Validate Enhancements Against the OAS Standard (pre-apply)

Before writing any changes, verify each planned enhancement is sanctioned by the
OpenAPI Specification for the detected version. This is a sanity check: "Does the
standard recommend or allow this change at this location?"

For each planned enhancement, confirm:

| Enhancement type      | Pre-apply check                                                    |
|-----------------------|--------------------------------------------------------------------|
| Property `example`    | Is the target a Schema Object? (`example` is valid on Schema Object, not on all objects) |
| Media-type `example`  | Is the target a Media Type Object under `content`?                 |
| `format` annotation   | Is the property `type: string`? Is the format value a recognized format for this OAS version? |
| Error response (404)  | Does the operation already have a Responses Object? (required by OAS) |
| Shared `$ref` response| Does the `$ref` target path follow the `#/components/responses/{name}` pattern? |
| `requestBody.required`| Is this field on a Request Body Object (not some other object)?    |

**If a planned enhancement fails the pre-apply check:**

- Drop it from the enhancement list silently — do not apply it
- Record it in the enhancement summary under a "Skipped" note explaining why:
  "Planned to add `example` to X, but the target is not a Schema Object per OAS 3.0.3
  §4.7.24. Enhancement skipped."

This prevents the skill from writing changes that look reasonable but are placed in
locations the OAS standard does not support.

### Step 9: Apply Changes

Write the enhanced spec directly.

**Rules:**

- Preserve the spec's existing YAML/JSON formatting style
- Insert new content adjacent to related existing content
- Never reorder existing content
- Never remove existing content
- Never modify existing values — only add missing fields

### Step 10: Validate the Enhanced Spec (post-apply)

After applying changes and before generating the summary, validate the modified spec.
This catches broken `$ref` pointers, invalid YAML, and structural issues before the
human sees the result.

**Two validation layers, in order:**

#### 10a. Built-in checks (always run, no dependencies)

These are performed by the skill itself — no external tools needed:

1. **`$ref` integrity** — scan every `$ref` in the modified spec and verify the target
   path exists. For example, if a response references `$ref: "#/components/responses/NotFound"`,
   confirm that `components.responses.NotFound` is present.
2. **YAML/JSON parse check** — re-read the modified spec file and confirm it parses
   without syntax errors.
3. **Type consistency** — verify that added `format` annotations match the property's
   `type` (e.g., `format: email` is only on `type: string` properties, not `integer`).

If any built-in check fails, fix the issue before proceeding. These are the skill's own
errors and must be corrected — do not present a broken spec to the human.

#### 10b. External OAS validator (opportunistic, not required)

Check if any of these CLI tools are available:

```
redocly lint <spec-file>
spectral lint <spec-file>
swagger-cli validate <spec-file>
```

Check in that order. Use the first one found:

- **If a validator is available:** run it against the modified spec. If it reports errors
  introduced by the skill's changes, fix them before proceeding. Pre-existing warnings
  from the original spec are not the skill's responsibility — only react to new issues.
- **If no validator is available:** proceed without external validation. Record this fact
  for the summary — the human needs to know.

### Step 11: Generate Enhancement Summary

Produce a summary with four sections:

**Section 1 — Context Report:**

```
## Context

**Tier:** [spec-only | spec + source code | spec + live API]
**OAS version:** [version]
**Domain:** [from info.description]

**Sources used:**
- `path/to/file` — N field values extracted
- ...

**Fields with context-sourced values:** N
**Fields with format-implied defaults only:** N
**Fields with no example source (skipped):** N
```

**Section 2 — Validation Report:**

If external validation was performed:

```
## Validation

**Tool:** redocly lint v1.x.x
**Result:** No errors introduced by enhancements.
```

If no external validator was available:

```
## Validation

Built-in checks passed ($ref integrity, YAML syntax, type consistency).

**Note:** No external OAS validator (redocly, spectral, swagger-cli) was found in this
environment. It is recommended to validate the enhanced spec before committing:

    redocly lint <spec-file>
    # or
    spectral lint <spec-file>
    # or
    swagger-cli validate <spec-file>
```

**Section 3 — Applied Changes:**

Grouped by category. Each change includes:

- **What:** The specific change made
- **Where:** Spec location (path, operation, schema, field)
- **Source:** (for examples) Where the value came from — file:line or "format default"
- **Why:** Justification citing the OAS standard with a URL for further reading
- **Impact:** What improves for spec consumers

**Section 4 — Flagged Observations:**

Issues noticed but not fixed. Each includes:

- **Where:** Spec location
- **Why flagged:** Why this requires human judgment
- **Ref:** OAS standard reference for further reading

### Step 12: Present for Review

Tell the user:

- The context tier used and how many sources were found
- Whether external validation was performed (and tool name) or skipped
- How many changes were applied, by category
- Where the modified spec file is
- Print the full enhancement summary

The user reviews the diff in their editor and accepts or reverts individual changes.

## Important Constraints

- **Never alter existing values** — only add missing fields
- **Never restructure schemas** — no extracting to `$ref`, no splitting models
- **Never invent domain-specific examples** without a confirmed context source
- **Always cite the OAS standard** — every applied change references the spec
- **Always confirm context findings** with the human before using them
- **Never proceed from context gathering to analysis silently** — even "I found nothing"
  is stated explicitly
- **Respect OAS version** — only use patterns valid for the detected version
