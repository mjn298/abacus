import { describe, it, expect, afterEach } from "vitest";
import { Project } from "ts-morph";
import { mkdtempSync, writeFileSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { hasPrismaImport, findPrismaAccesses } from "../src/matcher.js";
import { traceImports } from "../src/tracer.js";
import { findHandlerFiles } from "../src/parser.js";
import { processRoutes } from "../src/parser.js";

// Shared entity map for tests: lowercase model name -> entity node ID
const entityByName = new Map<string, string>([
  ["user", "entity:User"],
  ["post", "entity:Post"],
  ["comment", "entity:Comment"],
]);

describe("matcher", () => {
  describe("hasPrismaImport", () => {
    it("returns true for @prisma/client import", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();`,
      );
      expect(hasPrismaImport(file)).toBe(true);
    });

    it("returns true for PrismaClient type declaration", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `const db: PrismaClient = getClient();`,
      );
      expect(hasPrismaImport(file)).toBe(true);
    });

    it("returns true for new PrismaClient() initializer without type annotation", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `const db = new PrismaClient();`,
      );
      expect(hasPrismaImport(file)).toBe(true);
    });

    it("returns false for unrelated file", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/utils.ts",
        `export function add(a: number, b: number) { return a + b; }`,
      );
      expect(hasPrismaImport(file)).toBe(false);
    });
  });

  describe("findPrismaAccesses", () => {
    it("detects prisma.user.findMany()", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUsers() { return prisma.user.findMany(); }`,
      );

      const result = findPrismaAccesses(file, entityByName);
      expect(result.has("entity:User")).toBe(true);
      expect(result.size).toBe(1);
    });

    it("detects prisma.post.create()", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function createPost(data: any) { return prisma.post.create({ data }); }`,
      );

      const result = findPrismaAccesses(file, entityByName);
      expect(result.has("entity:Post")).toBe(true);
      expect(result.size).toBe(1);
    });

    it("handles custom PrismaClient variable name", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `const db = new PrismaClient();
export function getUsers() { return db.user.findMany(); }`,
      );

      const result = findPrismaAccesses(file, entityByName);
      expect(result.has("entity:User")).toBe(true);
    });

    it("returns empty set for no matches", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/utils.ts",
        `export function add(a: number, b: number) { return a + b; }`,
      );

      const result = findPrismaAccesses(file, entityByName);
      expect(result.size).toBe(0);
    });

    it("deduplicates same entity accessed multiple times", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUser(id: string) { return prisma.user.findUnique({ where: { id } }); }
export function listUsers() { return prisma.user.findMany(); }
export function countUsers() { return prisma.user.count(); }`,
      );

      const result = findPrismaAccesses(file, entityByName);
      expect(result.has("entity:User")).toBe(true);
      expect(result.size).toBe(1);
    });

    it("detects multiple different entities in one file", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const file = project.createSourceFile(
        "/src/repo.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUser() { return prisma.user.findMany(); }
export function getPost() { return prisma.post.findMany(); }`,
      );

      const result = findPrismaAccesses(file, entityByName);
      expect(result.has("entity:User")).toBe(true);
      expect(result.has("entity:Post")).toBe(true);
      expect(result.size).toBe(2);
    });
  });
});

describe("tracer", () => {
  it("traces direct prisma.user.findMany() call", () => {
    const project = new Project({ useInMemoryFileSystem: true });
    const file = project.createSourceFile(
      "/src/handler.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function handler() { return prisma.user.findMany(); }`,
    );

    const result = traceImports(file, entityByName);
    expect(result.entities.has("entity:User")).toBe(true);
    expect(result.entities.get("entity:User")).toEqual({ tracePath: ["/src/handler.ts"], depth: 0 });
    expect(result.filesVisited).toBe(1);
  });

  it("traces 2-hop chain: handler imports service which uses prisma", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // Repository file that uses Prisma directly
    project.createSourceFile(
      "/src/service.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUsers() { return prisma.user.findMany(); }`,
    );

    // Handler file that imports service
    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { getUsers } from "./service";
export function handle() { return getUsers(); }`,
    );

    const result = traceImports(handler, entityByName);
    expect(result.entities.has("entity:User")).toBe(true);
    expect(result.entities.get("entity:User")).toEqual({ tracePath: ["/src/handler.ts", "/src/service.ts"], depth: 1 });
    expect(result.filesVisited).toBe(2);
  });

  it("traces 3-hop chain: handler -> service -> repository -> prisma", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // Repository with Prisma access
    project.createSourceFile(
      "/src/repo.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function findUsers() { return prisma.user.findMany(); }`,
    );

    // Service imports from repo
    project.createSourceFile(
      "/src/service.ts",
      `import { findUsers } from "./repo";
export function getUsers() { return findUsers(); }`,
    );

    // Handler imports from service
    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { getUsers } from "./service";
