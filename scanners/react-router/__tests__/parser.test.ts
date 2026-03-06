import { describe, it, expect } from 'vitest';
import { join } from 'path';
import { parseReactRouterPages } from '../src/parser.js';

const FIXTURES_DIR = join(import.meta.dirname, 'fixtures');

describe('parseReactRouterPages', () => {
  it('parses a basic route with path and element', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const dashboard = result.nodes.find((n) => n.name === '/dashboard');
    expect(dashboard).toBeDefined();
    expect(dashboard!.id).toBe('page:/dashboard');
    expect(dashboard!.kind).toBe('page');
    expect(dashboard!.label).toBe('Dashboard');
    expect(dashboard!.source).toBe('scan');
    expect(dashboard!.sourceFile).toBe('App.tsx');
    expect(dashboard!.properties).toMatchObject({
      path: '/dashboard',
      component: 'Dashboard',
      isProtected: false,
      isIndex: false,
    });
  });

  it('parses routes with dynamic segments', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const userProfile = result.nodes.find((n) => n.name === '/users/:id');
    expect(userProfile).toBeDefined();
    expect(userProfile!.id).toBe('page:/users/:id');
    expect(userProfile!.properties.path).toBe('/users/:id');
    expect(userProfile!.properties.component).toBe('UserProfile');
  });

  it('parses index routes', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const indexRoute = result.nodes.find(
      (n) => n.properties.isIndex === true && n.properties.component === 'Home'
    );
    expect(indexRoute).toBeDefined();
    expect(indexRoute!.id).toBe('page:/');
    expect(indexRoute!.name).toBe('/');
    expect(indexRoute!.properties.isIndex).toBe(true);
  });

  it('composes paths for nested routes', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const profile = result.nodes.find(
      (n) => n.properties.component === 'Profile'
    );
    expect(profile).toBeDefined();
    expect(profile!.id).toBe('page:/settings/profile');
    expect(profile!.name).toBe('/settings/profile');
    expect(profile!.properties.path).toBe('/settings/profile');
    expect(profile!.properties.parentPath).toBe('/settings');

    const billing = result.nodes.find(
      (n) => n.properties.component === 'Billing'
    );
    expect(billing).toBeDefined();
    expect(billing!.id).toBe('page:/settings/billing');
    expect(billing!.name).toBe('/settings/billing');
    expect(billing!.properties.parentPath).toBe('/settings');
  });

  it('includes the layout route itself as a node', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const settingsLayout = result.nodes.find(
      (n) => n.properties.component === 'SettingsLayout'
    );
    expect(settingsLayout).toBeDefined();
    expect(settingsLayout!.id).toBe('page:/settings');
    expect(settingsLayout!.name).toBe('/settings');
  });

  it('parses catch-all routes', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const notFound = result.nodes.find(
      (n) => n.properties.component === 'NotFound'
    );
    expect(notFound).toBeDefined();
    expect(notFound!.id).toBe('page:*');
    expect(notFound!.name).toBe('*');
    expect(notFound!.properties.path).toBe('*');
  });

  it('detects protected routes via ternary with Navigate', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const admin = result.nodes.find(
      (n) => n.properties.component === 'AdminPanel'
    );
    expect(admin).toBeDefined();
    expect(admin!.properties.isProtected).toBe(true);

    const adminUsers = result.nodes.find(
      (n) => n.properties.component === 'AdminUsers'
    );
    expect(adminUsers).toBeDefined();
    expect(adminUsers!.properties.isProtected).toBe(true);
  });

  it('marks non-protected routes as isProtected: false', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const dashboard = result.nodes.find((n) => n.name === '/dashboard');
    expect(dashboard!.properties.isProtected).toBe(false);

    const login = result.nodes.find((n) => n.name === '/login');
    expect(login!.properties.isProtected).toBe(false);
  });

  it('handles files with no routes', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['Empty.tsx'],
    });

    expect(result.nodes).toHaveLength(0);
    expect(result.stats.filesScanned).toBe(1);
    expect(result.stats.nodesFound).toBe(0);
  });

  it('generates deterministic IDs (page:{fullPath})', async () => {
    const result1 = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });
    const result2 = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    const ids1 = result1.nodes.map((n) => n.id).sort();
    const ids2 = result2.nodes.map((n) => n.id).sort();
    expect(ids1).toEqual(ids2);
  });

  it('reports correct stats', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    expect(result.stats.filesScanned).toBe(1);
    expect(result.stats.nodesFound).toBe(result.nodes.length);
    expect(result.stats.edgesFound).toBe(0);
    expect(result.stats.errors).toBe(0);
    expect(result.stats.durationMs).toBeGreaterThanOrEqual(0);
  });

  it('returns correct scanner metadata', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    expect(result.version).toBe(1);
    expect(result.scanner.id).toBe('react-router');
    expect(result.scanner.name).toBe('React Router Page Scanner');
    expect(result.scanner.version).toBe('0.1.0');
  });

  it('returns empty edges array', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx'],
    });

    expect(result.edges).toEqual([]);
  });

  it('handles multiple route files', async () => {
    const result = await parseReactRouterPages(FIXTURES_DIR, {
      routeFiles: ['App.tsx', 'Empty.tsx'],
    });

    expect(result.stats.filesScanned).toBe(2);
    expect(result.nodes.length).toBeGreaterThan(0);
  });
});
