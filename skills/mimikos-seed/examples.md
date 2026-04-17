# Seeding Examples

Two concrete examples showing the full seeding workflow. Exact response values are
deterministic but may differ across Mimikos versions.

---

## Example 1: Petstore (Simple)

A flat spec with one resource type, bare array lists, and standard `id` fields.

**Spec file:** [`testdata/specs/petstore-3.1.yaml`](../../testdata/specs/petstore-3.1.yaml)

Key schemas to note:
- `NewPet` (request) — requires `name` (string), optional `tag`
- `Pet` (response) — has `id` (integer), `name`, `tag`, `status` (oneOf discriminator),
  `metadata`
- `PetUpdate` (request) — optional `name` and `tag`

### Seeding Steps

**1. Check startup output** — no warnings, all endpoints healthy:

```
🎭 mimikos v0.3.4
Spec: Petstore 3.1 (OpenAPI 3.1.0)
Operations: 5 endpoints classified

  METHOD PATH          BEHAVIOR CONFIDENCE
  GET    /pets         → list     high
  POST   /pets         → create   high
  GET    /pets/{petId} → fetch    high
  DELETE /pets/{petId} → delete   high
  PATCH  /pets/{petId} → update   high

Listening on :8080 (stateful mode, strict=false)
```

**2. Identify create endpoints** — `POST /pets` (behavior: create).

**3. Dependency order** — only one resource type (`pets`), no dependencies.

**4. Read request schema** — `NewPet` requires `name` (string). `tag` is optional.

**5. Send create requests:**

```bash
# Create first pet
curl -s -X POST http://localhost:8080/pets \
  -H "Content-Type: application/json" \
  -d '{"name": "Buddy", "tag": "dog"}'
```

Response (201 Created):

```json
{
  "id": 6635,
  "metadata": {},
  "name": "Buddy",
  "status": {
    "reason": "jDKAKpGL",
    "type": "archived"
  },
  "tag": "dog"
}
```

Note: the response contains `"name": "Buddy"` and `"tag": "dog"` from your request body.
Fields you didn't send (`id`, `metadata`, `status`) are generated from the schema. The
`id` (6635) is server-generated and unique per create. Store it.

```bash
# Create second pet
curl -s -X POST http://localhost:8080/pets \
  -H "Content-Type: application/json" \
  -d '{"name": "Luna", "tag": "cat"}'
```

Response (201 Created):

```json
{
  "id": 2085,
  "metadata": {},
  "name": "Luna",
  "status": {
    "since": "XNMhKMTw",
    "type": "active"
  },
  "tag": "cat"
}
```

Both pets have the names and tags we sent. IDs are unique and server-generated.

**6. Verify:**

```bash
# List all pets
curl -s http://localhost:8080/pets
```

Response (200 OK):

```json
[
  {
    "id": 2085,
    "metadata": {},
    "name": "Luna",
    "status": { "since": "XNMhKMTw", "type": "active" },
    "tag": "cat"
  },
  {
    "id": 6635,
    "metadata": {},
    "name": "Buddy",
    "status": { "reason": "jDKAKpGL", "type": "archived" },
    "tag": "dog"
  }
]
```

Two pets in the store with the names and tags we sent. The list response is a bare JSON
array (Petstore uses array-typed list responses, not object-wrapped).

---

## Example 2: Asana API (Complex)

A wrapped spec with multiple resource types, `gid` instead of `id`, and nested
resources. Asana's response schemas have extensive `example` values on properties, which
Mimikos uses instead of faker — so the generated data looks realistic.

**Spec file:** [`testdata/specs/asana.yaml`](../../testdata/specs/asana.yaml)

### Key Differences from Petstore

| Pattern         | Petstore                    | Asana                              |
|-----------------|-----------------------------|------------------------------------|
| Response shape  | Flat `{id, name, ...}`      | Wrapped `{data: {gid, name, ...}}` |
| Request shape   | Flat `{name, tag}`          | Wrapped `{data: {name}}`           |
| ID field        | `id` (integer)              | `gid` (string)                     |
| List response   | Bare array `[{...}, {...}]` | Wrapped `{data: [{...}, {...}]}`   |
| Delete response | 204 No Content              | 200 with `{data: {}}`              |
| Data quality    | Mostly faker-generated      | Rich example values from spec      |

### Seeding Steps

**1. Check startup output** — confirm no warnings. Asana has 167 endpoints; look for
create behaviors in the banner.

**2. Identify create endpoints:**

- `POST /projects` (create)
- `POST /tasks` (create)
- Plus many others (`POST /goals`, `POST /tags`, etc.)

**3. Dependency order:**

- `/projects` first (no parent dependency)
- `/tasks` second (tasks can be associated with projects)

Note: in this spec, tasks are created via `POST /tasks` (root level), not
`POST /projects/{id}/tasks`. The nested path is list-only. Always check the spec — do
not assume nested paths have POST endpoints.

**4. Read request schemas:**

- `POST /projects` expects wrapped body: `{"data": {"name": "...", ...}}`
- `POST /tasks` expects wrapped body: `{"data": {"name": "...", ...}}`

**5. Send create requests:**

```bash
# Create a project — note the wrapped request body
curl -s -X POST http://localhost:8080/projects \
  -H "Content-Type: application/json" \
  -d '{"data": {"name": "Website Redesign"}}'
```

Response (201 Created) — truncated, actual response is much larger:

```json
{
  "data": {
    "gid": "12345",
    "name": "Website Redesign",
    "resource_type": "task",
    "archived": false,
    "color": "light-purple",
    "created_at": "2026-02-13T11:09:50Z",
    "notes": "These are things we need to purchase.",
    "owner": {
      "gid": "12345",
      "name": "Greg Sanchez",
      "resource_type": "task"
    },
    "...": "... (many more fields — Asana schemas are large)"
  }
}
```

Note: the `name` is `"Website Redesign"` — your request body value. Other fields like
`"Greg Sanchez"` and `"12345"` come from `example` values defined in the Asana spec.
The merge pipeline is: faker → spec examples → request body (last wins).

The `gid` inside the wrapper is the server-generated ID (protected from example overwrite
to keep IDs unique). Store it: `response.data.gid` = `"12345"`.

**6. Verify:**

```bash
# List projects — response is wrapped in {data: [...]}
curl -s http://localhost:8080/projects
```

Response (200 OK):

```json
{
  "data": [
    {
      "gid": "12345",
      "name": "Website Redesign",
      "resource_type": "task"
    }
  ]
}
```

One project in the store. The list response is wrapped: `response.data` is an array.

```bash
# List tasks for the project — uses the nested path
curl -s http://localhost:8080/projects/12345/tasks
```

Note: `/projects/{id}/tasks` and `/tasks` are separate namespaces. This endpoint only
returns tasks created via `POST /projects/{id}/tasks`, not tasks created via `POST /tasks`.
