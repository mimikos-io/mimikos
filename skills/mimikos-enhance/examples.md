# Enhancement Examples

Two examples showing how the enhancement skill works at different context tiers.
Both use the same Bookstore API spec. Example 1 shows the floor (spec only).
Example 2 shows how source code context produces richer results using the same
starting spec.

---

## Example 1: Bookstore API — Spec Only

The user has a spec but no source code is available. The skill works from the spec alone.

### Before (user's spec)

```yaml
openapi: "3.0.3"
info:
  title: Bookstore API
  version: "1.0.0"
  description: A simple bookstore management API for browsing and purchasing books.

servers:
  - url: http://localhost:3000

paths:
  /books:
    get:
      operationId: listBooks
      summary: List all books
      responses:
        "200":
          description: A list of books
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/Book"
    post:
      operationId: createBook
      summary: Add a new book
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/NewBook"
      responses:
        "201":
          description: Book created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"

  /books/{bookId}:
    get:
      operationId: getBook
      summary: Get a book by ID
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: A single book
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"
    delete:
      operationId: deleteBook
      summary: Delete a book
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "204":
          description: Book deleted

components:
  schemas:
    Book:
      type: object
      properties:
        id:
          type: string
        title:
          type: string
        author:
          type: string
        isbn:
          type: string
        price:
          type: number
        published_at:
          type: string
        author_email:
          type: string
        website:
          type: string

    NewBook:
      type: object
      properties:
        title:
          type: string
        author:
          type: string
        isbn:
          type: string
        price:
          type: number
        published_at:
          type: string
```

### Context Gathering

```
AI:    Before I analyze the spec, I'd like to gather context to produce better
       enhancements. A few questions:

       1. Is this spec part of its API project? (i.e., is the source code for
          this API in the same repo?) If yes, where are the tests and any
          fixture/seed data?
       2. Do you have sample API responses (from a running instance, Postman
          collection, or saved files) I can reference?

       If you'd rather skip this and let me work from the spec alone, just say
       so — I'll proceed with what the spec provides.

Human: No, I just have the spec YAML.

AI:    Got it. No source code or example responses are available for this spec.
       I found nothing beyond the spec itself, so I'll proceed with spec-only
       context.

       Context tier: spec-only
       Domain: bookstore management (from info.description)
       OAS version: 3.0.3

       Fields with no context source (examples skipped):
         - Book.id, Book.title, Book.author, Book.isbn, Book.price
         - NewBook.title, NewBook.author, NewBook.isbn, NewBook.price

       Fields where format implies a default example:
         - Book.published_at (date-time)
         - Book.author_email (email)
         - Book.website (uri)

       Let me proceed with the analysis.
```

### After (enhanced spec)

```yaml
openapi: "3.0.3"
info:
  title: Bookstore API
  version: "1.0.0"
  description: A simple bookstore management API for browsing and purchasing books.

servers:
  - url: http://localhost:3000

paths:
  /books:
    get:
      operationId: listBooks
      summary: List all books
      responses:
        "200":
          description: A list of books
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/Book"
    post:
      operationId: createBook
      summary: Add a new book
      requestBody:
        required: true                                            # Added (C)
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/NewBook"
      responses:
        "201":
          description: Book created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"
        "422":                                                    # Added (B)
          description: Validation error
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ErrorResponse"

  /books/{bookId}:
    get:
      operationId: getBook
      summary: Get a book by ID
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: A single book
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"
        "404":                                                    # Added (B)
          $ref: "#/components/responses/NotFound"
    delete:
      operationId: deleteBook
      summary: Delete a book
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "204":
          description: Book deleted
        "404":                                                    # Added (B)
          $ref: "#/components/responses/NotFound"

components:
  responses:                                                      # Added (B)
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/ErrorResponse"

  schemas:
    Book:
      type: object
      properties:
        id:
          type: string
        title:
          type: string
        author:
          type: string
        isbn:
          type: string
        price:
          type: number
        published_at:
          type: string
          format: date-time                                       # Added (D)
          example: "2024-01-15T09:30:00Z"                         # Added (A)
        author_email:
          type: string
          format: email                                           # Added (D)
          example: "user@example.com"                             # Added (A)
        website:
          type: string
          format: uri                                             # Added (D)
          example: "https://example.com/resource"                 # Added (A)

    NewBook:
      type: object
      properties:
        title:
          type: string
        author:
          type: string
        isbn:
          type: string
        price:
          type: number
        published_at:
          type: string
          format: date-time                                       # Added (D)
          example: "2024-01-15T09:30:00Z"                         # Added (A)

    ErrorResponse:                                                # Added (B)
      type: object
      required:
        - code
        - message
      properties:
        code:
          type: integer
        message:
          type: string
```

