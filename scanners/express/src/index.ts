#!/usr/bin/env node
import { readFileSync } from 'fs';
import { parseExpressRoutes } from './parser.js';
import type { ScanInput } from './types.js';

const raw = readFileSync('/dev/stdin', 'utf8');
const input: ScanInput = JSON.parse(raw);
const result = await parseExpressRoutes(input.projectRoot, {
  ...input.options,
  ignorePaths: input.ignorePaths,
});
process.stdout.write(JSON.stringify(result));
