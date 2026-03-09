import { existsSync } from "node:fs";
import path from "node:path";
import { resolve, join, basename, extname } from "node:path";
import { Project, type SourceFile } from "ts-morph";
import { traceImports, DEFAULT_MAX_DEPTH } from "./tracer.js";
import type { ScanNodeRef, ScanNode, ScanEdge, ScanWarning } from "./types.js";

const INDEX_FILE_RE = /(?:^|[/\\])index\.(ts|tsx|js|jsx)$/;

function filePathToModuleId(absPath: string, projectRoot: string): string {
  let rel = path.relative(projectRoot, absPath);
  rel = rel.replace(/\\/g, "/"); // Windows compat
  rel = rel.replace(/\.(ts|tsx|js|jsx)$/, ""); // strip extension
  return `module:${rel}`;
}

function isIgnored(relativePath: string, ignorePaths: string[]): boolean {
  return ignorePaths.some(
    (ip) => relativePath === ip || relativePath.startsWith(ip + "/"),
  );
}

export interface ProcessRoutesResult {
  nodes: ScanNode[];
  edges: ScanEdge[];
  warnings: ScanWarning[];
  filesScanned: number;
}

/**
 * Find handler files that import a given contract file and contain
 * .handler() or .func() calls — indicating they implement the contract.
 */
export function findHandlerFiles(contractFile: SourceFile, project: Project): SourceFile[] {
  const contractPath = contractFile.getFilePath();
  // Strip extension for path-based fallback matching (handles Node16 moduleResolution
  // where ts-morph can't resolve imports without .js extensions)
  const contractPathNoExt = contractPath.replace(/\.[^.]+$/, "");
  const handlers: SourceFile[] = [];

  for (const sf of project.getSourceFiles()) {
    if (sf === contractFile) continue;

    // Check if this file imports the contract
    let importsContract = false;
    for (const imp of sf.getImportDeclarations()) {
      // Try ts-morph resolution first
      const resolved = imp.getModuleSpecifierSourceFile();
      if (resolved && resolved.getFilePath() === contractPath) {
        importsContract = true;
        break;
      }

      // Fallback: resolve the module specifier path manually
      if (!resolved) {
        const specifier = imp.getModuleSpecifierValue();
        if (specifier.startsWith(".")) {
          const sfDir = sf.getDirectoryPath();
          const resolvedPath = resolve(sfDir, specifier);
          const resolvedNoExt = resolvedPath.replace(/\.[^.]+$/, "");
          if (resolvedPath === contractPath || resolvedPath === contractPathNoExt || resolvedNoExt === contractPathNoExt) {
            importsContract = true;
            break;
          }
        }
      }
    }

    if (!importsContract) continue;

    // Check if the file contains .handler() or .func() calls
    const text = sf.getFullText();
    if (text.includes(".handler(") || text.includes(".func(")) {
      handlers.push(sf);
    }
  }

  // Fallback: name-based matching for cross-package contracts (e.g., oRPC)
  // When contracts live in a separate npm package, import resolution can't
  // bridge the package boundary. Match by filename convention instead:
  // contract "break.ts" → handler "break.handlers.ts" or "break.handler.ts"
  if (handlers.length === 0) {
    const contractBaseName = basename(contractPath, extname(contractPath));
    if (contractBaseName !== "index") {
      for (const sf of project.getSourceFiles()) {
        if (sf === contractFile) continue;
        const sfName = basename(sf.getFilePath());
        const matchesName =
          sfName.startsWith(contractBaseName + ".handler") ||
          sfName.startsWith(contractBaseName + ".Handler");
        if (!matchesName) continue;

        const text = sf.getFullText();
        if (text.includes(".handler(") || text.includes(".func(")) {
          handlers.push(sf);
        }
      }
    }
  }

  return handlers;
}

