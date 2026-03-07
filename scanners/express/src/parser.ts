import {
  Project,
  SyntaxKind,
  type SourceFile,
  type CallExpression,
} from 'ts-morph';
import { join, relative } from 'path';
import { globSync } from 'fs';
import type { ScanOutput, ScanNode, ScanWarning } from './types.js';

/**
 * Derive a router name from a relative file path by stripping common prefixes
 * and file extensions. E.g. "backend/routes/users.routes.ts" → "users"
 */
function deriveRouterName(relPath: string): string {
  let name = relPath;
  // Strip common leading directories
  name = name.replace(/^(src\/|backend\/|server\/|app\/)?(routes\/|routers\/|api\/)?/, '');
  // Strip file extensions (.routes.ts, .router.ts, .ts, .js, etc.)
  name = name.replace(/\.(routes|router)\.(ts|js|mjs|cjs)$/, '');
  name = name.replace(/\.(ts|js|mjs|cjs)$/, '');
  // Strip trailing /index
  name = name.replace(/\/index$/, '');
  return name || relPath;
}

const HTTP_METHODS = new Set([
  'get',
  'post',
  'put',
  'delete',
  'patch',
  'options',
  'head',
  'all',
]);

interface ParseOptions {
  routeGlobs: string[];
  ignorePaths?: string[];
}

export async function parseExpressRoutes(
  projectRoot: string,
  options: ParseOptions
): Promise<ScanOutput> {
  const startTime = Date.now();
  const nodes: ScanNode[] = [];
  const warnings: ScanWarning[] = [];
  let filesScanned = 0;
  let errors = 0;

  const globs = options?.routeGlobs ?? ['**/*.ts', '**/*.js'];
  const ignorePaths = options?.ignorePaths ?? [];
  const filePaths = resolveGlobs(projectRoot, globs, ignorePaths)
    .filter(f => !/(\.spec\.|\.test\.|__tests__|\/tests?\/)/.test(f));

  const tsMorphProject = new Project({
    compilerOptions: { allowJs: true, noEmit: true },
    skipAddingFilesFromTsConfig: true,
  });

  for (const filePath of filePaths) {
    filesScanned++;
    try {
      const sourceFile = tsMorphProject.addSourceFileAtPath(filePath);
      const relPath = relative(projectRoot, filePath);
      const fileNodes = extractRouteNodes(sourceFile, relPath);
      nodes.push(...fileNodes);
    } catch (err) {
      errors++;
      const relPath = relative(projectRoot, filePath);
      warnings.push({
        file: relPath,
        message: `Failed to parse: ${err instanceof Error ? err.message : String(err)}`,
        severity: 'error',
      });
    }
  }

  return {
    version: 1,
    scanner: {
      id: 'express',
      name: 'Express Route Scanner',
      version: '0.1.0',
    },
    nodes,
    edges: [],
    warnings,
    stats: {
      filesScanned,
      nodesFound: nodes.length,
      edgesFound: 0,
      errors,
      durationMs: Date.now() - startTime,
    },
  };
}

function resolveGlobs(projectRoot: string, globs: string[], ignorePaths: string[] = []): string[] {
  const files: string[] = [];
  for (const pattern of globs) {
    const matched = globSync(pattern, { cwd: projectRoot });
    for (const m of matched) {
      if (ignorePaths.some(ip => m === ip || m.startsWith(ip + '/'))) continue;
      const fullPath = join(projectRoot, m);
      if (!files.includes(fullPath)) {
        files.push(fullPath);
      }
    }
  }
  return files.sort();
}

function extractRouteNodes(
  sourceFile: SourceFile,
  relPath: string
): ScanNode[] {
  const nodes: ScanNode[] = [];
  const seenIds = new Set<string>();
  const callExprs = sourceFile.getDescendantsOfKind(SyntaxKind.CallExpression);

  for (const call of callExprs) {
    // Try direct route pattern: router.get('/path', ...handlers)
    const directRoutes = tryDirectRoute(call, relPath);
    if (directRoutes) {
      for (const node of directRoutes) {
        let id = node.id;
        let counter = 1;
        while (seenIds.has(id)) {
          counter++;
          id = `${node.id}#${counter}`;
        }
        node.id = id;
        seenIds.add(id);
      }
      nodes.push(...directRoutes);
      continue;
    }

    // Try chained route pattern: router.route('/path').get(...).post(...)
    const chainedRoutes = tryChainedRoute(call, relPath);
    if (chainedRoutes) {
      for (const node of chainedRoutes) {
        let id = node.id;
        let counter = 1;
        while (seenIds.has(id)) {
          counter++;
          id = `${node.id}#${counter}`;
        }
        node.id = id;
        seenIds.add(id);
      }
      nodes.push(...chainedRoutes);
    }
  }

  return nodes;
}

/**
 * Detect direct route calls: router.get('/path', middleware1, middleware2, handler)
 */
