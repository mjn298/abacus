# Abacus

A CLI tool and MCP server that builds a **living application graph** from source code using pluggable scanners. Abacus discovers routes, entities, pages, actions, and their relationships, stores them in SQLite with FTS5 full-text search, and exposes them via CLI commands and MCP tools for AI agents.

AI agents (like Claude Code) can query the graph to understand an application's structure -- what API routes exist, what database entities are defined, what pages are rendered, and how they connect.

## How It Works

```
.abacus/config.yaml
        |
        v
  Scanner Runner         Scanners are standalone executables.
        |                 Abacus sends ScanInput JSON on stdin,
        +---> express     reads ScanOutput JSON from stdout.
        +---> prisma
        +---> react-router
        +---> orpc
        |
        v
  SQLite Graph DB         Nodes + edges + FTS5 full-text index
   (.abacus/abacus.db)
        |
        +---> CLI commands (abacus routes, entities, pages, ...)
        +---> MCP server   (11 tools for AI agent integration)
```

The architecture is **BYOASTP** (Bring Your Own AST Parser): Abacus core is 100% Go with no language-specific parsing. Scanners are standalone executables that output JSON conforming to Scanner Protocol V1. Community can write scanners for any language or framework.

## Quick Start

```bash
# Install
make install
# or
go install github.com/mjn/abacus/cmd/abacus@latest

# Initialize in your project
cd your-project
abacus init

# Run scanners
abacus scan

# Query the graph
abacus routes
abacus entities
abacus pages
abacus stats
```

## CLI Commands

All commands support `--json` for machine-readable output, `--db` to override the database path (default: `.abacus/abacus.db`), and `--quiet`/`--verbose` flags.

### `abacus init`

Initialize Abacus for the current project. Creates `.abacus/` directory, auto-detects scanners from `package.json` and `go.mod`, generates `config.yaml`, and initializes the SQLite database.

| Flag | Description |
|------|-------------|
| `--dir` | Project directory to initialize (default: `.`) |
| `--force` | Re-detect scanners and overwrite config |

```bash
abacus init
abacus init --dir /path/to/project --force
```

### `abacus scan [type]`

Run all configured scanners (or a specific one by ID) and ingest discovered nodes and edges into the graph database.

```bash
abacus scan           # Run all scanners
abacus scan express   # Run only the express scanner
abacus scan --json    # JSON output
```

### `abacus routes`

List or search route nodes.

| Flag | Description |
|------|-------------|
| `-m`, `--match` | FTS5 search query |
| `-l`, `--limit` | Maximum results (default: 1000) |

```bash
abacus routes
abacus routes -m "users" -l 10
```

### `abacus entities`

List or search entity nodes. Same flags as `routes`.

```bash
abacus entities
abacus entities -m "User"
```

### `abacus pages`

List or search page nodes. Same flags as `routes`.

```bash
abacus pages
abacus pages -m "dashboard"
```

### `abacus actions`

List or search action nodes.

| Flag | Description |
|------|-------------|
| `-m`, `--match` | FTS5 search query |
| `-l`, `--limit` | Maximum results (default: 50) |

```bash
abacus actions
abacus actions -m "login"
```

### `abacus actions create <name>`

Create a new action node with optional Gherkin patterns and references.

| Flag | Description |
|------|-------------|
| `--label` | Human-readable description |
| `--gherkin` | Cucumber expression patterns (repeatable) |
| `--route-ref` | Route node IDs to link (repeatable) |
| `--entity-ref` | Entity node IDs to link (repeatable) |
| `--page-ref` | Page node IDs to link (repeatable) |

```bash
abacus actions create "login-user" \
  --label "User logs in" \
  --gherkin "the user logs in with {string}" \
  --route-ref "route:POST-/api/auth/login" \
  --entity-ref "entity:User"
```

### `abacus graph <node-id>`

Show the connected subgraph around a node.

| Flag | Description |
|------|-------------|
| `-d`, `--depth` | Maximum traversal depth (default: 2) |

```bash
abacus graph "route:GET-/api/users"
abacus graph "entity:User" -d 3 --json
```

