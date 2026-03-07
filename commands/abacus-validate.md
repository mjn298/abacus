---
description: Run full pipeline validation of Abacus against a real project
argument-hint: "[project-path]"
---

Walk through a complete end-to-end validation of the Abacus pipeline against a real project. This runs each step sequentially, reports pass/fail for each, and produces a final summary.

## Steps

Parse the optional argument as the target project path. If not provided, use the current working directory.

Run each of the following validation steps. For each step, report PASS or FAIL with details. Continue through all steps even if some fail.

### Step 1: Verify abacus is on PATH
Run `which abacus`. PASS if found, FAIL if not (suggest `make install` from the abacus repo).

### Step 2: Change to target project directory
Verify the project path exists and cd to it. PASS if the directory exists, FAIL if not.

### Step 3: Initialize Abacus
Check if `.abacus/config.yaml` exists. If not, run `abacus init --json`. PASS if `.abacus/config.yaml` exists after this step, FAIL if initialization failed.

### Step 4: Configure scanners
Read `.abacus/config.yaml` and check for configured scanners. Auto-detect which scanners should apply by checking for:
- `package.json` with express/fastify/koa/hono dependencies -> suggest route scanner
- `prisma/` directory -> suggest prisma entity scanner
- `pages/` or `app/` directory with Next.js -> suggest nextjs page scanner
- `*.feature` files -> suggest gherkin action scanner

PASS if at least one scanner is configured. FAIL if no scanners configured (list what was detected and suggest adding them).

### Step 5: Run scan
Run `abacus scan --json --db .abacus/abacus.db`. PASS if the command succeeds and returns nodes > 0. FAIL if it errors or returns 0 nodes.

### Step 6: Check stats
Run `abacus stats --json --db .abacus/abacus.db`. Report counts by kind (routes, entities, pages, actions). PASS if total nodes > 0. Report against expected minimums:
- Routes: 10+
- Entities: 5+
- Pages: 5+
- Total nodes: 30+

Note which minimums are met and which are not (not meeting minimums is a WARN, not FAIL).

### Step 7: Sample queries
Run these commands and verify they return results:
```
abacus routes --json --db .abacus/abacus.db --limit 5
abacus entities --json --db .abacus/abacus.db --limit 5
```
PASS if both return valid JSON with results. FAIL if either errors.

### Step 8: Graph exploration
From the routes query in Step 7, take the ID of the first route and run:
```
abacus graph <first-route-id> --depth 2 --json --db .abacus/abacus.db
```
PASS if the command returns a subgraph with at least 1 node. FAIL if it errors or returns empty. Skip if no routes were found in Step 7.

### Step 9: Action creation
Create a test action:
```
abacus actions create "test-user-login" --label "User logs in" --gherkin "the user logs in" --json --db .abacus/abacus.db
```
PASS if the action is created successfully. FAIL if it errors.

### Step 10: Action matching
Run the match command:
```
abacus match "the user logs in" --json --db .abacus/abacus.db
```
PASS if it returns a match for the action created in Step 9. FAIL if no match found or it errors.

### Step 11: MCP server startup
Start the MCP server and immediately kill it to verify it can start:
```
timeout 3 abacus mcp-server --db .abacus/abacus.db || true
```
PASS if the process starts without immediate crash (exit code 0 or timeout/SIGTERM is acceptable). FAIL if it crashes with an error on startup.

### Step 12: Cleanup
Delete the test action created in Step 9 if a delete command is available. If not, note that the test action `test-user-login` remains in the database.

## Final Report

After all steps, present a summary table:

```
Step                    Status
----                    ------
1. abacus on PATH       PASS/FAIL
2. Project directory    PASS/FAIL
3. Initialization       PASS/FAIL
4. Scanner config       PASS/FAIL
5. Scan                 PASS/FAIL
6. Stats                PASS/FAIL/WARN
7. Sample queries       PASS/FAIL
8. Graph exploration    PASS/FAIL/SKIP
9. Action creation      PASS/FAIL
10. Action matching     PASS/FAIL
11. MCP server          PASS/FAIL
12. Cleanup             PASS/SKIP
```

Report the overall result: ALL PASS, or list which steps failed with remediation suggestions.
