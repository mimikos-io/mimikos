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
  "name": "Sandy Yates",
  "status": {
    "reason": "jDKAKpGL",
    "type": "archived"
  },
  "tag": "OCVSzZLR"
}
```

Note: the response name is "Sandy Yates", not "Buddy" — Mimikos generates the response
from the Pet response schema, not from your request body. The `tag` is a random string
because the schema defines it as a plain `string` with no format, example, or semantic
match. The `id` (6635) is what matters. Store it.

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
  "name": "Connor Romero",
  "status": {
    "since": "XNMhKMTw",
    "type": "active"
  },
  "tag": "FiGjSHfY"
}
```

**6. Optionally update with desired values:**

If the user wants recognizable names:

```bash
curl -s -X PATCH http://localhost:8080/pets/6635 \
  -H "Content-Type: application/json" \
  -d '{"name": "Buddy", "tag": "dog"}'
```

Response (200 OK):

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

Now `name` is "Buddy" and `tag` is "dog" — the PATCH merged your values onto the stored
resource. Other fields (`status`, `metadata`) are preserved from the original create.

**7. Verify:**

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
    "name": "Connor Romero",
    "status": { "since": "XNMhKMTw", "type": "active" },
    "tag": "FiGjSHfY"
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

Two pets in the store. The list response is a bare JSON array (Petstore uses array-typed
list responses, not object-wrapped).

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
    "name": "Stuff to buy",
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

Note: the field values like `"Stuff to buy"`, `"Greg Sanchez"`, and `"12345"` come from
`example` values defined in the Asana spec — not faker. Mimikos uses spec examples when
available (precedence: const → enum → example → semantic → faker).

The ID is inside the wrapper: `response.data.gid` = `"12345"`. Store it.

**6. Optionally update with desired values:**

```bash
# Update the project name — wrapped request body
curl -s -X PUT http://localhost:8080/projects/12345 \
  -H "Content-Type: application/json" \
  -d '{"data": {"name": "Website Redesign"}}'
```

Response (200 OK) — showing key fields only:

```json
{
  "data": {
    "gid": "12345",
    "name": "Website Redesign",
    "resource_type": "task",
    "created_at": "2026-02-13T11:09:50Z",
    "...": "... (other fields preserved from create)"
  }
}
```

The update handler unwraps `{"data": {...}}`, merges the inner fields onto the stored
resource, then re-wraps the response. The `name` is now "Website Redesign" while all
other fields are preserved.

**7. Verify:**

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

Note: this returns ALL tasks (not just tasks for this project) because of a known
namespace limitation — `/projects/{id}/tasks` and `/tasks` share the same store
namespace.
