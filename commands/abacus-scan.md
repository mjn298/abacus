---
description: Scan a codebase with Abacus to build the application graph
argument-hint: "[scanner-id]"
---

Run an Abacus scan against the current project to discover routes, entities, pages, actions, and their relationships.

## Steps

1. Verify `abacus` is on PATH by running `which abacus`. If not found, tell the user to install it with `make install` from the abacus repo at /Users/mjn/workplace/abacus.

2. Check that `.abacus/config.yaml` exists in the current working directory. If it does not exist, tell the user to run `abacus init` first to initialize the project, then stop.

3. Read `.abacus/config.yaml` and check that at least one scanner is configured under the `scanners:` key. If no scanners are configured, tell the user they need to add scanner entries to `.abacus/config.yaml` and show them the expected format:
   ```yaml
   scanners:
     scanner-name:
       command: npx ts-node scanners/scanner-name/src/index.ts
       options: {}
   ```

4. Run the scan command:
   - If the user provided a scanner ID argument: `abacus scan $ARGUMENT --json --db .abacus/abacus.db`
   - If no argument: `abacus scan --json --db .abacus/abacus.db`

5. Parse the JSON output and report:
   - Total nodes ingested (broken down by kind: routes, entities, pages, actions)
   - Total edges created
   - Number of warnings (list them if fewer than 10)
   - Number of errors

6. If there are errors, show the scanner-specific error details from the JSON output so the user can diagnose the problem.

7. If the scan succeeded with nodes > 0, suggest next steps:
   - `abacus routes --json` to see discovered routes
   - `abacus entities --json` to see discovered entities
   - `/abacus-query` slash command for interactive querying
