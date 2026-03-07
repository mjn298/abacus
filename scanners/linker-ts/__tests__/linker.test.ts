import { describe, it, expect } from "vitest";
import { Project } from "ts-morph";
import { hasPrismaImport, findPrismaAccesses } from "../src/matcher.js";
import { traceImports } from "../src/tracer.js";

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
    expect(result.entities.get("entity:User")).toEqual(["/src/handler.ts"]);
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
    const tracePath = result.entities.get("entity:User")!;
    expect(tracePath).toEqual(["/src/handler.ts", "/src/service.ts"]);
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
    const tracePath = result.entities.get("entity:User")!;
    expect(tracePath).toEqual([
      "/src/handler.ts",
      "/src/service.ts",
      "/src/repo.ts",
    ]);
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
