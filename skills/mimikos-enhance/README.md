# Mimikos Spec Enhancement Skill

An AI agent skill that enhances OpenAPI specs to improve quality for human readers,
documentation tools, code generators, and mock servers like
[Mimikos](https://github.com/mimikos-io/mimikos).

The skill gathers context from your project's source code and tests before making
changes, producing domain-accurate examples instead of generic placeholders. Every
change cites the OpenAPI Specification standard for educational value.

## What It Does

1. Reads your OpenAPI spec and detects the OAS version
2. Asks about your project environment (source code, tests, sample responses)
3. Searches test fixtures and source code for realistic example values
4. Confirms findings with you before proceeding
5. Applies safe, mechanical enrichments to the spec
6. Produces an enhancement summary citing the OAS standard for each change

## Enhancement Categories

| Category               | What It Adds                                     | Example                                       |
|------------------------|--------------------------------------------------|-----------------------------------------------|
| **A. Examples**        | Property-level and media-type examples           | `example: "user@example.com"` on email fields |
| **B. Error Responses** | 404, 422 responses + shared error schemas        | `$ref: "#/components/responses/NotFound"`     |
| **C. Request Body**    | `required: true` on POST/PUT requestBodies       | Signals that the body is mandatory            |
| **D. Formats**         | `format` on typed fields (email, uri, date-time) | `format: date-time` on `*_at` fields          |

## What It Does NOT Do

- Change existing values or reorder content
- Restructure schemas (no extracting `$ref`, no splitting models)
- Invent domain-specific examples without a project source
- Modify schema `required` arrays (flags these as observations instead)
- Make data model or architectural decisions

## Files

| File          | Purpose                                               |
|---------------|-------------------------------------------------------|
| `SKILL.md`    | Main skill instructions — the agent reads this        |
| `examples.md` | Two before/after examples (spec-only vs. source code) |
| `README.md`   | This file — setup instructions for humans             |

## Skill Files

Download the skill directory from the Mimikos repository:

- [`SKILL.md`](https://github.com/mimikos-io/mimikos/blob/main/skills/mimikos-enhance/SKILL.md)
  — main skill instructions (required)
- [`examples.md`](https://github.com/mimikos-io/mimikos/blob/main/skills/mimikos-enhance/examples.md)
  — two before/after examples showing different context tiers (recommended)

## Setup: Claude Code

Copy the `mimikos-enhance/` directory into your project's skills directory:

```
.claude/skills/mimikos-enhance/
├── SKILL.md
└── examples.md
```

Then in Claude Code, invoke it with:

```
/mimikos-enhance
```

Or reference it naturally in conversation — Claude Code will use the skill when you ask
it to enhance, improve, or enrich your spec.

## Setup: Cursor

Cursor uses `.cursor/rules/` with `.mdc` files. Concatenate `SKILL.md` and
`examples.md` into a single rules file:

```
.cursor/rules/mimikos-enhance.mdc
```

## Prerequisites

- An OpenAPI 3.x spec file (`.yaml`, `.yml`, or `.json`)
- Optionally: source code with tests and fixture data in the same project

## Usage

Once the skill is set up, ask your agent:

- "Enhance my OpenAPI spec"
- "Improve my spec for Mimikos"
- "Add examples and error responses to my spec"
- "Review my spec for completeness"

The agent will gather context, apply enrichments, and produce a summary with OAS
standard references for every change.