### `abacus match [step-text]`

Match a Gherkin step text against the action graph using 3-tier matching (exact, fuzzy, suggest).

| Flag | Description |
|------|-------------|
| `-f`, `--file` | Feature file to match all steps |
| `-k`, `--keyword` | Step keyword (Given/When/Then) |
| `--create` | Auto-create action from suggestion |
| `--threshold` | Fuzzy match threshold |

```bash
abacus match "the user logs in with \"admin\""
abacus match -f features/login.feature --create
```

### `abacus coverage [glob]`

Show Gherkin step coverage report. Default glob: `**/*.feature`.

```bash
abacus coverage
abacus coverage "features/**/*.feature" --json
```

### `abacus stats`

Show graph statistics (node counts per kind).

```bash
abacus stats
abacus stats --json
```

### `abacus mcp-server`

Start the MCP server on stdio.

| Flag | Description |
|------|-------------|
| `--config` | Path to abacus config file (default: `.abacus/config.yaml`) |

### `abacus version`

Print the version.

## MCP Server

Abacus exposes 11 tools via the [Model Context Protocol](https://modelcontextprotocol.io/) for AI agent integration.

### Configuration

Add to your MCP client config (e.g., Claude Code `settings.json`):

```json
{
  "mcpServers": {
    "abacus": {
      "command": "abacus",
      "args": ["mcp-server"],
      "env": {}
    }
  }
}
```

### Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `abacus.scan` | Run all configured scanners and ingest results | _(none)_ |
| `abacus.query_routes` | Search route nodes | `query?`, `limit?` |
| `abacus.query_entities` | Search entity nodes | `query?`, `limit?` |
| `abacus.query_pages` | Search page nodes | `query?`, `limit?` |
| `abacus.query_actions` | Search action nodes | `query?`, `limit?` |
| `abacus.create_action` | Create action with Gherkin patterns and refs | `name`, `label?`, `gherkin_patterns?`, `route_refs?`, `entity_refs?`, `page_refs?`, `permission_refs?` |
| `abacus.update_action` | Update existing action | `id`, `label?`, `gherkin_patterns?`, `route_refs?`, `entity_refs?`, `page_refs?`, `permission_refs?` |
| `abacus.match_step` | 3-tier Gherkin step matching (exact / fuzzy / suggest) | `step_text` |
| `abacus.match_scenario` | Match all steps in a scenario | `steps[]` (keyword + text) |
| `abacus.graph_context` | Connected subgraph traversal | `node_id`, `max_depth?` |
| `abacus.stats` | Node counts per kind | _(none)_ |

## Scanner Protocol V1

The JSON interface between Abacus and scanners. Scanners are standalone executables that read from stdin and write to stdout.

### ScanInput (sent to scanner stdin)

```json
{
  "version": 1,
  "projectRoot": "/absolute/path/to/project",
  "options": { "routeGlobs": ["**/*.ts"] },
  "ignorePaths": ["node_modules", "dist"]
}
```

### ScanOutput (scanner writes to stdout)

```json
{
  "version": 1,
  "scanner": {
    "id": "express",
    "name": "Express Route Scanner",
    "version": "1.0.0"
  },
  "nodes": [
    {
      "id": "route:GET-/api/users",
      "kind": "route",
      "name": "GET /api/users",
      "label": "List all users",
      "properties": { "method": "GET", "path": "/api/users" },
      "source": "express",
      "sourceFile": "src/routes/users.ts"
    }
  ],
  "edges": [
    {
      "id": "edge:route:GET-/api/users->entity:User",
      "srcId": "route:GET-/api/users",
      "dstId": "entity:User",
      "kind": "uses_route",
      "properties": {}
    }
  ],
  "warnings": [
    { "file": "src/old.ts", "message": "Could not parse route", "severity": "warn" }
  ],
  "stats": {
    "filesScanned": 10,
    "nodesFound": 5,
    "edgesFound": 3,
    "errors": 0,
    "durationMs": 150
  }
}
```

### Node Kinds

| Kind | Description |
|------|-------------|
| `route` | API route / endpoint |
| `entity` | Database model / entity |
| `page` | UI page / view |
| `action` | User action (links routes, entities, pages via Gherkin patterns) |
| `permission` | Authorization permission / role |

### Edge Kinds

| Kind | Description |
|------|-------------|
| `uses_route` | Action uses a route |
| `touches_entity` | Action touches an entity |
| `on_page` | Action occurs on a page |
| `requires_permission` | Node requires a permission |
| `relates_to` | General relationship |
| `field_relation` | Entity field relationship (e.g., foreign key) |

### Node Sources

| Source | Description |
|--------|-------------|
| `scan` | Discovered by a scanner |
| `agent` | Created by an AI agent |
| `manual` | Created manually via CLI |

## Writing a New Scanner

1. Create a standalone executable in any language
2. Read ScanInput JSON from stdin
3. Parse the project's source files
4. Output ScanOutput JSON to stdout
5. Register in `.abacus/config.yaml`

### Minimal Scanner (Node.js)

```typescript
#!/usr/bin/env node
import { readFileSync } from 'fs';

const input = JSON.parse(readFileSync('/dev/stdin', 'utf8'));

const output = {
  version: 1,
  scanner: { id: 'my-scanner', name: 'My Scanner', version: '1.0.0' },
  nodes: [],
  edges: [],
  warnings: [],
  stats: { filesScanned: 0, nodesFound: 0, edgesFound: 0, errors: 0, durationMs: 0 }
};

// Parse files under input.projectRoot, populate output.nodes and output.edges

process.stdout.write(JSON.stringify(output));
```

### Register in Config

```yaml
scanners:
  my-scanner:
    command: node path/to/my-scanner.js
    options:
      customOption: value
```

## Built-in Scanners

Four reference scanners ship in the `scanners/` directory. All are Node.js/TypeScript packages.

| Scanner | Discovers | Details |
|---------|-----------|---------|
| **express** | Express.js routes | `router.get/post/put/delete`, route chaining, middleware detection |
| **prisma** | Prisma models and enums | Model fields, field types, relations between models |
| **react-router** | React Router pages | `<Route>` JSX, nested routes, route protection detection |
| **orpc** | oRPC contract definitions | `.route()` chains with input/output Zod schemas |

Build any scanner:

```bash
cd scanners/express && npm install && npm run build
```

Build all scanners:

```bash
./scanners/build-all.sh
```

## Configuration

Abacus stores its config and database in `.abacus/` at the project root.

### `.abacus/config.yaml`

```yaml
version: 1
project:
  name: my-app
  root: "."
  ignorePaths:
    - node_modules
    - dist
    - build
    - .git
scanners:
  express:
    command: node scanners/express/dist/index.js
    options:
      routeGlobs: ["**/*.ts", "**/*.js"]
  prisma:
    command: node scanners/prisma/dist/index.js
```

The `abacus init` command auto-generates this file by detecting frameworks from `package.json` dependencies.

## Project Structure

```
abacus/
├── cmd/abacus/main.go           # Entry point
├── internal/
│   ├── cli/                     # Cobra command definitions
│   ├── config/                  # Config loading (.abacus/config.yaml)
│   ├── db/                      # SQLite schema, migrations, types
│   ├── graph/                   # GraphRepository (CRUD, FTS5, traversal)
│   ├── match/                   # 3-tier Gherkin step matching
│   └── scanner/                 # Scanner protocol, subprocess runner
├── mcp/                         # MCP server (11 tools)
├── scanners/                    # Reference scanner implementations
│   ├── express/                 # Express.js route scanner
│   ├── prisma/                  # Prisma entity scanner
│   ├── react-router/            # React Router page scanner
│   └── orpc/                    # oRPC contract scanner
└── testdata/                    # Test fixtures
```

## Development

```bash
make build    # Build to bin/abacus
make test     # Run Go tests
make lint     # Run golangci-lint
make install  # Install to $GOPATH/bin
make clean    # Remove build artifacts
```

For scanners:

```bash
cd scanners/express && npm install && npm run build && npm test
```

## License

MIT
