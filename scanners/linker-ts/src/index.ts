#!/usr/bin/env node

import { processRoutes } from "./parser.js";
import type { ScanInput, ScanNodeRef, ScanOutput } from "./types.js";

async function readStdin(): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) {
    chunks.push(chunk as Buffer);
  }
  return Buffer.concat(chunks).toString("utf-8");
}

const SCANNER_INFO = {
  id: "ts-linker",
  name: "TypeScript Linker Scanner",
  version: "0.1.0",
} as const;

async function main(): Promise<void> {
  const startTime = Date.now();

  try {
    const inputJson = await readStdin();
    const input: ScanInput = JSON.parse(inputJson);

    const existingNodes = input.existingNodes ?? [];

    // Build lookup maps from existing nodes
    const routeNodes = new Map<string, ScanNodeRef>();
    const entityByName = new Map<string, string>();

    for (const node of existingNodes) {
      if (node.kind === "route") {
        routeNodes.set(node.id, node);
      } else if (node.kind === "entity") {
        entityByName.set(node.name.toLowerCase(), node.id);
      }
    }

    // If no route nodes, nothing to link
    if (routeNodes.size === 0) {
      const emptyOutput: ScanOutput = {
        version: 1,
        scanner: SCANNER_INFO,
        nodes: [],
        edges: [],
        warnings: [
          {
            file: "unknown",
            message: "No route nodes found in existingNodes — nothing to link",
            severity: "info",
          },
        ],
        stats: {
          filesScanned: 0,
          nodesFound: 0,
          edgesFound: 0,
          errors: 0,
          durationMs: Date.now() - startTime,
        },
      };
      process.stdout.write(JSON.stringify(emptyOutput, null, 2));
      return;
    }

    const result = processRoutes(routeNodes, entityByName, input.projectRoot, input.options);
    const durationMs = Date.now() - startTime;

    const output: ScanOutput = {
      version: 1,
      scanner: SCANNER_INFO,
      nodes: [],
      edges: result.edges,
      warnings: result.warnings,
      stats: {
        filesScanned: result.filesScanned,
        nodesFound: 0,
        edgesFound: result.edges.length,
        errors: result.warnings.filter((w: { severity: string }) => w.severity === "error").length,
        durationMs,
      },
    };

    process.stdout.write(JSON.stringify(output, null, 2));
  } catch (err) {
    const errorOutput: ScanOutput = {
      version: 1,
      scanner: SCANNER_INFO,
      nodes: [],
      edges: [],
      warnings: [
        {
          file: "unknown",
          message: `Scanner error: ${(err as Error).message}`,
          severity: "error",
        },
      ],
      stats: {
        filesScanned: 0,
        nodesFound: 0,
        edgesFound: 0,
        errors: 1,
        durationMs: Date.now() - startTime,
      },
    };
    process.stdout.write(JSON.stringify(errorOutput, null, 2));
    process.exit(1);
  }
}

main();
