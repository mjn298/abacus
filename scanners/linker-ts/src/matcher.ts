import { SourceFile, SyntaxKind, Node } from "ts-morph";

/**
 * Quick pre-filter: does this file import from @prisma/client
 * or declare a variable typed as PrismaClient?
 */
export function hasPrismaImport(sourceFile: SourceFile): boolean {
  // Check import declarations for @prisma/client
  for (const imp of sourceFile.getImportDeclarations()) {
    if (imp.getModuleSpecifierValue().includes("@prisma/client")) {
      return true;
    }
  }

  // Check for variable declarations typed as PrismaClient
  for (const varDecl of sourceFile.getDescendantsOfKind(SyntaxKind.VariableDeclaration)) {
    const typeNode = varDecl.getTypeNode();
    if (typeNode && typeNode.getText().includes("PrismaClient")) {
      return true;
    }
    // Check initializer: new PrismaClient()
    const init = varDecl.getInitializer();
    if (init && Node.isNewExpression(init) && init.getExpression().getText() === "PrismaClient") {
      return true;
    }
  }

  return false;
}

/**
 * Collect the names of all variables/parameters in the file that resolve to a PrismaClient instance.
 */
function collectPrismaClientNames(sourceFile: SourceFile): Set<string> {
  const names = new Set<string>();

  // Variables with PrismaClient type annotation or `new PrismaClient()` initializer
  for (const varDecl of sourceFile.getDescendantsOfKind(SyntaxKind.VariableDeclaration)) {
    const typeNode = varDecl.getTypeNode();
    if (typeNode && typeNode.getText().includes("PrismaClient")) {
      names.add(varDecl.getName());
      continue;
    }
    const init = varDecl.getInitializer();
    if (init && Node.isNewExpression(init) && init.getExpression().getText() === "PrismaClient") {
      names.add(varDecl.getName());
    }
  }

  // Constructor parameters and function parameters typed as PrismaClient
  for (const param of sourceFile.getDescendantsOfKind(SyntaxKind.Parameter)) {
    const typeNode = param.getTypeNode();
    if (typeNode && typeNode.getText().includes("PrismaClient")) {
      names.add(param.getName());
    }
  }

  // If nothing specific found but the file imports from @prisma/client,
  // fall back to the conventional name "prisma"
  if (names.size === 0) {
    for (const imp of sourceFile.getImportDeclarations()) {
      if (imp.getModuleSpecifierValue().includes("@prisma/client")) {
        names.add("prisma");
        break;
      }
    }
  }

  return names;
}

/**
 * Find all `prisma.modelName.method()` calls and return matched entity node IDs.
 *
 * Walks every CallExpression looking for a property-access chain of depth >= 2
 * where the root object is a known PrismaClient instance. The middle property
 * is treated as the model name, looked up (lowercased) in entityByName.
 */
export function findPrismaAccesses(
  sourceFile: SourceFile,
  entityByName: Map<string, string>,
): Set<string> {
  const matched = new Set<string>();
  const prismaNames = collectPrismaClientNames(sourceFile);

  if (prismaNames.size === 0) {
    return matched;
  }

  for (const call of sourceFile.getDescendantsOfKind(SyntaxKind.CallExpression)) {
    const expr = call.getExpression();

    // We need prisma.model.method() which is a PropertyAccessExpression
    // where the expression is also a PropertyAccessExpression
    if (!Node.isPropertyAccessExpression(expr)) {
      continue;
    }

    // expr = prisma.model.method  →  expr.getExpression() = prisma.model
    const innerExpr = expr.getExpression();
    if (!Node.isPropertyAccessExpression(innerExpr)) {
      continue;
    }

    // innerExpr = prisma.model  →  innerExpr.getExpression() = prisma (the root object)
    const rootExpr = innerExpr.getExpression();

    // The root must be an identifier matching a known PrismaClient variable
    if (!Node.isIdentifier(rootExpr)) {
      continue;
    }

    const rootName = rootExpr.getText();
    if (!prismaNames.has(rootName)) {
      continue;
    }

    // The model name is the property of the inner expression
    const modelName = innerExpr.getName().toLowerCase();

    const entityId = entityByName.get(modelName);
    if (entityId !== undefined) {
      matched.add(entityId);
    }
  }

  return matched;
}
