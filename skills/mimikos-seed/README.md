# Mimikos Seeding Skill

An AI agent skill that teaches Claude Code and Cursor agents how to seed a running
[Mimikos](https://github.com/mimikos-io/mimikos) mock server in stateful mode.

Instead of manually constructing curl commands and reading schemas, the agent reads your
OpenAPI spec, constructs valid request bodies, sends POST requests, and verifies the
seeded state — turning a 10-minute manual task into a 30-second automated one.

## What It Does

1. Reads your OpenAPI spec to discover create endpoints and their request schemas
2. Determines dependency order (parent resources before children)
3. Constructs valid JSON request bodies with realistic values
4. Sends POST requests to a running Mimikos instance
5. Extracts generated IDs from responses
6. Optionally updates resources with specific values via PATCH/PUT
7. Verifies the seeded state via GET/LIST endpoints

## Files

| File          | Purpose                                                |
|---------------|--------------------------------------------------------|
| `SKILL.md`    | Main skill instructions — the agent reads this         |
| `examples.md` | Two concrete seeding examples (Petstore + Asana-style) |
| `README.md`   | This file — setup instructions for humans              |

## Skill Files

Download the skill directory from the Mimikos repository:

- [`SKILL.md`](https://github.com/mimikos-io/mimikos/blob/main/skills/mimikos-seed/SKILL.md)
  — main skill instructions (required)
- [`examples.md`](https://github.com/mimikos-io/mimikos/blob/main/skills/mimikos-seed/examples.md)
  — two concrete seeding examples (recommended)

## Setup: Claude Code

Copy the `mimikos-seed/` directory into your project's skills directory:

```
.claude/skills/mimikos-seed/
├── SKILL.md
└── examples.md
```

Then in Claude Code, invoke it with:

```
/mimikos-seed
```

Or reference it naturally in conversation — Claude Code will use the skill when you ask
it to seed, populate, or create test data for Mimikos.

## Setup: Cursor

Cursor uses `.cursor/rules/` with `.mdc` files. Concatenate `SKILL.md` and
`examples.md` into a single rules file:

```
.cursor/rules/mimikos-seed.mdc
```

## Prerequisites

- Mimikos installed (see [Installation](https://github.com/mimikos-io/mimikos#installation))
- Mimikos running in stateful mode: `mimikos start --mode stateful <spec-file>`
- An OpenAPI 3.x spec file accessible to the agent

## Usage

Once the skill is set up, ask your agent:

- "Seed the Mimikos server with test data"
- "Populate the mock API with sample resources"
- "Create 5 pets in the Petstore mock"
- "Set up test data for the running mock server"

The agent will read your spec, construct valid requests, and populate the running
Mimikos instance.