### Enhancement Summary

```markdown
## Enhancement Summary — Bookstore API

### Context

**Tier:** Spec-only
**OAS version:** 3.0.3
**Domain:** Bookstore management (from info.description)

**Fields with format-implied defaults only:** 3 (published_at, author_email, website)
**Fields with no example source (skipped):** 7 (id, title, author, isbn, price in Book;
title, author, isbn, price in NewBook)

### Applied Changes

#### A. Examples (4 changes)

1. Added `example: "2024-01-15T09:30:00Z"` to `Book.published_at`
   - **Where:** components/schemas/Book/properties/published_at
   - **Source:** Format default for `date-time`
   - **Why:** The `example` field on Schema Object provides a free-form property value
     sample. Since this field has `format: date-time`, a standard placeholder is used.
     Ref: https://spec.openapis.org/oas/v3.0.3#schema-object
   - **Impact:** Mock servers return a realistic timestamp instead of a random string.

2. Added `example: "user@example.com"` to `Book.author_email`
   - **Where:** components/schemas/Book/properties/author_email
   - **Source:** Format default for `email`
   - **Why:** Same as above. Format-implied default provides a valid email address.
     Ref: https://spec.openapis.org/oas/v3.0.3#schema-object
   - **Impact:** Mock servers return a valid email instead of a random string.

3. Added `example: "https://example.com/resource"` to `Book.website`
   - **Where:** components/schemas/Book/properties/website
   - **Source:** Format default for `uri`
   - **Why:** Same as above. Format-implied default provides a valid URI.
     Ref: https://spec.openapis.org/oas/v3.0.3#schema-object
   - **Impact:** Mock servers return a valid URL instead of a random string.

4. Added `example: "2024-01-15T09:30:00Z"` to `NewBook.published_at`
   - **Where:** components/schemas/NewBook/properties/published_at
   - **Source:** Format default for `date-time`
   - **Why:** Same as above.

Note: Fields without a format and without a context source (id, title, author, isbn,
price) were skipped — no example was added because no domain-accurate value is
available.

#### B. Error Responses (4 changes)

1. Added `ErrorResponse` schema to components/schemas
   - **Where:** components/schemas/ErrorResponse
   - **Why:** The spec had no error schema. A minimal shared schema `{code: integer,
     message: string}` was created for use by error responses.
     Ref: https://spec.openapis.org/oas/v3.0.3#components-object
   - **Impact:** All error responses use a consistent structure.

2. Added shared `NotFound` response to components/responses
   - **Where:** components/responses/NotFound
   - **Why:** Both `GET /books/{bookId}` and `DELETE /books/{bookId}` need a 404
     response with the same shape. A shared response eliminates duplication.
     Ref: https://spec.openapis.org/oas/v3.0.3#components-object
   - **Impact:** DRY error responses via `$ref`.

3. Added `404` response on `getBook` and `deleteBook`
   - **Where:** paths//books/{bookId}/get/responses/404,
     paths//books/{bookId}/delete/responses/404
   - **Why:** GET and DELETE on a `/{param}` path should document the not-found case.
     Ref: https://spec.openapis.org/oas/v3.0.3#responses-object
   - **Impact:** Mimikos can serve 404 scenarios via `X-Mimikos-Status`. API contract
     is complete for consumers.

4. Added `422` response on `createBook`
   - **Where:** paths//books/post/responses/422
   - **Why:** POST with a request body should document the validation error case.
     Ref: https://spec.openapis.org/oas/v3.0.3#responses-object
   - **Impact:** Mimikos can serve validation error scenarios. Consumers know what
     error shape to expect.

#### C. Request Body Hygiene (1 change)

1. Added `required: true` on `createBook` requestBody
   - **Where:** paths//books/post/requestBody
   - **Why:** Per OAS 3.0.3, `requestBody.required` defaults to `false` when omitted.
     For a POST create operation, the body is required.
     Ref: https://spec.openapis.org/oas/v3.0.3#request-body-object
   - **Impact:** Validators and code generators correctly enforce the required body.

#### D. Format Annotations (3 changes)

1. Added `format: date-time` to `Book.published_at` and `NewBook.published_at`
   - **Where:** components/schemas/Book/properties/published_at,
     components/schemas/NewBook/properties/published_at
   - **Why:** Field name `*_at` unambiguously implies `date-time` format.
     Ref: https://spec.openapis.org/oas/v3.0.3#data-types
   - **Impact:** Code generators produce `DateTime` types. Mock servers generate
     valid timestamps.

2. Added `format: email` to `Book.author_email`
   - **Where:** components/schemas/Book/properties/author_email
   - **Why:** Field name `*_email` unambiguously implies `email` format.
     Ref: https://spec.openapis.org/oas/v3.0.3#data-types
   - **Impact:** Code generators produce email-typed fields. Mock servers generate
     valid email addresses.

3. Added `format: uri` to `Book.website`
   - **Where:** components/schemas/Book/properties/website
   - **Why:** Field name `website` unambiguously implies `uri` format.
     Ref: https://spec.openapis.org/oas/v3.0.3#data-types
   - **Impact:** Code generators produce URI-typed fields. Mock servers generate
     valid URLs.

### Flagged Observations

These are outside the scope of automated enrichment. Review manually.

1. **`NewBook` schema has no `required` array**
   - **Where:** components/schemas/NewBook
   - **Why flagged:** Deciding which fields are mandatory is a data model decision
     that depends on business rules the skill cannot infer. When omitted, no
     properties are required, which may cause consumers to submit incomplete requests.
   - **Ref:** https://spec.openapis.org/oas/v3.0.3#schema-object
   - This requires human review — it is outside the scope of automated enrichment.

2. **`Book` schema has no `required` array**
   - **Where:** components/schemas/Book
   - **Why flagged:** Same as above. Only the spec author knows which fields are
     truly mandatory on the read model.
   - **Ref:** https://spec.openapis.org/oas/v3.0.3#schema-object
   - This requires human review — it is outside the scope of automated enrichment.
```

