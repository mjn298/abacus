// Scanner Protocol v1 types

export interface ScanInput {
  version: number;
  projectRoot: string;
  options: {
    schemaPath?: string;
  };
  ignorePaths?: string[];
}

export interface ScannerInfo {
  id: string;
  name: string;
  version: string;
}

export interface ScanNode {
  id: string;
  kind: "entity";
  name: string;
  label: string;
  properties: Record<string, unknown>;
  source: "scan";
  sourceFile?: string;
}

export interface ScanEdge {
  id: string;
  srcId: string;
  dstId: string;
  kind: "field_relation";
  properties?: Record<string, unknown>;
}

export interface ScanWarning {
  file: string;
  line?: number;
  message: string;
  severity: "info" | "warning" | "error";
}

export interface ScanStats {
  filesScanned: number;
  nodesFound: number;
  edgesFound: number;
  errors: number;
  durationMs: number;
}

export interface ScanOutput {
  version: 1;
  scanner: ScannerInfo;
  nodes: ScanNode[];
  edges: ScanEdge[];
  warnings: ScanWarning[];
  stats: ScanStats;
}

// Internal parsing types

export interface PrismaField {
  name: string;
  type: string;
  isRequired: boolean;
  isList: boolean;
  isId: boolean;
  isUnique: boolean;
  isUpdatedAt: boolean;
  hasDefault: boolean;
  defaultValue?: string;
  relationName?: string;
  relationFields?: string[];
  relationReferences?: string[];
}

export interface PrismaModel {
  name: string;
  fields: PrismaField[];
  tableName?: string;
}

export interface PrismaEnum {
  name: string;
  values: string[];
}

export interface ParseResult {
  models: PrismaModel[];
  enums: PrismaEnum[];
}
