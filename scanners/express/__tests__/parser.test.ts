import { describe, it, expect } from 'vitest';
import { join } from 'path';
import { parseExpressRoutes } from '../src/parser.js';

const FIXTURES_DIR = join(import.meta.dirname, 'fixtures');

describe('parseExpressRoutes', () => {
  // --- Direct routes (router.get, router.post, etc.) ---

  it('parses simple GET route', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    const getUsers = result.nodes.find((n) => n.id === 'route:GET-/users');
    expect(getUsers).toBeDefined();
    expect(getUsers!.kind).toBe('route');
    expect(getUsers!.name).toBe('GET /users');
    expect(getUsers!.label).toBe('GET /users');
    expect(getUsers!.source).toBe('scan');
    expect(getUsers!.sourceFile).toBe('routes/users.ts');
    expect(getUsers!.properties).toMatchObject({
      method: 'GET',
      path: '/users',
      middleware: [],
      handler: 'getUsers',
    });
  });

  it('parses POST route with middleware', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    const postUsers = result.nodes.find((n) => n.id === 'route:POST-/users');
    expect(postUsers).toBeDefined();
    expect(postUsers!.properties).toMatchObject({
      method: 'POST',
      path: '/users',
      middleware: ['validateBody'],
      handler: 'createUser',
    });
  });

  it('parses parameterized routes (:id)', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    const getById = result.nodes.find(
      (n) => n.id === 'route:GET-/users/:id'
    );
    expect(getById).toBeDefined();
    expect(getById!.name).toBe('GET /users/:id');
    expect(getById!.properties.path).toBe('/users/:id');
    expect(getById!.properties.handler).toBe('getUserById');
  });

  it('extracts multiple middleware in order', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    const putUser = result.nodes.find(
      (n) => n.id === 'route:PUT-/users/:id'
    );
    expect(putUser).toBeDefined();
    expect(putUser!.properties.middleware).toEqual([
      'requireAuth',
      'validateBody',
    ]);
    expect(putUser!.properties.handler).toBe('updateUser');
  });

  it('handles all HTTP methods (GET, POST, PUT, DELETE)', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    const methods = result.nodes.map((n) => n.properties.method);
    expect(methods).toContain('GET');
    expect(methods).toContain('POST');
    expect(methods).toContain('PUT');
    expect(methods).toContain('DELETE');
  });

  it('finds all routes in users.ts (5 total)', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    expect(result.nodes).toHaveLength(5);
  });

  // --- Middleware with function calls ---

  it('extracts function-call middleware names (e.g., requireRole)', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/admin.ts'],
    });

    const getAdmin = result.nodes.find(
      (n) => n.id === 'route:GET-/admin/users'
    );
    expect(getAdmin).toBeDefined();
    expect(getAdmin!.properties.middleware).toEqual([
      'requireAuth',
      'requireRole',
    ]);
    expect(getAdmin!.properties.handler).toBe('listAllUsers');
  });

  it('extracts multiple function-call middleware', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/admin.ts'],
    });

    const postAdmin = result.nodes.find(
      (n) => n.id === 'route:POST-/admin/users'
    );
    expect(postAdmin).toBeDefined();
    expect(postAdmin!.properties.middleware).toEqual([
      'requireAuth',
      'requireRole',
      'rateLimiter',
    ]);
    expect(postAdmin!.properties.handler).toBe('createAdminUser');
  });

  // --- router.route() chaining ---

  it('parses router.route() chained GET', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/chained.ts'],
    });

    const getPost = result.nodes.find(
      (n) => n.id === 'route:GET-/posts/:id'
    );
    expect(getPost).toBeDefined();
    expect(getPost!.properties).toMatchObject({
      method: 'GET',
      path: '/posts/:id',
      middleware: [],
      handler: 'getPost',
    });
  });

  it('parses router.route() chained PUT with middleware', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/chained.ts'],
    });

    const putPost = result.nodes.find(
      (n) => n.id === 'route:PUT-/posts/:id'
    );
    expect(putPost).toBeDefined();
    expect(putPost!.properties).toMatchObject({
      method: 'PUT',
      path: '/posts/:id',
      middleware: ['requireAuth'],
      handler: 'updatePost',
    });
  });

  it('parses router.route() chained DELETE with multiple middleware', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/chained.ts'],
    });

    const deletePost = result.nodes.find(
      (n) => n.id === 'route:DELETE-/posts/:id'
    );
    expect(deletePost).toBeDefined();
    expect(deletePost!.properties).toMatchObject({
      method: 'DELETE',
      path: '/posts/:id',
      middleware: ['requireAuth', 'requireRole'],
      handler: 'deletePost',
    });
  });

  it('finds all chained routes (5 total: 3 for /posts/:id, 2 for /posts)', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/chained.ts'],
    });

    expect(result.nodes).toHaveLength(5);
  });

  // --- Edge cases ---

  it('handles files with no routes gracefully', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/empty.ts'],
    });

    expect(result.nodes).toHaveLength(0);
    expect(result.stats.filesScanned).toBe(1);
    expect(result.stats.nodesFound).toBe(0);
  });

  it('generates deterministic IDs', async () => {
    const result1 = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });
    const result2 = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    expect(result1.nodes.map((n) => n.id)).toEqual(
      result2.nodes.map((n) => n.id)
    );
  });

  // --- Output structure ---

  it('returns correct scanner info', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    expect(result.version).toBe(1);
    expect(result.scanner).toEqual({
      id: 'express',
      name: 'Express Route Scanner',
      version: '0.1.0',
    });
  });

  it('returns empty edges array', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts'],
    });

    expect(result.edges).toEqual([]);
  });

  it('reports correct stats', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/**/*.ts'],
    });

    expect(result.stats.filesScanned).toBe(4); // users, admin, chained, empty
    expect(result.stats.nodesFound).toBe(13); // 5 + 3 + 5
    expect(result.stats.edgesFound).toBe(0);
    expect(result.stats.errors).toBe(0);
    expect(result.stats.durationMs).toBeGreaterThanOrEqual(0);
  });

  it('handles multiple globs', async () => {
    const result = await parseExpressRoutes(FIXTURES_DIR, {
      routeGlobs: ['routes/users.ts', 'routes/admin.ts'],
    });

    expect(result.nodes).toHaveLength(8); // 5 + 3
    expect(result.stats.filesScanned).toBe(2);
  });
});