---

## Example 2: Bookstore API — With Source Code

The same spec as Example 1, but this time the user has source code with test fixtures.
The skill reads the codebase for richer enhancements. The contrast with Example 1 shows
why context gathering matters.

### Before (user's spec)

Same spec as Example 1 (repeated here for completeness).

```yaml
openapi: "3.0.3"
info:
  title: Bookstore API
  version: "1.0.0"
  description: A simple bookstore management API for browsing and purchasing books.

servers:
  - url: http://localhost:3000

paths:
  /books:
    get:
      operationId: listBooks
      summary: List all books
      responses:
        "200":
          description: A list of books
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/Book"
    post:
      operationId: createBook
      summary: Add a new book
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/NewBook"
      responses:
        "201":
          description: Book created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"

  /books/{bookId}:
    get:
      operationId: getBook
      summary: Get a book by ID
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: A single book
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"
    delete:
      operationId: deleteBook
      summary: Delete a book
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "204":
          description: Book deleted

components:
  schemas:
    Book:
      type: object
      properties:
        id:
          type: string
        title:
          type: string
        author:
          type: string
        isbn:
          type: string
        price:
          type: number
        published_at:
          type: string
        author_email:
          type: string
        website:
          type: string

    NewBook:
      type: object
      properties:
        title:
          type: string
        author:
          type: string
        isbn:
          type: string
        price:
          type: number
        published_at:
          type: string
```

### Simulated Project Structure

The spec lives in a Go project with the following relevant files:

```
bookstore-api/
  go.mod
  openapi.yaml            <-- the spec above
  handlers/
    books_test.go
  testdata/
    fixtures/
      books.json
```

**`handlers/books_test.go`** (excerpt)

```go
func TestCreateBook(t *testing.T) {
    body := `{
        "title": "The Pragmatic Programmer",
        "author": "David Thomas",
        "isbn": "978-0-13-468599-1",
        "price": 49.99,
        "published_at": "2019-09-23T00:00:00Z"
    }`
    // ...
}
```

**`testdata/fixtures/books.json`**

```json
[
  {
    "id": "a3bb189e-8bf9-3888-9912-ace4e6543002",
    "title": "The Pragmatic Programmer",
    "author": "David Thomas",
    "isbn": "978-0-13-468599-1",
    "price": 49.99,
    "published_at": "2019-09-23T00:00:00Z",
    "author_email": "dave@pragprog.com",
    "website": "https://pragprog.com/titles/tpp20"
  }
]
```

### Context Gathering

