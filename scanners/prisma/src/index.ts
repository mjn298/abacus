#!/usr/bin/env node

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { parsePrismaSchema, buildScanOutput } from "./parser.js";
import type { ScanInput, ScanOutput } from "./types.js";

async function readStdin(): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) {
    chunks.push(chunk as Buffer);
  }
  return Buffer.concat(chunks).toString("utf-8");
}

async function main(): Promise<void> {
  const startTime = Date.now();

  try {
    const inputJson = await readStdin();
    const input: ScanInput = JSON.parse(inputJson);

    const schemaPath = input.options?.schemaPath ?? "prisma/schema.prisma";
    const fullPath = join(input.projectRoot, schemaPath);

    let schema: string;
    try {
      schema = readFileSync(fullPath, "utf-8");
    } catch (err) {
      const errorOutput: ScanOutput = {
        version: 1,
        scanner: {
          id: "prisma",
          name: "Prisma Entity Scanner",
          version: "0.1.0",
        },
        nodes: [],
        edges: [],
        warnings: [
          {
            file: schemaPath,
            message: `Failed to read schema file: ${(err as Error).message}`,
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
      process.exit(0);
      return;
    }

    const parseResult = parsePrismaSchema(schema);
    const durationMs = Date.now() - startTime;
    const output = buildScanOutput(parseResult, schemaPath, 1, durationMs);

    process.stdout.write(JSON.stringify(output, null, 2));
    process.exit(0);
  } catch (err) {
    const errorOutput: ScanOutput = {
      version: 1,
      scanner: {
        id: "prisma",
        name: "Prisma Entity Scanner",
        version: "0.1.0",
      },
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
