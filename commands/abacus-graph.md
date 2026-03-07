---
description: Explore the connected subgraph around a node in the application graph
argument-hint: "<node-id> [depth]"
---

Explore the connected subgraph around a specific node in the Abacus application graph. Shows all nodes and edges reachable within the specified depth.

## Steps

1. Parse the arguments:
   - First argument (required): the node ID. If missing, tell the user they need to provide a node ID. Suggest running `/abacus-query routes` or `/abacus-query entities` to find node IDs.
   - Second argument (optional): traversal depth, defaults to 2. Must be a positive integer.

2. Verify `abacus` is on PATH. If not found, tell the user to install it with `make install` from the abacus repo.

3. Run the graph exploration command:
   ```
   abacus graph <node-id> --depth <depth> --json --db .abacus/abacus.db
   ```

4. Parse the JSON output and present:
   - The center node (the queried node) with its kind, name, and properties
   - Connected nodes grouped by kind (routes, entities, pages, actions)
   - Edges showing relationships between nodes (edge kind, source, destination)

5. Present the subgraph as a readable structure. For each connected node, show:
   - Node ID, kind, name
   - How it connects to the center node (edge kind and direction)
   - Distance from center (depth level)

6. Suggest next steps:
   - Pick a connected node ID and run `/abacus-graph <id>` to explore further
   - Increase depth if the subgraph seems incomplete
   - Use `/abacus-query` to search for specific nodes by name