```
AI:    Before I analyze the spec, I'd like to gather context to produce better
       enhancements. A few questions:

       1. Is this spec part of its API project? (i.e., is the source code for
          this API in the same repo?) If yes, where are the tests and any
          fixture/seed data?
       2. Do you have sample API responses (from a running instance, Postman
          collection, or saved files) I can reference?

       If you'd rather skip this and let me work from the spec alone, just say
       so — I'll proceed with what the spec provides.

Human: Yes, this spec is for the bookstore API. Tests are in handlers/ and
       fixtures are in testdata/fixtures/.

AI:    I found source code context in your project. Let me search for values
       matching the spec's schemas.

       Files examined:
       - handlers/books_test.go    — Test fixtures with example values
       - testdata/fixtures/books.json — JSON fixture data

       I found these example values from your project:

       - Book.id: "a3bb189e-8bf9-3888-9912-ace4e6543002"
         (from testdata/fixtures/books.json:3)
       - Book.title: "The Pragmatic Programmer"
         (from handlers/books_test.go:3)
       - Book.author: "David Thomas"
         (from handlers/books_test.go:4)
       - Book.isbn: "978-0-13-468599-1"
         (from testdata/fixtures/books.json:5)
       - Book.price: 49.99
         (from testdata/fixtures/books.json:6)
       - Book.author_email: "dave@pragprog.com"
         (from testdata/fixtures/books.json:8)
       - Book.website: "https://pragprog.com/titles/tpp20"
         (from testdata/fixtures/books.json:9)
       - Book.published_at: "2019-09-23T00:00:00Z"
         (from handlers/books_test.go:6)

       I'll use these as examples in the spec. Do any of these look wrong, or
       are there other sources I should check?

Human: Those look correct. Go ahead.

AI:    Context inventory compiled. Proceeding with analysis.

       Context tier: spec + source code
       Domain: bookstore management (from info.description)
       OAS version: 3.0.3

       Confirmed context-sourced example values: 8
       Fields with no context source: 0
```

### After (enhanced spec)

```yaml
openapi: "3.0.3"
info:
  title: Bookstore API
  version: "1.0.0"
  description: A simple bookstore management API for browsing and purchasing books.

servers:
  - url: http://localhost:3000

paths:
  /books:
    get:
      operationId: listBooks
      summary: List all books
      responses:
        "200":
          description: A list of books
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/Book"
    post:
      operationId: createBook
      summary: Add a new book
      requestBody:
        required: true                                            # Added (C)
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/NewBook"
      responses:
        "201":
          description: Book created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"
        "422":                                                    # Added (B)
          description: Validation error
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ErrorResponse"

  /books/{bookId}:
    get:
      operationId: getBook
      summary: Get a book by ID
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: A single book
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Book"
              example:                                             # Added (A)
                id: "a3bb189e-8bf9-3888-9912-ace4e6543002"
                title: "The Pragmatic Programmer"
                author: "David Thomas"
                isbn: "978-0-13-468599-1"
                price: 49.99
                published_at: "2019-09-23T00:00:00Z"
                author_email: "dave@pragprog.com"
                website: "https://pragprog.com/titles/tpp20"
        "404":                                                    # Added (B)
          $ref: "#/components/responses/NotFound"
    delete:
      operationId: deleteBook
      summary: Delete a book
      parameters:
        - name: bookId
          in: path
          required: true
          schema:
            type: string
      responses:
        "204":
          description: Book deleted
        "404":                                                    # Added (B)
          $ref: "#/components/responses/NotFound"

components:
  responses:                                                      # Added (B)
    NotFound:
      description: Resource not found
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/ErrorResponse"

  schemas:
    Book:
      type: object
      properties:
        id:
          type: string
          example: "a3bb189e-8bf9-3888-9912-ace4e6543002"         # Added (A)
        title:
          type: string
          example: "The Pragmatic Programmer"                     # Added (A)
        author:
          type: string
          example: "David Thomas"                                 # Added (A)
        isbn:
          type: string
          example: "978-0-13-468599-1"                            # Added (A)
        price:
          type: number
          example: 49.99                                          # Added (A)
        published_at:
          type: string
          format: date-time                                       # Added (D)
          example: "2019-09-23T00:00:00Z"                         # Added (A)
        author_email:
          type: string
          format: email                                           # Added (D)
          example: "dave@pragprog.com"                            # Added (A)
        website:
          type: string
          format: uri                                             # Added (D)
          example: "https://pragprog.com/titles/tpp20"            # Added (A)

    NewBook:
      type: object
      properties:
        title:
          type: string
          example: "The Pragmatic Programmer"                     # Added (A)
        author:
          type: string
          example: "David Thomas"                                 # Added (A)
        isbn:
          type: string
          example: "978-0-13-468599-1"                            # Added (A)
        price:
          type: number
          example: 49.99                                          # Added (A)
        published_at:
          type: string
          format: date-time                                       # Added (D)
          example: "2019-09-23T00:00:00Z"                         # Added (A)

    ErrorResponse:                                                # Added (B)
      type: object
      required:
        - code
        - message
      properties:
        code:
          type: integer
        message:
          type: string
```