export function handle() { return getUsers(); }`,
    );

    const result = traceImports(handler, entityByName);
    expect(result.entities.has("entity:User")).toBe(true);
    expect(result.entities.get("entity:User")).toEqual({
      tracePath: ["/src/handler.ts", "/src/service.ts", "/src/repo.ts"],
      depth: 2,
    });
    expect(result.filesVisited).toBe(3);
  });

  it("handles circular imports without infinite loop", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // File A imports from B
    const fileA = project.createSourceFile(
      "/src/a.ts",
      `import { helperB } from "./b";
export function helperA() { return helperB(); }`,
    );

    // File B imports from A (circular)
    project.createSourceFile(
      "/src/b.ts",
      `import { helperA } from "./a";
export function helperB() { return helperA(); }`,
    );

    // Should terminate without hanging or throwing
    const result = traceImports(fileA, entityByName);
    expect(result.entities.size).toBe(0);
    expect(result.filesVisited).toBe(2);
  });

  it("respects maxDepth limit", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // Deep chain: handler -> mid1 -> mid2 -> repo (depth 3)
    project.createSourceFile(
      "/src/repo.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function findUsers() { return prisma.user.findMany(); }`,
    );

    project.createSourceFile(
      "/src/mid2.ts",
      `import { findUsers } from "./repo";
export function mid2() { return findUsers(); }`,
    );

    project.createSourceFile(
      "/src/mid1.ts",
      `import { mid2 } from "./mid2";
export function mid1() { return mid2(); }`,
    );

    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { mid1 } from "./mid1";
export function handle() { return mid1(); }`,
    );

    // With maxDepth=2, we reach handler(0)->mid1(1)->mid2(2), but repo is at depth 3 — skipped
    const result = traceImports(handler, entityByName, { maxDepth: 2 });
    expect(result.entities.has("entity:User")).toBe(false);
  });

  it("discovers multiple entities from one file (fan-out)", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // Repo that accesses both user and post
    project.createSourceFile(
      "/src/repo.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUsers() { return prisma.user.findMany(); }
export function getPosts() { return prisma.post.findMany(); }`,
    );

    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { getUsers, getPosts } from "./repo";
export function handle() { return { users: getUsers(), posts: getPosts() }; }`,
    );

    const result = traceImports(handler, entityByName);
    expect(result.entities.has("entity:User")).toBe(true);
    expect(result.entities.has("entity:Post")).toBe(true);
    expect(result.entities.size).toBe(2);
  });

  it("skips test files", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // A test file that uses prisma
    project.createSourceFile(
      "/src/repo.test.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function testHelper() { return prisma.user.findMany(); }`,
    );

    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { testHelper } from "./repo.test";
