#!/usr/bin/env node

import { readFileSync, existsSync } from "node:fs";
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

    const ignorePaths = input.ignorePaths ?? [];
    let schemaPath = input.options?.schemaPath ?? "prisma/schema.prisma";
    let fullPath = join(input.projectRoot, schemaPath);

    // If the default path doesn't exist, try common alternatives
    if (!existsSync(fullPath)) {
      const fallbacks = [
        "backend/prisma/schema.prisma",
        "server/prisma/schema.prisma",
        "api/prisma/schema.prisma",
        "schema.prisma",
      ];
      for (const alt of fallbacks) {
        const altPath = join(input.projectRoot, alt);
        if (existsSync(altPath)) {
          schemaPath = alt;
          fullPath = altPath;
          break;
        }
      }
    }

    // Skip if schema path matches any ignorePath
    if (ignorePaths.some(ip => schemaPath === ip || schemaPath.startsWith(ip + "/"))) {
      const skippedOutput: ScanOutput = {
        version: 1,
        scanner: {
          id: "prisma",
          name: "Prisma Entity Scanner",
          version: "0.1.0",
        },
        nodes: [],
        edges: [],
        warnings: [],
        stats: {
          filesScanned: 0,
          nodesFound: 0,
          edgesFound: 0,
          errors: 0,
          durationMs: Date.now() - startTime,
        },
      };
      process.stdout.write(JSON.stringify(skippedOutput, null, 2));
      return;
    }

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
      return;
    }

    const parseResult = parsePrismaSchema(schema);
    const durationMs = Date.now() - startTime;
    const output = buildScanOutput(parseResult, schemaPath, 1, durationMs);

    process.stdout.write(JSON.stringify(output, null, 2));
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
