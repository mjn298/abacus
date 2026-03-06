import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { parsePrismaSchema, buildScanOutput } from "../src/parser.js";

const __dirname = dirname(fileURLToPath(import.meta.url));
const fixtureSchema = readFileSync(
  join(__dirname, "fixtures", "schema.prisma"),
  "utf-8",
);

describe("parsePrismaSchema", () => {
  const result = parsePrismaSchema(fixtureSchema);

  describe("model parsing", () => {
    it("extracts all models", () => {
      const modelNames = result.models.map((m) => m.name);
      expect(modelNames).toContain("User");
      expect(modelNames).toContain("Company");
      expect(modelNames).toContain("Post");
      expect(modelNames).toContain("Profile");
      expect(modelNames).toContain("Tag");
      expect(result.models).toHaveLength(5);
    });

    it("parses scalar fields with correct types", () => {
      const user = result.models.find((m) => m.name === "User")!;
      const emailField = user.fields.find((f) => f.name === "email")!;
      expect(emailField.type).toBe("String");
      expect(emailField.isRequired).toBe(true);
      expect(emailField.isUnique).toBe(true);
      expect(emailField.isId).toBe(false);
    });

    it("parses optional fields", () => {
      const user = result.models.find((m) => m.name === "User")!;
      const nameField = user.fields.find((f) => f.name === "name")!;
      expect(nameField.type).toBe("String");
      expect(nameField.isRequired).toBe(false);
    });

    it("parses @id fields", () => {
      const user = result.models.find((m) => m.name === "User")!;
      const idField = user.fields.find((f) => f.name === "id")!;
      expect(idField.isId).toBe(true);
      expect(idField.hasDefault).toBe(true);
    });

    it("parses @default fields", () => {
      const post = result.models.find((m) => m.name === "Post")!;
      const publishedField = post.fields.find((f) => f.name === "published")!;
      expect(publishedField.hasDefault).toBe(true);
      expect(publishedField.defaultValue).toBe("false");
    });

    it("parses @updatedAt fields", () => {
      const user = result.models.find((m) => m.name === "User")!;
      const updatedAtField = user.fields.find((f) => f.name === "updatedAt")!;
      expect(updatedAtField.isUpdatedAt).toBe(true);
    });

    it("parses list fields", () => {
      const user = result.models.find((m) => m.name === "User")!;
      const postsField = user.fields.find((f) => f.name === "posts")!;
      expect(postsField.isList).toBe(true);
      expect(postsField.type).toBe("Post");
    });

    it("detects @@map table name", () => {
      const user = result.models.find((m) => m.name === "User")!;
      expect(user.tableName).toBe("users");

      const post = result.models.find((m) => m.name === "Post")!;
      expect(post.tableName).toBe("posts");
    });

    it("has no @@map for models without it", () => {
      const company = result.models.find((m) => m.name === "Company")!;
      expect(company.tableName).toBeUndefined();
    });
  });

  describe("relation parsing", () => {
    it("parses @relation with fields and references", () => {
      const user = result.models.find((m) => m.name === "User")!;
      const companyField = user.fields.find((f) => f.name === "company")!;
      expect(companyField.type).toBe("Company");
      expect(companyField.relationFields).toEqual(["companyId"]);
      expect(companyField.relationReferences).toEqual(["id"]);
    });

    it("parses one-to-one relation (owning side)", () => {
      const profile = result.models.find((m) => m.name === "Profile")!;
      const userField = profile.fields.find((f) => f.name === "user")!;
      expect(userField.type).toBe("User");
      expect(userField.relationFields).toEqual(["userId"]);
      expect(userField.isList).toBe(false);
    });

    it("parses one-to-many relation (inverse side)", () => {
      const user = result.models.find((m) => m.name === "User")!;
      const postsField = user.fields.find((f) => f.name === "posts")!;
      expect(postsField.type).toBe("Post");
      expect(postsField.isList).toBe(true);
      expect(postsField.relationFields).toBeUndefined();
    });
  });

  describe("enum parsing", () => {
    it("extracts all enums", () => {
      const enumNames = result.enums.map((e) => e.name);
      expect(enumNames).toContain("Role");
      expect(enumNames).toContain("Status");
      expect(result.enums).toHaveLength(2);
    });

    it("extracts enum values", () => {
      const role = result.enums.find((e) => e.name === "Role")!;
      expect(role.values).toEqual(["ADMIN", "USER", "MANAGER"]);

      const status = result.enums.find((e) => e.name === "Status")!;
      expect(status.values).toEqual(["ACTIVE", "INACTIVE", "PENDING"]);
    });
  });
});