export function handle() { return testHelper(); }`,
    );

    const result = traceImports(handler, entityByName);
    // The test file should be skipped, so no entities found through it
    expect(result.entities.has("entity:User")).toBe(false);
  });

  it("skips node_modules imports", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // Handler imports from a package (non-relative import)
    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import express from "express";
import { something } from "@some/package";
export function handle() { return express(); }`,
    );

    // Should not crash and should have visited only the handler itself
    const result = traceImports(handler, entityByName);
    expect(result.entities.size).toBe(0);
    expect(result.filesVisited).toBe(1);
  });

  it("file-level cache prevents redundant analysis", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // Shared repo used by two services
    project.createSourceFile(
      "/src/repo.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function findUsers() { return prisma.user.findMany(); }`,
    );

    // Two separate traces from the same entry point should produce consistent results
    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { findUsers } from "./repo";
export function handle() { return findUsers(); }`,
    );

    const result1 = traceImports(handler, entityByName);
    const result2 = traceImports(handler, entityByName);

    expect(result1.entities.has("entity:User")).toBe(true);
    expect(result2.entities.has("entity:User")).toBe(true);
    expect(result1.filesVisited).toBe(result2.filesVisited);
  });

  describe("depth tracking", () => {
    it("reports depth 0 for direct prisma access in entry file", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      const handler = project.createSourceFile(
        "/src/handler.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function handler() { return prisma.user.findMany(); }`,
      );

      const result = traceImports(handler, entityByName);
      expect(result.entities.get("entity:User")).toEqual({ tracePath: ["/src/handler.ts"], depth: 0 });
    });

    it("reports depth 1 for entity found one hop away", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      project.createSourceFile(
        "/src/service.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUsers() { return prisma.user.findMany(); }`,
      );
      const handler = project.createSourceFile(
        "/src/handler.ts",
        `import { getUsers } from "./service";
export function handle() { return getUsers(); }`,
      );

      const result = traceImports(handler, entityByName);
      expect(result.entities.get("entity:User")).toEqual({
        tracePath: ["/src/handler.ts", "/src/service.ts"],
        depth: 1,
      });
    });

    it("reports depth 2 for entity found two hops away", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      project.createSourceFile(
        "/src/repo.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function findUsers() { return prisma.user.findMany(); }`,
      );
      project.createSourceFile(
        "/src/service.ts",
        `import { findUsers } from "./repo";
export function getUsers() { return findUsers(); }`,
      );
      const handler = project.createSourceFile(
        "/src/handler.ts",
        `import { getUsers } from "./service";
export function handle() { return getUsers(); }`,
      );

      const result = traceImports(handler, entityByName);
      expect(result.entities.get("entity:User")).toEqual({
        tracePath: ["/src/handler.ts", "/src/service.ts", "/src/repo.ts"],
        depth: 2,
      });
    });

    it("reports correct depth per entity in multi-entity traces", () => {
      const project = new Project({ useInMemoryFileSystem: true });
      project.createSourceFile(
        "/src/repo.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function findPosts() { return prisma.post.findMany(); }`,
      );
      project.createSourceFile(
        "/src/serviceA.ts",
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
import { findPosts } from "./repo";
export function getUsers() { return prisma.user.findMany(); }
export function getPosts() { return findPosts(); }`,
      );
      const handler = project.createSourceFile(
        "/src/handler.ts",
        `import { getUsers, getPosts } from "./serviceA";
export function handle() { return { users: getUsers(), posts: getPosts() }; }`,
      );

      const result = traceImports(handler, entityByName);
      expect(result.entities.get("entity:User")).toEqual({
        tracePath: ["/src/handler.ts", "/src/serviceA.ts"],
        depth: 1,
      });
      expect(result.entities.get("entity:Post")).toEqual({
        tracePath: ["/src/handler.ts", "/src/serviceA.ts", "/src/repo.ts"],
        depth: 2,
      });
    });
  });

  it("shared dependency visited through multiple paths returns entities for all paths", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    // Shared repo used by both serviceA and serviceB
    project.createSourceFile(
      "/src/repo.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function findUsers() { return prisma.user.findMany(); }`,
    );

    // serviceA directly imports repo
    project.createSourceFile(
      "/src/serviceA.ts",
      `import { findUsers } from "./repo";
export function getUsers() { return findUsers(); }`,
    );

    // serviceB is reached through serviceA, and also imports repo
    project.createSourceFile(
      "/src/serviceB.ts",
      `import { findUsers } from "./repo";
export function fetchUsers() { return findUsers(); }`,
    );

    // serviceA also imports serviceB (repo is reached through two paths)
    project.createSourceFile(
      "/src/serviceA2.ts",
      `import { getUsers } from "./serviceA";
import { fetchUsers } from "./serviceB";
export function handle() { return { a: getUsers(), b: fetchUsers() }; }`,
    );

    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { handle } from "./serviceA2";
export function main() { return handle(); }`,
    );

    const cache = new Map<string, { entities: Set<string>; remainingDepth: number }>();
    const result = traceImports(handler, entityByName, {}, cache);

    // repo.ts is reached via handler → serviceA2 → serviceA → repo (visited first)
    // AND via handler → serviceA2 → serviceB → repo (visited check)
    // Both paths should find the User entity
    expect(result.entities.has("entity:User")).toBe(true);

    // serviceB should also see repo's entities via cache
    const serviceBCache = cache.get("/src/serviceB.ts");
    expect(serviceBCache).toBeDefined();
    expect(serviceBCache!.entities.has("entity:User")).toBe(true);
  });

  it("skips __tests__ directory files", () => {
    const project = new Project({ useInMemoryFileSystem: true });

    project.createSourceFile(
      "/src/__tests__/helper.ts",
      `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function testUtil() { return prisma.user.findMany(); }`,
    );

    const handler = project.createSourceFile(
      "/src/handler.ts",
      `import { testUtil } from "./__tests__/helper";
export function handle() { return testUtil(); }`,
    );

    const result = traceImports(handler, entityByName);
    expect(result.entities.has("entity:User")).toBe(false);
  });
});

describe("reverse-import handler resolution", () => {
  describe("findHandlerFiles", () => {
    it("finds handler that imports contract and calls .handler()", () => {
      const project = new Project({ useInMemoryFileSystem: true });

      const contract = project.createSourceFile(
        "/src/contracts/user.ts",
        `export const userContract = { path: "/users" };`,
      );

      project.createSourceFile(
        "/src/handlers/user.handlers.ts",
        `import { userContract } from "../contracts/user";
export const userHandler = userContract.handler(() => { /* impl */ });`,
      );

      const handlers = findHandlerFiles(contract, project);
      expect(handlers.length).toBe(1);
      expect(handlers[0].getFilePath()).toContain("user.handlers.ts");
    });

    it("finds handler that imports contract and calls .func()", () => {
      const project = new Project({ useInMemoryFileSystem: true });

      const contract = project.createSourceFile(
        "/src/contracts/post.ts",
        `export const postContract = { path: "/posts" };`,
      );

      project.createSourceFile(
        "/src/handlers/post.handlers.ts",
        `import { postContract } from "../contracts/post";
export const postHandler = postContract.func(async () => { /* impl */ });`,
      );

      const handlers = findHandlerFiles(contract, project);
      expect(handlers.length).toBe(1);
      expect(handlers[0].getFilePath()).toContain("post.handlers.ts");
    });

    it("ignores non-handler importers (no .handler()/.func() call)", () => {
      const project = new Project({ useInMemoryFileSystem: true });

      const contract = project.createSourceFile(
        "/src/contracts/user.ts",
        `export const userContract = { path: "/users" };`,
      );

      // This file imports the contract but only uses it as a type — no .handler()/.func()
      project.createSourceFile(
        "/src/client/api.ts",
        `import { userContract } from "../contracts/user";
export type UserApi = typeof userContract;`,
      );

      const handlers = findHandlerFiles(contract, project);
      expect(handlers.length).toBe(0);
    });

    it("returns empty array when no files import the contract", () => {
      const project = new Project({ useInMemoryFileSystem: true });

      const contract = project.createSourceFile(
        "/src/contracts/orphan.ts",
        `export const orphanContract = {};`,
      );

      // No other files exist that import this contract
      const handlers = findHandlerFiles(contract, project);
      expect(handlers.length).toBe(0);
    });

    it("finds multiple handler files that import the same contract", () => {
      const project = new Project({ useInMemoryFileSystem: true });

      const contract = project.createSourceFile(
        "/src/contracts/user.ts",
        `export const userList = { path: "/users" };
export const userDetail = { path: "/users/:id" };`,
      );

      project.createSourceFile(
        "/src/handlers/user-list.handler.ts",
        `import { userList } from "../contracts/user";
export const listHandler = userList.handler(() => { /* list */ });`,
      );

      project.createSourceFile(
        "/src/handlers/user-detail.handler.ts",
        `import { userDetail } from "../contracts/user";
export const detailHandler = userDetail.handler(() => { /* detail */ });`,
      );

      const handlers = findHandlerFiles(contract, project);
      expect(handlers.length).toBe(2);
    });
  });

  describe("processRoutes with reverse-import resolution", () => {
    let tmpDir: string;

    afterEach(() => {
      if (tmpDir) {
        rmSync(tmpDir, { recursive: true, force: true });
      }
    });

    it("resolves handler file when contract yields no entities", () => {
      tmpDir = mkdtempSync(join(tmpdir(), "linker-rev-import-"));

      // Create directory structure
      mkdirSync(join(tmpDir, "src", "contracts"), { recursive: true });
      mkdirSync(join(tmpDir, "src", "handlers"), { recursive: true });
      mkdirSync(join(tmpDir, "src", "services"), { recursive: true });

      // Contract file — no Prisma usage, just defines the API shape
      writeFileSync(
        join(tmpDir, "src", "contracts", "user.ts"),
        `export const userContract = { path: "/users" };`,
      );

      // Service file — uses Prisma to access User entity
      writeFileSync(
        join(tmpDir, "src", "services", "user.service.ts"),
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUsers() { return prisma.user.findMany(); }`,
      );

      // Handler file — imports contract AND service
      writeFileSync(
        join(tmpDir, "src", "handlers", "user.handlers.ts"),
        `import { userContract } from "../contracts/user";
import { getUsers } from "../services/user.service";
export const userHandler = userContract.handler(() => {
  return getUsers();
});`,
      );

      // Minimal tsconfig
      writeFileSync(
        join(tmpDir, "tsconfig.json"),
        JSON.stringify({
          compilerOptions: {
            module: "ESNext",
            moduleResolution: "Node16",
            target: "ESNext",
            esModuleInterop: true,
            noEmit: true,
          },
          include: ["src/**/*.ts"],
        }),
      );

      const routeNodes = new Map([
        [
          "route:/users",
          {
            id: "route:/users",
            name: "/users",
            sourceFile: "src/contracts/user.ts",
            kind: "route" as const,
          },
        ],
      ]);

      const testEntityByName = new Map<string, string>([
        ["user", "entity:User"],
      ]);

      const result = processRoutes(routeNodes, testEntityByName, tmpDir);

      // The contract file has no Prisma usage, so reverse-import should kick in,
      // find user.handlers.ts (which imports the contract and calls .handler()),
      // trace from there through the service to discover entity:User
      expect(result.edges.length).toBe(1);
      expect(result.edges[0].srcId).toBe("route:/users");
      expect(result.edges[0].dstId).toBe("entity:User");
      expect(result.edges[0].properties?.resolvedVia).toBe("reverse-import");
    });

    it("does not use reverse resolution when contract itself yields entities", () => {
      tmpDir = mkdtempSync(join(tmpdir(), "linker-rev-import-"));

      mkdirSync(join(tmpDir, "src"), { recursive: true });

      // Source file that directly uses Prisma (not a typical contract, but tests the bypass)
      writeFileSync(
        join(tmpDir, "src", "direct-handler.ts"),
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function handler() { return prisma.user.findMany(); }`,
      );

      writeFileSync(
        join(tmpDir, "tsconfig.json"),
        JSON.stringify({
          compilerOptions: {
            module: "ESNext",
            moduleResolution: "Node16",
            target: "ESNext",
            esModuleInterop: true,
            noEmit: true,
          },
          include: ["src/**/*.ts"],
        }),
      );

      const routeNodes = new Map([
        [
          "route:/direct",
          {
            id: "route:/direct",
            name: "/direct",
            sourceFile: "src/direct-handler.ts",
            kind: "route" as const,
          },
        ],
      ]);

      const testEntityByName = new Map<string, string>([
        ["user", "entity:User"],
      ]);

      const result = processRoutes(routeNodes, testEntityByName, tmpDir);

      // Direct trace finds entities, so reverse-import should NOT be used
      expect(result.edges.length).toBe(1);
      expect(result.edges[0].dstId).toBe("entity:User");
      // No resolvedVia property — this was a direct trace
      expect(result.edges[0].properties?.resolvedVia).toBeUndefined();
    });

    it("traces through multiple handler files found via reverse resolution", () => {
      tmpDir = mkdtempSync(join(tmpdir(), "linker-rev-import-"));

      mkdirSync(join(tmpDir, "src", "contracts"), { recursive: true });
      mkdirSync(join(tmpDir, "src", "handlers"), { recursive: true });
      mkdirSync(join(tmpDir, "src", "services"), { recursive: true });

      // Contract file
      writeFileSync(
        join(tmpDir, "src", "contracts", "data.ts"),
        `export const dataContract = { path: "/data" };`,
      );

      // Service A — accesses User
      writeFileSync(
        join(tmpDir, "src", "services", "user.service.ts"),
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getUsers() { return prisma.user.findMany(); }`,
      );

      // Service B — accesses Post
      writeFileSync(
        join(tmpDir, "src", "services", "post.service.ts"),
        `import { PrismaClient } from "@prisma/client";
const prisma = new PrismaClient();
export function getPosts() { return prisma.post.findMany(); }`,
      );

      // Handler A — imports contract + user service
      writeFileSync(
        join(tmpDir, "src", "handlers", "data-users.handler.ts"),
        `import { dataContract } from "../contracts/data";
import { getUsers } from "../services/user.service";
export const handler = dataContract.handler(() => getUsers());`,
      );

      // Handler B — imports contract + post service
      writeFileSync(
        join(tmpDir, "src", "handlers", "data-posts.handler.ts"),
        `import { dataContract } from "../contracts/data";
import { getPosts } from "../services/post.service";
export const handler = dataContract.func(() => getPosts());`,
      );

      writeFileSync(
        join(tmpDir, "tsconfig.json"),
        JSON.stringify({
          compilerOptions: {
            module: "ESNext",
            moduleResolution: "Node16",
            target: "ESNext",
            esModuleInterop: true,
            noEmit: true,
          },
          include: ["src/**/*.ts"],
        }),
      );

      const routeNodes = new Map([
        [
          "route:/data",
          {
            id: "route:/data",
            name: "/data",
            sourceFile: "src/contracts/data.ts",
            kind: "route" as const,
          },
        ],
      ]);

      const testEntityByName = new Map<string, string>([
        ["user", "entity:User"],
        ["post", "entity:Post"],
      ]);

      const result = processRoutes(routeNodes, testEntityByName, tmpDir);

      // Both handler files should be discovered via reverse resolution,
      // yielding edges to both User and Post entities
      const entityIds = result.edges.map((e) => e.dstId).sort();
      expect(entityIds).toContain("entity:Post");
      expect(entityIds).toContain("entity:User");
      expect(result.edges.length).toBe(2);
    });
  });
});
