#!/usr/bin/env bash
# echo-scanner.sh — A test fixture scanner that reads ScanInput from stdin
# and outputs a valid ScanOutput with a few hardcoded nodes and edges.
set -euo pipefail

# Read stdin (ScanInput JSON) — we don't use it, but consume it.
INPUT=$(cat)

cat <<'EOF'
{
  "version": 1,
  "scanner": {
    "id": "echo-scanner",
    "name": "Echo Scanner",
    "version": "0.1.0"
  },
  "nodes": [
    {
      "id": "route:/api/users",
      "kind": "route",
      "name": "/api/users",
      "label": "GET /api/users",
      "properties": {"method": "GET"},
      "source": "scan",
      "sourceFile": "routes.ts"
    },
    {
      "id": "entity:User",
      "kind": "entity",
      "name": "User",
      "label": "User Entity",
      "properties": {},
      "source": "scan",
      "sourceFile": "models/user.ts"
    }
  ],
  "edges": [
    {
      "id": "edge:route-user",
      "srcId": "route:/api/users",
      "dstId": "entity:User",
      "kind": "touches_entity"
    }
  ],
  "warnings": [],
  "stats": {
    "filesScanned": 5,
    "nodesFound": 2,
    "edgesFound": 1,
    "errors": 0,
    "durationMs": 42
  }
}
EOF
