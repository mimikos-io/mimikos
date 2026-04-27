# Mimikos Seeding Skill

An AI agent skill that teaches Claude Code and Cursor agents how to seed a running
[Mimikos](https://github.com/mimikos-io/mimikos) mock server in stateful mode.

Instead of manually constructing curl commands and reading schemas, the agent reads your
OpenAPI spec, constructs valid request bodies, and creates resources via MCP tool calls —
turning a 10-minute manual task into a 30-second automated one.

## What It Does

1. Checks server status via MCP (or discovers a running instance)
2. Reads your OpenAPI spec to discover create endpoints and their request schemas
3. Determines dependency order (parent resources before children)
4. Constructs valid JSON request bodies with realistic values
5. Creates resources via `manage_state` MCP tool calls
6. Extracts generated IDs from responses
7. Verifies the seeded state via list/get operations

## Transport

The skill uses **MCP tool calls** as the primary transport when the Mimikos MCP server
is configured. If MCP is not available, it falls back to **curl** commands against the
running server.

| Operation        | MCP tool                                    | Curl fallback                     |
|------------------|---------------------------------------------|-----------------------------------|
| Check status     | `server_status()`                           | `pgrep -af mimikos`              |
| Start server     | `start_server(specPath, mode: "stateful")`  | `mimikos start --mode stateful`  |
| List endpoints   | `list_endpoints()`                          | Read startup banner              |
| Create resource  | `manage_state(action: "create", ...)`       | `curl -X POST ...`              |
| Verify state     | `manage_state(action: "list", ...)`         | `curl -s http://...`            |
| Reset state      | `manage_state(action: "reset")`             | Restart server                   |

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

For best results, also configure the Mimikos MCP server:

```bash
claude mcp add mimikos -- mimikos mcp
```

Restart Claude Code to pick up the new server.

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

Also configure the MCP server in `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "mimikos": {
      "type": "stdio",
      "command": "mimikos",
      "args": ["mcp"]
    }
  }
}
```

## Prerequisites

- Mimikos installed (see [Installation](https://github.com/mimikos-io/mimikos#installation))
- An OpenAPI 3.x spec file accessible to the agent
- (Recommended) Mimikos MCP server configured for your editor

## Usage

Once the skill is set up, ask your agent:

- "Seed the Mimikos server with test data"
- "Populate the mock API with sample resources"
- "Create 5 pets in the Petstore mock"
- "Set up test data for the running mock server"

The agent will read your spec, construct valid requests, and populate the running
Mimikos instance.