function tryDirectRoute(
  call: CallExpression,
  relPath: string
): ScanNode[] | null {
  const expr = call.getExpression();
  if (expr.getKind() !== SyntaxKind.PropertyAccessExpression) return null;

  const propAccess = expr.asKindOrThrow(SyntaxKind.PropertyAccessExpression);
  const methodName = propAccess.getName().toLowerCase();

  if (!HTTP_METHODS.has(methodName)) return null;

  // Check the object being called on — skip if it's a chained route call
  // (i.e., router.route('/path').get(...) — the object is a CallExpression)
  const object = propAccess.getExpression();
  if (object.getKind() === SyntaxKind.CallExpression) {
    // This might be a chained route call like router.route('/path').get(...)
    // We handle those in tryChainedRoute, but only from the router.route() call level
    return null;
  }

  const args = call.getArguments();
  if (args.length < 2) return null;

  // First arg should be a string literal (the path)
  const pathArg = args[0];
  if (pathArg.getKind() !== SyntaxKind.StringLiteral) return null;

  const path = pathArg.asKindOrThrow(SyntaxKind.StringLiteral).getLiteralValue();
  const method = methodName.toUpperCase();

  // Remaining args: middleware (all but last) + handler (last)
  const handlerArgs = args.slice(1);
  const { middleware, handler } = extractMiddlewareAndHandler(handlerArgs);

  const routerName = deriveRouterName(relPath);
  const id = `route:${method}-${routerName}:${path}`;
  const name = `${method} ${path}`;

  return [
    {
      id,
      kind: 'route',
      name,
      label: name,
      properties: {
        method,
        path,
        middleware,
        handler,
      },
      source: 'scan',
      sourceFile: relPath,
    },
  ];
}

/**
 * Detect chained route pattern: router.route('/path').get(...).put(...).delete(...)
 * We look for the router.route() call and then walk down the chain.
 */
function tryChainedRoute(
  call: CallExpression,
  relPath: string
): ScanNode[] | null {
  const expr = call.getExpression();
  if (expr.getKind() !== SyntaxKind.PropertyAccessExpression) return null;

  const propAccess = expr.asKindOrThrow(SyntaxKind.PropertyAccessExpression);
  if (propAccess.getName() !== 'route') return null;

  // Extract the path from router.route('/path')
  const args = call.getArguments();
  if (args.length === 0) return null;

  const pathArg = args[0];
  if (pathArg.getKind() !== SyntaxKind.StringLiteral) return null;

  const path = pathArg.asKindOrThrow(SyntaxKind.StringLiteral).getLiteralValue();

  // Now walk up the AST to find the chained method calls
  // The AST structure for router.route('/path').get(handler).put(handler) is:
  //   CallExpression (.put)
  //     PropertyAccessExpression
  //       CallExpression (.get)
  //         PropertyAccessExpression
  //           CallExpression (router.route)
  //
  // So we need to walk UP from router.route() to find the method calls

  const nodes: ScanNode[] = [];
  collectChainedMethods(call, path, relPath, nodes);

  return nodes.length > 0 ? nodes : null;
}

/**
 * Walk up the AST from a router.route() call to find chained HTTP method calls.
 */
function collectChainedMethods(
  routeCall: CallExpression,
  path: string,
  relPath: string,
  nodes: ScanNode[]
): void {
  let current: CallExpression = routeCall;

  for (let i = 0; i < 20; i++) {
    const parent = current.getParent();
    if (!parent) break;

    // The parent should be a PropertyAccessExpression (.get, .put, etc.)
    if (parent.getKind() !== SyntaxKind.PropertyAccessExpression) break;

    const parentProp = parent.asKindOrThrow(SyntaxKind.PropertyAccessExpression);
    const methodName = parentProp.getName().toLowerCase();

    // The PropertyAccessExpression should be inside a CallExpression
    const grandParent = parentProp.getParent();
    if (!grandParent || grandParent.getKind() !== SyntaxKind.CallExpression)
      break;

    const parentCall = grandParent.asKindOrThrow(SyntaxKind.CallExpression);

    if (HTTP_METHODS.has(methodName)) {
      const method = methodName.toUpperCase();
      const handlerArgs = parentCall.getArguments();
      const { middleware, handler } = extractMiddlewareAndHandler(handlerArgs);

      const routerName = deriveRouterName(relPath);
      const id = `route:${method}-${routerName}:${path}`;
      const name = `${method} ${path}`;

      nodes.push({
        id,
        kind: 'route',
        name,
        label: name,
        properties: {
          method,
          path,
          middleware,
          handler,
        },
        source: 'scan',
        sourceFile: relPath,
      });
    }

    current = parentCall;
  }
}

/**
 * Extract middleware names and handler name from arguments.
 * Last argument is the handler, all others are middleware.
 * For function calls like requireRole('admin'), extract just the function name.
 */
function extractMiddlewareAndHandler(
  args: ReturnType<CallExpression['getArguments']>
): { middleware: string[]; handler: string } {
  if (args.length === 0) {
    return { middleware: [], handler: '' };
  }

  const names = args.map((arg) => extractArgName(arg));

  if (names.length === 1) {
    return { middleware: [], handler: names[0] };
  }

  return {
    middleware: names.slice(0, -1),
    handler: names[names.length - 1],
  };
}

/**
 * Extract a name from an argument node.
 * - Identifier: returns the name (e.g., getUsers)
 * - CallExpression: returns the function name (e.g., requireRole from requireRole('admin'))
 * - Otherwise: returns the text
 */
function extractArgName(node: ReturnType<CallExpression['getArguments']>[0]): string {
  if (node.getKind() === SyntaxKind.Identifier) {
    return node.getText();
  }

  if (node.getKind() === SyntaxKind.CallExpression) {
    const callExpr = node.asKindOrThrow(SyntaxKind.CallExpression);
    const callee = callExpr.getExpression();
    if (callee.getKind() === SyntaxKind.Identifier) {
      return callee.getText();
    }
    // For property access like obj.method(), return method
    if (callee.getKind() === SyntaxKind.PropertyAccessExpression) {
      return callee
        .asKindOrThrow(SyntaxKind.PropertyAccessExpression)
        .getName();
    }
  }

  return node.getText();
}
