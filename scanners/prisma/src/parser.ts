import type {
  ParseResult,
  PrismaModel,
  PrismaEnum,
  PrismaField,
  ScanOutput,
  ScanNode,
  ScanEdge,
} from "./types.js";

/**
 * Parse a Prisma schema string into models and enums.
 */
export function parsePrismaSchema(schema: string): ParseResult {
  const models = parseModels(schema);
  const enums = parseEnums(schema);
  return { models, enums };
}

function parseModels(schema: string): PrismaModel[] {
  const models: PrismaModel[] = [];
  const modelRegex = /^model\s+(\w+)\s*\{([\s\S]*?)^\}/gm;

  let match: RegExpExecArray | null;
  while ((match = modelRegex.exec(schema)) !== null) {
    const name = match[1];
    const body = match[2];
    const fields = parseFields(body);
    const tableName = parseTableName(body);

    models.push({
      name,
      fields,
      ...(tableName ? { tableName } : {}),
    });
  }

  return models;
}

function parseFields(body: string): PrismaField[] {
  const fields: PrismaField[] = [];
  const lines = body.split("\n");

  for (const line of lines) {
    const trimmed = line.trim();
    // Skip empty lines, comments, and @@directives
    if (!trimmed || trimmed.startsWith("//") || trimmed.startsWith("@@")) {
      continue;
    }

    // Match: fieldName Type? or fieldName Type[] or fieldName Type
    const fieldMatch = trimmed.match(
      /^(\w+)\s+(\w+)(\[\])?\??(\[\])?\s*(.*)?$/,
    );
    if (!fieldMatch) continue;

    const name = fieldMatch[1];
    const type = fieldMatch[2];
    const isList = !!(fieldMatch[3] || fieldMatch[4]);
    const rest = fieldMatch[5] || "";

    // Check if the type is optional (has ? after type name)
    const isOptional = /^\w+\?/.test(trimmed.split(/\s+/)[1] || "");

    const isId = rest.includes("@id");
    const isUnique = rest.includes("@unique");
    const isUpdatedAt = rest.includes("@updatedAt");
    const hasDefault = rest.includes("@default");

    let defaultValue: string | undefined;
    if (hasDefault) {
      // Handle nested parens like @default(cuid()) or @default(now())
      const defaultMatch = rest.match(/@default\(([^)]*(?:\([^)]*\))?[^)]*)\)/);
      if (defaultMatch) {
        defaultValue = defaultMatch[1];
      }
    }

    // Parse @relation
    let relationFields: string[] | undefined;
    let relationReferences: string[] | undefined;
    let relationName: string | undefined;
    const relationMatch = rest.match(/@relation\(([^)]*)\)/);
    if (relationMatch) {
      const relContent = relationMatch[1];

      const fieldsMatch = relContent.match(/fields:\s*\[([^\]]*)\]/);
      if (fieldsMatch) {
        relationFields = fieldsMatch[1].split(",").map((s) => s.trim());
      }

      const refsMatch = relContent.match(/references:\s*\[([^\]]*)\]/);
      if (refsMatch) {
        relationReferences = refsMatch[1].split(",").map((s) => s.trim());
      }

      const nameMatch = relContent.match(/"([^"]+)"/);
      if (nameMatch) {
        relationName = nameMatch[1];
      }
    }

    const field: PrismaField = {
      name,
      type,
      isRequired: !isOptional && !isList,
      isList,
      isId,
      isUnique,
      isUpdatedAt,
      hasDefault,
      ...(defaultValue !== undefined ? { defaultValue } : {}),
      ...(relationFields ? { relationFields } : {}),
      ...(relationReferences ? { relationReferences } : {}),
      ...(relationName ? { relationName } : {}),
    };

    fields.push(field);
  }

  return fields;
}

function parseTableName(body: string): string | undefined {
  const match = body.match(/@@map\("([^"]+)"\)/);
  return match ? match[1] : undefined;
}

function parseEnums(schema: string): PrismaEnum[] {
  const enums: PrismaEnum[] = [];
  const enumRegex = /^enum\s+(\w+)\s*\{([\s\S]*?)^\}/gm;

  let match: RegExpExecArray | null;
  while ((match = enumRegex.exec(schema)) !== null) {
    const name = match[1];
    const body = match[2];
    const values = body
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith("//"));

    enums.push({ name, values });
  }

  return enums;
}

/**
 * Build ScanOutput from parse results.
 */
export function buildScanOutput(
  parseResult: ParseResult,
  sourceFile: string,
  filesScanned: number,
  durationMs: number = 0,
): ScanOutput {
  const nodes: ScanNode[] = [];
  const edges: ScanEdge[] = [];

  // All known model names for relation type detection
  const modelNames = new Set(parseResult.models.map((m) => m.name));

  // Create nodes for models
  for (const model of parseResult.models) {
    const relations: Array<Record<string, unknown>> = [];

    for (const field of model.fields) {
      if (modelNames.has(field.type) || field.relationFields) {
        relations.push({
          fieldName: field.name,
          relatedModel: field.type,
          isList: field.isList,
          ...(field.relationFields
            ? { relationFields: field.relationFields }
            : {}),
          ...(field.relationReferences
            ? { relationReferences: field.relationReferences }
            : {}),
        });
      }
    }

    // Determine relation type and create edges for owning side only
    for (const field of model.fields) {
      if (!field.relationFields) continue; // skip non-owning side

      let relationType: string;
      if (field.isList) {
        relationType = "one-to-many";
      } else {
        // Check if the FK field is @unique — that makes it one-to-one
        const fkFieldName = field.relationFields[0];
        const fkField = model.fields.find((f) => f.name === fkFieldName);
        if (fkField?.isUnique) {
          relationType = "one-to-one";
        } else {
          relationType = "many-to-one";
        }
      }

      edges.push({
        id: `edge:entity:${model.name}-field_relation:${field.name}-entity:${field.type}`,
        srcId: `entity:${model.name}`,
        dstId: `entity:${field.type}`,
        kind: "field_relation",
        properties: {
          fieldName: field.name,
          relationType,
        },
      });
    }

    const fieldsMeta = model.fields.map((f) => ({
      name: f.name,
      type: f.type,
      isRequired: f.isRequired,
      isList: f.isList,
      isId: f.isId,
      isUnique: f.isUnique,
      hasDefault: f.hasDefault,
      ...(f.defaultValue !== undefined ? { defaultValue: f.defaultValue } : {}),
      ...(f.isUpdatedAt ? { isUpdatedAt: true } : {}),
    }));

    nodes.push({
      id: `entity:${model.name}`,
      kind: "entity",
      name: model.name,
      label: model.name,
      properties: {
        isEnum: false,
        fields: fieldsMeta,
        relations,
        ...(model.tableName ? { tableName: model.tableName } : {}),
      },
      source: "scan",
      sourceFile,
    });
  }

  // Create nodes for enums
  for (const enumDef of parseResult.enums) {
    nodes.push({
      id: `entity:${enumDef.name}`,
      kind: "entity",
      name: enumDef.name,
      label: enumDef.name,
      properties: {
        isEnum: true,
        values: enumDef.values,
      },
      source: "scan",
      sourceFile,
    });
  }

  return {
    version: 1,
    scanner: {
      id: "prisma",
      name: "Prisma Entity Scanner",
      version: "0.1.0",
    },
    nodes,
    edges,
    warnings: [],
    stats: {
      filesScanned,
      nodesFound: nodes.length,
      edgesFound: edges.length,
      errors: 0,
      durationMs,
    },
  };
}
