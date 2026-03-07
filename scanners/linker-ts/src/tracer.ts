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
  fileEntityCache: Map<string, { entities: Set<string>; remainingDepth: number }> = new Map(),
): TraceResult {
  const maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;
  const visited = new Set<string>();
  const entities = new Map<string, string[]>();

  function traceFile(
    file: SourceFile,
    depth: number,
    currentPath: string[],
  ): Set<string> {
    const filePath = file.getFilePath();

    const remainingDepth = maxDepth - depth;

    if (depth > maxDepth || visited.has(filePath)) {
      return new Set();
    }

    if (TEST_FILE_RE.test(filePath)) {
      return new Set();
    }

    const pathWithFile = [...currentPath, filePath];

    // Check cache — stores TRANSITIVE entity IDs (direct + from imports)
    // Only use cache if the cached result was explored with at least as much depth budget
    const cached = fileEntityCache.get(filePath);
    if (cached && remainingDepth <= cached.remainingDepth) {
      for (const entityId of cached.entities) {
        if (!entities.has(entityId)) {
          entities.set(entityId, pathWithFile);
        }
      }
      visited.add(filePath);
      return cached.entities;
    }

    visited.add(filePath);

    const allEntities = new Set<string>();

    // Scan for prisma.modelName.method() calls via matcher
    const matchedEntityIds = findPrismaAccesses(file, entityByName);

    for (const entityId of matchedEntityIds) {
      allEntities.add(entityId);
      if (!entities.has(entityId)) {
        entities.set(entityId, pathWithFile);
      }
    }

    // Trace into imported files — collect their transitive entities
    const importDeclarations = file.getImportDeclarations();
    for (const imp of importDeclarations) {
      const specifier = imp.getModuleSpecifierValue();

      // Skip node_modules imports (not relative paths)
      if (!specifier.startsWith(".") && !specifier.startsWith("..")) {
        continue;
      }

      const resolved = imp.getModuleSpecifierSourceFile();
      if (resolved) {
        const childEntities = traceFile(resolved, depth + 1, pathWithFile);
        for (const entityId of childEntities) {
          allEntities.add(entityId);
        }
      }
    }

    // Cache the FULL transitive set with the depth budget used
    // Overwrite if we explored with a higher remaining depth than what was cached
    fileEntityCache.set(filePath, { entities: allEntities, remainingDepth });
    return allEntities;
  }

  traceFile(sourceFile, 0, []);

  return {
    entities,
    filesVisited: visited.size,
  };
}