export function processRoutes(
  routeNodes: Map<string, ScanNodeRef>,
  entityByName: Map<string, string>,
  projectRoot: string,
  options: { maxDepth?: number; tsConfigPath?: string; tsconfig?: string } = {},
  ignorePaths: string[] = [],
): ProcessRoutesResult {
  const edges: ScanEdge[] = [];
  const warnings: ScanWarning[] = [];
  let filesScanned = 0;
  const maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;

  // Module node and delegates_to edge tracking
  const seenModules = new Set<string>();
  const moduleNodes: ScanNode[] = [];
  const delegatesEdges: ScanEdge[] = [];

  // --- Create ts-morph Project ---
  const rawTsConfig = options.tsConfigPath ?? options.tsconfig;
  const tsConfigPath = rawTsConfig ? resolve(projectRoot, rawTsConfig) : join(projectRoot, "tsconfig.json");
  const hasTsConfig = existsSync(tsConfigPath);

  // Use Bundler moduleResolution (100) for reliable import resolution.
  // Node16/NodeNext resolution requires .js extensions on relative imports,
  // which prevents ts-morph from resolving .ts source files.
  const project = hasTsConfig
    ? new Project({
        tsConfigFilePath: tsConfigPath,
        skipAddingFilesFromTsConfig: true,
        compilerOptions: { noEmit: true, moduleResolution: 100 /* ModuleResolutionKind.Bundler */ },
      })
    : new Project({
        compilerOptions: {
          noEmit: true,
          module: 99 /* ModuleKind.ESNext */,
          moduleResolution: 100 /* ModuleResolutionKind.Bundler */,
          target: 99 /* ScriptTarget.ESNext */,
          esModuleInterop: true,
        },
      });

  // --- Shared cache across all route traces ---
  const fileEntityCache = new Map<string, { entities: Set<string>; remainingDepth: number }>();

  // --- Deduplicate tracker ---
  const seenEdges = new Set<string>();

  // --- Track which routes yielded edges (for reverse resolution) ---
  const routesWithEdges = new Set<string>();

  // --- First pass: normal tracing ---
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

      for (const [entityNodeId, { tracePath, depth }] of traceResult.entities) {
        const edgeKey = `${routeNodeId}-touches_entity-${entityNodeId}`;
        if (seenEdges.has(edgeKey)) {
          continue;
        }
        seenEdges.add(edgeKey);
        routesWithEdges.add(routeNodeId);

        // Summary touches_entity edge (backward compat)
        edges.push({
          id: `edge:${edgeKey}`,
          srcId: routeNodeId,
          dstId: entityNodeId,
          kind: "touches_entity",
          properties: { tracePath: tracePath.map(p => path.relative(projectRoot, p)), depth },
        });

        // Emit module nodes and delegates_to chain
        // Filter tracePath entries: skip [0] (route handler), index files, ignored paths
        const chainFiles: string[] = [];
        for (let i = 1; i < tracePath.length; i++) {
          const absFile = tracePath[i];
          if (INDEX_FILE_RE.test(absFile)) continue;
          const relFile = path.relative(projectRoot, absFile).replace(/\\/g, "/");
          if (isIgnored(relFile, ignorePaths)) continue;
          chainFiles.push(absFile);
        }

        // Create module nodes for each chain file
        for (const absFile of chainFiles) {
          const moduleId = filePathToModuleId(absFile, projectRoot);
          if (!seenModules.has(moduleId)) {
            seenModules.add(moduleId);
            const relFile = path.relative(projectRoot, absFile).replace(/\\/g, "/");
            moduleNodes.push({
              id: moduleId,
              kind: "module",
              name: moduleId.replace(/^module:/, ""),
              label: moduleId.replace(/^module:/, ""),
              source: "scan",
              sourceFile: relFile,
            });
          }
        }

        // Build delegates_to chain: route -> module1 -> module2 -> ... -> entity
        let prevId = routeNodeId;
        for (const absFile of chainFiles) {
          const moduleId = filePathToModuleId(absFile, projectRoot);
          const delegateKey = `${prevId}-delegates_to-${moduleId}`;
          if (!seenEdges.has(delegateKey)) {
            seenEdges.add(delegateKey);
            delegatesEdges.push({
              id: `edge:${delegateKey}`,
              srcId: prevId,
              dstId: moduleId,
              kind: "delegates_to",
            });
          }
          prevId = moduleId;
        }

        // Last module -> entity touches_entity edge
        if (chainFiles.length > 0) {
          const lastModuleId = filePathToModuleId(chainFiles[chainFiles.length - 1], projectRoot);
          const touchKey = `${lastModuleId}-touches_entity-${entityNodeId}`;
          if (!seenEdges.has(touchKey)) {
            seenEdges.add(touchKey);
            delegatesEdges.push({
              id: `edge:${touchKey}`,
              srcId: lastModuleId,
              dstId: entityNodeId,
              kind: "touches_entity",
            });
          }
        }
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

  // --- Second pass: reverse-import resolution for unresolved routes ---
  const unresolvedRoutes = [...routeNodes.entries()]
    .filter(([id]) => !routesWithEdges.has(id))
    .filter(([, node]) => node.sourceFile);

  // Deduplicate by sourceFile — resolve each contract once, apply to all its routes
  const unresolvedByFile = new Map<string, Array<[string, ScanNodeRef]>>();
  for (const [id, node] of unresolvedRoutes) {
    const key = node.sourceFile!;
    if (!unresolvedByFile.has(key)) {
      unresolvedByFile.set(key, []);
    }
    unresolvedByFile.get(key)!.push([id, node]);
  }

  if (unresolvedRoutes.length > 0) {
    // Load all project files from tsconfig for reverse-import search
    // This is lazy — only done when there are unresolved routes
    if (hasTsConfig) {
      try {
        project.addSourceFilesFromTsConfig(tsConfigPath);
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        warnings.push({
          file: tsConfigPath,
          message: `Failed to load project files for reverse resolution: ${msg}`,
          severity: "warning",
        });
      }
    }

    for (const [sourceFile, routeGroup] of unresolvedByFile) {
      try {
        const absPath = resolve(projectRoot, sourceFile);
        const contractFile = project.getSourceFile(absPath);
        if (!contractFile) {
          continue;
        }

        const handlerFiles = findHandlerFiles(contractFile, project);

        if (handlerFiles.length === 0) {
          for (const [_routeNodeId] of routeGroup) {
            warnings.push({
              file: sourceFile,
              message: `No handler files found for contract (no files import it with .handler()/.func())`,
              severity: "info",
            });
          }
          continue;
        }

        // Trace from each handler file, then create edges for all routes sharing this contract
        for (const handlerFile of handlerFiles) {
          const traceResult = traceImports(handlerFile, entityByName, { maxDepth }, fileEntityCache);
          filesScanned += traceResult.filesVisited;

          // Apply discovered entities to ALL routes that share this contract
          for (const [routeNodeId] of routeGroup) {
            for (const [entityNodeId, { tracePath, depth }] of traceResult.entities) {
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
                properties: {
                  tracePath: tracePath.map(p => path.relative(projectRoot, p)),
                  depth,
                  resolvedVia: "reverse-import",
                },
              });
            }
          }
        }
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        warnings.push({
          file: sourceFile,
          message: `Error during reverse resolution: ${msg}`,
          severity: "error",
        });
      }
    }
  }

  return { nodes: moduleNodes, edges: [...edges, ...delegatesEdges], warnings, filesScanned };
}