describe("buildScanOutput", () => {
  const parseResult = parsePrismaSchema(fixtureSchema);
  const output = buildScanOutput(parseResult, "prisma/schema.prisma", 1);

  it("has correct version and scanner info", () => {
    expect(output.version).toBe(1);
    expect(output.scanner).toEqual({
      id: "prisma",
      name: "Prisma Entity Scanner",
      version: "0.1.0",
    });
  });

  describe("entity nodes", () => {
    it("creates entity nodes for all models", () => {
      const entityNodes = output.nodes.filter(
        (n) => n.kind === "entity" && !n.properties.isEnum,
      );
      expect(entityNodes).toHaveLength(5);
    });

    it("creates entity nodes for all enums", () => {
      const enumNodes = output.nodes.filter(
        (n) => n.kind === "entity" && n.properties.isEnum === true,
      );
      expect(enumNodes).toHaveLength(2);
    });

    it("model node has correct structure", () => {
      const userNode = output.nodes.find((n) => n.id === "entity:User")!;
      expect(userNode).toBeDefined();
      expect(userNode.kind).toBe("entity");
      expect(userNode.name).toBe("User");
      expect(userNode.label).toBe("User");
      expect(userNode.source).toBe("scan");
      expect(userNode.sourceFile).toBe("prisma/schema.prisma");
      expect(userNode.properties.tableName).toBe("users");
      expect(userNode.properties.isEnum).toBe(false);
      expect(Array.isArray(userNode.properties.fields)).toBe(true);
    });

    it("model node fields contain field metadata", () => {
      const userNode = output.nodes.find((n) => n.id === "entity:User")!;
      const fields = userNode.properties.fields as Array<Record<string, unknown>>;
      const emailField = fields.find((f) => f.name === "email");
      expect(emailField).toBeDefined();
      expect(emailField!.type).toBe("String");
      expect(emailField!.isRequired).toBe(true);
      expect(emailField!.isUnique).toBe(true);
    });

    it("model node has relations array", () => {
      const userNode = output.nodes.find((n) => n.id === "entity:User")!;
      const relations = userNode.properties.relations as Array<Record<string, unknown>>;
      expect(Array.isArray(relations)).toBe(true);
      expect(relations.length).toBeGreaterThan(0);
      const companyRel = relations.find((r) => r.fieldName === "company");
      expect(companyRel).toBeDefined();
      expect(companyRel!.relatedModel).toBe("Company");
    });

    it("enum node has correct structure", () => {
      const roleNode = output.nodes.find((n) => n.id === "entity:Role")!;
      expect(roleNode).toBeDefined();
      expect(roleNode.kind).toBe("entity");
      expect(roleNode.name).toBe("Role");
      expect(roleNode.properties.isEnum).toBe(true);
      expect(roleNode.properties.values).toEqual(["ADMIN", "USER", "MANAGER"]);
    });
  });

  describe("relation edges", () => {
    it("creates edges only for owning side (fields with @relation)", () => {
      // Owning sides: User->Company, Post->User, Profile->User
      const edges = output.edges;
      expect(edges.length).toBe(3);
    });

    it("User->Company edge has correct structure", () => {
      const edge = output.edges.find(
        (e) => e.srcId === "entity:User" && e.dstId === "entity:Company",
      )!;
      expect(edge).toBeDefined();
      expect(edge.kind).toBe("field_relation");
      expect(edge.properties!.fieldName).toBe("company");
      expect(edge.properties!.relationType).toBe("many-to-one");
    });

    it("Post->User edge exists", () => {
      const edge = output.edges.find(
        (e) => e.srcId === "entity:Post" && e.dstId === "entity:User",
      )!;
      expect(edge).toBeDefined();
      expect(edge.properties!.fieldName).toBe("author");
      expect(edge.properties!.relationType).toBe("many-to-one");
    });

    it("Profile->User edge is one-to-one", () => {
      const edge = output.edges.find(
        (e) => e.srcId === "entity:Profile" && e.dstId === "entity:User",
      )!;
      expect(edge).toBeDefined();
      expect(edge.properties!.fieldName).toBe("user");
      expect(edge.properties!.relationType).toBe("one-to-one");
    });

    it("edge id follows naming convention", () => {
      const edge = output.edges.find(
        (e) => e.srcId === "entity:User" && e.dstId === "entity:Company",
      )!;
      expect(edge.id).toBe(
        "edge:entity:User-field_relation-entity:Company",
      );
    });
  });

  describe("stats", () => {
    it("has correct stats", () => {
      expect(output.stats.filesScanned).toBe(1);
      expect(output.stats.nodesFound).toBe(output.nodes.length);
      expect(output.stats.edgesFound).toBe(output.edges.length);
      expect(output.stats.errors).toBe(0);
      expect(output.stats.durationMs).toBeGreaterThanOrEqual(0);
    });
  });

  describe("protocol conformance", () => {
    it("all required top-level fields present", () => {
      expect(output).toHaveProperty("version");
      expect(output).toHaveProperty("scanner");
      expect(output).toHaveProperty("nodes");
      expect(output).toHaveProperty("edges");
      expect(output).toHaveProperty("warnings");
      expect(output).toHaveProperty("stats");
    });

    it("all node fields are present", () => {
      for (const node of output.nodes) {
        expect(node).toHaveProperty("id");
        expect(node).toHaveProperty("kind");
        expect(node).toHaveProperty("name");
        expect(node).toHaveProperty("label");
        expect(node).toHaveProperty("properties");
        expect(node).toHaveProperty("source");
        expect(node).toHaveProperty("sourceFile");
      }
    });

    it("all edge fields are present", () => {
      for (const edge of output.edges) {
        expect(edge).toHaveProperty("id");
        expect(edge).toHaveProperty("srcId");
        expect(edge).toHaveProperty("dstId");
        expect(edge).toHaveProperty("kind");
      }
    });
  });
});
