import { Node, SyntaxKind, type SourceFile } from "ts-morph";
import { findPrismaAccesses } from "./matcher.js";

export interface TraceResult {
  /**
   * entityNodeId → { tracePath, depth } for the first-discovered path (DFS order).
   * Uses first-discovered-wins semantics: if the same entity is reachable through
   * multiple import paths, the path visited first by DFS is recorded. This is NOT
   * necessarily the shortest path — DFS visit order depends on import declaration
   * order in each source file.
   */
  entities: Map<string, { tracePath: string[]; depth: number }>;
  /** total number of unique files visited */
  filesVisited: number;
}

export interface TraceOptions {
  maxDepth: number;
}

export const DEFAULT_MAX_DEPTH = 8;

const TEST_FILE_RE = /(?:\.test\.ts|\.spec\.ts|__tests__\/)/;

/**
 * For a re-export barrel: resolves requested bindings to their original source files.
 * For a factory barrel: traces through factory function to find the import source.
 * Returns source files to trace, or null if resolution fails (fallback to full trace).
 */
function resolveBindingsToSourceFiles(
  file: SourceFile,
  bindings: string[],
): SourceFile[] | null {
  try {
    const exportDecls = file.getExportedDeclarations();
    const targetFiles: SourceFile[] = [];
    const seen = new Set<string>();

    for (const bindingName of bindings) {
      const declarations = exportDecls.get(bindingName);
      if (!declarations || declarations.length === 0) continue;

      const decl = declarations[0];
      const declSourceFile = decl.getSourceFile();

      if (declSourceFile.getFilePath() !== file.getFilePath()) {
        // Re-export: declaration lives in a different file — follow it directly
        const targetPath = declSourceFile.getFilePath();
        if (!seen.has(targetPath)) {
          seen.add(targetPath);
          targetFiles.push(declSourceFile);
        }
      } else {
        // Local declaration — try factory barrel resolution
        const factoryTarget = resolveFactoryBinding(file, bindingName);
        if (factoryTarget && !seen.has(factoryTarget.getFilePath())) {
          seen.add(factoryTarget.getFilePath());
          targetFiles.push(factoryTarget);
        }
      }
    }

    return targetFiles.length > 0 ? targetFiles : null;
  } catch {
    return null;
  }
}

/**
 * Resolves a destructured export binding through a factory function.
 * Pattern: export const { userRepository, ... } = buildRepos(prisma)
 * where buildRepos returns { userRepository: createUserRepo(prisma), ... }
 * and createUserRepo is imported from './user.repo'.
 */