### Enhancement Summary

```markdown
## Enhancement Summary — Bookstore API

### Context

**Tier:** Spec + source code
**OAS version:** 3.0.3
**Domain:** Bookstore management (from info.description)

**Sources used:**
- `handlers/books_test.go` — 4 field values extracted (title, author, isbn, price)
- `testdata/fixtures/books.json` — 8 field values extracted (id, title, author, isbn,
  price, published_at, author_email, website)

**Fields with context-sourced values:** 8
**Fields with format-implied defaults only:** 0
**Fields with no example source (skipped):** 0

### Applied Changes

#### A. Examples (14 changes)

Property-level examples added to all 8 Book fields and all 5 NewBook fields, sourced
from test fixtures:

1. Added `example: "a3bb189e-8bf9-3888-9912-ace4e6543002"` to `Book.id`
   - **Source:** `testdata/fixtures/books.json:3`
   - **Why:** Context-sourced value from test fixture provides a domain-accurate example.
     Ref: https://spec.openapis.org/oas/v3.0.3#schema-object

2. Added `example: "The Pragmatic Programmer"` to `Book.title`
   - **Source:** `handlers/books_test.go:3`
   - **Why:** Same as above.

3. Added `example: "David Thomas"` to `Book.author`
   - **Source:** `handlers/books_test.go:4`

4. Added `example: "978-0-13-468599-1"` to `Book.isbn`
   - **Source:** `testdata/fixtures/books.json:5`

5. Added `example: 49.99` to `Book.price`
   - **Source:** `testdata/fixtures/books.json:6`

6. Added `example: "2019-09-23T00:00:00Z"` to `Book.published_at`
   - **Source:** `handlers/books_test.go:6` (context-sourced, overrides format default)

7. Added `example: "dave@pragprog.com"` to `Book.author_email`
   - **Source:** `testdata/fixtures/books.json:8` (context-sourced, overrides format default)

8. Added `example: "https://pragprog.com/titles/tpp20"` to `Book.website`
   - **Source:** `testdata/fixtures/books.json:9` (context-sourced, overrides format default)

9-13. NewBook fields (title, author, isbn, price, published_at) received the same
   context-sourced values as their Book counterparts.

14. Added media-type `example` on `GET /books/{bookId}` 200 response
    - **Where:** paths//books/{bookId}/get/responses/200/content/application/json
    - **Source:** Assembled from property-level examples (all fields had context sources)
    - **Why:** A complete media-type example provides a ready-to-use response sample for
      documentation, mock servers, and code generators.
      Ref: https://spec.openapis.org/oas/v3.0.3#media-type-object
    - **Impact:** Mimikos returns a complete, domain-accurate response for this endpoint.

#### B. Error Responses (4 changes)

Same as Example 1:
- Added `ErrorResponse` schema (components/schemas/ErrorResponse)
- Added shared `NotFound` response (components/responses/NotFound)
- Added `404` on `getBook` and `deleteBook`
- Added `422` on `createBook`

#### C. Request Body Hygiene (1 change)

Same as Example 1:
- Added `required: true` on `createBook` requestBody

#### D. Format Annotations (3 changes)

Same as Example 1:
- Added `format: date-time` to `published_at` (Book and NewBook)
- Added `format: email` to `author_email` (Book)
- Added `format: uri` to `website` (Book)

### Flagged Observations

Same as Example 1:
1. `NewBook` schema has no `required` array
2. `Book` schema has no `required` array

### Spec-Only vs. Source Code Comparison

| What changed                  | Example 1 (spec only)              | Example 2 (spec + source)                  |
|-------------------------------|------------------------------------|--------------------------------------------|
| Property examples on Book     | 3 (format-implied defaults only)   | 8 (all fields, context-sourced)            |
| Property examples on NewBook  | 1 (published_at format default)    | 5 (all fields, context-sourced)            |
| Media-type response example   | None (too few property examples)   | Yes, on GET /books/{bookId} 200            |
| Example quality               | Generic placeholders               | Domain-accurate values from test fixtures  |
| Error responses               | Same                               | Same                                       |
| Format annotations            | Same                               | Same                                       |
| Request body hygiene          | Same                               | Same                                       |

The mechanical enrichments (B, C, D) are identical. The difference is entirely in
examples (A): source code context unlocked domain-accurate values for every field,
including fields like `title`, `author`, `isbn`, and `price` that have no format and
would receive no example in the spec-only tier.
```
