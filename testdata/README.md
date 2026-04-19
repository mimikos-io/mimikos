# Testdata

Test data for the Mimikos mock server. Contains OpenAPI specs used by the E2E test suite and the
master demo spec.

## Specs

### `petshop.yaml` — Master Demo Spec

A single, coherent OpenAPI 3.0.3 spec that exercises every Mimikos feature path. Use it to explore
what Mimikos can do:

```
mimikos start testdata/specs/petshop.yaml
```

**Domain:** "PetShop Pro" — a pet shop management system with pets, owners, a store (inventory +
orders), veterinary clinics with nested rooms, and a current-user singleton.

#### Why it exists

Mimikos has many purpose-built E2E test specs, each exercising one or two features in isolation.
No single spec demonstrated the full breadth of what Mimikos supports. The petshop spec fills
that gap — it's a reference for:

- **Demos** — run `mimikos start petshop.yaml` and explore realistic responses
- **AI skills** — a "gold standard" that enhancement tools can compare user specs against
- **Documentation** — a teaching tool for the website and README examples

#### Feature coverage

Every Mimikos capability is exercised by at least one endpoint.

**Behavioral classification**

| Feature             | Endpoint                      | How                                            |
|---------------------|-------------------------------|------------------------------------------------|
| List                | `GET /pets`                   | Bare array response                            |
| Fetch               | `GET /pets/{petId}`           | Single object response                         |
| Create              | `POST /pets`                  | POST to collection, 201                        |
| Update (PATCH)      | `PATCH /pets/{petId}`         | PATCH to item, 200                             |
| Delete              | `DELETE /pets/{petId}`        | DELETE to item, 204                            |
| POST-as-update      | `POST /pets/{petId}`          | Summary: "Update a pet by ID" triggers L3 scan |
| Singleton fetch     | `GET /me`                     | operationId: `getCurrentUser` overrides L1     |
| Sub-resource delete | `DELETE /pets/{petId}/avatar` | Singular "avatar" detected at L1               |
| Action/generic      | `POST /pets/{petId}/verify`   | "verify" in action verb list                   |

**Example precedence (media-type → schema root → property → semantic → faker)**

| Level                         | Where                                | Notes                               |
|-------------------------------|--------------------------------------|-------------------------------------|
| Media-type singular `example` | `GET /store/inventory` 200           | Returned as-is, bypasses generation |
| Media-type plural `examples`  | `GET /pets/{petId}` 200              | Named example, returned as-is       |
| Media-type `$ref` example     | `GET /pets/{petId}/vaccinations` 200 | References `components/examples`    |
| Schema-root example (object)  | `Address` schema                     | Level 2 object example              |
| Schema-root example (array)   | `TagList` schema                     | Level 2 array example               |
| Property-level example        | `Pet.name: example: "Fido"`          | Property wins over semantic/faker   |
| Enum + example (enum wins)    | `Pet.status`                         | Enum constrains values              |
| Const value                   | `ActiveStatus.type: const: active`   | Discriminator branch                |
| Semantic mapping              | `Owner.email`                        | No example, field name → faker      |
| Faker fallback                | `Pet.microchip_id`                   | No example, no semantic match       |

**Response selection and errors**

| Feature                               | Endpoint                                    |
|---------------------------------------|---------------------------------------------|
| Explicit 404 with schema              | `GET /pets/{petId}`                         |
| Explicit 500 with schema              | `GET /pets`                                 |
| Schema-less error (422, no body)      | `POST /pets`                                |
| Default error response                | `GET /pets/{petId}`                         |
| `X-Mimikos-Status` header             | All endpoints                               |
| Response-level `$ref` (shared errors) | 403, 404, 429 via `components/responses`    |
| Request body `$ref`                   | `POST /pets` via `components/requestBodies` |

**Request validation**

| Feature            | Endpoint                                    |
|--------------------|---------------------------------------------|
| Required fields    | `POST /pets` — `name` required              |
| Type validation    | `POST /pets` — `age: integer`               |
| Enum validation    | `POST /pets` — `status: enum`               |
| Format validation  | `Owner.email` — `format: email`             |
| String constraints | `Pet.name` — `minLength: 1`                 |
| Required body      | `POST /pets` — `requestBody.required: true` |
| Optional body      | `PATCH /pets/{petId}` — `required: false`   |
| Content-Type check | All POST/PATCH/PUT                          |

**Schema features**

| Feature                              | Where                                   |
|--------------------------------------|-----------------------------------------|
| `$ref` to components                 | `Pet`, `Owner`, `ErrorResponse`, etc.   |
| Nullable (`nullable: true` + `$ref`) | `Adoption.pet`                          |
| Polymorphism (oneOf + discriminator) | `Pet.adoption_status` → Active/Archived |
| allOf composition                    | `VaccinationRecord` = Base + Details    |
| Circular references                  | `Category.parent` → `$ref: Category`    |
| additionalProperties                 | `Pet.metadata`                          |
| String formats                       | date, date-time, email, uuid, uri       |
| Integer constraints                  | min, max, format (int32/int64)          |
| Array maxItems                       | `GET /pets` response — `maxItems: 100`  |

