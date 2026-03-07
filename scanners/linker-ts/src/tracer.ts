import type { SourceFile } from "ts-morph";
import { findPrismaAccesses } from "./matcher.js";

export interface TraceResult {
  /** entityNodeId → array of filenames traversed to reach it */
  entities: Map<string, string[]>;
  /** total number of unique files visited */
  filesVisited: number;
}

export interface TraceOptions {
  maxDepth: number;
}

const DEFAULT_MAX_DEPTH = 8;

const TEST_FILE_RE = /(?:\.test\.ts|\.spec\.ts|__tests__\/)/;

/**
 * Traces import chains from a route handler's source file to discover
 * which entities (Prisma models) it touches.
 */
export function traceImports(
  sourceFile: SourceFile,
  entityByName: Map<string, string>,
  options: Partial<TraceOptions> = {},
  fileEntityCache: Map<string, Set<string>> = new Map(),
): TraceResult {
  const maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;
  const visited = new Set<string>();
  const entities = new Map<string, string[]>();

  function traceFile(
    file: SourceFile,
    depth: number,
    currentPath: string[],
  ): void {
    const filePath = file.getFilePath();

    if (depth > maxDepth || visited.has(filePath)) {
      return;
    }

    if (TEST_FILE_RE.test(filePath)) {
      return;
    }

    visited.add(filePath);

    const pathWithFile = [...currentPath, filePath];

    // Check cache first — cache stores entity node IDs directly
    const cached = fileEntityCache.get(filePath);
    if (cached) {
      for (const entityId of cached) {
        if (!entities.has(entityId)) {
          entities.set(entityId, pathWithFile);
        }
      }
      return;
    }

    const discoveredEntities = new Set<string>();

    // Scan for prisma.modelName.method() calls via matcher
    const matchedEntityIds = findPrismaAccesses(file, entityByName);

    for (const entityId of matchedEntityIds) {
      discoveredEntities.add(entityId);
      if (!entities.has(entityId)) {
        entities.set(entityId, pathWithFile);
      }
    }

    // Cache discovered entity IDs for this file
    fileEntityCache.set(filePath, discoveredEntities);

    // Trace into imported files
    const importDeclarations = file.getImportDeclarations();
    for (const imp of importDeclarations) {
      const specifier = imp.getModuleSpecifierValue();

      // Skip node_modules imports (not relative paths)
      if (!specifier.startsWith(".") && !specifier.startsWith("..")) {
        continue;
      }

      const resolved = imp.getModuleSpecifierSourceFile();
      if (resolved) {
        traceFile(resolved, depth + 1, pathWithFile);
      }
    }
  }

  traceFile(sourceFile, 0, []);

  return {
    entities,
    filesVisited: visited.size,
  };
}

