import { Project, SyntaxKind, type SourceFile, type CallExpression, type PropertyAccessExpression, type ObjectLiteralExpression, type Node } from 'ts-morph';
import { join, relative } from 'path';
import { globSync } from 'fs';
import type { ScanOutput, ScanNode, ScanWarning } from './types.js';

interface ParseOptions {
  contractGlobs: string[];
}

export async function parseOrpcContracts(
  projectRoot: string,
  options: ParseOptions
): Promise<ScanOutput> {
  const startTime = Date.now();
  const nodes: ScanNode[] = [];
  const warnings: ScanWarning[] = [];
  let filesScanned = 0;
  let errors = 0;

  // Resolve globs to file paths
  const filePaths = resolveGlobs(projectRoot, options.contractGlobs);

  // Create ts-morph project (no type checking needed, just AST)
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
      id: 'orpc',
      name: 'oRPC Route Scanner',
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

function resolveGlobs(projectRoot: string, globs: string[]): string[] {
  const files: string[] = [];
  for (const pattern of globs) {
    // Use Node.js built-in glob (available since Node 22)
    const matched = globSync(pattern, { cwd: projectRoot });
    for (const m of matched) {
      const fullPath = join(projectRoot, m);
      if (!files.includes(fullPath)) {
        files.push(fullPath);
      }
    }
  }
  return files.sort();
}

function extractRouteNodes(sourceFile: SourceFile, relPath: string): ScanNode[] {
  const nodes: ScanNode[] = [];

  // Find all .route() call expressions
  const callExprs = sourceFile.getDescendantsOfKind(SyntaxKind.CallExpression);

  for (const call of callExprs) {
    const expr = call.getExpression();
    if (expr.getKind() !== SyntaxKind.PropertyAccessExpression) continue;

    const propAccess = expr.asKindOrThrow(SyntaxKind.PropertyAccessExpression);
    if (propAccess.getName() !== 'route') continue;

    // Extract the route config object literal
    const args = call.getArguments();
    if (args.length === 0) continue;

    const configArg = args[0];
    if (configArg.getKind() !== SyntaxKind.ObjectLiteralExpression) continue;

    const configObj = configArg.asKindOrThrow(SyntaxKind.ObjectLiteralExpression);

    const method = getStringProperty(configObj, 'method');
    const path = getStringProperty(configObj, 'path');

    if (!method || !path) continue;

    const summary = getStringProperty(configObj, 'summary');
    const tags = getArrayProperty(configObj, 'tags');
    const successStatus = getNumericProperty(configObj, 'successStatus');

    // Walk the chain to find .input() and .output() calls
    const { inputSchema, outputSchema } = findChainedSchemas(call);

    const id = `route:${method}-${path}`;
    const name = `${method} ${path}`;
    const label = summary || name;

    const properties: Record<string, unknown> = {
      method,
      path,
      contractFile: relPath,
    };

    if (summary !== undefined) properties.summary = summary;
    if (tags !== undefined) properties.tags = tags;
    if (successStatus !== undefined) properties.successStatus = successStatus;
    if (inputSchema !== undefined) properties.inputSchema = inputSchema;
    if (outputSchema !== undefined) properties.outputSchema = outputSchema;

    nodes.push({
      id,
      kind: 'route',
      name,
      label,
      properties,
      source: 'scan',
      sourceFile: relPath,
    });
  }

  return nodes;
}

/**
 * Walk up the AST chain from a .route() call to find .input() and .output() calls.
 *
 * The chain looks like: oc.route({...}).input(Schema).output(Schema)
 * The .route() call is the innermost; .input() and .output() are parent CallExpressions
 * that use the result of .route() as their receiver.
 */
function findChainedSchemas(routeCall: CallExpression): {
  inputSchema?: string;
  outputSchema?: string;
} {
  let inputSchema: string | undefined;
  let outputSchema: string | undefined;

  // Walk up: the .route() call may be the expression inside a PropertyAccessExpression
  // for .input(), and that .input() call may be inside another for .output()
  let current: CallExpression = routeCall;

  // Walk up the chain (up to 5 levels to be safe)
  for (let i = 0; i < 5; i++) {
    const parent = current.getParent();
    if (!parent) break;

    // Check if parent is a PropertyAccessExpression (e.g., .input or .output)
    if (parent.getKind() === SyntaxKind.PropertyAccessExpression) {
      const parentProp = parent as PropertyAccessExpression;
      const methodName = parentProp.getName();

      // The PropertyAccessExpression should be inside a CallExpression
      const grandParent = parentProp.getParent();
      if (!grandParent || grandParent.getKind() !== SyntaxKind.CallExpression) break;

      const parentCall = grandParent as CallExpression;

      if (methodName === 'input' || methodName === 'output') {
        const schemaArgs = parentCall.getArguments();
        if (schemaArgs.length > 0) {
          const schemaName = schemaArgs[0].getText();
          if (methodName === 'input') {
            inputSchema = schemaName;
          } else {
            outputSchema = schemaName;
          }
        }
      }

      current = parentCall;
    } else {
      break;
    }
  }

  return { inputSchema, outputSchema };
}

function getStringProperty(
  obj: ObjectLiteralExpression,
  name: string
): string | undefined {
  const prop = obj.getProperty(name);
  if (!prop) return undefined;

  if (prop.getKind() === SyntaxKind.PropertyAssignment) {
    const init = prop.asKindOrThrow(SyntaxKind.PropertyAssignment).getInitializer();
    if (!init) return undefined;
    if (init.getKind() === SyntaxKind.StringLiteral) {
      return init.asKindOrThrow(SyntaxKind.StringLiteral).getLiteralValue();
    }
  }
  return undefined;
}

function getNumericProperty(
  obj: ObjectLiteralExpression,
  name: string
): number | undefined {
  const prop = obj.getProperty(name);
  if (!prop) return undefined;

  if (prop.getKind() === SyntaxKind.PropertyAssignment) {
    const init = prop.asKindOrThrow(SyntaxKind.PropertyAssignment).getInitializer();
    if (!init) return undefined;
    if (init.getKind() === SyntaxKind.NumericLiteral) {
      return Number(init.getText());
    }
  }
  return undefined;
}

function getArrayProperty(
  obj: ObjectLiteralExpression,
  name: string
): string[] | undefined {
  const prop = obj.getProperty(name);
  if (!prop) return undefined;

  if (prop.getKind() === SyntaxKind.PropertyAssignment) {
    const init = prop.asKindOrThrow(SyntaxKind.PropertyAssignment).getInitializer();
    if (!init) return undefined;
    if (init.getKind() === SyntaxKind.ArrayLiteralExpression) {
      const arr = init.asKindOrThrow(SyntaxKind.ArrayLiteralExpression);
      return arr.getElements().map((el) => {
        if (el.getKind() === SyntaxKind.StringLiteral) {
          return el.asKindOrThrow(SyntaxKind.StringLiteral).getLiteralValue();
        }
        return el.getText();
      });
    }
  }
  return undefined;
}
