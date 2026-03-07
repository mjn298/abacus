---
description: Check Gherkin step coverage against the application graph
argument-hint: "[glob-pattern]"
---

Check how well Gherkin feature file steps are covered by actions registered in the Abacus application graph.

## Steps

1. Parse the arguments:
   - Optional argument: a glob pattern for feature files. Defaults to `**/*.feature` if not provided.

2. Verify `abacus` is on PATH. If not found, tell the user to install it with `make install` from the abacus repo.

3. Run the coverage command:
   ```
   abacus coverage "<glob-pattern>" --json --db .abacus/abacus.db
   ```

4. Parse the JSON output and present:

   **Overall Coverage Summary:**
   - Total steps found across all feature files
   - Number of covered steps (matched to an action)
   - Number of uncovered steps
   - Coverage percentage

   **Per-File Breakdown:**
   - For each feature file, show: file path, total steps, covered count, uncovered count, coverage percentage

   **Uncovered Steps:**
   - List all uncovered step texts, grouped by feature file
   - For each uncovered step, show the exact Gherkin text (Given/When/Then)

5. If coverage is below 100%, suggest next steps:
   - For each uncovered step, suggest creating an action:
     ```
     abacus actions create "<step-id>" --label "<step text>" --gherkin "<step text>" --json
     ```
   - Mention that `/abacus-new-scanner` can scaffold a scanner to auto-discover actions from code
   - Suggest re-running coverage after adding actions to verify improvement

6. If coverage is 100%, congratulate the user and note that all Gherkin steps have matching actions in the graph.
