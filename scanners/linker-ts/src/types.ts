// Scanner Protocol v1 types — linker variant
// Mirrors Go ScanNodeRef, ScanInput, ScanOutput from internal/scanner/protocol.go

export interface ScanNodeRef {
  id: string;
  kind: string;
  name: string;
  sourceFile?: string;
}

export interface ScanInput {
  version: number;
  projectRoot: string;
  options: {
    maxDepth?: number;
    tsConfigPath?: string;
    tsconfig?: string;
  };
  ignorePaths?: string[];
  existingNodes?: ScanNodeRef[];
}

export interface ScannerInfo {
  id: string;
  name: string;
  version: string;
}

export interface ScanEdge {
  id: string;
  srcId: string;
  dstId: string;
  kind: string;
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
  nodes: never[];
  edges: ScanEdge[];
  warnings: ScanWarning[];
  stats: ScanStats;
}

// Internal types

export interface RouteEntityMatch {
  routeNodeId: string;
  entityNodeId: string;
  tracePath: string[];
}
