---
description: Query the Abacus application graph for routes, entities, pages, or actions
argument-hint: "<kind> [search-term]"
---

Query the Abacus application graph. The first argument specifies the kind of node to query (routes, entities, pages, or actions). An optional second argument provides a search term for FTS5 full-text search.

## Steps

1. Parse the arguments:
   - First argument (required): the kind — must be one of `routes`, `entities`, `pages`, or `actions`. If missing or invalid, show the valid kinds and stop.
   - Remaining arguments (optional): join them as the search term for `--match`.

2. Verify `abacus` is on PATH. If not found, tell the user to install it with `make install` from the abacus repo.

3. Build and run the command:
   - Base: `abacus <kind> --json --db .abacus/abacus.db --limit 50`
   - If a search term was provided, add `--match "<search-term>"`

   Examples:
   - `abacus routes --json --db .abacus/abacus.db --limit 50`
   - `abacus entities --json --db .abacus/abacus.db --limit 50 --match "User"`
   - `abacus actions --json --db .abacus/abacus.db --limit 50 --match "login"`

4. Parse the JSON output and present results in a structured format:
   - For routes: show method, path, handler, source file
   - For entities: show name, type/kind, source file, properties
   - For pages: show name, path/URL, source file
   - For actions: show name, label, gherkin step text

5. Report the total count of results. If the results were truncated at the limit, mention that more results may exist and suggest refining the search term.

6. If no results were found, suggest:
   - Checking the search term spelling
   - Running `/abacus-scan` if the graph might be stale
   - Trying a broader search term