**Stateful mode**

| Feature                              | Endpoint                                    |
|--------------------------------------|---------------------------------------------|
| Flat CRUD (bare array list)          | `GET/POST/PATCH/DELETE /pets`               |
| Object-wrapped list (non-"data" key) | `GET /store/orders` → `{orders: [...]}`     |
| Data-wrapped list                    | `GET /clinics` → `{data: [...]}`            |
| Nested resource with parent scope    | `GET/POST /clinics/{clinicId}/rooms`        |
| Non-standard ID field (IDFieldHint)  | `Clinic` uses `clinic_id`                   |
| Create merges request body           | `POST /pets` body fields appear in response |
| PUT update                           | `PUT /store/orders/{orderId}`               |
| Non-204 delete (200 with body)       | `DELETE /clinics/{clinicId}`                |

#### Endpoints

| Path                                 | Methods                  | Resource                                     |
|--------------------------------------|--------------------------|----------------------------------------------|
| `/pets`                              | GET, POST                | Pet list + create                            |
| `/pets/{petId}`                      | GET, POST, PATCH, DELETE | Pet fetch + update + delete + POST-as-update |
| `/pets/{petId}/avatar`               | DELETE                   | Sub-resource delete                          |
| `/pets/{petId}/vaccinations`         | GET                      | Sub-resource list                            |
| `/pets/{petId}/verify`               | POST                     | Action endpoint                              |
| `/owners`                            | GET, POST                | Owner list + create                          |
| `/owners/{ownerId}`                  | GET                      | Owner fetch                                  |
| `/store/inventory`                   | GET                      | Singleton (media-type example)               |
| `/store/orders`                      | GET, POST                | Wrapped list + create                        |
| `/store/orders/{orderId}`            | GET, PUT, DELETE         | Order fetch + update + delete                |
| `/clinics`                           | GET, POST                | Data-wrapped list + create                   |
| `/clinics/{clinicId}`                | GET, PUT, DELETE         | Clinic fetch + update + non-204 delete       |
| `/clinics/{clinicId}/rooms`          | GET, POST                | Nested list + create                         |
| `/clinics/{clinicId}/rooms/{roomId}` | GET                      | Nested fetch                                 |
| `/me`                                | GET                      | Singleton fetch                              |

---

### E2E Test Specs

Purpose-built specs that each test a specific Mimikos feature in isolation. Used by the Go test
suite in `internal/server/`.

| Spec                               | Tests                            | What it exercises                                                                |
|------------------------------------|----------------------------------|----------------------------------------------------------------------------------|
| `e2e-status-test.yaml`             | `deterministic_e2e_test.go`      | Explicit error codes (404/422/500), `X-Mimikos-Status`, optional body            |
| `e2e-example-test.yaml`            | `example_e2e_test.go`            | Property-level examples, enum priority, semantic/faker fallback, nullable `$ref` |
| `e2e-media-type-example-test.yaml` | `media_type_example_e2e_test.go` | Media-type examples (singular/plural/`$ref`), Level 2 schema examples            |
| `e2e-stateful-test.yaml`           | `stateful_e2e_test.go`           | Stripe-style wrapped list, nested resources, parent scope isolation              |
| `e2e-classifier-test.yaml`         | `classifier_e2e_test.go`         | POST-as-update, singleton fetch, sub-resource delete                             |
| `e2e-response-ref-test.yaml`       | `response_ref_e2e_test.go`       | Response-level `$ref`, request body `$ref`, shared errors                        |
| `validation-test.yaml`             | `deterministic_e2e_test.go`      | Required fields, type/enum/format/pattern validation                             |

### Petstore Specs

Standard OpenAPI Petstore variants used for baseline E2E testing.

| Spec                         | What it covers                                                    |
|------------------------------|-------------------------------------------------------------------|
| `petstore-3.0.yaml`          | OAS 3.0 basics: list, fetch, create, `default` error              |
| `petstore-3.0-expanded.yaml` | Fully resolved (no `$ref`s) variant of the 3.0 petstore           |
| `petstore-3.1.yaml`          | OAS 3.1: `type: [string, null]`, discriminator, PATCH, 204 delete |

### Corpus Specs (gitignored)

Large real-world API specs used for classifier accuracy testing. Downloaded manually, not checked
into the repository (>1 MB each).

| Spec                    | Endpoints | Source               |
|-------------------------|-----------|----------------------|
| `stripe.yaml`           | 228       | Stripe API           |
| `github.yaml`           | 181       | GitHub REST API      |
| `spotify.yaml`          | 89        | Spotify Web API      |
| `asana.yaml`            | 178       | Asana API            |
| `apivideo.yaml`         | 47        | api.video API        |
| `notion.yaml`           | 28        | Notion API           |
| `sendgrid.yaml`         | 160       | Twilio SendGrid API  |
| `twilio-api-v2010.yaml` | 162       | Twilio Voice/SMS API |

Expected classification results for these specs live in `testdata/expected/`.
