import { Project, SyntaxKind, type SourceFile, type JsxSelfClosingElement, type JsxElement, type Node } from 'ts-morph';
import { join, relative } from 'path';
import type { ScanOutput, ScanNode, ScanWarning } from './types.js';

interface ParseOptions {
  routeFiles: string[];
}

interface RouteInfo {
  path: string;
  component: string;
  isIndex: boolean;
  isProtected: boolean;
  parentPath?: string;
  line?: number;
}

export async function parseReactRouterPages(
  projectRoot: string,
  options: ParseOptions
): Promise<ScanOutput> {
  const startTime = Date.now();
  const nodes: ScanNode[] = [];
  const warnings: ScanWarning[] = [];
  let filesScanned = 0;
  let errors = 0;

  const tsMorphProject = new Project({
    compilerOptions: {
      allowJs: true,
      noEmit: true,
      jsx: 4, // JsxEmit.ReactJSX
    },
    skipAddingFilesFromTsConfig: true,
  });

  for (const file of options.routeFiles) {
    filesScanned++;
    const fullPath = join(projectRoot, file);
    try {
      const sourceFile = tsMorphProject.addSourceFileAtPath(fullPath);
      const relPath = relative(projectRoot, fullPath);
      const routes = extractRoutes(sourceFile);
      for (const route of routes) {
        const id = `page:${route.path}`;
        const node: ScanNode = {
          id,
          kind: 'page',
          name: route.path,
          label: route.component,
          properties: {
            path: route.path,
            component: route.component,
            isProtected: route.isProtected,
            isIndex: route.isIndex,
            ...(route.parentPath !== undefined ? { parentPath: route.parentPath } : {}),
          },
          source: 'scan',
          sourceFile: relPath,
        };
        nodes.push(node);
      }
    } catch (err) {
      errors++;
      warnings.push({
        file,
        message: `Failed to parse: ${err instanceof Error ? err.message : String(err)}`,
        severity: 'error',
      });
    }
  }

  return {
    version: 1,
    scanner: {
      id: 'react-router',
      name: 'React Router Page Scanner',
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

function extractRoutes(sourceFile: SourceFile): RouteInfo[] {
  const routes: RouteInfo[] = [];

  // Find all JSX elements (both self-closing and opening) named "Route"
  const selfClosing = sourceFile.getDescendantsOfKind(SyntaxKind.JsxSelfClosingElement)
    .filter((el) => el.getTagNameNode().getText() === 'Route');

  const jsxElements = sourceFile.getDescendantsOfKind(SyntaxKind.JsxElement)
    .filter((el) => el.getOpeningElement().getTagNameNode().getText() === 'Route');

  // Process self-closing Route elements: <Route path="/foo" element={<Bar />} />
  for (const el of selfClosing) {
    const route = extractRouteFromSelfClosing(el);
    if (route) routes.push(route);
  }

  // Process Route elements with children (layout routes)
  for (const el of jsxElements) {
    const route = extractRouteFromJsxElement(el);
    if (route) routes.push(route);
  }

  return routes;
}

function extractRouteFromSelfClosing(el: JsxSelfClosingElement): RouteInfo | null {
  const isIndex = hasAttribute(el, 'index');
  const path = getJsxAttributeStringValue(el, 'path');
  const component = getComponentFromElementAttr(el);

  if (!component) return null;

  const isProtected = isInProtectedContext(el);
  const parentPath = getParentRoutePath(el);

  const fullPath = composePath(parentPath, path, isIndex);

  return {
    path: fullPath,
    component,
    isIndex,
    isProtected,
    ...(parentPath !== undefined ? { parentPath } : {}),
    line: el.getStartLineNumber(),
  };
}

function extractRouteFromJsxElement(el: JsxElement): RouteInfo | null {
  const openingEl = el.getOpeningElement();
  const path = getJsxAttributeStringValueFromOpening(openingEl, 'path');
  const component = getComponentFromElementAttrOpening(openingEl);
  const isIndex = hasAttributeOpening(openingEl, 'index');

  if (!component) return null;

  const isProtected = isInProtectedContext(el);
  const parentPath = getParentRoutePath(el);

  const fullPath = composePath(parentPath, path, isIndex);

  return {
    path: fullPath,
    component,
    isIndex,
    isProtected,
    ...(parentPath !== undefined ? { parentPath } : {}),
    line: el.getStartLineNumber(),
  };
}

function composePath(parentPath: string | undefined, localPath: string | undefined, isIndex: boolean): string {
  if (isIndex) {
    return parentPath ?? '/';
  }
  if (!localPath) return parentPath ?? '/';

  if (localPath.startsWith('/')) {
    // Absolute path
    return localPath;
  }

  if (parentPath) {
    const base = parentPath.endsWith('/') ? parentPath.slice(0, -1) : parentPath;
    return `${base}/${localPath}`;
  }

  return localPath;
}

function getParentRoutePath(node: Node): string | undefined {
  let current = node.getParent();
  while (current) {
    // Check if parent is a JsxElement with tag name "Route"
    if (current.getKind() === SyntaxKind.JsxElement) {
      const jsxEl = current.asKindOrThrow(SyntaxKind.JsxElement);
      const tagName = jsxEl.getOpeningElement().getTagNameNode().getText();
      if (tagName === 'Route') {
        const path = getJsxAttributeStringValueFromOpening(jsxEl.getOpeningElement(), 'path');
        if (path) {
          // Recursively get parent's parent path
          const grandparentPath = getParentRoutePath(jsxEl);
          if (grandparentPath) {
            const base = grandparentPath.endsWith('/') ? grandparentPath.slice(0, -1) : grandparentPath;
            return path.startsWith('/') ? path : `${base}/${path}`;
          }
          return path;
        }
      }
    }
    current = current.getParent();
  }
  return undefined;
}

function isInProtectedContext(node: Node): boolean {
  let current = node.getParent();
  while (current) {
    // Check for JsxExpression containing a ConditionalExpression with Navigate
    if (current.getKind() === SyntaxKind.JsxExpression) {
      const expr = current.asKindOrThrow(SyntaxKind.JsxExpression);
      const expression = expr.getExpression();
      if (expression && expression.getKind() === SyntaxKind.ConditionalExpression) {
        const conditional = expression.asKindOrThrow(SyntaxKind.ConditionalExpression);
        // Check if the falsy branch (whenFalse) contains Navigate
        const whenFalse = conditional.getWhenFalse();
        const whenFalseText = whenFalse.getText();
        if (whenFalseText.includes('Navigate')) {
          return true;
        }
        // Also check if the truthy branch contains Navigate (reversed ternary)
        const whenTrue = conditional.getWhenTrue();
        const whenTrueText = whenTrue.getText();
        if (whenTrueText.includes('Navigate')) {
          // Routes are in the falsy branch = they ARE the protected ones? No.
          // If whenTrue has Navigate, routes in whenFalse are the non-auth path
          // This is the uncommon case, skip for now
        }
      }
    }
    current = current.getParent();
  }
  return false;
}

// --- JSX attribute helpers for JsxSelfClosingElement ---

function getJsxAttributeStringValue(el: JsxSelfClosingElement, attrName: string): string | undefined {
  const attr = el.getAttribute(attrName);
  if (!attr || attr.getKind() !== SyntaxKind.JsxAttribute) return undefined;
  const jsxAttr = attr.asKindOrThrow(SyntaxKind.JsxAttribute);
  const initializer = jsxAttr.getInitializer();
  if (!initializer) return undefined;
  if (initializer.getKind() === SyntaxKind.StringLiteral) {
    return initializer.asKindOrThrow(SyntaxKind.StringLiteral).getLiteralValue();
  }
  return undefined;
}

function hasAttribute(el: JsxSelfClosingElement, attrName: string): boolean {
  return el.getAttribute(attrName) !== undefined;
}

function getComponentFromElementAttr(el: JsxSelfClosingElement): string | undefined {
  const attr = el.getAttribute('element');
  if (!attr || attr.getKind() !== SyntaxKind.JsxAttribute) return undefined;
  const jsxAttr = attr.asKindOrThrow(SyntaxKind.JsxAttribute);
  const initializer = jsxAttr.getInitializer();
  if (!initializer || initializer.getKind() !== SyntaxKind.JsxExpression) return undefined;

  const jsxExpr = initializer.asKindOrThrow(SyntaxKind.JsxExpression);
  const expression = jsxExpr.getExpression();
  if (!expression) return undefined;

  // Look for JsxSelfClosingElement inside the expression: <Dashboard />
  if (expression.getKind() === SyntaxKind.JsxSelfClosingElement) {
    return expression.asKindOrThrow(SyntaxKind.JsxSelfClosingElement).getTagNameNode().getText();
  }

  // Also handle JsxElement (opening + closing tags)
  if (expression.getKind() === SyntaxKind.JsxElement) {
    return expression.asKindOrThrow(SyntaxKind.JsxElement).getOpeningElement().getTagNameNode().getText();
  }

  return undefined;
}

// --- JSX attribute helpers for JsxOpeningElement ---

function getJsxAttributeStringValueFromOpening(
  el: { getAttribute(name: string): Node | undefined },
  attrName: string
): string | undefined {
  const attr = el.getAttribute(attrName);
  if (!attr || attr.getKind() !== SyntaxKind.JsxAttribute) return undefined;
  const jsxAttr = attr.asKindOrThrow(SyntaxKind.JsxAttribute);
  const initializer = jsxAttr.getInitializer();
  if (!initializer) return undefined;
  if (initializer.getKind() === SyntaxKind.StringLiteral) {
    return initializer.asKindOrThrow(SyntaxKind.StringLiteral).getLiteralValue();
  }
  return undefined;
}

function hasAttributeOpening(
  el: { getAttribute(name: string): Node | undefined },
  attrName: string
): boolean {
  return el.getAttribute(attrName) !== undefined;
}

function getComponentFromElementAttrOpening(
  el: { getAttribute(name: string): Node | undefined }
): string | undefined {
  const attr = el.getAttribute('element');
  if (!attr || attr.getKind() !== SyntaxKind.JsxAttribute) return undefined;
  const jsxAttr = attr.asKindOrThrow(SyntaxKind.JsxAttribute);
  const initializer = jsxAttr.getInitializer();
  if (!initializer || initializer.getKind() !== SyntaxKind.JsxExpression) return undefined;

  const jsxExpr = initializer.asKindOrThrow(SyntaxKind.JsxExpression);
  const expression = jsxExpr.getExpression();
  if (!expression) return undefined;

  if (expression.getKind() === SyntaxKind.JsxSelfClosingElement) {
    return expression.asKindOrThrow(SyntaxKind.JsxSelfClosingElement).getTagNameNode().getText();
  }

  if (expression.getKind() === SyntaxKind.JsxElement) {
    return expression.asKindOrThrow(SyntaxKind.JsxElement).getOpeningElement().getTagNameNode().getText();
  }

  return undefined;
}
