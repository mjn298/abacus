---
description: Scaffold a new Abacus scanner with boilerplate code and tests
argument-hint: "<scanner-name> [language]"
---

Scaffold a new Abacus scanner. This creates all the boilerplate files needed for a scanner that reads the Scanner Protocol V1 input on stdin and writes scan results to stdout as JSON.

## Steps

1. Parse the arguments:
   - First argument (required): scanner name (e.g., `prisma`, `nextjs`, `express`). Must be lowercase alphanumeric with hyphens. If missing, ask the user for a name.
   - Second argument (optional): language — `typescript`, `ts`, or `go`. Defaults to `typescript`.

2. Determine the scanner directory: `scanners/<scanner-name>/` relative to the abacus repo root.

3. Check if the directory already exists. If it does, warn the user and ask before overwriting.

4. **If language is TypeScript (default):**

   Create the following files:

   **`scanners/<scanner-name>/package.json`:**
   ```json
   {
     "name": "@abacus/scanner-<scanner-name>",
     "version": "0.1.0",
     "private": true,
     "main": "dist/index.js",
     "scripts": {
       "build": "tsc",
       "start": "ts-node src/index.ts",
       "test": "jest"
     },
     "dependencies": {
       "ts-morph": "^21.0.0"
     },
     "devDependencies": {
       "@types/jest": "^29.0.0",
       "@types/node": "^20.0.0",
       "jest": "^29.0.0",
       "ts-jest": "^29.0.0",
       "ts-node": "^10.0.0",
       "typescript": "^5.0.0"
     }
   }
   ```

   **`scanners/<scanner-name>/tsconfig.json`:**
   ```json
   {
     "compilerOptions": {
       "target": "ES2020",
       "module": "commonjs",
       "lib": ["ES2020"],
       "outDir": "./dist",
       "rootDir": "./src",
       "strict": true,
       "esModuleInterop": true,
       "skipLibCheck": true,
       "forceConsistentCasingInFileNames": true,
       "resolveJsonModule": true,
       "declaration": true
     },
     "include": ["src/**/*"],
     "exclude": ["node_modules", "dist", "__tests__"]
   }
   ```

   **`scanners/<scanner-name>/src/types.ts`:**
   ```typescript
   export interface ScanInput {
     version: number;
     projectRoot: string;
     options: Record<string, any>;
   }

   export interface ScanOutput {
     version: number;
     scanner: { id: string; name: string; version: string };
     nodes: ScanNode[];
     edges: ScanEdge[];
     warnings: ScanWarning[];
     stats: { filesScanned: number; nodesFound: number; edgesFound: number; errors: number };
   }

   export interface ScanNode {
     id: string;
     kind: "route" | "entity" | "page" | "action" | "permission";
     name: string;
     label: string;
     properties: Record<string, any>;
     source: string;
     sourceFile?: string;
   }

   export interface ScanEdge {
     id: string;
     srcId: string;
     dstId: string;
     kind: string;
     properties?: Record<string, any>;
   }

   export interface ScanWarning {
     file: string;
     line?: number;
     message: string;
     severity: string;
   }
   ```

   **`scanners/<scanner-name>/src/parser.ts`:**
   ```typescript
   import { ScanInput, ScanOutput, ScanNode, ScanEdge, ScanWarning } from "./types";

   export function scan(input: ScanInput): ScanOutput {
     const nodes: ScanNode[] = [];
     const edges: ScanEdge[] = [];
     const warnings: ScanWarning[] = [];

     // TODO: Implement scanner logic here
     // 1. Read files from input.projectRoot
     // 2. Parse them to discover nodes (routes, entities, pages, actions)
     // 3. Discover edges (relationships between nodes)
     // 4. Return the results

     return {
       version: 1,
       scanner: {
         id: "<scanner-name>",
         name: "<Scanner Name>",
         version: "0.1.0",
       },
       nodes,
       edges,
       warnings,
       stats: {
         filesScanned: 0,
         nodesFound: nodes.length,
         edgesFound: edges.length,
         errors: 0,
       },
     };
   }
   ```

   Replace `<scanner-name>` and `<Scanner Name>` with the actual scanner name (hyphenated and title-cased respectively).

   **`scanners/<scanner-name>/src/index.ts`:**
   ```typescript
   import { ScanInput } from "./types";
   import { scan } from "./parser";

   async function main() {
     let inputData = "";

     process.stdin.setEncoding("utf-8");

     for await (const chunk of process.stdin) {
       inputData += chunk;
     }

     const input: ScanInput = JSON.parse(inputData);
     const output = scan(input);

     process.stdout.write(JSON.stringify(output, null, 2));
   }

   main().catch((err) => {
     console.error("Scanner error:", err.message);
     process.exit(1);
   });
   ```

   **`scanners/<scanner-name>/__tests__/parser.test.ts`:**
   ```typescript
   import { scan } from "../src/parser";
   import { ScanInput } from "../src/types";

   describe("<scanner-name> scanner", () => {
     const baseInput: ScanInput = {
       version: 1,
       projectRoot: "/tmp/test-project",
       options: {},
     };

     it("should return valid ScanOutput structure", () => {
       const result = scan(baseInput);

       expect(result.version).toBe(1);
       expect(result.scanner.id).toBe("<scanner-name>");
       expect(result.nodes).toBeInstanceOf(Array);
       expect(result.edges).toBeInstanceOf(Array);
       expect(result.warnings).toBeInstanceOf(Array);
       expect(result.stats).toBeDefined();
       expect(result.stats.nodesFound).toBe(result.nodes.length);
       expect(result.stats.edgesFound).toBe(result.edges.length);
     });

     // TODO: Add scanner-specific tests here
   });
   ```

   Replace `<scanner-name>` with the actual scanner name.

