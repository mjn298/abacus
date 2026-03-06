import { describe, it, expect } from 'vitest';
import { join } from 'path';
import { parseOrpcContracts } from '../src/parser.js';

const FIXTURES_DIR = join(import.meta.dirname, 'fixtures');

describe('parseOrpcContracts', () => {
  it('parses a simple .route() call with input and output', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/auth.ts'],
    });

    expect(result.version).toBe(1);
    expect(result.scanner.id).toBe('orpc');
    expect(result.nodes).toHaveLength(1);

    const node = result.nodes[0];
    expect(node.id).toBe('route:POST-/auth/register');
    expect(node.kind).toBe('route');
    expect(node.name).toBe('POST /auth/register');
    expect(node.label).toBe('Register a new user');
    expect(node.source).toBe('scan');
    expect(node.sourceFile).toBe('contracts/auth.ts');
    expect(node.properties).toMatchObject({
      method: 'POST',
      path: '/auth/register',
      summary: 'Register a new user',
      tags: ['Auth'],
      successStatus: 201,
      inputSchema: 'RegisterInputSchema',
      outputSchema: 'AuthResponseSchema',
      contractFile: 'contracts/auth.ts',
    });
  });

  it('handles all HTTP methods (GET, POST, PUT, DELETE, PATCH)', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    const methods = result.nodes.map((n) => n.properties.method);
    expect(methods).toContain('GET');
    expect(methods).toContain('PUT');
    expect(methods).toContain('DELETE');
    expect(methods).toContain('PATCH');
  });

  it('extracts tags, summary, and successStatus', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    const deleteNode = result.nodes.find(
      (n) => n.properties.method === 'DELETE'
    );
    expect(deleteNode).toBeDefined();
    expect(deleteNode!.properties.summary).toBe('Delete a user');
    expect(deleteNode!.properties.tags).toEqual(['Users']);
    expect(deleteNode!.properties.successStatus).toBe(204);
  });

  it('extracts input and output schema names', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    const listNode = result.nodes.find(
      (n) => n.properties.path === '/users' && n.properties.method === 'GET'
    );
    expect(listNode).toBeDefined();
    expect(listNode!.properties.inputSchema).toBe('ListUsersInputSchema');
    expect(listNode!.properties.outputSchema).toBe('ListUsersOutputSchema');
  });

  it('handles contracts without input', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    const getNode = result.nodes.find(
      (n) =>
        n.properties.path === '/users/{id}' && n.properties.method === 'GET'
    );
    expect(getNode).toBeDefined();
    expect(getNode!.properties.inputSchema).toBeUndefined();
    expect(getNode!.properties.outputSchema).toBe('UserSchema');
  });

  it('handles contracts without output', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    const deleteNode = result.nodes.find(
      (n) => n.properties.method === 'DELETE'
    );
    expect(deleteNode).toBeDefined();
    expect(deleteNode!.properties.outputSchema).toBeUndefined();
  });

  it('handles contracts without input (PATCH has input but no output)', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    const patchNode = result.nodes.find(
      (n) => n.properties.method === 'PATCH'
    );
    expect(patchNode).toBeDefined();
    expect(patchNode!.properties.inputSchema).toBe('UpdateUserInputSchema');
    expect(patchNode!.properties.outputSchema).toBeUndefined();
  });

  it('handles multiple contracts per file', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    expect(result.nodes.length).toBe(5);
  });

  it('generates deterministic IDs (route:{METHOD}-{path})', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/auth.ts'],
    });

    expect(result.nodes[0].id).toBe('route:POST-/auth/register');

    // Run again — same IDs
    const result2 = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/auth.ts'],
    });
    expect(result2.nodes[0].id).toBe(result.nodes[0].id);
  });

  it('uses summary as label when available, otherwise name', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/users.ts'],
    });

    const patchNode = result.nodes.find(
      (n) => n.properties.method === 'PATCH'
    );
    expect(patchNode).toBeDefined();
    // PATCH has no summary, so label should be the name
    expect(patchNode!.label).toBe(patchNode!.name);

    const getNode = result.nodes.find(
      (n) =>
        n.properties.path === '/users/{id}' && n.properties.method === 'GET'
    );
    expect(getNode).toBeDefined();
    expect(getNode!.label).toBe('Get user by ID');
  });

  it('skips files with no contracts', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/empty.ts'],
    });

    expect(result.nodes).toHaveLength(0);
    expect(result.stats.filesScanned).toBe(1);
    expect(result.stats.nodesFound).toBe(0);
  });

  it('handles multiple globs', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/auth.ts', 'contracts/users.ts'],
    });

    expect(result.nodes.length).toBe(6); // 1 from auth + 5 from users
    expect(result.stats.filesScanned).toBe(2);
  });

  it('reports correct stats', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/**/*.ts'],
    });

    expect(result.stats.filesScanned).toBe(3);
    expect(result.stats.nodesFound).toBe(6);
    expect(result.stats.edgesFound).toBe(0);
    expect(result.stats.errors).toBe(0);
    expect(result.stats.durationMs).toBeGreaterThanOrEqual(0);
  });

  it('returns empty edges array', async () => {
    const result = await parseOrpcContracts(FIXTURES_DIR, {
      contractGlobs: ['contracts/auth.ts'],
    });

    expect(result.edges).toEqual([]);
  });
});
