/** Scanner Protocol v1 -- Input from stdin */
export interface ScanInput {
  version: 1;
  projectRoot: string;
  options: {
    routeFiles: string[];
  };
}

/** Scanner Protocol v1 -- Output to stdout */
export interface ScanOutput {
  version: 1;
  scanner: ScannerInfo;
  nodes: ScanNode[];
  edges: ScanEdge[];
  warnings: ScanWarning[];
  stats: ScanStats;
}

export interface ScannerInfo {
  id: string;
  name: string;
  version: string;
}

export interface ScanNode {
  id: string;
  kind: 'route' | 'entity' | 'page' | 'action' | 'permission';
  name: string;
  label: string;
  properties: Record<string, unknown>;
  source: string;
  sourceFile?: string;
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
  severity: 'info' | 'warning' | 'error';
}

export interface ScanStats {
  filesScanned: number;
  nodesFound: number;
  edgesFound: number;
  errors: number;
  durationMs: number;
}
