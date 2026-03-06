import { describe, it, expect } from 'vitest';
import { execFileSync } from 'child_process';
import { join } from 'path';
import type { ScanOutput } from '../src/types.js';

const ROOT = join(import.meta.dirname, '..');
const FIXTURES_DIR = join(import.meta.dirname, 'fixtures');

describe('CLI stdin/stdout integration', () => {
  it('reads ScanInput from stdin and writes ScanOutput to stdout', () => {
    const input = JSON.stringify({
      version: 1,
      projectRoot: FIXTURES_DIR,
      options: {
        contractGlobs: ['contracts/auth.ts'],
      },
    });

    const stdout = execFileSync(
      'npx',
      ['tsx', join(ROOT, 'src', 'index.ts')],
      {
        input,
        encoding: 'utf8',
        cwd: ROOT,
      }
    );

    const result: ScanOutput = JSON.parse(stdout);

    expect(result.version).toBe(1);
    expect(result.scanner.id).toBe('orpc');
    expect(result.scanner.name).toBe('oRPC Route Scanner');
    expect(result.scanner.version).toBe('0.1.0');
    expect(result.nodes).toHaveLength(1);
    expect(result.nodes[0].id).toBe('route:POST-/auth/register');
    expect(result.edges).toEqual([]);
    expect(result.warnings).toEqual([]);
    expect(result.stats.filesScanned).toBe(1);
    expect(result.stats.nodesFound).toBe(1);
    expect(result.stats.edgesFound).toBe(0);
    expect(result.stats.errors).toBe(0);
    expect(result.stats.durationMs).toBeGreaterThanOrEqual(0);
  });

  it('handles multiple files via globs', () => {
    const input = JSON.stringify({
      version: 1,
      projectRoot: FIXTURES_DIR,
      options: {
        contractGlobs: ['contracts/**/*.ts'],
      },
    });

    const stdout = execFileSync(
      'npx',
      ['tsx', join(ROOT, 'src', 'index.ts')],
      {
        input,
        encoding: 'utf8',
        cwd: ROOT,
      }
    );

    const result: ScanOutput = JSON.parse(stdout);
    expect(result.nodes).toHaveLength(6);
    expect(result.stats.filesScanned).toBe(3);
  });
});