5. **If language is Go:**

   Create the following file:

   **`scanners/<scanner-name>/main.go`:**
   ```go
   package main

   import (
       "encoding/json"
       "fmt"
       "io"
       "os"
   )

   type ScanInput struct {
       Version     int                    `json:"version"`
       ProjectRoot string                 `json:"projectRoot"`
       Options     map[string]interface{} `json:"options"`
   }

   type ScanOutput struct {
       Version  int           `json:"version"`
       Scanner  ScannerInfo   `json:"scanner"`
       Nodes    []ScanNode    `json:"nodes"`
       Edges    []ScanEdge    `json:"edges"`
       Warnings []ScanWarning `json:"warnings"`
       Stats    ScanStats     `json:"stats"`
   }

   type ScannerInfo struct {
       ID      string `json:"id"`
       Name    string `json:"name"`
       Version string `json:"version"`
   }

   type ScanNode struct {
       ID         string                 `json:"id"`
       Kind       string                 `json:"kind"`
       Name       string                 `json:"name"`
       Label      string                 `json:"label"`
       Properties map[string]interface{} `json:"properties"`
       Source     string                 `json:"source"`
       SourceFile string                 `json:"sourceFile,omitempty"`
   }

   type ScanEdge struct {
       ID         string                 `json:"id"`
       SrcID      string                 `json:"srcId"`
       DstID      string                 `json:"dstId"`
       Kind       string                 `json:"kind"`
       Properties map[string]interface{} `json:"properties,omitempty"`
   }

   type ScanWarning struct {
       File     string `json:"file"`
       Line     int    `json:"line,omitempty"`
       Message  string `json:"message"`
       Severity string `json:"severity"`
   }

   type ScanStats struct {
       FilesScanned int `json:"filesScanned"`
       NodesFound   int `json:"nodesFound"`
       EdgesFound   int `json:"edgesFound"`
       Errors       int `json:"errors"`
   }

   func scan(input ScanInput) ScanOutput {
       // TODO: Implement scanner logic here
       return ScanOutput{
           Version: 1,
           Scanner: ScannerInfo{
               ID:      "<scanner-name>",
               Name:    "<Scanner Name>",
               Version: "0.1.0",
           },
           Nodes:    []ScanNode{},
           Edges:    []ScanEdge{},
           Warnings: []ScanWarning{},
           Stats:    ScanStats{},
       }
   }

   func main() {
       data, err := io.ReadAll(os.Stdin)
       if err != nil {
           fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
           os.Exit(1)
       }

       var input ScanInput
       if err := json.Unmarshal(data, &input); err != nil {
           fmt.Fprintf(os.Stderr, "Error parsing input: %v\n", err)
           os.Exit(1)
       }

       output := scan(input)

       result, err := json.MarshalIndent(output, "", "  ")
       if err != nil {
           fmt.Fprintf(os.Stderr, "Error marshaling output: %v\n", err)
           os.Exit(1)
       }

       fmt.Println(string(result))
   }
   ```

   Replace `<scanner-name>` and `<Scanner Name>` with the actual scanner name.

6. **Register the scanner in `.abacus/config.yaml`** if that file exists in the current working directory (the TARGET project, not the abacus repo). Append a scanner entry under the `scanners:` key:
   - For TypeScript: `command: npx ts-node scanners/<scanner-name>/src/index.ts`
   - For Go: `command: go run scanners/<scanner-name>/main.go`

   If `.abacus/config.yaml` does not exist, tell the user to run `abacus init` first and then manually add the scanner entry.

7. Report what was created and suggest next steps:
   - `cd scanners/<scanner-name> && npm install` (for TypeScript)
   - Edit `src/parser.ts` to implement the scanner logic
   - Run tests with `npm test`
   - Test the scanner: `echo '{"version":1,"projectRoot":".","options":{}}' | npx ts-node src/index.ts`