function resolveFactoryBinding(
  file: SourceFile,
  bindingName: string,
): SourceFile | null {
  try {
    for (const stmt of file.getVariableStatements()) {
      for (const decl of stmt.getDeclarationList().getDeclarations()) {
        const nameNode = decl.getNameNode();
        if (nameNode.getKind() !== SyntaxKind.ObjectBindingPattern) continue;

        // Check if this binding pattern contains our target
        const elements = nameNode.asKindOrThrow(SyntaxKind.ObjectBindingPattern).getElements();
        const hasBinding = elements.some(
          (el) => el.getNameNode().getText() === bindingName,
        );
        if (!hasBinding) continue;

        // Resolve the initializer (e.g., _repos → buildRepos(prisma))
        let initExpr = decl.getInitializer();
        if (!initExpr) continue;

        if (Node.isIdentifier(initExpr)) {
          const symbol = initExpr.getSymbol();
          if (symbol) {
            for (const d of symbol.getDeclarations()) {
              if (Node.isVariableDeclaration(d) && d.getInitializer()) {
                initExpr = d.getInitializer()!;
                break;
              }
            }
          }
        }

        // Extract CallExpression — handle ternary (cond ? a() : b()) by trying both branches
        const callCandidates: import("ts-morph").CallExpression[] = [];
        if (Node.isCallExpression(initExpr)) {
          callCandidates.push(initExpr);
        } else if (Node.isConditionalExpression(initExpr)) {
          const whenTrue = initExpr.getWhenTrue();
          const whenFalse = initExpr.getWhenFalse();
          if (Node.isCallExpression(whenTrue)) callCandidates.push(whenTrue);
          if (Node.isCallExpression(whenFalse)) callCandidates.push(whenFalse);
        }
        if (callCandidates.length === 0) continue;

        for (const callExpr of callCandidates) {
          const callee = callExpr.getExpression();
          if (!Node.isIdentifier(callee)) continue;

          const calleeSymbol = callee.getSymbol();
          if (!calleeSymbol) continue;

          // Find the function definition
          for (const funcDecl of calleeSymbol.getDeclarations()) {
            if (!Node.isFunctionDeclaration(funcDecl)) continue;

            // Find return statement with object literal
            const returnStmts = funcDecl.getDescendantsOfKind(SyntaxKind.ReturnStatement);
            for (const ret of returnStmts) {
              const retExpr = ret.getExpression();
              if (!retExpr || !Node.isObjectLiteralExpression(retExpr)) continue;

              const prop = retExpr.getProperty(bindingName);
              if (!prop || !Node.isPropertyAssignment(prop)) continue;

              const propInit = prop.getInitializer();
              if (!propInit || !Node.isCallExpression(propInit)) continue;

              const propCallee = propInit.getExpression();
              if (!Node.isIdentifier(propCallee)) continue;

              // Trace back to the import
              const propSymbol = propCallee.getSymbol();
              if (!propSymbol) continue;

              for (const pd of propSymbol.getDeclarations()) {
                // Named import: import { createFoo } from "./foo"
                if (Node.isImportSpecifier(pd)) {
                  const importDecl = pd.getImportDeclaration();
                  const sourceFile = importDecl.getModuleSpecifierSourceFile();
                  if (sourceFile) return sourceFile;
                }
                // Default import: import createFoo from "./foo"
                if (Node.isImportClause(pd)) {
                  const importDecl = pd.getParent();
                  if (Node.isImportDeclaration(importDecl)) {
                    const sourceFile = importDecl.getModuleSpecifierSourceFile();
                    if (sourceFile) return sourceFile;
                  }
                }
              }
            }
          }
        }
      }
    }
    return null;
  } catch {
    return null;
  }
}

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
  const entities = new Map<string, { tracePath: string[]; depth: number }>();

  function traceFile(
    file: SourceFile,
    depth: number,
    currentPath: string[],
    requestedBindings?: string[] | null,
  ): Set<string> {
    const filePath = file.getFilePath();

    const remainingDepth = maxDepth - depth;

    if (depth > maxDepth) {
      return new Set();
    }

    // Already visited in this trace — return cached entities if available.
    // The visited set prevents infinite recursion for circular imports,
    // but we must still return known entities for files that were already
    // fully explored earlier in this trace (e.g., shared dependencies
    // reached through multiple import paths).
    if (visited.has(filePath)) {
      const cached = fileEntityCache.get(filePath);
      if (cached) {
        for (const entityId of cached.entities) {
          if (!entities.has(entityId)) {
            entities.set(entityId, { tracePath: [...currentPath, filePath], depth });
          }
        }
        return cached.entities;
      }
      return new Set();
    }

    if (TEST_FILE_RE.test(filePath)) {
      return new Set();
    }

    const pathWithFile = [...currentPath, filePath];

    // Check cache — stores TRANSITIVE entity IDs (direct + from imports)
    // Only use cache if the cached result was explored with at least as much depth budget
    // Skip cache for binding-filtered traces (cache has full results, not per-binding)
    if (!requestedBindings) {
      const cached = fileEntityCache.get(filePath);
      if (cached && remainingDepth <= cached.remainingDepth) {
        for (const entityId of cached.entities) {
          if (!entities.has(entityId)) {
            entities.set(entityId, { tracePath: pathWithFile, depth });
          }
        }
        visited.add(filePath);
        return cached.entities;
      }
    }

    visited.add(filePath);

    const allEntities = new Set<string>();

    // Scan for prisma.modelName.method() calls via matcher
    const matchedEntityIds = findPrismaAccesses(file, entityByName);

    for (const entityId of matchedEntityIds) {
      allEntities.add(entityId);
      if (!entities.has(entityId)) {
        entities.set(entityId, { tracePath: pathWithFile, depth });
      }
    }

    // Binding-aware resolution: if we know which bindings were requested,
    // resolve them to specific source files instead of following all imports
    if (requestedBindings && requestedBindings.length > 0) {
      const resolvedTargets = resolveBindingsToSourceFiles(file, requestedBindings);
      if (resolvedTargets && resolvedTargets.length > 0) {
        for (const target of resolvedTargets) {
          const childEntities = traceFile(target, depth + 1, pathWithFile);
          for (const entityId of childEntities) allEntities.add(entityId);
        }
        // Don't cache binding-filtered results — partial set would poison future full traces
        return allEntities;
      }
      // Resolution failed — fall through to full trace
    }

    // Trace into imported files — collect their transitive entities
    const importDeclarations = file.getImportDeclarations();
    for (const imp of importDeclarations) {
      const specifier = imp.getModuleSpecifierValue();

      // Skip node_modules imports (not relative paths)
      if (!specifier.startsWith(".") && !specifier.startsWith("..")) {
        continue;
      }

      // Extract named bindings to pass as context to the target file
      const namedImports = imp.getNamedImports();
      const namespaceImport = imp.getNamespaceImport();
      const defaultImport = imp.getDefaultImport();

      let childBindings: string[] | null = null;
      if (namedImports.length > 0 && !namespaceImport && !defaultImport) {
        // Use the EXPORTED name (before "as"), not the local alias
        childBindings = namedImports.map((ni) => {
          const compilerNode = ni.compilerNode as any;
          return compilerNode.propertyName?.text ?? ni.getName();
        });
      }

      const resolved = imp.getModuleSpecifierSourceFile();
      if (resolved) {
        const childEntities = traceFile(resolved, depth + 1, pathWithFile, childBindings);
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

