---
description: Classify Gherkin steps and discover implementation patterns via the application graph
argument-hint: '"step text" | path/to/file.feature | --entity EntityName'
---

Classify Gherkin steps as COVERED, WIRABLE, or NEW using the Abacus application graph, and discover implementation patterns from established entities.

**Abacus is the MAP, not the DRIVER.** This skill queries the graph and points you to source files — it does not generate code. You read the files it surfaces and replicate the patterns you find.

## Input Modes

Parse the argument to determine the mode:

- **Step text** (quoted string): Classify a single Gherkin step. Example: `/abacus-genesis "the user creates a notification"`
- **Feature file** (path ending in `.feature`): Classify all steps in the file. Example: `/abacus-genesis features/login.feature`
- **Entity pattern** (`--entity Name`): Show the full implementation shape of an entity. Example: `/abacus-genesis --entity User`

## Steps

### Mode A: Single Step Classification

1. Call `abacus.genesis` with `{ "step": "<the step text>" }`.

2. Classify the result based on the `classification` and `match_tier` fields:

   **COVERED** (classification = `covered`, match_tier = `exact`):
   - An action already exists that matches this step.
   - Report: the `action.name`, its `action.gherkin_patterns`, and the returned `routes`, `entities`, `modules`, `pages`.
   - Show `delegation_chains` — each chain is an ordered path from route to entity.
   - No further work needed — this step is fully mapped.

   **WIRABLE** (classification = `wirable`):
   - No action exists, but the graph contains matching infrastructure.
   - Report: the suggested infrastructure from `routes`, `entities`, `modules`, `pages`.
   - Recommend creating an action to wire the step to existing infrastructure:
     - Use `abacus.create_action` with the appropriate fields (name, label, gherkin_patterns, route_refs, entity_refs, page_refs).
     - **Important**: First call `abacus.match_step` to verify no action was created since the last check (idempotency guard).

   **NEW** (classification = `new`):
   - No matching infrastructure exists in the graph.
   - Report: this step requires new code — proceed to Pattern Discovery (Step 3).

   **Fuzzy** (match_tier = `fuzzy`):
   - Similar actions exist but none match well enough.
   - Report this as informational. Suggest calling `abacus.match_step` directly with `{ "step_text": "<step>" }` if the user wants fuzzy candidate details and scores.
   - Fall through to the classification-based logic above (the `classification` field still tells you covered/wirable/new).

3. **Pattern Discovery** (only for NEW steps where `delegation_chains` is empty):

   The goal is to find an established entity with a rich delegation chain and present its source files as a template.

   a. Extract the entity hint from the step text. You are an LLM — extracting "notification" from "the user creates a notification" is trivial. Do it yourself, do not call any tool.

   b. Call `abacus.genesis` with `{ "entity": "<entity hint>" }` to search for a similar entity and its full context.

   c. If the result has `delegation_chains`, you have your exemplar — skip to step (e).

   d. If no delegation chains are found, try a well-known entity:
      - Call `abacus.genesis` with `{ "entity": "User" }` or another common entity name.
      - Pick the first result that has `delegation_chains`.

   e. Present the **Pattern Template**:
      - Show each `delegation_chain` as an ordered list of files from outermost (route handler) to innermost (entity/model).
      - For each node in the chain, list: the node ID (which encodes kind) and its `source_file`.
      - Also list the `routes`, `modules`, `entities` from the response with their `source_file` values.
      - Tell the agent: "Read these files in order to understand the implementation pattern. Then replicate this pattern for your new entity."

4. Present a summary:
   ```
   ## Classification: [COVERED | WIRABLE | NEW]

   **Step:** "<step text>"
   **Match Tier:** <exact | fuzzy | suggest>

   [Details based on classification...]
   ```

### Mode B: Feature File Classification

1. Read the feature file at the provided path.

2. Parse the Gherkin manually — you are an LLM, this is straightforward:
   - Extract all `Given`, `When`, `Then`, `And`, `But` step lines.
   - For `Scenario Outline` steps with `<placeholder>`, replace placeholders with `{string}` for matching.
   - `Background` steps apply to all scenarios — classify them once.
   - Ignore comments (lines starting with `#`), tags (lines starting with `@`), and blank lines.

3. For each unique step text, call `abacus.genesis` with `{ "step": "<step text>" }` and classify per Mode A above.

4. Present a summary table:

   ```
   ## Feature: <feature name>

   | Step | Classification | Details |
   |------|---------------|---------|
   | Given the user is logged in | COVERED | Action: user-login |
   | When the user creates a post | WIRABLE | Routes: POST /posts, Entity: Post |
   | Then a notification is sent | NEW | No infrastructure found |

   ### Summary
   - COVERED: N steps (already mapped)
   - WIRABLE: N steps (infrastructure exists, needs Action)
   - NEW: N steps (requires new code)

   ### Pattern Discovery (for NEW steps)
   [Pattern template from the exemplar entity...]
   ```

5. For WIRABLE steps, offer to create actions in bulk:
   - List each WIRABLE step with its suggested action.
   - Ask: "Create actions for these WIRABLE steps? (y/n)"
   - If yes, call `abacus.create_action` for each, checking `abacus.match_step` first for idempotency.

### Mode C: Entity Pattern

1. Call `abacus.genesis` with `{ "entity": "<entity name>" }`.

2. If the result has no entities, report it and suggest running `/abacus-scan` first to populate the graph.

3. Present the full implementation shape from the response:

   ```
   ## Entity: <name>

   ### Delegation Chains
   For each delegation chain in the response, show the full path:

   Route: POST /users (source: src/routes/users.ts)
     → Module: src/services/user.service.ts
       → Module: src/repositories/user.repository.ts
         → Entity: User (source: prisma/schema.prisma)

   ### Source Files to Read
   1. src/routes/users.ts (route handler)
   2. src/services/user.service.ts (business logic)
   3. src/repositories/user.repository.ts (data access)
   4. prisma/schema.prisma (entity definition)

   ### Connected Nodes
   - Routes: [from response.routes]
   - Modules: [from response.modules]
   - Pages: [from response.pages]
   - Entities: [from response.entities]
   ```

4. Suggest next steps:
   - "Read these source files to understand how this entity is implemented"
   - "Use this as a template when implementing similar entities"
   - "Run `/abacus-genesis \"<step text>\"` to classify a specific step"
