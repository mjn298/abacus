import { existsSync } from "node:fs";
import { resolve, join } from "node:path";
import { Project } from "ts-morph";
import { traceImports } from "./tracer.js";
import type { ScanNodeRef, ScanEdge, ScanWarning } from "./types.js";

const DEFAULT_MAX_DEPTH = 8;

export interface ProcessRoutesResult {
  edges: ScanEdge[];
  warnings: ScanWarning[];
  filesScanned: number;
}

export function processRoutes(
  routeNodes: Map<string, ScanNodeRef>,
  entityByName: Map<string, string>,
  projectRoot: string,
  options: { maxDepth?: number; tsConfigPath?: string } = {},
): ProcessRoutesResult {
  const edges: ScanEdge[] = [];
  const warnings: ScanWarning[] = [];
  let filesScanned = 0;
  const maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;

  // --- Create ts-morph Project ---
  const tsConfigPath = options.tsConfigPath ?? join(projectRoot, "tsconfig.json");
  const hasTsConfig = existsSync(tsConfigPath);

  const project = hasTsConfig
    ? new Project({
        tsConfigFilePath: tsConfigPath,
        skipAddingFilesFromTsConfig: true,
        compilerOptions: { noEmit: true },
      })
    : new Project({
        compilerOptions: {
          noEmit: true,
          module: 99 /* ModuleKind.ESNext */,
          moduleResolution: 100 /* ModuleResolutionKind.Node16 */,
          target: 99 /* ScriptTarget.ESNext */,
          esModuleInterop: true,
        },
      });

  // --- Shared cache across all route traces ---
  const fileEntityCache = new Map<string, { entities: Set<string>; remainingDepth: number }>();

  // --- Deduplicate tracker ---
  const seenEdges = new Set<string>();

  // --- Process each route ---
  for (const [routeNodeId, routeNode] of routeNodes) {
    if (!routeNode.sourceFile) {
      continue;
    }

    try {
      const absPath = resolve(projectRoot, routeNode.sourceFile);

      let sourceFile = project.getSourceFile(absPath);
      if (!sourceFile) {
        try {
          sourceFile = project.addSourceFileAtPath(absPath);
        } catch (err: unknown) {
          const msg = err instanceof Error ? err.message : String(err);
          warnings.push({
            file: routeNode.sourceFile,
            message: `Failed to add source file: ${msg}`,
            severity: "warning",
          });
          continue;
        }
      }

      const traceResult = traceImports(sourceFile, entityByName, { maxDepth }, fileEntityCache);
      filesScanned += traceResult.filesVisited;

      for (const [entityNodeId, tracePath] of traceResult.entities) {
        const edgeKey = `${routeNodeId}-touches_entity-${entityNodeId}`;
        if (seenEdges.has(edgeKey)) {
          continue;
        }
        seenEdges.add(edgeKey);

        edges.push({
          id: `edge:${edgeKey}`,
          srcId: routeNodeId,
          dstId: entityNodeId,
          kind: "touches_entity",
          properties: { tracePath },
        });
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      warnings.push({
        file: routeNode.sourceFile ?? routeNodeId,
        message: `Error processing route: ${msg}`,
        severity: "error",
      });
    }
  }

  return { edges, warnings, filesScanned };
}
