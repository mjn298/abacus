---
description: Implement a feature from a Gherkin step or narrative description using the application graph
argument-hint: '"step text" | narrative description of feature'
---

Implement a feature end-to-end by classifying a Gherkin step against the application graph, discovering implementation patterns from established entities, and building new code that follows existing conventions.

**Abacus is the MAP. You are the DRIVER.** This skill uses the graph to find exemplar source files, reads them to learn the project's conventions, then implements new code following those patterns exactly.

## Input Parsing

Parse the argument to determine the input type:

- **Gherkin step** (quoted string starting with Given/When/Then/And, or any quoted string): Treat as a ready-to-classify step. Example: `/implement-step "the user archives a contest"`
- **Narrative description** (unquoted natural language): Convert to Gherkin first. Example: `/implement-step users should be able to export contest results as CSV`

### Narrative → Gherkin Conversion

If the input is narrative (not a quoted Gherkin step):

1. Ask clarifying questions to understand the feature:
   - What is the user trying to do?
   - What entity does this operate on?
   - What's the expected outcome?

2. Propose 1-3 Gherkin steps that capture the behavior. Use standard Given/When/Then:
   ```
   Given the user is on the contest page
   When the user archives the contest
   Then the contest status is "archived"
   ```

3. Confirm with the user: "These are the steps I'll implement. Adjust or proceed?"

4. Process each confirmed step through the Classification Pipeline below.

## Classification Pipeline

For each Gherkin step:

### Step 1: Classify

Call `abacus.genesis` with `{ "step": "<step text>" }`.

If MCP tools are unavailable, fall back to CLI:
```bash
abacus genesis "<step text>" --json
```

### Step 2: Branch on Classification

---

#### COVERED (classification = "covered")

The step is already implemented.

1. Report the existing wiring:
   - Action name and gherkin_patterns
   - Connected routes, entities, modules
   - Delegation chains (ordered paths from route to entity)

2. Ask: "This step is already implemented. Would you like to modify the existing implementation, or work on a different step?"

3. If modifying: read the source files from the delegation chains and proceed as a modification task rather than new implementation.

**Stop here unless the user wants modifications.**

---

#### WIRABLE (classification = "wirable")

Infrastructure exists but no action wires the step to it.

1. Report the matching infrastructure from `routes`, `entities`, `modules`, `pages`.

2. Show which routes and entities will be wired.

3. Create the action to wire the step:
   - First, call `abacus.match_step` with `{ "step_text": "<step>" }` as an idempotency guard.
   - If no exact match, call `abacus.create_action` with:
     - `name`: derive from step text (e.g., "archive-contest" from "the user archives a contest")
     - `gherkin_patterns`: the step text (and reasonable variants)
     - `route_refs`: IDs of matching routes
     - `entity_refs`: IDs of matching entities
     - `page_refs`: IDs of matching pages (if any)

4. Report: "Step is now wired to existing infrastructure. The route handlers and entity logic already exist — no new code needed."

**Stop here — wirable steps need no new code.**

---

#### NEW (classification = "new")

No matching infrastructure exists. This requires implementing new code.

**Phase A: Find the Exemplar**

1. Extract the entity noun from the step text. You are an LLM — extracting "contest" from "the user archives a contest" is trivial. Do it yourself, do not call any tool.

2. Call `abacus.genesis` with `{ "entity": "<entity>" }` to find the entity and its implementation pattern.

3. If the entity is found and has `delegation_chains`: proceed to Phase B with this exemplar.

4. If the entity has no delegation chains, or was not found:
   - The entity may not exist yet. Find a *different* entity to use as a pattern template.
   - Call `abacus.genesis` with `{ "entity": "User" }` or another well-known entity.
   - Pick the first result with non-empty `delegation_chains`.
   - If nothing works, report: "No exemplar found. The graph may not have enough data — try running `/abacus-scan` first."

**Phase B: Learn the Pattern**

1. Pick the best exemplar delegation chain:
   - Prefer the longest chain that contains the entity name in its intermediate nodes (e.g., `contest.handlers`, `contest.service`).
   - If no entity-specific chain exists, use the longest chain overall — even if it starts from a generic route like `/health`, the intermediate modules reveal the project's layering conventions.

2. Read EVERY source file in the chain, in order from outermost to innermost:
   - Route handler file (the entry point)
   - Service/middleware files (business logic layer)
   - Repository/data-access files (persistence layer)
   - Entity/model file (schema definition)

   Use the `source_file` field from each node in the chain.

3. As you read each file, note:
   - **Naming conventions**: how files, functions, classes, and variables are named
   - **File structure**: imports, exports, class vs function style
   - **Error handling**: how errors are created, caught, and propagated
   - **Patterns**: dependency injection, middleware chains, validation, response formatting
   - **Testing**: if test files exist alongside source files, read those too

4. Present the pattern to the user:
   ```
   ## Implementation Pattern (from <Entity> exemplar)

   I've read the following files to learn your project's conventions:

   1. <route file> — Route handler pattern
   2. <service file> — Service layer pattern
   3. <repository file> — Data access pattern
   4. <entity file> — Entity definition

   Key conventions I found:
   - [List 3-5 specific conventions observed]

   I'll replicate this pattern for <new entity/feature>.
   ```

5. Confirm with the user before implementing: "Ready to implement following this pattern?"

**Phase C: Implement**

1. For each layer in the delegation chain (adapting the exemplar pattern):

   a. **Entity/Model** (if new entity needed):
      - Create the entity definition following the exemplar's schema conventions
      - Write tests for the model

   b. **Repository/Data Access**:
      - Create the repository following the exemplar's data access patterns
      - Write tests with the same testing patterns observed

   c. **Service/Business Logic**:
      - Create the service following the exemplar's service patterns
      - Wire dependencies the same way the exemplar does
      - Write tests

   d. **Route Handler**:
      - Create the route following the exemplar's routing patterns
      - Register the route the same way existing routes are registered
      - Write integration tests

   Follow TDD throughout: write each test first (RED), then implement (GREEN).

2. After implementation, wire the step to the new infrastructure:
   - Call `abacus.create_action` with:
     - `name`: derived from step text
     - `gherkin_patterns`: the step text
     - `route_refs`: IDs of the new routes (use the same ID format as existing routes)
     - `entity_refs`: IDs of the entities involved

3. Suggest updating the graph:
   - "Run `/abacus-scan` to update the application graph with the new routes and modules."

## Summary Format

After completing work on all steps, present:

```
## Implementation Summary

### Steps Processed
| Step | Classification | Action Taken |
|------|---------------|--------------|
| <step text> | COVERED | Already implemented |
| <step text> | WIRABLE | Wired to existing routes |
| <step text> | NEW | Implemented following <Entity> pattern |

### Files Created/Modified
- <list of files>

### Next Steps
- Run `/abacus-scan` to update the graph
- Run tests: <project test command>
- Review the implementation for completeness
```
